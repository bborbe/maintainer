---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-15T13:05:25Z"
generating: "2026-05-15T13:05:26Z"
prompted: "2026-05-15T13:11:56Z"
verifying: "2026-05-15T13:40:28Z"
branch: dark-factory/pr-reviewer-binary-verdict
---

## Summary

- The pr-reviewer agent currently emits three verdicts (`approve`, `request-changes`, `comment`). `comment` is the in-between state: it neither blocks merge nor approves, leaving every PR in limbo until a human decides what to do with the findings.
- This spec collapses the rubric to a binary verdict: every review ends with exactly one of `approve` or `request-changes`. `comment` is removed end-to-end — from the rubric prompts the agent reads, the JSON parser that extracts the verdict, the heuristic fallback when no JSON is found, and the downstream code paths that act on the verdict.
- Findings categories are unchanged. Must Fix and Should Fix both map to `request-changes`; Nice to Have only (or nothing flagged) maps to `approve`. When the agent produces no parseable verdict at all, the system defaults to `request-changes` — fail-closed, never silently approve.
- Confined to one module (`agent/pr-reviewer/`). No schema, infra, or downstream changes — the poster just stops needing a `comment` branch.
- After this lands, the agent becomes a first-class GitHub reviewer whose verdict is always actionable: every review is either a green check or a red X.

## Problem

GitHub PRs in `bborbe/*` repos are blocked until they receive an `APPROVE` review. The current rubric maps Should-Fix findings to `comment`, leaving PRs in limbo — neither approved nor explicitly rejected. Each `comment` verdict requires a human to decide whether to address findings and re-trigger the review, or merge anyway. This is the opposite of the no-human-in-the-loop goal driving the [[GitHub Code Reviewer Agent]]: the agent produces output that requires the very human judgement it is meant to replace.

## Goal

After this work, every pr-reviewer review terminates in `approve` or `request-changes`. Unparseable agent output defaults to `request-changes` (fail-closed). No code path can produce `comment`.

## Assumptions

- Only the pr-reviewer agent uses this verdict rubric. Other agents (build-fixer, sentry-triager, etc.) have their own contracts and are out of scope.
- No external system (Kafka consumer, dashboard, notification pipeline) parses `"comment"` as a meaningful verdict value. The wire format `{"verdict": "...", "reason": "..."}` stays; only the value set shrinks.
- `pkg/prompts/execution.go` (the Go-embedded rubric the LLM reads at runtime) is the canonical source-of-truth for the verdict mapping. Other docs link to it.
- Historical `comment` reviews already posted on open PRs do not need migration. They remain as artifacts; the next agent run on a new SHA produces a binary verdict.

## Non-goals

- Changing the finding categories (Must Fix / Should Fix / Nice to Have stay exactly as they are).
- Modifying the watcher or the posting pipeline beyond accepting only the binary verdict set. The poster simply loses its `comment` branch.
- Retroactively re-reviewing or rewriting previously-posted `comment` reviews on currently-open PRs.
- Adding new severity levels, sub-verdicts, or fine-grained rejection reasons.
- Changes to other agents (build-fixer, sentry-triager, etc.) — they have their own rubrics and their own do-nothing arguments.
- Changing the JSON output format the agent writes (still `{"verdict": "...", "reason": "..."}`); only the set of acceptable verdict values shrinks.
- Renaming the per-line review-comment messages or their `nit / minor / major / critical` severity scale. The word "comment" remains valid as a noun for these messages — only the verdict value `comment` is removed.

## Desired Behavior

1. The verdict produced by every successful pr-reviewer run is exactly one of `approve` or `request-changes`. No code path produces any other value.
2. When the agent's structured output declares `verdict: "comment"`, the parser rejects it as if it were any other unknown verdict value. The system falls through to the heuristic, which returns a binary verdict.
3. Findings rubric, applied identically by the prompts and by the heuristic fallback:

   | Finding category present | Verdict |
   |---|---|
   | Must Fix (with content) | `request-changes` |
   | Should Fix (with content) | `request-changes` |
   | Nice to Have only | `approve` |
   | Nothing flagged | `approve` |

