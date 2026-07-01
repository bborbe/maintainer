---
status: completed
summary: Replaced sameStringSet with isSubsetIncludingChangelog across execution guard and AI-review check, relaxed the release-commit file-set gate to accept byte-identical manifests while preserving fail-closed on unexpected files.
execution_id: maintainer-guard-noop-manifest-exec-266-relax-guard-noop-manifest
dark-factory-version: dev
created: "2026-07-01T00:00:00Z"
queued: "2026-07-01T21:39:08Z"
started: "2026-07-01T21:39:32Z"
completed: "2026-07-01T21:43:49Z"
---

<summary>
- A release commit is accepted when a detected plugin manifest was already at the target version, so it never entered the commit (byte-identical bump → no-op).
- The file-set gate changes from strict set-equality to a subset rule: committed files must be a subset of `{CHANGELOG.md} ∪ detected_manifests` AND must include `CHANGELOG.md`.
- Any file outside that allowed set still hard-fails the release (`unexpected_diff`) before a tag is created — the closed trust surface is preserved.
- A commit missing `CHANGELOG.md` still fails — the changelog remains mandatory.
- Fixes a live incident: `bborbe/agent` release stuck at v0.71.0 because a PR pre-set `plugin.json` to the target version, so the manifest never appeared in the release commit and the gate rejected it.
- BOTH gates that inspect the release commit adopt the same relaxed rule via a single shared helper `isSubsetIncludingChangelog`: the execution pre-push guard AND the AI-review file-set check. This intentionally changes AI-review behavior — a no-op-manifest release no longer routes to `human_review`, it auto-releases. Relaxing only the execution guard would NOT unblock the incident, because the AI-review check runs the identical gate on the same commit and would still fail it.
- The old equality helper `sameStringSet` is removed entirely — after both call sites switch to the new helper it has no remaining caller and would be flagged as dead code.
- New tests cover both layers: execution-step no-op / mixed / out-of-set-reject / missing-changelog-reject, plus an AI-review no-op case that must stay approved.
</summary>

<objective>
Relax the release-commit file-set gate in the github-releaser so a commit is accepted when a detected plugin manifest was already at the target version (byte-identical → absent from the commit), while still rejecting any commit that touches a file outside `{CHANGELOG.md} ∪ detected_manifests` or that omits `CHANGELOG.md`. Both places that enforce this gate — the execution pre-push guard and the AI-review file-set check — must adopt the identical relaxed rule via one shared helper.
</objective>

<context>
Read CLAUDE.md at the repo root and at `agent/github-releaser/` for conventions before editing.

Detail level: this is Level 3 (Medium) translation work — the patterns already exist in-tree. Reference the exemplars below and adopt their style; do NOT inline full function bodies from memory.

Why both layers matter: the execution guard (`guardCommittedFiles`) and the AI-review check (`checkUnexpectedFileChange`) run the SAME `if !sameStringSet(files, expected)` gate against the SAME preserved release commit. If only the execution guard is relaxed, a no-op-manifest release passes the guard/tag but then the AI-review check still fails → `checks.UnexpectedFileChange=true` → `FailedChecks` gets `CheckUnexpectedFileChange` → the verdict rolls up to `approved=false` → the release routes to `human_review` instead of auto-releasing. The incident stays blocked. Both must use the same relaxed definition.

Files to read first (all paths relative to the worktree root):
- `agent/github-releaser/pkg/steps_execution.go` — `executeLocalRelease` builds `expectedFiles` (line ~379) and calls `guardCommittedFiles` (defined ~399-424; the gate is `sameStringSet(files, expectedFiles)` at ~416). `sameStringSet` is defined at ~664-675. `changelogFileName = "CHANGELOG.md"` constant at ~34. `slices` is already imported (line ~11).
- `agent/github-releaser/pkg/steps_ai_review.go` — `checkUnexpectedFileChange` (~499-547): builds `expected = {changelogFileName} ∪ plugin.DetectManifests(...)` (~519-533), runs the SAME gate `if !sameStringSet(files, expected)` at ~534, and on failure returns `diffStringSet(files, expected)` at ~543 (`diffStringSet` defined ~679 — STAYS, still used). The comment at ~494-498 currently says "sameStringSet is the same helper used by steps_execution.go" — it becomes stale and must be updated.
- `agent/github-releaser/pkg/steps_execution_test.go` — test patterns to mirror. Happy-path Run-level assertions (`CommittedFilesReturns`, `TagCallCount`, `agentlib.ExtractSection[pkg.ResultOutput]`) at ~92-170. Plugin-manifest exemplar (`taskMDPlugin`, `writeManifest`, `writeChangelogAndBothManifests`) at ~975-1064. The `Describe("sameStringSet")` `DescribeTable` at ~926-951 (to be replaced).
- `agent/github-releaser/pkg/steps_ai_review_test.go` — the "committed plugin manifest is in the expected set → UnexpectedFileChange=false" exemplar at ~809-856 (seeds a real temp workdir + `plugin.json` so `DetectManifests` finds it, `CommittedFilesReturns(...)`, then asserts `review.Checks.UnexpectedFileChange` and `review.FailedChecks`). Mirror this for the new no-op case.
- `agent/github-releaser/pkg/git/git.go` — `GitOps` interface (~37); `CommittedFiles(ctx, workdir) ([]string, error)` at ~63. UNCHANGED — do not touch.
- `agent/github-releaser/mocks/git_ops.go` — counterfeiter fake `GitOps`, package `gitmocks`, imported as `gitmocks "github.com/bborbe/maintainer/agent/github-releaser/mocks"`. Do NOT regenerate mocks or touch `//counterfeiter:generate`.
- `agent/github-releaser/pkg/export_test.go` — `SameStringSetForTest = sameStringSet` (line ~15-17), to be replaced.
- `agent/github-releaser/pkg/git/error_classifier.go` — `ErrorCategoryUnexpectedDiff ErrorCategory = "unexpected_diff"` (~52).

