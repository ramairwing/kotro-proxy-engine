package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kotro-labs/proxy-engine/internal/metrics"
)

func TestRegistryHandlerExposesCoreMetrics(t *testing.T) {
	reg := metrics.NewRegistry()
	t.Cleanup(reg.Unregister)

	reg.RecordRequest("openai", "gpt-4o", "/v1/chat/completions", "true", "hit", 0)
	reg.RecordCacheHit("openai", 128)
	reg.RecordCompression(2, 512)
	reg.RecordRedaction("api_key")
	reg.RecordScopeMode("credential")
	reg.SetCacheKeyStrategy("window_n", 4)

	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"kotro_requests_total",
		"kotro_cache_hits_total",
		"kotro_compressor_blocks_stripped_total",
		"kotro_redactions_total",
		"kotro_scope_mode_total",
		"kotro_cache_key_strategy",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metric %q in exposition:\n%s", want, body)
		}
	}
}

func TestStatusClass(t *testing.T) {
	if metrics.StatusClass(200) != "2xx" {
		t.Fatal("expected 2xx")
	}
	if metrics.StatusClass(502) != "5xx" {
		t.Fatal("expected 5xx")
	}
}
