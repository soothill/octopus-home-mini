package octopus

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/machinebox/graphql"
	"github.com/sony/gobreaker"
)

const (
	graphqlEndpoint       = "https://api.octopus.energy/v1/graphql/"
	maxRetries            = 3
	maxElapsedTime        = 30 * time.Second
	tokenValidityDuration = 1 * time.Hour // Tokens are valid for 1 hour
)

// Client handles communication with Octopus Energy GraphQL API
type Client struct {
	client         *graphql.Client
	circuitBreaker *gobreaker.CircuitBreaker
	apiKey         string
	accountNumber  string
	token          string
	meterGUID      string
	tokenExpiry    time.Time    // Track when token will expire
	mu             sync.RWMutex // Protect token and expiry fields
}

// TelemetryData represents energy consumption data
type TelemetryData struct {
	ReadAt           time.Time `json:"readAt"`
	ConsumptionDelta float64   `json:"consumptionDelta"`
	Demand           float64   `json:"demand"`
	CostDelta        float64   `json:"costDelta"`
	Consumption      float64   `json:"consumption"`
}

// RateLimitError indicates the Octopus API asked the client to slow down.
type RateLimitError struct {
	Err error
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("octopus API rate limited request: %v", e.Err)
}

func (e *RateLimitError) Unwrap() error {
	return e.Err
}

// IsRateLimitError reports whether err is an Octopus rate-limit/backoff response.
func IsRateLimitError(err error) bool {
	var rateLimitErr *RateLimitError
	return errors.As(err, &rateLimitErr)
}

// NewClient creates a new Octopus Energy API client
func NewClient(apiKey, accountNumber string) *Client {
	return NewClientWithEndpoint(apiKey, accountNumber, graphqlEndpoint)
}

// NewClientWithEndpoint creates a new Octopus Energy API client with a specific endpoint
func NewClientWithEndpoint(apiKey, accountNumber, endpoint string) *Client {
	// Configure circuit breaker
	cbSettings := gobreaker.Settings{
		Name:        "OctopusAPI",
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Log state changes (could be enhanced with structured logging)
			// fmt.Printf("Circuit breaker %s changed from %s to %s\n", name, from, to)
		},
	}

	return &Client{
		apiKey:         apiKey,
		accountNumber:  accountNumber,
		client:         graphql.NewClient(endpoint),
		circuitBreaker: gobreaker.NewCircuitBreaker(cbSettings),
	}
}

// newBackoff creates a new exponential backoff configuration
func newBackoff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = maxElapsedTime
	return b
}

func isBackoffResponse(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "back off") ||
		strings.Contains(msg, "backoff") ||
		strings.Contains(msg, "too aggressive") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "rate-limit") ||
		strings.Contains(msg, "status code: 429") ||
		strings.Contains(msg, "status 429")
}

func permanentRateLimitError(err error) error {
	return backoff.Permanent(&RateLimitError{Err: err})
}

// Authenticate obtains a JWT token from API with exponential backoff retry
func (c *Client) Authenticate(ctx context.Context) error {
	operation := func() error {
		req := graphql.NewRequest(`
			mutation obtainKrakenToken($apiKey: String!) {
				obtainKrakenToken(input: {APIKey: $apiKey}) {
					token
				}
			}
		`)

		req.Var("apiKey", c.apiKey)

		var resp struct {
			ObtainKrakenToken struct {
				Token string `json:"token"`
			} `json:"obtainKrakenToken"`
		}

		if err := c.client.Run(ctx, req, &resp); err != nil {
			if isBackoffResponse(err) {
				return permanentRateLimitError(fmt.Errorf("failed to authenticate: %w", err))
			}
			return fmt.Errorf("failed to authenticate: %w", err)
		}

		c.mu.Lock()
		c.token = resp.ObtainKrakenToken.Token
		// Set token expiry - tokens typically last 1 hour
		c.tokenExpiry = time.Now().Add(tokenValidityDuration)
		c.mu.Unlock()

		return nil
	}

	b := newBackoff()
	return backoff.Retry(operation, backoff.WithContext(b, ctx))
}

