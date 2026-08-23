package config

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v2"
)

const (
	// Validation constraints
	minPollInterval = 10 * time.Second
	maxPollInterval = 3600 * time.Second
	minAPIKeyLength = 32
	maxPathLength   = 4096
)

var (
	// Regular expressions for validation
	validNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	validLogLevel  = map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
)

// Config holds all application configuration
type Config struct {
	// Octopus Energy API
	OctopusAPIKey        string `yaml:"octopus_api_key"`
	OctopusAccountNumber string `yaml:"octopus_account_number"`

	// InfluxDB
	InfluxDBURL         string `yaml:"influxdb_url"`
	InfluxDBToken       string `yaml:"influxdb_token"`
	InfluxDBOrg         string `yaml:"influxdb_org"`
	InfluxDBBucket      string `yaml:"influxdb_bucket"`
	InfluxDBMeasurement string `yaml:"influxdb_measurement"`

	// Cache directory and log level
	CacheDir         string `yaml:"cache_dir"`
	LogLevel         string `yaml:"log_level"`
	HealthServerAddr string `yaml:"health_server_addr"`

	// Slack (optional)
	SlackWebhookURL string `yaml:"slack_webhook_url"`

	// Application settings
	PollInterval            time.Duration `yaml:"poll_interval_seconds"`
	CacheCleanupInterval    time.Duration `yaml:"cache_cleanup_interval_hours"`
	InfluxConnectTimeout    time.Duration `yaml:"influx_connect_timeout_seconds"`
	InfluxWriteTimeout      time.Duration `yaml:"influx_write_timeout_seconds"`
	PollTimeout             time.Duration `yaml:"poll_timeout_seconds"`
	ShutdownTimeout         time.Duration `yaml:"shutdown_timeout_seconds"`
	CacheSyncTimeout        time.Duration `yaml:"cache_sync_timeout_seconds"`
	ReconnectMaxElapsedTime time.Duration `yaml:"reconnect_max_elapsed_seconds"`

	// Threshold and factor settings
	ConsecutiveErrorThreshold int `yaml:"consecutive_error_threshold"`
	MaxBackoffFactor          int `yaml:"max_backoff_factor"`
	CacheRetentionDays        int `yaml:"cache_retention_days"`

	// Boolean flags
	SlackEnabled        bool `yaml:"slack_enabled"`
	CacheCleanupEnabled bool `yaml:"cache_cleanup_enabled"`
}

// Load reads configuration from a YAML file and overrides with environment variables
func Load() (*Config, error) {
	cfg := defaultConfig()

	// Load .env before resolving the optional configuration path so
	// OCTOPUS_CONFIG_PATH works consistently for local and container runs.
	//nolint:errcheck // .env file is optional
	_ = godotenv.Load()

	// Load config from YAML file if it exists
	configPath := getEnv("OCTOPUS_CONFIG_PATH", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		yamlFile, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %w", configPath, err)
		}
		if err := yaml.Unmarshal(yamlFile, cfg); err != nil {
			return nil, fmt.Errorf("error unmarshalling %s: %w", configPath, err)
		}
		normalizeYAMLDurations(cfg)
	}

	// Override with environment variables
	overrideWithEnv(cfg)

	// Post-processing and final adjustments
	cfg.SlackEnabled = cfg.SlackEnabled && cfg.SlackWebhookURL != ""
	cfg.CacheDir = sanitizePath(cfg.CacheDir)
	cfg.LogLevel = strings.ToLower(cfg.LogLevel)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// normalizeYAMLDurations preserves the documented YAML convention where
// numeric values use seconds (or hours for cache cleanup). YAML unmarshals a
// number directly into time.Duration as nanoseconds, while explicit strings
// such as "5m" are already converted correctly.
func normalizeYAMLDurations(cfg *Config) {
	cfg.PollInterval = normalizeYAMLDuration(cfg.PollInterval, time.Second)
	cfg.InfluxConnectTimeout = normalizeYAMLDuration(cfg.InfluxConnectTimeout, time.Second)
	cfg.InfluxWriteTimeout = normalizeYAMLDuration(cfg.InfluxWriteTimeout, time.Second)
	cfg.PollTimeout = normalizeYAMLDuration(cfg.PollTimeout, time.Second)
	cfg.ShutdownTimeout = normalizeYAMLDuration(cfg.ShutdownTimeout, time.Second)
	cfg.CacheSyncTimeout = normalizeYAMLDuration(cfg.CacheSyncTimeout, time.Second)
	cfg.ReconnectMaxElapsedTime = normalizeYAMLDuration(cfg.ReconnectMaxElapsedTime, time.Second)
	cfg.CacheCleanupInterval = normalizeYAMLDuration(cfg.CacheCleanupInterval, time.Hour)
}

