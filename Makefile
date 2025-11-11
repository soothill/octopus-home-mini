.PHONY: build run test clean install deps setup configure test-slack test-influx verify-config

# Setup and Configuration
setup: deps
	@echo "Setting up Octopus Home Mini Monitor..."
	@bash scripts/setup.sh

# Interactive configuration wizard
configure:
	@bash scripts/configure.sh

# Test Slack webhook connection
test-slack:
	@echo "Testing Slack webhook..."
	@bash scripts/test-slack.sh

# Test InfluxDB connection
test-influx:
	@echo "Testing InfluxDB connection..."
	@bash scripts/test-influx.sh

# Verify all configuration is correct
verify-config:
	@echo "Verifying configuration..."
	@bash scripts/verify-config.sh

# Build the application
build:
	@echo "Building octopus-monitor..."
	@go build -o octopus-monitor cmd/octopus-monitor/main.go

# Build for production (static binary)
build-prod:
	@echo "Building production binary..."
	@CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-w -s' -o octopus-monitor cmd/octopus-monitor/main.go

# Run the application
run:
	@go run cmd/octopus-monitor/main.go

# Run tests
test:
	@go test -v ./...

# Run tests with coverage
coverage:
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f octopus-monitor
	@rm -f coverage.out
	@rm -rf cache/

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	@golangci-lint run

# Install the application
install: build
	@echo "Installing octopus-monitor..."
	@sudo cp octopus-monitor /usr/local/bin/

# Check for updates
update:
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy

# Docker build
docker-build:
	@echo "Building Docker image..."
	@docker build -t octopus-monitor:latest .

# Docker run
docker-run:
	@echo "Running Docker container..."
	@docker run -d --name octopus-monitor --env-file .env octopus-monitor:latest

# Show help
help:
	@echo "Octopus Home Mini Monitor - Available targets:"
	@echo ""
	@echo "Setup & Configuration:"
	@echo "  setup         - Complete setup wizard (deps + configure)"
	@echo "  configure     - Interactive configuration wizard"
	@echo "  test-slack    - Test Slack webhook connection"
	@echo "  test-influx   - Test InfluxDB connection"
	@echo "  verify-config - Verify all configuration settings"
	@echo ""
	@echo "Build & Run:"
	@echo "  build         - Build the application"
	@echo "  build-prod    - Build production binary (static)"
	@echo "  run           - Run the application"
	@echo "  install       - Install to /usr/local/bin"
	@echo ""
	@echo "Development:"
	@echo "  deps          - Install dependencies"
	@echo "  test          - Run tests"
	@echo "  coverage      - Run tests with coverage report"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code"
	@echo "  update        - Update dependencies"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo ""
	@echo "Maintenance:"
	@echo "  clean         - Clean build artifacts"
	@echo "  help          - Show this help message"
