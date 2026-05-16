---
status: completed
spec: [028-repoallowlist-lib-bootstrap]
summary: Bootstrapped lib/ Go module at github.com/bborbe/maintainer/lib with repoallowlist package implementing IsAllowed predicate and Validate validator supporting literal, wildcard (github.com/<owner>/*), and allow-all semantics; make precommit passes with 97.6% test coverage.
container: maintainer-116-spec-028-repoallowlist-lib-bootstrap
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-15T19:30:00Z"
queued: "2026-05-15T19:09:50Z"
started: "2026-05-15T19:09:52Z"
completed: "2026-05-15T19:18:47Z"
branch: dark-factory/repoallowlist-lib-bootstrap
---

<summary>

- A new Go module is created at `lib/` with module path `github.com/bborbe/maintainer/lib` and its own `go.mod`, `go.sum`, and `Makefile`
- The new module is automatically included in root-level `make precommit` via the existing `Makefile.folder` mechanism (no changes to `Makefile.folder`)
- The module contains a single package `repoallowlist` that exports a predicate and a validator
- The predicate `IsAllowed` returns true for any target when the allowlist is empty or nil (allow-all), false for an empty target against a non-empty allowlist, and evaluates each entry otherwise
- Literal entries match exactly and case-sensitively; wildcard entries of shape `github.com/<owner>/*` match any repo under that owner
- Wildcards outside the repo segment (star in host or owner position, or only two path segments) are logged as errors by the predicate and cause the validator to return an aggregate error
- Whitespace surrounding entries is trimmed; empty-after-trim entries are silently skipped by both functions
- Ginkgo `DescribeTable` tests cover every failure mode and happy path at ≥80% coverage; `make precommit` passes inside `lib/`

</summary>

<objective>

Bootstrap the maintainer repo's first shared library module at `lib/` and ship a single `repoallowlist` package that evaluates whether a target repo is allowed by a configured allowlist. The package's wildcard support (`github.com/<owner>/*`) eliminates the need to enumerate every repo individually once the five existing inline parsers are migrated in a follow-up spec.

</objective>

<context>

Read `CLAUDE.md` at the repo root for project conventions and the YOLO container rules.

Read these guides before writing any code:
- `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern, error wrapping
- `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo v2, DescribeTable, suite setup, external `_test` package, coverage ≥80%
- `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors` only; never `fmt.Errorf`; aggregate error pattern
- `go-library-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — library module conventions
- `go-logging-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — glog usage (existing codebase uses glog)
- `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` — which test types to write (pure-Go predicate logic → unit tests only)

**How root `make precommit` picks up `lib/`:**

`Makefile.folder` at repo root does:
```
DIRS += $(shell find */* -maxdepth 0 -name Makefile -exec dirname "{}" \;)
```
The shell glob `*/*` expands to all depth-2 paths including files. A `Makefile` at `lib/Makefile` (exactly two path components) matches `*/*` and is picked up. `dirname lib/Makefile` = `lib`. Root precommit then runs `make precommit` in `lib/`. No changes to `Makefile.folder` are needed.

**Makefile paths:** `lib/Makefile` is one level from repo root, so use `../` for includes:
```makefile
include ../Makefile.variables
include ../Makefile.precommit
```

**License header (BSD-style, must appear on every `.go` file):**
```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
```
`addlicense` (run by `make precommit`) adds this automatically, but include it explicitly anyway.

**Dependency versions** — match what is used in `agent/pr-reviewer/go.mod`:
- `github.com/bborbe/errors v1.5.13`
- `github.com/golang/glog v1.2.5`
- `github.com/onsi/ginkgo/v2 v2.28.3`
- `github.com/onsi/gomega v1.40.0`

**`generate` target side effect:** `Makefile.precommit`'s `generate` target runs `rm -rf mocks avro && mkdir -p mocks && echo "package mocks" > mocks/mocks.go && go generate -mod=mod ./...`. Since the `repoallowlist` package has no `//counterfeiter:generate` directives, `go generate` is a no-op. The `mocks/mocks.go` stub is still created and `addlicense` will add a header to it — this is correct behaviour.

