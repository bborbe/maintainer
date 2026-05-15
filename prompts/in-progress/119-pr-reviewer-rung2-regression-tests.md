---
status: committing
summary: 'Added 4 regression tests locking in spec-027 bugs: extracted ResolveBotLogin helper with DescribeTable test, added COMMENTED-dismissal filter test in poster_test.go, added multi-line fenced JSON verdict tests against spec-025 schema, and extracted ExtractPRURL helper with 5-entry table test covering the watcher H1-section-body regression.'
container: maintainer-119-pr-reviewer-rung2-regression-tests
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-15T20:30:00Z"
queued: "2026-05-15T20:56:55Z"
started: "2026-05-15T21:02:24Z"
completed: "2026-05-15T20:59:31Z"
lastFailReason: 'setup workflow: git fetch origin: fetch from origin: exit status 128'
---

<summary>
- Add 4 regression tests in `agent/pr-reviewer/` to lock in bugs caught during spec-027 Rung-2 verification on 2026-05-15
- Each bug shipped past spec-027's unit tests because the original tests used synthetic fixtures that didn't mirror real LLM output / watcher-authored task format
- Pure test additions — no production code changes (all 4 fixes are already shipped in commits e5536b2 and e6743af)
- Existing test files are extended in place; no new files
- `make precommit` in `agent/pr-reviewer/` clean at the end
</summary>

<objective>
Lock in 4 regression tests against bugs already fixed in commits e5536b2 (3 fixes) + e6743af (1 fix). Each test must FAIL against the pre-fix code and PASS against the current code. Goal: prevent these bug classes from recurring; spec-027's own unit tests missed them all because the fixtures were synthetic.
</objective>

<context>

**The 4 bugs (all already fixed — verify by reading the commits before writing tests):**

1. **`BOT_GITHUB_LOGIN` env unset → empty `botLogin` passed to poster → bot-identity self-check rejects with "expected '' got pr-review-of-ben"** (3 call sites in `pkg/factory/factory.go` lines 167, 215 and `pkg/factory/runner.go` line 78). Fix added `if botLogin == "" { botLogin = githubposter.DefaultBotLogin }` at all 3 sites. Commit `e5536b2`.

2. **Dismissal filter does NOT exclude `state=COMMENTED` reviews** in `pkg/githubposter/poster.go` `listBotReviews`. GitHub's API rejects dismissal of comment-state reviews with HTTP 422 "Can not dismiss a commented pull request review". Fix added `&& r.State != "COMMENTED"` to the filter. Commit `e5536b2`.

3. **`parseJSONVerdict` required both `"verdict"` AND `"reason"` on a single line** in `pkg/verdict.go`. The pre-spec-025 schema had `"reason"`; the new schema (post spec 025) uses `"summary"` instead, and Claude emits pretty-printed multi-line JSON. The old parser always failed → heuristic fell back to fail-closed `request-changes` for every successful review. Fix replaced the per-line scanner with a brace-balanced JSON-block extractor + `json.Unmarshal`. Commit `e5536b2`.

4. **PR URL extraction used `md.Preamble`** in `pkg/steps_checkout_execution.go`. agentlib splits sections at both `# ` (H1) and `## ` (H2). The watcher writes task body as `# PR Review: ...\n\n<URL>\n## Plan`, so the H1 starts immediately and `md.Preamble` is always empty. The URL sits inside the H1 section's Body. Pre-fix code always reported "no GitHub PR URL found in task preamble" and routed every prod task to `human_review` — even though the URL was clearly in the body. Fix extended the scan to include every section before the first `## ` heading. Commit `e6743af`.

**Files to read fully before writing tests:**

