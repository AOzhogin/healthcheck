# Observability for `github.com/AOzhogin/healthcheck`

Ready-to-use Grafana dashboard, Prometheus/vmalert alerting rules, and VictoriaMetrics Operator
manifests for the metrics this library exposes. Pick the plain-files path or the VM Operator path.

```
observability/
├── grafana/
│   ├── healthcheck-dashboard.json         # per-service dashboard (Health checks + Runtime)
│   └── healthcheck-fleet-dashboard.json   # fleet overview across all services (drill-down)
├── prometheus/
│   ├── healthcheck.rules.yaml             # alerting rules (Prometheus AND vmalert)
│   ├── healthcheck.slo.rules.yaml         # SLO recording rules + burn-rate alerts
│   └── alertmanager.example.yaml          # sample routing/receivers
├── victoriametrics-operator/
│   ├── vmrule.yaml                        # VMRule (same alerts)
│   ├── vmrule-slo.yaml                    # VMRule (SLO recording + burn-rate)
│   └── vmservicescrape.yaml              # VMServiceScrape for /metrics
├── k8s/
│   └── probes-example.yaml               # how to wire k8s probes to /live,/ready,/startup
├── kustomization.yaml                     # kubectl apply -k observability/ (rules + scrape + dashboard CM)
└── demo/                                  # minikube live demo (see demo/README.md)
```

## Metrics reference

The library serves these on `/metrics` (enable with `WithMetrics(...)`):

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `healthcheck_metrics_up` | gauge | `check` | Check availability: `1` = ok, `0` = error |
| `healthcheck_metrics_duration_seconds` | histogram | `check` | Check execution time (sec); default buckets 0.005…10 (`_bucket`/`_sum`/`_count`) |
| `go_build_info`, `go_*` | gauge/… | — | Go runtime (only if `goCollector`/`buildInfo` enabled) |
| `process_*` | gauge/counter | — | Process CPU/RSS/FDs (only if `processCollector` enabled) |

> **`up` vs `healthcheck_metrics_up`:** Prometheus' built-in `up` means *"the target was scraped
> successfully"*; `healthcheck_metrics_up` means *"the check passed"*. The dashboards/alerts here
> use the latter; the commented `HealthcheckTargetDown` rule uses the former.

Dashboards and rules template on the `job`, `instance`, and `check` labels — `job`/`instance`
come from your scrape config.

## Grafana dashboards

Two dashboards:

- **Healthcheck — Fleet** (`healthcheck-fleet`, `grafana/healthcheck-fleet-dashboard.json`) —
  one view across **all** services using the lib (by `job`): counts of services/checks/failures,
  a table of every check with status, slowest checks (p99 top-20), and failures over time.
  Click a row to **drill down** into the per-service dashboard for that `job`.
- **Healthcheck — Service** (`healthcheck-overview`, `grafana/healthcheck-dashboard.json`) —
  the per-service detail: **Health checks** row (status, failing count, availability, duration
  quantiles + heatmap, execution rate) and a **Runtime (Go / process)** row. Variables:
  `$datasource`, `$job`, `$instance`, `$check`.

Install either way:

- **Import (any Grafana):** Dashboards → New → Import → upload the JSON, pick your datasource.
- **Provision via sidecar (k8s):** the root `kustomization.yaml` generates a ConfigMap labeled
  `grafana_dashboard: "1"` containing both dashboards, which the Grafana sidecar auto-imports.

## Alerting rules

`prometheus/healthcheck.rules.yaml` (group `healthcheck`): `HealthcheckDown` (critical),
`HealthcheckFlapping` (warning), `HealthcheckSlow` (p99 > 1s, warning), `HealthcheckMetricsAbsent`
(critical), and a commented `HealthcheckTargetDown`. Tune the `for` windows, the p99 threshold, and
the job regex to your SLOs.

- **Prometheus:** add the file to `rule_files:`; point Alertmanager at `alertmanager.example.yaml`.
- **vmalert:** `vmalert -rule=observability/prometheus/healthcheck.rules.yaml -datasource.url=...`.
- **VM Operator:** the same rules ship as `victoriametrics-operator/vmrule.yaml`.

### SLO (error-budget) rules

`prometheus/healthcheck.slo.rules.yaml` (VM Operator: `vmrule-slo.yaml`) adds recording rules
(per-check error ratio over 5m/30m/1h/6h and a precomputed p99) plus multi-window burn-rate alerts
`HealthcheckErrorBudgetBurnFast`/`Slow` for a **99.9% availability SLO**. Change the target by editing
the `<burn-factor> * 0.001` thresholds. Use these instead of (or alongside) the instantaneous
`HealthcheckDown` when you track an availability objective.

## Kubernetes probes

`k8s/probes-example.yaml` shows how to map `startupProbe`/`livenessProbe`/`readinessProbe` to the
endpoints `StartHTTPServer` serves (`/startup`, `/live`, `/ready`). Copy the probe stanzas into your
own Deployment/Helm — the library does **not** ship your Deployment/Service, GitOps provisioning, or
Alertmanager routing; those are service/org-specific.

## VictoriaMetrics Operator (k8s)

```bash
# set the Service selector/port in victoriametrics-operator/vmservicescrape.yaml first, then:
kubectl apply -k observability/
```

Applies the `VMServiceScrape` (scrape your service's `/metrics`), the `VMRule` (alerts), and the
generated Grafana dashboard ConfigMap. CRDs require the VM Operator to be installed in the cluster.

## Live demo

See [`demo/README.md`](demo/README.md) to bring the whole stack up in minikube and view the
dashboard with real data.
