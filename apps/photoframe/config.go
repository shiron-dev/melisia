package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the runtime configuration sourced from environment variables.
type Config struct {
	ListenAddr string

	// WebDAV source.
	WebDAVBaseURL  string // e.g. https://nextcloud.example.net/remote.php/dav/files/USER
	WebDAVPath     string // folder under the base URL, e.g. /Photos/Frame
	WebDAVUsername string
	WebDAVPassword string

	// Cloudflare Access service token (optional). When set, every outbound
	// WebDAV request carries the CF-Access-Client-Id / CF-Access-Client-Secret
	// headers so it can pass through a Cloudflare Access protected endpoint.
	CFAccessClientID     string
	CFAccessClientSecret string

	// Behaviour.
	SlideInterval   time.Duration // client-side slide rotation interval
	RefreshInterval time.Duration // how often the server re-lists the folder
	RequestTimeout  time.Duration // per outbound WebDAV request timeout
}

// LoadConfig reads configuration from the environment, applying defaults and
// validating required fields.
func LoadConfig() (Config, error) {
	cfg := Config{
		ListenAddr:           getenv("LISTEN_ADDR", ":8080"),
		WebDAVBaseURL:        strings.TrimRight(os.Getenv("WEBDAV_BASE_URL"), "/"),
		WebDAVPath:           normalizePath(os.Getenv("WEBDAV_PATH")),
		WebDAVUsername:       os.Getenv("WEBDAV_USERNAME"),
		WebDAVPassword:       os.Getenv("WEBDAV_PASSWORD"),
		CFAccessClientID:     os.Getenv("CF_ACCESS_CLIENT_ID"),
		CFAccessClientSecret: os.Getenv("CF_ACCESS_CLIENT_SECRET"),
		SlideInterval:        getdur("SLIDE_INTERVAL", 10*time.Second),
		RefreshInterval:      getdur("REFRESH_INTERVAL", 5*time.Minute),
		RequestTimeout:       getdur("REQUEST_TIMEOUT", 30*time.Second),
	}

	if cfg.WebDAVBaseURL == "" {
		return cfg, fmt.Errorf("WEBDAV_BASE_URL is required")
	}
	if cfg.WebDAVUsername == "" || cfg.WebDAVPassword == "" {
		return cfg, fmt.Errorf("WEBDAV_USERNAME and WEBDAV_PASSWORD are required")
	}
	// CF Access headers must be supplied as a pair or not at all.
	if (cfg.CFAccessClientID == "") != (cfg.CFAccessClientSecret == "") {
		return cfg, fmt.Errorf("CF_ACCESS_CLIENT_ID and CF_ACCESS_CLIENT_SECRET must be set together")
	}

	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getdur reads a duration. A bare integer is interpreted as seconds; otherwise
// any time.ParseDuration string (e.g. "90s", "5m") is accepted.
func getdur(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return def
}

// normalizePath ensures the configured folder path starts with a single slash
// and has no trailing slash (root becomes "").
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	p = "/" + strings.Trim(p, "/")
	return p
}
