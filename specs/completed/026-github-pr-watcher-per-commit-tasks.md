---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-15T13:08:18Z"
generating: "2026-05-15T13:11:56Z"
prompted: "2026-05-15T13:29:32Z"
verifying: "2026-05-15T13:47:38Z"
completed: "2026-05-15T15:36:43Z"
branch: dark-factory/github-pr-watcher-per-commit-tasks
---

## Summary

- The github-pr-watcher currently spawns one review task per PR and mutates that same task on every new push (clears body, bumps `ref`, resets `trigger_count`, posts a force-push notice). The mutation path is fragile: stale headings left in the file can short-circuit the agent's planning phase, and operators routinely have to perform a manual reset dance to get a fresh review.
- This spec switches the spawn model to per-(PR, head SHA) tuples. Each commit produces its own task file; existing files for prior commits are immutable historical artifacts. There is no longer any code path that mutates a previously-spawned task on a new push.
- Filename and dedup key both gain a short-SHA segment. Filename: `PR Review github - <owner>-<repo> - <PR#> - <sha[:8]> - <slug>.md`. Dedup key: `<owner>/<repo>#<PR#>@<sha>` feeding the same UUID5 derivation as today.
- The "force-push outdated" mutation path is removed. A new commit naturally produces a new task that supersedes the old one by appearing later in the vault; the old task remains a valid review of the code that existed at that SHA.
- After this lands, "re-run the review on the same PR" stops being an operator workflow — pushing a commit IS the re-run trigger, and there is no manual reset to perform.

## Problem

The watcher's current spawn model conflates "the PR" with "the unit of review work". When a contributor pushes a new commit, the watcher mutates the existing task file in place: it clears the body, resets the trigger count, bumps the ref, and prepends a "## Outdated by force-push <oldSHA>" heading. This mutation is fragile in three ways. First, anything the agent wrote into the file during the previous round (plans, partial verdicts, stale headings) can survive the reset and confuse the next round's agent — this caused two "planning short-circuits to done" failures during the bborbe/coding#1 review session because empty `## Plan` headings tricked the agent's "plan already exists" detector. Second, the operator runbook for force-rerunning a review is a manual six-step reset that human operators get wrong. Third, the audit trail collapses: we cannot inspect what the agent decided about commit A after commit B has overwritten the file. Per-commit task files eliminate all three problems by construction.

## Goal

After this work, every push to a watched PR causes the watcher to spawn a brand-new task file whose filename and `task_identifier` both encode the head commit SHA. Re-spawn for an unchanged head SHA is a no-op (skip — never mutate). Re-spawn after a force-push or a new commit creates an additional file alongside the existing one; the existing file is never opened or mutated by the watcher again. The vault accumulates one task per (PR, SHA) tuple, providing an immutable audit trail of every review round. The "re-run on the same PR" operator workflow disappears: pushing a commit IS the trigger.

## Non-goals

- Changes to the pr-reviewer agent itself (verdict logic, planning phase, output format) — covered by the sibling binary-verdict spec.
- Garbage-collecting, archiving, or deduplicating old per-commit task files. Vault history is cheap; if cleanup is needed later it is a separate concern with its own retention policy.
- Bitbucket Server or any non-GitHub PR source. Same logic should eventually apply, but this spec stays GitHub-only; Bitbucket can inherit via a follow-up.
- Backfilling per-commit task files for already-reviewed PRs. The change applies forward only.
- Cursor schema or `HeadSHAs` map redesign. The cursor's job — remembering "have I already published for this (PR, SHA)?" — is preserved; only the dedup key it stores changes shape.
- Posting per-commit review verdicts as separate GitHub PR review submissions. The poster keeps its current behavior; one task → one review posted.

## Desired Behavior

1. **Filename encodes the head SHA.** Every spawned task file's title contains a short-SHA segment positioned between the PR number and the slug. With a non-empty slug the title shape is `PR Review github - <owner>-<repo> - <PR#> - <sha[:8]> - <slug>`. With an empty slug it is `PR Review github - <owner>-<repo> - <PR#> - <sha[:8]>`. The short-SHA segment is the first 8 characters of the lowercase hex head SHA.

2. **Dedup key encodes the head SHA.** The deterministic UUID5 input string for `task_identifier` derivation is `<owner>/<repo>#<PR#>@<sha>` (full SHA, not truncated, to avoid collision risk in the dedup space). UUID5 derivation itself — the namespace UUID and the SHA1-based v5 algorithm — is unchanged. Two calls with the same `(owner, repo, number, sha)` tuple produce the same UUID; any change to any input produces a different UUID. The controller's existing dedup logic blocks duplicate spawns for the same SHA without any controller-side change.

