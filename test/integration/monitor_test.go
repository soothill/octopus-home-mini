package integration

import (
	"testing"
	"time"

	"github.com/soothill/octopus-home-mini/pkg/influx"
	"github.com/soothill/octopus-home-mini/pkg/monitor"
	"github.com/soothill/octopus-home-mini/pkg/octopus"
)

func TestMonitorLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg, server := SetupTestEnvironment(t)
	defer server.Close()

	// Create clients
	octopusClient := octopus.NewClientWithEndpoint(cfg.OctopusAPIKey, cfg.OctopusAccountNumber, server.URL)
	influxClient, err := influx.NewClient(cfg.InfluxDBURL, cfg.InfluxDBToken, cfg.InfluxDBOrg, cfg.InfluxDBBucket, cfg.InfluxDBMeasurement)
	if err != nil {
		t.Fatalf("Failed to create InfluxDB client: %v", err)
	}
	defer influxClient.Close()

	cache := CreateTestCache(t)
	defer cache.Clear()

	// Create monitor
	appMonitor := monitor.New(cfg, octopusClient, influxClient, cache, nil)

	// Run monitor in a goroutine
	stopChan := make(chan struct{})
	go appMonitor.Run(stopChan)

	// Let it run for a short period
	time.Sleep(2 * time.Second)

	// Send shutdown signal
	close(stopChan)

	// Allow time for graceful shutdown
	time.Sleep(1 * time.Second)
}
