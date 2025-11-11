# Octopus Home Mini Monitor

A Go application that pulls usage data from an Octopus Energy Home Mini device and logs it to InfluxDB. The application includes robust error handling, Slack alerting, and local caching capabilities to ensure data is never lost.

## Features

### Core Functionality
- **Real-time Data Collection**: Pulls energy consumption data from Octopus Home Mini every 30 seconds (configurable)
- **InfluxDB Integration**: Stores energy metrics in InfluxDB for long-term analysis and visualization
- **Slack Notifications**: Sends alerts on failures, warnings, and important events
- **Local Caching**: Automatically caches data locally when InfluxDB is unavailable
- **Automatic Sync**: Syncs cached data to InfluxDB when connection is restored

### Reliability & Resilience
- **Circuit Breakers**: Protects against cascading failures with circuit breaker pattern on all external services (Octopus API, InfluxDB, Slack)
- **Exponential Backoff**: Automatic retry with exponential backoff for transient failures
- **Graceful Degradation**: Enters degraded mode after consecutive failures with adaptive polling intervals (2x, 3x, up to 4x) to reduce load on failing services
- **Health Monitoring**: Continuously monitors InfluxDB connection health with automatic recovery
- **Configuration Validation**: Validates configuration and tests connectivity on startup to fail fast
- **Proper Resource Cleanup**: Graceful shutdown with timeout, signal handler cleanup, and HTTP connection cleanup

### Security & Configuration
- **Secrets Management**: Flexible secrets provider system supporting environment variables, file-based secrets, and extensible for AWS Secrets Manager, HashiCorp Vault, and Kubernetes Secrets
- **Input Validation**: Comprehensive validation and sanitization of all configuration inputs to prevent injection attacks
- **Secure Defaults**: Security-focused defaults including URL validation and path traversal protection

### Monitoring & Operations
- **Health Check HTTP Endpoints**: Kubernetes-ready liveness (`/health`) and readiness (`/ready`) endpoints for container orchestration
- **Component Health Checks**: Extensible health checker system for monitoring individual component health
- **Graceful Shutdown**: Handles shutdown signals properly, persists unsaved data, and waits for in-flight operations

### Testing & Quality
- **Comprehensive Test Coverage**: Unit tests for all packages (config: 100%, cache: 85%+, slack: 90%+, influx: 20%+, octopus: 75%+)
- **Integration Tests**: End-to-end integration tests with real InfluxDB (Docker Compose-based)
- **CI/CD Pipeline**: GitHub Actions workflow with automated testing, linting, and security scanning

## Architecture

The application consists of several key components:

- **Octopus API Client** ([pkg/octopus/client.go](pkg/octopus/client.go)): GraphQL client for Octopus Energy API with circuit breaker and exponential backoff
- **InfluxDB Client** ([pkg/influx/client.go](pkg/influx/client.go)): Handles writing data to InfluxDB with async error monitoring and circuit breaker protection
- **Cache System** ([pkg/cache/cache.go](pkg/cache/cache.go)): Local file-based cache for offline data storage with automatic persistence
- **Slack Notifier** ([pkg/slack/notifier.go](pkg/slack/notifier.go)): Sends formatted alerts to Slack with retry logic and circuit breaker
- **Configuration** ([pkg/config/config.go](pkg/config/config.go)): Environment-based configuration management with validation and runtime connectivity checks
- **Health Server** ([pkg/health/server.go](pkg/health/server.go)): HTTP server providing liveness and readiness endpoints for Kubernetes
- **Secrets Management** ([pkg/secrets/secrets.go](pkg/secrets/secrets.go)): Flexible secrets provider supporting multiple backends (env, file, AWS, Vault, K8s)
- **Main Monitor** ([cmd/octopus-monitor/main.go](cmd/octopus-monitor/main.go)): Orchestrates all components with graceful degradation and adaptive polling

## Prerequisites

