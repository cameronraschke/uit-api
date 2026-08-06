package webserver

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"time"
	"uit-api/logger"
)

func StartPprofServer(ctx context.Context) error {
	log := logger.GetLogger().With(slog.String("func", "StartPprofServer"))
	pprofServer := &http.Server{Addr: "localhost:6060", Handler: nil}

	return startWebServer(
		ctx,
		log,
		"pprof server",
		pprofServer.ListenAndServe,
		pprofServer.Shutdown,
		5*time.Second,
	)
}
