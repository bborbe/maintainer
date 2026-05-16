---
status: completed
spec: [029-migrate-callers-to-repoallowlist-lib-and-wildcard-rollout]
summary: Migrated all five REPO_ALLOWLIST callers (agent/pr-reviewer main + run-task, watcher/github-pr main, watcher/github-build main + run-once) to use shared lib/repoallowlist package — replaced inline regex parsers with IsAllowed predicate and Validate validator, added replace directives to three go.mod files, updated all affected tests, and fixed existing filter_test.go to use 3-segment host-qualified entries.
container: maintainer-117-spec-029-code-migration
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-15T20:05:00Z"
queued: "2026-05-15T20:09:41Z"
started: "2026-05-15T20:09:43Z"
completed: "2026-05-15T20:23:37Z"
branch: dark-factory/migrate-callers-to-repoallowlist-lib-and-wildcard-rollout
---

<summary>

- All five binaries that consume `REPO_ALLOWLIST` now route their allowlist decisions through the shared `lib/repoallowlist` package — no inline regex validation or exact-match loops remain in any of the five callers
- PR reviewer, its run-task CLI, and the PR watcher preserve "empty allowlist means allow-all" semantics via the library's predicate; malformed entries are logged at match time (not startup) and silently skipped
- Build watcher (main + run-once CLI) validates the full allowlist at startup via the library's validator; every malformed entry surfaces in a single aggregate error, not one at a time; empty list still causes startup refusal
- All five `main.go` files now import `github.com/bborbe/maintainer/lib/repoallowlist` directly (for Validate calls or warning logs)
- The `github.com/<owner>/*` wildcard shape is now accepted by every caller: optional callers pass wildcards to `IsAllowed` which matches any repo under that owner; required callers accept wildcards in `Validate` without error
- Each service's `go.mod` gains a `replace github.com/bborbe/maintainer/lib => ../../lib` directive so the local module is resolved without a published tag
- All modified test files are updated: error-on-malformed tests are removed (library handles gracefully), wildcard-match tests are added for filter and allowlist check code
- `make precommit` passes in all three affected service directories

</summary>

<objective>

Replace the three copies of inline `ParseRepoAllowlist` (using a literal regex and exact-match loop) with calls to `lib/repoallowlist.IsAllowed` for matching and `lib/repoallowlist.Validate` for startup validation. After this change, every REPO_ALLOWLIST consumer evaluates allow decisions through the same shared library, enabling the `github.com/<owner>/*` wildcard syntax to work consistently across the full watcher → agent pipeline.

</objective>

<context>

Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.

Read these guides before writing any code (in `~/.claude/plugins/marketplaces/coding/docs/`):
- `go-patterns.md` — interface/constructor/struct, error wrapping
- `go-error-wrapping-guide.md` — `bborbe/errors` only; never `fmt.Errorf`; aggregate error pattern
- `go-testing-guide.md` — Ginkgo v2, DescribeTable, external `_test` package, coverage ≥80%
- `go-factory-pattern.md` — `Create*` prefix, zero-logic factories
- `go-composition.md` — DI, no package-level calls
- `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` — which test types to write

**Dependency on spec 028:**

Verify the `lib/repoallowlist` package exists before proceeding:

```bash
grep "^func IsAllowed\|^func Validate" /workspace/lib/repoallowlist/repoallowlist.go
```

Expected: both `IsAllowed` and `Validate` present. If the file is missing, STOP and report
`{"status":"failed","message":"lib/repoallowlist not found — spec 028 must be complete first"}`.

**API contract (do NOT import symbols that differ):**

```go
// IsAllowed reports whether target is permitted by allowlist.
// Empty/nil allowlist returns true (allow-all). Empty target with non-empty allowlist returns false.
// Malformed entries are logged via glog.Errorf and skipped — no error returned.
func IsAllowed(allowlist []string, target string) bool

// Validate returns nil for empty/valid allowlist, or aggregate error listing every malformed entry.
// Whitespace-only and empty entries are silently skipped (not malformed).
func Validate(ctx context.Context, allowlist []string) error
```

Import path: `github.com/bborbe/maintainer/lib/repoallowlist`

**Files to read fully before making any changes:**

