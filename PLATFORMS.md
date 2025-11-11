# Platform Support

Octopus Home Mini Monitor supports multiple CPU architectures and operating systems through Go's excellent cross-compilation capabilities.

## Supported Platforms

### Linux

| Architecture | GOARCH | Description | Example Hardware |
|-------------|--------|-------------|------------------|
| **x86_64 (AMD64)** | `amd64` | Standard Intel/AMD 64-bit | Servers, Desktop PCs, VPS |
| **ARM64 (ARMv8)** | `arm64` | Modern 64-bit ARM | Raspberry Pi 4/5, AWS Graviton, Oracle ARM |
| **ARMv7** | `arm` (v7) | 32-bit ARM | Raspberry Pi 2/3, older ARM devices |

### macOS

| Architecture | GOARCH | Description | Example Hardware |
|-------------|--------|-------------|------------------|
| **Intel (AMD64)** | `amd64` | Intel-based Macs | MacBook Pro (pre-2020), iMac |
| **Apple Silicon** | `arm64` | Apple M-series chips | MacBook Air M1/M2/M3, Mac Mini M1 |

### Windows

| Architecture | GOARCH | Description | Example Hardware |
|-------------|--------|-------------|------------------|
| **x86_64 (AMD64)** | `amd64` | Standard 64-bit Windows | Desktop PCs, Laptops |

## Building for Specific Platforms

### Quick Build Commands

```bash
# Build for all platforms at once
make build-all

# Build for specific platforms
make build-linux-amd64    # Linux x86_64
make build-linux-arm64    # Linux ARM64
make build-linux-armv7    # Linux ARMv7
make build-darwin-amd64   # macOS Intel
make build-darwin-arm64   # macOS Apple Silicon
make build-windows-amd64  # Windows x86_64
```

### Manual Cross-Compilation

If you prefer to compile manually:

```bash
# Linux AMD64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o octopus-monitor-linux-amd64 cmd/octopus-monitor/main.go

# Linux ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o octopus-monitor-linux-arm64 cmd/octopus-monitor/main.go

# Linux ARMv7
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o octopus-monitor-linux-armv7 cmd/octopus-monitor/main.go

# macOS Intel
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o octopus-monitor-darwin-amd64 cmd/octopus-monitor/main.go

# macOS Apple Silicon
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o octopus-monitor-darwin-arm64 cmd/octopus-monitor/main.go

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o octopus-monitor-windows-amd64.exe cmd/octopus-monitor/main.go
```

## Docker Multi-Platform Support

The project supports Docker multi-platform builds using Docker Buildx.

### Supported Docker Platforms

- `linux/amd64` - Standard x86_64 servers and cloud instances
- `linux/arm64` - ARM64 servers (AWS Graviton, Raspberry Pi 4/5)
- `linux/arm/v7` - ARMv7 devices (Raspberry Pi 2/3)

### Building Multi-Platform Docker Images

```bash
# Build for multiple platforms
make docker-buildx

# Build and push to a registry (replace with your registry)
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 \
  -t your-registry/octopus-monitor:latest --push .
```

## Platform-Specific Considerations

### Raspberry Pi

#### Raspberry Pi 4/5 (64-bit)
- Use `build-linux-arm64` or Docker `linux/arm64` image
- Recommended: Use 64-bit Raspberry Pi OS for better performance
- Memory: Minimum 1GB RAM recommended

#### Raspberry Pi 2/3 (32-bit)
- Use `build-linux-armv7` or Docker `linux/arm/v7` image
- Works with standard Raspberry Pi OS (32-bit)
- Memory: Minimum 512MB RAM

### AWS Graviton

AWS Graviton and Graviton2 instances use ARM64 architecture:

```bash
# Build for AWS Graviton
make build-linux-arm64

# Or use Docker multi-platform
docker pull octopus-monitor:latest  # Automatically uses arm64 image on Graviton
```

### Oracle Cloud ARM

Oracle Cloud's Ampere A1 instances use ARM64:

```bash
# Build for Oracle ARM
make build-linux-arm64
```

### Apple Silicon (M1/M2/M3)

For macOS with Apple Silicon:

```bash
# Native build
make build-darwin-arm64

# Or just use regular build on M1/M2/M3 Macs
make build
```

### Cross-Compilation Notes

Go's cross-compilation is excellent and doesn't require any platform-specific toolchains. You can build for ARM on an x86_64 machine and vice versa.

**Important:** All builds use `CGO_ENABLED=0` to create static binaries that don't require external C libraries. This makes the binaries fully portable.

## Binary Verification

After building, verify the binary is for the correct architecture:

```bash
# Linux
file dist/octopus-monitor-linux-amd64
# Output: ELF 64-bit LSB executable, x86-64, ...

file dist/octopus-monitor-linux-arm64
# Output: ELF 64-bit LSB executable, ARM aarch64, ...

file dist/octopus-monitor-linux-armv7
# Output: ELF 32-bit LSB executable, ARM, EABI5 version 1, ...

# macOS
file dist/octopus-monitor-darwin-arm64
# Output: Mach-O 64-bit executable arm64

# Windows (on Linux)
file dist/octopus-monitor-windows-amd64.exe
# Output: PE32+ executable (console) x86-64, ...
```

## Performance Characteristics

### Binary Sizes

Approximate binary sizes after stripping (`-ldflags '-w -s'`):

- **Linux AMD64**: ~5.6 MB
- **Linux ARM64**: ~5.5 MB
- **Linux ARMv7**: ~5.5 MB
- **macOS AMD64**: ~5.8 MB
- **macOS ARM64**: ~5.7 MB
- **Windows AMD64**: ~5.6 MB

All binaries are statically linked and include the Go runtime.

### Performance

- **x86_64**: Excellent performance on Intel/AMD processors
- **ARM64**: Near-native performance, excellent for Raspberry Pi 4/5 and cloud ARM instances
- **ARMv7**: Good performance for lighter workloads on older Raspberry Pi models

## Testing Multi-Platform Builds

Test that your builds work correctly:

```bash
# On the target platform, run:
./octopus-monitor-linux-amd64 --version  # Or appropriate binary name

# Test Docker multi-platform:
docker run --rm --platform linux/amd64 octopus-monitor:latest
docker run --rm --platform linux/arm64 octopus-monitor:latest
docker run --rm --platform linux/arm/v7 octopus-monitor:latest
```

## Continuous Integration

The GitHub Actions workflow automatically tests builds for multiple platforms. See [`.github/workflows/test.yml`](.github/workflows/test.yml) for the CI/CD configuration.

## Troubleshooting

### "exec format error" on Raspberry Pi

This means you're trying to run the wrong architecture binary. Ensure you're using:
- **ARM64** binary for Raspberry Pi 4/5 with 64-bit OS
- **ARMv7** binary for Raspberry Pi 2/3 or 32-bit OS

### Docker platform mismatch

If Docker pulls the wrong platform image, specify it explicitly:

```bash
docker run --platform linux/arm64 octopus-monitor:latest
```

### "cannot execute binary file"

Check the binary architecture matches your system:

```bash
uname -m  # Shows your system architecture
file ./octopus-monitor-*  # Shows binary architecture
```

Common architecture names:
- `x86_64` or `amd64` = AMD64/x86_64
- `aarch64` = ARM64
- `armv7l` = ARMv7

## Contributing

When adding dependencies, ensure they support cross-compilation and don't require CGO unless absolutely necessary. The project maintains `CGO_ENABLED=0` for maximum portability.