4. When the agent's output is empty, malformed, or contains no recognizable findings sections, the verdict defaults to `request-changes`. The system fails closed: an unreadable review never silently approves a PR.
5. **Only the execution phase emits the merge verdict.** Planning emits no verdict; review (ai_review) emits a separate `pass / fail` meta-verdict judging whether execution did a good job. `pkg/prompts/execution.go` (Go-embedded rubric) and `pkg/prompts/execution_output-format.md` (JSON schema) are the single canonical source of the rubric. Other prompt files (`review_workflow.md`) and guardrails (`agent/.claude/CLAUDE.md`) reference the rubric without duplicating it.
6. **The token `comment` no longer appears as a verdict value in any prompt the agent reads at runtime.** This includes the execution prompt, the execution output-format schema, the review workflow's consistency-check rules (which currently special-case the `comment` value), and the agent's in-directory guardrails. The token "comment" remains valid only as a noun for per-line PR review comment messages (separate `nit / minor / major / critical` severity scale, untouched by this spec).
7. **Documentation links, never duplicates the rubric.** The existing `agent/pr-reviewer/docs/architecture.md` is updated where it currently describes the three-verdict rubric: any reference to `comment` as a verdict value is removed; the heuristic fallback section is updated to describe the new fail-closed `request-changes` default; the verdict-emission table reflects the binary set. The doc continues to point to `pkg/prompts/execution.go` as canonical and does not introduce a duplicated mapping table. The pr-reviewer README's link to `docs/architecture.md` and to `pkg/prompts/execution.go` is preserved as-is.
8. `make precommit` in the pr-reviewer module passes. Existing tests that asserted `comment` behavior are rewritten to assert the new binary mapping. New tests cover the three previously-`comment` paths: structured `"comment"` rejected, Should-Fix-only triggers `request-changes`, empty/unparseable triggers `request-changes`.

## Constraints

- **Verdict JSON wire format is unchanged.** The agent still emits `{"verdict": "...", "reason": "..."}`. Only the accepted value set shrinks from three to two. Downstream consumers parsing the JSON directly continue to work; they just never see `"comment"`.
- **Findings categories are frozen.** Must Fix, Should Fix, Nice to Have remain as-is. Only the verdict mapping changes.
- **Existing JSON-parser behavior on `approve` and `request-changes` must not regress.** The parser's structure (first try JSON, fall through to heuristic) is preserved; only the value-validation switch changes.
- **Existing heuristic behavior on Must Fix detection must not regress.** The Must Fix scanner, the "None"-content detection, and the horizontal-rule handling all stay. The change adds a Should Fix detector that runs alongside Must Fix; both contribute to a `request-changes` verdict.
- **Confined to one module.** All changes live under `agent/pr-reviewer/`. No shared libraries, no other agents, no dark-factory infrastructure.
- **Risk surface justifies Rung-1 verification.** The only failure mode is a wrong verdict on a PR. The fail-closed default makes any unparseable agent output safer (`request-changes`), not riskier. The verdict parser and heuristic are pure functions covered exhaustively by unit tests. There is no Kafka schema change, no PVC, no secret, no infrastructure deploy concern. Rung-2 (dev k8s deploy + e2e) is not warranted; `make precommit` + the new unit tests are sufficient evidence of correctness per `docs/verifying-specs.md`.
- **Existing knowledge to reference**:
  - `docs/architecture.md` — agent contract; no schema field changes.
  - `docs/verifying-specs.md` — Rung-1 verification (unit tests + `cmd/run-once`) is the appropriate verification level for this risk surface.
  - `~/Documents/workspaces/coding/docs/go-enum-type-pattern.md` — guidance on safely removing an enum value (audit all references, remove constant, update parser, update tests, update docs).
  - `~/Documents/workspaces/coding/docs/go-parse-pattern.md` — validate-before-accept: the parser's value-validation switch is the choke point; reject anything outside the binary set there.
  - `~/Documents/workspaces/coding/docs/go-testing-guide.md` — Ginkgo `DescribeTable` pattern for the verdict-mapping table.
  - `~/Documents/workspaces/coding/docs/readme-guide.md` — README structure; this spec deliberately keeps the rubric out of the README and links to the prompt files instead (single source of truth).

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Agent emits `verdict: "comment"` in structured output | Parser rejects the value (treats as unknown), falls through to heuristic, heuristic returns a binary verdict based on findings sections | Automatic — heuristic handles it |
| Agent emits an unknown verdict value (typo, hallucinated word) | Same as above: parser rejects, heuristic decides | Automatic |
| Agent's review text is empty | Heuristic returns `request-changes` with reason "empty review text" (was: `comment`) | Automatic — fail closed |
| Agent's review text has no recognizable sections (no Must Fix, no Should Fix, no Nice to Have) | Heuristic returns `request-changes` with reason "unparseable review format" (was: `comment`) | Automatic — fail closed |
| Agent's review has Must Fix with content | `request-changes` (unchanged behavior) | N/A |
| Agent's review has Should Fix with content but no Must Fix | `request-changes` (was: `comment`) | N/A |
| Agent's review has only Nice to Have, or Must/Should Fix sections all empty/None | `approve` (unchanged for empty Must Fix; new for Should-Fix-empty path) | N/A |
| Operator inspects an already-posted historical `comment` review on an open PR | Out of scope; the review remains as-is until the next agent run, which will produce a binary verdict | Re-trigger the review on the PR if a binary verdict is desired |

