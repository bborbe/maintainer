---
status: completed
spec: [074-pr-reviewer-plan-json-resilience]
summary: Added JSON-safety pre-validation in planningStep.Run (returns AgentStatusFailed without persisting on malformed JSON) and appended JSON safety rules to planning_output-format.md; updated the malformed-JSON test and added the vault-cli#27 live-bug regression test; 284/284 tests pass, make precommit exits 0.
container: maintainer-pr-reviewer-plan-fix-exec-264-spec-074-pr-reviewer-plan-json-resilience
dark-factory-version: v0.183.0
created: "2026-06-26T12:37:36Z"
queued: "2026-06-26T12:37:36Z"
started: "2026-06-26T12:37:37Z"
completed: "2026-06-26T12:41:06Z"
---

# 074 — PR Reviewer: Validate ## Plan JSON Before Persisting

## Context

`agent/pr-reviewer/pkg/steps_planning.go` currently writes Claude's raw output straight to `## Plan` without validating the JSON first. When Claude embeds a Go code snippet with literal double quotes (e.g. `name != ""`), the bare quotes break JSON parsing. `routeFromPlan` catches the malformed body and routes to `human_review` — a terminal dead-end that no human can productively resolve (the markdown looks fine; only the JSON parser is unhappy). Three triggers on vault-cli#27 (2026-06-26) all hit this same dead-end.

This prompt adds two complementary defences:
1. **Prompt fix**: add an explicit "JSON safety" rule to `planning_output-format.md` telling Claude to escape inner double quotes.
2. **Write-time validation**: parse `runResult.Result` via `parsePlanningConcerns` *before* calling `md.ReplaceSection("## Plan", ...)`. If parsing fails, return `AgentStatusFailed` (do NOT write the malformed body) so the controller's retry policy spawns a fresh planning attempt.

The idempotency path (`if section, exists := md.FindSection("## Plan"); exists`) at line 84-87 of `steps_planning.go` is **unchanged** — it is only exercised on retrigger with a valid persisted Plan.

**Working directory:** `agent/pr-reviewer/`  
**Branch:** `dark-factory/pr-reviewer-plan-json-resilience`

### Files to read before implementing

- `agent/pr-reviewer/pkg/steps_planning.go` — full file; understand the `Run` method (lines 83–115) and `routeFromPlan` (lines 121–150)
- `agent/pr-reviewer/pkg/prompts/planning_output-format.md` — current output-format prompt
- `agent/pr-reviewer/pkg/steps_planning_test.go` — existing tests, especially the "when ## Plan JSON is malformed" context (lines 379–400) which **must be updated** to reflect the new behaviour

### Key code currently at steps_planning.go lines 108–114

```go
// Write ## Plan to vault first (vault-first, same invariant as ## Review).
md.ReplaceSection(agentlib.Section{
    Heading: "## Plan",
    Body:    runResult.Result,
})

return s.routeFromPlan(ctx, md, runResult.Result)
```

This is the section that must be restructured to validate before writing.

## Goal

1. `planning_output-format.md` gains an explicit "JSON safety" rule with code-snippet examples.
2. `planningStep.Run` validates `runResult.Result` via `parsePlanningConcerns` before `md.ReplaceSection`. Parse failure → `AgentStatusFailed`, `## Plan` not written.
3. Existing happy-path tests continue to pass unchanged.
4. The existing "routes to human_review" test for malformed JSON is updated to the new expected behaviour: `AgentStatusFailed` + `## Plan` not written.
5. Three new regression tests are added (see Requirements below).

## Requirements

### 1 — `agent/pr-reviewer/pkg/prompts/planning_output-format.md`

Append a "JSON safety" section **after** the `concerns` field rule and **before** the closing paragraph about the fenced code block. Exact wording (do not rephrase):

```
JSON safety: All string values in the JSON output MUST be valid JSON strings.
Double quotes that appear inside a string value (e.g. in code snippets) MUST
be escaped as `\"`. Examples:

- Go snippet `name != ""`  → write as `"note": "name != \"\""`
- Go snippet `if s == ""`  → write as `"note": "if s == \"\""`

