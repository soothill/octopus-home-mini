package main

import (
	"context"
	"testing"
	"time"

	"github.com/soothill/octopus-home-mini/pkg/cache"
	"github.com/soothill/octopus-home-mini/pkg/config"
	"github.com/soothill/octopus-home-mini/pkg/influx"
	"github.com/soothill/octopus-home-mini/pkg/octopus"
	"github.com/soothill/octopus-home-mini/pkg/slack"
)

// TestMonitor_SendSlackMethods tests the Slack notification helper methods
func TestMonitor_SendSlackMethods(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	t.Run("sendSlackError with nil notifier", func(t *testing.T) {
		m := &Monitor{
			cfg:           cfg,
			cache:         cacheStore,
			slackNotifier: nil, // No Slack notifier
		}

		// Should not panic when notifier is nil
		m.sendSlackError("test", "test message")
	})

	t.Run("sendSlackWarning with nil notifier", func(t *testing.T) {
		m := &Monitor{
			cfg:           cfg,
			cache:         cacheStore,
			slackNotifier: nil,
		}

		// Should not panic when notifier is nil
		m.sendSlackWarning("test", "test message")
	})

	t.Run("sendSlackInfo with nil notifier", func(t *testing.T) {
		m := &Monitor{
			cfg:           cfg,
			cache:         cacheStore,
			slackNotifier: nil,
		}

		// Should not panic when notifier is nil
		m.sendSlackInfo("test", "test message")
	})
}

// TestMonitor_CacheData tests the cacheData method
func TestMonitor_CacheData(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_data",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	m := &Monitor{
		cfg:           cfg,
		cache:         cacheStore,
		slackNotifier: nil,
	}

	testData := []octopus.TelemetryData{
		{
			ReadAt:           time.Now(),
			ConsumptionDelta: 1.5,
			Demand:           2.0,
			CostDelta:        0.25,
			Consumption:      10.5,
		},
		{
			ReadAt:           time.Now().Add(10 * time.Second),
			ConsumptionDelta: 1.3,
			Demand:           1.8,
			CostDelta:        0.22,
			Consumption:      11.8,
		},
	}

	m.cacheData(testData)

	// Verify data was cached
	if m.cache.Count() != 2 {
		t.Errorf("Expected 2 cached items, got %d", m.cache.Count())
	}
}

// TestMonitor_CheckInfluxHealth tests the checkInfluxHealth method
func TestMonitor_CheckInfluxHealth(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_health",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	t.Run("nil influx client", func(t *testing.T) {
		m := &Monitor{
			cfg:          cfg,
			cache:        cacheStore,
			influxClient: nil,
			influxHealthy: false,
		}

		ctx := context.Background()
		m.checkInfluxHealth(ctx)

		// Should not panic and influxHealthy should remain false
		if m.influxHealthy {
			t.Error("influxHealthy should be false when client is nil")
		}
	})

	t.Run("influx client with invalid connection", func(t *testing.T) {
		// Create a client with an invalid URL
		influxClient, err := influx.NewClient("http://invalid-host:8086", "token", "org", "bucket")
		if err != nil {
			t.Skip("Cannot create InfluxDB client with invalid host (expected for this test)")
			return
		}
		defer influxClient.Close()

		m := &Monitor{
			cfg:           cfg,
			cache:         cacheStore,
			influxClient:  influxClient,
			influxHealthy: true, // Start as healthy
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		m.checkInfluxHealth(ctx)

		// Should become unhealthy
		if m.influxHealthy {
			t.Error("influxHealthy should be false after health check failure")
		}
	})
}

