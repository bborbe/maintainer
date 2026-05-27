---
status: completed
spec: ["045"]
summary: Created github.com/bborbe/maintainer/agent/github-releaser/pkg/semver with BumpVersion(current, bump) function, 12 Ginkgo table entries, 92.6% coverage, and CHANGELOG update
container: maintainer-github-releaser-exec-189-spec-045-semver-bump
dark-factory-version: v0.173.0
created: "2026-05-27T21:30:00Z"
queued: "2026-05-27T21:24:36Z"
started: "2026-05-27T21:27:15Z"
completed: "2026-05-27T21:29:40Z"
---

<summary>
- New pure-Go library that computes the next semantic version from a current version + bump kind
- Single exported function with a frozen signature, consumed later by the github-releaser planning step
- Accepts both `vX.Y.Z` and `X.Y.Z` on input; always returns numeric `X.Y.Z` (no `v` prefix) on output
- Implements a first-release sentinel: every bump kind from `0.0.0` collapses to `0.1.0`
- Rejects malformed version strings and unknown bump kinds with wrapped errors (no `fmt.Errorf`)
- Comes with 9 named Ginkgo table cases covering happy paths and both error classes
- Lifts coverage to ≥ 90% and updates the root CHANGELOG Unreleased section
</summary>

<objective>
Create the package `github.com/bborbe/maintainer/agent/github-releaser/pkg/semver` exporting a single function `BumpVersion(current, bump string) (next string, err error)` that encodes the Phase 1 version-bump rules verbatim, including the `0.0.0 → 0.1.0` first-release sentinel, with Ginkgo table-driven tests at ≥ 90% coverage.
</objective>

<context>
Read `agent/github-releaser/CLAUDE.md` and root `CLAUDE.md` for project conventions before editing.

Read these files before writing code (all paths are repo-relative; container mounts repo root at `/workspace`):

- `agent/github-releaser/go.mod` — confirm module path `github.com/bborbe/maintainer/agent/github-releaser` and that `github.com/bborbe/errors v1.5.13` is already listed (it is currently `// indirect`; introducing a direct import will let `go mod tidy` promote it on its own — no manual edit).
- `watcher/github-pr/pkg/suite_test.go` — canonical Ginkgo v2 suite file in this repo; copy its structure verbatim (set `time.Local = time.UTC`, `format.TruncatedDiff = false`, register fail handler, configure timeout, call `RunSpecs`).
- `lib/repoallowlist/repoallowlist_suite_test.go` — minimal alternative suite pattern in this repo (no timeout config). Either pattern is acceptable; prefer the watcher/github-pr pattern with `format.TruncatedDiff = false` and `time.Local = time.UTC` to match the spec's constraints.
- `watcher/github-pr/main.go` lines 40-100 — examples of `errors.Errorf(ctx, ...)` and `errors.Wrapf(ctx, err, ...)`. NOTE: both functions take a `context.Context` as the first argument. Since `BumpVersion`'s frozen signature has no `context.Context` parameter, use `context.Background()` at the call site, e.g. `return "", errors.Wrapf(context.Background(), err, "parse version: %q", current)`.