- `agent/pr-reviewer/go.mod` — current replace block and require versions
- `watcher/github-pr/go.mod` — same
- `watcher/github-build/go.mod` — same
- `lib/go.mod` — lib module path and go version
- `agent/pr-reviewer/pkg/allowlist.go` — current ParseRepoAllowlist implementation to replace
- `agent/pr-reviewer/pkg/allowlist_test.go` — current tests to update
- `agent/pr-reviewer/pkg/steps_checkout_execution.go` — `checkAllowlist` method at lines ~166-200
- `agent/pr-reviewer/main.go` — current ParseRepoAllowlist call site
- `agent/pr-reviewer/cmd/run-task/main.go` — current ParseRepoAllowlist call site
- `watcher/github-pr/pkg/filter/repo_allowlist_filter.go` — current ParseRepoAllowlist + Skip impl
- `watcher/github-pr/pkg/filter/repo_allowlist_filter_test.go` — current tests to update
- `watcher/github-pr/main.go` — lines 130-145, ParseRepoAllowlist call site
- `watcher/github-build/pkg/filter/repo_allowlist_filter.go` — current ParseRepoAllowlist + Skip impl
- `watcher/github-build/pkg/filter/repo_allowlist_filter_test.go` — current tests to update
- `watcher/github-build/main.go` — lines 59-70, ParseRepoAllowlist + non-empty check
- `watcher/github-build/cmd/run-once/main.go` — lines 43-52, same pattern
- `CHANGELOG.md` at repo root — check for existing `## Unreleased` section

**Key fact about watcher/github-build allowlist semantics:**

The build watcher uses `allowlist []string` in two ways:
1. As the list of repo keys to iterate in `Poll` (each entry is polled for failed GitHub Actions runs)
2. As the source for `filter.NewRepoAllowlistFilter` which then calls `repoFilter.Skip(repoKey)`

After migration, `repoAllowlistFilter.Skip(repoKey string)` delegates to `repoallowlist.IsAllowed(f.allowlist, repoKey)`. When the allowlist contains `github.com/bborbe/*` as a wildcard, `IsAllowed` will return true for any three-segment target under `bborbe`, including the wildcard entry itself when used as a repoKey. This means `pollRepo(ctx, cursor, "github.com/bborbe/*")` will be called during `Poll` — it will get a 404 from GitHub API for a repo named `*`. The watcher handles GitHub API errors per-repo gracefully (logs and continues). This is an accepted known limitation in the context of this spec; future work may add wildcard-expansion via repo enumeration.

</context>

<requirements>

Execute steps in order. Run `make test` after completing each module's changes as fast-feedback. Run `make precommit` in each service dir only at the final step for that service.

---

## Step 1 — Read all referenced files fully

Read every file listed in `<context>` before writing any code.

---

## Step 2 — Add lib dependency to `agent/pr-reviewer/go.mod`

Edit `agent/pr-reviewer/go.mod`:

1. Add `github.com/bborbe/maintainer/lib => ../../lib` to the existing `replace (...)` block.
2. Add `github.com/bborbe/maintainer/lib v0.0.0-00010101000000-000000000000` to the `require (...)` block (direct dependencies section).

Then run:
```bash
cd /workspace/agent/pr-reviewer && go mod tidy
```

`go mod tidy` will resolve the indirect transitive deps from `lib/go.mod` (bborbe/errors and glog are already present; no new deps expected). Verify the replace directive is in go.mod after tidy.

---

## Step 3 — Migrate `agent/pr-reviewer/pkg/allowlist.go`

Read the full file first.

Replace the function body of `ParseRepoAllowlist` to remove the regex validation. The new implementation simply splits, trims, and returns — no error for malformed entries. The function signature stays unchanged for backward compatibility.

**New `agent/pr-reviewer/pkg/allowlist.go`:**

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"strings"
)

// ParseRepoAllowlist parses a comma-separated allowlist string into a slice
// of host-qualified repo keys. Whitespace is trimmed; empty entries are skipped.
// Returns (nil, nil) for empty input (allow-all).
// Entry well-formedness is NOT validated here — call repoallowlist.Validate at
// startup for fail-fast validation, or rely on repoallowlist.IsAllowed which
// logs and skips malformed entries at match time.
func ParseRepoAllowlist(_ context.Context, raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var result []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			result = append(result, entry)
		}
	}
	return result, nil
}
```

Remove the `regexp` and `github.com/bborbe/errors` imports (no longer needed). Add `"context"` if not present (it's the `_` parameter).

---

## Step 4 — Update `agent/pr-reviewer/pkg/allowlist_test.go`

Read the full file first.

Remove the test cases that expect errors for malformed entries (two-segment, single-segment, four-segment). Those behaviors now live in the library.

Add new test cases:

```go
It("accepts wildcard entry without error", func() {
    result, err := pkg.ParseRepoAllowlist(ctx, "github.com/bborbe/*")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal([]string{"github.com/bborbe/*"}))
})

