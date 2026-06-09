---
status: approved
spec: ["069"]
created: "2026-06-09T20:30:00Z"
queued: "2026-06-09T20:18:04Z"
---

<summary>
- Threads a `force` boolean from the executor through `Watcher.Poll` into `applyStateMachine`
- On `red × red` the state machine today no-ops; with `force=true` it now falls through to the same publish path used by `green × red`, but using a salted task identifier so the agent controller does not dedup-skip it
- The salted task identifier consumes a microsecond-precision nonce derived from an injected clock; no `time.Now()` in business logic
- The clock is plumbed through `factory.CreateWatcher` and `NewWatcher` so tests can advance it deterministically
- Every call site of `Poll` is updated in this same prompt — production binary, run-once smoke binary, watcher test suite, and poll-loop test — so the compile stays green
- Counterfeiter mocks for `Watcher` are regenerated to match the new signature
- The non-force code path is asserted byte-identical to current master via a captured fixture
</summary>

<objective>
Make the github-build watcher honour `force=true` end-to-end inside the in-pod consumer:

1. `Watcher.Poll(ctx context.Context, force bool) error` — interface + implementation
2. `applyStateMachine` gains a `force` parameter; the `red × red` arm gains a `&& force` fall-through that mirrors the `green × red` publish path but calls `DeriveTaskIDForce` with a microsecond nonce from `libtime.CurrentDateTimeGetter`
3. `NewWatcher` and `factory.CreateWatcher` accept a `libtime.CurrentDateTimeGetter`; `main.go` and `cmd/run-once/main.go` wire `libtime.NewCurrentDateTime()` at the top
4. The executor reads `cmd.Force` and calls `watcher.Poll(ctx, cmd.Force)`
5. Every existing `Poll(ctx)` call site becomes `Poll(ctx, false)` so the non-force path is byte-identical to master
6. Counterfeiter mocks under `watcher/github-build/mocks/` are regenerated