## Security / Abuse Cases

Not applicable in the strict sense — this spec touches no HTTP, no files, no user input. The only security-adjacent property is the fail-closed default: an empty or unparseable agent output now produces `request-changes` instead of `comment`. This is strictly safer: it cannot cause a PR to be silently approved by a flaky agent run.

## Acceptance Criteria

- [ ] The verdict enum exposed by the pr-reviewer module contains exactly two values: `approve` and `request-changes`. The `comment` constant is removed. All references in production code are gone (deletion is enforced by the compiler).
- [ ] The JSON-line parser rejects `"comment"` as a verdict value (returns "not parsed" so the caller falls through to the heuristic). It accepts `"approve"` and `"request-changes"` as before.
- [ ] The heuristic fallback returns `request-changes` for empty review text (was: `comment`).
- [ ] The heuristic fallback returns `request-changes` for review text with no recognizable sections (was: `comment`).
- [ ] The heuristic fallback detects a non-empty Should Fix section (using the same content-inspection logic that already exists for Must Fix: skip "None", skip horizontal rules, skip empty lines) and returns `request-changes` when one is present, even if Must Fix is empty/absent.
- [ ] The heuristic continues to return `approve` when only Nice to Have content is present, or when nothing is flagged.
- [ ] The agent's prompt files under `pkg/prompts/` (planning workflow, review workflow, execution output-format) describe exactly two verdicts and the four-row mapping table from Desired Behavior #3. The literal word "comment" no longer appears as a verdict option in any prompt the agent reads at runtime. **These prompts are the single source of truth for the rubric.**
- [ ] The agent's in-directory guardrails (the CLAUDE.md inside the agent's `.claude/` directory) reference the rubric without duplicating the table — readers are pointed to the prompt files.
- [ ] The existing `agent/pr-reviewer/docs/architecture.md` is updated: every reference to `comment` as a verdict value is removed; the heuristic-fallback section describes the new fail-closed `request-changes` default (currently describes the bug it documents); the verdict-emission table reflects the binary set. The doc still points to `pkg/prompts/execution.go` as canonical and does not duplicate the mapping table.
- [ ] The pr-reviewer README's existing link to `docs/architecture.md` is preserved. README still does not contain the rubric table. If the repo-root README mentions the rubric, it is updated to link rather than duplicate.
- [ ] CHANGELOG has an `## Unreleased` entry describing the verdict collapse.
- [ ] Existing tests that asserted `VerdictComment` behavior are rewritten to assert the new binary mapping. No test references the removed constant. New tests cover: (a) JSON `"comment"` is rejected by the parser, (b) Should-Fix-only heuristic input returns `request-changes`, (c) empty input returns `request-changes`, (d) unparseable input returns `request-changes`.
- [ ] `make precommit` in the pr-reviewer module passes (format, generate, test, lint, license).
- [ ] **No new scenario.** Per `docs/scenario-writing.md`, this spec is satisfied by unit tests on the parser and heuristic. The behavior is fully reachable from in-process tests; there is no integration seam that unit tests cannot exercise.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Expected: all targets pass. The test output should show new test cases covering the three previously-`comment` paths (structured `"comment"` rejected, Should-Fix-only → `request-changes`, empty/unparseable → `request-changes`) and the rewritten Must Fix tests confirming `approve`/`request-changes` are unchanged.

