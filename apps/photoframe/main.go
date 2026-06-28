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

func main() {
	// "photoframe healthcheck" probes the local /healthz endpoint and exits
	// 0/1. Used as the container HEALTHCHECK since the distroless image has no
	// shell or curl/wget.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := LoadConfig()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}
	dav, err := NewWebDAVClient(cfg, httpClient)
	if err != nil {
		log.Error("init webdav client", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := NewServer(cfg, dav, log)
	go srv.refreshLoop(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info("photoframe listening", "addr", cfg.ListenAddr, "webdav", cfg.WebDAVBaseURL+cfg.WebDAVPath)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http server", "error", err)
		os.Exit(1)
	}
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

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
