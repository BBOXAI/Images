# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WebP Image Proxy Service - A Go-based image conversion and caching proxy that fetches remote images, converts them to WebP format, and serves them with comprehensive statistics and management features.

## Key Technologies

- Go 1.24.4
- Pure Go implementation (no CGO dependencies)
- SQLite for metadata storage
- WebP encoding via github.com/HugoSmits86/nativewebp

## Architecture

The entire application is in `main.go` (1,163 lines) with the following components:

1. **Image Processing Pipeline**: Downloads → Format Detection → WebP Conversion (except animated GIFs) → Cache Storage
2. **Cache System**: File-based cache with SQLite metadata, MD5-based keys, 10-minute validity
3. **Web Interface**: Statistics dashboard, cache management UI with thumbnails
4. **API Endpoints**: `/` (proxy), `/stats` (JSON), `/cache` (management), `/thumb/` (thumbnails)

## Development Commands

```bash
# Build the application
go build -o webpimg main.go

# Cross-compile for production
GOOS=linux GOARCH=amd64 go build -o webpimg-linux-amd64 main.go
GOOS=windows GOARCH=amd64 go build -o webpimg-windows-amd64.exe main.go

# Run locally
./webpimg

# Update dependencies
go mod tidy

# Build for release (Windows)
build.bat
```

## Testing

Currently no tests exist. When adding tests:
```bash
# Run tests (when implemented)
go test ./...

# Run with coverage (when implemented)
go test -cover ./...
```

## Important Implementation Details

1. **Port**: Hard-coded to 8080 in main.go:1160
2. **Cache Directory**: `cache/` created automatically
3. **Database**: `imgproxy.db` SQLite file
4. **Cache Validity**: 10 minutes (main.go:89)
5. **Max Cache Size**: 100MB limit enforced

## Code Conventions

- All logic in single `main.go` file
- Error handling uses `log.Printf` for logging
- HTTP handlers follow pattern: parse request → check cache → process → respond
- Database operations use prepared statements
- Image processing uses standard Go image libraries

## CI/CD Pipeline

GitHub Actions workflow (`.github/workflows/build.yml`):
- Triggers on version tags (`v*`)
- Cross-compiles for Windows and Linux AMD64
- Creates GitHub releases with binaries

## When Making Changes

1. **Adding Features**: Update the relevant handler functions in main.go
2. **Modifying Cache Logic**: Check `getCachedImage()` and `saveToCache()` functions
3. **Changing Web UI**: HTML templates are embedded in handler functions
4. **Database Schema Changes**: Update `initDB()` function and migration logic
5. **Performance Improvements**: Focus on image processing pipeline and concurrent handling

## Areas Needing Attention

- No automated tests exist - consider adding when modifying core functionality
- Configuration is hard-coded - consider making configurable via environment variables or config file
- No request rate limiting or authentication mechanisms
- Cache cleanup runs on every request - could be optimized with background worker