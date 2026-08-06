package webserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
	"uit-api/logger"
)

func runServerLifecycle(
	ctx context.Context,
	log *logger.Slogger,
	serverName string,
	shutdownTimeout time.Duration,
	serveFn func() error,
	shutdownFn func(context.Context) error,
) error {
	serverErr := make(chan error, 1)
	go func() {
		if err := serveFn(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Infof("%s shutting down...", serverName)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := shutdownFn(shutdownCtx); err != nil {
			return fmt.Errorf("error shutting down %s: %w", serverName, err)
		}
		log.Infof("%s stopped", serverName)
		return nil
	case err := <-serverErr:
		return err
	}
}
