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
make e2e-ui         # real UI + real backend, needs Wayland
make e2e-audio      # isolated PipeWire playback/microphones + UI, needs Hyprland
make e2e-clipboard  # private nested Wayland clipboard test, needs Hyprland
make e2e-shelf      # private Wayland shelf test with synthetic files
make e2e-windows    # private Hyprland window-switching test
make e2e-apps       # private desktop-entry launch and real Apps page controls
make e2e-search     # unified app/window query and real keyboard launch/focus
make e2e-previews   # exact-window thumbnail pixels, UI, and lifecycle checks
make package        # build the Arch package into dist/ (see docs/development/packaging.md)
./build/omnibar     # run the backend
quickshell -p ui/qml/shell.qml  # run the UI in another terminal
```

### Committing
Use conventional commit prefixes: `feat:`, `fix:`, `test:`, `docs:`, `chore:`. Keep commits small and scoped to a single milestone task.
