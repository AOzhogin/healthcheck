package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func Test_healthCheck_HandlerHealth(t *testing.T) {
	type args struct {
		Options []HCOption
		CheckFN func(h HealthCheck)
	}
	type want struct {
		status int
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{name: "Normal 200", args: args{Options: nil}, want: want{status: http.StatusOK}},
		{name: "With custom success code", args: args{Options: []HCOption{WithSuccessStatus(http.StatusCreated)}}, want: want{status: http.StatusCreated}},
		{name: "Normal error 503", args: args{Options: nil,
			CheckFN: func(h HealthCheck) {
				h.Add("fail test", "always error check", func(ctx context.Context) error {
					return fmt.Errorf("error")
				})
			},
		}, want: want{status: http.StatusServiceUnavailable}},
		{name: "With custom error 500", args: args{Options: []HCOption{WithErrorStatus(http.StatusInternalServerError)},
			CheckFN: func(h HealthCheck) {
				h.Add("fail test", "always error check", func(ctx context.Context) error {
					return fmt.Errorf("error")
				})
			},
		}, want: want{status: http.StatusInternalServerError}},
		{name: "Check time out", args: args{Options: []HCOption{WithTimeOut(2 * time.Second)},
			CheckFN: func(h HealthCheck) {
				h.Add("fail test", "long time check", func(ctx context.Context) error {
					select {
					case <-ctx.Done():
						return fmt.Errorf("timed out")
					case <-time.After(5 * time.Second):
						return nil
					}
				})
			},
		}, want: want{status: http.StatusServiceUnavailable}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			h := New(tt.args.Options...)

			if tt.args.CheckFN != nil {
				tt.args.CheckFN(h)
			}

			request := httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil)
			response := httptest.NewRecorder()

			h.HandlerHealth(response, request)

			assertResponseCode(t, response.Code, tt.want.status)
			assertResponseContentType(t, response.Header().Get("Content-Type"), "application/json")
			assertResponseBody(t, response.Body.String(), "")
		})
	}
}

func Test_healthCheck_HandlerMetrics(t *testing.T) {

	type args struct {
		Options []HCOption
	}
	type want struct {
		status int
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{name: "Normal", args: args{Options: []HCOption{WithMetrics(false, false, false)}}, want: want{status: http.StatusOK}},
		{name: "Not implemented", args: args{Options: nil}, want: want{status: http.StatusNotImplemented}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			request := httptest.NewRequest(http.MethodGet, HandlerMetrics, nil)
			response := httptest.NewRecorder()

			h := New(tt.args.Options...)

			h.HandlerMetrics(response, request)

			assertResponseCode(t, response.Code, tt.want.status)
			assertResponseContentType(t, response.Header().Get("Content-Type"), "")
			assertResponseBody(t, response.Body.String(), "")
		})
	}
}

func Test_healthCheck_HandlerPProf(t *testing.T) {

	type args struct {
		Options []HCOption
	}
	type want struct {
		status int
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{name: "Normal", args: args{Options: nil}, want: want{status: http.StatusOK}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			request := httptest.NewRequest(http.MethodGet, HandlerDebug, nil)
			response := httptest.NewRecorder()

			h := New(tt.args.Options...)

			h.HandlerPProf(response, request)

			assertResponseCode(t, response.Code, tt.want.status)
			assertResponseContentType(t, response.Header().Get("Content-Type"), "text/html; charset=utf-8")
			assertResponseBody(t, response.Body.String(), "<html>\n<head>\n<title>/debug/pprof/</title>\n<style>\n.profile-name{\n\tdisplay:inline-block;\n\twidth:6rem;\n}\n</style>\n</head>\n<body>\n/debug/pprof/\n<br>\n<p>Set debug=1 as a query parameter to export in legacy text format</p>\n<br>\nTypes of profiles available:\n<table>\n<thead><td>Count</td><td>Profile</td></thead>\n<tr><td>1</td><td><a href='allocs?debug=1'>allocs</a></td></tr>\n<tr><td>0</td><td><a href='block?debug=1'>block</a></td></tr>\n<tr><td>0</td><td><a href='cmdline?debug=1'>cmdline</a></td></tr>\n<tr><td>3</td><td><a href='goroutine?debug=1'>goroutine</a></td></tr>\n<tr><td>1</td><td><a href='heap?debug=1'>heap</a></td></tr>\n<tr><td>0</td><td><a href='mutex?debug=1'>mutex</a></td></tr>\n<tr><td>0</td><td><a href='profile?debug=1'>profile</a></td></tr>\n<tr><td>7</td><td><a href='threadcreate?debug=1'>threadcreate</a></td></tr>\n<tr><td>0</td><td><a href='trace?debug=1'>trace</a></td></tr>\n</table>\n<a href=\"goroutine?debug=2\">full goroutine stack dump</a>\n<br>\n<p>\nProfile Descriptions:\n<ul>\n<li><div class=profile-name>allocs: </div> A sampling of all past memory allocations</li>\n<li><div class=profile-name>block: </div> Stack traces that led to blocking on synchronization primitives</li>\n<li><div class=profile-name>cmdline: </div> The command line invocation of the current program</li>\n<li><div class=profile-name>goroutine: </div> Stack traces of all current goroutines. Use debug=2 as a query parameter to export in the same format as an unrecovered panic.</li>\n<li><div class=profile-name>heap: </div> A sampling of memory allocations of live objects. You can specify the gc GET parameter to run GC before taking the heap sample.</li>\n<li><div class=profile-name>mutex: </div> Stack traces of holders of contended mutexes</li>\n<li><div class=profile-name>profile: </div> CPU profile. You can specify the duration in the seconds GET parameter. After you get the profile file, use the go tool pprof command to investigate the profile.</li>\n<li><div class=profile-name>threadcreate: </div> Stack traces that led to the creation of new OS threads</li>\n<li><div class=profile-name>trace: </div> A trace of execution of the current program. You can specify the duration in the seconds GET parameter. After you get the trace file, use the go tool trace command to investigate the trace.</li>\n</ul>\n</p>\n</body>\n</html>")
		})
	}
}

func assertResponseCode(t testing.TB, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("response statusCode is wrong, got %d want %d", got, want)
	}
}

func assertResponseContentType(t testing.TB, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("response content type is wrong, got %q want %q", got, want)
	}
}

func assertResponseBody(t testing.TB, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("response body is wrong, got %q want %q", got, want)
	}
}
