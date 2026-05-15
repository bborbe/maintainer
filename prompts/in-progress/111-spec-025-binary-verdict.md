---
status: committing
spec: [025-pr-reviewer-binary-verdict]
summary: 'Collapsed the pr-reviewer verdict enum from three values to two (approve/request-changes): removed VerdictComment constant, updated tryParseJSONLine to reject ''comment'' JSON verdict, rewrote ParseVerdict with Should Fix detection and fail-closed defaults, updated all tests, prompts, docs, READMEs, fixture files, and CHANGELOG.'
container: maintainer-111-spec-025-binary-verdict
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-15T13:20:00Z"
queued: "2026-05-15T13:27:01Z"
started: "2026-05-15T13:32:03Z"
branch: dark-factory/pr-reviewer-binary-verdict
---

<summary>
- The `comment` verdict is removed end-to-end from the pr-reviewer agent — compiler enforces the deletion
- The verdict enum shrinks from three values to two: `approve` and `request-changes` only
- The JSON-line parser rejects `"comment"` as an unknown verdict and falls through to the heuristic (same as any other unknown string)
- The heuristic gains a Should Fix detector using the same content-inspection logic already used for Must Fix (skips "None", skips horizontal rules, skips empty lines)
- Empty or unparseable review text now returns `request-changes` instead of `comment` — fail-closed, never silently approves
- The LLM-facing prompts (execution footer, output-format schema) describe exactly two verdicts and the four-row mapping table
- The ai_review consistency-check rule that special-cased `comment` is removed
- `docs/architecture.md` phase table and heuristic-fallback section are updated; the note that calls the `comment` fallback a bug is converted to accurate documentation
- All tests that asserted `VerdictComment` are rewritten; new tests cover the three previously-`comment` paths
- `make precommit` in `agent/pr-reviewer/` passes
</summary>

<objective>
Collapse the pr-reviewer verdict enum from three values to two. Every review now ends with exactly `approve` or `request-changes`. Should Fix findings escalate to `request-changes` (was `comment`). Empty or unparseable agent output defaults to `request-changes` (fail-closed). No code path may produce `comment` after this change.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions.

Read the following guides before writing any code (in `~/.claude/plugins/marketplaces/coding/docs/`):
- `go-enum-type-pattern.md` — how to safely remove an enum value: audit all references, remove constant, update parser, update tests, update docs
- `go-testing-guide.md` — Ginkgo/Gomega, `DescribeTable`, external `_test` package, coverage ≥80%
- `go-parse-pattern.md` — validate-before-accept: the parser's value-validation switch is the choke point; reject anything outside the allowed set there
- `test-pyramid-triggers.md` — which test types to write for each code change

Read `docs/verifying-specs.md` in `/workspace/` — confirms Rung-1 (unit tests + `make precommit`) is the correct verification level for this spec.

**Files to read fully before making any changes:**

1. `agent/pr-reviewer/pkg/verdict.go` — full file; the enum, `tryParseJSONLine` switch, `ParseVerdict` heuristic, `hasExpectedReviewSections`, `checkMustFixContent`
2. `agent/pr-reviewer/pkg/verdict_test.go` — full file; understand every test case that will change
3. `agent/pr-reviewer/pkg/prompts/execution.go` — full file; the `verdictTranslationFooter` const contains the LLM-facing rubric
4. `agent/pr-reviewer/pkg/prompts/execution_output-format.md` — the JSON schema (`"verdict": "approve | request_changes | comment"` changes)
5. `agent/pr-reviewer/pkg/prompts/review_workflow.md` — the `comment is always consistent` line in the Verdict consistency check
6. `agent/pr-reviewer/agent/.claude/CLAUDE.md` — the headless guardrails; verify no verdict values need changes
7. `agent/pr-reviewer/docs/architecture.md` — phase table and heuristic-fallback section need updating
8. `agent/pr-reviewer/cmd/cli/main.go` — two three-branch dispatches reference `VerdictComment` indirectly (the fallback after `VerdictApprove`/`VerdictRequestChanges` checks)
9. `agent/pr-reviewer/pkg/github/client.go` — `SubmitReview` rejects non-binary verdicts; comment text references `comment` verdict
10. `agent/pr-reviewer/pkg/github/client_test.go` — `It("returns error for VerdictComment", ...)` test will fail to compile after constant removal
11. `agent/pr-reviewer/cmd/run-task/dummy-task.md` — fixture contains `{"verdict":"comment",...}`
12. `agent/pr-reviewer/README.md` — agent README documents three-verdict rubric
13. `README.md` (repo root) — also documents three-verdict rubric in agent description