Non-force paths emit byte-identical `CreateTaskCommand` payloads to current master HEAD — this is checked by capturing a fixture from master BEFORE editing the state machine.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these coding plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — executor exit-path mapping, fire-and-forget conventions
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md` — `libtime.CurrentDateTimeGetter` injection rules (no `time.Now()` in business logic)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — factory composition, zero logic
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo + counterfeiter
- `/home/node/.claude/plugins/marketplaces/coding/docs/dod.md`

Read the files this prompt changes (verify before writing):
- `watcher/github-build/pkg/watcher.go` — `Watcher` interface at line 53, `NewWatcher` at line 78, `buildWatcher` struct at line 108, `Poll` at line 123, `pollRepo` at line 164, `applyStateMachine` at line 210 (note the `red × red` arm at line 249 is the empty no-op being modified)
- `watcher/github-build/pkg/taskid.go` — `DeriveTaskIDForce` from prompt 1 (verify present)
- `watcher/github-build/pkg/command/trigger_build_check_executor.go` — `NewTriggerBuildCheckCommandExecutor` and `runTriggerBuildCheck`; current `w.Poll(ctx)` at line 68
- `watcher/github-build/pkg/factory/factory.go` — `CreateWatcher` at line 72; do NOT change `CreateSyncProducer`, `CreateKafkaCreateSender`, `CreateCommandConsumer`, `CreateTriggerBuildCheckCommandSender`, `CreateAllowlistSnapshot`
- `watcher/github-build/main.go` — `application.Run` at line 72; `factory.CreateWatcher` call site at line 119; `pollOnce` closure at line 178 calls `w.Poll(ctx)` at line 181
- `watcher/github-build/cmd/run-once/main.go` — `Application.Run` at line 78; `a.CreateWatcher` call at line 123; `w.Poll(ctx)` at line 142; `WatcherFactory` type alias at line 63
- `watcher/github-build/main_poll_loop_test.go` — uses `app.runPollLoop(pollFunc, ...)` where `pollFunc` matches `run.Func` (signature is `func(ctx context.Context) error`). This file does NOT call `Watcher.Poll` directly — its `pollFunc` is a local closure, so no signature change is needed here. Re-verify by reading the file in this prompt; if a direct `Watcher.Poll` call is found, update it to `Poll(ctx, false)`.
- `watcher/github-build/pkg/watcher_test.go` — contains 30+ `w.Poll(ctx)` invocations that must all become `w.Poll(ctx, false)` (force=false preserves current behaviour for every existing test). Use grep+sed-style mechanical replacement; do NOT add force=true assertions in those existing tests.
- `watcher/github-build/mocks/watcher.go` — counterfeiter-generated. Must be regenerated, not hand-edited.

Read the sibling reference patterns (READ-ONLY — separate worktrees):
- `/Users/bborbe/Documents/workspaces/maintainer-trigger-force/watcher/github-pr/pkg/command/trigger_pr_review_executor.go` — see `publishCreateCommand`: `nonce := strconv.FormatInt(currentDateTime.Now().UnixMicro(), 10)` then `if cmd.Force { ... DeriveTaskIDForce ... } else { ... DeriveTaskID ... }`
- `/Users/bborbe/Documents/workspaces/maintainer-trigger-force/watcher/github-pr/main.go` line 205 — `currentDateTime := libtime.NewCurrentDateTime()` wiring
- `/Users/bborbe/Documents/workspaces/maintainer-release-force/watcher/github-release/pkg/watcher.go` — `Poll(ctx context.Context, skipSHAUnchanged bool) error` is the rhyme for github-build's new signature (different name `force`, same shape)

Key facts (verified):
- `libtime` import path: `libtime "github.com/bborbe/time"`. Constructor `libtime.NewCurrentDateTime()` returns a `libtime.CurrentDateTimeGetter`.
- The `Now()` method on the getter returns `libtime.DateTime` which has a `.Time()` accessor returning `time.Time`. To call `.UnixMicro()` on the underlying `time.Time`, use `currentDateTime.Now().Time().UnixMicro()`. If your verification grep shows the sibling github-pr executor uses `currentDateTime.Now().UnixMicro()` directly, the sibling is calling `UnixMicro()` on `libtime.DateTime` (which embeds `time.Time` and thus exposes its methods). Mirror whichever form actually compiles in this worktree — confirm by checking the sibling file's actual line and the libtime package shape.
- Import for nonce formatting: `"strconv"`.
- `task.CreateCommandSender.SendCommand(ctx, cmd)` is the existing publish call inside `applyStateMachine`'s `green × red` arm at line 239.
- Counterfeiter regeneration is triggered by `go generate ./...` per the `//counterfeiter:generate` directive at `watcher/github-build/pkg/watcher.go:25`. `make precommit` runs `go generate` for you.
- The executor's success log already includes `force=%t` (see `trigger_build_check_executor.go:73` and `:78`) — DO NOT touch those two log lines; the AC says they are already correct.
- `runTriggerBuildCheck`'s doc block (the comment starting at line 23) currently says `does NOT read Scope or Force — both are reserved-unread`. THIS prompt rewrites that comment to say only `Scope` is reserved-unread; the handler-side / struct-side comments are owned by prompt 3.
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

### Phase A — Capture the master-HEAD `CreateTaskCommand` fixture BEFORE editing the state machine

This is load-bearing for AC 102 (byte-identical non-force path). If you skip it and the test fails at the end, you will not be able to tell whether your edit caused drift or the fixture was wrong.

1. **Before changing any production code**, write a small reproducer test that exercises the existing `green × red` happy path through master-HEAD's unmodified state machine, captures the produced `task.CreateCommand`, and persists it as a fixture file. Add this as a new Ginkgo spec at `watcher/github-build/pkg/watcher_byteidentity_test.go` (external `pkg_test` package). The fixture format is up to you (deep-equal-able Go value persisted via `json.Marshal` to `testdata/byteidentity_green_to_red.json` is the recommended shape). Constraints on the fixture capture step:
   - Use the existing watcher_test.go fixtures as your input shape — a cursor with `LastKnownState == ""` (or `"green"`) for the repo, a fake `GitHubClient` returning one failing run.
   - Wire `NewWatcher` with `libtime.CurrentDateTimeGetter` set to `libtime.NewCurrentDateTime()` (real clock) — the canonical path does not call the clock, so the fixture is deterministic regardless.
   - Capture `task.CreateCommand` via the `task.CreateCommandSender` counterfeiter mock's `SendCommandArgsForCall(0)`.
   - Persist the captured payload to a fixture file under `watcher/github-build/pkg/testdata/`.
   - Run `make precommit` and confirm the capture passes BEFORE step 2.
   - In a later step you will assert deep-equality against this fixture from a `Poll(ctx, false)` invocation.

   This step exists because once you edit `applyStateMachine` you can no longer trivially diff against master. Capture first; edit second.

