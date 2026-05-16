---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-04-27T19:44:33Z"
generating: "2026-04-27T19:44:54Z"
prompted: "2026-04-27T19:47:35Z"
verifying: "2026-04-27T20:03:13Z"
completed: "2026-05-03T18:24:50Z"
branch: dark-factory/ensure-plugins-installer
---
Tags: [[Dark Factory - Spec Writing Guide]]

---

## Summary

- Add a Go library that ensures a list of Claude Code plugins are installed (or refreshed) by shelling out to the `claude` CLI before the agent starts handling tasks.
- Lives in `pr-reviewer/pkg/plugins/` for fast iteration; promotion to shared `agent/lib/claude/` is a later, separate task.
- Unblocks PR-reviewer goal by making the `bborbe/coding` plugin (provides `/coding:pr-review` and specialist sub-agents) available in `agent-pr-reviewer` pods.
- Idempotent: safe to call on every pod boot. PVC-backed plugin cache makes the steady-state path a fast `update`.
- Hard failures on install or list propagate as wrapped errors; update failures are logged warnings (non-fatal).

## Problem

`agent-pr-reviewer` pods do not have the `bborbe/coding` Claude plugin installed when they boot. Without it, the planned `/coding:pr-review` slash command and specialist sub-agents (go-quality, go-security, etc.) are unavailable, blocking the next phase of PR-reviewer work. We also have no reusable mechanism for any future agent that wraps the `claude` CLI to declare and install its plugin dependencies.

## Goal

After completion, an agent process can declare a list of required Claude plugins at startup and be guaranteed that those plugins are installed and reasonably up-to-date before it begins handling tasks. The mechanism is a Go library, testable without shelling out, and wired into `agent-pr-reviewer`'s `main` before `agent.Run`.

## Non-goals

- Pinning plugins to specific versions (always tracks marketplace latest)
- Offline / no-github fallback (first boot requires github.com; subsequent boots use PVC cache)
- Multi-replica concurrency locking (PR-reviewer is single-pod per Config CRD)
- Promotion of the package to `agent/lib/claude/` (separate Phase 2 task)
- Wiring the installer into agents other than `pr-reviewer`
- A CLI/binary entrypoint — this is a library, called from `main`

## Desired Behavior

1. The installer accepts a list of plugin specs, each identifying a marketplace and a plugin name (e.g. marketplace `bborbe/coding`, name `coding@coding`).
2. An empty list is a no-op that returns success without invoking the `claude` CLI.
3. For each spec, the installer first determines whether the plugin is already installed by inspecting `claude plugin list` output.
4. If the plugin is absent, the installer adds the marketplace and then installs the plugin.
5. If the plugin is present, the installer refreshes the marketplace and then updates the plugin.
6. The installer is idempotent: running it repeatedly with the same input converges to the same end state and produces no spurious errors.
7. The installer is wired into `agent-pr-reviewer` and runs before `agent.Run`, so task handling never starts until plugins are ready.
8. The library exposes an injectable command-runner seam so tests can verify behavior without invoking the real `claude` binary.

## Constraints

- API surface (frozen for this spec):
  - A `Spec` value with `Marketplace` (e.g. `bborbe/coding`) and `Name` (e.g. `coding`) fields. `Name` holds the plain plugin name as used by `claude plugin install`.
  - An `Installer` interface with `EnsureInstalled(ctx, []Spec) error`.
  - A `Commander` interface with `Run(ctx, name, args...) (stdout string, err error)`.
  - Constructors `NewInstaller(Commander) Installer` and `NewExecCommander() Commander`.