**Grep audit — run before any edits:**

```bash
grep -rn "VerdictComment\|\"comment\"" agent/pr-reviewer/
```

This is the full list of places that must be touched. Every match must either be deleted or replaced with a binary value before the work is complete. If you find a match not covered by the requirements below, fix it and document it in `## Improvements`.
</context>

<requirements>
Execute steps in order. Run `make test` after step 3. Run `make precommit` only at the final step.

---

## 1. Update `agent/pr-reviewer/pkg/verdict.go`

Read the full file first.

### 1a. Remove `VerdictComment` from the enum

Delete the `VerdictComment` constant line:

```go
const (
    VerdictApprove        Verdict = "approve"
    VerdictRequestChanges Verdict = "request-changes"
)
```

The compiler will now fail on every remaining `VerdictComment` reference — use that list to drive the rest of the changes.

### 1b. Update `tryParseJSONLine` — reject `"comment"`

Remove the `"comment"` case from the switch so it falls through to the `default` (unknown verdict → return false, caller falls through to heuristic):

```go
switch jv.Verdict {
case "approve":
    v = VerdictApprove
case "request-changes":
    v = VerdictRequestChanges
default:
    // Unknown verdict value (including "comment") — fall back to heuristic
    return Result{}, false
}
```

### 1c. Rewrite `ParseVerdict` — add Should Fix detection, make fail-closed

Replace the entire `ParseVerdict` function body. The new implementation:

1. Tries JSON parser first (unchanged — `parseJSONVerdict` call stays)
2. Empty text → `VerdictRequestChanges` "empty review text" (was `VerdictComment`)
3. Scans for both Must Fix and Should Fix section indices in a single loop
4. Must Fix with content → `VerdictRequestChanges` "must-fix items found" (unchanged)
5. Should Fix with content → `VerdictRequestChanges` "should-fix items found" (new)
6. Must Fix header exists but is empty → `VerdictApprove` "no must-fix items" (preserve existing reason)
7. No Must Fix but has review sections (Should Fix empty/None or Nice to Have) → `VerdictApprove` "no must-fix section" (preserve existing reason)
8. No recognizable sections → `VerdictRequestChanges` "unparseable review format" (was `VerdictComment`)

Note: `checkMustFixContent` already implements the correct content-inspection logic (skips None indicators, empty lines, horizontal rules, stops at next heading). Reuse it verbatim for the Should Fix section — the logic is identical.

```go
// ParseVerdict analyzes Claude review output and determines the appropriate verdict.
// The verdict is binary: approve or request-changes. No other value is returned.
// Fail-closed: empty or unparseable output returns request-changes.
func ParseVerdict(reviewText string) Result {
    // First try to extract JSON verdict
    if result, ok := parseJSONVerdict(reviewText); ok {
        return result
    }

    // Fail-closed: empty review text
    if reviewText == "" {
        return Result{
            Verdict: VerdictRequestChanges,
            Reason:  "empty review text",
        }
    }

    mustFixPattern := regexp.MustCompile(`(?i)^##+ Must Fix`)
    shouldFixPattern := regexp.MustCompile(`(?i)^##+ Should Fix`)
    lines := strings.Split(reviewText, "\n")

    mustFixIndex := -1
    shouldFixIndex := -1
    for i, line := range lines {
        trimmed := strings.TrimSpace(line)
        if mustFixIndex == -1 && mustFixPattern.MatchString(trimmed) {
            mustFixIndex = i
        }
        if shouldFixIndex == -1 && shouldFixPattern.MatchString(trimmed) {
            shouldFixIndex = i
        }
    }

    // Must Fix with content → request-changes
    if mustFixIndex != -1 && checkMustFixContent(lines, mustFixIndex) {
        return Result{
            Verdict: VerdictRequestChanges,
            Reason:  "must-fix items found",
        }
    }

    // Should Fix with content → request-changes (new: was previously approve)
    if shouldFixIndex != -1 && checkMustFixContent(lines, shouldFixIndex) {
        return Result{
            Verdict: VerdictRequestChanges,
            Reason:  "should-fix items found",
        }
    }

    // Must Fix section exists but is empty/None → approve
    if mustFixIndex != -1 {
        return Result{
            Verdict: VerdictApprove,
            Reason:  "no must-fix items",
        }
    }

    // No Must Fix; has Should Fix (empty/None) or Nice to Have → approve
    if hasExpectedReviewSections(reviewText) {
        return Result{
            Verdict: VerdictApprove,
            Reason:  "no must-fix section",
        }
    }

    // Fail-closed: no recognizable review sections
    return Result{
        Verdict: VerdictRequestChanges,
        Reason:  "unparseable review format",
    }
}
```

Remove the now-unused `hasExpectedReviewSections` call in the old body. Keep `hasExpectedReviewSections` as a function — it is still called in step 7 above.

### 1d. Update `agent/pr-reviewer/cmd/cli/main.go` — collapse three-branch dispatches to two-branch

Two functions have a three-branch verdict dispatch that ends in a `VerdictComment` fallback comment. After the enum collapse the only verdicts are `Approve` and `RequestChanges` — the fallback path is dead.

**Around line 340 (GitHub dispatch):**

The current shape is `if VerdictApprove { ... } if VerdictRequestChanges { ... }` followed by a `// Fallback to plain comment for VerdictComment` comment and a fall-through call. Restructure to an explicit `switch result.Verdict` with the two cases and a `default` that returns an error: `errors.Errorf("unsupported verdict %q after binary collapse", result.Verdict)`. The default is unreachable for valid enum values but is required for exhaustiveness — wrap with `errors.Wrapf(ctx, ...)` per `bborbe/errors` convention. Remove the stale `// Fallback to plain comment for VerdictComment` line.

