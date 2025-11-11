package secrets

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Provider defines the interface for secret providers
type Provider interface {
	// GetSecret retrieves a secret by key
	GetSecret(ctx context.Context, key string) (string, error)
	// SetSecret stores a secret (for providers that support it)
	SetSecret(ctx context.Context, key, value string) error
	// Close cleans up any resources
	Close() error
}

// ProviderType represents the type of secret provider
type ProviderType string

const (
	// ProviderTypeEnv uses environment variables
	ProviderTypeEnv ProviderType = "env"
	// ProviderTypeFile uses .env files
	ProviderTypeFile ProviderType = "file"
	// ProviderTypeAWS uses AWS Secrets Manager
	ProviderTypeAWS ProviderType = "aws"
	// ProviderTypeVault uses HashiCorp Vault
	ProviderTypeVault ProviderType = "vault"
	// ProviderTypeK8s uses Kubernetes Secrets
	ProviderTypeK8s ProviderType = "k8s"
)

// Config holds configuration for secret providers
type Config struct {
	Type ProviderType
	// Additional provider-specific configuration
	Options map[string]string
}

// NewProvider creates a new secret provider based on configuration
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Type {
	case ProviderTypeEnv:
		return NewEnvProvider(), nil
	case ProviderTypeFile:
		filePath := cfg.Options["file_path"]
		if filePath == "" {
			filePath = ".env"
		}
		return NewFileProvider(filePath)
	case ProviderTypeAWS:
		return nil, fmt.Errorf("AWS Secrets Manager provider not yet implemented")
	case ProviderTypeVault:
		return nil, fmt.Errorf("HashiCorp Vault provider not yet implemented")
	case ProviderTypeK8s:
		return nil, fmt.Errorf("Kubernetes Secrets provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}

// Manager manages multiple secret providers with fallback
type Manager struct {
	providers []Provider
}

// NewManager creates a new secret manager with multiple providers
func NewManager(providers ...Provider) *Manager {
	return &Manager{
		providers: providers,
	}
}

// GetSecret retrieves a secret from the first provider that has it
func (m *Manager) GetSecret(ctx context.Context, key string) (string, error) {
	var lastErr error
	for _, provider := range m.providers {
		value, err := provider.GetSecret(ctx, key)
		if err == nil && value != "" {
			return value, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", fmt.Errorf("secret %q not found in any provider: %w", key, lastErr)
	}
	return "", fmt.Errorf("secret %q not found in any provider", key)
}

// Close closes all providers
func (m *Manager) Close() error {
	var errs []error
	for _, provider := range m.providers {
		if err := provider.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing providers: %v", errs)
	}
	return nil
}

// EnvProvider retrieves secrets from environment variables
type EnvProvider struct{}

// NewEnvProvider creates a new environment variable provider
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{}
}

// GetSecret retrieves a secret from environment variables
func (p *EnvProvider) GetSecret(ctx context.Context, key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("environment variable %q not set", key)
	}
	return value, nil
}

// SetSecret is not supported for environment variables
func (p *EnvProvider) SetSecret(ctx context.Context, key, value string) error {
	return fmt.Errorf("SetSecret not supported for environment provider")
}

// Close does nothing for environment provider
func (p *EnvProvider) Close() error {
	return nil
}

// FileProvider retrieves secrets from .env files
type FileProvider struct {
	filePath string
	secrets  map[string]string
	mu       sync.RWMutex
}

// NewFileProvider creates a new file-based provider
func NewFileProvider(filePath string) (*FileProvider, error) {
	p := &FileProvider{
		filePath: filePath,
		secrets:  make(map[string]string),
	}

	// Load secrets from file if it exists
	if err := p.load(); err != nil {
		// If file doesn't exist, that's okay - we'll create it on first write
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load secrets from %s: %w", filePath, err)
		}
	}

	return p, nil
}

// load reads and parses the .env file
func (p *FileProvider) load() error {
	file, err := os.Open(p.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	p.mu.Lock()
	defer p.mu.Unlock()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, "\"'")

		p.secrets[key] = value
	}

	return scanner.Err()
}

// save writes the secrets back to the file
func (p *FileProvider) save() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	file, err := os.Create(p.filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for key, value := range p.secrets {
		if _, err := fmt.Fprintf(writer, "%s=%s\n", key, value); err != nil {
			return fmt.Errorf("failed to write secret: %w", err)
		}
	}

	return writer.Flush()
}

// GetSecret retrieves a secret from the file
func (p *FileProvider) GetSecret(ctx context.Context, key string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	value, ok := p.secrets[key]
	if !ok {
		return "", fmt.Errorf("secret %q not found in file", key)
	}

	return value, nil
}

// SetSecret stores a secret in the file
func (p *FileProvider) SetSecret(ctx context.Context, key, value string) error {
	p.mu.Lock()
	p.secrets[key] = value
	p.mu.Unlock()

	return p.save()
}

// Close does nothing for file provider
func (p *FileProvider) Close() error {
	return nil
}
