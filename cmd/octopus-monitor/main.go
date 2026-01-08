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

type application struct {
	config        *config.Config
	cache         *cache.Cache
	slackNotifier *slack.Notifier
	octopusClient *octopus.Client
	influxClient  *influx.Client
	healthServer  *health.Server
	monitor       *monitor.Monitor
}

func setupLogger(logLevelStr string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	logLevel, err := zerolog.ParseLevel(logLevelStr)
	if err != nil {
		log.Warn().Str("log_level", logLevelStr).Msg("Invalid log level, defaulting to 'info'")
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)
}

func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	ctx := context.Background()
	if err := cfg.ValidateRuntime(ctx); err != nil {
		// Log warning but don't fail startup if it's just InfluxDB connectivity
		if strings.Contains(err.Error(), "warning") {
			log.Warn().Err(err).Msg("Runtime validation warning")
		} else {
			return nil, fmt.Errorf("runtime validation failed: %w", err)
		}
	}

	return cfg, nil
}

func main() {
	log.Info().Msg("Starting Octopus Home Mini Monitor...")

	app, err := setupApplication()
	if err != nil {
		log.Fatal().Err(err).Msg("Application setup failed")
	}

	if app.influxClient != nil {
		defer app.influxClient.Close()
	}

	app.monitor.SendSlackInfo("Monitor Started", "Octopus Home Mini monitor has started successfully")
	app.monitor.SyncCache()

	var wg sync.WaitGroup
	stopChan := make(chan struct{})
	runGoroutines(&wg, app.monitor, stopChan)

	waitForShutdown(stopChan)
	app.shutdown(&wg)
}

func setupApplication() (*application, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	setupLogger(cfg.LogLevel)

	cacheStore, err := initCache(cfg.CacheDir)
	if err != nil {
		return nil, err
	}

	slackNotifier := initSlackNotifier(cfg.SlackEnabled, cfg.SlackWebhookURL)

	octopusClient, err := initOctopusClient(cfg.OctopusAPIKey, cfg.OctopusAccountNumber)
	if err != nil {
		return nil, err
	}

	influxClient, err := initInfluxClient(cfg, slackNotifier)
	if err != nil {
		log.Warn().Err(err).Msg("InfluxDB client initialization failed")
	}

	appMonitor := monitor.New(cfg, octopusClient, influxClient, cacheStore, slackNotifier)

	healthServer := health.NewServer(cfg.HealthServerAddr, "1.0.0")
	setupHealthChecks(healthServer, influxClient, octopusClient, cacheStore)
	if err := healthServer.Start(); err != nil {
		log.Warn().Err(err).Msg("Failed to start health server")
	}

	return &application{
		config:        cfg,
		cache:         cacheStore,
		slackNotifier: slackNotifier,
		octopusClient: octopusClient,
		influxClient:  influxClient,
		healthServer:  healthServer,
		monitor:       appMonitor,
	}, nil
}

func initCache(cacheDir string) (*cache.Cache, error) {
	cacheStore, err := cache.NewCache(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}
	return cacheStore, nil
}

func initSlackNotifier(enabled bool, webhookURL string) *slack.Notifier {
	if !enabled {
		log.Info().Msg("Slack notifications disabled")
		return nil
	}
	log.Info().Msg("Slack notifications enabled")
	return slack.NewNotifier(webhookURL)
}

func initOctopusClient(apiKey, accountNumber string) (*octopus.Client, error) {
	octopusClient := octopus.NewClient(apiKey, accountNumber)
	ctx := context.Background()
	if err := octopusClient.Initialize(ctx); err != nil {
		return nil, err
	}
	log.Info().Msg("Octopus client initialized successfully")
	return octopusClient, nil
}

func initInfluxClient(cfg *config.Config, slackNotifier *slack.Notifier) (*influx.Client, error) {
	influxErrorHandler := func(err error) {
		log.Error().Err(err).Msg("InfluxDB write error")
		if slackNotifier != nil {
			if err := slackNotifier.SendError("InfluxDB Write", fmt.Sprintf("Async write failed: %v", err)); err != nil {
				log.Error().Err(err).Msg("Error sending Slack error notification for InfluxDB")
			}
		}
	}

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

	err := backoff.Retry(operation, expBackoff)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to InfluxDB after retries. Will cache data locally.")
		if slackNotifier != nil {
			if err := slackNotifier.SendWarning("InfluxDB", fmt.Sprintf("Failed to connect to InfluxDB: %v. Caching data locally.", err)); err != nil {
				log.Error().Err(err).Msg("Error sending Slack warning notification for InfluxDB connection failure")
			}
		}
		return nil, err
	}

	log.Info().Msg("InfluxDB client initialized successfully")
	return influxClient, nil
}

func setupHealthChecks(healthServer *health.Server, influxClient *influx.Client, octopusClient *octopus.Client, cacheStore *cache.Cache) {
	if influxClient != nil {
		healthServer.RegisterChecker("influxdb", health.ContextChecker("InfluxDB", func(ctx context.Context) error {
			return influxClient.CheckConnection(ctx)
		}))
	}

	healthServer.RegisterChecker("octopus_api", health.SimpleChecker("Octopus API", func() error {
		if octopusClient == nil {
			return fmt.Errorf("octopus client not initialized")
		}
		return nil
	}))

	healthServer.RegisterChecker("cache", health.SimpleChecker("Cache", func() error {
		if cacheStore == nil {
			return fmt.Errorf("cache not initialized")
		}
		return nil
	}))
}

func runGoroutines(wg *sync.WaitGroup, appMonitor *monitor.Monitor, stopChan chan struct{}) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		appMonitor.Run(stopChan)
	}()

	if appMonitor.Cfg.CacheCleanupEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			appMonitor.RunCacheCleanup(stopChan)
		}()
		log.Info().
			Dur("interval", appMonitor.Cfg.CacheCleanupInterval).
			Int("retention_days", appMonitor.Cfg.CacheRetentionDays).
			Msg("Cache cleanup enabled")
	}
}

func waitForShutdown(stopChan chan struct{}) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	log.Info().Msg("Shutdown signal received, stopping monitor...")
	close(stopChan)
}

func gracefulShutdown(wg *sync.WaitGroup, timeout time.Duration) {
	shutdownComplete := make(chan struct{})
	go func() {
		wg.Wait()
		close(shutdownComplete)
	}()

	select {
	case <-shutdownComplete:
		log.Info().Msg("All services stopped gracefully")
	case <-time.After(timeout):
		log.Warn().Msg("Shutdown timed out")
	}
}

func (app *application) shutdown(wg *sync.WaitGroup) {
	gracefulShutdown(wg, app.config.ShutdownTimeout)

	if app.monitor.Cache.Count() > 0 {
		app.monitor.SendSlackWarning("Monitor Stopped", fmt.Sprintf("Monitor stopped with %d data points in cache", app.monitor.Cache.Count()))
	} else {
		app.monitor.SendSlackInfo("Monitor Stopped", "Monitor stopped gracefully")
	}

	time.Sleep(500 * time.Millisecond)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := app.healthServer.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error stopping health server")
	}

	if app.slackNotifier != nil {
		app.slackNotifier.Close()
	}

	log.Info().Msg("Monitor stopped")
}
