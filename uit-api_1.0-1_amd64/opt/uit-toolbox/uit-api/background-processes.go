package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"uit-api/appstate"
	"uit-api/auth"
	"uit-api/database"
	"uit-api/logger"

	"golang.org/x/sync/errgroup"
)

// Expiration for auth sessions and banned clients is checked on each request, this is just a periodic cleanup to free memory
// last_heard values get written to DB on app shutdown, this is in case of a DB/app error
const (
	flushLogBufferInterval        = 3 * time.Second
	authMapCleanupInterval        = 5 * time.Minute
	bannedClientsCleanupInterval  = 5 * time.Minute
	liveScreenshotCleanupInterval = 30 * time.Minute
	rateLimiterCleanupInterval    = 5 * time.Minute
	writeLastHeardInterval        = 30 * time.Minute
)

type backgroundLogMessage struct {
	Level   slog.Level
	Message string
}

func logMessage(ctx context.Context, logChan chan<- backgroundLogMessage, msg backgroundLogMessage) bool {
	if logChan == nil {
		return false
	}
	if msg.Message == "" {
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

type backgroundProcessConfig struct {
	ProcCtx      context.Context
	LogChan      chan<- backgroundLogMessage
	ErrChan      chan<- error
	ShutdownMsg  backgroundLogMessage
	Exec         func(context.Context) ([]backgroundLogMessage, error)
	ExecInterval time.Duration
}

func initBackgroundProcesses(ctx context.Context, errChan chan error) {
	errGroup, errCtx := errgroup.WithContext(ctx)

	log := logger.GetLogger().With(slog.String("func", "initBackgroundProcesses"))
	logChan := make(chan backgroundLogMessage, 100) // Buffered channel for log messages

	// Listen to logChan for log messages
	errGroup.Go(func() error {
		for {
			select {
			case msg, ok := <-logChan:
				if !ok {
					log.Infof("(background) log channel closed, discarding message")
					return nil
				}
				log.Logf(context.Background(), msg.Level, "(background): %s", msg.Message)
			case <-errCtx.Done():
				log.Infof("(background) log channel closed due to context cancellation: %v", errCtx.Err())
				return nil
			}
		}
	})

	// Listen to errChan for errors
	errGroup.Go(func() error {
		select {
		case err, ok := <-errChan:
			if !ok {
				log.Infof("(background) error channel closed, discarding message")
				return nil
			}
			log.Warnf("error received on errChan: %v", err)
			return err
		case <-errCtx.Done():
			log.Infof("(background) error channel closed due to context cancellation: %v", errCtx.Err())
			return nil
		}
	})

	// Start log buffer flush goroutine
	errGroup.Go(func() error {
		return startBackgroundProcess(backgroundProcessConfig{
			ProcCtx:     errCtx,
			LogChan:     logChan,
			ErrChan:     errChan,
			ShutdownMsg: backgroundLogMessage{Level: slog.LevelInfo, Message: "stopping flushing log buffers..."},
			Exec: func(context.Context) (logMsg []backgroundLogMessage, err error) {
				if err := logger.FlushLogBuffers(); err != nil {
					logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelError, Message: fmt.Sprintf("error flushing log buffers: %v", err)})
				}
				return logMsg, err
			},
			ExecInterval: flushLogBufferInterval,
		})
	})

	// Start auth map cleanup goroutine
	errGroup.Go(func() error {
		return startBackgroundProcess(backgroundProcessConfig{
			ProcCtx:     errCtx,
			LogChan:     logChan,
			ErrChan:     errChan,
			ShutdownMsg: backgroundLogMessage{Level: slog.LevelInfo, Message: "stopping auth map cleanup..."},
			Exec: func(context.Context) (logMsg []backgroundLogMessage, err error) {
				activeSessions, expiredSessions := auth.ClearExpiredAuthSessions()
				return []backgroundLogMessage{{Level: slog.LevelInfo, Message: fmt.Sprintf("auth session cleanup done (expired=%d, active=%d)", expiredSessions, activeSessions)}}, nil
			},
			ExecInterval: authMapCleanupInterval,
		})
	})

	// Start last_heard write goroutine
	errGroup.Go(func() error {
		return startBackgroundProcess(backgroundProcessConfig{
			ProcCtx:     errCtx,
			LogChan:     logChan,
			ErrChan:     errChan,
			ShutdownMsg: backgroundLogMessage{Level: slog.LevelInfo, Message: "stopping updating last_heard values..."},
			Exec: func(workerCtx context.Context) ([]backgroundLogMessage, error) {
				return writeLastHeardToDB(workerCtx, 1*time.Second) // 1s sleep between writes during regular operation
			},
			ExecInterval: writeLastHeardInterval,
		})
	})

	// Start rate limiter cleanup goroutine
	errGroup.Go(func() error {
		return startBackgroundProcess(backgroundProcessConfig{
			ProcCtx:     errCtx,
			LogChan:     logChan,
			ErrChan:     errChan,
			ShutdownMsg: backgroundLogMessage{Level: slog.LevelInfo, Message: "stopping rate limiter cleanup..."},
			Exec: func(context.Context) (logMsg []backgroundLogMessage, err error) {
				entriesDeleted, totalEntries, err := auth.CleanupOldLimiterEntries()
				if err != nil {
					logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelError, Message: fmt.Sprintf("error cleaning up expired rate limiter entries: %v", err)})
				}
				logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelInfo, Message: fmt.Sprintf("rate limiter cleanup done (expired=%d, active=%d)", entriesDeleted, totalEntries)})
				return logMsg, err
			},
			ExecInterval: rateLimiterCleanupInterval,
		})
	})

	// Start banned clients cleanup goroutine
	errGroup.Go(func() error {
		return startBackgroundProcess(backgroundProcessConfig{
			ProcCtx:     errCtx,
			LogChan:     logChan,
			ErrChan:     errChan,
			ShutdownMsg: backgroundLogMessage{Level: slog.LevelInfo, Message: "stopping banned clients cleanup..."},
			Exec: func(context.Context) (logMsg []backgroundLogMessage, err error) {
				deletedCount, totalCount, err := auth.CleanupExpiredBans()
				if err != nil {
					logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelError, Message: fmt.Sprintf("error cleaning up expired banned clients entries: %v", err)})
				}
				logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelInfo, Message: fmt.Sprintf("banned clients cleanup done (expired=%d, active=%d)", deletedCount, totalCount)})
				return logMsg, err
			},
			ExecInterval: bannedClientsCleanupInterval,
		})
	})

	// Start offline clients cleanup goroutine
	errGroup.Go(func() error {
		return startBackgroundProcess(backgroundProcessConfig{
			ProcCtx:     errCtx,
			LogChan:     logChan,
			ErrChan:     errChan,
			ShutdownMsg: backgroundLogMessage{Level: slog.LevelInfo, Message: "stopping cleanup of offline clients' live screenshots..."},
			Exec: func(context.Context) (logMsg []backgroundLogMessage, err error) {
				entriesDeleted, entriesSkipped, totalEntries := appstate.ClearOfflineLiveImageBytes()
				logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelInfo, Message: fmt.Sprintf("live screenshot cleanup done (deleted=%d, skipped=%d, total=%d)", entriesDeleted, entriesSkipped, totalEntries)})
				return logMsg, nil
			},
			ExecInterval: liveScreenshotCleanupInterval,
		})
	})

	log.Infof("background processes started")
	if err := errGroup.Wait(); err != nil {
		log.Errorf("background processes exited with error: %v", err)
	} else {
		log.Infof("background processes exited without error")
	}
}

