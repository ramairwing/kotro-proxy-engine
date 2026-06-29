package proxy_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kortolabs/proxy-engine/internal/cache"
	"github.com/kortolabs/proxy-engine/internal/proxy"
)

func TestProxyCacheMissThenHit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := `{"id":"t","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hello "}}]}`
		_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
		flusher.Flush()
		time.Sleep(5 * time.Millisecond)
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := proxy.NewHandler(upstream.URL, store, logger)
	if err != nil {
		t.Fatal(err)
	}

	body := `{"model":"gpt-4","stream":true,"messages":[{"role":"system","content":"sys"},{"role":"user","content":"ping"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req)

	if w1.Code != http.StatusOK {
		t.Fatalf("miss status %d body %s", w1.Code, w1.Body.String())
	}
	if !strings.Contains(w1.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatal("expected SSE content type on miss")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("hit status %d", w2.Code)
	}
	scanner := bufio.NewScanner(w2.Body)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) < 2 {
		t.Fatalf("expected SSE lines on cache hit, got %v", lines)
	}
}

func TestProxyRedactsSecretsBeforeUpstream(t *testing.T) {
	var received []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	store, _ := cache.Open(filepath.Join(dir, "c.db"))
	defer store.Close()

	handler, _ := proxy.NewHandler(upstream.URL, store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	body := `{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"key AKIAIOSFODNN7EXAMPLE"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var parsed struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(received, &parsed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(received, []byte("AKIAIOSFODNN7EXAMPLE")) {
		t.Fatalf("upstream received unredacted secret: %s", received)
	}
	if !strings.Contains(parsed.Messages[0].Content, "REDACTED") {
		t.Fatalf("expected placeholder in upstream payload: %s", received)
	}
}
