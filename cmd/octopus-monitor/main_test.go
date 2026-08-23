package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
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
octopus_api_key: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
octopus_account_number: "A-12345678"
influxdb_url: "http://localhost:8086"
influxdb_token: "test-token"
influxdb_org: "test-org"
influxdb_bucket: "test-bucket"
slack_enabled: false
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		body, _ := io.ReadAll(r.Body)

		switch {
		case strings.Contains(string(body), "obtainKrakenToken"):
			_, _ = w.Write([]byte(`{"data":{"obtainKrakenToken":{"token":"test-token"}}}`))
		case strings.Contains(string(body), "account"):
			_, _ = w.Write([]byte(`{"data":{"account":{"electricityAgreements":[{"meterPoint":{"meters":[{"smartDevices":[{"deviceId":"test-device"}]}]}}]}}}`))
		default:
			http.Error(w, `{"errors":[{"message":"unexpected query"}]}`, http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client, err := initOctopusClientWithEndpoint("test-api-key", "test-account-number", server.URL)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestInitOctopusClientRetriesRateLimit(t *testing.T) {
	var authAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)

		switch {
		case strings.Contains(string(body), "obtainKrakenToken"):
			if authAttempts.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"errors":[{"message":"Too many requests."}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"obtainKrakenToken":{"token":"test-token"}}}`))
		case strings.Contains(string(body), "account"):
			_, _ = w.Write([]byte(`{"data":{"account":{"electricityAgreements":[{"meterPoint":{"meters":[{"smartDevices":[{"deviceId":"test-device"}]}]}}]}}}`))
		default:
			http.Error(w, `{"errors":[{"message":"unexpected query"}]}`, http.StatusBadRequest)
		}
	}))
	defer server.Close()

	retryBackoff := backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Millisecond), 2)
	client, err := initOctopusClientWithBackoff("test-api-key", "test-account-number", server.URL, retryBackoff)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, int32(2), authAttempts.Load())
}