**Around line 457 (Bitbucket dispatch):**

Identical pattern. Apply the same `switch` collapse. Remove the `// VerdictComment - no verdict action needed` comment on line 498.

After this change, neither function references `VerdictComment` (compiler-enforced) and neither contains a stale comment about it.

### 1e. Update `agent/pr-reviewer/pkg/github/client.go` — fix stale comment

Line 144 currently reads `// For comment verdict, caller should use PostComment instead` (or similar — check the exact wording). After the binary collapse, the `SubmitReview` defensive branch (`if v != VerdictApprove && v != VerdictRequestChanges`) is unreachable for valid enum values; the only way it triggers is an out-of-enum `Verdict` cast. Update the comment to: `// Defensive: rejects out-of-enum verdict values (the binary set is approve/request-changes)`. No behavioral change.

---

## 2. Update `agent/pr-reviewer/pkg/verdict_test.go`

Read the full file first.

### 2a. Tests to REWRITE (behavior changes — assert new binary values)

The following existing test contexts assert `VerdictComment` or assert `VerdictApprove` for cases that now return `VerdictRequestChanges`. Rewrite each one in-place:

**Context: "empty review text"**
- Was: `VerdictComment`, reason "empty review text"
- Now: `VerdictRequestChanges`, reason "empty review text"

**Context: "review with no recognizable sections"**
- Was: `VerdictComment`, reason "unparseable review format"
- Now: `VerdictRequestChanges`, reason "unparseable review format"

**Context: "JSON verdict with extra whitespace"** (input: `{"verdict": "comment", "reason": "informational only"}`)
- Was: `VerdictComment` from JSON
- Now: JSON "comment" is rejected → falls to heuristic → review has no recognizable sections → `VerdictRequestChanges`, reason "unparseable review format"
- Update both `It` assertions accordingly

**Context: "review with Must Fix section saying *None*"** (has Should Fix: "Add error handling")
- Was: `VerdictApprove`, reason "no must-fix items"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "review with Must Fix section saying None identified"** (has Should Fix: "Improve error messages")
- Was: `VerdictApprove`, reason "no must-fix items"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "review with Must Fix section that is empty"** (has Should Fix: "Add tests")
- Was: `VerdictApprove`, reason "no must-fix items"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "review with only Should Fix and Nice to Have sections"** (Should Fix has content)
- Was: `VerdictApprove`, reason "no must-fix section"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "review with only Should Fix section"** (Should Fix has content)
- Was: `VerdictApprove`, reason "no must-fix section"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "review with h2 Must Fix saying None"** (has Should Fix: "Add tests")
- Was: `VerdictApprove`, reason "no must-fix items"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "review with Must Fix containing 'No issues found' text"** (has Should Fix: "Improve code structure")
- Was: `VerdictApprove`, reason "no must-fix items"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "complex review with multiple sections"** (Must Fix: *None*, Should Fix has two items)
- Was: `VerdictApprove`, reason "no must-fix items"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "missing verdict field falls back to heuristic"** (Should Fix: "Add tests")
- Was: `VerdictApprove`, reason "no must-fix section"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

