package main

import (
	"sort"
	"strings"
	"testing"
)

// Every VectorSelector in an expr is extracted with its matchers, and record rules are ignored.
func TestExtractSelectors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string // "group|alert|selector"
	}{
		{
			name: "extracts every selector in the expr with matchers",
			yaml: `
groups:
  - name: web
    rules:
      - alert: HighErrorRatio
        expr: sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) > 0.05
`,
			want: []string{
				`web|HighErrorRatio|http_requests_total`,
				`web|HighErrorRatio|http_requests_total{status=~"5.."}`,
			},
		},
		{
			name: "ignores record and only handles alert",
			yaml: `
groups:
  - name: g
    rules:
      - record: job:up:sum
        expr: sum(up) by (job)
      - alert: AppDown
        expr: up{job="app"} == 0
`,
			want: []string{`g|AppDown|up{job="app"}`},
		},
		{
			name: "multiple groups / inside range and function / inside absent",
			yaml: `
groups:
  - name: a
    rules:
      - alert: A
        expr: node_up == 0
  - name: b
    rules:
      - alert: B
        expr: absent(foo{bar="baz"})
`,
			want: []string{`a|A|node_up`, `b|B|foo{bar="baz"}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sels, err := extractSelectors([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("extractSelectors: %v", err)
			}
			got := sortedTriples(sels)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("selectors mismatch\n got: %v\nwant: %v", got, want)
			}
		})
	}
}

func sortedTriples(sels []ruleSelector) []string {
	out := make([]string, len(sels))
	for i, s := range sels {
		out[i] = s.group + "|" + s.alert + "|" + s.selector
	}
	sort.Strings(out)
	return out
}

// A broken expr in one alert does not drop the selectors of other alerts.
func TestExtractSelectorsBadExprIsSkipped(t *testing.T) {
	y := `
groups:
  - name: g
    rules:
      - alert: Broken
        expr: this is not ) valid promql
      - alert: Good
        expr: up{job="app"} == 0
`
	sels, err := extractSelectors([]byte(y))
	if err == nil {
		t.Error("err is nil despite a broken expr")
	}
	if len(sels) != 1 || sels[0].selector != `up{job="app"}` {
		t.Errorf("the good selector did not survive: %v", sels)
	}
}
