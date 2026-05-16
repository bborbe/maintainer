---
status: completed
spec: [031-bug-pr-reviewer-dismisses-current-head-review]
summary: Inverted SHA filter in listBotReviews from == to != so dismissal removes only prior-commit reviews, never the current-head review; added invariant comment and Dismissal Contract doc section; updated three existing tests and added DescribeTable with 6 rows covering the full dismissal eligibility matrix.
container: maintainer-121-fix-031-dismissal-sha-filter
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-16T10:45:00Z"
queued: "2026-05-16T11:53:12Z"
started: "2026-05-16T11:56:22Z"
completed: "2026-05-16T12:03:36Z"
branch: dark-factory/bug-pr-reviewer-dismisses-current-head-review
---

<summary>
- The dismissal filter in `listBotReviews` is inverted: `r.CommitID == headSHA` selects the just-posted current-head review for dismissal, not prior reviews — this is the root cause of PR #5 ending with zero GitHub reviews after a successful agent run.
- The single-character fix (== → !=) makes the filter semantically match the function name `dismissPriorReviews`: only reviews at SHAs other than the current head are selected for dismissal.
- An invariant comment is planted immediately above the filter naming the SHA rule and referencing `docs/pr-post-back.md §Dismissal Contract` and spec 031.
- A `## Dismissal Contract` subsection is added to `agent/pr-reviewer/docs/pr-post-back.md`, stating that only reviews at superseded SHAs are dismissed and the current-head review is always preserved.
- Three existing tests that placed prior reviews at `testHeadSHA` and relied on the buggy `==` behavior are updated to use `"sha-prior"` so their intent (call ordering, COMMENTED-filter, permanent-failure routing) is preserved under the corrected `!=` predicate.
- A new `DescribeTable("listBotReviews SHA filter")` with 6 entries (Rows A–F) covers the full multi-SHA history matrix and will fail against the pre-fix `==` operator.
- `make precommit` in `agent/pr-reviewer/` exits 0.
</summary>

<objective>
Invert the SHA comparison in `listBotReviews` from `==` to `!=` so the dismissal step removes only reviews at prior (superseded) commits — never the review the current pod just posted at the current head SHA. Plant an invariant comment and add a Dismissal Contract doc section so a future reader cannot re-invert it without first arguing against them.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.

Read these files in full before writing any code:
- `agent/pr-reviewer/pkg/githubposter/poster.go` — specifically the `listBotReviews` method (around line 160–205); this is the only production file being changed.
- `agent/pr-reviewer/pkg/githubposter/poster_test.go` — understand every existing `Context` and fixture before touching any test. Three tests placed prior reviews at `testHeadSHA` and will fail after the fix without remediation (details in requirements).
- `agent/pr-reviewer/docs/pr-post-back.md` — understand the existing section structure before adding the new section.

Read these coding-guideline files (the `bborbe/coding` plugin is mounted in the container at `/home/node/.claude/plugins/marketplaces/coding/docs/`; if not at that path, locate via `find / -name go-testing-guide.md 2>/dev/null | head -1`):
- `go-testing-guide.md` — Ginkgo v2 `DescribeTable`/`Entry`, external `*_test` package, coverage ≥80%.
- `go-filter-pattern.md` — predicate filter patterns for Go.
- `test-pyramid-triggers.md` — which test types to write for each code change kind.

Inline summary (sufficient even if the guide files cannot be read):
- Tests live in external `*_test` package (`package githubposter_test`), use Ginkgo v2 + Gomega, prefer `DescribeTable`/`Entry` for table-driven cases.
- Filter predicate semantics: return `true` to KEEP an item in the output set, return `false` to EXCLUDE. Match the variable/comment direction to that convention.

**Key fact — the bug location:** in `poster.go`, inside `listBotReviews`, inside the `for _, r := range all {` loop:

```go
// CURRENT (BUGGY):
if r.User.Login == p.botLogin && r.CommitID == headSHA && r.State != "COMMENTED" {
```

The predicate `r.CommitID == headSHA` selects the current-head review for dismissal — the exact opposite of the intended behavior. Fix: change `==` to `!=`.

