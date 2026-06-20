package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
)

//go:embed web/*
var webFS embed.FS

// store keeps the current set of image hrefs behind an opaque id so the browser
// never sees real WebDAV paths or credentials.
type store struct {
	mu      sync.RWMutex
	byID    map[string]string // id -> href
	order   []string          // ids in display order
	updated time.Time
	lastErr string
}

func newStore() *store {
	return &store{byID: map[string]string{}}
}

func (s *store) replace(hrefs []string) {
	sort.Strings(hrefs)
	byID := make(map[string]string, len(hrefs))
	order := make([]string, 0, len(hrefs))
	for _, h := range hrefs {
		sum := sha256.Sum256([]byte(h))
		id := hex.EncodeToString(sum[:8])
		byID[id] = h
		order = append(order, id)
	}
	s.mu.Lock()
	s.byID = byID
	s.order = order
	s.updated = time.Now()
	s.lastErr = ""
	s.mu.Unlock()
}

func (s *store) setErr(msg string) {
	s.mu.Lock()
	s.lastErr = msg
	s.mu.Unlock()
}

func (s *store) snapshot() ([]string, time.Time, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, len(s.order))
	copy(ids, s.order)
	return ids, s.updated, s.lastErr
}

func (s *store) href(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.byID[id]
	return h, ok
}

// Server wires the WebDAV client, image store and HTTP handlers together.
type Server struct {
	cfg   Config
	dav   *WebDAVClient
	store *store
	log   *slog.Logger
}

func NewServer(cfg Config, dav *WebDAVClient, log *slog.Logger) *Server {
	return &Server{cfg: cfg, dav: dav, store: newStore(), log: log}
}

// refreshLoop refreshes the image list immediately and then on RefreshInterval
// until the context is cancelled.
func (s *Server) refreshLoop(ctx context.Context) {
	s.refresh(ctx)
	t := time.NewTicker(s.cfg.RefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refresh(ctx)
		}
	}
}

func (s *Server) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
	defer cancel()

	hrefs, err := s.dav.List(ctx)
	if err != nil {
		s.log.Error("list webdav folder", "error", err)
		s.store.setErr(err.Error())
		return
	}
	s.store.replace(hrefs)
	s.log.Info("refreshed image list", "count", len(hrefs))
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/images", s.handleImages)
	mux.HandleFunc("GET /img/{id}", s.handleImage)
	mux.HandleFunc("GET /", s.handleIndex)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "index unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

type imagesResponse struct {
	Images               []string `json:"images"` // image URLs the browser can request
	IntervalSeconds      float64  `json:"intervalSeconds"`
	FadeSeconds          float64  `json:"fadeSeconds"`
	ClientRefreshSeconds float64  `json:"clientRefreshSeconds"`
	UpdatedAt            string   `json:"updatedAt,omitempty"`
	Error                string   `json:"error,omitempty"`
}

func (s *Server) handleImages(w http.ResponseWriter, _ *http.Request) {
	ids, updated, lastErr := s.store.snapshot()
	urls := make([]string, len(ids))
	for i, id := range ids {
		urls[i] = "img/" + id
	}
	resp := imagesResponse{
		Images:               urls,
		IntervalSeconds:      s.cfg.SlideInterval.Seconds(),
		FadeSeconds:          s.cfg.FadeDuration.Seconds(),
		ClientRefreshSeconds: s.cfg.ClientRefresh.Seconds(),
		Error:                lastErr,
	}
	if !updated.IsZero() {
		resp.UpdatedAt = updated.UTC().Format(time.RFC3339)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	href, ok := s.store.href(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()

	upstream, err := s.dav.Fetch(ctx, href)
	if err != nil {
		s.log.Error("fetch image", "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer upstream.Body.Close()

	if upstream.StatusCode != http.StatusOK {
		http.Error(w, "upstream status "+upstream.Status, http.StatusBadGateway)
		return
	}

	if ct := upstream.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := upstream.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", int(s.cfg.ImageCacheMaxAge.Seconds())))
	_, _ = io.Copy(w, upstream.Body)
}
