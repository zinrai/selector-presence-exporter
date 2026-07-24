package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/prometheus/prometheus/promql/parser"
	"go.yaml.in/yaml/v3"
)

type ruleSelector struct {
	group    string
	alert    string
	selector string
}

type ruleFile struct {
	Groups []ruleGroup `yaml:"groups"`
}

type ruleGroup struct {
	Name  string     `yaml:"name"`
	Rules []ruleSpec `yaml:"rules"`
}

type ruleSpec struct {
	Alert string `yaml:"alert"`
	Expr  string `yaml:"expr"`
}

// Shared rather than created per call or per scrape: it only holds Options, and ParseExpr creates and
// closes a low-level parser internally on each call, so sharing is safe even under concurrent scrapes.
var exprParser = parser.NewParser(parser.Options{})

func loadSelectors(ruleFiles []string) []ruleSelector {
	var all []ruleSelector
	for _, path := range ruleFiles {
		all = append(all, fileSelectors(path)...)
	}
	return dedupe(all)
}

// fileSelectors logs a parse error but still returns the selectors that parsed, so one typo does not
// wipe a whole file's selectors.
func fileSelectors(path string) []ruleSelector {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("rule file %s: %v", path, err)
		return nil
	}
	sels, err := extractSelectors(data)
	if err != nil {
		log.Printf("rule file %s: %v", path, err)
	}
	return sels
}

func dedupe(in []ruleSelector) []ruleSelector {
	seen := map[ruleSelector]struct{}{}
	var out []ruleSelector
	for _, rs := range in {
		if _, ok := seen[rs]; ok {
			continue
		}
		seen[rs] = struct{}{}
		out = append(out, rs)
	}
	return out
}

// extractSelectors takes bytes rather than a path so tests need no fixture. A YAML decode failure is
// fatal because it leaves nothing usable. A single expr that fails to parse is folded into the error
// but does not drop the selectors that parsed.
func extractSelectors(data []byte) ([]ruleSelector, error) {
	var rf ruleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, err
	}
	var out []ruleSelector
	var errs []error
	for _, g := range rf.Groups {
		sels, gerrs := groupSelectors(g)
		out = append(out, sels...)
		errs = append(errs, gerrs...)
	}
	return out, errors.Join(errs...)
}

func groupSelectors(g ruleGroup) ([]ruleSelector, []error) {
	var out []ruleSelector
	var errs []error
	for _, r := range g.Rules {
		if r.Alert == "" {
			continue // record and other non-alert rules are out of scope
		}
		sels, err := selectorsOf(r.Expr)
		if err != nil {
			errs = append(errs, fmt.Errorf("group %q alert %q: %w", g.Name, r.Alert, err))
			continue
		}
		out = append(out, tagSelectors(g.Name, r.Alert, sels)...)
	}
	return out, errs
}

func tagSelectors(group, alert string, selectors []string) []ruleSelector {
	out := make([]ruleSelector, len(selectors))
	for i, sel := range selectors {
		out[i] = ruleSelector{group, alert, sel}
	}
	return out
}

// selectorsOf keeps the matchers, not just the metric name, because the unit that must resolve is the
// selector: a metric can survive an upgrade while a matcher stops matching. Range vectors collapse to
// the inner instant form because existence only needs the instant form. PromQL is parsed by
// prometheus/prometheus rather than by hand.
func selectorsOf(expr string) ([]string, error) {
	ast, err := exprParser.ParseExpr(expr)
	if err != nil {
		return nil, err
	}
	var sels []string
	parser.Inspect(ast, func(node parser.Node, _ []parser.Node) error {
		if vs, ok := node.(*parser.VectorSelector); ok {
			sels = append(sels, vs.String())
		}
		return nil
	})
	return sels, nil
}