**Files to read fully before making any changes:**
- `Makefile.folder` at repo root — understand the `find */*` mechanism
- `Makefile.precommit` at repo root — understand what `precommit` depends on (ensure, format, generate, test, check, addlicense)
- `Makefile.variables` at repo root — understand exported variables
- `agent/pr-reviewer/Makefile` — canonical module Makefile pattern to mirror (using `../../Makefile.variables`, `../../Makefile.precommit`; note lib/ uses `../` equivalents)
- `agent/pr-reviewer/pkg/poster_types.go` — license header example and package structure style
- `CHANGELOG.md` at repo root — check for existing `## Unreleased` section before adding

</context>

<requirements>

Execute steps in order. Run `cd lib && make test` as the fast-feedback checkpoint after step 4. Run `cd lib && make precommit` only at the final step.

---

## Step 1 — Create `lib/` directory scaffold

Create the following files. Read `agent/pr-reviewer/Makefile` for the canonical module Makefile pattern first.

**`lib/go.mod`:**

```
module github.com/bborbe/maintainer/lib

go 1.26.3

require (
	github.com/bborbe/errors v1.5.13
	github.com/golang/glog v1.2.5
	github.com/onsi/ginkgo/v2 v2.28.3
	github.com/onsi/gomega v1.40.0
)
```

(Indirect dependencies will be filled by `go mod tidy` in the `ensure` step of precommit.)

**`lib/Makefile`:**

```makefile
include ../Makefile.variables
include ../Makefile.precommit
```

No `SERVICE` variable needed. No Docker, no env includes.

After creating these files, run:
```bash
cd lib && go mod tidy
```
This populates `go.sum` and adds indirect deps to `go.mod`.

---

## Step 2 — Create `lib/repoallowlist/repoallowlist.go`

The package name is `repoallowlist`. Import path (for future callers): `github.com/bborbe/maintainer/lib/repoallowlist`.

Include the BSD license header.

Export exactly two functions:

### `IsAllowed`

```go
// IsAllowed reports whether the target is permitted by the allowlist.
// target must be a "host/owner/repo" string (e.g. "github.com/bborbe/maintainer").
// If the allowlist is empty or nil, all targets are allowed (allow-all semantics).
// If target is empty and the allowlist is non-empty, returns false.
// Malformed or invalid wildcard entries are logged with glog.Errorf and skipped.
//
// No ctx parameter: malformed-entry errors are logged via glog and discarded;
// they never escape the function, so there is nothing for ctx to enrich.
// Validate carries the ctx since it returns the error to the caller.
func IsAllowed(allowlist []string, target string) bool
```

Behavior:
1. If `len(allowlist) == 0` (or `allowlist == nil`): return `true`.
2. If `target == ""`: return `false`.
3. For each entry in `allowlist`:
   a. Trim surrounding whitespace.
   b. Skip if empty after trim.
   c. Validate and classify the entry (see entry classification below).
   d. If malformed: `glog.Errorf("repoallowlist: malformed entry %q: %v", entry, err)` and skip.
   e. If the entry matches `target`: return `true`.
4. If no entry matched: return `false`.

### `Validate`

```go
// Validate checks all entries in the allowlist for well-formedness.
// Returns nil if the allowlist is empty/nil or all entries are valid.
// Returns an aggregate error listing every malformed entry found.
// Whitespace-only and empty entries are silently skipped (not malformed).
func Validate(ctx context.Context, allowlist []string) error
```

Behavior:
1. Iterate every entry, trim whitespace, skip empty.
2. Classify each entry (see below).
3. Collect one error per malformed entry.
4. Return nil if no errors; return an aggregate error containing all collected errors otherwise.

For aggregation: create each per-entry error with `errors.Errorf(ctx, "repoallowlist: malformed entry %q: ...", entry)`, collect them in a `[]error` slice, then return `errors.Join(errs...)`. **Important:** `errors.Join` does NOT take a `ctx` parameter — its signature is `func Join(errs ...error) error`. Only `New`, `Errorf`, and `Wrapf` take `ctx`. Every error must be created/wrapped with `bborbe/errors` (no `fmt.Errorf`, no stdlib `errors.New`).

### Entry classification logic

This logic is shared by both functions. Extract into a private `classifyEntry` helper:

