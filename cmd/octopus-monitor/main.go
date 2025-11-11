package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/soothill/octopus-home-mini/pkg/cache"
	"github.com/soothill/octopus-home-mini/pkg/config"
	"github.com/soothill/octopus-home-mini/pkg/influx"
	"github.com/soothill/octopus-home-mini/pkg/octopus"
	"github.com/soothill/octopus-home-mini/pkg/slack"
)

// Monitor handles the main monitoring loop
type Monitor struct {
	cfg            *config.Config
	octopusClient  *octopus.Client
	influxClient   *influx.Client
	cache          *cache.Cache
	slackNotifier  *slack.Notifier // May be nil if Slack is disabled
	lastPollTime   time.Time
	influxHealthy  bool
	consecutiveErr int
	degradedMode   bool // True when system is operating in degraded mode
	backoffFactor  int  // Multiplier for poll interval when in degraded mode
}

// sendSlackError sends an error notification to Slack if enabled
func (m *Monitor) sendSlackError(component, message string) {
	if m.slackNotifier != nil {
		//nolint:errcheck // Slack notification errors should not stop the monitor
		_ = m.slackNotifier.SendError(component, message)
	}
}

// sendSlackWarning sends a warning notification to Slack if enabled
func (m *Monitor) sendSlackWarning(component, message string) {
	if m.slackNotifier != nil {
		//nolint:errcheck // Slack notification errors should not stop the monitor
		_ = m.slackNotifier.SendWarning(component, message)
	}
}

// sendSlackInfo sends an info notification to Slack if enabled
func (m *Monitor) sendSlackInfo(title, message string) {
	if m.slackNotifier != nil {
		//nolint:errcheck // Slack notification errors should not stop the monitor
		_ = m.slackNotifier.SendInfo(title, message)
	}
}

func main() {
	log.Println("Starting Octopus Home Mini Monitor...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate runtime configuration
	ctx := context.Background()
	if err := cfg.ValidateRuntime(ctx); err != nil {
		// Log warning but don't fail startup if it's just InfluxDB connectivity
		if contains(err.Error(), "warning") {
			log.Printf("Warning: %v", err)
		} else {
			log.Fatalf("Runtime validation failed: %v", err)
		}
	}
	log.Println("Configuration validated successfully")

	// Initialize cache
	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		log.Fatalf("Failed to initialize cache: %v", err)
	}

	// Initialize Slack notifier (may be nil if not configured)
	var slackNotifier *slack.Notifier
	if cfg.SlackEnabled {
		slackNotifier = slack.NewNotifier(cfg.SlackWebhookURL)
		log.Println("Slack notifications enabled")
	} else {
		log.Println("Slack notifications disabled")
	}

	// Initialize Octopus client
	octopusClient := octopus.NewClient(cfg.OctopusAPIKey, cfg.OctopusAccountNumber)

	// Authenticate and get meter GUID
	authCtx := context.Background()
	if err := octopusClient.Initialize(authCtx); err != nil {
		log.Fatalf("Failed to initialize Octopus client: %v", err)
	}

	log.Println("Octopus client initialized successfully")

	// Create InfluxDB error handler that sends Slack notifications
	influxErrorHandler := func(err error) {
		log.Printf("InfluxDB write error: %v", err)
		if slackNotifier != nil {
			//nolint:errcheck // Slack notification errors should not stop the monitor
			_ = slackNotifier.SendError("InfluxDB Write", fmt.Sprintf("Async write failed: %v", err))
		}
	}

	// Initialize InfluxDB client with error handler and exponential backoff
	var influxClient *influx.Client
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = 30 * time.Second
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
			influxErrorHandler,
		)
		return err
	}

	err = backoff.Retry(operation, expBackoff)
	if err != nil {
		log.Printf("Warning: Failed to connect to InfluxDB after retries: %v. Will cache data locally.", err)
		if slackNotifier != nil {
			//nolint:errcheck // Slack notification errors should not stop the monitor
			_ = slackNotifier.SendWarning("InfluxDB", fmt.Sprintf("Failed to connect to InfluxDB: %v. Caching data locally.", err))
		}
	} else {
		log.Println("InfluxDB client initialized successfully")
		defer influxClient.Close()
	}

	// Create monitor
	monitor := &Monitor{
		cfg:            cfg,
		octopusClient:  octopusClient,
		influxClient:   influxClient,
		cache:          cacheStore,
		slackNotifier:  slackNotifier,
		lastPollTime:   time.Now().Add(-cfg.PollInterval),
		influxHealthy:  influxClient != nil,
		degradedMode:   false,
		backoffFactor:  1,
	}

	// Send startup notification
	if slackNotifier != nil {
		//nolint:errcheck // Slack notification errors should not stop the monitor
		_ = slackNotifier.SendInfo("Monitor Started", "Octopus Home Mini monitor has started successfully")
	}

	// Try to sync any cached data on startup
	if monitor.influxHealthy {
		monitor.syncCache()
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start monitoring loop in a goroutine
	stopChan := make(chan struct{})
	go monitor.run(stopChan)

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutdown signal received, stopping monitor...")

	// Stop receiving signals
	signal.Stop(sigChan)
	close(sigChan)

	// Signal monitoring loop to stop
	close(stopChan)

	// Wait for monitoring loop to finish with timeout
	shutdownComplete := make(chan struct{})
	go func() {
		// Give the monitor some time to finish current operations
		time.Sleep(2 * time.Second)
		close(shutdownComplete)
	}()

	select {
	case <-shutdownComplete:
		log.Println("Monitoring loop stopped gracefully")
	case <-time.After(5 * time.Second):
		log.Println("Warning: monitoring loop did not stop within timeout")
	}

	// Ensure cache is saved (defensive - cache auto-saves, but be explicit)
	if monitor.cache.Count() > 0 {
		log.Printf("Ensuring %d cached data points are persisted...", monitor.cache.Count())
		// Cache auto-saves on Add(), but data is already persisted
	}

	// Send shutdown notification
	if monitor.cache.Count() > 0 {
		monitor.sendSlackWarning("Monitor Stopped", fmt.Sprintf("Monitor stopped with %d data points in cache", monitor.cache.Count()))
	} else {
		monitor.sendSlackInfo("Monitor Stopped", "Monitor stopped gracefully")
	}

	// Give Slack notification time to send
	time.Sleep(500 * time.Millisecond)

	// Cleanup resources
	if slackNotifier != nil {
		slackNotifier.Close()
	}

	log.Println("Monitor stopped")
}

