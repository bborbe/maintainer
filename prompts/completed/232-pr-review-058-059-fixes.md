---
status: completed
summary: 'All 9 fixes applied: fail-closed transport errors in ai-review structural checks, plugin-manifest + rewrite_needed + empty-body test coverage, rollupVerdict unknown+structural spec, counterfeiter directive position, aiReviewTools local-scope, CheckPush constant, setupWorkdir godoc clarification, fetchTimeout constant'
container: maintainer-changelog-rewrite-exec-232-pr-review-058-059-fixes
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T17:25:00Z"
queued: "2026-06-02T19:53:30Z"
started: "2026-06-02T19:53:32Z"
completed: "2026-06-02T20:09:30Z"
---

<summary>
- Fixes a fail-open bug in ai-review: transient GitHub API errors during the tag-commit and changelog-header checks no longer leave the corresponding boolean `true` — they now fail closed.
- Adds Ginkgo coverage for the previously untested execution-step body-rewrite branch (rewrite_needed=true happy path + missing-Unreleased error mapping).
- Adds Ginkgo coverage for the plugin-manifest path in the ai-review unexpected-file-change check (both detected-manifest success and detect-error fallback).
- Adds a regression test for rolling up a `unknown` LLM verdict alongside a structural-check failure (both must surface).
- Adds a regression test for the empty-body 200-OK case in the maintainer-config fetcher.
- Moves the counterfeiter directive in `pkg/maintainerconfig/fetcher.go` to sit directly above its interface (project convention).
- Drops a package-level `var` in `pkg/factory/factory.go` that should be a local variable.
- Replaces the raw `"Push"` string in ai-review with a `CheckPush` constant and clarifies one godoc.
- Promotes the 15-second HTTP timeout in the fetcher to a named constant.
- Both `agent/github-releaser/` and `lib/` precommit suites stay green.
</summary>

<objective>
Address PR review findings against the merged `feat/changelog-rewrite` branch (specs 058 + 059). Fix one correctness bug (transport-error fail-open in ai-review), close the named test-coverage gaps, and apply small style fixes. No behavior change other than the fail-closed correction.
</objective>

<context>
Read `CLAUDE.md` at repo root and `agent/github-releaser/CLAUDE.md`.