1. `agent/pr-reviewer/pkg/factory/factory.go` — `CreateAgent` line 167 + `CreateAgentProvider` line 215; understand the env-var read + fallback pattern
2. `agent/pr-reviewer/pkg/factory/factory_test.go` — current Ginkgo structure
3. `agent/pr-reviewer/pkg/factory/runner.go` line 78 — third fallback site
4. `agent/pr-reviewer/pkg/githubposter/poster.go` lines 188-200 (`listBotReviews`) — dismissal filter
5. `agent/pr-reviewer/pkg/githubposter/poster_test.go` — Counterfeiter mock setup for `HTTPClient`
6. `agent/pr-reviewer/pkg/verdict.go` — new `parseJSONVerdict` + `findLastJSONVerdictBlock` + helpers; understand the contract before testing
7. `agent/pr-reviewer/pkg/verdict_test.go` — existing test contexts; new tests append after the last `Context`
8. `agent/pr-reviewer/pkg/steps_checkout_execution.go` lines 207-232 — URL extraction logic
9. `agent/pr-reviewer/pkg/steps_checkout_execution_test.go` — existing test patterns for the execution step
10. Read commits `e5536b2` and `e6743af` via `git show <sha>` to see the exact pre-fix vs post-fix diffs

**Coding guides** (in `~/.claude/plugins/marketplaces/coding/docs/`):
- `go-testing-guide.md` — Ginkgo v2 + Gomega + `DescribeTable`/`Entry`; external `*_test` package; never call error-returning funcs bare in `It` blocks
- `go-mocking-guide.md` — Counterfeiter patterns for `HTTPClient`
- `go-error-wrapping-guide.md` — `bborbe/errors` only (test code uses `Expect(err).NotTo(HaveOccurred())`, not raw checks)

</context>

<requirements>

Execute steps in order. Run `make test` after each step for fast feedback. Run `make precommit` only at the final step.

---

## Step 1 — Extract `resolveBotLogin` helper + test it directly

The current code duplicates the same 3-line fallback at 3 call sites in `pkg/factory/`:
- `CreateAgent` (factory.go around line 167)
- `CreateAgentProvider` (factory.go around line 218)
- `RunAgent` (runner.go around line 78)

**1a. Extract a helper** in `pkg/factory/factory.go` (or a new `pkg/factory/botlogin.go` if cleaner). Behavior-preserving refactor — pure composition, no I/O:

```go
// resolveBotLogin returns env[githubposter.BotLoginEnv] when set, else
// githubposter.DefaultBotLogin. Centralizes the fallback so all 3 call
// sites stay in sync.
func resolveBotLogin(env map[string]string) string {
    if v := env[githubposter.BotLoginEnv]; v != "" {
        return v
    }
    return githubposter.DefaultBotLogin
}
```

Replace the 3 inline `if botLogin == ""` blocks with `botLogin := resolveBotLogin(env)`. Net diff: 9 lines removed, 1 helper added.

**1b. Add `pkg/factory/botlogin_test.go`** (Ginkgo, external `factory_test` package) using `DescribeTable`:

```go
DescribeTable("resolveBotLogin",
    func(env map[string]string, expected string) {
        Expect(factory.ResolveBotLogin(env)).To(Equal(expected))
    },
    Entry("env nil → DefaultBotLogin", nil, githubposter.DefaultBotLogin),
    Entry("env empty map → DefaultBotLogin", map[string]string{}, githubposter.DefaultBotLogin),
    Entry("env has empty string → DefaultBotLogin", map[string]string{githubposter.BotLoginEnv: ""}, githubposter.DefaultBotLogin),
    Entry("env has custom value → returns it verbatim", map[string]string{githubposter.BotLoginEnv: "custom-bot"}, "custom-bot"),
)
```

Export as `ResolveBotLogin` (uppercase) since the test is in an external package. Update the 3 call sites to use the exported name too — they're all in the same module.

This single table-driven test covers all 3 call sites at once: any of them deviating from the helper would need re-introducing the inline fallback, which the refactor in 1a forbids.

**No `runner_test.go` is created** — the helper test replaces the need for it.

---

## Step 2 — Test dismissal filter skipping `COMMENTED` in `pkg/githubposter/poster_test.go`

Append a new `Context` to the existing `Describe("Post", ...)`:

**Context: dismissal skips state=COMMENTED prior bot reviews**

- BeforeEach: inject `FakeHTTPClient` returning:
  - `GET /user` → 200 with `{"login":"pr-review-of-ben"}`
  - `GET /pulls/N/reviews` (dismiss-list) → 200 with a list containing THREE entries by the bot on the same commit_id:
    - one `state=COMMENTED`
    - one `state=APPROVED`
    - one `state=CHANGES_REQUESTED`
  - `PUT /pulls/N/reviews/{id}/dismissals` (any id) → 200
  - `POST /pulls/N/reviews` → 200 with the new review
  - verify `GET /pulls/N/reviews` → 200 with the new review present