It("accepts malformed two-segment entry without error (library handles at match time)", func() {
    result, err := pkg.ParseRepoAllowlist(ctx, "bborbe/maintainer")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal([]string{"bborbe/maintainer"}))
})
```

Keep the remaining tests (empty string → nil, single valid entry, multiple entries, whitespace trimming, trailing comma).

---

## Step 5 — Update `agent/pr-reviewer/pkg/steps_checkout_execution.go`

Read the full file first. Locate `checkAllowlist` (around line 166).

Replace the exact-match loop with `repoallowlist.IsAllowed`. Add import `repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"`.

**Replace this block:**
```go
for _, entry := range s.repoAllowlist {
    if entry == repoKey {
        return nil
    }
}
return &agentlib.Result{
    Status: agentlib.AgentStatusNeedsInput,
    Message: fmt.Sprintf(
        "execution step: repo %q is not on the allowlist (%d entries); task routed to human review without clone",
        repoKey,
        len(s.repoAllowlist),
    ),
}
```

**With:**
```go
if repoallowlist.IsAllowed(s.repoAllowlist, repoKey) {
    return nil
}
return &agentlib.Result{
    Status: agentlib.AgentStatusNeedsInput,
    Message: fmt.Sprintf(
        "execution step: repo %q is not on the allowlist (%d entries); task routed to human review without clone",
        repoKey,
        len(s.repoAllowlist),
    ),
}
```

Do NOT change the early-return `if len(s.repoAllowlist) == 0 { return nil }` check — it remains for efficiency (avoids unnecessary imports when allow-all).

---

## Step 6 — Update `agent/pr-reviewer/pkg/steps_checkout_execution_test.go`

Read the full file. Find the `allowlist checks` context block.

Add a new test case inside the "when allowlist is non-empty" group for wildcard matching:

```go
Context("when allowlist contains a wildcard and clone_url matches the owner", func() {
    It("permits the clone (wildcard match)", func() {
        stepWithWildcard := pkg.NewCheckoutExecutionStep(
            fakeRepoManager,
            claudeConfigDir,
            agentDir,
            model,
            env,
            allowedTools,
            reviewMode,
            []string{"github.com/bborbe/*"},
            nil,
        )
        // Use the same valid task markdown with a github.com/bborbe/* clone URL
        // The clone_url in the fixture should resolve to github.com/bborbe/<some-repo>
        // Run the step — it must NOT return needs_input
        // (it may fail for other reasons like actual clone failing, but not the allowlist)
        result, runErr := stepWithWildcard.Run(ctx, md)
        _ = runErr
        if result != nil {
            Expect(result.Status).NotTo(Equal(agentlib.AgentStatusNeedsInput),
                "wildcard allowlist should permit bborbe repo but got needs_input")
        }
    })
})
```

Read the existing test fixtures (the `md` variable and `fakeRepoManager` setup) to adapt the test to the actual fixture shape. Mirror the exact parameter count for `NewCheckoutExecutionStep` from the existing tests in the file.

---

## Step 7 — Update `agent/pr-reviewer/main.go` (optional caller)

Read the full file first.

Add import `repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"`.

After the existing `ParseRepoAllowlist` call and error check, add a non-fatal Validate call:

```go
repoAllowlist, err := prpkg.ParseRepoAllowlist(ctx, a.RepoAllowlist)
if err != nil {
    jobMetrics.RecordRun(agentlib.AgentStatusFailed)
    jobMetrics.RecordDuration(time.Since(start))
    return err
}
// Warn on malformed entries; allow-all and wildcard semantics handled by IsAllowed at match time.
if validationErr := repoallowlist.Validate(ctx, repoAllowlist); validationErr != nil {
    glog.Warningf("REPO_ALLOWLIST contains malformed entries (will be ignored at match time): %v", validationErr)
}
glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))
```

No other changes to main.go.

---

## Step 8 — Update `agent/pr-reviewer/cmd/run-task/main.go` (optional caller)

Same pattern as step 7. Add the same import and non-fatal Validate call after `ParseRepoAllowlist`. No other changes.