### Phase B — Surgery

2. **Update `watcher/github-build/pkg/watcher.go`**:

   a. **Interface change** at line 51-54:
      ```go
      // Watcher polls GitHub Actions for build status changes.
      // When force is true, the cycle bypasses the red×red episode lock and
      // emits a salted TaskIdentifier (see DeriveTaskIDForce). Every other
      // state-machine transition is unaffected.
      type Watcher interface {
          Poll(ctx context.Context, force bool) error
      }
      ```

   b. **Import `libtime`** alongside the existing imports:
      ```go
      libtime "github.com/bborbe/time"
      ```
      and `"strconv"` (for the nonce formatter).

   c. **`buildWatcher` struct** — add a field for the clock:
      ```go
      type buildWatcher struct {
          // ... existing fields ...
          currentDateTime libtime.CurrentDateTimeGetter
      }
      ```

   d. **`NewWatcher` signature** — append `currentDateTime libtime.CurrentDateTimeGetter` as the final parameter, and pass it through to the struct literal:
      ```go
      func NewWatcher(
          githubClient GitHubClient,
          createSender task.CreateCommandSender,
          metrics Metrics,
          repoFilter filter.RepoFilter,
          allowlist AllowlistSnapshot,
          cursorPath string,
          assignee string,
          taskStatus string,
          taskPhase string,
          maintenanceLoader maintenance.Loader,
          maxTitleLen int,
          taskSuffix string,
          currentDateTime libtime.CurrentDateTimeGetter,
      ) Watcher {
          return &buildWatcher{
              // ... existing fields ...
              currentDateTime: currentDateTime,
          }
      }
      ```

   e. **`Poll` signature** at line 123 — add the `force bool` parameter and thread it through `pollRepo` → `applyStateMachine`. The cursor save, metric increment, and snapshot iteration are unchanged:
      ```go
      func (w *buildWatcher) Poll(ctx context.Context, force bool) error {
          // ... unchanged through ...
          if rateLimited := w.pollRepo(ctx, cursor, repoKey, force); rateLimited {
              break
          }
          // ... unchanged ...
      }
      ```

   f. **`pollRepo` signature** at line 164 — add `force bool` parameter and forward it to `applyStateMachine`:
      ```go
      func (w *buildWatcher) pollRepo(ctx context.Context, cursor *Cursor, repoKey string, force bool) bool {
          // ... unchanged ...
          w.applyStateMachine(ctx, repoKey, repoState, currState, episodeSHA, failingRuns, owner, repo, force)
          return false
      }
      ```

   g. **`applyStateMachine` change** at line 210 — add `force bool` as the final parameter. The `green × red` arm at line 221-247 stays exactly as-is (it calls the canonical `DeriveTaskID`). Modify the `red × red` arm at line 249 to gate on `force`:

      ```go
      case prevState == "red" && currState == "red" && !force:
          // Episode locked on first red; skip regardless of SHA change

      case prevState == "red" && currState == "red" && force:
          // Operator-requested re-publish. Mirror the green×red publish arm
          // but salt the TaskIdentifier so the controller's idempotent
          // create-task dedup does not skip the new vault file. The
          // episode lock semantics (repoState.LastKnownState stays "red",
          // CurrentEpisodeSHA unchanged) are preserved — this branch does
          // not advance the state machine, it just emits one extra task.
          overrides := w.maintenanceLoader.LoadOverrides(ctx, owner, repo, repoState.DefaultBranch)
          effectiveAssignee := coalesceString(overrides.Assignee, w.assignee)
          effectiveStatus := coalesceString(overrides.Status, w.taskStatus)
          effectivePhase := coalesceString(overrides.Phase, w.taskPhase)
          nonce := strconv.FormatInt(w.currentDateTime.Now().UnixMicro(), 10)
          taskID := DeriveTaskIDForce(owner, repo, episodeSHA, nonce)
          cmd := w.buildCreateTaskCommand(
              ctx,
              taskID,
              owner,
              repo,
              episodeSHA,
              failingRuns,
              effectiveAssignee,
              effectiveStatus,
              effectivePhase,
              overrides.IncludeLogs,
          )
          if err := w.createSender.SendCommand(ctx, cmd); err != nil {
              glog.Errorf("publish create-task (force) failed repo=%s err=%v", repoKey, err)
              w.metrics.IncPollError("kafka_error")
              return // do NOT mutate cursor — next force-retry can re-publish
          }
          w.metrics.IncTaskPublished()
          w.metrics.IncStateTransition("green_to_red")
          // Intentionally do NOT mutate repoState.LastKnownState or
          // CurrentEpisodeSHA — the episode-lock state machine is
          // unchanged; force just emits an extra task in this cycle.
      ```

      Important details verified against the spec:
      - The forced publish reuses the SAME metric labels as `green × red` (`IncTaskPublished`, `IncStateTransition("green_to_red")`) — spec Non-goal: "Do NOT add a new metric label for force cycles."
      - The clock call MUST go through `w.currentDateTime.Now()`. No direct `time.Now()`. If `w.currentDateTime.Now()` returns `libtime.DateTime`, call `.UnixMicro()` on it directly if libtime exposes it (the sibling github-pr executor does); otherwise `.Time().UnixMicro()`. Verify which form compiles in this worktree by reading the sibling file and `vendor/github.com/bborbe/time/` if present, then commit to the chosen form — do NOT leave both.
      - The split into two separate `case` arms is deliberate: it keeps the `!force` arm visually identical to today (still an empty body — the comment is preserved). A combined `case prevState == "red" && currState == "red":` body with an `if force { ... }` branch would also work; pick whichever passes `gofmt` + linter cleanly. State only the chosen form in the final code.

   h. **Doc-comment** on `applyStateMachine` — extend the existing one-liner so the `force` parameter is documented:
      ```go
      // applyStateMachine applies the green/red state machine for a single repo.
      // When force is true, the red×red arm falls through to a salted publish
      // (see DeriveTaskIDForce); every other transition is unaffected.
      ```

