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

See [docs/development/local-development.md](docs/development/local-development.md)
for the complete workflow: running the backend and UI separately, the manual
end-to-end checklist, integration tests, fail-closed permission verification,
and troubleshooting.

Quick reference:

```sh
make build          # build build/omnibar
make test           # Go unit + integration tests
make ui-test        # Qt Test QML logic tests
./build/omnibar     # run the backend
quickshell -p ui/qml/shell.qml  # run the UI in another terminal
```

### Committing
Use conventional commit prefixes: `feat:`, `fix:`, `test:`, `docs:`, `chore:`. Keep commits small and scoped to a single milestone task.
