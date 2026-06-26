---
status: draft
spec: [069-force-trigger-on-github-build-watcher]
created: "2026-06-26T12:20:00Z"
branch: dark-factory/force-trigger-on-github-build-watcher
---

<summary>
- Threads a `force` boolean from the executor through `Watcher.Poll` into `applyStateMachine`
- On `red × red` the state machine today no-ops; with `force=true` it now falls through to a publish path that mirrors `green × red`, but using a salted task identifier so the agent controller does not dedup-skip it
- The salted task identifier consumes a microsecond-precision nonce derived from an injected clock; no `time.Now()` in business logic
- The clock is plumbed through `factory.CreateWatcher` and `NewWatcher` so tests can advance it deterministically
- Every call site of `Poll` is updated in this same prompt — production binary, run-once smoke binary, and watcher test suite — so the compile stays green
- Counterfeiter mocks for `Watcher` are regenerated to match the new signature
- The non-force code path stays byte-identical to today (every other state-machine arm and metric is untouched)
</summary>

<objective>
Make the github-build watcher honour `force=true` end-to-end inside the in-pod consumer:

1. `Watcher.Poll(ctx context.Context, force bool) error` — interface + implementation
2. `applyStateMachine` gains a `force` parameter; a new `prevState == "red" && currState == "red" && force` arm mirrors the `green × red` publish path but calls `DeriveTaskIDForce` with a microsecond nonce from `libtime.CurrentDateTimeGetter`
3. `NewWatcher` and `factory.CreateWatcher` accept a `libtime.CurrentDateTimeGetter`; `main.go` and `cmd/run-once/main.go` wire `libtime.NewCurrentDateTime()`
4. The executor reads `cmd.Force` and calls `watcher.Poll(ctx, cmd.Force)`
5. Every existing `Poll(ctx)` call site becomes `Poll(ctx, false)` so the non-force path is byte-identical to master
6. Counterfeiter mocks under `watcher/github-build/mocks/` are regenerated
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these coding plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — executor exit-path mapping, fire-and-forget conventions
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md` — `libtime.CurrentDateTimeGetter` injection rules (no `time.Now()` in business logic)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — factory composition, zero logic
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo + counterfeiter
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`

Read the files this prompt changes (verify before writing):
- `watcher/github-build/pkg/watcher.go` — `Watcher` interface, `NewWatcher`, `buildWatcher` struct, `Poll`, `pollRepo`, `applyStateMachine` (the `red × red` arm is the empty no-op being modified). Prompt 1 added `DeriveTaskIDForce` here's sibling file `taskid.go` (verify present).
- `watcher/github-build/pkg/taskid.go` — `DeriveTaskIDForce` from prompt 1. If missing, STOP and report `failed` with `"DeriveTaskIDForce not yet deployed (prompt 1)"`.
- `watcher/github-build/pkg/command/trigger_build_check_executor.go` — `NewTriggerBuildCheckCommandExecutor` and `runTriggerBuildCheck`; current `watcher.Poll(ctx)` call.
- `watcher/github-build/pkg/factory/factory.go` — `CreateWatcher`. Do NOT change `CreateSyncProducer`, `CreateKafkaCreateSender`, `CreateCommandConsumer`, `CreateTriggerBuildCheckCommandSender`, `CreateTriggerBuildCheckHandler`, `CreateAllowlistSnapshot`.
- `watcher/github-build/main.go` — `application.Run`; the `factory.CreateWatcher` call site; the `pollOnce` closure calls `w.Poll(ctx)`.
- `watcher/github-build/cmd/run-once/main.go` — `Application.Run`; the `a.CreateWatcher(...)` call; `w.Poll(ctx)`; the `WatcherFactory` type alias.
- `watcher/github-build/pkg/watcher_test.go` — contains many `w.Poll(ctx)` invocations that must all become `w.Poll(ctx, false)`; every `NewWatcher(...)` call must pass the new clock argument.
- `watcher/github-build/main_poll_loop_test.go` — re-read; its `pollFunc` is a local `func(ctx context.Context) error` closure (a `run.Func`), NOT a `Watcher.Poll` call. Confirm with grep; only update if a direct `Watcher.Poll` call is found.
- `watcher/github-build/mocks/watcher.go` — counterfeiter-generated. Must be regenerated, not hand-edited.