3. **Update `watcher/github-build/pkg/command/trigger_build_check_executor.go`**:

   a. **Doc-block** on `runTriggerBuildCheck` (lines 47-56 in current master) — rewrite the line that reads `does NOT read Scope or Force — both are reserved-unread` so it ONLY mentions Scope:

      Old (current text):
      ```
      // The executor does NOT read Scope or Force — both are reserved-unread
      // (spec Non-goal: per-repo filter UX and Force flag are separate specs).
      ```

      New:
      ```
      // The executor reads cmd.Force and forwards it to Watcher.Poll. Scope
      // remains reserved-unread (spec Non-goal: per-repo filter UX is a
      // separate spec).
      ```

      The block-comment above `NewTriggerBuildCheckCommandExecutor` also contains a "Force" mention if any — re-verify by reading the file, and update only the Force-reservation phrasing. Scope-only references stay.

   b. **`runTriggerBuildCheck` body** — change line 68 from `if err := watcher.Poll(ctx); err != nil {` to `if err := watcher.Poll(ctx, cmd.Force); err != nil {`.

   c. **Do NOT touch the two existing v(2) log lines** at the bottom of `runTriggerBuildCheck` (lines 73 and 77 in current master). They already include `force=%t` — verified.

   d. **AC pin**: after this edit, `grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' watcher/github-build/pkg/command/trigger_build_check_executor.go` MUST return zero matches.