```go
// classifyEntry returns ("literal", nil), ("wildcard", nil), or ("", error) for malformed.
func classifyEntry(ctx context.Context, entry string) (kind string, err error)
```

Classification rules (applied to the already-trimmed, non-empty entry):

1. **Split by `/`** to get segments. Entry MUST have exactly 3 segments (host, owner, repo). If fewer or more: return malformed error `"entry %q: must have exactly 3 path segments (host/owner/repo)"`.

2. **Check for `*` in host or owner positions**: if `segments[0]` contains `*` OR `segments[1]` contains `*` → return malformed error `"entry %q: wildcard '*' is only valid in the repo (third) segment"`.

3. **Check repo segment**: if `segments[2] == "*"` → kind is `"wildcard"`. Otherwise → kind is `"literal"`.

**Wildcard match:** `segments[0] == targetSegments[0]` (host equal) AND `segments[1] == targetSegments[1]` (owner equal). Target must also have exactly 3 segments; if not, no match.

**Literal match:** `entry == target` (exact string equality, case-significant).

---

## Step 3 — Create `lib/repoallowlist/repoallowlist_suite_test.go`

Standard Ginkgo bootstrap. External test package `package repoallowlist_test`.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repoallowlist_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRepoallowlist(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Repoallowlist Suite")
}
```

---

## Step 4 — Create `lib/repoallowlist/repoallowlist_test.go`

External test package `package repoallowlist_test`. Import the package under test as `"github.com/bborbe/maintainer/lib/repoallowlist"`.

Use `context.Background()` in tests ONLY (not in production code). Use `DescribeTable` for tabular cases.

Write the following test blocks. Every spec AC case must be covered.

### `IsAllowed` tests

```go
DescribeTable("IsAllowed",
    func(allowlist []string, target string, expected bool) {
        Expect(repoallowlist.IsAllowed(allowlist, target)).To(Equal(expected))
    },
    // Allow-all cases
    Entry("nil allowlist allows everything",
        nil, "github.com/bborbe/maintainer", true),
    Entry("empty allowlist allows everything",
        []string{}, "github.com/bborbe/maintainer", true),
    Entry("nil allowlist allows empty target",
        nil, "", true),
    Entry("empty allowlist allows empty target",
        []string{}, "", true),

    // Literal match
    Entry("literal entry matches exact target",
        []string{"github.com/bborbe/maintainer"}, "github.com/bborbe/maintainer", true),
    Entry("literal entry does not match different repo",
        []string{"github.com/bborbe/maintainer"}, "github.com/bborbe/other", false),
    Entry("literal match is case-sensitive — uppercase does not match lowercase entry",
        []string{"github.com/bborbe/maintainer"}, "github.com/bborbe/Maintainer", false),
    Entry("literal match is case-sensitive — lowercase does not match uppercase entry",
        []string{"github.com/bborbe/Maintainer"}, "github.com/bborbe/maintainer", false),

    // Wildcard match
    Entry("wildcard entry matches any repo under same owner",
        []string{"github.com/bborbe/*"}, "github.com/bborbe/maintainer", true),
    Entry("wildcard entry matches another repo under same owner",
        []string{"github.com/bborbe/*"}, "github.com/bborbe/other-repo", true),
    Entry("wildcard entry does NOT match different owner",
        []string{"github.com/bborbe/*"}, "github.com/other-owner/maintainer", false),
    Entry("wildcard entry does NOT match different host",
        []string{"github.com/bborbe/*"}, "gitlab.com/bborbe/maintainer", false),

    // Malformed entries — skipped, do not match
    Entry("entry with fewer than three segments is skipped",
        []string{"github.com/bborbe"}, "github.com/bborbe", false),
    Entry("wildcard in owner position is skipped",
        []string{"github.com/*/maintainer"}, "github.com/bborbe/maintainer", false),
    Entry("wildcard in host position is skipped",
        []string{"*/bborbe/maintainer"}, "github.com/bborbe/maintainer", false),
    Entry("two wildcards (owner and repo) are skipped",
        []string{"github.com/*/*"}, "github.com/bborbe/maintainer", false),

    // Empty target against non-empty allowlist
    Entry("empty target returns false against non-empty allowlist",
        []string{"github.com/bborbe/*"}, "", false),

    // Whitespace handling
    Entry("entry with surrounding whitespace is trimmed and matched literally",
        []string{"  github.com/bborbe/maintainer  "}, "github.com/bborbe/maintainer", true),
    Entry("entry with surrounding whitespace is trimmed for wildcard match",
        []string{"  github.com/bborbe/*  "}, "github.com/bborbe/maintainer", true),
    Entry("whitespace-only entry is skipped",
        []string{"   "}, "github.com/bborbe/maintainer", false),
    Entry("empty string entry is skipped",
        []string{""}, "github.com/bborbe/maintainer", false),

    // Multiple entries — first match wins
    Entry("second entry matches when first does not",
        []string{"github.com/other/repo", "github.com/bborbe/maintainer"}, "github.com/bborbe/maintainer", true),
    Entry("malformed entry skipped, valid entry behind it still matches",
        []string{"github.com/bborbe", "github.com/bborbe/*"}, "github.com/bborbe/maintainer", true),
)
```

### `Validate` tests

```go
DescribeTable("Validate",
    func(allowlist []string, expectErr bool) {
        err := repoallowlist.Validate(context.Background(), allowlist)
        if expectErr {
            Expect(err).To(HaveOccurred())
        } else {
            Expect(err).NotTo(HaveOccurred())
        }
    },
    Entry("nil allowlist is valid", nil, false),
    Entry("empty allowlist is valid", []string{}, false),
    Entry("valid literal entry is valid",
        []string{"github.com/bborbe/maintainer"}, false),
    Entry("valid wildcard entry is valid",
        []string{"github.com/bborbe/*"}, false),
    Entry("mixed valid literal and wildcard is valid",
        []string{"github.com/bborbe/maintainer", "github.com/other/*"}, false),
    Entry("whitespace-only entry is skipped (valid)",
        []string{"   "}, false),
    Entry("empty string entry is skipped (valid)",
        []string{""}, false),

    // Malformed entries — each causes a validation error
    Entry("fewer than three segments returns error",
        []string{"github.com/bborbe"}, true),
    Entry("wildcard in owner position returns error",
        []string{"github.com/*/maintainer"}, true),
    Entry("wildcard in host position returns error",
        []string{"*/bborbe/maintainer"}, true),
    Entry("two wildcards (owner and repo) returns error",
        []string{"github.com/*/*"}, true),

    // Aggregate: ALL malformed entries are reported, not just the first
    Entry("multiple malformed entries both appear in aggregate error",
        []string{"github.com/bborbe", "github.com/*/foo"}, true),
)
```

Also add an `It` block that verifies the aggregate error contains details about ALL malformed entries when multiple are present:

```go
It("Validate returns aggregate error mentioning each malformed entry", func() {
    allowlist := []string{"github.com/bborbe", "github.com/*/foo"}
    err := repoallowlist.Validate(context.Background(), allowlist)
    Expect(err).To(HaveOccurred())
    msg := err.Error()
    Expect(msg).To(ContainSubstring("github.com/bborbe"))
    Expect(msg).To(ContainSubstring("github.com/*/foo"))
})
```

---

## Step 5 — Run `cd lib && make test` (fast feedback)

```bash
cd lib && go mod tidy && make test
```

Fix all compile errors and test failures before proceeding.

---

## Step 6 — Add CHANGELOG entry

Check `CHANGELOG.md` at repo root. If `## Unreleased` already exists, append to it; otherwise create the section above the most recent `## vX.Y.Z` heading.

