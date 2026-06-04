---
status: verifying
approved: "2026-05-29T09:05:42Z"
generating: "2026-05-29T09:23:36Z"
prompted: "2026-05-29T09:23:36Z"
verifying: "2026-05-29T09:26:42Z"
---

# introduce-maintainer-yaml-release-gate

## Summary

- Introduce `.maintainer.yaml` at the repo root as the trust gate that opts a target repo into maintainer-bot behaviors, replacing the watcher's current dependency on `.dark-factory/config.yml`.
- v1 schema: a single top-level `release` namespace with one field, `autoRelease`. Future bot namespaces (`pr-reviewer`, `build-fix`, …) live as sibling top-level keys.
- Trust model is gate-only: the github-release watcher emits a release task **only** if `.maintainer.yaml` exists and `release.autoRelease` is `true`. Anything else — missing file, missing key, `false` — means the watcher skips the repo silently.
- The github-releaser agent does NOT consult `.maintainer.yaml`. Operators can always hand-author a release task to bypass the gate; the file gates only the automatic watcher path.
- Clean break: the old `GetAutoReleaseConfig` method, its YAML parser, and its tests are removed. No fallback to `.dark-factory/config.yml`. The release watcher reads `.maintainer.yaml` exclusively.

## Problem

The github-release watcher currently decides whether to emit a release task by reading `.dark-factory/config.yml: autoRelease` from the target repo. This conflates two independent tools: dark-factory (a spec/prompt orchestration system) and maintainer-bot (a fleet-management agent collection). A repo can legitimately use dark-factory without consenting to maintainer auto-release, or want maintainer auto-release without ever touching dark-factory. Keying the trust gate off another tool's config file is an architectural error that leaks dark-factory's name into every target repo and blocks the fleet from cleanly adopting maintainer features. The watcher must read its own config file, owned by maintainer, namespaced for future maintainer agents.

## Goal

The github-release watcher's gate decision is made exclusively by reading `.maintainer.yaml` from the target repo's default branch. A repo is automatically released only when that file exists and contains `release.autoRelease: true`. The watcher has no remaining reference, in code or tests, to `.dark-factory/config.yml`. The config type is namespaced per agent (`release:` is one of N future top-level keys), so adding the next maintainer bot does not require a schema migration.

## Non-goals

- Migrating the pr-reviewer agent off `.pr-reviewer.yaml` onto `.maintainer.yaml: pr-reviewer:`. Tracked separately.
- Authoring `.maintainer.yaml` into the bborbe-org fleet repos. That is an operational rollout task, not code in this repo.
- Adding additional top-level namespaces (`build-fix:`, `dep-pin:`, etc.). Out of scope until those agents exist.
- A `/release-repo` slash command or other manual-release UX. The manual path remains: an operator hand-writes a task file assigned to `github-releaser-agent`. No new UI.
- Teaching the github-releaser agent to read `.maintainer.yaml`. The gate is watcher-side only; the agent must remain config-file-agnostic so manual tasks continue to work.
- Backward-compat fallback to `.dark-factory/config.yml`. Do NOT add a "read maintainer.yaml first, fall back to dark-factory config" path — invariant; if a future migration window demands one, that's a separate spec.
- A per-repo opt-out flag inside `.maintainer.yaml` for the gate itself. The absence of the file (or the field) IS the opt-out; adding an explicit `enabled: false` would be an escape hatch on the Goal.

## Desired Behavior

