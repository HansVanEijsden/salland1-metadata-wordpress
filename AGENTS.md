# AGENTS.md

A small Go HTTP service that polls a WordPress metadata API, caches the response in memory, and serves plain-text endpoints for radio middleware. Uses only the standard library (no third-party deps).

## Commands

```bash
go test ./...            # unit tests (also in CI: go test -v -race ./...)
go vet ./...             # static checks (CI enforces)
gofmt -w .               # CI fails on unformatted files
go build -v ./cmd/server
docker compose up -d     # runs with compose.yaml (uses external "metanet" network)
```

CI (`.github/workflows/ci.yml`) runs `go mod verify`, `go vet`, `gofmt` check, tests with `-race`, and build — keep these green.

## Architecture

Standard library only. Module path: `salland1-metadata-wordpress`.

- `cmd/server/main.go` — entrypoint; wires config, logger, cache, fetcher, handlers; graceful shutdown on SIGINT/SIGTERM.
- `internal/config` — env-driven config via `config.Load()` (see env table in `README.md`).
- `internal/fetcher` — background goroutine fetching the WordPress API on an interval with random jitter; tracks `HasSuccessfulFetch()`.
- `internal/cache` — thread-safe in-memory cache (mutex-protected).
- `internal/parser` — `Parse(data interface{})` safely navigates the raw JSON (`broadcast.current_show`, `broadcast.next_show`) into `ParsedData` with type-assertion-based defaulting (handles JSON numbers as float64/int).
- `internal/handlers` — 7 plain-text endpoints + `/health` (see endpoint table in `README.md`).
- `internal/logger` — `slog` JSON logging to stdout; `HTTPLogger` middleware.
- `pkg/utils` — shared formatting helpers (`FormatHosts`, `FormatTime`).
- `tests/parser_test.go` — table-driven tests for the parser and utils.

## Conventions & pitfalls

- **No external dependencies** — keep new code on the standard library unless clearly justified.
- **Log via `slog`** (JSON), not `fmt.Println`. Use `slog.Debug` for jitter/fetch details, `slog.Info` for lifecycle events, `slog.Error` for failures.
- **Environment variables** for any new configuration — add them to `config.Load()` with defaults.
- **Graceful degradation** is a core requirement: fetcher/parser must never panic on malformed upstream JSON or missing fields; handlers return `503` when no data, `204` when show name is empty.
- **Thread safety**: anything shared with the fetch goroutine (cache, fetcher state) must be mutex-protected or otherwise safe.
- **Stable HTTP contract**: endpoint paths and plain-text formats are consumed by external radio middleware — don't change response formats casually.
- **JSON number handling**: Go's `encoding/json` unmarshals numbers as `float64`; parser explicitly handles int/float64 cases — keep this in mind when adding fields.
- Go version is 1.26 (`go.mod`); CI pins `go-version: '1.26'`.

## Docs

See `README.md` for endpoints, env vars, and deployment details — link to it rather than duplicating.
