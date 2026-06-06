### Health check

Simple health check library with more options

### Main fiatures

1. Health check for k8s style
2. Full answer customization
3. Simple metrics support

### Installation

```
go get -u github.com/AOzhogin/healthcheck
```

### Work logic

1. On request /health (or any another) start checking all checks (or get cached result)
2. When all checks is OK - you will get http status code 200 and empty body
3. When some check is got Error - you will get http status code 503 and empty body
4. If you want to get more info about checks - you can request with ?body=true parametr

### How to use

```go
// initialize
hc := healthcheck.New(
    healthcheck.WithHTTPAddress(":8081"), // custom HTTP listen address
    healthcheck.WithMetrics(false, false, false),
    healthcheck.WithBackCheck(5*time.Second),
)

// add health check
hc.Add("db", // health check name
       "db.company:1521", // notes
       func(ctx context.Context) error {
           // your check here
           return nil
       },
)

// start HTTP server (handlers registered automatically)
if err := hc.StartHTTPServer(); err != nil {
    panic(err)
}
```

### Request with response

Example:

```bash
curl -X GET http://localhost:8080/health?body=true
```

Response:

```json
{
  "status": "ok",
  "checks": {
    "db": {
      "time": "2024-01-06T22:13:45.77529977+03:00",
      "status": "ok",
      "exec": 0.382560396,
      "notes": "db.company:1521"
    },
    "oracle11g": {
      "time": "2024-01-06T22:13:47.566212962+03:00",
      "status": "ok",
      "exec": 0.885256101,
      "notes": "oracle.company"
    },
    "redis": {
      "time": "2024-01-06T22:13:46.680934092+03:00",
      "status": "ok",
      "exec": 0.905602405,
      "notes": "redis.company:9056"
    }
  }
}
```

### Special info about lib

Lib already constain constants for handlers:

```go
HandlerHealthCheck = "/health"
HandlerLive        = "/live"
HandlerReady       = "/ready"
HandlerStartup     = "/startup"
```

#### Custom k8s probe handlers

`StartHTTPServer` serves `/live`, `/ready`, and `/startup`. By default each runs the
registered checks (like `/health`). Assign your own handler per probe with the setters
(pass `nil` to fall back to the default). Call them **before** `StartHTTPServer` — handlers
are read once at route registration.

```go
hc := healthcheck.New(healthcheck.WithHTTPAddress(":8080"))

// Liveness should be cheap and dependency-free (a DB blip must not restart the pod).
hc.SetHandlerLive(func(w http.ResponseWriter, _ *http.Request) {
    w.WriteHeader(http.StatusOK)
})

// Readiness / startup can gate on dependencies or warmup state.
hc.SetHandlerReady(myReadinessHandler)
hc.SetHandlerStartup(myStartupHandler)

if err := hc.StartHTTPServer(); err != nil {
    panic(err)
}
```

#### Options

```go
WithSuccessStatus(status int)           // set HTTP status code for successful checks (default: 200)
WithErrorStatus(status int)             // set HTTP status code when any checker fails (default: 503)
WithTimeOut(timeout time.Duration)      // set global timeout for all checkers (default: 30s)
WithMetrics(buildInfo, goCollector, processCollector bool) // enable Prometheus metrics
WithMetricsRegistry(r *prometheus.Registry) // use external Prometheus registry for metrics
WithMetricsBuckets(buckets ...float64)  // custom duration histogram buckets (default: Prometheus DefBuckets)
WithBackCheck(interval time.Duration)   // run checks in background with specified interval
WithCheckStatusSuccess(status string)   // set string status for successful check (default: "ok")
WithCheckStatusError(status string)     // set string status for failed check (default: "error")
WithHTTPAddress(addr string)            // set HTTP listen address for StartHTTPServer (default: ":8080")
WithBasicAuth(user, pass string)        // enable HTTP Basic Auth on the served endpoints
WithMiddleware(mw func(http.Handler) http.Handler) // apply custom middleware to served endpoints
WithoutPProf()                          // disable the pprof (/debug/) endpoint (enabled by default)
WithLogger(logger *slog.Logger)         // log check state transitions (ok<->error); off when nil
WithTracer(t Tracer)                    // span per check; default no-op (see otelhc adapter)
```

### Logging & tracing

Both are opt-in via injected dependencies — the core has no logging framework or tracing
dependency baked in.

- **`WithLogger(*slog.Logger)`** logs a check's state transitions: error-level when a check starts
  failing, info-level on recovery (attributes: `check`, `error`, `duration`).