**Key fact — affected existing tests:** three existing `Context` blocks in `poster_test.go` use `reviewJSON(..., testHeadSHA, ...)` for a "prior" review and expect it to be dismissed. After the fix, a review at `testHeadSHA` is never dismissed. These tests must be updated to use `"sha-prior"` as the prior review's commit_id (details in Step 3).

</context>

<requirements>
Execute steps in order. Run `make test` after Step 4 for fast feedback. Run `make precommit` only at the final step.

---

## Step 1 — Read all referenced files

Read every file listed in `<context>` before writing a single line of code. Specifically identify the three tests that need updating (they use `reviewJSON(..., testBotLogin, testHeadSHA, ...)` for a prior review and assert dismissal). Grep to confirm:

```bash
grep -n "testHeadSHA" agent/pr-reviewer/pkg/githubposter/poster_test.go
```

---

## Step 2 — Fix the filter predicate in `poster.go` (the one-line bug fix)

File: `agent/pr-reviewer/pkg/githubposter/poster.go`

Locate the `listBotReviews` method. Find the line inside the `for _, r := range all {` loop that reads (approximately line 195):

```go
if r.User.Login == p.botLogin && r.CommitID == headSHA && r.State != "COMMENTED" {
```

Make two changes in-place:

**2a. Add invariant comment immediately ABOVE this `if` line** (preserve the existing COMMENTED-skip comment block above — add the new invariant comment between that comment block and the `if` line):

```go
// Invariant (spec 031, docs/pr-post-back.md §Dismissal Contract):
// reviews at the current head SHA are NEVER dismissed — only reviews at
// superseded (prior) SHAs are eligible. A re-spawned pod must not wipe the
// review that a previous pod left at the same head.
```

**2b. Change the predicate from `==` to `!=`:**

```go
if r.User.Login == p.botLogin && r.CommitID != headSHA && r.State != "COMMENTED" {
```

No other changes to `poster.go`. The COMMENTED-skip comment block above the `if` line must be preserved verbatim.

---

## Step 3 — Update three existing tests in `poster_test.go`

All three tests placed prior reviews at `testHeadSHA`, which the old `==` filter incorrectly selected for dismissal. After the fix (`!=`), a review at the current head SHA is never selected. Update each to use `"sha-prior"` as the prior review's `commit_id` so the test's intent (ordering, COMMENTED-filter, permanent-failure routing) remains valid:

**Test 3a — "dismissal before POST" context**

Find the line:
```go
priorReview := reviewJSON(99, testBotLogin, testHeadSHA, "APPROVED")
```
Change `testHeadSHA` to `"sha-prior"`:
```go
priorReview := reviewJSON(99, testBotLogin, "sha-prior", "APPROVED")
```
With `!=`, a review at `"sha-prior"` IS selected for dismissal. The 5-call seqStub (GET /user + GET list + PUT + POST + verify-GET) remains correct. No other changes needed in this context.

**Test 3b — "permanent dismissal failure" context**

Find the line:
```go
priorReview := reviewJSON(99, testBotLogin, testHeadSHA, "APPROVED")
```
Change to:
```go
priorReview := reviewJSON(99, testBotLogin, "sha-prior", "APPROVED")
```
With `!=`, the review at `"sha-prior"` IS selected → PUT is attempted → 403 → permanent failure at "PUT .../dismissals". The 3-call seqStub and all assertions remain correct.

**Test 3c — "dismissal skips state=COMMENTED" context**

Find the `BeforeEach` block with three `reviewJSON` calls all at `testHeadSHA`:
```go
reviewJSON(commentedID,  testBotLogin, testHeadSHA, "COMMENTED"),
reviewJSON(approvedID,   testBotLogin, testHeadSHA, "APPROVED"),
reviewJSON(changesReqID, testBotLogin, testHeadSHA, "CHANGES_REQUESTED"),
```
Change all three `testHeadSHA` to `"sha-prior"`:
```go
reviewJSON(commentedID,  testBotLogin, "sha-prior", "COMMENTED"),
reviewJSON(approvedID,   testBotLogin, "sha-prior", "APPROVED"),
reviewJSON(changesReqID, testBotLogin, "sha-prior", "CHANGES_REQUESTED"),
```
With `!=`, all three reviews pass the SHA predicate (sha-prior ≠ testHeadSHA). COMMENTED is still excluded by `r.State != "COMMENTED"`. APPROVED and CHANGES_REQUESTED are dismissed. The 6-call sequence and all existing assertions (exactly 2 PUT calls, correct IDs, COMMENTED not dismissed) remain correct.