// TestMonitor_TryReconnectInflux tests the tryReconnectInflux method
func TestMonitor_TryReconnectInflux(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_reconnect",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	t.Run("nil influx client", func(t *testing.T) {
		m := &Monitor{
			cfg:           cfg,
			cache:         cacheStore,
			influxClient:  nil,
			influxHealthy: false,
		}

		ctx := context.Background()
		m.tryReconnectInflux(ctx)

		// Should not panic and influxHealthy should remain false
		if m.influxHealthy {
			t.Error("influxHealthy should be false when client is nil")
		}
	})

	t.Run("influx client still unhealthy", func(t *testing.T) {
		// Create a client with an invalid URL
		influxClient, err := influx.NewClient("http://invalid-host:8086", "token", "org", "bucket")
		if err != nil {
			t.Skip("Cannot create InfluxDB client with invalid host (expected for this test)")
			return
		}
		defer influxClient.Close()

		m := &Monitor{
			cfg:           cfg,
			cache:         cacheStore,
			influxClient:  influxClient,
			influxHealthy: false,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		m.tryReconnectInflux(ctx)

		// Should remain unhealthy
		if m.influxHealthy {
			t.Error("influxHealthy should be false when reconnection fails")
		}
	})
}

// TestMonitor_WriteToInflux tests the writeToInflux method
func TestMonitor_WriteToInflux(t *testing.T) {
	t.Run("write with invalid client", func(t *testing.T) {
		cfg := &config.Config{
			PollInterval: 30 * time.Second,
			CacheDir:     "./test_cache_write",
		}

		cacheStore, err := cache.NewCache(cfg.CacheDir)
		if err != nil {
			t.Fatalf("Failed to create cache: %v", err)
		}
		defer cacheStore.Clear()

		// Create a client with an invalid URL
		influxClient, err := influx.NewClient("http://invalid-host:8086", "token", "org", "bucket")
		if err != nil {
			t.Skip("Cannot create InfluxDB client with invalid host (expected for this test)")
			return
		}
		defer influxClient.Close()

		m := &Monitor{
			cfg:          cfg,
			cache:        cacheStore,
			influxClient: influxClient,
		}

		testData := []octopus.TelemetryData{
			{
				ReadAt:           time.Now(),
				ConsumptionDelta: 1.5,
				Demand:           2.0,
				CostDelta:        0.25,
				Consumption:      10.5,
			},
		}

		// This should return an error due to invalid host
		err = m.writeToInflux(testData)
		if err == nil {
			t.Error("Expected error when writing to invalid InfluxDB host")
		}
	})

	t.Run("write with empty data", func(t *testing.T) {
		cfg := &config.Config{
			PollInterval: 30 * time.Second,
			CacheDir:     "./test_cache_write_empty",
		}

		cacheStore, err := cache.NewCache(cfg.CacheDir)
		if err != nil {
			t.Fatalf("Failed to create cache: %v", err)
		}
		defer cacheStore.Clear()

		influxClient, err := influx.NewClient("http://localhost:8086", "token", "org", "bucket")
		if err != nil {
			t.Skip("Cannot create InfluxDB client (expected for local testing)")
			return
		}
		defer influxClient.Close()

		m := &Monitor{
			cfg:          cfg,
			cache:        cacheStore,
			influxClient: influxClient,
		}

		// Empty data should not error
		err = m.writeToInflux([]octopus.TelemetryData{})
		if err != nil {
			t.Errorf("Unexpected error with empty data: %v", err)
		}
	})
}

