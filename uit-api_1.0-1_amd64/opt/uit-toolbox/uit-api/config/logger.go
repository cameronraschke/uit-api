package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type logLevelRangeHandler struct {
	handler  slog.Handler
	minLevel slog.Level
	maxLevel slog.Level
}

func newLevelRangeHandler(handler slog.Handler, minLevel slog.Level, maxLevel slog.Level) slog.Handler {
	return &logLevelRangeHandler{handler: handler, minLevel: minLevel, maxLevel: maxLevel}
}

func (handler *logLevelRangeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level < handler.minLevel || level > handler.maxLevel {
		return false
	}
	return handler.handler.Enabled(ctx, level)
}

func (handler *logLevelRangeHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level < handler.minLevel || record.Level > handler.maxLevel {
		return nil
	}
	return handler.handler.Handle(ctx, record)
}

func (handler *logLevelRangeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logLevelRangeHandler{
		handler:  handler.handler.WithAttrs(attrs),
		minLevel: handler.minLevel,
		maxLevel: handler.maxLevel,
	}
}

func (handler *logLevelRangeHandler) WithGroup(name string) slog.Handler {
	return &logLevelRangeHandler{
		handler:  handler.handler.WithGroup(name),
		minLevel: handler.minLevel,
		maxLevel: handler.maxLevel,
	}
}

func (as *AppState) initLogger() error {
	if as == nil {
		return fmt.Errorf("app state is nil in initLogger")
	}
	// Set logger to nil initially
	as.appLogger.Store(nil)

	removeTime := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey && len(groups) == 0 {
			return slog.Attr{}
		}
		return a
	}

	if err := initLogBuffers(); err != nil {
		return fmt.Errorf("failed to initialize log buffers: %w", err)
	}

	stdoutTextHandler := newLevelRangeHandler(
		slog.NewTextHandler(StdoutBuffer.Load(), &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: removeTime,
		}),
		slog.LevelInfo,
		slog.LevelInfo,
	)
	stderrTextHandler := newLevelRangeHandler(
		slog.NewTextHandler(StderrBuffer.Load(), &slog.HandlerOptions{
			Level:       slog.LevelWarn,
			ReplaceAttr: removeTime,
		}),
		slog.LevelWarn,
		slog.LevelError,
	)
	// jsonLogFile, err := os.OpenFile("/var/log/uit-web/"+time.Now().Format("2006-01-02_15-04-05")+".json.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o0640)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to open JSON log file: %w", err)
	// }
	// jsonFileHandler := newLevelRangeHandler(
	// 	slog.NewJSONHandler(jsonLogFile, &slog.HandlerOptions{Level: slog.LevelInfo}),
	// 	slog.LevelInfo,
	// 	slog.LevelError,
	// )

	// multiHandler := slog.NewMultiHandler(stdoutTextHandler, stderrTextHandler, jsonFileHandler)
	multiHandler := slog.NewMultiHandler(stdoutTextHandler, stderrTextHandler)
	logger := slog.New(multiHandler)
	slog.SetDefault(logger)
	as.appLogger.Store(logger)
	return nil
}

func newDefaultLogger() *slog.Logger {
	removeTime := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey && len(groups) == 0 {
			return slog.Attr{}
		}
		return a
	}

	stdoutTextHandler := newLevelRangeHandler(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: removeTime,
		}),
		slog.LevelInfo,
		slog.LevelInfo,
	)

	multiHandler := slog.NewMultiHandler(stdoutTextHandler)
	logger := slog.New(multiHandler)
	return logger
}

// This is not a method of AppState because we want the most updated version of AppState to be used on every call
func GetLogger() *slog.Logger {
	as, err := GetAppState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] unable to get AppState in func GetLogger, returning default logger: %v\n", err)
		newLogger := newDefaultLogger()
		slog.SetDefault(newLogger)
		return newLogger
	}
	logger := as.appLogger.Load()
	if logger == nil {
		fmt.Fprintf(os.Stderr, "[ERROR] logger is nil in GetLogger, using default logger")
		newLogger := newDefaultLogger()
		as.appLogger.Store(newLogger)
		slog.SetDefault(newLogger)
		return newLogger
	}

	return logger
}
