---
status: completed
spec: [071-pr-reviewer-verdict-decides-review-event]
summary: 'Replaced autoApprove-gated switch in mapVerdictAndSummary with a verdict-driven switch (approve→APPROVE, request-changes→REQUEST_CHANGES, no COMMENT fallback), dropped COMMENTED from verifier''s ExpectedStates and added a defense-in-depth COMMENTED filter in findReview, deleted the obsolete Entry from poster_test.go, and added a fix-prefix CHANGELOG entry under ## Unreleased.'
container: maintainer-pr-verdict-exec-235-spec-060-fix-verdict-mapping
dark-factory-version: v0.175.0
created: "2026-06-03T15:10:31Z"
queued: "2026-06-03T15:48:33Z"
started: "2026-06-03T15:53:03Z"
completed: "2026-06-03T16:03:17Z"
branch: dark-factory/pr-reviewer-verdict-decides-review-event
---

<summary>
- `mapVerdictAndSummary` in `agent/pr-reviewer/pkg/githubposter/poster.go` no longer has a `default` branch that assigns `event = "COMMENT"` or prepends the `"auto-approve disabled for this repo"` preamble — verdict `approve` always maps to event `APPROVE` regardless of the `autoApprove` config flag. Branch protection's "approving review" requirement is now satisfied on docs-only PRs with `autoApprove=false`.
- Verdict `request-changes` continues to map to `REQUEST_CHANGES`. No other verdict code path is touched.
- The existing `Entry("approve+autoApprove:false → COMMENT", ...)` is DELETED from the `DescribeTable`; the new Entry from the sibling TDD prompt (now landing on top) becomes the contract. The pre-existing `Entry("approve+autoApprove:true → APPROVE", ...)` and `Entry("request-changes → REQUEST_CHANGES", ...)` Entries are preserved verbatim.
- `agent/pr-reviewer/pkg/steps_review.go` drops `"COMMENTED"` from the `ExpectedStates` slice at the verifier call site (currently around line 241). The fresh-bot-review allow-list is now `[APPROVED, CHANGES_REQUESTED]` only — stale `COMMENTED` reviews from before this fix do NOT count as a match.
- `eventToState` is unchanged — its `COMMENT → COMMENTED` mapping is still used by the historical `eventToState` test in `poster_test.go` and is load-bearing for the `Export_test.go` shim. The spec explicitly forbids touching it.
- The dismissal-skip-on-`COMMENTED` filter at `poster.go:175` is unchanged — historical `COMMENTED` reviews on prior SHAs must still be skipped during dismissal.
- The 65,536-byte body truncation, empty-summary soft-warning, and `autoApprove` config field/YAML key are unchanged. `ReadAutoApprove` is unchanged.
- `make precommit` from `agent/pr-reviewer/` exits 0. The sibling TDD prompt's failing tests now pass.
</summary>

<objective>
Apply the production half of the TDD cycle for spec 060 (pr-reviewer verdict decides review event). After this prompt: verdict alone — not the `autoApprove` config flag — decides which GitHub review event the poster sends. The verifier's allow-list at the call site drops `COMMENTED` for fresh reviews. The change is invariant after this prompt: no opt-out flag, no fallback to `COMMENT` ever.
</objective>

<context>
Read `CLAUDE.md` at the repo root AND `agent/pr-reviewer/CLAUDE.md` (if it exists).

This is the GREEN step of a TDD cycle. The sibling prompt `1-spec-060-tdd-failing-test.md` lands first and adds two failing tests:
- A new Entry in `poster_test.go` asserting `verdict=VerdictApprove, autoApprove=false` → `event=APPROVE, state=APPROVED` (currently fails).
- A new Context in `verifier_test.go` asserting `ExpectedStates=["COMMENTED"]` → `Found:false, Class:Transient` (currently fails because the verifier today accepts the COMMENTED review).

Read these files fully BEFORE editing (anchors and patterns, not line numbers — lines go stale):

