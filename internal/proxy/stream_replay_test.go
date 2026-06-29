package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReplayCachedAbortsOnContextCancel(t *testing.T) {
	var raw bytes.Buffer
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&raw, "data: {\"chunk\":%d}\n\n", i)
	}
	raw.WriteString("data: [DONE]\n\n")

	pipeline := streamPipeline{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		opts:   Options{CacheHitDelay: 25 * time.Millisecond},
	}

	ctx, cancel := context.WithCancel(context.Background())
	rec := httptest.NewRecorder()

	done := make(chan error, 1)
	go func() {
		done <- pipeline.replayCached(ctx, rec, raw.Bytes(), nil, StreamOpenAI)
	}()

	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cache replay did not abort after context cancellation")
	}

	body := rec.Body.String()
	if strings.Count(body, `"chunk":`) > 50 {
		t.Fatalf("replay wrote too many frames after cancel: %d chunks", strings.Count(body, `"chunk":`))
	}
}

func TestReplayCachedCompletesWhenContextActive(t *testing.T) {
	raw := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	pipeline := streamPipeline{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		opts:   Options{CacheHitDelay: 0},
	}

	rec := httptest.NewRecorder()
	err := pipeline.replayCached(context.Background(), rec, raw, nil, StreamOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `"content":"ok"`) {
		t.Fatalf("expected cached payload in body: %q", rec.Body.String())
	}
}
