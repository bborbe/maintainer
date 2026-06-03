---
status: completed
spec: [035-bug-pr-reviewer-planning-stale-phase-name]
summary: Replaced four stale 'in_progress' phase literals with 'execution' via domain.TaskPhaseExecution across steps_planning.go, factory.go, steps_planning_test.go, and the k8s Config CR
container: maintainer-exec-132-spec-035-fix-stale-phase-literal
dark-factory-version: v0.169.0
created: "2026-05-23T14:10:00Z"
queued: "2026-05-23T14:58:08Z"
started: "2026-05-23T14:58:09Z"
completed: "2026-05-23T15:08:30Z"
branch: dark-factory/bug-pr-reviewer-planning-stale-phase-name
---

<summary>
- The pr-reviewer planning step still emits the stale phase literal `"in_progress"` instead of the canonical `"execution"` value introduced by spec 032, causing the agentlib validator to reject every non-empty-concerns advancement and short-circuit the task to `done`.
- Four sites in `agent/pr-reviewer/` carry the stale literal: the planner result, the factory phase registration, the planner unit test assertion, and the k8s Config CR `trigger.phases:` list. All four move together in this prompt.
- The three Go sites switch to the exported constant `domain.TaskPhaseExecution` from `github.com/bborbe/vault-cli/pkg/domain`; the YAML site switches to the bare string `execution`.
- The planner unit test that pinned the stale literal is updated to assert the canonical value via the same constant, becoming a regression test for the rename.
- After this fix, non-empty-concerns tasks advance planning → execution → ai_review, the executor Job spawns, and the bot posts a real review with verdict — restoring the spec 034 F2 invariant for the non-empty branch.
- The status-axis literal `in_progress` in the YAML (`trigger.statuses:`) stays untouched — spec 032 renamed phases, not statuses.
</summary>

<objective>
Replace the four stale `"in_progress"` phase-axis literals in `agent/pr-reviewer/` with the canonical `execution` phase value (via `domain.TaskPhaseExecution` at the three Go sites, bare string at the YAML site) so the planner advances non-empty-concerns tasks correctly and the agentlib validator accepts the write.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.

Read these files in full before writing any code:

- `agent/pr-reviewer/pkg/steps_planning.go` — focus on the `planningStep.Run` method (around lines 80–104); the `NextPhase: "in_progress"` literal is the offending site.
- `agent/pr-reviewer/pkg/factory/factory.go` — focus on the `CreateAgent` function (around lines 180–198); the second `agentlib.NewPhase("in_progress", ...)` is the offending site.
- `agent/pr-reviewer/pkg/steps_planning_test.go` — focus on the "non-empty concerns" `Context` block (around lines 255–275); the assertion `Expect(result.NextPhase).To(Equal("in_progress"))` is the offending site.
- `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml` — the `spec.trigger.phases:` list (lines 45–48); the second list entry `- in_progress` is the offending site. The earlier `spec.trigger.statuses: - in_progress` (line 44) is the canonical *status* value and stays untouched.
- `agent/pr-reviewer/domain_normalize_test.go` — confirms `domain.TaskPhaseExecution` is already imported and addressable from this module.
- `specs/in-progress/035-bug-pr-reviewer-planning-stale-phase-name.md` — the spec authorising this work; cross-check every rung-1 acceptance criterion before declaring done.

Read these coding-guideline files (the `bborbe/coding` plugin is mounted in the container at `/home/node/.claude/plugins/marketplaces/coding/docs/`; if not at that path, locate via `find / -name go-testing-guide.md 2>/dev/null | head -1`):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 conventions, external `*_test` package, coverage ≥80%.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — top-level CHANGELOG entry format.

**Key fact — verified type signatures:**

```go
// agent/lib/agent_step.go
type Result struct {
    Status    AgentStatus
    NextPhase string   // <-- string, NOT domain.TaskPhase
    Message   string
    ...
}

// agent/lib/agent_phase.go
import "github.com/bborbe/vault-cli/pkg/domain"

type Phase struct {
    Name  domain.TaskPhase
    Steps []Step
}

func NewPhase(name domain.TaskPhase, steps ...Step) Phase { ... }

// vault-cli/pkg/domain/task_phase.go
type TaskPhase string

const (
    TaskPhaseExecution TaskPhase = "execution"
    ...
)
```

