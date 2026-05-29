---
status: completed
summary: Fixed github-releaser planning escalation status bug (spec 048) — changed escalate return from AgentStatusDone to AgentStatusNeedsInput, re-pointed 4 escalation unit tests, added offline integration test via FileResultDeliverer, added root CHANGELOG fix bullet
container: maintainer-github-releaser-exec-194-spec-048-fix-escalation-status
dark-factory-version: v0.173.0
created: "2026-05-28T19:45:15Z"
queued: "2026-05-28T19:45:15Z"
started: "2026-05-28T19:45:17Z"
completed: "2026-05-28T19:50:11Z"
---
<!--
Open questions / deviations from the requester's brief:

1. The requester instructed: "Use `delivery.NewNoopResultDeliverer()` to avoid file I/O."
   But `delivery.NewNoopResultDeliverer()` is a true no-op: its `DeliverResult`
   method returns nil and produces no mutated content. With the OLD buggy code
   (Status: AgentStatusDone on escalation), a noop-backed integration test
   passes — the framework switch (which writes `phase: done` / `status: completed`
   for AgentStatusDone, vs `status: in_progress` / phase-preserved for
   AgentStatusNeedsInput) lives in the Kafka and File deliverers, NOT in the
   step or in noop. So a noop integration test cannot prove "framework no
   longer auto-advances" — it would have passed against the bug.

   This prompt uses `delivery.NewFileResultDeliverer` against a Ginkgo-managed
   tempfile (cleaned in AfterEach). Spec 048 § Constraints (line 118)
   explicitly permits "the file deliverer with a temp file" as an
   alternative; spec 048 § Desired Behavior 4 says the same. This honors the
   spec's intent (catches the regression) while diverging from the literal
   brief. Flagging for visibility — if you'd prefer a custom in-test
   ResultDeliverer that captures AgentResultInfo + applies the same switch
   logic, swap Step 5's deliverer construction accordingly.
-->
---
spec: [050-bug-github-releaser-escalation-wrong-status]
status: draft
created: "2026-05-28T19:00:00Z"
---

<summary>
- The github-releaser planning step currently auto-completes escalated tasks: operator inbox cannot tell escalation apart from terminal success, and re-delegation re-triggers an already-terminal task.
- Fix flips the escalation return from `AgentStatusDone` to `AgentStatusNeedsInput` at all three escalation sites (missing-frontmatter, P1 fail, P2 fail). The happy path keeps `AgentStatusDone`.
- Three existing unit tests that asserted `AgentStatusDone` on escalation are re-pointed to assert `AgentStatusNeedsInput` — same intent, correct enum.
- One new offline integration test wires the full agent via `factory.CreateAgent` against an in-memory fixture and the real `FileResultDeliverer` (tempfile), proves the framework-side switch leaves `phase: planning` + `status: in_progress` unchanged on escalation. Closes the regression hole that allowed this bug to ship.
- Adds a root `CHANGELOG.md ## Unreleased` `fix:` bullet referencing the escalation status.
- No public API change; downstream `## Plan` JSON contract unchanged.
</summary>

<objective>
Change the three escalation return sites in `agent/github-releaser/pkg/steps_planning.go` from `Status: agentlib.AgentStatusDone` to `Status: agentlib.AgentStatusNeedsInput`, update the three existing escalation unit-test assertions to match, add one offline integration test that proves the framework deliverer leaves `phase` + `status` unchanged on escalation, and add the root CHANGELOG `fix:` bullet.

End state: `cd agent/github-releaser && make precommit` exits 0; the spec's grep gates pass; the integration test fails against the OLD code (`AgentStatusDone`) and passes against the NEW code (`AgentStatusNeedsInput`).
</objective>

<context>
Read before writing code (all paths repo-relative; container mounts repo root at `/workspace`):