func startBackgroundProcess(cfg backgroundProcessConfig) error {
	ticker := time.NewTicker(cfg.ExecInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cfg.ProcCtx.Done():
			if !logMessage(cfg.ProcCtx, cfg.LogChan, cfg.ShutdownMsg) {
				if err := cfg.ProcCtx.Err(); err != nil {
					if err != context.Canceled {
						return err // Return the error if not context.Canceled
					}
					return nil // Return nil on regular shutdown
				}
			}
			return nil
		case <-ticker.C:
			logMsgs, err := cfg.Exec(cfg.ProcCtx)
			if err != nil && cfg.ErrChan != nil {
				select {
				case cfg.ErrChan <- err:
				default:
					_ = logMessage(cfg.ProcCtx, cfg.LogChan, backgroundLogMessage{Level: slog.LevelError, Message: fmt.Sprintf("background err channel full, dropping error: %v", err)})
				}
			}
			for _, logMsg := range logMsgs {
				if !logMessage(cfg.ProcCtx, cfg.LogChan, logMsg) {
					if err := cfg.ProcCtx.Err(); err != nil {
						if err != context.Canceled {
							return err // Return the error if not context.Canceled
						}
						return nil // Return nil on regular shutdown
					}
				}
			}
		}
	}
}

func writeLastHeardToDB(parentCtx context.Context, d time.Duration) (logMsg []backgroundLogMessage, err error) {
	backgroundCtx, cancel := context.WithTimeout(parentCtx, 45*time.Second)
	defer cancel()

	realtimeDataCopy, err := appstate.GetAllClientRealtimeData()
	if err != nil {
		logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelError, Message: fmt.Sprintf("cannot retrieve realtime data: %v", err)})
		return logMsg, nil
	}

	var attempted, succeeded, failed int
	for tag, data := range realtimeDataCopy {
		if data.Tagnumber == 0 {
			failed++
			logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelWarn, Message: "skipping realtime update: invalid tag value"})
			continue
		}

		if data.LastHeard == nil || data.LastHeard.IsZero() {
			logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelWarn, Message: fmt.Sprintf("skipping realtime update for tag '%d': invalid last_heard value", tag)})
			continue
		}

		if data.LastHeardUpdatedInDB {
			continue // Skip if last_heard has already been updated in the database
		}

		attempted++
		updateCtx, updateCancel := context.WithTimeout(backgroundCtx, 5*time.Second) // 5 seconds for each update
		if err := database.UpdateClientLastHeard(updateCtx, tag, data.LastHeard); err != nil {
			failed++
			logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelError, Message: fmt.Sprintf("cannot update last_heard value for tag '%d': %v", tag, err)})
			updateCancel()
			continue
		}
		succeeded++
		updateCancel()
		if d > 0 {
			select {
			case <-backgroundCtx.Done():
				logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelWarn, Message: "stopping last_heard updates early due to parent context cancellation"})
				return logMsg, nil
			case <-time.After(d):
				// Continue to next write interval.
			}
		}
	}

	logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelInfo, Message: fmt.Sprintf("finished writing persistent last-heard values to DB (total_clients=%d attempted=%d succeeded=%d failed=%d)", len(realtimeDataCopy), attempted, succeeded, failed)})
	if attempted == 0 {
		logMsg = append(logMsg, backgroundLogMessage{Level: slog.LevelWarn, Message: "no realtime data was updated: either no clients are online, or all entries had missing/zero last_heard data"})
	}

	return logMsg, nil
}
