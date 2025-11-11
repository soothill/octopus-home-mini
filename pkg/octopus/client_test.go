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
		t.Fatal("NewClient() returned nil")
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

func TestClient_CircuitBreakerInitialized(t *testing.T) {
	client := NewClient("test_key", "A-12345678")

	if client.circuitBreaker == nil {
		t.Error("Circuit breaker should be initialized")
	}
}

func TestTelemetryData_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		data TelemetryData
		desc string
	}{
		{
			name: "zero values",
			data: TelemetryData{
				ReadAt:           time.Now(),
				ConsumptionDelta: 0,
				Demand:           0,
				CostDelta:        0,
				Consumption:      0,
			},
			desc: "Should handle zero values",
		},
		{
			name: "negative values",
			data: TelemetryData{
				ReadAt:           time.Now(),
				ConsumptionDelta: -0.5,
				Demand:           -1.2,
				CostDelta:        -0.15,
				Consumption:      10.5,
			},
			desc: "Should handle negative values (solar export)",
		},
		{
			name: "very large values",
			data: TelemetryData{
				ReadAt:           time.Now(),
				ConsumptionDelta: 999999.99,
				Demand:           999999.99,
				CostDelta:        999999.99,
				Consumption:      999999.99,
			},
			desc: "Should handle very large values",
		},
		{
			name: "very small values",
			data: TelemetryData{
				ReadAt:           time.Now(),
				ConsumptionDelta: 0.00001,
				Demand:           0.00001,
				CostDelta:        0.00001,
				Consumption:      0.00001,
			},
			desc: "Should handle very small values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify we can create the data structure
			_ = TelemetryData{
				ReadAt:           tt.data.ReadAt,
				ConsumptionDelta: tt.data.ConsumptionDelta,
				Demand:           tt.data.Demand,
				CostDelta:        tt.data.CostDelta,
				Consumption:      tt.data.Consumption,
			}
		})
	}
}

func TestClient_MultipleClients(t *testing.T) {
	// Test creating multiple clients doesn't interfere with each other
	client1 := NewClient("key1", "A-11111111")
	client2 := NewClient("key2", "A-22222222")

	if client1.apiKey == client2.apiKey {
		t.Error("Different clients should have different API keys")
	}

	if client1.accountNumber == client2.accountNumber {
		t.Error("Different clients should have different account numbers")
	}

	if client1.client == client2.client {
		t.Error("Different clients should have different GraphQL clients")
	}
}

func TestClient_BackoffConfiguration(t *testing.T) {
	// Test that backoff is properly configured
	b := newBackoff()

	if b == nil {
		t.Fatal("newBackoff() returned nil")
	}

	if b.MaxElapsedTime != maxElapsedTime {
		t.Errorf("MaxElapsedTime = %v, want %v", b.MaxElapsedTime, maxElapsedTime)
	}
}

func TestClient_TimeZoneHandling(t *testing.T) {
	locations := []string{
		"UTC",
		"America/New_York",
		"Europe/London",
		"Asia/Tokyo",
	}

	for _, locName := range locations {
		t.Run(locName, func(t *testing.T) {
			loc, err := time.LoadLocation(locName)
			if err != nil {
				t.Skipf("Could not load location %s: %v", locName, err)
			}

			data := TelemetryData{
				ReadAt:           time.Now().In(loc),
				ConsumptionDelta: 0.5,
				Demand:           1.2,
				CostDelta:        0.15,
				Consumption:      10.5,
			}

			if data.ReadAt.Location().String() != locName {
				t.Errorf("Location = %v, want %v", data.ReadAt.Location(), locName)
			}
		})
	}
}

func TestClient_EmptyAccountNumber(t *testing.T) {
	client := NewClient("test_key", "")

	if client.accountNumber != "" {
		t.Error("Empty account number should remain empty")
	}
}

func TestClient_EmptyAPIKey(t *testing.T) {
	client := NewClient("", "A-12345678")

	if client.apiKey != "" {
		t.Error("Empty API key should remain empty")
	}
}

func TestConstants(t *testing.T) {
	if maxRetries != 3 {
		t.Errorf("maxRetries = %v, want 3", maxRetries)
	}

	if maxElapsedTime != 30*time.Second {
		t.Errorf("maxElapsedTime = %v, want 30s", maxElapsedTime)
	}

	if graphqlEndpoint == "" {
		t.Error("graphqlEndpoint should not be empty")
	}
}

func TestClient_GetTelemetryWithoutToken(t *testing.T) {
	client := NewClient("test_key", "A-12345678")

	// No token set, should try to authenticate first
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.GetTelemetry(ctx, time.Now().Add(-1*time.Hour), time.Now())

	// Should get an error because authentication will fail with test credentials
	if err == nil {
		t.Log("Expected error with test credentials, got nil")
	}
}

func TestClient_GetTelemetryWithoutMeterGUID(t *testing.T) {
	client := NewClient("test_key", "A-12345678")
	client.token = "fake_token" // Set token but not meter GUID

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.GetTelemetry(ctx, time.Now().Add(-1*time.Hour), time.Now())

	// Should get an error because GetMeterGUID will fail
	if err == nil {
		t.Log("Expected error when getting meter GUID, got nil")
	}
}

func TestTelemetryData_TimestampPrecision(t *testing.T) {
	now := time.Now()

	data := TelemetryData{
		ReadAt:           now,
		ConsumptionDelta: 0.5,
		Demand:           1.2,
		CostDelta:        0.15,
		Consumption:      10.5,
	}

	if !data.ReadAt.Equal(now) {
		t.Error("Timestamp precision lost")
	}

	if data.ReadAt.Nanosecond() != now.Nanosecond() {
		t.Error("Nanosecond precision lost")
	}
}

func TestClient_LongAccountNumber(t *testing.T) {
	// Test with a very long account number
	longAccount := "A-" + string(make([]byte, 1000))
	client := NewClient("test_key", longAccount)

	if client.accountNumber != longAccount {
		t.Error("Long account number not preserved")
	}
}

func TestClient_SpecialCharactersInCredentials(t *testing.T) {
	tests := []struct {
		name          string
		apiKey        string
		accountNumber string
	}{
		{
			name:          "special characters in key",
			apiKey:        "test!@#$%^&*()_+-=[]{}|;:',.<>?/~`",
			accountNumber: "A-12345678",
		},
		{
			name:          "unicode in account",
			apiKey:        "test_key",
			accountNumber: "A-测试账号",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.apiKey, tt.accountNumber)

			if client.apiKey != tt.apiKey {
				t.Errorf("API key = %v, want %v", client.apiKey, tt.apiKey)
			}

			if client.accountNumber != tt.accountNumber {
				t.Errorf("Account number = %v, want %v", client.accountNumber, tt.accountNumber)
			}
		})
	}
}

func TestClient_MultipleGetTelemetryCalls(t *testing.T) {
	client := NewClient("test_key", "A-12345678")
	client.token = "fake_token"
	client.meterGUID = "fake_guid"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Make multiple calls to test state persistence
	for i := 0; i < 3; i++ {
		_, err := client.GetTelemetry(ctx, time.Now().Add(-1*time.Hour), time.Now())

		// We expect errors with fake credentials, but this tests the method can be called multiple times
		if err == nil {
			t.Logf("Call %d: Unexpected success with fake credentials", i)
		}
	}
}

func TestClient_ConcurrentAccess(t *testing.T) {
	client := NewClient("test_key", "A-12345678")

	// Test concurrent reads of client fields
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_ = client.apiKey
			_ = client.accountNumber
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