---

## Step 4 — Add `DescribeTable("listBotReviews SHA filter")` with 6 entries

**4a. Add `testPriorSHA` constant** alongside the existing constants at the top of `poster_test.go`:

```go
const (
    testBotLogin = "pr-review-of-ben"
    testHeadSHA  = "sha123abc"
    testPriorSHA = "sha-prior"
)
```

**4b. Append the DescribeTable** inside the top-level `Describe("PrPoster", ...)`, after the "POST request body shape" context (the last existing context):

```go
DescribeTable("listBotReviews SHA filter — dismissal eligibility",
    func(inputReviews []string, expectedDismissedIDs []int64) {
        // Build the full HTTP call sequence:
        //   GET /user + GET dismiss-list + N×PUT dismissal + POST + GET verify
        specs := []callSpec{
            {200, botUserJSON(), nil},
            {200, reviewListJSON(inputReviews...), nil},
        }
        for range expectedDismissedIDs {
            specs = append(specs, callSpec{200, `{}`, nil})
        }
        specs = append(specs, callSpec{201, postRespJSON(999), nil})
        specs = append(specs, callSpec{200, reviewListJSON(
            reviewJSON(999, testBotLogin, testHeadSHA, "CHANGES_REQUESTED"),
        ), nil})

        fakeClient.DoStub = seqStub(specs)

        result := poster.Post(ctx, prpkg.PostRequest{
            PR:      pr,
            HeadSHA: testHeadSHA,
            Verdict: prpkg.VerdictRequestChanges,
            Summary: "test",
            WorkDir: tmpDir,
        })
        Expect(result.Outcome).To(Equal("success"),
            "expected full posting sequence to complete; got outcome=%s class=%s step=%s msg=%s",
            result.Outcome, result.Class, result.FailureStep, result.ErrorMessage,
        )

        // Collect dismissed review IDs from PUT /dismissals calls
        invs := fakeClient.Invocations()["Do"]
        var dismissedIDs []int64
        for _, call := range invs {
            req, ok := call[0].(*http.Request)
            Expect(ok).To(BeTrue())
            if req.Method == "PUT" && strings.Contains(req.URL.Path, "dismissals") {
                // URL.Path: /repos/owner/repo/pulls/1/reviews/<id>/dismissals
                // Split: ["", "repos", "owner", "repo", "pulls", "1", "reviews", "<id>", "dismissals"]
                parts := strings.Split(req.URL.Path, "/")
                if len(parts) == 9 && parts[8] == "dismissals" {
                    var id int64
                    _, _ = fmt.Sscanf(parts[7], "%d", &id)
                    dismissedIDs = append(dismissedIDs, id)
                }
            }
        }
        if len(expectedDismissedIDs) == 0 {
            Expect(dismissedIDs).To(BeEmpty(),
                "expected no dismissals but got %v", dismissedIDs)
        } else {
            Expect(dismissedIDs).To(ConsistOf(expectedDismissedIDs))
        }
    },
    // Row A: two bot reviews — one at older SHA (dismissable), one at head SHA (preserved)
    Entry("Row A: older-SHA+head-SHA reviews → only older-SHA dismissed",
        []string{
            reviewJSON(10, testBotLogin, testPriorSHA, "APPROVED"),  // older SHA → selected
            reviewJSON(20, testBotLogin, testHeadSHA, "APPROVED"),   // head SHA → preserved
        },
        []int64{10},
    ),
    // Row B: single review at head SHA → nothing dismissed
    Entry("Row B: single review at head SHA → nothing dismissed",
        []string{
            reviewJSON(30, testBotLogin, testHeadSHA, "APPROVED"),
        },
        []int64{},
    ),
    // Row C: two reviews both at head SHA → neither dismissed
    Entry("Row C: two reviews at head SHA → neither dismissed",
        []string{
            reviewJSON(40, testBotLogin, testHeadSHA, "APPROVED"),
            reviewJSON(41, testBotLogin, testHeadSHA, "CHANGES_REQUESTED"),
        },
        []int64{},
    ),
    // Row D: COMMENTED at older SHA excluded by state filter; CHANGES_REQUESTED at older SHA dismissed
    Entry("Row D: COMMENTED+CHANGES_REQUESTED at older SHA → only CHANGES_REQUESTED dismissed",
        []string{
            reviewJSON(50, testBotLogin, testPriorSHA, "COMMENTED"),
            reviewJSON(51, testBotLogin, testPriorSHA, "CHANGES_REQUESTED"),
        },
        []int64{51},
    ),
    // Row E: non-bot review at older SHA → never dismissed (botLogin filter)
    Entry("Row E: non-bot review at older SHA → nothing dismissed",
        []string{
            reviewJSON(60, "someone-else", testPriorSHA, "APPROVED"),
        },
        []int64{},
    ),
    // Row F: empty review list → nothing dismissed
    Entry("Row F: empty review list → nothing dismissed",
        []string{},
        []int64{},
    ),
)
```

