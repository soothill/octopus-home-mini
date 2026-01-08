package monitor

import (
	"errors"
	"testing"
	"time"

	"github.com/soothill/octopus-home-mini/pkg/config"
	"github.com/soothill/octopus-home-mini/pkg/octopus"
	"github.com/stretchr/testify/assert"
)

// MockClock is a mock implementation of the Clock interface for testing
type MockClock struct {
	now time.Time
}

func (c *MockClock) Now() time.Time {
	return c.now
}

func (c *MockClock) NewTicker(d time.Duration) *time.Ticker {
	// Return a ticker that never ticks for most tests
	return time.NewTicker(24 * time.Hour)
}

// MockOctopusClient is a mock implementation of the OctopusClient for testing
type MockOctopusClient struct {
	getTelemetryFunc func() ([]octopus.TelemetryData, error)
}

func (c *MockOctopusClient) GetTelemetry(ctx context.Context, start, end time.Time) ([]octopus.TelemetryData, error) {
	return c.getTelemetryFunc()
}

func (c *MockOctopusClient) Initialize(ctx context.Context) error {
	return nil
}

func TestPoll_FetchError(t *testing.T) {
	cfg := &config.Config{
		PollInterval:            1 * time.Minute,
		ConsecutiveErrorThreshold: 3,
		MaxBackoffFactor:        5,
	}
	mockClock := &MockClock{now: time.Now()}
	mockOctopusClient := &MockOctopusClient{
		getTelemetryFunc: func() ([]octopus.TelemetryData, error) {
			return nil, errors.New("fetch error")
		},
	}

	monitor := New(cfg, mockOctopusClient, nil, nil, nil)
	monitor.Clock = mockClock

	// First error
	monitor.poll()
	assert.Equal(t, 1, monitor.getConsecutiveErr())
	assert.False(t, monitor.getDegradedMode())

	// Second error
	monitor.poll()
	assert.Equal(t, 2, monitor.getConsecutiveErr())
	assert.False(t, monitor.getDegradedMode())

	// Third error - enter degraded mode
	monitor.poll()
	assert.Equal(t, 3, monitor.getConsecutiveErr())
	assert.True(t, monitor.getDegradedMode())
	assert.Equal(t, 2, monitor.getBackoffFactor())

	// Fourth error - increase backoff
	monitor.poll()
	assert.Equal(t, 4, monitor.getConsecutiveErr())
	assert.True(t, monitor.getDegradedMode())
	assert.Equal(t, 3, monitor.getBackoffFactor())
}

func TestPoll_FetchSuccess(t *testing.T) {
	cfg := &config.Config{
		PollInterval: 1 * time.Minute,
	}
	mockClock := &MockClock{now: time.Now()}
	mockOctopusClient := &MockOctopusClient{
		getTelemetryFunc: func() ([]octopus.TelemetryData, error) {
			return []octopus.TelemetryData{}, nil
		},
	}

	monitor := New(cfg, mockOctopusClient, nil, nil, nil)
	monitor.Clock = mockClock
	monitor.setDegradedMode(true)
	monitor.setBackoffFactor(5)
	monitor.incrementConsecutiveErr()

	monitor.poll()

	assert.Equal(t, 0, monitor.getConsecutiveErr())
	assert.False(t, monitor.getDegradedMode())
	assert.Equal(t, 1, monitor.getBackoffFactor())
}