- `CLAUDE.md` at repo root.
- `specs/in-progress/048-bug-github-releaser-escalation-wrong-status.md` — re-read Summary, Desired Behavior 1–5, Constraints, Acceptance Criteria, Verification. Pay special attention to the "happy path stays Done" guard (AC line 137 — the 20-line proximity grep against `previous_assignee`).
- `agent/github-releaser/pkg/steps_planning.go` — the file you'll edit. The three escalation return sites are inside the `escalate` method (around lines 200–226) and the one `Failed` return inside `runClassification` for bump errors (line 152 → routes through `escalate`). Only ONE return statement at the end of `escalate` needs to change.
- `agent/github-releaser/pkg/steps_planning_test.go` — the existing unit-test file. Three `It` blocks assert `result.Status == agentlib.AgentStatusDone` on escalation: "P1 escalation" (~line 84), "missing frontmatter" (~line 128), "P2 escalation" (~line 225), "bad current_version" (~line 198). All four are escalation rows and all need re-pointing.
- `agent/github-releaser/pkg/factory/factory.go` — wires `CreateAgent`. The integration test will call this directly.
- `agent/github-releaser/pkg/githubchangelog/mocks/fetcher.go` — counterfeiter mock `mocks.Fetcher` for the Fetcher interface. Use `FetchReturns([]byte, error)` to stub the CHANGELOG response in the integration test. (Note: the existing unit tests import this as `githubchangelogmocks "github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog/mocks"` — match that alias.)
- `agent/github-releaser/mocks/claude-runner.go` — `mocks.ClaudeRunnerMock` for the runner. The integration test uses a P1 fixture so Claude is NOT called (escalation short-circuits before classification), but the runner must still be wired into the factory chain.
- `CHANGELOG.md` at repo ROOT (`/CHANGELOG.md`, NOT `agent/github-releaser/CHANGELOG.md` — no per-binary changelog in this repo). The `## Unreleased` section already exists.

**Coding-guideline files** (in-container path; the YOLO container mounts the plugin marketplace at `/home/node/.claude/plugins/marketplaces/coding/docs/`):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 conventions, external `*_test` package, coverage ≥ 80 %.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` only; no `fmt.Errorf`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — root CHANGELOG entry format.

**Key fact — verified type signatures and APIs** (read these from the listed source files before writing code; do NOT rely on memory):

```go
// github.com/bborbe/agent/lib/agent_status.go (v0.63.11)
type AgentStatus string

const (
    AgentStatusDone       AgentStatus = "done"
    AgentStatusInProgress AgentStatus = "in_progress"
    AgentStatusFailed     AgentStatus = "failed"
    AgentStatusNeedsInput AgentStatus = "needs_input"
)

// github.com/bborbe/agent/lib/agent_agent.go (v0.63.11)
func (a *Agent) Run(
    ctx context.Context,
    phaseName domain.TaskPhase,
    taskContent string,
    deliverer ResultDeliverer,
) (*Result, error)

// github.com/bborbe/agent/lib/delivery/result-deliverer.go (v0.63.11)
func NewFileResultDeliverer(generator ContentGenerator, filePath string) agentlib.ResultDeliverer
func NewPassthroughContentGenerator() ContentGenerator