// ensureValidToken checks if current token is valid and re-authenticates if needed
func (c *Client) ensureValidToken(ctx context.Context) error {
	c.mu.Lock()
	needsRefresh := c.token == "" || time.Now().After(c.tokenExpiry)
	c.mu.Unlock()

	if needsRefresh {
		return c.Authenticate(ctx)
	}
	return nil
}

// GetMeterGUID retrieves meter GUID for account with exponential backoff retry
func (c *Client) GetMeterGUID(ctx context.Context) error {
	// Ensure token is valid before making request
	if err := c.ensureValidToken(ctx); err != nil {
		return err
	}

	operation := func() error {
		req := graphql.NewRequest(`
			query getAccount($accountNumber: String!) {
				account(accountNumber: $accountNumber) {
					electricityAgreements {
						meterPoint {
							meters {
								smartDevices {
									deviceId
								}
							}
						}
					}
				}
			}
		`)

		req.Var("accountNumber", c.accountNumber)

		c.mu.RLock()
		token := c.token
		c.mu.RUnlock()

		req.Header.Set("Authorization", token)

		var resp struct {
			Account struct {
				ElectricityAgreements []struct {
					MeterPoint struct {
						Meters []struct {
							SmartDevices []struct {
								DeviceID string `json:"deviceId"`
							} `json:"smartDevices"`
						} `json:"meters"`
					} `json:"meterPoint"`
				} `json:"electricityAgreements"`
			} `json:"account"`
		}

		if err := c.client.Run(ctx, req, &resp); err != nil {
			if isBackoffResponse(err) {
				return permanentRateLimitError(fmt.Errorf("failed to get meter GUID: %w", err))
			}
			return fmt.Errorf("failed to get meter GUID: %w", err)
		}

		if len(resp.Account.ElectricityAgreements) == 0 ||
			len(resp.Account.ElectricityAgreements[0].MeterPoint.Meters) == 0 ||
			len(resp.Account.ElectricityAgreements[0].MeterPoint.Meters[0].SmartDevices) == 0 {
			// Don't retry if no devices found - this is a permanent error
			return backoff.Permanent(fmt.Errorf("no smart devices found for account"))
		}

		c.mu.Lock()
		c.meterGUID = resp.Account.ElectricityAgreements[0].MeterPoint.Meters[0].SmartDevices[0].DeviceID
		c.mu.Unlock()

		return nil
	}

	b := newBackoff()
	return backoff.Retry(operation, backoff.WithContext(b, ctx))
}

// GetTelemetry retrieves smart meter telemetry data with exponential backoff retry and circuit breaker
func (c *Client) GetTelemetry(ctx context.Context, start, end time.Time) ([]TelemetryData, error) {
	// Ensure token is valid and meter GUID is set
	if err := c.ensureValidToken(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure valid token: %w", err)
	}

	c.mu.RLock()
	guid := c.meterGUID
	c.mu.RUnlock()

	if guid == "" {
		if err := c.GetMeterGUID(ctx); err != nil {
			return nil, fmt.Errorf("failed to get meter GUID: %w", err)
		}
		c.mu.RLock()
		guid = c.meterGUID
		c.mu.RUnlock()
	}

	// Wrap operation in circuit breaker
	result, err := c.circuitBreaker.Execute(func() (interface{}, error) {
		return c.fetchTelemetryWithRetry(ctx, guid, start, end)
	})

	if err != nil {
		return nil, fmt.Errorf("circuit breaker error: %w", err)
	}

	data, ok := result.([]TelemetryData)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from circuit breaker")
	}
	return data, nil
}