**Implication:**
- At the `NewPhase` call site (factory), `domain.TaskPhaseExecution` is passed directly — its type is `domain.TaskPhase`, which matches the parameter type.
- At the `Result.NextPhase` assignment site (planner), `domain.TaskPhaseExecution` must be converted to `string` because `NextPhase` is plain `string`. Use `string(domain.TaskPhaseExecution)`.
- Same conversion applies in the test assertion: `Equal(string(domain.TaskPhaseExecution))`.

**Import path** for the constant (both files): `github.com/bborbe/vault-cli/pkg/domain`. The pr-reviewer module already depends on `github.com/bborbe/vault-cli v0.66.3` per `agent/pr-reviewer/go.mod`.
</context>

<requirements>
Execute steps in order. Run `make test` after Step 5 for fast feedback; run `make precommit` only at the final step. All commands operate in the YOLO container; paths are repo-relative.

---

## Step 1 — Read every file listed in `<context>`

Do not skip this step. Confirm each offending line is at approximately the location described, by running:

```bash
grep -n 'NextPhase: "in_progress"\|NewPhase("in_progress"\|Equal("in_progress")\|- in_progress' \
  agent/pr-reviewer/pkg/steps_planning.go \
  agent/pr-reviewer/pkg/factory/factory.go \
  agent/pr-reviewer/pkg/steps_planning_test.go \
  agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml
```

Expected: exactly four matches (one per file). If the count is different, STOP and re-read each file — the spec assumes exactly these four sites.

---

## Step 2 — Fix the planner (`pkg/steps_planning.go`)

File: `agent/pr-reviewer/pkg/steps_planning.go`

**2a. Add the import.** Locate the import block at lines 7–18. Add `domain "github.com/bborbe/vault-cli/pkg/domain"` alongside the other named imports. Keep imports gofmt-sorted: stdlib block, then third-party block. The final third-party block must read:

```go
agentlib "github.com/bborbe/agent/lib"
claudelib "github.com/bborbe/agent/lib/claude"
"github.com/bborbe/errors"
domain "github.com/bborbe/vault-cli/pkg/domain"
```

(Goimports will re-sort if needed; the explicit alias `domain` is optional since the package name is already `domain`, but matching the existing pattern in `domain_normalize_test.go` keeps readers oriented.)

**2b. Replace the literal.** Locate the non-empty-concerns return (around lines 99–103):

```go
// Non-empty concerns — advance to in_progress.
return &agentlib.Result{
    Status:    agentlib.AgentStatusDone,
    NextPhase: "in_progress",
}, nil
```

Change to:

```go
// Non-empty concerns — advance to the execution phase (canonical name per
// spec 032; do NOT revert to "in_progress" — the agentlib frontmatter validator
// rejects that stale literal and the task silently short-circuits to done).
return &agentlib.Result{
    Status:    agentlib.AgentStatusDone,
    NextPhase: string(domain.TaskPhaseExecution),
}, nil
```

The `string(...)` conversion is required because `agentlib.Result.NextPhase` is plain `string`, but `domain.TaskPhaseExecution` is of named type `domain.TaskPhase`. Go does not auto-convert named string types in struct literal positions.

**2c. Update the type-doc comment.** The struct doc comment above `planningStep` (around lines 25–28) reads:

```go
// planningStep runs Claude to produce the ## Plan section, then branches:
// - concerns empty → POST LGTM via PrPoster → write ## Verdict → done
// - concerns non-empty → advance to in_progress
```

Change the last line to:

```go
// - concerns non-empty → advance to the execution phase
```

No other changes to this file.

---

## Step 3 — Fix the factory (`pkg/factory/factory.go`)

File: `agent/pr-reviewer/pkg/factory/factory.go`

**3a. Add the import.** Locate the import block (lines 11–29). Add `domain "github.com/bborbe/vault-cli/pkg/domain"` to the third-party block. Final third-party block (alphabetised, matching the existing style of grouped third-party imports separated by a blank line from the local `prpkg`/`git`/`githubposter`/`prompts` block):