Coding-plugin docs available inside the YOLO container — reference by basename (the container's claude-yolo image mounts the coding plugin at the standard plugin marketplace path):

- `go-testing-guide.md` — Ginkgo v2 + Gomega conventions, `DescribeTable`/`Entry`, external `_test` packages
- `go-error-wrapping-guide.md` — `github.com/bborbe/errors` `Wrapf`/`Errorf` usage with `context.Context`
- `go-library-guide.md` — package layout and exported-API discipline
- `go-precommit.md` — what `make precommit` runs
- `changelog-guide.md` — Unreleased-section editing rules

The spec lives at `specs/in-progress/045-github-releaser-semver-bump.md`. Re-read it once to align on the 9 behaviors before coding; do not duplicate that file's contents here.
</context>

<requirements>

**Run order: do steps in sequence. Run `cd agent/github-releaser && make test` after step 4. Run `cd agent/github-releaser && make precommit` only as the final verification step.**

1. **Create the package directory** `agent/github-releaser/pkg/semver/`. It must contain exactly three Go files (no subdirectories, no extra files):
   - `semver.go` — implementation
   - `semver_test.go` — Ginkgo `DescribeTable` cases (external test package `package semver_test`)
   - `suite_test.go` — Ginkgo suite bootstrap (external test package `package semver_test`)

2. **Write `agent/github-releaser/pkg/semver/semver.go`** with this exact structure:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package semver computes the next semantic version given a current
   // version and a bump kind (patch | minor | major). It is a pure-Go
   // leaf library with no IO; consumed by the github-releaser planning step.
   //
   // The function intentionally implements one Phase 1 quirk: when the
   // current version is 0.0.0 (the first-release sentinel), every bump kind
   // returns 0.1.0 — including "major". See spec 045 for rationale.
   package semver

   import (
       "context"
       "strconv"
       "strings"

       "github.com/bborbe/errors"
   )

   // BumpVersion returns the next version string given current ("vX.Y.Z" or
   // "X.Y.Z") and bump (one of "patch", "minor", "major"). The returned
   // version is numeric ("X.Y.Z") — the caller composes the final header by
   // prepending any "v" prefix.
   //
   // Special case: when current is 0.0.0 the result is always 0.1.0,
   // regardless of bump kind. Major-on-first-release does NOT yield 1.0.0.
   //
   // Errors are wrapped via github.com/bborbe/errors. Malformed current
   // versions produce an error whose message contains "parse version";
   // unknown bump kinds produce one containing "invalid bump".
   func BumpVersion(current string, bump string) (string, error) {
       ctx := context.Background()

       stripped := strings.TrimPrefix(current, "v")
       parts := strings.Split(stripped, ".")
       if len(parts) != 3 {
           return "", errors.Errorf(ctx, "parse version: %q has %d components, want 3", current, len(parts))
       }

       major, err := strconv.Atoi(parts[0])
       if err != nil {
           return "", errors.Wrapf(ctx, err, "parse version: %q major component", current)
       }
       minor, err := strconv.Atoi(parts[1])
       if err != nil {
           return "", errors.Wrapf(ctx, err, "parse version: %q minor component", current)
       }
       patch, err := strconv.Atoi(parts[2])
       if err != nil {
           return "", errors.Wrapf(ctx, err, "parse version: %q patch component", current)
       }

       // First-release sentinel: every bump from 0.0.0 collapses to 0.1.0.
       if major == 0 && minor == 0 && patch == 0 {
           return "0.1.0", nil
       }

       switch bump {
       case "patch":
           patch++
       case "minor":
           minor++
           patch = 0
       case "major":
           major++
           minor = 0
           patch = 0
       default:
           return "", errors.Errorf(ctx, "invalid bump: %q (want patch|minor|major)", bump)
       }

       return strconv.Itoa(major) + "." + strconv.Itoa(minor) + "." + strconv.Itoa(patch), nil
   }
   ```

   Notes:
   - Use `errors.Wrapf` when wrapping a non-nil downstream error (the `strconv.Atoi` cases).
   - Use `errors.Errorf` when constructing a fresh error (the 3-component-count case and the default-bump case).
   - `context.Background()` is acceptable here because the signature is frozen and the package has no caller context to thread through. Do NOT change the signature to add `ctx`.
   - Negative input (`"v-1.2.3"`) flows through `TrimPrefix` to `"-1.2.3"`, splits cleanly into 3 parts; `strconv.Atoi("-1")` SUCCEEDS in Go (it parses negative ints). Therefore the negative-component case from the spec's Failure Modes table is handled implicitly: `BumpVersion("v-1.2.3", "patch")` produces `"-1.2.4"` with `nil` error — which is wrong per the spec. To enforce rejection, add a guard immediately after the three `strconv.Atoi` calls: `if major < 0 || minor < 0 || patch < 0 { return "", errors.Errorf(ctx, "parse version: %q has negative component", current) }`. Insert this guard BEFORE the `0.0.0` sentinel check.

3. **Write `agent/github-releaser/pkg/semver/suite_test.go`** following the watcher/github-pr pattern:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package semver_test

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Semver Suite", suiteConfig, reporterConfig)
   }
   ```

4. **Write `agent/github-releaser/pkg/semver/semver_test.go`** with one `DescribeTable` containing exactly the 9 named `Entry` cases below. Entry descriptions must be the literal strings in quotes — they are grep-asserted by the acceptance criteria.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package semver_test

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/agent/github-releaser/pkg/semver"
   )

   var _ = DescribeTable("BumpVersion",
       func(current, bump, wantNext, wantErrSubstr string) {
           next, err := semver.BumpVersion(current, bump)
           if wantErrSubstr == "" {
               Expect(err).NotTo(HaveOccurred())
               Expect(next).To(Equal(wantNext))
           } else {
               Expect(err).To(HaveOccurred())
               Expect(err.Error()).To(ContainSubstring(wantErrSubstr))
               Expect(next).To(Equal(""))
           }
       },
       Entry("patch bump from v1.2.6",            "v1.2.6", "patch", "1.2.7", ""),
       Entry("minor bump from v1.2.6",            "v1.2.6", "minor", "1.3.0", ""),
       Entry("major bump from v1.2.6",            "v1.2.6", "major", "2.0.0", ""),
       Entry("no v prefix input tolerated",       "1.2.6",  "patch", "1.2.7", ""),
       Entry("v0.0.0 patch defaults to 0.1.0",    "v0.0.0", "patch", "0.1.0", ""),
       Entry("v0.0.0 minor defaults to 0.1.0",    "v0.0.0", "minor", "0.1.0", ""),
       Entry("v0.0.0 major defaults to 0.1.0",    "v0.0.0", "major", "0.1.0", ""),
       Entry("malformed current version",         "not-a-version", "patch", "", "parse version"),
       Entry("invalid bump kind",                 "v1.2.3", "giant", "", "invalid bump"),
   )
   ```

   Verification rules baked into the test:
   - Happy-path entries assert `next` has NO `v` prefix (the Expect-equal already enforces this — the acceptance criterion `grep -c 'Expect(next).To(Equal("v' ...` returns 0 holds because no test compares against a `v`-prefixed string).
   - Error entries assert (a) `err != nil`, (b) `err.Error()` contains the spec-mandated substring, and (c) `next == ""` (matches the implementation's `return "", err`).

5. **Run package-level tests** to confirm everything compiles and passes:

   ```bash
   cd agent/github-releaser && go test ./pkg/semver/...
   ```

   All 9 entries must pass. If `go.sum` complains about `github.com/bborbe/errors`, run `go mod tidy` from `agent/github-releaser/` — `errors v1.5.13` is already pinned as indirect; promoting it to direct should be a no-op for go.sum entries.

6. **Confirm coverage ≥ 90%**:

   ```bash
   cd agent/github-releaser && go test -cover ./pkg/semver/...
   ```

   Output must match the regex `coverage: 9[0-9]\.[0-9]%` (or `100.0%`). The 9 entries naturally exercise: the 3-component-count error branch (via `"not-a-version"` which splits into 1 component), the `strconv.Atoi` error branch (NOT directly — see below), the negative-component guard (NOT directly), and the `default` bump branch. If coverage falls below 90%, add 1-2 more `Entry` rows to hit the uncovered paths — candidate additions (NOT grep-asserted, freely named):
   - `Entry("alphabetic major component", "v1.x.3", "patch", "", "parse version")` — hits the `strconv.Atoi` failure for major
   - `Entry("negative component rejected", "v-1.2.3", "patch", "", "parse version")` — hits the negative-guard
   - `Entry("empty bump rejected", "v1.2.3", "", "", "invalid bump")` — hits the default branch with empty string
   - `Entry("uppercase bump rejected", "v1.2.3", "Patch", "", "invalid bump")` — case-sensitivity check

   Add only as many extras as needed to clear 90%. The 9 spec-named entries are mandatory; extras are optional but expected.

7. **Update the root `CHANGELOG.md`** Unreleased section. Read it first (it currently contains one bullet about Pattern B Job skeleton). Add exactly ONE new bullet under `## Unreleased`, before the existing bullet (Unreleased section reads top-to-bottom: newest at top):

   ```
   - feat(agent/github-releaser): add pkg/semver with BumpVersion(current, bump) for Phase 1 → Phase 2 version arithmetic (spec 045)
   ```

   The bullet must contain the literal substring `pkg/semver` (acceptance-criteria grep target).