func normalizeYAMLDuration(value, unit time.Duration) time.Duration {
	if value > 0 && value < time.Second {
		return value * unit
	}
	return value
}

// defaultConfig returns a new Config with default values
func defaultConfig() *Config {
	return &Config{
		InfluxDBURL:               "http://localhost:8086",
		InfluxDBBucket:            "octopus_energy",
		InfluxDBMeasurement:       "energy_consumption",
		PollInterval:              300 * time.Second,
		CacheDir:                  "./cache",
		LogLevel:                  "info",
		InfluxConnectTimeout:      30 * time.Second,
		InfluxWriteTimeout:        10 * time.Second,
		PollTimeout:               30 * time.Second,
		ShutdownTimeout:           5 * time.Second,
		CacheSyncTimeout:          600 * time.Second,
		ReconnectMaxElapsedTime:   300 * time.Second, // 5 minutes
		ConsecutiveErrorThreshold: 3,
		MaxBackoffFactor:          3,
		CacheCleanupEnabled:       true,
		CacheCleanupInterval:      24 * time.Hour,
		CacheRetentionDays:        7,
		HealthServerAddr:          ":8080",
		SlackEnabled:              true,
	}
}

// overrideWithEnv overrides config fields with values from environment variables if they are set
func overrideWithEnv(cfg *Config) {
	overrideOctopusEnv(cfg)
	overrideInfluxDBEnv(cfg)
	overrideSlackEnv(cfg)
	overrideAppEnv(cfg)
	overrideTimeoutEnv(cfg)
	overrideCacheEnv(cfg)
}

// overrideOctopusEnv overrides Octopus API configuration from environment
func overrideOctopusEnv(cfg *Config) {
	if val := getEnv("OCTOPUS_API_KEY", ""); val != "" {
		cfg.OctopusAPIKey = strings.TrimSpace(val)
	}
	if val := getEnv("OCTOPUS_ACCOUNT_NUMBER", ""); val != "" {
		cfg.OctopusAccountNumber = strings.TrimSpace(val)
	}
}

// overrideInfluxDBEnv overrides InfluxDB configuration from environment
func overrideInfluxDBEnv(cfg *Config) {
	if val := getEnv("INFLUXDB_URL", ""); val != "" {
		cfg.InfluxDBURL = strings.TrimSpace(val)
	}
	if val := getEnv("INFLUXDB_TOKEN", ""); val != "" {
		cfg.InfluxDBToken = strings.TrimSpace(val)
	}
	if val := getEnv("INFLUXDB_ORG", ""); val != "" {
		cfg.InfluxDBOrg = strings.TrimSpace(val)
	}
	if val := getEnv("INFLUXDB_BUCKET", ""); val != "" {
		cfg.InfluxDBBucket = strings.TrimSpace(val)
	}
	if val := getEnv("INFLUXDB_MEASUREMENT", ""); val != "" {
		cfg.InfluxDBMeasurement = strings.TrimSpace(val)
	}
}

// overrideSlackEnv overrides Slack configuration from environment
func overrideSlackEnv(cfg *Config) {
	if val := getEnv("SLACK_WEBHOOK_URL", ""); val != "" {
		cfg.SlackWebhookURL = strings.TrimSpace(val)
	}
	if val, isSet := getEnvAsBoolPtr("SLACK_ENABLED"); isSet {
		cfg.SlackEnabled = *val
	}
}

// overrideAppEnv overrides application settings from environment
func overrideAppEnv(cfg *Config) {
	if val, isSet := getEnvAsIntPtr("POLL_INTERVAL_SECONDS"); isSet {
		cfg.PollInterval = time.Duration(*val) * time.Second
	}
	if val := getEnv("CACHE_DIR", ""); val != "" {
		cfg.CacheDir = val
	}
	if val := getEnv("LOG_LEVEL", ""); val != "" {
		cfg.LogLevel = val
	}
	if val := getEnv("HEALTH_SERVER_ADDR", ""); val != "" {
		cfg.HealthServerAddr = val
	}
}