Read these files BEFORE editing (verify current signatures + style):
- `agent/github-releaser/pkg/steps_ai_review.go` — current ai-review step. Touched in fixes 1, 4, 7.
- `agent/github-releaser/pkg/steps_ai_review_test.go` — existing Ginkgo style; add new specs here for fixes 1, 3, 5.
- `agent/github-releaser/pkg/steps_execution.go` — read to confirm body-rewrite branch + setupWorkdir contract for fixes 2, 8.
- `agent/github-releaser/pkg/steps_execution_test.go` — existing Ginkgo style; add the two body-rewrite specs here for fix 2.
- `agent/github-releaser/pkg/maintainerconfig/fetcher.go` — counterfeiter directive position + 15s timeout. Touched in fixes 6, 9.
- `agent/github-releaser/pkg/maintainerconfig/fetcher_test.go` — existing fetcher test style; add empty-body spec here for fix 6.
- `agent/github-releaser/pkg/factory/factory.go` — package-level `aiReviewTools` to move into `CreateAgent`. Touched in fix 5b.
- `agent/github-releaser/pkg/git/errors.go` (it actually lives at `agent/github-releaser/pkg/git/error_classifier.go`) — `ErrorCategoryUnreleasedNotFound` and related categories; needed for the fix-2 assertion.
- `agent/github-releaser/pkg/changelog/changelog.go` — `ReplaceUnreleasedBody(ctx, content []byte, newBody string) ([]byte, error)` and its "unreleased header not found" wrapped-error contract.
- `agent/github-releaser/mocks/review_client.go` — counterfeiter fake type is `ReviewClient` with stubs `ResolveTagCommitReturns(string, error)` and `FetchChangelogReturns([]byte, error)`. Use these to inject transport errors for fix 1 tests.
- `agent/github-releaser/mocks/git_ops.go` — counterfeiter `GitOps` fake; `CommittedFilesReturns([]string, error)`.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`

Verified symbols (do NOT change):
- `githubreview.Client` interface methods: `TagExists(ctx, owner, repo, tag string) (tagSHA string, error)`, `ResolveTagCommit(ctx, owner, repo, tagSHA string) (commitSHA string, error)`, `FetchChangelog(ctx, owner, repo string) ([]byte, error)`.
- Failed-check constants in `pkg/steps_ai_review.go`: `CheckTagExists`, `CheckTagAtExpectedSHA`, `CheckChangelogHeaderRewritten`, `CheckFaithfulness`, `CheckUnexpectedFileChange`.
- Overall constants: `OverallPass`, `OverallFail`, `OverallUnknown`.
- Error categories in `pkg/git/error_classifier.go`: `ErrorCategoryUnreleasedNotFound = "unreleased_not_found"`.
- `changelog.ReplaceUnreleasedBody` returns a wrapped error when "## Unreleased" is absent; the execution step maps this to `ErrorCategoryUnreleasedNotFound` (see `pkg/steps_execution.go` lines ~229-240).
- Existing fetcher test pattern: `httptest.NewServer` → `maintainerconfig.NewHTTPFetcherForTest("", server.URL)` → `Fetch(ctx, "bborbe", "maintainer", "master")` → `Expect(err.Error()).To(ContainSubstring(...))`.
</context>

<requirements>

## Fix 1 — Fail closed on transport error in two ai-review structural checks (correctness)

In `agent/github-releaser/pkg/steps_ai_review.go`, the helpers `verifyTagAtExpectedCommit` and `verifyChangelogHeaderRewritten` currently leave `checks.TagAtExpectedSHA = true` / `checks.ChangelogHeaderRewritten = true` on transport / API error, then return a wrapped error that the caller `runStructuralChecks` only logs (`glog.Warningf`) and discards. Net effect: a transient GitHub blip silently passes both checks. The companion `verifyTagExists` already follows the correct pattern (sets `checks.TagExists = false` AND appends `CheckTagExists` to `failedChecks` on non-sentinel error). Mirror that.

1a. Change `verifyTagAtExpectedCommit` (around line 454) so that on `s.client.ResolveTagCommit` error it:
   - Sets `checks.TagAtExpectedSHA = false`.
   - Appends `CheckTagAtExpectedSHA` to `*failedChecks`.
   - Logs at V(2) including the error.
   - Returns `nil` (NOT a wrapped error — the failure is a recorded check, not a controller-retry trigger; this matches the existing pattern for the SHA-mismatch branch directly below in the same function).

1b. Change `verifyChangelogHeaderRewritten` (around line 486) so that on `s.client.FetchChangelog` error it:
   - Sets `checks.ChangelogHeaderRewritten = false`.
   - Appends `CheckChangelogHeaderRewritten` to `*failedChecks`.
   - Logs at V(2) including the error.
   - Returns `nil` (same rationale as 1a).

1c. In `runStructuralChecks` (around lines 390-413), since 1a/1b now return `nil` on transport errors, the `glog.Warningf` lines at the call sites become unreachable. Remove the `if err := ...; err != nil { glog.Warningf(...) }` wrappers entirely — just call the two helpers directly. The function's existing top-level `if err := s.verifyTagExists(...); err != nil { ... return errors.Wrapf(...) }` block stays unchanged — that helper continues to return a real error for non-sentinel transport failure (controller-retry path), which is the intentional asymmetry between `verifyTagExists` (transport → retry) and the two helpers being fixed (transport → fail-closed record). Keep `verifyTagExists` exactly as it is.

1d. Update the godoc on both helpers to reflect the new contract:
   - `verifyTagAtExpectedCommit`: "On API/transport error: records `CheckTagAtExpectedSHA` as failed (sets check false + appends to failedChecks) and returns nil. The release trust model requires fail-closed on transient errors — a network blip must not leave the check passing."
   - `verifyChangelogHeaderRewritten`: same pattern, substituting `CheckChangelogHeaderRewritten`.

1e. Add Ginkgo specs to `agent/github-releaser/pkg/steps_ai_review_test.go` covering the new fail-closed paths. Use the existing `Describe("AIReviewStep", ...)` block and add a nested `Describe` or `Context` for "transport-error fail-closed":

   - Spec "verifyTagAtExpectedCommit transport error sets TagAtExpectedSHA=false and appends CheckTagAtExpectedSHA":
     - Build a fake `mocks.ReviewClient` where:
       - `TagExistsReturns("abc123", nil)` (so the tag-exists check passes and we proceed to the tag-commit check).
       - `ResolveTagCommitReturns("", errors.New("connection reset by peer"))`.
       - `FetchChangelogReturns(<bytes of a rewritten changelog starting with "## v1.0.0\n">, nil)` (so the third check passes — this isolates the fix to its branch).
     - Build the step via `pkg.NewAIReviewStep(fakeClient, claudeRunner, fakeGitOps, "")` matching the existing test pattern.
     - Provide a Markdown with the `## Result` `outcome: released` + `commit_sha: abc123` + a `## Plan` so `Run` proceeds past the short-circuit.
     - Invoke `Run`, parse `## Review` via `agentlib.ExtractSection[pkg.ReviewOutput]`.
     - Set `result.Workdir = ""` so `checkFaithfulness` short-circuits to `OverallUnknown` without invoking Claude.
     - Assert `output.Checks.TagAtExpectedSHA` is `false`.
     - Assert `output.FailedChecks` contains `pkg.CheckTagAtExpectedSHA` via `ContainElement`.
     - Assert `output.Approved` is `false`.
     - Do NOT assert on `output.Overall` (the empty-workdir branch overrides the `OverallFail` rollup with `OverallUnknown`).

   - Spec "verifyChangelogHeaderRewritten transport error sets ChangelogHeaderRewritten=false and appends CheckChangelogHeaderRewritten":
     - Same scaffolding but with `TagExistsReturns("abc123", nil)`, `ResolveTagCommitReturns("abc123", nil)` (matches expected commit so middle check passes), and `FetchChangelogReturns(nil, errors.New("timeout"))`.
     - Set `result.Workdir = ""` so `checkFaithfulness` short-circuits to `OverallUnknown` without invoking Claude.
     - Assert `output.Checks.ChangelogHeaderRewritten == false`.
     - Assert `output.FailedChecks` contains `pkg.CheckChangelogHeaderRewritten` via `ContainElement`.
     - Assert `output.Approved == false`.
     - Do NOT assert on `output.Overall` (same rationale as the prior spec).

