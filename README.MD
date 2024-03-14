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
    healthcheck.WithMetrics(false, false, false),
    healthcheck.WithBackCheck(5*time.Second),
)

// add health check
hc.Add("db", // health check name
       "db.company:1521", // notes
       func(ctx context.Context) error {
	// you check here
})

// initialize mux server
mux := http.NewServeMux()

// add handlers
mux.HandleFunc(healthcheck.HandlerHealthCheck, hc.HandlerHealth)
mux.HandleFunc(healthcheck.HandlerMetrics, hc.HandlerMetrics)

// start http server
http.ListenAndServe(":8080", mux)
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
HandlerReady       = "/read"
HandlerStartup     = "/startup"
```

#### Options

```go
WithSuccessStatus(status int) - set success status code

WithErrorStatus(status int) - set status code when any one checkers is failed

WithTimeOut(timeout time.Duration) - set global checkers time out

WithMetrics(buildInfo, goCollector, processCollector bool) - collect prometheus metrics

WithMetricsRegistry(r *prometheus.Registry) - collect prometheus metrics with external registry

WithBackCheck(interval time.Duration) - check in routine

WithCheckStatusSuccess(status string) - set string status when check is success, default "ok"

WithCheckStatusError(status string) - set string status when check is error, default "error"
```

### Metrics

With option "WithMetrics" you can use metrics handler for Prometheus
Option include configuration:

##### main health check metrics

gauge - when 0 is check have error and 1 when check is ok
histogram - with timeout execution

```
# HELP healthcheck_metrics_db Checking available db
# TYPE healthcheck_metrics_db gauge
healthcheck_metrics_db 1
# HELP healthcheck_metrics_db_dur Checking duration db
# TYPE healthcheck_metrics_db_dur histogram
healthcheck_metrics_db_dur_bucket{le="0.005"} 0
healthcheck_metrics_db_dur_bucket{le="0.01"} 0
healthcheck_metrics_db_dur_bucket{le="0.025"} 0
healthcheck_metrics_db_dur_bucket{le="0.05"} 0
healthcheck_metrics_db_dur_bucket{le="0.1"} 1
healthcheck_metrics_db_dur_bucket{le="0.25"} 1
healthcheck_metrics_db_dur_bucket{le="0.5"} 1
healthcheck_metrics_db_dur_bucket{le="1"} 1
healthcheck_metrics_db_dur_bucket{le="2.5"} 1
healthcheck_metrics_db_dur_bucket{le="5"} 1
healthcheck_metrics_db_dur_bucket{le="10"} 1
healthcheck_metrics_db_dur_bucket{le="+Inf"} 1
healthcheck_metrics_db_dur_sum 0.094148065
healthcheck_metrics_db_dur_count 1
```

##### buildInfo

```
# HELP go_build_info Build information about the main Go module.
# TYPE go_build_info gauge
go_build_info{checksum="",path="",version=""} 1
```

##### goCollector

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

##### processCollector

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
