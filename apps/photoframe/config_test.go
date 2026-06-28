package main

import (
	"testing"
	"time"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"   ", ""},
		{"Photos/Frame", "/Photos/Frame"},
		{"/Photos/Frame", "/Photos/Frame"},
		{"/Photos/Frame/", "/Photos/Frame"},
		{"  /Photos/Frame/  ", "/Photos/Frame"},
		{"///Photos///", "/Photos"},
	}
	for _, c := range cases {
		if got := normalizePath(c.in); got != c.want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGetenv(t *testing.T) {
	t.Setenv("PF_TEST_GETENV", "value")

	if got := getenv("PF_TEST_GETENV", "def"); got != "value" {
		t.Errorf("getenv set = %q, want value", got)
	}

	if got := getenv("PF_TEST_GETENV_UNSET", "def"); got != "def" {
		t.Errorf("getenv unset = %q, want def", got)
	}
}

func TestGetdur(t *testing.T) {
	const key = "PF_TEST_DUR"

	cases := []struct {
		name string
		set  bool
		val  string
		want time.Duration
	}{
		{"unset uses default", false, "", 7 * time.Second},
		{"bare integer is seconds", true, "30", 30 * time.Second},
		{"duration string", true, "1m30s", 90 * time.Second},
		{"millisecond string", true, "800ms", 800 * time.Millisecond},
		{"invalid falls back to default", true, "not-a-duration", 7 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				t.Setenv(key, c.val)
			} else {
				t.Setenv(key, "")
			}

			if got := getdur(key, 7*time.Second); got != c.want {
				t.Errorf("getdur(%q) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}

// setRequiredEnv sets the minimal env for a valid LoadConfig and clears the
// optional knobs so defaults apply deterministically.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WEBDAV_BASE_URL", "https://nextcloud.example/remote.php/dav/files/me/")
	t.Setenv("WEBDAV_USERNAME", "me")
	t.Setenv("WEBDAV_PASSWORD", "secret")

	for _, k := range []string{
		"WEBDAV_PATH", "CF_ACCESS_CLIENT_ID", "CF_ACCESS_CLIENT_SECRET",
		"LISTEN_ADDR", "SLIDE_INTERVAL", "FADE_DURATION",
		"CLIENT_REFRESH_INTERVAL", "REFRESH_INTERVAL", "REQUEST_TIMEOUT",
		"IMAGE_CACHE_MAX_AGE",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WEBDAV_PATH", "/Photos/Frame/")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.WebDAVBaseURL != "https://nextcloud.example/remote.php/dav/files/me" {
		t.Errorf("base URL = %q, want trailing slash trimmed", cfg.WebDAVBaseURL)
	}

	if cfg.WebDAVPath != "/Photos/Frame" {
		t.Errorf("path = %q, want /Photos/Frame", cfg.WebDAVPath)
	}

	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}

	if cfg.SlideInterval != 10*time.Second {
		t.Errorf("SlideInterval = %v, want 10s", cfg.SlideInterval)
	}

	if cfg.FadeDuration != 1200*time.Millisecond {
		t.Errorf("FadeDuration = %v, want 1200ms", cfg.FadeDuration)
	}

	if cfg.RefreshInterval != 5*time.Minute {
		t.Errorf("RefreshInterval = %v, want 5m", cfg.RefreshInterval)
	}

	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("RequestTimeout = %v, want 30s", cfg.RequestTimeout)
	}

	if cfg.ImageCacheMaxAge != time.Hour {
		t.Errorf("ImageCacheMaxAge = %v, want 1h", cfg.ImageCacheMaxAge)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LISTEN_ADDR", ":9000")
	t.Setenv("SLIDE_INTERVAL", "45")
	t.Setenv("CF_ACCESS_CLIENT_ID", "id")
	t.Setenv("CF_ACCESS_CLIENT_SECRET", "secret")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.ListenAddr != ":9000" {
		t.Errorf("ListenAddr = %q, want :9000", cfg.ListenAddr)
	}

	if cfg.SlideInterval != 45*time.Second {
		t.Errorf("SlideInterval = %v, want 45s", cfg.SlideInterval)
	}

	if cfg.CFAccessClientID != "id" || cfg.CFAccessClientSecret != "secret" {
		t.Errorf("CF Access pair not loaded: %q / %q", cfg.CFAccessClientID, cfg.CFAccessClientSecret)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string // applied on top of setRequiredEnv
		wantErr bool
	}{
		{"missing base url", map[string]string{"WEBDAV_BASE_URL": ""}, true},
		{"missing username", map[string]string{"WEBDAV_USERNAME": ""}, true},
		{"missing password", map[string]string{"WEBDAV_PASSWORD": ""}, true},
		{"cf access id without secret", map[string]string{"CF_ACCESS_CLIENT_ID": "id-only"}, true},
		{"valid", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setRequiredEnv(t)

			for k, v := range c.env {
				t.Setenv(k, v)
			}

			_, err := LoadConfig()
			if (err != nil) != c.wantErr {
				t.Errorf("LoadConfig error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