Key facts (verified):
- `libtime` import path: `libtime "github.com/bborbe/time"`. Constructor `libtime.NewCurrentDateTime()` returns a `libtime.CurrentDateTime` (which satisfies `libtime.CurrentDateTimeGetter`).
- `libtime.CurrentDateTimeGetter` is `interface { Now() DateTime }`. `Now()` returns `libtime.DateTime`, which exposes `UnixMicro() int64` directly. So `w.currentDateTime.Now().UnixMicro()` compiles — use that exact form. Do NOT call `.Time().UnixMicro()`.
- For deterministic test clocks, `libtime.DateTime` is convertible from `time.Time` via `libtime.DateTime(t)` and there is `libtime.DateTimeFromUnixMicro(int64) DateTime`. A tiny in-test `fakeClock` implementing `Now() libtime.DateTime` is sufficient.
- Import for nonce formatting: `"strconv"`.
- `task.CreateCommandSender.SendCommand(ctx, cmd)` is the existing publish call inside `applyStateMachine`'s `green × red` arm.
- Counterfeiter regeneration is triggered by `go generate ./...` per the `//counterfeiter:generate` directive at the top of `watcher/github-build/pkg/watcher.go`. `make precommit` runs `go generate` for you.
- The executor's success log already includes `force=%t` (the `scope=%q force=%t` lines) — DO NOT touch those two log lines; the spec says they are already correct.
- `runTriggerBuildCheck`'s doc-block mentions Scope/Force reservation. THIS prompt rewrites only the Force phrasing to say Force is now wired; Scope stays reserved-unread. The handler-side and struct-side comments are owned by prompt 3.
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

### Phase A — Capture the master-HEAD `CreateTaskCommand` fixture BEFORE editing the state machine

This is load-bearing for the byte-identical non-force-path acceptance criterion.

1. **Before changing any production code**, write a Ginkgo spec at `watcher/github-build/pkg/watcher_byteidentity_test.go` (external `pkg_test` package) that exercises the existing `green × red` happy path through the unmodified state machine, captures the produced `task.CreateCommand`, and persists it as a fixture (recommended: `json.Marshal` to `watcher/github-build/pkg/testdata/byteidentity_green_to_red.json`). Constraints on the capture step:
   - Use the existing `watcher_test.go` fixtures as input shape — a cursor with `LastKnownState == ""` (or `"green"`) for the repo, a fake `GitHubClient` returning one failing run.
   - Wire `NewWatcher` exactly as it exists in master (no clock argument yet, this runs before the signature change). Capture `task.CreateCommand` via the `task.CreateCommandSender` counterfeiter mock's `SendCommandArgsForCall(0)`.
   - Run `go test ./pkg -run 'ByteIdentity|GreenToRed' -v` and confirm the capture passes BEFORE step 2.
   - In a later step you will assert deep-equality against this fixture from a `Poll(ctx, false)` invocation.

   Capture first; edit second.

### Phase B — Surgery

