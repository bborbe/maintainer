---
status: completed
spec: [061-extend-repoallowlist-exclusion-syntax]
summary: Added `!`-prefix syntax table to three consumer READMEs and a `feat(lib/repoallowlist)` entry under `## Unreleased` in CHANGELOG.md; no consumer service code touched
container: maintainer-allowlist-exclude-exec-241-docs-ripple-and-no-consumer-diff-proof
dark-factory-version: v0.175.0
created: "2026-06-03T18:26:57Z"
queued: "2026-06-03T18:26:57Z"
started: "2026-06-03T18:33:41Z"
completed: "2026-06-03T18:35:36Z"
branch: dark-factory/extend-repoallowlist-exclusion-syntax
---

<objective>
Document the new `!`-prefix exclusion syntax in the three consumer READMEs that already document `REPO_ALLOWLIST` (watcher/github-release/README.md, watcher/github-pr/README.md, watcher/github-build/README.md), append a single `## Unreleased` CHANGELOG entry under a `lib/repoallowlist` heading, and produce the spec's grep proof that consumer service code (any path under `watcher/` or `agent/`, excluding READMEs and `_test.go` files) carries zero diff. The proof is the load-bearing evidence of the spec's "five callsites need zero code change" lock-in. The docs MUST be honest — README examples are checked against the shipped semantics from prompt 1, not invented locally.
</objective>

<context>
Read `CLAUDE.md` for repo conventions.

Read these files BEFORE editing:
- `/workspace/specs/in-progress/061-extend-repoallowlist-exclusion-syntax.md` — the spec under implementation. § Desired Behavior 1-7 (the lock-in semantics), § Acceptance Criteria 13 (README evidence), § Acceptance Criteria 14 (CHANGELOG evidence), § Acceptance Criteria 15 (no-consumer-code-change grep proof), § Verification (the canonical grep commands) are the load-bearing references. The spec's load-bearing grep is `git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go` returning empty.
- `/workspace/lib/repoallowlist/repoallowlist.go` — read for the shipped semantics from prompt 1. The doc comment now names the `!`-prefix syntax, the matching rule, and the allow-all-except semantic. The README examples must match this doc comment — same example values, same allow-all-except phrasing. If prompt 1's doc uses `github.com/bborbe/*` and `!github.com/bborbe/go-skeleton`, the READMEs use the same pair.
- `/workspace/watcher/github-release/README.md`, `/workspace/watcher/github-pr/README.md`, `/workspace/watcher/github-build/README.md` — the three REPO_ALLOWLIST consumer READMEs. Each README already has a section (or table) documenting the `REPO_ALLOWLIST` env var; find it via `grep -n 'REPO_ALLOWLIST' <file>`. The spec's AC 14 evidence command is `grep -c '!github.com' <file> ≥ 1` for each of the three files. Add a new short section OR extend the existing section with a `Syntax` table covering the five required rows. The decision between "new sub-section" and "extend existing section" is at the executor's discretion (Level 4) — pick what reads cleanest in each README's existing structure.
- `/workspace/CHANGELOG.md` — repo root. Read the existing `## Unreleased` block (if present; if `## Unreleased` is missing because the latest release was cut, add it back per the changelog-guide rules). The spec's AC 15 evidence command is `grep -n 'repoallowlist' CHANGELOG.md` returning a line under `## Unreleased`. The new bullet goes under `## Unreleased` and uses a `feat:` prefix (per spec 060-style precedents in the existing `## Unreleased` block) naming the `!`-prefix exclude syntax.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — entry format, `feat:` prefix conventions, anti-patterns, `## Unreleased` rules, the SemVer-preamble header preservation requirement when `## Unreleased` is reintroduced.
- `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md` — README structure and tone conventions; operator-facing language, terse prose, no marketing.

