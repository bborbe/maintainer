---
spec: ["031"]
status: draft
created: "2026-05-16T11:30:00Z"
---

<summary>
- Locate the task-controller spawn predicate in the bborbe/agent repository and add a terminal-phase gate so tasks with phase ∈ {human_review, done} are never re-spawned
- Encode the terminal-phase set as a package-level `terminalPhases` variable using the existing `domain.TaskPhases{...}.Contains()` pattern — single source of truth
- Add a code comment at the gate naming the invariant: "terminal phases must not be spawned again — operator escalation required"
- Emit an info-level log line (`controller: spawn suppressed phase=<phase> task=<task-id>`) for every suppressed spawn — once per reconcile cycle per task
- Write a Ginkgo `DescribeTable` regression test covering all 6 rows from the spec's reproduction section (status/phase matrix + cross-cycle sequential test)
- The cross-cycle test asserts exactly 1 spawn via a mock/spy counter when phase transitions from in_progress → human_review between cycles
- The parse-error path does NOT emit a spawn_suppressed log line — tested separately
- `make precommit` passes in the bborbe/agent directory that owns the predicate
- A PR is created in the bborbe/agent repository; /workspace is NOT committed (dark-factory handles that)
</summary>

<objective>
Add a terminal-phase gate to the task-controller's spawn predicate so that any task with phase ∈ {human_review, done} is never re-spawned regardless of status. This closes the trigger for the 2026-05-16 incident where task 22fda7e7 spawned a second pr-reviewer pod after pod 1 had already set phase: human_review, causing pod 2 to dismiss pod 1's correctly-posted GitHub review and hide a hallucination escalation signal.
</objective>

<context>
Read `CLAUDE.md` at `/workspace` for project conventions and YOLO container rules.

Read these guides before writing any code (in `~/.claude/plugins/marketplaces/coding/docs/`):
- `go-enum-type-pattern.md` — `AvailableX` collection, `Contains()`, behavior-method conventions
- `go-testing-guide.md` — Ginkgo v2 + Gomega, `DescribeTable`/`Entry`, external `*_test` package, coverage ≥80%
- `go-logging-guide.md` — glog vs slog, verbosity levels, structured fields, do-not-log-and-return-error rule
- `go-patterns.md` — error wrapping, counterfeiter annotations
- `test-pyramid-triggers.md` — which test types to write (pure Go predicate logic → unit tests only)

**Phase enum (verified from Go module cache `github.com/bborbe/vault-cli@v0.64.0/pkg/domain/task_phase.go`):**

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

var AvailableTaskPhases = TaskPhases{...}

type TaskPhases []TaskPhase
func (t TaskPhases) Contains(phase TaskPhase) bool { ... }
```

`TaskPhaseAborted` does NOT exist in vault-cli v0.64.0. Terminal set is exactly:
`{domain.TaskPhaseHumanReview, domain.TaskPhaseDone}`.

**TaskFrontmatter accessors in bborbe/agent/lib v0.62.16 (`agent_task-frontmatter.go`):**

```go
// Phase() returns *domain.TaskPhase from frontmatter["phase"]; nil if absent or empty.
func (f TaskFrontmatter) Phase() *domain.TaskPhase

// Status() returns domain.TaskStatus from frontmatter["status"].
func (f TaskFrontmatter) Status() domain.TaskStatus
```

**TaskStatus constants (`github.com/bborbe/vault-cli@v0.64.0/pkg/domain/task_status.go`):**

```go
const (
    TaskStatusTodo       TaskStatus = "todo"
    TaskStatusInProgress TaskStatus = "in_progress"
    TaskStatusBacklog    TaskStatus = "backlog"
    TaskStatusCompleted  TaskStatus = "completed"
    TaskStatusHold       TaskStatus = "hold"
    TaskStatusAborted    TaskStatus = "aborted"
)
```

**Coding guides** (in `~/.claude/plugins/marketplaces/coding/docs/`):
- `go-enum-type-pattern.md`
- `go-testing-guide.md`
- `go-logging-guide.md`
- `test-pyramid-triggers.md`
</context>

<requirements>

Execute steps in order. Run `make test` after each meaningful code change. Run `make precommit` only at the final step.

---

## Step 1 — Clone bborbe/agent and locate the spawn predicate

**1a. Check /workspace first:**

```bash
grep -rn "spawn\|Spawn\|CreateJob\|shouldSpawn\|ShouldSpawn\|IsTerminal\|TaskPhaseInProgress\|in_progress.*phase\|phase.*spawn" \
  /workspace/agent/ /workspace/watcher/ 2>/dev/null \
  | grep -v "_test.go\|vendor\|mocks\|CHANGELOG\|\.md:" | head -30
