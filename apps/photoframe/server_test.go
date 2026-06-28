package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T, davURL string) *Server {
	t.Helper()

	cfg := Config{
		WebDAVBaseURL:    davURL,
		WebDAVUsername:   "user",
		WebDAVPassword:   "pass",
		SlideInterval:    10 * time.Second,
		FadeDuration:     1200 * time.Millisecond,
		ClientRefresh:    time.Minute,
		RefreshInterval:  5 * time.Minute,
		RequestTimeout:   5 * time.Second,
		ImageCacheMaxAge: time.Hour,
	}

	dav, err := NewWebDAVClient(cfg, http.DefaultClient)
	if err != nil {
		t.Fatalf("NewWebDAVClient: %v", err)
	}

	return NewServer(cfg, dav, slog.New(slog.DiscardHandler))
}

func TestStoreReplaceSnapshotHref(t *testing.T) {
	s := newStore()
	s.replace([]string{"/b.jpg", "/a.jpg"})

	ids, updated, lastErr := s.snapshot()
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want 2 entries", ids)
	}

	if lastErr != "" {
		t.Errorf("lastErr = %q, want empty", lastErr)
	}

	if updated.IsZero() {
		t.Error("updated timestamp should be set after replace")
	}

	// hrefs are sorted, so the first id must map to /a.jpg.
	href, ok := s.href(ids[0])
	if !ok || href != "/a.jpg" {
		t.Errorf("href(ids[0]) = %q, %v, want /a.jpg, true", href, ok)
	}

	if _, ok := s.href("does-not-exist"); ok {
		t.Error("href(unknown) should report not found")
	}
}

func TestStoreReplaceClearsError(t *testing.T) {
	s := newStore()
	s.setErr("boom")

	if _, _, lastErr := s.snapshot(); lastErr != "boom" {
		t.Fatalf("lastErr = %q, want boom", lastErr)
	}

	s.replace([]string{"/a.jpg"})

	if _, _, lastErr := s.snapshot(); lastErr != "" {
		t.Errorf("lastErr after replace = %q, want empty", lastErr)
	}
}

func TestHandleHealthz(t *testing.T) {
	srv := testServer(t, "https://example.net/dav")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if body := rec.Body.String(); body != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestHandleIndex(t *testing.T) {
	srv := testServer(t, "https://example.net/dav")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	if rec.Body.Len() == 0 {
		t.Error("index body should not be empty")
	}

	// Unknown paths fall through to the catch-all handler and 404.
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status for /nope = %d, want 404", rec.Code)
	}
}

func TestHandleImages(t *testing.T) {
	srv := testServer(t, "https://example.net/dav")
	srv.store.replace([]string{"/a.jpg", "/b.jpg"})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/images", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	var resp imagesResponse

	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Images) != 2 {
		t.Fatalf("images = %v, want 2", resp.Images)
	}

	for _, u := range resp.Images {
		if !strings.HasPrefix(u, "img/") {
			t.Errorf("image URL %q should be prefixed with img/", u)
		}
	}

	if resp.IntervalSeconds != 10 {
		t.Errorf("IntervalSeconds = %v, want 10", resp.IntervalSeconds)
	}

	if resp.ClientRefreshSeconds != 60 {
		t.Errorf("ClientRefreshSeconds = %v, want 60", resp.ClientRefreshSeconds)
	}

	if resp.Error != "" {
		t.Errorf("Error = %q, want empty", resp.Error)
	}
}

func TestHandleImagesReportsError(t *testing.T) {
	srv := testServer(t, "https://example.net/dav")
	srv.store.setErr("list failed")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/images", nil))

	var resp imagesResponse

	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Error != "list failed" {
		t.Errorf("Error = %q, want list failed", resp.Error)
	}

	if len(resp.Images) != 0 {
		t.Errorf("images = %v, want empty", resp.Images)
	}
}

func TestHandleImageProxies(t *testing.T) {
	const body = "\xff\xd8\xff binary jpeg bytes"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/photo.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = io.WriteString(w, body)
		default:
			http.Error(w, "missing", http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	srv := testServer(t, upstream.URL+"/dav")
	srv.store.replace([]string{"/photo.jpg", "/gone.jpg"})

	ids, _, _ := srv.store.snapshot()

	var photoID, goneID string

	for _, id := range ids {
		if href, _ := srv.store.href(id); href == "/photo.jpg" {
			photoID = id
		} else {
			goneID = id
		}
	}

	// Happy path: proxied bytes, content type and cache headers.
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/img/"+photoID, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if rec.Body.String() != body {
		t.Errorf("body = %q, want proxied bytes", rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}

	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=3600" {
		t.Errorf("Cache-Control = %q, want private, max-age=3600", cc)
	}

	// Unknown opaque id → 404.
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/img/deadbeef", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status for unknown id = %d, want 404", rec.Code)
	}

	// Known id but upstream 404 → 502 Bad Gateway.
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/img/"+goneID, nil))

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status for upstream miss = %d, want 502", rec.Code)
	}
}

func TestRefresh(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(samplePropfind))
	}))
	defer upstream.Close()

	srv := testServer(t, upstream.URL+"/remote.php/dav/files/user")
	srv.cfg.WebDAVPath = "/Frame"
	srv.dav.cfg.WebDAVPath = "/Frame"

	srv.refresh(context.Background())

	ids, _, lastErr := srv.store.snapshot()
	if lastErr != "" {
		t.Fatalf("lastErr = %q, want empty", lastErr)
	}

	if len(ids) != 1 {
		t.Fatalf("ids = %v, want exactly the one jpg", ids)
	}
}

func TestRefreshRecordsError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	srv := testServer(t, upstream.URL+"/dav")
	srv.refresh(context.Background())

	if _, _, lastErr := srv.store.snapshot(); lastErr == "" {
		t.Error("refresh should record an error when the listing fails")
	}
}