8. **Final verification**: run `make precommit` from the agent directory:

   ```bash
   cd agent/github-releaser && make precommit
   ```

   It must exit 0. If linters complain (e.g. import ordering, missing license header, unused import), fix the underlying issue rather than disabling the check. Do NOT use `--no-verify` and do NOT modify `Makefile.precommit`.

</requirements>

<constraints>
- Package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/semver`. Single directory, three files only: `semver.go`, `semver_test.go`, `suite_test.go`.
- Function signature is FROZEN: `func BumpVersion(current string, bump string) (next string, err error)`. Do not add a `context.Context` parameter — use `context.Background()` internally for the bborbe/errors calls.
- Output `next` string format is `"X.Y.Z"` — numeric, NO `v` prefix.
- Input `current` accepts both `"vX.Y.Z"` and `"X.Y.Z"`. Strip a single leading `v` before parsing.
- Input `bump` is case-sensitive and must be exactly one of `patch | minor | major`. Empty string, capitalized variants, and abbreviations all error.
- `0.0.0` (numeric, regardless of `v` prefix) is the sole first-release sentinel. `0.0.1` is a normal version (`patch` → `0.0.2`).
- Errors MUST be wrapped via `github.com/bborbe/errors` (`Wrapf` for downstream errors, `Errorf` for fresh ones). Plain `fmt.Errorf` is banned in `semver.go`. The acceptance criteria grep this: `grep -c 'fmt.Errorf' pkg/semver/semver.go` must return 0; `grep -c 'errors.Wrap' pkg/semver/semver.go` must return ≥ 1.
- Test framework: Ginkgo v2 + Gomega. External test package (`package semver_test`). Use `DescribeTable` / `Entry` — no hand-rolled `[]struct` loops.
- Suite file sets `format.TruncatedDiff = false` and `time.Local = time.UTC`.
- Stdlib only inside `semver.go` PLUS `github.com/bborbe/errors`. No third-party semver libraries (no `github.com/Masterminds/semver`).
- Coverage target: ≥ 90% on `pkg/semver/`.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` is green before and after.
- All `make precommit` invocations run from `agent/github-releaser/`, never from the repo root.
- License header (3 lines) is required at the top of every `.go` file — copy from any existing file in the agent.
</constraints>