```

If this returns a function that gates on task status to decide whether to spawn a K8s Job, read that file fully and skip to Step 2 with `PREDICATE_REPO=/workspace`.

**1b. Clone bborbe/agent:**

If not found in /workspace, the predicate is in the bborbe/agent repository:

```bash
cd /tmp && git clone git@github.com:bborbe/agent.git bborbe-agent 2>&1 | tail -5
```

**1c. Find the spawn predicate:**

```bash
grep -rn "spawn\|Spawn\|CreateJob\|ShouldSpawn\|phase\|Phase" \
  /tmp/bborbe-agent/ 2>/dev/null \
  | grep -v "_test.go\|vendor\|\.git\|mocks\|CHANGELOG\|\.md:" | head -60
```

Look for a function that:
- Reads a task's `Status` and/or `Phase` fields
- Returns a bool indicating whether to create/spawn a K8s Job
- The expected shape is something like:
  ```go
  func shouldSpawn(task Task) bool {
      return task.Frontmatter.Status() == domain.TaskStatusInProgress
  }
  ```

Find the specific file and line:

```bash
find /tmp/bborbe-agent -name "*.go" \
  | xargs grep -l "spawn\|Spawn\|CreateJob" 2>/dev/null \
  | grep -v "_test.go\|vendor\|mocks" | head -10
```

Read the spawn predicate file and its test file fully before proceeding.

Record:
- `PREDICATE_FILE` = absolute path to the file
- `PREDICATE_TEST_FILE` = the corresponding `*_test.go` file
- `PREDICATE_SUBDIR` = the service subdirectory (e.g., `/tmp/bborbe-agent/task/executor/`)

---

## Step 2 — Add the `terminalPhases` variable

In the same package as the spawn predicate (either add to the predicate file or create a new file `terminal_phases.go` in the same package), add:

```go
// terminalPhases is the set of task phases that block automated re-spawning.
// terminal phases must not be spawned again — operator escalation required.
var terminalPhases = domain.TaskPhases{
    domain.TaskPhaseHumanReview,
    domain.TaskPhaseDone,
}
```

Import: `"github.com/bborbe/vault-cli/pkg/domain"` (already present in the predicate file — confirm before adding).

Do NOT add `domain.TaskPhaseAborted` — it does not exist in vault-cli v0.64.0.

**Before writing**: verify `domain.TaskPhases.Contains` exists:
```bash
grep -n "func.*TaskPhases.*Contains" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/vault-cli@v0.64.0/pkg/domain/task_phase.go
```
Expected: at least one match.

---

## Step 3 — Add the terminal-phase gate to the spawn predicate

Read the spawn predicate function body fully. Identify:
1. Where the function decides to spawn (the `return true` or equivalent path)
2. Whether it already reads `Frontmatter.Phase()`

Add the terminal-phase gate BEFORE the function returns true (spawn). The gate must check the phase from the task frontmatter and suppress spawning if it is terminal.

**Gate shape (adapt to the actual function signature and variable names):**

```go
// terminal phases must not be spawned again — operator escalation required.
if phase := task.Frontmatter.Phase(); phase != nil && terminalPhases.Contains(*phase) {
    glog.Infof("controller: spawn suppressed phase=%s task=%s", *phase, task.TaskIdentifier)
    return false // or whatever the "do not spawn" return value is
}
```

**Log level**: check what logger the existing spawn code uses:
- If using `glog`: use `glog.Infof(...)` (info level 0, not V(2) or V(3))
- If using `slog` or structured logger: use `slog.Info("spawn suppressed", "phase", phase, "task", taskID)` with keys `phase` and `task`

Match the existing logging style exactly. The log line must contain the string `"spawn suppressed"` so operators can grep it.

**Phase re-read**: confirm the function reads `task.Frontmatter.Phase()` on every call (not from a cached field). If the task struct is rebuilt from the vault file on every reconcile cycle, this is already satisfied — just verify and note it.

**Parse-error path**: confirm the parse-error return (if one exists) is BEFORE the terminal-phase check. The suppression log must NOT be emitted when the task cannot be parsed — the existing parse-error path blocks spawning for a different reason.

---

## Step 4 — Write Ginkgo regression tests

Add tests to `PREDICATE_TEST_FILE` (or create a new `*_suite_test.go` and `*_test.go` pair in the same package following existing patterns). Use external test package.

**Required DescribeTable covering all 6 rows:**

```go
DescribeTable("spawn predicate — terminal-phase gate",
    func(status domain.TaskStatus, phase *domain.TaskPhase, expectSpawn bool) {
        // Build a minimal task with the given status and phase frontmatter
        // Call the spawn predicate (or the reconcile function) with a spy/mock spawner
        // Assert spawn was called (expectSpawn=true) or NOT called (expectSpawn=false)
    },
    Entry("status=in_progress phase=in_progress => spawn",
        domain.TaskStatusInProgress, domain.TaskPhaseInProgress.Ptr(), true),
    Entry("status=in_progress phase=human_review => no spawn",
        domain.TaskStatusInProgress, domain.TaskPhaseHumanReview.Ptr(), false),
    Entry("status=in_progress phase=done => no spawn",
        domain.TaskStatusInProgress, domain.TaskPhaseDone.Ptr(), false),
    Entry("status=in_progress phase=ai_review => spawn (non-terminal)",
        domain.TaskStatusInProgress, domain.TaskPhaseAIReview.Ptr(), true),
    Entry("status=completed phase=in_progress => no spawn (status gate)",
        domain.TaskStatusCompleted, domain.TaskPhaseInProgress.Ptr(), false),
    Entry("status=in_progress phase=nil => spawn (phase absent from frontmatter)",
        domain.TaskStatusInProgress, nil, true),
)
```

Adapt the Entry parameters and mock/spy setup to the actual function signature found in Step 1. Read the existing test file patterns before writing.

**Cross-cycle test (sequential reconcile — the load-bearing regression test):**

```go
It("sequential reconcile: in_progress→human_review between cycles triggers exactly 1 spawn total", func() {
    spawnCount := 0
    // Set up a spy: spawnCount++ on each spawn
    
    // Cycle 1: task has status=in_progress, phase=in_progress
    // Run reconcile → expect one spawn
    Expect(spawnCount).To(Equal(1))
    
    // Simulate agent write between cycles: change phase to human_review
    // (mutate the task fixture's frontmatter["phase"] = "human_review")
    
    // Cycle 2: same task now has status=in_progress, phase=human_review
    // Run reconcile → expect NO second spawn
    Expect(spawnCount).To(Equal(1)) // still 1, not 2
})
```

**Log-line test (suppression log emitted for terminal phase):**

If the codebase has a glog test-capture infrastructure (grep the test file for `glog.*test\|capture\|hook`), use it to assert the suppression log line is emitted for human_review and NOT emitted for in_progress. If no capture infrastructure exists, skip this specific assertion and document it in `## Improvements`.

