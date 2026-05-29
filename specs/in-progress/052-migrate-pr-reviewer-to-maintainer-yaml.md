---
status: approved
tags:
    - dark-factory
    - spec
approved: "2026-05-29T14:47:05Z"
branch: dark-factory/migrate-pr-reviewer-to-maintainer-yaml
---

## Summary

- Migrate the pr-reviewer agent's per-repo trust config off `.pr-reviewer.yaml` onto `.maintainer.yaml`, under a `prReviewer` top-level namespace — the migration spec 045 explicitly deferred (its Non-goals: "Migrating the pr-reviewer agent off `.pr-reviewer.yaml` onto `.maintainer.yaml`. Tracked separately.").
- Namespace key is camelCase `prReviewer` (NOT kebab `pr-reviewer`), consistent with the camelCase field names (`autoApprove`, `autoRelease`) and the already-shipped `release` namespace. The whole `.maintainer.yaml` document stays camelCase. This intentionally supersedes spec 045's tentative kebab `pr-reviewer:` key (045 named it in its Non-goals as future work but shipped no consumer of that key — its parser ignores unknown top-level keys — so there is nothing to migrate).
- v1 `prReviewer` schema: a single field, `autoApprove: bool`, mapped from `.maintainer.yaml: prReviewer.autoApprove`. Same semantics as today's `.pr-reviewer.yaml: autoApprove`.
- Introduce a shared `lib/maintainerconfig` package holding the **single** `.maintainer.yaml` schema with all bot namespaces as sibling top-level keys (`release`, `prReviewer`) plus a pure `Parse([]byte)`. Both the github-release watcher and the pr-reviewer agent consume this one type — no divergent definitions of the same file.
- Clean break: the pr-reviewer agent reads `.maintainer.yaml: prReviewer.autoApprove` exclusively. The `.pr-reviewer.yaml` reader, its `AutoApproveConfig` type, and its tests are removed. No fallback. We are early and we control every repo currently using pr-reviewer.
- The github-release watcher's behavior is unchanged; only the *source* of its `MaintainerConfig` type changes (local struct → shared lib). Its GitHub-API fetch path stays.
- Fleet file rollout (renaming `.pr-reviewer.yaml` → `.maintainer.yaml: prReviewer:` in each target repo) is operational and lives in the vault task, NOT this spec.

## Problem

The github-release watcher (spec 045) established `.maintainer.yaml` as maintainer-bot's own per-repo trust file, namespaced for future bots, decoupled from any other tool. The pr-reviewer agent still reads a separate `.pr-reviewer.yaml: autoApprove` from the cloned workDir (`agent/pr-reviewer/pkg/githubposter/config.go:18`, type `AutoApproveConfig` in `types.go:17`). Two maintainer bots now mean two per-repo dot-files with two parsers — exactly the dot-file proliferation spec 045's namespacing was designed to prevent. Every repo opting into both bots carries `.pr-reviewer.yaml` AND `.maintainer.yaml`. The schema author intended `prReviewer:` to be a sibling key in the one `.maintainer.yaml` document; this spec realizes that intent and collapses the two parsers into one shared library.

## Goal

The pr-reviewer agent's auto-approve decision is made exclusively by reading `.maintainer.yaml: prReviewer.autoApprove` from the cloned workDir. No reference to `.pr-reviewer.yaml` remains in `agent/pr-reviewer/` source or tests. A single `lib/maintainerconfig` package defines the `.maintainer.yaml` schema (all namespaces) and its parser; the github-release watcher and the pr-reviewer agent both import it, so adding the next bot namespace is a one-file schema edit, not a per-consumer parser change.

## Non-goals

