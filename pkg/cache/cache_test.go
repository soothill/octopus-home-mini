package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	tests := []struct {
		name      string
		cacheDir  string
		wantErr   bool
		setupFunc func(string) error
	}{
		{
			name:     "valid cache directory",
			cacheDir: filepath.Join(os.TempDir(), "test_cache_valid"),
			wantErr:  false,
		},
		{
			name:     "cache directory with spaces",
			cacheDir: filepath.Join(os.TempDir(), "test cache with spaces"),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setupFunc != nil {
				if err := tt.setupFunc(tt.cacheDir); err != nil {
					t.Fatalf("Setup failed: %v", err)
				}
			}

			// Cleanup
			defer os.RemoveAll(tt.cacheDir)

			cache, err := NewCache(tt.cacheDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewCache() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewCache() unexpected error = %v", err)
				return
			}

			if cache == nil {
				t.Error("NewCache() returned nil cache")
			}

			// Verify directory was created
			if _, err := os.Stat(tt.cacheDir); os.IsNotExist(err) {
				t.Errorf("Cache directory was not created: %v", err)
			}
		})
	}
}

func TestCache_AddAndGetAll(t *testing.T) {
	cacheDir := filepath.Join(os.TempDir(), "test_cache_add")
	defer os.RemoveAll(cacheDir)

	cache, err := NewCache(cacheDir)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	testData := []DataPoint{
		{
			Timestamp:        time.Now(),
			ConsumptionDelta: 0.5,
			Demand:           1.2,
			CostDelta:        0.15,
			Consumption:      10.5,
		},
		{
			Timestamp:        time.Now().Add(10 * time.Second),
			ConsumptionDelta: 0.6,
			Demand:           1.3,
			CostDelta:        0.18,
			Consumption:      11.1,
		},
	}

	// Test Add
	err = cache.Add(testData)
	if err != nil {
		t.Errorf("Add() error = %v", err)
	}

	// Test GetAll
	retrieved := cache.GetAll()
	if len(retrieved) != len(testData) {
		t.Errorf("GetAll() returned %d items, want %d", len(retrieved), len(testData))
	}

	for i, dp := range retrieved {
		if dp.ConsumptionDelta != testData[i].ConsumptionDelta {
			t.Errorf("DataPoint[%d].ConsumptionDelta = %v, want %v", i, dp.ConsumptionDelta, testData[i].ConsumptionDelta)
		}
		if dp.Demand != testData[i].Demand {
			t.Errorf("DataPoint[%d].Demand = %v, want %v", i, dp.Demand, testData[i].Demand)
		}
	}
}

func TestCache_AddSingle(t *testing.T) {
	cacheDir := filepath.Join(os.TempDir(), "test_cache_add_single")
	defer os.RemoveAll(cacheDir)

	cache, err := NewCache(cacheDir)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	testDP := DataPoint{
		Timestamp:        time.Now(),
		ConsumptionDelta: 0.5,
		Demand:           1.2,
		CostDelta:        0.15,
		Consumption:      10.5,
	}

	err = cache.AddSingle(testDP)
	if err != nil {
		t.Errorf("AddSingle() error = %v", err)
	}

	retrieved := cache.GetAll()
	if len(retrieved) != 1 {
		t.Errorf("GetAll() returned %d items, want 1", len(retrieved))
	}
}

func TestCache_Count(t *testing.T) {
	cacheDir := filepath.Join(os.TempDir(), "test_cache_count")
	defer os.RemoveAll(cacheDir)

	cache, err := NewCache(cacheDir)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	if cache.Count() != 0 {
		t.Errorf("Count() = %d, want 0 for empty cache", cache.Count())
	}

	testData := []DataPoint{
		{Timestamp: time.Now(), ConsumptionDelta: 0.5},
		{Timestamp: time.Now(), ConsumptionDelta: 0.6},
		{Timestamp: time.Now(), ConsumptionDelta: 0.7},
	}

	cache.Add(testData)

	if cache.Count() != 3 {
		t.Errorf("Count() = %d, want 3", cache.Count())
	}
}

func TestCache_Clear(t *testing.T) {
	cacheDir := filepath.Join(os.TempDir(), "test_cache_clear")
	defer os.RemoveAll(cacheDir)

	cache, err := NewCache(cacheDir)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Add some data
	testData := []DataPoint{
		{Timestamp: time.Now(), ConsumptionDelta: 0.5},
		{Timestamp: time.Now(), ConsumptionDelta: 0.6},
	}
	cache.Add(testData)

	// Clear
	err = cache.Clear()
	if err != nil {
		t.Errorf("Clear() error = %v", err)
	}

	if cache.Count() != 0 {
		t.Errorf("Count() = %d after Clear(), want 0", cache.Count())
	}
}

func TestCache_LoadAndSave(t *testing.T) {
	cacheDir := filepath.Join(os.TempDir(), "test_cache_load_save")
	defer os.RemoveAll(cacheDir)

	// Create first cache instance and add data
	cache1, err := NewCache(cacheDir)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	testData := []DataPoint{
		{
			Timestamp:        time.Now().Truncate(time.Second), // Truncate for comparison
			ConsumptionDelta: 0.5,
			Demand:           1.2,
			CostDelta:        0.15,
			Consumption:      10.5,
		},
	}

	err = cache1.Add(testData)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Create second cache instance and load data
	cache2, err := NewCache(cacheDir)
	if err != nil {
		t.Fatalf("NewCache() second instance error = %v", err)
	}

	retrieved := cache2.GetAll()
	if len(retrieved) != len(testData) {
		t.Errorf("Loaded cache has %d items, want %d", len(retrieved), len(testData))
	}

	if len(retrieved) > 0 {
		if retrieved[0].ConsumptionDelta != testData[0].ConsumptionDelta {
			t.Errorf("Loaded ConsumptionDelta = %v, want %v", retrieved[0].ConsumptionDelta, testData[0].ConsumptionDelta)
		}
	}
}

func TestCache_CleanupOldFiles(t *testing.T) {
	cacheDir := filepath.Join(os.TempDir(), "test_cache_cleanup")
	defer os.RemoveAll(cacheDir)

	cache, err := NewCache(cacheDir)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Create an old cache file
	oldFile := filepath.Join(cacheDir, "cache_2020-01-01.json")
	err = os.WriteFile(oldFile, []byte("[]"), 0644)
	if err != nil {
		t.Fatalf("Failed to create old cache file: %v", err)
	}

	// Set file modification time to 2 days ago
	oldTime := time.Now().Add(-48 * time.Hour)
	err = os.Chtimes(oldFile, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to set file time: %v", err)
	}

	// Cleanup files older than 24 hours
	err = cache.CleanupOldFiles(24 * time.Hour)
	if err != nil {
		t.Errorf("CleanupOldFiles() error = %v", err)
	}

	// Verify old file was removed
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("Old cache file was not removed")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cacheDir := filepath.Join(os.TempDir(), "test_cache_concurrent")
	defer os.RemoveAll(cacheDir)

	cache, err := NewCache(cacheDir)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Test concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			dp := DataPoint{
				Timestamp:        time.Now(),
				ConsumptionDelta: float64(n),
			}
			cache.AddSingle(dp)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all items were added
	count := cache.Count()
	if count != 10 {
		t.Errorf("Count() = %d after concurrent writes, want 10", count)
	}
}