---

## Step 9 — Run `make test` in agent/pr-reviewer (fast feedback)

```bash
cd /workspace/agent/pr-reviewer && make test
```

Fix all compile errors and test failures before proceeding to the next module.

---

## Step 10 — Add lib dependency to `watcher/github-pr/go.mod`

Similar to step 2, BUT `watcher/github-pr/go.mod` does NOT currently have a `replace (...)` block (verified before this prompt was written). Edit it:
- **If a `replace (...)` block exists**: append `github.com/bborbe/maintainer/lib => ../../lib` to it.
- **Otherwise (this is the current state)**: insert a new replace block ABOVE the `require (...)` block:
  ```go
  replace (
  	github.com/bborbe/maintainer/lib => ../../lib
  )
  ```
- In the `require (...)` block, add: `github.com/bborbe/maintainer/lib v0.0.0-00010101000000-000000000000`

```bash
cd /workspace/watcher/github-pr && go mod tidy
```

---

## Step 11 — Migrate `watcher/github-pr/pkg/filter/repo_allowlist_filter.go`

Read the full file first.

**Changes:**

1. Add import `repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"`.
2. Remove the `regexp` import and `repoAllowlistEntryPattern` var.
3. Replace `ParseRepoAllowlist` body with the same simple split+trim pattern (no regex, no error):

```go
// ParseRepoAllowlist parses a comma-separated allowlist string into a slice
// of host-qualified repo keys. Whitespace is trimmed; empty entries are skipped.
// Returns (nil, nil) for empty input (allow-all).
// Entry well-formedness is NOT validated here — repoallowlist.IsAllowed handles
// malformed entries gracefully at match time (logs and skips).
func ParseRepoAllowlist(_ context.Context, raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var result []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			result = append(result, entry)
		}
	}
	return result, nil
}
```

4. Update `repoAllowlistFilter.Skip` to use `repoallowlist.IsAllowed`:

```go
func (f *repoAllowlistFilter) Skip(pr PR) bool {
	return !repoallowlist.IsAllowed(f.allowlist, pr.RepoKey)
}
```

Remove the manual loop and the `len(f.allowlist) == 0` check (IsAllowed already handles allow-all semantics).

Remove `github.com/bborbe/errors` from imports if no longer used.

---

## Step 12 — Update `watcher/github-pr/pkg/filter/repo_allowlist_filter_test.go`

Read the full file first.

**Remove** test cases that expect errors for malformed entries (two-segment, single-segment, four-segment entries). These behaviors now live in the library.

**Add** new test cases for `ParseRepoAllowlist`:

```go
It("accepts wildcard entry without error", func() {
    result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/*")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal([]string{"github.com/bborbe/*"}))
})

It("accepts malformed two-segment entry without error", func() {
    result, err := filter.ParseRepoAllowlist(ctx, "bborbe/code-reviewer")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal([]string{"bborbe/code-reviewer"}))
})
```

**Add** new test cases for `RepoAllowlistFilter`:

```go
It("does not skip a PR whose RepoKey matches a wildcard allowlist entry", func() {
    f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/*"})
    Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/maintainer"})).To(BeFalse())
})

It("skips a PR whose RepoKey owner does not match the wildcard entry", func() {
    f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/*"})
    Expect(f.Skip(filter.PR{RepoKey: "github.com/other-owner/repo"})).To(BeTrue())
})
```

Check the `PR` struct definition to confirm the field name `RepoKey` before writing the tests:
```bash
grep -n "type PR struct\|RepoKey" /workspace/watcher/github-pr/pkg/filter/*.go | head -10
```

---

## Step 13 — Update `watcher/github-pr/main.go` (optional caller)

Read the relevant section (lines 130-145) first.

Add import `repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"`.

After the `filter.ParseRepoAllowlist` call and existing error/empty-check log, add non-fatal Validate:

```go
repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
if err != nil {
    return err
}
// Warn on malformed entries; allow-all and wildcard semantics handled by IsAllowed at match time.
if validationErr := repoallowlist.Validate(ctx, repoAllowlist); validationErr != nil {
    glog.Warningf("REPO_ALLOWLIST contains malformed entries (will be ignored at match time): %v", validationErr)
}
if len(repoAllowlist) == 0 {
    glog.V(2).Infof("repo-allowlist count=0 (allow-all)")
} else {
    glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))
}
```

No other changes.