- Renaming `.pr-reviewer.yaml` → `.maintainer.yaml` inside the bborbe-org fleet repos. That is the operational rollout in the vault task, not code in this repo. (Distinct repos affected: `agent`, `maintainer`, `trading`, `go-skeleton`.)
- Touching the operator-side global config `~/.config/maintainer/pr-reviewer.yaml` (the local-CLI repo allowlist read by `cmd/cli/main.go` + `pkg/config.go`). That file is a different concern (operator's machine, not per-target-repo) and keeps its name. This spec migrates ONLY the per-repo in-repo `autoApprove` trust file.
- Changing auto-approve *semantics*. `autoApprove: true` still means "post an approving review on an `approve` verdict"; absence/false still means comment-only. Behavior identical — only the file + key path move.
- Adding new `prReviewer` config fields beyond `autoApprove`. Out of scope until needed.
- Backward-compat fallback (read `.maintainer.yaml`, fall back to `.pr-reviewer.yaml`). Explicitly NOT wanted — hard cut. If a future migration window demands one, that is a separate spec.
- A read-both transition mode. The fleet rollout (vault task) moves every repo's file in lockstep with the deploy; no transition window needed.
- Changing the github-release watcher's gate behavior, its GitHub-API fetch, or its `release` semantics. Only the Go *type* it parses into moves to the shared lib.

## Desired Behavior

1. A shared package `lib/maintainerconfig` exposes the full `.maintainer.yaml` schema as one struct: a `Release` section (`AutoRelease bool`, yaml `release.autoRelease`) and a `PrReviewer` section (`AutoApprove bool`, yaml `prReviewer.autoApprove`). Unknown top-level keys are accepted and ignored (forward-compat with future namespaces, e.g. `build-fix:`).
2. The package exposes a pure parse function that takes raw bytes and a context and returns the parsed config plus a wrapped error on malformed YAML. It does no I/O — fetching/reading the bytes is each consumer's responsibility (the watcher fetches via GitHub API; the agent reads from the cloned workDir on disk).
3. The pr-reviewer agent's workDir config reader (`githubposter.ReadAutoApproveConfig`, today reading `.pr-reviewer.yaml`) reads `.maintainer.yaml` from the same workDir, parses it via `lib/maintainerconfig`, and returns `cfg.PrReviewer.AutoApprove`. A missing file is not an error — returns `autoApprove: false` (unchanged default). Malformed YAML surfaces as a wrapped error (NOT silently false), matching the watcher's spec-045 stance.
4. The github-release watcher's `GetMaintainerConfig` GitHub-client method now returns the `lib/maintainerconfig` type instead of its locally-defined `MaintainerConfig`. The watcher's local `MaintainerConfig`, `MaintainerReleaseConfig`, and private `parseMaintainerConfig` are deleted; the fetch method delegates parsing to the shared lib. The 404/rate-limit/size-cap behavior is preserved exactly.
5. The pr-reviewer agent's `AutoApproveConfig` type and the `.pr-reviewer.yaml` filepath constant are removed. No string `.pr-reviewer.yaml` remains in `agent/pr-reviewer/` source, tests, or godoc.
6. Counterfeiter mocks affected by the watcher's `GetMaintainerConfig` return-type change are regenerated via `make generate` so test code compiles against the shared type.
7. The auto-approve decision path in the ai_review phase (`githubposter`) is exercised by tests against the new file + key: `autoApprove: true` under `prReviewer:` → approving review; absent file / absent `prReviewer:` key / `false` → comment-only.

## Constraints

- Repository: `~/Documents/workspaces/maintainer`; new worktree `~/Documents/workspaces/maintainer-pr-reviewer-yaml`, branch `feat/pr-reviewer-maintainer-yaml`, forked off `origin/master`.
- New shared code lives in `lib/maintainerconfig/` as a plain package inside the existing `lib` Go module (`github.com/bborbe/maintainer/lib`) — NOT its own module, NOT its own Makefile. It sits beside `lib/repoallowlist` (the precedent: plain package, no per-package Makefile) and is built/tested via the `lib` module's top-level `make precommit`. Consumers changed: `agent/pr-reviewer/pkg/githubposter/` and `watcher/github-release/pkg/`. No other agent or watcher is touched.
- Error wrapping must use `github.com/bborbe/errors` context form (`errors.New(ctx, …)`, `errors.Wrap(ctx, err, …)`, `errors.Wrapf(ctx, err, …)`). Never `fmt.Errorf` on production paths.
- Tests use Ginkgo v2 + Gomega, external `_test` packages, matching existing conventions.
- Mocks regenerated via the existing `make generate` (counterfeiter v6). No new tooling.
- No `time.Now()` in business logic; use `github.com/bborbe/time` `libtime` if time is ever needed (it should not be).
- The shared `lib/maintainerconfig` parse function uses the camelCase yaml key `prReviewer` (struct tag `yaml:"prReviewer"`).
- Coverage targets: `lib/maintainerconfig/` ≥90% (pure parser, easy to cover); `agent/pr-reviewer/pkg/` and `watcher/github-release/pkg/` stay ≥80% (existing bar).
- The github-releaser agent is NOT touched (it never read either config file; spec 045 invariant holds).
- The pr-reviewer agent's task/frontmatter contract is unchanged.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| `.maintainer.yaml` absent in workDir | Reader returns `autoApprove: false`, nil error → comment-only | Operator commits `.maintainer.yaml: prReviewer.autoApprove: true` | Review posted as comment, not approval |
| File present, `prReviewer.autoApprove: true` | Approving review posted on `approve` verdict | n/a — happy path | GitHub approval event |
| File present, `prReviewer:` key absent | `autoApprove: false` → comment-only | Operator adds the namespace | Comment-only review |
| File present, `prReviewer.autoApprove: false` | comment-only | Operator flips to true | Comment-only review |
| File present, malformed YAML | Wrapped error from the reader; the ai_review step fails loudly (not silently false) | Operator fixes YAML | Wrapped error in agent logs / failure status |
| File present, only `release:` populated (no `prReviewer:`) | `release` ignored by the agent; `autoApprove: false` | n/a — forward/back-compat | Comment-only review |
| `.pr-reviewer.yaml` still present in a repo after deploy | IGNORED — the agent no longer reads it; repo behaves as `autoApprove: false` until `.maintainer.yaml` lands | Rollout (vault task) moves the file | Comment-only review until migrated |
| Decoded `.maintainer.yaml` exceeds size cap (watcher path) | Watcher returns wrapped size error; repo pruned for cycle (unchanged from 045) | Operator trims file | Wrapped size-error log |

## Security / Abuse Cases

- An attacker who can land a PR into a target repo can flip `.maintainer.yaml: prReviewer.autoApprove`. This is identical to today's exposure on `.pr-reviewer.yaml: autoApprove` — the trust boundary remains the target repo's branch-protection / review policy. No new surface.
- Malformed/hostile YAML must not crash the agent or be silently dropped: a parse error surfaces as a wrapped error so the ai_review step fails visibly rather than silently downgrading to comment-only (which would mask operator typos).
- The parsed file is bounded in size on the watcher path (≤1 MiB, unchanged from 045). The agent's workDir read is of a repo the agent already cloned and trusts; no additional bound required beyond normal file read.
- Unknown YAML keys are tolerated by design but deserialized only into plain-data struct fields — no side-effecting types.
- The github-releaser agent and the operator manual-task path are unaffected: neither reads `.maintainer.yaml`.

## Acceptance Criteria

- [ ] `make precommit` exits 0 in each touched module (`lib`, `agent/pr-reviewer`, `watcher/github-release`) — evidence: exit code 0.
- [ ] A shared package exists at `lib/maintainerconfig/` exporting a struct with both `Release` (`AutoRelease bool`) and `PrReviewer` (`AutoApprove bool`) sections and a pure parse function — evidence: `grep -rn "PrReviewer\|AutoApprove\|AutoRelease" lib/maintainerconfig/` shows all three fields.
- [ ] `grep -rn "\.pr-reviewer\.yaml\|AutoApproveConfig" agent/pr-reviewer/` returns zero matches — evidence: empty output, exit code 1.
- [ ] `grep -rn "\.maintainer\.yaml" agent/pr-reviewer/pkg/githubposter/` returns at least one match — evidence: non-empty output.
- [ ] The watcher's GitHub client `GetMaintainerConfig` returns the `lib/maintainerconfig` type; the local `MaintainerConfig`/`MaintainerReleaseConfig`/`parseMaintainerConfig` are gone — evidence: `grep -rn "parseMaintainerConfig\|MaintainerReleaseConfig" watcher/github-release/` exits 1; `grep -n "maintainerconfig" watcher/github-release/pkg/githubclient.go` shows the lib import.
- [ ] Counterfeiter mocks affected by the return-type change are regenerated and compile — evidence: `make generate` produces no diff beyond the intended mock; `go build ./...` exits 0.
- [ ] `lib/maintainerconfig` Ginkgo suite covers: (a) empty bytes → zero-value, nil; (b) `prReviewer.autoApprove: true` → `PrReviewer.AutoApprove=true`; (c) `prReviewer:` absent → false; (d) `release.autoRelease: true` still parses → `Release.AutoRelease=true`; (e) both namespaces populated → both parsed; (f) unknown top-level key (e.g. `build-fix:`) ignored, no error; (g) malformed YAML → wrapped error. Evidence: named `It(...)` blocks; `go test ./lib/maintainerconfig/... -v` lists each, 0 failures.
- [ ] pr-reviewer `githubposter` tests exercise the auto-approve decision against `.maintainer.yaml: prReviewer.autoApprove` (true → approve path; absent/false → comment-only). Evidence: `go test ./agent/pr-reviewer/pkg/githubposter/...` exit 0.
- [ ] Coverage: `lib/maintainerconfig` ≥90%; `agent/pr-reviewer/pkg/...` and `watcher/github-release/pkg/...` ≥80%. Evidence: `go test -cover` lines.
- [ ] No `.pr-reviewer.yaml` reference remains anywhere under `agent/pr-reviewer/` (source, tests, godoc). Evidence: `grep -rn "pr-reviewer.yaml" agent/pr-reviewer/` — only the unrelated `~/.config/maintainer/pr-reviewer.yaml` operator-CLI references in `cmd/cli/main.go` + `pkg/config.go` may remain (those are the global operator config, explicitly out of scope); the per-repo `.pr-reviewer.yaml` workDir references are gone.

## Verification

```
cd ~/Documents/workspaces/maintainer-pr-reviewer-yaml
cd lib && make precommit
cd ../agent/pr-reviewer && make precommit
cd ../../watcher/github-release && make precommit
cd ../..
grep -rn "\.pr-reviewer\.yaml\|AutoApproveConfig" agent/pr-reviewer/pkg/
grep -rn "parseMaintainerConfig\|MaintainerReleaseConfig" watcher/github-release/
grep -rn "maintainerconfig" agent/pr-reviewer/pkg/githubposter/ watcher/github-release/pkg/
```

Expected:
- All `make precommit` exit 0.
- First grep (per-repo `.pr-reviewer.yaml` in pkg) exits 1 / empty.
- Second grep exits 1 / empty (local watcher types removed).
- Third grep shows both consumers importing the shared lib.

## Do-Nothing Option

If we ship nothing, pr-reviewer keeps reading `.pr-reviewer.yaml` and the watcher keeps its own `MaintainerConfig` copy. The fleet works, but every repo opting into both bots carries two dot-files, and the next maintainer bot (`build-fix`, `dep-pin`) faces the same fork: its own dot-file (proliferation) or hand-rolled `.maintainer.yaml` parsing (a third divergent copy of the same schema). The schema author's "sibling top-level keys in one file" intent never materializes. The cost compounds per bot; doing it now — two consumers, small fleet we fully control — is the cheapest moment, exactly as spec 045 argued for the release gate.

## Open Questions

- Whether to keep the agent reader function named `ReadAutoApproveConfig` (returns just the bool) or rename to e.g. `ReadMaintainerConfig` (returns the whole config, caller picks `.PrReviewer.AutoApprove`). Either is fine; the spec constrains behavior, not the name.
