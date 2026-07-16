# Salland1 Metadata WordPress Service

[![CI](https://github.com/HansVanEijsden/salland1-metadata-wordpress/actions/workflows/ci.yml/badge.svg)](https://github.com/HansVanEijsden/salland1-metadata-wordpress/actions/workflows/ci.yml)
[![Security](https://github.com/HansVanEijsden/salland1-metadata-wordpress/actions/workflows/security.yml/badge.svg)](https://github.com/HansVanEijsden/salland1-metadata-wordpress/actions/workflows/security.yml)

A lightweight HTTP service that fetches, caches, and transforms metadata from the Salland1 WordPress API.


## Features

- Fetches data from WordPress API every minute with jitter
- Caches data in memory
- Exposes 7 endpoints for radio middleware
- Robust error handling with graceful degradation
- Health check endpoint
- Comprehensive logging
- Docker ready

## Architecture

The service is written in Go for its:

- Low memory footprint
- Fast startup time
- Excellent concurrency support
- Built-in HTTP server
- Strong standard library

## Endpoints

| Endpoint | Description | Format |
|----------|-------------|--------|
| `/radio-fm-pty` | FM RDS PTY code | Plain text |
| `/radio-fm-ptyn` | FM RDS PTYN string | Plain text |
| `/radio-fm-programme` | FM programme announcement | Plain text |
| `/radio-stream-programme` | Stream programme information | Plain text |
| `/radio-dab-programme` | DAB programme information | Plain text |
| `/radio-tv-programme` | TV programme name | Plain text |
| `/radio-tv-host` | TV host information | Plain text |
| `/health` | Health check | JSON |

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SOURCE_URL` | `https://www.salland1.nl/wp-json/metadata/v1/current` | WordPress API URL |
| `FETCH_INTERVAL` | `60s` | Fetch interval |
| `JITTER` | `10s` | Random jitter for fetch timing |
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Log level (debug/info/warn/error) |

## Building and Running

### Using Docker Compose

```bash
docker-compose up -d
```

## Manual Build

```bash
go build -o server ./cmd/server
./server
```

## Testing

Run unit tests:

```bash
go test ./...
```

## Health Check

The health check endpoint (/health) returns:

- 200 OK if the service has data and the HTTP server is running
- 503 Service Unavailable if no data has been fetched yet

## Logging

All requests and system events are logged in JSON format to stdout. Docker Compose is configured to keep logs with rotation.

## Dependencies

- Go 1.21+
- Docker
- Docker Compose

## License

Proprietary - Salland1 Radio

This complete solution provides:

1. **Clean Architecture**: Separate packages for configuration, fetching, parsing, caching, HTTP handlers, and logging
2. **Robust Error Handling**: Graceful degradation when upstream API fails
3. **Thread-Safe**: Concurrent reads and writes to the cache with mutex protection
4. **Health Checks**: Proper health endpoint that validates both server and data availability
5. **Comprehensive Logging**: JSON-structured logs for all operations
6. **Unit Tests**: Tests for host formatting, time formatting, missing fields, and malformed JSON
7. **Docker Ready**: Complete Dockerfile and docker-compose configuration
8. **Configuration**: Environment variable based configuration
9. **Low Resource Usage**: Go's minimal memory footprint and fast startup

The service will run on the specified network with the fixed IP address and can be accessed from other containers on the same network.
