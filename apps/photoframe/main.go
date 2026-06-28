// Command photoframe is a lightweight web slideshow that displays images from a
// WebDAV folder (e.g. a Nextcloud directory), optionally reaching the source
// through a Cloudflare Access protected endpoint via a service token.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	readHeaderTimeout  = 10 * time.Second
	shutdownTimeout    = 10 * time.Second
	healthcheckTimeout = 5 * time.Second
)

func main() {
	// "photoframe healthcheck" probes the local /healthz endpoint and exits
	// 0/1. Used as the container HEALTHCHECK since the distroless image has no
	// shell or curl/wget.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// run owns all deferred cleanup; main only translates its error into an exit
	// code so os.Exit never skips a defer.
	err := run(log)
	if err != nil {
		log.Error("photoframe exited", "error", err)
		os.Exit(1)
	}
}

// run wires up the server and blocks until the process is signalled to stop.
func run(log *slog.Logger) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}

	dav, err := NewWebDAVClient(cfg, httpClient)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := NewServer(cfg, dav, log)
	go srv.refreshLoop(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	go func() {
		<-ctx.Done()

		// A fresh context is intentional: the signal context is already done, so
		// it cannot bound the graceful-shutdown deadline.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		//nolint:contextcheck // signal ctx is already done; shutdown needs its own deadline
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info("photoframe listening", "addr", cfg.ListenAddr, "webdav", cfg.WebDAVBaseURL+cfg.WebDAVPath)

	err = httpSrv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

// runHealthcheck performs a single GET against the local /healthz endpoint.
func runHealthcheck() int {
	addr := getenv("LISTEN_ADDR", ":8080")

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = "", strings.TrimPrefix(addr, ":")
	}

	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return 1
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 1
	}

	return 0
}