- `agent/pr-reviewer/pkg/githubposter/poster.go` — the `mapVerdictAndSummary` function (currently lines 617-652) and the `dismissPriorReviews` / `listBotReviews` filter at the `r.State != "COMMENTED"` check around line 175. Read both, do NOT touch the dismissal filter.
- `agent/pr-reviewer/pkg/githubposter/poster_test.go` — the `DescribeTable("verdict to event/state mapping", ...)` block (lines 142-164). The sibling TDD prompt adds a new Entry to this block; the existing `Entry("approve+autoApprove:false → COMMENT", ...)` (lines 159-161) is REMOVED in this prompt. All other Entries stay.
- `agent/pr-reviewer/pkg/githubposter/verifier.go` — read for context. The function `findReview` iterates over `req.ExpectedStates` and returns true on the first match. The function itself does NOT need to change — the allow-list drop is at the CALLER, in `steps_review.go`.
- `agent/pr-reviewer/pkg/githubposter/verifier_test.go` — the new Context added by the sibling prompt; the new "approve+autoApprove:false → APPROVE (post-fix contract — spec 060)" Entry on the `verdict to event/state mapping` `DescribeTable`. Both must pass after this prompt's edits.
- `agent/pr-reviewer/pkg/steps_review.go` — the call site `s.verifier.VerifyReview(ctx, VerifyRequest{... ExpectedStates: []string{"APPROVED", "CHANGES_REQUESTED", "COMMENTED"}})` (around line 238-242). This is the only place in the codebase that constructs the fresh-review allow-list.
- `agent/pr-reviewer/pkg/githubposter/eventToState` (lines 654-664) — read but DO NOT MODIFY. The spec explicitly forbids changing the `COMMENT → COMMENTED` mapping; it is still used by `EventToStateForTest` and the existing `eventToState` Describe block in `poster_test.go` (lines 584-594).
- `agent/pr-reviewer/pkg/githubposter/config.go` — `ReadAutoApprove` reads the `prReviewer.autoApprove` flag from `.maintainer.yaml`. The spec explicitly forbids removing or renaming the `autoApprove` field, the `AutoApprove` struct field, or its YAML key. The function call in `poster.go:Post` stays. The function itself stays.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` is the project's wrapping idiom. Not used in this prompt (no new error paths), but the absence of `fmt.Errorf` matters.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo `DescribeTable` / `Entry` shape. The new Entry from the sibling prompt is already in the file; this prompt just deletes the obsolete one.

Verified symbols (do NOT change — they are load-bearing for downstream):
- `prpkg.Verdict` (in `agent/pr-reviewer/pkg/verdict.go:13-18`) — the `VerdictApprove` and `VerdictRequestChanges` constants are the only two values passed to `mapVerdictAndSummary` in production. The new switch must keep both cases; the `default` branch is what disappears.
- `mapVerdictAndSummary` function signature: `func mapVerdictAndSummary(verdict prpkg.Verdict, autoApprove bool, summary string) (event, body string, warnings []string)` — the signature is FROZEN. Only the body changes.
- `autoApprove` parameter — the parameter is still present (the function still takes it), but the new switch ignores it for the verdict=approve case. The parameter is preserved because other call sites (current or future) may still pass it and because removing it would break the frozen signature constraint.
- `result.PostedEvent` on `prpkg.PostResult` — populated from the returned `event`. The test asserts `result.PostedEvent == "APPROVE"`.
- `pkg.VerifyRequest.ExpectedStates` (`agent/pr-reviewer/pkg/poster_types.go:103`) — `[]string`. The call site in `steps_review.go` changes from `["APPROVED", "CHANGES_REQUESTED", "COMMENTED"]` to `["APPROVED", "CHANGES_REQUESTED"]`.
- `errPhantomPOST` in `verifier.go` — the sentinel that fires when the allow-list excludes the only state the bot actually wrote. After this prompt's edits to `steps_review.go`, a stale `COMMENTED` review at the head SHA correctly returns the phantom-POST failure (transient class), so the retry loop continues until the fresh `APPROVED` / `CHANGES_REQUESTED` review appears.
- `mocks.HTTPClient` counterfeiter fake — used by both new tests. No signature change.
- `prpkg.Hallucination` and `prpkg.DismissCurrentReview` (added by spec 057) — unrelated to this change. The fix touches only the `mapVerdictAndSummary` function and one call site.
</context>

<requirements>

## Step 1 — Remove the `COMMENT` fallback from `mapVerdictAndSummary` in `poster.go`

In `agent/pr-reviewer/pkg/githubposter/poster.go`, the function `mapVerdictAndSummary` (currently lines 617-652) has a `switch` block at lines 625-633 with three cases:

```go
switch {
case verdict == prpkg.VerdictRequestChanges:
    event = "REQUEST_CHANGES"
case autoApprove:
    event = "APPROVE"
default:
    event = "COMMENT"
    body = "auto-approve disabled for this repo, review submitted as comment\n\n"
}
```

Replace this switch with a verdict-driven switch. The new switch MUST:
- Map `verdict == prpkg.VerdictRequestChanges` to `event = "REQUEST_CHANGES"` (unchanged).
- Map `verdict == prpkg.VerdictApprove` to `event = "APPROVE"` (NEW — previously this was guarded by `autoApprove`).
- Have NO `default` branch that produces `event = "COMMENT"` or prepends the "auto-approve disabled" preamble.

The exact replacement:

```go
switch verdict {
case prpkg.VerdictRequestChanges:
    event = "REQUEST_CHANGES"
case prpkg.VerdictApprove:
    event = "APPROVE"
}
```

Notes on the change:
- The `autoApprove` parameter is still in the function signature but no longer referenced in the switch. The Go compiler will reject unused parameters only for closures; named function parameters are allowed to be unused. Verify after the edit with `go build ./agent/pr-reviewer/...` — if the linter flags the unused parameter, the cleanest fix is to add a godoc note like `// autoApprove is reserved for future per-repo gating; the verdict alone decides the event today.` immediately above the function, but DO NOT remove the parameter from the signature. (The spec explicitly freezes the signature.)
- The `body` initialization (lines 632, `body = "auto-approve disabled for this repo, review submitted as comment\n\n"`) is removed entirely. The downstream `if summary == ""` block (currently lines 634-637) and the truncation block (lines 640-650) are unchanged. `body` starts as `""` and the `body += summary` line (line 638) writes the user-supplied summary verbatim. For a verdict-approve review, the body is the user's review text — no preamble.
- The function's godoc (lines 617-619) mentions "Empty summary is substituted with a default" and "Over-length bodies are truncated". The godoc does NOT mention `autoApprove`. Do NOT add a godoc line about the `autoApprove` parameter being unused — the function signature already implies the parameter exists. If the linter does NOT flag the unused parameter, do not add commentary; the code is self-explanatory.

