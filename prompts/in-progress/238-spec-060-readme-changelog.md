---
status: approved
spec: [060-github-releaser-major-bump-guard]
created: "2026-06-03T15:05:00Z"
queued: "2026-06-03T14:34:37Z"
branch: dark-factory/github-releaser-major-bump-guard
---

<summary>
- The repo-root `README.md` gains a new sub-section under the existing github-releaser-agent docs documenting: (a) the new `release.allowMajorBump` YAML field with a one-line YAML example, (b) the new `--allow-major` CLI flag with the `ALLOW_MAJOR` env name, (c) one-paragraph rationale pointing at the false-negative class of bug, (d) the two ways an operator re-delegates a tripped task
- The repo-root `CHANGELOG.md` `## Unreleased` block gains a single `feat:` bullet naming both `release.allowMajorBump` and `--allow-major` together (per spec 060 § Constraints; a single combined entry, not two split entries)
- The README prose is operator-facing language: it explains what the field does, why it exists (the false-negative class of bug that prefix-based bump classifiers can miss), and what an operator does when a task is tripped
- The CHANGELOG entry follows the prefix conventions from `changelog-guide.md`: `feat:` prefix, names both the new field AND the new flag in one bullet, includes a brief sentence explaining the guard's purpose
- The Wording is left to the executor's judgement (Level 4) — the prompt pins the structural requirements (sections, examples, env-var names) but does not pre-inline the exact prose
</summary>

<objective>
Document the new spec 060 major-bump guard in the repo-root `README.md` (sub-section under the existing github-releaser-agent docs) and the repo-root `CHANGELOG.md` `## Unreleased` block. The README must explain both opt-in levers (the durable per-repo YAML field and the transient per-run CLI flag) and the rationale; the CHANGELOG must record a single combined `feat:` entry that names both the field and the flag. Wording is at the executor's discretion within the structural requirements pinned below.
</objective>

<context>
Read `CLAUDE.md` and `agent/github-releaser/CLAUDE.md` for project conventions.

Read these files BEFORE editing:
- `/workspace/README.md` — repo root. The github-releaser-agent docs are an existing sub-section (find via `grep -n 'github-releaser' /workspace/README.md`). Add a new sub-section under the github-releaser-agent docs, OR extend the existing sub-section with the new content. The spec says "NOT a new file" — extend the existing structure. Match the existing heading style (the project uses `##` for top-level sections and `###` for sub-sections; verify by reading the existing github-releaser-agent section's heading hierarchy before deciding the new sub-section's level).
- `/workspace/CHANGELOG.md` — repo root. Read the existing `## Unreleased` block (if present; if `## Unreleased` is missing because the latest release was cut, add it back per the changelog-guide rules). The new spec 060 entry goes into `## Unreleased` as a single combined `feat:` bullet naming both the new YAML field and the new CLI flag. The existing entries (e.g. the v0.32.0 release notes) show the style: `- feat(agent/github-releaser): ...`. Use that prefix style.
- `/workspace/specs/in-progress/060-github-releaser-major-bump-guard.md` — the spec under implementation. § Problem (the originating incident), § Goal 5, § Desired Behavior 7, and § AC 21 are the load-bearing references for the README content. § AC 21 is the load-bearing reference for the CHANGELOG entry.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — entry format, prefix conventions, anti-patterns, `## Unreleased` rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md` — README structure and tone conventions.
- `/home/node/.claude/plugins/marketplaces/coding/docs/git-commit-guide.md` — commit-message style; not directly relevant here but mirrors the changelog style.

Verified symbols (from module source — grep-confirmed):
- `release.allowMajorBump` YAML field name is the spec's documented name. The README must use this exact casing (camelCase). The spec's AC `grep -c 'allowMajorBump' README.md` returns ≥ 1.
- `--allow-major` is the CLI flag name. The README must use this exact long-form (no short form). The spec's AC `grep -c '\-\-allow-major' README.md` returns ≥ 1.
- `ALLOW_MAJOR` is the env-var name. The README must use this exact uppercase. The spec's AC `grep -c 'ALLOW_MAJOR' README.md` returns ≥ 1.
- The CHANGELOG entry's bullet MUST contain the substring `allowMajorBump` (the spec's AC `grep -c 'allowMajorBump' CHANGELOG.md` returns ≥ 1). Including `--allow-major` in the same bullet is recommended for completeness.
- The reference incident bullet `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` is documented in spec 060 § Problem. The README rationale paragraph MAY cite it (as a real-world example of the false-negative class) or describe the class more generally — either is acceptable; do not invent different example bullets.
</context>

<requirements>

1. **README — new sub-section under the existing github-releaser-agent docs.** In `/workspace/README.md`, locate the existing github-releaser-agent section (search for the section heading that contains "github-releaser-agent" or "agent/github-releaser"; the project may use either form). Add a new sub-section immediately under the existing sub-sections (NOT as a new top-level `##` heading — match the existing heading hierarchy). The new sub-section's content MUST cover the four spec 060 § Desired Behavior 7 items:

   a. **The new `release.allowMajorBump` YAML field** with a one-line YAML example, e.g.:

      ```yaml
      # .maintainer.yaml
      release:
        allowMajorBump: true   # opt in to automatic major-version releases
      ```

      Document the default (false), the per-repo scope, and the trip-on-missing semantics. Name the field exactly `allowMajorBump` (camelCase) so the AC grep matches.

   b. **The new `--allow-major` CLI flag** with the `ALLOW_MAJOR` env name. Document the default (false), the per-run scope, and the override-only-on-major semantics. The README should make clear that the flag is a TRANSIENT ops escape hatch alongside the durable per-repo YAML policy, NOT a replacement for it. The flag name MUST be `--allow-major` (with double-dash, long-form only) so the AC grep matches.

   c. **One-paragraph rationale** pointing at the false-negative class of bug — the class where a prefix-based bump classifier mis-classifies a breaking change as `patch` (or misses a hidden `BREAKING CHANGE:` body line in a `refactor:` bullet). The spec 060 § Problem gives the originating incident (`refactor(lib): rename TaskTypeClaude → TaskTypeLLM`); the README MAY cite it as a concrete example but does not need to name the bullet verbatim. The rationale should explain: the guard catches the false-positive side (a `major` verdict that an operator wants to downgrade or that came from a classifier false positive) and provides a per-run override for the false-negative side (a `major` verdict that the classifier got right but the repo isn't opted in for yet).

   d. **The two ways an operator re-delegates a tripped task.** The spec 060 § Desired Behavior 7 says exactly two paths:
      1. Commit `release.allowMajorBump: true` to the target repo's `.maintainer.yaml`, push, and re-set `assignee: github-releaser-agent` on the task.
      2. Re-fire the Job with `--allow-major=true` (or `ALLOW_MAJOR=true` in the env).

      Document both clearly so an on-call operator has a copy-pasteable recipe.

   The README sub-section is a few paragraphs plus the YAML example. Match the existing README's tone (terse, operator-facing, no marketing prose; see the existing github-releaser-agent sub-section's voice for the canonical example).

