module github.com/AOzhogin/healthcheck/pkg/otelhc

go 1.24

require (
	github.com/AOzhogin/healthcheck v1.3.0
	go.opentelemetry.io/otel v1.31.0
	go.opentelemetry.io/otel/trace v1.31.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.4.1-0.20230718164431-9a2bf3000d16 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	go.opentelemetry.io/otel/metric v1.31.0 // indirect
	golang.org/x/sys v0.11.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

// Dev-only: builds the adapter against the local core before healthcheck v1.3.0 is tagged.
// Ignored by `go get` of this module (replace directives don't apply transitively), so external
// consumers resolve the required healthcheck v1.3.0. Drop this once v1.3.0 is published if desired.
replace github.com/AOzhogin/healthcheck => ../../
