package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Injected at build time by goreleaser via -ldflags -X.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "selector-presence-exporter:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}
	if cfg.showVersion {
		fmt.Printf("selector-presence-exporter %s\ncommit: %s\ndate: %s\n", version, commit, date)
		return nil
	}

	client := &http.Client{Timeout: queryTimeout}
	c := &collector{
		selectors: func() []ruleSelector { return loadSelectors(cfg.ruleFiles) },
		query:     httpQuerier(client, cfg.endpoint),
	}
	prometheus.MustRegister(c)

	http.Handle("/metrics", promhttp.Handler())
	log.Printf("listening on %s", cfg.listen)
	return http.ListenAndServe(cfg.listen, nil)
}
