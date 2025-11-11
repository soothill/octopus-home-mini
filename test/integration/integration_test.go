package integration

import (
	"context"
	"testing"
	"time"

	"github.com/soothill/octopus-home-mini/pkg/influx"
)

// TestInfluxDBIntegration tests the full integration with real InfluxDB
func TestInfluxDBIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewTestConfig(t)
	SkipIfNoInfluxDB(t, cfg)
	defer CleanupInfluxDB(t, cfg)

	// Create InfluxDB client
	influxClient, err := influx.NewClient(cfg.InfluxDBURL, cfg.InfluxDBToken, cfg.InfluxDBOrg, cfg.InfluxDBBucket, cfg.InfluxDBMeasurement)
	if err != nil {
		t.Fatalf("Failed to create InfluxDB client: %v", err)
	}
	defer influxClient.Close()

	// Test writing data points
	testData := CreateInfluxDataPoints(10)

	for _, dp := range testData {
		err := influxClient.WriteDataPoint(dp)
		if err != nil {
			t.Errorf("Failed to write data point: %v", err)
		}
	}

	// Flush to ensure writes complete
	influxClient.Flush()
	time.Sleep(2 * time.Second)

	// Verify connection is still healthy
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = influxClient.CheckConnection(ctx)
	if err != nil {
		t.Errorf("InfluxDB connection check failed: %v", err)
	}
}

// TestInfluxDBBlockingWrites tests synchronous writes to InfluxDB
func TestInfluxDBBlockingWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewTestConfig(t)
	SkipIfNoInfluxDB(t, cfg)
	defer CleanupInfluxDB(t, cfg)

	influxClient, err := influx.NewClient(cfg.InfluxDBURL, cfg.InfluxDBToken, cfg.InfluxDBOrg, cfg.InfluxDBBucket, cfg.InfluxDBMeasurement)
	if err != nil {
		t.Fatalf("Failed to create InfluxDB client: %v", err)
	}
	defer influxClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testData := CreateInfluxDataPoints(5)

	// Test blocking writes
	for _, dp := range testData {
		err := influxClient.WritePointDirectly(ctx, dp)
		if err != nil {
			t.Errorf("WritePointDirectly failed: %v", err)
		}
	}
}

// TestInfluxDBHealthCheck tests InfluxDB health checking
func TestInfluxDBHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewTestConfig(t)
	SkipIfNoInfluxDB(t, cfg)

	influxClient, err := influx.NewClient(cfg.InfluxDBURL, cfg.InfluxDBToken, cfg.InfluxDBOrg, cfg.InfluxDBBucket, cfg.InfluxDBMeasurement)
	if err != nil {
		t.Fatalf("Failed to create InfluxDB client: %v", err)
	}
	defer influxClient.Close()

	// Perform multiple health checks
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := influxClient.CheckConnection(ctx)
		cancel()

		if err != nil {
			t.Errorf("Health check %d failed: %v", i, err)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// TestInfluxDBBatchWrites tests writing multiple data points in batches
func TestInfluxDBBatchWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewTestConfig(t)
	SkipIfNoInfluxDB(t, cfg)
	defer CleanupInfluxDB(t, cfg)

	influxClient, err := influx.NewClient(cfg.InfluxDBURL, cfg.InfluxDBToken, cfg.InfluxDBOrg, cfg.InfluxDBBucket, cfg.InfluxDBMeasurement)
	if err != nil {
		t.Fatalf("Failed to create InfluxDB client: %v", err)
	}
	defer influxClient.Close()

	// Create large batch
	testData := CreateInfluxDataPoints(100)

	// Write batch
	err = influxClient.WriteDataPoints(testData)
	if err != nil {
		t.Errorf("WriteDataPoints failed: %v", err)
	}

	influxClient.Flush()
	time.Sleep(2 * time.Second)

	// Verify connection is still healthy after batch write
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = influxClient.CheckConnection(ctx)
	if err != nil {
		t.Errorf("InfluxDB connection check failed after batch write: %v", err)
	}
}

// TestCacheDataFlow tests cache operations
func TestCacheDataFlow(t *testing.T) {
	testCache := CreateTestCache(t)
	defer testCache.Clear()

	// Test adding data
	testData := CreateCacheDataPoints(20)

	err := testCache.Add(testData)
	if err != nil {
		t.Fatalf("Failed to add data to cache: %v", err)
	}

	if testCache.Count() != 20 {
		t.Errorf("Expected 20 cached items, got %d", testCache.Count())
	}

	// Retrieve data
	cachedData := testCache.GetAll()
	if len(cachedData) != 20 {
		t.Errorf("Expected 20 items from GetAll, got %d", len(cachedData))
	}

	// Clear cache
	if err := testCache.Clear(); err != nil {
		t.Errorf("Failed to clear cache: %v", err)
	}

	if testCache.Count() != 0 {
		t.Errorf("Cache should be empty after clear, got %d items", testCache.Count())
	}
}
