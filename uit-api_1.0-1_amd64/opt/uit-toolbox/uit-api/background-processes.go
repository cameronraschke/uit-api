package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"uit-api/config"
	"uit-api/database"

	"golang.org/x/sync/errgroup"
)

func sendBackgroundLog(ctx context.Context, logChan chan<- string, msg string) bool {
	if logChan == nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case logChan <- msg:
		return true
	default:
		return false
	}
}

func backgroundProcesses(ctx context.Context, errChan chan error) {
	errGroup, errCtx := errgroup.WithContext(ctx)

	log := config.GetLogger().With(slog.String("func", "backgroundProcesses"))
	logChan := make(chan string, 10) // Buffered channel for log messages

	// Listen for log messages from background processes
	errGroup.Go(func() error {
		for {
			select {
			case msg, ok := <-logChan:
				if !ok {
					log.Info("Background process log channel closed")
					return nil
				}
				log.Info("(Background): " + msg)
			case <-errCtx.Done():
				log.Info("Background process log channel closed due to context cancellation")
				return nil
			}
		}
	})

	// Listen for errors on errChan
	errGroup.Go(func() error {
		select {
		case err := <-errChan:
			log.Info(fmt.Sprintf("Background process error, exiting: %v", err))
			return err
		case <-errCtx.Done():
			log.Info("Background processes stopping...")
			return nil
		}
	})

	// Start auth map cleanup goroutine
	errGroup.Go(func() error {
		interval := 5 * time.Minute
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-errCtx.Done():
				if !sendBackgroundLog(errCtx, logChan, "Auth map cleanup goroutine stopping...") {
					if err := errCtx.Err(); err != nil {
						return nil // No error on regular shutdown
					}
				}
				return nil
			case <-ticker.C:
				logMsg, err := cleanupAuthMap()
				if err != nil {
					log.Error(fmt.Sprintf("Error during auth map cleanup: %v", err))
				}
				if !sendBackgroundLog(errCtx, logChan, logMsg) {
					if err := errCtx.Err(); err != nil {
						return nil // No error on regular shutdown
					}
				}
			}
		}
	})

	// Start last_heard write goroutine
	errGroup.Go(func() error {
		interval := 5 * time.Minute
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-errCtx.Done():
				if !sendBackgroundLog(errCtx, logChan, "Last heard goroutine stopping...") {
					if err := errCtx.Err(); err != nil {
						return nil // No error on regular shutdown
					}
				}
				return nil
			case <-ticker.C:
				logMsgs, err := writeLastHeardToDB(ctx, 0) // No sleep between writes during regular operation
				if err != nil {
					log.Error(fmt.Sprintf("Error writing last_heard to DB: %v", err))
				}
				for _, logMsg := range logMsgs {
					if !sendBackgroundLog(errCtx, logChan, logMsg) {
						if err := errCtx.Err(); err != nil {
							return nil // No error on regular shutdown
						}
					}
				}
			}
		}
	})

	// Start rate limiter cleanup goroutine
	errGroup.Go(func() error {
		interval := 1 * time.Minute
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-errCtx.Done():
				if !sendBackgroundLog(errCtx, logChan, "Rate limiter cleanup goroutine stopping...") {
					if err := errCtx.Err(); err != nil {
						return nil // No error on regular shutdown
					}
				}
				return nil
			case <-ticker.C:
				logMsg, err := cleanupRateLimiterEntries()
				if err != nil {
					log.Error(fmt.Sprintf("Error during rate limiter cleanup: %v", err))
				}
				if !sendBackgroundLog(errCtx, logChan, logMsg) {
					if err := errCtx.Err(); err != nil {
						return nil // No error on regular shutdown
					}
				}
			}
		}
	})

	// Start banned clients cleanup goroutine
	errGroup.Go(func() error {
		interval := 1 * time.Minute
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-errCtx.Done():
				if !sendBackgroundLog(errCtx, logChan, "Banned clients cleanup goroutine stopping...") {
					if err := errCtx.Err(); err != nil {
						return nil // No error on regular shutdown
					}
				}
				return nil
			case <-ticker.C:
				logMsg, err := cleanupBannedClients()
				if err != nil {
					log.Error(fmt.Sprintf("Error during banned clients cleanup: %v", err))
				}
				if !sendBackgroundLog(errCtx, logChan, logMsg) {
					if err := errCtx.Err(); err != nil {
						return nil // No error on regular shutdown
					}
				}
			}
		}
	})

	log.Info("Background processes started")
	if err := errGroup.Wait(); err != nil {
		log.Error(fmt.Sprintf("Background processes exited with error: %v", err))
	} else {
		log.Info("Background processes exited without error")
	}
}

