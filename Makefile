.PHONY: build run test clean install deps setup configure get-api-key test-slack test-influx verify-config build-all build-linux-amd64 build-linux-arm64 build-linux-armv7 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 docker-build docker-buildx docker-buildx-push docker-run

# Setup and Configuration
setup: deps
	@echo "Setting up Octopus Home Mini Monitor..."
	@bash scripts/setup.sh

# Interactive configuration wizard
configure:
	@bash scripts/configure.sh

# Help get Octopus API key
get-api-key:
	@bash scripts/get-api-key.sh

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

# Build for all platforms
build-all: build-linux-amd64 build-linux-arm64 build-linux-armv7 build-darwin-amd64 build-darwin-arm64 build-windows-amd64
	@echo "All platform builds complete!"
	@ls -lh dist/

# Build for Linux AMD64 (x86_64)
build-linux-amd64:
	@echo "Building for Linux AMD64..."
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w -s' -o dist/octopus-monitor-linux-amd64 cmd/octopus-monitor/main.go

# Build for Linux ARM64 (ARMv8)
build-linux-arm64:
	@echo "Building for Linux ARM64..."
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo -ldflags '-w -s' -o dist/octopus-monitor-linux-arm64 cmd/octopus-monitor/main.go

# Build for Linux ARMv7 (32-bit ARM, e.g., Raspberry Pi)
build-linux-armv7:
	@echo "Building for Linux ARMv7..."
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -a -installsuffix cgo -ldflags '-w -s' -o dist/octopus-monitor-linux-armv7 cmd/octopus-monitor/main.go

# Build for macOS AMD64 (Intel Mac)
build-darwin-amd64:
	@echo "Building for macOS AMD64..."
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w -s' -o dist/octopus-monitor-darwin-amd64 cmd/octopus-monitor/main.go

# Build for macOS ARM64 (Apple Silicon)
build-darwin-arm64:
	@echo "Building for macOS ARM64..."
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -a -installsuffix cgo -ldflags '-w -s' -o dist/octopus-monitor-darwin-arm64 cmd/octopus-monitor/main.go

# Build for Windows AMD64
build-windows-amd64:
	@echo "Building for Windows AMD64..."
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w -s' -o dist/octopus-monitor-windows-amd64.exe cmd/octopus-monitor/main.go

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
	@rm -rf dist/

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

# Docker build for current platform
docker-build:
	@echo "Building Docker image for current platform..."
	@docker build -t octopus-monitor:latest .

# Docker build for multiple platforms (builds only, stores in cache)
docker-buildx:
	@echo "Building multi-platform Docker images (cached, not loaded)..."
	@docker buildx create --name multiplatform --use || true
	@docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 \
		-t octopus-monitor:latest \
		--cache-from=type=local,src=/tmp/.buildx-cache \
		--cache-to=type=local,dest=/tmp/.buildx-cache \
		.
	@echo "Multi-platform images built and cached successfully!"
	@echo "Note: Images are not loaded into Docker (use docker-buildx-local for that)"

# Docker build for current platform only (can be loaded locally)
docker-buildx-local:
	@echo "Building Docker image for current platform..."
	@docker buildx create --name multiplatform --use || true
	@docker buildx build --platform linux/amd64 \
		-t octopus-monitor:latest \
		--load .
	@echo "Docker image loaded successfully!"

# Docker build and push to registry (multi-platform)
docker-buildx-push:
	@echo "Building and pushing multi-platform Docker images..."
	@docker buildx create --name multiplatform --use || true
	@docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 \
		-t octopus-monitor:latest \
		--push .

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
	@echo "  get-api-key   - Help get your Octopus Energy API key"
	@echo "  test-slack    - Test Slack webhook connection"
	@echo "  test-influx   - Test InfluxDB connection"
	@echo "  verify-config - Verify all configuration settings"
	@echo ""
	@echo "Build & Run:"
	@echo "  build              - Build the application for current platform"
	@echo "  build-prod         - Build production binary (static, Linux AMD64)"
	@echo "  build-all          - Build for all platforms (Linux, macOS, Windows)"
	@echo "  build-linux-amd64  - Build for Linux x86_64"
	@echo "  build-linux-arm64  - Build for Linux ARM64 (ARMv8)"
	@echo "  build-linux-armv7  - Build for Linux ARMv7 (Raspberry Pi)"
	@echo "  build-darwin-amd64 - Build for macOS Intel"
	@echo "  build-darwin-arm64 - Build for macOS Apple Silicon"
	@echo "  build-windows-amd64- Build for Windows x86_64"
	@echo "  run                - Run the application"
	@echo "  install            - Install to /usr/local/bin"
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
	@echo "  docker-build       - Build Docker image for current platform"
	@echo "  docker-buildx      - Build multi-platform images (AMD64, ARM64, ARMv7)"
	@echo "  docker-buildx-push - Build and push multi-platform images to registry"
	@echo "  docker-run         - Run Docker container"
	@echo ""
	@echo "Maintenance:"
	@echo "  clean         - Clean build artifacts"
	@echo "  help          - Show this help message"
