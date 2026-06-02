---
status: completed
spec: [058-changelog-rewrite-flow]
summary: 'Moved git push out of execution step: renamed executeDirectPush to executeLocalRelease, dropped ops.Push, added changelog.ReplaceUnreleasedBody helper, applied plan.RewrittenUnreleased body in a single commit (when plan.RewriteNeeded), persisted workdir+local tag in ResultOutput for ai-review, all 94 Ginkgo specs pass with make precommit exit 0'
container: maintainer-changelog-rewrite-exec-228-spec-058-execution-no-push
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T16:30:08Z"
queued: "2026-06-02T16:43:50Z"
started: "2026-06-02T17:57:39Z"
completed: "2026-06-02T18:10:23Z"
---

<summary>
- Execution phase applies the planning-time rewrite (when one was requested) before renaming the `## Unreleased` header — both changes land in a single commit that touches only `CHANGELOG.md`.
- Execution creates a local annotated tag pointing at that commit.
- Execution NO LONGER PUSHES — the push is moved out of this phase entirely and is gated by ai-review in the next prompt.
- The agent's workdir clone keeps the local commit and local tag after execution returns, so ai-review can read the on-disk state.
- Re-firing execution against the same `## Plan` is idempotent: same input → same single commit, same tag, no duplicates.
- Already-clean changelogs (`rewrite_needed=false`) still get the header rename with no body changes — same single commit, same local tag, still no push.
- The pre-push diff guard that asserts only `CHANGELOG.md` changed stays in place as the belt to ai-review's braces — it just now fires at ai-review-pass time, not inside execution.
</summary>

<objective>
Move the push out of the execution step and tighten execution to: apply rewrite (when planned) + rename header + commit + local-tag, then return. The local clone, commit, and tag must survive past the step's return so the (later) ai-review step can read them. This prompt does NOT add the semantic ai-review — that is the next prompt. This prompt only delivers the execution slice and the surface ai-review will read.
</objective>

<context>
Read `~/Documents/workspaces/maintainer-changelog-rewrite/CLAUDE.md` and `agent/github-releaser/CLAUDE.md`.

Read these files BEFORE editing:
- `agent/github-releaser/pkg/steps_execution.go` — current execution step; the function to change is `executeDirectPush`. Read every requirement step against what is already there.
- `agent/github-releaser/pkg/steps_execution_test.go` — current Ginkgo test fixtures, including `taskMD` constant and `writeChangelog` helper.
- `agent/github-releaser/pkg/git/git.go` — `GitOps` interface: `Clone`, `Commit`, `Tag`, `Push`, `CommittedFiles`. Do NOT change the interface shape.
- `agent/github-releaser/pkg/changelog/changelog.go` — `RewriteUnreleasedHeader` (will continue to be used).
- `agent/github-releaser/pkg/plan_output.go` — should already contain `OriginalUnreleased`, `RewriteNeeded`, `RewrittenUnreleased` fields after prompt 1 lands.
- `agent/github-releaser/pkg/result_output.go` — `ResultOutput` struct; you will ADD fields (do not remove any).
- `agent/github-releaser/pkg/factory/factory.go` — `CreateAgent` wiring; you do NOT need to change it (the constructor signature stays the same), but verify by reading.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` (counterfeiter `GitOps` mock pattern)

Verified symbols:
- `agentlib.Result{Status, NextPhase, Message}`; `agentlib.AgentStatusDone = "done"`, `agentlib.AgentStatusFailed = "failed"` (from prior reads of `steps_execution.go`).
- `domain.TaskPhaseAIReview = "ai_review"` from `github.com/bborbe/vault-cli@v0.67.5/pkg/domain/task_phase.go`.
- `git.GitOps` interface — `Push` method exists today; it MUST NOT be called from `executeDirectPush` after this prompt.
- `gitmocks.GitOps` counterfeiter mock — assertions like `fakeOps.PushCallCount()`, `fakeOps.TagCallCount()`, `fakeOps.CommittedFilesReturns(...)` are available (per existing test file).
</context>

<requirements>

1. **Rename the inner method (clarity).** In `agent/github-releaser/pkg/steps_execution.go`, rename `executeDirectPush` to `executeLocalRelease` (and its single call site inside `Run`). The name "direct push" is misleading now that the push has moved out. Keep `ResultPathDirectPush = "direct-push"` for backward compat with persisted task pages — only the in-code method name changes.

2. **Apply the rewrite when planning asked for one.** Inside `executeLocalRelease`, BEFORE the existing `changelog.RewriteUnreleasedHeader` call, add a conditional rewrite of the `## Unreleased` body:

   - When `plan.RewriteNeeded == true`:
     1. Use a new pure helper `changelog.ReplaceUnreleasedBody(ctx, content, plan.RewrittenUnreleased) ([]byte, error)` (add it to `pkg/changelog/changelog.go`). It replaces the body of the `## Unreleased` section (every line after `## Unreleased` up to but not including the next `## ` heading or EOF) with `plan.RewrittenUnreleased`. Line endings normalized to `\n`. The body inserted is exactly the supplied string; if it does not end with `\n`, append one before the next heading.
     2. Returns a wrapped error if `## Unreleased` is not present (mapped to `git.ErrorCategoryUnreleasedNotFound`, same as the existing rewrite path).

   - When `plan.RewriteNeeded == false`: skip the body replacement entirely; proceed straight to the existing `RewriteUnreleasedHeader` call.

   Then the existing `RewriteUnreleasedHeader` call runs unchanged on the (possibly body-rewritten) content, renaming the header to `plan.NextVersionHeader`. Final write to `changelogPath` with the existing `os.WriteFile(..., 0o644)`.

   Add focused table tests for `ReplaceUnreleasedBody` in `pkg/changelog/changelog_test.go` covering: (a) typical replacement preserves text before and after; (b) empty new body produces an empty section block; (c) no `## Unreleased` heading → error.