Single quotes (`'`) and backticks (`` ` ``) do NOT need escaping.
If in doubt, rephrase the note to avoid literal double quotes.
```

Acceptance: `grep -F 'name != \"\"' agent/pr-reviewer/pkg/prompts/planning_output-format.md` returns ≥1 line.

### 2 — `agent/pr-reviewer/pkg/steps_planning.go`

Restructure the block at lines 108–114 inside `planningStep.Run` so that parse-validation happens **before** the vault write. The new block (replace lines 108–114):

```go
// Validate the JSON before persisting. If Claude emitted unescaped double
// quotes (e.g. code snippets like name != ""), do NOT write the bad body —
// return AgentStatusFailed so the controller's retry spawns a fresh call.
if _, parseErr := parsePlanningConcerns(ctx, runResult.Result); parseErr != nil {
    glog.V(2).Infof("planning: malformed JSON from claude, not persisting err=%v", parseErr)
    return &agentlib.Result{
        Status:  agentlib.AgentStatusFailed,
        Message: fmt.Sprintf("planning: malformed JSON: %v", parseErr),
    }, nil
}

// Write ## Plan to vault (vault-first invariant, same as ## Review).
md.ReplaceSection(agentlib.Section{
    Heading: "## Plan",
    Body:    runResult.Result,
})

return s.routeFromPlan(ctx, md, runResult.Result)
```

No other changes to `steps_planning.go`. The `routeFromPlan` method (lines 121–150) is **unchanged** — it stays as defense-in-depth for any historical task files with stale malformed `## Plan` from before this fix.

### 3 — `agent/pr-reviewer/pkg/steps_planning_test.go`

#### 3a — Update existing malformed-JSON test

The `Context("when ## Plan JSON is malformed")` block (around lines 379–400) currently asserts:
```go
Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
Expect(result.NextPhase).To(Equal("human_review"))
Expect(result.Message).To(ContainSubstring("parse ## Plan JSON"))
```

Update it to reflect the new behaviour:
```go
Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
Expect(result.Message).To(ContainSubstring("planning: malformed JSON"))
```

Also add an assertion that `## Plan` was NOT written:
```go
_, exists := md.FindSection("## Plan")
Expect(exists).To(BeFalse())
```

The `md` variable in this test needs to be defined in the `BeforeEach` or at the top of the `Context` so the post-Run assertion can inspect it.

#### 3b — New test: live sample with unescaped double quotes

Add a new `Context` inside `Describe("Run — error cases")`:

```go
Context("when Claude returns a ## Plan body containing unescaped double quotes (live bug vault-cli#27)", func() {
    BeforeEach(func() {
        // The exact class of failure observed on 2026-06-26: a Go zero-string-check
        // `name != ""` embedded literally inside a JSON string value. The outer
        // quote parser closes the string at the first `"`, leaving `!= ` as
        // unexpected tokens.
        liveSample := "```json\n" +
            `{"pr_url":"https://github.com/bborbe/vault-cli/pull/27","pr_title":"fix","base_branch":"main","head_branch":"fix/args","files_changed":["cmd/main.go"],"scope":"bugfix","focus_areas":["correctness"],"concerns":[{"area":"correctness","file":"cmd/main.go","note":"Arg order matters: name != "" must appear after --print"}]}` +
            "\n```"
        runner.RunReturns(&claudelib.ClaudeResult{Result: liveSample}, nil)
    })

    It("returns AgentStatusFailed and does not write ## Plan", func() {
        md, err := agentlib.ParseMarkdown(
            ctx,
            "---\nref: abc123\ntask_identifier: 00000000-0000-0000-0000-000000000001\n---\n# PR Review\n\nhttps://github.com/bborbe/vault-cli/pull/27\n",
        )
        Expect(err).NotTo(HaveOccurred())
        result, err := step.Run(ctx, md)
        Expect(err).NotTo(HaveOccurred())
        Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
        Expect(result.Message).To(ContainSubstring("planning: malformed JSON"))
        _, exists := md.FindSection("## Plan")
        Expect(exists).To(BeFalse())
    })
})
```

#### 3c — New test: retrigger with valid existing ## Plan skips claude

The retrigger idempotency path is already covered in the `Describe("Run — retrigger with existing ## Plan")` block (lines 315–377). Verify those tests still pass — no new test needed for this path.

**Note:** The spec's acceptance criteria list `TestPlanningRereadsExistingPlan` as a required test name. In this codebase the equivalent coverage is the `Describe("Run — retrigger with existing ## Plan")` block. Run it with:
```
go test ./agent/pr-reviewer/pkg/ -v -run TestPkg
```
All existing retrigger tests must pass.

### 4 — `CHANGELOG.md`

Append a bullet under the `## Unreleased` section (create the section if it doesn't exist, just below the top-of-file preamble and above the most recent `## vX.Y.Z` heading) describing the fix. Required wording must contain `pr-reviewer` AND (`plan JSON` OR `planning malformed`) so it satisfies:

```bash
awk '/^## Unreleased/,/^## v/' CHANGELOG.md | grep -niE 'pr-reviewer.*plan.*json|planning.*malformed' | head -1
```

Sample bullet:

```markdown
- fix(pr-reviewer): validate `## Plan` JSON before persisting — malformed JSON (e.g. Claude embedding unescaped quotes from code snippets like `name != ""`) now returns `AgentStatusFailed` and is retried, instead of writing a broken Plan that routes every retrigger to `human_review` as a dead-end.
```

## Verification

```bash
cd agent/pr-reviewer
go test ./pkg/ -v -run TestPkg 2>&1 | tail -30
make precommit
```

## Success Criteria

- [ ] `grep -F 'name != \"\"' agent/pr-reviewer/pkg/prompts/planning_output-format.md` returns ≥1 line
- [ ] `grep -niE 'escape.*quote|backslash.*quote' agent/pr-reviewer/pkg/prompts/planning_output-format.md` returns ≥1 line
- [ ] `planningStep.Run` calls `parsePlanningConcerns` before `md.ReplaceSection("## Plan", ...)` for fresh Claude output — confirmed by reading the final diff
- [ ] When Claude returns malformed JSON, `step.Run` returns `AgentStatusFailed` with message containing `"planning: malformed JSON"` and `## Plan` is NOT written
- [ ] When Claude returns the live sample containing `name != ""` (unescaped), `step.Run` returns `AgentStatusFailed`
- [ ] All pre-existing tests for valid-JSON happy paths (empty concerns, non-empty concerns, retrigger, nil poster) continue to pass
- [ ] `awk '/^## Unreleased/,/^## v/' CHANGELOG.md | grep -niE 'pr-reviewer.*plan.*json|planning.*malformed' | head -1` returns ≥1 line (CHANGELOG bullet present)
- [ ] `make precommit` exits 0

---
<!--dark-factory-completion-report-->
```json
{
  "status": "{{success|partial|failed}}",
  "summary": "{{one sentence}}",
  "verification": {
    "command": "make precommit",
    "exitCode": {{N}}
  }
}
```