## Fix 2 — Cover rewrite_needed=true execution branch (test gap)

In `agent/github-releaser/pkg/steps_execution_test.go`, add two Ginkgo specs inside the existing `Describe("ExecutionStep", ...)` block (or a new nested `Describe("rewrite_needed branch", ...)`):

2a. **Happy path:** Plan JSON includes `"rewrite_needed": true` AND `"rewritten_unreleased": "- clean entry one\n- clean entry two\n"`. Set up the `mocks.GitOps` fake:
   - `CloneStub` writes (via `os.WriteFile`) a `CHANGELOG.md` into the workdir with body:
     ```
     ## Unreleased

     - noisy original entry that gets rewritten
     - another noisy one

     ## v0.9.0

     - old release
     ```
   - `CommitStub` captures the workdir path and the commit path list, then reads `<workdir>/CHANGELOG.md` and stashes the bytes in a closure variable for assertion. Return a synthetic SHA like `"deadbee"` and `nil` error.
   - `TagReturns(nil)`.
   - `CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)`.
   - Build the markdown with frontmatter `task_identifier: rewrite-happy`, `clone_url: https://github.com/x/y.git`, `ref: main`, plus `## Plan` with the rewrite_needed/rewritten_unreleased fields and `next_version_header: ## v1.0.0`.
   - Invoke `Run`. Assert: result `Status == agentlib.AgentStatusDone`, `NextPhase == string(domain.TaskPhaseAIReview)`.
   - On the captured commit bytes: assert (a) `string(bytes)` contains `"## v1.0.0\n\n- clean entry one\n- clean entry two\n"` (verbatim rewritten body under the new header), (b) does NOT contain `"noisy original entry"`, (c) does NOT contain a `## Unreleased` line.