2. **CHANGELOG — single combined `feat:` entry under `## Unreleased`.** In `/workspace/CHANGELOG.md`, locate the `## Unreleased` block. If the block exists, append a new bullet. If the block does NOT exist (because the latest release was cut and `## Unreleased` was removed), add it back per the changelog-guide rules — the block goes immediately under the latest `## vX.Y.Z` heading, with the SemVer-preamble header (the `# Changelog` / "All notable changes..." / SemVer-link / MAJOR-MINOR-PATCH bullets) preserved verbatim.

   The new bullet's content:
   - Prefix: `feat(agent/github-releaser):` — matches the existing entries' style.
   - Body: a single sentence (or two) naming both `release.allowMajorBump` and `--allow-major` (the field AND the flag, together). Briefly mention the guard's purpose: blocks `major` releases without an explicit opt-in (per-repo YAML or per-run CLI). Optionally mention the originating false-negative class of bug as context.
   - No bash comments, no `verify:` / `confirm:` lines, no test instructions.

   The acceptance criterion `grep -c 'allowMajorBump' CHANGELOG.md` returns ≥ 1. The bullet MUST contain the substring `allowMajorBump` (camelCase). It is RECOMMENDED (not required) to also include `--allow-major` in the same bullet for completeness.

   Suggested entry (the executor may tighten the prose):

   ```
   - feat(agent/github-releaser): planning phase now blocks `major` bump verdicts without an explicit opt-in — `release.allowMajorBump: true` in the target repo's `.maintainer.yaml` (durable, per-repo) or `--allow-major` / `ALLOW_MAJOR=true` on the agent binary (transient, per-run). Without either, a `major` verdict returns `Status=NeedsInput` with `precondition_failed=major_bump_not_allowed` so the operator can confirm before tag + push. Guards the false-negative class of bug where a prefix-based classifier mis-categorizes a breaking change (spec 060)
   ```

3. **Do NOT add a new file.** The README is `README.md` at repo root, and the CHANGELOG is `CHANGELOG.md` at repo root. Do not create `docs/spec-060.md` or any sibling file. The spec 060 § Constraints locks this: "NOT a new file."

4. **Do NOT touch other CHANGELOG entries.** The `## v0.32.0` block and earlier releases are frozen; do not reformat, reorder, or add cross-references. Append the new bullet to `## Unreleased` only.

5. **Do NOT touch other README sections.** The github-releaser-agent section may have multiple sub-sections (env vars, deployment, debug commands, etc.); do not edit the existing sub-sections. Add a NEW sub-section immediately under the existing github-releaser-agent docs, or extend the most relevant existing sub-section with the new content — the executor picks based on what reads cleanest.