- Command-form derivation: `claude plugin install` and `claude plugin marketplace update` take `<Name>` (e.g. `coding`); `claude plugin update` takes `<Name>@<marketplace-alias>` (e.g. `coding@coding`) where the marketplace alias is the last path segment of `Marketplace`. The installer derives the update form from `Spec` — callers pass only the install form.
- A Counterfeiter fake must be generated for `Commander` and used in tests; tests must not invoke the real `claude` binary.
- Package location for Phase 1: `agent/pr-reviewer/pkg/plugins/` in `bborbe/code-reviewer`. Do not introduce a dependency on `bborbe/agent/lib` for this code.
- Must not modify or break the existing PR-reviewer end-to-end behavior verified by PR #2 (verdict=pass, phase=done).
- Hard failures (cannot determine installed state, marketplace add fails, install fails, context cancelled) must surface as wrapped errors that abort startup.
- Soft failures (marketplace update, plugin update) must be logged as warnings and must not abort startup — the cached plugin is still usable.
- "Already installed" detection uses substring/line matching of the spec's `Name` against `claude plugin list` stdout.
- Soft-failure warnings (marketplace update / plugin update failures) MUST be logged with greppable structured fields: plugin name, command, underlying error. The dev verification grep relies on this shape — log format changes are a breaking change.
- The `<Name>@<marketplace-alias>` derivation rule and list-output line/substring matching are durable institutional knowledge about the `claude` CLI. After implementation, extract them to `docs/claude-plugin-cli.md` so the rules survive after this spec is archived.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Empty plugin list | Return nil, no commands invoked | n/a |
| `claude plugin list` exits non-zero | Wrapped error (cannot determine state) | Pod restart / fix env |
| `claude plugin list` exits zero with empty stdout | Treat as "no plugins installed", proceed via install path | Automatic |
| Plugin name not found in list output | Run install path | Automatic |
| Plugin name found in list output | Run update path | Automatic |
| `claude plugin marketplace add` fails | Wrapped error | Pod restart |
| `claude plugin install` fails | Wrapped error | Pod restart |
| `claude plugin marketplace update` fails | Log warning, continue to plugin update | Automatic |
| `claude plugin update` fails | Log warning, continue (cached plugin still works) | Automatic |
| Context cancelled mid-call | Wrapped error from underlying exec | Pod restart |

## Security / Abuse Cases

- Plugin specs come from the agent's own configuration (operator-controlled), not user input — no untrusted plugin names are accepted at runtime.
- The installer shells out to `claude`. The library must pass marketplace and name as separate exec arguments (no shell interpolation) so a malformed spec cannot inject extra commands.
- First boot reaches out to github.com via the `claude` CLI; failures here surface as wrapped errors and prevent task handling, which is the desired safe default.

## Open Questions (decided)

1. **`claude plugin list` exit code with no plugins installed** — assume exit 0 with empty stdout and proceed via install path. If exit is non-zero, treat as hard error (cannot determine state).
2. **"Already installed" detection** — substring match of `Name` against the lines of `claude plugin list` stdout. Good enough for the small, operator-controlled plugin sets we use.
3. **`claude plugin marketplace add` idempotency on re-add** — assumed to succeed; if it errors on a previously-added marketplace, we still attempt the install/update path. Revisit only if observed in production.

## Acceptance Criteria

- [ ] Plugin install + update logic lives in `agent/pr-reviewer/pkg/plugins/`
- [ ] `Installer` and `Commander` interfaces exist with the frozen signatures above
- [ ] Counterfeiter fake for `Commander` is generated and used in unit tests
- [ ] Unit tests cover: empty input, install path, update path, list-failure, install-failure, update soft-failure, context cancellation
- [ ] Installer is invoked from `agent/pr-reviewer/main.go` before `agent.Run`
- [ ] `make precommit` passes
- [ ] `code-reviewer` master is updated and tagged `v0.15.0`
- [ ] **E2E scenario** (subprocess interface seam — required per spec-writing.md): deploy v0.15.0 to dev; observe first job-pod logs include `claude plugin install coding`; observe subsequent job-pod logs include `claude plugin update coding@coding`; trigger the existing PR #2 e2e task and confirm it still produces verdict=pass, phase=done, single trigger sequence — confirms (a) the new subprocess seam works for both install and update forms, (b) idempotency holds, (c) no regression on existing phase-system behavior

## Verification

```
make precommit
```

Manual verification in dev (the agent runs as K8s Jobs, not Deployment — one Job per task phase):

```
# First job after deploy — expect install path
kubectlquant -n dev logs $(kubectlquant -n dev get jobs -l agent=pr-reviewer-agent -o name | sort | tail -1) | grep "claude plugin"

# A subsequent job — expect update path
kubectlquant -n dev logs <next-job-name> | grep "claude plugin"
```

Re-run the existing PR #2 e2e (`tasks/e2e-test-pr-reviewer-3phase-20260427.md` with `phase: planning, status: in_progress, trigger_count: 0`) and confirm verdict=pass, phase=done.

## Do-Nothing Option

Stay at code-reviewer v0.14.2: agent continues running its hand-rolled per-phase prompts on `gh pr diff` strings — mechanically functional but no project context, no specialist sub-agent dispatch, no severity buckets. Reviews stay shallow on real PRs. Tasks A2 + A3 of [GitHub PR Reviewer](../../../Obsidian/Personal/23%20Goals/GitHub%20PR%20Reviewer.md) cannot start, blocking the entire MVP wave.

A weaker alternative — hand-install the plugin into the PVC out-of-band and rely on persistence — is brittle (fresh PVC, node migration, or plugin update silently breaks the agent) and provides no reusable mechanism for future agents.
