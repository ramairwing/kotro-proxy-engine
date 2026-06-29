package proxy_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kortolabs/proxy-engine/internal/cache"
	"github.com/kortolabs/proxy-engine/internal/proxy"
)

func TestBootstrapUpstreamSSE(t *testing.T) {
	rec := httptest.NewRecorder()
	_, err := proxy.BootstrapUpstreamSSEForTest(rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Body.String(), ": kortolabs bootstrap stream") {
		t.Fatalf("unexpected body prefix: %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatal("missing SSE content type")
	}
	if rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatal("missing X-Accel-Buffering")
	}
}

func TestSSEBootstrapOnCacheMiss(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	dir := t.TempDir()
	store, err := cache.Open(filepath.Join(dir, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler, err := proxy.NewHandlerFromURL(upstream.URL, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	body := `{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"bootstrap-test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Body.String(), ": kortolabs bootstrap stream") {
		t.Fatalf("expected bootstrap comment first, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Fatal("expected upstream SSE after bootstrap")
	}
}

func TestSSEBootstrapOnCacheHit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	store, err := cache.Open(filepath.Join(dir, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler, err := proxy.NewHandlerFromURL(upstream.URL, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	body := `{"model":"gpt-4","stream":true,"messages":[{"role":"system","content":"s"},{"role":"user","content":"hit-bootstrap"}]}`
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	post() // warm cache
	rec := post()
	if rec.Header().Get("X-KortoLabs-Cache") != "HIT" {
		t.Fatal("expected cache hit")
	}
	if !strings.HasPrefix(rec.Body.String(), ": kortolabs bootstrap stream") {
		t.Fatalf("cache hit should also bootstrap, got %q", rec.Body.String())
	}
}