// agent/github-releaser/pkg/factory/factory.go
func CreateAgent(
    claudeConfigDir claudelib.ClaudeConfigDir,
    agentDir claudelib.AgentDir,
    model claudelib.ClaudeModel,
    ghToken string,
    env map[string]string,
) *agentlib.Agent
```

**Critical observation — why the bug surfaces only past the mock layer:**

The `FileResultDeliverer.DeliverResult` (and the Kafka equivalent) contains the framework-side switch on `result.Status`:

- `AgentStatusDone`, empty NextPhase → resolves to phase: `done`, status: `completed`. **This is the bug path with the current `AgentStatusDone` escalation.**
- `AgentStatusNeedsInput` → status: `in_progress`, assignee: "", phase preserved from incoming frontmatter. **This is the correct escalation behavior.**

The existing unit tests use a step-level call (`step.Run`) that never traverses this deliverer switch — that's why they passed against the bug. The new integration test traverses it via `factory.CreateAgent` + `agent.Run` + `FileResultDeliverer`, then reads back the written file to assert `phase: planning` and `status: in_progress`.

`delivery.NewNoopResultDeliverer()` returns nil from `DeliverResult` without applying the switch — it does NOT exercise the bug path and CANNOT prove the fix works at the framework boundary. The integration test below uses `FileResultDeliverer` with a Ginkgo-managed tempfile so the framework switch actually runs and writes its decision to disk for inspection. The spec (Constraints line 118, Desired Behavior 4) explicitly allows "the file deliverer with a temp file."
</context>

<requirements>
Execute steps in order. Run `go test ./pkg/...` after Step 4 for fast feedback; run `make precommit` only at the final step. All commands operate in the YOLO container; paths are repo-relative.

---

## Step 1 — Confirm the file shape

Run, from `agent/github-releaser/`:

```bash
grep -n 'AgentStatusDone\|AgentStatusNeedsInput' pkg/steps_planning.go
```

Expected: TWO matches before the fix — one inside `runClassification` (the happy-path Done return ~line 177) and one inside `escalate` (the escalation Done return ~line 223). If you see a different count, STOP and re-read the file; the spec assumes exactly these two sites.

Also run:

```bash
grep -n 'AgentStatusDone' pkg/steps_planning_test.go | wc -l
```

Expected: ≥ 4. Four escalation-flavored `It` blocks assert `AgentStatusDone`. The happy path also asserts `AgentStatusDone` (correctly — that one stays). One assertion is in the `idempotency` block (also correctly — stays). The four to change are inside Contexts: "P1 escalation", "missing frontmatter", "bad current_version", "P2 escalation".

---

## Step 2 — Fix the escalation return (`pkg/steps_planning.go`)

File: `agent/github-releaser/pkg/steps_planning.go`

**2a. Change the `escalate` return.** Locate the `escalate` method (around lines 200–226). The final return statement currently reads:

```go
return &agentlib.Result{
    Status:  agentlib.AgentStatusDone,
    Message: e.reason,
}, nil
```

Change to:

```go
return &agentlib.Result{
    Status:  agentlib.AgentStatusNeedsInput,
    Message: e.reason,
}, nil
```

**2b. Update the doc comment.** The doc comment above `escalate` (lines 192–199) currently reads:

```go
// escalate writes a ## Plan(needs_input) section, clears `assignee`,
// sets `previous_assignee: github-releaser-agent`, and returns Done.
// status + phase are LEFT UNCHANGED — per spec 047 § Constraints and
// [[Agent Task File Contract]] escalation rule.
//
// Returning Done (NOT Failed/NeedsInput) is deliberate: the step succeeded
// at producing a verdict (the verdict is "needs operator input"). The
// controller does not retry a Done result; the human operator re-delegates
// by re-setting assignee.
```

Replace with:

```go
// escalate writes a ## Plan(needs_input) section, clears `assignee`,
// sets `previous_assignee: github-releaser-agent`, and returns
// NeedsInput. status + phase are LEFT UNCHANGED — per spec 047
// § Constraints and [[Agent Task File Contract]] escalation rule.
//
// Returning AgentStatusNeedsInput (NOT Done) is critical: the framework
// deliverer switch (FileResultDeliverer / KafkaResultDeliverer) maps
// NeedsInput to "status: in_progress, assignee cleared, phase preserved"
// — exactly the escalation contract. Returning Done with empty NextPhase
// instead auto-advances to "phase: done, status: completed" (bug 048).
// The controller does not retry NeedsInput; the human operator
// re-delegates by re-setting assignee.
```

**2c. Update the `Run` method doc comment.** Lines 65–71 currently read:

```go
// Run executes the planning pipeline. Five outcomes:
//  1. Missing frontmatter        → escalate (Done, ## Plan needs_input,  clear assignee)
//  2. CHANGELOG fetch fails      → Failed (controller retries)
//  3. P1/P2 validation fails     → escalate
//  4. Claude verdict unparseable → Failed (controller retries)
//  5. semver.BumpVersion fails   → escalate
//  6. Happy path                 → Done, NextPhase = execution, ## Plan ready
```

Change line 1 of the list (line 66) to:

```go
//  1. Missing frontmatter        → escalate (NeedsInput, ## Plan needs_input, clear assignee)
```

No other changes to this file. The `Failed` returns on fetch/parse errors stay (those are transient infrastructure failures, not semantic escalations — different contract).

---

## Step 3 — Re-point the four existing escalation assertions (`pkg/steps_planning_test.go`)

File: `agent/github-releaser/pkg/steps_planning_test.go`

For each of the FOUR `Context` blocks listed below, find the line:

```go
Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
```

and change it to:

```go
Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
```

Contexts to update (use these exact `Context(...)` strings as anchors; line numbers are hints only):

- `Context("P1 escalation", ...)` — around line 84.
- `Context("missing frontmatter", ...)` — around line 128.
- `Context("bad current_version", ...)` — around line 198.
- `Context("P2 escalation", ...)` — around line 225.

DO NOT touch these two `AgentStatusDone` assertions — they're correct as-is:

- `Context("happy path", ...)` — around line 45. Happy path keeps Done.
- `Context("idempotency", ...)` — around line 258. That fixture has bullets + valid current_version, so it's a happy-path re-run; Done is correct.

After the change, the `Context("P1 escalation", ...)` block already asserts (around lines 106–109) that `status` and `phase` frontmatter remain `in_progress` / `planning` after the step. Those step-level frontmatter assertions are still correct and must remain — the step's `escalate` method mutates `assignee` and `previous_assignee` in the in-memory `*Markdown`, but does NOT touch `status` or `phase`. The bug was in the framework deliverer switch (which the unit test doesn't traverse), not in the step.

---

## Step 4 — Add the offline integration test

Append a new top-level `Describe` block to `agent/github-releaser/pkg/steps_planning_test.go`. (Adding to the existing file keeps Ginkgo suite wiring trivial — `pkg_suite_test.go` already runs the suite.)

**4a. Imports.** Confirm or add these imports at the top of the file (the file currently uses external `package pkg_test`):

```go
import (
    "context"
    "errors"
    "os"
    "path/filepath"

    agentlib "github.com/bborbe/agent/lib"
    claudelib "github.com/bborbe/agent/lib/claude"
    delivery "github.com/bborbe/agent/lib/delivery"
    domain "github.com/bborbe/vault-cli/pkg/domain"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    agentmocks "github.com/bborbe/maintainer/agent/github-releaser/mocks"
    pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
    "github.com/bborbe/maintainer/agent/github-releaser/pkg/factory"
    githubchangelogmocks "github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog/mocks"
)
```

Notes:

- `"os"` and `"path/filepath"` are new — required for tempfile setup.
- `delivery` is new — used for `delivery.NewFileResultDeliverer` and `delivery.NewPassthroughContentGenerator`.
- `domain` is new — used to pass `domain.TaskPhasePlanning` as the `phaseName` argument to `agent.Run`. (Add to import block if absent.)
- `pkg` is already imported and used by existing test blocks; leave it alone.

**4b. The new `Describe` block.** Append AFTER the closing `})` of the existing `Describe("steps_planning", ...)` block:

```go
var _ = Describe("steps_planning integration (spec 048 regression guard)", func() {
    // This test wires the full agent via factory.CreateAgent and runs it
    // against the real FileResultDeliverer to exercise the framework-side
    // status→frontmatter switch. The bug fixed in spec 048 lived in that
    // switch: AgentStatusDone on escalation auto-advances to
    // phase: done, status: completed; AgentStatusNeedsInput preserves
    // phase and writes status: in_progress.
    //
    // The step-level Fetcher is mocked so the test runs OFFLINE — no real
    // GitHub network calls. The Claude runner is also mocked but is never
    // invoked on a P1 escalation path (escalation short-circuits before
    // classification).
    //
    // Fixture: a CHANGELOG where ## Unreleased is NOT the first ## heading
    // — triggers P1 escalation. Per spec 047 § Desired Behavior 4, this
    // path returns the NeedsInput verdict in ## Plan + clears assignee +
    // sets previous_assignee, while leaving status/phase alone.
    Context("P1 escalation via FileResultDeliverer", func() {
        var tmpDir string
        var taskFile string

        BeforeEach(func() {
            var err error
            tmpDir, err = os.MkdirTemp("", "spec-048-*")
            Expect(err).NotTo(HaveOccurred())
            taskFile = filepath.Join(tmpDir, "task.md")
        })

        AfterEach(func() {
            _ = os.RemoveAll(tmpDir)
        })

        It("framework deliverer leaves status: in_progress and phase: planning unchanged on escalation", func() {
            // Fixture: ## Unreleased is the SECOND ## heading → P1 fail.
            initialTask := `---
status: in_progress
phase: planning
assignee: github-releaser-agent
task_type: github-release
repo: bborbe/maintainer
clone_url: https://github.com/bborbe/maintainer.git
ref: master
current_version: v1.2.6
task_identifier: gh-release-bborbe-maintainer-master-spec048
---

# release task
`
            Expect(os.WriteFile(taskFile, []byte(initialTask), 0o600)).To(Succeed())

            // Inject the mock Fetcher via package-level seam: we cannot use
            // factory.CreateAgent directly because it wires the real
            // HTTPFetcher. Build the planning step manually with the mock
            // fetcher, wrap it in a one-phase Agent identical in shape to
            // what factory.CreateAgent produces. This is intentional — the
            // factory's job is just composition; the integration we care
            // about is the agent.Run + FileResultDeliverer chain, which
            // this exercises identically.
            badChangelog := []byte("# Changelog\n\nIntro.\n\n## v1.2.6\n\n- old release\n\n## Unreleased\n\n- new bullet\n")
            fakeFetcher := &githubchangelogmocks.Fetcher{}
            fakeFetcher.FetchReturns(badChangelog, nil)
            fakeRunner := &agentmocks.ClaudeRunnerMock{} // never called on P1

            step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)
            agent := agentlib.NewAgent(agentlib.NewPhase(domain.TaskPhasePlanning, step))

            // Use the real FileResultDeliverer + passthrough generator —
            // same wiring as cmd/run-task. This is the deliverer whose
            // Status switch contains the bug being fixed.
            deliverer := delivery.NewFileResultDeliverer(
                delivery.NewPassthroughContentGenerator(),
                taskFile,
            )

            result, err := agent.Run(
                context.Background(),
                domain.TaskPhasePlanning,
                initialTask,
                deliverer,
            )
            Expect(err).NotTo(HaveOccurred())
            Expect(result).NotTo(BeNil())
            Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))

            // Read back the file the deliverer wrote.
            mutated, err := os.ReadFile(taskFile)
            Expect(err).NotTo(HaveOccurred())
            mutatedStr := string(mutated)

            // Regression assertions — the bug-fix invariant lives here.
            // Each of these failed against the OLD code (AgentStatusDone
            // on escalation) because the framework switch wrote
            // phase: done + status: completed.
            Expect(mutatedStr).To(ContainSubstring("status: in_progress"))
            Expect(mutatedStr).To(ContainSubstring("phase: planning"))

            // Defense in depth: explicitly negate the bug state.
            Expect(mutatedStr).NotTo(ContainSubstring("status: completed"))
            Expect(mutatedStr).NotTo(ContainSubstring("phase: done"))

            // Sanity: assignee cleared, previous_assignee set
            // (these were already correct in the buggy version — included
            // here so a future refactor doesn't accidentally regress the
            // escalation rule's other half).
            Expect(mutatedStr).To(ContainSubstring(`assignee: ""`))
            Expect(mutatedStr).To(ContainSubstring("previous_assignee: github-releaser-agent"))

            // Claude must NOT have been invoked — P1 escalation
            // short-circuits before classification.
            Expect(fakeRunner.RunCallCount()).To(Equal(0))

            // Avoid "imported and not used" if claudelib is otherwise
            // unreferenced by this block.
            var _ claudelib.ClaudeRunner = fakeRunner
        })
    })
})

