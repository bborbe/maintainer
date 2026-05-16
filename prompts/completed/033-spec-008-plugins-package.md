---
status: completed
spec: [008-ensure-plugins-installer]
summary: Created agent/pr-reviewer/pkg/plugins/ package with Commander and Installer interfaces, ExecCommander implementation, counterfeiter mocks, and full Ginkgo test suite achieving 100% statement coverage
container: code-reviewer-033-spec-008-plugins-package
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-27T20:00:00Z"
queued: "2026-04-27T19:52:38Z"
started: "2026-04-27T19:53:09Z"
completed: "2026-04-27T19:58:06Z"
branch: dark-factory/ensure-plugins-installer
---

<summary>
- New Go package `agent/pr-reviewer/pkg/plugins/` provides a reusable Claude plugin installer library.
- `Spec` value type carries a marketplace slug (e.g. `bborbe/coding`) and a plugin name (e.g. `coding`).
- `Installer` interface exposes a single method `EnsureInstalled(ctx, []Spec) error` — callers never touch the `claude` CLI directly.
- `Commander` interface abstracts exec calls; `NewExecCommander()` provides the real implementation; tests use a Counterfeiter fake exclusively.
- Empty spec list is a no-op — no `claude` binary is invoked, no error returned.
- Install path: `claude plugin marketplace add <marketplace>` then `claude plugin install <name>`.
- Update path: `claude plugin marketplace update <alias>` (soft failure — log warning, continue) then `claude plugin update <name>@<alias>` (soft failure — log warning, continue).
- Hard failures (list command exits non-zero, marketplace add fails, install fails, context cancelled) return wrapped errors.
- All unit tests use the Counterfeiter fake; no real `claude` binary is invoked during `make test`.
- Coverage: empty input, install path, update path, list failure, install failure, update soft-failure, context cancellation.
</summary>

<objective>
Create the `agent/pr-reviewer/pkg/plugins/` package implementing the plugin installer library described in spec 008. The library must be fully unit-tested with a Counterfeiter fake for `Commander` and must not invoke the real `claude` CLI in any test.
</objective>

<context>
Read `CLAUDE.md` for project conventions (Pattern B Job, no vendor, module at `github.com/bborbe/maintainer/agent/pr-reviewer`).

Read the following guides from `~/.claude/plugins/marketplaces/coding/docs/`:
- `go-patterns.md` — Interface → Constructor → Struct pattern, counterfeiter annotations
- `go-factory-pattern.md` — `New*` constructor, zero business logic in factories
- `go-testing-guide.md` — Ginkgo v2/Gomega suite files, counterfeiter mocks, coverage ≥80%
- `go-error-wrapping-guide.md` — `bborbe/errors` wrapping, never `fmt.Errorf`

Read `agent/pr-reviewer/pkg/factory/factory.go` to understand how the package wires constructors.

**Key CLI command derivation rules (durable knowledge about the `claude` CLI):**

| Operation | Command form |
|-----------|-------------|
| List installed plugins | `claude plugin list` |
| Add a marketplace | `claude plugin marketplace add <marketplace>` (e.g. `bborbe/coding`) |
| Install a plugin | `claude plugin install <name>` (e.g. `coding`) |
| Refresh a marketplace | `claude plugin marketplace update <alias>` where `<alias>` = last path segment of `Marketplace` (e.g. `coding` from `bborbe/coding`) |
| Update a plugin | `claude plugin update <name>@<alias>` (e.g. `coding@coding`) |

"Already installed" detection: substring match of `Spec.Name` against each line of `claude plugin list` stdout. If the name appears as a substring of any line, the plugin is considered installed.

**`claude plugin list` exit codes:**
- Exit 0 with empty stdout → no plugins installed → install path
- Exit 0 with stdout lines → check each line for plugin name substring
- Exit non-zero → hard error (cannot determine state)

**Arguments must be passed as separate exec args, never via shell interpolation** (security: operator-supplied but still important to enforce).
</context>

