---
status: failed
spec: [030-bug-pr-reviewer-verdict-parser-silently-inverts-request-changes]
created: "2026-05-16T10:30:00Z"
queued: "2026-05-16T11:05:23Z"
completed: "2026-05-16T11:06:19Z"
branch: dark-factory/bug-pr-reviewer-verdict-parser-silently-inverts-request-changes
lastFailReason: 'setup workflow: git merge origin default branch: merge origin/master: exit status 2'
---

<summary>
- Fix the model-facing output-format prompt: change `request_changes` (underscore) to `request-changes` (hyphen) so the model's output matches the parser's expected spelling
- The JSON verdict parser now normalises common drift before the switch: underscore-vs-hyphen and case variants (`REQUEST-CHANGES`, `Approve`) all map to the canonical binary values
- Unknown verdicts after normalisation (e.g. `"comment"`, `"block"`) fail closed to `request-changes` with a reason naming the raw value
- Missing JSON verdict block fails closed to `request-changes` with reason `"no verdict block"`
- Malformed JSON that contains the `"verdict"` key but cannot be parsed fails closed to `request-changes` with reason `"malformed JSON: <error>"`
- Empty review text fails closed to `request-changes` with reason `"empty review text"` (preserves existing behaviour)
- The entire markdown-heading fallback heuristic is deleted: `checkMustFixContent`, `hasExpectedReviewSections`, `isHorizontalRule`, `isNoneIndicator`, and the `regexp` import are removed from `verdict.go`
- After the fix, the ONLY path to `VerdictApprove` is an explicit, well-formed `"verdict": "approve"` (or case-normalised variant) in the last JSON block
- A `DescribeTable` regression test in `verdict_test.go` covers the 9 normalisation cases required by spec-030 AC
- Existing tests that encoded heuristic reasons (`"must-fix items found"`, `"should-fix items found"`, etc.) are re-pointed to `"no verdict block"` or the appropriate new reason; tests previously returning `VerdictApprove` from the heuristic now return `VerdictRequestChanges`
</summary>

<objective>
Fix the silent-inversion bug where the PR reviewer agent posts APPROVE on GitHub even when its structured JSON verdict says `request_changes`. Root cause: the output-format prompt tells the model to emit `request_changes` (underscore) but the parser only accepts `request-changes` (hyphen). The parser's mismatch causes a silent failure, which previously fell through to a markdown-heading heuristic that defaulted to APPROVE. With the prompt fixed, the parser normalised, and the heuristic deleted, every non-approve outcome falls closed to `request-changes`.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions.

Read the following coding-guideline files (the dark-factory container mounts the `bborbe/coding` plugin at the path resolved by `/coding:find-guide <name>` or the plugin marketplace path provided in your environment — locate via `find / -name go-testing-guide.md 2>/dev/null | head -1` if the standard path is unavailable):
- `go-testing-guide.md` — Ginkgo v2 + Gomega, `DescribeTable`/`Entry`, external `_test` package, coverage ≥80%
- `go-parse-pattern.md` — validate-before-accept: the parser's value-normalisation switch is the choke point; reject anything outside the allowed set there
- `go-enum-type-pattern.md` — normalise before switching; unknown values must be rejected explicitly
- `test-pyramid-triggers.md` — which test types to write for each code change (this is a pure `pkg/` change: unit tests only)

Read `docs/verifying-specs.md` (repo-relative from the maintainer root) — confirms Rung-1 (unit tests + `make precommit`) is the correct verification level for this pure-code-path fix.

**Files to read fully before making any changes:**

1. `agent/pr-reviewer/pkg/prompts/execution_output-format.md` — the model-facing schema; `request_changes` must change to `request-changes`
2. `agent/pr-reviewer/pkg/verdict.go` — full file; understand `parseJSONVerdict`, `ParseVerdict`, the heuristic helpers, and `StripJSONVerdict` (DO NOT touch `StripJSONVerdict`)
3. `agent/pr-reviewer/pkg/verdict_test.go` — full file; every test that will change is listed in step 3 of requirements

**Grep audit — run before any edits:**