4. **Update `watcher/github-build/pkg/factory/factory.go`** — `CreateWatcher` (line 72):

   - Add a new final parameter `currentDateTime libtime.CurrentDateTimeGetter` of type `libtime "github.com/bborbe/time"`
   - Forward it to `pkg.NewWatcher` in the constructor call at line 94-107
   - Do NOT change any other `factory.*` function in this file

   ```go
   import (
       // ... existing imports ...
       libtime "github.com/bborbe/time"
   )

   func CreateWatcher(
       ctx context.Context,
       ghClient pkg.GitHubClient,
       brokers libkafka.Brokers,
       stage string,
       inputAllowlist []string,
       resolved pkg.AllowlistSnapshot,
       cursorPath string,
       assignee string,
       taskStatus string,
       taskPhase string,
       maxTitleLen int,
       taskSuffix string,
       currentDateTime libtime.CurrentDateTimeGetter,
   ) (pkg.Watcher, libkafka.SyncProducer, func(), error) {
       // ... existing body ...
       w := pkg.NewWatcher(
           ghClient,
           createSender,
           pkg.NewMetrics(),
           repoFilter,
           resolved,
           cursorPath,
           assignee,
           taskStatus,
           taskPhase,
           maintenanceLoader,
           maxTitleLen,
           taskSuffix,
           currentDateTime,
       )
       return w, syncProducer, producerCleanup, nil
   }
   ```

5. **Update `watcher/github-build/main.go`**:

   a. **Import** `libtime "github.com/bborbe/time"`.
   b. **Inside `application.Run`** (line 72), construct `currentDateTime := libtime.NewCurrentDateTime()` before the `factory.CreateWatcher` call site (line 119). Pass it as the final argument:
      ```go
      currentDateTime := libtime.NewCurrentDateTime()

      w, syncProducer, watcherCleanup, err := factory.CreateWatcher(
          ctx,
          ghClient,
          a.KafkaBrokers,
          a.Stage,
          repoAllowlist,
          resolved,
          "/data/cursor.json",
          a.BuildAssignee,
          a.BuildTaskStatus,
          a.BuildTaskPhase,
          a.MaxTitleLen,
          a.TaskSuffix,
          currentDateTime,
      )
      ```
   c. **`pollOnce` closure** at line 178-183 — current body `return w.Poll(ctx)` becomes `return w.Poll(ctx, false)`. The poll-interval loop always passes `false` — only the /trigger HTTP path (via the command consumer + executor) propagates `force=true`.

6. **Update `watcher/github-build/cmd/run-once/main.go`**:

   a. **Import** `libtime "github.com/bborbe/time"`.
   b. **`WatcherFactory` type alias** at line 63-76 — add `currentDateTime libtime.CurrentDateTimeGetter` as the final parameter to match `factory.CreateWatcher`:
      ```go
      type WatcherFactory func(
          ctx context.Context,
          ghClient pkg.GitHubClient,
          brokers libkafka.Brokers,
          stage string,
          inputAllowlist []string,
          resolved pkg.AllowlistSnapshot,
          cursorPath string,
          assignee string,
          taskStatus string,
          taskPhase string,
          maxTitleLen int,
          taskSuffix string,
          currentDateTime libtime.CurrentDateTimeGetter,
      ) (pkg.Watcher, libkafka.SyncProducer, func(), error)
      ```
   c. **Inside `Application.Run`** (line 78), construct `currentDateTime := libtime.NewCurrentDateTime()` and pass it through the `a.CreateWatcher(...)` call at line 123.
   d. **Line 142** — change `return w.Poll(ctx)` to `return w.Poll(ctx, false)`. run-once is a one-shot smoke test; it never forces.

7. **Update `watcher/github-build/main_poll_loop_test.go`** — re-read the file in this prompt. Its `pollFunc` is a local `func(ctx context.Context) error` closure (a `run.Func`), NOT a `Watcher.Poll` call. No signature change needed there UNLESS a future edit has added one. Confirm by grep:
   ```bash
   grep -n "Watcher\|w\.Poll" watcher/github-build/main_poll_loop_test.go
   ```
   If grep returns nothing, leave the file untouched. If grep returns a `Watcher.Poll`-style call, update it to `Poll(ctx, false)`.

