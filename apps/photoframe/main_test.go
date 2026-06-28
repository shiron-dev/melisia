package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestRunInvalidConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WEBDAV_BASE_URL", "") // required field missing

	err := run(t.Context(), slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("run should fail when configuration is invalid")
	}
}

func TestRunWebDAVClientError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WEBDAV_BASE_URL", "not-an-absolute-url") // passes LoadConfig, fails client init

	err := run(t.Context(), slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("run should fail when the WebDAV base URL is not absolute")
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LISTEN_ADDR", "127.0.0.1:0") // ephemeral port
	t.Setenv("WEBDAV_BASE_URL", "http://127.0.0.1:9/dav")
	t.Setenv("REQUEST_TIMEOUT", "1s")
	t.Setenv("REFRESH_INTERVAL", "1h")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() { errCh <- run(ctx, slog.New(slog.DiscardHandler)) }()

	// Give the server a moment to start listening, then trigger shutdown.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run returned %v, want nil after graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after context cancellation")
	}
}

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
