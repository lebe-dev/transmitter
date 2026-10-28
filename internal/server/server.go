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
// movies is nil while the Movie Plugin is disabled.
func New(cfg *config.Config, client *transmission.Client, noteStore NoteStore, shiftStore ShiftStore, impactStore ImpactStore, memoStore MemoStore, pauser Pauser, movies MovieService, version string, staticFS fs.FS, logger *slog.Logger) (*Server, error) {
	staticHandler, err := StaticHandler(staticFS)
	if err != nil {
		return nil, err
	}

	priorityCfg := AutoPriorityConfig{
		Enabled:   cfg.FilePriorityEnabled,
		HighCount: cfg.FilePriorityHighCount,
	}

	configuredShifts := ConfiguredShifts{
		Night: cfg.NightShiftEnabled,
		Day:   cfg.DayShiftEnabled,
	}

	uiSettings := UISettings{
		Version:              version,
		DeleteWithData:       cfg.DeleteWithData,
		NightShiftConfigured: cfg.NightShiftEnabled,
		NightShiftEnabled:    cfg.NightShiftEnabled,
		DayShiftConfigured:   cfg.DayShiftEnabled,
		DayShiftEnabled:      cfg.DayShiftEnabled,
		NoteMaxLength:        noteStore.MaxLength(),
		ImpactTargetRatio:    cfg.ImpactTargetRatio,
		MoviePluginEnabled:   movies != nil,
	}
	if cfg.NightShiftEnabled {
		uiSettings.NightShiftStart = cfg.NightShiftStart.String()
		uiSettings.NightShiftEnd = cfg.NightShiftEnd.String()
	}
	if cfg.DayShiftEnabled {
		uiSettings.DayShiftStart = cfg.DayShiftStart.String()
		uiSettings.DayShiftEnd = cfg.DayShiftEnd.String()
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
		NightShiftConfigured:  cfg.NightShiftEnabled,
		DayShiftConfigured:    cfg.DayShiftEnabled,
		DBPath:                cfg.DBPath,
		NoteMaxLength:         cfg.NoteMaxLength,
		NoteCleanupInterval:   cfg.NoteCleanupInterval.String(),
		ImpactTargetRatio:     cfg.ImpactTargetRatio,
		ImpactInterval:        cfg.ImpactInterval.String(),
		MoviePluginEnabled:    movies != nil,
	}
	if movies != nil {
		serverConfig.Movie = &MovieServerConfig{
			TMDBAPIURL:   cfg.Movie.TMDBAPIURL,
			TMDBImageURL: cfg.Movie.TMDBImageURL,
			Language:     cfg.Movie.Language,
			AutoDetect:   cfg.Movie.AutoDetect,
			Backfill:     cfg.Movie.Backfill,
			AutoRename:   cfg.Movie.AutoRename,
			Template:     cfg.Movie.Template,
			ScanInterval: cfg.Movie.ScanInterval.String(),
			ScanBatch:    cfg.Movie.ScanBatch,
			PosterDir:    cfg.Movie.PosterDir,
		}
	}
	if cfg.NightShiftEnabled {
		serverConfig.NightShiftStart = cfg.NightShiftStart.String()
		serverConfig.NightShiftEnd = cfg.NightShiftEnd.String()
	}
	if cfg.DayShiftEnabled {
		serverConfig.DayShiftStart = cfg.DayShiftStart.String()
		serverConfig.DayShiftEnd = cfg.DayShiftEnd.String()
	}

	var onAdded func()
	if movies != nil {
		onAdded = movies.Wake
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/rpc", ProxyHandler(client, priorityCfg, cfg.MaxRequestBodyBytes, noteStore, onAdded))
	mux.Handle("GET /api/health", HealthHandler(client))
	mux.Handle("GET /api/settings", SettingsHandler(uiSettings, shiftStore, impactStore))
	mux.Handle("GET /api/config", ConfigHandler(serverConfig))
	mux.Handle("PUT /api/shifts/{shift}", ShiftToggleHandler(shiftStore, configuredShifts))
	mux.Handle("PUT /api/impact", ImpactToggleHandler(impactStore))
	mux.Handle("GET /api/memo", MemoHandler(memoStore))
	mux.Handle("PUT /api/memo", MemoUpdateHandler(memoStore))
	mux.Handle("GET /api/pause", PauseStateHandler(pauser))
	mux.Handle("PUT /api/pause", PauseToggleHandler(pauser))
	mux.Handle("GET /api/notes", NotesHandler(noteStore))
	mux.Handle("PUT /api/notes/{hash}", NoteUpdateHandler(noteStore))
	mux.Handle("DELETE /api/notes/{hash}", NoteDeleteHandler(noteStore))
	if movies != nil {
		registerMovieRoutes(mux, movies)
	}
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
