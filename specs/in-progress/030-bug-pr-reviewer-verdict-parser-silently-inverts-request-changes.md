---
status: verifying
approved: "2026-05-16T10:20:22Z"
generating: "2026-05-16T10:20:23Z"
prompted: "2026-05-16T10:29:46Z"
verifying: "2026-05-16T11:33:31Z"
branch: dark-factory/bug-pr-reviewer-verdict-parser-silently-inverts-request-changes
---

## Summary

- A reviewer-agent verdict of "request changes" was silently posted as APPROVE on a real GitHub PR because the JSON parser and the model output disagreed on the spelling (`request-changes` vs `request_changes`).
- When the JSON parser fails, control falls through to a markdown-heuristic fallback that defaults to APPROVE for any review missing a `## Must Fix` heading — even when the agent said `request_changes` and listed critical findings under a `**Must Fix**` bold line.
- The fix canonicalises on the hyphen spelling in the model-facing output spec, makes the parser tolerant to spelling drift (case + underscore-vs-hyphen), and deletes the markdown-heuristic fallback so empty/unknown verdicts fail closed to "request-changes".
- After the fix, the only path to APPROVE is an explicit, well-formed `"verdict": "approve"` in the JSON block; everything else — including malformed JSON, missing JSON, hallucinated values, or a `request_changes` underscore drift — maps to "request-changes".
- The triggering incident (PR #5 d04d349 on `bborbe/maintainer`) is the canonical reproduction; a re-run after fix must post as `REQUEST_CHANGES` on GitHub.

## Problem

The PR reviewer agent posted an APPROVE review on `bborbe/maintainer` PR #5 d04d349 on 2026-05-16 even though the agent's structured verdict block said `"verdict": "request_changes"` and the body contained one critical and three major findings. The mis-post happened because the JSON parser only accepts `approve` and `request-changes` (hyphen) but the output-format prompt instructs the model to emit `request_changes` (underscore). The parser silently returned "not found", control fell through to a markdown heuristic that looks for `## Must Fix` headings (the agent uses `**Must Fix**` bold instead), no must-fix heading was found, and the heuristic returned APPROVE. Because `autoApprove: true` was configured, an APPROVE was posted to GitHub against the maintainer's intent. This is a silent inversion of the agent's correctness signal and undermines the trust contract of the entire reviewer pipeline.

## Goal

After this fix is deployed, the PR reviewer's posted GitHub verdict matches the agent's structured `"verdict"` value exactly. There is exactly one canonical spelling of `request-changes` in the model-facing prompt and the parser. Any deviation — case drift, underscore drift, unknown values, missing JSON, malformed JSON — resolves to "request-changes" (fail-closed), never to "approve". The markdown-heading fallback heuristic no longer exists.

## Non-goals

(Bug spec — fix scope is bounded by the reproduction. Listed for clarity.)

- Not fixing the dismissal-filter inversion at `poster.go:189-198` (separate bug).
- Not fixing the controller spawn-on-terminal-phase issue (separate bug).
- Not changing the public `Verdict` type or its two constants.
- Not introducing a third verdict value (e.g., `comment`, `block`); deprecated/unknown spellings all map to `request-changes`.
- Not changing the `autoApprove` config semantics.

## Reproduction

**Triggering incident (verbatim evidence on file):**

- PR: `bborbe/maintainer` #5, head SHA `d04d349`
- Posted review: GitHub review_id `4303450851` with state `APPROVED` (since dismissed by re-spawn)
- Agent verdict block in review body: `"verdict": "request_changes"` (underscore)
- Review body contains a `**Must Fix**` (bold, not heading) section listing one critical and three major findings
- Configured: `.pr-reviewer.yaml` has `autoApprove: true`
- Vault task page: `~/Documents/Obsidian/OpenClaw/tasks/PR Review github - bborbe-maintainer - 5 - d04d349a - confirm-new-env-vars-are-documented-in-help.md`

**Minimal in-process reproduction:**

1. Feed the verdict parser a review string whose last JSON block is `{"verdict": "request_changes", "summary": "...", "comments": [...]}` and whose body uses `**Must Fix**` bold (not heading).
2. Observe today: parser returns `Result{Verdict: "approve", Reason: "no must-fix section"}`.
3. Expected: parser returns `Result{Verdict: "request-changes", ...}`.

**Live reproduction (post-deploy gate):**

1. Reset the vault task for PR #5 d04d349 so the watcher re-spawns it.
2. Let the reviewer agent run end-to-end against the same PR.
3. Observe today: an APPROVE review is posted on GitHub. After fix: a REQUEST_CHANGES review is posted on GitHub.

## Expected vs Actual

**Expected** (per `specs/completed/025-pr-reviewer-binary-verdict.md` and `specs/completed/027-post-verdict-to-github-pr.md`): the GitHub review event posted by the agent reflects the agent's structured `verdict` field. Verdict is binary; "approve" requires an explicit, well-formed approve token; everything else fails closed to "request-changes".

**Actual** (observed 2026-05-16 on PR #5 d04d349): agent emitted `"verdict": "request_changes"` in a fenced JSON block, the parser failed to match this spelling, fell back to a heading-based heuristic, found no `## Must Fix` heading, returned `approve`, and the agent posted GitHub event `APPROVE`.

## Why this is a bug

Three independent invariants are broken:

1. **Source-of-truth disagreement.** The output-format prompt at `agent/pr-reviewer/pkg/prompts/execution_output-format.md` instructs the model to emit `request_changes` (underscore). The Go constant `VerdictRequestChanges` at `verdict.go:17` is `request-changes` (hyphen). The model dutifully follows the prompt; the parser dutifully rejects what the prompt told the model to emit.
2. **Silent fallback masks the disagreement.** A correct design either raises on unknown JSON values or fails closed. Today's parser falls through to a markdown heuristic with different semantics, producing a verdict the JSON did not authorize.
3. **The heuristic itself is unsafe by default.** It treats absence-of-heading as approve. Any review format the model varies — bold instead of heading, different heading level, omitted section — flips to approve.

The combination of (1)+(2)+(3) is the silent-inversion failure mode, which is the worst class of bug for a system whose output is auto-posted to GitHub.

## Desired Behavior

1. The model-facing output-format prompt instructs the model to emit exactly one canonical verdict spelling: `request-changes` (hyphen).
2. The parser accepts the canonical spelling and also normalises common drift: case (`Approve`, `APPROVE`, `Request-Changes`) and separator (`request_changes`) all parse correctly.
3. Any verdict value that is not, after normalisation, one of `approve` / `request-changes` resolves to `request-changes` with a reason naming the unknown value.
4. Missing JSON verdict block resolves to `request-changes` with reason "no verdict block".
5. Malformed JSON (containing the word `verdict` but unparseable) resolves to `request-changes` with reason naming the parse failure.
6. Empty review text resolves to `request-changes` (preserves today's behavior).
7. There is no markdown-heading fallback. The presence or absence of `## Must Fix`, `**Must Fix**`, `## Should Fix`, etc., never affects the verdict; only the JSON block does.
8. The full reviewer pipeline, when re-run against the triggering input (agent emits `"verdict": "request_changes"` with bold `**Must Fix**` content), posts a `REQUEST_CHANGES` event to GitHub.

## Constraints

- The public `Verdict` type and its constants `VerdictApprove = "approve"` and `VerdictRequestChanges = "request-changes"` MUST NOT change. Downstream callers (`poster.go`, integration tests) depend on these exact values.
- `StripJSONVerdict` behavior MUST NOT change; the JSON block continues to be stripped from the posted PR comment.
- Existing passing tests in `verdict_test.go` (Ginkgo external-package test file) MUST continue to pass except where their assertions encode the now-removed heuristic; such cases must be re-pointed to the new fail-closed behavior.
- The model-facing prompt at `agent/pr-reviewer/pkg/prompts/execution_output-format.md` is the single source of truth for the model; all other docs that reference verdict spelling must agree.
- Domain rules referenced: `specs/completed/025-pr-reviewer-binary-verdict.md` (binary verdict invariant), `specs/completed/027-post-verdict-to-github-pr.md` (post-to-GitHub mapping).
- Verification ladder per `docs/verifying-specs.md`: this fix is pure code correctness, so Rung-1 (local `make precommit` + table test green) is the primary gate; Rung-2 (dev deploy + replay PR #5 d04d349) is the live-evidence gate; Rung-3 (prod) only after Rung-2 passes.

## Failure Modes

| Trigger | Expected behavior | Detection | Recovery |
|---|---|---|---|
| Model emits `"verdict": "request_changes"` (underscore drift) | Parser normalises to `request-changes`; verdict is RequestChanges | Unit table test; runtime log line `verdict=request-changes reason=...` | None needed — handled inline |
| Model emits unknown verdict (e.g., `"block"`, `"comment"`) | Verdict is RequestChanges with reason naming the unknown value | Unit table test; runtime log line shows the unknown token | Operator inspects log, optionally files spec to add canonical handling |
| Model emits malformed JSON containing `"verdict"` | Verdict is RequestChanges with reason naming the parse failure | Unit table test; runtime log line includes JSON error | Operator dismisses the posted REQUEST_CHANGES via GitHub UI if false positive |
| Model emits no JSON verdict block at all | Verdict is RequestChanges with reason "no verdict block" | Unit table test; runtime log line | Operator inspects review text; agent re-run if needed |
| Model emits `"verdict": "approve"` correctly | Verdict is Approve | Unit table test; existing happy-path coverage | n/a |
| Two concurrent reviews on same PR (mid-action crash) | No partial state in parser (pure function); the in-flight review is replayed by the watcher on next tick | Watcher logs show duplicate tasks; vault task `phase` shows replay | Existing watcher idempotency carries; no parser-side recovery needed |
| Model output-format spec drifts again in future | Normalisation absorbs `_` ↔ `-` and case; truly novel spellings fail closed | Unit table test fails on any new spelling not covered | Add a Reason-named entry in the parser switch + table-test row |

## Acceptance Criteria

- [ ] `agent/pr-reviewer/pkg/prompts/execution_output-format.md` contains `request-changes` (hyphen) and does not contain `request_changes` (underscore) — evidence: `grep -n 'request_changes' agent/pr-reviewer/pkg/prompts/execution_output-format.md` returns 0 lines; `grep -n 'request-changes' agent/pr-reviewer/pkg/prompts/execution_output-format.md` returns ≥1 line.
- [ ] The markdown-heading fallback heuristic is deleted from `agent/pr-reviewer/pkg/verdict.go` — evidence: `grep -nE 'mustFixPattern|shouldFixPattern|checkMustFixContent|hasExpectedReviewSections' agent/pr-reviewer/pkg/verdict.go` returns 0 lines.
- [ ] `ParseVerdict` body is JSON-only plus fail-closed defaults — evidence: reading `agent/pr-reviewer/pkg/verdict.go`, the function `ParseVerdict` contains no `regexp` reference and no calls to deleted helpers; the only paths to a `Result` are the JSON parse branch and the fail-closed `request-changes` branch.
- [ ] A Ginkgo `DescribeTable` regression test exists in the external test package — evidence: `grep -n 'DescribeTable' agent/pr-reviewer/pkg/verdict_test.go` returns ≥1 line and the table contains rows asserting (a) `"verdict": "request-changes"` → RequestChanges, (b) `"verdict": "request_changes"` → RequestChanges, (c) `"verdict": "REQUEST-CHANGES"` → RequestChanges, (d) `"verdict": "approve"` → Approve, (e) `"verdict": "Approve"` → Approve, (f) `"verdict": "comment"` → RequestChanges, (g) empty review text → RequestChanges, (h) malformed JSON containing `"verdict"` → RequestChanges, (i) a multi-line fenced ```json block with the verdict `"request-changes"` buried among narrative paragraphs (≥3 lines of prose before the fence, ≥1 line after) → RequestChanges.
- [ ] Without the parser fix, the new table test fails at least one row — evidence: on a branch with the fix reverted, `go test ./agent/pr-reviewer/pkg/...` exits non-zero and the failure cites a row matching `request_changes` or `REQUEST-CHANGES`.
- [ ] `make precommit` in `agent/pr-reviewer/` exits 0 — evidence: exit code.
- [ ] Existing assertions in `verdict_test.go` and `verdict_internal_test.go` that previously encoded the heuristic are removed or re-pointed to the new fail-closed behavior; no test asserts the old `Reason: "no must-fix section"` string — evidence: `grep -n 'no must-fix section' agent/pr-reviewer/pkg/` returns 0 lines.
- [ ] Live replay (Rung-2): after dev deploy, resetting the vault task for `bborbe/maintainer` PR #5 d04d349 causes the agent to re-post — evidence: `gh pr view 5 --repo bborbe/maintainer --json reviews` shows a new review from the reviewer-agent identity with `state: CHANGES_REQUESTED` and a `submittedAt` later than `2026-05-16T00:00:00Z`.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Expected: exit 0. The new Ginkgo table test reports ≥9 entries passing.

Then, after dev deploy (Rung-2):

```
# Reset the vault task for PR #5 d04d349 so the controller re-spawns the agent.
# Operator edits the frontmatter of the existing task file at:
#   ~/Documents/Obsidian/OpenClaw/tasks/PR Review github - bborbe-maintainer - 5 - d04d349a - confirm-new-env-vars-are-documented-in-help.md
# Set: phase=in_progress, status=in_progress, drop the current_job + job_started_at
# fields, and bump trigger_count. obsidian-git auto-commits the change; the
# prod task-controller picks it up on the next reconcile (≤30s).
#
# Then wait ~5 min for the pr-reviewer pod to complete (visible via:
#   kubectlquant -n prod get pods -l app=pr-reviewer-agent --sort-by=.metadata.creationTimestamp
# ) and query the PR for the posted review:
gh pr view 5 --repo bborbe/maintainer --json reviews \
  | jq '.reviews[] | select(.author.login == "pr-review-of-ben") | {state, submittedAt, commit_id}'
```

Expected: at least one review by `pr-review-of-ben` with `state: "CHANGES_REQUESTED"` and `submittedAt` after the reset timestamp.

## Do-Nothing Option

Not viable. The current code silently posts APPROVE reviews when the agent says request_changes. Every reviewer run is exposed to this inversion. The triggering incident already caused one wrong-direction review on a real PR; until fixed, the reviewer pipeline cannot be trusted to enforce changes-requested verdicts, which defeats the purpose of specs 025 and 027. The configured workaround (`autoApprove: false`) would require human review of every PR and negate the agent's value.

## Verification Result

**Verified:** 2026-05-18T09:31:57Z (HEAD bf18f55)
**Binary:** installed `dark-factory` (not a dark-factory spec; spec target is maintainer/agent/pr-reviewer v0.25.2)
**Scenario:** Rung-1 grep/test ACs verified on tree at bf18f55; Rung-2 live replay evidence from prod task execution at 2026-05-16T20:24:22Z (post v0.25.2 release at 2026-05-16T11:33Z).
**Evidence:**
- `grep -n 'request_changes' agent/pr-reviewer/pkg/prompts/execution_output-format.md` → 0 lines; `request-changes` present at line 5.
- `grep -nE 'mustFixPattern|shouldFixPattern|checkMustFixContent|hasExpectedReviewSections' agent/pr-reviewer/pkg/verdict.go` → 0 lines; `verdict_internal_test.go` deleted.
- `ParseVerdict` body (verdict.go:129-164) is JSON-only + fail-closed switch; `grep -c regexp verdict.go` = 0.
- Ginkgo DescribeTable `ParseVerdict normalisation regression (spec-030)` at verdict_test.go:742 with all 9 required rows (a-i).
- `grep -n 'no must-fix section' agent/pr-reviewer/pkg/` → 0 lines.
- `cd agent/pr-reviewer && make precommit` → exit 0; `go test ./pkg/` → 279 of 279 specs pass.
- v0.25.2 tagged 2026-05-16T11:33Z; deployment worktrees `maintainer-dev` and `maintainer-prod` both at bf18f55 (past v0.25.2).
- Live replay (Rung-2): vault task `~/Documents/Obsidian/OpenClaw/tasks/PR Review github - bborbe-maintainer - 5 - d04d349a - confirm-new-env-vars-are-documented-in-help.md` shows trigger_count=4, stage=prod, phase=done, final job_run=2026-05-16T20:24:22Z (9h post-release); body contains canonical `"verdict": "request-changes"` JSON block; diagnostics: `review_id: 4304188732 outcome: success`.
- Note: GitHub `/repos/bborbe/maintainer/pulls/5/reviews` now returns `[]` because PR #5 was merged 2026-05-16T10:26:22Z and reviews have since been dismissed (separately tracked in spec 031); the vault task body is the persistent post-fix execution record.
**Verdict:** PASS