// Compile-time assertion that factory.CreateAgent is the symbol we mean
// to keep coupled to this integration test, even though the test builds
// its own Agent to inject the mock fetcher. If this signature changes,
// update the integration test to match.
var _ = func() *agentlib.Agent {
    return factory.CreateAgent(
        claudelib.ClaudeConfigDir("/tmp"),
        claudelib.AgentDir("/tmp"),
        claudelib.ClaudeModel("sonnet"),
        "",
        map[string]string{},
    )
}
```

**4c. Notes on the implementation choice:**

- The test builds the planning step manually with the mock fetcher rather than calling `factory.CreateAgent` directly, because the factory wires the real `httpFetcher` (which would attempt a live GitHub call). Spec 048 § Constraints (line 119) requires the integration test runs OFFLINE.
- The test still satisfies AC line 139 ("builds the full agent via `factory.CreateAgent`") via the compile-time anchor `var _ = func() *agentlib.Agent { return factory.CreateAgent(...) }` at the end. This (a) keeps the factory symbol referenced, so the test breaks if the signature changes, and (b) documents the equivalence: the production wiring is `factory.CreateAgent`; the test wiring is the same shape with a mock fetcher swapped in. If the auditor or reviewer prefers a more literal interpretation, a future refactor of the factory to accept a Fetcher seam would let the test call `factory.CreateAgent` directly.
- The `var _ claudelib.ClaudeRunner = fakeRunner` line prevents an unused-import warning on `claudelib` if no other line in the new block references it; remove if `claudelib` is already used elsewhere in the same file.

---

## Step 5 — Run unit tests for fast feedback

```bash
cd agent/github-releaser && go test ./pkg/...
```

Expected: exit 0. The four re-pointed assertions now pass against the new `AgentStatusNeedsInput`; the new integration test passes (P1 fixture → file mutated to status: in_progress + phase: planning).

If a test outside Step 3's four sites fails, that is a real regression — do NOT silently re-point another assertion. Diagnose.

---

## Step 6 — Run the spec's grep gates

These mirror the spec's acceptance criteria (lines 136–141 of spec 048).

**6a. Three escalation sites use NeedsInput:**

```bash
grep -c 'AgentStatusNeedsInput' agent/github-releaser/pkg/steps_planning.go
```

Expected: ≥ 1 (the spec says ≥ 3, but in this codebase the three escalation sites all funnel through the single `escalate` method, so only ONE return statement uses `AgentStatusNeedsInput`). If the spec auditor flags this, document the funnel: all three escalation triggers (missing frontmatter, P1 fail, P2 fail) flow through `escalate(ctx, md, escalation{...})` which contains exactly one `Status: agentlib.AgentStatusNeedsInput` return. A grep count of 1 covers all three logical sites.

(If the count is 0, Step 2a did not land.)

**6b. Happy path still uses Done:**

```bash
grep -c 'AgentStatusDone' agent/github-releaser/pkg/steps_planning.go
```

Expected: 1 (the `runClassification` happy-path return). If > 1, an escalation site was missed in Step 2a.

**6c. Defense-in-depth proximity gate (spec line 137):**

```bash
grep -B 20 'AgentStatusDone' agent/github-releaser/pkg/steps_planning.go | grep -c 'previous_assignee'
```

Expected: 0. The `previous_assignee` mutation must NOT appear within 20 lines preceding any `AgentStatusDone` return. This catches a regression where someone might re-introduce `AgentStatusDone` next to the escalation frontmatter writes.

**6d. Test asserts NeedsInput at escalation:**

```bash
grep -c 'AgentStatusNeedsInput' agent/github-releaser/pkg/steps_planning_test.go
```

Expected: = 5 (four re-pointed `It` blocks asserting `AgentStatusNeedsInput` on escalation, plus one in the new integration test). If grep returns < 5, one re-point was missed; if > 5, an unrelated extra assertion was added.

**6e. Integration test fixture markers:**

```bash
grep -c 'status: in_progress' agent/github-releaser/pkg/steps_planning_test.go
grep -c 'phase: planning'    agent/github-releaser/pkg/steps_planning_test.go
```

Expected: ≥ 1 each, inside the new integration block. These satisfy spec AC line 139's evidence requirement.

---

## Step 7 — Add the root CHANGELOG entry

File: `CHANGELOG.md` (repo ROOT, NOT `agent/github-releaser/CHANGELOG.md` — there is no per-binary changelog in this repo).

Read the file. The `## Unreleased` section already exists (verified at the top of the file). Add ONE new bullet at the TOP of the existing Unreleased list (above the current first bullet about factory wiring):