- **`WithTracer(Tracer)`** opens a span per check (`healthcheck/<name>`) and passes its context to the
  check func, so your check code nests its own spans. `Tracer`/`Span` are small interfaces in this
  package; the OpenTelemetry adapter lives in the separate `otelhc` module so OTel stays out of the
  core's dependencies:

  ```go
  import (
      "github.com/AOzhogin/healthcheck"
      "github.com/AOzhogin/healthcheck/pkg/otelhc"
      "go.opentelemetry.io/otel"
  )

  hc := healthcheck.New(
      healthcheck.WithTracer(otelhc.New(otel.GetTracerProvider())),
  )
  ```

Runnable examples: [`example/logging`](example/logging) (WithLogger + WithMetricsBuckets) and
[`pkg/otelhc/example`](pkg/otelhc/example) (WithTracer via the OTel adapter).

### Metrics

Prometheus metrics examples

#### main health check metrics example

```
# HELP healthcheck_metrics_up Check availability (1 = ok, 0 = error)
# TYPE healthcheck_metrics_up gauge
healthcheck_metrics_up{check="db"} 1
# HELP healthcheck_metrics_duration_seconds Check execution duration in seconds
# TYPE healthcheck_metrics_duration_seconds histogram
healthcheck_metrics_duration_seconds_bucket{check="db",le="0.005"} 0
healthcheck_metrics_duration_seconds_bucket{check="db",le="0.01"} 0
healthcheck_metrics_duration_seconds_bucket{check="db",le="0.025"} 0
healthcheck_metrics_duration_seconds_bucket{check="db",le="0.05"} 0
healthcheck_metrics_duration_seconds_bucket{check="db",le="0.1"} 1
healthcheck_metrics_duration_seconds_bucket{check="db",le="0.25"} 1
healthcheck_metrics_duration_seconds_bucket{check="db",le="0.5"} 1
healthcheck_metrics_duration_seconds_bucket{check="db",le="1"} 1
healthcheck_metrics_duration_seconds_bucket{check="db",le="2.5"} 1
healthcheck_metrics_duration_seconds_bucket{check="db",le="5"} 1
healthcheck_metrics_duration_seconds_bucket{check="db",le="10"} 1
healthcheck_metrics_duration_seconds_bucket{check="db",le="+Inf"} 1
healthcheck_metrics_duration_seconds_sum{check="db"} 0.094148065
healthcheck_metrics_duration_seconds_count{check="db"} 1
# HELP healthcheck_metrics_last_success_timestamp_seconds Unix timestamp of the last successful check
# TYPE healthcheck_metrics_last_success_timestamp_seconds gauge
healthcheck_metrics_last_success_timestamp_seconds{check="db"} 1.704573e+09
# HELP healthcheck_metrics_errors_total Total failed checks by reason
# TYPE healthcheck_metrics_errors_total counter
healthcheck_metrics_errors_total{check="db",reason="timeout"} 2
```

`last_success_timestamp_seconds` enables staleness alerting (`time() - ... > N`). `errors_total`
carries a low-cardinality `reason` label: `timeout` / `canceled` are derived from the context, and a
check may set its own reason by returning an error that implements `interface{ Reason() string }`;
everything else is `error`.

> Each registered check is one series distinguished by the `check` label, e.g.
> `healthcheck_metrics_up{check="db"}` and `healthcheck_metrics_up{check="redis"}`,
> instead of a separate metric name per check.

#### buildInfo example

```
# HELP go_build_info Build information about the main Go module.
# TYPE go_build_info gauge
go_build_info{checksum="",path="",version=""} 1
```

#### goCollector example