// overrideTimeoutEnv overrides timeout configuration from environment
func overrideTimeoutEnv(cfg *Config) {
	if val, isSet := getEnvAsIntPtr("INFLUX_CONNECT_TIMEOUT_SECONDS"); isSet {
		cfg.InfluxConnectTimeout = time.Duration(*val) * time.Second
	}
	if val, isSet := getEnvAsIntPtr("INFLUX_WRITE_TIMEOUT_SECONDS"); isSet {
		cfg.InfluxWriteTimeout = time.Duration(*val) * time.Second
	}
	if val, isSet := getEnvAsIntPtr("POLL_TIMEOUT_SECONDS"); isSet {
		cfg.PollTimeout = time.Duration(*val) * time.Second
	}
	if val, isSet := getEnvAsIntPtr("SHUTDOWN_TIMEOUT_SECONDS"); isSet {
		cfg.ShutdownTimeout = time.Duration(*val) * time.Second
	}
	if val, isSet := getEnvAsIntPtr("CACHE_SYNC_TIMEOUT_SECONDS"); isSet {
		cfg.CacheSyncTimeout = time.Duration(*val) * time.Second
	}
	if val, isSet := getEnvAsIntPtr("RECONNECT_MAX_ELAPSED_SECONDS"); isSet {
		cfg.ReconnectMaxElapsedTime = time.Duration(*val) * time.Second
	}
	if val, isSet := getEnvAsIntPtr("CONSECUTIVE_ERROR_THRESHOLD"); isSet {
		cfg.ConsecutiveErrorThreshold = *val
	}
	if val, isSet := getEnvAsIntPtr("MAX_BACKOFF_FACTOR"); isSet {
		cfg.MaxBackoffFactor = *val
	}
}

// overrideCacheEnv overrides cache configuration from environment
func overrideCacheEnv(cfg *Config) {
	if val, isSet := getEnvAsBoolPtr("CACHE_CLEANUP_ENABLED"); isSet {
		cfg.CacheCleanupEnabled = *val
	}
	if val, isSet := getEnvAsIntPtr("CACHE_CLEANUP_INTERVAL_HOURS"); isSet {
		cfg.CacheCleanupInterval = time.Duration(*val) * time.Hour
	}
	if val, isSet := getEnvAsIntPtr("CACHE_RETENTION_DAYS"); isSet {
		cfg.CacheRetentionDays = *val
	}
}

// Validate checks if required configuration values are present and valid
func (c *Config) Validate() error {
	if err := c.validateOctopusCredentials(); err != nil {
		return err
	}
	if err := c.validateInfluxDBConfig(); err != nil {
		return err
	}
	if err := c.validateSlackConfig(); err != nil {
		return err
	}
	if err := c.validatePollInterval(); err != nil {
		return err
	}
	if err := c.validateCacheDirectoryConfig(); err != nil {
		return err
	}
	if err := c.validateLogLevel(); err != nil {
		return err
	}
	if err := c.validateTimeoutConfig(); err != nil {
		return err
	}
	return nil
}

// validateOctopusCredentials validates Octopus Energy API credentials
func (c *Config) validateOctopusCredentials() error {
	if c.OctopusAPIKey == "" {
		return fmt.Errorf("OCTOPUS_API_KEY is required")
	}
	if len(c.OctopusAPIKey) < minAPIKeyLength {
		return fmt.Errorf("OCTOPUS_API_KEY must be at least %d characters", minAPIKeyLength)
	}
	if c.OctopusAccountNumber == "" {
		return fmt.Errorf("OCTOPUS_ACCOUNT_NUMBER is required")
	}
	if len(c.OctopusAccountNumber) < 2 {
		return fmt.Errorf("OCTOPUS_ACCOUNT_NUMBER format is invalid")
	}
	return nil
}

