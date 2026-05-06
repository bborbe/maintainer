---
status: approved
approved: "2026-05-06T20:23:41Z"
branch: dark-factory/human-readable-filenames-for-pr-review-tasks
---

## Summary

- The github-pr watcher publishes `CreateTaskCommand` messages with deterministic UUID5 task identifiers; today the agent task controller writes the materialized vault file at `tasks/<uuid>.md`
- Operators triaging the vault see filenames like `a6f38ef6-c979-5c0b-9843-70e540d4bf35.md` — invisible without opening the file
- After this work, PR-review tasks land at `tasks/PR Review github - {owner}-{repo} - {number} - {slug}.md`, with the same UUID still in `task_identifier:` frontmatter
- Filename derivation happens at the watcher (publisher side) via the same `filename_hint` mechanism introduced by spec-018; controller validates and falls back to UUID when missing/invalid
- Existing UUID-named files in the vault are NOT auto-renamed; only new publishes get the readable name
- Format is provider-aware so future watchers (`watcher/bitbucket-pr`, `watcher/gitlab-pr`) drop in without filename collisions

## Problem

Same shape as spec-018 (build failures), different consumer. PR-review vault files use UUID5 from `(owner, repo, number)` — operators cannot scan a directory listing of `OpenClaw/tasks/` and identify which PR each task belongs to. As multiple watcher consumers (PR review + build failure + future) accumulate, the UUID soup gets worse.

The architectural decision (`filename_hint` field on `CreateTaskCommand`, controller-side validation + UUID fallback) is shared with spec-018; this spec adds the second producer.

## Goal

After this work ships, PR-review vault tasks have human-readable filenames the operator can scan from a `ls` listing alone, while the controller's task-lookup path (`FindTaskFilePath` keying on `task_identifier` UUID from frontmatter) is unaffected. Existing UUID-named files in the vault keep working without rename. The `filename_hint` mechanism reuses the contract introduced by spec-018, and the format is provider-aware so future watchers (`watcher/bitbucket-pr`, `watcher/gitlab-pr`) drop in without filename collisions and without controller code change.

## Desired Behavior

1. A new PR review on `bborbe/maintainer#2` produces a vault file named `PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never.md` (slug from PR title)
2. The same task's frontmatter contains the unchanged `task_identifier: <UUID>` — the controller's UUID-keyed lookup path works identically
3. A PR with an empty or unicode-only title produces `PR Review github - bborbe-maintainer - 2.md` (slug segment omitted, no trailing ` - `)
4. A `CreateTaskCommand` with no `filename_hint` field still produces a valid `<uuid>.md` file (backward compatibility)
5. A `CreateTaskCommand` with a `filename_hint` containing `..` or `/` produces the UUID-named file and a WARN log; the publish is NOT dropped
6. Force-pushed PRs (which the watcher republishes via `UpdateFrontmatterCommand`) keep their original readable filename — the filename is set on first create only; subsequent updates target the same file
7. Existing UUID-named PR-review files in `OpenClaw/tasks/` are unaffected — no auto-rename, no orphans

## Filename Format

```
PR Review {provider} - {owner}-{repo} - {number} - {slug}.md
```

