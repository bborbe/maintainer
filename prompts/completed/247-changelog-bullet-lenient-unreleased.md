---
status: failed
spec: [065-github-releaser-agent-lenient-unreleased]
container: maintainer-agent-lenient-exec-247-changelog-bullet-lenient-unreleased
dark-factory-version: v0.175.0
created: "2026-06-08T15:09:00Z"
queued: "2026-06-08T15:37:26Z"
started: "2026-06-08T15:52:10Z"
completed: "2026-06-08T15:53:47Z"
branch: dark-factory/github-releaser-agent-lenient-unreleased
lastFailReason: 'validate completion report: completion report status: partial'
---

<summary>

- A single `fix:` bullet is added to root `CHANGELOG.md` under a new `## Unreleased` section (currently absent), describing the agent-side parity with the watcher's lenient rule (spec 064)
- The bullet text names the agent package touched (`agent/github-releaser`), the structural change (first non-version H2 = unreleased section), the accepted variants (`## unreleased`, `## Unreleased changes`, `## WIP`, `## Next`), and the originating spec (`spec 065` / parity with `spec 064`) — dark-factory reads the prefix to determine the version bump, and the conventional prefix is `fix:` (this closes a silent-failure bug class, not a new feature)
- The pre-release CHANGELOG block is preserved: the new `## Unreleased` H2 sits between the existing SemVer preamble (ending at the PATCH bullet) and the current `## v0.35.1` release section. No preamble line is moved, deleted, or modified. No existing `## vX.Y.Z` section is touched
- This prompt depends on `lenient-unreleased-section-detection.md` having shipped first (the bullet describes the code change that prompt delivered); the prompt refuses to run if that prompt's CHANGELOG bullet is not yet in the file

</summary>

<objective>

Record the spec-065 lenient-unreleased change in the root `CHANGELOG.md` as a single `fix:` bullet under `## Unreleased`, so the next release cut by dark-factory includes it in the changelog automatically. The bullet text mirrors the spec-064 watcher bullet (which sits one release below at `## v0.35.1`) but is scoped to the agent layer and points to spec 065.

</objective>

<context>

Read `/workspace/CLAUDE.md` for project conventions. Read these files fully BEFORE editing:

- `/workspace/CHANGELOG.md` — the file to edit. Today the structure is:
  - Lines 1-9: SemVer preamble (`# Changelog` title, "All notable changes..." line, SemVer link, MAJOR/MINOR/PATCH bullets). THIS BLOCK IS FROZEN — do NOT modify, move, or insert anything above or inside it.
  - Line 10: blank line.
  - Line 11: `## v0.35.1` — the most recent release (the watcher's spec-064 bullet). DO NOT modify.
  - Lines 12-14: the spec-064 bullet body.
  - Line 15: blank line.
  - Line 16: `## v0.35.0` — the previous release. DO NOT modify.

  The current file has NO `## Unreleased` section. You must INSERT one between the preamble and `## v0.35.1`. The insertion site is between line 9 (last preamble line, `* PATCH version...`) and line 11 (current `## v0.35.1`).

- Sibling spec 064 bullet (line 13, verbatim — use as the style template):

  ```
  - fix(watcher/github-release): lenient unreleased-section detection — the first H2 that is not a version header (vX.Y.Z / X.Y.Z) is now treated as the unreleased section, so "## unreleased", "## Unreleased changes", "## WIP", and similar variants release correctly instead of silently producing no task (spec 064)
  ```

  The new bullet follows the same shape but is scoped to the agent:

  ```
  - fix(agent/github-releaser): lenient unreleased-section detection — the first H2 that is not a version header (vX.Y.Z / X.Y.Z) is now treated as the unreleased section, so "## unreleased", "## Unreleased changes", "## WIP", and "## Next" release correctly end-to-end (watcher publishes the Kafka task, agent validates + cuts the tag) instead of silently halting at the planning step (parity with spec 064, spec 065)
  ```

- The `## Unreleased` section may already exist if the previous spec in this release cycle added a different bullet there. If it does, append this bullet after the existing ones (do NOT replace them). If it does not, insert the section.

- The `lenient-unreleased-section-detection.md` prompt (sibling) adds the code change. This prompt's `## Unreleased` bullet records the user-facing impact. They are the two halves of a single release's worth of work — the spec 064 release added a watcher bullet (under `## v0.35.1`); spec 065 adds the agent bullet (under `## Unreleased`).

Coding plugin guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — the `## Unreleased` H2 sits between preamble and `## vX.Y.Z`. Preamble is frozen. Conventional prefix is required (`fix:` is correct here — this is a bug fix to silent-failure behavior, not a new feature). Newest version at top — the new `## Unreleased` is the newest section, sits directly above `## v0.35.1`.

</context>

<requirements>

1. **Insert a `## Unreleased` section into root `/workspace/CHANGELOG.md` and add a single `fix:` bullet.** The file currently has NO `## Unreleased` heading. After the edit, the file structure MUST be:

   ```
   ... (lines 1-9 preamble unchanged) ...

   ## Unreleased

   - fix(agent/github-releaser): lenient unreleased-section detection — the first H2 that is not a version header (vX.Y.Z / X.Y.Z) is now treated as the unreleased section, so "## unreleased", "## Unreleased changes", "## WIP", and "## Next" release correctly end-to-end (watcher publishes the Kafka task, agent validates + cuts the tag) instead of silently halting at the planning step (parity with spec 064, spec 065)

   ## v0.35.1
   ... (rest unchanged) ...
   ```

   Concretely, insert three lines between the existing line 9 (last PATCH bullet) and the existing line 11 (current `## v0.35.1`):
   - One blank line (preserves the existing 1-line gap between sections).
   - `## Unreleased` (the H2 heading).
   - One blank line.
   - The bullet line above.
   - One blank line (preserves the 1-line gap between the new section and the next).

   The `## v0.35.1` line (currently line 11) shifts down by 5 lines. The bullet text wording above is the canonical form — copy it verbatim, do NOT rewrite for length. The wording names: the agent package, the structural rule (first non-version H2 = unreleased), the four accepted variants, the end-to-end impact (watcher + agent parity), and the originating specs (spec 064 watcher, spec 065 agent).

2. **Do NOT touch the preamble (lines 1-9), do NOT modify any existing `## vX.Y.Z` section, do NOT add anything above the preamble.** The preamble is FROZEN per the changelog-guide rule `changelog/preamble-frozen` (MUST) — the spec 064 sibling prompt confirmed this and the same rule applies here.

3. **Do NOT add a `feat:` or `refactor:` prefix; use `fix:` exactly.** The change closes a silent-failure bug class (typo'd headings were silently halting the release at the planning step). This is a bug fix, not a new feature, not a refactor. dark-factory reads the prefix to determine the version bump — `fix:` → patch bump, `feat:` → minor bump. Wrong prefix = wrong version bump on the next release.

4. **If a `## Unreleased` section already exists in the file, append this bullet to the existing bullet list, do NOT replace the existing bullets.** The grep in step 1 of `<verification>` distinguishes the two cases:
   - No `## Unreleased` present → insert the section per req 1.
   - `## Unreleased` present but with no `agent/github-releaser` bullet yet → append a single new line with this bullet after the last existing bullet, separated by a blank line.
   - `## Unreleased` present with an `agent/github-releaser` bullet already → no change needed (the bullet is already recorded). This should not happen in practice (only one prompt per release touches this file), but the check is defensive.

5. **Precondition gate (enforced in `<verification>` step 0, NOT in the agent's runtime).** The verification block below now starts with a precondition grep that requires `func isVersionHeader` to exist in `agent/github-releaser/pkg/changelog/changelog.go`. If the sibling parser-refactor prompt has not shipped yet, the grep returns zero matches and the verification step exits non-zero → dark-factory marks the completion `partial` and the bullet is never recorded on a branch where the code doesn't yet exist. The agent doesn't need a runtime "refuse to run" branch — the precondition grep IS the gate.

</requirements>

<constraints>

- Spec § Acceptance Criterion: "Root `CHANGELOG.md` contains a single new bullet under `## Unreleased`: `- fix(agent/github-releaser): lenient unreleased-section detection (parity with watcher spec 064)`". The bullet text in req 1 above is a faithful expansion of this one-liner (it adds the four variant names + the end-to-end impact phrase + the spec 065 reference) — both are acceptable; the expanded wording is the version the spec author would write by hand given the option.
- Spec § Non-goals: do NOT touch `watcher/github-release/`, do NOT add a feature under a different prefix, do NOT touch any other `## vX.Y.Z` section.
- Do NOT commit — dark-factory handles git.
- Do NOT modify the preamble (lines 1-9). Do NOT move the `## v0.35.1` line's content — only its line number changes as a downstream effect of the insertion above.
- Coding plugin rule `changelog/preamble-frozen` (MUST) — preamble is canonical and untouchable.
- Coding plugin rule `changelog/conventional-prefix-required` (MUST) — every `## Unreleased` bullet must start with a conventional prefix. `fix:` is correct here.
- Sibling dependency: this prompt MUST NOT run before `lenient-unreleased-section-detection.md` has shipped (the bullet describes code that doesn't exist yet would be a lie). Use the `git log` check in req 5 as a hard gate.

</constraints>

<verification>

Run from `/workspace`:

0. **Precondition** — `grep -c '^func isVersionHeader' agent/github-releaser/pkg/changelog/changelog.go` must print `1`. If it prints `0`, the sibling parser-refactor prompt has not shipped yet — abort verification with non-zero exit so dark-factory marks completion `partial` and does NOT commit the CHANGELOG bullet. This is the real gate; req 5 documents the rationale.
1. `grep -c '^## Unreleased' CHANGELOG.md` — must print `1` (exactly one `## Unreleased` heading exists after the edit).
2. `grep -A 1 '^## Unreleased' CHANGELOG.md | head -5` — must show `## Unreleased` followed by a blank line followed by a line starting with `- fix(agent/github-releaser): lenient unreleased-section detection`.
3. `grep -n '^- fix(agent/github-releaser):' CHANGELOG.md` — must show exactly one match, on the line directly under `## Unreleased`.
4. `git diff CHANGELOG.md | head -20` — must show ONLY the insertion of `## Unreleased` + blank line + bullet + blank line. No preamble line is changed, no `## vX.Y.Z` section is changed.

Expected: all 5 checks pass. The grep counts in steps 0, 1, and 3 are exact — over-counts and under-counts are both failures.

</verification>

<success_criteria>

- A single new bullet exists in root `CHANGELOG.md` under `## Unreleased`, starting with `- fix(agent/github-releaser): lenient unreleased-section detection` (spec AC #15)
- The preamble (lines 1-9) is byte-identical to its pre-edit content
- The `## v0.35.1` section (and all prior `## vX.Y.Z` sections) is byte-identical to its pre-edit content (only its line number shifts)
- The bullet uses the conventional `fix:` prefix (not `feat:`, not `refactor:`)
- The bullet text names the structural change (first non-version H2), the accepted variants (`## unreleased`, `## Unreleased changes`, `## WIP`, `## Next`), and the originating specs (spec 064 watcher, spec 065 agent)
- This prompt only ran after the sibling code-change prompt (spec § "atomic-batch constraints belong in spec")

</success_criteria>

<reference>

- Spec: `/workspace/specs/in-progress/065-github-releaser-agent-lenient-unreleased.md` — Acceptance Criterion #15 is the source of the bullet text
- File under edit: `/workspace/CHANGELOG.md` — root, single global changelog per `CLAUDE.md` § Versioning and tags
- Sibling code-change prompt (must ship first): `/workspace/prompts/lenient-unreleased-section-detection.md`
- Spec 064 sibling bullet (style template, line 13 of CHANGELOG.md): `watcher/github-release` package scope, same `fix:` prefix, same structural description
- Sibling spec 064 prompt (template + style reference): `/workspace/prompts/completed/245-spec-064-lenient-unreleased-detection.md` (req 6 in particular)
- Coding plugin docs (in-container paths):
  - `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — preamble-frozen rule, conventional-prefix-required rule, `## Unreleased` placement

</reference>
