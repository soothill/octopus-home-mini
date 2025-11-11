package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DataPoint represents a cached energy measurement
type DataPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	ConsumptionDelta float64   `json:"consumption_delta"`
	Demand           float64   `json:"demand"`
	CostDelta        float64   `json:"cost_delta"`
	Consumption      float64   `json:"consumption"`
}

// Cache handles local storage of data points when InfluxDB is unavailable
type Cache struct {
	cacheDir string
	mu       sync.Mutex
	data     []DataPoint
}

// NewCache creates a new cache instance
func NewCache(cacheDir string) (*Cache, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cache := &Cache{
		cacheDir: cacheDir,
		data:     make([]DataPoint, 0),
	}

	// Load existing cached data
	if err := cache.Load(); err != nil {
		// Log error but don't fail - start with empty cache
		fmt.Printf("Warning: failed to load existing cache: %v\n", err)
	}

	return cache, nil
}

// Add adds data points to the cache
func (c *Cache) Add(dataPoints []DataPoint) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = append(c.data, dataPoints...)

	return c.save()
}

// AddSingle adds a single data point to the cache
func (c *Cache) AddSingle(dp DataPoint) error {
	return c.Add([]DataPoint{dp})
}

// GetAll returns all cached data points
func (c *Cache) GetAll() []DataPoint {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return a copy to prevent external modification
	result := make([]DataPoint, len(c.data))
	copy(result, c.data)
	return result
}

// Clear removes all cached data
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make([]DataPoint, 0)
	return c.save()
}

// Count returns the number of cached data points
func (c *Cache) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.data)
}

// save persists the cache to disk
func (c *Cache) save() error {
	filename := filepath.Join(c.cacheDir, fmt.Sprintf("cache_%s.json", time.Now().Format("2006-01-02")))

	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Load loads cached data from disk
func (c *Cache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find the most recent cache file
	files, err := filepath.Glob(filepath.Join(c.cacheDir, "cache_*.json"))
	if err != nil {
		return fmt.Errorf("failed to list cache files: %w", err)
	}

	if len(files) == 0 {
		// No cache files found, start fresh
		c.data = make([]DataPoint, 0)
		return nil
	}

	// Get the most recent file
	latestFile := files[len(files)-1]

	data, err := os.ReadFile(latestFile)
	if err != nil {
		return fmt.Errorf("failed to read cache file: %w", err)
	}

	if err := json.Unmarshal(data, &c.data); err != nil {
		return fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	return nil
}

// CleanupOldFiles removes cache files older than the specified duration
func (c *Cache) CleanupOldFiles(maxAge time.Duration) error {
	files, err := filepath.Glob(filepath.Join(c.cacheDir, "cache_*.json"))
	if err != nil {
		return fmt.Errorf("failed to list cache files: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(file); err != nil {
				fmt.Printf("Warning: failed to remove old cache file %s: %v\n", file, err)
			}
		}
	}

	return nil
}