| Component | Source | Format |
|---|---|---|
| `PR Review` | constant | literal |
| `{provider}` | watcher service segment (today: hard-coded `github`; future watchers carry their own constant) | lowercase |
| `{owner}-{repo}` | parsed from the allowlist entry (`host/owner/repo` → drop host, replace `/` with `-`) | as-is from GitHub API; no slugification (today's bborbe repos are filesystem-safe) |
| `{number}` | PR number from the GitHub API | integer |
| `{slug}` | slugified PR title | lowercase, `[a-z0-9-]`, max 50 chars; empty/unicode-only title → omit segment + the leading ` - ` separator |

Concrete examples:
- `PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never.md`
- `PR Review github - bborbe-trading - 110 - fix-chromium-trixie.md`
- `PR Review github - bborbe-x - 7.md` (empty PR title — slug segment + separator omitted)

Future providers slot in:
- `PR Review bitbucket - team-svc - 42 - fix-auth-bug.md`
- `PR Review gitlab - org-repo - 99 - bump-deps.md`

### Slug rules

- Source: `pr.Title` from the GitHub API
- Lowercase
- Replace any non-`[a-z0-9]` character (including spaces, punctuation, unicode) with `-`
- Collapse runs of `-` to a single `-`
- Trim leading/trailing `-`
- Truncate to max 50 characters; if truncation would leave a trailing `-`, trim once more (no ellipsis)
- If the result is empty (title was unicode-only or all whitespace), omit the slug segment AND the leading ` - ` separator

## Cross-repo dependency direction

This change spans two repos: the watcher (this repo, `bborbe/maintainer`) and the agent task controller (`bborbe/agent`). The `filename_hint` controller-side handling is shared with spec-018 (build failures); both producers land via that one shared controller change.

**Sequencing**:
1. **Spec-018 ships first** (per its own dependency direction): watcher emits `filename_hint` for build failures; controller honors it. This unlocks the `filename_hint` plumbing across both repos.
2. **This spec ships after**: PR watcher emits `filename_hint` for PR reviews. No new controller change — controller already honors any `filename_hint` field shape after spec-018's controller-side work lands.

If this spec's prompt approves before spec-018's controller-side change has landed in `bborbe/agent`, the watcher still emits the hint safely (existing controllers ignore unknown JSON fields), and PR-review files land at the legacy UUID-named path until the controller catches up. No data loss, just a delayed rollout.

**Sequencing rule**: do NOT approve this spec's watcher-side prompt until spec-018's watcher-side prompt has landed. Avoids two prompts editing shared `agentlib`/`cqrs` types concurrently.

## Non-goals

- Changing the build-failure task filename — separate spec `human-readable-filenames-for-build-tasks` (spec-018), uses the same `filename_hint` mechanism with a different format
- Auto-renaming existing UUID-named PR-review files in the vault — leave them; only new publishes get readable names
- Including additional context in the filename (assignee, draft status, branch name, author) — body of the markdown carries those; filename stays compact
- Slugifying the repo name beyond the simple `/` → `-` substitution — bborbe's repos are already filesystem-safe; if a future repo has unsafe characters, add slugification then
- Adding the slug to the `UpdateFrontmatterCommand` payload — the controller already locates by `task_identifier` UUID; updates don't need to know the file's display name
- Validating that two different PRs never collide on the slug — same repo + same number is unique by construction; truncation collisions on the slug fall back to the same file (controller's UUID lookup still distinguishes them in frontmatter)

## Constraints

- The PR watcher MUST emit `filename_hint` ONLY in `CreateTaskCommand`; `UpdateFrontmatterCommand` MUST NOT carry the hint (the controller targets the existing file by UUID and the file's display name is set once at create time)
- The `filename_hint` value MUST NOT include the `.md` extension; the controller appends it
- Same controller-side validation as spec-018 applies: no `/`, no `..`, length cap 200, on validation failure fall back to UUID-named file (don't drop the publish)
- Slug truncation MUST NOT introduce ellipsis or hash markers; just hard-truncate at the last word boundary or character, whichever is simpler
- `task_identifier: <UUID>` in frontmatter is unchanged; controller's `FindTaskFilePath` still keys on this — filename change is purely cosmetic
- The Kafka command schema change MUST be backward compatible: existing controllers without the field handler MUST still process the message correctly (use UUID name) — this constraint is satisfied trivially because spec-018 already lands the controller-side change
- `make precommit` clean in `watcher/github-pr/`

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `filename_hint` field absent in command | Controller writes `tasks/<uuid>.md` (legacy behavior) | none — backward compat |
| `filename_hint` contains `/` or `..` | Controller logs WARN, writes `tasks/<uuid>.md` | watcher fix; this is a hard validation barrier |
| `filename_hint` length > 200 chars | Controller logs WARN, writes `tasks/<uuid>.md` | watcher fix; should be impossible given 50-char slug cap |
| PR title is empty or whitespace-only | Slug segment + ` - ` separator omitted; filename ends with `... - {number}.md` | n/a — by design |
| PR title is unicode-only (e.g. emoji-only) | After regex stripping, slug is empty → same as above | n/a — by design |
| Slug truncation cuts a word mid-character | Hard truncate; trim trailing `-`; accept the visual artifact | rare; truncation is a max-length safety, not a UX guarantee |
| PR title contains `..` or `/` | Slug regex strips them as part of the `[a-z0-9]` filter; the validation barrier never triggers from PR titles | by design |
| Two PRs same repo same number (impossible per GitHub) | n/a | n/a |
| Force-pushed PR (`UpdateFrontmatterCommand`) | Controller locates the file by UUID and updates frontmatter in place; filename unchanged from original create | unchanged |

## Acceptance Criteria

- [ ] A PR review on `bborbe/maintainer#N` produces vault file `PR Review github - bborbe-maintainer - N - {slug}.md` (assert by `ls` after the publish completes)
- [ ] A PR with an empty/unicode-only title produces `PR Review github - bborbe-maintainer - N.md` (no trailing ` - `)
- [ ] The same task's frontmatter contains the unchanged `task_identifier: <UUID>`
- [ ] A `CreateTaskCommand` with no `filename_hint` field still produces a valid `<uuid>.md` file (backward compatibility)
- [ ] A `CreateTaskCommand` with a `filename_hint` containing `..` or `/` produces the UUID-named file and a WARN log; the publish is NOT dropped
- [ ] A force-pushed PR (`UpdateFrontmatterCommand`) updates the existing readable-named file in place; no new file is created
- [ ] Existing UUID-named PR-review files in `OpenClaw/tasks/` are unaffected — no auto-rename, no orphans
- [ ] CHANGELOG entry under `## Unreleased`
- [ ] `make precommit` clean from `watcher/github-pr/`
- [ ] Slug unit tests cover: lowercase, special-char stripping, multi-`-` collapse, leading/trailing `-` trim, truncation at 50 chars, empty-after-strip → omit segment
- [ ] After dev deploy + trigger via a real PR, the new vault task lands at the readable filename (rung-2 verification per `docs/verifying-specs.md`)
- [ ] No e2e scenario file added in this repo (cross-repo `filename_hint` honor + validation coverage shipped by spec-018's `bborbe/agent` follow-up; this spec's tests cover only the PR-watcher emit side)

## Verification

```bash
cd watcher/github-pr && make precommit
```

Live verification (rung 2):

```bash
# Trigger a real PR review
curl -s -X POST http://<pr-watcher-svc>:9090/trigger
# Open or push a PR on bborbe/maintainer
# Wait one poll cycle (≤ 5 min)

# Confirm the readable filename exists in the vault repo:
cd ~/Documents/Obsidian/OpenClaw && git pull --quiet
ls "tasks/PR Review github - bborbe-maintainer - "*

# Confirm UUID still in frontmatter:
grep "task_identifier:" "tasks/PR Review github - bborbe-maintainer - "*
```

## Do-Nothing Option

Keep UUID filenames for PR-review tasks. Cost: same triage friction as the build-failure case, scales linearly with the number of open PRs the watcher tracks. Symmetric to spec-018; if 018 ships and this doesn't, the vault has a mixed UUID + readable layout for different task types — strictly worse than either fully-readable or fully-UUID. Ship together (or this directly after 018) for consistency.

## Related

- Companion spec (build failures, ships first): `human-readable-filenames-for-build-tasks` (spec-018)
- Builds on: spec-018's controller-side `filename_hint` handler in `bborbe/agent`
- Pipeline overview: `docs/architecture.md`
- PR-watcher decision chains: `docs/watcher-decision-chains.md`
- Verification ladder: `docs/verifying-specs.md`
- Cross-repo: `bborbe/agent/task/controller/pkg/command/task_create_task_executor.go` (where the `filename_hint` honor logic lives — shipped by spec-018)
