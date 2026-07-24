# selector-presence-exporter

An exporter that extracts the metric selectors from Prometheus alert rules and, on every scrape, checks whether each one still resolves on a live server that serves the Prometheus query API, exposing present / absent as gauges.

## Why this exists

The most dangerous monitoring failure is an alert that should fire but stays silent. A false alarm gets noticed and fixed. Silence produces nothing to notice.

When an alert expression returns no series, both Prometheus and vmalert read that as "everything is fine" and do not fire. There is no error and no failed-evaluation counter, because an empty result is not an error. See the [Prometheus alerting rules docs](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/) and [PromLabs on missing time series](https://promlabs.com/blog/2023/09/13/dealing-with-missing-time-series-in-prometheus/), which notes this is especially dangerous for "too-low" alerts that watch for a value dropping.

The quietest moment for this to happen is a version upgrade of a metrics component, when a metric is renamed or removed deep in a third-party changelog. node_exporter 0.16.0 renamed many metrics to follow naming conventions, breaking existing dashboards and alerts (see the [0.16 upgrade guide](https://github.com/prometheus/node_exporter/blob/master/docs/V0_16_UPGRADE_GUIDE.md) and [issue #830](https://github.com/prometheus/node_exporter/issues/830)). A renamed selector does not error. It silently returns empty, and the alert quietly stops working.

Both [Cloudflare](https://blog.cloudflare.com/monitoring-our-monitoring/) and [VictoriaMetrics](https://victoriametrics.com/blog/never-firing-alerts/) describe this exact silent failure from their own operations.

## How existing tools fare

- **[pint](https://cloudflare.github.io/pint/checks/promql/series.html)** is the closest existing tool. Its `promql/series` check queries a live Prometheus and warns when a rule uses a metric that is not currently present. But it is tied to Prometheus and cannot be used against other servers that serve the same Prometheus-compatible query API.
- **vmalert's built-in metric.** Since v1.91.0, vmalert exposes `vmalert_alerting_rules_last_evaluation_series_fetched`, and `max(...) by(group, alertname) == 0` detects a rule whose whole expression fetched nothing (see the [vmalert docs](https://docs.victoriametrics.com/victoriametrics/vmalert/)). That is per rule and over the full expression, so a rule where one selector disappears while another still returns series is not flagged. It is also VictoriaMetrics-specific and version-gated.
- **[promtool unit tests](https://prometheus.io/docs/prometheus/latest/configuration/unit_testing_rules/)** verify rule logic against handwritten data but never ask a live server whether a metric disappeared. `absent()` can be hand-written but requires enumerating in advance which selectors to watch.

The gap is a per-selector, live existence check that works over the shared query API (so on any Prometheus-compatible server, not just Prometheus) and leaves the verdict outside the tool.

## Approach

Extract every selector from the alert rules with the Prometheus PromQL parser, keeping the label matchers and not just the metric name. Query each selector against the server's instant query API. Expose present / absent as a gauge per (group, alert, selector). Hold no verdict, and defer judgment to PromQL over the gauge history.

The shape follows from the gap above:

- **Per selector**, so a partial disappearance that a per-rule signal misses is caught.
- **Shared query API only**, so it works against any server that serves the Prometheus-compatible query API, not just Prometheus.
- **Verdict outside the tool**, so normal-absence handling lives in PromQL you control rather than baked in.

One instance validates one alert rule set against one server: the server those rules are evaluated against, which is the only one that holds every metric they reference. A single Prometheus in a sharded fleet does not hold the whole set, so the server to point at is the aggregation or query point the rules are evaluated against, for example the VictoriaMetrics behind a ruler. For several independent (rule set, server) pairs, run several instances.

## How it works

On every scrape (collect-on-scrape), it:

1. reads the rule files and extracts every selector from the `alert:` exprs, keeping the matchers and ignoring `record:` rules,
2. removes duplicate selectors,
3. sends one instant query per selector to the server,
4. exposes two gauges:
   - `selector_presence{group, alert, selector}` is 1 when the instant query is non-empty, 0 when empty,
   - `selector_presence_endpoint_up` is 1 when the server could be queried, 0 when it could not.

A selector used by several alerts is queried once and fanned out to each alert, so the output keeps the (group, alert, selector) granularity. A failed query emits no present for its selectors and sets `endpoint_up=0`, so a stopped server is not read as absence.

The scrape interval is the only timer. The collection has no interval, cache, or staleness of its own, and its duration is left to the `scrape_interval` / `scrape_timeout` of a scrape job dedicated to this exporter.

## Usage

Pass rule files as arguments and the server to query with `--endpoint`, the full instant query URL.

```
selector-presence-exporter \
  --endpoint http://localhost:8428/api/v1/query \
  --listen :9099 \
  rules/*.yml
```

The `/metrics` output looks like:

```
selector_presence{alert="HighErrorRatio",group="example",selector="http_requests_total{status=~\"5..\"}"} 0
selector_presence_endpoint_up 1
```

`present` (1) means the instant query returned a series. When the server cannot be queried, `selector_presence_endpoint_up` becomes 0 and no present is emitted, so a stopped server is not mistaken for absence.

Turning the fact into an alert is left to the consumer. For example, to fire only on a selector that was present in the past but is absent now:

```promql
# Example consumer alert: present in the past but absent now, while the server is reachable.
selector_presence == 0
  and max_over_time(selector_presence[7d]) == 1
  and on() selector_presence_endpoint_up == 1
```

A selector that is legitimately empty in normal operation keeps `max_over_time` at 0, so it never fires.

## Design

- **The verdict stays outside the tool.** present / absent is emitted as a continuous gauge, so a consumer can tell a normal absence apart from a real disappearance in PromQL over the gauge history (the example above). The exporter therefore implements no lookback of its own and sticks to observing.
- **The endpoint URL is used as-is.** Only the `query` parameter is added, so no API path is baked into the tool. Any server, tenant path (such as a VictoriaMetrics `/select/<id>/prometheus/api/v1/query`), or reverse-proxy layout works without per-server logic. The cost is that the caller provides the full path and the tool cannot check it points at the instant query endpoint.
- **Queries run sequentially, each with a per-query timeout.** No concurrency is added. Disappearance detection can be slow, and a single in-flight query at a time is gentlest on a shared server. The per-query timeout keeps one hung query from stalling the rest and letting the whole scrape die by timeout with the period's data wiped out, and that one query is treated as an error so the others survive. Collection duration is absorbed by the scrape config, not by a scheduler inside the tool. If a single collection outgrows a sensible `scrape_timeout`, first raise the timeout, then move to concurrent queries, and only then to a separate interval plus cache. Concurrency is a future escape hatch for when the selector count grows by an order of magnitude.
- **A custom client_golang Collector.** Because the label value is a selector string with quotes and regexps that need escaping, a custom Collector is used instead of writing the text format by hand. It also rebuilds the output on every scrape, so selectors removed from the rules leave no stale series. PromQL parsing is delegated to prometheus/prometheus with a shared, effectively stateless parser.

## Limitations

- **Normal absence cannot be told apart automatically.** A selector that is legitimately empty in normal operation (dynamic cardinality such as `status=~"5.."`) also returns `present=0`, and the tool does not distinguish that from a real disappearance. This is inherent to checking existence by query, and [pint acknowledges the same limitation](https://blog.cloudflare.com/monitoring-our-monitoring/). Handle it with the `max_over_time` form above or an allow-list on the alert side.
- **This is presence, not correctness.** It confirms that a selector returns a series but does not verify the thresholds or logic of the alert expression. That is the domain of unit tests.

## License

This project is licensed under the [MIT License](LICENSE).