3. **Single commit guarantee.** Do NOT split rewrite + header rename into two commits. The existing pattern (read → rewrite → write → single `ops.Commit`) is correct; you are only inserting the optional body-replacement step into the in-memory transform pipeline before `RewriteUnreleasedHeader`. The pre-commit guard `guardCommittedFiles` continues to assert that the HEAD commit touched only `expectedFiles` (which is `[]string{"CHANGELOG.md"}` plus any detected plugin manifests — UNCHANGED from today).

4. **Local tag, NO PUSH.** After the existing `s.ops.Tag(ctx, workdir, tagName, "release "+tagName)` call succeeds, **delete the `s.ops.Push(ctx, workdir, "HEAD", "refs/tags/"+tagName)` call and its error handling**. `executeLocalRelease` now returns `(sha, tagName, nil)` immediately after Tag succeeds.

5. **Do NOT remove the workdir on the happy path.** This is the load-bearing change for ai-review-after-execution. In `Run`, the existing `defer func() { os.RemoveAll(workdir) }()` MUST be replaced with a controlled-lifetime model:

   - On any **failure path** (`s.fail(...)` returned), DO remove the workdir.
   - On the **happy path**, the workdir MUST persist past the step's return — ai-review (next prompt) needs to read `git log -1 --name-only` and (more importantly, in the future) compare `## Unreleased` body bytes against `## Plan.OriginalUnreleased`.

   Implementation: drop the unconditional `defer os.RemoveAll`. Instead, track success in a local `var releaseSuccess bool` and use `defer func() { if !releaseSuccess { os.RemoveAll(workdir) } }()`. Set `releaseSuccess = true` immediately before the final `&agentlib.Result{Status: AgentStatusDone, NextPhase: TaskPhaseAIReview}` return.

   Persist the workdir path into `## Result` so ai-review can find the clone — extend `ResultOutput`:

   ```go
   // Workdir is the absolute path of the local clone created by execution.
   // ai-review reads CHANGELOG.md and runs `git log -1 --name-only` against
   // this path. Empty on the failure path (no clone survives).
   Workdir string `json:"workdir,omitempty"`

   // LocalTag is the annotated tag created in the local clone. ai-review
   // checks the tag exists and points at CommitSHA before pushing. Empty
   // on the failure path.
   LocalTag string `json:"local_tag,omitempty"`
   ```

   On the success path, populate `output.Workdir = workdir` and `output.LocalTag = tagName` in addition to the existing `CommitSHA` and `Tag` fields. (Keep `Tag` populated to `tagName` for back-compat with persisted task pages; `LocalTag` is the new explicit "this lives only locally for now" field.)

   **Workdir lifetime ownership.** This prompt CREATES `result.Workdir`; this prompt does NOT remove it on success. The ai-review step (prompt 3) is the LIFETIME OWNER on terminal transitions: prompt 3 MUST `os.RemoveAll(result.Workdir)` after a successful Push (`Approved=true` + push succeeded → Done) AND on the `human_review` terminal exit. See prompt 3 Req 6 (workdir cleanup after final verdict). Failure paths inside THIS prompt still remove the workdir locally (the `!releaseSuccess` defer above).