2b. **Error mapping:** Same `rewrite_needed: true` plan, but `CloneStub` writes a `CHANGELOG.md` with NO `## Unreleased` heading (e.g. only `## v0.9.0\n\n- old\n`). `changelog.ReplaceUnreleasedBody` returns a wrapped "unreleased header not found" error. Expected behavior per `steps_execution.go` lines ~229-240: `s.fail(ctx, md, git.ErrorCategoryUnreleasedNotFound, ...)`.
   - Invoke `Run`. Assert: result `Status == agentlib.AgentStatusFailed`.
   - Parse `## Result` via `agentlib.ExtractSection[pkg.ResultOutput]` and assert `result.Outcome == pkg.ResultOutcomeFailed` AND `result.ErrorCategory == git.ErrorCategoryUnreleasedNotFound`.
   - Assert `CommitStub` was NOT invoked (the failure occurs before commit) — use `fakeOps.CommitCallCount() == 0`.

## Fix 3 — Cover plugin-manifest branch in ai-review unexpected-file-change check (test gap)

FIRST, read `agent/github-releaser/pkg/plugin/detect.go` (or the file declaring `plugin.DetectManifests`) and determine whether the function CAN return a non-nil error. Inspect the return signature and every `return` statement. Decide one of two cases:

- **Case A: `DetectManifests` can return an error** (any code path returns a non-nil error). Write BOTH specs 3a and 3b below.
- **Case B: `DetectManifests` is infallible** (return type has no `error`, OR every return path returns `nil` for the error). Write ONLY spec 3a below; SKIP 3b entirely.

In the completion summary you MUST explicitly report which case applies, citing the file + line(s) you read, and which specs you wrote. No silent skips: even in Case B, the summary must include the line "DetectManifests is infallible — error-branch test not applicable" along with the file + line evidence.

3a. **Plugin manifest in expected set passes:** Use a real on-disk temp workdir created with `os.MkdirTemp("", "ai-review-test-")` and `DeferCleanup` for removal. Seed `<workdir>/.claude-plugin/plugin.json` with a minimal valid JSON object (verify the exact expected location by reading `agent/github-releaser/pkg/plugin/detect.go` — the existing execution test for plugin bumps already does this; reuse that fixture pattern). Also seed a `CHANGELOG.md` (any content) so the faithfulness branch can run or be short-circuited.
   - `mocks.GitOps.CommittedFilesReturns([]string{"CHANGELOG.md", ".claude-plugin/plugin.json"}, nil)`. (Match the exact relative-path format `plugin.DetectManifests` returns — grep `plugin.DetectManifests` callers and copy the format verbatim.)
   - Build a `## Result` with `outcome: released`, `workdir: <real tempdir>`, `commit_sha: abc`, `tag: v1.0.0`, `local_tag: v1.0.0`.
   - Build a `## Plan` so `Run` proceeds.
   - Stub `mocks.ReviewClient` so the three structural checks all pass (return matching shas, return a changelog body with a non-Unreleased first heading).
   - Stub `mocks.ClaudeRunnerMock` (existing fake from `pkg/steps_mocks.go`) to return a faithfulness response with `overall: pass` and no per-entry drift, OR alternatively read the existing test's helper that builds such a response (look for `mustJSON` usage near top of `steps_ai_review_test.go`).
   - Invoke `Run`. Assert `output.Checks.UnexpectedFileChange == false`, `output.UnexpectedFiles` is empty/nil, `output.FailedChecks` does NOT contain `pkg.CheckUnexpectedFileChange`.

3b. **DetectManifests error → falls back to changelog-only expected set:** ONLY include this spec in Case A above. Same scaffolding as 3a but seed `<workdir>/.claude-plugin/plugin.json` with INVALID JSON (e.g. literal bytes `not-json`) so `DetectManifests` returns an error. Then `CommittedFilesReturns([]string{"CHANGELOG.md", "plugin.json"}, nil)` (committed an unexpected `plugin.json`). The expected behavior per `steps_ai_review.go` lines 571-579 is: log the error, fall through with `expected = []string{changelogFileName}`. The extra committed `plugin.json` then triggers `UnexpectedFileChange = true`.
   - Assert `output.Checks.UnexpectedFileChange == true`.
   - Assert `output.FailedChecks` contains `pkg.CheckUnexpectedFileChange`.
   - Assert `output.UnexpectedFiles` contains `"plugin.json"`.

## Fix 4 — Move counterfeiter directive next to its interface (style)