Note: `tmpDir` has no `.pr-reviewer.yaml` by default → `ReadAutoApproveConfig` returns `{AutoApprove: false}`. With `VerdictRequestChanges`, the event is `REQUEST_CHANGES` regardless of autoApprove. The verify-GET stub returns `CHANGES_REQUESTED` — matching `eventToState("REQUEST_CHANGES")`. The sequence is self-consistent.

**Row A is the primary regression test** — it directly exercises the fixed predicate (`!=`). If the predicate is reverted to `==`, Row A fails because review 10 (at older SHA) is no longer selected and review 20 (at head SHA) is selected instead — the `ConsistOf([]int64{10})` assertion catches the mismatch. Row B also fails on revert: the single review at head SHA would be selected for dismissal, consuming the PUT stub, making the POST stub unavailable, and causing the test to fail.

---

## Step 5 — Add "Dismissal Contract" section to `pr-post-back.md`

File: `agent/pr-reviewer/docs/pr-post-back.md`

Read the file first. Add a new `## Dismissal Contract` section after the existing `## nil Poster — Local / Backward-Compatible Mode` section and before the existing `## Key Files` section:

```markdown
## Dismissal Contract

`dismissPriorReviews` removes bot reviews that were left at **superseded** (older) commit SHAs as a PR accumulates new commits. It never removes a review whose `commit_id` equals the PR's current head SHA.

**Invariant:** a bot review at the current head SHA is always preserved by the dismissal step. The verifier (`verifier.go`) looks for a review at the current head SHA to confirm the POST succeeded — the dismissal step must not remove that artifact.

**SHA filter rule** (source: `pkg/githubposter/poster.go` `listBotReviews`, spec 031):

- Review `commit_id == current head SHA` → **never dismissed** (preserves the just-posted review)
- Review `commit_id != current head SHA` → eligible for dismissal, subject to:
  - bot identity filter: `user.login == BOT_GITHUB_LOGIN`
  - state filter: `COMMENTED` reviews are never dismissed — the GitHub API rejects their dismissal with HTTP 422
  - state filter: `DISMISSED` reviews are skipped in the caller loop (already inactive)

**Re-spawn safety:** if a controller re-spawns a pod on the same head SHA, the second pod's dismissal step returns an empty list (the first pod's review is at the current head SHA, which is preserved). The second pod short-circuits on vault idempotency and the PR ends with the first pod's review intact. This is the intended behavior; the original bug (`commit_id == headSHA` instead of `!=`) caused the second pod to wipe the first pod's review, leaving the PR with zero reviews despite a successful agent run.
```

---

## Step 6 — Add CHANGELOG entry

Read `CHANGELOG.md` at the repo root. If `## Unreleased` already exists, append to it; otherwise create it above the most recent `## vX.Y.Z` heading:

```
- fix(agent/pr-reviewer): invert SHA filter in `listBotReviews` from `==` to `!=`; dismissal now removes only prior-commit reviews and never the current-head review; add Dismissal Contract invariant comment and doc section (spec 031)
```

---

## Step 7 — Run `make test` (fast feedback)

```bash
cd agent/pr-reviewer && make test
```