6. **Idempotent re-fire.** A second invocation of `Run` against the same `## Plan` MUST produce exactly one new commit ahead of `origin/master` and exactly one tag named `vX.Y.Z` on the local clone. Today the workdir prefix is `github-releaser-<task_identifier>`; `setupWorkdir` already does `os.RemoveAll(workdir)` before clone, so a stale workdir from a prior run is wiped. KEEP this behavior — it is what makes re-fire idempotent. Verify by reading `setupWorkdir` and confirming the existing `os.RemoveAll` line is preserved.

   Add a Ginkgo `It("re-fire produces no duplicate commit and no duplicate tag")` in `pkg/steps_execution_test.go`:
   - First invocation: standard happy-path stubs (Clone writes a CHANGELOG; Commit returns "abc1234"; Tag returns nil; CommittedFiles returns `[]string{"CHANGELOG.md"}`).
   - Second invocation against the same `taskMD`: assert `fakeOps.CloneCallCount() == 2`, `fakeOps.CommitCallCount() == 2`, `fakeOps.TagCallCount() == 2`, and crucially `fakeOps.PushCallCount() == 0`. The contract "no duplicate" is enforced at the local-filesystem layer by `setupWorkdir`'s `RemoveAll` — the test asserts the mock counts to prove the re-fire path is exercised end-to-end without invoking Push.

7. **Existing Ginkgo specs to update.** In `pkg/steps_execution_test.go`:

   a. The existing happy-path spec ("clones, rewrites, commits, tags, pushes; writes ## Result(released); returns Done/NextPhase=ai_review") must drop the "pushes" assertion. Rename the `It` to "clones, rewrites, commits, tags (no push); writes ## Result(released); returns Done/NextPhase=ai_review". Replace `fakeOps.PushReturns(nil)` with an explicit assertion at the end of the `It`: `Expect(fakeOps.PushCallCount()).To(Equal(0))`.

   b. Any existing spec that exercises `Push` failure modes (search for `PushReturns(errors.` or `PushStub`) must be DELETED (push has moved out of this step). Each deletion is fine — the same failure surface will be re-tested in the ai-review push-gating prompt (next prompt). Leave a `// MOVED: push failure tests now live in the ai-review push-gating spec (spec 058 prompt 3).` comment where the deletions happened.

   c. Add `It("rewrite_needed=true: ## Unreleased body is replaced before header rename")`:
      - `taskMD` `## Plan` has `"rewrite_needed": true` and `"rewritten_unreleased": "- feat: cleaned\n"`.
      - Clone stub writes a CHANGELOG whose `## Unreleased` body contains noisy content (e.g. `- raw commit line one\n- raw commit line two\n`).
      - Commit stub reads `CHANGELOG.md` at the workdir and asserts the on-disk bytes contain `- feat: cleaned` AND do NOT contain `raw commit line one` AND do NOT contain `## Unreleased` (header was renamed) AND DO contain `## v1.2.8`.
      - Assert `fakeOps.CommitCallCount() == 1` (single commit covering both body and header).

   d. Add `It("rewrite_needed=false: ## Unreleased body is preserved, only header is renamed")`:
      - `taskMD` `## Plan` has `"rewrite_needed": false` and `"rewritten_unreleased": ""`.
      - Clone stub writes a CHANGELOG with `## Unreleased\n\n- feat: original\n`.
      - Commit stub asserts on-disk bytes contain `- feat: original` AND do NOT contain `## Unreleased` AND DO contain `## v1.2.8`.

   e. Add `It("does NOT push and workdir survives execution return")`:
      - Happy-path stubs as in (a).
      - After `step.Run` returns, assert:
        - `Expect(fakeOps.PushCallCount()).To(Equal(0))`
        - The `## Result` section parsed from `md` has `result.Workdir != ""` AND `_, statErr := os.Stat(result.Workdir)` returns nil (directory still exists).
        - `result.LocalTag == "v1.2.8"`.
      - Cleanup uses Ginkgo `DeferCleanup` so it fires even if an assertion above fails:
        ```go
        DeferCleanup(func() { _ = os.RemoveAll(result.Workdir) })
        ```
        Place the `DeferCleanup` registration IMMEDIATELY after parsing `result.Workdir` out of `## Result` (before the assertions), so the cleanup is registered regardless of subsequent failures. Do NOT use a trailing `os.RemoveAll(result.Workdir)` — it is skipped on assertion failure and leaks tmpdirs in CI.

   f. **Boundary round-trip for `Workdir` + `LocalTag` JSON tags.** Add `It("ResultOutput round-trip preserves Workdir and LocalTag tags + omitempty")` in `pkg/result_output_test.go` (create the file if absent; mirror the style of the existing prompts_test fixtures):
      - Marshal a `ResultOutput{Workdir: "/tmp/x", LocalTag: "v1.2.8", CommitSHA: "abc", Tag: "v1.2.8", Outcome: ResultOutcomeReleased}` to JSON. Assert the JSON literally contains `"workdir":"/tmp/x"` and `"local_tag":"v1.2.8"` (field-name stability under `## Result` serialization).
      - Marshal a `ResultOutput{}` (both fields zero). Assert the JSON does NOT contain the substrings `"workdir"` or `"local_tag"` (proves `omitempty` fires for zero values).
      - Round-trip: unmarshal the first JSON back into a fresh `ResultOutput` and assert `Workdir`, `LocalTag`, `CommitSHA`, `Tag`, `Outcome` all match the original.
      - This test traverses the JSON encoder boundary the `## Result` section serialization crosses — protects against silent field-tag drift.