## Step 2 — Delete the obsolete `Entry` from the `DescribeTable`

In `agent/pr-reviewer/pkg/githubposter/poster_test.go`, the `DescribeTable("verdict to event/state mapping", ...)` block (currently lines 142-164) contains the Entry `Entry("approve+autoApprove:false → COMMENT", pkg.VerdictApprove, false, "COMMENT", "COMMENTED", "auto-approve disabled for this repo")` at lines 159-161. DELETE this Entry entirely.

After the deletion, the `DescribeTable` has exactly two Entries:
- `Entry("approve+autoApprove:true → APPROVE", pkg.VerdictApprove, true, "APPROVE", "APPROVED", "")` — unchanged.
- `Entry("request-changes → REQUEST_CHANGES", pkg.VerdictRequestChanges, false, "REQUEST_CHANGES", "CHANGES_REQUESTED", "")` — unchanged.

PLUS the new Entry added by the sibling TDD prompt:
- `Entry("approve+autoApprove:false → APPROVE (post-fix contract — spec 060)", pkg.VerdictApprove, false, "APPROVE", "APPROVED", "")` — now passing.

The `DescribeTable` body (lines 142-156) is unchanged. The function signature `func(verdict pkg.Verdict, autoApprove bool, wantEvent, wantState, wantBodyPrefix string)` is unchanged.

## Step 3 — Drop `"COMMENTED"` from the verifier's `ExpectedStates` allow-list in `steps_review.go`

