package webserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
	"uit-api/logger"
)

func startWebServer(
	parentContext context.Context,
	log *logger.Slogger,
	serverName string,
	startupFn func() error,
	shutdownFn func(context.Context) error,
	shutdownTimeout time.Duration,
) error {
	// This channel receives any errors that occur during server startup
	serverErrChan := make(chan error, 1)
	go func() {
		if err := startupFn(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- fmt.Errorf("%s encountered an error on startup: %w", serverName, err)
		}
	}()

	select {
	case <-parentContext.Done():
		log.Infof("%s shutting down due to parent context cancellation...", serverName)
		serverCtx, serverCtxCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer serverCtxCancel()
		if err := shutdownFn(serverCtx); err != nil {
			return fmt.Errorf("%s encountered an error while shutting down: %w", serverName, err)
		}
		log.Infof("%s stopped", serverName)
		return nil
	case err := <-serverErrChan:
		return err
	}
}