---

## Step 14 — Run `make test` in watcher/github-pr (fast feedback)

```bash
cd /workspace/watcher/github-pr && make test
```

Fix all compile errors and test failures before proceeding.

---

## Step 15 — Add lib dependency to `watcher/github-build/go.mod`

Same conditional handling as step 10 — `watcher/github-build/go.mod` does NOT currently have a `replace (...)` block. Edit it:
- **If a `replace (...)` block exists**: append `github.com/bborbe/maintainer/lib => ../../lib` to it.
- **Otherwise (current state)**: insert a new replace block above `require (...)`:
  ```go
  replace (
  	github.com/bborbe/maintainer/lib => ../../lib
  )
  ```
- In `require (...)`, add: `github.com/bborbe/maintainer/lib v0.0.0-00010101000000-000000000000`

```bash
cd /workspace/watcher/github-build && go mod tidy
```

---

## Step 16 — Migrate `watcher/github-build/pkg/filter/repo_allowlist_filter.go`

Same pattern as step 11. Read the full file first.

1. Add import `repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"`.
2. Remove `regexp` import and `repoAllowlistEntryPattern` var.
3. Replace `ParseRepoAllowlist` body with simple split+trim (no regex, always nil error).
4. Update `repoAllowlistFilter.Skip(repoKey string)` to use `repoallowlist.IsAllowed`:

```go
func (f *repoAllowlistFilter) Skip(repoKey string) bool {
	return !repoallowlist.IsAllowed(f.allowlist, repoKey)
}
```

