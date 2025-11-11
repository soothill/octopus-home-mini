package secrets

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestEnvProvider_GetSecret(t *testing.T) {
	provider := NewEnvProvider()

	// Set a test environment variable
	testKey := "TEST_SECRET_KEY"
	testValue := "test_secret_value"
	os.Setenv(testKey, testValue)
	defer os.Unsetenv(testKey)

	ctx := context.Background()
	value, err := provider.GetSecret(ctx, testKey)

	if err != nil {
		t.Errorf("GetSecret() error = %v, want nil", err)
	}

	if value != testValue {
		t.Errorf("GetSecret() value = %v, want %v", value, testValue)
	}
}

func TestEnvProvider_GetSecret_NotFound(t *testing.T) {
	provider := NewEnvProvider()

	ctx := context.Background()
	_, err := provider.GetSecret(ctx, "NONEXISTENT_KEY")

	if err == nil {
		t.Error("GetSecret() expected error for missing key, got nil")
	}
}

func TestEnvProvider_SetSecret(t *testing.T) {
	provider := NewEnvProvider()

	ctx := context.Background()
	err := provider.SetSecret(ctx, "TEST_KEY", "TEST_VALUE")

	if err == nil {
		t.Error("SetSecret() expected error, got nil")
	}
}

func TestEnvProvider_Close(t *testing.T) {
	provider := NewEnvProvider()

	err := provider.Close()

	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestFileProvider_NewFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	provider, err := NewFileProvider(filePath)

	if err != nil {
		t.Fatalf("NewFileProvider() error = %v, want nil", err)
	}

	if provider == nil {
		t.Fatal("NewFileProvider() returned nil provider")
	}

	if provider.filePath != filePath {
		t.Errorf("filePath = %v, want %v", provider.filePath, filePath)
	}
}

func TestFileProvider_LoadExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	// Create a test .env file
	content := `
# Test secrets
API_KEY=secret123
DATABASE_URL="postgres://localhost/db"
PORT='8080'

# Empty values should be skipped
EMPTY=

# Malformed lines should be skipped
MALFORMED

TOKEN=abc-def-ghi
`
	err := os.WriteFile(filePath, []byte(content), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v, want nil", err)
	}

	ctx := context.Background()

	tests := []struct {
		name      string
		key       string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "simple value",
			key:       "API_KEY",
			wantValue: "secret123",
			wantErr:   false,
		},
		{
			name:      "double quoted value",
			key:       "DATABASE_URL",
			wantValue: "postgres://localhost/db",
			wantErr:   false,
		},
		{
			name:      "single quoted value",
			key:       "PORT",
			wantValue: "8080",
			wantErr:   false,
		},
		{
			name:      "value with dashes",
			key:       "TOKEN",
			wantValue: "abc-def-ghi",
			wantErr:   false,
		},
		{
			name:      "nonexistent key",
			key:       "NONEXISTENT",
			wantValue: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := provider.GetSecret(ctx, tt.key)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetSecret() error = %v, wantErr %v", err, tt.wantErr)
			}

			if value != tt.wantValue {
				t.Errorf("GetSecret() value = %v, want %v", value, tt.wantValue)
			}
		})
	}
}

func TestFileProvider_SetSecret(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	ctx := context.Background()

	// Set a new secret
	err = provider.SetSecret(ctx, "NEW_KEY", "new_value")
	if err != nil {
		t.Errorf("SetSecret() error = %v, want nil", err)
	}

	// Verify it was saved
	value, err := provider.GetSecret(ctx, "NEW_KEY")
	if err != nil {
		t.Errorf("GetSecret() error = %v, want nil", err)
	}

	if value != "new_value" {
		t.Errorf("GetSecret() value = %v, want %v", value, "new_value")
	}

	// Verify file was created and contains the secret
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Errorf("Failed to read file: %v", err)
	}

	if len(content) == 0 {
		t.Error("File is empty after SetSecret")
	}
}

func TestFileProvider_UpdateSecret(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	ctx := context.Background()

	// Set initial value
	err = provider.SetSecret(ctx, "KEY", "value1")
	if err != nil {
		t.Fatalf("SetSecret() error = %v", err)
	}

	// Update the value
	err = provider.SetSecret(ctx, "KEY", "value2")
	if err != nil {
		t.Fatalf("SetSecret() error = %v", err)
	}

	// Verify updated value
	value, err := provider.GetSecret(ctx, "KEY")
	if err != nil {
		t.Errorf("GetSecret() error = %v", err)
	}

	if value != "value2" {
		t.Errorf("GetSecret() value = %v, want %v", value, "value2")
	}
}