func cleanupAuthMap() (logMsg string, err error) {
	originalSessionCount := config.GetAuthSessionCount()
	config.ClearExpiredAuthSessions()
	newSessionCount := config.GetAuthSessionCount()
	sessionDiff := originalSessionCount - newSessionCount
	return fmt.Sprintf("auth session cleanup done (expired=%d, active=%d)", newSessionCount, sessionDiff), nil
}

func cleanupRateLimiterEntries() (logMsg string, err error) {
	entriesDeleted, totalEntries, err := config.CleanupOldLimiterEntries()
	if err != nil {
		return fmt.Sprintf("error cleaning up old rate limiter entries: %v", err), nil
	}
	return fmt.Sprintf("rate limiter cleanup done (expired=%d, active=%d)", entriesDeleted, totalEntries), nil
}

func cleanupBannedClients() (logMsg string, err error) {
	deletedCount, totalCount, err := config.CleanupExpiredBans()
	if err != nil {
		return fmt.Sprintf("error cleaning up expired banned clients: %v", err), nil
	}
	return fmt.Sprintf("banned clients cleanup done (expired=%d, active=%d)", deletedCount, totalCount), nil
}

func writeLastHeardToDB(parentCtx context.Context, d time.Duration) (logMsg []string, err error) {
	log := config.GetLogger().With(slog.String("func", "writeLastHeardToDB"))
	backgroundCtx, cancel := context.WithTimeout(parentCtx, 45*time.Second)
	defer cancel()

	realtimeData, err := config.GetAllClientRealtimeData()
	if err != nil {
		logMsg = append(logMsg, fmt.Sprintf("Error retrieving client realtime data: %v", err))
		return logMsg, nil
	}

	if len(realtimeData) == 0 {
		logMsg = append(logMsg, "No realtime client data found, last_heard update skipped")
		return logMsg, nil
	}

	var attempted, succeeded, failed int
	for tag, data := range realtimeData {
		if data.Tagnumber == 0 || data.LastHeard == nil || data.LastHeard.IsZero() {
			failed++
			logMsg = append(logMsg, fmt.Sprintf("Skipping tag %d: missing tag or nil last_heard", tag))
			continue
		}

		attempted++
		updateCtx, updateCancel := context.WithTimeout(backgroundCtx, 3*time.Second)
		if err := database.UpdateClientLastHeard(updateCtx, tag, data.LastHeard); err != nil {
			failed++
			log.Error(fmt.Sprintf("Failed to write last heard for tag '%d': %s", tag, err.Error()))
			updateCancel()
			continue
		}
		succeeded++
		updateCancel()
		if d > 0 {
			select {
			case <-backgroundCtx.Done():
				log.Warn("Stopping last-heard write loop early due to context cancellation")
				return logMsg, nil
			case <-time.After(d):
				// Continue to next write interval.
			}
		}
	}

	log.Info(fmt.Sprintf("Finished writing last-heard values (total_clients=%d attempted=%d succeeded=%d failed=%d)", len(realtimeData), attempted, succeeded, failed))
	if attempted == 0 {
		log.Warn("No DB writes were attempted for last_heard because all realtime entries had missing/zero last_heard data")
	}

	return logMsg, nil
}