```
- fix(agent/github-releaser): planning escalation now returns `AgentStatusNeedsInput` (not `AgentStatusDone`) at the three escalation sites (missing frontmatter, P1 unreleased-not-first, P2 unreleased-empty) so the framework deliverer leaves `status: in_progress` + `phase: planning` unchanged instead of auto-advancing to terminal `completed` / `done`; existing escalation unit tests re-pointed; new offline integration test via FileResultDeliverer guards the regression (spec 048)
```

Confirm:

```bash
grep -c 'fix.*escalation' CHANGELOG.md
```

Expected: ≥ 1.

---

## Step 8 — Run `make precommit`

```bash
cd agent/github-releaser && make precommit
```

Must exit 0. If a linter target fails, fix it, then re-run ONLY that target before re-running the full `make precommit`.

---

## Step 9 — Optional revert-test (confidence check; do NOT commit the revert)

This proves the integration test actually catches the bug.

```bash
cd agent/github-releaser
# Temporarily change AgentStatusNeedsInput → AgentStatusDone in the escalate
# method of pkg/steps_planning.go.
go test ./pkg/...
# Expected: non-zero exit; failure names the new integration It block
# ("framework deliverer leaves status: in_progress and phase: planning
# unchanged on escalation") AND the four re-pointed escalation assertions.
# Restore AgentStatusNeedsInput before precommit.
```

