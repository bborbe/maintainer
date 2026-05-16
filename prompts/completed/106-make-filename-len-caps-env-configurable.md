---
status: completed
summary: Made filename length caps configurable via MAX_SLUG_LEN (default 80, up from 50) and MAX_TITLE_LEN (default 200) env vars in both watchers; watchers fail at startup on invalid values.
container: maintainer-106-make-filename-len-caps-env-configurable
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-08T15:31:49Z"
queued: "2026-05-08T15:31:49Z"
started: "2026-05-08T15:31:51Z"
completed: "2026-05-08T15:49:24Z"
branch: dark-factory/106-make-filename-len-caps-env-configurable
---

<summary>
- Make the slug-length and title-length caps in both maintainer watchers configurable via env vars `MAX_SLUG_LEN` and `MAX_TITLE_LEN`. Currently both are hardcoded `const` in package files (no runtime config).
- Bump default `MAX_SLUG_LEN` from `50` to `80`. The 50 default truncated meaningful PR-title information at byte 50 (e.g. `bborbe/maintainer#4` produced `…rung-2-verificat` instead of `…rung-2-verification`); 80 preserves the full word in typical PR titles while staying well under terminal display widths and the 200-char total cap.
- `MAX_TITLE_LEN` default stays at `200` (filesystem-and-cross-platform safety cap; not aesthetic — see prompt 104's spec-019 rationale).
- Two watchers updated: `watcher/github-pr` (uses both caps) and `watcher/github-build` (uses only `MAX_TITLE_LEN` — there is no slug in build-failure filenames).
- Existing `_test.go` contract assertions stay unchanged; new tests cover env-override + invalid-value-startup-failure.
- Add CHANGELOG entry under the next unreleased section noting the default bump (behaviour change for any PR title >50 chars).
- `make precommit` clean in both watcher modules.
</summary>

<objective>
Eliminate the need to redeploy the watcher just to change the filename length caps. Make the slug cap a runtime tuneable so operators can experiment with longer values per environment without code changes; same for the safety cap. While doing this, bump the slug default to 80 because today's 50 is shorter than typical readable PR titles (GitHub's own list view shows ~50 chars then truncates — we don't need to mirror that for vault filenames).

Why both caps env-configurable, not just slug: symmetry. The 200 hard cap is unlikely to be tuned routinely, but having it config-driven from day one means you can tune it without a code change if a cross-platform / filesystem requirement emerges. Same shape, same plumbing — minimal incremental cost.
</objective>

<context>

## Files to edit

### `watcher/github-pr/pkg/filename.go` — current state

```go
const maxTitleLen = 200
const maxSlugLen = 50

func computePRTitle(provider, owner, repo string, number int, title string) string {
    base := fmt.Sprintf("PR Review %s - %s-%s - %d", provider, owner, repo, number)
    slug := slugifyTitle(title)
    var t string
    if slug == "" {
        t = base
    } else {
        t = base + " - " + slug
    }
    if len(t) > maxTitleLen {
        glog.Warningf(
            "PR title exceeds max length: len=%d max=%d — truncating",
            len(t),
            maxTitleLen,
        )
        t = t[:maxTitleLen]
    }
    return t
}

func slugifyTitle(title string) string {
    // ... lower, replace non-[a-z0-9] with hyphens, collapse, trim ...
    if len(result) > maxSlugLen {
        result = result[:maxSlugLen]
        result = strings.TrimRight(result, "-")
    }
    return result
}
```

After:

```go
// DefaultMaxTitleLen is the default safety cap for the whole title, including segments and separators.
// Crosses Windows MAX_PATH=260 and ext4 NAME_MAX=255 with margin. Override via MAX_TITLE_LEN.
const DefaultMaxTitleLen = 200

// DefaultMaxSlugLen is the default cap for the slugified PR-title segment alone.
// Bumped from 50 to 80 (2026-05-08) — 50 cut typical PR titles mid-word. Override via MAX_SLUG_LEN.
const DefaultMaxSlugLen = 80

// computePRTitle returns the human-readable title for a PR-review task.
// maxSlug caps the slug segment alone; maxTitle is a safety cap on the full title.
// Both are passed by the caller (read from env at startup) — see watcher/github-pr/main.go.
func computePRTitle(provider, owner, repo string, number int, title string, maxSlug, maxTitle int) string {
    base := fmt.Sprintf("PR Review %s - %s-%s - %d", provider, owner, repo, number)
    slug := slugifyTitle(title, maxSlug)
    var t string
    if slug == "" {
        t = base
    } else {
        t = base + " - " + slug
    }
    if len(t) > maxTitle {
        glog.Warningf(
            "PR title exceeds max length: len=%d max=%d — truncating",
            len(t),
            maxTitle,
        )
        t = t[:maxTitle]
    }
    return t
}

func slugifyTitle(title string, maxSlug int) string {
    // ... same body as before, but replace `maxSlugLen` with `maxSlug` ...
    if len(result) > maxSlug {
        result = result[:maxSlug]
        result = strings.TrimRight(result, "-")
    }
    return result
}
```

### `watcher/github-pr/pkg/watcher.go` — current call sites (lines ~225, ~236)

```go
Title: computePRTitle("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
```

After (both call sites):

```go
Title: computePRTitle("github", pr.Owner, pr.Repo, pr.Number, pr.Title, w.maxSlugLen, w.maxTitleLen),
```

The watcher struct gains two int fields (`maxSlugLen`, `maxTitleLen`) populated by the factory at construction.

### `watcher/github-pr/pkg/factory/factory.go` — current `CreateWatcher` signature

Read the file to find the actual `CreateWatcher` signature; add two `int` parameters at the end:

```go
func CreateWatcher(
    ctx context.Context,
    ghToken string,
    // ... existing params ...
    maxSlugLen int,
    maxTitleLen int,
) (Watcher, func(), error) {
    // ... existing body ...
    // when constructing the watcher struct, set the new fields
}
```

### `watcher/github-pr/main.go` — `application` struct

Add two fields (after the existing block, before the closing brace of the struct):

```go
MaxSlugLen   int `required:"false" arg:"max-slug-len"   env:"MAX_SLUG_LEN"   usage:"Max length of slugified PR-title segment in vault filenames"   default:"80"`
MaxTitleLen  int `required:"false" arg:"max-title-len"  env:"MAX_TITLE_LEN"  usage:"Max length of vault task filename (whole title; safety cap)"   default:"200"`
```

In `Run(ctx)`, validate before calling `factory.CreateWatcher`:

```go
if a.MaxSlugLen <= 0 {
    return errors.Errorf(ctx, "MAX_SLUG_LEN must be > 0; got %d", a.MaxSlugLen)
}
if a.MaxTitleLen <= 0 {
    return errors.Errorf(ctx, "MAX_TITLE_LEN must be > 0; got %d", a.MaxTitleLen)
}
if a.MaxSlugLen >= a.MaxTitleLen {
    return errors.Errorf(ctx, "MAX_SLUG_LEN (%d) must be < MAX_TITLE_LEN (%d)", a.MaxSlugLen, a.MaxTitleLen)
}
```

Then pass `a.MaxSlugLen, a.MaxTitleLen` into `factory.CreateWatcher(...)`.

### `watcher/github-build/pkg/filename.go` — current state

```go
const maxTitleLen = 200

func computeBuildTitle(provider, owner, repo, episodeSHA string) string {
    sha7 := episodeSHA
    if len(sha7) > 7 {
        sha7 = sha7[:7]
    }
    ownerRepo := slugifySegment(owner) + "-" + slugifySegment(repo)
    t := "Build Failure " + provider + " - " + ownerRepo + " - " + sha7
    if len(t) > maxTitleLen {
        glog.Warningf(...)
        t = t[:maxTitleLen]
    }
    return t
}
```

After:

```go
// DefaultMaxTitleLen is the default safety cap for build-failure filenames. Override via MAX_TITLE_LEN.
const DefaultMaxTitleLen = 200

func computeBuildTitle(provider, owner, repo, episodeSHA string, maxTitle int) string {
    // ... same body, replace maxTitleLen with maxTitle ...
}
```

(Build-watcher has no slug segment — only `MAX_TITLE_LEN` applies. `MAX_SLUG_LEN` is not added to its `application` struct.)

### `watcher/github-build/pkg/watcher.go` — current call site (line ~305)

```go
Title: computeBuildTitle("github", owner, repo, episodeSHA),
```

After:

```go
Title: computeBuildTitle("github", owner, repo, episodeSHA, w.maxTitleLen),
```

The watcher struct gains a `maxTitleLen int` field.

### `watcher/github-build/pkg/factory/factory.go`

Same shape — add `maxTitleLen int` parameter to `CreateWatcher`, plumb to watcher struct.

### `watcher/github-build/main.go` — `application` struct

Add ONE field (no slug for build):

```go
MaxTitleLen int `required:"false" arg:"max-title-len" env:"MAX_TITLE_LEN" usage:"Max length of vault task filename (whole title; safety cap)" default:"200"`
```

Validate `> 0` in `Run(ctx)` before passing to factory.

### Tests

**`watcher/github-pr/pkg/filename_internal_test.go`** — keep all existing wire-format negative-assertion entries verbatim. Add table entries (or new `Describe` block) to cover:

1. Default-shape: `computePRTitle("github", "bborbe", "x", 1, "test", 80, 200)` produces a slug truncated at 80, not 50.
2. Custom slug cap: `computePRTitle("github", "bborbe", "x", 1, "<long-title>", 30, 200)` produces a slug truncated at 30 with hyphen-trim.
3. Title cap kicks in: small slug cap + huge owner/repo combo → total truncated at maxTitle.

**`watcher/github-build/pkg/watcher_internal_test.go`** — add table entries covering custom maxTitle.

**`main_test.go` (or new test next to `main.go` if preferred)** — startup validation rejects `MAX_SLUG_LEN=0`, `MAX_SLUG_LEN=-5`, `MAX_TITLE_LEN=0`, and `MAX_SLUG_LEN >= MAX_TITLE_LEN`.

### CHANGELOG.md — new unreleased-version entry

Top of file currently has `v0.23.32` as the latest entry. Create a NEW unreleased section above it (or place under existing `## Unreleased` if it's already there from prompt 105) with this entry:

```
- feat(watcher/github-pr,watcher/github-build): make filename length caps configurable via env vars `MAX_SLUG_LEN` (default `80`, was `50` const) and `MAX_TITLE_LEN` (default `200`, unchanged). Bump of slug default from 50→80 preserves typical PR-title information that previously truncated mid-word. Watchers fail-loud at startup if either value is ≤0 or if MAX_SLUG_LEN >= MAX_TITLE_LEN. github-build only honors MAX_TITLE_LEN (no slug in build-failure filenames).
```

</context>

<constraints>

- Wire format MUST NOT change. The Kafka-published JSON still has `"title":` field. Negative-assertion contract tests (`Expect(...).NotTo(ContainSubstring(\`"filename_hint"\`))`) MUST remain unchanged.
- All existing CHANGELOG entries MUST be preserved unchanged.
- Existing exported APIs in the `pkg` packages MUST not break unrelated callers — search for any external use of `computePRTitle` / `computeBuildTitle` / `slugifyTitle` before changing signatures (`grep -rn 'computePRTitle\|computeBuildTitle\|slugifyTitle' --include='*.go'`). If only the package's own watcher.go and tests use them, refactor freely.
- Errors MUST be wrapped with `github.com/bborbe/errors`. No `fmt.Errorf` for error wrapping.
- Validation must reject invalid env at startup with a clear error message — not silently fall back to defaults. Operators who set `MAX_SLUG_LEN=banana` should see the watcher refuse to start.
- Coverage on changed packages MUST remain ≥80%.
- `make precommit` MUST be clean in both `watcher/github-pr/` and `watcher/github-build/`.

</constraints>

<failure_modes>

| Trigger | Expected behaviour | Recovery |
|---|---|---|
| `MAX_SLUG_LEN=0` or unset to `0` via flag | Startup fails loudly with `MAX_SLUG_LEN must be > 0; got 0` | Fix env var; watcher restarts cleanly |
| `MAX_SLUG_LEN=banana` (non-int) | Startup fails (the `argument` library's int parsing fails before `Run` is called); error mentions the invalid value | Fix env var |
| `MAX_SLUG_LEN=300, MAX_TITLE_LEN=200` | Startup fails: `MAX_SLUG_LEN (300) must be < MAX_TITLE_LEN (200)` | Reduce slug cap or raise title cap |
| Default-only run (no env set) | Behaves as if `MAX_SLUG_LEN=80, MAX_TITLE_LEN=200` — slug now goes to 80 instead of 50 | This is the intended bump — verify in dev before prod |
| Existing vault tasks with 50-char-truncated names | Untouched. Existing `task_identifier`-keyed lookup unchanged | None |
| External caller of `computePRTitle` / `slugifyTitle` outside the watcher package | Compile error → caller updated | If grep reveals an unexpected caller, update it; if many callers exist, consider keeping a no-arg wrapper that calls the new function with defaults |

</failure_modes>

<acceptance_criteria>

- [ ] `watcher/github-pr/pkg/filename.go` exports `DefaultMaxSlugLen = 80` and `DefaultMaxTitleLen = 200` consts.
- [ ] `watcher/github-build/pkg/filename.go` exports `DefaultMaxTitleLen = 200`.
- [ ] `computePRTitle(...)` signature gains `maxSlug, maxTitle int`; `slugifyTitle(...)` gains `maxSlug int`.
- [ ] `computeBuildTitle(...)` signature gains `maxTitle int`.
- [ ] `watcher/github-pr/main.go` `application` struct adds `MaxSlugLen` + `MaxTitleLen` fields with `env:` and `default:` tags.
- [ ] `watcher/github-build/main.go` `application` struct adds `MaxTitleLen` field with `env:` and `default:` tag.
- [ ] Both watchers' `Run(ctx)` validate the values before passing to factory; reject ≤0 and `MaxSlugLen >= MaxTitleLen`.
- [ ] Both factories accept the new int params and plumb them onto the watcher struct.
- [ ] `grep -rn 'computePRTitle\|computeBuildTitle\|slugifyTitle' --include='*.go' watcher/` shows callers passing the new params (not the removed const).
- [ ] `grep -rnE 'const\s+(maxSlugLen|maxTitleLen)\b' --include='*.go' watcher/` returns ZERO matches (old lowercase consts removed; only renamed `DefaultMaxSlugLen` / `DefaultMaxTitleLen` exist).
- [ ] Existing wire-format contract tests (`emits 'title' as the top-level key`) pass unchanged.
- [ ] New tests cover: default-cap of 80, custom-slug-cap of 30, title-cap-kicks-in, and `MaxSlugLen >= MaxTitleLen` startup rejection.
- [ ] CHANGELOG has a new unreleased-version entry.
- [ ] `cd watcher/github-pr && make precommit` exits 0; coverage ≥80%.
- [ ] `cd watcher/github-build && make precommit` exits 0; coverage ≥80%.
- [ ] No production behaviour change other than the slug-default bump 50→80 (which is the documented intent).

</acceptance_criteria>

<verification>

```bash
cd watcher/github-pr && make precommit
cd watcher/github-build && make precommit

# Confirm new env vars are documented in --help
go run ./watcher/github-pr --help 2>&1 | grep -E "MAX_SLUG_LEN|MAX_TITLE_LEN"
go run ./watcher/github-build --help 2>&1 | grep -E "MAX_TITLE_LEN"

# Confirm grep evidence — old lowercase consts gone; new exported defaults present
grep -rnE 'const\s+(maxSlugLen|maxTitleLen)\b' --include='*.go' watcher/   # expect zero matches (renamed to DefaultMax*)
grep -rn 'DefaultMaxSlugLen\|DefaultMaxTitleLen' --include='*.go' watcher/  # expect matches in both filename.go files

# Negative test (manual): start a watcher with MAX_SLUG_LEN=0; expect startup error
MAX_SLUG_LEN=0 GH_TOKEN=fake KAFKA_BROKERS=localhost STAGE=dev TRUSTED_AUTHORS=bborbe \
    go run ./watcher/github-pr 2>&1 | grep -E "MAX_SLUG_LEN must be > 0"
```

Expected:
- Both `make precommit` exit 0
- Help text mentions both env vars in github-pr, only `MAX_TITLE_LEN` in github-build
- No remaining lowercased `maxSlugLen` / `maxTitleLen` consts (renamed exported `Default*` versions only)
- Manual startup with invalid env produces the expected error message

</verification>

<do_nothing_option>

Without env-configurability, tuning the slug cap requires a code change + redeploy cycle. The default of 50 has aged: typical informative PR titles are 50-72 chars, so the cap routinely truncates mid-word. Operators wanting a longer cap have no escape hatch.

Bumping the default 50→80 alone (without env config) buys time but doesn't solve the underlying lack-of-tuning problem. Doing both — env config + sensible new default — closes both issues in one prompt.

If we don't ship this, expect another prompt in 2-3 months when someone wants 100 (then again when someone wants 60 because they think 80 is too long, etc.).

</do_nothing_option>
