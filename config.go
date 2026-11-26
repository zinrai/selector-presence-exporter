package main

import (
	"errors"
	"flag"
	"fmt"
)

type config struct {
	ruleFiles   string
	endpoint    string
	listen      string
	showVersion bool
}

func parseArgs(args []string) (config, error) {
	fs := flag.NewFlagSet("selector-presence-exporter", flag.ContinueOnError)
	ruleFiles := fs.String("rule-files", "", "glob pattern of the rule files to read")
	endpoint := fs.String("endpoint", "", "instant query URL of the server the rules are evaluated against")
	listen := fs.String("listen", ":9099", "listen address")
	showVersion := fs.Bool("version", false, "print version information and exit")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if *showVersion {
		return config{showVersion: true}, nil
	}

	if *ruleFiles == "" {
		return config{}, errors.New("specify --rule-files with a glob pattern of the rule files")
	}
	// Not ignored: an unquoted pattern is expanded by the shell, which leaves --rule-files with the
	// first file only and the rest here, and those would go unchecked without a word.
	if fs.NArg() > 0 {
		return config{}, fmt.Errorf("unexpected argument %q: quote the --rule-files pattern", fs.Arg(0))
	}
	if *endpoint == "" {
		return config{}, errors.New("specify --endpoint with the instant query URL")
	}
	return config{ruleFiles: *ruleFiles, endpoint: *endpoint, listen: *listen}, nil
}