(The build watcher filter's `Skip` takes a `string`, not a `PR` struct — confirm by reading the file.)

---

## Step 17 — Update `watcher/github-build/pkg/filter/repo_allowlist_filter_test.go`

Read the full file first.

Remove error-for-malformed test cases. Add wildcard tests:

```go
It("accepts wildcard entry without error", func() {
    result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/*")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal([]string{"github.com/bborbe/*"}))
})

It("accepts malformed entry without error", func() {
    result, err := filter.ParseRepoAllowlist(ctx, "bborbe/repo")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal([]string{"bborbe/repo"}))
})
```

For `RepoAllowlistFilter`:

```go
It("does not skip a repoKey that matches a wildcard allowlist entry", func() {
    f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/*"})
    Expect(f.Skip("github.com/bborbe/go-skeleton")).To(BeFalse())
})

It("skips a repoKey whose owner does not match the wildcard entry", func() {
    f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/*"})
    Expect(f.Skip("github.com/other-owner/repo")).To(BeTrue())
})
```

**Also update the regression test** that uses the dev.env literal value:

```go
It("parses real dev.env value (regression: startup shape)", func() {
    result, err := filter.ParseRepoAllowlist(
        ctx,
        "github.com/bborbe/go-skeleton,github.com/bborbe/jira-task-creator",
    )
    Expect(err).NotTo(HaveOccurred())
    Expect(
        result,
    ).To(ConsistOf("github.com/bborbe/go-skeleton", "github.com/bborbe/jira-task-creator"))
})

It("parses wildcard value (future dev.env shape after env update)", func() {
    result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/*")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal([]string{"github.com/bborbe/*"}))
})
```

---

## Step 18 — Update `watcher/github-build/main.go` (required caller)

Read the full file first.

Add import `repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"`.

Replace the current pattern:
```go
repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
if err != nil {
    return err
}
if len(repoAllowlist) == 0 {
    return errors.Errorf(
        ctx,
        "REPO_ALLOWLIST must be non-empty: set at least one host/owner/repo entry",
    )
}
glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))
```

With:
```go
repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
if err != nil {
    return err
}
// Validate ALL entries at startup — aggregate error names every malformed entry.
if validationErr := repoallowlist.Validate(ctx, repoAllowlist); validationErr != nil {
    return errors.Wrap(ctx, validationErr, "REPO_ALLOWLIST contains malformed entries")
}
if len(repoAllowlist) == 0 {
    return errors.Errorf(
        ctx,
        "REPO_ALLOWLIST must be non-empty: set at least one host/owner/repo entry",
    )
}
glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))
```

---

## Step 19 — Update `watcher/github-build/cmd/run-once/main.go` (required caller)

Same pattern as step 18. Read the full file first. Add the same import and Validate call.

---

## Step 19b — Add level-1 boundary test for `Validate` aggregate-error behavior

Required callers (`watcher/github-build/main.go` + `cmd/run-once/main.go`) add a NEW critical control point: `repoallowlist.Validate(ctx, allowlist)` as a fail-fast gate at the top of `Run`. This boundary needs a test that exercises the **real production path** (not just `Validate` in isolation — that's covered in spec 028's tests).

Add a test in `watcher/github-build/main_test.go` (or `cmd/run-once/main_test.go` — pick whichever has the existing test scaffolding; or add in BOTH if both already have tests). Use Ginkgo + Gomega:

```go
Context("Run with malformed REPO_ALLOWLIST entries", func() {
    It("fails fast with an aggregate error naming every malformed entry", func() {
        app := App{
            // ... minimal fields to reach the Validate call;
            // do NOT spin up Kafka/HTTP — Validate is the FIRST thing in Run
            RepoAllowlist: "github.com/bborbe/maintainer,bad-entry,also/bad",
        }
        err := app.Run(ctx, sentryClient)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("bad-entry"))
        Expect(err.Error()).To(ContainSubstring("also/bad"))
    })

    It("accepts a well-formed wildcard without error from Validate", func() {
        // Use a stub or short-circuited App that returns early after Validate succeeds,
        // OR catch a known-later error (e.g. missing Kafka broker) and assert it is
        // NOT the Validate error. The point: Validate alone does not reject
        // "github.com/bborbe/*".
        app := App{RepoAllowlist: "github.com/bborbe/*", /* ... */}
        err := app.Run(ctx, sentryClient)
        if err != nil {
            Expect(err.Error()).NotTo(ContainSubstring("repo-allowlist"))
        }
    })
})
```

These two cases together prove (1) `Validate`'s aggregate-error semantics propagate through the wrap, and (2) wildcard entries reach `Validate` without false rejection — directly satisfying spec 029 AC #3.

---

## Step 20 — Run `make test` in watcher/github-build (fast feedback)

```bash
cd /workspace/watcher/github-build && make test
```

Fix all compile errors and test failures before proceeding.

---

## Step 21 — Add CHANGELOG entry

Read `CHANGELOG.md` at repo root. If `## Unreleased` already exists, append to it; otherwise create it above the most recent `## vX.Y.Z` heading.

```markdown
## Unreleased

- feat: migrate all five REPO_ALLOWLIST callers to shared `lib/repoallowlist` package; replace inline regex parsers with `IsAllowed` predicate (supporting `github.com/<owner>/*` wildcard) and `Validate` validator (aggregate error for required callers); add `replace github.com/bborbe/maintainer/lib => ../../lib` to three service go.mod files
```

---

## Step 22 — Run `make precommit` in each service dir

```bash
cd /workspace/agent/pr-reviewer && make precommit
```

Must exit 0. If any target fails, fix and re-run only the failing target (`make lint`, `make gosec`, etc.) before re-running full precommit.

```bash
cd /workspace/watcher/github-pr && make precommit
```

Must exit 0.

```bash
cd /workspace/watcher/github-build && make precommit
```

Must exit 0.

</requirements>

<constraints>

- The `lib/repoallowlist` API is frozen by spec 028 — do NOT modify any file under `lib/`. This spec is a consumer only.
- Every `main.go` under `agent/pr-reviewer/`, `agent/pr-reviewer/cmd/run-task/`, `watcher/github-pr/`, `watcher/github-build/`, and `watcher/github-build/cmd/run-once/` MUST import `github.com/bborbe/maintainer/lib/repoallowlist`. The import is satisfied by the `repoallowlist.Validate` call added in each main.go.
- Optional callers (PR reviewer, cmd/run-task, PR watcher): Validate call is non-fatal — log a warning, do NOT return an error. Empty allowlist remains allow-all.
- Required callers (build watcher main + cmd/run-once): Validate call is fatal — return wrapped error on malformed entries. Empty allowlist still causes startup refusal (separate `len == 0` check after Validate).
- `ParseRepoAllowlist` signature `(ctx context.Context, raw string) ([]string, error)` is preserved in all three pkg files. The context parameter becomes `_` (unused) in the new implementation since validation moved to the caller. The error return is always nil. Do NOT rename or remove the function — callers in main.go depend on it.
- The `replace github.com/bborbe/maintainer/lib => ../../lib` directive uses a relative path. This resolves correctly because YOLO containers run with the workspace at `/workspace`.
- Errors wrapped with `github.com/bborbe/errors` only (`errors.Wrap`, `errors.Wrapf`, `errors.Errorf`). Never `fmt.Errorf`.
- `context.Background()` is allowed in TEST code only (Ginkgo `BeforeEach`). Production code receives `ctx` from the caller.
- Do NOT modify files under `lib/`, `k8s/`, `*.env`, `scenarios/`, or any file not listed in the requirements.
- Do NOT commit — dark-factory handles git.
- Run `make test` after each module (fast feedback), `make precommit` only at the end of each module.
- Test coverage ≥80% for every modified package.

</constraints>

<verification>

Run precommit in all three service dirs:
```bash
cd /workspace/agent/pr-reviewer && make precommit
cd /workspace/watcher/github-pr && make precommit
cd /workspace/watcher/github-build && make precommit
```
Expected: all three exit 0.

Confirm no inline parsing remains. The current inline parsers use `regexp.MustCompile` against a named pattern variable (not `strings.Split` over `REPO_ALLOWLIST` and not `filepath.Match`). Grep for the actual patterns:
```bash
grep -rn 'repoAllowlistEntryPattern\|regexp\.MustCompile.*\\\\b\\\\[a-zA-Z0-9' /workspace/agent/ /workspace/watcher/
```
Expected: zero matches.

Also confirm by parser-function name (the helper this prompt removes):
```bash
grep -rn 'ParseRepoAllowlist' /workspace/agent/ /workspace/watcher/
```
Expected: matches ONLY inside `lib/repoallowlist/` (the library's own internal use) and the test files in `pkg/allowlist_test.go` where the old function is being replaced (those tests should be updated to call `repoallowlist.Validate` or `repoallowlist.IsAllowed` instead).

Confirm every main.go imports repoallowlist:
```bash
grep -rn "bborbe/maintainer/lib/repoallowlist" \
  /workspace/agent/pr-reviewer/main.go \
  /workspace/agent/pr-reviewer/cmd/run-task/main.go \
  /workspace/watcher/github-pr/main.go \
  /workspace/watcher/github-build/main.go \
  /workspace/watcher/github-build/cmd/run-once/main.go
```
Expected: five matches (one per file).

Confirm replace directives in all three go.mod files:
```bash
grep "maintainer/lib" \
  /workspace/agent/pr-reviewer/go.mod \
  /workspace/watcher/github-pr/go.mod \
  /workspace/watcher/github-build/go.mod
```
Expected: `replace` and `require` lines in each.

Confirm no regex pattern remains in the migrated pkg files:
```bash
grep -rn "repoAllowlistEntryPattern\|regexp\.MustCompile" \
  /workspace/agent/pr-reviewer/pkg/allowlist.go \
  /workspace/watcher/github-pr/pkg/filter/repo_allowlist_filter.go \
  /workspace/watcher/github-build/pkg/filter/repo_allowlist_filter.go
```
Expected: zero matches.

Confirm IsAllowed used in filter/check code:
```bash
grep -rn "repoallowlist\.IsAllowed" \
  /workspace/agent/pr-reviewer/pkg/steps_checkout_execution.go \
  /workspace/watcher/github-pr/pkg/filter/repo_allowlist_filter.go \
  /workspace/watcher/github-build/pkg/filter/repo_allowlist_filter.go
```
Expected: one match in each file (three total).

Confirm Validate used in main.go files:
```bash
grep -rn "repoallowlist\.Validate" \
  /workspace/agent/pr-reviewer/main.go \
  /workspace/agent/pr-reviewer/cmd/run-task/main.go \
  /workspace/watcher/github-pr/main.go \
  /workspace/watcher/github-build/main.go \
  /workspace/watcher/github-build/cmd/run-once/main.go
```
Expected: five matches.

Confirm no fmt.Errorf in modified Go files:
```bash
grep -rn "fmt\.Errorf" \
  /workspace/agent/pr-reviewer/pkg/allowlist.go \
  /workspace/watcher/github-pr/pkg/filter/repo_allowlist_filter.go \
  /workspace/watcher/github-build/pkg/filter/repo_allowlist_filter.go
```
Expected: zero matches.

Confirm CHANGELOG entry:
```bash
grep -n "lib/repoallowlist\|REPO_ALLOWLIST.*caller\|IsAllowed.*wildcard" /workspace/CHANGELOG.md | head -5
```
Expected: one entry under `## Unreleased`.

</verification>
