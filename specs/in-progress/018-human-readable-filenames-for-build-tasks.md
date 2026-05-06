---
status: generating
approved: "2026-05-06T20:18:23Z"
generating: "2026-05-06T20:18:24Z"
branch: dark-factory/human-readable-filenames-for-build-tasks
---

## Summary

- The github-build watcher publishes `CreateTaskCommand` messages with deterministic UUID5 task identifiers; today the agent task controller writes the materialized vault file at `tasks/<uuid>.md`
- Operators triaging the vault see filenames like `fe5d20b3-e6ae-54e0-9a7e-26ae48b160be.md` — invisible without opening the file
- After this work, build-failure tasks land at `tasks/Build Failure github - {owner}-{repo} - {sha7}.md`, with the same UUID still in `task_identifier:` frontmatter
- Filename derivation happens at the watcher (publisher side); the controller honors a `filename_hint` in the command and falls back to the existing UUID-based name when the hint is missing or invalid
- Existing UUID-named files in the vault are NOT auto-renamed; only new publishes get the readable name
- Format is provider-aware so future watchers (`watcher/bitbucket-build`, `watcher/gitlab-build`) drop in without filename collisions

## Problem

`OpenClaw/tasks/` accumulates one file per build episode. Today every entry is a UUID5 — there is no way to scan the directory listing and answer "which repo broke" without opening files or grepping frontmatter. As the watcher rolls out across more repos and the auto-fix loop scales, this gets worse linearly.

The PR-reviewer-agent task `[[Human-readable Filenames for Vault Tasks]]` (in the personal vault) captures the same operator pain for PR review tasks. This spec covers the build-failure half. The architectural decision (where the filename is computed, how the controller honors a hint) should be the same; if/when the PR-review task ships first, this work should consume the same `filename_hint` extension point.

## Goal

After this work ships, build-failure vault tasks have human-readable filenames the operator can scan from a `ls` listing alone, while the controller's task-lookup path (`FindTaskFilePath` keying on `task_identifier` UUID from frontmatter) is unaffected. Existing UUID-named files in the vault keep working without rename.

## Desired Behavior

1. A new build failure on `bborbe/maintainer` produces a vault file named `Build Failure github - bborbe-maintainer - 5886450.md` (sha7 is the first 7 chars of `episode_sha`)
2. The same task's frontmatter contains the unchanged `task_identifier: <UUID>` — the controller's UUID-keyed lookup path works identically
3. A `CreateTaskCommand` with no `filename_hint` field still produces a valid `<uuid>.md` file (backward compatibility for any legacy publisher)
4. A `CreateTaskCommand` with a `filename_hint` containing `..` or `/` produces the UUID-named file and a WARN log; the publish is NOT dropped
5. Existing UUID-named files in `OpenClaw/tasks/` are unaffected — no auto-rename, no orphans
6. Future provider watchers (`watcher/bitbucket-build`, `watcher/gitlab-build`) drop in by emitting their own provider segment in the hint — no controller code change needed

## Filename Format

```
Build Failure {provider} - {owner}-{repo} - {sha7}.md
```

