# GitHub Copilot Instructions

Go service: polls a WordPress metadata API, caches in memory, serves plain-text radio-middleware endpoints. Standard library only — no third-party deps.

## How to work in this repo

- **Go commands**: verify changes with `go build -v ./cmd/server`, `go vet ./...`, `gofmt -w .`, and `go test ./...`. Keep the CI workflow (`.github/workflows/ci.yml`) green: `go mod verify`, vet, gofmt, tests with `-race`, build.
- **Twin repo & public**: this service is the functional twin of `1zwolle-metadata-wordpress` — mirror each change there, adapting only module path, `SOURCE_URL`, station branding in `internal/handlers`, compose IP/container_name, and test route URLs. Both repos are **public**: never commit credentials; keep comments and log messages English-only; no client-specific (hosting customer) names.
- **No external dependencies** — stay on the standard library unless clearly justified.
- **Log via `slog`** JSON handlers, not `fmt.Println`. Use `slog.Debug` for fetch/jitter detail, `slog.Info` for lifecycle, `slog.Error` for failures.
- **Configuration** goes through `internal/config.Load()` env vars with defaults — don't hardcode values.
- **Graceful degradation is required**: never panic on malformed JSON or missing fields; handlers return `503` when no cached data and `204` when the show name is empty.
- **Thread safety**: anything shared with the fetch goroutine (cache, fetcher state) must be mutex-protected.
- **Stable HTTP contract**: endpoint paths and plain-text formats are consumed by external middleware — don't change them casually.
- **JSON numbers** parse as `float64` in Go; `internal/parser` handles int/float64 explicitly — keep that pattern when adding fields.

## Editing patterns

- Keep package boundaries in `internal/` (config, fetcher, cache, parser, handlers, logger); shared helpers live in `pkg/utils`.
- New tests: table-driven, placed in `tests/` (see `tests/parser_test.go`); cover-art tests are co-located in `internal/coverart/`.

## References

- Full architecture and conventions: `AGENTS.md`
- Endpoints, env vars, deployment: `README.md`