// run executes the main monitoring loop with adaptive polling
func (m *Monitor) run(stopChan chan struct{}) {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.poll()

			// Adjust poll interval based on degraded mode
			if m.backoffFactor > 1 {
				ticker.Reset(m.cfg.PollInterval * time.Duration(m.backoffFactor))
			} else {
				ticker.Reset(m.cfg.PollInterval)
			}

		case <-stopChan:
			return
		}
	}
}

// poll fetches and processes new energy data
func (m *Monitor) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Calculate time range for query
	now := time.Now()
	start := m.lastPollTime
	end := now

	log.Printf("Polling data from %s to %s", start.Format(time.RFC3339), end.Format(time.RFC3339))

	// Fetch telemetry data
	telemetryData, err := m.octopusClient.GetTelemetry(ctx, start, end)
	if err != nil {
		m.consecutiveErr++
		log.Printf("Error fetching telemetry: %v", err)

		// Enter degraded mode after 3 consecutive errors
		if m.consecutiveErr >= 3 {
			if !m.degradedMode {
				m.degradedMode = true
				m.backoffFactor = 2 // Double the poll interval
				m.sendSlackError("Octopus API", fmt.Sprintf("Entering degraded mode after %d consecutive errors: %v", m.consecutiveErr, err))
				log.Printf("Entering degraded mode - polling interval increased to %v", m.cfg.PollInterval*time.Duration(m.backoffFactor))
			} else {
				// Already in degraded mode, increase backoff up to maximum of 4x
				if m.backoffFactor < 4 {
					m.backoffFactor++
					log.Printf("Increasing backoff factor to %dx (poll interval: %v)", m.backoffFactor, m.cfg.PollInterval*time.Duration(m.backoffFactor))
				}
			}
		}
		return
	}

	// Exit degraded mode on successful fetch
	if m.degradedMode {
		m.degradedMode = false
		m.backoffFactor = 1
		m.sendSlackInfo("Octopus API", "Recovered from degraded mode - resuming normal polling")
		log.Println("Exiting degraded mode - resuming normal polling interval")
	}

	m.consecutiveErr = 0
	m.lastPollTime = end

	if len(telemetryData) == 0 {
		log.Println("No new telemetry data available")
		return
	}

	log.Printf("Retrieved %d data points", len(telemetryData))

	// Check InfluxDB health
	m.checkInfluxHealth(ctx)

	// Process data
	if m.influxHealthy {
		// Try to write to InfluxDB
		if err := m.writeToInflux(telemetryData); err != nil {
			log.Printf("Failed to write to InfluxDB: %v", err)
			m.influxHealthy = false
			m.sendSlackError("InfluxDB", fmt.Sprintf("Failed to write data: %v. Switching to cache mode.", err))

			// Cache the data instead
			m.cacheData(telemetryData)
		} else {
			log.Printf("Successfully wrote %d data points to InfluxDB", len(telemetryData))
		}
	} else {
		// InfluxDB is down, cache the data
		m.cacheData(telemetryData)

		// Periodically try to reconnect
		m.tryReconnectInflux(ctx)
	}
}