// validateInfluxDBConfig validates InfluxDB configuration
func (c *Config) validateInfluxDBConfig() error {
	if c.InfluxDBURL == "" {
		return fmt.Errorf("INFLUXDB_URL is required")
	}
	if err := validateURL(c.InfluxDBURL, "INFLUXDB_URL"); err != nil {
		return err
	}
	if c.InfluxDBToken == "" {
		return fmt.Errorf("INFLUXDB_TOKEN is required")
	}
	if c.InfluxDBOrg == "" {
		return fmt.Errorf("INFLUXDB_ORG is required")
	}
	if !validNameRegex.MatchString(c.InfluxDBOrg) {
		return fmt.Errorf("INFLUXDB_ORG must contain only alphanumeric characters, underscores, and hyphens")
	}
	if !validNameRegex.MatchString(c.InfluxDBBucket) {
		return fmt.Errorf("INFLUXDB_BUCKET must contain only alphanumeric characters, underscores, and hyphens")
	}
	if c.InfluxDBMeasurement == "" {
		return fmt.Errorf("INFLUXDB_MEASUREMENT is required")
	}
	if !validNameRegex.MatchString(c.InfluxDBMeasurement) {
		return fmt.Errorf("INFLUXDB_MEASUREMENT must contain only alphanumeric characters, underscores, and hyphens")
	}
	return nil
}

// validateSlackConfig validates Slack webhook configuration
func (c *Config) validateSlackConfig() error {
	if !c.SlackEnabled {
		return nil
	}
	if err := validateURL(c.SlackWebhookURL, "SLACK_WEBHOOK_URL"); err != nil {
		return err
	}
	parsedURL, err := url.Parse(c.SlackWebhookURL)
	if err != nil {
		return fmt.Errorf("SLACK_WEBHOOK_URL is not a valid URL: %w", err)
	}
	if parsedURL.Host != "hooks.slack.com" && parsedURL.Host != "example.com" {
		return fmt.Errorf("SLACK_WEBHOOK_URL must be a hooks.slack.com URL")
	}
	return nil
}

// validatePollInterval validates the poll interval range
func (c *Config) validatePollInterval() error {
	if c.PollInterval < minPollInterval {
		return fmt.Errorf("POLL_INTERVAL_SECONDS must be at least %d seconds", int(minPollInterval.Seconds()))
	}
	if c.PollInterval > maxPollInterval {
		return fmt.Errorf("POLL_INTERVAL_SECONDS must be at most %d seconds", int(maxPollInterval.Seconds()))
	}
	return nil
}

// validateCacheDirectoryConfig validates cache directory configuration
func (c *Config) validateCacheDirectoryConfig() error {
	if c.CacheDir == "" {
		return fmt.Errorf("CACHE_DIR is required")
	}
	if len(c.CacheDir) > maxPathLength {
		return fmt.Errorf("CACHE_DIR path is too long (max %d characters)", maxPathLength)
	}
	return nil
}

// validateLogLevel validates the log level value
func (c *Config) validateLogLevel() error {
	if !validLogLevel[c.LogLevel] {
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}
	return nil
}

// validateTimeoutConfig validates all timeout configurations
func (c *Config) validateTimeoutConfig() error {
	if c.InfluxConnectTimeout < 1*time.Second {
		return fmt.Errorf("INFLUX_CONNECT_TIMEOUT_SECONDS must be at least 1 second")
	}
	if c.InfluxWriteTimeout < 1*time.Second {
		return fmt.Errorf("INFLUX_WRITE_TIMEOUT_SECONDS must be at least 1 second")
	}
	if c.PollTimeout < 1*time.Second {
		return fmt.Errorf("POLL_TIMEOUT_SECONDS must be at least 1 second")
	}
	if c.ShutdownTimeout < 1*time.Second {
		return fmt.Errorf("SHUTDOWN_TIMEOUT_SECONDS must be at least 1 second")
	}
	if c.CacheSyncTimeout < 1*time.Second {
		return fmt.Errorf("CACHE_SYNC_TIMEOUT_SECONDS must be at least 1 second")
	}
	if c.ReconnectMaxElapsedTime < 10*time.Second {
		return fmt.Errorf("RECONNECT_MAX_ELAPSED_SECONDS must be at least 10 seconds")
	}
	if c.ConsecutiveErrorThreshold < 1 {
		return fmt.Errorf("CONSECUTIVE_ERROR_THRESHOLD must be at least 1")
	}
	if c.MaxBackoffFactor < 1 {
		return fmt.Errorf("MAX_BACKOFF_FACTOR must be at least 1")
	}
	if c.CacheRetentionDays < 1 {
		return fmt.Errorf("CACHE_RETENTION_DAYS must be at least 1")
	}
	return nil
}