<verification>

Run from the repo root unless noted.

```bash
# Package builds + tests pass + coverage ≥ 90%
cd agent/github-releaser && make precommit
cd agent/github-releaser && go test -cover ./pkg/semver/...

# File layout (=3 files, exactly)
ls agent/github-releaser/pkg/semver/ | sort
ls agent/github-releaser/pkg/semver/ | wc -l   # =3

# Frozen signature exists exactly once
grep -c '^func BumpVersion(' agent/github-releaser/pkg/semver/semver.go   # =1

# Error-wrapping convention (bborbe/errors only)
grep -c 'fmt.Errorf' agent/github-releaser/pkg/semver/semver.go            # =0
grep -c 'errors.Wrap' agent/github-releaser/pkg/semver/semver.go           # ≥1

# All 9 spec-named DescribeTable entries are present
grep -c '"patch bump from v1.2.6"'          agent/github-releaser/pkg/semver/semver_test.go   # =1
grep -c '"minor bump from v1.2.6"'          agent/github-releaser/pkg/semver/semver_test.go   # =1
grep -c '"major bump from v1.2.6"'          agent/github-releaser/pkg/semver/semver_test.go   # =1
grep -c '"no v prefix input tolerated"'     agent/github-releaser/pkg/semver/semver_test.go   # =1
grep -c '"v0.0.0 patch defaults to 0.1.0"'  agent/github-releaser/pkg/semver/semver_test.go   # =1
grep -c '"v0.0.0 minor defaults to 0.1.0"'  agent/github-releaser/pkg/semver/semver_test.go   # =1
grep -c '"v0.0.0 major defaults to 0.1.0"'  agent/github-releaser/pkg/semver/semver_test.go   # =1
grep -c '"malformed current version"'       agent/github-releaser/pkg/semver/semver_test.go   # =1
grep -c '"invalid bump kind"'               agent/github-releaser/pkg/semver/semver_test.go   # =1

# Negative-component guard exists (Failure Modes row 2 — "v-1.2.3" must reject)
grep -cE '(major|minor|patch) *< *0' agent/github-releaser/pkg/semver/semver.go   # ≥1

# Output-format invariant: no test asserts a v-prefixed output
grep -c 'Expect(next).To(Equal("v' agent/github-releaser/pkg/semver/semver_test.go   # =0

# Root CHANGELOG mentions pkg/semver
grep -c 'pkg/semver' CHANGELOG.md   # ≥1

# Existing tests still pass at the agent level
cd agent/github-releaser && make test
```

</verification>
