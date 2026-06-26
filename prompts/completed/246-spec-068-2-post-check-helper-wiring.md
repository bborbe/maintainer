---
status: completed
spec: [068-cqrs-trigger-github-build]
summary: 'Wired spec-064 post-check tail into the github-releaser execution step: added ResolutionOutput contract, postCheck helper (idempotent, LsRemote-driven, released/superseded verdict upgrade), widened s.fail signature across all 16 call sites, and 7 new Ginkgo tests cover all branches'
container: maintainer-release-postcheck-exec-246-spec-064-2-post-check-helper-wiring
dark-factory-version: v0.175.0
created: "2026-06-08T16:30:00Z"
queued: "2026-06-08T18:48:42Z"
started: "2026-06-09T06:32:43Z"
completed: "2026-06-09T06:54:11Z"
branch: dark-factory/github-releaser-post-check-verdict-from-github-truth
---

<summary>
- The github-releaser execution step gains a post-check helper that, after the existing `## Result` would be written on either the success-return path or any failure-return path, calls the new `git.GitOps.LsRemote` to ask the remote what commit SHA, if any, sits at the planned version's tag — and uses that answer to decide the final verdict
- When the remote shows the tag at the SHA the agent just produced, the verdict is upgraded to `released` with a `## Resolution` block naming the planned version and the observed SHA; the frontmatter is rewritten to `status: completed`, `phase: done`
- When the remote shows the tag at a different SHA, the verdict is upgraded to `superseded` with a `## Resolution` block citing the planned version and the observed SHA; the frontmatter is rewritten to `status: completed`, `phase: done` (a later release won the slot)
- When the remote shows no tag, the post-check is a no-op: the existing success-path or failure-path verdict the execution step would have written stands unchanged
- When the remote query itself errors, the post-check is a no-op: the existing verdict stands; the error is logged with `redactToken` applied; the post-check never downgrades a verdict on its own failure
- The helper is idempotent: when invoked against a task whose `status` is already `completed` or `aborted` (read from frontmatter at the helper's entry), it returns immediately without re-querying the remote, without re-writing frontmatter, and without re-appending the `## Resolution` block
- The post-check runs on BOTH the success-return path (where the execution step would have written `## Result(outcome=released)` at the existing call site near the bottom of `Run` in `steps_execution.go`) AND every failure-return path (every `s.fail` call site — currently **15** of them in the file; **verify with `grep -n 's\.fail(' agent/github-releaser/pkg/steps_execution.go` before threading parameters, since the count may shift as the file evolves**). The exact wiring shape — whether by widening `s.fail`'s signature, by wrapping `s.fail`, or by inserting a post-check pass at `Run`'s tail — is left to the executor as long as all current call sites participate
- The verdict change is internal to the agent. No Kafka envelope, no command schema, no agent-lib API changes. No new `CompleteCommand` / `CloseCommand` agent-lib commands. The verdict change rides on the existing `agentlib.Markdown.Frontmatter` map (which is a `map[string]interface{}` per `agent-lib@v0.65.0/agent_task-frontmatter.go:16`) — set `md.Frontmatter["status"] = "completed"` and `md.Frontmatter["phase"] = "done"`.
- Every post-check invocation emits exactly one structured log line via `glog.V(2)` naming `task_identifier`, planned version, observed remote SHA (or empty), and verdict from the set `{released, superseded, no-op-remote-empty, no-op-remote-error, no-op-already-terminal}` so operators can grep the deciding fact for any task in agent logs
- The factory wiring is unchanged — `NewExecutionStep(ops, ghToken)` still takes the same `git.GitOps` interface, the new `LsRemote` method is part of the same interface. The execution step signature does not change; the post-check logic is internal.
- New Ginkgo integration tests in `pkg/steps_execution_test.go` cover: (a) success path with `LsRemote` returning the expected SHA → `status: completed` / `phase: done` / `## Resolution` block with verdict `released`; (b) success path with `LsRemote` returning a different SHA → `status: completed` / `phase: done` / `## Resolution` block with verdict `superseded`; (c) success path with `LsRemote` returning empty → existing `## Result(outcome=released)` stands unchanged; (d) success path with `LsRemote` returning an error → existing verdict stands; (e) each `s.fail` call site participating in the post-check on its branch's verdict set; (f) re-invocation on an already-`completed` task produces no observable change (byte-equal task file before and after the second helper run).
- The repo-root `CHANGELOG.md` is updated under `## Unreleased` with a `feat(agent/github-releaser):` entry describing the post-check behavior.
- All existing `agent/github-releaser/pkg/...` Ginkgo tests continue to pass.

</summary>

<objective>
Wire the post-check helper into the github-releaser execution step so the agent's terminal verdict reflects GitHub's view of the release-tag slot, not the agent's local belief. A successful `git ls-remote` against the planned-version tag upgrades an existing `## Result(outcome=released)` to `released` + a `## Resolution` block (when the remote SHA matches what the agent produced) or `superseded` (when a later release won the slot at a different SHA), and an empty result or a subcommand error leaves the existing verdict unchanged. The helper participates in the success-return path AND every `s.fail` call site, has an idempotency guard at its entry, and emits one structured log line per invocation.

</objective>

<context>
Read `/workspace/agent/github-releaser/pkg/steps_execution.go` end-to-end. The `executionStep` struct at line 42 has two fields: `ops git.GitOps` and `ghToken string`. The `Run` method at line 82 is the entry point. The success-return path is at line 119-131 (marshal `## Result(outcome=released)`, `md.ReplaceSection`, return `agentlib.AgentStatusDone` / `NextPhase=ai_review`). The `s.fail` method at line 396 is the failure-return helper; it is called from **15 sites** in this file (verified count): lines 85, 90, 204, 211, 225, 240, 252, 257, 270, 281, 286, 292, 303, 317, 336, 341. These break down as: 2 direct sites in `Run` itself (85, 90), 12 sites inside `executeLocalRelease` (204 through 317), and 2 sites inside `guardCommittedFiles` (336 AND 341 — both must participate). **Before threading parameters, re-grep to confirm the count and line numbers** (`grep -n 's\.fail(' agent/github-releaser/pkg/steps_execution.go`). The `executeLocalRelease` helper at line 194 returns `(sha, tagName, failResult)` and ALREADY invokes `s.fail` internally on each error path before returning the result — the post-check must therefore run on the result returned by `executeLocalRelease` AS WELL AS on every direct `s.fail` call site in `Run` itself.

The `plan *PlanOutput` is already extracted at the top of `Run` (line 83). It carries `plan.NextVersion` (the bare semver, e.g. `1.2.8`) and `plan.NextVersionHeader` (e.g. `## v1.2.8`). The `tagName` (e.g. `v1.2.8`) is the string used for the `git ls-remote refs/tags/<tag>` query — derive it once via `strings.TrimPrefix(plan.NextVersionHeader, "## ")` (line 298 already does this in `executeLocalRelease`; the post-check can re-derive or accept it as a parameter).

The `taskID` is in `md.Frontmatter["task_identifier"]` (already extracted at line 162 via `md.Frontmatter.String("task_identifier")`). The `cloneURL` is in `md.Frontmatter["clone_url"]` (line 160); the post-check uses the SAME URL the execution step already authed (the `injectToken` model from line 382). For the post-check, you need the authed URL: the existing `executeLocalRelease` already does `authedURL := s.injectToken(normalizeCloneURLToHTTPS(cloneURL))` and then calls `s.ops.Clone(ctx, authedURL, ref, workdir)`. Mirror that: `authedURL := s.injectToken(normalizeCloneURLToHTTPS(cloneURL))` followed by `s.ops.LsRemote(ctx, authedURL, ref, tag)`. The token-injection model is reused at the call site — the post-check does not need to know about tokens.

The `*agentlib.Markdown` (parameter `md`) carries the frontmatter and the body sections. Per `agent-lib@v0.65.0/agent_task-frontmatter.go:16`, `TaskFrontmatter` is `type TaskFrontmatter map[string]interface{}` — so `md.Frontmatter["status"] = "completed"` and `md.Frontmatter["phase"] = "done"` are direct map writes. The body is modified via `md.ReplaceSection(section)` where `section` is an `agentlib.Section` produced by `agentlib.MarshalSectionTyped(ctx, "## Resolution", resolutionOutput)`.

Read `/workspace/agent/github-releaser/pkg/result_output.go` — the existing `ResultOutput` type at line 18 is the typed contract for `## Result`. The post-check does NOT need to extend this struct — the upgrade replaces `## Result` on disk, and `## Resolution` is a NEW section appended after `## Result`. Keep the `ResultOutput` struct unchanged.

Read `/workspace/agent/github-releaser/pkg/steps_execution_test.go` to see the existing Ginkgo test scaffolding (counterfeiter `gitmocks.GitOps`, the `taskMD` constant at line 56, the `writeChangelog` helper at line 86, the `runGuard` closure pattern at line 186, and the integration assertions on the `## Result` section via `agentlib.ExtractSection[pkg.ResultOutput]`). The new post-check tests follow the same shape.

Read `/workspace/agent/github-releaser/pkg/plan_output.go` — `PlanOutput` is the typed contract for `## Plan`. The post-check needs `plan.NextVersion` (or `plan.NextVersionHeader` from which the tag name is derived) to log the planned version and to build the `refs/tags/<tag>` query.

Read `/workspace/agent/github-releaser/pkg/export_test.go` — exposes test hooks (`SameStringSetForTest`, `DeriveUnprefixedVersionForTest`, etc.). If the new helper is unexported and the external `_test` package needs to drive it, add an export hook here.

Read `/workspace/agent/github-releaser/pkg/factory/factory.go` — `NewExecutionStep(ops, ghToken)` is the constructor (line 51 in steps_execution.go, called from line 124 in factory.go). The factory does NOT need to change — the new `LsRemote` method is part of the same `git.GitOps` interface, and the execution step's signature stays the same.

Read `/workspace/agent/github-releaser/pkg/git/os_exec_git_ops.go` — the new `LsRemote` method from prompt 1. Its signature is `LsRemote(ctx context.Context, cloneURL, ref, tag string) (sha string, err error)`. The counterfeiter mock at `mocks/git_ops.go` (regenerated in prompt 1) provides `LsRemoteStub`, `LsRemoteReturns(sha, err)`, `LsRemoteCallCount()`, `LsRemoteArgsForCall(i)` — use these in tests.

The repo-root `/workspace/CHANGELOG.md` uses the `feat:` prefix style. The new entry for this prompt belongs under `## Unreleased` (the most recent change goes first, matching the reverse-chronological shape).

</context>

<requirements>

The work is split into two cooperating changes:
(A) Define the `## Resolution` typed contract + the `postCheck` helper method on `*executionStep`.
(B) Wire the helper into the success-return path AND every `s.fail` call site.

Both halves ship in this prompt.

---

### Part A — the `## Resolution` contract and the post-check helper

1. **Add a `ResolutionOutput` typed struct** in a new file `agent/github-releaser/pkg/resolution_output.go` (new file, package `pkg`). Round-trips with `agentlib.MarshalSectionTyped` + `agentlib.ExtractSection`. Shape:

   ```go
   // ResolutionOutput is the typed contract for the `## Resolution` JSON
   // section the post-check helper appends when it upgrades a verdict
   // from local-failed to remote-confirmed (released or superseded).
   //
   // Two shapes are valid:
   //   - Verdict="released"     — remote shows the planned tag at the
   //                              SHA the agent just produced
   //   - Verdict="superseded"   — remote shows the planned tag at a
   //                              different SHA; a later release won
   //                              the slot
   //
   // Both shapes populate PlannedVersion, ObservedRemoteSHA, Note.
   type ResolutionOutput struct {
       Verdict         string `json:"verdict"`
       PlannedVersion  string `json:"planned_version"`
       ObservedRemoteSHA string `json:"observed_remote_sha"`
   }

   // Verdict values for ResolutionOutput.Verdict.
   const (
       ResolutionVerdictReleased   = "released"
       ResolutionVerdictSuperseded = "superseded"
   )
   ```

   Add the `verdict ∈ {released, superseded}` `Validate` method if the rest of the codebase uses one for these typed contracts (search for `Validate(` on similar types — if none exist, skip).

2. **Add the post-check helper** as a new method on `*executionStep` in `agent/github-releaser/pkg/steps_execution.go` (add at the bottom of the file, below `sameStringSet`):

   ```go
   // postCheck runs the spec-064 post-check. After the execution step
   // has written ## Result (success or failure), this method:
   //  1. Reads md.Frontmatter["status"] — if it's already "completed"
   //     or "aborted", return immediately (idempotency guard).
   //  2. Asks the remote via ops.LsRemote(authedURL, ref, tag) what
   //     SHA, if any, sits at refs/tags/<tag> for the planned version.
   //  3. Compares the observed SHA against the agent's expected SHA
   //     (result.CommitSHA from the just-written ## Result) when
   //     available. On the failure path the expected SHA is empty —
   //     any non-empty observed SHA still fires the superseded
   //     branch (a later release won the slot).
   //  4. Upgrades the verdict: writes a ## Resolution block, rewrites
   //     md.Frontmatter["status"]="completed" and ["phase"]="done",
   //     and emits one structured log line via glog.V(2).
   //
   // On empty remote result or LsRemote error: the existing ## Result
   // is NOT touched. The post-check is a no-op (other than the log line).
   //
   // The structured log line format (glog.V(2), one line per call):
   //   post-check: task_id=<taskID> planned_version=<tag> observed_remote_sha=<sha-or-empty> verdict=<verdict>
   // where <verdict> ∈ {released, superseded, no-op-remote-empty, no-op-remote-error, no-op-already-terminal}.
   func (s *executionStep) postCheck(
       ctx context.Context,
       md *agentlib.Markdown,
       taskID string,
       tag string,
       authedURL string,
       ref string,
       expectedSHA string,
   )
   ```

   Implementation notes (the executor must follow these):

   - The idempotency guard is the FIRST statement: `if status, _ := md.Frontmatter.String("status"); status == "completed" || status == "aborted" { glog.V(2).Infof(...verdict=no-op-already-terminal...); return }`. Use `String` to handle the `interface{}` cast safely (returns `"", false` if the key is absent or non-string — treat both as not-yet-terminal).
   - Authed URL: accept the already-authed URL as a parameter and trust the caller. The success path in `Run` builds it once with `s.injectToken(normalizeCloneURLToHTTPS(cloneURL))`; the `s.fail` callers pass the same one through. This matches the helper signature `postCheck(ctx, md, authedURL, ref, expectedSHA, tag)` and avoids re-authing inside the helper.
   - Call `s.ops.LsRemote(ctx, authedURL, ref, tag)`. The error path wraps with `redactToken` before logging: `glog.V(2).Infof("...verdict=no-op-remote-error err=%s", redactToken(err.Error()))`. The post-check NEVER returns an error itself — the agent's verdict is the existing `## Result`'s verdict, full stop.
   - The empty-result branch: `if sha == "" { glog.V(2).Infof("...verdict=no-op-remote-empty"); return }`.
   - The released branch: `if expectedSHA != "" && sha == expectedSHA { ... }`. Marshal `ResolutionOutput{Verdict: "released", PlannedVersion: tag, ObservedRemoteSHA: sha}` via `agentlib.MarshalSectionTyped(ctx, "## Resolution", ...)`, then `md.ReplaceSection(section)`. Set `md.Frontmatter["status"] = "completed"` and `md.Frontmatter["phase"] = "done"`. Log `verdict=released`.
   - The superseded branch: any other non-empty SHA. Same write path with `Verdict: "superseded"`. Log `verdict=superseded`.
   - The failure-path-with-non-empty-SHA edge case: when `expectedSHA` is empty (failure path) and the remote returns a non-empty SHA, the verdict is `superseded` — a later release won the slot. This is the spec's "expected SHA does not exist, e.g. the failure path where push never reached the tag step" branch (DB #2 / DB #3 / DB #4 in the spec).

3. **Add an export hook** in `agent/github-releaser/pkg/export_test.go` if the external `_test` package needs to drive the helper directly (e.g. `PostCheckForTest = (*executionStep).postCheck`). Only add it if the integration tests cannot reach the helper through the public `Run` entry point — preference is to test through `Run`.

---

### Part B — wire the helper into the success-return path AND every `s.fail` call site

4. **Success-return path** in `Run` (the existing block at line 119-131 of `steps_execution.go`). After `md.ReplaceSection(section)` writes `## Result(outcome=released)`, but BEFORE the `releaseSuccess = true; return &agentlib.Result{...}` line, insert the post-check call. The post-check needs:
   - `taskID` — already in scope as `taskID` from line 88 (or re-read from `md.Frontmatter.String("task_identifier")` if you prefer; both work).
   - `tag` — `tagName` from `executeLocalRelease`'s return at line 106 (or re-derive via `strings.TrimPrefix(plan.NextVersionHeader, "## ")` if you want to call post-check before the `executeLocalRelease` return is destructured).
   - `authedURL` — re-build via `s.injectToken(normalizeCloneURLToHTTPS(cloneURL))`. The `cloneURL` is the unauthed form already in scope at line 88.
   - `ref` — already in scope at line 88.
   - `expectedSHA` — `sha` from `executeLocalRelease`'s return at line 106.

   The exact code shape (the executor picks the cleanest one that compiles):

   ```go
   sha, tagName, failResult := s.executeLocalRelease(...)
   if failResult != nil {
       return failResult, nil
   }
   // ... existing ## Result write ...
   md.ReplaceSection(section)
   // NEW: post-check verifies the local commit shipped to the remote.
   s.postCheck(ctx, md, taskID, tagName,
       s.injectToken(normalizeCloneURLToHTTPS(cloneURL)), ref, sha)
   releaseSuccess = true
   return &agentlib.Result{...}, nil
   ```

5. **Failure-return paths — the `s.fail` widening**. The cleanest shape is to widen `s.fail` to take an additional `authedURL` + `ref` + `expectedSHA` + `tag` parameter set, then call `s.postCheck(...)` inside `s.fail` BEFORE returning. This way every existing `s.fail(...)` call site participates in the post-check automatically with one change. Alternative shapes (wrapping `s.fail`, inserting a post-check pass at `Run`'s tail) are acceptable AS LONG AS all current `s.fail` call sites participate. The constraint from the spec is the outcome, not the mechanism.

   The recommended shape (the executor should follow this unless there's a strong local reason to deviate):

   ```go
   // Updated s.fail signature:
   func (s *executionStep) fail(
       ctx context.Context,
       md *agentlib.Markdown,
       category git.ErrorCategory,
       cause error,
       taskID string,                 // NEW
       tag string,                    // NEW
       authedURL string,              // NEW
       ref string,                    // NEW
       expectedSHA string,            // NEW (empty on failure path)
   ) (*agentlib.Result, error) {
       // ... existing ## Result(outcome=failed) write ...
       // NEW: post-check fires here, BEFORE the return.
       s.postCheck(ctx, md, taskID, tag, authedURL, ref, expectedSHA)
       return &agentlib.Result{...}, nil
   }
   ```

   Update every `s.fail(...)` call site in `steps_execution.go` to pass the new parameters. **There are currently 15 call sites** (re-verify with `grep -n 's\.fail(' agent/github-releaser/pkg/steps_execution.go` before threading) — distributed across `Run`, `executeLocalRelease`, and `guardCommittedFiles`. The executor must:
   - Pass `taskID` (from the `extractFrontmatter` return at line 88) at the top-of-`Run` call sites (lines 85, 90).
   - Pass the planned tag (`strings.TrimPrefix(plan.NextVersionHeader, "## ")`) at every site — the tag is determined by the plan, not by whether execution succeeded.
   - Pass the authed URL (`s.injectToken(normalizeCloneURLToHTTPS(cloneURL))`) — same URL the success path uses.
   - Pass `ref` (from the `extractFrontmatter` return).
   - Pass `expectedSHA=""` on every failure path (push never reached the tag step, so the agent has no "expected" SHA — the post-check still fires the superseded branch if a later release won the slot, per DB #3 of the spec).

   The two `s.fail` call sites in `Run` itself (lines 85, 90) are easy — they have `taskID`, `plan.NextVersionHeader`, `cloneURL`, `ref` all in scope. The call sites inside `executeLocalRelease` (lines 204, 211, 225, 240, 252, 257, 270, 281, 286, 292, 303, 317 — every `s.fail` inside this helper) need the parameters threaded as additional return values or as closure variables. The cleanest approach: change `executeLocalRelease`'s signature to also return the `tagName` and `authedURL` (or accept them as `*string` out-params), so `s.fail(ctx, md, ..., tag, authedURL, ref, "")` works at every site. Alternatively, build the authed URL once at the top of `executeLocalRelease` and pass it to every internal `s.fail` call — the executor picks the cleanest local shape.

6. **The pre-push guard's `s.fail` call sites** in `guardCommittedFiles` — **both** at line 336 AND line 341 — are participants. Thread the same parameters through `guardCommittedFiles`'s signature, OR have it accept `authedURL` + `ref` + `tag` as additional args, so both sites get the post-check.

7. **Idempotency in re-fire tests**. The existing `Context("re-fire idempotency")` block at line 651 of `steps_execution_test.go` runs `Run` twice against the same taskMD. The post-check is now part of the helper, so on the second run, the frontmatter `status: completed` (set by the first run's post-check) triggers the idempotency guard and the post-check is a no-op. Add a NEW `It` block under this context that asserts:
   - First run: `LsRemote` is called once, the `## Resolution` block is written, the frontmatter is rewritten.
   - Second run: `LsRemote` is called AGAIN (the helper does not gate at helper entry, only on the frontmatter status — the helper itself is invoked on both runs; the second run's helper sees `status=completed` and exits immediately). The `## Resolution` block byte-equal to the first run (no second copy, no rewrite). The frontmatter byte-equal.
   - Concretely: `Expect(md2.Frontmatter["status"]).To(Equal("completed"))` after both runs; the body bytes after run 2 equal the body bytes after run 1 (capture via `md2.Marshal` and compare).

8. **Add new Ginkgo integration tests** in `agent/github-releaser/pkg/steps_execution_test.go` under a new `Context("post-check (spec 064)", func() { ... })` block. Cover, at minimum:

   - **Released branch (success path)**: `fakeOps.LsRemoteReturns("abc1234", nil)` where `"abc1234"` matches `Commit`'s return. After `Run`, assert: `md.Frontmatter["status"] == "completed"`, `md.Frontmatter["phase"] == "done"`, the `## Resolution` block exists, `ResolutionOutput.Verdict == "released"`, `ResolutionOutput.PlannedVersion == "v1.2.8"` (the tag from the fixture), `ResolutionOutput.ObservedRemoteSHA == "abc1234"`. The existing `## Result(outcome=released)` is still on disk (the post-check does NOT replace it — it appends `## Resolution` after it).

   - **Superseded branch (success path)**: `fakeOps.LsRemoteReturns("deadbee", nil)` where `"deadbee"` differs from `Commit`'s return `"abc1234"`. After `Run`, assert: frontmatter is `completed`/`done`, `## Resolution.Verdict == "superseded"`, `ObservedRemoteSHA == "deadbee"`.

   - **Empty-result no-op (success path)**: `fakeOps.LsRemoteReturns("", nil)`. After `Run`, assert: frontmatter is whatever the existing success path set (NOT `completed` — the post-check is a no-op, so the success path's existing frontmatter state stands), NO `## Resolution` block on disk. Use `agentlib.FindSection` or `agentlib.ExtractSection[ResolutionOutput]` and assert an empty result or missing section.

   - **LsRemote error no-op (success path)**: `fakeOps.LsRemoteReturns("", errors.Errorf(ctx, "ls-remote boom"))`. After `Run`, assert: same as empty-result case (no frontmatter rewrite, no `## Resolution` block). The error is logged but not surfaced.

   - **Failure-path participates**: drive `Run` to a failure (e.g. `fakeOps.CloneReturns(...)` error). Assert: `## Result(outcome=failed)` is on disk, `s.fail` was called, and the post-check fired (assert `fakeOps.LsRemoteCallCount() == 1`). Use a faked `LsRemoteReturns` that returns a non-empty SHA so the superseded branch fires — assert frontmatter is rewritten to `completed`/`done` AND `## Resolution.Verdict == "superseded"`.

   - **Idempotency on already-terminal**: parse a `taskMD` whose frontmatter is `status: completed` from the start. Run `Run`. Assert: `fakeOps.LsRemoteCallCount() == 0` (the helper exits at the idempotency guard before invoking `LsRemote`). The body bytes are unchanged. (The helper still fires, but the `LsRemote` mock is never called — the frontmatter gate is the assertion.)

   - **Token-redaction on LsRemote error**: `fakeOps.LsRemoteReturns("", errors.Errorf(ctx, "fatal: unable to access 'https://x-access-token:ghp_LEAKEDTOKEN@github.com/...': repository not found"))`. After `Run`, capture `glog` output via the existing test helper (search for an existing glog-capture helper in the repo — `pkg/steps_execution_test.go` does not appear to have one; if absent, follow the pattern from `os_exec_git_ops_test.go`'s `redactToken` test which only checks the helper directly, OR add a minimal `glog.SetLogger` stub in the test setup). The test asserts the captured log line contains `verdict=no-op-remote-error` AND does NOT contain `ghp_LEAKEDTOKEN` AND does NOT contain the literal `x-access-token:ghp_LEAKEDTOKEN` (i.e. the redaction worked on the err message that was wrapped through the post-check log call).

   The test scaffolding follows the existing `It("...", func() { ... })` pattern. Use `fakeOps.LsRemoteStub` (not just `LsRemoteReturns`) when you need to drive different return values per-call index — the counterfeiter mock supports both.

9. **Update the `executeLocalRelease` doc comment** at line 186 of `steps_execution.go` to note that the network push is NOT part of this helper (it happens in ai_review per spec 058), and that the post-check (this prompt's helper) is the only place the remote is consulted for tag state.

10. **Update the `Run` doc comment** at line 65-81 of `steps_execution.go` to mention the post-check tail (one extra sentence: "After the success path writes `## Result(outcome=released)`, the post-check verifies the local commit shipped to the remote and may upgrade the verdict to `released` or `superseded`.").

11. **Update the repo-root `CHANGELOG.md`** at `/workspace/CHANGELOG.md`. Append a new bullet to `## Unreleased` (after the LsRemote seam bullet from prompt 1, which goes first because it's earlier in the release cycle; the post-check goes immediately after). Use the prefix `feat(agent/github-releaser):` and describe the post-check behavior — DO NOT include the seam (that's prompt 1's entry). Suggested wording:

    ```
    - feat(agent/github-releaser): post-check tail on the execution step — after `## Result` is written (success or failure), the agent shells out `git ls-remote refs/tags/<planned-version>` and uses the response to decide the terminal verdict. Remote shows tag at expected SHA → upgrade to `released` + `## Resolution` block + `status: completed` / `phase: done`. Remote shows tag at different SHA → upgrade to `superseded` + `## Resolution` block + `status: completed` / `phase: done`. Remote empty → no-op. `ls-remote` error → no-op (error logged, never downgrades a verdict). The post-check is idempotent: `status ∈ {completed, aborted}` → return immediately at the first statement. One structured log line per invocation via `glog.V(2)` naming `task_id`, `planned_version`, `observed_remote_sha`, and `verdict` from `{released, superseded, no-op-remote-empty, no-op-remote-error, no-op-already-terminal}`. All existing `s.fail` call sites participate via a widened signature (the verdict change is internal to the agent — no Kafka envelope, no agent-lib API changes). Closes the false-negative class of bug where a successful GitHub release is recorded as `failed` because the agent only consulted local state (spec 064)
    ```

    Place this at the top of `## Unreleased` if the LsRemote seam bullet is already there, immediately after that bullet. Match the existing reverse-chronological order.

12. **No factory changes**. `pkg/factory/factory.go` does NOT need to change — `NewExecutionStep(ops, ghToken)` still takes the same `git.GitOps` interface, and the new `LsRemote` method is part of the same interface. The execution step's external signature is unchanged. Verify with `cd /workspace/agent/github-releaser && go build ./...` — if it fails, the prompt is wrong; do not modify the factory.

13. **No agent-lib API changes**. Do NOT add a `CompleteCommand` or `CloseCommand` to `github.com/bborbe/agent/lib`. The post-check is internal to the github-releaser agent; the verdict change rides on `md.Frontmatter` map writes and `md.ReplaceSection` of a new `## Resolution` block.

</requirements>

<constraints>
- **Do NOT commit** — dark-factory handles git
- The post-check upgrades verdicts only: failed → completed is allowed; released → failed is FORBIDDEN. A successful release whose remote query somehow returns empty (impossible in steady state but plausible during a partial GitHub outage) is left as released — the existing success-path verdict stands.
- The verdict change is internal to the agent. No Kafka envelope, no command schema, no agent-lib API changes.
- The post-check uses the same GitHub auth the execution step already uses (HTTPS clone URL with installation token injected by the existing `s.injectToken` helper at line 382 of `steps_execution.go`). No new token scope, no new secret mount.
- All existing passing tests under `agent/github-releaser/pkg/...` continue to pass.
- The `LsRemote` method is part of the `git.GitOps` interface from prompt 1. The execution step does not need a new constructor parameter — the `ops` field already carries the interface, and the new method is reachable.
- **Sequential dependency:** this prompt MUST NOT be approved or executed until prompt `spec-064-1-ls-remote-interface-impl` has merged. The `LsRemote` interface method and `mocks/git_ops.go` regeneration both ship in prompt 1; without them, this prompt's edits will not compile.
- **No Prometheus metrics** for post-check outcomes. The spec's Non-goals explicitly forbid this — "log lines are the only observability surface in this spec; metrics are deferred until a concrete consumer demands them".
- **No opt-out flag, config knob, or tunable threshold** for the post-check behavior. This is a correctness fix, not a feature. If a future consumer demands variation, that is a separate spec.
- **No new agent-lib commands** (`CompleteCommand`, `CloseCommand`, etc.). Reuse the existing `md.Frontmatter` map and `md.ReplaceSection` for the verdict change.
- The post-check NEVER mutates the body sections on the empty-result or error paths — only the frontmatter status/phase are touched on the released/superseded branches (and only there).

</constraints>

<verification>
```
cd /workspace/agent/github-releaser && go build ./...
```
Expected: exit code 0. The widened `s.fail` signature, the new `postCheck` method, the new `ResolutionOutput` type, and the `## Resolution` section writes all compile.

```
cd /workspace/agent/github-releaser && go vet ./...
```
Expected: exit code 0. No new vet warnings on the widened signatures or the new `postCheck` method.

```
cd /workspace/agent/github-releaser && make test
```
Expected: exit code 0. All existing tests + new post-check tests pass. The new test count: at least 7 `It` blocks under `Context("post-check (spec 064)", ...)` (released, superseded, empty-result, ls-remote-error, failure-path-participates, idempotency-on-already-terminal, token-redaction).

```
cd /workspace/agent/github-releaser && make precommit
```
Expected: exit code 0. Format, generate, test, lint, license, gosec all pass.

```
cd /workspace && grep -n "post-check" CHANGELOG.md
```
Expected: ≥1 line under `## Unreleased` mentioning `post-check`.

```
cd /workspace && grep -nE '^## Resolution' agent/github-releaser/pkg/steps_execution.go 2>/dev/null
```
Expected: NOT a hit — the `## Resolution` block is written at runtime, not declared in source. The source contains `agentlib.MarshalSectionTyped(ctx, "## Resolution", ...)` and the `ResolutionOutput` struct.

```
cd /workspace && grep -n "ResolutionOutput" agent/github-releaser/pkg/resolution_output.go
```
Expected: ≥1 hit, the new typed contract file exists at the named path.

```
cd /workspace && grep -n "postCheck\|post-check" agent/github-releaser/pkg/steps_execution.go
```
Expected: ≥2 hits — the helper definition + at least one call site (the success-return path).

```
cd /workspace/agent/github-releaser && go test -run TestSuite -v -ginkgo.focus="post-check" ./pkg/...
```
Expected: exit code 0. The new `Context("post-check (spec 064)", ...)` block's tests run and pass.

</verification>

<success_criteria>
- AC 5: The post-check helper in `steps_execution.go` runs on the success-return path AND on every failure-return path. Evidence: Ginkgo integration test under `pkg/steps_execution_test.go` exercises each `s.fail` call site and asserts the post-check fires (a faked `LsRemote` is invoked exactly once per `Run` call, including on the failure paths); on the success path, asserts the post-check fires after the existing `## Result(outcome=released)` write.
- AC 6: When the faked `LsRemote` returns the agent's expected SHA, the task frontmatter is rewritten to `status: completed`, `phase: done`, and a `## Resolution` block is appended naming the verdict `released`, the planned version, and the observed SHA. Evidence: integration test reads back the resulting task markdown and asserts frontmatter values + presence of the `## Resolution` block containing the SHA string returned by the faked `LsRemote` AND the planned version string from the agent's `## Plan` block.
- AC 7: When the faked `LsRemote` returns a different SHA, the task is closed as `completed` / `superseded` with the observed SHA in the `## Resolution` block. Evidence: integration test asserts frontmatter + `## Resolution` content.
- AC 8: When the faked `LsRemote` returns empty, the existing failure-path or success-path verdict stands unchanged. Evidence: integration test asserts no frontmatter mutation beyond what the existing path writes, no `## Resolution` block.
- AC 9: When the faked `LsRemote` returns an error, the existing verdict stands and a redacted error is logged. Evidence: integration test asserts no frontmatter mutation; log capture contains the verdict `no-op-remote-error` log line and does not contain `x-access-token:` substring (or the literal token).
- AC 10: Re-invoking the post-check on a task whose `status` is already `completed` or `aborted` produces no observable change to the task file. Evidence: integration test runs the helper twice in succession; assert the task file's byte content after the second run equals the byte content after the first run (no double `## Resolution` block, no frontmatter re-rewrite).
- AC 11: The post-check helper emits one structured log line per invocation naming `task_identifier`, planned version, observed remote SHA (or empty), and verdict from the set `{released, superseded, no-op-remote-empty, no-op-remote-error, no-op-already-terminal}`. Evidence: Ginkgo unit test captures `glog` output (via existing test helpers OR a minimal `glog.SetLogger` stub added in this prompt) and asserts the log line for each branch.
- AC 12: All existing `agent/github-releaser/pkg/...` Ginkgo tests continue to pass. Evidence: `make test` in `agent/github-releaser/` exits 0.
- AC 13: `make precommit` in `agent/github-releaser/` exits 0. Evidence: exit code.
- AC 14: `CHANGELOG.md` in repo root has a new `## Unreleased` entry describing the post-check behavior. Evidence: `grep -n 'post-check' /workspace/CHANGELOG.md` returns a line under `## Unreleased`.

</success_criteria>