In `agent/pr-reviewer/pkg/steps_review.go`, find the call to `s.verifier.VerifyReview(ctx, VerifyRequest{...})` (around lines 238-242). The current value is `ExpectedStates: []string{"APPROVED", "CHANGES_REQUESTED", "COMMENTED"}`. Change it to `ExpectedStates: []string{"APPROVED", "CHANGES_REQUESTED"}`.

The literal value is constructed inline (no constant). Edit the line in-place. Do NOT introduce a package-level constant for the allow-list — the spec does not ask for it, and the inline literal is the established style.

The godoc/comment immediately above the function (if any) that mentions "COMMENTED" must be updated or removed. If a comment says something like "verifier allows COMMENTED for backward compat with the legacy poster path", delete that comment — the spec explicitly forbids the `COMMENT` fallback and the spec's non-goals are LOAD-BEARING for the invariant.

## Step 4 — Confirm `eventToState` is untouched

The function `eventToState` in `poster.go` (lines 654-664) has the mapping:

```go
case "APPROVE": return "APPROVED"
case "REQUEST_CHANGES": return "CHANGES_REQUESTED"
default: return "COMMENTED"
```

This is intentionally preserved. The `default → "COMMENTED"` branch is still used by `EventToStateForTest` and the test in `poster_test.go:584-594` (`Describe("eventToState", ...)` block with the `It("maps COMMENT to COMMENTED", ...)` case). Do NOT change the function. Do NOT remove the test. Do NOT add an `EventToStateForTest` call for a fresh-review allow-list — that is at the caller (`steps_review.go`), not in `eventToState`.

## Step 5 — Confirm the dismissal-skip-on-`COMMENTED` filter is untouched

In `poster.go` around line 175, the `listBotReviews` filter has the predicate `r.User.Login == p.botLogin && r.CommitID != headSHA && r.State != "COMMENTED"`. The `r.State != "COMMENTED"` clause is FROZEN. Historical `COMMENTED` reviews from before this fix must still be skipped during dismissal. The tests at `poster_test.go` around lines 305, 485, 591, and 845 continue to assert the dismissal-skip behavior. Do NOT touch the predicate. Do NOT touch those tests.

## Step 6 — Run the full precommit suite from `agent/pr-reviewer/`

```
cd agent/pr-reviewer
make precommit
go test ./pkg/githubposter/...
```

Expected outcomes:
- `make precommit` exits 0 (format, generate, test, lint, license).
- `go test ./pkg/githubposter/...` exits 0. The two new tests added by the sibling TDD prompt now pass:
  - `Entry("approve+autoApprove:false → APPROVE (post-fix contract — spec 060)", ...)` passes — the production code now returns `event = "APPROVE"` for verdict=approve regardless of `autoApprove`.
  - `Context("allow-list excludes COMMENTED for fresh review (spec 060)", ...)` passes — the verifier returns `Found:false, Class:Transient` for an allow-list of just `["COMMENTED"]` (no review matches; the `findReview` loop finds no state match, so `findReview` returns `(_, false)` and the function returns the phantom-POST failure).
- All pre-existing tests continue to pass. In particular:
  - The existing `Entry("approve+autoApprove:true → APPROVE", ...)` still passes.
  - The existing `Entry("request-changes → REQUEST_CHANGES", ...)` still passes.
  - The `Describe("eventToState", ...)` block (lines 584-594) still passes — `eventToState` is unchanged.
  - The `Context("dismissal skips state=COMMENTED prior bot reviews", ...)` block (lines 305-369) still passes — the dismissal filter is unchanged.
  - The `Entry("Row D: COMMENTED+CHANGES_REQUESTED at older SHA → only CHANGES_REQUESTED dismissed", ...)` (line 486) still passes — same reason.
  - The `Context("case (j): bot review at head SHA is COMMENTED (excluded)", ...)` (lines 845-865) still passes — same reason.

If any pre-existing test fails, STOP and report the regression. Do NOT paper over it by editing the test.

## Step 7 — Add the `## Unreleased` CHANGELOG entry

In `CHANGELOG.md` at the repo root, add (or extend) the `## Unreleased` section at the top of the file. The change is a bug fix (silent downgrade of an approving review to a comment-state review), so the prefix is `fix:`.

The entry:

```
- fix(agent/pr-reviewer): verdict now decides the GitHub review event — `approve` always maps to `APPROVE` and `request-changes` always maps to `REQUEST_CHANGES`, regardless of the per-repo `autoApprove` config. The `autoApprove` flag remains as a config field/YAML key for operator tooling; it no longer downgrades a verdict. The verifier's fresh-review allow-list at the call site in `pkg/steps_review.go` drops `COMMENTED` so a stale `COMMENTED` review at the head SHA is correctly treated as a non-match. Branch protection's "approving review" requirement is now satisfied on docs-only PRs with `autoApprove=false`.
```

Per the changelog guide (`/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`):
- Format: `- <prefix>(<scope>): <what> [context]` — prefix is required.
- `fix:` triggers a patch bump.
- One bullet for the verdict-to-event mapping; do NOT split into multiple bullets.
- Be specific: name the file path (`pkg/githubposter/poster.go` for the function, `pkg/steps_review.go` for the allow-list) and the verdict strings. Do NOT write `- fix: fix bug`.

If `## Unreleased` does NOT exist, create it. If it exists, append to it. Do NOT replace or rewrite existing entries.

## Step 8 — Commit the production fix (TDD trail, part 2)

The dark-factory daemon assigns the branch and creates the commit. The commit subject MUST start with `fix:` and MUST name the githubposter package and the spec — e.g.:

```
fix(agent/pr-reviewer/pkg/githubposter): verdict alone decides the review event (spec 060)
```

The body MUST explain: the production `mapVerdictAndSummary` no longer has a `COMMENT` fallback; the `autoApprove` flag is no longer consulted for the verdict-approve path; the verifier allow-list at the `steps_review.go` call site drops `COMMENTED`. Name the sibling TDD prompt (`1-spec-060-tdd-failing-test.md`) that added the failing tests; reference the changelog entry.

The commit MUST touch only the following files:
- `agent/pr-reviewer/pkg/githubposter/poster.go` (the `mapVerdictAndSummary` body change in Step 1).
- `agent/pr-reviewer/pkg/githubposter/poster_test.go` (the Entry deletion in Step 2).
- `agent/pr-reviewer/pkg/steps_review.go` (the allow-list edit in Step 3).
- `CHANGELOG.md` (the `## Unreleased` entry in Step 7).

No other file in the repo may be modified by this commit. Run `git diff --stat HEAD~1 HEAD` after the commit and confirm only those four files appear. `poster.go` and `verifier.go`'s test files may be implicitly listed by `go generate` if the linter auto-regenerates mocks — verify that no interface signatures changed (they did not), so the `mocks/` directory MUST be untouched. If `go generate` rewrote any mock, investigate and STOP — the signature invariant was violated.

</requirements>

<constraints>
- This is the GREEN step of a TDD cycle. The sibling prompt `1-spec-060-tdd-failing-test.md` lands first and adds two failing tests. Do NOT add the failing tests in this prompt — they are already in the workdir from the prior commit. Do NOT modify the test code beyond deleting the obsolete `Entry("approve+autoApprove:false → COMMENT", ...)`.
- `mapVerdictAndSummary` signature is FROZEN. Do NOT add or remove parameters, do NOT change return arity. Only the body changes.
- `eventToState` is FROZEN. Do NOT modify the function. Do NOT delete the `Describe("eventToState", ...)` test block. The `default → "COMMENTED"` mapping is preserved.
- The dismissal filter `r.State != "COMMENTED"` at `poster.go:175` is FROZEN. Do NOT modify the predicate. Do NOT modify the `dismissal skips state=COMMENTED` tests at `poster_test.go` lines 305, 485, 591, 845.
- The `autoApprove` field on the `maintainerconfig.PrReviewerConfig` struct, the `autoApprove` parameter on `mapVerdictAndSummary`, the `prReviewer.autoApprove` YAML key, and the `ReadAutoApprove` function in `config.go` are FROZEN. Do NOT remove or rename.
- Do NOT add a new config flag, opt-out, or override that re-enables the old `COMMENT` fallback. The mapping is invariant after this spec. The spec's Non-Goals section is LOAD-BEARING — if a "future caller needs a comment-only path" emerges, that is a separate spec.
- Do NOT change behavior of the CLI-level `handleGitHubApprove` / `submitBitbucketReview` paths in `agent/pr-reviewer/cmd/cli/main.go`. They have their own gating semantics and are tracked separately.
- Do NOT modify the body-truncation logic (`maxGitHubCommentBody`, `maxGitHubCommentBodyNotice`), the empty-summary soft-warning, or the 65,536-byte constant.
- Do NOT touch `lib/`, `watcher/`, `agent/github-releaser/`, or any sibling service.
- The `cli/main.go` paths (`handleGitHubApprove`, `submitBitbucketReview`) are explicitly out of scope per spec Non-Goals — do NOT edit them.
- Do NOT regenerate counterfeiter mocks — no interface signatures changed.
- Do NOT commit to git yourself. The dark-factory daemon handles the commit subject, the commit body, and the push. You write the code; the daemon commits.
- The DARK-FACTORY-REPORT block, the Fast Feedback Command section, and the Changelog suffix are auto-injected by the daemon. Do NOT include them in this prompt's body.
- Atomic-batch constraint: this prompt MUST land AFTER the sibling TDD prompt `1-spec-060-tdd-failing-test.md`. The daemon's prompt ordering guarantees sequential execution when both are queued; do not assume parallel execution. If the failing-test commit is not in `HEAD~1` when this prompt starts, STOP and report — the TDD trail will be missing.
</constraints>

