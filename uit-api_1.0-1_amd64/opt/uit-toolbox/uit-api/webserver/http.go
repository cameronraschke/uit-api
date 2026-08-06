package webserver

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"
	"uit-api/endpoints"
	"uit-api/logger"
)

func StartFileServer(ctx context.Context, serverHost string) error {
	log := logger.GetLogger().With(slog.String("func", "StartFileServer"))
	httpBaseChain := endpoints.NewChain(
		endpoints.StoreLoggerMiddleware,
		endpoints.PanicRecoveryMiddleware,
		endpoints.LimitRequestSizeMiddleware,
		endpoints.StoreClientIPMiddleware,
		endpoints.AllowIPRangeMiddleware("lan"),
		endpoints.WebEndpointConfigMiddleware,
		endpoints.TLSMiddleware,
		endpoints.CheckHttpVersionMiddleware,
		endpoints.RateLimitMiddleware("file"),
		endpoints.FileServerTimeoutMiddleware,
		endpoints.HTTPMethodMiddleware,
		endpoints.CheckValidURLMiddleware,
		endpoints.CheckForRedirectsMiddleware,
		endpoints.CheckHeadersMiddleware,
		endpoints.SetHeadersMiddleware,
	)

	fileServerChain := httpBaseChain.Append(
		endpoints.AllowedFilesMiddleware,
	)

	httpMux := http.NewServeMux()
	httpMux.Handle("/client/", fileServerChain.ThenFunc(endpoints.FileServerHandler))
	httpMux.Handle("/client", fileServerChain.ThenFunc(endpoints.RejectRequest))
	httpMux.Handle("/", httpBaseChain.ThenFunc(endpoints.RejectRequest))

	httpServer := &http.Server{
		Addr:           serverHost + ":8080",
		Handler:        httpMux,
		ReadTimeout:    1 * time.Minute,
		WriteTimeout:   1 * time.Minute,
		IdleTimeout:    2 * time.Minute,
		MaxHeaderBytes: 1 << 20,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx // Propagate cancellation to requests
		},
	}

	return startWebServer(
		ctx,
		log,
		"HTTP file server",
		httpServer.ListenAndServe,
		httpServer.Shutdown,
		1*time.Minute,
	)
}
