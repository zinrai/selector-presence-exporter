package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Bounds each query on its own rather than the collection as a whole: since queries run sequentially,
// a single hung one would otherwise stall everything after it and let the scrape die by timeout with
// the period's data wiped out.
const queryTimeout = 5 * time.Second

// An error means the query itself failed, which is distinct from absent (false, nil). This type is
// the injection point that lets the collector be tested without a real server.
type querier func(selector string) (present bool, err error)

func httpQuerier(client *http.Client, queryURL string) querier {
	return func(selector string) (bool, error) {
		return queryPresent(client, queryURL, selector)
	}
}

// The endpoint is the full instant-query URL, used as-is with only the query parameter added, so no
// API path is hardcoded and any server, tenant path, or proxy layout works without per-server logic.
func queryPresent(client *http.Client, queryURL, selector string) (bool, error) {
	u, err := url.Parse(queryURL)
	if err != nil {
		return false, err
	}
	q := u.Query()
	q.Set("query", selector)
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	return parseQueryResult(body)
}

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []json.RawMessage `json:"result"`
	} `json:"data"`
}

// Split from the HTTP call so the present/absent decision can be tested with inline JSON, without a
// network.
func parseQueryResult(body []byte) (bool, error) {
	var qr queryResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return false, err
	}
	if qr.Status != "success" {
		return false, fmt.Errorf("query status %q", qr.Status)
	}
	return len(qr.Data.Result) > 0, nil
}
