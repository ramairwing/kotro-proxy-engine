package proxy

import (
	"log/slog"
	"net/http"
)

const sseBootstrapComment = ": kortolabs bootstrap stream\n\n"

// sseBootstrapWriter primes the client connection before upstream bytes arrive.
// It is safe to pass to httputil.ReverseProxy after prime(); duplicate WriteHeader
// calls from the proxy are suppressed.
type sseBootstrapWriter struct {
	http.ResponseWriter
	logger        *slog.Logger
	headerWritten bool
	primed        bool
}

func bootstrapUpstreamSSE(w http.ResponseWriter, logger *slog.Logger) (*sseBootstrapWriter, error) {
	if logger == nil {
		logger = slog.Default()
	}
	bw := &sseBootstrapWriter{ResponseWriter: w, logger: logger}
	if err := bw.prime(); err != nil {
		return nil, err
	}
	return bw, nil
}

// BootstrapUpstreamSSEForTest exposes bootstrap for unit tests in the proxy_test package.
func BootstrapUpstreamSSEForTest(w http.ResponseWriter, logger *slog.Logger) (*sseBootstrapWriter, error) {
	return bootstrapUpstreamSSE(w, logger)
}

func (bw *sseBootstrapWriter) Unwrap() http.ResponseWriter {
	return bw.ResponseWriter
}

func (bw *sseBootstrapWriter) WriteHeader(code int) {
	if bw.headerWritten {
		return
	}
	bw.ResponseWriter.WriteHeader(code)
	bw.headerWritten = true
}

func (bw *sseBootstrapWriter) prime() error {
	if bw.primed {
		return nil
	}

	h := bw.ResponseWriter.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	bw.WriteHeader(http.StatusOK)

	if _, err := bw.ResponseWriter.Write([]byte(sseBootstrapComment)); err != nil {
		return err
	}

	if err := flushResponse(bw.ResponseWriter); err != nil {
		bw.logger.Debug("sse bootstrap flush skipped", "err", err)
	}

	bw.primed = true
	bw.logger.Debug("sse wire pipeline primed")
	return nil
}

func setSSEHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
}

func flushResponse(w http.ResponseWriter) error {
	rc := http.NewResponseController(w)
	if err := rc.Flush(); err == nil {
		return nil
	} else if f, ok := w.(http.Flusher); ok {
		f.Flush()
		return nil
	} else {
		return err
	}
}