3. **Idempotent spawn — same SHA is a no-op.** When the watcher polls a PR whose head SHA matches what it already spawned for (recorded in the cursor), it does not publish any command and does not touch the existing task file. This is observably the same as today's "no change, skipping" branch.

4. **New SHA spawns a new task — never mutates the old one.** When the watcher polls a PR whose head SHA differs from the one it last spawned for (new commit, force-push, or amend), it publishes a `CreateTaskCommand` for the new SHA. It does NOT publish any update or mutation command for the prior SHA's task. The prior task file remains untouched in the vault.

5. **Force-push outdated mutation path is removed.** The current `publishForcePush` flow (which mutates the existing task with a "## Outdated by force-push <oldSHA>" heading and resets `trigger_count`/`ref`) is deleted. Justification: in the per-SHA model the old task is a valid historical review of the code that existed at that SHA, and the new task naturally supersedes it by appearing later. There is no need to mark the old task as outdated. The "Outdated by force-push" heading and any code that writes it are gone end-to-end.

6. **Cursor remembers per-task-identifier SHAs as before.** The cursor's `HeadSHAs` map keeps its current shape (`taskID → SHA`), but every spawn now produces a distinct `taskID`, so over time the map grows with one entry per (PR, SHA) tuple of any open PR. Pruning of closed/merged PRs continues to work as it does today (entries not in the current open-PR batch are dropped on each poll cycle).

7. **Operator runbook drops the manual reset workflow.** The vault runbook `Create PR Review Agent Task.md` no longer documents a "Re-run on the same PR (force fresh agent run)" section. It documents the new behavior: pushing a commit produces a new task file automatically. There is no in-vault reset operation and no documented mechanism to re-run a review on the same SHA — by design, each (PR, SHA) gets exactly one review.

8. **Fail-closed on missing head SHA.** If the watcher cannot determine a PR's head commit SHA (API error, malformed response, missing field), it spawns no task for that PR on this poll. The watcher does not invent a placeholder, does not use the empty string, does not fall back to the parent commit. The PR is simply skipped; the next poll retries.

## Constraints

- **Filename grammar is the only canonical title shape.** The exact segment order is `PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]} - {slug}` (or the no-slug variant). The `maxTitleLen` truncation, slug truncation, and slugify rules are unchanged.
- **UUID5 namespace is frozen.** The fixed namespace UUID currently used for derivation (`prWatcherNamespace`) does not change. Only the input key string changes from `<owner>/<repo>#<number>` to `<owner>/<repo>#<number>@<sha>`. Changing the namespace would orphan every historical task; changing only the key shape is the intended one-way migration.
- **Wire format / Kafka schema is unchanged.** The watcher continues to publish `CreateTaskCommand` (and only `CreateTaskCommand` in this code path post-change). No new fields, no schema bump, no new command type. The `UpdateFrontmatterCommand` send for force-push outdated is removed but the command type itself remains in the agent library — other watchers may still use it.
- **Provider is hardcoded `github`.** This watcher is GitHub-only; the provider segment in the filename grammar stays the literal string `github`. Multi-provider support is a future spec.
- **Dedup key uses the FULL SHA, not the short SHA.** The filename uses `sha[:8]` for human readability; the UUID5 input uses the full 40-character SHA to keep the dedup keyspace collision-free. These two are independent on purpose — a hash-collision in `sha[:8]` would still produce distinct UUIDs.
- **Cursor format is unchanged.** `HeadSHAs map[string]string` keyed by task-identifier string remains. The set of task-identifier values it can hold simply expands (one per (PR, SHA) over time, scoped to open PRs).
- **Cursor cross-version compatibility (forward-only).** A cursor written by the old binary and read by the new binary contains task-identifiers for PR-only keys. The new binary will see these as "no entry for the new PR@SHA key" on the next poll, which means it will spawn a new task for the current head SHA — this is the correct, intended behavior. Old per-PR task files remain in the vault; they are not retroactively renamed.
- **Existing knowledge to reference**:
  - `~/Documents/workspaces/coding/docs/go-testing-guide.md` — Ginkgo `DescribeTable` for the filename grammar and UUID5 stability cases.
  - `~/Documents/workspaces/coding/docs/go-architecture-patterns.md` — interface boundaries; if the spawn path needs a "task already exists?" check, it is injected, not file-system-coupled.
  - `~/Documents/workspaces/maintainer/docs/architecture.md` — where this watcher sits in the pipeline (upstream of controller and executor).
  - `~/Documents/workspaces/maintainer/docs/verifying-specs.md` — Rung-2 procedure (dev k8s deploy + e2e push to test PR) is the appropriate verification level here; rung-1 alone is insufficient because the bug being fixed is reachable only via real vault state.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| New commit pushed to a watched PR | Watcher spawns a new task with a filename containing the new `sha[:8]`. Old task file is untouched. | Automatic |
