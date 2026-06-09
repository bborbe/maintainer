---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-06-08T16:00:30Z"
generating: "2026-06-08T16:00:31Z"
prompted: "2026-06-08T16:29:04Z"
verifying: "2026-06-09T07:13:35Z"
completed: "2026-06-09T16:30:32Z"
branch: dark-factory/github-releaser-post-check-verdict-from-github-truth
previous_id: 064
---

## Summary

- After every github-releaser execution attempt — success or failure — the agent asks GitHub whether the planned version tag already exists on the remote, and writes its terminal verdict from that answer rather than from local state alone.
- When the remote shows the tag at any commit, the release task is closed (verdict `released` if the tag matches the SHA the agent just produced; verdict `superseded` if the tag is at a different SHA, meaning a later release already filled the slot).
- When the remote shows no tag, behavior is unchanged from today — the existing success or failure path stands.
- The post-check only ever upgrades a verdict from failed to completed; it never downgrades a successful release to failed, and it never mutates an already-terminal (`completed` / `aborted`) task.
- Net effect: three of four currently stuck `bborbe/go-skeleton` release tasks (the ones that actually shipped on GitHub) auto-close on the next agent run instead of accumulating in the human triage queue.

## Problem

The github-releaser agent conflates *local push succeeded* with *release succeeded*, and *review approved* with *release happened*. Both are wrong. Concrete evidence: four `bborbe/go-skeleton` release tasks accumulated as stuck in OpenClaw over seven days (2026-05-29 → 06-03). Three of them describe releases that actually shipped to GitHub — the agent just wrote "Failure" because it never asked GitHub what state the release tag was in. The pattern produces vault dust at the rate of `failed-release-attempts × repo-activity` and pulls a human into the loop for resolutions that are purely mechanical: ask GitHub, see what's there, write that down.

The mechanical-resolution boundary is the principle: any answer the agent can compute by querying the remote should be computed by the agent, not by the human. This spec draws that line for the github-releaser's terminal verdict.

## Goal

After every release execution attempt, the agent's terminal verdict on the task reflects GitHub's view of the release-tag slot, not the agent's local belief. A release task is `failed` only when the remote also shows no tag for the planned version. A release task whose target version tag already exists on the remote — at any commit — is `completed` (either `released` when the tag SHA matches what the agent produced, or `superseded` when a later release won the slot at a different SHA).

## Non-goals

- Do NOT add a watcher-side close-on-shipped backstop in `watcher/github-release/pkg/watcher.go`. Stage-1 prototype proved the agent-side post-check plus the operator slash command together cover every fixture class. Watcher work stays deferred.
- Do NOT introduce new agent-lib commands (no new `CompleteCommand` / `CloseCommand`). Reuse existing phase-transition mechanics.
- Do NOT change any Kafka message contract. The verdict change is internal to the agent.
- Do NOT fix the `df89963` "container missing `ssh` binary" failure here — that is an image-layer concern handled by a separate task.
- Do NOT add a Prometheus metric for post-check outcomes. Log lines are the only observability surface in this spec; metrics are deferred until a concrete consumer demands them.
- Do NOT add an opt-out flag, config knob, or tunable threshold for the post-check behavior. This is a correctness fix, not a feature. If a future consumer demands variation, that is a separate spec.
- Do NOT move the network push out of the ai_review step into the execution step. Push location stays where it is today.
- Do NOT relocate where the existing `## Result(outcome=released)` / `## Result(outcome=failed)` section is written. The post-check is an upgrade pass over the verdict that the execution step would have written; it does not restructure the step pipeline.

## Desired Behavior