| Component | Source | Format |
|---|---|---|
| `Build Failure` | constant | literal |
| `{provider}` | watcher service segment (today: hard-coded `github`; future watchers carry their own constant) | lowercase |
| `{owner}-{repo}` | parsed from the allowlist entry (`host/owner/repo` → drop host, replace `/` with `-`) | as-is from GitHub API; no slugification (today's bborbe repos are filesystem-safe) |
| `{sha7}` | first 7 chars of `episode_sha` | matches git's default short-hash |

Concrete example: `Build Failure github - bborbe-maintainer - 5886450.md`

Future providers slot in:
- `Build Failure bitbucket - team-svc - a1b2c3d.md`
- `Build Failure gitlab - org-repo - 9f8e7d6.md`

## Cross-repo dependency direction

This change spans two repos: the watcher (this repo, `bborbe/maintainer`) and the agent task controller (`bborbe/agent`). Ship in **two separate PRs in this fixed order**:

1. **Watcher first**: emit `filename_hint` in `CreateTaskCommand`. Existing controllers ignore unknown JSON fields by default (`encoding/json` permissive default), so this is a safe no-op until step 2. Verify by deploying the watcher and confirming the field appears in published Kafka messages while existing controller behavior is unchanged.
2. **Controller second** (in `bborbe/agent` repo): honor the field. Includes the validation (no `/`, no `..`, length cap) and fallback to UUID. Ship as a separate spec in that repo. Verify rung-2 in dev: new build failure → readable filename in vault.

This ordering means a half-deployed state (new watcher, old controller) is safe — files keep their UUID names until the controller catches up.

## Non-goals

- Changing the PR-reviewer task filename — that's tracked in `[[Human-readable Filenames for Vault Tasks]]` (personal vault); this spec only ships the build-failure half. Coordinate the underlying `filename_hint` plumbing change across both consumers
- Auto-renaming existing UUID-named files in the vault — leave them; only new publishes get readable names. A separate cleanup script can rename in bulk later if desired
- Encoding additional context (commit message, workflow names, timestamps) into the filename — body of the markdown carries those; filename stays compact
- Slugifying the repo name beyond the simple `/` → `-` substitution — bborbe's repos are already filesystem-safe; if a future repo has unsafe characters, add slugification then
- Validating that two different episodes never collide on `sha7` — a 7-char hex prefix has 16M values per repo; collisions would produce two episodes both legitimately needing tasks, and the UUID5 in frontmatter still distinguishes them. The filename ambiguity is acceptable and the controller's `FindTaskFilePath` (UUID-based) handles it

## Constraints

- The build watcher MUST emit `filename_hint` in EVERY `CreateTaskCommand`; absent hint = controller falls back to UUID-named file (preserves backward compat for any legacy publisher that hasn't been updated)
- The `filename_hint` value MUST NOT include the `.md` extension; the controller appends it
- The hint MUST NOT contain `/`, `..`, or any path traversal sequence — controller MUST validate and reject hints that escape the `tasks/` directory; on validation failure, fall back to UUID-named file (don't drop the publish)
- Maximum hint length: 200 characters (filesystem limits; longer hints get truncated, which would alias different episodes onto the same file). Watcher enforces at emit time
- `task_identifier: <UUID>` in frontmatter is unchanged; controller's `FindTaskFilePath` still keys on this — filename change is purely cosmetic
- The Kafka command schema change (adding `filename_hint`) MUST be backward compatible: existing controllers without the field handler MUST still process the message correctly (use UUID name)
- `make precommit` clean in both `watcher/github-build/` and any agent component touched
- Coordinate with `bborbe/agent` repo — the controller's `task_create_task_executor` is in that repo; this spec implies a corresponding change there. If that's a single PR or two, document the dependency direction (watcher can ship the hint first; controller honoring it ships separately)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `filename_hint` field absent in command | Controller writes `tasks/<uuid>.md` (legacy behavior) | none — backward compat |
| `filename_hint` contains `/` or `..` | Controller logs WARN, writes `tasks/<uuid>.md` | watcher fix; this is a hard validation barrier |
| `filename_hint` length > 200 chars | Controller logs WARN, writes `tasks/<uuid>.md` | watcher fix |
| Two episodes hash-collide on `sha7` | Both produce the same filename; one overwrites the other on disk; both UUIDs distinct in frontmatter | rare (2 episodes from same repo, hex prefix collision); operator reads frontmatter for task_identifier; UUID dedup at controller still prevents duplicate task creation |
| Episode SHA shorter than 7 chars (impossible for SHA-1, but defensive) | Use whatever length the SHA has; do not pad | n/a — git SHAs are always ≥40 chars |
| Repo name contains characters illegal on the vault filesystem (`:`, `*`, etc.) | Watcher slugifies (lowercase + `[a-z0-9-]`); same rule that PR-review filenames will use when they ship | watcher fix if a real repo name fails |

## Acceptance Criteria

- [ ] A build failure on `bborbe/maintainer` produces vault file `Build Failure github - bborbe-maintainer - {sha7}.md` (assert by `ls` after the publish completes)
- [ ] The same task's frontmatter contains the unchanged `task_identifier: <UUID>`
- [ ] A `CreateTaskCommand` with no `filename_hint` field still produces a valid `<uuid>.md` file (backward compatibility)
- [ ] A `CreateTaskCommand` with a `filename_hint` containing `..` or `/` produces the UUID-named file and a WARN log; the publish is NOT dropped
- [ ] Existing UUID-named files in `OpenClaw/tasks/` are unaffected — no auto-rename, no orphans
- [ ] The watcher's published payload schema documented in `docs/architecture.md` (in this repo) lists the new `filename_hint` field
- [ ] CHANGELOG entry under `## Unreleased` (in this repo)
- [ ] `make precommit` clean
- [ ] After dev deploy + cursor wipe + trigger, the new vault task lands at the readable filename (rung-2 verification per `docs/verifying-specs.md`)
- [ ] End-to-end scenario coverage for `filename_hint` honor + validation lives in the `bborbe/agent` follow-up spec; this spec's tests cover only the watcher emit side (counterfeit publisher + `cdb.CommandObjectSender` round-trip)

## Verification

```bash
cd watcher/github-build && make precommit
```

End-to-end (rung 2):

```bash
# Reset cursor + trigger publish
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- rm -f /data/cursor.json
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- wget -qO- http://localhost:9090/trigger

# Watch the controller create the file
kubectlquant -n dev logs agent-task-controller-0 --since=30s | grep "create-task: created"

# Confirm the readable filename exists in the vault repo:
cd ~/Documents/Obsidian/OpenClaw && git pull --quiet
ls "tasks/Build Failure github - bborbe-maintainer"*
```

## Do-Nothing Option

Keep UUID filenames. Cost: triage friction grows with the vault (today already painful at ~100 build/PR-review tasks; will be worse once the auto-fix loop ships and tasks accumulate). Filename change is a small, isolated piece of work — defer only if controller-side coordination is currently expensive.

## Related

- Companion spec (PR-review filenames, separate consumer of the same hint mechanism): `[[Human-readable Filenames for Vault Tasks]]` in personal vault
- Pipeline overview: `docs/architecture.md`
- Episode-SHA semantics: `docs/build-watcher.md`
- Verification ladder: `docs/verifying-specs.md`
- Cross-repo: `bborbe/agent/task/controller/pkg/command/task_create_task_executor.go` (where the `filename_hint` honor logic lives)
