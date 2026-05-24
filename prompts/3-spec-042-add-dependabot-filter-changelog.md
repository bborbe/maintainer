---
status: draft
spec: [042-github-build-watcher-filter-dependabot-graph-update]
created: "2026-05-24T21:30:00Z"
branch: dark-factory/github-build-watcher-filter-dependabot-graph-update
---

<summary>
- Add a changelog entry under `## Unreleased` in root `CHANGELOG.md` documenting the new Dependabot graph-update workflow filter
- Entry uses `fix:` prefix (patch bump) since this is a false-positive bug fix
- Entry is specific: names the filtered prefixes, the package, and the effect
</summary>

<objective>
Update the root `CHANGELOG.md` to document that `watcher/github-build` now filters out Dependabot internal graph-update workflows from build-failure task emission.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `changelog-guide.md` in the coding plugin docs (`/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`).

Files to read fully before making changes:
- `CHANGELOG.md` — confirm whether `## Unreleased` already exists; understand existing entry format and prefix conventions

Key facts:
- This is a bug fix (false positive suppression) → prefix is `fix:`
- Format: `- fix(watcher/github-build): ...`
- The repo has a `## Unreleased` section at the top
</context>

<requirements>

**Execute steps in order.**

1. **Confirm prompts 1 and 2 are deployed** — verify the filter implementation and tests exist:

   ```bash
   grep -n "DependabotGraphUpdatePrefixes\|isDependabotGraphUpdateWorkflow" watcher/github-build/pkg/watcher.go
   grep -n "Graph Update:\|Dependabot Updates" watcher/github-build/pkg/watcher_test.go
   ```

   If either is missing, STOP and report `status: failed` with `"Implementation not yet deployed (prompts 1 and 2)"`.

2. **Read `CHANGELOG.md`** — check if `## Unreleased` exists at the top. The changelog structure from recent runs is:

   ```
   # Changelog

   ## v0.26.8
   - chore: ...

   ## v0.26.7
   ...
   ```

   Note: the existing CHANGELOG does NOT have an `## Unreleased` section at the top. If there is no `## Unreleased`, create one. If there is one, append to it.

3. **Update `CHANGELOG.md`**

   If `## Unreleased` does not exist, prepend it:

   ```markdown
   # Changelog

   ## Unreleased

   - fix(watcher/github-build): skip workflow runs named `Graph Update:` or `Dependabot Updates` (prefix match, case-sensitive) so Dependabot's internal graph-maintenance job failures (HTTP 503s) do not generate OpenClaw build-failure tasks. Real CI workflows on the same commits are unaffected.

   ## v0.26.8
   ...
   ```

   If `## Unreleased` already exists, append the entry to it (after any existing bullets).

4. **Verify the change** — confirm the entry appears exactly once:

   ```bash
   grep -n "Graph Update:\|Dependabot Updates" CHANGELOG.md
   ```

</requirements>

<constraints>
- Only edit `CHANGELOG.md` at repo root
- Do NOT commit — dark-factory handles git
- Prefix is `fix:` (not `feat:` or `chore:`) — this is a false-positive bug fix, patch bump
- Entry must be under `## Unreleased` — if the repo uses a different section convention (e.g. versioned headers only), follow the existing pattern from recent entries
- `make precommit` does NOT need to be run for this prompt — this is a pure doc/config change
</constraints>

<verification>
grep "Graph Update:\|Dependabot Updates" CHANGELOG.md
# Expected: one line under ## Unreleased

grep -n "## Unreleased" CHANGELOG.md
# Expected: confirmed at the top of the file
</verification>