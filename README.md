# Octopus Home Mini Monitor

A Go application that pulls usage data from an Octopus Energy Home Mini device and logs it to InfluxDB. The application includes robust error handling, Slack alerting, and local caching capabilities to ensure data is never lost.

## Features

- **Real-time Data Collection**: Pulls energy consumption data from Octopus Home Mini every 30 seconds (configurable)
- **InfluxDB Integration**: Stores energy metrics in InfluxDB for long-term analysis and visualization
- **Slack Notifications**: Sends alerts on failures, warnings, and important events
- **Local Caching**: Automatically caches data locally when InfluxDB is unavailable
- **Automatic Sync**: Syncs cached data to InfluxDB when connection is restored
- **Graceful Shutdown**: Handles shutdown signals properly and preserves unsaved data
- **Health Monitoring**: Continuously monitors InfluxDB connection health

## Architecture

The application consists of several key components:

- **Octopus API Client** ([pkg/octopus/client.go](pkg/octopus/client.go)): GraphQL client for Octopus Energy API
- **InfluxDB Client** ([pkg/influx/client.go](pkg/influx/client.go)): Handles writing data to InfluxDB
- **Cache System** ([pkg/cache/cache.go](pkg/cache/cache.go)): Local file-based cache for offline data storage
- **Slack Notifier** ([pkg/slack/notifier.go](pkg/slack/notifier.go)): Sends formatted alerts to Slack
- **Configuration** ([pkg/config/config.go](pkg/config/config.go)): Environment-based configuration management
- **Main Monitor** ([cmd/octopus-monitor/main.go](cmd/octopus-monitor/main.go)): Orchestrates all components

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

Create a `Dockerfile`:

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o octopus-monitor cmd/octopus-monitor/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/octopus-monitor .
CMD ["./octopus-monitor"]
```

Build and run:

```bash
docker build -t octopus-monitor .
docker run -d --name octopus-monitor --env-file .env octopus-monitor
```

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

## Monitoring and Alerts

The application sends Slack notifications for:

- **Errors**:
  - Failed to fetch data from Octopus API (after 3 consecutive failures)
  - Failed to write to InfluxDB
  - Failed to cache data locally
  - Failed to sync cached data

- **Warnings**:
  - InfluxDB connection lost (switching to cache mode)
  - Monitor stopped with data in cache

- **Info**:
  - Monitor started successfully
  - InfluxDB connection restored
  - Cache successfully synced

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
```

**Test Coverage:**
- **config**: Environment variable loading, validation
- **cache**: Local file caching, concurrent access, persistence
- **slack**: Webhook notifications, error handling, message formatting
- **octopus**: API client structure, authentication flow, data parsing
- **influx**: Data point structure, client initialization, time zone handling

All tests use table-driven testing patterns and cover:
- Happy path scenarios
- Error conditions
- Edge cases
- Concurrent access patterns
- Input validation

### Building for production

```bash
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o octopus-monitor cmd/octopus-monitor/main.go
```

## Project Structure

```
octopus-home-mini/
├── cmd/
│   └── octopus-monitor/
│       └── main.go           # Main application entry point
├── pkg/
│   ├── cache/
│   │   └── cache.go          # Local caching system
│   ├── config/
│   │   └── config.go         # Configuration management
│   ├── influx/
│   │   └── client.go         # InfluxDB client
│   ├── octopus/
│   │   └── client.go         # Octopus Energy API client
│   └── slack/
│       └── notifier.go       # Slack notification client
├── .env.example              # Example environment configuration
├── .gitattributes
├── go.mod                    # Go module definition
└── README.md                 # This file
```

## API Rate Limits

The Octopus Energy API has a rate limit of **100 calls per hour** shared across all integrations (including their mobile app). The default polling interval of 30 seconds should stay well within this limit.

## License

MIT License - feel free to use this project for your own monitoring needs.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgments

- [Octopus Energy](https://octopus.energy/) for providing the GraphQL API
- [InfluxDB](https://www.influxdata.com/) for time-series data storage
- The Home Assistant community for documenting the Octopus Energy API