<requirements>
1. Create `agent/pr-reviewer/pkg/plugins/plugins.go` with the following exported types and interfaces:

   ```go
   // Spec identifies a Claude Code plugin to ensure is installed.
   type Spec struct {
       Marketplace string // e.g. "bborbe/coding"
       Name        string // e.g. "coding"
   }

   // Commander runs an external command and returns its combined stdout.
   //counterfeiter:generate -o mocks/commander.go --fake-name Commander . Commander
   type Commander interface {
       Run(ctx context.Context, name string, args ...string) (string, error)
   }

   // Installer ensures a list of Claude plugins are installed or updated.
   //counterfeiter:generate -o mocks/installer.go --fake-name Installer . Installer
   type Installer interface {
       EnsureInstalled(ctx context.Context, specs []Spec) error
   }
   ```

   Note: generate both fakes (`Commander` and `Installer`) so callers of this package can also mock `Installer`.

2. Add constructor `NewExecCommander() Commander` in the same file. The implementation must:
   - Use `exec.CommandContext(ctx, name, args...)` — args are separate (no shell interpolation)
   - Return combined stdout (stderr is absorbed into the error message on failure)
   - Return a wrapped error on non-zero exit

3. Add constructor `NewInstaller(commander Commander) Installer` returning a private struct.

4. Implement `EnsureInstalled(ctx context.Context, specs []Spec) error` on the private struct:

   a. If `len(specs) == 0`, return `nil` immediately — no commands invoked.

   b. For each spec:
      - Derive `alias` = last path segment of `spec.Marketplace` (e.g. `"coding"` from `"bborbe/coding"`). Use `path.Base` (package `"path"`, not `"path/filepath"`).
      - Derive `updateForm` = `spec.Name + "@" + alias` (e.g. `"coding@coding"`).

   c. Call `commander.Run(ctx, "claude", "plugin", "list")`.
      - If it returns an error → wrap and return: `errors.Wrapf(ctx, err, "list plugins")`.
      - Parse stdout: split by newline, check if any line contains `spec.Name` as a substring (`strings.Contains`).

   d. If plugin **not found** (install path):
      - `commander.Run(ctx, "claude", "plugin", "marketplace", "add", spec.Marketplace)` — error → wrap and return.
      - `commander.Run(ctx, "claude", "plugin", "install", spec.Name)` — error → wrap and return.

   e. If plugin **found** (update path):
      - `commander.Run(ctx, "claude", "plugin", "marketplace", "update", alias)` — on error: log warning (do NOT return error): `glog.Warningf("marketplace update failed plugin=%s cmd=%s err=%v", spec.Name, "claude plugin marketplace update "+alias, err)`
      - `commander.Run(ctx, "claude", "plugin", "update", updateForm)` — on error: log warning (do NOT return error): `glog.Warningf("plugin update failed plugin=%s cmd=%s err=%v", spec.Name, "claude plugin update "+updateForm, err)`

   f. Continue to the next spec. Return `nil` after all specs processed without hard failure.

5. Run `go generate ./pkg/plugins/...` from `agent/pr-reviewer/` to produce:
   - `agent/pr-reviewer/pkg/plugins/mocks/commander.go`
   - `agent/pr-reviewer/pkg/plugins/mocks/installer.go`

   Verify counterfeiter is available: `grep counterfeiter agent/pr-reviewer/go.mod` — if missing, add it with `go get github.com/maxbrunsfeld/counterfeiter/v6`.