8. **Update `watcher/github-build/pkg/watcher_test.go`** — mechanical: every `w.Poll(ctx)` call becomes `w.Poll(ctx, false)`. There are 30+ call sites at lines 76, 102, 106, 131, 134, 157, 161, 182, 188, 212, 215, 239, 258, 281, 299, 314, 369, 373, 379, 387, 395, 429, 451, 466, 492, 560, 575, 588, 601, 613, 677, 697, 715, 732, 747, 759, 763, 771 (verify with `grep -n "w.Poll(ctx)" pkg/watcher_test.go`). Use a sed-style replacement; do NOT add `force=true` cases here. Also update every test that calls `NewWatcher(...)` to pass `libtime.NewCurrentDateTime()` (or a fake getter — the existing tests do not exercise the clock so the real one is fine) as the new final argument.

   Add `libtime "github.com/bborbe/time"` to the imports of `watcher_test.go` if not already present.

### Phase C — New behavioural tests for the force path

9. **Add new test cases to `watcher/github-build/pkg/watcher_test.go`** (or a new sibling file `watcher_force_test.go` in the same `pkg_test` package — your choice; the sibling file is cleaner). The new tests MUST cover all six watcher-behaviour ACs from the spec:

   a. `Poll(ctx, false)` on a red-red fixture does NOT call `createSender.SendCommand` (the existing skip is preserved).
   b. `Poll(ctx, true)` on the same red-red fixture DOES call `createSender.SendCommand` exactly once.
   c. The captured `CreateTaskCommand` from (b) has a `TaskIdentifier` *different* from `pkg.DeriveTaskID(owner, repo, episodeSHA).String()` for the same inputs.
   d. Two `Poll(ctx, true)` invocations against the same fixture, with the injected clock advanced by ≥1 microsecond between calls, produce two captured commands with distinct `TaskIdentifier` values.
   e. `Poll(ctx, false)` on a `green × red` fixture produces a `CreateTaskCommand` deep-equal to the master-HEAD fixture you captured in Phase A.
   f. `Poll(ctx, true)` saves the cursor at end-of-cycle (existing cursor-save behaviour is unchanged in the force branch).

   For (d), inject a programmable clock — implement a tiny in-test `fakeClock` that satisfies `libtime.CurrentDateTimeGetter` and returns successive timestamps from a slice. Mirror the pattern at `~/.claude/plugins/marketplaces/coding/docs/go-time-injection.md`.

   For the executor side, also add to `watcher/github-build/pkg/command/trigger_build_check_executor_test.go` (or a sibling file in the same package):
   - `TestExecutor_ForceTrueCallsPollForceTrue`: build a `TriggerBuildCheckCommand{Force: true}`, drive `RunTriggerBuildCheck`, assert `watcher.PollCallCount() == 1` and `_, force := watcher.PollArgsForCall(0); force == true`.
   - `TestExecutor_ForceFalseCallsPollForceFalse`: same but with `Force: false`; assert `force == false`.

   Counterfeiter mock signatures: the regenerated `mocks.Watcher` will expose `PollCallCount() int`, `PollArgsForCall(int) (context.Context, bool)`, and `PollReturns(error)`. Verify after regeneration (step 10).

### Phase D — Mocks + precommit

10. **Regenerate counterfeiter mocks**:
    ```bash
    cd watcher/github-build && go generate ./...
    ```
    This regenerates `watcher/github-build/mocks/watcher.go` to match the new `Poll(ctx, force)` signature. Verify with:
    ```bash
    grep -n "PollArgsForCall\|PollCallCount\|PollReturns" mocks/watcher.go
    ```
    Expected: the regenerated file exposes the two-arg `PollArgsForCall` returning `(context.Context, bool)`.

11. **Run precommit**:
    ```bash
    cd watcher/github-build && make precommit
    ```
    Must exit 0. If the build breaks on a stale mock, re-run `go generate ./...` then `make precommit` again. If it breaks on a forgotten call site, search:
    ```bash
    grep -rn "\.Poll(ctx)" watcher/github-build/
    ```
    Every remaining hit must be updated to `Poll(ctx, false)` or `Poll(ctx, <bool>)`.

