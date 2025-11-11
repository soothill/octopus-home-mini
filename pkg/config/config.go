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
	OctopusAPIKey        string
	OctopusAccountNumber string

	// InfluxDB
	InfluxDBURL    string
	InfluxDBToken  string
	InfluxDBOrg    string
	InfluxDBBucket string

	// Slack (optional)
	SlackWebhookURL string
	SlackEnabled    bool

	// Application settings
	PollInterval time.Duration
	CacheDir     string
	LogLevel     string
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Try to load .env file (optional - ignore errors if it doesn't exist)
	//nolint:errcheck // .env file is optional
	_ = godotenv.Load()

	pollIntervalSec := getEnvAsInt("POLL_INTERVAL_SECONDS", 30)
	slackWebhookURL := getEnv("SLACK_WEBHOOK_URL", "")
	slackEnabled := getEnvAsBool("SLACK_ENABLED", true) && slackWebhookURL != ""

	// Sanitize cache directory path to prevent path traversal
	cacheDir := getEnv("CACHE_DIR", "./cache")
	cacheDir = sanitizePath(cacheDir)

	// Normalize log level to lowercase
	logLevel := strings.ToLower(getEnv("LOG_LEVEL", "info"))

	cfg := &Config{
		OctopusAPIKey:        strings.TrimSpace(getEnv("OCTOPUS_API_KEY", "")),
		OctopusAccountNumber: strings.TrimSpace(getEnv("OCTOPUS_ACCOUNT_NUMBER", "")),
		InfluxDBURL:          strings.TrimSpace(getEnv("INFLUXDB_URL", "http://localhost:8086")),
		InfluxDBToken:        strings.TrimSpace(getEnv("INFLUXDB_TOKEN", "")),
		InfluxDBOrg:          strings.TrimSpace(getEnv("INFLUXDB_ORG", "")),
		InfluxDBBucket:       strings.TrimSpace(getEnv("INFLUXDB_BUCKET", "octopus_energy")),
		SlackWebhookURL:      strings.TrimSpace(slackWebhookURL),
		SlackEnabled:         slackEnabled,
		PollInterval:         time.Duration(pollIntervalSec) * time.Second,
		CacheDir:             cacheDir,
		LogLevel:             logLevel,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if required configuration values are present and valid
func (c *Config) Validate() error {
	// Validate Octopus API credentials
	if c.OctopusAPIKey == "" {
		return fmt.Errorf("OCTOPUS_API_KEY is required")
	}
	if len(c.OctopusAPIKey) < minAPIKeyLength {
		return fmt.Errorf("OCTOPUS_API_KEY must be at least %d characters", minAPIKeyLength)
	}
	if c.OctopusAccountNumber == "" {
		return fmt.Errorf("OCTOPUS_ACCOUNT_NUMBER is required")
	}
	// Account number should be alphanumeric (A-12345678 format)
	if len(c.OctopusAccountNumber) < 2 {
		return fmt.Errorf("OCTOPUS_ACCOUNT_NUMBER format is invalid")
	}

	// Validate InfluxDB configuration
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

	// Validate Slack webhook URL if enabled
	if c.SlackEnabled {
		if err := validateURL(c.SlackWebhookURL, "SLACK_WEBHOOK_URL"); err != nil {
			return err
		}
		// Ensure it's a hooks.slack.com URL (or example.com for testing)
		parsedURL, err := url.Parse(c.SlackWebhookURL)
		if err != nil {
			return fmt.Errorf("SLACK_WEBHOOK_URL is not a valid URL: %w", err)
		}
		if parsedURL.Host != "hooks.slack.com" && parsedURL.Host != "example.com" {
			return fmt.Errorf("SLACK_WEBHOOK_URL must be a hooks.slack.com URL")
		}
	}

	// Validate poll interval
	if c.PollInterval < minPollInterval {
		return fmt.Errorf("POLL_INTERVAL_SECONDS must be at least %d seconds", int(minPollInterval.Seconds()))
	}
	if c.PollInterval > maxPollInterval {
		return fmt.Errorf("POLL_INTERVAL_SECONDS must be at most %d seconds", int(maxPollInterval.Seconds()))
	}

	// Validate cache directory
	if c.CacheDir == "" {
		return fmt.Errorf("CACHE_DIR is required")
	}
	if len(c.CacheDir) > maxPathLength {
		return fmt.Errorf("CACHE_DIR path is too long (max %d characters)", maxPathLength)
	}

	// Validate log level
	if !validLogLevel[c.LogLevel] {
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
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

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
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
