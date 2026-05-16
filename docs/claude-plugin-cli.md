# Claude Plugin CLI Conventions

This document captures durable knowledge about the `claude plugin` CLI command
forms used by the plugin installer library (`agent/pr-reviewer/pkg/plugins/`).
These rules are derived from the spec-008 investigation and must be updated if
the `claude` CLI changes behaviour.

## Command Reference

| Operation | Command | Notes |
|-----------|---------|-------|
| List installed plugins | `claude plugin list` | Exit 0 + empty stdout = no plugins installed |
| Add a marketplace | `claude plugin marketplace add <marketplace>` | `<marketplace>` = full slug, e.g. `bborbe/coding` |
| Install a plugin | `claude plugin install <name>` | `<name>` = plain plugin name, e.g. `coding` |
| Refresh a marketplace | `claude plugin marketplace update <alias>` | `<alias>` = last path segment of marketplace slug |
| Update an installed plugin | `claude plugin update <name>@<alias>` | e.g. `coding@coding` |

## Alias Derivation

The marketplace alias used in `marketplace update` and `plugin update` commands
is the **last path segment** of the `Marketplace` field:

```
Marketplace = "bborbe/coding"  →  alias = "coding"
update form = "coding@coding"
```

Use `path.Base(spec.Marketplace)` (package `"path"`, not `"path/filepath"`).

## Plugin Detection

"Already installed" is determined by a **substring match** of `Spec.Name` against
each line of `claude plugin list` stdout. If the name appears in any line, the
plugin is treated as installed and the update path is taken.

`claude plugin list` exit codes:
- Exit 0, empty stdout → no plugins installed → take install path
- Exit 0, non-empty stdout → check lines for substring match
- Exit non-zero → hard error (cannot determine state)

## Hard vs Soft Failures

| Step | Failure type | Effect |
|------|-------------|--------|
| `claude plugin list` | Hard | Return wrapped error, abort startup |
| `claude plugin marketplace add` | Hard | Return wrapped error, abort startup |
| `claude plugin install` | Hard | Return wrapped error, abort startup |
| `claude plugin marketplace update` | Soft | Log warning (`glog.Warningf`), continue |
| `claude plugin update` | Soft | Log warning (`glog.Warningf`), continue |

## Security Note

Marketplace and plugin names are passed as **separate exec arguments** (not via
shell interpolation). This ensures a malformed spec cannot inject extra commands.