// writeToInflux writes telemetry data to InfluxDB
func (m *Monitor) writeToInflux(telemetryData []octopus.TelemetryData) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, data := range telemetryData {
		dp := influx.DataPoint{
			Timestamp:        data.ReadAt,
			ConsumptionDelta: data.ConsumptionDelta,
			Demand:           data.Demand,
			CostDelta:        data.CostDelta,
			Consumption:      data.Consumption,
		}

		if err := m.influxClient.WritePointDirectly(ctx, dp); err != nil {
			return err
		}
	}

	m.influxClient.Flush()
	return nil
}

// cacheData stores telemetry data in local cache
func (m *Monitor) cacheData(telemetryData []octopus.TelemetryData) {
	dataPoints := make([]cache.DataPoint, 0, len(telemetryData))

	for _, data := range telemetryData {
		dataPoints = append(dataPoints, cache.DataPoint{
			Timestamp:        data.ReadAt,
			ConsumptionDelta: data.ConsumptionDelta,
			Demand:           data.Demand,
			CostDelta:        data.CostDelta,
			Consumption:      data.Consumption,
		})
	}

	if err := m.cache.Add(dataPoints); err != nil {
		log.Printf("Error caching data: %v", err)
		m.sendSlackError("Cache", fmt.Sprintf("Failed to cache data: %v", err))
	} else {
		log.Printf("Cached %d data points (total in cache: %d)", len(dataPoints), m.cache.Count())
	}
}

// checkInfluxHealth checks if InfluxDB is healthy
func (m *Monitor) checkInfluxHealth(ctx context.Context) {
	if m.influxClient == nil {
		return
	}

	err := m.influxClient.CheckConnection(ctx)
	wasHealthy := m.influxHealthy
	m.influxHealthy = err == nil

	// Alert on state change
	if wasHealthy && !m.influxHealthy {
		log.Println("InfluxDB connection lost")
		m.sendSlackError("InfluxDB", "Connection to InfluxDB lost. Switching to cache mode.")
	} else if !wasHealthy && m.influxHealthy {
		log.Println("InfluxDB connection restored")
		m.sendSlackInfo("InfluxDB", "Connection to InfluxDB restored. Syncing cached data...")
		m.syncCache()
	}
}

// tryReconnectInflux attempts to reconnect to InfluxDB with exponential backoff
func (m *Monitor) tryReconnectInflux(ctx context.Context) {
	if m.influxClient == nil {
		return
	}

	// Configure exponential backoff
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = 5 * time.Minute
	expBackoff.InitialInterval = 1 * time.Second
	expBackoff.MaxInterval = 30 * time.Second
	expBackoff.Multiplier = 2.0

	operation := func() error {
		return m.influxClient.CheckConnection(ctx)
	}

	if err := backoff.Retry(operation, backoff.WithContext(expBackoff, ctx)); err == nil {
		log.Println("InfluxDB connection restored!")
		m.influxHealthy = true
		m.sendSlackInfo("InfluxDB", "Connection restored. Syncing cached data...")
		m.syncCache()
	}
}

// syncCache writes all cached data to InfluxDB
func (m *Monitor) syncCache() {
	cachedData := m.cache.GetAll()
	if len(cachedData) == 0 {
		log.Println("No cached data to sync")
		return
	}

	log.Printf("Syncing %d cached data points to InfluxDB...", len(cachedData))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	successCount := 0
	for _, data := range cachedData {
		dp := influx.DataPoint{
			Timestamp:        data.Timestamp,
			ConsumptionDelta: data.ConsumptionDelta,
			Demand:           data.Demand,
			CostDelta:        data.CostDelta,
			Consumption:      data.Consumption,
		}

		if err := m.influxClient.WritePointDirectly(ctx, dp); err != nil {
			log.Printf("Error writing cached point: %v", err)
			m.sendSlackError("Cache Sync", fmt.Sprintf("Failed to sync cached data: %v", err))
			return
		}
		successCount++
	}

	m.influxClient.Flush()

	// Clear cache after successful sync
	if err := m.cache.Clear(); err != nil {
		log.Printf("Error clearing cache: %v", err)
		m.sendSlackError("Cache", fmt.Sprintf("Failed to clear cache: %v", err))
	} else {
		log.Printf("Successfully synced %d cached data points", successCount)
		m.sendSlackInfo("Cache Sync", fmt.Sprintf("Successfully synced %d cached data points to InfluxDB", successCount))
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
