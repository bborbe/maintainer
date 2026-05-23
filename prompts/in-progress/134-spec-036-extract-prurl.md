---
status: approved
spec: [036-watcher-pr-rename-trigger-add-single-pr-trigger]
created: "2026-05-23T21:00:00Z"
queued: "2026-05-23T21:13:16Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

## Summary

- `lib/prurl/prurl.go` created (verbatim copy of `agent/pr-reviewer/pkg/prurl.go`)
- `lib/prurl/prurl_test.go` created (updated import + type references)
- `lib/prurl/prurl_suite_test.go` created as Ginkgo v2 suite
- `agent/pr-reviewer/pkg/prurl.go` and `agent/pr-reviewer/pkg/prurl_test.go` deleted
- All same-package callers in `agent/pr-reviewer/pkg/` updated to use `github.com/bborbe/maintainer/lib/prurl`
- `agent/pr-reviewer/cmd/cli/main.go` updated to use `github.com/bborbe/maintainer/lib/prurl`

## Objective

Move the `prurl` package from `agent/pr-reviewer/pkg/` to `lib/prurl/` and update all callers. Both `agent/pr-reviewer` and `watcher/github-pr` will then import from the shared lib.

## Context

Read these files before making changes:

**Source (being moved):**
- `/workspace/agent/pr-reviewer/pkg/prurl.go` — package `pkg`, contains `Platform` type, `PRInfo` struct, `ParsePRURL` function
- `/workspace/agent/pr-reviewer/pkg/prurl_test.go` — package `pkg_test`, Ginkgo v2 test suite

**Destination pattern (follow these exactly):**
- `/workspace/lib/repoallowlist/repoallowlist.go` — BSD license, `errors` usage, package-level docs
- `/workspace/lib/repoallowlist/repoallowlist_suite_test.go` — external test package, Ginkgo v2 suite

**Internal callers in agent/pr-reviewer/pkg/ (same-package, need import + prefix):**
- `/workspace/agent/pr-reviewer/pkg/steps_review.go` — line 192: `prInfo, err := ParsePRURL(ctx, prURLStr)` → needs `prurl.ParsePRURL`
- `/workspace/agent/pr-reviewer/pkg/steps_planning.go` — line 141: `prInfo, parseErr := ParsePRURL(ctx, prURLStr)` → needs `prurl.ParsePRURL`
- `/workspace/agent/pr-reviewer/pkg/steps_checkout_execution.go` — line 352: `prInfo, parseErr := ParsePRURL(ctx, prURLStr)` → needs `prurl.ParsePRURL`; also uses `PRInfo` type throughout
- `/workspace/agent/pr-reviewer/pkg/poster_types.go` — uses `PRInfo` type in interface definitions
- `/workspace/agent/pr-reviewer/pkg/githubposter/poster.go` — uses `pr pkg.PRInfo` in method signatures
- `/workspace/agent/pr-reviewer/pkg/githubposter/verifier_test.go` — uses `prpkg.PRInfo` in test file
- `/workspace/agent/pr-reviewer/pkg/githubposter/poster_test.go` — uses `prpkg.PRInfo` in test file

**CLI caller (already uses import prefix):**
- `/workspace/agent/pr-reviewer/cmd/cli/main.go` — imports `prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"` → needs `prpkg "github.com/bborbe/maintainer/lib/prurl"`

**Key verified signatures:**
```go
// lib/prurl/prurl.go
package prurl
type Platform string
const PlatformGitHub Platform = "github"
const PlatformBitbucket Platform = "bitbucket"
type PRInfo struct {
    Platform Platform
    Host     string
    Owner    string
    Project  string
    Repo     string
    Number   int
    RepoURL  string
}
func ParsePRURL(ctx context.Context, rawURL string) (*PRInfo, error)
```

## Requirements

### Step 1: Create lib/prurl/prurl.go

Copy `/workspace/agent/pr-reviewer/pkg/prurl.go` verbatim to `/workspace/lib/prurl/prurl.go`, changing only:
- Package declaration: `package pkg` → `package prurl`

Keep all copyright headers, imports, function bodies, and comments unchanged.

### Step 2: Create lib/prurl/prurl_test.go

Copy `/workspace/agent/pr-reviewer/pkg/prurl_test.go` to `/workspace/lib/prurl/prurl_test.go`, making these changes:
- `package pkg_test` → `package prurl_test`
- Import `pkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"` → `prurl "github.com/bborbe/maintainer/lib/prurl"`
- All `pkg.PRInfo` → `prurl.PRInfo`
- All `pkg.PlatformGitHub` → `prurl.PlatformGitHub`
- All `pkg.PlatformBitbucket` → `prurl.PlatformBitbucket`
- All `pkg.ParsePRURL` → `prurl.ParsePRURL`

### Step 3: Create lib/prurl/prurl_suite_test.go

Following `/workspace/lib/repoallowlist/repoallowlist_suite_test.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prurl_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestPrurl(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Prurl Suite")
}
```

### Step 4: Update same-package callers in agent/pr-reviewer/pkg/

For each of these files, add the import and prefix all usages:

**`agent/pr-reviewer/pkg/steps_review.go`:**
- Add to imports: `prurl "github.com/bborbe/maintainer/lib/prurl"`
- Change `ParsePRURL` → `prurl.ParsePRURL`
- Change `PRInfo` → `prurl.PRInfo`