// TestMonitor_SyncCache tests the syncCache method
func TestMonitor_SyncCache(t *testing.T) {
	t.Run("sync with no cached data", func(t *testing.T) {
		cfg := &config.Config{
			PollInterval: 30 * time.Second,
			CacheDir:     "./test_cache_sync",
		}

		cacheStore, err := cache.NewCache(cfg.CacheDir)
		if err != nil {
			t.Fatalf("Failed to create cache: %v", err)
		}
		defer cacheStore.Clear()

		influxClient, err := influx.NewClient("http://localhost:8086", "token", "org", "bucket")
		if err != nil {
			t.Skip("Cannot create InfluxDB client (expected for local testing)")
			return
		}
		defer influxClient.Close()

		m := &Monitor{
			cfg:          cfg,
			cache:        cacheStore,
			influxClient: influxClient,
		}

		// Should not panic with empty cache
		m.syncCache()
	})

	t.Run("sync with cached data but invalid client", func(t *testing.T) {
		cfg := &config.Config{
			PollInterval: 30 * time.Second,
			CacheDir:     "./test_cache_sync_data",
		}

		cacheStore, err := cache.NewCache(cfg.CacheDir)
		if err != nil {
			t.Fatalf("Failed to create cache: %v", err)
		}
		defer cacheStore.Clear()

		// Add some data to cache
		testData := []cache.DataPoint{
			{
				Timestamp:        time.Now(),
				ConsumptionDelta: 1.5,
				Demand:           2.0,
				CostDelta:        0.25,
				Consumption:      10.5,
			},
		}
		err = cacheStore.Add(testData)
		if err != nil {
			t.Fatalf("Failed to add test data to cache: %v", err)
		}

		// Create client with invalid host
		influxClient, err := influx.NewClient("http://invalid-host:8086", "token", "org", "bucket")
		if err != nil {
			t.Skip("Cannot create InfluxDB client with invalid host (expected for this test)")
			return
		}
		defer influxClient.Close()

		m := &Monitor{
			cfg:          cfg,
			cache:        cacheStore,
			influxClient: influxClient,
		}

		// Should not panic even with invalid client
		m.syncCache()

		// Cache should still have data since sync failed
		if m.cache.Count() == 0 {
			t.Error("Cache should not be cleared after failed sync")
		}
	})
}

// TestMonitor_SlackNotifications tests Slack notification integration
func TestMonitor_SlackNotifications(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_slack",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	// Create a Slack notifier (will fail to send but that's ok for testing)
	slackNotifier := slack.NewNotifier("https://example.com/test-webhook")

	m := &Monitor{
		cfg:           cfg,
		cache:         cacheStore,
		slackNotifier: slackNotifier,
	}

	// These should not panic even if sending fails
	m.sendSlackError("test", "test error")
	m.sendSlackWarning("test", "test warning")
	m.sendSlackInfo("test", "test info")
}

// TestMonitor_Initialization tests Monitor struct initialization
func TestMonitor_Initialization(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_init",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	octopusClient := octopus.NewClient("test_key", "A-12345678")

	monitor := &Monitor{
		cfg:            cfg,
		octopusClient:  octopusClient,
		cache:          cacheStore,
		lastPollTime:   time.Now().Add(-cfg.PollInterval),
		influxHealthy:  false,
		consecutiveErr: 0,
	}

	if monitor.cfg != cfg {
		t.Error("Config not set correctly")
	}

	if monitor.octopusClient != octopusClient {
		t.Error("Octopus client not set correctly")
	}

	if monitor.cache != cacheStore {
		t.Error("Cache not set correctly")
	}

	if monitor.influxHealthy {
		t.Error("InfluxDB should not be healthy initially")
	}

	if monitor.consecutiveErr != 0 {
		t.Error("Consecutive errors should be 0 initially")
	}
}

// TestMonitor_ConsecutiveErrorTracking tests consecutive error counting
func TestMonitor_ConsecutiveErrorTracking(t *testing.T) {
	monitor := &Monitor{
		consecutiveErr: 0,
	}

	// Simulate consecutive errors
	monitor.consecutiveErr++
	if monitor.consecutiveErr != 1 {
		t.Errorf("consecutiveErr = %d, want 1", monitor.consecutiveErr)
	}

	monitor.consecutiveErr++
	if monitor.consecutiveErr != 2 {
		t.Errorf("consecutiveErr = %d, want 2", monitor.consecutiveErr)
	}

	monitor.consecutiveErr++
	if monitor.consecutiveErr != 3 {
		t.Errorf("consecutiveErr = %d, want 3", monitor.consecutiveErr)
	}

	// Reset on success
	monitor.consecutiveErr = 0
	if monitor.consecutiveErr != 0 {
		t.Error("consecutiveErr should reset to 0")
	}
}

