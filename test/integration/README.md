# Integration Tests

This directory contains end-to-end integration tests for the Octopus Home Mini Monitor application.

## Overview

The integration tests verify the complete system behavior including:
- Full monitoring loop with real InfluxDB
- Cache fallback and recovery scenarios
- InfluxDB failover and reconnection
- Concurrent operations and data integrity
- Long-running connection stability

## Prerequisites

### Docker and Docker Compose

The integration tests require InfluxDB to be running. You can start it using Docker Compose:

```bash
cd test/integration
docker-compose -f docker-compose.test.yml up -d
```

Wait a few seconds for InfluxDB to be ready:

```bash
docker-compose -f docker-compose.test.yml ps
```

### Environment Variables

The tests use the following environment variables (with defaults):

- `INFLUXDB_URL`: InfluxDB URL (default: `http://localhost:8086`)
- `INFLUXDB_TOKEN`: InfluxDB authentication token (default: `test-token-12345678901234567890`)
- `INFLUXDB_ORG`: InfluxDB organization (default: `test-org`)
- `INFLUXDB_BUCKET`: InfluxDB bucket (default: `test-bucket`)

These defaults match the docker-compose configuration.

## Running the Tests

### Run all integration tests

From the project root:

```bash
go test -v ./test/integration/
```

### Run with InfluxDB (full integration)

```bash
# Start InfluxDB
cd test/integration
docker-compose -f docker-compose.test.yml up -d

# Run tests
cd ../..
go test -v ./test/integration/

# Stop InfluxDB
cd test/integration
docker-compose -f docker-compose.test.yml down -v
```

### Skip integration tests (unit tests only)

```bash
go test -short ./test/integration/
```

All integration tests respect the `-short` flag and will be skipped when it's present.

### Run specific test

```bash
go test -v ./test/integration/ -run TestMonitorWithRealInfluxDB
```

### Run with coverage

```bash
go test -v -cover ./test/integration/
```

## Test Files

### `helpers_test.go`
Common test utilities and helper functions:
- `TestConfig()`: Creates test configuration
- `SkipIfNoInfluxDB()`: Skips tests if InfluxDB unavailable
- `CreateTestCache()`: Creates temporary cache for testing
- `CreateTestDataPoints()`: Generates test data
- `WaitForCondition()`: Waits for conditions with timeout
- `MockOctopusServer()`: Creates mock Octopus API server

### `monitor_test.go`
Main monitoring loop integration tests:
- Full monitoring loop with real InfluxDB
- Synchronous and asynchronous writes
- Complete data flow from cache to InfluxDB
- Concurrent write operations
- Batch write operations
- Health check verification
- Error handling

### `cache_recovery_test.go`
Cache fallback and recovery scenarios:
- Cache fallback when InfluxDB unavailable
- Cache data persistence across restarts
- Cache overflow handling
- Partial failure recovery
- Concurrent cache access
- Complete failure and recovery scenario
- Data integrity verification

### `influx_failover_test.go`
InfluxDB failover and reconnection tests:
- Connection failover behavior
- Automatic reconnection attempts
- Circuit breaker functionality
- Write error handling
- Health checks during writes
- Context cancellation
- Multiple concurrent clients
- Long-running connection stability
- Recovery after errors

## Test Database

The tests use a separate InfluxDB instance configured via Docker Compose:
- Organization: `test-org`
- Bucket: `test-bucket`
- Token: `test-token-12345678901234567890`
- Port: `8086`

Data written during tests is isolated to the test bucket and can be safely cleaned up:

```bash
docker-compose -f docker-compose.test.yml down -v
```

The `-v` flag removes the volume, ensuring a clean state for the next test run.

## Continuous Integration

Integration tests can be run in CI/CD pipelines. Example GitHub Actions workflow:

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  integration:
    runs-on: ubuntu-latest

    services:
      influxdb:
        image: influxdb:2.7-alpine
        ports:
          - 8086:8086
        env:
          DOCKER_INFLUXDB_INIT_MODE: setup
          DOCKER_INFLUXDB_INIT_USERNAME: admin
          DOCKER_INFLUXDB_INIT_PASSWORD: testpassword
          DOCKER_INFLUXDB_INIT_ORG: test-org
          DOCKER_INFLUXDB_INIT_BUCKET: test-bucket
          DOCKER_INFLUXDB_INIT_ADMIN_TOKEN: test-token-12345678901234567890

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Wait for InfluxDB
        run: |
          timeout 60 bash -c 'until curl -f http://localhost:8086/health; do sleep 2; done'

      - name: Run integration tests
        run: go test -v ./test/integration/
        env:
          INFLUXDB_URL: http://localhost:8086
          INFLUXDB_TOKEN: test-token-12345678901234567890
          INFLUXDB_ORG: test-org
          INFLUXDB_BUCKET: test-bucket
```

## Debugging

### View InfluxDB logs

```bash
cd test/integration
docker-compose -f docker-compose.test.yml logs -f influxdb-test
```

### Access InfluxDB UI

Open http://localhost:8086 in your browser and login with:
- Username: `admin`
- Password: `testpassword`

### Run tests with verbose output

```bash
go test -v -count=1 ./test/integration/ 2>&1 | tee test-output.log
```

### Check InfluxDB health

```bash
curl http://localhost:8086/health
```

## Troubleshooting

### Tests fail with "InfluxDB not available"

Ensure InfluxDB is running and healthy:

```bash
docker-compose -f test/integration/docker-compose.test.yml ps
docker-compose -f test/integration/docker-compose.test.yml logs
```

### Port 8086 already in use

Stop any existing InfluxDB instances:

```bash
docker ps | grep influx
docker stop <container_id>
```

Or change the port in `docker-compose.test.yml`.

### Tests hang or timeout

Increase test timeout:

```bash
go test -v -timeout 30m ./test/integration/
```

### Permission denied errors

Ensure Docker daemon is running and you have permissions:

```bash
sudo systemctl start docker
sudo usermod -aG docker $USER
```

## Best Practices

1. **Always run tests in isolation**: Use temporary directories and test databases
2. **Clean up after tests**: Use `defer` statements and cleanup functions
3. **Use short mode**: Respect the `-short` flag for fast CI pipelines
4. **Test timeouts**: Use contexts with timeouts to prevent hanging
5. **Concurrent safety**: Test concurrent operations to catch race conditions
6. **Error scenarios**: Test both success and failure paths
7. **Data integrity**: Verify data accuracy after operations

## Contributing

When adding new integration tests:

1. Use the helper functions in `helpers_test.go`
2. Follow the existing test patterns
3. Add appropriate comments and documentation
4. Test both success and failure scenarios
5. Use `testing.Short()` to allow skipping in fast CI
6. Clean up resources with `defer` statements
7. Update this README if adding new test categories
