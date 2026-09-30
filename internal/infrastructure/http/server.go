package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Server wraps the standard http.Server with graceful shutdown capabilities.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// NewServer configures a new resilient HTTP server instance.
func NewServer(port int, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		logger: logger,
	}
}

// Start runs the HTTP server and gracefully shuts down when ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	serverErr := make(chan error, 1)

	go func() {
		s.logger.Info("starting resilient ai gateway server", slog.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down http server gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}