Manual smoke check (post-deploy, NOT part of this spec's automated verification): re-trigger a review on an open PR known to have produced a `comment` verdict historically; confirm the new verdict is binary (`approve` or `request-changes`).

## Do-Nothing Option

Keep the three-verdict rubric. Every Should-Fix review continues to land as `comment`, leaving the PR in limbo. Either:

- A human reads the review, decides whether to re-trigger after fixes, or merges anyway — the exact human-in-the-loop step the parent goal exists to eliminate.
- Or the PR sits indefinitely, defeating the purpose of running the agent at all.

A weaker alternative would be to keep the `comment` verdict in the code but stop emitting it (always demote `comment` → `request-changes` at the boundary). This saves a few lines of deletion but leaves a dead code path that drifts from the prompts: future maintainers see three verdict constants and assume all three are reachable, then accidentally re-introduce `comment` paths. The strict deletion enforces the contract through the compiler. Not recommended.

## Verification Result

**Verified:** 2026-05-15T14:41:49Z (HEAD dc1356c, change in v0.23.38 commit 36df18f)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory (v0.156.1-1-g04f3863-dirty)
**Scenario:** Rung-1 verification per spec — code-level grep evidence + `make test` + `make precommit` in `agent/pr-reviewer/`. Both maintainer-dev and maintainer-prod clusters were redeployed with v0.23.38 by the operator before verification.
**Evidence:**
- `grep VerdictComment agent/pr-reviewer/{pkg/verdict.go,pkg/verdict_test.go,cmd/cli/main.go,pkg/github/client.go,pkg/github/client_test.go}` → zero matches; enum at `pkg/verdict.go:16-19` contains only `VerdictApprove` and `VerdictRequestChanges`.
- `tryParseJSONLine` switch (`pkg/verdict.go:50-58`) accepts only `approve` / `request-changes`; `default` returns `(Result{}, false)` so `"comment"` falls through to heuristic.
- `ParseVerdict` (`pkg/verdict.go:99-166`) implements: empty → `request-changes` "empty review text"; Should Fix detection via `shouldFixPattern` reusing `checkMustFixContent`; unparseable → `request-changes` "unparseable review format".
- Prompts: `pkg/prompts/execution.go:44-47` roll-up "binary — exactly one of two values"; `execution_output-format.md:4` schema `"verdict": "approve | request_changes"`; `review_workflow.md` no longer special-cases `comment` verdict (only "comment" noun usage remains).
- `docs/architecture.md:21` phase table emits `(approve / request_changes)`; heuristic-fallback section (lines 40-49) documents fail-closed default; canonical pointer to `pkg/prompts/execution.go` retained at line 34.
- READMEs: `agent/pr-reviewer/README.md:3,93` and root `README.md:26,133` both show `approve / request-changes` only.
- `CHANGELOG.md` v0.23.38 (released from Unreleased section) entry: "feat(agent/pr-reviewer): collapse verdict from three values to two..."
- `pkg/verdict_test.go` has rewritten + new tests: empty → `request-changes`; unparseable → `request-changes`; JSON `"comment"` rejected → heuristic; Should-Fix-only → `request-changes` "should-fix items found".
- `cd agent/pr-reviewer && make test` → all packages PASS, `pkg` coverage 89.1%.
- `cd agent/pr-reviewer && make precommit` → exits 0 (gosec 0 issues, trivy 0 vulns, addlicense ready).
**Verdict:** PASS
