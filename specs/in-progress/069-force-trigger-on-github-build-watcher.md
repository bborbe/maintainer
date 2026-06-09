---
status: approved
tags:
    - dark-factory
    - spec
approved: "2026-06-09T20:07:26Z"
branch: dark-factory/force-trigger-on-github-build-watcher
---

## Summary

- Adds `?force=true` query parameter to `POST /trigger` on `watcher/github-build` so operators can re-publish a `CreateTaskCommand` for a repo whose build is currently red — today the cycle silently no-ops on `red × red` and the controller would also skip on the deterministic task identifier.
- Third and final watcher in the force-flag rollout. Sibling to spec 069 (github-pr, salted-identifier bypass) and spec 071 (github-release, filter-omission bypass). github-build needs **both** bypasses because it has two dedup layers stacked.
- Wires the already-shipped `TriggerBuildCheckCommand.Force` field (spec 068) through the handler → executor → watcher → state machine → task-identifier helper.
- `force=false` (or absent / unparseable) is byte-identical to today: same cursor save, same state-machine transitions, same canonical `TaskIdentifier`, same metrics.
- Closes the parent vault task `Add Force Flag to Maintainer Watcher Trigger Endpoints` (which scoped all three watchers).

## Problem

The github-build watcher tracks per-repo state and locks an episode on the first `red` transition. On every subsequent poll while the repo is still red, the state machine's `red × red` branch is a deliberate no-op — the episode is locked and re-publish is suppressed. This is the right default (it prevents floods on every poll), but it also means an operator who hits `POST /trigger` to re-run a cycle for an already-red repo gets nothing: no new `CreateTaskCommand`, no new vault file, no observable effect. Even if the state-machine skip were removed, the agent controller's idempotent skip would still fire on the deterministic `TaskIdentifier` derived from `(owner, repo, episodeSHA)`. Operators today have no way to demand a fresh task for an already-known red episode after, for example, a prompt-template change, a fix-agent config tweak, or a vault-task deletion mishap. The `Force` field on `TriggerBuildCheckCommand` shipped reserved-unread in spec 068 anticipating exactly this work; this spec finishes the plumbing.

## Goal

After this work, an operator hitting `POST /admin/maintainer-watcher-github-build/trigger?force=true` causes the watcher to run one poll cycle in which the `red × red` episode-lock is bypassed for the cycle AND any resulting publish carries a salted `TaskIdentifier` distinct from the canonical one, so the controller creates a fresh vault file. Every other behavior — green/red derivation, cursor save semantics, all other state-machine transitions, `.maintenance.yaml` lookup, metrics — is unchanged. Calls without `force` (or with `force=false` / unparseable) are bit-identical to current master.

## Non-goals

