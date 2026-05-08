---
status: committing
summary: 'Renamed internal filename_hint terminology to title across both watchers: computePRFilenameHint→computePRTitle, computeFilenameHint→computeBuildTitle, maxFilenameHintLen→maxTitleLen in code files and callers; updated doc comments and log messages; rewrote stale docs/build-watcher.md section and docs/architecture.md JSON example + prose; added CHANGELOG entry; contract tests preserved verbatim.'
container: maintainer-104-rename-filename-hint-to-title-in-watchers
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-08T13:30:43Z"
queued: "2026-05-08T13:30:43Z"
started: "2026-05-08T13:30:44Z"
---

<summary>
- Rename internal helper functions, constants, comments, and log strings in both maintainer watchers from `filename_hint` terminology to `title` — wire format already migrated to `Title` field (per CHANGELOG entries for 0.31.0 + 0.31.1 referencing `task.CreateCommandSender`); only naming residue remains.
- Two code files: `watcher/github-pr/pkg/filename.go`, `watcher/github-build/pkg/filename.go`. Two callers: each watcher's `pkg/watcher.go`. Two doc files: `docs/build-watcher.md`, `docs/architecture.md`.
- Preserve wire-format contract tests verbatim — they assert "emits 'title', not 'filename_hint'" and lock the regression. Their negative-assertion strings stay exactly as-is.
- Preserve all CHANGELOG entries — they are the historical record of how the `filename_hint` → `Title` migration happened. Do NOT edit lines 32 + 36 (`feat: emit filename_hint`) or 12 + 16 (`refactor: removes WatcherCreateTaskCommand wrapper`).
- After this prompt, `grep -rn filename_hint watcher/ docs/` returns zero matches (CHANGELOG and tests excluded).
- `make precommit` clean in both `watcher/github-pr/` and `watcher/github-build/`.
</summary>

<objective>
The `filename_hint` term is dead. The wire format moved to `Title` field on the canonical `task.CreateCommand` (per maintainer 0.31.x refactor that removed `WatcherCreateTaskCommand` wrapper). The bborbe/agent controller honors `Title` exclusively — `grep -rn filename_hint ~/Documents/workspaces/agent/` returns zero matches. The only residue is internal helper names and stale doc sections describing a schema that no longer exists.

Cleaning this up matters now because:

1. **Single source of truth** — function name `computeFilenameHint` no longer matches what it does (it now produces a `Title`).
2. **Stale docs are misleading** — `docs/build-watcher.md` line 119 says "the `filename_hint` field is emitted but ignored" which is **wrong** (it's no longer emitted; the controller honors `Title`).
3. **Pattern alignment for new producers** — future planner agents (sentry-bug-analyser, updater-agent) will use `Title`. The watchers should be the canonical reference.

This is a cosmetic cleanup PR — zero behaviour change. Wire format unchanged. Vault filenames unchanged. Producer/consumer compatibility unchanged.
</objective>

<context>

## Files to edit

### Code (rename + log/comment cleanup)

**`watcher/github-pr/pkg/filename.go`** — current state:

```go
// maxFilenameHintLen is the maximum byte length of a filename_hint value.
// Hints that exceed this limit are truncated with a WARN log.
const maxFilenameHintLen = 200

// maxSlugLen is the maximum character length of the slugified PR-title segment.
const maxSlugLen = 50

// computePRFilenameHint returns the human-readable filename hint for a PR-review task.
// Format (with slug): "PR Review {provider} - {owner}-{repo} - {number} - {slug}"
// Format (empty slug): "PR Review {provider} - {owner}-{repo} - {number}"
// The returned string MUST NOT include the .md extension; the controller appends it.
func computePRFilenameHint(provider, owner, repo string, number int, title string) string {
    base := fmt.Sprintf("PR Review %s - %s-%s - %d", provider, owner, repo, number)
    slug := slugifyTitle(title)
    var hint string
    if slug == "" {
        hint = base
    } else {
        hint = base + " - " + slug
    }
    if len(hint) > maxFilenameHintLen {
        glog.Warningf(
            "filename_hint exceeds max length: len=%d max=%d — truncating",
            len(hint),
            maxFilenameHintLen,
        )
        hint = hint[:maxFilenameHintLen]
    }
    return hint
}
```

After:
- Rename `maxFilenameHintLen` → `maxTitleLen` (200 stays)
- Rename `computePRFilenameHint` → `computePRTitle`
- Rename local var `hint` → `title` inside the function
- Update the doc comment on the const + function: "filename hint" → "title"
- Update the WARN log message: `"filename_hint exceeds max length"` → `"PR title exceeds max length"`

**`watcher/github-build/pkg/filename.go`** — current state:

```go
// maxFilenameHintLen is the maximum byte length of a filename_hint value.
// Hints that exceed this limit are truncated with a WARN log to prevent filesystem aliasing.
const maxFilenameHintLen = 200

// computeFilenameHint returns the human-readable filename hint for a build-failure task.
// Format: "Build Failure {provider} - {slugifySegment(owner)}-{slugifySegment(repo)} - {sha7}"
// The returned string MUST NOT include the .md extension; the controller appends it.
func computeFilenameHint(provider, owner, repo, episodeSHA string) string {
    sha7 := episodeSHA
    if len(sha7) > 7 {
        sha7 = sha7[:7]
    }
    ownerRepo := slugifySegment(owner) + "-" + slugifySegment(repo)
    hint := "Build Failure " + provider + " - " + ownerRepo + " - " + sha7
    if len(hint) > maxFilenameHintLen {
        glog.Warningf(
            "filename_hint exceeds max length: len=%d max=%d — truncating",
            len(hint),
            maxFilenameHintLen,
        )
        hint = hint[:maxFilenameHintLen]
    }
    return hint
}
```

After:
- Rename `maxFilenameHintLen` → `maxTitleLen`
- Rename `computeFilenameHint` → `computeBuildTitle`
- Rename local var `hint` → `title`
- Update doc comment on the const + function
- Update the WARN log: `"filename_hint exceeds max length"` → `"build task title exceeds max length"`

### Callers (update function calls)

**`watcher/github-pr/pkg/watcher.go`** — two call sites (currently around lines 225 and 236):

```go
Title: computePRFilenameHint("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
```

→ becomes:

```go
Title: computePRTitle("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
```

**`watcher/github-build/pkg/watcher.go`** — one call site (currently around line 305):

```go
Title: computeFilenameHint("github", owner, repo, episodeSHA),
```

→ becomes:

```go
Title: computeBuildTitle("github", owner, repo, episodeSHA),
```

### Tests (KEEP unchanged — contract tests)

These test files contain negative-assertion strings that intentionally reference "filename_hint" to lock wire-format regression. **Do NOT edit them.**

- `watcher/github-pr/pkg/filename_internal_test.go` — line ~91-104 asserts `emits 'title' as the top-level key (not 'filename_hint')` and `Expect(string(raw)).NotTo(ContainSubstring(\`"filename_hint"\`))`.
- `watcher/github-build/pkg/watcher_internal_test.go` — line ~134-148 same pattern.

If these tests reference the renamed Go function names (`computeFilenameHint` etc.), update only the function-call sites — keep all string literals containing `"filename_hint"` exactly as-is.

**Specific Go-identifier sites in test files (update these):**

- `watcher/github-pr/pkg/filename_internal_test.go` lines ~51, ~54, ~110, ~112 — calls to `computePRFilenameHint(...)` (rename to `computePRTitle`)
- `watcher/github-build/pkg/watcher_internal_test.go` lines ~34, ~38, ~153, ~155 — calls to `computeFilenameHint(...)` (rename to `computeBuildTitle`)

(Line numbers approximate; use `grep -n 'computeFilenameHint\|computePRFilenameHint' watcher/**/*_test.go` to locate them precisely.)

**Critical**: Do NOT touch any string literal `"filename_hint"` (with quotes). Those are the wire-format negative-assertion contract. After your edits, `git diff` MUST show zero edits inside any `"filename_hint"` string literal.

### Docs (rewrite stale sections)

**`docs/build-watcher.md`** — section currently titled `## filename_hint Field` spans lines 96-122 (header on line 96, body and final paragraph "Schema compatibility" ending around line 122). The section ends just before the next H2 heading `## Per-Repo Configuration` (around line 124). Replace the **entire** section (lines 96 through the blank line before `## Per-Repo Configuration`) with the block below:

```markdown
## Title Field

Every `task.CreateCommand` published by the build watcher sets the `Title` field
with the human-readable filename stem for the vault task file:

\```
Build Failure {provider} - {owner}-{repo} - {sha7}
\```

| Component | Source | Notes |
|---|---|---|
| `Build Failure` | constant | literal |
| `{provider}` | hard-coded `github` in this watcher | future watchers carry their own constant |
| `{owner}-{repo}` | `owner` and `repo` from allowlist entry, slugified independently, joined with `-` | lowercase; non-`[a-z0-9-]` → `-`; leading/trailing hyphens stripped |
| `{sha7}` | first 7 chars of `episode_sha` | matches git's default short-hash length; not slugified |

**Example:** `Build Failure github - bborbe-maintainer - 5886450`

**Future provider slots:** `Build Failure bitbucket - team-svc - a1b2c3d.md`

**Controller behaviour:** The bborbe/agent task controller writes the vault file at
`tasks/{Title}.md` when `Title` is present and passes validation. On validation failure
the controller logs WARN and falls back to `tasks/{task_identifier}.md`. See
`bborbe/agent/specs/completed/019-human-readable-vault-task-paths.md` for the
controller-side contract.
```

(Drop the "Schema compatibility" paragraph entirely — `Title` is a required field on the canonical command, not an optional extension.)

**`docs/architecture.md`** — TWO sites:

1. **Line 187** — JSON example field:
   ```json
   "filename_hint": "Build Failure github - bborbe-maintainer - 5886450",
   ```
   → becomes:
   ```json
   "title": "Build Failure github - bborbe-maintainer - 5886450",
   ```

2. **Lines 196-197** — prose annotation right after the JSON example:
   ```
   `filename_hint` (optional) — human-readable filename stem for the vault task file;
   absent in messages from older watchers; controller falls back to UUID-based name.
   ```
   → becomes:
   ```
   `title` (required) — human-readable filename stem for the vault task file. The
   bborbe/agent task controller writes `tasks/{title}.md`; on validation failure the
   controller logs WARN and falls back to `tasks/{task_identifier}.md`. See
   `bborbe/agent/specs/completed/019-human-readable-vault-task-paths.md` for the
   controller-side contract.
   ```

After both edits, `grep -n filename_hint docs/architecture.md` returns zero matches.

### CHANGELOG (DO NOT EDIT)

Lines 32 + 36 (`feat: emit filename_hint`) and lines 12 + 16 (`refactor: removes WatcherCreateTaskCommand wrapper`) describe what shipped historically. Do not modify them.

This prompt's CHANGELOG entry should be a new line under the next unreleased version, something like:

```
- refactor(watcher): rename internal `filename_hint` terminology to `title` across both watchers — function names, constants, comments, log messages, and stale doc sections updated. Wire format unchanged (already on `Title` field per 0.31.x). Contract tests preserved verbatim.
```

</context>

<constraints>

- Wire format MUST NOT change. The Kafka-published JSON has `"title":` field (not `"filename_hint":`) — this is already true and must remain so.
- Negative-assertion contract tests (`Expect(...).NotTo(ContainSubstring(\`"filename_hint"\`))`) MUST remain unchanged, including the literal string `"filename_hint"` inside them.
- All existing CHANGELOG entries about the historical `filename_hint` shipping/refactor MUST be preserved.
- Behavior MUST NOT change. Vault filenames MUST be byte-identical before/after this change for any given input. (The function bodies are unchanged; only their names + their wrappers change.)
- Errors must be wrapped with `github.com/bborbe/errors`. (No errors are introduced here, but the rule applies if any new ones surface.)
- `make precommit` MUST be clean in both `watcher/github-pr/` and `watcher/github-build/` after the change.
- Coverage on changed packages MUST remain ≥ 80%.

</constraints>

<failure_modes>

| Trigger | Expected behaviour | Recovery |
|---|---|---|
| Test files contain Go-identifier references to old function names (`computeFilenameHint(`, `computePRFilenameHint(`) | Renamed in tests too (Go identifiers, not string literals) | Compiler catches; rename and re-run |
| String literal `"filename_hint"` inside a test still being asserted | KEEP — that's the contract test | Do not change |
| `grep -rn filename_hint watcher/ docs/` returns matches after change | Stop and inspect each: code-residue → fix; CHANGELOG → keep; test-string-literal → keep | Re-run grep until only intended matches remain |
| `make precommit` fails on coverage drop | The function-rename should be value-preserving — investigate which test references stale name | Update test-side identifier references |

</failure_modes>

<acceptance_criteria>

- [ ] `grep -rn computeFilenameHint watcher/ docs/` returns zero matches outside `_test.go` files.
- [ ] `grep -rn computePRFilenameHint watcher/ docs/` returns zero matches outside `_test.go` files.
- [ ] `grep -rn maxFilenameHintLen watcher/ docs/` returns zero matches.
- [ ] `grep -rn '"filename_hint"' watcher/` returns matches ONLY inside `_test.go` files (the negative-assertion contract).
- [ ] `grep -rn filename_hint docs/` returns zero matches.
- [ ] `grep -rn filename_hint CHANGELOG.md` returns matches (historical record preserved).
- [ ] New helpers exist with renamed identifiers: `computePRTitle`, `computeBuildTitle`, `maxTitleLen`.
- [ ] `cd watcher/github-pr && make precommit` exits 0; coverage ≥ 80% on changed packages.
- [ ] `cd watcher/github-build && make precommit` exits 0; coverage ≥ 80% on changed packages.
- [ ] `docs/build-watcher.md` section title is `## Title Field`; describes `task.CreateCommand.Title`; references the agent spec-019.
- [ ] `docs/architecture.md` JSON example uses `"title":` not `"filename_hint":`.
- [ ] CHANGELOG has a new unreleased-version entry describing this refactor.
- [ ] `git diff` shows zero edits inside any string literal `"filename_hint"` (with the surrounding double quotes). The negative-assertion contract is preserved verbatim.
- [ ] `grep -n filename_hint docs/architecture.md` returns zero matches (both the JSON example and the prose annotation are updated).

</acceptance_criteria>

<verification>

```
cd watcher/github-pr && make precommit
cd watcher/github-build && make precommit
grep -rn filename_hint watcher/ docs/   # should match only CHANGELOG + _test.go negative assertions
grep -rn maxFilenameHintLen watcher/    # should be zero
grep -rn computeFilenameHint watcher/   # should be zero in non-_test.go
grep -rn computePRFilenameHint watcher/ # should be zero in non-_test.go
```

Expected:
- Both `make precommit` exit 0
- Greps return only intended matches (CHANGELOG history + test-side string literals)
- No production code or doc references the old terminology

</verification>

<do_nothing_option>

Leaving the residue costs nothing functionally — wire is already on `Title`, controller is already honoring `Title`. Cost of inaction:

- Stale `docs/build-watcher.md` continues to claim `filename_hint` is emitted (false; misleading to anyone reading the docs).
- Function names like `computeFilenameHint` keep returning what is actually a `Title` value — confusing for anyone reading the watcher code cold.
- New planner agents copying the watcher's code path would inherit the stale terminology.

This is a small, low-risk cleanup. Worth doing now while the migration is fresh; less worth doing 6 months from now when the residue has been copy-pasted into more places.

</do_nothing_option>