All tests must pass before proceeding. If any fail, fix the root cause — do not proceed to `make precommit` with failing tests.

---

## Step 8 — Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0. If any linter target fails, fix it, then re-run ONLY that target (`make lint`, `make gosec`, etc.) before re-running the full `make precommit`.

</requirements>

<constraints>

- **One-line production code change:** only `poster.go` `listBotReviews` changes (the `==` → `!=` inversion and the invariant comment). No other production Go files change.
- **`verifier.go` MUST NOT change** — its `r.CommitID == headSHA` predicate is correct and load-bearing for the verify path. The spec explicitly lists this as a non-goal.
- **`PrPoster` interface MUST NOT change** — callers (`controller`, `executor`) depend on the existing signature.
- **The `COMMENTED`-state skip MUST be preserved verbatim** — keep the existing comment about HTTP 422. The new invariant comment is added below (or between) it; it does not replace it.
- **The `DISMISSED`-state skip in the caller loop** (`dismissPriorReviews`, approximately `poster.go:150–152`) MUST be preserved.
- **The dismissal HTTP call** (method `PUT`, payload `{"message":"superseded by new automated review"}`, retry policy) MUST NOT change.
- **Existing passing tests** in `poster_test.go` and `verifier_test.go` that do not depend on the old inverted filter MUST continue to pass after Step 3's updates.
- **External `_test` package** for all test code — existing convention `package githubposter_test`.
- **Coverage ≥80%** for `pkg/githubposter` (do not regress from the 88.4% achieved in spec 027).
- **`bborbe/errors`** for any non-test error wrapping. No `fmt.Errorf`. Tests use `Expect(err).NotTo(HaveOccurred())`.
- **No `context.Background()` in non-test code** (tests may use it in `BeforeEach`).
- **`make precommit` in `agent/pr-reviewer/`** only — never at repo root.
- **Do NOT commit** — dark-factory handles git.

</constraints>

<verification>

Run precommit:
```bash
cd agent/pr-reviewer && make precommit
```
Expected: exit 0.

Confirm the fix is present and uses `!=`:
```bash
grep -nE 'r\.CommitID\s*(==|!=)\s*headSHA' agent/pr-reviewer/pkg/githubposter/poster.go
```
Expected: exactly one match using `!=`.

Confirm invariant comment is present above the filter:
```bash
grep -n "current head SHA\|never dismissed\|Dismissal Contract" agent/pr-reviewer/pkg/githubposter/poster.go
```
Expected: ≥2 lines (the invariant comment references both phrases).

Confirm DescribeTable and all 6 rows are present:
```bash
grep -nE 'Row [A-F]|DescribeTable.*SHA filter' agent/pr-reviewer/pkg/githubposter/poster_test.go
```
Expected: ≥7 lines (1 DescribeTable declaration + 6 Entry rows).

Confirm no existing test still asserts old "current-head review is dismissed" behavior:
```bash
grep -n "testHeadSHA" agent/pr-reviewer/pkg/githubposter/poster_test.go
```
Read the output. Any occurrence of `testHeadSHA` in a `reviewJSON(...)` call inside a seqStub must appear in the VERIFY-GET position (the last GET call returning the newly posted review) — NOT in the dismiss-list GET position as a review that is expected to be dismissed. Confirm this by reading the three updated tests and the new DescribeTable entries.

Confirm Dismissal Contract section in docs:
```bash
grep -nE '^#+\s+Dismissal Contract' agent/pr-reviewer/docs/pr-post-back.md
```
Expected: ≥1 line. Confirm the prose contains both "current head" and "superseded":
```bash
grep -n "current head\|superseded" agent/pr-reviewer/docs/pr-post-back.md
```
Expected: matches in the new section.

Confirm CHANGELOG entry:
```bash
grep -n "listBotReviews\|SHA filter\|spec 031\|dismissal.*invert\|invert.*dismissal" CHANGELOG.md | head -5
```
Expected: one entry under `## Unreleased`.

Revert-test (manual — document result in PR description):
```bash
# Temporarily change != back to == in poster.go listBotReviews
cd agent/pr-reviewer && go test ./pkg/githubposter/...
# Expected: non-zero exit; failure output names Row A and/or Row B
# Revert the != back before committing
```

</verification>