**Parse-error test:**

```go
It("does not emit spawn_suppressed when frontmatter cannot be parsed", func() {
    // Feed the predicate a task with nil/missing frontmatter
    // Assert: spawn count = 0 (blocked by the parse/nil-check, not the terminal-phase gate)
    // If log capture available: assert "spawn suppressed" is NOT in captured log output
})
```

---

## Step 5 — Run `make test` (fast feedback)

```bash
cd $PREDICATE_SUBDIR && make test
```

Fix all compile errors and test failures before proceeding.

---

## Step 6 — Update CHANGELOG in bborbe/agent

Check whether `CHANGELOG.md` exists in `/tmp/bborbe-agent` (or the owning repo). If it does:

```bash
grep -n "## Unreleased\|## v" /tmp/bborbe-agent/CHANGELOG.md | head -5
```

Append under `## Unreleased` (create the section if absent):

```markdown
## Unreleased

- fix(task-controller): add terminal-phase gate to spawn predicate — tasks with phase ∈ {human_review, done} are never re-spawned; suppressed spawns emit info log line `controller: spawn suppressed phase=<phase> task=<id>`
```

If no CHANGELOG exists in the owning repo, skip this step.

---

## Step 7 — Run `make precommit`

```bash
cd $PREDICATE_SUBDIR && make precommit
```

Must exit 0. If any target fails, fix it and re-run only the failing target (`make lint`, `make gosec`, etc.) before re-running full `make precommit`.

---

## Step 8 — Push branch and create PR in bborbe/agent

**If the fix is in /workspace (maintainer):** stop here — dark-factory handles git for /workspace. Do NOT commit or push.

**If the fix is in /tmp/bborbe-agent (external repo):**

