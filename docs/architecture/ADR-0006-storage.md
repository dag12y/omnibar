# ADR-0006: Storage — SQLite (WAL) + TOML Config

Status: Accepted

## Context

Omnibar needs persistent storage for module state (e.g., clipboard history, monitoring history, per-module settings). The original architecture proposal suggested SQLite. We must decide how storage integrates with TOML configuration and what database driver to use, and when to introduce it.

## Decision

- **Persistent application data:** SQLite in WAL mode, stored at `$XDG_DATA_HOME/omnibar/omnibar.db` — introduced only when a feature requires it.
- **Configuration:** TOML at `$XDG_CONFIG_HOME/omnibar/config.toml` (not in SQLite).
- **Database driver:** `modernc.org/sqlite` (pure Go, no CGO) — evaluated as the selected driver, but **not introduced in Phase 1**.
- **Schema:** Created only for features that require persistent data. Phase 1 has no database tables (settings live in TOML).
- **Phase 1:** No database implementation, no driver import, no tables, no migrations.

## Alternatives considered

### SQLite + `modernc.org/sqlite` (chosen for future use)

**Advantages:**
- Zero server, single file, ACID transactions
- WAL mode allows concurrent reads without blocking
- Survives application upgrades
- Pure Go driver: no CGO, simpler cross-compilation and builds
- Well-supported in the Go ecosystem

**Disadvantages:**
- Single writer at a time (mitigated by WAL and the fact that the core is the sole writer)
- Larger binary than a pure in-memory store
- Introduces a dependency that is unnecessary in Phase 1

### SQLite + `mattn/go-sqlite3` (CGO)

**Advantages:**
- Mature, widely used, potentially faster

**Disadvantages:**
- Requires CGO and gcc at build time
- Complicates cross-compilation
- C dependency introduces build and security considerations

**Decision:** Rejected for Phase 1. Prefer pure Go for simplicity. Revisit if performance measurements show a need.

### TOML-only storage (all state in config file)

**Advantages:**
- No database dependency
- Simple

**Disadvantages:**
- Not suitable for append-only histories (clipboard, monitoring)
- No efficient querying
- Rewriting the whole config file on every change is impractical

**Decision:** Rejected. SQLite for persistent data; TOML for configuration.

### JSON file storage

**Advantages:**
- Go standard library
- Simple

**Disadvantages:**
- No concurrent access safety
- No efficient querying
- Rewrite-on-every-change

**Decision:** Rejected.

## Phase 1 storage

Phase 1 does **not** introduce SQLite. Settings live in TOML. No database file is created, no driver is imported, and no migrations exist. The database is introduced when the first feature actually requires persistent structured data (e.g., clipboard history in Phase 4, or monitoring history in Phase 2 if persistence is needed).

## Database design principles (for future implementation)

- One SQLite database file: `$XDG_DATA_HOME/omnibar/omnibar.db`
- Schema version tracked in a `meta` table
- Migrations are transactional and additive only
- No tables are created speculatively; tables are added when a feature needs them

### Anticipated tables (not created in Phase 1)

| Table | Purpose | Phase |
|-------|---------|-------|
| `module_settings` | Per-module persistent key/value | Future |
| `clipboard_history` | Clipboard entries | Phase 4 |
| `monitoring_history` | System metrics time series | Phase 2 |

## Data retention and deletion

- User-configurable retention per table (default: 30 days for history tables)
- "Clear all data" option deletes the database and configuration
- Uninstalling the package preserves user data; manual removal of XDG directories is required for complete data removal

## SQLite-safe backup

Copying only the main SQLite database file while WAL mode is active may omit committed data that exists only in the WAL file. A safe backup uses one of:

1. The **SQLite online backup API** (`sqlite3_backup_init` / `sqlite3_backup_step`), which produces a consistent snapshot of the database including WAL contents.
2. A **controlled checkpoint and shutdown**: run `PRAGMA wal_checkpoint(TRUNCATE)`, shut down all database connections cleanly, then copy the main database file.

## Consequences

**Positive:**
- Phase 1 remains dependency-light (no SQLite driver)
- Single file, zero-server database when introduced
- WAL mode for concurrent read performance
- Survives upgrades; migrations are straightforward

**Negative:**
- Deferring SQLite means persistent-data features cannot be built in Phase 1
- Database file must be backed up separately from config when introduced

## References

- SQLite documentation: https://www.sqlite.org/
- `modernc.org/sqlite`: https://github.com/modernc/sqlite
- SQLite online backup API: https://www.sqlite.org/backup.html
- WAL mode: https://www.sqlite.org/wal.html