2. **Update `watcher/github-build/pkg/watcher.go`**:

   a. **Interface change** — rewrite the `Watcher` doc + signature:
      ```go
      // Watcher polls GitHub Actions for build status changes.
      //
      // When force is true, the red×red episode-lock arm of the state machine
      // publishes a salted CreateTaskCommand instead of skipping (spec 069), so
      // operators can force a re-publish for a still-red build via the /trigger
      // HTTP path. The poll-interval loop always passes false.
      type Watcher interface {
          Poll(ctx context.Context, force bool) error
      }
      ```

   b. **Imports** — add `"strconv"` and `libtime "github.com/bborbe/time"` to the import block.

   c. **`buildWatcher` struct** — append a clock field:
      ```go
      currentDateTime   libtime.CurrentDateTimeGetter
      ```

   d. **`NewWatcher` signature** — append `currentDateTime libtime.CurrentDateTimeGetter` as the final parameter and pass it through to the struct literal as `currentDateTime: currentDateTime`. Keep all existing params and field assignments unchanged.

   e. **`Poll` signature** — add `force bool` and thread it through to `pollRepo`:
      ```go
      func (w *buildWatcher) Poll(ctx context.Context, force bool) error {
          // ... unchanged through the loop ...
          if rateLimited := w.pollRepo(ctx, cursor, repoKey, force); rateLimited {
              break
          }
          // ... cursor save, metrics, IncPollCycle("success") all unchanged ...
      }
      ```

   f. **`pollRepo` signature** — add `force bool` as the final parameter and forward it to `applyStateMachine` as the final argument.

   g. **`applyStateMachine` change** — add `force bool` as the final parameter. The `(prevState == "" || prevState == "green") && currState == "red"` arm stays exactly as-is (canonical `DeriveTaskID`). Insert a NEW force arm BEFORE the existing `red × red` skip arm, and leave the skip arm in place for the non-force case. Use exactly this structure:

      ```go
      case prevState == "red" && currState == "red" && force:
          // Spec 069: force=true on a still-red repo re-publishes with a salted
          // TaskIdentifier so the agent controller's file-exists skip does NOT
          // fire and a fresh vault task is created. Cursor state stays "red";
          // CurrentEpisodeSHA is unchanged (the episode is the same; only the
          // task identifier is salted to evade dedup).
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
              return
          }
          w.metrics.IncTaskPublished()
          // NOTE: IncStateTransition("green_to_red") is NOT incremented on the force
          // path — the state didn't actually transition (it was already red), so
          // labeling it as green_to_red would be misleading. Force publishes count
          // only toward IncTaskPublished (spec 069).

      case prevState == "red" && currState == "red":
          // Episode locked on first red; skip regardless of SHA change (force=false)
      ```

      Important details verified against the spec:
      - The forced publish increments ONLY `IncTaskPublished`. It MUST NOT call `IncStateTransition("green_to_red")` — the state did not transition. (This differs from the `green × red` arm, which DOES increment `IncStateTransition("green_to_red")`.) Do NOT add a new metric or label.
      - The clock call MUST go through `w.currentDateTime.Now().UnixMicro()`. No direct `time.Now()`.
      - Do NOT mutate `repoState.LastKnownState` or `repoState.CurrentEpisodeSHA` in the force arm — the episode lock is unchanged; force just emits one extra task.
      - The non-force `red × red` skip arm stays an empty body with its comment, byte-identical to master.

   h. **Doc-comment** on `applyStateMachine` — extend it so `force` is documented:
      ```go
      // applyStateMachine applies the green/red state machine for a single repo.
      //
      // When force is true and prevState==currState=="red", the episode-lock skip is
      // bypassed and a salted CreateTaskCommand is published (spec 069). All other
      // arms ignore force.
      ```

3. **Update `watcher/github-build/pkg/command/trigger_build_check_executor.go`**:

   a. **Doc-block** on `NewTriggerBuildCheckCommandExecutor` / `runTriggerBuildCheck` — rewrite the Force-reservation phrasing so it states the executor reads `cmd.Force` and forwards it to `Watcher.Poll`, and that the `red×red` arm publishes a salted `CreateTaskCommand` when true. Scope stays reserved-unread. Example replacement text:
      ```
      // The executor reads cmd.Force and forwards it to Watcher.Poll (spec 069):
      // when true, the red×red episode-lock arm of the state machine publishes a
      // salted CreateTaskCommand instead of skipping. The cmd.Scope field is still
      // reserved-unread (spec Non-goal: per-repo filter UX is a separate spec).
      ```

   b. **`runTriggerBuildCheck` body** — change `if err := watcher.Poll(ctx); err != nil {` to `if err := watcher.Poll(ctx, cmd.Force); err != nil {`.

   c. **Do NOT touch the two existing v(2) / error log lines** that already include `scope=%q force=%t` — they are already correct.

   d. **AC pin**: after this edit, `grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' watcher/github-build/pkg/command/trigger_build_check_executor.go` MUST return zero matches. (A bare "Scope ... reserved-unread" mention does NOT match — every alternative in the grep includes "deferred" or "Force".) Re-check by eye that the only remaining reservation mention is Scope-specific and does not contain the literal `reserved-unread` adjacent to Force.