```bash
grep -n 'request_changes' agent/pr-reviewer/pkg/prompts/execution_output-format.md
# Expected: ≥1 match (this is the bug — fix it in step 1)

grep -nE 'mustFixPattern|shouldFixPattern|checkMustFixContent|hasExpectedReviewSections' agent/pr-reviewer/pkg/verdict.go
# Expected: ≥1 match (these are the heuristic helpers to delete in step 2)

grep -n 'no must-fix section\|no must-fix items\|must-fix items found\|should-fix items found' agent/pr-reviewer/pkg/verdict_test.go
# Expected: ≥1 match (these are the test assertions to re-point in step 3)
```
</context>

<requirements>
Execute steps in order. Run `make test` after step 3. Run `make precommit` only at the final step.

---

## Step 1 — Fix `execution_output-format.md`

Read `agent/pr-reviewer/pkg/prompts/execution_output-format.md` fully.

Change the verdict field in the schema from:
```
  "verdict": "approve | request_changes",
```
to:
```
  "verdict": "approve | request-changes",
```

That is the only change to this file. Do not touch anything else.

**Verify after edit:**
```bash
grep -n 'request_changes' agent/pr-reviewer/pkg/prompts/execution_output-format.md
# Expected: 0 matches
grep -n 'request-changes' agent/pr-reviewer/pkg/prompts/execution_output-format.md
# Expected: ≥1 match
```

---

## Step 1b — Fix all remaining `request_changes` references (spec constraint)

The spec's Constraints section says "all other docs that reference verdict spelling must agree" with the canonical `request-changes`. The parser normalisation handles runtime drift, but the model-facing prompt text + docs must use the canonical spelling so a future reader does not propagate the wrong form.

Edit each occurrence below, changing `request_changes` → `request-changes`. Read each file before editing.

**File 1: `agent/pr-reviewer/pkg/prompts/execution.go`** — 4 occurrences (lines 39, 40, 45, 46). These are model-facing prompt strings; highest priority because the model copies the spelling it sees.

```bash
grep -n 'request_changes' agent/pr-reviewer/pkg/prompts/execution.go
# Expected before: 4 matches on lines 39, 40, 45, 46
# Expected after: 0 matches
```

**File 2: `agent/pr-reviewer/pkg/prompts/review_workflow.md`** — 1 occurrence on line 24. Model-facing prompt; same priority as execution.go.

```bash
grep -n 'request_changes' agent/pr-reviewer/pkg/prompts/review_workflow.md
# Expected before: 1 match on line 24
# Expected after: 0 matches
```

**File 3: `agent/pr-reviewer/docs/architecture.md`** — 2 occurrences (lines 21, 57). Human-facing architecture doc; canonical spelling for future readers.

```bash
grep -n 'request_changes' agent/pr-reviewer/docs/architecture.md
# Expected before: 2 matches on lines 21, 57
# Expected after: 0 matches
```

**Global verification after step 1b:**

```bash
grep -rn 'request_changes' agent/pr-reviewer/
# Expected: 0 matches across the entire pr-reviewer subtree
```

---

## Step 2 — Rewrite `verdict.go`

Read `agent/pr-reviewer/pkg/verdict.go` fully before editing.

### 2a. Remove the `parseJSONVerdict` function

Delete the entire `parseJSONVerdict` function (currently lines ~38–56). Its logic will be inlined into `ParseVerdict` in 2b.

### 2b. Rewrite `ParseVerdict` — JSON-only, fail-closed, with normalisation

Replace the entire `ParseVerdict` function body with the following implementation. The function signature and doc comment remain the same.

```go
func ParseVerdict(reviewText string) Result {
	if reviewText == "" {
		return Result{
			Verdict: VerdictRequestChanges,
			Reason:  "empty review text",
		}
	}

	block, ok := findLastJSONVerdictBlock(reviewText)
	if !ok {
		return Result{
			Verdict: VerdictRequestChanges,
			Reason:  "no verdict block",
		}
	}

	var jv jsonVerdict
	if err := json.Unmarshal([]byte(block), &jv); err != nil {
		return Result{
			Verdict: VerdictRequestChanges,
			Reason:  "malformed JSON: " + err.Error(),
		}
	}

	// Normalise: lowercase + replace underscores with hyphens
	// so "request_changes", "REQUEST-CHANGES", "Request-Changes" all parse correctly.
	normalized := strings.ToLower(strings.ReplaceAll(jv.Verdict, "_", "-"))
	switch normalized {
	case "approve":
		return Result{Verdict: VerdictApprove, Reason: jv.Reason}
	case "request-changes":
		return Result{Verdict: VerdictRequestChanges, Reason: jv.Reason}
	default:
		return Result{Verdict: VerdictRequestChanges, Reason: "unknown verdict: " + jv.Verdict}
	}
}
```

