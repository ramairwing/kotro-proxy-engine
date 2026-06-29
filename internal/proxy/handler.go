// Package proxy implements the core reverse-proxy handler with SSE interception,
// semantic cache lookup/store, and streaming passthrough.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/kortolabs/proxy-engine/internal/cache"
	"github.com/kortolabs/proxy-engine/internal/compressor"
	"github.com/kortolabs/proxy-engine/internal/guardrail"
	"github.com/kortolabs/proxy-engine/internal/models"
)

// Handler is the main chat-completions interceptor.
type Handler struct {
	upstream   *url.URL
	reverse    *httputil.ReverseProxy
	cache      *cache.Store
	compressor *compressor.StateTracker
	logger     *slog.Logger
}

// NewHandler wires the reverse proxy to the upstream base URL and cache store.
func NewHandler(upstreamURL string, store *cache.Store, logger *slog.Logger) (*Handler, error) {
	u, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("proxy: invalid upstream URL: %w", err)
	}

	h := &Handler{
		upstream:   u,
		cache:      store,
		compressor: compressor.NewStateTracker(),
		logger:     logger,
	}

	rp := httputil.NewSingleHostReverseProxy(u)
	rp.FlushInterval = -1 // flush each write for SSE
	rp.ModifyResponse = h.modifyResponse
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("upstream error", "err", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}

	// Preserve the original path (/v1/chat/completions) when forwarding.
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
		req.URL.Host = u.Host
		req.URL.Scheme = u.Scheme
	}

	h.reverse = rp
	return h, nil
}

// ServeHTTP implements http.Handler for POST /v1/chat/completions.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	req, err := models.ParseChatCompletionRequest(body)
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Pipeline: guardrail redaction -> context compression -> cache lookup
	processed, redactionMap := h.applyMiddleware(req)

	systemPrompt, latestUser := processed.ExtractPromptState()
	cacheKey := cache.KeyForRequest(systemPrompt, latestUser, processed.Model)

	// Cache hit: simulate SSE stream from local DB
	if entry, err := h.cache.Get(cacheKey); err == nil && entry != nil {
		h.logger.Info("cache hit", "key", cache.EntryID(cacheKey))
		h.streamCachedSSE(w, entry.RawSSE, redactionMap)
		return
	}

	h.logger.Info("cache miss", "key", cache.EntryID(cacheKey))

	// Rewrite request body with processed (redacted/compressed) payload
	newBody, err := processed.Marshal()
	if err != nil {
		http.Error(w, "marshal", http.StatusInternalServerError)
		return
	}

	// Stash cache key and redaction map in request context for modifyResponse
	ctx := context.WithValue(r.Context(), ctxKeyCacheKey{}, cacheKey)
	ctx = context.WithValue(ctx, ctxKeyRedaction{}, redactionMap)
	ctx = context.WithValue(ctx, ctxKeyModel{}, processed.Model)
	r = r.WithContext(ctx)
	r.Body = io.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))
	r.Header.Set("Content-Type", "application/json")

	h.reverse.ServeHTTP(w, r)
}

type ctxKeyCacheKey struct{}
type ctxKeyRedaction struct{}
type ctxKeyModel struct{}

// modifyResponse intercepts upstream SSE, streams to client, and async-caches.
func (h *Handler) modifyResponse(resp *http.Response) error {
	cacheKey, _ := resp.Request.Context().Value(ctxKeyCacheKey{}).(string)
	redactionMap, _ := resp.Request.Context().Value(ctxKeyRedaction{}).(*guardrail.RedactionMap)
	model, _ := resp.Request.Context().Value(ctxKeyModel{}).(string)

	if cacheKey == "" || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return nil
	}

	pr, pw := io.Pipe()
	origBody := resp.Body
	resp.Body = pr

	go func() {
		defer pw.Close()
		defer origBody.Close()

		var captured bytes.Buffer
		scanner := bufio.NewScanner(origBody)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()
			outLine := line

			// Restore redacted placeholders in SSE data lines
			if redactionMap != nil && bytes.HasPrefix(line, []byte("data: ")) {
				payload := bytes.TrimPrefix(line, []byte("data: "))
				if !bytes.Equal(payload, []byte("[DONE]")) {
					restored := h.restoreSSEChunk(payload, redactionMap)
					outLine = append([]byte("data: "), restored...)
				}
			}

			captured.Write(line)
			captured.WriteByte('\n')

			if _, err := pw.Write(outLine); err != nil {
				return
			}
			if _, err := pw.Write([]byte("\n")); err != nil {
				return
			}
		}

		// Async persist complete stream to embedded DB
		entry := cache.Entry{
			Key:       cacheKey,
			RawSSE:    append([]byte(nil), captured.Bytes()...),
			Model:     model,
			CreatedAt: time.Now().Unix(),
		}
		if err := h.cache.Put(entry); err != nil {
			h.logger.Error("cache put failed", "key", cache.EntryID(cacheKey), "err", err)
		} else {
			h.logger.Info("cache stored", "key", cache.EntryID(cacheKey), "bytes", len(entry.RawSSE))
		}
	}()

	return nil
}

// restoreSSEChunk unmarshals an SSE JSON chunk, restores placeholders in content
// deltas, and re-marshals.
func (h *Handler) restoreSSEChunk(payload []byte, rm *guardrail.RedactionMap) []byte {
	var chunk models.StreamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return payload
	}
	for i := range chunk.Choices {
		if chunk.Choices[i].Delta.Content != "" {
			chunk.Choices[i].Delta.Content = rm.Restore(chunk.Choices[i].Delta.Content)
		}
	}
	out, err := json.Marshal(chunk)
	if err != nil {
		return payload
	}
	return out
}

// streamCachedSSE replays a stored SSE body to the client with optional restoration.
func (h *Handler) streamCachedSSE(w http.ResponseWriter, raw []byte, rm *guardrail.RedactionMap) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Bytes()
		outLine := line

		if rm != nil && bytes.HasPrefix(line, []byte("data: ")) {
			payload := bytes.TrimPrefix(line, []byte("data: "))
			if !bytes.Equal(payload, []byte("[DONE]")) {
				restored := h.restoreSSEChunk(payload, rm)
				outLine = append([]byte("data: "), restored...)
			}
		}

		_, _ = w.Write(outLine)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()

		// Simulate natural streaming cadence for cache hits (minimal delay)
		time.Sleep(2 * time.Millisecond)
	}
}