Verified symbols (from module source — grep-confirmed):
- `lib/repoallowlist` module path — `github.com/bborbe/maintainer/lib/repoallowlist` (read from `lib/go.mod` and the `repoallowlist.go` package declaration). The CHANGELOG bullet's prefix follows the existing style: `feat(lib/repoallowlist): ...` (mirror the spec-060 entries' `(agent/github-releaser):` sub-scope pattern; the new bullet uses `(lib/repoallowlist):`).
- The nine callsites across the five consumer services that MUST stay unchanged (per spec § Constraints, § AC 16):
   1. `watcher/github-release/pkg/filter/repo_allowlist_filter.go:44` — `repoallowlist.IsAllowed(f.allowlist, release.RepoKey)`.
   2. `watcher/github-pr/pkg/filter/repo_allowlist_filter.go:44` — `!repoallowlist.IsAllowed(f.allowlist, pr.RepoKey)`.
   3. `watcher/github-pr/main.go:225` — `repoallowlist.Validate(ctx, repoAllowlist)`.
   4. `watcher/github-build/main.go:87` — `repoallowlist.Validate(ctx, repoAllowlist)`.
   5. `watcher/github-build/cmd/run-once/main.go:83` — `repoallowlist.Validate(ctx, repoAllowlist)`.
   6. `watcher/github-build/pkg/filter/repo_allowlist_filter.go:43` — `!repoallowlist.IsAllowed(f.allowlist, repoKey)`.
   7. `agent/pr-reviewer/main.go:131` — `repoallowlist.Validate(ctx, repoAllowlist)`.
   8. `agent/pr-reviewer/cmd/run-task/main.go:90` — `repoallowlist.Validate(ctx, repoAllowlist)`.
   9. `agent/pr-reviewer/pkg/steps_checkout_execution.go:200` — `repoallowlist.IsAllowed(s.repoAllowlist, repoKey)`.
   The spec mentions `agent/github-releaser` as a fifth consumer; that path does not yet exist in the tree, so the no-diff proof trivially covers it (no file to diff against).
- The spec's grep proof is `git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go` — empty output, non-zero exit. The auditor runs this command; the executor's job is to ensure no source file in the consumer paths is touched by this prompt.
</context>

<requirements>