The `default` branch preserves the raw (unnormalized) value in the reason so logs name the actual hallucinated string.

### 2c. Delete the heuristic helper functions

Delete ALL of the following functions from `verdict.go`. None are called by the new `ParseVerdict` or by `StripJSONVerdict`.

- `hasExpectedReviewSections`
- `checkMustFixContent`
- `isHorizontalRule`
- `isNoneIndicator`

Confirm deletion:
```bash
grep -nE 'func hasExpectedReviewSections|func checkMustFixContent|func isHorizontalRule|func isNoneIndicator' agent/pr-reviewer/pkg/verdict.go
# Expected: 0 matches
```

### 2d. Remove the `regexp` import

After deleting the heuristic, `"regexp"` is no longer used. Remove it from the import block. The remaining imports should be `"encoding/json"` and `"strings"`.

Confirm:
```bash
grep -n '"regexp"' agent/pr-reviewer/pkg/verdict.go
# Expected: 0 matches
```

### 2e. Do NOT touch `StripJSONVerdict` or its helpers

`StripJSONVerdict`, `findVerdictLinesToRemove`, `calculateStartIndex`, `handleCodeFenceStart`, `handleCodeFenceEnd`, `containsVerdictJSON`, `processVerdictLine`, `isValidVerdictJSON`, `markCodeFenceLinesForRemoval`, `buildCleanedText` — leave all of these unchanged. The spec constraint is explicit: `StripJSONVerdict` behaviour MUST NOT change.

**Self-check:** After step 2, run:
```bash
grep -nE 'mustFixPattern|shouldFixPattern|checkMustFixContent|hasExpectedReviewSections|regexp' agent/pr-reviewer/pkg/verdict.go
# Expected: 0 matches
```

---

## Step 3 — Update `verdict_test.go`

Read `agent/pr-reviewer/pkg/verdict_test.go` fully before editing.

There is no `verdict_internal_test.go` — only the external package file needs changes.

### 3a. Tests whose verdict FLIPS from Approve to RequestChanges

The following existing `Context` blocks previously returned `VerdictApprove` via the heuristic. With the heuristic deleted and no JSON block in the review text, they now return `VerdictRequestChanges` with reason `"no verdict block"`.

For each block listed, update BOTH the verdict assertion and the reason assertion:

**"review with Must Fix section saying none (lowercase)"**
- `result.Verdict` assertion: `VerdictApprove` → `VerdictRequestChanges`
- `result.Reason` assertion: `"no must-fix items"` → `"no verdict block"`

**"review with only Nice to Have section"**
- `result.Verdict` assertion: `VerdictApprove` → `VerdictRequestChanges`
- `result.Reason` assertion: `"no must-fix section"` → `"no verdict block"`

**"review with Must Fix None separated by horizontal rules"**
- `result.Verdict` assertion: `VerdictApprove` → `VerdictRequestChanges`
- `result.Reason` assertion: `"no must-fix items"` → `"no verdict block"`

**"Should Fix present but empty — approve"** (currently in the file near line 550)
- `result.Verdict` assertion: `VerdictApprove` → `VerdictRequestChanges`
- Add a reason assertion: `Expect(result.Reason).To(Equal("no verdict block"))`

**"unknown verdict value falls back to heuristic"** (JSON has `{"verdict": "unknown-verdict"}`)
- `result.Verdict` assertion: `VerdictApprove` → `VerdictRequestChanges`
- `result.Reason` assertion: `"no must-fix section"` → `"unknown verdict: unknown-verdict"`

### 3b. Tests whose verdict stays RequestChanges but reason changes

For each block listed, update only the reason assertion (verdict assertion is correct already):

**"review with no recognizable sections"**
- `result.Reason`: `"unparseable review format"` → `"no verdict block"`

**"review with Must Fix section containing items"**
- `result.Reason`: `"must-fix items found"` → `"no verdict block"`

**"review with Must Fix section saying *None*"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"review with Must Fix section saying None identified"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"review with Must Fix section that is empty"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"review with only Should Fix and Nice to Have sections"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"review with only Should Fix section"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"review with h2 Must Fix instead of h3"**
- `result.Reason`: `"must-fix items found"` → `"no verdict block"`