Error-wrapping convention: `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`.
Testing convention: `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`.

Why (incident context): `bborbe/agent` release stuck at commit `defa467` / v0.71.0 because a PR pre-set `plugin.json` to 0.71.0. The manifest was already at the target version, so the byte-identical bump produced no change and never entered the release commit — the exact-equality gate then failed with `unexpected_diff`.
</context>

<requirements>
1. In `agent/github-releaser/pkg/steps_execution.go`, add a NEW unexported helper (package `pkg`, so both `steps_execution.go` and `steps_ai_review.go` can call it). Suggested shape (adapt naming/comments to the file's style; `slices` is already imported):
   ```go
   // isSubsetIncludingChangelog reports whether every file in committed is
   // present in allowed AND committed contains changelogFileName. It relaxes
   // the release-commit file-set gate so a detected manifest that was already
   // at the target version (byte-identical → absent from the commit) does not
   // fail the release, while any file outside the allowed set still fails
   // closed. Shared by the execution pre-push guard and the ai_review file-set
   // check — both must enforce the identical rule against the same commit.
   func isSubsetIncludingChangelog(committed, allowed []string) bool {
       if !slices.Contains(committed, changelogFileName) {
           return false
       }
       for _, f := range committed {
           if !slices.Contains(allowed, f) {
               return false
           }
       }
       return true
   }
   ```

2. Execution guard: in `guardCommittedFiles` (`steps_execution.go:~416`), swap `!sameStringSet(files, expectedFiles)` → `!isSubsetIncludingChangelog(files, expectedFiles)`. Keep the failed-Result branch (category `ErrorCategoryUnexpectedDiff`) and the `CommittedFiles` error branch unchanged. Optionally rename the `expectedFiles` param/local (`steps_execution.go:~379, ~403`) to `allowedFiles` for clarity — cosmetic; if renamed, update every reference consistently.

3. AI-review call site: in `steps_ai_review.go:~534`, swap `!sameStringSet(files, expected)` → `!isSubsetIncludingChangelog(files, expected)`. Leave the failure-diff `return diffStringSet(files, expected)` at ~543 as-is (`diffStringSet` stays, still used). Update the now-stale comment at `steps_ai_review.go:~494-498`: it must describe the shared `isSubsetIncludingChangelog` subset rule (committed ⊆ allowed AND changelog present) instead of "sameStringSet is the same helper used by steps_execution.go".

4. Remove the now-dead helper cleanly:
   - Delete `sameStringSet` (func + its doc comment) at `steps_execution.go:~664-675`. After steps 2-3 it has zero production callers.
   - In `export_test.go`: replace `SameStringSetForTest = sameStringSet` (~15-17) with `var IsSubsetIncludingChangelogForTest = isSubsetIncludingChangelog` (plus a short doc comment matching the file's style).

5. Error handling: keep the guard failure using `github.com/bborbe/errors` — `errors.Errorf(ctx, ...)` / `errors.Wrapf(ctx, err, ...)`. Never `fmt.Errorf` or bare `return err`. Match the existing guard message shape (`"release commit must change only %v, got %v"`). Prefer naming the specific unexpected file (the first `f` in `committed` not in `allowed`) so the failure log points at the offending path; fall back to the whole-set message if that complicates the code.

6. Preserve sequencing: the execution guard runs BEFORE `Tag` in `executeLocalRelease`, and a failed guard returns before any tag/push. Do not move the guard call. Do not change the AI-review rollup/verdict wiring beyond the single call-site swap in step 3.

7. Replace the `sameStringSet` helper `DescribeTable` at `steps_execution_test.go:~926-951` with a `Describe("isSubsetIncludingChangelog", ...)` `DescribeTable` exercised via `pkg.IsSubsetIncludingChangelogForTest`. The old entries tested SYMMETRIC set-equality — rewrite them, do not mechanically port, because the new rule is ASYMMETRIC (committed vs allowed). Entries must cover:
   - committed ⊆ allowed AND `CHANGELOG.md` present → true
   - committed contains a file NOT in allowed → false
   - committed missing `CHANGELOG.md` → false
   - committed == allowed (exact match) → true
   - committed is only `{CHANGELOG.md}` while allowed also lists manifests (the no-op case) → true

8. Add execution-step Run-level cases to `steps_execution_test.go`, mirroring the plugin-manifest exemplar (~975-1064) and the happy-path assertions (~92-170). Drive detected manifests by writing them to the workdir in `CloneStub` (real `plugin.DetectManifests` finds them); drive the committed set via `fakeOps.CommittedFilesReturns(...)`:
   a. No-op: both `plugin.json` and `marketplace.json` written to workdir (both detected → in allowed set), but `CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)`. Assert ACCEPT: `result.Status == agentlib.AgentStatusDone`, `## Result` outcome `released`, empty `ErrorCategory`, `fakeOps.TagCallCount() == 1`.
   b. Mixed: both manifests written, `CommittedFilesReturns([]string{"CHANGELOG.md", ".claude-plugin/plugin.json"}, nil)`. Assert ACCEPT (same as 8a).
   c. Out-of-set reject: `CommittedFilesReturns([]string{"CHANGELOG.md", "README.md"}, nil)`. Assert REJECT: `## Result` outcome `failed`, `ErrorCategory == "unexpected_diff"`, `fakeOps.TagCallCount() == 0`.
   d. Missing-changelog reject: `CommittedFilesReturns([]string{".claude-plugin/plugin.json"}, nil)`. Assert REJECT: `unexpected_diff`, `TagCallCount() == 0`.

9. Add an AI-review Run-level case to `steps_ai_review_test.go`, mirroring the exemplar at ~809-856. Seed both manifests into a real temp workdir (same mechanism as the exemplar) so `plugin.DetectManifests` finds them, but `fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)` (no-op manifest absent from the commit). Assert `review.Checks.UnexpectedFileChange == false` AND `review.FailedChecks` does NOT contain `pkg.CheckUnexpectedFileChange` (and, as in the exemplar, `review.Approved` stays true on the otherwise-passing path). Keep/confirm an out-of-set AI-review case (committed includes e.g. `README.md`) still sets `UnexpectedFileChange == true`.

10. Do NOT touch the `GitOps` interface, `//counterfeiter:generate`, or regenerate the mock.
</requirements>

<constraints>
- New invariant: committed files MUST be a subset of `{CHANGELOG.md} ∪ detected_manifests` AND MUST include `CHANGELOG.md`. This relaxes only the "manifests optional in the commit" direction; the closed trust surface is otherwise preserved.
- Do NOT introduce a second definition of the allowed-set rule — BOTH call sites (execution guard `steps_execution.go` + ai_review `steps_ai_review.go`) must call the single new `isSubsetIncludingChangelog`. `sameStringSet` is removed (no remaining caller). `diffStringSet` stays (still used at `steps_ai_review.go:~543`).
- Do NOT touch the `GitOps` interface, `//counterfeiter:generate`, or regenerate `mocks/git_ops.go`.
- Error handling must use `github.com/bborbe/errors` (`errors.Errorf` / `errors.Wrapf`), never `fmt.Errorf` or bare `return err`.
- Run `make precommit` from `agent/github-releaser/` (the service dir), NOT the repo root.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass (aside from the intentionally-rewritten helper table).
</constraints>

<verification>
Run from the service directory:
```
cd agent/github-releaser && make precommit
```
Must pass. Confirm specifically:
- Execution no-op and mixed cases (8a, 8b) pass and reach `Tag` (`TagCallCount() == 1`, outcome `released`); out-of-set and missing-changelog cases (8c, 8d) fail closed with `unexpected_diff` and `TagCallCount() == 0`.
- AI-review no-op case (step 9) passes: `review.Checks.UnexpectedFileChange == false` and `pkg.CheckUnexpectedFileChange` is NOT in `review.FailedChecks`; the out-of-set AI-review case still sets `UnexpectedFileChange == true`.
- The new `isSubsetIncludingChangelog` `DescribeTable` passes with the asymmetric entries.
- `sameStringSet` is fully removed: `rg -n 'sameStringSet|SameStringSetForTest' agent/github-releaser/` returns NOTHING.
- Both call sites reference the shared helper: `rg -n 'isSubsetIncludingChangelog' agent/github-releaser/pkg/steps_execution.go agent/github-releaser/pkg/steps_ai_review.go` shows the guard and the ai_review check.
- `diffStringSet` is still present and referenced at `steps_ai_review.go:~543`.
</verification>
