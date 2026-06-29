package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestPipeWatchdogUnblocksBlockedWrite(t *testing.T) {
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	block := make(chan struct{})
	upstream := io.NopCloser(blockingReader{block: block})

	startPipeWatchdog(ctx, pw, upstream, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var wg sync.WaitGroup
	wg.Add(1)
	var writeErr error
	go func() {
		defer wg.Done()
		_, writeErr = pw.Write([]byte("token"))
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipe write did not unblock after context cancellation")
	}

	if writeErr == nil {
		t.Fatal("expected write error after cancellation")
	}

	_, readErr := pr.Read(make([]byte, 16))
	if readErr == nil {
		t.Fatal("expected read error from closed pipe")
	}
}

func TestInterceptResponseStopsOnClientCancel(t *testing.T) {
	hold := make(chan struct{})
	upstream := &holdReader{hold: hold}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       upstream,
		Request:    req,
	}

	pipeline := streamPipeline{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := pipeline.interceptResponse(resp, requestContext{
		cacheKey:  "test-key",
		streaming: true,
		format:    StreamOpenAI,
	}); err != nil {
		t.Fatal(err)
	}

	readDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		close(readDone)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
		t.Fatal("intercepted body read did not unblock after cancellation")
	}
}

type blockingReader struct {
	block <-chan struct{}
}

func (b blockingReader) Read([]byte) (int, error) {
	<-b.block
	return 0, context.Canceled
}

type holdReader struct {
	mu     sync.Mutex
	hold   chan struct{}
	closed bool
	sent   bool
}

func (h *holdReader) Read(p []byte) (int, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return 0, context.Canceled
	}
	if !h.sent {
		h.sent = true
		h.mu.Unlock()
		const chunk = "data: {}\n\n"
		n := copy(p, chunk)
		return n, nil
	}
	h.mu.Unlock()

	select {
	case <-h.hold:
		return 0, context.Canceled
	case <-time.After(5 * time.Second):
		return 0, context.DeadlineExceeded
	}
}

func (h *holdReader) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		h.closed = true
		close(h.hold)
	}
	return nil
}