**Context: "no JSON at all uses heuristic"** (Must Fix: None, Should Fix: "Add error handling")
- Was: `VerdictApprove`, reason "no must-fix items"
- Now: `VerdictRequestChanges`, reason "should-fix items found"

### 2b. Tests that STAY UNCHANGED (verify these still pass)

These tests assert correct behavior that is not changing — do not modify them:

- "review with Must Fix section containing items" → `VerdictRequestChanges` "must-fix items found" ✓
- "review with Must Fix section saying none (lowercase)" (only Nice to Have) → `VerdictApprove` "no must-fix items" ✓
- "review with only Nice to Have section" → `VerdictApprove` "no must-fix section" ✓
- "review with h2 Must Fix instead of h3" → `VerdictRequestChanges` "must-fix items found" ✓
- "review with case variations in Must Fix header" → `VerdictRequestChanges` ✓
- "review with Must Fix None separated by horizontal rules" (Should Fix: None.) → `VerdictApprove` "no must-fix items" ✓
- "review with Must Fix at end of document" → `VerdictRequestChanges` "must-fix items found" ✓
- "JSON verdict on bare line" → `VerdictApprove` from JSON ✓
- "JSON verdict inside code fence" → `VerdictRequestChanges` from JSON ✓
- "invalid JSON falls back to heuristic" → `VerdictRequestChanges` "must-fix items found" ✓
- "unknown verdict value falls back to heuristic" (only Nice to Have) → `VerdictApprove` "no must-fix section" ✓
- "JSON verdict in middle of review is ignored" → `VerdictRequestChanges` "must-fix items found" ✓

### 2c. Add new test cases

Append these new `Context` blocks inside the `Describe("Parse", ...)` block, after all existing contexts:

```go
Context("JSON verdict with 'comment' value is rejected — falls to heuristic", func() {
    BeforeEach(func() {
        // Should Fix has content → after JSON rejection, heuristic returns request-changes
        reviewText = `### Should Fix

- Add error handling

{"verdict": "comment", "reason": "informational only"}`
    })

    It("returns VerdictRequestChanges (not comment)", func() {
        Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
    })

    It("returns reason from heuristic, not from JSON", func() {
        Expect(result.Reason).To(Equal("should-fix items found"))
    })
})

Context("Should Fix only — non-empty content triggers request-changes", func() {
    BeforeEach(func() {
        reviewText = `### Should Fix (Important)

- Improve error handling in pkg/server.go:42`
    })

    It("returns VerdictRequestChanges", func() {
        Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
    })

    It("returns reason 'should-fix items found'", func() {
        Expect(result.Reason).To(Equal("should-fix items found"))
    })
})

Context("Should Fix present but empty — approve", func() {
    BeforeEach(func() {
        reviewText = `### Should Fix (Important)

None.

### Nice to Have

- Add docstrings`
    })

    It("returns VerdictApprove", func() {
        Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
    })
})

Context("empty input returns request-changes (fail-closed)", func() {
    BeforeEach(func() {
        reviewText = ""
    })

    It("returns VerdictRequestChanges", func() {
        Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
    })

    It("returns reason 'empty review text'", func() {
        Expect(result.Reason).To(Equal("empty review text"))
    })
})

Context("unparseable input (no sections) returns request-changes (fail-closed)", func() {
    BeforeEach(func() {
        reviewText = "Just some random prose without any review sections."
    })

    It("returns VerdictRequestChanges", func() {
        Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
    })

    It("returns reason 'unparseable review format'", func() {
        Expect(result.Reason).To(Equal("unparseable review format"))
    })
})
```

---

### 2d. Delete the `VerdictComment` test in `agent/pr-reviewer/pkg/github/client_test.go`

Around line 77, there is `It("returns error for VerdictComment", ...)` that explicitly references `prpkg.VerdictComment`. After constant removal this test will fail to compile.