- Do NOT add force to github-pr (spec 069 / PR #55) or github-release (spec 071 / PR #56) — out of scope here.
- Do NOT change `/admin` gating, auth, or rate-limit posture on the trigger endpoint.
- Do NOT wire up `Scope` on the command — stays reserved-unread; this spec touches `Force` only.
- Do NOT touch `agent/lib/command/task.CreateCommand` or any code in the agent repo — the salted identifier in github-build accomplishes the controller-side bypass without modifying the agent contract.
- Do NOT add a per-feature opt-out flag that disables `force` — invariant; if a future consumer demands a way to switch it off, that's a separate spec. An escape hatch on the Goal is itself a regression.
- Do NOT branch any other state-machine transition (`""/green × red`, `red × green`, no-transition cases) on `Force` — only the `red × red` lock is conditional.
- Do NOT modify `DeriveTaskID`, `buildWatcherNamespace`, or its input encoding `<owner>/<repo>#build-<episodeSHA>`. Non-force path stays bit-identical.
- Do NOT change cursor save semantics. The cursor is written at end-of-cycle on force and non-force paths alike.
- Do NOT introduce a result topic for `TriggerBuildCheckCommand` — `SendResultEnabled` stays `false`.
- Do NOT add a new metric label for force cycles. Forced publishes count as ordinary `IncTaskPublished` / `IncStateTransition("green_to_red")`.

## Desired Behavior

1. `POST /trigger?force=true` (case-insensitive per `libparse.ParseBoolDefault`) causes the handler to publish `TriggerBuildCheckCommand{Force: true}`. Missing parameter, `force=false`, or unparseable value (e.g. `force=banana`) resolves to `Force: false` (lenient default per the parse helper).
2. The handler's v(2) success log line includes `force=%t` (matches the existing executor log shape from spec 068 and the sibling handlers from specs 069/071).
3. The executor reads `cmd.Force` and forwards it to the watcher via a new `Poll(ctx context.Context, force bool) error` signature. The existing v(2) success log (`scope=%q force=%t`) is unchanged — already correct.
4. The watcher's `applyStateMachine` adds a `force` parameter. When `prevState == "red" && currState == "red" && force == true`, the cycle falls through to the same publish path used by the `(prevState == "" || prevState == "green") && currState == "red"` arm: load maintenance overrides, derive a task ID, build the `CreateTaskCommand`, send. When `force == false`, the existing `red × red` skip stays.
5. The forced publish path uses a new helper `DeriveTaskIDForce(owner, repo, episodeSHA, nonce string) uuid.UUID` defined in `pkg/taskid.go`, reusing `buildWatcherNamespace`. Key format `<owner>/<repo>#build-<episodeSHA>!<nonce>` — the `!` separator is intentionally invalid in GitHub owner/repo names and SHAs, so no collision is possible with the canonical key built by `DeriveTaskID`.
6. The nonce is derived from an injected `libtime.CurrentDateTimeGetter`: `strconv.FormatInt(currentDateTimeGetter.Now().UnixMicro(), 10)`. Microsecond resolution. No `time.Now()` call in business logic.
7. Two `force=true` triggers fired against the same repo with the same `episodeSHA`, separated by at least one microsecond of wall-clock distance, produce two distinct `TaskIdentifier` values and therefore two distinct downstream vault files.
8. Non-force path is byte-identical: every other state-machine arm, every metric increment, the cursor-save call, the `.maintenance.yaml` lookup, the `IncStateTransition("green_to_red")` label, and the canonical `DeriveTaskID` invocation are unchanged.

## Constraints

- The "reserved-unread" comment blocks on `TriggerBuildCheckCommand.Force`, on `runTriggerBuildCheck`'s doc block (currently says "does NOT read Scope or Force — both are reserved-unread"), and on `TriggerBuildCheckHandler`'s doc block (currently says "No request body or query string is consumed (both Scope and Force are reserved-unread)") MUST be rewritten to describe the wired behavior. `Scope` remains reserved-unread; only `Force` is wired. (Code-hygiene rule; verified by AC's `grep -nE 'reserved-unread|...' watcher/github-build/pkg/...` returning zero matches.)
- `DeriveTaskID` signature, `buildWatcherNamespace` UUID, and input encoding (`<owner>/<repo>#build-<episodeSHA>`) are frozen. Do NOT modify.
- `TriggerBuildCheckCommand` struct field set, JSON tags, and `Validate` rules are frozen. The `Force` field already exists; do not rename, retype, or drop it.
- `TriggerBuildCheckCommandOperation` wire string (`"trigger-build-check"`) is frozen.
- HTTP response shape on the success path (`{"status":"accepted"}`, HTTP 202) is frozen — no new fields when `force=true`.
- `Watcher` interface change is allowed (single in-pod consumer); production call sites are the executor plus any `main.go` / `cmd/run-once` / poll-loop test that invokes `Poll`. All must be updated in the same prompt that changes the signature.
- `Watcher.Poll`'s new parameter is named `force` (matching `cmd.Force` and the operator-facing query param), not `skipEpisodeLock` or any other implementation-language name. The watcher implementation uses it both to decide whether to bypass the `red × red` skip AND to decide which task-ID helper to call.
- Cursor save semantics are unchanged: end-of-cycle write on success, `IncPollCycle("success")` increment unchanged, warning log on save failure unchanged.
- All existing tests in `watcher/github-build/` must still pass. Tests that invoke `Poll(ctx)` are updated to `Poll(ctx, false)` to preserve current behavior in the non-force path.
- `libtime.CurrentDateTimeGetter` from `github.com/bborbe/time` is the only allowed clock source in business logic (see `~/.claude/plugins/marketplaces/coding/docs/go-time-injection.md`). No direct `time.Now()` in watcher, helper, executor, factory, or handler code.
- Query-param boolean parsing uses `libparse.ParseBoolDefault` from `github.com/bborbe/parse` (see `~/.claude/plugins/marketplaces/coding/docs/go-parse-pattern.md`). Unparseable `force` values resolve to `false`; the handler MUST NOT return 400 on bad `force` syntax.
- CQRS layering rules from `~/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` apply: HTTP handler stays thin (parse → publish); business logic lives in the executor and watcher.
- Counterfeiter mocks under `watcher/github-build/mocks/` MUST be regenerated when interface signatures change (`Watcher.Poll`, `buildWatcher` factory, anything new). Regeneration via `go generate ./...` is part of `make precommit`.
- Reference patterns (read-only): spec 069 sibling worktree at `~/Documents/workspaces/maintainer-trigger-force/`; spec 071 sibling worktree at `~/Documents/workspaces/maintainer-release-force/`. Domain background in `docs/build-watcher.md` (state-machine table, episode-SHA semantics).

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility | Concurrency |
|---------|-----------|-------------------|----------|---------------|-------------|
| `force=true` on a red repo, executor and watcher succeed | `IncTaskPublished` increments; new vault file appears with salted identifier | New `CreateTaskCommand` published with salted `TaskIdentifier`; controller writes new vault file; prior file untouched | None — intended | Reversible only by manually deleting the new vault file | Two `force=true` calls land on the same partition; consumer is single-threaded per spec 068 — executed serially |
| `force=true` on a green repo | `IncStateTransition` not triggered for `red × red`; `green × green` no-op fires | Identical to `force=false` path on a green repo: no publish, cursor saved | None — handled | n/a | Same as today |
| `force=true` on a repo with `undefined` state (zero qualifying runs) | Watcher's `currState == "undefined"` early-return at `watcher.go:201-203` fires | Skipped before `applyStateMachine`; no publish, no metric on transition | Operator waits for real workflow runs | n/a | Same as today |
| `force=true` but Kafka publish of `TriggerBuildCheckCommand` fails in handler | Sender returns error; handler returns HTTP 502 | Same as today; no executor invocation, no watcher cycle, no metric | Operator retries | Reversible | n/a |
| `force=true` accepted but executor crashes mid-cycle (e.g. SIGKILL after publish but before metric increment) | Framework emits Failure on result topic; Kafka redelivers | Command redelivered; watcher re-derives nonce from current time; new salted ID differs from crashed attempt → controller creates one vault file under the second ID; the first ID never produced a file. At-least-once delivery: one nonce consumed without a file, the second produces it | None — handled by framework | Partial — at-least-once: one identifier consumed without a vault file; user-visible state is the second identifier's file. Not corruption | n/a |
| `force=garbage` (unparseable) | `libparse.ParseBoolDefault` returns default `false` | Treated as `force=false`; HTTP 202 with non-force command published. Pinned in Constraints; unit test asserts the lenient path | None — handled | Reversible (operator retries with `force=true`) | n/a |
| Two `force=true` triggers fire within one microsecond | Both reach executor; nonce derived from `UnixMicro` may collide | Both produce the same salted `TaskIdentifier`; second is deduped by the controller's existing file-exists skip; only one new vault file | Operator retries the second after a short delay | Reversible — the second silently no-ops, no corruption | Serial executor processing; collision only possible if two commands' processing instants land in the same microsecond, which is below human trigger cadence |
| `force=true` after recent state-machine cleanup (e.g. cursor file deleted out from under the pod) | `LoadCursor` returns error, watcher returns wrapped error | Cycle aborts; framework retries the command. Once cursor is rebuildable, cycle proceeds; the now-cold-start treats the repo as `green` initially so the next observation is `green × red` (canonical path) — `force` becomes irrelevant for that one cycle | None — degenerates to canonical path safely | Reversible | n/a |
| Forced publish on a repo where `.maintenance.yaml` lookup fails | `LoadOverrides` already silently falls through to env defaults (per `docs/build-watcher.md` failure modes) | Forced publish proceeds with env defaults; same as today's `green × red` failure-mode behavior | None — handled | n/a | n/a |
| Clock jump backward between two `force=true` triggers | Time-derived nonce may decrease | Two distinct nonces still likely (resolution finer than realistic jumps); on collision, see "two triggers within one microsecond" row | None — handled | Reversible | n/a |

## Security / Abuse Cases

- `/trigger` is mounted under `/admin/maintainer-watcher-github-build/`. Gating is unchanged; only authenticated operators reach this handler.
- An operator with `/admin` access can flood `force=true` to create unbounded vault files for the same red episode. Mitigation is existing `/admin` access control plus the sibling rate-limiting task — not this spec.
- `force` is a single boolean parsed by the library helper — no string-interpolation or injection surface.
- The salted identifier is still a UUID5 — same shape, length, wire encoding as the canonical one. Downstream code that treats `TaskIdentifier` as an opaque UUID is unaffected.
- The time-derived nonce is not security-sensitive — it is not a secret or a token, only needs uniqueness across human-trigger cadence. Predictability is acceptable.
- The `!` separator in the salted key cannot appear in canonical input (GitHub disallows `!` in owner/repo names; SHAs are hex). No way for an attacker who controls owner/repo to spoof a salted identifier by injecting `!` into the canonical input.

## Acceptance Criteria

- [ ] `cd watcher/github-build && make precommit` exits 0 — evidence: exit code 0
- [ ] A new exported helper exists in `watcher/github-build/pkg/taskid.go` with signature `func DeriveTaskIDForce(owner, repo, episodeSHA, nonce string) uuid.UUID` — evidence: `grep -nE 'func DeriveTaskIDForce' watcher/github-build/pkg/taskid.go` returns one line; signature matches exactly
- [ ] Unit test `TestDeriveTaskIDForce_DiffersFromCanonical` (or Ginkgo equivalent) asserts that for the same `(owner, repo, episodeSHA)` inputs, `DeriveTaskID` and `DeriveTaskIDForce(..., nonce="x")` produce different UUIDs — evidence: `go test ./pkg -run DeriveTaskIDForce -v` prints PASS for the case
- [ ] Unit test `TestDeriveTaskIDForce_StableForSameNonce` asserts identical inputs (including nonce) return equal UUIDs — evidence: assertion in test source; test PASS
- [ ] Unit test `TestDeriveTaskIDForce_DiffersAcrossNonces` asserts that different nonces with the same `(owner, repo, episodeSHA)` return different UUIDs — evidence: assertion in test source; test PASS
- [ ] `Watcher.Poll` signature is `Poll(ctx context.Context, force bool) error` — evidence: `grep -nE 'Poll\(ctx context\.Context, force bool\) error' watcher/github-build/pkg/watcher.go` returns the interface line; the implementation matches
- [ ] Executor unit test `TestExecutor_ForceTrueCallsPollForceTrue`: receiving a command with `Force: true` invokes `watcher.Poll(ctx, true)` exactly once — evidence: counterfeiter mock invocation count + second-argument capture assertion
- [ ] Executor unit test `TestExecutor_ForceFalseCallsPollForceFalse`: receiving a command with `Force: false` invokes `watcher.Poll(ctx, false)` exactly once — evidence: counterfeiter mock invocation count + second-argument capture assertion
- [ ] Watcher behaviour test: with cursor pre-populated so `repoState.LastKnownState == "red"` and the fake GitHub client returning a failing run, `Poll(ctx, false)` does NOT call `createSender.SendCommand`; `Poll(ctx, true)` DOES call it exactly once — evidence: counterfeiter mock invocation count on the `task.CreateCommandSender` mock
- [ ] Watcher behaviour test: with the same red-red fixture, `Poll(ctx, true)`'s emitted `CreateTaskCommand` carries a `TaskIdentifier` *different* from `pkg.DeriveTaskID(owner, repo, episodeSHA)` for the same inputs — evidence: captured `CreateTaskCommand.TaskIdentifier` assertion
- [ ] Watcher behaviour test: two `Poll(ctx, true)` invocations against the same red-red fixture with the injected clock advanced by ≥1 microsecond between calls produce two distinct `TaskIdentifier` values — evidence: two captured commands; assertion on identifier inequality
- [ ] Watcher behaviour test: `Poll(ctx, false)` on a `green × red` fixture produces a `CreateTaskCommand` byte-identical to master HEAD for the same fixture (canonical `DeriveTaskID`, same overrides, same payload) — evidence: deep-equal assertion against the master-HEAD-captured `CreateTaskCommand` fixture
- [ ] Watcher behaviour test: `Poll(ctx, true)` saves the cursor at end-of-cycle (same as `Poll(ctx, false)`) — evidence: cursor file diff or counterfeiter mock invocation count if cursor saver is mocked
- [ ] Handler unit test `TestHandler_ParsesForceTrue`: `POST /trigger?force=true` results in `sender.SendCommand` invoked once with `TriggerBuildCheckCommand{Force: true}` — evidence: counterfeiter mock argument capture
- [ ] Handler unit test `TestHandler_ParsesForceFalseAndAbsent`: `POST /trigger?force=false` and `POST /trigger` (no param) both produce `Force: false` — evidence: two captured commands; argument assertions
- [ ] Handler unit test `TestHandler_ParsesForceGarbage`: `POST /trigger?force=banana` returns HTTP 202 (NOT 400) and publishes `Force: false` — evidence: response status assertion + captured command assertion + negative assertion on HTTP 400
- [ ] Handler v(2) success log includes the substring `force=true` for a forced request and `force=false` otherwise — evidence: captured glog output via test buffer; substring assertion
- [ ] No `time.Now()` call exists in business-logic code: `grep -rnE '\btime\.Now\(\)' watcher/github-build/pkg/` returns zero matches outside `*_test.go` files — evidence: grep exits 1 (no matches) when `--include='*.go' --exclude='*_test.go'` is applied, or the single-pipe variant `grep -rnE '\btime\.Now\(\)' watcher/github-build/pkg/ | grep -v _test.go` is empty
- [ ] Reserved-unread comment blocks are rewritten: `grep -nE 'reserved-unread|reserved.*Force|Force.*reserved|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' watcher/github-build/pkg/command/trigger_build_check_command.go watcher/github-build/pkg/command/trigger_build_check_executor.go watcher/github-build/pkg/handler/trigger_handler.go` returns zero matches — evidence: grep exit code 1 (Scope-only references like "Scope is reserved" MAY remain; the grep above does NOT match those because none of the patterns include the word "Scope")
- [ ] `git diff master -- watcher/github-build/pkg/taskid.go` shows the `DeriveTaskID` function body and `buildWatcherNamespace` constant unchanged — only additions for `DeriveTaskIDForce` — evidence: diff inspection
- [ ] `git diff master -- watcher/github-build/pkg/watcher.go` shows the `red × red` skip arm structurally unchanged for the `force=false` branch; the only logic addition is the conditional fall-through when `force=true` — evidence: diff inspection
- [ ] `git diff --stat master` shows changes confined to `watcher/github-build/`, the regenerated mocks under `watcher/github-build/mocks/`, and `CHANGELOG.md` — evidence: diff inspection
- [ ] `CHANGELOG.md` at repo root contains a new bullet under `## Unreleased` describing the `?force=true` query parameter and naming the spec number — evidence: `awk '/^## Unreleased/,/^## /' CHANGELOG.md | grep -ni "force" | head -1` returns ≥1 line (anchors the grep inside the Unreleased section, not just anywhere in the file)

**Scenario coverage:** none. All paths reachable via unit tests with counterfeiter-backed mocks (GitHub client, create-sender, cursor I/O, clock). No Docker, no `gh`, no cluster required. Manual post-deploy operator smoke is documented in Verification as informational, not gating.

## Verification

```
cd watcher/github-build
make precommit
go test ./pkg -run 'DeriveTaskIDForce|Poll' -v
go test ./pkg/command -run 'Force' -v
go test ./pkg/handler -run 'Force' -v
grep -nE 'func DeriveTaskIDForce' pkg/taskid.go
grep -nE 'Poll\(ctx context\.Context, force bool\) error' pkg/watcher.go
! grep -rnE '\btime\.Now\(\)' pkg/ --include='*.go' --exclude='*_test.go'
! grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' pkg/command/trigger_build_check_command.go pkg/command/trigger_build_check_executor.go pkg/handler/trigger_handler.go
cd ../..
awk '/^## Unreleased/,/^## /' CHANGELOG.md | grep -ni "force"
```

Expected:
- `make precommit` exits 0
- All named test cases print PASS
- Grep for `DeriveTaskIDForce` returns exactly one line
- Grep for new `Poll` signature returns at least one line
- `! grep` for `time.Now()` exits 0 (the negation succeeds only when grep itself found nothing)
- `! grep` for stale comment patterns exits 0
- CHANGELOG grep returns ≥1 line

**Verification rungs (per `docs/verifying-specs.md`):**
- Rung 1 (unit tests via `make precommit`) is the primary and sufficient surface. All behavior changes (handler parse, executor `Force` branch, watcher `Poll` signature, state-machine force fall-through, helper derivation, clock injection) are reachable by unit tests with mocked sender, mocked GitHub client, mocked clock, and mocked create-sender.
- Rung 2 (dev k8s deploy + e2e) is NOT required. No new env var, no Dockerfile change, no k8s manifest change beyond what spec 068 shipped. The `Force` field is already on the wire schema.
- Rung 3 (prod) — operator-initiated manual verification (hitting prod `/admin/maintainer-watcher-github-build/trigger?force=true` on an already-red repo and observing a new vault file appear under `tasks/Build Failure github - <owner>-<repo> - <sha7>*.md` with a distinct task-identifier suffix) is OPTIONAL post-deploy validation, not a required AC. Record it in the spec log on first prod use.

## Suggested Decomposition

Three concerns: pure helper, cross-layer watcher change (signature + state machine + executor + factory + clock injection + call-site updates + mock regen), thin handler edge change + comments + changelog.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Add `DeriveTaskIDForce` helper in `pkg/taskid.go` reusing `buildWatcherNamespace` with key format `<owner>/<repo>#build-<episodeSHA>!<nonce>`. Pure additive change. Add three table-test cases (`DiffersFromCanonical`, `StableForSameNonce`, `DiffersAcrossNonces`). No callers wired yet. | 5 (partial: helper exists) | helper-existence AC, three `DeriveTaskIDForce_*` test ACs, `pkg/taskid.go` diff AC | — |
| 2 | Change `Watcher.Poll` to `Poll(ctx context.Context, force bool) error`. Add `force` parameter to `applyStateMachine` and add the `red × red && force` fall-through that mirrors the `green × red` publish arm but calls `DeriveTaskIDForce` with a microsecond nonce from injected `libtime.CurrentDateTimeGetter`. Plumb the clock through `NewWatcher` and the factory. Update the executor to forward `cmd.Force` to `Poll`. Update every existing `Poll(ctx)` call site (`main.go`, `cmd/run-once`, poll-loop and watcher tests) to `Poll(ctx, false)` for byte-identity. Rewrite the `runTriggerBuildCheck` doc block to drop the "Scope or Force — both are reserved-unread" line (Scope still reserved-unread; only the joint phrasing changes). Regenerate counterfeiter mocks. | 3, 4, 5 (full), 6, 7, 8 | `Watcher.Poll` signature AC, two executor ACs, four watcher behaviour ACs (red-red skip vs publish, salted ID, distinct IDs across clock advance, byte-identical non-force, cursor save), `time.Now()` grep AC, executor-comment half of the reserved-unread grep AC, `pkg/watcher.go` diff AC, `git diff --stat` confinement AC | prompt 1 |
| 3 | Parse `force` query param in `pkg/handler/trigger_handler.go` via `libparse.ParseBoolDefault`. Set `Force` on the published command. Update the handler v(2) log to include `force=%t`. Rewrite the `TriggerBuildCheckHandler` doc block (drop the "No request body or query string is consumed (both Scope and Force are reserved-unread)" sentence; keep the Scope-only mention). Rewrite the `TriggerBuildCheckCommand.Force` doc block on the struct so it reflects the wired behaviour (Scope's reserved-unread paragraph stays). Add CHANGELOG bullet under `## Unreleased` mentioning `force` and this spec number. | 1, 2, 9 | three handler test ACs, handler-log AC, struct-and-handler halves of the reserved-unread grep AC, `make precommit`, CHANGELOG AC | prompt 2 |

Rationale: prompt 1 is the smallest reviewable diff (one helper + table tests, no wiring). Prompt 2 owns the cross-layer surgery (interface change, state-machine fall-through, clock injection, mock regen, all call-site updates) — this is the riskiest seam and gets its own prompt to keep review focus tight, with byte-identity for the non-force path explicitly asserted. Prompt 3 is the HTTP-edge change, fully reachable once the executor accepts and acts on `Force`; bundling the changelog and the user-facing doc-comment rewrites here keeps the user-visible-behaviour commit and the release note in the same prompt. Ordering prevents cycles: prompt 2 cannot test "force publishes with salted ID" without the helper from prompt 1; prompt 3 cannot meaningfully assert end-to-end handler-to-state-machine behaviour without the executor branch and watcher signature from prompt 2.

## Do-Nothing Option

Leave `/trigger` as-is on github-build. The parent vault task `Add Force Flag to Maintainer Watcher Trigger Endpoints` remains two-thirds done (PRs #55 and #56 ship for github-pr and github-release respectively) but with the asymmetric, surprising gap that the build watcher — the noisiest of the three in practice — has no force knob. Operators recovering from a deleted vault file, a prompt-template change, or a fix-agent config tweak continue to have only ugly workarounds: pushing a no-op commit to change the SHA (which mutates the actual repo!), restarting the pod (loses the cursor and re-floods other repos), or manually publishing a CreateTaskCommand directly to Kafka (bypasses the watcher's title/format/override logic). The `Force` field is already on the command wire schema (spec 068) explicitly anticipating this spec; leaving it reserved-unread indefinitely is dead code on the contract and the third sibling spec it was scoped against. Not acceptable as a steady state.
