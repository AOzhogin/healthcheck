# Live demo in minikube (VictoriaMetrics Operator + Grafana)

Brings up the example service in minikube, scrapes it with the VM Operator stack, and opens
Grafana to view the [Healthcheck dashboard](../grafana/healthcheck-dashboard.json) with real data.

> ⚠️ Requires `minikube start` and pulling the VM Operator / Grafana images and helm charts —
> i.e. **network access**. If image/chart pulls fail, use the [local-Docker fallback](#fallback-no-cluster).

All commands run from the repo root. Everything lands in the `healthcheck` namespace.

## 1. Start the cluster

```bash
minikube start --driver=docker
kubectl get nodes
kubectl create namespace healthcheck
```

## 2. Build the example image into minikube (no base-image pull)

```bash
GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o observability/demo/app ./example/metrics
minikube image build -t healthcheck-example:dev observability/demo
```

## 3. Install the VM Operator

```bash
helm repo add vm https://victoriametrics.github.io/helm-charts/
helm repo update
helm install vmoperator vm/victoria-metrics-operator -n healthcheck
```

## 4. Deploy the example + VM stack + scrape/rules

```bash
kubectl apply -n healthcheck -f observability/demo/deployment.yaml
kubectl apply -n healthcheck -f observability/demo/vmsingle.yaml
kubectl apply -n healthcheck -f observability/demo/vmagent.yaml
# VMServiceScrape + VMRule + the Grafana dashboard ConfigMap (kustomize-generated):
kubectl apply -n healthcheck -k observability/
```

## 5. Install Grafana (datasource + dashboard auto-provisioned)

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm install grafana grafana/grafana -n healthcheck -f observability/demo/grafana-values.yaml
```

## 6. Open Grafana

```bash
kubectl port-forward -n healthcheck svc/grafana 3000:80
# user: admin   password: admin (from grafana-values.yaml)
```

Open <http://localhost:3000> → **Dashboards → Healthcheck**. The *Health checks* row populates
from the 3 example checks (`db`, `redis`, `oracle11g`); the *Runtime* row shows `process_*`
(the example enables the process collector). `go_*` panels stay empty unless the service is
built with `goCollector=true` in `WithMetrics(...)`.

## 7. Teardown

```bash
helm uninstall grafana vmoperator -n healthcheck
kubectl delete -n healthcheck -k observability/
kubectl delete -n healthcheck -f observability/demo/
minikube stop
```

## Fallback (no cluster)

If k8s image pulls fail, you can still see the dashboard with plain Docker + the running example:

```bash
GOWORK=off go run ./example/metrics            # serves :8080 /metrics
docker run --rm -p 8428:8428 victoriametrics/victoria-metrics \
  -promscrape.config=/dev/stdin <<'EOF'
scrape_configs:
  - job_name: healthcheck
    static_configs: [{ targets: ['host.docker.internal:8080'] }]
EOF
docker run --rm -p 3000:3000 grafana/grafana
# In Grafana: add a Prometheus datasource http://host.docker.internal:8428,
# then import ../grafana/healthcheck-dashboard.json.
```
