package influx

import (
	"context"
	"fmt"
	"log"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/sony/gobreaker"
)

// ErrorHandler is a callback function for handling write errors
type ErrorHandler func(err error)

// Client handles writing data to InfluxDB
type Client struct {
	client         influxdb2.Client
	writeAPI       api.WriteAPI
	bucket         string
	org            string
	measurement    string
	errorHandler   ErrorHandler
	stopChan       chan struct{}
	circuitBreaker *gobreaker.CircuitBreaker
}

// DataPoint represents a single energy measurement
type DataPoint struct {
	Timestamp        time.Time
	ConsumptionDelta float64
	Demand           float64
	CostDelta        float64
	Consumption      float64
}

// NewClient creates a new InfluxDB client
func NewClient(url, token, org, bucket, measurement string) (*Client, error) {
	return NewClientWithErrorHandler(url, token, org, bucket, measurement, nil)
}

// NewClientWithErrorHandler creates a new InfluxDB client with a custom error handler
func NewClientWithErrorHandler(url, token, org, bucket, measurement string, errorHandler ErrorHandler) (*Client, error) {
	client := influxdb2.NewClient(url, token)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to InfluxDB: %w", err)
	}

	if health.Status != "pass" {
		return nil, fmt.Errorf("InfluxDB health check failed: %s", health.Status)
	}

	writeAPI := client.WriteAPI(org, bucket)

	// Default error handler logs errors
	if errorHandler == nil {
		errorHandler = func(err error) {
			log.Printf("InfluxDB write error: %v", err)
		}
	}

	// Configure circuit breaker
	cbSettings := gobreaker.Settings{
		Name:        "InfluxDB",
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.6
		},
	}

	c := &Client{
		client:         client,
		writeAPI:       writeAPI,
		bucket:         bucket,
		org:            org,
		measurement:    measurement,
		errorHandler:   errorHandler,
		stopChan:       make(chan struct{}),
		circuitBreaker: gobreaker.NewCircuitBreaker(cbSettings),
	}

	// Start error monitoring goroutine
	go c.monitorErrors()

	return c, nil
}

// monitorErrors continuously monitors the WriteAPI error channel
func (c *Client) monitorErrors() {
	errorsChan := c.writeAPI.Errors()
	for {
		select {
		case err := <-errorsChan:
			if err != nil && c.errorHandler != nil {
				c.errorHandler(err)
			}
		case <-c.stopChan:
			return
		}
	}
}

// WriteDataPoint writes a single data point to InfluxDB
func (c *Client) WriteDataPoint(dp DataPoint) error {
	p := influxdb2.NewPoint(
		c.measurement,
		map[string]string{
			"source": "octopus_home_mini",
		},
		map[string]interface{}{
			"consumption_delta": dp.ConsumptionDelta,
			"demand":            dp.Demand,
			"cost_delta":        dp.CostDelta,
			"consumption":       dp.Consumption,
		},
		dp.Timestamp,
	)

	c.writeAPI.WritePoint(p)
	return nil
}

// WriteDataPoints writes multiple data points to InfluxDB
func (c *Client) WriteDataPoints(dataPoints []DataPoint) error {
	for _, dp := range dataPoints {
		if err := c.WriteDataPoint(dp); err != nil {
			return err
		}
	}
	return nil
}

// Flush ensures all pending writes are sent to InfluxDB
func (c *Client) Flush() {
	c.writeAPI.Flush()
}

// GetErrors returns a channel for write errors
func (c *Client) GetErrors() <-chan error {
	return c.writeAPI.Errors()
}

// CheckConnection tests if the connection to InfluxDB is healthy
func (c *Client) CheckConnection(ctx context.Context) error {
	health, err := c.client.Health(ctx)
	if err != nil {
		return fmt.Errorf("connection check failed: %w", err)
	}

	if health.Status != "pass" {
		return fmt.Errorf("InfluxDB unhealthy: %s", health.Status)
	}

	return nil
}

// Close closes the InfluxDB client
func (c *Client) Close() {
	// Signal error monitoring goroutine to stop
	close(c.stopChan)

	// Flush any pending writes
	c.writeAPI.Flush()

	// Close the client connection
	c.client.Close()
}

// WritePointDirectly writes a point directly (synchronous, returns error immediately) with circuit breaker
func (c *Client) WritePointDirectly(ctx context.Context, dp DataPoint) error {
	_, err := c.circuitBreaker.Execute(func() (interface{}, error) {
		p := write.NewPoint(
			c.measurement,
			map[string]string{
				"source": "octopus_home_mini",
			},
			map[string]interface{}{
				"consumption_delta": dp.ConsumptionDelta,
				"demand":            dp.Demand,
				"cost_delta":        dp.CostDelta,
				"consumption":       dp.Consumption,
			},
			dp.Timestamp,
		)

		writeAPIBlocking := c.client.WriteAPIBlocking(c.org, c.bucket)
		return nil, writeAPIBlocking.WritePoint(ctx, p)
	})
	return err
}