6. Create `agent/pr-reviewer/pkg/plugins/plugins_test.go` using Ginkgo v2 + Gomega. The test suite must use the generated `FakeCommander` from `mocks/` and must NOT invoke the real `claude` binary. Cover all of the following cases:

   a. **Empty input** — `EnsureInstalled(ctx, nil)` → returns nil, `FakeCommander.RunCallCount()` == 0.

   b. **Install path** — plugin name NOT in list output:
      - `Run` returns `("other-plugin\n", nil)` on first call (list)
      - Subsequent `Run` calls return `("", nil)`
      - Expect: `Run` called 3 times total (list, marketplace add, install)
      - Verify call args: call 1 = `["claude","plugin","list"]`, call 2 = `["claude","plugin","marketplace","add","bborbe/coding"]`, call 3 = `["claude","plugin","install","coding"]`

   c. **Update path** — plugin name IS in list output:
      - `Run` returns `("coding v1.0\n", nil)` on first call (list)
      - Subsequent `Run` calls return `("", nil)`
      - Expect: `Run` called 3 times total (list, marketplace update, plugin update)
      - Verify call args: call 2 = `["claude","plugin","marketplace","update","coding"]`, call 3 = `["claude","plugin","update","coding@coding"]`

   d. **List failure** — `Run` returns error on first call:
      - Expect `EnsureInstalled` returns non-nil error containing "list plugins"

   e. **Marketplace add failure** (install path) — `Run` returns `("", nil)` (list, no match), then error on second call:
      - Expect `EnsureInstalled` returns non-nil error

   f. **Plugin install failure** (install path) — list returns empty, marketplace add succeeds, install returns error:
      - Expect `EnsureInstalled` returns non-nil error

   g. **Soft failure: marketplace update** (update path) — list shows plugin, marketplace update returns error:
      - Expect `EnsureInstalled` returns nil (soft failure — warning only)
      - Expect plugin update is still attempted (Run called for update)

   h. **Soft failure: plugin update** (update path) — list shows plugin, marketplace update succeeds, plugin update returns error:
      - Expect `EnsureInstalled` returns nil (soft failure — warning only)

   i. **Context cancellation** — use a cancelled context (`ctx, cancel := context.WithCancel(context.Background()); cancel()`):
      - `Run` returns `(ctx.Err())` when called
      - Expect `EnsureInstalled` returns non-nil error

7. Run `cd agent/pr-reviewer && make test` — must pass.

8. Check coverage:
   ```bash
   cd agent/pr-reviewer && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/plugins/... && go tool cover -func=/tmp/cover.out
   ```
   Statement coverage for `pkg/plugins` must be ≥80%.
</requirements>

<constraints>
- Package location: `agent/pr-reviewer/pkg/plugins/` — do NOT create a `lib/` package or touch other modules
- Do NOT introduce a dependency on `github.com/bborbe/agent/lib` in this package
- All `Commander.Run` calls must pass args as separate exec arguments — never via shell or string concatenation in the exec call
- Hard failures (list exits non-zero, marketplace add fails, install fails, context cancelled): return wrapped errors using `errors.Wrapf(ctx, err, "...")` — never `fmt.Errorf`
- Soft failures (marketplace update fails, plugin update fails): log with `glog.Warningf` using the greppable structured format from the spec — do NOT return error
- Log format is a breaking change — use exactly: `glog.Warningf("marketplace update failed plugin=%s cmd=%s err=%v", ...)` and `glog.Warningf("plugin update failed plugin=%s cmd=%s err=%v", ...)`
- Counterfeiter fakes must be generated (`go generate`), NOT hand-written
- Tests must use the generated `FakeCommander`, never the real `claude` binary
- Do NOT run `go mod vendor` — vendor is regenerated by `make buca`, never committed
- Do NOT commit — dark-factory handles git
- All existing tests must still pass
- `make precommit` must exit 0
</constraints>

<verification>
```bash
# Package files exist
ls agent/pr-reviewer/pkg/plugins/

# Mocks generated
ls agent/pr-reviewer/pkg/plugins/mocks/

# Tests pass
cd agent/pr-reviewer && make test

# Coverage ≥80% for pkg/plugins
cd agent/pr-reviewer && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/plugins/... && go tool cover -func=/tmp/cover.out

# Tests do NOT instantiate the real Commander (must use FakeCommander only)
grep -n 'NewExecCommander\|exec\.Command\|os/exec' agent/pr-reviewer/pkg/plugins/plugins_test.go
# Expect: no matches (test only uses the generated FakeCommander)

# Full precommit
cd agent/pr-reviewer && make precommit
```
</verification>