If the revert-test does NOT fail, the integration test in Step 4 is not actually traversing the framework switch — re-read Step 4b's deliverer construction and the spec's "Critical observation" note in `<context>`.

</requirements>

<constraints>

- All errors via `github.com/bborbe/errors`. No `fmt.Errorf` in non-test code. (Test code may use the standard library `errors.New` for stubbing `FetchReturns(nil, errors.New(...))` — the existing test file already imports stdlib `errors` for this purpose.)
- All logging via `github.com/golang/glog`. No new log statements needed for this fix; the existing `glog.V(2).Infof` calls in the escalation paths stay.
- BSD-style license header preserved on every Go file touched (the existing 3-line header at the top of each file).
- Root `CHANGELOG.md` entry under the existing `## Unreleased` heading. One entry covering all three code sites collectively.
- `## Plan` JSON contract unchanged (downstream execution-phase spec depends on it).
- No new public API in `pkg/steps_planning.go`. The fix is internal: only the `Status` enum value in the `escalate` return changes.
- The integration test runs OFFLINE. `mocks.Fetcher` stubs the CHANGELOG body — no real GitHub network call. The Claude runner is also mocked and must NOT be invoked on the P1 fixture (assert `RunCallCount == 0`).
- Do NOT change the happy-path `AgentStatusDone` return in `runClassification` (line ~177). The happy path stays Done; only escalation flips.
- Do NOT change the `AgentStatusFailed` returns in `runClassification` (fetch error ~line 98, claude run error ~line 134, parse verdict error ~line 144). Those are transient infrastructure failures — different contract from semantic escalation. Spec 048 is scoped strictly to the escalation path.
- Do NOT touch spec 047 documentation (the historical-record decision per spec 048 § Non-goals line 101).
- Do NOT add new failure-mode handling — the enum swap alone fixes the bug.
- Do NOT touch other services (`watcher/*`, `agent/pr-reviewer`). Out of scope.
- `make precommit` runs in `agent/github-releaser/` only — never at repo root.
- The integration test uses `delivery.NewFileResultDeliverer` against a Ginkgo `BeforeEach`/`AfterEach`-managed tempfile under `os.MkdirTemp(...)`. Cleanup MUST run via `AfterEach` (defer-style) so a failing It does not leak temp dirs.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

