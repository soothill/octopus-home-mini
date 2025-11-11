package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
				"OCTOPUS_API_KEY":        "test_api_key_12345678901234567890",
				"OCTOPUS_ACCOUNT_NUMBER": "A-12345678",
				"INFLUXDB_URL":           "http://localhost:8086",
				"INFLUXDB_TOKEN":         "test_token",
				"INFLUXDB_ORG":           "test_org",
				"INFLUXDB_BUCKET":        "test_bucket",
				"SLACK_WEBHOOK_URL":      "https://example.com/test-webhook",
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
				"SLACK_WEBHOOK_URL":      "https://example.com/test-webhook",
			},
			wantErr:     true,
			errContains: "OCTOPUS_API_KEY",
		},
		{
			name: "missing account number",
			envVars: map[string]string{
				"OCTOPUS_API_KEY":   "test_api_key_12345678901234567890",
				"INFLUXDB_URL":      "http://localhost:8086",
				"INFLUXDB_TOKEN":    "test_token",
				"INFLUXDB_ORG":      "test_org",
				"SLACK_WEBHOOK_URL": "https://example.com/test-webhook",
			},
			wantErr:     true,
			errContains: "OCTOPUS_ACCOUNT_NUMBER",
		},
		{
			name: "missing influxdb token",
			envVars: map[string]string{
				"OCTOPUS_API_KEY":        "test_api_key_12345678901234567890",
				"OCTOPUS_ACCOUNT_NUMBER": "A-12345678",
				"INFLUXDB_URL":           "http://localhost:8086",
				"INFLUXDB_ORG":           "test_org",
				"SLACK_WEBHOOK_URL":      "https://example.com/test-webhook",
			},
			wantErr:     true,
			errContains: "INFLUXDB_TOKEN",
		},
		{
			name: "slack disabled",
			envVars: map[string]string{
				"OCTOPUS_API_KEY":        "test_api_key_12345678901234567890",
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
				"OCTOPUS_API_KEY":        "test_api_key_12345678901234567890",
				"OCTOPUS_ACCOUNT_NUMBER": "A-12345678",
				"INFLUXDB_URL":           "http://localhost:8086",
				"INFLUXDB_TOKEN":         "test_token",
				"INFLUXDB_ORG":           "test_org",
				"SLACK_WEBHOOK_URL":      "https://example.com/test-webhook",
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
				OctopusAPIKey:             "test_key_123456789012345678901234",
				OctopusAccountNumber:      "A-12345678",
				InfluxDBURL:               "http://localhost:8086",
				InfluxDBToken:             "test_token",
				InfluxDBOrg:               "test_org",
				InfluxDBBucket:            "test_bucket",
				InfluxDBMeasurement:       "energy_consumption",
				SlackWebhookURL:           "https://example.com/test-webhook",
				SlackEnabled:              false,
				PollInterval:              30 * time.Second,
				CacheDir:                  "./cache",
				LogLevel:                  "info",
				InfluxConnectTimeout:      30 * time.Second,
				InfluxWriteTimeout:        10 * time.Second,
				PollTimeout:               30 * time.Second,
				ShutdownTimeout:           5 * time.Second,
				CacheSyncTimeout:          60 * time.Second,
				ReconnectMaxElapsedTime:   300 * time.Second,
				ConsecutiveErrorThreshold: 3,
				MaxBackoffFactor:          4,
				CacheCleanupEnabled:       true,
				CacheCleanupInterval:      24 * time.Hour,
				CacheRetentionDays:        7,
				HealthServerAddr:          ":8080",
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
				InfluxDBBucket:       "test_bucket",
				SlackWebhookURL:      "https://example.com/test-webhook",
				PollInterval:         30 * time.Second,
				CacheDir:             "./cache",
				LogLevel:             "info",
			},
			wantErr: true,
			errMsg:  "OCTOPUS_API_KEY",
		},
		{
			name: "empty influxdb url",
			cfg: &Config{
				OctopusAPIKey:        "test_key_123456789012345678901234",
				OctopusAccountNumber: "A-12345678",
				InfluxDBToken:        "test_token",
				InfluxDBOrg:          "test_org",
				InfluxDBBucket:       "test_bucket",
				SlackWebhookURL:      "https://example.com/test-webhook",
				PollInterval:         30 * time.Second,
				CacheDir:             "./cache",
				LogLevel:             "info",
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

func TestValidateCacheDirectory(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid writable directory",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr: false,
		},
		{
			name: "directory does not exist - creates it",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "new_cache_dir")
			},
			wantErr: false,
		},
		{
			name: "path exists but is a file not directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "not_a_dir")
				f, _ := os.Create(filePath)
				f.Close()
				return filePath
			},
			wantErr: true,
			errMsg:  "not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cachePath := tt.setup(t)
			cfg := &Config{
				CacheDir: cachePath,
			}

			err := cfg.validateCacheDirectory()

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateCacheDirectory() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateCacheDirectory() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateCacheDirectory() unexpected error = %v", err)
				}
				// Verify directory was created and is writable
				if _, err := os.Stat(cachePath); os.IsNotExist(err) {
					t.Errorf("validateCacheDirectory() directory was not created: %s", cachePath)
				}
			}
		})
	}
}

