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

	uiSettings := UISettings{
		DeleteWithData: cfg.DeleteWithData,
	}

	telegramUsers := cfg.TelegramUsers
	if telegramUsers == nil {
		telegramUsers = []string{}
	}
	serverConfig := ServerConfig{
		TransmissionURL:       cfg.TransmissionURL,
		ListenAddr:            cfg.ListenAddr,
		CORSOrigin:            cfg.CORSOrigin,
		MaxRequestBodyBytes:   cfg.MaxRequestBodyBytes,
		WebUIEnabled:          cfg.WebUIEnabled,
		TelegramBotEnabled:    cfg.TelegramBotEnabled,
		TelegramUsers:         telegramUsers,
		LogLevel:              cfg.LogLevel.String(),
		FilePriorityEnabled:   cfg.FilePriorityEnabled,
		FilePriorityHighCount: cfg.FilePriorityHighCount,
		DeleteWithData:        cfg.DeleteWithData,
		MonitorInterval:       cfg.MonitorInterval.String(),
		FileSelectTimeout:     cfg.FileSelectTimeout.String(),
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/rpc", ProxyHandler(client, priorityCfg, cfg.MaxRequestBodyBytes))
	mux.Handle("GET /api/health", HealthHandler(client))
	mux.Handle("GET /api/settings", SettingsHandler(uiSettings))
	mux.Handle("GET /api/config", ConfigHandler(serverConfig))
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
