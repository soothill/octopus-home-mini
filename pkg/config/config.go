package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
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
	// Try to load .env file (optional)
	_ = godotenv.Load()

	pollIntervalSec := getEnvAsInt("POLL_INTERVAL_SECONDS", 30)
	slackWebhookURL := getEnv("SLACK_WEBHOOK_URL", "")
	slackEnabled := getEnvAsBool("SLACK_ENABLED", true) && slackWebhookURL != ""

	cfg := &Config{
		OctopusAPIKey:        getEnv("OCTOPUS_API_KEY", ""),
		OctopusAccountNumber: getEnv("OCTOPUS_ACCOUNT_NUMBER", ""),
		InfluxDBURL:          getEnv("INFLUXDB_URL", "http://localhost:8086"),
		InfluxDBToken:        getEnv("INFLUXDB_TOKEN", ""),
		InfluxDBOrg:          getEnv("INFLUXDB_ORG", ""),
		InfluxDBBucket:       getEnv("INFLUXDB_BUCKET", "octopus_energy"),
		SlackWebhookURL:      slackWebhookURL,
		SlackEnabled:         slackEnabled,
		PollInterval:         time.Duration(pollIntervalSec) * time.Second,
		CacheDir:             getEnv("CACHE_DIR", "./cache"),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if required configuration values are present
func (c *Config) Validate() error {
	if c.OctopusAPIKey == "" {
		return fmt.Errorf("OCTOPUS_API_KEY is required")
	}
	if c.OctopusAccountNumber == "" {
		return fmt.Errorf("OCTOPUS_ACCOUNT_NUMBER is required")
	}
	if c.InfluxDBURL == "" {
		return fmt.Errorf("INFLUXDB_URL is required")
	}
	if c.InfluxDBToken == "" {
		return fmt.Errorf("INFLUXDB_TOKEN is required")
	}
	if c.InfluxDBOrg == "" {
		return fmt.Errorf("INFLUXDB_ORG is required")
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