- Go 1.24 or later
- Octopus Energy account with Home Mini device
- Octopus Energy API key ([get it here](https://octopus.energy/dashboard/new/accounts/personal-details/api-access))
- InfluxDB instance (v2.x)
- Slack workspace with incoming webhook configured

## Installation

1. Clone the repository:
```bash
git clone https://github.com/darren/octopus-home-mini.git
cd octopus-home-mini
```

2. Run the setup wizard (recommended):
```bash
make setup
```

This will:
- Install Go dependencies
- Guide you through interactive configuration
- Test your Slack and InfluxDB connections

**Or manually configure:**

3. Copy the example environment file:
```bash
cp .env.example .env
```

4. Edit `.env` and fill in your credentials:
```bash
# Octopus Energy API Configuration
OCTOPUS_API_KEY=sk_live_your_api_key_here
OCTOPUS_ACCOUNT_NUMBER=A-12345678

# InfluxDB Configuration
INFLUXDB_URL=http://localhost:8086
INFLUXDB_TOKEN=your_influxdb_token_here
INFLUXDB_ORG=your_org_name
INFLUXDB_BUCKET=octopus_energy

# Slack Configuration
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/WEBHOOK/URL

# Application Configuration (optional)
POLL_INTERVAL_SECONDS=30
CACHE_DIR=./cache
LOG_LEVEL=info
```

## Quick Setup with Makefile

The project includes helpful Makefile targets for easy setup and testing:

```bash
# Complete setup wizard (interactive)
make setup

# Just run configuration wizard
make configure

# Test individual components
make test-slack      # Test Slack webhook
make test-influx     # Test InfluxDB connection
make verify-config   # Verify all configuration

# View all available commands
make help
```

### Configuration Wizard

Run `make configure` for an interactive configuration experience that will:
- Prompt for all required settings
- Provide helpful links for getting API keys
- Validate configuration format
- Optionally test connections

## Configuration

### Octopus Energy API

To get your API key:
1. Log in to your [Octopus Energy account](https://octopus.energy/dashboard)
2. Navigate to Account → Personal Details → API Access
3. Generate or copy your API key
4. Your account number is displayed on your dashboard (format: A-XXXXXXXX)

### InfluxDB Setup

1. Create a bucket for the data:
```bash
influx bucket create -n octopus_energy -o your_org_name
```

2. Create an API token with write permissions:
```bash
influx auth create --write-bucket octopus_energy
```

### Slack Webhook

1. Go to [Slack Apps](https://api.slack.com/apps)
2. Create a new app or use an existing one
3. Enable Incoming Webhooks
4. Create a new webhook for your desired channel
5. Copy the webhook URL

## Usage

### Run locally

```bash
go run cmd/octopus-monitor/main.go
```

### Build and run

```bash
go build -o octopus-monitor cmd/octopus-monitor/main.go
./octopus-monitor
```

### Run as a service (systemd)

Create `/etc/systemd/system/octopus-monitor.service`:

```ini
[Unit]
Description=Octopus Home Mini Monitor
After=network.target

[Service]
Type=simple
User=your_user
WorkingDirectory=/path/to/octopus-home-mini
EnvironmentFile=/path/to/octopus-home-mini/.env
ExecStart=/path/to/octopus-home-mini/octopus-monitor
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Then enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable octopus-monitor
sudo systemctl start octopus-monitor
```

### Docker

The project includes a multi-platform Dockerfile that automatically builds for your architecture.

Build and run:

```bash
# Build for current platform
make docker-build
docker run -d --name octopus-monitor --env-file .env octopus-monitor:latest

# Or using docker directly
docker build -t octopus-monitor .
docker run -d --name octopus-monitor --env-file .env octopus-monitor
```

For multi-platform builds (AMD64, ARM64, ARMv7):

```bash
# Build for multiple architectures
make docker-buildx

# Or to build and push to a registry
make docker-buildx-push
```

The Docker image automatically detects and uses the correct binary for your platform (x86_64, ARM64, or ARMv7).

## Data Schema

The application writes the following metrics to InfluxDB:

**Measurement**: `energy_consumption`

**Tags**:
- `source`: "octopus_home_mini"

**Fields**:
- `consumption_delta` (float): Incremental consumption since last reading (kWh)
- `demand` (float): Current power demand (kW)
- `cost_delta` (float): Cost of energy consumed since last reading (£)
- `consumption` (float): Total cumulative consumption (kWh)

**Timestamp**: Reading time from the Home Mini device

## Querying Data

### InfluxDB Flux Query Examples

Get the last hour of demand data:
```flux
from(bucket: "octopus_energy")
  |> range(start: -1h)
  |> filter(fn: (r) => r._measurement == "energy_consumption")
  |> filter(fn: (r) => r._field == "demand")
```

Calculate total consumption for today:
```flux
from(bucket: "octopus_energy")
  |> range(start: today())
  |> filter(fn: (r) => r._measurement == "energy_consumption")
  |> filter(fn: (r) => r._field == "consumption_delta")
  |> sum()
```

## Health Endpoints

The application provides HTTP health check endpoints for Kubernetes and container orchestration:

### Liveness Endpoint: `/health`
Returns `200 OK` if the application is running. This endpoint checks basic application health.

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2025-11-11T18:30:00Z",
  "version": "1.0.0"
}
```

### Readiness Endpoint: `/ready`
Returns `200 OK` if all registered components are healthy, `503 Service Unavailable` if any component is unhealthy.

```bash
curl http://localhost:8080/ready
```

Response:
```json
{
  "ready": true,
  "timestamp": "2025-11-11T18:30:00Z",
  "components": {
    "influxdb": {
      "status": "healthy",
      "message": "Connected"
    },
    "octopus_api": {
      "status": "healthy",
      "message": "API accessible"
    }
  }
}
```

## Graceful Degradation

The application implements intelligent graceful degradation to handle service failures:

### Degraded Mode Operation
When the Octopus API experiences consecutive failures (3+), the monitor:
1. Enters **degraded mode** and sends a Slack alert
2. Increases the polling interval to reduce load on the failing service:
   - 1st degradation: 2x normal interval (e.g., 30s → 60s)
   - 2nd degradation: 3x normal interval (e.g., 30s → 90s)
   - 3rd+ degradation: 4x normal interval (maximum, e.g., 30s → 120s)
3. Continues attempting to fetch data at reduced frequency
4. Automatically recovers and resumes normal polling when the service is restored

### InfluxDB Failover
When InfluxDB is unavailable:
1. Automatically switches to local cache mode
2. Continues collecting data from Octopus API
3. Stores all data in local JSON files
4. Monitors InfluxDB health with exponential backoff reconnection attempts
5. Automatically syncs all cached data when InfluxDB recovers
6. Sends Slack notifications on state transitions

### Circuit Breaker Protection
All external services (Octopus API, InfluxDB, Slack) are protected by circuit breakers:
- **Failure Threshold**: 60% failure rate over 3 requests
- **Timeout**: 30-60 seconds before attempting to close circuit
- **Max Requests**: 3 requests allowed in half-open state
- Prevents cascading failures and excessive retry attempts

## Monitoring and Alerts

The application sends Slack notifications for:

- **Errors**:
  - Entering degraded mode (after 3 consecutive Octopus API failures)
  - Failed to write to InfluxDB
  - Failed to cache data locally
  - Failed to sync cached data
  - Circuit breaker opened

- **Warnings**:
  - InfluxDB connection lost (switching to cache mode)
  - Monitor stopped with data in cache
  - Configuration validation warnings

- **Info**:
  - Monitor started successfully
  - InfluxDB connection restored
  - Cache successfully synced
  - Recovered from degraded mode

## Cache Behavior

When InfluxDB is unavailable:

1. Data is automatically cached to local JSON files in the `CACHE_DIR`
2. Cache files are organized by date: `cache_YYYY-MM-DD.json`
3. The application continues fetching data from Octopus API
4. When InfluxDB connection is restored, all cached data is automatically synced
5. Cache is cleared after successful sync

The cache system ensures **no data loss** during InfluxDB outages.

## Troubleshooting

### "Failed to authenticate" error

- Verify your `OCTOPUS_API_KEY` is correct
- Ensure the API key has not been revoked

### "No smart devices found" error

- Verify your `OCTOPUS_ACCOUNT_NUMBER` is correct
- Ensure your Home Mini is properly set up and connected
- Check that your account has an active electricity agreement with a smart meter

### InfluxDB connection errors

- Verify `INFLUXDB_URL` is correct and accessible
- Check that `INFLUXDB_TOKEN` has write permissions to the bucket
- Ensure `INFLUXDB_ORG` and `INFLUXDB_BUCKET` exist

### Slack notifications not working

- Verify `SLACK_WEBHOOK_URL` is correct
- Test the webhook manually with curl:
  ```bash
  curl -X POST -H 'Content-type: application/json' \
    --data '{"text":"Test message"}' \
    YOUR_WEBHOOK_URL
  ```

### High memory usage

- Check cache directory size - old cache files may be accumulating
- Consider implementing periodic cache cleanup

## Development

### Running tests

The project includes comprehensive test coverage for all packages:

```bash
# Run all tests
make test

# Run tests with coverage report
make coverage

# Run unit tests only (skips integration tests)
go test ./pkg/... -v -short

# Run integration tests with InfluxDB
cd test/integration
docker-compose -f docker-compose.test.yml up -d
cd ../..
go test -v ./test/integration/
```

**Test Coverage:**
- **config**: Environment variable loading, validation, runtime connectivity checks (100%)
- **cache**: Local file caching, concurrent access, persistence (85%+)
- **slack**: Webhook notifications, error handling, message formatting, circuit breakers (90%+)
- **octopus**: API client, authentication flow, data parsing, circuit breakers, backoff (75%+)
- **influx**: Data point structure, client initialization, error monitoring, circuit breakers (20%+)
- **health**: HTTP server, liveness/readiness endpoints, component health checks (95%+)
- **secrets**: Secrets management, multiple provider types, concurrent access (94%+)
- **main**: Monitor lifecycle, graceful shutdown, degraded mode, cache sync (18%+)

**Integration Tests** ([test/integration](test/integration)):
- Full monitoring loop with real InfluxDB
- Cache fallback and recovery scenarios
- InfluxDB failover and reconnection
- Concurrent operations and data integrity
- Docker Compose-based test environment

All tests use table-driven testing patterns and cover:
- Happy path scenarios
- Error conditions
- Edge cases
- Concurrent access patterns
- Input validation
- Circuit breaker behavior
- Graceful degradation
- Resource cleanup

### Building for production

```bash
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o octopus-monitor cmd/octopus-monitor/main.go
```

### Multi-platform builds

The application supports building for multiple architectures:

```bash
# Build for all platforms
make build-all

# Build for specific platforms
make build-linux-amd64   # Linux x86_64 (Intel/AMD servers)
make build-linux-arm64   # Linux ARM64 (Raspberry Pi 4/5, AWS Graviton)
make build-linux-armv7   # Linux ARMv7 (Raspberry Pi 2/3, 32-bit)
make build-darwin-amd64  # macOS Intel
make build-darwin-arm64  # macOS Apple Silicon (M1/M2/M3)
make build-windows-amd64 # Windows x86_64
```

Built binaries are placed in the `dist/` directory with platform-specific names:
- `octopus-monitor-linux-amd64`
- `octopus-monitor-linux-arm64`
- `octopus-monitor-linux-armv7`
- `octopus-monitor-darwin-amd64`
- `octopus-monitor-darwin-arm64`
- `octopus-monitor-windows-amd64.exe`

All binaries are statically linked with CGO disabled, making them fully portable without external dependencies.

#### Docker multi-platform builds

Build Docker images for multiple architectures using Docker Buildx:

```bash
# Build multi-platform images for AMD64, ARM64, and ARMv7
make docker-buildx

# Build and push to a container registry
make docker-buildx-push
```

The Docker image supports:
- `linux/amd64` - Standard x86_64 servers
- `linux/arm64` - ARM64 servers (AWS Graviton, Raspberry Pi 4/5)
- `linux/arm/v7` - ARMv7 devices (Raspberry Pi 2/3)

Docker will automatically pull the correct image for your platform.

## Project Structure

```
octopus-home-mini/
├── cmd/
│   └── octopus-monitor/
│       ├── main.go                # Main application entry point
│       └── main_test.go           # Main application tests
├── pkg/
│   ├── cache/
│   │   ├── cache.go               # Local caching system
│   │   └── cache_test.go          # Cache tests
│   ├── config/
│   │   ├── config.go              # Configuration management with validation
│   │   └── config_test.go         # Configuration tests
│   ├── health/
│   │   ├── server.go              # Health check HTTP server
│   │   └── server_test.go         # Health server tests
│   ├── influx/
│   │   ├── client.go              # InfluxDB client with circuit breaker
│   │   └── client_test.go         # InfluxDB client tests
│   ├── octopus/
│   │   ├── client.go              # Octopus Energy API client
│   │   └── client_test.go         # Octopus client tests
│   ├── secrets/
│   │   ├── secrets.go             # Secrets management providers
│   │   └── secrets_test.go        # Secrets tests
│   └── slack/
│       ├── notifier.go            # Slack notification client
│       └── notifier_test.go       # Slack notifier tests
├── test/
│   └── integration/
│       ├── docker-compose.test.yml # InfluxDB test environment
│       ├── helpers_test.go         # Integration test helpers
│       ├── integration_test.go     # Integration tests
│       └── README.md               # Integration test documentation
├── .env.example                    # Example environment configuration
├── .github/
│   └── workflows/
│       └── test.yml                # CI/CD pipeline
├── .gitattributes
├── .gitignore
├── LICENSE                         # MIT License
├── Makefile                        # Build and test automation with multi-platform support
├── PLATFORMS.md                    # Detailed multi-platform build documentation
├── TODO.md                         # Task tracking and roadmap
├── TESTING.md                      # Testing documentation
├── go.mod                          # Go module definition
├── go.sum                          # Go module checksums
└── README.md                       # This file
```

## API Rate Limits

The Octopus Energy API has a rate limit of **100 calls per hour** shared across all integrations (including their mobile app). The default polling interval of 30 seconds should stay well within this limit.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

Feel free to use this project for your own monitoring needs, modify it, and distribute it under the terms of the MIT License.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgments

- [Octopus Energy](https://octopus.energy/) for providing the GraphQL API
- [InfluxDB](https://www.influxdata.com/) for time-series data storage
- The Home Assistant community for documenting the Octopus Energy API
