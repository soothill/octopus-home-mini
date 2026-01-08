package main

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestSetupLogger(t *testing.T) {
	// Test with a valid log level
	setupLogger("debug")
	assert.Equal(t, zerolog.GlobalLevel(), zerolog.DebugLevel)

	// Test with an invalid log level
	setupLogger("invalid")
	assert.Equal(t, zerolog.GlobalLevel(), zerolog.InfoLevel)
}

func TestLoadConfig(t *testing.T) {
	// Create a dummy config file for testing
	configContent := `
log_level: "debug"
cache_dir: "/tmp/octopus-home-mini-test"
octopus_api_key: "test-api-key"
octopus_account_number: "test-account-number"
influxdb:
  url: "http://localhost:8086"
  token: "test-token"
  org: "test-org"
  bucket: "test-bucket"
`
	configFile, err := os.CreateTemp("", "config-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(configFile.Name())

	_, err = configFile.WriteString(configContent)
	assert.NoError(t, err)
	configFile.Close()

	os.Setenv("OCTOPUS_CONFIG_PATH", configFile.Name())
	defer os.Unsetenv("OCTOPUS_CONFIG_PATH")

	cfg, err := loadConfig()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestInitCache(t *testing.T) {
	cacheDir, err := os.MkdirTemp("", "cache-test")
	assert.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	cache, err := initCache(cacheDir)
	assert.NoError(t, err)
	assert.NotNil(t, cache)
}

func TestInitSlackNotifier(t *testing.T) {
	// Test with Slack disabled
	notifier := initSlackNotifier(false, "")
	assert.Nil(t, notifier)

	// Test with Slack enabled
	notifier = initSlackNotifier(true, "http://localhost:12345")
	assert.NotNil(t, notifier)
}

func TestInitOctopusClient(t *testing.T) {
	// This is a basic test to ensure the client is created.
	// A more comprehensive test would require mocking the Octopus API.
	client, err := initOctopusClient("test-api-key", "test-account-number")
	assert.NoError(t, err)
	assert.NotNil(t, client)
}