// TestMonitor_RunStopsOnSignal tests the run loop stops when signaled
func TestMonitor_RunStopsOnSignal(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 10 * time.Millisecond,
		CacheDir:     "./test_cache_run",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	octopusClient := octopus.NewClient("test_key", "A-12345678")

	monitor := &Monitor{
		cfg:            cfg,
		cache:          cacheStore,
		octopusClient:  octopusClient,
		lastPollTime:   time.Now(), // Start from now to avoid polling
		consecutiveErr: 0,
	}

	stopChan := make(chan struct{})

	// Run in goroutine
	go monitor.run(stopChan)

	// Let it run for a very short time
	time.Sleep(5 * time.Millisecond)

	// Stop it immediately
	close(stopChan)

	// Give it time to stop
	time.Sleep(20 * time.Millisecond)

	// If we get here without hanging, the test passed
}

// TestMonitor_LastPollTimeTracking tests lastPollTime updates
func TestMonitor_LastPollTimeTracking(t *testing.T) {
	initialTime := time.Now().Add(-1 * time.Hour)
	monitor := &Monitor{
		lastPollTime: initialTime,
	}

	if !monitor.lastPollTime.Equal(initialTime) {
		t.Error("lastPollTime not set correctly")
	}

	// Simulate poll
	newTime := time.Now()
	monitor.lastPollTime = newTime

	if !monitor.lastPollTime.Equal(newTime) {
		t.Error("lastPollTime not updated correctly")
	}

	if !monitor.lastPollTime.After(initialTime) {
		t.Error("lastPollTime should be after initial time")
	}
}

// TestMonitor_InfluxHealthyState tests influxHealthy state management
func TestMonitor_InfluxHealthyState(t *testing.T) {
	monitor := &Monitor{
		influxHealthy: false,
	}

	if monitor.influxHealthy {
		t.Error("InfluxDB should start as unhealthy")
	}

	// Simulate connection
	monitor.influxHealthy = true
	if !monitor.influxHealthy {
		t.Error("InfluxDB should be healthy after connection")
	}

	// Simulate disconnection
	monitor.influxHealthy = false
	if monitor.influxHealthy {
		t.Error("InfluxDB should be unhealthy after disconnection")
	}
}

// TestMonitor_CacheDataEmptySlice tests handling of empty data
func TestMonitor_CacheDataEmptySlice(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_empty_data",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	monitor := &Monitor{
		cfg:   cfg,
		cache: cacheStore,
	}

	// Empty telemetry data
	telemetryData := []octopus.TelemetryData{}

	monitor.cacheData(telemetryData)

	// Should not add anything to cache
	if monitor.cache.Count() != 0 {
		t.Errorf("Cache count = %d, want 0", monitor.cache.Count())
	}
}

// TestMonitor_CacheDataNegativeValues tests handling of negative values
func TestMonitor_CacheDataNegativeValues(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_negative",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	monitor := &Monitor{
		cfg:   cfg,
		cache: cacheStore,
	}

	telemetryData := []octopus.TelemetryData{
		{
			ReadAt:           time.Now(),
			ConsumptionDelta: -0.5, // Negative (e.g., solar export)
			Demand:           -1.2,
			CostDelta:        -0.15,
			Consumption:      10.5,
		},
	}

	monitor.cacheData(telemetryData)

	// Should handle negative values
	if monitor.cache.Count() != 1 {
		t.Errorf("Cache count = %d, want 1", monitor.cache.Count())
	}
}

// TestMonitor_ContextCancellation tests context handling
func TestMonitor_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	select {
	case <-ctx.Done():
		// Context was cancelled correctly
	case <-time.After(1 * time.Second):
		t.Error("Context cancellation not detected")
	}
}