func TestValidateInfluxDBConnectivity(t *testing.T) {
	tests := []struct {
		name       string
		serverFunc func() *httptest.Server
		wantErr    bool
		errMsg     string
	}{
		{
			name: "healthy influxdb",
			serverFunc: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/health" {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"status":"pass"}`))
					}
				}))
			},
			wantErr: false,
		},
		{
			name: "influxdb health check fails",
			serverFunc: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/health" {
						w.WriteHeader(http.StatusServiceUnavailable)
						w.Write([]byte(`{"status":"fail"}`))
					}
				}))
			},
			wantErr: true,
			errMsg:  "health check failed",
		},
		{
			name: "influxdb unreachable",
			serverFunc: func() *httptest.Server {
				// Create and immediately close the server to simulate unreachable
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				s.Close()
				return s
			},
			wantErr: true,
			errMsg:  "failed to connect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.serverFunc()
			if server != nil {
				defer server.Close()
			}

			cfg := &Config{
				InfluxDBURL: server.URL,
			}

			ctx := context.Background()
			err := cfg.validateInfluxDBConnectivity(ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateInfluxDBConnectivity() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateInfluxDBConnectivity() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("validateInfluxDBConnectivity() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateRuntime(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (*Config, *httptest.Server)
		wantErr   bool
		errMsg    string
		isWarning bool // If true, error is a warning, not fatal
	}{
		{
			name: "valid runtime configuration",
			setup: func(t *testing.T) (*Config, *httptest.Server) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/health" {
						w.WriteHeader(http.StatusOK)
					}
				}))
				return &Config{
					CacheDir:    t.TempDir(),
					InfluxDBURL: server.URL,
				}, server
			},
			wantErr: false,
		},
		{
			name: "cache directory invalid",
			setup: func(t *testing.T) (*Config, *httptest.Server) {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "not_a_dir")
				f, _ := os.Create(filePath)
				f.Close()

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))

				return &Config{
					CacheDir:    filePath,
					InfluxDBURL: server.URL,
				}, server
			},
			wantErr: true,
			errMsg:  "cache directory validation failed",
		},
		{
			name: "influxdb connectivity warning",
			setup: func(t *testing.T) (*Config, *httptest.Server) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				return &Config{
					CacheDir:    t.TempDir(),
					InfluxDBURL: server.URL,
				}, server
			},
			wantErr:   true,
			errMsg:    "warning",
			isWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, server := tt.setup(t)
			if server != nil {
				defer server.Close()
			}

			ctx := context.Background()
			err := cfg.ValidateRuntime(ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateRuntime() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateRuntime() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("ValidateRuntime() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateRuntimeContextTimeout(t *testing.T) {
	// Test that context cancellation is respected
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow server that doesn't respond quickly
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		CacheDir:    t.TempDir(),
		InfluxDBURL: server.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := cfg.ValidateRuntime(ctx)
	if err == nil {
		t.Error("ValidateRuntime() expected timeout error, got nil")
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