```
# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 0
go_gc_duration_seconds{quantile="0.25"} 0
go_gc_duration_seconds{quantile="0.5"} 0
go_gc_duration_seconds{quantile="0.75"} 0
go_gc_duration_seconds{quantile="1"} 0
go_gc_duration_seconds_sum 0
go_gc_duration_seconds_count 0
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 7
# HELP go_info Information about the Go environment.
# TYPE go_info gauge
go_info{version="go1.21.5"} 1
# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 275984
# HELP go_memstats_alloc_bytes_total Total number of bytes allocated, even if freed.
# TYPE go_memstats_alloc_bytes_total counter
go_memstats_alloc_bytes_total 275984
# HELP go_memstats_buck_hash_sys_bytes Number of bytes used by the profiling bucket hash table.
# TYPE go_memstats_buck_hash_sys_bytes gauge
go_memstats_buck_hash_sys_bytes 4238
# HELP go_memstats_frees_total Total number of frees.
# TYPE go_memstats_frees_total counter
go_memstats_frees_total 0
# HELP go_memstats_gc_sys_bytes Number of bytes used for garbage collection system metadata.
# TYPE go_memstats_gc_sys_bytes gauge
go_memstats_gc_sys_bytes 2.471448e+06
# HELP go_memstats_heap_alloc_bytes Number of heap bytes allocated and still in use.
# TYPE go_memstats_heap_alloc_bytes gauge
go_memstats_heap_alloc_bytes 275984
# HELP go_memstats_heap_idle_bytes Number of heap bytes waiting to be used.
# TYPE go_memstats_heap_idle_bytes gauge
go_memstats_heap_idle_bytes 2.236416e+06
# HELP go_memstats_heap_inuse_bytes Number of heap bytes that are in use.
# TYPE go_memstats_heap_inuse_bytes gauge
go_memstats_heap_inuse_bytes 1.564672e+06
# HELP go_memstats_heap_objects Number of allocated objects.
# TYPE go_memstats_heap_objects gauge
go_memstats_heap_objects 334
# HELP go_memstats_heap_released_bytes Number of heap bytes released to OS.
# TYPE go_memstats_heap_released_bytes gauge
go_memstats_heap_released_bytes 2.236416e+06
# HELP go_memstats_heap_sys_bytes Number of heap bytes obtained from system.
# TYPE go_memstats_heap_sys_bytes gauge
go_memstats_heap_sys_bytes 3.801088e+06
# HELP go_memstats_last_gc_time_seconds Number of seconds since 1970 of last garbage collection.
# TYPE go_memstats_last_gc_time_seconds gauge
go_memstats_last_gc_time_seconds 0
# HELP go_memstats_lookups_total Total number of pointer lookups.
# TYPE go_memstats_lookups_total counter
go_memstats_lookups_total 0
# HELP go_memstats_mallocs_total Total number of mallocs.
# TYPE go_memstats_mallocs_total counter
go_memstats_mallocs_total 334
# HELP go_memstats_mcache_inuse_bytes Number of bytes in use by mcache structures.
# TYPE go_memstats_mcache_inuse_bytes gauge
go_memstats_mcache_inuse_bytes 19200
# HELP go_memstats_mcache_sys_bytes Number of bytes used for mcache structures obtained from system.
# TYPE go_memstats_mcache_sys_bytes gauge
go_memstats_mcache_sys_bytes 31200
# HELP go_memstats_mspan_inuse_bytes Number of bytes in use by mspan structures.
# TYPE go_memstats_mspan_inuse_bytes gauge
go_memstats_mspan_inuse_bytes 36288
# HELP go_memstats_mspan_sys_bytes Number of bytes used for mspan structures obtained from system.
# TYPE go_memstats_mspan_sys_bytes gauge
go_memstats_mspan_sys_bytes 48888
# HELP go_memstats_next_gc_bytes Number of heap bytes when next garbage collection will take place.
# TYPE go_memstats_next_gc_bytes gauge
go_memstats_next_gc_bytes 4.194304e+06
# HELP go_memstats_other_sys_bytes Number of bytes used for other system allocations.
# TYPE go_memstats_other_sys_bytes gauge
go_memstats_other_sys_bytes 957586
# HELP go_memstats_stack_inuse_bytes Number of bytes in use by the stack allocator.
# TYPE go_memstats_stack_inuse_bytes gauge
go_memstats_stack_inuse_bytes 393216
# HELP go_memstats_stack_sys_bytes Number of bytes obtained from system for stack allocator.
# TYPE go_memstats_stack_sys_bytes gauge
go_memstats_stack_sys_bytes 393216
# HELP go_memstats_sys_bytes Number of bytes obtained from system.
# TYPE go_memstats_sys_bytes gauge
go_memstats_sys_bytes 7.707664e+06
# HELP go_threads Number of OS threads created.
# TYPE go_threads gauge
go_threads 7
```

#### processCollector example

```
# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 0
# HELP process_max_fds Maximum number of open file descriptors.
# TYPE process_max_fds gauge
process_max_fds 524288
# HELP process_open_fds Number of open file descriptors.
# TYPE process_open_fds gauge
process_open_fds 9
# HELP process_resident_memory_bytes Resident memory size in bytes.
# TYPE process_resident_memory_bytes gauge
process_resident_memory_bytes 1.6379904e+07
# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1.7045707849e+09
# HELP process_virtual_memory_bytes Virtual memory size in bytes.
# TYPE process_virtual_memory_bytes gauge
process_virtual_memory_bytes 1.645735936e+09
# HELP process_virtual_memory_max_bytes Maximum amount of virtual memory available in bytes.
# TYPE process_virtual_memory_max_bytes gauge
process_virtual_memory_max_bytes 1.8446744073709552e+19
```

</details>

</details>
