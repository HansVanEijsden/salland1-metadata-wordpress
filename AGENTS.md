# AGENTS.md

A small Go HTTP service that polls a WordPress metadata API, caches the response in memory, and serves plain-text endpoints for radio middleware. Uses only the standard library (no third-party deps).

## Twin repository

This repo is the functional twin of **1zwolle-metadata-wordpress** — the two Go services are identical except for: module path, `SOURCE_URL` host, station branding strings in `internal/handlers`, compose `container_name`/IP, test route URLs, and the `COVERART_HUB_URL` example. Make each change **once**, then mirror it to the sibling adapting only those bits. Both repos are **public** on GitHub — never commit credentials, keep comments/log messages English-only, and don't add client-specific (hosting customer) names.

## Commands

```bash
go test ./...            # unit tests (also in CI: go test -v -race ./...)
go vet ./...             # static checks (CI enforces)
gofmt -w .               # CI fails on unformatted files
go build -v ./cmd/server
docker compose up -d     # runs with compose.yaml (uses external "metanet" network)
```

CI (`.github/workflows/ci.yml`) runs `go mod verify`, `go vet`, `gofmt` check, tests with `-race`, and build — keep these green. Dependabot minor/patch PRs and the golang base-image update bot auto-merge (squash) via `.github/workflows/dependabot-automerge.yml`; majors and non-dependency changes wait for human review.

## Architecture

Standard library only. Module path: `salland1-metadata-wordpress`.

- `cmd/server/main.go` — entrypoint; wires config, logger, cache, fetcher, handlers; graceful shutdown on SIGINT/SIGTERM.
- `internal/config` — env-driven config via `config.Load()` (see env table in `README.md`).
- `internal/fetcher` — background goroutine fetching the WordPress API on an interval with random jitter; tracks `HasSuccessfulFetch()`; also follows the current show's `route` to fetch and cache the programme excerpt.
- `internal/cache` — thread-safe in-memory cache (mutex-protected).
- `internal/parser` — `Parse(data interface{})` safely navigates the raw JSON (`broadcast.current_show`, `broadcast.next_show`) into `ParsedData` with type-assertion-based defaulting (handles JSON numbers as float64/int).
- `internal/handlers` — 8 plain-text endpoints + `/health` (see endpoint table in `README.md`).
- `internal/coverart` — optional album-art resolver (`POST /cover-art`); replaces the legacy PHP cover-art scripts. Off unless `COVERART_ENABLED=true`.
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

## Cover-art resolver

Optional album-art resolver that replaces the legacy PHP scripts on cloud.hansvaneijsden.nl.
Disabled unless `COVERART_ENABLED=true`, so the metadata endpoints are unaffected during rollout.

Flow: the metadata hub POSTs each track change to `/cover-art` (its `radio-cover-art-req` output) →
the resolver picks album art → pushes the result back to the hub's `radio-cover-art` dynamic input →
the hub serves it via `/ws/radio-cover-website`.

Resolution order:
1. Special song (e.g. Sallandschijf) when `COVERART_SPECIAL_TRIGGER` + `COVERART_SPECIAL_URL` are set and the title/text starts with the trigger.
2. iTunes album art when the track has an artist and a duration ≥ `COVERART_MIN_MUSIC_SECONDS` (default 120).
3. Current show avatar from the already-cached WordPress metadata (no extra HTTP call).
4. `COVERART_FALLBACK_IMAGE` placeholder.

Anti-rate-limit hardening (the legacy PHP was blacklisted by iTunes):
- Per-track cache keyed by normalized artist+title (`COVERART_CACHE_TTL`, default 6h).
- Minimum interval between iTunes calls (`COVERART_MIN_INTERVAL`, default 2s).
- Error cooldown after 403/429/5xx (`COVERART_ERROR_COOLDOWN`, default 5m).
- Single-flight serialization of iTunes calls.

### Cover-art environment variables

| Variable | Default | Description |
|---|---|---|
| `COVERART_ENABLED` | `false` | Master switch |
| `COVERART_HUB_URL` | — | Hub base URL to push results to (e.g. `http://172.21.0.66:9000`) |
| `COVERART_HUB_INPUT` | `radio-cover-art` | Hub dynamic input receiving the resolved URL |
| `COVERART_HUB_SECRET` | — | Secret of that hub input (same value as in the hub's config.json) |
| `COVERART_FALLBACK_IMAGE` | — | Station placeholder image |
| `COVERART_WP_URL` | `SOURCE_URL` | WordPress metadata API for the show-avatar fallback |
| `COVERART_ITUNES_COUNTRY` | `nl` | iTunes store country |
| `COVERART_ITUNES_LIMIT` | `5` | Search results to score |
| `COVERART_SPECIAL_TRIGGER` | — | Special-song title/text prefix (empty = disabled) |
| `COVERART_SPECIAL_URL` | — | Special-song API returning `image_url` |
| `COVERART_CACHE_TTL` | `6h` | Per-track cache lifetime |
| `COVERART_MIN_INTERVAL` | `2s` | Min spacing between iTunes calls |
| `COVERART_ERROR_COOLDOWN` | `5m` | iTunes error/rate-limit backoff |
| `COVERART_MIN_MUSIC_SECONDS` | `120` | Minimum track length to treat as music |

## Docs

See `README.md` for endpoints, env vars, and deployment details — link to it rather than duplicating.