**"review with h2 Must Fix saying None"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"review with Must Fix containing 'No issues found' text"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"review with Must Fix at end of document"**
- `result.Reason`: `"must-fix items found"` → `"no verdict block"`

**"complex review with multiple sections"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"missing verdict field falls back to heuristic"** (JSON `{"reason": "just a reason"}` — no `"verdict"` key)
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"no JSON at all uses heuristic"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"JSON verdict in middle of review is ignored"**
- `result.Reason`: `"must-fix items found"` → `"no verdict block"`

**"Should Fix only — non-empty content triggers request-changes"**
- `result.Reason`: `"should-fix items found"` → `"no verdict block"`

**"JSON verdict with 'comment' value is rejected — falls to heuristic"** (JSON `{"verdict": "comment", "reason": "..."}`)
- `result.Reason`: `"should-fix items found"` → `"unknown verdict: comment"`

**"JSON verdict with extra whitespace"** (JSON `{"verdict": "comment", "reason": "informational only"}`)
- `result.Reason`: `"unparseable review format"` → `"unknown verdict: comment"`

### 3c. Tests with malformed JSON — reason becomes prefixed

**"invalid JSON falls back to heuristic"**

The review text ends with `{"verdict": "approve", "reason": invalid json}`. `findLastJSONVerdictBlock` WILL find this block (the key `"verdict"` is quoted), extract it, then `json.Unmarshal` fails.

- `result.Verdict` assertion stays: `VerdictRequestChanges` ✓
- `result.Reason` assertion: change from `Equal("must-fix items found")` to `HavePrefix("malformed JSON:")`

**"malformed JSON in fenced block falls back to heuristic"**

The review text is `` ```json\n{verdict: invalid no quotes\n``` ``... Note: the JSON key `verdict` is NOT quoted here — `findLastJSONVerdictBlock` scans for lines containing the string `"verdict"` (with quotes). This line does NOT match. So `findLastJSONVerdictBlock` returns false → reason is `"no verdict block"`.

- `result.Verdict` assertion stays: `VerdictRequestChanges` ✓
- `result.Reason` assertion: change from `Equal("must-fix items found")` to `Equal("no verdict block")`

### 3d. Tests that stay unchanged

Do NOT touch the following tests — their assertions remain correct:

- `"empty review text"` — `VerdictRequestChanges`, `"empty review text"` ✓
- `"empty input returns request-changes (fail-closed)"` — same as above ✓
- `"unparseable input (no sections) returns request-changes (fail-closed)"` — `VerdictRequestChanges`, `"no verdict block"` (already the new behaviour)
- `"JSON verdict on bare line"` — `VerdictApprove`, reason `"all checks passed"` (from `jv.Reason`) ✓
- `"JSON verdict inside code fence"` — `VerdictRequestChanges`, reason `"critical security issues"` ✓
- `"multi-line fenced JSON ... approve"` — `VerdictApprove` ✓
- `"multi-line fenced JSON ... request-changes"` — `VerdictRequestChanges` ✓
- `"review with case variations in Must Fix header"` — only checks `result.Verdict` == `VerdictRequestChanges`; no reason assertion. The verdict stays `VerdictRequestChanges` (fail-closed "no verdict block") ✓
- All `Describe("StripJSONVerdict", ...)` tests — unaffected ✓

Also check: `"Should Fix present but empty — approve"` currently has only ONE `It` block (`VerdictApprove`). After changing to `VerdictRequestChanges`, add the reason assertion as a separate `It` block:
```go
It("returns reason 'no verdict block'", func() {
    Expect(result.Reason).To(Equal("no verdict block"))
})
```

### 3e. Verify no stale reason strings remain

```bash
grep -n 'no must-fix section\|no must-fix items\|must-fix items found\|should-fix items found\|unparseable review format' agent/pr-reviewer/pkg/verdict_test.go
# Expected: 0 matches (all re-pointed)
```

### 3f. Add DescribeTable regression test (spec-030 AC requirement)

Append a NEW `Describe` block after the existing `Describe("StripJSONVerdict", ...)` block:

```go
var _ = Describe("ParseVerdict normalisation regression (spec-030)", func() {
	DescribeTable("verdict spelling and case normalisation",
		func(reviewText string, expectedVerdict pkg.Verdict) {
			result := pkg.ParseVerdict(reviewText)
			Expect(result.Verdict).To(Equal(expectedVerdict))
		},
		// (a) canonical hyphen spelling — the parser must always accept this
		Entry("request-changes hyphen → RequestChanges",
			`{"verdict": "request-changes"}`,
			pkg.VerdictRequestChanges,
		),
		// (b) underscore drift — THE SMOKING-GUN ROW.
		// Pre-fix `ParseVerdict` returned VerdictApprove for this input via the
		// deleted heuristic; this row must remain RequestChanges to prove the
		// normalisation switch is load-bearing. The spec-030 revert-test AC
		// requires this row to fail when `strings.ReplaceAll(_, "_", "-")` is
		// removed from the parser.
		Entry("request_changes underscore → RequestChanges (normalised)",
			`{"verdict": "request_changes"}`,
			pkg.VerdictRequestChanges,
		),
		// (c) ALL-CAPS hyphen
		Entry("REQUEST-CHANGES caps → RequestChanges (normalised)",
			`{"verdict": "REQUEST-CHANGES"}`,
			pkg.VerdictRequestChanges,
		),
		// (d) approve canonical
		Entry("approve → Approve",
			`{"verdict": "approve"}`,
			pkg.VerdictApprove,
		),
		// (e) mixed-case approve
		Entry("Approve mixed-case → Approve (normalised)",
			`{"verdict": "Approve"}`,
			pkg.VerdictApprove,
		),
		// (f) unknown value fails closed
		Entry("comment → RequestChanges (fail-closed, unknown verdict)",
			`{"verdict": "comment"}`,
			pkg.VerdictRequestChanges,
		),
		// (g) empty review text fails closed
		Entry("empty text → RequestChanges (fail-closed)",
			``,
			pkg.VerdictRequestChanges,
		),
		// (h) malformed JSON containing quoted "verdict" key — block found, unmarshal fails
		Entry("malformed JSON → RequestChanges (fail-closed)",
			`{"verdict": invalid}`,
			pkg.VerdictRequestChanges,
		),
		// (i) multi-line fenced block with ≥3 prose lines before and ≥1 after
		Entry("multi-line fenced request-changes amid prose → RequestChanges",
			"Line of prose 1.\nLine of prose 2.\nLine of prose 3.\n\n```json\n{\n  \"verdict\": \"request-changes\",\n  \"summary\": \"Issues found.\",\n  \"comments\": []\n}\n```\nTrailing prose line.",
			pkg.VerdictRequestChanges,
		),
	)

	It("unknown verdict reason names the raw value", func() {
		result := pkg.ParseVerdict(`{"verdict": "block"}`)
		Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		Expect(result.Reason).To(Equal("unknown verdict: block"))
	})

	It("request_changes reason is preserved from JSON reason field", func() {
		result := pkg.ParseVerdict(`{"verdict": "request_changes", "reason": "security issue"}`)
		Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		Expect(result.Reason).To(Equal("security issue"))
	})

	It("no verdict block reason is 'no verdict block'", func() {
		result := pkg.ParseVerdict("### Must Fix\n\n- critical item")
		Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		Expect(result.Reason).To(Equal("no verdict block"))
	})
})
```

The last three `It` blocks are in addition to the nine `DescribeTable` entries and cover the reason propagation contract.

---

## Step 4 — Run `make test`

```bash
cd agent/pr-reviewer && make test
```

All tests must pass. If any test fails for a reason NOT listed in step 3 (unexpected heuristic reference), grep the failure message and fix accordingly.

---

## Step 5 — Add CHANGELOG entry

Add to root `CHANGELOG.md` under `## Unreleased` (create the section if it does not exist):

```
- fix(agent/pr-reviewer): verdict parser normalises spelling drift (request_changes → request-changes, case variants); deletes markdown-heading fallback heuristic; any non-approve or absent JSON verdict fails closed to request-changes
```

---

## Step 6 — Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0. If any linter or errcheck target fails, fix it, then re-run only the failing target before re-running `make precommit`.

</requirements>

