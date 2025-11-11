package influx

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

// Client handles writing data to InfluxDB
type Client struct {
	client   influxdb2.Client
	writeAPI api.WriteAPI
	bucket   string
	org      string
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
func NewClient(url, token, org, bucket string) (*Client, error) {
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

	return &Client{
		client:   client,
		writeAPI: writeAPI,
		bucket:   bucket,
		org:      org,
	}, nil
}

// WriteDataPoint writes a single data point to InfluxDB
func (c *Client) WriteDataPoint(dp DataPoint) error {
	p := influxdb2.NewPoint(
		"energy_consumption",
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
	c.writeAPI.Flush()
	c.client.Close()
}

// WritePointDirectly writes a point directly (synchronous, returns error immediately)
func (c *Client) WritePointDirectly(ctx context.Context, dp DataPoint) error {
	p := write.NewPoint(
		"energy_consumption",
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
	return writeAPIBlocking.WritePoint(ctx, p)
}