- Run `Post` with verdict `request-changes` (so a real REQUEST_CHANGES is posted; mapping is incidental).

- Assertions:
  - `result.Outcome == "success"`
  - The fake HTTPClient received EXACTLY 2 `PUT .../dismissals` calls (for the APPROVED and CHANGES_REQUESTED reviews) — NOT 3. The COMMENTED review's ID is never dismissed.
  - Capture the dismissed review IDs via `Invocations()` and assert the COMMENTED review's id is NOT among them.

This test would have failed before the `r.State != "COMMENTED"` filter was added — the third PUT would have hit 422 from GitHub in production.

---

## Step 3 — Test `parseJSONVerdict` against the actual Claude output format in `pkg/verdict_test.go`

Append two new `Context` blocks to the existing `Describe("Parse", ...)`:

**Context: multi-line fenced JSON with new spec-025 schema (no "reason" field)**

```go
BeforeEach(func() {
    reviewText = `# Code Review

The PR adds an HTML comment to README.md.

` + "```json" + `
{
  "verdict": "approve",
  "summary": "Trivial doc-only change, no findings.",
  "comments": [],
  "concerns_addressed": [
    "correctness: no logic changes"
  ]
}
` + "```" + ``
})

It("returns VerdictApprove from the multi-line block", func() {
    Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
})
```

Add a second test for `request-changes` with the same multi-line fenced shape — assert `VerdictRequestChanges`.

The **load-bearing assertion** is that `parseJSONVerdict` returns `(result, true)` for the multi-line block. Pre-fix the per-line scanner returned `(_, false)` and the heuristic fell back to fail-closed `request-changes`. Skip any `result.Reason == ""` assertion — it's tautological under the new schema since `jsonVerdict` has no `Summary` field; `Reason` is always empty for current LLM output.

To strengthen, add one more `Context`:

**Context: malformed JSON in fenced block falls back to heuristic**

```go
reviewText = "```json\n{verdict: invalid no quotes\n```\n## Must Fix\n- problem"
// Expect: heuristic kicks in → VerdictRequestChanges, Reason "must-fix items found"
```

This locks in the recovery path: a broken fenced block must NOT swallow the entire parse — the `## Must Fix` section's verdict still wins.

---

## Step 4 — Extract `extractPRURL` helper + test it directly

The current URL-extraction logic lives inside the unexported `runClaude` method (steps_checkout_execution.go:209-232). Testing it through `runClaude` requires a working `claudelib.NewClaudeRunner` + container path — non-trivial integration.

**4a. Extract** the URL-extraction lines into a small unexported helper in the same file:

```go
// extractPRURL scans md.Preamble plus every section before the first "## "
// (H2) heading. The watcher writes the task body as
//   # PR Review: <title>
//   <PR URL>
//   ## Plan
// so the H1 starts immediately and md.Preamble is always empty under
// agentlib's section-split-on-# semantics. Scanning the H1 section's body
// finds the URL; stopping at the first H2 prevents matching URLs Claude
// later writes inside ## Review.
func extractPRURL(md *agentlib.Markdown) string {
    if u := githubPRURLPattern.FindString(md.Preamble); u != "" {
        return u
    }
    for _, sec := range md.Sections {
        if strings.HasPrefix(sec.Heading, "## ") {
            break
        }
        if u := githubPRURLPattern.FindString(sec.Heading + "\n" + sec.Body); u != "" {
            return u
        }
    }
    return ""
}
```

Replace the inline block at the top of `runClaude` with `prURLStr := extractPRURL(md)`.

**4b. Add test cases** in `pkg/steps_checkout_execution_test.go` (or a new dedicated file). Use a table:

| input markdown body | expected return |
|---|---|
| `# PR Review: test\n\nhttps://github.com/bborbe/maintainer/pull/2\n## Plan\n\nbody` | `https://github.com/bborbe/maintainer/pull/2` |
| `# H1\n\nhttps://github.com/owner/repo/pull/42\n## Plan` | `https://github.com/owner/repo/pull/42` |
| `https://github.com/owner/repo/pull/1\n\n## Plan` (URL in preamble — no H1) | `https://github.com/owner/repo/pull/1` |
| `# H1\n\n## Plan\n\nhttps://github.com/owner/repo/pull/1` (URL only after H2) | `""` |
| `# H1 only\n\nno url here\n## Plan` | `""` |