**Delete the entire `It` block** (the one named "returns error for VerdictComment"). Do NOT replace it — the existing `It("returns error for unknown verdict", ...)` test (or equivalent) at the end of that `Describe` block already covers the out-of-enum rejection path with an arbitrary `Verdict("unknown")` literal. If no such generic test exists, add one:

```go
It("returns error for unknown verdict value", func() {
    err := client.SubmitReview(ctx, owner, repo, prNum, prpkg.Verdict("unknown"), "body")
    Expect(err).To(HaveOccurred())
    Expect(errors.Is(err, prpkg.ErrUnsupportedVerdict)).To(BeTrue())
})
```

(Adjust to match the existing error type used in `client.go`.)

---

## 3. Run `make test`

```bash
cd agent/pr-reviewer && make test
```

All tests must pass. If any test fails because `VerdictComment` is still referenced in a place not listed above (the compiler should have caught it, but `grep` the test output carefully), fix it before proceeding.

---

## 4. Update `agent/pr-reviewer/pkg/prompts/execution.go`

Read the full file first.

Update only the `verdictTranslationFooter` constant. Change the verdict roll-up section and the Should Fix severity line:

**Old `verdictTranslationFooter` (relevant excerpt):**
```go
"Severity map (deterministic):\n" +
"- Must Fix finding → comment severity \"critical\", contributes to verdict \"request_changes\"\n" +
"- Should Fix finding → comment severity \"major\"\n" +
"- Nice to Have finding → comment severity \"nit\"\n" +
...
"Verdict roll-up:\n" +
"- Any Must Fix present → verdict \"request_changes\"\n" +
"- Else any Should Fix or Nice to Have present → verdict \"comment\"\n" +
"- All sections empty (or \"None.\") → verdict \"approve\"\n\n" +
```

**New `verdictTranslationFooter` (replace the entire const):**
```go
const verdictTranslationFooter = "---\n\n" +
    "## Final step — emit verdict JSON\n\n" +
    "After Step 7 (Manual Review) completes and the consolidated report is\n" +
    "produced, ALSO emit a JSON verdict matching the agent's frozen schema (see\n" +
    "`<output-format>`).\n\n" +
    "Severity map (deterministic):\n" +
    "- Must Fix finding → comment severity \"critical\", contributes to verdict \"request_changes\"\n" +
    "- Should Fix finding → comment severity \"major\", contributes to verdict \"request_changes\"\n" +
    "- Nice to Have finding → comment severity \"nit\"\n" +
    "- The severity \"minor\" is reserved for LLM judgment on findings that\n" +
    "  genuinely don't fit a plugin bucket; the deterministic map never emits it.\n\n" +
    "Verdict roll-up (binary — exactly one of two values):\n" +
    "- Any Must Fix present → verdict \"request_changes\"\n" +
    "- Any Should Fix present → verdict \"request_changes\"\n" +
    "- Only Nice to Have, or nothing flagged → verdict \"approve\"\n\n" +
    "Each comment must pin to a real `file` and `line` from the report. If a\n" +
    "finding has no coordinates, fold it into `summary` instead of emitting an\n" +
    "un-pinned comment. Preserve the plugin's bucket label verbatim in the\n" +
    "comment `message` for traceability.\n"
```

---

## 5. Update `agent/pr-reviewer/pkg/prompts/execution_output-format.md`

Read the file first.

Change line 4 from:
```
  "verdict": "approve | request_changes | comment",
```
To:
```
  "verdict": "approve | request_changes",
```

---

## 6. Update `agent/pr-reviewer/pkg/prompts/review_workflow.md`

Read the file first.

In the "Verdict consistency" check section, remove the `comment` special-case line. The section currently reads:

```
3. **Verdict consistency.** Does the verdict match the comments?
   - `approve` + critical/major comments → inconsistent
   - `request_changes` + only nit/minor comments → inconsistent
   - `comment` is always consistent (informational)
```

Change it to:

```
3. **Verdict consistency.** Does the verdict match the comments?
   - `approve` + critical/major comments → inconsistent
   - `request_changes` + only nit/minor comments → inconsistent
```

---

## 7. Update `agent/pr-reviewer/docs/architecture.md`

Read the full file first.

### 7a. Phase table (line ~21)

In the table row for `execution`, the `Emits` column currently says:
```
**the review verdict** (`approve` / `request_changes` / `comment`)
```

Change to:
```
**the review verdict** (`approve` / `request_changes`)
```

### 7b. Heuristic-fallback section (lines ~39-47)