1. A new config type representing the parsed `.maintainer.yaml` document exposes a nested `Release` section with an `AutoRelease bool` field, mapped from YAML key `release.autoRelease`. Unknown top-level keys (e.g., `pr-reviewer:`, `build-fix:`) are accepted and ignored without error — forward-compat with future namespaces.
2. The watcher's GitHub client exposes one method that fetches and parses `.maintainer.yaml` from the target repo's default branch. The method returns the parsed config plus a nil error in the happy path; it returns a zero-value config plus a nil error on HTTP 404 (file absent — the common case); it returns the sentinel `ErrRateLimited` when GitHub responds with rate-limit / abuse-rate-limit; it returns a wrapped error on every other failure (network, 5xx, decode failure, YAML parse failure).
3. Malformed YAML in `.maintainer.yaml` surfaces as a wrapped error — NOT silently treated as `autoRelease: false`. Silent fallback would mask operator typos and let a misconfigured repo sit indefinitely in the "skipped, no idea why" state.
4. The watcher's per-cycle release gathering reads `.maintainer.yaml` instead of `.dark-factory/config.yml`. The gate predicate in the filter chain emits a release task ONLY when the parsed config's `Release.AutoRelease` is `true`. Every other outcome — file absent, `release:` key absent, `autoRelease` key absent, value `false` — skips the repo with the same metric label.
5. The old `GetAutoReleaseConfig` method, its private `parseAutoReleaseConfig` helper, the `darkFactoryConfig` struct, and every test exercising them are removed from `watcher/github-release/pkg/`. No reference to `.dark-factory/config.yml` remains in this watcher's source or tests.
6. Counterfeiter mocks for `GitHubClient` are regenerated so that test code referencing the removed method no longer compiles and the new method is mockable from the same path the existing mocks live at.
7. The skip-reason metric label exposed by the gate filter stays semantically "this repo is not opted into auto-release" so existing dashboards continue to work; the exact label string is one the agent decides at impl time, documented in the implementation prompt and the watcher godoc.

## Constraints

