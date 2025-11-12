package monitor

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog/log"
	"github.com/soothill/octopus-home-mini/pkg/cache"
	"github.com/soothill/octopus-home-mini/pkg/config"
	"github.com/soothill/octopus-home-mini/pkg/influx"
	"github.com/soothill/octopus-home-mini/pkg/octopus"
	"github.com/soothill/octopus-home-mini/pkg/slack"
)

// Monitor handles the main monitoring loop
type Monitor struct {
	Cfg           *config.Config
	OctopusClient *octopus.Client
	InfluxClient  *influx.Client
	Cache         *cache.Cache
	SlackNotifier *slack.Notifier // May be nil if Slack is disabled
	LastPollTime  time.Time

	// Fields accessed from multiple goroutines - protected by mu
	mu             sync.RWMutex
	influxHealthy  bool
	consecutiveErr int
	degradedMode   bool // True when system is operating in degraded mode
	backoffFactor  int  // Multiplier for poll interval when in degraded mode
}

func New(cfg *config.Config, octopusClient *octopus.Client, influxClient *influx.Client, cache *cache.Cache, slackNotifier *slack.Notifier) *Monitor {
	return &Monitor{
		Cfg:           cfg,
		OctopusClient: octopusClient,
		InfluxClient:  influxClient,
		Cache:         cache,
		SlackNotifier: slackNotifier,
		LastPollTime:  time.Now().Add(-cfg.PollInterval),
		influxHealthy: influxClient != nil,
		degradedMode:  false,
		backoffFactor: 1,
	}
}

// SendSlackError sends an error notification to Slack if enabled
func (m *Monitor) SendSlackError(component, message string) {
	if m.SlackNotifier != nil {
		if err := m.SlackNotifier.SendError(component, message); err != nil {
			log.Error().Err(err).Msg("Error sending Slack error notification")
		}
	}
}

// SendSlackWarning sends a warning notification to Slack if enabled
func (m *Monitor) SendSlackWarning(component, message string) {
	if m.SlackNotifier != nil {
		if err := m.SlackNotifier.SendWarning(component, message); err != nil {
			log.Error().Err(err).Msg("Error sending Slack warning notification")
		}
	}
}