The section currently ends with a parenthetical that calls the `comment` fallback a bug:

```
(The current heuristic fallback returns `comment` on empty/unparseable; this is the bug spec `pr-reviewer-binary-verdict.md` corrects.)
```

Replace the entire heuristic-fallback section with accurate documentation of the new behavior:

```markdown
## Verdict Parsing — The Heuristic Fallback

The execution phase emits the verdict as a JSON block at the end of its output. The deliverer must extract a structured verdict from free-form text the LLM produces, so two parsers run in sequence in `pkg/verdict.go`:

1. **JSON-line parser** (`tryParseJSONLine` → `parseJSONVerdict`) — scans the last 50 lines for a `{"verdict": "...", "reason": "..."}` block, validates the verdict value against the binary set (`approve`, `request-changes`). Any other value (including the old `comment`) is rejected and falls through to the heuristic.
2. **Heuristic fallback** (`ParseVerdict`) — if no valid JSON block is found, scan section headers (`## Must Fix`, `## Should Fix`) and apply the same rubric as the LLM prompt:
   - Must Fix with content → `request-changes`
   - Should Fix with content → `request-changes`
   - Must Fix or Should Fix present but empty/None, or only Nice to Have → `approve`
   - No recognizable sections → `request-changes` (fail-closed)

The fallback exists because LLMs sometimes drop the JSON block under load or wrap it in unexpected markup. **Fail-closed default**: empty or unparseable text returns `request-changes`, never `approve` — a flaky agent run cannot silently green-light a PR.
```

---

### 7c. Update READMEs — drop `comment` from verdict descriptions

**`agent/pr-reviewer/README.md`:**
- Line 3: `(approve / request-changes / comment)` → `(approve / request-changes)`
- Line 93 (in the "Verdict Contract" code-fenced JSON): `"verdict": "approve|request-changes|comment"` → `"verdict": "approve|request-changes"`

**`README.md` (repo root):**
- Line 26: `(approve / request-changes / comment)` → `(approve / request-changes)`
- Line 133 (in the JSON example): `"verdict": "approve|request-changes|comment"` → `"verdict": "approve|request-changes"`

These are textual edits only. Do NOT touch other parts of the README (e.g. `--comment-only` flag references, "post a comment" prose about per-line review messages — those are about messages, not the verdict value).

### 7d. Update `agent/pr-reviewer/cmd/run-task/dummy-task.md` fixture

Line 22 contains `{"verdict":"comment","summary":"..."}`. After the change, this fixture would fall through the JSON parser ("comment" rejected) into the heuristic — misleading for replay use.

Change `"verdict":"comment"` → `"verdict":"approve"` (the Team Health Check fixture summary suggests no blocking findings, so `approve` is the natural binary verdict). Leave the `summary` and rest of the JSON object as-is.

---

## 8. Add CHANGELOG entry

Add to root `CHANGELOG.md` under `## Unreleased` (create the section if it does not exist):

```
- feat(agent/pr-reviewer): collapse verdict from three values to two — every review now ends with approve or request-changes; Should Fix findings escalate to request-changes (was comment); empty or unparseable agent output defaults to request-changes (fail-closed); comment constant removed, compiler-enforced
```

---

## 9. Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0. If any linter or errcheck target fails, fix it, then re-run only that failing target before running `make precommit` again.
</requirements>

<constraints>
- All changes are confined to `agent/pr-reviewer/` and root `CHANGELOG.md`. Do NOT touch any other module, watcher, or k8s YAML.
- **`VerdictComment` must be completely removed.** After the change, `grep -rn "VerdictComment" agent/pr-reviewer/` must return zero matches. The compiler enforces this; build failure = incomplete work.
- **The word `"comment"` must not appear as a verdict value in any file the agent reads at runtime.** After editing, verify: `grep -rn '"comment"' agent/pr-reviewer/pkg/prompts/` must return zero matches.
- **Verdict JSON wire format is unchanged.** The agent still emits `{"verdict": "...", "reason": "..."}`. Only the set of accepted verdict values changes.
- **`parseJSONVerdict` and `tryParseJSONLine` structure is preserved.** Only the switch cases inside `tryParseJSONLine` change — remove the `"comment"` case, keep the rest exactly as-is.
- **`checkMustFixContent` is not modified.** It is reused verbatim for Should Fix detection. Its content-inspection logic (skip None indicators, skip horizontal rules, skip empty lines, stop at next heading) is correct as-is.
- **`hasExpectedReviewSections` is not modified.** It is still called in the `ParseVerdict` fallback path.
- **`StripJSONVerdict` is not modified.** It strips any JSON block with a non-empty `verdict` field regardless of value — this is correct behavior for the binary world too.
- **Existing tests that assert correct binary behavior must not regress.** The full list of tests that must remain unchanged is in requirements step 2b.
- **New tests must use Ginkgo `Context`/`It` blocks.** No raw `testing.T` tests. External test package (`package pkg_test`).
- Do NOT commit — dark-factory handles git.
- `make precommit` runs from `agent/pr-reviewer/`, never at repo root.
- Coverage ≥80% for changed packages.
- No scenario. Per `docs/scenario-writing.md`, this spec is satisfied by unit tests on the parser and heuristic — pure functions with no integration seam.
</constraints>