```go
agentlib "github.com/bborbe/agent/lib"
claudelib "github.com/bborbe/agent/lib/claude"
delivery "github.com/bborbe/agent/lib/delivery"
"github.com/bborbe/agent/lib/healthcheck"
"github.com/bborbe/cqrs/base"
"github.com/bborbe/errors"
libkafka "github.com/bborbe/kafka"
libtime "github.com/bborbe/time"
domain "github.com/bborbe/vault-cli/pkg/domain"
"github.com/golang/glog"
```

Goimports may reorder — that is fine; the requirement is the import is present and the file compiles.

**3b. Replace the literal.** Locate the `NewAgent` call (around lines 193–197):

```go
return agentlib.NewAgent(
    planningPhase,
    agentlib.NewPhase("in_progress", tokenCheck, executionStep),
    agentlib.NewPhase("ai_review", tokenCheck, reviewStep),
)
```

Change the middle line to use the canonical constant:

```go
return agentlib.NewAgent(
    planningPhase,
    agentlib.NewPhase(domain.TaskPhaseExecution, tokenCheck, executionStep),
    agentlib.NewPhase("ai_review", tokenCheck, reviewStep),
)
```

Note: `agentlib.NewPhase` takes `name domain.TaskPhase`, so `domain.TaskPhaseExecution` passes directly with no conversion (the parameter type matches the constant's type). The other two `NewPhase` calls (`"planning"` and `"ai_review"`) are out of scope — spec 035 only addresses the phase-axis leftover named `in_progress`.

No other changes to this file.

---

## Step 4 — Fix the planner test (`pkg/steps_planning_test.go`)

File: `agent/pr-reviewer/pkg/steps_planning_test.go`

**4a. Add the import.** Locate the import block (lines 7–18). Add `domain "github.com/bborbe/vault-cli/pkg/domain"` to the third-party block. The file uses external `package pkg_test` — keep the existing style.

**4b. Re-point the assertion.** Locate the `It` block (around lines 263–268):

```go
It("returns status done with NextPhase in_progress", func() {
    result, err := step.Run(ctx, md)
    Expect(err).NotTo(HaveOccurred())
    Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
    Expect(result.NextPhase).To(Equal("in_progress"))
})
```

Change to:

```go
It("returns status done with NextPhase execution (canonical phase per spec 032)", func() {
    result, err := step.Run(ctx, md)
    Expect(err).NotTo(HaveOccurred())
    Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
    // Boundary contract: the value emitted here must equal the canonical
    // domain.TaskPhase constant — string-typed because agentlib.Result.NextPhase
    // is plain string. Reverting to "in_progress" causes the agentlib frontmatter
    // validator to reject the write at delivery time (spec 035 root cause).
    Expect(result.NextPhase).To(Equal(string(domain.TaskPhaseExecution)))
})
```

This is a boundary-contract test: it traverses the same `string(domain.TaskPhaseExecution)` path the production planner traverses, so reverting the planner literal back to `"in_progress"` would make this assertion fail. (The spec's rung-1 revert-test gate depends on this.)

No other changes to this file. The "does NOT call PostLGTM" and "does NOT write ## Verdict section" `It` blocks immediately after are unchanged.

---

## Step 5 — Fix the k8s Config CR (`k8s/maintainer-agent-pr-reviewer.yaml`)

File: `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml`

Locate the `spec.trigger:` block (lines 42–48):

```yaml
  trigger:
    statuses:
      - in_progress
    phases:
      - planning
      - in_progress
      - ai_review
```

Change ONLY the `phases:` list entry (line 47) from `- in_progress` to `- execution`. The result must be:

```yaml
  trigger:
    statuses:
      - in_progress
    phases:
      - planning
      - execution
      - ai_review
```

**Critical:** do NOT change `statuses: - in_progress` (line 44). Spec 032 kept `in_progress` as the canonical *status* value; only the phase axis was renamed. Changing the status entry is out of scope and would break unrelated trigger routing.

No other changes to this file. The YAML cannot reference Go constants — use the bare string `execution`.

---

## Step 6 — Run fast tests

```bash
cd agent/pr-reviewer && go test ./pkg/...
```

Expected: exit 0. The updated planner test asserts the canonical value via `string(domain.TaskPhaseExecution)`.

If any test fails, fix the root cause before proceeding. Do NOT mass-update other tests — only the four sites in this prompt's scope change. If a test outside the four sites fails, that is a real regression; surface it.

---

## Step 7 — Run the grep gates from the spec

These mirror the spec's rung-1 acceptance criteria. All must pass before precommit.

**7a. Zero stale literals in production code:**

```bash
grep -rn '"in_progress"' agent/pr-reviewer/pkg/ agent/pr-reviewer/k8s/ \
  --include='*.go' --include='*.yaml' \
  | grep -v '_test.go'
```

Expected: zero lines. (Test files exercising legacy-frontmatter read-compat — e.g. `domain_normalize_test.go` — are allowed to keep the literal; they exercise the read-side compat path, not the write path this spec is fixing.)

The YAML `statuses: - in_progress` line WILL show as a match. Confirm by reading the output that the only YAML match is on the `statuses:` line (line 44 area), NOT the `phases:` block. If a `phases:` line still shows the literal, Step 5 was not applied correctly.

**7b. Planner uses the constant:**

```bash
grep -n 'NextPhase' agent/pr-reviewer/pkg/steps_planning.go
```

Expected: the production assignment line shows `string(domain.TaskPhaseExecution)`. No `"in_progress"` or bare `"execution"` literal.

**7c. Factory uses the constant:**

```bash
grep -n 'NewPhase' agent/pr-reviewer/pkg/factory/factory.go
```

Expected: the second `NewPhase(...)` call's first argument is `domain.TaskPhaseExecution`. No `"in_progress"` literal.

**7d. YAML trigger phases:**

```bash
grep -nA3 'trigger:' agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml
```

Expected output contains `phases:` followed by `- planning`, `- execution`, `- ai_review`. Does NOT contain `- in_progress` under `phases:`.

**7e. Test asserts the constant:**

```bash
grep -n 'TaskPhaseExecution\|"in_progress"' agent/pr-reviewer/pkg/steps_planning_test.go
```

Expected: ≥1 `TaskPhaseExecution` match; 0 `"in_progress"` matches.

---

## Step 8 — Add CHANGELOG entry

File: `CHANGELOG.md` (repo root, NOT `agent/pr-reviewer/CHANGELOG.md` — there is no per-binary changelog in this repo).

Read the file. The most recent heading is `## v0.25.10`. If a `## Unreleased` heading already exists above it, append to that section; otherwise add `## Unreleased` immediately above `## v0.25.10` and place the entry there:

```
- fix(agent/pr-reviewer): planner now advances non-empty-concerns tasks with `NextPhase: "execution"` (via `domain.TaskPhaseExecution`) instead of the stale `"in_progress"` literal that spec 032 renamed; factory + k8s Config CR `trigger.phases` + planner unit test all moved to the canonical value; restores the spec 034 F2 invariant for the non-empty-concerns branch (spec 035)
```

---

## Step 9 — Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0. If any linter target fails, fix it, then re-run ONLY that target (`make lint`, `make gosec`, etc.) before re-running the full `make precommit`.

---

## Step 10 — Manual revert-test (per spec rung-1 gate)

This is optional but the spec calls it out as a confidence check. Do NOT commit the revert.

```bash
cd agent/pr-reviewer
# Temporarily revert pkg/steps_planning.go NextPhase assignment back to "in_progress".
go test ./pkg/...
# Expected: non-zero exit; failure output names the planner non-empty-concerns row
#   ("returns status done with NextPhase execution (canonical phase per spec 032)").
# Revert the revert (restore string(domain.TaskPhaseExecution)) before continuing.
```

If the revert-test does NOT fail, the assertion in Step 4 is not actually checking the canonical value — re-read Step 4b and fix.

</requirements>

<constraints>

- All errors via `github.com/bborbe/errors`. No `fmt.Errorf` in non-test code.
- All logging via `github.com/golang/glog`.
- BSD-style license header preserved on all touched Go files (the existing 3-line header at the top of each file).
- CHANGELOG entry under the `## Unreleased` heading (create if missing). One entry, covering all four sites collectively.
- The canonical constant `domain.TaskPhaseExecution` (import path `github.com/bborbe/vault-cli/pkg/domain`) MUST be used at all three Go sites — no bare `"execution"` string literal in Go code. The YAML site uses bare `execution` (YAML cannot reference Go constants).
- At the `agentlib.Result.NextPhase` site (planner + test), use `string(domain.TaskPhaseExecution)` because `NextPhase` is plain `string`. At the `agentlib.NewPhase` site (factory), pass `domain.TaskPhaseExecution` directly because the parameter type is `domain.TaskPhase`.
- Do NOT introduce a phase-name alias map or backwards-compat shim. Spec 032's invariant is one canonical name per phase; aliases would re-create the drift this spec fixes.
- Do NOT touch other services (`watcher/github-pr`, `watcher/github-build`, other agents). Out of scope.
- Do NOT touch the YAML `statuses: - in_progress` line (canonical status value, spec 032 only renamed phases).
- Do NOT change `agentlib` validator behavior.
- Do NOT add new failure-mode handling — the rename alone restores the happy path.
- Existing passing tests under `agent/pr-reviewer/pkg/` MUST continue to pass after Step 4's assertion update — only the one assertion in `steps_planning_test.go` (non-empty-concerns row) changes.
- `make precommit` runs in `agent/pr-reviewer/` only — never at repo root.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

Run precommit:

```bash
cd agent/pr-reviewer && make precommit
```

Expected: exit 0.

Confirm the rename landed at all four sites:

```bash
grep -n 'NextPhase' agent/pr-reviewer/pkg/steps_planning.go | grep -i 'execution\|TaskPhase'
grep -n 'NewPhase' agent/pr-reviewer/pkg/factory/factory.go | grep -i 'TaskPhaseExecution\|"planning"\|"ai_review"'
grep -n 'TaskPhaseExecution' agent/pr-reviewer/pkg/steps_planning_test.go
grep -nA3 'trigger:' agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml
```

Expected:
- planner: production line shows `string(domain.TaskPhaseExecution)`.
- factory: shows three NewPhase calls — `"planning"`, `domain.TaskPhaseExecution`, `"ai_review"`.
- test: ≥1 line in the non-empty-concerns assertion.
- yaml: `phases:` block contains `- planning`, `- execution`, `- ai_review` and NOT `- in_progress`.

Confirm zero stale literals in production code paths:

```bash
grep -rn '"in_progress"' agent/pr-reviewer/pkg/ agent/pr-reviewer/k8s/ \
  --include='*.go' --include='*.yaml' \
  | grep -v '_test.go'
```

Expected: at most ONE line — the YAML `statuses: - in_progress` line (line 44). NO matches in `.go` files. NO matches in the YAML `phases:` block.

Confirm imports landed:

```bash
grep -n 'bborbe/vault-cli/pkg/domain' \
  agent/pr-reviewer/pkg/steps_planning.go \
  agent/pr-reviewer/pkg/factory/factory.go \
  agent/pr-reviewer/pkg/steps_planning_test.go
```

Expected: three lines (one per file).

Confirm CHANGELOG entry:

```bash
grep -n 'spec 035\|stale.*in_progress\|TaskPhaseExecution\|NextPhase.*execution' CHANGELOG.md | head -3
```

Expected: ≥1 entry under `## Unreleased`.

Tests pass:

```bash
cd agent/pr-reviewer && go test ./pkg/...
```

Expected: exit 0.

Revert-test (optional confidence check — do NOT commit the revert):

```bash
cd agent/pr-reviewer
# Temporarily set NextPhase back to "in_progress" in pkg/steps_planning.go.
go test ./pkg/...
# Expected: non-zero exit, failure names the planner non-empty-concerns It block.
# Restore string(domain.TaskPhaseExecution) before precommit.
```

</verification>
