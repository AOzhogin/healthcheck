package healthcheck

import (
	"io"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestHTTPServer_DefaultConfig(t *testing.T) {
	h := New(WithHTTPAddress(":8080"))
	go func() {
		if err := h.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://localhost:8080/health?body=true")
	if err != nil {
		t.Fatalf("failed to GET /health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHTTPServer_CustomConfig(t *testing.T) {
	h := New(
		WithHTTPAddress(":18080"),
		WithSuccessStatus(201),
		WithCheckStatusSuccess("GOOD"),
		WithCheckStatusError("FAIL"),
	)
	go func(t *testing.T) {
		if err := h.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}(t)
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://localhost:18080/health?body=true")
	if err != nil {
		t.Fatalf("failed to GET /health: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) == "" || (!contains(string(body), "GOOD") && !contains(string(body), "FAIL")) {
		t.Errorf("expected body to contain custom status strings, got: %s", string(body))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (contains(s[1:], substr) || contains(s[:len(s)-1], substr))))
}
