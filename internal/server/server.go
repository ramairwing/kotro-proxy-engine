// Package server boots the KortoLabs local HTTP listener and routes.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kortolabs/proxy-engine/internal/cache"
	"github.com/kortolabs/proxy-engine/internal/config"
	"github.com/kortolabs/proxy-engine/internal/proxy"
)

// Server wraps the HTTP listener and its dependencies.
type Server struct {
	cfg    config.Config
	http   *http.Server
	cache  *cache.Store
	logger *slog.Logger
}

// New constructs a Server without starting it.
func New(cfg config.Config, logger *slog.Logger) (*Server, error) {
	store, err := cache.Open(cfg.CacheDBPath)
	if err != nil {
		return nil, fmt.Errorf("server: cache: %w", err)
	}

	handler, err := proxy.NewHandler(cfg.UpstreamURL, store, logger)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("server: proxy: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &Server{
		cfg:    cfg,
		http:   srv,
		cache:  store,
		logger: logger,
	}, nil
}

// ListenAndServe blocks until the server exits.
func (s *Server) ListenAndServe() error {
	s.logger.Info("kortolabs proxy listening",
		"addr", s.cfg.ListenAddr,
		"upstream", s.cfg.UpstreamURL,
		"cache_db", s.cache.Path(),
	)
	return s.http.ListenAndServe()
}

// Shutdown gracefully stops the server and closes the cache store.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	if closeErr := s.cache.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

// Addr returns the configured listen address.
func (s *Server) Addr() string { return s.cfg.ListenAddr }
