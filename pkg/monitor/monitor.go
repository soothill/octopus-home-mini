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

// Clock interface for time-related functions
type Clock interface {
	Now() time.Time
	NewTicker(d time.Duration) *time.Ticker
}

// SystemClock implements the Clock interface using the time package
type SystemClock struct{}

func (c *SystemClock) Now() time.Time {
	return time.Now()
}

func (c *SystemClock) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

// OctopusClient defines the interface for fetching telemetry data
type OctopusClient interface {
	GetTelemetry(ctx context.Context, start, end time.Time) ([]octopus.TelemetryData, error)
	Initialize(ctx context.Context) error
}

// Monitor handles the main monitoring loop
type Monitor struct {
	// Configuration and clients (pointers first for optimal alignment)
	Cfg           *config.Config
	OctopusClient OctopusClient
	InfluxClient  *influx.Client
	Cache         *cache.Cache
	SlackNotifier *slack.Notifier // May be nil if Slack is disabled
	state         *State
	Clock         Clock

	// Internal state
	mu           sync.RWMutex
	LastPollTime time.Time
}

// State holds the internal state of the monitor
type State struct {
	InfluxHealthy  bool
	DegradedMode   bool
	ConsecutiveErr int
	BackoffFactor  int
}

func New(cfg *config.Config, octopusClient OctopusClient, influxClient *influx.Client, cache *cache.Cache, slackNotifier *slack.Notifier) *Monitor {
	clock := &SystemClock{}
	return &Monitor{
		Cfg:           cfg,
		OctopusClient: octopusClient,
		InfluxClient:  influxClient,
		Cache:         cache,
		SlackNotifier: slackNotifier,
		Clock:         clock,
		LastPollTime:  clock.Now().Add(-cfg.PollInterval),
		state: &State{
			InfluxHealthy: influxClient != nil,
			DegradedMode:  false,
			BackoffFactor: 1,
		},
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
	return m.state.InfluxHealthy
}

func (m *Monitor) setInfluxHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.InfluxHealthy = healthy
}

func (m *Monitor) getConsecutiveErr() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.ConsecutiveErr
}

func (m *Monitor) incrementConsecutiveErr() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.ConsecutiveErr++
}

func (m *Monitor) resetConsecutiveErr() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.ConsecutiveErr = 0
}

func (m *Monitor) getDegradedMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.DegradedMode
}

func (m *Monitor) setDegradedMode(degraded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.DegradedMode = degraded
}

func (m *Monitor) getBackoffFactor() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.BackoffFactor
}

func (m *Monitor) setBackoffFactor(factor int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.BackoffFactor = factor
}

func (m *Monitor) incrementBackoffFactor() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.BackoffFactor++
}

var (
	// Pre-compile regex for efficiency
	sensitivePatterns = []*regexp.Regexp{
		regexp.MustCompile(`sk_[a-zA-Z0-9_-]{20,}`),      // Octopus API keys
		regexp.MustCompile(`[a-zA-Z0-9_-]{32,}`),         // Generic long tokens
		regexp.MustCompile(`Bearer\s+[a-zA-Z0-9_\-\.]+`), // Bearer tokens
		regexp.MustCompile(`token=[a-zA-Z0-9_\-\.]+`),    // URL query tokens
		regexp.MustCompile(`api_key=[a-zA-Z0-9_\-\.]+`),  // URL query API keys
		regexp.MustCompile(`password=[^&\s]+`),           // Passwords in URLs
		regexp.MustCompile(`Authorization:\s*[^\s]+`),    // Authorization headers
	}
	authPattern = regexp.MustCompile(`://[^:]+:[^@]+@`)
)

// sanitizeError removes sensitive information from error messages
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	for _, re := range sensitivePatterns {
		errStr = re.ReplaceAllString(errStr, "[REDACTED]")
	}

	if strings.Contains(errStr, "://") && strings.Contains(errStr, "@") {
		errStr = authPattern.ReplaceAllString(errStr, "://[REDACTED]:[REDACTED]@")
	}

	return errStr
}

// Run executes the main monitoring loop with adaptive polling
func (m *Monitor) Run(stopChan chan struct{}) {
	ticker := m.Clock.NewTicker(m.Cfg.PollInterval)
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

	telemetryData, err := m.fetchData(ctx)
	if err != nil {
		m.handleFetchError(err)
		return
	}

	m.handleFetchSuccess(telemetryData)
}

func (m *Monitor) fetchData(ctx context.Context) ([]octopus.TelemetryData, error) {
	now := m.Clock.Now()
	start := m.LastPollTime
	end := now

	log.Info().
		Time("start", start).
		Time("end", end).
		Msg("Polling for telemetry data")

	return m.OctopusClient.GetTelemetry(ctx, start, end)
}

func (m *Monitor) handleFetchError(err error) {
	m.incrementConsecutiveErr()
	log.Error().Err(err).Msg("Error fetching telemetry")

	consecutiveErrs := m.getConsecutiveErr()
	if consecutiveErrs < m.Cfg.ConsecutiveErrorThreshold {
		return
	}

	if !m.getDegradedMode() {
		m.enterDegradedMode(consecutiveErrs, err)
	} else {
		m.increaseBackoff()
	}
}

func (m *Monitor) enterDegradedMode(consecutiveErrs int, err error) {
	m.setDegradedMode(true)
	m.setBackoffFactor(2)
	m.SendSlackError("Octopus API", fmt.Sprintf("Entering degraded mode after %d consecutive errors: %v", consecutiveErrs, sanitizeError(err)))
	log.Warn().
		Int("consecutive_errors", consecutiveErrs).
		Dur("new_interval", m.Cfg.PollInterval*2).
		Msg("Entering degraded mode")
}

func (m *Monitor) increaseBackoff() {
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

func (m *Monitor) handleFetchSuccess(telemetryData []octopus.TelemetryData) {
	if m.getDegradedMode() {
		m.exitDegradedMode()
	}

	m.resetConsecutiveErr()
	m.LastPollTime = m.Clock.Now()

	if len(telemetryData) == 0 {
		log.Info().Msg("No new telemetry data available")
		return
	}

	log.Info().Int("count", len(telemetryData)).Msg("Retrieved telemetry data")

	m.processData(telemetryData)
}

func (m *Monitor) exitDegradedMode() {
	m.setDegradedMode(false)
	m.setBackoffFactor(1)
	m.SendSlackInfo("Octopus API", "Recovered from degraded mode - resuming normal polling")
	log.Info().Msg("Exiting degraded mode - resuming normal polling interval")
}

func (m *Monitor) processData(telemetryData []octopus.TelemetryData) {
	ctx := context.Background()
	m.checkInfluxHealth(ctx)

	if m.getInfluxHealthy() {
		if err := m.writeToInflux(telemetryData); err != nil {
			log.Error().Err(err).Msg("Failed to write to InfluxDB")
			m.setInfluxHealthy(false)
			m.SendSlackError("InfluxDB", fmt.Sprintf("Failed to write data: %v. Switching to cache mode.", sanitizeError(err)))
			m.cacheData(telemetryData)
		} else {
			log.Info().Int("count", len(telemetryData)).Msg("Successfully wrote data points to InfluxDB")
		}
	} else {
		m.cacheData(telemetryData)
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
