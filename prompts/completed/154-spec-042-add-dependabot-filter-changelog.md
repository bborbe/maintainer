---
status: completed
spec: [042-github-build-watcher-filter-dependabot-graph-update]
container: maintainer-exec-154-spec-042-add-dependabot-filter-changelog
dark-factory-version: v0.169.0
created: "2026-05-24T21:30:00Z"
queued: "2026-05-24T20:37:11Z"
started: "2026-05-24T20:40:36Z"
completed: "2026-05-24T22:42:23Z"
branch: dark-factory/github-build-watcher-filter-dependabot-graph-update
---

<summary>
- Add a changelog entry under `## Unreleased` in root `CHANGELOG.md` documenting the new Dependabot graph-update workflow filter
- Entry uses `feat:` prefix (patch bump) since this is a false-positive bug fix
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
- This is a bug fix (false positive suppression) → prefix is `feat:`
- Format: `- feat(watcher/github-build): ...`
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

3. **Update `CHANGELOG.md`** — use the `Edit` tool to make a **minimal, surgical** change. Do NOT rewrite the file.

   **If `## Unreleased` does not exist** (current state per recent runs):
   - Use Edit to insert a new section. Match on the literal `## v0.26.8` line (the current first version section) and prepend a new section above it.
   - The Edit's `old_string` should be exactly: `## v0.26.8`
   - The Edit's `new_string` should be: `## Unreleased\n\n- feat(watcher/github-build): skip workflow runs named \`Graph Update:\` or \`Dependabot Updates\` (prefix match, case-sensitive) so Dependabot's internal graph-maintenance job failures (HTTP 503s) do not generate OpenClaw build-failure tasks. Real CI workflows on the same commits are unaffected.\n\n## v0.26.8`

   This guarantees ALL existing content above `## v0.26.8` (the `# Changelog` title, any preamble paragraphs, semver bullets) and ALL content below stays byte-identical.

   **If `## Unreleased` already exists**, use Edit to append the new bullet to it instead — match a unique anchor within the existing Unreleased section.

   **Never** read the whole CHANGELOG and write it back. Surgical Edit only.

4. **Verify the change** — confirm the entry appears exactly once:

   ```bash
   grep -n "Graph Update:\|Dependabot Updates" CHANGELOG.md
   ```

</requirements>

<constraints>
- Only edit `CHANGELOG.md` at repo root
- Do NOT commit — dark-factory handles git
- Prefix is `feat:` (not `feat:` or `chore:`) — this is a false-positive bug fix, patch bump
- Entry must be under `## Unreleased` — if the repo uses a different section convention (e.g. versioned headers only), follow the existing pattern from recent entries
- `make precommit` does NOT need to be run for this prompt — this is a pure doc/config change
</constraints>

<verification>
# Exactly one entry line for this change:
grep -cE '^- feat\(watcher/github-build\).*Graph Update' CHANGELOG.md
# Expected: 1

# Unreleased section header present:
grep -c '^## Unreleased' CHANGELOG.md
# Expected: 1

# Preamble preserved (if "All notable changes" was previously present, it remains):
grep -c 'All notable changes' CHANGELOG.md
# Expected: same count as before this prompt ran (0 or 1; whatever it was)
</verification>