**`agent/pr-reviewer/pkg/steps_planning.go`:**
- Add to imports: `prurl "github.com/bborbe/maintainer/lib/prurl"`
- Change `ParsePRURL` → `prurl.ParsePRURL`
- Change `PRInfo` → `prurl.PRInfo`

**`agent/pr-reviewer/pkg/steps_checkout_execution.go`:**
- Add to imports: `prurl "github.com/bborbe/maintainer/lib/prurl"`
- Change `ParsePRURL` → `prurl.ParsePRURL`
- Change `PRInfo` → `prurl.PRInfo`

**`agent/pr-reviewer/pkg/poster_types.go`:**
- Add to imports: `prurl "github.com/bborbe/maintainer/lib/prurl"`
- Change `PRInfo` → `prurl.PRInfo`

**`agent/pr-reviewer/pkg/githubposter/poster.go`:**
- Add to imports: `prurl "github.com/bborbe/maintainer/lib/prurl"`
- Change `pr pkg.PRInfo` → `pr prurl.PRInfo`

**`agent/pr-reviewer/pkg/githubposter/verifier_test.go`:**
- Change import `prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"` → `prpkg "github.com/bborbe/maintainer/lib/prurl"`

**`agent/pr-reviewer/pkg/githubposter/poster_test.go`:**
- Change import `prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"` → `prpkg "github.com/bborbe/maintainer/lib/prurl"`

### Step 5: Update CLI caller

**`agent/pr-reviewer/cmd/cli/main.go`:**
- Change import `prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"` → `prpkg "github.com/bborbe/maintainer/lib/prurl"`

### Step 6: Delete old files

- Delete `/workspace/agent/pr-reviewer/pkg/prurl.go`
- Delete `/workspace/agent/pr-reviewer/pkg/prurl_test.go`

### Step 7: Regenerate mocks (or hand-edit if `go generate` is not available)

The counterfeiter mock at `agent/pr-reviewer/mocks/pr-poster.go` contains 6 references to `pkg.PRInfo` (lines ~24, 28, 105, 110, 134, 140). After deleting `prurl.go` from `agent/pr-reviewer/pkg/`, the bare `pkg.PRInfo` symbol no longer exists there and the build will break unless the mock is updated.

Try regeneration first:
```bash
cd /workspace/agent/pr-reviewer && go generate ./...
```

Then verify the mock no longer references the old type. If `go generate` silently failed (counterfeiter not installed) or left the old references, edit the mock by hand:
- Replace the bare import `"github.com/bborbe/maintainer/agent/pr-reviewer/pkg"` with `prurl "github.com/bborbe/maintainer/lib/prurl"` (keep any other existing imports)
- Replace every `pkg.PRInfo` → `prurl.PRInfo` at the 6 occurrences

Required post-condition (must hold whichever path you took):
```bash
! grep -nE 'pkg\.PRInfo' /workspace/agent/pr-reviewer/mocks/pr-poster.go
```

### Step 8: Clean module

Run `cd /workspace/agent/pr-reviewer && go mod tidy`

## Constraints

- Do NOT change the `ParsePRURL` function body, `PRInfo` struct fields, or `Platform` constants — move verbatim
- BSD license header must appear on every new file
- All errors via `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors`

## Verification

```bash
# Verify new files exist
test -f /workspace/lib/prurl/prurl.go && echo "prurl.go exists"
test -f /workspace/lib/prurl/prurl_test.go && echo "prurl_test.go exists"
test -f /workspace/lib/prurl/prurl_suite_test.go && echo "prurl_suite_test.go exists"

# Verify old files are gone
! test -f /workspace/agent/pr-reviewer/pkg/prurl.go && echo "old prurl.go deleted"
! test -f /workspace/agent/pr-reviewer/pkg/prurl_test.go && echo "old prurl_test.go deleted"

# Verify import paths updated (grep for both patterns)
grep -l 'github.com/bborbe/maintainer/lib/prurl' /workspace/agent/pr-reviewer/cmd/cli/main.go
grep -l 'github.com/bborbe/maintainer/lib/prurl' /workspace/agent/pr-reviewer/pkg/steps_review.go
grep -l 'github.com/bborbe/maintainer/lib/prurl' /workspace/agent/pr-reviewer/pkg/steps_planning.go
grep -l 'github.com/bborbe/maintainer/lib/prurl' /workspace/agent/pr-reviewer/pkg/steps_checkout_execution.go

# Verify old import paths removed
! grep 'github.com/bborbe/maintainer/agent/pr-reviewer/pkg' /workspace/agent/pr-reviewer/cmd/cli/main.go && echo "old import removed from cli"
! grep 'github.com/bborbe/maintainer/agent/pr-reviewer/pkg' /workspace/agent/pr-reviewer/pkg/steps_review.go && echo "old import removed from steps_review"
! grep 'github.com/bborbe/maintainer/agent/pr-reviewer/pkg' /workspace/agent/pr-reviewer/pkg/steps_planning.go && echo "old import removed from steps_planning"

# Mock-regen post-condition (Step 7): no stale pkg.PRInfo refs remain
! grep -nE 'pkg\.PRInfo' /workspace/agent/pr-reviewer/mocks/pr-poster.go

# Build + test the new lib/prurl package (its own Go module — has its own Makefile)
cd /workspace/lib && make precommit

# Build + test the agent module (verifies the prurl move didn't break callers + mocks)
cd /workspace/agent/pr-reviewer && make precommit
```