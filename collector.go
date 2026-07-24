package main

import (
	"log"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	presentDesc = prometheus.NewDesc(
		"selector_presence",
		"Whether the alert rule selector resolves on the server (1=present, 0=absent).",
		[]string{"group", "alert", "selector"}, nil)
	endpointUpDesc = prometheus.NewDesc(
		"selector_presence_endpoint_up",
		"Whether the server could be queried at scrape time (1=ok, 0=error).",
		nil, nil)
)

// A custom Collector, not a set of pre-registered gauges, so that selectors removed from the rules
// leave no stale series: each scrape rebuilds the output from a fresh read.
type collector struct {
	selectors func() []ruleSelector
	query     querier
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- presentDesc
	ch <- endpointUpDesc
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	samples, up := observe(c.selectors(), c.query)
	for _, s := range samples {
		ch <- prometheus.MustNewConstMetric(presentDesc, prometheus.GaugeValue,
			boolToFloat(s.present), s.group, s.alert, s.selector)
	}
	ch <- prometheus.MustNewConstMetric(endpointUpDesc, prometheus.GaugeValue, boolToFloat(up))
}

type sample struct {
	group, alert, selector string
	present                bool
}

// observe takes a querier rather than calling the server directly so a fake can drive it without a
// network.
func observe(rules []ruleSelector, query querier) ([]sample, bool) {
	present, up := probe(uniqueSelectors(rules), query)
	return fanOut(rules, present), up
}

func probe(selectors []string, query querier) (map[string]bool, bool) {
	present := map[string]bool{}
	var firstErr error
	for _, sel := range selectors {
		ok, err := query(sel)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue // not present=0: a failed query must not look like absent
		}
		present[sel] = ok
	}
	if firstErr != nil {
		log.Printf("query error: %v", firstErr)
	}
	return present, firstErr == nil
}

func fanOut(rules []ruleSelector, present map[string]bool) []sample {
	var samples []sample
	for _, r := range rules {
		v, ok := present[r.selector]
		if !ok {
			continue // this selector's query failed
		}
		samples = append(samples, sample{r.group, r.alert, r.selector, v})
	}
	return samples
}

// Deduplicated so a selector shared by several alerts is queried once, not once per alert.
func uniqueSelectors(rules []ruleSelector) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, r := range rules {
		if _, ok := seen[r.selector]; ok {
			continue
		}
		seen[r.selector] = struct{}{}
		out = append(out, r.selector)
	}
	return out
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