12. **Sanity-grep the new signature and the absence of `time.Now()`**:
    ```bash
    grep -nE 'Poll\(ctx context\.Context, force bool\) error' watcher/github-build/pkg/watcher.go
    # one line (interface) + matched implementation

    grep -rnE '\btime\.Now\(\)' watcher/github-build/pkg/ --include='*.go' --exclude='*_test.go'
    # zero matches
    ```

</requirements>

<constraints>
- Edit only: `watcher/github-build/pkg/watcher.go`, `watcher/github-build/pkg/command/trigger_build_check_executor.go`, `watcher/github-build/pkg/factory/factory.go`, `watcher/github-build/main.go`, `watcher/github-build/cmd/run-once/main.go`, `watcher/github-build/pkg/watcher_test.go` (and optionally a new `watcher_force_test.go` sibling), `watcher/github-build/pkg/watcher_byteidentity_test.go` (new), test files under `watcher/github-build/pkg/command/`, and the regenerated `watcher/github-build/mocks/watcher.go`. Do NOT touch `watcher/github-build/pkg/handler/`, `watcher/github-build/pkg/command/trigger_build_check_command.go`, or `CHANGELOG.md` — those belong to prompt 3.
- Do NOT commit — dark-factory handles git.
- Do NOT modify `DeriveTaskID`, `buildWatcherNamespace`, `splitRepoKey`, `deriveState`, or `buildCreateTaskCommand`. Force path reuses `buildCreateTaskCommand` unchanged.
- Do NOT add a new Prometheus metric label for force cycles. Force-publishes count as `IncTaskPublished` + `IncStateTransition("green_to_red")`.
- Do NOT mutate `repoState.LastKnownState` or `CurrentEpisodeSHA` in the force fall-through. Episode-lock state stays red.
- Do NOT add `time.Now()` anywhere in `watcher/github-build/pkg/` outside `*_test.go`. All time goes through `libtime.CurrentDateTimeGetter`.
- Do NOT use `fmt.Errorf`. Use `github.com/bborbe/errors` (`errors.Wrapf`, `errors.Errorf`).
- Do NOT branch any other state-machine arm on `force`. Only `red × red` is conditional.
- Do NOT add a per-feature opt-out flag — spec Non-goal explicitly forbids it.
- `Watcher.Poll`'s new parameter is named `force` (not `skipEpisodeLock` or anything else).
- Counterfeiter mocks are regenerated (not hand-edited) via `go generate ./...`.
- Existing tests in `watcher/github-build/` must still pass after this prompt; the `Poll(ctx)` → `Poll(ctx, false)` mechanical rewrite preserves all current assertions.
- `make precommit` runs from `watcher/github-build/`, never from repo root.
</constraints>

<verification>
cd watcher/github-build && make precommit

# Interface + impl signature:
grep -nE 'Poll\(ctx context\.Context, force bool\) error' pkg/watcher.go
# Expect: ≥1 line (interface) and the implementation matches.

# Executor forwards Force:
grep -nE 'watcher\.Poll\(ctx, cmd\.Force\)' pkg/command/trigger_build_check_executor.go
# Expect: one line.

# No time.Now() in business logic:
! grep -rnE '\btime\.Now\(\)' pkg/ --include='*.go' --exclude='*_test.go'
# Expect: exit 0 (no matches).

# Stale "reserved-unread" Force phrasing is gone from the executor:
! grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' pkg/command/trigger_build_check_executor.go
# Expect: exit 0.

# Mock has the new two-arg signature:
grep -n 'PollArgsForCall' mocks/watcher.go
# Expect: at least one line returning (context.Context, bool).

# Force-path test names exist:
go test ./pkg/command -run 'Force' -v
go test ./pkg -run 'Poll' -v
# All cases PASS.

# Byte-identity fixture test passes:
go test ./pkg -run 'byteidentity|ByteIdentity|GreenToRed' -v
# PASS.

# git diff confinement (informational — dark-factory tracks the actual confinement):
git diff --stat
# Expect changes confined to watcher/github-build/ (pkg/, cmd/, mocks/, main.go).
</verification>
