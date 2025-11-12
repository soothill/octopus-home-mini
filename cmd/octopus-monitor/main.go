package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/soothill/octopus-home-mini/pkg/cache"
	"github.com/soothill/octopus-home-mini/pkg/config"
	"github.com/soothill/octopus-home-mini/pkg/health"
	"github.com/soothill/octopus-home-mini/pkg/influx"
	"github.com/soothill/octopus-home-mini/pkg/monitor"
	"github.com/soothill/octopus-home-mini/pkg/octopus"
	"github.com/soothill/octopus-home-mini/pkg/slack"
)

func main() {
	// Configure logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	log.Info().Msg("Starting Octopus Home Mini Monitor...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Set log level from config
	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warn().Str("log_level", cfg.LogLevel).Msg("Invalid log level, defaulting to 'info'")
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)

	// Validate runtime configuration
	ctx := context.Background()
	if err := cfg.ValidateRuntime(ctx); err != nil {
		// Log warning but don't fail startup if it's just InfluxDB connectivity
		if strings.Contains(err.Error(), "warning") {
			log.Warn().Err(err).Msg("Runtime validation warning")
		} else {
			log.Fatal().Err(err).Msg("Runtime validation failed")
		}
	}
	log.Info().Msg("Configuration validated successfully")

	// Initialize cache
	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize cache")
	}

	// Initialize Slack notifier (may be nil if not configured)
	var slackNotifier *slack.Notifier
	if cfg.SlackEnabled {
		slackNotifier = slack.NewNotifier(cfg.SlackWebhookURL)
		log.Info().Msg("Slack notifications enabled")
	} else {
		log.Info().Msg("Slack notifications disabled")
	}

	// Initialize Octopus client
	octopusClient := octopus.NewClient(cfg.OctopusAPIKey, cfg.OctopusAccountNumber)

	// Authenticate and get meter GUID
	authCtx := context.Background()
	if err := octopusClient.Initialize(authCtx); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize Octopus client")
	}

	log.Info().Msg("Octopus client initialized successfully")

	// Create InfluxDB error handler that sends Slack notifications
	influxErrorHandler := func(err error) {
		log.Error().Err(err).Msg("InfluxDB write error")
		if slackNotifier != nil {
			if err := slackNotifier.SendError("InfluxDB Write", fmt.Sprintf("Async write failed: %v", err)); err != nil {
				log.Error().Err(err).Msg("Error sending Slack error notification for InfluxDB")
			}
		}
	}

	// Initialize InfluxDB client with error handler and exponential backoff
	var influxClient *influx.Client
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = cfg.InfluxConnectTimeout
	expBackoff.InitialInterval = 1 * time.Second
	expBackoff.MaxInterval = 5 * time.Second
	expBackoff.Multiplier = 2.0

	operation := func() error {
		var err error
		influxClient, err = influx.NewClientWithErrorHandler(
			cfg.InfluxDBURL,
			cfg.InfluxDBToken,
			cfg.InfluxDBOrg,
			cfg.InfluxDBBucket,
			cfg.InfluxDBMeasurement,
			influxErrorHandler,
		)
		return err
	}

	err = backoff.Retry(operation, expBackoff)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to InfluxDB after retries. Will cache data locally.")
		if slackNotifier != nil {
			if err := slackNotifier.SendWarning("InfluxDB", fmt.Sprintf("Failed to connect to InfluxDB: %v. Caching data locally.", err)); err != nil {
				log.Error().Err(err).Msg("Error sending Slack warning notification for InfluxDB connection failure")
			}
		}
	} else {
		log.Info().Msg("InfluxDB client initialized successfully")
		defer influxClient.Close()
	}

	// Create monitor
	appMonitor := monitor.New(cfg, octopusClient, influxClient, cacheStore, slackNotifier)

	// Initialize and start health check server
	healthServer := health.NewServer(cfg.HealthServerAddr, "1.0.0")

	// Register health checkers
	if influxClient != nil {
		healthServer.RegisterChecker("influxdb", health.ContextChecker("InfluxDB", func(ctx context.Context) error {
			return influxClient.CheckConnection(ctx)
		}))
	}

	healthServer.RegisterChecker("octopus_api", health.SimpleChecker("Octopus API", func() error {
		// Simple check - if the client is initialized, it's considered healthy
		// More sophisticated checks could be added here
		if octopusClient == nil {
			return fmt.Errorf("octopus client not initialized")
		}
		return nil
	}))

	healthServer.RegisterChecker("cache", health.SimpleChecker("Cache", func() error {
		// Check if cache is accessible
		if cacheStore == nil {
			return fmt.Errorf("cache not initialized")
		}
		return nil
	}))

	if err := healthServer.Start(); err != nil {
		log.Warn().Err(err).Msg("Failed to start health server")
	}

	// Send startup notification
	appMonitor.SendSlackInfo("Monitor Started", "Octopus Home Mini monitor has started successfully")

	// Try to sync any cached data on startup
	appMonitor.SyncCache()

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start monitoring loop in a goroutine
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		appMonitor.Run(stopChan)
	}()

	// Start cache cleanup goroutine if enabled
	if cfg.CacheCleanupEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			appMonitor.RunCacheCleanup(stopChan)
		}()
		log.Info().
			Dur("interval", cfg.CacheCleanupInterval).
			Int("retention_days", cfg.CacheRetentionDays).
			Msg("Cache cleanup enabled")
	}

	// Wait for shutdown signal
	<-sigChan
	log.Info().Msg("Shutdown signal received, stopping monitor...")

	// Stop receiving signals
	signal.Stop(sigChan)
	close(sigChan)

	// Signal goroutines to stop
	close(stopChan)

	// Wait for goroutines to finish with timeout
	shutdownComplete := make(chan struct{})
	go func() {
		wg.Wait()
		close(shutdownComplete)
	}()

	select {
	case <-shutdownComplete:
		log.Info().Msg("All services stopped gracefully")
	case <-time.After(cfg.ShutdownTimeout):
		log.Warn().Msg("Shutdown timed out")
	}

	// Ensure cache is saved (defensive - cache auto-saves, but be explicit)
	if appMonitor.Cache.Count() > 0 {
		log.Info().Int("count", appMonitor.Cache.Count()).Msg("Ensuring cached data points are persisted...")
		// Cache auto-saves on Add(), but data is already persisted
	}

	// Send shutdown notification
	if appMonitor.Cache.Count() > 0 {
		appMonitor.SendSlackWarning("Monitor Stopped", fmt.Sprintf("Monitor stopped with %d data points in cache", appMonitor.Cache.Count()))
	} else {
		appMonitor.SendSlackInfo("Monitor Stopped", "Monitor stopped gracefully")
	}

	// Give Slack notification time to send
	time.Sleep(500 * time.Millisecond)

	// Stop health check server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := healthServer.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error stopping health server")
	}

	// Cleanup resources
	if slackNotifier != nil {
		slackNotifier.Close()
	}

	log.Info().Msg("Monitor stopped")
}
