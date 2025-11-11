package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/soothill/octopus-home-mini/pkg/cache"
	"github.com/soothill/octopus-home-mini/pkg/config"
	"github.com/soothill/octopus-home-mini/pkg/influx"
)

// NewTestConfig returns a configuration for integration tests
func NewTestConfig(t *testing.T) *config.Config {
	t.Helper()

	return &config.Config{
		OctopusAPIKey:        "test_api_key",
		OctopusAccountNumber: "A-12345678",
		InfluxDBURL:          getEnvOrDefault("INFLUXDB_URL", "http://localhost:8086"),
		InfluxDBToken:        getEnvOrDefault("INFLUXDB_TOKEN", "test-token-12345678901234567890"),
		InfluxDBOrg:          getEnvOrDefault("INFLUXDB_ORG", "test-org"),
		InfluxDBBucket:       getEnvOrDefault("INFLUXDB_BUCKET", "test-bucket"),
		PollInterval:         10 * time.Second,
		CacheDir:             t.TempDir(),
		LogLevel:             "info",
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// SkipIfNoInfluxDB skips the test if InfluxDB is not available
func SkipIfNoInfluxDB(t *testing.T, cfg *config.Config) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := influx.NewClient(cfg.InfluxDBURL, cfg.InfluxDBToken, cfg.InfluxDBOrg, cfg.InfluxDBBucket)
	if err != nil {
		t.Skipf("InfluxDB not available: %v", err)
		return
	}
	defer client.Close()

	if err := client.CheckConnection(ctx); err != nil {
		t.Skipf("Cannot connect to InfluxDB: %v", err)
	}
}

// MockOctopusServer creates a mock Octopus API server for testing
func MockOctopusServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{
			"data": {
				"smartMeterTelemetry": [
					{
						"readAt": "%s",
						"consumptionDelta": 0.5,
						"demand": 1.2,
						"costDelta": 0.15,
						"consumption": 10.5
					}
				]
			}
		}`, time.Now().Format(time.RFC3339))
	})

	return httptest.NewServer(handler)
}

// CreateTestCache creates a cache for testing
func CreateTestCache(t *testing.T) *cache.Cache {
	t.Helper()

	cacheDir := t.TempDir()
	c, err := cache.NewCache(cacheDir)
	if err != nil {
		t.Fatalf("Failed to create test cache: %v", err)
	}

	return c
}

// CleanupInfluxDB cleans up test data from InfluxDB
func CleanupInfluxDB(t *testing.T, cfg *config.Config) {
	t.Helper()
	// Using a separate test bucket that can be cleared
}

// CreateInfluxDataPoints creates test influx data points
func CreateInfluxDataPoints(count int) []influx.DataPoint {
	data := make([]influx.DataPoint, count)
	baseTime := time.Now().Add(-1 * time.Hour)

	for i := 0; i < count; i++ {
		data[i] = influx.DataPoint{
			Timestamp:        baseTime.Add(time.Duration(i) * 10 * time.Second),
			ConsumptionDelta: float64(i) * 0.1,
			Demand:           float64(i) * 0.2,
			CostDelta:        float64(i) * 0.05,
			Consumption:      float64(i) * 1.0,
		}
	}

	return data
}

// CreateCacheDataPoints creates test cache data points
func CreateCacheDataPoints(count int) []cache.DataPoint {
	data := make([]cache.DataPoint, count)
	baseTime := time.Now().Add(-1 * time.Hour)

	for i := 0; i < count; i++ {
		data[i] = cache.DataPoint{
			Timestamp:        baseTime.Add(time.Duration(i) * 10 * time.Second),
			ConsumptionDelta: float64(i) * 0.1,
			Demand:           float64(i) * 0.2,
			CostDelta:        float64(i) * 0.05,
			Consumption:      float64(i) * 1.0,
		}
	}

	return data
}
