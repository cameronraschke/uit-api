package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
)

type Slogger struct {
	*slog.Logger
}

func NewSlogger(l *slog.Logger) *Slogger {
	if l == nil {
		l = slog.Default()
	}
	return &Slogger{Logger: l}
}

func (l *Slogger) Debugf(format string, args ...any) {
	l.Log(context.Background(), slog.LevelDebug, fmt.Sprintf(format, args...))
}

func (l *Slogger) Infof(format string, args ...any) {
	l.Log(context.Background(), slog.LevelInfo, fmt.Sprintf(format, args...))
}

func (l *Slogger) Warnf(format string, args ...any) {
	l.Log(context.Background(), slog.LevelWarn, fmt.Sprintf(format, args...))
}

func (l *Slogger) Errorf(format string, args ...any) {
	l.Log(context.Background(), slog.LevelError, fmt.Sprintf(format, args...))
}

func (l *Slogger) Logf(ctx context.Context, level slog.Level, format string, args ...any) {
	l.Log(ctx, level, fmt.Sprintf(format, args...))
}

func (l *Slogger) WithAttrs(attrs ...any) *Slogger {
	return &Slogger{Logger: l.Logger.With(attrs...)}
}

func (l *Slogger) WithGroup(name string) *Slogger {
	return &Slogger{Logger: l.Logger.WithGroup(name)}
}

func (l *Slogger) With(args ...any) *Slogger {
	return &Slogger{Logger: l.Logger.With(args...)}
}

type logLevelRangeHandler struct {
	handler  slog.Handler
	minLevel slog.Level
	maxLevel slog.Level
}

var (
	appLoggerInstance atomic.Pointer[Slogger]
)

func InitLogger() error {
	// Set logger to nil initially
	appLoggerInstance.Store(nil)

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
	appLoggerInstance.Store(NewSlogger(logger))
	return nil
}

func GetLogger() *Slogger {
	slogger := appLoggerInstance.Load()
	if slogger == nil {
		fmt.Fprintf(os.Stderr, "[ERROR] logger is nil in GetLogger, returning default logger")
		newLogger := newDefaultLogger()
		newSlogger := NewSlogger(newLogger)
		appLoggerInstance.Store(newSlogger)
		slog.SetDefault(newLogger)
		return newSlogger
	}
	return slogger
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
