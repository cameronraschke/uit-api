package main

import (
	"context"
	"flag"
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
	if err != nil {
		return fmt.Errorf("failed to retrieve all live OS data from DB: %w", err)
	}
	for tag, data := range res {
		if err := appstate.ReplaceClientRealtimeData(tag, data); err != nil {
			return fmt.Errorf("failed to update client realtime data for tag %d: %w", tag, err)
		}
	}
	return err
}

func main() {
	debugMode := flag.Bool("debug", false, "--debug")
	flag.Parse()

	fmt.Fprintln(os.Stdout, "starting UIT API...")

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
	fmt.Fprintln(os.Stdout, "server time: "+startTime.Format("01-02-2006 15:04:05"))

	// Recover from panics
	defer func() {
		if recoveryErr := recover(); recoveryErr != nil {
			fmt.Fprintln(os.Stderr, "panic: "+fmt.Sprint(recoveryErr))
			fmt.Fprintln(os.Stderr, "stack:\n"+string(debug.Stack()))
			time.Sleep(100 * time.Millisecond) // Buffer
			stopRootCtx()                      // Cancel context to stop all goroutines
			return
		}
	}()

	// Initialize application
	var initApp sync.Once
	initApp.Do(func() {
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
		if err := auth.InitRateLimiters(); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to initialize rate limiters: "+err.Error())
			os.Exit(1)
		}
		if err := auth.InitAuthSessions(); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to initialize authentication sessions: "+err.Error())
			os.Exit(1)
		}
		// We can't defer closing db pools because of sync.Once, done later in main()
		dbPool, pgxPool, err := database.InitDatabasePools()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to initialize database pools: "+err.Error())
			dbPool.Close()
			pgxPool.Close()
			os.Exit(1)
		}
		// Load all client realtime data from DB into app state
		if err := replaceAllRealtimeDataFromDB(rootCtx); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to load client realtime data from DB: "+err.Error())
			dbPool.Close()
			pgxPool.Close()
			os.Exit(1)
		}
	})
	// Make sure db pools close on exit
	dbPool, _ := database.GetDatabasePool()
	defer dbPool.Close()
	pgxPool, _ := database.GetPGXPool()
	defer pgxPool.Close()

	log := logger.GetLogger().With(slog.String("func", "main"))
	logger.FlushLogBuffers()

	httpHost, _, err := config.GetWebServerIPs()
	if err != nil || strings.TrimSpace(httpHost) == "" {
		log.Errorf("failed to retrieve an HTTP server IP from config: %v", err)
		logger.FlushLogBuffers()
		os.Exit(1)
	}

	var wg sync.WaitGroup
	rootErrChan := make(chan error, 10) // Buffer in case of multiple errors

	log.Infof("starting pprof HTTP server on http://localhost:6060...")
	wg.Go(func() {
		if err := webserver.StartPprofServer(rootCtx); err != nil {
			select {
			case rootErrChan <- err:
			default:
				log.Warnf("root error channel full, cannot log error from pprof server")
			}
		}
	})

	// Start HTTP server
	log.Infof("starting HTTP server on http://%s:8080...", httpHost)
	wg.Go(func() {
		if err := webserver.StartFileServer(rootCtx, httpHost); err != nil {
			select {
			case rootErrChan <- err:
			default:
				log.Warnf("root error channel full, cannot log error from HTTP server")
			}
		}
	})

	// Start HTTPS server
	log.Infof("starting HTTPS server on https://*:31411")
	wg.Go(func() {
		if err := webserver.StartWebServer(rootCtx, *debugMode); err != nil {
			select {
			case rootErrChan <- err:
			default:
				log.Errorf("root error channel full, cannot log error from HTTPS server")
			}
		}
	})

	// Start background processes
	wg.Go(func() {
		defer func() {
			if recoveryErr := recover(); recoveryErr != nil {
				log.Errorf("background process panic: %v", recoveryErr)
				log.Errorf("stack:\n%v", string(debug.Stack()))
				select {
				case rootErrChan <- fmt.Errorf("panic received from background process: %v", recoveryErr):
				default:
					log.Errorf("root error channel full, cannot send panic from background processes")
				}
			}
		}()
		log.Infof("starting background processes...")
		initBackgroundProcesses(rootCtx, rootErrChan)
	})

	logger.FlushLogBuffers()

	// Wait for shutdown signal or error
	select {
	case <-rootCtx.Done():
		log.Infof("shutdown signal received from OS: %v", rootCtx.Err())
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 20*time.Second)
		log.Infof("writing last_heard values to DB on shutdown...")
		writeLastHeardToDB(flushCtx, 0) // Write last_heard values to DB on shutdown, no delay
		flushCancel()
	case err, ok := <-rootErrChan:
		if !ok {
			log.Errorf("root error channel closed unexpectedly")
			break
		}
		log.Errorf("error received from root error channel: %v", err)
		stopRootCtx() // Cancel context to stop all goroutines
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 20*time.Second)
		log.Infof("writing last_heard values to DB on shutdown...")
		writeLastHeardToDB(flushCtx, 0) // Write last_heard values to DB on shutdown, no delay
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
		log.Infof("all goroutines stopped gracefully")
	case <-waitCtx.Done():
		log.Errorf("shutdown timeout reached: %v", waitCtx.Err())
		log.Errorf("forcing process exit; goroutines still running")
		log.Errorf("goroutine dump: \n%v", string(debug.Stack()))
	}

	logger.FlushLogBuffers()
	log.Infof("UIT API has been stopped gracefully")
	logger.FlushLogBuffers()
}
