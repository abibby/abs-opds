# Repository Guidelines

## Project Structure & Module Organization

This Go service exposes Audiobookshelf EPUBs as an OPDS 1.x Atom catalog.
- `main.go` loads configuration and starts the HTTP server.
- `abs/` contains the Audiobookshelf HTTP client, API models, and EPUB discovery logic.
- `opds/` contains routing, XML feed models, catalog/search handlers, and download/cover streaming.
- Tests live beside source files as `*_test.go`; fixtures are embedded in Go tests. There is no separate frontend or asset directory.
- `Dockerfile` packages the service; `compose.yaml` also starts opds-proxy.

## Build, Test, and Development Commands

Use the Go version declared in `go.mod` (currently `1.27.0`). Run commands from the repository root:

- `go build ./...` — compile all packages.
- `go run .` — serve the catalog at `http://localhost:12665/opds` with default settings.
- `go test -race ./...` — run all tests with race detection.
- `go test ./opds -run TestAPIPageRequests` — run a focused test.
- `go vet ./...` — check for suspicious Go constructs.
- `docker compose up -d --build` — build and start the stack; opds-proxy defaults to port 8080.

## Coding Style & Naming Conventions

Format changed Go files with `gofmt -w`. Use standard Go indentation (tabs), lowercase package names, `PascalCase` exported identifiers, and `camelCase` internal identifiers. Follow the existing separation between upstream API handling in `abs` and HTTP/feed behavior in `opds`. Propagate request contexts, close response bodies, and return errors with useful context without exposing credentials.

## Testing Guidelines

Use Go's `testing` package and `net/http/httptest` mock servers; tests should not require a live Audiobookshelf instance. Name tests `TestBehavior` and use table-driven subtests for related cases. Add regression coverage for changed behavior, especially library isolation, EPUB eligibility, pagination, XML escaping, and streaming headers. No numerical coverage threshold is configured. Run the race-enabled suite and `go vet` before submitting code changes.

## Commit & Pull Request Guidelines

Existing history contains only short subjects (`first commit`, `mvp`); no formal commit convention is established. Use concise, imperative subjects describing the change. Pull requests should explain the problem, resulting behavior, and validation performed; link relevant issues. Update `README.md` and configuration examples when endpoints or settings change.

## Security & Configuration

Copy `.env.example` to `.env` and configure `ABS_URL`, `ABS_API_KEY`, and `ABS_LIBRARY_ID`. Never commit real credentials. Keep API keys server-side and preserve the configured library boundary for feeds, downloads, and covers.