The first row is the **load-bearing regression test** — it would have failed pre-fix because `md.Preamble == ""` and the H1-section-body scan didn't exist. The fourth row catches the inverse regression: URL inside `## Review` (Claude-written) must NOT be matched.

To construct the `*agentlib.Markdown`, call `agentlib.ParseMarkdown(ctx, []byte(body))` and pass the result to `extractPRURL`.

If the helper signature is exported as `ExtractPRURL` (for external `_test` package access), update the call site in `runClaude` accordingly.

Skip the negative-failure-mode test in the diagnostics path — that's an integration concern; the helper test above proves the URL-extraction logic itself.

---

## Step 5 — Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0. Coverage for `pkg/githubposter` and `pkg/factory` should not regress.

---

## Step 6 — No CHANGELOG entry

Skip. The root CHANGELOG uses release-section headers (`## vX.Y.Z`) — no `## Unreleased` convention. Test-only commits and the prior bug-fix commits (`e5536b2`, `e6743af`) don't carry their own changelog entries; they'll be summarized at the next release. Adding a stray `## Unreleased` deviates from the existing pattern.

</requirements>

<constraints>

- **No behavioral changes to production code.** The 2 helper extractions in steps 1a and 4a are pure refactors — they centralize duplicated logic that already exists at multiple sites. Confirm via `git diff` that the runtime semantics of factory.go / runner.go / steps_checkout_execution.go are unchanged.
- **If a test reveals a NEW bug** not covered by commits e5536b2/e6743af, STOP and report rather than fix — surface to operator.
- **All tests use Ginkgo v2 + Gomega + Counterfeiter** per existing patterns.
- **External `*_test` package** for all new tests; helpers exported (uppercase) where needed.
- **`bborbe/errors`** in any non-test code touched. Tests use `Expect(err).NotTo(HaveOccurred())` per existing convention.
- **Coverage ≥80%** preserved for both `pkg/githubposter` and `pkg/factory`.
- **`make precommit`** runs from `agent/pr-reviewer/`, never repo root.
- **Do not commit.** dark-factory handles git.
- **Each test must clearly fail against pre-fix code.** If a test passes both before and after, drop it or rewrite to assert the fix's actual behavior change.
- **No new files** unless a refactor genuinely warrants one (e.g., the `botlogin_test.go` in step 1b — appended to a new file is fine if `factory_test.go` is already crowded).

</constraints>

<verification>

```bash
cd agent/pr-reviewer && make precommit
```

Then sanity-grep:

```bash
# Step 1: ResolveBotLogin helper + table test exist
grep -n "ResolveBotLogin\|resolveBotLogin" agent/pr-reviewer/pkg/factory/*.go
# Expected: helper defined + at least 3 call sites use it (factory.CreateAgent, CreateAgentProvider, runner.RunAgent)

grep -rn "ResolveBotLogin" agent/pr-reviewer/pkg/factory/botlogin_test.go agent/pr-reviewer/pkg/factory/factory_test.go 2>/dev/null
# Expected: DescribeTable with at least 4 Entry rows

# Step 2: dismissal test for COMMENTED state present
grep -n "COMMENTED.*dismiss\|state=COMMENTED\|skip.*COMMENT" agent/pr-reviewer/pkg/githubposter/poster_test.go
# Expected: one or more matches in a new Context

# Step 3: verdict test against new schema
grep -n "multi-line fenced\|spec-025 schema\|concerns_addressed" agent/pr-reviewer/pkg/verdict_test.go
# Expected: one or more matches

# Step 4: ExtractPRURL helper + table test
grep -n "ExtractPRURL\|extractPRURL" agent/pr-reviewer/pkg/steps_checkout_execution.go agent/pr-reviewer/pkg/steps_checkout_execution_test.go
# Expected: helper defined; call site uses it; test has DescribeTable / Entries

# No stray CHANGELOG entry added (per Step 6 — skip):
git diff HEAD CHANGELOG.md
# Expected: empty diff (no test-only changelog entry)
```

</verification>
