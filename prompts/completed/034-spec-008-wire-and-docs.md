---
status: completed
spec: [008-ensure-plugins-installer]
summary: Wired plugins.NewInstaller into agent/pr-reviewer/main.go before factory.CreateDeliverer, created docs/claude-plugin-cli.md, and updated CHANGELOG.md with Unreleased section.
container: code-reviewer-034-spec-008-wire-and-docs
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-27T20:00:00Z"
queued: "2026-04-27T19:52:38Z"
started: "2026-04-27T19:58:08Z"
completed: "2026-04-27T20:03:13Z"
branch: dark-factory/ensure-plugins-installer
---

<summary>
- The plugin installer from prompt 1 is wired into `agent/pr-reviewer/main.go` and runs before `agent.Run`.
- On first pod boot, the installer runs `claude plugin marketplace add bborbe/coding` then `claude plugin install coding`.
- On subsequent boots (PVC-cached), the installer runs `claude plugin marketplace update coding` (soft) then `claude plugin update coding@coding` (soft).
- If plugin installation fails, startup aborts — no tasks are handled until plugins are ready.
- `docs/claude-plugin-cli.md` is created documenting the durable CLI command derivation rules and list-matching logic so the knowledge survives after this spec is archived.
- `CHANGELOG.md` is updated with a `feat:` entry under `## Unreleased`.
- All existing tests still pass; `make precommit` exits 0.
</summary>

<objective>
Wire the `plugins.Installer` into `agent/pr-reviewer/main.go` so it runs before `agent.Run`, and document the `claude plugin` CLI conventions in `docs/claude-plugin-cli.md`. This prompt assumes prompt `1-spec-008-plugins-package` has already been applied and `agent/pr-reviewer/pkg/plugins/` exists.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read the following guides from `~/.claude/plugins/marketplaces/coding/docs/`:
- `changelog-guide.md` — `## Unreleased` format and conventional-commit prefixes

Read these files fully before making changes:
- `agent/pr-reviewer/main.go` — current `Run` method and `application` struct
- `agent/pr-reviewer/pkg/plugins/plugins.go` — `Spec`, `Installer`, `Commander` interfaces and constructors (`NewInstaller`, `NewExecCommander`)

**Precondition check:** Before making any changes, verify the plugins package exists:
```bash
ls agent/pr-reviewer/pkg/plugins/plugins.go
```
If missing, stop and report `status: failed` with message "pkg/plugins package not found — apply prompt 1-spec-008-plugins-package first".

**`claude plugin` CLI derivation rules (inline for this prompt — also written to docs/):**

| Operation | Command | Notes |
|-----------|---------|-------|
| List installed | `claude plugin list` | Exit 0 + empty stdout = no plugins |
| Add marketplace | `claude plugin marketplace add <marketplace>` | `marketplace` = full slug e.g. `bborbe/coding` |
| Install plugin | `claude plugin install <name>` | `name` = plain name e.g. `coding` |
| Refresh marketplace | `claude plugin marketplace update <alias>` | `alias` = last segment of marketplace slug |
| Update plugin | `claude plugin update <name>@<alias>` | e.g. `coding@coding` |

Detection: substring match of `Name` against lines of `claude plugin list` stdout.
</context>

<requirements>
1. **Verify precondition** — check `agent/pr-reviewer/pkg/plugins/plugins.go` exists. If missing, stop with `status: failed`.

2. **Wire installer into `agent/pr-reviewer/main.go`**:

   a. Add import for the plugins package:
      ```go
      "github.com/bborbe/maintainer/agent/pr-reviewer/pkg/plugins"
      ```

   b. At the top of the `Run(ctx context.Context, _ libsentry.Client) error` method, before the `deliverer` creation, add:
      ```go
      installer := plugins.NewInstaller(plugins.NewExecCommander())
      if err := installer.EnsureInstalled(ctx, []plugins.Spec{
          {Marketplace: "bborbe/coding", Name: "coding"},
      }); err != nil {
          return errors.Wrap(ctx, err, "ensure plugins installed")
      }
      ```

   The complete Run method order must be:
   1. `glog.V(2).Infof(...)` (existing)
   2. `installer.EnsureInstalled(...)` (new)
   3. `factory.CreateDeliverer(...)` (existing)
   4. `factory.CreateAgent(...)` (existing)
   5. `agent.Run(...)` (existing)
   6. `agentlib.PrintResult(result)` (existing)

3. **Run `cd agent/pr-reviewer && make test`** — all tests must pass. The new wiring in main.go is exercised via `main_test.go` if one exists; otherwise, compilation is the check.

4. **Create `docs/claude-plugin-cli.md`** in the repo root `docs/` directory:

   ```markdown
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
   ```

5. **Update `CHANGELOG.md`** at repo root:
   - Ensure `## Unreleased` section exists at the top (create above the first `## vX.Y.Z` if missing).
   - Add these bullets under `## Unreleased` (in this order, append to any existing bullets):
     ```
     - feat: add plugin installer library (`pkg/plugins/`) ensuring Claude Code plugins are installed before task handling
     - feat: wire plugin installer into agent-pr-reviewer startup — ensures `bborbe/coding` plugin is present on every pod boot
     - docs: add `docs/claude-plugin-cli.md` documenting claude plugin CLI derivation rules
     ```
   - Do not alter existing version sections or reorder existing bullets.

6. **Run `cd agent/pr-reviewer && make precommit`** — must exit 0.
</requirements>

<constraints>
- Insert the `EnsureInstalled` call BEFORE `factory.CreateDeliverer` — plugins must be ready before any task handling begins
- Do NOT modify `pkg/plugins/` or any test files — this prompt only touches `main.go`, `docs/claude-plugin-cli.md`, and `CHANGELOG.md`
- Do NOT run `go mod vendor` — vendor is regenerated by `make buca`, never committed
- Do NOT commit — dark-factory handles git
- All existing tests must still pass
- `make precommit` must exit 0
- If the precondition check fails (plugins package missing), stop immediately with `status: failed`
</constraints>

<verification>
```bash
# Precondition: plugins package exists
ls agent/pr-reviewer/pkg/plugins/plugins.go

# EnsureInstalled call present in main.go
grep -n "EnsureInstalled" agent/pr-reviewer/main.go
# Expect: one match

# bborbe/coding plugin spec is wired
grep -n 'bborbe/coding' agent/pr-reviewer/main.go
# Expect: one match

# docs file created
ls docs/claude-plugin-cli.md

# CHANGELOG updated
grep -n "plugin installer" CHANGELOG.md
# Expect: match under ## Unreleased

# Full check
cd agent/pr-reviewer && make precommit
```
</verification>
