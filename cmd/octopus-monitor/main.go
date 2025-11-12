package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/soothill/octopus-home-mini/pkg/cache"
	"github.com/soothill/octopus-home-mini/pkg/config"
	"github.com/soothill/octopus-home-mini/pkg/health"
	"github.com/soothill/octopus-home-mini/pkg/influx"
	"github.com/soothill/octopus-home-mini/pkg/octopus"
	"github.com/soothill/octopus-home-mini/pkg/slack"
)

// Monitor handles the main monitoring loop
type Monitor struct {
	cfg           *config.Config
	octopusClient *octopus.Client
	influxClient  *influx.Client
	cache         *cache.Cache
	slackNotifier *slack.Notifier // May be nil if Slack is disabled
	lastPollTime  time.Time

	// Fields accessed from multiple goroutines - protected by mu
	mu             sync.RWMutex
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

// Thread-safe accessors for concurrent fields

func (m *Monitor) getInfluxHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.influxHealthy
}

func (m *Monitor) setInfluxHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.influxHealthy = healthy
}

func (m *Monitor) getConsecutiveErr() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.consecutiveErr
}

func (m *Monitor) incrementConsecutiveErr() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consecutiveErr++
}

func (m *Monitor) resetConsecutiveErr() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consecutiveErr = 0
}

func (m *Monitor) getDegradedMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.degradedMode
}

func (m *Monitor) setDegradedMode(degraded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.degradedMode = degraded
}

func (m *Monitor) getBackoffFactor() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.backoffFactor
}

func (m *Monitor) setBackoffFactor(factor int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backoffFactor = factor
}

func (m *Monitor) incrementBackoffFactor() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backoffFactor++
}

