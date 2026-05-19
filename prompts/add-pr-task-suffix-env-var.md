---
status: draft
summary: Add WATCHER_GITHUB_PR_TASK_SUFFIX env var to watcher/github-pr so dev and prod watchers writing to the same OpenClaw vault produce distinct PR task filenames (prod empty → no change; dev = "dev" → " - dev" appended after slug). Eliminates YAML merge conflicts in dashboard.
---

<summary>
- Currently both dev and prod `watcher/github-pr` deployments poll `bborbe/go-skeleton` (dev has `REPO_ALLOWLIST=github.com/bborbe/go-skeleton`, prod has `bborbe/*`) and write the same task filename to the OpenClaw vault. obsidian-git merges the two writes and commits unresolved `<<<<<<< HEAD` markers into YAML frontmatter, which the task dashboard parser silently skips — open PRs disappear from the user's view.
- Fix: add `WATCHER_GITHUB_PR_TASK_SUFFIX` config (env `WATCHER_GITHUB_PR_TASK_SUFFIX`, arg `task-suffix`, default empty). Value is a bare label like `dev` — code prepends ` - ` when non-empty and appends it after the existing title (after slug, before .md). Empty value → existing format unchanged.
- Plumb the new param through `application` struct → `factory.CreateWatcher` → `pkg.NewWatcher` → `w.taskSuffix` → `computePRTitle`.
- `computePRTitle` appends ` - <suffix>` AFTER slug truncation but BEFORE maxTitle truncation. If the final title would exceed maxTitle, slug is shrunk; suffix is preserved (it's the disambiguator — losing it defeats the purpose).
- Add test cases in `pkg/filename_internal_test.go`: empty suffix (existing behavior), `suffix="dev"` produces `... - dev`, suffix-pushes-past-maxTitle truncates slug not suffix.
- `make precommit` in `watcher/github-pr/` exits 0.
- CHANGELOG entry, no version bump (next deploy bumps).
</summary>

<objective>
Add a configurable `WATCHER_GITHUB_PR_TASK_SUFFIX` to the `maintainer-watcher-github-pr` binary so two watchers polling overlapping repos can write distinct task filenames into the same OpenClaw vault without colliding. Dev sets the suffix to `dev`; prod leaves it empty.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.

Read these files in full before writing any code:
- `watcher/github-pr/main.go` — `application` struct (around lines 104–121) and `Run` method (around line 188) showing how `MaxSlugLen`/`MaxTitleLen` flow into `factory.CreateWatcher`.
- `watcher/github-pr/pkg/filename.go` — `computePRTitle` function and `DefaultMaxTitleLen`/`DefaultMaxSlugLen` constants.
- `watcher/github-pr/pkg/filename_internal_test.go` — existing table-driven tests for `computePRTitle`.
- `watcher/github-pr/pkg/factory/factory.go` — `CreateWatcher` signature and how it constructs `pkg.NewWatcher`.
- `watcher/github-pr/pkg/watcher.go` — the two `computePRTitle` call sites (trusted at ~line 249, untrusted at ~line 269) and the `watcher` struct fields `maxSlugLen`, `maxTitleLen`.
- `dev.env` and `prod.env` at repo root.
- `k8s/maintainer-watcher-github-pr-sts.yaml` (or whatever StatefulSet manifest sets env vars) — to find where to add the new env var declaration.

Read these coding-guideline files (mounted at `/home/node/.claude/plugins/marketplaces/coding/docs/` in the YOLO container; if not, `find / -name go-testing-guide.md 2>/dev/null | head -1`):
- `go-testing-guide.md`
- `go-error-handling-guide.md` (bborbe/errors usage)

**Background — the bug:**

Same vault (`OpenClaw`), two watcher deployments:
- dev (`REPO_ALLOWLIST=github.com/bborbe/go-skeleton`, `STAGE=dev`)
- prod (`REPO_ALLOWLIST=github.com/bborbe/*`, `STAGE=prod`)

Both see `bborbe/go-skeleton` PRs → both publish `CreateTaskCommand` with the same `Title` → task controller writes the same filename → git push race → obsidian-git merge → committed conflict markers in YAML → dashboard parser skips → user sees no entry for the PR.

Confirmed 2026-05-19 on PR #12 of `bborbe/go-skeleton`. Four task files in OpenClaw had `<<<<<<< HEAD` markers; they have been manually cleaned. This prompt is the durable fix.

**Filename grammar today** (`pkg/filename.go:22-24`):
```
PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]} - {slug}
```

**Filename grammar after this change**, with suffix=`dev`:
```
PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]} - {slug} - dev
```

With suffix=`""`:
```
PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]} - {slug}    (unchanged)
```

</context>

<requirements>
Execute steps in order. Run `make test` after Step 4 for fast feedback. Run `make precommit` only at the final step.

---

## Step 1 — Read all referenced files

Read every file listed in `<context>` before writing a single line of code.

Grep to confirm call sites:
```bash
grep -n "computePRTitle\|maxSlugLen\|maxTitleLen" watcher/github-pr/pkg/watcher.go watcher/github-pr/pkg/factory/factory.go watcher/github-pr/main.go
```

---

## Step 2 — Add `TaskSuffix` field to `application` struct in `main.go`

File: `watcher/github-pr/main.go`

Append after the `MaxTitleLen` field (around line 120). Match the existing field-tag style (note the multi-space alignment is intentional):

```go
TaskSuffix       string           `required:"false" arg:"task-suffix"       env:"WATCHER_GITHUB_PR_TASK_SUFFIX" usage:"Optional suffix appended to PR task filenames as ' - <value>'; empty = no suffix. Set differently per stage (e.g. dev=\"dev\", prod=\"\") to prevent task-file collisions when both watchers poll the same repo into the same vault."`
```

Pass the new field into `factory.CreateWatcher` in `Run` (the existing call around `main.go:188`). Insert `a.TaskSuffix` as the last argument:

```go
w, cleanup, err := factory.CreateWatcher(
    ctx,
    a.GHToken,
    a.KafkaBrokers,
    a.Stage,
    a.RepoScope,
    taskCreationFilter,
    startTime,
    trustedAuthors,
    a.MaxSlugLen,
    a.MaxTitleLen,
    a.TaskSuffix,
)
```

No new validation: an empty suffix is the default and is valid.

---

## Step 3 — Plumb `taskSuffix` through `factory.CreateWatcher` and `pkg.NewWatcher`

File: `watcher/github-pr/pkg/factory/factory.go`

Update `CreateWatcher` signature to accept `taskSuffix string` as the last parameter, and forward it to `pkg.NewWatcher`:

```go
func CreateWatcher(
    ctx context.Context,
    ghToken string,
    brokers libkafka.Brokers,
    stage string,
    repoScope string,
    taskCreationFilter filter.TaskCreationFilter,
    startTime libtime.DateTime,
    trustedAuthors []string,
    maxSlugLen int,
    maxTitleLen int,
    taskSuffix string,
) (pkg.Watcher, func(), error) {
    // ... existing body unchanged until NewWatcher call ...
    w := pkg.NewWatcher(
        ghClient,
        createSender,
        pkg.DefaultCursorPath,
        startTime,
        repoScope,
        taskCreationFilter,
        stage,
        pkg.NewMetrics(),
        trustDecision,
        maxSlugLen,
        maxTitleLen,
        taskSuffix,
    )
```

File: `watcher/github-pr/pkg/watcher.go`

Update `NewWatcher` to accept `taskSuffix string` as the last parameter and store it on the `watcher` struct as `taskSuffix string` (alongside `maxSlugLen`, `maxTitleLen`).

Update both `computePRTitle` call sites (trusted ~line 249, untrusted ~line 269) to pass `w.taskSuffix` as the new last argument.

---

## Step 4 — Extend `computePRTitle` in `pkg/filename.go`

File: `watcher/github-pr/pkg/filename.go`

Add `taskSuffix string` as the last parameter to `computePRTitle`. Append the suffix after the slug-joined base, with the format ` - <suffix>` only when `taskSuffix != ""`. Truncation logic:

- If the combined title exceeds `maxTitle` and a suffix is present, shrink the slug (or the whole pre-suffix base) so the suffix is preserved — the suffix is the disambiguator; losing it defeats the purpose.
- If no suffix, behavior is identical to today.

Concrete implementation:

```go
func computePRTitle(
    provider, owner, repo string,
    number int,
    sha, title string,
    maxSlug, maxTitle int,
    taskSuffix string,
) string {
    shortSHA := sha
    if len(shortSHA) > 8 {
        shortSHA = shortSHA[:8]
    }
    base := fmt.Sprintf("PR Review %s - %s-%s - %d - %s", provider, owner, repo, number, shortSHA)
    slug := slugifyTitle(title, maxSlug)
    var t string
    if slug == "" {
        t = base
    } else {
        t = base + " - " + slug
    }
    var suffixPart string
    if taskSuffix != "" {
        suffixPart = " - " + taskSuffix
    }
    if len(t)+len(suffixPart) > maxTitle {
        glog.Warningf(
            "PR title exceeds max length: len=%d max=%d suffix=%q — truncating slug to preserve suffix",
            len(t)+len(suffixPart),
            maxTitle,
            taskSuffix,
        )
        budget := maxTitle - len(suffixPart)
        if budget < 0 {
            budget = 0
        }
        if len(t) > budget {
            t = t[:budget]
        }
    }
    return t + suffixPart
}
```

Note: trailing-hyphen trim after slug truncation is NOT applied to the suffixed result — `... - dev` ends in `v`, not `-`. The existing `slugifyTitle` already trims trailing hyphens from the slug itself.

---

## Step 5 — Update `pkg/filename_internal_test.go`

Update every existing call to `computePRTitle` to pass `""` as the new last argument (preserving today's behavior).

Then add new table-driven entries / cases covering:

1. **Empty suffix, normal title** — output unchanged from current behavior.
2. **`suffix="dev"`, short title** — output ends in ` - dev`.
3. **`suffix="dev"`, slug empty (unicode-only PR title)** — output is `PR Review github - bborbe-repo - 1 - abc12345 - dev` (no double `- -`, just one ` - dev` after the sha).
4. **`suffix="dev"`, maxTitle small enough that slug must shrink** — suffix preserved; slug truncated.
5. **`suffix="dev"`, maxTitle smaller than suffix alone** — degenerate case; document expected behavior (suffix returned, base eaten entirely; or whatever the code in Step 4 produces — write the assertion to match the implementation, not the other way around).

Use the existing `DescribeTable`/`Entry` style. Keep test names descriptive.

---

## Step 6 — Update `dev.env` and `prod.env`

File: `dev.env`

Append (alphabetical-ish position near other `WATCHER_GITHUB_PR_*` vars):

```
export WATCHER_GITHUB_PR_TASK_SUFFIX=dev
```

File: `prod.env`

Append the same var explicitly empty (so the contract is visible — empty is intentional, not forgotten):

```
export WATCHER_GITHUB_PR_TASK_SUFFIX=
```

---

## Step 7 — Update StatefulSet manifest

File: `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml`

The manifest uses Go-template substitution: `value: '{{ "VARNAME" | env }}'`. Confirm with:

```bash
grep -n "STAGE\|env " watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml
```

Add a new env entry in the container's `env:` block directly after the `STAGE` entry (around line 55), using the same template syntax:

```yaml
            - name: WATCHER_GITHUB_PR_TASK_SUFFIX
              value: '{{ "WATCHER_GITHUB_PR_TASK_SUFFIX" | env }}'
```

When the env var is unset (prod), the template resolves to an empty string — `computePRTitle` treats this as "no suffix" and emits the legacy filename format.

---

## Step 8 — Add CHANGELOG entry

Read `CHANGELOG.md` at the repo root. If `## Unreleased` already exists, append to it; otherwise create it above the most recent `## vX.Y.Z` heading:

```
- feat(watcher/github-pr): add `WATCHER_GITHUB_PR_TASK_SUFFIX` env var; non-empty value is appended as ` - <suffix>` to PR task filenames so dev and prod watchers writing into the same vault produce distinct filenames (dev=`dev` → ` - dev`; prod empty → unchanged). Fixes YAML merge-conflict markers in OpenClaw task files when two watchers poll overlapping repos.
```

---

## Step 9 — Run `make test` (fast feedback)

```bash
cd watcher/github-pr && make test
```

All tests must pass before proceeding. If any fail, fix the root cause — do not proceed to `make precommit` with failing tests.

---

## Step 10 — Run `make precommit`

```bash
cd watcher/github-pr && make precommit
```

Must exit 0.

</requirements>

<constraints>

- **`computePRTitle` is the only behavior-changing function in `pkg/filename.go`**. `slugifyTitle` MUST NOT change.
- **Empty suffix produces byte-identical output to today's behavior**. All existing test rows that don't add a suffix must still pass with `suffix=""`.
- **Suffix preservation under truncation:** when `len(t) + len(" - "+suffix) > maxTitle`, the slug shrinks; the suffix is preserved. Goal: filenames remain distinct between watchers even at the cap.
- **No new validation** in `validateConfig` — empty is a valid suffix value. Do not add a "suffix must be alphanumeric" check; trust the operator. (If desired later, add behind a separate prompt.)
- **No StatefulSet changes if `*.env` files are the source of truth** — confirm via `git log -p k8s/` whether the manifest is generated or hand-maintained. Modify only what is needed.
- **`bborbe/errors`** for any non-test error wrapping. No `fmt.Errorf`. Tests use `Expect(err).NotTo(HaveOccurred())`.
- **No `context.Background()` in non-test code**.
- **`make precommit` in `watcher/github-pr/`** only — never at repo root.
- **Do NOT commit** — dark-factory handles git.
- **Do NOT change `watcher/github-build/`** — that watcher has the same potential issue but is out of scope for this prompt. Note the gap in a comment if necessary.

</constraints>

<verification>

Run precommit:
```bash
cd watcher/github-pr && make precommit
```
Expected: exit 0.

Confirm the new env var is wired:
```bash
grep -n "WATCHER_GITHUB_PR_TASK_SUFFIX\|TaskSuffix\|taskSuffix\|task-suffix" watcher/github-pr/main.go watcher/github-pr/pkg/factory/factory.go watcher/github-pr/pkg/watcher.go watcher/github-pr/pkg/filename.go
```
Expected: matches in all four files.

Confirm test coverage of suffix cases:
```bash
grep -nE 'dev|suffix' watcher/github-pr/pkg/filename_internal_test.go | head -20
```
Expected: ≥3 entries mentioning the suffix in test names.

Confirm dev.env and prod.env updated:
```bash
grep -n "WATCHER_GITHUB_PR_TASK_SUFFIX" dev.env prod.env
```
Expected: one line per file.

Confirm CHANGELOG entry:
```bash
grep -n "WATCHER_GITHUB_PR_TASK_SUFFIX\|task suffix\|task-suffix" CHANGELOG.md | head -3
```
Expected: one entry under `## Unreleased`.

Manual dry-run check (no execution required):
- With suffix="dev", PR #12 of bborbe/go-skeleton would produce: `PR Review github - bborbe-go-skeleton - 12 - 76fe3e86 - improve-readme-fix-header-typo-and-remove-empty-doc-go-files - dev`
- With suffix="", PR #12 produces the same filename it does today.

</verification>