<constraints>
- The public `Verdict` type and constants `VerdictApprove = "approve"` and `VerdictRequestChanges = "request-changes"` MUST NOT change. Downstream callers (`poster.go`, integration tests) depend on these exact values.
- `StripJSONVerdict` and ALL its private helpers (`findVerdictLinesToRemove`, `containsVerdictJSON`, `processVerdictLine`, `isValidVerdictJSON`, `markCodeFenceLinesForRemoval`, `buildCleanedText`, `calculateStartIndex`, `handleCodeFenceStart`, `handleCodeFenceEnd`) MUST NOT be touched. The spec constraint is explicit.
- `findLastJSONVerdictBlock` and its helpers (`lastIndexOfBrace`, `nextIndexOfMatchingClose`, `extractBlock`, `charPos`) MUST NOT be touched — they are correct and are still used by the new `ParseVerdict`.
- `grep -nE 'mustFixPattern|shouldFixPattern|checkMustFixContent|hasExpectedReviewSections' agent/pr-reviewer/pkg/verdict.go` must return 0 lines after step 2.
- `grep -n '"regexp"' agent/pr-reviewer/pkg/verdict.go` must return 0 lines after step 2.
- `grep -n 'no must-fix section\|no must-fix items\|must-fix items found\|should-fix items found\|unparseable review format' agent/pr-reviewer/pkg/verdict_test.go` must return 0 lines after step 3.
- `grep -n 'DescribeTable' agent/pr-reviewer/pkg/verdict_test.go` must return ≥1 line after step 3f.
- All new and updated tests use Ginkgo v2 + Gomega. External test package (`package pkg_test`). No raw `testing.T` tests.
- No scenario file. This fix is pure `pkg/` code: unit tests are the correct test pyramid tier per `test-pyramid-triggers.md`.
- Do NOT commit — dark-factory handles git.
- `make precommit` runs from `agent/pr-reviewer/`, never repo root.
- Changes are confined to: `agent/pr-reviewer/pkg/verdict.go`, `agent/pr-reviewer/pkg/verdict_test.go`, `agent/pr-reviewer/pkg/prompts/execution_output-format.md`, `agent/pr-reviewer/pkg/prompts/execution.go`, `agent/pr-reviewer/pkg/prompts/review_workflow.md`, `agent/pr-reviewer/docs/architecture.md`, root `CHANGELOG.md`. No other files.
- After step 1b, `grep -rn 'request_changes' agent/pr-reviewer/` returns 0 matches across the entire subtree. The canonical spelling `request-changes` is the only form present in source.
</constraints>

<verification>
```bash
cd agent/pr-reviewer && make precommit
```

Then sanity-grep:

```bash
# Prompt fix — no underscore spelling anywhere in pr-reviewer subtree:
grep -rn 'request_changes' agent/pr-reviewer/
# Expected: 0 matches

grep -n 'request-changes' agent/pr-reviewer/pkg/prompts/execution_output-format.md
# Expected: ≥1 match

grep -rn 'request-changes' agent/pr-reviewer/pkg/prompts/ agent/pr-reviewer/docs/
# Expected: ≥7 matches (execution_output-format.md, execution.go x4, review_workflow.md, architecture.md x2)

# Heuristic helpers deleted:
grep -nE 'mustFixPattern|shouldFixPattern|checkMustFixContent|hasExpectedReviewSections' agent/pr-reviewer/pkg/verdict.go
# Expected: 0 matches

# regexp import removed:
grep -n '"regexp"' agent/pr-reviewer/pkg/verdict.go
# Expected: 0 matches

# ParseVerdict is JSON-only (no regexp, no deleted helper calls):
grep -A 40 'func ParseVerdict' agent/pr-reviewer/pkg/verdict.go
# Expected: only findLastJSONVerdictBlock + json.Unmarshal + normalised switch + fail-closed returns

# Stale heuristic reason strings gone from tests:
grep -n 'no must-fix section\|no must-fix items\|must-fix items found\|should-fix items found\|unparseable review format' agent/pr-reviewer/pkg/verdict_test.go
# Expected: 0 matches

# DescribeTable regression test present:
grep -n 'DescribeTable' agent/pr-reviewer/pkg/verdict_test.go
# Expected: ≥1 match

# All 9 required normalisation rows present:
grep -nE 'request_changes|REQUEST-CHANGES|Approve|unknown verdict|fail-closed|multi-line' agent/pr-reviewer/pkg/verdict_test.go
# Expected: multiple matches covering rows (b), (c), (e), (f), (i)

# StripJSONVerdict untouched:
grep -n 'func StripJSONVerdict\|func findVerdictLinesToRemove\|func containsVerdictJSON' agent/pr-reviewer/pkg/verdict.go
# Expected: all 3 present

# CHANGELOG entry:
grep -n 'verdict parser normalises\|spelling drift' CHANGELOG.md
# Expected: 1 match under ## Unreleased
```
</verification>