// SendSlackInfo sends an info notification to Slack if enabled
func (m *Monitor) SendSlackInfo(title, message string) {
	if m.SlackNotifier != nil {
		if err := m.SlackNotifier.SendInfo(title, message); err != nil {
			log.Error().Err(err).Msg("Error sending Slack info notification")
		}
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

// Run executes the main monitoring loop with adaptive polling
func (m *Monitor) Run(stopChan chan struct{}) {
	ticker := time.NewTicker(m.Cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.poll()

			// Adjust poll interval based on degraded mode
			backoff := m.getBackoffFactor()
			if backoff > 1 {
				ticker.Reset(m.Cfg.PollInterval * time.Duration(backoff))
			} else {
				ticker.Reset(m.Cfg.PollInterval)
			}

		case <-stopChan:
			return
		}
	}
}

// poll fetches and processes new energy data
func (m *Monitor) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), m.Cfg.PollTimeout)
	defer cancel()

	// Calculate time range for query
	now := time.Now()
	start := m.LastPollTime
	end := now

	log.Info().
		Time("start", start).
		Time("end", end).
		Msg("Polling for telemetry data")

	// Fetch telemetry data
	telemetryData, err := m.OctopusClient.GetTelemetry(ctx, start, end)
	if err != nil {
		m.incrementConsecutiveErr()
		log.Error().Err(err).Msg("Error fetching telemetry")

		// Enter degraded mode after consecutive error threshold
		consecutiveErrs := m.getConsecutiveErr()
		if consecutiveErrs >= m.Cfg.ConsecutiveErrorThreshold {
			if !m.getDegradedMode() {
				m.setDegradedMode(true)
				m.setBackoffFactor(2) // Double the poll interval
				m.SendSlackError("Octopus API", fmt.Sprintf("Entering degraded mode after %d consecutive errors: %v", consecutiveErrs, sanitizeError(err)))
				log.Warn().
					Int("consecutive_errors", consecutiveErrs).
					Dur("new_interval", m.Cfg.PollInterval*2).
					Msg("Entering degraded mode")
			} else {
				// Already in degraded mode, increase backoff up to maximum configured factor
				currentBackoff := m.getBackoffFactor()
				if currentBackoff < m.Cfg.MaxBackoffFactor {
					m.incrementBackoffFactor()
					newBackoff := m.getBackoffFactor()
					log.Warn().
						Int("backoff_factor", newBackoff).
						Dur("new_interval", m.Cfg.PollInterval*time.Duration(newBackoff)).
						Msg("Increasing backoff factor")
				}
			}
		}
		return
	}

	// Exit degraded mode on successful fetch
	if m.getDegradedMode() {
		m.setDegradedMode(false)
		m.setBackoffFactor(1)
		m.SendSlackInfo("Octopus API", "Recovered from degraded mode - resuming normal polling")
		log.Info().Msg("Exiting degraded mode - resuming normal polling interval")
	}

	m.resetConsecutiveErr()
	m.LastPollTime = end

	if len(telemetryData) == 0 {
		log.Info().Msg("No new telemetry data available")
		return
	}

	log.Info().Int("count", len(telemetryData)).Msg("Retrieved telemetry data")

	// Check InfluxDB health
	m.checkInfluxHealth(ctx)

	// Process data
	if m.getInfluxHealthy() {
		// Try to write to InfluxDB
		if err := m.writeToInflux(telemetryData); err != nil {
			log.Error().Err(err).Msg("Failed to write to InfluxDB")
			m.setInfluxHealthy(false)
			m.SendSlackError("InfluxDB", fmt.Sprintf("Failed to write data: %v. Switching to cache mode.", sanitizeError(err)))

			// Cache the data instead
			m.cacheData(telemetryData)
		} else {
			log.Info().Int("count", len(telemetryData)).Msg("Successfully wrote data points to InfluxDB")
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
	ctx, cancel := context.WithTimeout(context.Background(), m.Cfg.InfluxWriteTimeout)
	defer cancel()

	for _, data := range telemetryData {
		dp := influx.DataPoint{
			Timestamp:        data.ReadAt,
			ConsumptionDelta: data.ConsumptionDelta,
			Demand:           data.Demand,
			CostDelta:        data.CostDelta,
			Consumption:      data.Consumption,
		}

		if err := m.InfluxClient.WritePointDirectly(ctx, dp); err != nil {
			return err
		}
	}

	m.InfluxClient.Flush()
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

	if err := m.Cache.Add(dataPoints); err != nil {
		log.Error().Err(err).Msg("Error caching data")
		m.SendSlackError("Cache", fmt.Sprintf("Failed to cache data: %v", err))
	} else {
		log.Info().
			Int("count", len(dataPoints)).
			Int("total_in_cache", m.Cache.Count()).
			Msg("Cached data points")
	}
}

// checkInfluxHealth checks if InfluxDB is healthy
func (m *Monitor) checkInfluxHealth(ctx context.Context) {
	if m.InfluxClient == nil {
		return
	}

	err := m.InfluxClient.CheckConnection(ctx)
	wasHealthy := m.getInfluxHealthy()
	isHealthy := err == nil
	m.setInfluxHealthy(isHealthy)

	// Alert on state change
	if wasHealthy && !isHealthy {
		log.Warn().Msg("InfluxDB connection lost")
		m.SendSlackError("InfluxDB", "Connection to InfluxDB lost. Switching to cache mode.")
	} else if !wasHealthy && isHealthy {
		log.Info().Msg("InfluxDB connection restored")
		m.SendSlackInfo("InfluxDB", "Connection to InfluxDB restored. Syncing cached data...")
		m.SyncCache()
	}
}

// tryReconnectInflux attempts to reconnect to InfluxDB with exponential backoff
func (m *Monitor) tryReconnectInflux(ctx context.Context) {
	if m.InfluxClient == nil {
		return
	}

	// Configure exponential backoff
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = m.Cfg.ReconnectMaxElapsedTime
	expBackoff.InitialInterval = 1 * time.Second
	expBackoff.MaxInterval = 30 * time.Second
	expBackoff.Multiplier = 2.0

	operation := func() error {
		return m.InfluxClient.CheckConnection(ctx)
	}

	if err := backoff.Retry(operation, backoff.WithContext(expBackoff, ctx)); err == nil {
		log.Info().Msg("InfluxDB connection restored!")
		m.setInfluxHealthy(true)
		m.SendSlackInfo("InfluxDB", "Connection restored. Syncing cached data...")
		m.SyncCache()
	}
}

// SyncCache writes all cached data to InfluxDB
func (m *Monitor) SyncCache() {
	if !m.getInfluxHealthy() {
		log.Warn().Msg("InfluxDB not healthy, skipping cache sync")
		return
	}
	cachedData := m.Cache.GetAll()
	if len(cachedData) == 0 {
		log.Info().Msg("No cached data to sync")
		return
	}

	log.Info().Int("count", len(cachedData)).Msg("Syncing cached data points to InfluxDB...")

	ctx, cancel := context.WithTimeout(context.Background(), m.Cfg.CacheSyncTimeout)
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

		if err := m.InfluxClient.WritePointDirectly(ctx, dp); err != nil {
			log.Error().Err(err).Msg("Error writing cached point")
			m.SendSlackError("Cache Sync", fmt.Sprintf("Failed to sync cached data: %v", sanitizeError(err)))
			return
		}
		successCount++
	}

	m.InfluxClient.Flush()

	// Clear cache after successful sync
	if err := m.Cache.Clear(); err != nil {
		log.Error().Err(err).Msg("Error clearing cache")
		m.SendSlackError("Cache", fmt.Sprintf("Failed to clear cache: %v", err))
	} else {
		log.Info().Int("count", successCount).Msg("Successfully synced cached data points")
		m.SendSlackInfo("Cache Sync", fmt.Sprintf("Successfully synced %d cached data points to InfluxDB", successCount))
	}
}

// RunCacheCleanup periodically cleans up old cache files
func (m *Monitor) RunCacheCleanup(stopChan chan struct{}) {
	// Run cleanup immediately on startup
	m.cleanupCache()

	// Setup periodic cleanup
	ticker := time.NewTicker(m.Cfg.CacheCleanupInterval)
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
	log.Info().Int("retention_days", m.Cfg.CacheRetentionDays).Msg("Running cache cleanup...")

	retentionDuration := time.Duration(m.Cfg.CacheRetentionDays) * 24 * time.Hour
	err := m.Cache.CleanupOldFiles(retentionDuration)
	if err != nil {
		log.Error().Err(err).Msg("Error during cache cleanup")
		m.SendSlackWarning("Cache Cleanup", fmt.Sprintf("Failed to cleanup old cache files: %v", err))
	} else {
		log.Info().Msg("Cache cleanup completed successfully")
	}
}
