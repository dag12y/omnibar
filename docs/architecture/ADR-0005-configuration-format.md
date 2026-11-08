# ADR-0005: Configuration Format — TOML

Status: Accepted

## Context

Omnibar needs a user-editable configuration file for module settings, permissions, and core options. The original architecture proposal suggested JSON. JSON lacks comments and is less friendly for hand-editing. We must choose a configuration format.

## Decision

Use **TOML** for Omnibar configuration.

## Alternatives considered

### TOML (chosen)

**Advantages:**
- Human-editable with native comment support (`#`)
- Well-suited to nested sections (modules, permissions)
- Widely used in Linux desktop and system configuration
- Go libraries available (`BurntSushi/toml`, `pelletier/go-toml`)
- Hyprland-adjacent ecosystem uses TOML-like config styles

**Disadvantages:**
- Not in Go standard library (requires a dependency)
- Slightly less universally known than JSON

### JSON

**Advantages:**
- Go standard library (`encoding/json`)
- Universal familiarity

**Disadvantages:**
- No comments
- Verbose for hand-editing
- Stricter syntax (trailing commas not allowed)

**Decision:** Rejected. User-editable desktop config benefits from comments and a lighter syntax.

### YAML

**Advantages:**
- Human-friendly, comments supported
- Popular in configuration

**Disadvantages:**
- Complex specification with surprising edge cases (e.g., `yes`/`no` boolean parsing)
- Indentation-sensitive
- More complex Go parsing dependencies

**Decision:** Rejected. TOML is simpler and more predictable for this use case.

## Configuration structure

Configuration lives at `$XDG_CONFIG_HOME/omnibar/config.toml`.

Permission keys in `[permissions]` are **quoted dotted strings** because TOML bare keys cannot contain dots:

```toml
config_version = 1

[core]
log_level = "info"

[modules.system_monitor]
enabled = true
interval_seconds = 5

[modules.audio]
enabled = true

[permissions]
"system.info" = true
"window.manage" = false
"clipboard.read" = false
"clipboard.write" = false
"audio.control" = true
```

### Declared vs granted permissions

Two distinct concepts must not be confused:

- **Declared permissions:** What a module says it needs to function. Defined in Go code as part of the module's `ModuleInfo` (static registration).
- **Granted permissions:** What the user has allowed. Defined in `config.toml` under `[permissions]`.

A module can only use a capability if it both declares the permission **and** the user has granted it.

### Fail-closed permission evaluation

Missing, malformed, or invalid permission grants **fail closed** — they must never accidentally grant access:

- A missing `[permissions]` table means no permissions are granted.
- A permission key that is malformed (not a string, wrong type) is ignored and treated as not granted.
- A permission granted to a module that does not declare it is ignored (granting does not imply authorization).
- Unknown permission names produce warnings, not grants.

### Validation

- Configuration is parsed with a TOML library, then validated against Omnibar's own expectations.
- Configuration validation is **application-defined** — TOML itself has no built-in schema feature. Omnibar parses TOML and then validates the parsed structure against its own expectations (module names, permission names, value types).
- Invalid sections fall back to defaults without discarding the entire configuration.
- Unknown modules produce warnings, not failures.
- `config_version` field supports additive, backward-compatible migrations.

## Consequences

**Positive:**
- Comments make the config self-documenting
- Nested tables map cleanly to module/permission structure
- Quoted dotted keys avoid TOML bare-key restrictions for permission names
- Fail-closed semantics prevent accidental privilege escalation through misconfiguration

**Negative:**
- Requires a third-party Go dependency (mitigated: well-maintained, small API surface)
- Permission evaluation logic must be carefully implemented to maintain fail-closed guarantees

## References

- TOML specification: https://toml.io/
- `BurntSushi/toml`: https://github.com/BurntSushi/toml
- `pelletier/go-toml`: https://github.com/pelletier/go-toml
