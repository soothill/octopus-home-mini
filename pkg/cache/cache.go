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
	data     []DataPoint
	mu       sync.Mutex
	cacheDir string
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

	previousData := c.data
	c.data = make([]DataPoint, 0)
	if err := c.save(); err != nil {
		c.data = previousData
		return err
	}

	// Cache files are outage buffers, not archives. After every point has been
	// synced, retain only today's empty snapshot and remove redundant historical
	// snapshots so disk use does not grow indefinitely.
	currentFile := c.filename()
	files, err := filepath.Glob(filepath.Join(c.cacheDir, "cache_*.json"))
	if err != nil {
		return fmt.Errorf("failed to list cache files after clear: %w", err)
	}
	for _, file := range files {
		if file == currentFile {
			continue
		}
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("failed to remove synced cache file %s: %w", file, err)
		}
	}

	return nil
}

// Count returns the number of cached data points
func (c *Cache) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.data)
}

// save persists the cache to disk
func (c *Cache) save() error {
	filename := c.filename()
	tempFile, err := os.CreateTemp(c.cacheDir, ".cache-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary cache file: %w", err)
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	encoder := json.NewEncoder(tempFile)
	if err := encoder.Encode(c.data); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to encode cache data: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync cache data: %w", err)
	}
	if err := tempFile.Chmod(0o644); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to set cache permissions: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close cache file: %w", err)
	}
	if err := os.Rename(tempName, filename); err != nil {
		return fmt.Errorf("failed to replace cache file: %w", err)
	}

	return nil
}

func (c *Cache) filename() string {
	return filepath.Join(c.cacheDir, fmt.Sprintf("cache_%s.json", time.Now().Format("2006-01-02")))
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
