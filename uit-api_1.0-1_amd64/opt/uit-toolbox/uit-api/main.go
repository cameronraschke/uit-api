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

func replaceAllRealtimeDataFromDB(ctx context.Context) error {
	res, err := database.GetAllLiveOSData(ctx)
	for tag, data := range res {
		if err := appstate.ReplaceClientRealtimeData(tag, &data); err != nil {
			return fmt.Errorf("failed to update client realtime data for tag %d: %w", tag, err)
		}
	}
	return err
}

func main() {
	fmt.Fprintln(os.Stdout, "Starting UIT Web...")

	// Create root context
	rootCtx, stopRootCtx := signal.NotifyContext(
		context.Background(),
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGABRT,
		syscall.SIGTERM,
	)
	defer stopRootCtx()

	startTime := time.Now()
	fmt.Fprintln(os.Stdout, "Server time: "+startTime.Format("01-02-2006 15:04:05"))

	// Recover from panics
	defer func() {
		if recoveryErr := recover(); recoveryErr != nil {
			fmt.Fprintln(os.Stderr, "Panic: "+fmt.Sprint(recoveryErr))
			fmt.Fprintln(os.Stderr, "Stack:\n"+string(debug.Stack()))
			time.Sleep(100 * time.Millisecond) // Buffer
			stopRootCtx()                      // Cancel context to stop all goroutines
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
	// Load all client realtime data from DB into app state
	if err := replaceAllRealtimeDataFromDB(rootCtx); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to load client realtime data from DB: "+err.Error())
		os.Exit(1)
	}
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
		log.Errorf("Failed to retrieve HTTP server IP: %v", err)
		logger.FlushLogBuffers()
		os.Exit(1)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 10) // Buffer in case of multiple errors

	wg.Go(func() {
		if err := webserver.StartPprofServer(rootCtx); err != nil {
			select {
			case errChan <- err:
			default:
				log.Warnf("Error channel full, cannot send pprof server error (func main - StartPprofServer)")
			}
		}
	})

	// Start HTTP server
	log.Infof("Starting HTTP server on http://%s:8080", httpHost)
	wg.Go(func() {
		if err := webserver.StartFileServer(rootCtx, httpHost); err != nil {
			select {
			case errChan <- err:
			default:
				log.Warnf("Error channel full, cannot send HTTP server error (func main - StartFileServer)")
			}
		}
	})

	// Start HTTPS server
	log.Infof("Starting the HTTPS server on https://*:31411")
	wg.Go(func() {
		if err := webserver.StartWebServer(rootCtx); err != nil {
			select {
			case errChan <- err:
			default:
				log.Warnf("Error channel full, cannot send HTTPS server error (func main - StartWebServer)")
			}
		}
	})

	// Start background processes
	wg.Go(func() {
		defer func() {
			if recoveryErr := recover(); recoveryErr != nil {
				log.Errorf("Background process panic: %v", recoveryErr)
				log.Errorf("Stack:\n%v", string(debug.Stack()))
				select {
				case errChan <- fmt.Errorf("background process panic: %v", recoveryErr):
				default:
					log.Warnf("Error channel full, cannot send panic error (func main - startBackgroundProcesses)")
				}
			}
		}()
		log.Infof("Starting background processes...")
		startBackgroundProcesses(rootCtx, errChan)
	})

	log.Infof("Servers started in: %dms", time.Since(startTime).Milliseconds())
	logger.FlushLogBuffers()

	// Wait for shutdown signal or error
	select {
	case <-rootCtx.Done():
		log.Infof("Shutdown signal received.")
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 20*time.Second)
		writeLastHeardToDB(flushCtx, 1*time.Second) // Write last_heard values to DB on shutdown
		flushCancel()
	case err := <-errChan:
		log.Errorf("Error received: %v", err)
		stopRootCtx() // Cancel context to stop all goroutines
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
		log.Infof("All goroutines stopped gracefully")
	case <-waitCtx.Done():
		log.Errorf("Shutdown timeout reached: %v", waitCtx.Err())
		log.Errorf("Forcing process exit; goroutines still running")
		log.Errorf("Goroutine dump: \n%v", string(debug.Stack()))
	}

	log.Infof("UIT Web has been stopped.")
	logger.FlushLogBuffers()
}
