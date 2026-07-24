package main

import (
	"errors"
	"flag"
)

type config struct {
	ruleFiles   []string
	endpoint    string
	listen      string
	showVersion bool
}

func parseArgs(args []string) (config, error) {
	fs := flag.NewFlagSet("selector-presence-exporter", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "instant query URL of the server the rules are evaluated against")
	listen := fs.String("listen", ":9099", "listen address")
	showVersion := fs.Bool("version", false, "print version information and exit")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if *showVersion {
		return config{showVersion: true}, nil
	}

	ruleFiles := fs.Args()
	if len(ruleFiles) == 0 {
		return config{}, errors.New("specify rule files as arguments")
	}
	if *endpoint == "" {
		return config{}, errors.New("specify --endpoint with the instant query URL")
	}
	return config{ruleFiles: ruleFiles, endpoint: *endpoint, listen: *listen}, nil
}
