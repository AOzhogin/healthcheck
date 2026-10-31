module github.com/AOzhogin/healthcheck/pkg/mcphc

go 1.26.0

require (
	github.com/AOzhogin/healthcheck v1.5.0
	github.com/modelcontextprotocol/go-sdk v1.6.0
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.4.1-0.20230718164431-9a2bf3000d16 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Dev-only: builds the adapter against the local core before healthcheck v1.5.0 is tagged.
// Ignored by `go get` of this module (replace directives don't apply transitively), so external
// consumers resolve the required healthcheck v1.5.0. Drop this once v1.5.0 is published if desired.
replace github.com/AOzhogin/healthcheck => ../../
