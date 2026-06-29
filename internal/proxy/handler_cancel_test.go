package proxy_test

import (
	"context"
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

func TestHandlerCacheHitAbortsOnClientCancel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		var b strings.Builder
		for i := 0; i < 200; i++ {
			b.WriteString("data: {\"choices\":[{\"delta\":{\"content\":\"tok\"}}]}\n\n")
		}
		b.WriteString("data: [DONE]\n\n")
		_, _ = w.Write([]byte(b.String()))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	store, err := cache.Open(filepath.Join(dir, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler, err := proxy.NewHandler(proxy.Options{
		UpstreamURL:       upstream.URL,
		EnableCache:       true,
		EnableRedaction:   false,
		EnableCompression: false,
		CacheHitDelay:     30 * time.Millisecond,
	}, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	body := `{"model":"gpt-4","stream":true,"messages":[{"role":"system","content":"s"},{"role":"user","content":"cancel-hit"}]}`
	warm := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	handler.ServeHTTP(httptest.NewRecorder(), warm)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client context cancellation during cache hit")
	}

	if rec.Header().Get("X-KortoLabs-Cache") != "HIT" {
		t.Fatal("expected cache hit path")
	}
	if strings.Count(rec.Body.String(), `"content":"tok"`) > 30 {
		t.Fatalf("handler replayed too many frames after cancel: %d", strings.Count(rec.Body.String(), `"content":"tok"`))
	}
}