4. **Update `watcher/github-build/pkg/factory/factory.go`** — `CreateWatcher`:
   - Add `libtime "github.com/bborbe/time"` to imports.
   - Add a new final parameter `currentDateTime libtime.CurrentDateTimeGetter`.
   - Forward it as the final argument to `pkg.NewWatcher(...)`.
   - Do NOT change any other `factory.*` function.

5. **Update `watcher/github-build/main.go`**:
   - Import `libtime "github.com/bborbe/time"`.
   - Inside `application.Run`, construct `currentDateTime := libtime.NewCurrentDateTime()` before the `factory.CreateWatcher` call and pass it as the final argument (add a trailing-comment `// spec 069: clock for force=true salt nonce`).
   - The `pollOnce` closure body `return w.Poll(ctx)` becomes `return w.Poll(ctx, false)`. The poll-interval loop always passes `false`.

6. **Update `watcher/github-build/cmd/run-once/main.go`**:
   - Import `libtime "github.com/bborbe/time"`.
   - `WatcherFactory` type alias — add `currentDateTime libtime.CurrentDateTimeGetter` as the final parameter to match `factory.CreateWatcher`.
   - Inside `Application.Run`, pass `libtime.NewCurrentDateTime()` as the final argument to the `a.CreateWatcher(...)` call.
   - Change `return w.Poll(ctx)` to `return w.Poll(ctx, false)`. run-once never forces.

7. **Update `watcher/github-build/main_poll_loop_test.go`** — re-read. Confirm with:
   ```bash
   grep -n "Watcher\|w\.Poll" watcher/github-build/main_poll_loop_test.go
   ```
   If grep shows only a local `func(ctx context.Context) error` closure (no `Watcher.Poll` call), leave the file untouched. If it shows a direct `Watcher.Poll` call, update it to `Poll(ctx, false)`.

8. **Update `watcher/github-build/pkg/watcher_test.go`** — mechanical: every `w.Poll(ctx)` becomes `w.Poll(ctx, false)` (verify the count with `grep -n "w.Poll(ctx)" pkg/watcher_test.go` first). Do NOT add `force=true` cases here. Also update every `NewWatcher(...)` call to pass a clock as the new final argument (`libtime.NewCurrentDateTime()` is fine — the existing tests do not exercise the clock). Add `libtime "github.com/bborbe/time"` to imports if not already present.

### Phase C — New behavioural tests for the force path

9. **Add force-path tests** to `watcher/github-build/pkg/watcher_test.go` (or a new sibling file `watcher_force_test.go` in the same `pkg_test` package — sibling file is cleaner). Cover all six watcher-behaviour acceptance criteria:
   a. `Poll(ctx, false)` on a red-red fixture does NOT call `createSender.SendCommand`.
   b. `Poll(ctx, true)` on the same red-red fixture DOES call `createSender.SendCommand` exactly once.
   c. The captured `CreateTaskCommand` from (b) has a `TaskIdentifier` different from `pkg.DeriveTaskID(owner, repo, episodeSHA).String()` for the same inputs.
   d. Two `Poll(ctx, true)` invocations against the same fixture with the injected clock advanced by ≥1 microsecond between calls produce two captured commands with distinct `TaskIdentifier` values.
   e. `Poll(ctx, false)` on a `green × red` fixture produces a `CreateTaskCommand` deep-equal to the Phase A master-HEAD fixture.
   f. `Poll(ctx, true)` saves the cursor at end-of-cycle (unchanged cursor-save behaviour).

   For (d), implement a tiny in-test `fakeClock` satisfying `libtime.CurrentDateTimeGetter` that returns successive timestamps from a slice (use `libtime.DateTimeFromUnixMicro` to build the values). Mirror the pattern in `go-time-injection.md`.

   For the executor side, add to `watcher/github-build/pkg/command/trigger_build_check_executor_test.go` (or a sibling in the same package):
   - `TestExecutor_ForceTrueCallsPollForceTrue` (or Ginkgo equivalent named with `Force`): drive the executor with `TriggerBuildCheckCommand{Force: true}`; assert `watcher.PollCallCount() == 1` and the captured second arg of `PollArgsForCall(0)` is `true`.
   - `TestExecutor_ForceFalseCallsPollForceFalse`: same with `Force: false`; assert the captured force arg is `false`.

   The regenerated `mocks.Watcher` exposes `PollCallCount() int`, `PollArgsForCall(int) (context.Context, bool)`, and `PollReturns(error)`. Verify after regeneration (step 10).