// fetchTelemetryWithRetry performs actual telemetry fetch with retry logic
func (c *Client) fetchTelemetryWithRetry(ctx context.Context, guid string, start, end time.Time) ([]TelemetryData, error) {
	var telemetry []TelemetryData

	operation := func() error {
		req := graphql.NewRequest(`
			query getTelemetry($deviceId: String!, $start: DateTime!, $end: DateTime!) {
				smartMeterTelemetry(
					deviceId: $deviceId
					start: $start
					end: $end
					grouping: TEN_SECONDS
				) {
					readAt
					consumptionDelta
					demand
					costDelta
					consumption
				}
			}
		`)

		req.Var("deviceId", guid)
		req.Var("start", start.Format(time.RFC3339))
		req.Var("end", end.Format(time.RFC3339))

		c.mu.RLock()
		token := c.token
		c.mu.RUnlock()

		req.Header.Set("Authorization", token)

		var resp struct {
			SmartMeterTelemetry []struct {
				ReadAt           string `json:"readAt"`
				ConsumptionDelta string `json:"consumptionDelta"`
				Demand           string `json:"demand"`
				CostDelta        string `json:"costDelta"`
				Consumption      string `json:"consumption"`
			} `json:"smartMeterTelemetry"`
		}

		if err := c.client.Run(ctx, req, &resp); err != nil {
			wrappedErr := fmt.Errorf("failed to get telemetry: %w", err)
			fmt.Printf("ERROR: Octopus API request failed: %v\n", err)
			if isBackoffResponse(err) {
				return permanentRateLimitError(wrappedErr)
			}
			return wrappedErr
		}

		// Log response details
		fmt.Printf("INFO: Octopus API response: raw_data_points=%d\n",
			len(resp.SmartMeterTelemetry))

		telemetry = make([]TelemetryData, 0, len(resp.SmartMeterTelemetry))
		skippedCount := 0

		for i, data := range resp.SmartMeterTelemetry {
			readAt, err := time.Parse(time.RFC3339, data.ReadAt)
			if err != nil {
				skippedCount++
				fmt.Printf("ERROR: Failed to parse timestamp for data point %d: %v (raw='%s')\n",
					i, err, data.ReadAt)
				continue // Skip invalid timestamps
			}

			// Parse string values to float64
			// Handle empty strings by treating them as 0 (no data = no value)
			consumptionDelta, err := strconv.ParseFloat(data.ConsumptionDelta, 64)
			if err != nil {
				skippedCount++
				fmt.Printf("ERROR: Failed to parse consumptionDelta for data point %d: %v (raw='%s')\n",
					i, err, data.ConsumptionDelta)
				continue // Skip invalid data
			}

			demand, err := strconv.ParseFloat(data.Demand, 64)
			if err != nil {
				skippedCount++
				fmt.Printf("ERROR: Failed to parse demand for data point %d: %v (raw='%s')\n",
					i, err, data.Demand)
				continue // Skip invalid data
			}

			// costDelta may be empty (not available from API), treat as 0
			costDelta := 0.0
			if data.CostDelta != "" {
				if parsed, err := strconv.ParseFloat(data.CostDelta, 64); err == nil {
					costDelta = parsed
				}
				// Silently ignore parse errors for costDelta as it may not be available
			}

			consumption, err := strconv.ParseFloat(data.Consumption, 64)
			if err != nil {
				skippedCount++
				fmt.Printf("ERROR: Failed to parse consumption for data point %d: %v (raw='%s')\n",
					i, err, data.Consumption)
				continue // Skip invalid data
			}

			telemetry = append(telemetry, TelemetryData{
				ReadAt:           readAt,
				ConsumptionDelta: consumptionDelta,
				Demand:           demand,
				CostDelta:        costDelta,
				Consumption:      consumption,
			})
		}

		fmt.Printf("INFO: Successfully parsed %d data points, skipped %d\n",
			len(telemetry), skippedCount)

		if len(telemetry) == 0 {
			if len(resp.SmartMeterTelemetry) == 0 {
				fmt.Printf("INFO: No telemetry data available for device %s, time range %s to %s\n",
					guid, start.Format(time.RFC3339), end.Format(time.RFC3339))
			} else {
				fmt.Printf("ERROR: No valid data points after parsing for device %s, time range %s to %s (skipped %d of %d raw points)\n",
					guid, start.Format(time.RFC3339), end.Format(time.RFC3339), skippedCount, len(resp.SmartMeterTelemetry))
			}
		}

		return nil
	}

	b := newBackoff()
	if err := backoff.Retry(operation, backoff.WithContext(b, ctx)); err != nil {
		return nil, err
	}

	return telemetry, nil
}

// Initialize performs authentication and retrieves meter GUID
func (c *Client) Initialize(ctx context.Context) error {
	if err := c.Authenticate(ctx); err != nil {
		return err
	}
	return c.GetMeterGUID(ctx)
}
