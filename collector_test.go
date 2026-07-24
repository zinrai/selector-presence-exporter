package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// present and absent map to gauge values 1 and 0, a selector shared by several alerts fans out to
// each, and selector strings are escaped on exposition.
func TestCollectPresentAbsentFanout(t *testing.T) {
	rules := []ruleSelector{
		{"consul", "NoLeader", "consul_raft_leader"},
		{"consul", "MultiLeader", "consul_raft_leader"},
		{"phy", "Stock", `phy_stock{status="free"}`},
	}
	c := &collector{
		selectors: func() []ruleSelector { return rules },
		query: func(sel string) (bool, error) {
			return sel == "consul_raft_leader", nil
		},
	}
	want := `
# HELP selector_presence Whether the alert rule selector resolves on the server (1=present, 0=absent).
# TYPE selector_presence gauge
selector_presence{alert="MultiLeader",group="consul",selector="consul_raft_leader"} 1
selector_presence{alert="NoLeader",group="consul",selector="consul_raft_leader"} 1
selector_presence{alert="Stock",group="phy",selector="phy_stock{status=\"free\"}"} 0
# HELP selector_presence_endpoint_up Whether the server could be queried at scrape time (1=ok, 0=error).
# TYPE selector_presence_endpoint_up gauge
selector_presence_endpoint_up 1
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Error(err)
	}
}

// A failed query emits no selector_presence series and sets endpoint_up=0.
func TestCollectErrorIsNotAbsent(t *testing.T) {
	rules := []ruleSelector{{"g", "A", "up"}}
	c := &collector{
		selectors: func() []ruleSelector { return rules },
		query: func(_ string) (bool, error) {
			return false, errors.New("connection refused")
		},
	}
	want := `
# HELP selector_presence_endpoint_up Whether the server could be queried at scrape time (1=ok, 0=error).
# TYPE selector_presence_endpoint_up gauge
selector_presence_endpoint_up 0
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want), "selector_presence", "selector_presence_endpoint_up"); err != nil {
		t.Error(err)
	}
}