func TestFileProvider_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	ctx := context.Background()

	// Set initial secrets
	for i := 0; i < 10; i++ {
		key := "KEY_" + string(rune('0'+i))
		value := "value_" + string(rune('0'+i))
		provider.SetSecret(ctx, key, value)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key := "KEY_" + string(rune('0'+(index%10)))
			_, err := provider.GetSecret(ctx, key)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key := "WRITE_KEY_" + string(rune('0'+(index%10)))
			value := "write_value"
			err := provider.SetSecret(ctx, key, value)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestFileProvider_Close(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	err = provider.Close()
	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestManager_SingleProvider(t *testing.T) {
	testKey := "TEST_MANAGER_KEY"
	testValue := "test_manager_value"
	os.Setenv(testKey, testValue)
	defer os.Unsetenv(testKey)

	envProvider := NewEnvProvider()
	manager := NewManager(envProvider)

	ctx := context.Background()
	value, err := manager.GetSecret(ctx, testKey)

	if err != nil {
		t.Errorf("GetSecret() error = %v, want nil", err)
	}

	if value != testValue {
		t.Errorf("GetSecret() value = %v, want %v", value, testValue)
	}
}

func TestManager_MultipleProviders_Fallback(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	// Create file provider with a secret
	fileProvider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	ctx := context.Background()
	fileProvider.SetSecret(ctx, "FILE_SECRET", "from_file")

	// Create env provider with a different secret
	envKey := "ENV_SECRET"
	os.Setenv(envKey, "from_env")
	defer os.Unsetenv(envKey)

	envProvider := NewEnvProvider()

	// Manager tries env first, then file
	manager := NewManager(envProvider, fileProvider)

	// Test getting from env (first provider)
	value, err := manager.GetSecret(ctx, envKey)
	if err != nil {
		t.Errorf("GetSecret(ENV_SECRET) error = %v, want nil", err)
	}
	if value != "from_env" {
		t.Errorf("GetSecret(ENV_SECRET) value = %v, want %v", value, "from_env")
	}

	// Test getting from file (fallback to second provider)
	value, err = manager.GetSecret(ctx, "FILE_SECRET")
	if err != nil {
		t.Errorf("GetSecret(FILE_SECRET) error = %v, want nil", err)
	}
	if value != "from_file" {
		t.Errorf("GetSecret(FILE_SECRET) value = %v, want %v", value, "from_file")
	}
}

func TestManager_NotFound(t *testing.T) {
	envProvider := NewEnvProvider()
	manager := NewManager(envProvider)

	ctx := context.Background()
	_, err := manager.GetSecret(ctx, "NONEXISTENT_KEY")

	if err == nil {
		t.Error("GetSecret() expected error for missing key, got nil")
	}
}

func TestManager_Close(t *testing.T) {
	envProvider := NewEnvProvider()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")
	fileProvider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	manager := NewManager(envProvider, fileProvider)

	err = manager.Close()
	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestNewProvider_Env(t *testing.T) {
	cfg := Config{
		Type: ProviderTypeEnv,
	}

	provider, err := NewProvider(cfg)

	if err != nil {
		t.Errorf("NewProvider() error = %v, want nil", err)
	}

	if provider == nil {
		t.Fatal("NewProvider() returned nil")
	}

	if _, ok := provider.(*EnvProvider); !ok {
		t.Error("NewProvider() did not return EnvProvider")
	}
}

func TestNewProvider_File(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	cfg := Config{
		Type: ProviderTypeFile,
		Options: map[string]string{
			"file_path": filePath,
		},
	}

	provider, err := NewProvider(cfg)

	if err != nil {
		t.Errorf("NewProvider() error = %v, want nil", err)
	}

	if provider == nil {
		t.Fatal("NewProvider() returned nil")
	}

	if _, ok := provider.(*FileProvider); !ok {
		t.Error("NewProvider() did not return FileProvider")
	}
}

func TestNewProvider_File_DefaultPath(t *testing.T) {
	cfg := Config{
		Type:    ProviderTypeFile,
		Options: map[string]string{},
	}

	provider, err := NewProvider(cfg)

	if err != nil {
		// It's okay if the default .env doesn't exist
		if !os.IsNotExist(err) {
			t.Errorf("NewProvider() unexpected error = %v", err)
		}
		return
	}

	if provider == nil {
		t.Fatal("NewProvider() returned nil")
	}

	if fileProvider, ok := provider.(*FileProvider); ok {
		if fileProvider.filePath != ".env" {
			t.Errorf("filePath = %v, want .env", fileProvider.filePath)
		}
	}
}

func TestNewProvider_AWS(t *testing.T) {
	cfg := Config{
		Type: ProviderTypeAWS,
	}

	_, err := NewProvider(cfg)

	if err == nil {
		t.Error("NewProvider(AWS) expected error, got nil")
	}
}

func TestNewProvider_Vault(t *testing.T) {
	cfg := Config{
		Type: ProviderTypeVault,
	}

	_, err := NewProvider(cfg)

	if err == nil {
		t.Error("NewProvider(Vault) expected error, got nil")
	}
}

func TestNewProvider_K8s(t *testing.T) {
	cfg := Config{
		Type: ProviderTypeK8s,
	}

	_, err := NewProvider(cfg)

	if err == nil {
		t.Error("NewProvider(K8s) expected error, got nil")
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	cfg := Config{
		Type: ProviderType("unknown"),
	}

	_, err := NewProvider(cfg)

	if err == nil {
		t.Error("NewProvider(unknown) expected error, got nil")
	}
}

func TestFileProvider_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	// Create empty file
	err := os.WriteFile(filePath, []byte(""), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	ctx := context.Background()
	_, err = provider.GetSecret(ctx, "ANY_KEY")

	if err == nil {
		t.Error("GetSecret() on empty file should return error")
	}
}

func TestFileProvider_CommentsOnly(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	content := `
# This is a comment
# Another comment

# Yet another comment
`
	err := os.WriteFile(filePath, []byte(content), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	if len(provider.secrets) != 0 {
		t.Errorf("Expected 0 secrets from comments-only file, got %d", len(provider.secrets))
	}
}

func TestFileProvider_SpecialCharacters(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	content := `
KEY_WITH_SPECIAL=value!@#$%^&*()
URL=https://example.com/path?query=1&other=2
EMAIL=user@example.com
`
	err := os.WriteFile(filePath, []byte(content), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		key       string
		wantValue string
	}{
		{"KEY_WITH_SPECIAL", "value!@#$%^&*()"},
		{"URL", "https://example.com/path?query=1&other=2"},
		{"EMAIL", "user@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			value, err := provider.GetSecret(ctx, tt.key)
			if err != nil {
				t.Errorf("GetSecret() error = %v", err)
			}
			if value != tt.wantValue {
				t.Errorf("GetSecret() value = %v, want %v", value, tt.wantValue)
			}
		})
	}
}

func TestManager_EmptyProviders(t *testing.T) {
	manager := NewManager()

	ctx := context.Background()
	_, err := manager.GetSecret(ctx, "ANY_KEY")

	if err == nil {
		t.Error("GetSecret() with no providers should return error")
	}
}

func TestFileProvider_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	// Create first provider and set a secret
	provider1, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	ctx := context.Background()
	err = provider1.SetSecret(ctx, "PERSIST_KEY", "persist_value")
	if err != nil {
		t.Fatalf("SetSecret() error = %v", err)
	}

	// Close first provider
	provider1.Close()

	// Create new provider and verify secret persisted
	provider2, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	value, err := provider2.GetSecret(ctx, "PERSIST_KEY")
	if err != nil {
		t.Errorf("GetSecret() error = %v", err)
	}

	if value != "persist_value" {
		t.Errorf("GetSecret() value = %v, want %v", value, "persist_value")
	}
}

func TestFileProvider_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".env")

	provider, err := NewFileProvider(filePath)
	if err != nil {
		t.Fatalf("NewFileProvider() error = %v", err)
	}

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations should still work (context not used in current implementation)
	err = provider.SetSecret(ctx, "KEY", "value")
	if err != nil {
		t.Errorf("SetSecret() with cancelled context error = %v", err)
	}
}

func TestManager_GetSecret_ContextTimeout(t *testing.T) {
	envProvider := NewEnvProvider()
	manager := NewManager(envProvider)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to timeout
	time.Sleep(1 * time.Millisecond)

	// Operation should still work (context not used in current implementation)
	testKey := "TEST_TIMEOUT_KEY"
	testValue := "test_value"
	os.Setenv(testKey, testValue)
	defer os.Unsetenv(testKey)

	value, err := manager.GetSecret(ctx, testKey)
	if err != nil {
		t.Errorf("GetSecret() error = %v", err)
	}
	if value != testValue {
		t.Errorf("GetSecret() value = %v, want %v", value, testValue)
	}
}

func TestProviderType_String(t *testing.T) {
	tests := []struct {
		name string
		pt   ProviderType
		want string
	}{
		{"env", ProviderTypeEnv, "env"},
		{"file", ProviderTypeFile, "file"},
		{"aws", ProviderTypeAWS, "aws"},
		{"vault", ProviderTypeVault, "vault"},
		{"k8s", ProviderTypeK8s, "k8s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.pt) != tt.want {
				t.Errorf("ProviderType string = %v, want %v", string(tt.pt), tt.want)
			}
		})
	}
}