| Force-push that rewrites history (new head SHA) | Same as above: new task spawned for the new SHA. Old task is preserved as a historical artifact of the pre-force-push commit. | Automatic |
| Watcher polls and head SHA matches the cursor entry | No command published, no file touched. | Automatic — same as today's "no change" branch |
| Two watcher pods race-publish for the same (PR, SHA) | Both produce identical `task_identifier` (UUID5 is deterministic). Controller dedup blocks the second create. Vault sees one file. | Automatic — controller's existing dedup |
| Watcher restart with empty cursor sees a PR whose head SHA already has a vault file | Watcher publishes `CreateTaskCommand`. Controller sees the same UUID5 task-identifier already exists in vault and rejects/dedups. Cursor entry repopulates on the next successful poll. | Automatic — controller dedup is the source of truth, cursor is an optimization |
| Watcher upgrades from old binary; cursor contains PR-only task-identifiers | New binary treats them as "unknown"; spawns a new task for the current head SHA at next poll. Vault accumulates one new file per open PR (one-time migration cost). Old per-PR task files are not renamed and are never touched again by the watcher. | Automatic; expected one-time spawn burst on first poll after deploy |
| Operator wants to re-run a review on the same SHA | Not supported and not documented. Each (PR, SHA) gets exactly one review; the result is the result. If the agent itself was upgraded and you want fresh verdicts on existing PRs, that is a separate operational concern outside the watcher's contract. | None — by design |
| Old "Outdated by force-push" heading appears on a historical task in the vault | These were written by the previous version of the watcher. The new watcher never writes such headings. Existing files are not retroactively cleaned up. | None needed — visual artifact only, no behavioral impact |
| Vault grows large over time (many old per-commit task files) | Out of scope for this spec. Files remain searchable in the vault; the operator can manually archive or a follow-up spec can introduce retention. | Separate concern |

## Security / Abuse Cases

This spec touches the watcher's spawn path, which writes filenames derived from PR metadata (owner, repo, title) into the vault. The slugifier already constrains titles to `[a-z0-9-]` and truncates to 50 characters. The new SHA segment is the first 8 chars of a hex SHA — by construction in `[a-f0-9]`, no escaping needed, no path-traversal vector. There is no new HTTP surface, no new file-system path computation that depends on attacker-controlled input beyond what the current watcher already accepts. The dedup key (UUID5 input) does include the SHA, but UUID5 is a one-way hash; even a chosen SHA cannot collide a target UUID with feasible work. (The fail-closed behavior on missing head SHA is captured as Desired Behavior #8 below — it is observable, not a security note.)

## Acceptance Criteria

