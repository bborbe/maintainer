---
status: draft
spec: [032-bug-task-controller-respawns-on-terminal-phase]
created: "2026-05-16T12:00:00Z"
branch: dark-factory/bug-task-controller-respawns-on-terminal-phase
---

<summary>
- Clone bborbe/agent, find the task-executor spawn predicate, record its file:line in the spec's Reproduction section
- Add a local `isTerminalPhase` helper (human_review, done) and gate spawning on it
- Plant a code comment at the gate: "terminal phases must not be spawned again — operator escalation required"
- Emit one info log line "controller: spawn suppressed phase=<phase> task=<id>" per suppressed reconcile cycle
- Write a Ginkgo DescribeTable covering all 6 spec-032 rows; add a revert-test and a parse-error-path test
- Push a feature branch to bborbe/agent and create a PR via `gh pr create`
- Update `specs/in-progress/032-bug-task-controller-respawns-on-terminal-phase.md` in /workspace with the confirmed predicate location
</summary>

<objective>
Add a terminal-phase gate to the task-executor's spawn predicate in `bborbe/agent` so a vault task
whose phase is `human_review` or `done` is never re-spawned. This closes the trigger for the
2026-05-16 double-spawn incident (task `22fda7e7`, PR bborbe/maintainer #5).
</objective>

<context>
Read `CLAUDE.md` at `/workspace` for YOLO container rules.

Read these guides before writing any code (in `/home/node/.claude/plugins/marketplaces/coding/docs/`):
- `go-enum-type-pattern.md` — Available* constants, helper function pattern for terminal classification
- `go-testing-guide.md` — Ginkgo v2 + Gomega, DescribeTable/Entry, suite setup, external `_test` package, coverage ≥80%
- `go-logging-guide.md` — glog V-levels vs info; structured log keys; existing codebase uses glog
- `go-error-wrapping-guide.md` — bborbe/errors only; never fmt.Errorf
- `test-pyramid-triggers.md` — which test types to write (pure predicate logic → unit tests only)
- `git-workflow.md` — branch naming, commit format, PR creation; applies to bborbe/agent (not /workspace)

**Why bborbe/agent, not /workspace:**
The maintainer repo's watcher (`/workspace/watcher/github-pr/`) publishes Kafka events when PRs change;
it does NOT spawn K8s Jobs. The task executor in `bborbe/agent/task/executor/` reads Kafka events and
spawns K8s Jobs. The spawn predicate is there. The maintainer `/workspace` is a dark-factory project
(never commit/push from YOLO); bborbe/agent (cloned below) follows the regular git workflow.

**TaskPhase reference (vault-cli v0.64.0 — do NOT modify vault-cli):**

`github.com/bborbe/vault-cli/pkg/domain/task_phase.go` defines:
```go
type TaskPhase string
const (
    TaskPhaseTodo        TaskPhase = "todo"
    TaskPhasePlanning    TaskPhase = "planning"
    TaskPhaseInProgress  TaskPhase = "in_progress"
    TaskPhaseAIReview    TaskPhase = "ai_review"
    TaskPhaseHumanReview TaskPhase = "human_review"
    TaskPhaseDone        TaskPhase = "done"
)
```
There is NO `IsTerminal()` method and NO `TaskPhaseAborted` constant in vault-cli v0.64.0.
You CANNOT add methods to external types in Go — use a local `isTerminalPhase` helper in the executor package.
Terminal-phase set for this fix: `{human_review, done}`.
If an `aborted` phase constant IS found in the actual running bborbe/agent version (grep before assuming),
include it; do NOT fabricate constants that don't exist.

Verify with:
```bash
grep -n "Aborted\|aborted" \
  "$(go env GOPATH)/pkg/mod/github.com/bborbe/vault-cli@v0.64.0/pkg/domain/task_phase.go"
# Expected: zero matches → omit aborted from the terminal set
```
</context>

<requirements>
Execute steps in order. Run `make test` after Step 4 for fast feedback. Run `make precommit` only at Step 6.

---

## Step 1 — Clone bborbe/agent and find the spawn predicate

```bash
# Clone into a working directory (YOLO containers have GitHub SSH access)
cd /tmp && git clone git@github.com:bborbe/agent.git bborbe-agent
cd /tmp/bborbe-agent && git checkout master && git pull

# Find the spawn predicate
grep -rn "shouldSpawn\|CreateJob\|TaskPhaseHumanReview\|TaskPhaseInProgress\|spawn\|phase.*in_progress" \
  task/executor/ agent/claude/ agent/ \
  2>/dev/null | grep -v "_test.go\|vendor" | head -30
```

The expected shape is a filter condition in `task/executor/` that allows only certain phases to spawn.
From spec 004 (the original executor spec): "skip unless status=in_progress AND phase ∈ {planning, in_progress, ai_review}".

Read the predicate file fully. Read the existing test file for the same package.

If the clone fails (SSH auth, network): STOP and report
`{"status":"failed","message":"git clone git@github.com:bborbe/agent.git failed: <error>"}`.

If the grep finds zero results: try a broader search:
```bash
find /tmp/bborbe-agent -name "*.go" | xargs grep -l "phase\|spawn\|CreateJob" 2>/dev/null | head -20
```

If still not found after exhausting searches: STOP and report
`{"status":"failed","message":"spawn predicate not found in bborbe/agent. Grep output: <paste output>"}`.

Record the confirmed location. Note the exact file path and line number.

---

## Step 2 — Read all referenced files fully

Before writing any code:
1. The full file containing the spawn predicate (chunked reads if > 2000 lines)
2. The existing test file for the same package (full file)
3. Any filter function or helper the predicate uses
4. The go.mod for the executor service to confirm the vault-cli version in use

---

## Step 3 — Create a feature branch in bborbe/agent

```bash
cd /tmp/bborbe-agent
git checkout -b fix/terminal-phase-spawn-gate
```

---

## Step 4 — Add the terminal-phase gate

### 4a. Define a local `isTerminalPhase` helper

In the package that owns the spawn predicate, add a package-level helper.
Place it in the same Go file as the spawn predicate, or in a small `phase_helpers.go` in the same package.
Match the existing import alias for vault-cli's domain package (check existing imports in the predicate file).

```go
// isTerminalPhase reports whether a task phase represents operator escalation.
// terminal phases must not be spawned again — operator escalation required.
func isTerminalPhase(phase domain.TaskPhase) bool {
	return phase == domain.TaskPhaseHumanReview || phase == domain.TaskPhaseDone
}
```

If the executor service defines its own phase constants (not vault-cli's), mirror the same pattern
using the project's actual constant names — do NOT fabricate constants.

**Unknown-phase conservative default:** if the task's phase is unknown (not in the vault-cli enum),
treat it as non-terminal (preserve spawn behavior) and emit a warning:
```go
if !domain.AvailableTaskPhases.Contains(task.Phase) {
    glog.Warningf("controller: unknown phase=%s task=%s — treating as non-terminal", task.Phase, taskID)
}
```

Export as `IsTerminalPhase` (uppercase) if the test file uses an external `_test` package.

### 4b. Insert the gate into the spawn predicate

Immediately before the spawn/Job-creation call, add the gate and its invariant comment:

```go
// terminal phases must not be spawned again — operator escalation required.
if isTerminalPhase(task.Phase) {
    glog.Infof("controller: spawn suppressed phase=%s task=%s", task.Phase, taskID)
    return // or 'continue' if inside a range loop; match the surrounding control flow
}
```

Placement requirements:
- AFTER the task is parsed (reads current committed phase, not cached)
- BEFORE any Job-creation / spawn call
- NOT on the existing parse-error path (parse errors return/skip before this gate)
- Emits exactly one log line per suppressed reconcile cycle per task

Adapt field names (`task.Phase`, `taskID`) to the actual struct field names in the predicate file.

---

## Step 5 — Write Ginkgo regression tests

In the existing test file for the spawn predicate package, add tests covering all 6 spec-032 rows.
Mirror the exact test patterns (Ginkgo suite setup, Counterfeiter mocks, struct constructors) already
in the test file — read the file fully in Step 2 before writing any test.

### 5a. DescribeTable for phase-gate rows (rows 1–5)

Add a `DescribeTable` covering these 5 scenarios. Adapt the test helper calls and mock setup to match
the actual test file's patterns:

```go
DescribeTable("spawn predicate terminal-phase gate",
    func(status domain.TaskStatus, phase domain.TaskPhase, expectSpawn bool) {
        // Use the actual task builder / mock setup pattern from the existing tests
        // The spawn function is typically a mock K8s client or a spy counter
        spawnCount := 0
        // ... set up task and call reconcile/spawn function ...
        if expectSpawn {
            Expect(spawnCount).To(Equal(1))
        } else {
            Expect(spawnCount).To(Equal(0))
        }
    },
    Entry("status=in_progress phase=in_progress => spawn",
        domain.TaskStatusInProgress, domain.TaskPhaseInProgress, true),
    Entry("status=in_progress phase=human_review => no spawn",
        domain.TaskStatusInProgress, domain.TaskPhaseHumanReview, false),
    Entry("status=in_progress phase=done => no spawn",
        domain.TaskStatusInProgress, domain.TaskPhaseDone, false),
    Entry("status=in_progress phase=ai_review => spawn",
        domain.TaskStatusInProgress, domain.TaskPhaseAIReview, true),
    Entry("status=completed phase=in_progress => no spawn",
        domain.TaskStatusCompleted, domain.TaskPhaseInProgress, false),
)
```

### 5b. Sequential-reconcile cross-cycle test (row 6)

```go
Context("sequential reconcile: in_progress → human_review", func() {
    It("spawns exactly once across two reconcile cycles", func() {
        spawnCount := 0
        // ... mock setup ...

        // Cycle 1: phase=in_progress → should spawn
        // (use actual task builder and reconcile call from the existing test patterns)
        // ... reconcile with in_progress phase ...
        Expect(spawnCount).To(Equal(1))

        // Cycle 2: same task, phase mutated to human_review → must NOT spawn again
        // (use same task ID, only phase changes — simulates agent write + obsidian-git commit)
        // ... reconcile with human_review phase ...
        Expect(spawnCount).To(Equal(1),
            "second reconcile cycle with human_review phase must not increment spawn count")
    })
})
```

### 5c. Revert-test proving the gate is load-bearing

Add a `DescribeTable` testing `isTerminalPhase` directly — this serves as the revert-test:

```go
DescribeTable("isTerminalPhase — gate is load-bearing",
    func(phase domain.TaskPhase, wantTerminal bool) {
        Expect(IsTerminalPhase(phase)).To(Equal(wantTerminal))
    },
    Entry("in_progress is NOT terminal", domain.TaskPhaseInProgress, false),
    Entry("human_review IS terminal",    domain.TaskPhaseHumanReview, true),
    Entry("done IS terminal",            domain.TaskPhaseDone, true),
    Entry("ai_review is NOT terminal",   domain.TaskPhaseAIReview, false),
    Entry("planning is NOT terminal",    domain.TaskPhasePlanning, false),
    Entry("todo is NOT terminal",        domain.TaskPhaseTodo, false),
)
```

This table fails if the gate is removed (the `human_review` and `done` rows would return false, failing
their `wantTerminal: true` assertion). It directly encodes the spec's terminal-phase contract.

### 5d. Parse-error-path test (failure-mode row 4)

Add a test that verifies the parse-error path does NOT trigger the `spawn suppressed` log.
If the executor already has error handling for unreadable/unparseable tasks, write a test that:
1. Feeds the reconciler a task with nil/empty phase (or a task object that triggers early return in the predicate)
2. Asserts spawn count remains 0
3. Asserts (via spawn-count only, not log-string matching) that the code path hit is the parse-error path, not the terminal-phase gate

```go
Context("nil or missing phase does not masquerade as terminal-phase suppression", func() {
    It("spawn count is 0 — parse-error path, not terminal-phase gate", func() {
        spawnCount := 0
        // Construct a task with nil/empty phase — triggers the existing parse/validation path
        // ... call reconcile ...
        Expect(spawnCount).To(Equal(0),
            "parse-error path must block spawn without going through the terminal-phase gate")
    })
})
```

Note: do NOT use `glog.SetOutput` to capture log strings — `glog.SetOutput` does not exist on
`github.com/golang/glog` v1.2.x. Assert via spawn-count behavior only for the parse-error test.
If the project provides a log-capture helper (check existing tests in the file), use it — but only
if it's already present; do not introduce new log-capture infrastructure.

---

## Step 6 — Add CHANGELOG entry

Read `CHANGELOG.md` in `/tmp/bborbe-agent/`. If `## Unreleased` already exists, append to it;
otherwise create it above the most recent `## vX.Y.Z` heading:

```
- fix: suppress task re-spawn when phase is terminal (human_review, done); emit info log "controller: spawn suppressed phase=<phase> task=<id>" per suppressed reconcile cycle; add regression tests covering all 6 spec-032 rows
```

---

## Step 7 — Run `make test` in the executor service dir (fast feedback)

```bash
# Run tests in the executor's service directory (exact path found in Step 1)
cd /tmp/bborbe-agent/<executor-service-dir> && make test
```

All tests must pass. Fix compilation errors and test failures before proceeding.

---

## Step 8 — Run `make precommit` in the executor service dir

```bash
cd /tmp/bborbe-agent/<executor-service-dir> && make precommit
```

Must exit 0. If any target fails: fix the issue, re-run ONLY the failing target
(`make lint`, `make gosec`, etc.), then re-run `make precommit` once all pass.

---

## Step 9 — Commit and push the branch

```bash
cd /tmp/bborbe-agent
git add <specific-changed-files>   # do NOT use git add -A
git status                         # verify no sensitive files are staged
git commit -m "fix: suppress task re-spawn when phase is terminal (spec 032)"
git push origin fix/terminal-phase-spawn-gate
```

```bash
gh pr create \
  --title "fix: suppress task re-spawn on terminal phase (spec 032)" \
  --body "$(cat <<'EOF'
## Summary
- Adds `isTerminalPhase` helper in task executor to classify `human_review` and `done` as terminal
- Gates spawning: tasks with terminal phase emit an info log and skip job creation
- Invariant comment at the gate: "terminal phases must not be spawned again — operator escalation required"
- Regression tests cover all 6 spec-032 rows including sequential-cycle cross-phase test

## Root cause
The spawn predicate gated on `status` but not `phase`. A reconcile cycle while `status: in_progress`
and `phase: human_review` re-spawned a fresh pod, causing the 2026-05-16 double-spawn incident.

## Test plan
- [ ] `make precommit` exits 0 in the executor service dir
- [ ] DescribeTable with 6 rows all pass
- [ ] Cross-cycle sequential test asserts spawn count = 1 after phase flips to human_review
- [ ] Revert-test (isTerminalPhase table) fails on reverting human_review/done to non-terminal
EOF
)"
```

---

## Step 10 — Update the spec file in /workspace with the confirmed predicate location

After finding the predicate in Step 1, update the spec file in the maintainer project to record it:

File: `/workspace/specs/in-progress/032-bug-task-controller-respawns-on-terminal-phase.md`

Find the section:
```
**Predicate location (to be confirmed during implementation):**
```

Add a line immediately after the paragraph that starts "The implementor must...":
```
Predicate location: bborbe/agent/<path>:<line>
```

Where `<path>` and `<line>` are the actual file path and line number found in Step 1.

This is the only change to `/workspace` — dark-factory handles committing it.

</requirements>

<constraints>

- **Phase enum constants MUST NOT change.** Do not rename, remove, or reorder existing TaskPhase constants.
- **No new phase constants.** Do not introduce `needs_input`, `escalated`, or any new constant not already in the enum.
- **Cannot add methods to external types.** `TaskPhase` is vault-cli's type — use a local `isTerminalPhase` function in the executor package.
- **Terminal-phase set: {human_review, done}** unless grep of the actual bborbe/agent vault-cli version reveals a `TaskPhaseAborted` constant (then include it).
- **Gate uses current committed phase** — re-read from the task event/file on every reconcile cycle; no cross-cycle caching.
- **Log line is additive.** The `spawn suppressed` line is additional to existing spawn/skip logs; do not replace or remove existing log lines.
- **Log NOT on parse-error path.** Parse errors return early before the terminal-phase gate; `spawn suppressed` is only emitted when parsing succeeds AND phase is explicitly terminal.
- **Unknown phase → non-terminal (conservative default).** Preserves existing spawn behavior; log a warning about the unknown phase value.
- **Reconcile interval and existing log keys MUST NOT change.**
- **Existing tests MUST continue to pass.** `git diff` on test files shows additions only; no existing assertions are weakened or removed.
- **Coverage ≥80%** for any modified package.
- **`bborbe/errors` for error wrapping** in non-test code — never `fmt.Errorf`.
- **`context.Background()` forbidden in non-test code** — use the ctx from the caller.
- **YOLO handles bborbe/agent git.** Create branch, commit, push, `gh pr create` — all in `/tmp/bborbe-agent`. The `/workspace` (maintainer) is dark-factory managed: NEVER commit/push from `/workspace`.
- **Do NOT use `glog.SetOutput` in tests.** The `glog` v1.2.x package has no public `SetOutput`. Assert log behavior via spawn-count proxy tests only, unless the existing test file already has a log-capture helper (use only what exists).
- **`make precommit` runs only in the changed executor service dir** — never at the bborbe/agent repo root.

</constraints>

<verification>

```bash
# 1. Predicate location recorded in the spec
grep -n "Predicate location:" \
  /workspace/specs/in-progress/032-bug-task-controller-respawns-on-terminal-phase.md
# Expected: one line of the form "Predicate location: bborbe/agent/<path>:<line>"

# 2. Terminal-phase helper defined in executor code
grep -rn "isTerminalPhase\|IsTerminalPhase\|terminalPhases" \
  /tmp/bborbe-agent/<executor-package-dir>/*.go
# Expected: ≥1 match in implementation file AND ≥1 match in test file
# The set must contain human_review and done

# 3. Code comment at the gate
grep -rn "terminal phases must not be spawned" \
  /tmp/bborbe-agent/<executor-package-dir>/*.go
# Expected: ≥1 match

# 4. Spawn predicate calls the gate
grep -n "isTerminalPhase\|IsTerminalPhase" \
  /tmp/bborbe-agent/<predicate-function-file>
# Expected: ≥1 match inside the reconcile/spawn function

# 5. All 6 spec rows present in test file
grep -n "human_review.*no spawn\|done.*no spawn\|sequential reconcile\|isTerminalPhase\|IsTerminalPhase" \
  /tmp/bborbe-agent/<test-file>
# Expected: entries matching all 6 row names

# 6. make precommit passes in the executor service dir
cd /tmp/bborbe-agent/<executor-service-dir> && make precommit
# Expected: exit 0

# 7. PR created for bborbe/agent
gh pr view --repo bborbe/agent | head -10
# Expected: PR visible with title matching "terminal phase" or "spec 032"

# 8. No fmt.Errorf in modified Go files
grep -rn "fmt\.Errorf" /tmp/bborbe-agent/<executor-package-dir>/*.go 2>/dev/null
# Expected: zero matches in implementation files (tests may use it if project convention allows)
```

</verification>