```markdown
## Unreleased

- feat(lib): bootstrap new shared Go module at `lib/` (module path `github.com/bborbe/maintainer/lib`); add `repoallowlist` package with `IsAllowed` predicate and `Validate` validator supporting literal matching, `github.com/<owner>/*` wildcard, and allow-all semantics for empty/nil allowlists
```

---

## Step 7 — Run `cd lib && make precommit`

```bash
cd lib && make precommit
```

Must exit 0. If any target fails, fix it and re-run only the failing target (e.g., `make lint`, `make gosec`). Do NOT re-run full `make precommit` until all individual targets pass. Then run `make precommit` one final time.

If `lint` or `errcheck` fails for `mocks/mocks.go` (the auto-generated empty stub), this is unexpected — investigate rather than suppressing.

</requirements>

<constraints>

- Module path is `github.com/bborbe/maintainer/lib` — this is a frozen interface for the follow-up migration spec that will import this package. Do NOT change it.
- The `IsAllowed` predicate MUST return `true` for both `nil` and `[]string{}` allowlists — this preserves the existing "empty allowlist means allow all" semantics used by `agent/pr-reviewer`, `agent/pr-reviewer/cmd/run-task`, and `watcher/github-pr`.
- The `Validate` function MUST return `nil` for nil and empty allowlists — the library never refuses an empty list; callers enforce their own "non-empty" requirement at the `main.go` level.
- `Validate` MUST return an aggregate error describing ALL malformed entries, not just the first. Callers need to see all mistakes in one iteration.
- All errors constructed and wrapped with `github.com/bborbe/errors`. No `fmt.Errorf`, no stdlib `errors.New`.
- Logging uses `glog` (matching the rest of the maintainer codebase). Use `glog.Errorf(...)` for per-entry parse errors in `IsAllowed`. No logging in `Validate` — it returns errors instead.
- `context.Background()` is allowed in TEST code only. Production functions accept `ctx context.Context` from the caller.
- No `context.Background()` in `repoallowlist.go` — the `ctx` parameter must be threaded through from the caller.
- Do NOT generate a counterfeiter mock. The package exports pure functions; no caller has expressed a mocking need.
- Do NOT support negative/exclusion patterns, prefix patterns (e.g. `agent-*`), or non-GitHub platforms.
- Only the form `github.com/<owner>/*` (star as the ENTIRE repo segment, valid host, non-wildcard owner) is a valid wildcard. A `*` that appears as a substring of a segment (e.g. `agent-*`) is treated as a literal character — no error, no match.
- No caller code, no env file, no runbook is modified.
- Do NOT commit — dark-factory handles git.
- `make precommit` runs from `lib/`, never at repo root.
- Tests: external test package (`package repoallowlist_test`), coverage ≥80% for `lib/repoallowlist/`.

</constraints>

<verification>

Run precommit:
```bash
cd lib && make precommit
```
Expected: exit 0.

Confirm module path:
```bash
grep "^module" lib/go.mod
```
Expected: `module github.com/bborbe/maintainer/lib`

Confirm Makefile.folder picks up lib:
```bash
cd /workspace && make -n precommit 2>&1 | grep -E "precommit (lib|agent|watcher)"
```
Expected: output includes `precommit lib`.

Confirm IsAllowed and Validate are exported:
```bash
grep -n "^func IsAllowed\|^func Validate" lib/repoallowlist/repoallowlist.go
```
Expected: both functions present.

Confirm no fmt.Errorf in new package:
```bash
grep -rn "fmt\.Errorf" lib/
```
Expected: zero matches.

Confirm no stdlib errors.New:
```bash
grep -rn '"errors"' lib/repoallowlist/repoallowlist.go
```
Expected: zero matches (only `github.com/bborbe/errors` used).

Confirm no context.Background() in production code:
```bash
grep -n "context\.Background" lib/repoallowlist/repoallowlist.go
```
Expected: zero matches.

Confirm glog used for predicate errors:
```bash
grep -n "glog\.Errorf" lib/repoallowlist/repoallowlist.go
```
Expected: at least one match (malformed entry logging in IsAllowed).

Confirm test coverage:
```bash
cd lib && go test -coverprofile=/tmp/cover.out -mod=mod ./repoallowlist/... && go tool cover -func=/tmp/cover.out | grep -E "^total|repoallowlist"
```
Expected: coverage ≥80%.

Confirm CHANGELOG entry:
```bash
grep -n "repoallowlist\|lib.*bootstrap" CHANGELOG.md | head -5
```
Expected: one entry under `## Unreleased`.

Confirm no caller code touched:
```bash
git diff --name-only | grep -v "^lib/\|^CHANGELOG.md"
```
Expected: zero matches (only lib/ and CHANGELOG.md modified).

</verification>
