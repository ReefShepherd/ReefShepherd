// Package app wires the configuration, middleware, routes and HTTP server together.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/version"
)

const (
	// maxBodyBytes is the default request body limit; routes that take uploads need a different one.
	maxBodyBytes = 1 << 20
	// contentSecurityPolicy allows only same-origin resources and no framing.
	contentSecurityPolicy = "default-src 'self'; img-src 'self' data: blob:; object-src 'none'; " +
		"base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

	shutdownTimeout   = 15 * time.Second
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
)

// App is the assembled server.
type App struct {
	cfg     config.Config
	logger  *slog.Logger
	handler http.Handler
}

// New builds the route table and wraps it in the middleware stack.
func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	crossOrigin, err := httpx.CrossOrigin(cfg.TrustedOrigins)
	if err != nil {
		return nil, fmt.Errorf("build cross-origin protection: %w", err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux)

	handler := httpx.Chain(mux,
		httpx.RequestLog(logger),
		httpx.Recover(logger),
		httpx.SecurityHeaders(contentSecurityPolicy),
		httpx.MaxBody(maxBodyBytes),
		crossOrigin,
	)
	return &App{cfg: cfg, logger: logger, handler: handler}, nil
}

// Handler is the full HTTP handler including middleware.
func (a *App) Handler() http.Handler {
	return a.handler
}

// Run creates the data directory, listens on the configured address and serves until ctx is cancelled.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	a, err := New(cfg, logger)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Listen, err)
	}
	logger.Info("starting",
		slog.String("version", version.Version()),
		slog.String("listen", ln.Addr().String()),
		slog.String("data_dir", cfg.DataDir),
		slog.Bool("dev", cfg.Dev),
	)
	return a.Serve(ctx, ln)
}

// Serve serves on ln until ctx is cancelled, then lets in-flight requests finish for up to 15 seconds.
func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           a.handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(a.logger.Handler(), slog.LevelWarn),
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	a.logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
