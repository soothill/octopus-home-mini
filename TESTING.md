# Testing Guide

This document describes the test suite for the Octopus Home Mini Monitor.

## Test Coverage

The project includes **5 comprehensive test files** covering all major components:

### 1. Configuration Tests ([pkg/config/config_test.go](pkg/config/config_test.go))

**Coverage:**
- Environment variable loading with defaults
- Configuration validation
- Missing required fields detection
- Integer parsing from environment variables
- Custom poll interval handling

**Test Cases: 10**
- Valid configuration
- Missing API keys
- Missing account numbers
- Missing InfluxDB credentials
- Missing Slack webhook
- Custom intervals
- Invalid integer values

### 2. Cache Tests ([pkg/cache/cache_test.go](pkg/cache/cache_test.go))

**Coverage:**
- Cache directory creation
- Adding single and multiple data points
- Retrieving all cached data
- Counting cached items
- Clearing cache
- Persistence (save/load)
- Cleanup of old files
- Concurrent access safety

**Test Cases: 8**
- Cache initialization with various directory paths
- Data addition and retrieval
- Cache persistence across instances
- Old file cleanup with time-based retention
- Thread-safe concurrent writes
- Data integrity after reload

### 3. Slack Notifier Tests ([pkg/slack/notifier_test.go](pkg/slack/notifier_test.go))

**Coverage:**
- Webhook initialization
- Error notifications
- Warning notifications
- Info notifications
- Cache alerts
- HTTP error handling
- Network failures
- JSON payload formatting
- Special character handling

**Test Cases: 7**
- Successful message delivery
- Server error responses
- Network connectivity issues
- Message structure validation
- Color coding (danger, warning, good)
- Field formatting
- Timestamp inclusion

### 4. Octopus Client Tests ([pkg/octopus/client_test.go](pkg/octopus/client_test.go))

**Coverage:**
- Client initialization
- Telemetry data structure
- Authentication flow
- Context timeout handling
- Time range validation
- State management
- GraphQL endpoint verification

**Test Cases: 8**
- Client creation with API credentials
- Data structure validation
- Authentication with invalid credentials
- Timeout handling
- Various time range scenarios
- Token and meter GUID state

### 5. InfluxDB Client Tests ([pkg/influx/client_test.go](pkg/influx/client_test.go))

**Coverage:**
- Data point structure
- Client initialization
- Connection validation
- Data point validation
- Multiple data point handling
- Time zone support
- Error handling

**Test Cases: 8**
- Invalid URL handling
- Connection timeout
- Data point structure validation
- Zero and negative values
- Time zone conversions
- Multiple data point creation

## Running Tests

### Run All Tests

```bash
make test
```

### Run with Coverage

```bash
make coverage
```

This generates a coverage report and opens it in your browser.

### Run Short Tests (Unit Tests Only)

```bash
go test ./pkg/... -v -short
```

This skips integration tests that require real services (InfluxDB, Octopus API).

### Run Specific Package Tests

```bash
# Test config package
go test ./pkg/config -v

# Test cache package
go test ./pkg/cache -v

# Test slack package
go test ./pkg/slack -v

# Test octopus package
go test ./pkg/octopus -v

# Test influx package
go test ./pkg/influx -v
```

### Run with Race Detection

```bash
go test -race ./pkg/...
```

This detects race conditions in concurrent code (particularly useful for cache tests).

## Test Patterns

### Table-Driven Tests

Most tests use table-driven patterns for clarity and maintainability:

```go
tests := []struct {
    name    string
    input   Input
    want    Output
    wantErr bool
}{
    {
        name:    "valid input",
        input:   validInput,
        want:    expectedOutput,
        wantErr: false,
    },
    // More test cases...
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // Test implementation
    })
}
```

### Mock HTTP Servers

Slack tests use `httptest.NewServer` to mock webhook endpoints:

```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Verify request
    w.WriteHeader(http.StatusOK)
}))
defer server.Close()
```

### Concurrent Testing

Cache tests verify thread-safety:

```go
done := make(chan bool)
for i := 0; i < 10; i++ {
    go func(n int) {
        cache.AddSingle(dataPoint)
        done <- true
    }(i)
}
```

### Temporary Directories

Tests use temporary directories for file operations:

```go
cacheDir := filepath.Join(os.TempDir(), "test_cache")
defer os.RemoveAll(cacheDir)
```

## Integration Tests

Some tests are marked as integration tests and skipped during short test runs:

```go
if testing.Short() {
    t.Skip("Skipping integration test")
}
```

These tests would require:
- Running InfluxDB instance
- Valid Octopus Energy credentials
- Active Slack webhook

## Continuous Integration

The project includes GitHub Actions workflow (`.github/workflows/test.yml`) that runs:
- All unit tests with race detection
- Coverage reporting
- Code linting with golangci-lint
- Build verification

## Coverage Goals

Current coverage by package:
- **config**: ~95% (all major paths covered)
- **cache**: ~90% (core functionality fully tested)
- **slack**: ~85% (webhook and formatting covered)
- **octopus**: ~60% (structure tests, requires mocking for full coverage)
- **influx**: ~55% (structure tests, integration tests skipped)

## Writing New Tests

When adding new features, follow these guidelines:

1. **Create test file**: `filename_test.go` in the same package
2. **Use table-driven tests**: For multiple scenarios
3. **Test edge cases**: Empty inputs, nil values, boundary conditions
4. **Test errors**: Verify error messages and types
5. **Use subtests**: `t.Run()` for organization
6. **Clean up resources**: Use `defer` for cleanup
7. **Add comments**: Explain non-obvious test logic

### Example Test Template

```go
func TestNewFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:    "happy path",
            input:   "valid",
            want:    "expected",
            wantErr: false,
        },
        {
            name:    "error case",
            input:   "invalid",
            want:    "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := NewFeature(tt.input)

            if (err != nil) != tt.wantErr {
                t.Errorf("NewFeature() error = %v, wantErr %v", err, tt.wantErr)
                return
            }

            if got != tt.want {
                t.Errorf("NewFeature() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Test Utilities

### Setup Scripts Testing

The `scripts/` directory contains setup and configuration scripts that can be tested manually:

```bash
# Test setup wizard
make setup

# Test configuration wizard
make configure

# Test Slack webhook
make test-slack

# Test InfluxDB connection
make test-influx

# Verify all configuration
make verify-config
```

## Benchmarking

To add benchmarks for performance-critical code:

```go
func BenchmarkCacheAdd(b *testing.B) {
    cache, _ := NewCache(os.TempDir())
    dp := DataPoint{Timestamp: time.Now(), ConsumptionDelta: 0.5}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cache.AddSingle(dp)
    }
}
```

Run benchmarks:

```bash
go test -bench=. ./pkg/cache
```

## Troubleshooting Tests

### Tests Hanging

If tests hang, check for:
- Unclosed HTTP connections
- Blocked channels
- Missing context cancellation

Add timeouts to tests:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
```

### Flaky Tests

If tests fail intermittently:
- Check for race conditions: `go test -race`
- Review concurrent code
- Ensure proper cleanup
- Add synchronization where needed

### Test Failures

When tests fail:
1. Read the error message carefully
2. Check if test data matches expectations
3. Verify mocks and test servers
4. Run test individually: `go test -run TestName`
5. Add debug output: `t.Logf("Debug: %v", value)`

## Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Table-Driven Tests in Go](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [httptest Package](https://pkg.go.dev/net/http/httptest)
- [Go Test Coverage](https://go.dev/blog/cover)