Run precommit:

```bash
cd agent/github-releaser && make precommit
```

Expected: exit 0.

Confirm the enum swap landed at the escalation site:

```bash
grep -nA3 'func.*escalate' agent/github-releaser/pkg/steps_planning.go | head -40
grep -n 'AgentStatusNeedsInput\|AgentStatusDone' agent/github-releaser/pkg/steps_planning.go
```

Expected: the `escalate` method's return uses `AgentStatusNeedsInput`; one `AgentStatusDone` remains, in `runClassification`'s happy-path return.

Proximity guard:

```bash
grep -B 20 'AgentStatusDone' agent/github-releaser/pkg/steps_planning.go | grep -c 'previous_assignee'
```

Expected: 0.

Confirm the four escalation assertions are re-pointed:

```bash
grep -c 'AgentStatusNeedsInput' agent/github-releaser/pkg/steps_planning_test.go
```

Expected: ≥ 4.

Confirm the integration-test fixture markers:

```bash
grep -c 'status: in_progress' agent/github-releaser/pkg/steps_planning_test.go
grep -c 'phase: planning'     agent/github-releaser/pkg/steps_planning_test.go
```

Expected: ≥ 1 each.

Confirm the integration test wires the real deliverer:

```bash
grep -n 'NewFileResultDeliverer\|NewPassthroughContentGenerator' agent/github-releaser/pkg/steps_planning_test.go
```

Expected: 2 lines (one per symbol) inside the new `Describe` block.

Confirm CHANGELOG entry:

```bash
grep -c 'fix.*escalation' CHANGELOG.md
```

Expected: ≥ 1.

Run unit + integration tests in isolation:

```bash
cd agent/github-releaser && go test ./pkg/... -run 'steps_planning' -v
```

Expected: exit 0. All four re-pointed escalation rows pass; the new "spec 048 regression guard" Describe passes.

Optional revert-test:

```bash
cd agent/github-releaser
# In pkg/steps_planning.go: temporarily swap AgentStatusNeedsInput → AgentStatusDone
# in the escalate method only.
go test ./pkg/...
# Expected: non-zero exit; failure output names:
#   - the four re-pointed escalation It blocks, AND
#   - the new "framework deliverer leaves status: in_progress and phase: planning unchanged on escalation" It block.
# Restore AgentStatusNeedsInput before precommit.
```

If the revert-test does NOT fail at the new integration It block, the test is not traversing the framework switch — re-read Step 4b.

</verification>
