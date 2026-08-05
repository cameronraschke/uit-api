package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"
	"uit-api/appstate"
	"uit-api/auth"
	"uit-api/config"
	"uit-api/database"
	"uit-api/endpoints"
	"uit-api/logger"
	"uit-api/webserver"
)

func main() {
	fmt.Fprintln(os.Stdout, "Starting UIT Web...")

	// Create root context
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGABRT,
		syscall.SIGTERM,
	)
	defer stop()

	startTime := time.Now()
	fmt.Fprintln(os.Stdout, "Server time: "+startTime.Format("01-02-2006 15:04:05"))

	// Recover from panics
	defer func() {
		if recoveryErr := recover(); recoveryErr != nil {
			fmt.Fprintln(os.Stderr, "Panic: "+fmt.Sprint(recoveryErr))
			fmt.Fprintln(os.Stderr, "Stack:\n"+string(debug.Stack()))
			time.Sleep(100 * time.Millisecond) // Buffer
			stop()                             // Cancel context to stop all goroutines
			return
		}
	}()

	// Initialize application
	if err := logger.InitLogger(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize logger: "+err.Error())
		os.Exit(1)
	}
	if err := config.InitAppConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize application configuration: "+err.Error())
		os.Exit(1)
	}
	if err := endpoints.InitEndpointConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize endpoint configuration: "+err.Error())
		os.Exit(1)
	}
	if err := appstate.InitAppState(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize application state: "+err.Error())
		os.Exit(1)
	}
	dbPool, pgxPool, err := database.InitDatabasePools()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize database pools: "+err.Error())
		os.Exit(1)
	}
	defer dbPool.Close()
	defer pgxPool.Close()
	if err := auth.InitRateLimiters(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize rate limiters: "+err.Error())
		os.Exit(1)
	}
	if err := auth.InitAuthSessions(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize authentication sessions: "+err.Error())
		os.Exit(1)
	}

	log := logger.GetLogger().With(slog.String("func", "main"))
	logger.FlushLogBuffers()

	httpHost, _, err := config.GetWebServerIPs()
	if err != nil || strings.TrimSpace(httpHost) == "" {
		log.Error("Failed to retrieve HTTP server IP: " + err.Error())
		logger.FlushLogBuffers()
		os.Exit(1)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 10) // Buffer in case of multiple errors

	wg.Go(func() {
		if err := webserver.StartPprofServer(ctx); err != nil {
			select {
			case errChan <- err:
			default:
				log.Warn("Error channel full, cannot send pprof server error (func main - StartPprofServer)")
			}
		}
	})

	// Start HTTP server
	log.Info("Starting HTTP server on http://" + httpHost + ":8080")
	wg.Go(func() {
		if err := webserver.StartFileServer(ctx, httpHost); err != nil {
			select {
			case errChan <- err:
			default:
				log.Warn("Error channel full, cannot send HTTP server error (func main - StartFileServer)")
			}
		}
	})

	// Start HTTPS server
	log.Info("Starting the HTTPS server on https://*:31411")
	wg.Go(func() {
		if err := webserver.StartWebServer(ctx); err != nil {
			select {
			case errChan <- err:
			default:
				log.Warn("Error channel full, cannot send HTTPS server error (func main - StartWebServer)")
			}
		}
	})

	// Start background processes
	wg.Go(func() {
		defer func() {
			if recoveryErr := recover(); recoveryErr != nil {
				log.Error("Background process panic: " + fmt.Sprintf("%v", recoveryErr))
				log.Error("Stack:\n" + string(debug.Stack()))
				select {
				case errChan <- fmt.Errorf("background process panic: %v", recoveryErr):
				default:
					log.Warn("Error channel full, cannot send panic error (func main - startBackgroundProcesses)")
				}
			}
		}()
		log.Info("Starting background processes...")
		startBackgroundProcesses(ctx, errChan)
	})

	log.Info("Servers started in: " + time.Since(startTime).String())
	logger.FlushLogBuffers()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		log.Info("Shutdown signal received.")
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 20*time.Second)
		writeLastHeardToDB(flushCtx, 1*time.Second) // Write last_heard values to DB on shutdown
		flushCancel()
	case err := <-errChan:
		log.Error("Error received: " + err.Error())
		stop() // Cancel context to stop all goroutines
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 20*time.Second)
		writeLastHeardToDB(flushCtx, 1*time.Second) // Write last_heard values to DB on shutdown
		flushCancel()
	}
	logger.FlushLogBuffers()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer waitCancel()

	wgDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(wgDone)
	}()

	select {
	case <-wgDone:
		log.Info("All goroutines stopped gracefully")
	case <-waitCtx.Done():
		log.Error("Shutdown timeout reached: " + waitCtx.Err().Error())
		log.Error("Forcing process exit; goroutines still running")
		log.Error("Goroutine dump:\n" + string(debug.Stack()))
	}

	log.Info("UIT Web has been stopped.")
	logger.FlushLogBuffers()
}
