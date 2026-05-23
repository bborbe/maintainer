---
status: approved
spec: [036-watcher-pr-rename-trigger-add-single-pr-trigger]
created: "2026-05-23T21:03:00Z"
queued: "2026-05-23T21:13:16Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

## Summary

- Update `watcher/github-pr/README.md` to document both `/check` and `/trigger?url=` endpoints + the known-limit operator note
- Update any in-repo callers of the old `/trigger` (poll) endpoint to `/check`
- Update CHANGELOG: rename, new endpoint, and `lib/prurl` extraction (flat per-version style matching existing entries)
- Obsidian runbook updates are host-only and OUT OF SCOPE (vault is not mounted)

## Objective

Replace stale `/trigger` references in in-repo docs/manifests with `/check`, document the new `/trigger?url=` endpoint in the watcher README (including the operator-facing known-limit), and add the three matching CHANGELOG entries under `## Unreleased`. No Go code touched.

## Context

Read these files before making changes:

**README to update:**
- `/workspace/watcher/github-pr/README.md` — currently has 4 stale `/trigger` references at lines 10, 14, 36, 53. Endpoint table at lines 48-53 must be rewritten to cover both `/check` (poll) and `/trigger?url=` (single-PR).

**CHANGELOG to update:**
- `/workspace/CHANGELOG.md` — existing style: flat per-version section `## vX.Y.Z` with bullets like `- feat(agent/pr-reviewer): ...` / `- fix(agent/pr-reviewer): ...` / `- refactor(scope): ...`. There is currently NO `## Unreleased` section. Add one immediately after the introductory block (just before `## v0.25.12` or whichever is the latest version).

**Files to check for old `/trigger` references (recursive grep across repo, exclude vendored/build dirs):**
- `/workspace/watcher/github-pr/k8s/` — per-service k8s manifests (NOTE: there is no top-level `/workspace/k8s/` directory; manifests live under each service)
- `/workspace/agent/pr-reviewer/k8s/` — agent k8s manifests
- Any `.md`, `.yaml`, `.yml`, `.sh`, `.json` referencing the watcher's admin endpoint

**Key changes from prompts 1-3:**
- Prompt 1: `lib/prurl` extraction — `lib/prurl/prurl.go` + `lib/prurl/prurl_test.go` created; `agent/pr-reviewer/pkg/prurl.go` deleted
- Prompt 2: `/trigger` (poll) renamed to `/check`
- Prompt 3: New `POST /trigger?url=<pr_url>` handler added

## Requirements

### Step 1: Update `watcher/github-pr/README.md`

Read the current file. Replace every naked `/trigger` reference (for the poll behavior) with `/check`. Then rewrite the HTTP endpoint table to document BOTH endpoints with example `curl` commands, response shapes, and — critically — the **known limit** for `/trigger?url=`:

> **Known limit:** if a vault task already exists for the same `(PR, SHA)`, the controller's `create-if-not-exists` is idempotent and no fresh agent Job spawns. To force a re-run in that case, reset the vault task's frontmatter (`phase`, `status`, `trigger_count`) manually OR push a new commit so the SHA changes.

The README must after edits:
- Mention `/check` ≥ 2 times (Links section + endpoint table)
- Mention `/trigger?url=` ≥ 2 times (endpoint table + known-limit section)
- Contain the known-limit text verbatim or substantively equivalent

### Step 2: Update in-repo callers of old `/trigger`

Run a recursive grep across the repo:
```bash
grep -rn '/admin/maintainer-watcher-github-pr/trigger' /workspace/ \
  --include='*.yaml' --include='*.yml' --include='*.md' --include='*.sh' --include='*.json' \
  2>/dev/null | grep -v '?url='
```

For each match that is NOT followed by `?url=`, update the path to `/check`. Do NOT touch matches that already include `?url=` — those are the new single-PR endpoint references.

Likely-affected locations (verify by grep, do not assume):
- `/workspace/watcher/github-pr/k8s/` per-service k8s manifests
- `/workspace/agent/pr-reviewer/k8s/` per-service k8s manifests
- Any `docs/` or root-level markdown referencing the admin gateway

### Step 3: Update CHANGELOG

Read `/workspace/CHANGELOG.md`. The file uses flat per-version sections (`## v0.25.12` etc.) with `- prefix(scope): description` bullets. **There is currently no `## Unreleased` section** — add one immediately after the title/intro block and before the most recent version heading.

Add exactly these three entries (and nothing else — do NOT fabricate entries for unrelated changes):

```markdown
## Unreleased

- refactor(lib): extract `ParsePRURL` from `agent/pr-reviewer/pkg/prurl.go` to shared `lib/prurl/prurl.go` so both `agent/pr-reviewer` and `watcher/github-pr` import the same parser (spec 036)
- refactor(watcher/github-pr): rename admin endpoint `/trigger` (multi-repo poll) to `/check` — name now reflects behavior; hard cutover, no backwards-compat alias (spec 036)
- feat(watcher/github-pr): add `POST /trigger?url=<pr_url>` admin endpoint to fire a single-PR review by URL; reuses the existing filter chain and trust evaluation; known limit — if a vault task already exists for the same (PR, SHA) the operator must still reset vault frontmatter or push a new commit (spec 036)
```

Match the existing flat-bullet style; do NOT introduce `### Added` / `### Changed` / `### Fixed` Keep-a-Changelog subheadings (the repo doesn't use them).

## Constraints

- Do NOT modify any Go source files in this step — that was done in prompts 1-3
- Do NOT run `make precommit` or `go test` — this prompt edits only markdown and YAML; the Go modules were already verified by prompts 1-3's own verification blocks
- Do NOT edit files outside the `/workspace/` tree — the Obsidian runbook updates on the host are a manual step per spec AC Rung 4 and CANNOT be performed in this container
- Do NOT fabricate CHANGELOG entries for unrelated changes
- Preserve existing CHANGELOG flat-bullet style (no Keep-a-Changelog subheadings)
- Match existing CHANGELOG scope-prefix convention: `feat(scope):` / `fix(scope):` / `refactor(scope):`

## Verification

```bash
# Gate 1: no naked /trigger references remain in repo docs/manifests
! grep -rn '/admin/maintainer-watcher-github-pr/trigger' /workspace/ \
    --include='*.yaml' --include='*.yml' --include='*.md' --include='*.sh' --include='*.json' \
    2>/dev/null | grep -v '?url=' | grep -v 'keel.sh/trigger'

# Gate 2: README documents both endpoints
grep -c '/check' /workspace/watcher/github-pr/README.md  # expect ≥2
grep -c '/trigger?url=' /workspace/watcher/github-pr/README.md  # expect ≥2

# Gate 3: README contains the known-limit operator note
grep -iE 'known limit|already exists|vault.*frontmatter' /workspace/watcher/github-pr/README.md

# Gate 4: CHANGELOG has the Unreleased section with all three required entries
grep -A20 -E '^## (Unreleased|\[Unreleased\]|Next)' /workspace/CHANGELOG.md | grep -E '/check' || { echo "CHANGELOG missing /check"; exit 1; }
grep -A20 -E '^## (Unreleased|\[Unreleased\]|Next)' /workspace/CHANGELOG.md | grep -E '/trigger\?url=' || { echo "CHANGELOG missing /trigger?url="; exit 1; }
grep -A20 -E '^## (Unreleased|\[Unreleased\]|Next)' /workspace/CHANGELOG.md | grep -E 'lib/prurl' || { echo "CHANGELOG missing lib/prurl"; exit 1; }
```