<verification>
cd agent/pr-reviewer && make precommit

# Confirm VerdictComment is gone (must return zero matches):
grep -rn "VerdictComment" agent/pr-reviewer/
# Expected: zero matches

# Confirm "comment" does not appear as verdict value in any runtime prompt:
grep -rn '"comment"' agent/pr-reviewer/pkg/prompts/
# Expected: zero matches

# Confirm "comment" does not appear as verdict value anywhere it would be parsed:
grep -rn '"verdict"\s*:\s*"comment"\|"verdict":"comment"' agent/pr-reviewer/ README.md
# Expected: zero matches (covers READMEs + dummy-task fixture + any leftover prompts)

# Confirm cli/main.go has no VerdictComment-fallback comments:
grep -n "VerdictComment\|Fallback to plain comment\|VerdictComment - no verdict action" agent/pr-reviewer/cmd/cli/main.go
# Expected: zero matches

# Confirm binary enum:
grep -A 5 "const (" agent/pr-reviewer/pkg/verdict.go | grep "Verdict"
# Expected: VerdictApprove and VerdictRequestChanges only; VerdictComment absent

# Confirm "comment" case removed from tryParseJSONLine switch:
grep -A 15 "func tryParseJSONLine" agent/pr-reviewer/pkg/verdict.go
# Expected: switch has only "approve" and "request-changes" cases

# Confirm Should Fix detection added to ParseVerdict:
grep -n "shouldFixPattern\|shouldFixIndex\|should-fix items found" agent/pr-reviewer/pkg/verdict.go
# Expected: shouldFixPattern, shouldFixIndex variable, and reason string present

# Confirm fail-closed for empty text:
grep -A 3 'reviewText == ""' agent/pr-reviewer/pkg/verdict.go
# Expected: VerdictRequestChanges returned for empty text

# Confirm fail-closed for unparseable:
grep -n '"unparseable review format"' agent/pr-reviewer/pkg/verdict.go
# Expected: paired with VerdictRequestChanges (not VerdictComment)

# Confirm execution output-format updated:
grep -n "verdict" agent/pr-reviewer/pkg/prompts/execution_output-format.md
# Expected: "approve | request_changes" — no "comment"

# Confirm execution.go verdict roll-up updated:
grep -n "Should Fix\|comment\|roll-up" agent/pr-reviewer/pkg/prompts/execution.go
# Expected: "Should Fix present → verdict request_changes"; no "comment" verdict in roll-up

# Confirm review_workflow.md comment line removed:
grep -n "comment" agent/pr-reviewer/pkg/prompts/review_workflow.md
# Expected: zero matches (the word "comment" no longer appears as a verdict option)

# Confirm architecture.md updated:
grep -n "comment" agent/pr-reviewer/docs/architecture.md
# Expected: "comment" appears only as a noun in prose (e.g. "PR review comment"), never as a verdict value

# Confirm no VerdictComment in tests:
grep -n "VerdictComment" agent/pr-reviewer/pkg/verdict_test.go
# Expected: zero matches

# Confirm new test cases present:
grep -n "should-fix items found\|JSON.*comment.*rejected\|fail-closed" agent/pr-reviewer/pkg/verdict_test.go
# Expected: at least the three new test contexts present

# Confirm CHANGELOG entry:
grep -n "binary\|collapse.*verdict\|approve.*request-changes" CHANGELOG.md
# Expected: one match under ## Unreleased
</verification>