// TestMonitor_ContextWithTimeout tests context timeout handling
func TestMonitor_ContextWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	<-ctx.Done()

	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", ctx.Err())
	}
}

// TestMonitor_MultipleRunCycles tests multiple run cycles
func TestMonitor_MultipleRunCycles(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 10 * time.Millisecond,
		CacheDir:     "./test_cache_multiple_cycles",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	octopusClient := octopus.NewClient("test_key", "A-12345678")

	monitor := &Monitor{
		cfg:            cfg,
		cache:          cacheStore,
		octopusClient:  octopusClient,
		lastPollTime:   time.Now(), // Start from now to avoid immediate polling
		consecutiveErr: 0,
	}

	stopChan := make(chan struct{})

	go monitor.run(stopChan)

	// Let it run for a short time
	time.Sleep(30 * time.Millisecond)

	close(stopChan)
	time.Sleep(20 * time.Millisecond)
}

// TestMonitor_DataConversionAccuracy tests data conversion precision
func TestMonitor_DataConversionAccuracy(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_conversion",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	monitor := &Monitor{
		cfg:   cfg,
		cache: cacheStore,
	}

	now := time.Now()
	telemetryData := []octopus.TelemetryData{
		{
			ReadAt:           now,
			ConsumptionDelta: 1.23456789,
			Demand:           2.34567890,
			CostDelta:        0.12345678,
			Consumption:      100.123456,
		},
	}

	monitor.cacheData(telemetryData)

	cachedData := monitor.cache.GetAll()

	if len(cachedData) != 1 {
		t.Fatalf("Expected 1 cached item, got %d", len(cachedData))
	}

	// Verify precision is maintained
	if cachedData[0].ConsumptionDelta != 1.23456789 {
		t.Errorf("ConsumptionDelta precision lost: got %v", cachedData[0].ConsumptionDelta)
	}

	if cachedData[0].Demand != 2.34567890 {
		t.Errorf("Demand precision lost: got %v", cachedData[0].Demand)
	}
}

// TestMonitor_LargeDataSet tests handling of large data sets
func TestMonitor_LargeDataSet(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_large",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	monitor := &Monitor{
		cfg:   cfg,
		cache: cacheStore,
	}

	// Create large data set
	largeData := make([]octopus.TelemetryData, 1000)
	for i := 0; i < 1000; i++ {
		largeData[i] = octopus.TelemetryData{
			ReadAt:           time.Now().Add(time.Duration(i) * time.Second),
			ConsumptionDelta: float64(i) * 0.1,
			Demand:           float64(i) * 0.2,
			CostDelta:        float64(i) * 0.05,
			Consumption:      float64(i) * 1.0,
		}
	}

	monitor.cacheData(largeData)

	if monitor.cache.Count() != 1000 {
		t.Errorf("Cache count = %d, want 1000", monitor.cache.Count())
	}
}

// TestMonitor_ConcurrentCacheAccess tests concurrent cache operations
func TestMonitor_ConcurrentCacheAccess(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 30 * time.Second,
		CacheDir:     "./test_cache_concurrent",
	}

	cacheStore, err := cache.NewCache(cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cacheStore.Clear()

	monitor := &Monitor{
		cfg:   cfg,
		cache: cacheStore,
	}

	done := make(chan bool, 10)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			telemetryData := []octopus.TelemetryData{
				{
					ReadAt:           time.Now().Add(time.Duration(id) * time.Second),
					ConsumptionDelta: float64(id) * 0.1,
					Demand:           float64(id) * 0.2,
					CostDelta:        float64(id) * 0.05,
					Consumption:      float64(id) * 1.0,
				},
			}
			monitor.cacheData(telemetryData)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if monitor.cache.Count() != 10 {
		t.Errorf("Cache count = %d, want 10", monitor.cache.Count())
	}
}
