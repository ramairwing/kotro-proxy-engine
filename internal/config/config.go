// Package config loads runtime settings for the KortoLabs proxy engine.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all tunable proxy parameters.
type Config struct {
	// ListenAddr is the local bind address (default :8080).
	ListenAddr string

	// UpstreamURL is the base URL of the upstream LLM provider.
	UpstreamURL string

	// CacheDBPath is the on-disk path for the embedded bbolt cache store.
	CacheDBPath string

	// ReadTimeout caps idle time waiting for client request bodies.
	ReadTimeout time.Duration

	// WriteTimeout caps time writing streaming responses to clients.
	WriteTimeout time.Duration

	// IdleTimeout closes keep-alive connections after this duration.
	IdleTimeout time.Duration
}

// Load reads configuration from environment variables with sensible defaults
// for local development against the mock upstream on port 9000.
func Load() Config {
	return Config{
		ListenAddr:   envOr("KORTO_LISTEN_ADDR", ":8080"),
		UpstreamURL:  envOr("KORTO_UPSTREAM_URL", "http://127.0.0.1:9000"),
		CacheDBPath:  envOr("KORTO_CACHE_DB", "./kortolabs-cache.db"),
		ReadTimeout:  envDurationOr("KORTO_READ_TIMEOUT", 30*time.Second),
		WriteTimeout: envDurationOr("KORTO_WRITE_TIMEOUT", 0), // 0 = no timeout for SSE streams
		IdleTimeout:  envDurationOr("KORTO_IDLE_TIMEOUT", 120*time.Second),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return fallback
}