- [ ] `computePRTitle` (or its successor) returns titles with the SHA segment positioned exactly between PR number and slug, matching the grammar in Desired Behavior #1.
- [ ] `DeriveTaskID` (or its successor) accepts a SHA argument and produces UUID5 derived from `<owner>/<repo>#<number>@<sha>` (full SHA). Two calls with identical inputs produce identical UUIDs (covered by a stability test).
- [ ] When the watcher polls a PR with a previously-unseen head SHA, it publishes exactly one `CreateTaskCommand` whose `task_identifier` is the new UUID5 and whose title contains `sha[:8]`.
- [ ] When the watcher polls a PR whose head SHA matches the cursor entry, it publishes no command (same observable behavior as today's no-change branch).
- [ ] When the watcher polls a PR whose head SHA differs from the cursor entry, it publishes a `CreateTaskCommand` for the new SHA. It does NOT publish `UpdateFrontmatterCommand`. The cursor records the new (taskID, SHA) pair; the prior taskID's cursor entry persists until the standard open-PR-batch pruning step removes it on a subsequent poll (closed-PR cleanup is unchanged by this spec). The prior taskID is never used to address an in-vault file again by the watcher.
- [ ] The `publishForcePush` function and the `## Outdated by force-push` heading string are removed from the production code path. No remaining call sites; no remaining references in tests.
- [ ] Existing tests that asserted force-push mutation behavior are deleted or rewritten to assert "force-push produces a new task, leaves the old one alone".
- [ ] Filename test (`filename_internal_test.go`) asserts the SHA segment in the correct position for both slug and no-slug variants.
- [ ] New test cases exercise: (a) new SHA → new file created with expected name and UUID; (b) same SHA → no command published; (c) different SHA from same PR → new file, distinct UUID, no mutation of prior; (d) UUID5 stability across two calls with identical inputs.
- [ ] Vault runbook `Create PR Review Agent Task.md` no longer contains the "Re-run on the same PR (force fresh agent run)" reset workflow. It documents the new per-commit behavior. It does NOT document any mechanism for re-running a review on the same SHA — by design, each (PR, SHA) gets exactly one review.
- [ ] CHANGELOG has an `## Unreleased` entry describing the per-(PR, SHA) spawn model and the removal of the force-push mutation path.
- [ ] `make precommit` in `watcher/github-pr/` passes (format, generate, test, lint, license).
- [ ] **No new automated scenario test.** Per `docs/scenario-writing.md` and `docs/verifying-specs.md`, this spec is verified at Rung-2 by a manual e2e check (push to a real test PR, observe two coexisting task files in OpenClaw vault) plus comprehensive unit tests on the spawn-decision and filename/UUID5 layers. The behavior the unit tests cannot reach (real vault file persistence under controller dedup) is reachable only via real cluster state, not via a pre-built scenario harness.

## Verification

```
cd watcher/github-pr && make precommit
```

Expected: all targets pass. Test output should include the new filename grammar cases (with and without slug, both containing the SHA segment), the new UUID5 stability case, and the rewritten spawn-decision cases (new SHA → create, same SHA → noop, different SHA → create + no mutation).

**Rung-2 manual verification (post-deploy, NOT part of automated `make precommit`):**

The rung-selection table in `docs/verifying-specs.md` would call this Rung-1 (`pkg/`-only change, no manifest, no env, no new remote API). We elevate to Rung-2 because the bug being fixed (mutate-vs-create on new SHA) only manifests against a populated vault under controller dedup — unit tests cannot reach that interaction surface.

1. Deploy to dev per `docs/verifying-specs.md` Rung 2 instructions.
2. Push a new commit to an open test PR in an allowlisted repo.
3. Observe in dev k8s logs: watcher publishes `CreateTaskCommand`; controller consumes it.
4. Inspect `~/Documents/Obsidian/OpenClaw/tasks/` (or the dev vault equivalent): a new file matching `PR Review github - <owner>-<repo> - <PR#> - <new-sha[:8]> - *.md` appears.
5. Confirm the prior commit's task file (with the older `sha[:8]`) still exists, untouched (mtime unchanged from before the push).
6. Push a second commit; repeat. Confirm three coexisting task files for the three SHAs of the PR.

## Do-Nothing Option

Keep the mutate-in-place spawn model. Operators continue to perform manual six-step resets when a re-run is needed; the watcher continues to clobber prior review state on every push; the planning-short-circuit bug class remains reachable. The two failures already observed during the bborbe/coding#1 session are evidence that this is not a theoretical risk — it has fired in production-equivalent conditions.

A weaker alternative would be to keep the mutation path but stop writing the "Outdated by force-push" heading and explicitly clear all stale headings on reset. This would require enumerating every heading the agent might leave behind and is a permanent maintenance burden whenever the agent's output format evolves. The per-SHA model removes the entire problem by making the file immutable from the watcher's perspective. Not recommended.

## Verification Result

**Verified:** 2026-05-15T15:36:11Z (HEAD d7586ee)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory (v0.156.1-1-g04f3863-dirty)
**Scenario:** Rung-2 manual e2e — deployed to dev+prod via `make buca`; pushed two empty commits to `bborbe/maintainer#2` (`delete-this-pr-never` branch); prod watcher (`maintainer-watcher-github-pr-0`) spawned two coexisting per-(PR, SHA) task files.
**Evidence:**
- Two coexisting vault task files in `~/Documents/Obsidian/OpenClaw/tasks/`: `PR Review github - bborbe-maintainer - 2 - 80226917 - test-delete-this-pr-never.md` (task_identifier `bf535ba9-8910-553b-933b-dc1d5ebf95fb`) and `PR Review github - bborbe-maintainer - 2 - 19c513b8 - test-delete-this-pr-never.md` (task_identifier `3e8acd8b-585d-5a73-a456-10730d995f17`) — distinct SHAs → distinct UUID5 task_identifiers.
- First file untouched by watcher after second commit (mtime moved only via agent's own task processing).
- Code grep `grep -rn "publishForcePush\|Outdated by force-push" watcher/github-pr/pkg/` → 0 matches (force-push mutation path removed).
- `make precommit` in `watcher/github-pr/`: PASS (gosec 0 issues, trivy clean, lint+test+license green, "ready to commit").
- Vault runbook `~/Documents/Obsidian/Personal/65 Runbooks/Create PR Review Agent Task.md` contains "Per-commit spawn — no re-run mechanism" section explicitly stating "no documented mechanism for re-running a review on the same SHA — by design, each (PR, SHA) gets exactly one review"; troubleshooting row updated to "Push a new commit to the PR — watcher spawns a fresh task per (PR, SHA) on the next poll".
**Verdict:** PASS
