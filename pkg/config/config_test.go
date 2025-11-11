package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid configuration",
			envVars: map[string]string{
				"OCTOPUS_API_KEY":        "test_api_key",
				"OCTOPUS_ACCOUNT_NUMBER": "A-12345678",
				"INFLUXDB_URL":           "http://localhost:8086",
				"INFLUXDB_TOKEN":         "test_token",
				"INFLUXDB_ORG":           "test_org",
				"INFLUXDB_BUCKET":        "test_bucket",
				"SLACK_WEBHOOK_URL":      "https://hooks.slack.com/test",
				"POLL_INTERVAL_SECONDS":  "30",
				"CACHE_DIR":              "./test_cache",
				"LOG_LEVEL":              "info",
			},
			wantErr: false,
		},
		{
			name: "missing octopus api key",
			envVars: map[string]string{
				"OCTOPUS_ACCOUNT_NUMBER": "A-12345678",
				"INFLUXDB_URL":           "http://localhost:8086",
				"INFLUXDB_TOKEN":         "test_token",
				"INFLUXDB_ORG":           "test_org",
				"SLACK_WEBHOOK_URL":      "https://hooks.slack.com/test",
			},
			wantErr:     true,
			errContains: "OCTOPUS_API_KEY",
		},
		{
			name: "missing account number",
			envVars: map[string]string{
				"OCTOPUS_API_KEY":   "test_api_key",
				"INFLUXDB_URL":      "http://localhost:8086",
				"INFLUXDB_TOKEN":    "test_token",
				"INFLUXDB_ORG":      "test_org",
				"SLACK_WEBHOOK_URL": "https://hooks.slack.com/test",
			},
			wantErr:     true,
			errContains: "OCTOPUS_ACCOUNT_NUMBER",
		},
		{
			name: "missing influxdb token",
			envVars: map[string]string{
				"OCTOPUS_API_KEY":        "test_api_key",
				"OCTOPUS_ACCOUNT_NUMBER": "A-12345678",
				"INFLUXDB_URL":           "http://localhost:8086",
				"INFLUXDB_ORG":           "test_org",
				"SLACK_WEBHOOK_URL":      "https://hooks.slack.com/test",
			},
			wantErr:     true,
			errContains: "INFLUXDB_TOKEN",
		},
		{
			name: "slack disabled",
			envVars: map[string]string{
				"OCTOPUS_API_KEY":        "test_api_key",
				"OCTOPUS_ACCOUNT_NUMBER": "A-12345678",
				"INFLUXDB_URL":           "http://localhost:8086",
				"INFLUXDB_TOKEN":         "test_token",
				"INFLUXDB_ORG":           "test_org",
				"SLACK_ENABLED":          "false",
			},
			wantErr: false,
		},
		{
			name: "custom poll interval",
			envVars: map[string]string{
				"OCTOPUS_API_KEY":        "test_api_key",
				"OCTOPUS_ACCOUNT_NUMBER": "A-12345678",
				"INFLUXDB_URL":           "http://localhost:8086",
				"INFLUXDB_TOKEN":         "test_token",
				"INFLUXDB_ORG":           "test_org",
				"SLACK_WEBHOOK_URL":      "https://hooks.slack.com/test",
				"POLL_INTERVAL_SECONDS":  "60",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			cfg, err := Load()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Load() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Load() unexpected error = %v", err)
				return
			}

			// Verify configuration values
			if cfg.OctopusAPIKey != tt.envVars["OCTOPUS_API_KEY"] {
				t.Errorf("OctopusAPIKey = %v, want %v", cfg.OctopusAPIKey, tt.envVars["OCTOPUS_API_KEY"])
			}

			if cfg.OctopusAccountNumber != tt.envVars["OCTOPUS_ACCOUNT_NUMBER"] {
				t.Errorf("OctopusAccountNumber = %v, want %v", cfg.OctopusAccountNumber, tt.envVars["OCTOPUS_ACCOUNT_NUMBER"])
			}

			// Check poll interval
			if tt.envVars["POLL_INTERVAL_SECONDS"] != "" {
				expectedInterval, _ := time.ParseDuration(tt.envVars["POLL_INTERVAL_SECONDS"] + "s")
				if cfg.PollInterval != expectedInterval {
					t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, expectedInterval)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: &Config{
				OctopusAPIKey:        "test_key",
				OctopusAccountNumber: "A-12345678",
				InfluxDBURL:          "http://localhost:8086",
				InfluxDBToken:        "test_token",
				InfluxDBOrg:          "test_org",
				SlackWebhookURL:      "https://hooks.slack.com/test",
			},
			wantErr: false,
		},
		{
			name: "missing api key",
			cfg: &Config{
				OctopusAccountNumber: "A-12345678",
				InfluxDBURL:          "http://localhost:8086",
				InfluxDBToken:        "test_token",
				InfluxDBOrg:          "test_org",
				SlackWebhookURL:      "https://hooks.slack.com/test",
			},
			wantErr: true,
			errMsg:  "OCTOPUS_API_KEY",
		},
		{
			name: "empty influxdb url",
			cfg: &Config{
				OctopusAPIKey:        "test_key",
				OctopusAccountNumber: "A-12345678",
				InfluxDBToken:        "test_token",
				InfluxDBOrg:          "test_org",
				SlackWebhookURL:      "https://hooks.slack.com/test",
			},
			wantErr: true,
			errMsg:  "INFLUXDB_URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				} else if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestGetEnvAsInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		value        string
		defaultValue int
		want         int
	}{
		{
			name:         "valid integer",
			key:          "TEST_INT",
			value:        "42",
			defaultValue: 10,
			want:         42,
		},
		{
			name:         "invalid integer uses default",
			key:          "TEST_INT",
			value:        "not_a_number",
			defaultValue: 10,
			want:         10,
		},
		{
			name:         "empty value uses default",
			key:          "TEST_INT",
			value:        "",
			defaultValue: 10,
			want:         10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.value != "" {
				os.Setenv(tt.key, tt.value)
			}

			got := getEnvAsInt(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvAsInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