8. **Wiring unchanged.** `factory.CreateAgent` keeps the same signature; `NewExecutionStep(ops git.GitOps, ghToken string) agentlib.Step` keeps the same signature. The execution step continues to return `NextPhase: string(domain.TaskPhaseAIReview)` on success. Do NOT add a new step constructor; do NOT add new factory entries.

9. **Acceptance gate — `make precommit` exits 0 in `agent/github-releaser`.** Investigate and fix any failures. Counterfeiter mocks should NOT need regeneration — the `GitOps` interface is unchanged.

</requirements>

<constraints>
- The pre-push diff guard `guardCommittedFiles` MUST continue to fire on every release commit (it asserts the commit touched only `CHANGELOG.md` plus detected plugin manifests). Do not weaken it.
- The 3-phase task lifecycle is frozen — execution still advances to `ai_review` on success.
- Single commit per release — rewrite + rename must not split into two commits. The integration test must assert `CommitCallCount() == 1` on the happy path.
- The agent's workdir clone is the only place the local-but-not-yet-pushed commit + tag live; cleanup-on-exit must NOT delete it on the success path before ai-review has run.
- Do NOT change the structural ai-review checks (`TagExists`, `TagAtExpectedSHA`, `ChangelogHeaderRewritten`) — that is the next prompt's scope.
- Do NOT change the single-CHANGELOG assumption (no mono-repo handling).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass (after the targeted updates listed above).
</constraints>

<verification>
```
cd ~/Documents/workspaces/maintainer-changelog-rewrite/agent/github-releaser
make precommit
```

Expected: exit code 0; all Ginkgo specs pass.

Evidence commands the auditor will run:
- `grep -n 'ops.Push\|s.ops.Push' agent/github-releaser/pkg/steps_execution.go` → returns NOTHING (no Push call in the execution step).
- `grep -n 'Workdir\|LocalTag' agent/github-releaser/pkg/result_output.go` → both new fields present with documented JSON tags.
- `ginkgo --v ./pkg | grep -E 'no push|workdir survives|re-fire|body is replaced|body is preserved'` → all five new It descriptions appear and pass.
</verification>