// sanitizeError removes sensitive information from error messages
// This prevents API keys, tokens, and other credentials from being exposed in logs
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// List of sensitive patterns to redact
	sensitivePatterns := []string{
		// API keys (typically 32+ alphanumeric characters)
		`sk_[a-zA-Z0-9_-]{20,}`,      // Octopus API keys
		`[a-zA-Z0-9_-]{32,}`,         // Generic long tokens
		`Bearer\s+[a-zA-Z0-9_\-\.]+`, // Bearer tokens
		`token=[a-zA-Z0-9_\-\.]+`,    // URL query tokens
		`api_key=[a-zA-Z0-9_\-\.]+`,  // URL query API keys
		`password=[^&\s]+`,           // Passwords in URLs
		`Authorization:\s*[^\s]+`,    // Authorization headers
	}

	// Replace each sensitive pattern with [REDACTED]
	for _, pattern := range sensitivePatterns {
		errStr = regexp.MustCompile(pattern).ReplaceAllString(errStr, "[REDACTED]")
	}

	// Also redact any basic auth credentials in URLs
	// Format: http://username:password@host
	if strings.Contains(errStr, "://") && strings.Contains(errStr, "@") {
		errStr = regexp.MustCompile(`://[^:]+:[^@]+@`).ReplaceAllString(errStr, "://[REDACTED]:[REDACTED]@")
	}

	return errStr
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
		if strings.Contains(err.Error(), "warning") {
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
		cfg:           cfg,
		octopusClient: octopusClient,
		influxClient:  influxClient,
		cache:         cacheStore,
		slackNotifier: slackNotifier,
		lastPollTime:  time.Now().Add(-cfg.PollInterval),
		influxHealthy: influxClient != nil,
		degradedMode:  false,
		backoffFactor: 1,
	}

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
		log.Printf("Warning: Failed to start health server: %v", err)
	}

	// Send startup notification
	if slackNotifier != nil {
		//nolint:errcheck // Slack notification errors should not stop the monitor
		_ = slackNotifier.SendInfo("Monitor Started", "Octopus Home Mini monitor has started successfully")
	}

	// Try to sync any cached data on startup
	if monitor.getInfluxHealthy() {
		monitor.syncCache()
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start monitoring loop in a goroutine
	stopChan := make(chan struct{})
	go monitor.run(stopChan)

	// Start cache cleanup goroutine if enabled
	if cfg.CacheCleanupEnabled {
		go monitor.runCacheCleanup(stopChan)
		log.Printf("Cache cleanup enabled: running every %v (retention: %d days)", cfg.CacheCleanupInterval, cfg.CacheRetentionDays)
	}

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
	case <-time.After(cfg.ShutdownTimeout):
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

	// Stop health check server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := healthServer.Stop(shutdownCtx); err != nil {
		log.Printf("Error stopping health server: %v", err)
	}

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
			backoff := m.getBackoffFactor()
			if backoff > 1 {
				ticker.Reset(m.cfg.PollInterval * time.Duration(backoff))
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
	ctx, cancel := context.WithTimeout(context.Background(), m.cfg.PollTimeout)
	defer cancel()

	// Calculate time range for query
	now := time.Now()
	start := m.lastPollTime
	end := now

	log.Printf("Polling data from %s to %s", start.Format(time.RFC3339), end.Format(time.RFC3339))

	// Fetch telemetry data
	telemetryData, err := m.octopusClient.GetTelemetry(ctx, start, end)
	if err != nil {
		m.incrementConsecutiveErr()
		log.Printf("Error fetching telemetry: %v", err)

		// Enter degraded mode after consecutive error threshold
		consecutiveErrs := m.getConsecutiveErr()
		if consecutiveErrs >= m.cfg.ConsecutiveErrorThreshold {
			if !m.getDegradedMode() {
				m.setDegradedMode(true)
				m.setBackoffFactor(2) // Double the poll interval
				m.sendSlackError("Octopus API", fmt.Sprintf("Entering degraded mode after %d consecutive errors: %v", consecutiveErrs, sanitizeError(err)))
				log.Printf("Entering degraded mode - polling interval increased to %v", m.cfg.PollInterval*time.Duration(2))
			} else {
				// Already in degraded mode, increase backoff up to maximum configured factor
				currentBackoff := m.getBackoffFactor()
				if currentBackoff < m.cfg.MaxBackoffFactor {
					m.incrementBackoffFactor()
					newBackoff := m.getBackoffFactor()
					log.Printf("Increasing backoff factor to %dx (poll interval: %v)", newBackoff, m.cfg.PollInterval*time.Duration(newBackoff))
				}
			}
		}
		return
	}

	// Exit degraded mode on successful fetch
	if m.getDegradedMode() {
		m.setDegradedMode(false)
		m.setBackoffFactor(1)
		m.sendSlackInfo("Octopus API", "Recovered from degraded mode - resuming normal polling")
		log.Println("Exiting degraded mode - resuming normal polling interval")
	}

	m.resetConsecutiveErr()
	m.lastPollTime = end

	if len(telemetryData) == 0 {
		log.Println("No new telemetry data available")
		return
	}

	log.Printf("Retrieved %d data points", len(telemetryData))

	// Check InfluxDB health
	m.checkInfluxHealth(ctx)

	// Process data
	if m.getInfluxHealthy() {
		// Try to write to InfluxDB
		if err := m.writeToInflux(telemetryData); err != nil {
			log.Printf("Failed to write to InfluxDB: %v", sanitizeError(err))
			m.setInfluxHealthy(false)
			m.sendSlackError("InfluxDB", fmt.Sprintf("Failed to write data: %v. Switching to cache mode.", sanitizeError(err)))

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
	ctx, cancel := context.WithTimeout(context.Background(), m.cfg.InfluxWriteTimeout)
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
	wasHealthy := m.getInfluxHealthy()
	isHealthy := err == nil
	m.setInfluxHealthy(isHealthy)

	// Alert on state change
	if wasHealthy && !isHealthy {
		log.Println("InfluxDB connection lost")
		m.sendSlackError("InfluxDB", "Connection to InfluxDB lost. Switching to cache mode.")
	} else if !wasHealthy && isHealthy {
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
	expBackoff.MaxElapsedTime = m.cfg.ReconnectMaxElapsedTime
	expBackoff.InitialInterval = 1 * time.Second
	expBackoff.MaxInterval = 30 * time.Second
	expBackoff.Multiplier = 2.0

	operation := func() error {
		return m.influxClient.CheckConnection(ctx)
	}

	if err := backoff.Retry(operation, backoff.WithContext(expBackoff, ctx)); err == nil {
		log.Println("InfluxDB connection restored!")
		m.setInfluxHealthy(true)
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

	ctx, cancel := context.WithTimeout(context.Background(), m.cfg.CacheSyncTimeout)
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
			log.Printf("Error writing cached point: %v", sanitizeError(err))
			m.sendSlackError("Cache Sync", fmt.Sprintf("Failed to sync cached data: %v", sanitizeError(err)))
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

// runCacheCleanup periodically cleans up old cache files
func (m *Monitor) runCacheCleanup(stopChan chan struct{}) {
	// Run cleanup immediately on startup
	m.cleanupCache()

	// Setup periodic cleanup
	ticker := time.NewTicker(m.cfg.CacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupCache()
		case <-stopChan:
			return
		}
	}
}

// cleanupCache removes cache files older than the retention period
func (m *Monitor) cleanupCache() {
	log.Printf("Running cache cleanup (retention: %d days)...", m.cfg.CacheRetentionDays)

	retentionDuration := time.Duration(m.cfg.CacheRetentionDays) * 24 * time.Hour
	err := m.cache.CleanupOldFiles(retentionDuration)
	if err != nil {
		log.Printf("Error during cache cleanup: %v", err)
		m.sendSlackWarning("Cache Cleanup", fmt.Sprintf("Failed to cleanup old cache files: %v", err))
	} else {
		log.Printf("Cache cleanup completed successfully")
	}
}