1. At the tail of the execution step's `Run`, after the existing `## Result` section would be written for either the success or the failure path, the agent invokes one `ls-remote` query against the planned-version tag on the target repo and uses the response to decide the final verdict.
2. When the remote query returns a commit SHA equal to the SHA the agent just produced (the success path's freshly-tagged commit), the verdict is `released` with a note "tag verified at expected SHA"; the task is set to `status: completed`, `phase: done`; a `## Resolution` block is appended recording the verdict, the remote tag, and the observed SHA.
3. When the remote query returns a commit SHA different from what the agent expected (or no expected SHA exists, e.g. the failure path where push never reached the tag step), the verdict is `superseded` with a note citing the planned version and the observed SHA prefix; the task is set to `status: completed`, `phase: done`; a `## Resolution` block is appended.
4. When the remote query returns no SHA for the planned version tag, the post-check is a no-op: the existing success-path or failure-path verdict the execution step would have written stands unchanged.
5. When the remote query itself errors (network failure, auth failure, transient remote rejection), the post-check logs the error and is a no-op: the existing success-path or failure-path verdict stands unchanged. The post-check never downgrades a verdict on its own failure.
6. The post-check is idempotent: when invoked against a task whose current `status` is already `completed` or `aborted` (read from frontmatter at the post-check helper's entry), the helper returns immediately without re-querying the remote, without re-writing frontmatter, and without re-appending the `## Resolution` block. A re-triggered run on an already-closed task produces no observable change to the task file.
7. Review rejection alone no longer flips the task status to `failed` when the planned-version tag exists on the remote at the SHA the agent produced. The review verdict is preserved as a recorded warning (in a `## Review Warning` block, or its existing equivalent within the ai_review step's contract — the spec leaves the exact mechanism to the implementing prompt as long as the review verdict is durably visible on the task body and the task closes as `completed`).
8. Every post-check invocation emits a structured log line naming the task identifier, the planned version, the observed remote SHA (or empty), and the chosen verdict (`released` / `superseded` / `no-op-remote-empty` / `no-op-remote-error` / `no-op-already-terminal`), so operators can grep the deciding fact for any task in agent logs.

## Constraints

- The `GitOps` interface in `agent/github-releaser/pkg/git/git.go` retains its `//counterfeiter:generate` annotation. Any new method added to the interface follows the existing context-first, error-returning, argv-only contract.
- The concrete `LsRemote` implementation in `agent/github-releaser/pkg/git/os_exec_git_ops.go` uses argv-only `exec.CommandContext` (no `sh -c`, no shell interpolation), reuses the package's existing token-injection model at the call site, and runs every error string through `redactToken` before wrapping — the agent must never emit `x-access-token:<TOK>@github.com/...` to logs.
- The `LsRemote` query against an annotated tag returns the dereferenced *commit* SHA (the `refs/tags/<tag>^{}` line), not the tag-object SHA (the `refs/tags/<tag>` line). For lightweight (un-annotated) tags where no `^{}` line is returned, the tag-object SHA is returned as a fallback. Comparing against the tag-object SHA is a silent correctness bug — the stage-1 prototype hit it on iteration one. The trap is fully captured inline in this spec; the off-repo Obsidian doc [[Release Close-on-Shipped Prototype Learnings]] is supplementary background, not a dependency. **A `docs/release-close-on-shipped-prototype-learnings.md` mirror in this repo is OUT OF SCOPE for these prompts** — the spec's Failure Modes table + this Constraints section + the unit-test fixture (annotated + lightweight tag) carry the load. If a future doc-only PR ports the learnings into `docs/`, it can backfill the reference; the post-check correctness does not depend on it.
- The mock under `agent/github-releaser/mocks/git_ops.go` is regenerated via the existing `go generate -mod=mod ./...` workflow. It is NOT hand-edited.
- The post-check helper runs on **both** the success-return path (where the execution step would have written `## Result(outcome=released)` at the existing call site near line 119 of `steps_execution.go`) **and** every failure-return path (every `s.fail` call site). The shape of how the post-check is wired in — whether by widening `s.fail`'s signature, by wrapping `s.fail`, or by inserting a post-check pass at `Run`'s tail — is left to the implementing prompt; the constraint is that all current `s.fail` call sites participate.
- The post-check upgrades verdicts only: failed → completed is allowed; released → failed is forbidden. A successful release whose remote query somehow returns empty (impossible in steady state but plausible during a partial GitHub outage) is left as released — the existing success-path verdict stands.
- The verdict change is internal to the agent. No Kafka envelope, no command schema, no agent-lib API changes.
- The post-check uses the same GitHub auth the execution step already uses (HTTPS clone URL with installation token injected by the existing helper). No new token scope, no new secret mount.
- All existing passing tests under `agent/github-releaser/pkg/...` continue to pass.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility | Concurrency |
|---------|-------------------|----------|-----------|---------------|-------------|
| `ls-remote` returns no tag for the planned version (genuine failure) | Existing failure-path verdict stands unchanged; post-check is a no-op | Existing controller retry path runs | Log line `verdict=no-op-remote-empty` | n/a — no state mutation | Re-trigger re-runs the post-check; same input, same no-op outcome |
| `ls-remote` returns the planned tag at the SHA the agent just produced | Verdict upgraded to `completed` / `released`; `## Resolution` block appended | None needed | Log line `verdict=released`; frontmatter `status: completed`; `## Resolution` block on disk | Reversible only by human edit of the task file (acceptable — `## Resolution` documents the deciding fact) | Idempotency guard at helper entry blocks double-mutation on re-trigger |
| `ls-remote` returns the planned tag at a SHA different from what the agent expected (superseded case) | Verdict upgraded to `completed` / `superseded`; `## Resolution` block appended | None needed | Log line `verdict=superseded`; frontmatter `status: completed`; `## Resolution` block on disk | Reversible only by human edit (acceptable) | Idempotency guard at helper entry blocks double-mutation |
| `ls-remote` errors (network / auth / transient) | Post-check is a no-op; existing verdict stands; error logged with `redactToken` applied to any URL fragment | Operator may re-delegate the task to re-trigger the execution step + post-check | Log line `verdict=no-op-remote-error` + redacted error | n/a — no state mutation | Re-trigger re-queries on the next run |
| Re-trigger of an already-closed (`status ∈ {completed, aborted}`) task | Post-check helper returns immediately at first statement; no `ls-remote` call; no frontmatter rewrite; no `## Resolution` re-append | None needed | Log line `verdict=no-op-already-terminal` | n/a — no state mutation | Concurrent invocations protected by the controller's existing per-task lock; the idempotency guard makes the helper itself safe under serial re-trigger |
| Annotated tag on the remote whose `^{}` line is dropped by an exact-ref filter | The `LsRemote` query returns the dereferenced commit SHA, not the tag-object SHA, by querying both `refs/tags/<tag>` and `refs/tags/<tag>^{}` and preferring the `^{}` line | n/a — handled by impl policy (see Constraints); no operator action required | Unit test covers both annotated and lightweight tag fixtures | n/a | n/a |
| Tag overwritten on the remote (fixture e19e5fb: task claimed `d0fa576`, remote now shows `2ce8f8f`) | Rule 1 does not fire (SHAs differ); Rule 2 fires — verdict is `superseded`, citing the observed remote SHA | None needed; this is the correct classification | Log line `verdict=superseded`; `## Resolution` block names the observed SHA | n/a | n/a |
| Agent crashes before the execution step runs (D-class, e.g. `df89963` missing `ssh` binary) | Out of scope for this spec. The agent cannot post-check itself. Operator-run slash command (stage-1 artifact) closes these from outside the agent | Operator runs the existing `release-task-close-on-shipped` skill against the stuck task | n/a — this spec does not change D-class handling | n/a | n/a |

## Security / Abuse Cases

- The `ls-remote` query uses an attacker-controllable URL only insofar as `clone_url` is attacker-controllable — but `clone_url` is already used by the existing `Clone` call with the same auth model, so the trust boundary is unchanged.
- The post-check executes one `git ls-remote` per release attempt, with a context that inherits the agent's existing per-step timeout. There is no unbounded retry loop and no per-tag amplification.
- Token leakage prevention: the existing `redactToken` helper is applied to every error string that may contain the authenticated URL. The post-check's error path is not exempt.
- The `LsRemote` argv is constructed without shell interpolation (argv-only). Tag and URL strings are passed as separate argv elements; there is no `sh -c` and no per-input formatting that could introduce argument injection.
- An attacker who can race the agent and publish a `v<planned>` tag at any SHA can cause a `superseded` verdict instead of a `failed` verdict. This is acceptable: the task closes as completed, the human sees the release-tag slot is filled, and the next release attempt operates against the new remote state. The agent does not push over a remote tag at any point, so a race-published tag cannot cause the agent to overwrite remote state.

## Acceptance Criteria

- [ ] `agent/github-releaser/pkg/git/git.go` exposes a `LsRemote` method on the `GitOps` interface; `//counterfeiter:generate` annotation present and unchanged — evidence: `grep -n 'LsRemote' agent/github-releaser/pkg/git/git.go` returns ≥1 line within the `GitOps` interface block; `grep -n 'counterfeiter:generate' agent/github-releaser/pkg/git/git.go` returns 1 line.
- [ ] `agent/github-releaser/mocks/git_ops.go` was regenerated (not hand-edited) and is in sync with the interface — evidence: running `go generate -mod=mod ./...` from `agent/github-releaser/` leaves `git status` clean (no diff).
- [ ] The concrete `LsRemote` impl in `os_exec_git_ops.go` uses argv-only `exec.CommandContext` and returns the dereferenced commit SHA for annotated tags, the tag-object SHA for lightweight tags — evidence: Ginkgo unit tests in `pkg/git/` cover both an annotated-tag fixture and a lightweight-tag fixture; `make test` in `agent/github-releaser/` exits 0.
- [ ] Every error string the `LsRemote` impl wraps is passed through `redactToken` before being returned — evidence: Ginkgo unit test asserts no `x-access-token:` substring survives in the returned error when a faked subprocess emits a URL with token.
- [ ] The post-check helper in `steps_execution.go` runs on the success-return path AND on every failure-return path — evidence: Ginkgo integration test under `pkg/` exercises each `s.fail` call site (using existing test scaffolding) and asserts the post-check fires; on the success path, asserts the post-check fires after the existing `## Result(outcome=released)` write.
- [ ] When the faked `LsRemote` returns the agent's expected SHA, the task frontmatter is rewritten to `status: completed`, `phase: done`, and a `## Resolution` block is appended naming the verdict `released`, the planned version, and the observed SHA — evidence: integration test reads back the resulting task markdown and asserts frontmatter values + presence of the `## Resolution` block containing **the SHA string returned by the faked `LsRemote`** (not a hardcoded constant) AND **the planned version string from the agent's `## Plan` block**.
- [ ] When the faked `LsRemote` returns a different SHA, the task is closed as `completed` / `superseded` with the observed SHA in the `## Resolution` block — evidence: integration test asserts frontmatter + `## Resolution` content.
- [ ] When the faked `LsRemote` returns empty, the existing failure-path or success-path verdict stands unchanged — evidence: integration test asserts no frontmatter mutation beyond what the existing path writes, no `## Resolution` block.
- [ ] When the faked `LsRemote` returns an error, the existing verdict stands and a redacted error is logged — evidence: integration test asserts no frontmatter mutation; log capture contains the verdict `no-op-remote-error` log line and does not contain `x-access-token:` substring.
- [ ] Re-invoking the post-check on a task whose `status` is already `completed` or `aborted` produces no observable change to the task file — evidence: integration test runs the helper twice in succession; assert the task file's byte content after the second run equals the byte content after the first run (no double `## Resolution` block, no frontmatter re-rewrite).
- [ ] The post-check helper emits one structured log line per invocation naming `task_identifier`, planned version, observed remote SHA (or empty), and verdict from the set `{released, superseded, no-op-remote-empty, no-op-remote-error, no-op-already-terminal}` — evidence: Ginkgo unit test captures `glog` output (via existing test helpers) and asserts the log line for each branch.
- [ ] All existing `agent/github-releaser/pkg/...` Ginkgo tests continue to pass — evidence: `make test` in `agent/github-releaser/` exits 0.
- [ ] `make precommit` in `agent/github-releaser/` exits 0 — evidence: exit code.
- [ ] CHANGELOG.md in `agent/github-releaser/` has a new `## Unreleased` entry describing the post-check behavior — evidence: `grep -n 'post-check' agent/github-releaser/CHANGELOG.md` returns a line under `## Unreleased`.

No scenario AC. Unit + integration tests with a counterfeiter `GitOps` fake reach every branch (success, superseded, remote-empty, remote-error, idempotency, both tag shapes); no real Docker, no real `gh`, no real cluster behavior is exercised that isn't already covered by Rung 2 deploy verification in the Verification section. Adding a scenario here would re-test the same `LsRemote` seam slower.

## Verification

Per `docs/verifying-specs.md`:

**Rung 1 — local `run-once` plus `make test`:**

```
cd agent/github-releaser && make precommit
```

Expected: exit code 0. All new Ginkgo tests pass; mock regeneration is no-op against committed mocks.

Additional rung-1 check: run the agent's existing single-cycle entry point (e.g. `cmd/run-once` if present, or the existing local-trigger pathway) against three fixture task files derived from the validation set, with a faked `GitOps` returning each of the three relevant `LsRemote` cases:

- expected-SHA-matches: task closes with `## Resolution` verdict `released`
- different-SHA: task closes with `## Resolution` verdict `superseded`
- empty: task file is unchanged from the existing failure-path output
- idempotency: run twice on the already-closed task, assert single `## Resolution` block

Expected: each fixture's post-run task markdown matches the verdict in the Validation Fixtures table in the parent goal `[[Auto-Close Release Tasks When Superseded]]`.

**Rung 2 — dev cluster e2e:**

After the spec ships and master is at the autoRelease tag, deploy the new image to dev. Re-delegate the three closable fixtures (773f4ed, c00f363, e19e5fb) by setting `assignee: github-releaser-agent` on each — this refills the trigger budget and the controller spawns a fresh Job. Observe each task closes with the verdict named in the parent-goal Validation Fixtures table within one agent run.

Expected: three fixtures land in `status: completed`, `phase: done`, with `## Resolution` blocks recording the per-fixture verdict; df89963 stays open (expected — D-class, env-fix scope).

Evidence to capture:

```
grep -A 5 '^## Resolution' ~/Documents/Obsidian/OpenClaw/tasks/Release\ bborbe-go-skeleton\ 773f4ed.md
grep -A 5 '^## Resolution' ~/Documents/Obsidian/OpenClaw/tasks/Release\ bborbe-go-skeleton\ c00f363.md
grep -A 5 '^## Resolution' ~/Documents/Obsidian/OpenClaw/tasks/Release\ bborbe-go-skeleton\ e19e5fb.md
```

Each grep returns the verdict line + observed remote SHA.

**Rung 3 — prod:** one week of clean dev observation, then promote to prod via the standard deploy pattern. Prod rung-3 evidence: first prod release attempt after promotion either closes correctly (any verdict from the post-check set, with corresponding log line and `## Resolution` block) or is a clean genuine `failed` (no remote tag → existing failure-path behavior unchanged).

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Add `LsRemote` to `GitOps` interface + concrete impl in `os_exec_git_ops.go`; regenerate counterfeiter mock; unit tests cover annotated tag, lightweight tag, token redaction, argv-only invocation | — (interface seam, no observable agent behavior yet) | ACs 1, 2, 3, 4, 12, 13 | — |
| 2 | Wire post-check helper at the tail of execution step's `Run`; cover all `s.fail` call sites and the success-return path; idempotency guard at helper entry; structured log line for each verdict branch; CHANGELOG entry | 1, 2, 3, 4, 5, 6, 8 | ACs 5, 6, 7, 8, 9, 10, 11, 12, 13, 14 | prompt 1 |
| 3 | Adjust ai_review step so a review rejection that follows a confirmed remote tag at the expected SHA does not flip status to failed; rejection recorded as warning, task closes as completed | 7 | (covered by integration tests added in prompt 2 if the ai_review verdict-write site participates in the post-check; otherwise a small, targeted integration test added in prompt 3) | prompt 2 |

Rationale: prompt 1 builds the seam (interface + impl + mock) without touching behavior; prompt 2 ships the verdict-upgrade behavior on the execution-step tail; prompt 3 closes the "review-rejected but tag shipped" case (DB 7) which is a smaller, separable change on the ai_review side. Cycles avoided: prompt 2 depends on the mock landing in prompt 1, but prompt 1 produces no user-visible behavior so it can ship independently and be reverted cleanly if prompt 2 stalls. Prompt 3 is the smallest of the three and depends on the verdict-upgrade infrastructure being in place.

## Do-Nothing Option

If we don't ship this, the four currently stuck `bborbe/go-skeleton` release tasks remain in the human triage queue, and the same pattern keeps producing one to three stuck-task additions per repo per failed-release-burst. The cost is bounded (operator can run the stage-1 slash command `~/.claude/skills/release-task-close-on-shipped/` to sweep stuck tasks manually) but recurring and growing with repo count. Manual sweep is acceptable as a transitional state while this spec is in flight; it is not acceptable as the steady-state operating model — that contradicts the mechanical-resolution boundary principle the parent goal is built on.
