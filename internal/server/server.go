package server

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/lebe-dev/transmitter/internal/config"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

// Server wraps the HTTP server with graceful shutdown support.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New creates and configures the HTTP server with all routes.
func New(cfg *config.Config, client *transmission.Client, staticFS fs.FS, logger *slog.Logger) (*Server, error) {
	staticHandler, err := StaticHandler(staticFS)
	if err != nil {
		return nil, err
	}

	priorityCfg := AutoPriorityConfig{
		Enabled:   cfg.FilePriorityEnabled,
		HighCount: cfg.FilePriorityHighCount,
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/rpc", ProxyHandler(client, priorityCfg))
	mux.Handle("GET /api/health", HealthHandler(client))
	mux.Handle("/", staticHandler)

	handler := CORSMiddleware(cfg.CORSOrigin, mux)

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		httpServer: httpServer,
		logger:     logger,
	}, nil
}

// Start begins listening for HTTP connections. Returns nil on graceful shutdown.
func (s *Server) Start() error {
	s.logger.Info("http server starting", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the HTTP server within the given context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("http server shutting down")
	return s.httpServer.Shutdown(ctx)
}
