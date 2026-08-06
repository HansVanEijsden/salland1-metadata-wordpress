# Salland1 Metadata WordPress Service

[![CI](https://github.com/HansVanEijsden/salland1-metadata-wordpress/actions/workflows/ci.yml/badge.svg)](https://github.com/HansVanEijsden/salland1-metadata-wordpress/actions/workflows/ci.yml)
[![Security](https://github.com/HansVanEijsden/salland1-metadata-wordpress/actions/workflows/security.yml/badge.svg)](https://github.com/HansVanEijsden/salland1-metadata-wordpress/actions/workflows/security.yml)

A lightweight HTTP service that fetches, caches, and transforms metadata from the Salland1 WordPress API.


## Features

- Fetches data from WordPress API every minute with jitter
- Caches data in memory
- Exposes 8 endpoints for radio middleware
- Also fetches the current programme excerpt from the current show's route endpoint
- Robust error handling with graceful degradation
- Health check endpoint
- Comprehensive logging
- Optional cover-art resolver (`POST /cover-art`) with iTunes album art, special-song and show-avatar fallbacks
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
| `/radio-programme-excerpt` | Current programme excerpt (short description) | Plain text |
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

## Cover-art resolver

Optional album-art resolver (replaces the legacy PHP scripts on cloud.hansvaneijsden.nl), enabled with `COVERART_ENABLED=true`. The metadata hub POSTs each track to `POST /cover-art`; the resolver picks album art and pushes it back to the hub's `radio-cover-art` dynamic input, which the hub serves via `/ws/radio-cover-website`. `GET /cover-art/current` returns the last resolved URL (debugging / polling input).

Resolution order: **special song** (e.g. Sallandschijf, configurable) → **iTunes** (fuzzy match, ≥ 120s track) → **show avatar** (from the already-cached WordPress metadata) → **fallback image**.

Anti-rate-limit protection: per-track cache (6h), minimum 2s between iTunes calls, and a 5m cooldown after 403/429/5xx errors.

Relevant env vars (all prefixed `COVERART_`): `HUB_URL`, `HUB_INPUT`, `HUB_SECRET`, `FALLBACK_IMAGE`, `SPECIAL_TRIGGER`, `SPECIAL_URL`, `ITUNES_COUNTRY`, `ITUNES_LIMIT`, `CACHE_TTL`, `MIN_INTERVAL`, `ERROR_COOLDOWN`, `MIN_MUSIC_SECONDS`. See `AGENTS.md` for the full table.

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
