package octopus

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/machinebox/graphql"
	"github.com/sony/gobreaker"
)

const (
	graphqlEndpoint = "https://api.octopus.energy/v1/graphql/"
	maxRetries      = 3
	maxElapsedTime  = 30 * time.Second
)

// Client handles communication with the Octopus Energy GraphQL API
type Client struct {
	apiKey         string
	accountNumber  string
	token          string
	client         *graphql.Client
	meterGUID      string
	circuitBreaker *gobreaker.CircuitBreaker
}

// TelemetryData represents energy consumption data
type TelemetryData struct {
	ReadAt           time.Time `json:"readAt"`
	ConsumptionDelta float64   `json:"consumptionDelta"`
	Demand           float64   `json:"demand"`
	CostDelta        float64   `json:"costDelta"`
	Consumption      float64   `json:"consumption"`
}

// NewClient creates a new Octopus Energy API client
func NewClient(apiKey, accountNumber string) *Client {
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
		client:         graphql.NewClient(graphqlEndpoint),
		circuitBreaker: gobreaker.NewCircuitBreaker(cbSettings),
	}
}

// newBackoff creates a new exponential backoff configuration
func newBackoff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = maxElapsedTime
	return b
}

// Authenticate obtains a JWT token from the API with exponential backoff retry
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
			return fmt.Errorf("failed to authenticate: %w", err)
		}

		c.token = resp.ObtainKrakenToken.Token
		return nil
	}

	b := newBackoff()
	return backoff.Retry(operation, backoff.WithContext(b, ctx))
}

// GetMeterGUID retrieves the meter GUID for the account with exponential backoff retry
func (c *Client) GetMeterGUID(ctx context.Context) error {
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
		req.Header.Set("Authorization", c.token)

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
			return fmt.Errorf("failed to get meter GUID: %w", err)
		}

		if len(resp.Account.ElectricityAgreements) == 0 ||
			len(resp.Account.ElectricityAgreements[0].MeterPoint.Meters) == 0 ||
			len(resp.Account.ElectricityAgreements[0].MeterPoint.Meters[0].SmartDevices) == 0 {
			// Don't retry if no devices found - this is a permanent error
			return backoff.Permanent(fmt.Errorf("no smart devices found for account"))
		}

		c.meterGUID = resp.Account.ElectricityAgreements[0].MeterPoint.Meters[0].SmartDevices[0].DeviceID
		return nil
	}

	b := newBackoff()
	return backoff.Retry(operation, backoff.WithContext(b, ctx))
}

// GetTelemetry retrieves smart meter telemetry data with exponential backoff retry and circuit breaker
func (c *Client) GetTelemetry(ctx context.Context, start, end time.Time) ([]TelemetryData, error) {
	if c.token == "" {
		if err := c.Authenticate(ctx); err != nil {
			return nil, err
		}
	}

	if c.meterGUID == "" {
		if err := c.GetMeterGUID(ctx); err != nil {
			return nil, err
		}
	}

	// Wrap the operation in circuit breaker
	result, err := c.circuitBreaker.Execute(func() (interface{}, error) {
		return c.fetchTelemetryWithRetry(ctx, start, end)
	})

	if err != nil {
		return nil, err
	}

	data, ok := result.([]TelemetryData)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from circuit breaker")
	}
	return data, nil
}

// fetchTelemetryWithRetry performs the actual telemetry fetch with retry logic
func (c *Client) fetchTelemetryWithRetry(ctx context.Context, start, end time.Time) ([]TelemetryData, error) {
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

		req.Var("deviceId", c.meterGUID)
		req.Var("start", start.Format(time.RFC3339))
		req.Var("end", end.Format(time.RFC3339))
		req.Header.Set("Authorization", c.token)

		var resp struct {
			SmartMeterTelemetry []struct {
				ReadAt           string  `json:"readAt"`
				ConsumptionDelta float64 `json:"consumptionDelta"`
				Demand           float64 `json:"demand"`
				CostDelta        float64 `json:"costDelta"`
				Consumption      float64 `json:"consumption"`
			} `json:"smartMeterTelemetry"`
		}

		if err := c.client.Run(ctx, req, &resp); err != nil {
			return fmt.Errorf("failed to get telemetry: %w", err)
		}

		telemetry = make([]TelemetryData, 0, len(resp.SmartMeterTelemetry))
		for _, data := range resp.SmartMeterTelemetry {
			readAt, err := time.Parse(time.RFC3339, data.ReadAt)
			if err != nil {
				continue // Skip invalid timestamps
			}

			telemetry = append(telemetry, TelemetryData{
				ReadAt:           readAt,
				ConsumptionDelta: data.ConsumptionDelta,
				Demand:           data.Demand,
				CostDelta:        data.CostDelta,
				Consumption:      data.Consumption,
			})
		}

		return nil
	}

	b := newBackoff()
	if err := backoff.Retry(operation, backoff.WithContext(b, ctx)); err != nil {
		return nil, err
	}

	return telemetry, nil
}

// Initialize performs authentication and retrieves the meter GUID
func (c *Client) Initialize(ctx context.Context) error {
	if err := c.Authenticate(ctx); err != nil {
		return err
	}
	return c.GetMeterGUID(ctx)
}