```bash
cd /tmp/bborbe-agent
git checkout -b fix/task-controller-terminal-phase-spawn-gate
git add $PREDICATE_FILE $PREDICATE_TEST_FILE CHANGELOG.md  # add only touched files
git status  # verify only intended files are staged
git commit -m "fix(task-controller): add terminal-phase gate to spawn predicate

Tasks with phase in {human_review, done} are never re-spawned
regardless of status, preventing the re-spawn loop observed on
2026-05-16 task 22fda7e7.

Suppressed spawns emit info log line for operator visibility.
Regression test covers all 6 rows from the bug report."

git push -u origin fix/task-controller-terminal-phase-spawn-gate

gh pr create \
  --title "fix(task-controller): add terminal-phase gate to spawn predicate" \
  --body "$(cat <<'EOF'
## Summary

- Adds terminal-phase gate to the spawn predicate: tasks with `phase ∈ {human_review, done}` are never re-spawned regardless of `status`
- Encodes terminal-phase set as a named `terminalPhases` variable — single source of truth
- Suppressed spawns emit `controller: spawn suppressed phase=<phase> task=<id>` at info level
- Code comment at the gate names the contract: "terminal phases must not be spawned again — operator escalation required"

## Motivation

On 2026-05-16, task `22fda7e7` spawned a second `pr-reviewer` pod after pod 1 had already set `phase: human_review`. Pod 2 dismissed pod 1's correctly-posted GitHub review (via a separate dismissal-filter bug), hiding a hallucination escalation signal the operator needed to see.

Root cause: the spawn predicate gated on `status: in_progress` but ignored `phase`. This PR fixes the controller-side trigger.

## Test plan

- [ ] `DescribeTable` covers all 6 regression rows: `phase=in_progress` spawns, `phase=human_review` suppresses, `phase=done` suppresses, `phase=ai_review` spawns, `status=completed` suppresses, `phase=nil` spawns
- [ ] Cross-cycle test: in_progress→human_review between two reconcile cycles → exactly 1 spawn total
- [ ] Parse-error path: corrupted frontmatter → no spawn, no `spawn_suppressed` log
- [ ] `make precommit` exits 0
EOF
)"
```

Report the PR URL in the completion report.

</requirements>

<constraints>
- The phase enum's existing constants MUST NOT be renamed or removed. `terminalPhases` is a new package-level variable; no existing constants change.
- Terminal-phase set: exactly `{domain.TaskPhaseHumanReview, domain.TaskPhaseDone}`. Do NOT add `TaskPhaseAborted` — it does not exist in vault-cli v0.64.0.
- The controller's reconcile interval and existing log keys for successful spawn events MUST NOT change. The suppression log line is additive only.
- Existing controller tests MUST continue to pass; new tests are additive only.
- Phase is re-read from the task frontmatter on every reconcile call (not cached). Confirm by reading the predicate code; document the confirmation in `## Improvements` if re-reading was already guaranteed.
- The spawn suppression log is NOT emitted on the parse-error path. The parse-error return must be before the phase check.
- **If the fix is in /workspace:** do NOT commit or push — dark-factory handles git.
- **If the fix is in bborbe/agent:** push a feature branch and create a PR via `gh pr create`. Stop after `gh pr create` — do NOT merge.
- `make precommit` runs from the owning service subdirectory, never at repo root.
- Error wrapping: `github.com/bborbe/errors` only — never `fmt.Errorf` — in production code paths.
- Tests: external test package (`package_test`), coverage ≥80% for the modified package.
- Do NOT commit in /workspace. dark-factory handles git for /workspace.
- No Claude attribution in commit messages.
</constraints>

<verification>

```bash
# In the owning repo's service subdirectory:
cd $PREDICATE_SUBDIR && make precommit
# Expected: exit 0
```

Confirm terminal-phase gate and comment exist in the predicate file:
```bash
grep -n "terminal phases must not be spawned\|spawn suppressed\|terminalPhases" $PREDICATE_FILE
# Expected: ≥2 lines — the comment + the terminalPhases var declaration + the gate/log
```

Confirm terminal-phase set contains exactly {human_review, done} — no aborted:
```bash
grep -A3 "terminalPhases\s*=" $PREDICATE_FILE
# Expected: human_review and done; NO aborted
```

Confirm IsTerminal() does NOT exist (we used terminalPhases.Contains instead):
```bash
grep -rn "func.*IsTerminal" $PREDICATE_FILE
# Expected: zero matches (terminalPhases.Contains is used, not a method)
```

Confirm all 6 DescribeTable rows are present in the test file:
```bash
grep -n "in_progress.*spawn\|human_review.*no spawn\|done.*no spawn\|ai_review.*spawn\|completed.*no spawn\|nil.*spawn\|phase=nil\|phase absent" $PREDICATE_TEST_FILE
# Expected: ≥6 matches
```

Confirm cross-cycle test is present:
```bash
grep -n "sequential\|cross.*cycle\|in_progress.*human_review.*1 spawn\|spawnCount.*Equal.*1" $PREDICATE_TEST_FILE
# Expected: ≥1 match
```

Confirm spawn_suppressed log in predicate source, NOT in parse-error path:
```bash
grep -n "spawn suppressed\|spawn_suppressed" $PREDICATE_FILE
# Expected: ≥1 match; verify by reading context that it is inside the terminal-phase gate, not in a parse-error branch
```

If fix is in external repo — confirm PR was created:
```bash
cd /tmp/bborbe-agent && gh pr view --json url -q .url
# Expected: a GitHub PR URL
```

</verification>