6. **Acceptance gate — `make precommit` exits 0 in `agent/github-releaser/`.** This is the final precommit run for spec 060. After this prompt's edits, run `cd /workspace/agent/github-releaser && make precommit` and confirm exit code 0. The precommit runs format + generate + test + lint + gosec + license; it MUST pass. Investigate and fix any failures.

   Also run `cd /workspace/lib && make precommit` (per spec 060 § AC 1) and confirm exit code 0.

   If precommit fails on an unrelated existing issue, fix it or document the gap in `## Improvements` (per the global completion-report rules). Do NOT skip precommit.

7. **Cross-spec evidence — the verification block from spec 060 § Verification.** Run the spec's verification commands after both README and CHANGELOG edits land:

   ```
   grep -c 'allowMajorBump' /workspace/README.md                    # ≥ 1
   grep -c '\-\-allow-major' /workspace/README.md                    # ≥ 1
   grep -c 'ALLOW_MAJOR' /workspace/README.md                       # ≥ 1
   grep -c 'allowMajorBump' /workspace/CHANGELOG.md                  # ≥ 1
   ```

   All four must return ≥ 1. If any returns 0, the prompt's README/CHANGELOG content is missing a required substring; fix and re-verify.

8. **Cross-prompt dependency declaration.** This prompt depends on prompts 1, 2, 3, and 4 having shipped (the schema, the CLI flag, the guard logic, the Ginkgo tests). The README and CHANGELOG entry name the new field and flag; the new functionality must be in place before the docs are written. If any of prompts 1-4 failed to ship, this prompt's verification will fail and the auditor will flag the cross-prompt regression.
</requirements>

<constraints>
- The README sub-section MUST use the exact field name `allowMajorBump` (camelCase) and the exact flag name `--allow-major` (with double-dash, long-form) and the exact env name `ALLOW_MAJOR` (uppercase). These are the three strings the spec's AC greps look for. The spec 060 § AC 21: `grep -c 'allowMajorBump' README.md` returns ≥ 1, `grep -c '\-\-allow-major' README.md` returns ≥ 1, `grep -c 'ALLOW_MAJOR' README.md` returns ≥ 1.
- The CHANGELOG bullet MUST use the substring `allowMajorBump` (camelCase) so the spec's AC `grep -c 'allowMajorBump' CHANGELOG.md` returns ≥ 1.
- The CHANGELOG entry is a SINGLE `feat:` bullet naming both the field and the flag, per spec 060 § Constraints. Do NOT split into two bullets (one for the field, one for the flag) — the spec is one logical change.
- The README rationale paragraph MUST point at the false-negative class of bug that the guard addresses. The spec's § Problem documents the class; the README should explain it in operator-facing language.
- The README MUST document BOTH operator re-delegation paths (commit YAML + re-set assignee, OR re-fire Job with `--allow-major`). The spec 060 § Desired Behavior 7 lists both as required.
- Do NOT invent additional knobs (per-PR override, per-task frontmatter, etc.) in the README. The spec 060 § Non-goals explicitly forbids these.
- Do NOT add a `release.allowMinorBump` or `release.allowPatchBump` knob to the README or CHANGELOG. Spec 060 § Non-goals explicitly forbids the symmetric knobs.
- Do NOT touch other CHANGELOG entries (`## v0.32.0` and earlier) or other README sections.
- Do NOT add a new `docs/spec-060.md` file. Spec 060 § Constraints locks the README and CHANGELOG as the only doc surfaces.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && make precommit
```
Expected: exit code 0; lint passes; all Ginkgo tests pass; coverage ≥ 75% on `pkg/...`; license headers valid; gosec clean; trivy clean.

```
cd /workspace/lib && make precommit
```
Expected: exit code 0; the new `AllowMajorBump` Ginkgo entries pass; the new `It("release.allowMajorBump: non-bool -> strict error", ...)` case passes; the existing 11+ `Entry(...)` lines and 4 `It(...)` cases still pass.

Evidence commands the auditor will run (spec 060 § AC 1, 2, 21):
- `cd /workspace/agent/github-releaser && make precommit` → exit code 0.
- `cd /workspace/lib && go test ./...` → exit code 0.
- `grep -c 'allowMajorBump' /workspace/README.md` → ≥ 1.
- `grep -c '\-\-allow-major' /workspace/README.md` → ≥ 1.
- `grep -c 'ALLOW_MAJOR' /workspace/README.md` → ≥ 1.
- `grep -c 'allowMajorBump' /workspace/CHANGELOG.md` → ≥ 1.
- `git diff master -- /workspace/CHANGELOG.md | grep -E '^\+[^-]' | head -1` → matches `^-\s*feat\(.*\): ` (the new bullet has the canonical `feat(...)` prefix).
</verification>
