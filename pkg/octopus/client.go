package octopus

import (
	"context"
	"fmt"
	"time"

	"github.com/machinebox/graphql"
)

const (
	graphqlEndpoint = "https://api.octopus.energy/v1/graphql/"
)

// Client handles communication with the Octopus Energy GraphQL API
type Client struct {
	apiKey        string
	accountNumber string
	token         string
	client        *graphql.Client
	meterGUID     string
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
	return &Client{
		apiKey:        apiKey,
		accountNumber: accountNumber,
		client:        graphql.NewClient(graphqlEndpoint),
	}
}

// Authenticate obtains a JWT token from the API
func (c *Client) Authenticate(ctx context.Context) error {
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

// GetMeterGUID retrieves the meter GUID for the account
func (c *Client) GetMeterGUID(ctx context.Context) error {
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
		return fmt.Errorf("no smart devices found for account")
	}

	c.meterGUID = resp.Account.ElectricityAgreements[0].MeterPoint.Meters[0].SmartDevices[0].DeviceID
	return nil
}

// GetTelemetry retrieves smart meter telemetry data
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
		return nil, fmt.Errorf("failed to get telemetry: %w", err)
	}

	telemetry := make([]TelemetryData, 0, len(resp.SmartMeterTelemetry))
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

	return telemetry, nil
}

// Initialize performs authentication and retrieves the meter GUID
func (c *Client) Initialize(ctx context.Context) error {
	if err := c.Authenticate(ctx); err != nil {
		return err
	}
	return c.GetMeterGUID(ctx)
}