In `agent/github-releaser/pkg/maintainerconfig/fetcher.go`, the directive on line 42 is separated from `type Fetcher interface` (line 52) by godoc lines 44-51. Project convention (per other `//counterfeiter:generate` directives in the codebase — see `pkg/git/git.go`, `pkg/githubreview/client.go`) is: directive sits directly above the `type` declaration with no blank line between them. Godoc goes ABOVE the directive.

Reshape the block to:

```go
// Fetcher reads .maintainer.yaml bytes from a remote GitHub repo at a ref.
// Implementations MUST be safe for concurrent use. Returned bytes are the
// raw decoded file contents (no base64, no JSON wrapper).
//
// HTTP 404 (file absent at the ref's tip) returns the sentinel ErrFileNotFound
// so callers can treat the absent-file case as a clean default-valued config
// (see spec 059 § Desired Behavior 6: missing .maintainer.yaml is treated as
// `changelogRewrite: false`).
//
//counterfeiter:generate -o ../../mocks/maintainer_config_fetcher.go --fake-name MaintainerConfigFetcher . Fetcher
type Fetcher interface {
    Fetch(ctx context.Context, owner, repo, ref string) ([]byte, error)
}
```

Do NOT regenerate the mock — the directive change is whitespace-only relative to the generator; existing `mocks/maintainer_config_fetcher.go` stays valid.

## Fix 5 — Drop package-level `aiReviewTools` and tighten `rollupVerdict` coverage

5a. In `agent/github-releaser/pkg/factory/factory.go`, the package-level `var aiReviewTools = claudelib.AllowedTools{}` (around line 99) is only used inside `CreateAgent`. Move it to a local variable inside `CreateAgent` next to where `executionOps` is declared. Delete the package-level declaration and its godoc comment. The local var keeps its existing one-line context comment (paraphrase: "AI-review LLM is read-only — same tool policy as planning").

5b. In `agent/github-releaser/pkg/steps_ai_review_test.go`, add a Ginkgo spec covering `rollupVerdict` when the LLM returns `OverallUnknown` AND a structural check ALSO fails. The function is unexported but called from `Run`; drive it through `Run` — DO NOT add an export_test.go just for this.
   - Stub the `mocks.ReviewClient` so `TagExistsReturns("", githubreview.ErrTagNotFound)` (tag missing → `CheckTagExists` recorded as failed).
   - Stub the `mocks.ClaudeRunnerMock.RunReturns(nil, errors.New("claude unavailable"))` (faithfulness path returns `OverallUnknown` AND records `CheckFaithfulness`).
   - Invoke `Run`. Parse `## Review`.
   - Assert `output.Overall == pkg.OverallUnknown` (the LLM-unknown override per `rollupVerdict` last branch).
   - Assert `output.Approved == false`.
   - Assert `output.FailedChecks` contains BOTH `pkg.CheckTagExists` AND `pkg.CheckFaithfulness` (use `ContainElements`).

## Fix 6 — Test empty-body 200 in fetcher (test gap)

In `agent/github-releaser/pkg/maintainerconfig/fetcher_test.go`, add a Ginkgo spec inside the existing `Describe("httpFetcher", ...)` block:

```go
It("empty 200 OK body with no encoding field rejected", func() {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{}`))
    }))
    defer server.Close()

    fetcher := maintainerconfig.NewHTTPFetcherForTest("", server.URL)
    _, err := fetcher.Fetch(ctx, "bborbe", "maintainer", "master")
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring(`unsupported encoding ""`))
})
```

Verify the substring `unsupported encoding ""` matches the actual error format produced by `decodeContent` (line 227-231 of `fetcher.go`) — the format string is `"fetch .maintainer.yaml: unsupported encoding %q (want base64)"` and an empty string formatted with `%q` is `""`, so the substring above is correct.

## Fix 7 — Replace raw `"Push"` with `CheckPush` constant

In `agent/github-releaser/pkg/steps_ai_review.go`:
- Add `CheckPush = "Push"` to the existing `const ( CheckTagExists = ... )` block (around lines 50-56), with a one-line godoc matching the other check constants.
- At line 300 inside `finishApproved`, replace `output.FailedChecks = append(output.FailedChecks, "Push")` with `output.FailedChecks = append(output.FailedChecks, CheckPush)`.
- Grep the codebase for any tests asserting on the literal string `"Push"` in `FailedChecks` (`grep -rn '"Push"' agent/github-releaser/`). If any exist, leave the literal in the test (the test still passes because `CheckPush == "Push"`); no test changes required.

## Fix 8 — Clarify `setupWorkdir` godoc

In `agent/github-releaser/pkg/steps_execution.go` around line 173-180, the function `setupWorkdir` builds the workdir path and removes any stale copy but does NOT create the directory (the subsequent `Clone` call creates it). Update the godoc to:

```go
// setupWorkdir returns the canonical workdir path for the given task ID
// and removes any stale copy from a prior run. Does NOT create the
// directory — the subsequent ops.Clone call creates it. Stale-removal
// failure is logged at Warning level and the path is returned anyway
// (Clone will then fail with a more actionable error).
```

Do NOT rename the function (renaming touches every caller and the test fixtures; the godoc clarification is the lower-risk fix the user asked for).

## Fix 9 — Promote 15-second timeout to named constant

In `agent/github-releaser/pkg/maintainerconfig/fetcher.go`:
- Add a package-level constant near the top (after the type re-exports and `Parse` alias, before the counterfeiter directive):
  ```go
  // fetchTimeout caps the GitHub contents-API call. Set high enough to
  // survive typical transient latency, low enough to fail the planning
  // step within the controller's per-step budget.
  const fetchTimeout = 15 * time.Second
  ```
- Replace both `15 * time.Second` literals (in `NewHTTPFetcher` line 73 and `newHTTPFetcherWithBase` line 83) with `fetchTimeout`.
- No test changes required — the timeout is not directly asserted.

## Out of scope (do NOT change)

- The `verifyTagExists` helper — its existing transport-error → retry path is intentional and the user did not flag it.
- The `lib/maintainerconfig/` Go package itself — fix 4 (counterfeiter directive) is in `agent/github-releaser/pkg/maintainerconfig/`, NOT `lib/maintainerconfig/`. Re-read the file paths above if unsure.
- Any change to the rollupVerdict ordering or override semantics — fix 5b is a TEST addition only.
- Regeneration of counterfeiter mocks — fix 4 is a comment-block move only; no mock regen.

</requirements>

<constraints>
- All write edits must be inside `agent/github-releaser/`. Do NOT modify any file under `lib/` (including `lib/maintainerconfig/`) — there are no edits required there; the `lib/` precommit run below is a safety check, not a write location.
- No changes elsewhere (no watcher edits, no top-level config changes, no `agent/pr-reviewer/` edits).
- Existing tests must continue to pass.
- Do NOT commit — dark-factory handles git.
- Use `bborbe/errors` for error wrapping (`errors.Wrapf`, `errors.Errorf`); never stdlib `fmt.Errorf` for new wrapping.
- Use Ginkgo v2 + Gomega for any new test. Match existing file style (table-driven where the existing block uses it, individual `It(...)` blocks otherwise).
- Use counterfeiter fakes from `agent/github-releaser/mocks/` for `GitOps`, `ReviewClient`, `ClaudeRunner`, `Fetcher`. Do NOT hand-roll fakes.
- For any new test that needs the workdir on disk, use `os.MkdirTemp` + `DeferCleanup(func() { _ = os.RemoveAll(dir) })` — mirror existing patterns in `steps_execution_test.go`.
- Do NOT regenerate counterfeiter mocks for fix 4 (comment-only move).
- Do NOT modify the bborbe/errors import path or any go.mod.
- All public-API godoc additions must follow `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md`.
</constraints>

<verification>
Run BOTH precommit suites — both must exit 0:

```
cd agent/github-releaser && make precommit
cd lib && make precommit
```

After both pass, report a completion summary noting:
1. The `verifyTagAtExpectedCommit` and `verifyChangelogHeaderRewritten` transport-error paths now fail closed (boolean false + appended to `failedChecks`), with the specific assertion lines in `steps_ai_review_test.go` that prove it.
2. The new test cases added, grouped by fix number, with the Ginkgo file path and the `It("...")` description of each.
3. For Fix 3 specifically: which case (A or B) applied for `plugin.DetectManifests`, the file + line(s) inspected, and which of 3a / 3b were written. If Case B, include the exact phrase "DetectManifests is infallible — error-branch test not applicable".
4. Precommit pass evidence (the final line of each `make precommit` run, or the exit-zero confirmation).
</verification>
