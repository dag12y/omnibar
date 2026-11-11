# Development Guidelines

## Development principles

1. Understand before implementing.
2. Research official documentation first.
3. Keep modules independent.
4. Prefer simple and maintainable solutions.
5. Write tests alongside implementation.
6. Document architectural decisions.
7. Avoid unnecessary dependencies.
8. Review AI-generated code.

## Git conventions

- feat: new functionality
- fix: bug fixes
- docs: documentation
- refactor: code restructuring
- test: testing
- chore: maintenance

## Workflow

1. Create an issue or milestone.
2. Create a feature branch from `main`.
3. Implement a small, testable change for the current milestone.
4. Run `go build ./...` and `go test ./...`.
5. Verify manually (backend + UI connection) where applicable.
6. Commit with a descriptive message (`feat:`, `fix:`, `test:`, `docs:`, `chore:`).
7. Merge after review.

## Local development workflow

### Building
```sh
make build
```

### Running tests
```sh
make test
# or
go test ./...
```

### Running the backend (development)
```sh
make run
# or
go run ./cmd/omnibar
```

The backend reads config from `$XDG_CONFIG_HOME/omnibar/config.toml`, creates the socket at `$XDG_RUNTIME_DIR/omnibar/omnibar.sock`, and logs via structured `slog`.

### Running the UI (development)
```sh
quickshell -p ui/qml/shell.qml
```

The UI connects to the backend socket using `Quickshell.Io.Socket`. It can
start before the backend and reconnects automatically with exponential
backoff (1s, 2s, 4s, 8s, 16s, then every 30s). Quickshell 0.3.2 is the pinned UI
framework version.

### Testing QML logic
```sh
make ui-test
```

This runs the Qt Test suite under `ui/qml/tests/` with the Qt 6 test runner
in offscreen mode.

### Backend and UI separation
- The Go backend is a standalone daemon; it stays alive when the UI closes.
- The UI is launched separately via `quickshell`.
- If the backend restarts, the UI reconnects with exponential backoff.
- For development, run both in separate terminals.

### Committing
Use conventional commit prefixes: `feat:`, `fix:`, `test:`, `docs:`, `chore:`. Keep commits small and scoped to a single milestone task.