1. **Extend the three consumer READMEs with a `!`-prefix syntax table.** For each of `watcher/github-release/README.md`, `watcher/github-pr/README.md`, `watcher/github-build/README.md`:

   a. Find the existing `REPO_ALLOWLIST` section (or sub-section) via `grep -n 'REPO_ALLOWLIST' <file>`. The existing section typically uses a heading like `## REPO_ALLOWLIST` or a table row. The new content goes either as a new sub-section immediately under the existing `REPO_ALLOWLIST` heading, OR as a new `Syntax` table appended to the existing section. Pick the structure that matches each README's existing heading hierarchy (e.g. if the existing `REPO_ALLOWLIST` is a `##` heading with no sub-headings, add a `### Syntax` sub-heading; if it's a `###`, add a `#### Syntax` sub-heading; the project uses `##` for top-level sections and `###` for sub-sections — verify by reading the file's heading hierarchy).

   b. The new content MUST include the literal substring `!github.com` (the spec's AC 14 evidence command is `grep -c '!github.com' <file> ≥ 1` for each of the three files). A `Syntax` table is the cleanest fit:

      ```markdown
      | Entry shape | Example | Meaning |
      |-------------|---------|---------|
      | Literal include | `github.com/bborbe/maintainer` | Allow exactly this repo |
      | Wildcard include | `github.com/bborbe/*` | Allow every repo under this owner |
      | Literal exclude | `!github.com/bborbe/go-skeleton` | Reject exactly this repo (overrides any matching include) |
      | Wildcard exclude | `!github.com/bborbe/*` | Reject every repo under this owner |
      ```

   c. Below the table, add a one-paragraph example illustrating the allow-all-except semantic — the most common operator use case. The spec's § Goal names this as the canonical case the operator most often wants:

      > An allowlist consisting of only exclude entries is treated as allow-all-except: every target passes the include gate, and only the exclude gate filters. Example: `REPO_ALLOWLIST=!github.com/bborbe/go-skeleton` rejects go-skeleton and allows every other repo (including all other bborbe repos). To allow every bborbe repo except go-skeleton, write `github.com/bborbe/*,!github.com/bborbe/go-skeleton`.

   d. The example values in the table and the allow-all-except paragraph MUST be consistent across the three READMEs (same example pairs: `github.com/bborbe/maintainer`, `github.com/bborbe/*`, `!github.com/bborbe/go-skeleton`, `!github.com/bborbe/*`). Pick one canonical example set and use it identically in all three files.

   e. The wording should match the existing README tone — terse, operator-facing, no marketing prose. Each existing README is the canonical example; mirror its voice.

2. **Append a `## Unreleased` CHANGELOG entry under a `lib/repoallowlist` heading.** In `/workspace/CHANGELOG.md`:

   a. Find the `## Unreleased` block. If present, append a new bullet at the end of the existing entries. If absent (because the latest release was cut and `## Unreleased` was removed), add it back per the changelog-guide rules: the block goes immediately under the latest `## vX.Y.Z` heading, with the SemVer-preamble header (`# Changelog` / "All notable changes..." / SemVer-link / MAJOR-MINOR-PATCH bullets) preserved verbatim.

   b. The new bullet's content:
      - Prefix: `feat(lib/repoallowlist):` — mirror the spec-060 entries' style (`feat(agent/github-releaser):`).
      - Body: a single sentence (or two) naming the new `!`-prefix exclude syntax. Briefly mention: excludes always override includes; exclude-only allowlists mean allow-all-except; the existing public API is frozen. Optionally mention the operator use case the unblocks (the dev/prod release pipeline collision on go-skeleton) as one short clause of context.
      - No bash comments, no `verify:` / `confirm:` lines, no test instructions.
      - MUST contain the substring `repoallowlist` (the spec's AC 15 evidence command is `grep -n 'repoallowlist' CHANGELOG.md` returning a line under `## Unreleased`).

   Suggested entry (the executor may tighten the prose):

   ```
   - feat(lib/repoallowlist): allow `!`-prefix entries as exclusions — a target is allowed iff `(includes empty OR any include matches) AND (no exclude matches)`. Excludes always override includes; an exclude-only allowlist means allow-all-except. Existing `IsAllowed` / `Validate` signatures unchanged; consumer services pick up the new semantics with zero code change (spec 061)
   ```

3. **No-diff proof — consumer service code is untouched.** This prompt does NOT edit any file under `watcher/` or `agent/` other than the three named READMEs. The five callsites listed in the Verified Symbols block above (in the `<context>` section of this prompt) stay byte-identical to `origin/master`. The proof is the spec's own grep:

   ```
   git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go
   ```

   Expected: empty output, non-zero exit. The auditor runs this command; the executor's job is to ensure the prompt's edits are confined to the three READMEs and `CHANGELOG.md`.

4. **Acceptance gate — three grep commands return the expected counts.** Run these and confirm:

   ```
   for f in /workspace/watcher/github-release/README.md /workspace/watcher/github-pr/README.md /workspace/watcher/github-build/README.md; do
       echo "=== $f ==="
       grep -c '!github.com' "$f"
   done
   ```

   Expected: each prints a number ≥ 1. The spec's AC 14 evidence command. If any returns 0, the README's syntax table is missing the `!github.com` example — fix and re-verify.

   ```
   grep -n 'repoallowlist' /workspace/CHANGELOG.md
   ```

   Expected: at least one line is under the `## Unreleased` block (the new bullet) AND the line contains the substring `!` or `exclude` to confirm the bullet names the new syntax (not just the module name).

   ```
   git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go
   ```

   Expected: empty output, non-zero exit code. The spec's AC 16 evidence command.

5. **Cross-prompt dependency declaration.** This prompt depends on prompt 1 having shipped first — the README examples and CHANGELOG entry name the new `!`-prefix syntax, and the spec's lock-in is that the doc examples match the shipped semantics. If prompt 1 failed to ship, this prompt's verification will fail and the auditor will flag the cross-prompt regression. If the executor discovers prompt 1's `IsAllowed` doc comment uses different example values than the ones suggested in this prompt's table (e.g. prompt 1 used `github.com/example/*` instead of `github.com/bborbe/*`), the executor MUST update the README tables and the allow-all-except paragraph to match prompt 1's shipped values — the docs are downstream of the library, not the other way around.
</requirements>

<constraints>
- The three README syntax tables MUST use the literal substring `!github.com` (camelCase for the prefix). The spec's AC 14 evidence is `grep -c '!github.com' <file> ≥ 1` for each of the three files. Do not use a different example prefix (e.g. `!gitlab.com`) — the canonical example is `!github.com/bborbe/go-skeleton` everywhere.
- The three README tables MUST be consistent — same example pairs in all three files. Pick the canonical set once and use it identically. `github.com/bborbe/maintainer`, `github.com/bborbe/*`, `!github.com/bborbe/go-skeleton`, `!github.com/bborbe/*` is the recommended set.
- The CHANGELOG bullet MUST contain the substring `repoallowlist` (the spec's AC 15 evidence). The recommended prefix is `feat(lib/repoallowlist):` to mirror the spec-060 entries' `(agent/github-releaser):` sub-scope pattern.
- The CHANGELOG bullet is a SINGLE `feat:` entry. Do NOT split into two bullets (one for the syntax, one for the allow-all-except semantic) — the spec is one logical change.
- Do NOT edit any source file under `watcher/` or `agent/` (only the three named READMEs). Any diff under those paths in `git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go` is a spec violation.
- Do NOT edit `lib/repoallowlist/repoallowlist.go` or `lib/repoallowlist/repoallowlist_test.go` in this prompt — that is prompt 1's scope. This prompt is docs-only.
- Do NOT edit the existing `## Unreleased` bullets. Append a new bullet; do not reorder or reformat existing ones.
- Do NOT touch other CHANGELOG entries (`## vX.Y.Z` and earlier releases) — they are frozen.
- Do NOT touch the three READMEs' existing sections — only add a new sub-section or extend the existing `REPO_ALLOWLIST` section with the new `Syntax` table and the allow-all-except example paragraph.
- Do NOT add a new file. The spec's § Constraints locks the three existing READMEs and `CHANGELOG.md` as the only doc surfaces. Do not create `docs/spec-061.md` or any sibling file.
- Do NOT invent additional knobs in the README. The `!`-prefix in `REPO_ALLOWLIST` is the only new operator-facing surface; do not mention a `REPO_BLOCKLIST` env var, a `TASK_SUFFIX` knob, or any other configuration that the spec explicitly forbids.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
```
# Spec AC 14: each of the three READMEs has at least one !github.com match.
for f in /workspace/watcher/github-release/README.md /workspace/watcher/github-pr/README.md /workspace/watcher/github-build/README.md; do
    count=$(grep -c '!github.com' "$f")
    echo "$f: $count"
    test "$count" -ge 1 || { echo "FAIL: $f has 0 !github.com matches"; exit 1; }
done
```
Expected: each line prints a count ≥ 1; the loop exits 0.

```
# Spec AC 15: CHANGELOG has a repoallowlist line scoped to ## Unreleased.
# Use awk to slice the Unreleased section, then grep within that slice — avoids false
# positives from past releases that already named (lib/repoallowlist) in their bullets.
awk '/^## Unreleased/{f=1;next} /^## /{f=0} f' /workspace/CHANGELOG.md | grep -q 'repoallowlist' \
  || { echo "FAIL: no repoallowlist bullet under ## Unreleased"; exit 1; }
```
Expected: exit 0. Additionally verify the bullet's prefix is `feat(lib/repoallowlist):` and its body names the `!`-prefix / exclude syntax by reading the `## Unreleased` block directly.

```
# Spec AC 16: zero diff in consumer service code.
git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go
```
Expected: empty output, non-zero exit code. The `grep` step in the pipeline (after `git diff`) finds no matches because the only files touched under `watcher/` and `agent/` are the three READMEs, all of which are excluded by `grep -v README.md`.

```
# Spec verification: callers delegate to the shared library.
grep -rn 'repoallowlist\.\(IsAllowed\|Validate\|ParseRepoAllowlist\)' /workspace/lib/ /workspace/watcher/ /workspace/agent/ | grep -v _test.go
```
Expected: every match is a delegation to the shared library; no consumer parses entries itself. This is the spec's own § Verification grep.

Evidence commands the auditor will run (spec 061 § AC 13, 14, 15, 16):
- `grep -c '!github.com' /workspace/watcher/github-release/README.md` → ≥ 1.
- `grep -c '!github.com' /workspace/watcher/github-pr/README.md` → ≥ 1.
- `grep -c '!github.com' /workspace/watcher/github-build/README.md` → ≥ 1.
- `grep -n 'repoallowlist' /workspace/CHANGELOG.md` → ≥ 1, with the matched line in the `## Unreleased` block.
- `git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go` → empty output, non-zero exit.
- `git diff --stat origin/master...HEAD -- watcher/ agent/ CHANGELOG.md` → shows the three README files plus `CHANGELOG.md` and nothing else.
</verification>
