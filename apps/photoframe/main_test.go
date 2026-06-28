package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRunHealthcheckOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("LISTEN_ADDR", ":"+mustPort(t, srv.URL))
	if got := runHealthcheck(); got != 0 {
		t.Errorf("runHealthcheck() = %d, want 0", got)
	}
}

func TestRunHealthcheckUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	t.Setenv("LISTEN_ADDR", ":"+mustPort(t, srv.URL))
	if got := runHealthcheck(); got != 1 {
		t.Errorf("runHealthcheck() = %d, want 1", got)
	}
}

func TestRunHealthcheckNoServer(t *testing.T) {
	// Start a server only to claim a free port, then close it so the probe
	// hits a refused connection deterministically.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	port := mustPort(t, srv.URL)
	srv.Close()

	t.Setenv("LISTEN_ADDR", ":"+port)
	if got := runHealthcheck(); got != 1 {
		t.Errorf("runHealthcheck() with no server = %d, want 1", got)
	}
}

func mustPort(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	return u.Port()
}