- Repository: `~/Documents/workspaces/maintainer`, worktree `~/Documents/workspaces/maintainer-yaml`, branch `feat/maintainer-yaml-release-gate`, forked off `origin/master`.
- All code lives under `watcher/github-release/`. No other watcher, no other agent, no shared lib package is modified by this spec.
- Error wrapping must use `github.com/bborbe/errors` in context form: `errors.New(ctx, msg)`, `errors.Wrap(ctx, err, msg)`, `errors.Wrapf(ctx, err, fmt, args...)`. Never `fmt.Errorf` on production paths.
- Tests must use Ginkgo v2 + Gomega, external `_test` packages, matching the existing watcher conventions.
- Mocks are regenerated via the existing `make generate` target using counterfeiter v6. No new tooling.
- The fetch path must mirror the existing `GetChangelogContent` shape: `Repositories.GetContents` with `Ref: repo.DefaultBranch`, 404 mapping to a zero-value happy-path return, rate-limit mapping to `ErrRateLimited`.
- No `time.Now()` in business logic; if the implementation needs time (it shouldn't), it uses `github.com/bborbe/time` `libtime`.
- The frontmatter contract for tasks the watcher emits is unchanged. This spec touches the gate-decision input only, not task content.
- The github-releaser agent's behavior is unchanged. It never reads `.maintainer.yaml`. Manual tasks (operator-authored, assigned to `github-releaser-agent`) continue to flow without consulting any config file.
- Watcher coverage target ≥80% for the `pkg/` directory, matching the existing module's bar.
- Reference docs in this repo: `docs/architecture.md`, `docs/watcher-decision-chains.md`, `docs/build-watcher.md`. The `auto_release` predicate is one node of the decision chain in `docs/watcher-decision-chains.md`; updating the rendered chain there is the prompt's responsibility.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| `.maintainer.yaml` absent (HTTP 404) | Fetch returns zero-value config + nil error; gate skips the repo | Operator commits the file with `release.autoRelease: true` | Skipped-repo metric increments with the gate label |
| File present, valid YAML, `release.autoRelease: true` | Gate passes; watcher proceeds to emit the release task | n/a — happy path | Release task appears on Kafka task topic |
| File present, valid YAML, `release:` key absent | Gate skips the repo | Operator adds `release: {autoRelease: true}` | Same skip metric as the absent-file case |
| File present, valid YAML, `release.autoRelease: false` | Gate skips the repo | Operator flips to `true` | Same skip metric |
| File present, malformed YAML | Fetch returns wrapped error; per-repo prune; the cycle continues for other repos | Operator fixes the YAML | Wrapped error appears in watcher logs with repo identity; per-repo error metric increments |
| File present, unknown top-level keys present (e.g., `pr-reviewer:`) | Unknown keys ignored; `release:` parsed normally | n/a — forward-compat | Happy or skip path as above |
| GitHub rate-limit on the fetch | Fetch returns `ErrRateLimited`; the cycle aborts without cursor save; `rate_limited` metric increments | Watcher retries on next interval | `rate_limited` poll-cycle metric |
| GitHub 5xx on the fetch | Fetch returns wrapped error; per-repo prune (consistent with how `GetChangelogContent` failures are handled today) | Watcher retries on next interval | Per-repo error log + metric |
| Decoded file exceeds 1 MiB hard cap | Fetch returns wrapped error citing size limit; repo pruned for the cycle | Operator trims the file | Wrapped size-error log |
| Operator wants to release a repo NOT opted-in via `.maintainer.yaml` | Hand-authored task file assigned to `github-releaser-agent` proceeds normally — manual path bypasses the gate | n/a — manual path is always available | Task file commit in vault |

## Security / Abuse Cases

- An attacker who can land a PR into a target repo can flip `.maintainer.yaml: release.autoRelease` between `true` and `false`. That is consistent with the current model — they could equally flip `.dark-factory/config.yml: autoRelease` today. The trust boundary is the target repo's branch-protection / review policy, unchanged.
- Malformed or hostile YAML must not crash the watcher process, hang the fetch, or be silently dropped. A parse error must surface as a wrapped, per-repo error so the repo is pruned for the cycle while other repos continue.
- The fetched file is bounded in size (≤ 1 MiB, matching the existing `GetChangelogContent` cap). The watcher must not load arbitrarily large files into memory.
- Unknown YAML keys are tolerated by design (forward-compat) but must not be deserialized into types that could trigger side effects. The parser surface is plain-data struct fields only.
- The github-releaser agent does NOT consult `.maintainer.yaml`. A manual task remains valid even for repos with `autoRelease: false`. This is intentional: the gate restricts the automatic path only, never the operator's explicit intent.

## Acceptance Criteria

- [ ] `make precommit` in `watcher/github-release/` exits 0 — evidence: exit code 0.
- [ ] `grep -rn "GetAutoReleaseConfig\|dark-factory/config\|darkFactoryConfig\|parseAutoReleaseConfig" watcher/github-release/` returns zero matches — evidence: empty grep output, exit code 1.
- [ ] `grep -rn "\.maintainer\.yaml" watcher/github-release/pkg/` returns at least one match in the GitHub client source — evidence: non-empty grep output.
- [ ] The `GitHubClient` interface in `watcher/github-release/pkg/githubclient.go` declares a method that returns the parsed maintainer config and replaces `GetAutoReleaseConfig`; the old method name does not appear — evidence: `grep -n "GetMaintainerConfig\|GetAutoReleaseConfig" watcher/github-release/pkg/githubclient.go` shows the new name and not the old.
- [ ] Counterfeiter-generated mock at `watcher/github-release/pkg/mocks/github_client.go` exposes the new method and not the old — evidence: `grep -n "GetMaintainerConfigStub\|GetAutoReleaseConfigStub" watcher/github-release/pkg/mocks/github_client.go` shows new, not old.
- [ ] Ginkgo test suite for the GitHub client covers, at minimum: (a) file absent → zero-value config, nil error; (b) empty file → zero-value config, nil error; (c) `release:` key absent → `AutoRelease=false`, nil error; (d) `release.autoRelease: false` → `AutoRelease=false`, nil error; (e) `release.autoRelease: true` → `AutoRelease=true`, nil error; (f) malformed YAML → wrapped error, NOT silently false; (g) unknown top-level keys present (e.g., `pr-reviewer:`) → `release` parsed normally, no error; (h) GitHub rate-limit → `ErrRateLimited`. Evidence: each scenario maps to a named `It(...)` block; `go test ./watcher/github-release/pkg/... -run Maintainer -v` lists every block by name and reports 0 failures.
- [ ] Filter-chain gate behavior under the new semantics is exercised: a `Release` whose backing config has `autoRelease: true` passes the gate; every other config shape (`false`, absent, zero-value) is skipped with a stable metric label. Evidence: Ginkgo `It` blocks in the filter package, `go test` exit 0.
- [ ] Watcher `pkg/` coverage ≥ 80%. Evidence: `go test -cover ./watcher/github-release/pkg/...` reports a coverage line ≥ 80.0%.
- [ ] No reference to `.dark-factory/config.yml` remains anywhere in `watcher/github-release/` (source, tests, godoc, comments). Evidence: `grep -rn "dark-factory/config" watcher/github-release/` exits with status 1 / empty output.
- [ ] The `docs/watcher-decision-chains.md` node describing the auto-release predicate refers to `.maintainer.yaml`, not `.dark-factory/config.yml`. Evidence: `grep -n "maintainer.yaml" docs/watcher-decision-chains.md` returns at least one line and `grep -n "dark-factory/config" docs/watcher-decision-chains.md` returns none.

No new scenario test is required. The existing watcher Ginkgo suite covers the GitHub client surface, the parser, and the gate filter under mocked `GitHubClient` — that is the right test level for a config-source swap. Adding an E2E scenario for a YAML-rename would be top-of-pyramid bloat.

## Verification

```
cd watcher/github-release
make precommit
go test -cover ./pkg/...
grep -rn "GetAutoReleaseConfig\|dark-factory/config\|darkFactoryConfig\|parseAutoReleaseConfig" .
grep -rn "\.maintainer\.yaml" pkg/
```

Expected:
- `make precommit` exits 0.
- `go test -cover` reports ≥ 80.0% for `./pkg/...`.
- First `grep` exits 1 with no matches.
- Second `grep` exits 0 with at least one match in `githubclient.go` (or its sibling parser file).

## Do-Nothing Option

If we ship nothing, the watcher continues to read `.dark-factory/config.yml: autoRelease` on every cycle. Functionally the fleet keeps working — every currently-opted-in repo has that file, every currently-opted-out repo lacks it. The cost is durable architectural confusion: maintainer-bot's trust gate is named after a different tool, every operator onboarding a new repo has to be told "yes, you need a dark-factory file even if you never use dark-factory," and the future `pr-reviewer`/`build-fix`/`dep-pin` bots either each grow their own dot-file (proliferation) or also key off `.dark-factory/config.yml` (deeper coupling). The do-nothing path is sustainable but compounds: every additional maintainer bot makes the rename more expensive. Now — when the only consumer is the just-shipped release watcher and the fleet is small — is the cheapest moment.

## Open Questions

- Exact skip-reason metric label string. The current code emits `"auto_release"` for "dark-factory handles it" (i.e., the inverse of the new gate). The new gate's skip is semantically "not opted in." Agent decides at impl time; the prompt should pin one label (e.g., `"not_opted_in"` or keep `"auto_release"` for dashboard continuity) and document the choice in godoc.
- Whether to keep the gate predicate file named `auto_release_filter.go` or rename to e.g. `maintainer_gate_filter.go`. Agent decides at impl time — either is fine; the spec only constrains behavior.

## Verification Result

**Verified:** 2026-06-04T15:12:26Z (HEAD 9a0d185)
**Binary:** installed `dark-factory` (target repo: maintainer, not dark-factory itself)
**Scenario:** Ginkgo suite replay against current master — client `GetMaintainerConfig` + `AutoReleaseFilter` + grep guards.
**Evidence:**
- `make precommit` in `watcher/github-release/`: exit 0; `pkg` coverage 81.9%, `pkg/filter` 100%, `pkg/auth` 100%.
- `grep -rn "GetAutoReleaseConfig|dark-factory/config|darkFactoryConfig|parseAutoReleaseConfig" watcher/github-release/` → empty, exit 1.
- `grep -n "GetMaintainerConfig" watcher/github-release/pkg/githubclient.go` → interface decl line 62, impl line 255.
- `grep -n "GetMaintainerConfigStub" watcher/github-release/mocks/github_client.go` → line 30 (mock regenerated; no `GetAutoReleaseConfigStub`).
- `go test ./pkg/... -ginkgo.focus="GetMaintainerConfig"` → 10 Passed, 0 Failed; covers (a) 404 (b) empty (c) release-absent (d) false (e) true (f) malformed (g) unknown-keys (h) rate-limit + HTTP 500 + oversize.
- `go test ./pkg/filter/... -ginkgo.focus="AutoReleaseFilter"` → 3 Passed: pass-on-true, skip-on-false (label `auto_release`), skip-on-zero-value (label `auto_release`).
- `docs/watcher-decision-chains.md` lines 77-81 reference `.maintainer.yaml`; `grep "dark-factory/config" docs/watcher-decision-chains.md` empty.
**Verdict:** PASS