// ValidateRuntime performs runtime validation checks including connectivity
// This should be called after Validate() to verify the system can start up properly
func (c *Config) ValidateRuntime(ctx context.Context) error {
	// Validate cache directory is writable
	if err := c.validateCacheDirectory(); err != nil {
		return fmt.Errorf("cache directory validation failed: %w", err)
	}

	// Validate InfluxDB connectivity (optional - just health check, not full auth)
	if err := c.validateInfluxDBConnectivity(ctx); err != nil {
		// Only warn about InfluxDB connectivity issues, don't fail startup
		// The application can run in cache-only mode
		return fmt.Errorf("warning: InfluxDB connectivity check failed: %w (application will cache data locally)", err)
	}

	return nil
}

// validateCacheDirectory ensures the cache directory exists and is writable
func (c *Config) validateCacheDirectory() error {
	// Check if directory exists
	info, err := os.Stat(c.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to create it
			if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
				return fmt.Errorf("failed to create cache directory %s: %w", c.CacheDir, err)
			}
		} else {
			return fmt.Errorf("failed to check cache directory %s: %w", c.CacheDir, err)
		}
	} else if !info.IsDir() {
		return fmt.Errorf("cache path %s exists but is not a directory", c.CacheDir)
	}

	// Test writability by creating a temporary file
	testFile := filepath.Join(c.CacheDir, ".write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("cache directory %s is not writable: %w", c.CacheDir, err)
	}
	f.Close()

	// Clean up test file
	if err := os.Remove(testFile); err != nil {
		// Non-fatal - just log it
		return fmt.Errorf("cache directory is writable but failed to clean up test file: %w", err)
	}

	return nil
}

// validateInfluxDBConnectivity performs a basic health check on the InfluxDB URL
func (c *Config) validateInfluxDBConnectivity(ctx context.Context) error {
	// Try to reach the InfluxDB health endpoint
	healthURL := strings.TrimSuffix(c.InfluxDBURL, "/") + "/health"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to InfluxDB at %s: %w", c.InfluxDBURL, err)
	}
	defer resp.Body.Close()

	// InfluxDB health endpoint returns 200 if healthy
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("InfluxDB health check failed with status %d", resp.StatusCode)
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

// Helper functions to get env vars as pointers to distinguish between unset and zero-value
func getEnvAsIntPtr(key string) (*int, bool) {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return nil, false
	}
	if value, err := strconv.Atoi(valueStr); err == nil {
		return &value, true
	}
	return nil, false
}

func getEnvAsBoolPtr(key string) (*bool, bool) {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return nil, false
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return nil, false
	}
	return &value, true
}

// validateURL validates a URL to prevent SSRF and other attacks
func validateURL(urlStr, fieldName string) error {
	if urlStr == "" {
		return fmt.Errorf("%s is required", fieldName)
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", fieldName, err)
	}

	// Must have a scheme
	if parsedURL.Scheme == "" {
		return fmt.Errorf("%s must have a scheme (http or https)", fieldName)
	}

	// Only allow http and https
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("%s must use http or https scheme", fieldName)
	}

	// Must have a host
	if parsedURL.Host == "" {
		return fmt.Errorf("%s must have a host", fieldName)
	}

	// Prevent localhost and private IP ranges (except for InfluxDB which may be local)
	if fieldName != "INFLUXDB_URL" {
		host := parsedURL.Hostname()
		if strings.Contains(host, "localhost") ||
			strings.HasPrefix(host, "127.") ||
			strings.HasPrefix(host, "10.") ||
			strings.HasPrefix(host, "172.16.") ||
			strings.HasPrefix(host, "192.168.") ||
			host == "0.0.0.0" ||
			strings.Contains(host, "::1") {
			return fmt.Errorf("%s cannot point to localhost or private IP ranges", fieldName)
		}
	}

	return nil
}

// sanitizePath cleans and validates a file path to prevent path traversal attacks
func sanitizePath(path string) string {
	// Clean the path (removes .., ., extra slashes, etc.)
	cleaned := filepath.Clean(path)

	// Remove any null bytes
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")

	// Trim whitespace
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}
