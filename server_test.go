package healthcheck

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestHTTPServer_DefaultConfig(t *testing.T) {
	h := New().(*healthCheck)
	go func() {
		if err := h.StartHTTPServer(); err != http.ErrServerClosed {
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
		WithPort(":18080"),
		WithSuccessStatus(201),
		WithCheckStatusSuccess("GOOD"),
		WithCheckStatusError("FAIL"),
	).(*healthCheck)
	go func(t *testing.T) {
		if err := h.StartHTTPServer(); err != http.ErrServerClosed {
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
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) == "" || (string(body) != "" && !(contains(string(body), "GOOD") || contains(string(body), "FAIL"))) {
		t.Errorf("expected body to contain custom status strings, got: %s", string(body))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (contains(s[1:], substr) || contains(s[:len(s)-1], substr))))
}