### Phase D — Mocks + precommit

10. **Regenerate counterfeiter mocks**:
    ```bash
    cd watcher/github-build && go generate ./...
    ```
    Verify with:
    ```bash
    grep -n "PollArgsForCall\|PollCallCount\|PollReturns" mocks/watcher.go
    ```
    Expected: `PollArgsForCall` returns `(context.Context, bool)`.

11. **Run precommit**:
    ```bash
    cd watcher/github-build && make precommit
    ```
    Must exit 0. If a stale mock breaks the build, re-run `go generate ./...` then `make precommit`. If a forgotten call site breaks it:
    ```bash
    grep -rn "\.Poll(ctx)" watcher/github-build/
    ```
    Every remaining hit must be `Poll(ctx, false)` or `Poll(ctx, <bool>)`.

12. **Sanity-grep**:
    ```bash
    grep -nE 'Poll\(ctx context\.Context, force bool\) error' watcher/github-build/pkg/watcher.go
    grep -rnE '\btime\.Now\(\)' watcher/github-build/pkg/ --include='*.go' --exclude='*_test.go'
    # second grep: zero matches
    ```

</requirements>

<constraints>
- Edit only: `watcher/github-build/pkg/watcher.go`, `watcher/github-build/pkg/command/trigger_build_check_executor.go`, `watcher/github-build/pkg/factory/factory.go`, `watcher/github-build/main.go`, `watcher/github-build/cmd/run-once/main.go`, `watcher/github-build/pkg/watcher_test.go` (and optionally a new `watcher_force_test.go` sibling), `watcher/github-build/pkg/watcher_byteidentity_test.go` (new) plus its `testdata/`, test files under `watcher/github-build/pkg/command/`, and the regenerated `watcher/github-build/mocks/watcher.go`. Do NOT touch `watcher/github-build/pkg/handler/`, `watcher/github-build/pkg/command/trigger_build_check_command.go`, or `CHANGELOG.md` — those belong to prompt 3.
- Do NOT commit — dark-factory handles git.
- Do NOT modify `DeriveTaskID`, `buildWatcherNamespace`, `splitRepoKey`, `deriveState`, or `buildCreateTaskCommand`. Force path reuses `buildCreateTaskCommand` unchanged.
- Do NOT add a new Prometheus metric or label for force cycles. Force-publishes increment ONLY `IncTaskPublished` (NOT `IncStateTransition`).
- Do NOT mutate `repoState.LastKnownState` or `CurrentEpisodeSHA` in the force arm. Episode-lock state stays red.
- Do NOT add `time.Now()` anywhere in `watcher/github-build/pkg/` outside `*_test.go`. All time goes through `libtime.CurrentDateTimeGetter`.
- Do NOT use `fmt.Errorf`. Use `github.com/bborbe/errors`.
- Do NOT branch any other state-machine arm on `force`. Only `red × red` is conditional.
- Do NOT add a per-feature opt-out flag — spec Non-goal explicitly forbids it.
- `Watcher.Poll`'s new parameter is named `force` (not `skipEpisodeLock` or anything else).
- Counterfeiter mocks are regenerated (not hand-edited) via `go generate ./...`.
- Existing tests must still pass after the `Poll(ctx)` → `Poll(ctx, false)` mechanical rewrite.
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

# Stale Force reservation phrasing gone from the executor:
! grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' pkg/command/trigger_build_check_executor.go
# Expect: exit 0.

# Mock has the new two-arg signature:
grep -n 'PollArgsForCall' mocks/watcher.go

# Force-path tests pass:
go test ./pkg/command -run 'Force' -v
go test ./pkg -run 'Poll|Force' -v

# Byte-identity fixture test passes:
go test ./pkg -run 'byteidentity|ByteIdentity|GreenToRed' -v

# git diff confinement (informational):
git diff --stat
</verification>
