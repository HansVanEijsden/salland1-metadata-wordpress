---
description: "Mirror a change to the sibling metadata-wordpress twin repo, adapting only the per-repo bits (module path, SOURCE_URL, branding, compose IP, test URLs)"
name: "Mirror to Twin Repo"
argument-hint: "Describe the change to port, or leave blank to use the current uncommitted diff"
agent: "agent"
---
Port a change from this repo to its sibling twin so the two Go services stay functionally identical.

- This repo: `salland1-metadata-wordpress` (local clone `/Users/hans/Documents/Docker/salland1-metadata-wordpress`)
- Sibling:  `1zwolle-metadata-wordpress` (local clone `/Users/hans/Documents/Docker/1zwolle-metadata-wordpress`)

Use the current uncommitted diff (or the change described by the user). Apply it to the sibling, adapting **only** these per-repo bits — everything else must be byte-identical:

1. Module path / import prefixes (`go.mod` module name, `internal/**` import paths)
2. `SOURCE_URL` default host in `internal/config/config.go`
3. Station branding strings in `internal/handlers/handlers.go`
4. Compose `container_name` + metanet IP in `compose.yaml`
5. Test route URLs in `tests/parser_test.go`
6. `COVERART_HUB_URL` example in `AGENTS.md`
7. Startup log line in `cmd/server/main.go`

Procedure:
1. Apply the change here, then port it to the sibling adapting only the bits above.
2. In BOTH repos run: `gofmt -w .`, `go vet ./...`, `go test -race ./...`, `go build -v ./cmd/server`.
3. Diff the touched files across the twins — only the 7 bits above may differ.
4. Commit one logical change per repo and push both to `main` (working trees clean first).

Rules (both repos are public):
- Never commit credentials/secrets; comments and log messages English-only; no client (hosting customer) names.
- Don't change endpoint paths or plain-text formats — external radio middleware depends on them.
- `slog` JSON logging (not `fmt.Println`); env-driven config with defaults; graceful degradation (503 when no data, 204 when show name empty); mutex-protect anything shared with the fetch goroutine.
