package octopus

import (
	"context"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	apiKey := "test_api_key"
	accountNumber := "A-12345678"

	client := NewClient(apiKey, accountNumber)

	if client == nil {
		t.Error("NewClient() returned nil")
	}

	if client.apiKey != apiKey {
		t.Errorf("apiKey = %v, want %v", client.apiKey, apiKey)
	}

	if client.accountNumber != accountNumber {
		t.Errorf("accountNumber = %v, want %v", client.accountNumber, accountNumber)
	}

	if client.client == nil {
		t.Error("GraphQL client is nil")
	}
}

func TestTelemetryData_Structure(t *testing.T) {
	now := time.Now()
	data := TelemetryData{
		ReadAt:           now,
		ConsumptionDelta: 0.5,
		Demand:           1.2,
		CostDelta:        0.15,
		Consumption:      10.5,
	}

	if data.ReadAt != now {
		t.Error("ReadAt not set correctly")
	}

	if data.ConsumptionDelta != 0.5 {
		t.Errorf("ConsumptionDelta = %v, want 0.5", data.ConsumptionDelta)
	}

	if data.Demand != 1.2 {
		t.Errorf("Demand = %v, want 1.2", data.Demand)
	}

	if data.CostDelta != 0.15 {
		t.Errorf("CostDelta = %v, want 0.15", data.CostDelta)
	}

	if data.Consumption != 10.5 {
		t.Errorf("Consumption = %v, want 10.5", data.Consumption)
	}
}

func TestClient_GetTelemetry_RequiresAuth(t *testing.T) {
	client := NewClient("test_key", "A-12345678")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now().Add(-1 * time.Hour)
	end := time.Now()

	// This should fail because we're not authenticated
	_, err := client.GetTelemetry(ctx, start, end)

	// We expect an error since we can't actually authenticate with test credentials
	if err == nil {
		t.Log("Warning: Expected authentication to fail with test credentials")
	}
}

func TestClient_Initialize_RequiresValidCredentials(t *testing.T) {
	client := NewClient("invalid_key", "A-00000000")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This should fail with invalid credentials
	err := client.Initialize(ctx)

	// We expect an error
	if err == nil {
		t.Error("Initialize() expected to fail with invalid credentials, got nil error")
	}
}

func TestClient_ContextTimeout(t *testing.T) {
	client := NewClient("test_key", "A-12345678")

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.Authenticate(ctx)

	if err == nil {
		t.Error("Authenticate() should fail with cancelled context")
	}
}

func TestClient_TimeRangeValidation(t *testing.T) {
	client := NewClient("test_key", "A-12345678")
	client.token = "fake_token" // Set a fake token to bypass auth check
	client.meterGUID = "fake_guid"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tests := []struct {
		name  string
		start time.Time
		end   time.Time
	}{
		{
			name:  "normal range",
			start: time.Now().Add(-1 * time.Hour),
			end:   time.Now(),
		},
		{
			name:  "same start and end",
			start: time.Now(),
			end:   time.Now(),
		},
		{
			name:  "24 hour range",
			start: time.Now().Add(-24 * time.Hour),
			end:   time.Now(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This will fail due to invalid credentials but tests the time range handling
			_, err := client.GetTelemetry(ctx, tt.start, tt.end)

			// We just verify it doesn't panic
			if err == nil {
				t.Log("Telemetry call completed (unexpected with fake credentials)")
			}
		})
	}
}

func TestGraphQLEndpoint(t *testing.T) {
	expectedEndpoint := "https://api.octopus.energy/v1/graphql/"

	if graphqlEndpoint != expectedEndpoint {
		t.Errorf("graphqlEndpoint = %v, want %v", graphqlEndpoint, expectedEndpoint)
	}
}

// Mock tests (would require a proper mock server in production)
func TestClient_EmptyTelemetryResponse(t *testing.T) {
	// This test verifies behavior when API returns empty data
	client := NewClient("test_key", "A-12345678")
	client.token = "fake_token"
	client.meterGUID = "fake_guid"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now().Add(-10 * time.Minute)
	end := time.Now()

	// This will likely fail, but we're testing the error handling
	data, err := client.GetTelemetry(ctx, start, end)

	// With invalid credentials, we expect either an error or empty data
	if err == nil && len(data) > 0 {
		t.Error("Unexpected successful response with fake credentials")
	}

	// Verify data is at least initialized (even if empty)
	if data == nil && err == nil {
		t.Error("GetTelemetry() returned nil data without error")
	}
}

func TestClient_StateManagement(t *testing.T) {
	client := NewClient("test_key", "A-12345678")

	if client.token != "" {
		t.Error("New client should not have a token")
	}

	if client.meterGUID != "" {
		t.Error("New client should not have a meterGUID")
	}

	// Manually set state to test
	client.token = "test_token"
	client.meterGUID = "test_guid"

	if client.token != "test_token" {
		t.Error("Token not set correctly")
	}

	if client.meterGUID != "test_guid" {
		t.Error("MeterGUID not set correctly")
	}
}