<verification>
```
cd agent/pr-reviewer
make precommit
go test ./pkg/githubposter/...
go test ./pkg/... 2>&1 | tail -20
grep -n 'event = "COMMENT"' pkg/githubposter/poster.go
grep -n 'auto-approve disabled' pkg/githubposter/poster.go
grep -n 'COMMENTED' pkg/steps_review.go
```

Expected (REPORT these explicitly in the completion summary):

1. `make precommit` exits 0. Quote the final lines of the make output that show the lint, test, and license checks passing.
2. `go test ./pkg/githubposter/...` exits 0. The failing tests from the sibling TDD prompt now pass:
   - Quote the test runner's pass line for the `verdict to event/state mapping` `DescribeTable` — confirm three Entries pass (the two pre-existing plus the new "approve+autoApprove:false → APPROVE (post-fix contract — spec 060)").
   - Quote the test runner's pass line for the `Context("allow-list excludes COMMENTED for fresh review (spec 060)", ...)` block.
3. `go test ./pkg/...` exits 0 — all pr-reviewer package tests pass. In particular, the `Context("case (j): bot review at head SHA is COMMENTED (excluded)", ...)` test at `poster_test.go:845-865` and the `Context("dismissal skips state=COMMENTED prior bot reviews", ...)` at `poster_test.go:305-369` continue to pass.
4. `grep -n 'event = "COMMENT"' pkg/githubposter/poster.go` returns ZERO lines.
5. `grep -n 'auto-approve disabled' pkg/githubposter/poster.go` returns ZERO lines.
6. `grep -n 'COMMENTED' pkg/steps_review.go` returns ZERO lines (the only occurrence was the third element of the `ExpectedStates` slice; it is now gone).
7. `git diff --stat HEAD~1 HEAD` shows ONLY the four files from this prompt (poster.go, poster_test.go, steps_review.go, CHANGELOG.md). Quote that diff output verbatim. (The sibling TDD commit's diff stat is informational, not load-bearing here — its presence is asserted by the atomic-batch constraint above.)
8. CHANGELOG.md has the new `fix:` entry under `## Unreleased`. Quote the verbatim bullet.
9. The `pkg/githubposter/eventToState` function and its test block are UNCHANGED — quote the relevant lines from `git diff` to prove `eventToState` has no production diff and `Describe("eventToState", ...)` has no test diff.
10. The dismissal filter at `poster.go:175` is UNCHANGED — quote the line from `git diff` showing `r.State != "COMMENTED"` is still present (or that `git diff` for that line is empty).

If `make precommit` fails, STOP and report the failure with the exact lint/test error. Do NOT paper over it by adding `//nolint` or weakening the test.

If `grep` in step 4, 5, or 6 returns any non-zero matches, STOP — the production fix is incomplete.
</verification>
