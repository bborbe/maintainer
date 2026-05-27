---
status: completed
spec: [044-github-release-watcher-implementation]
summary: Implemented ParseChangelog and DeriveTaskID as pure-Go functions with full Ginkgo v2 test coverage
container: maintainer-github-release-exec-188-spec-044-changelog-and-taskid
dark-factory-version: v0.173.0
created: "2026-05-27T20:38:37Z"
queued: "2026-05-27T20:57:47Z"
started: "2026-05-27T20:57:49Z"
completed: "2026-05-27T21:02:32Z"
---

<summary>
- Two pure-Go modules implemented: `ParseChangelog` and `DeriveTaskID`
- CHANGELOG parser handles canonical and inverted Keep-a-Changelog ordering, mixed `v` prefix, missing or empty `## Unreleased`
- `DeriveTaskID` produces a deterministic UUID5 over `(owner, repo, head_sha)` using a frozen namespace constant
- Ginkgo tests cover named spec acceptance criteria: `ParseChangelog handles Unreleased at bottom with mixed v-prefix` and `DeriveTaskID is deterministic for identical inputs`
- No external library or filesystem state — easiest pure-Go layer; lands first so later prompts can use it
</summary>

<objective>
Replace the TODO stubs in `watcher/github-release/pkg/changelog.go` and `watcher/github-release/pkg/taskid.go` with working implementations and create Ginkgo v2 + Gomega tests for both. Both are pure-Go (no I/O, no network). The frozen namespace UUID in `taskid.go` MUST NOT change.
</objective>

<context>
Read CLAUDE.md at the repo root for project conventions.

Read these guides before writing code:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — wrap with `github.com/bborbe/errors`, never `fmt.Errorf` in production
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega, external `_test` packages
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`

Read these files in the worktree end-to-end before writing any code (the skeleton TYPE SIGNATURES and godoc are load-bearing — do not change the exported shape):
- `watcher/github-release/pkg/changelog.go` — the `ChangelogSummary` struct + `ParseChangelog` stub
- `watcher/github-release/pkg/taskid.go` — the `taskIDNamespace` UUID and `DeriveTaskID` stub
- `watcher/github-release/pkg/doc.go`
- `watcher/github-release/pkg/release.go` — confirms the consumer fields (`UnreleasedBullets`, `LatestVersion`, `UnreleasedIsFirst`)

Reference implementations (read for pattern parity, not literal copy — the github-release variant uses different inputs):
- `/workspace/watcher/github-pr/pkg/taskid.go` — same `uuid.NewSHA1(namespace, []byte(key))` pattern, different key fields. Note: `prWatcherNamespace` there is `var` (not const) because `uuid.UUID` is a struct.
- `/workspace/watcher/github-pr/pkg/taskid_test.go` — Ginkgo test shape: `It("is deterministic ...")`, `It("differs when ...")`, hard-pinned expected UUID via `uuid.NewSHA1(...)` inline computation.

Phase 1 prototype reference for the changelog parser semantics: the `/github-unreleased-repo-watcher` slash command in the agent vault. The Go port must produce the same counts/ordering decisions.

Counterfeiter / mock regen note: `pkg/changelog.go` and `pkg/taskid.go` declare no `//counterfeiter:generate` directives, so this prompt requires no mock regen.
</context>

<requirements>

**Execute steps in order. Run `make test` after step 4. Run `make precommit` only at the final step.**

1. **Implement `ParseChangelog` in `watcher/github-release/pkg/changelog.go`** (replacing the TODO body). Keep the exported signature exactly:

   ```go
   func ParseChangelog(content []byte) ChangelogSummary
   ```

   Semantics (mirror the Phase 1 bash `/github-unreleased-repo-watcher` parser):
   - Scan line-by-line via `bufio.Scanner` over `bytes.NewReader(content)`.
   - Track three state booleans: `inUnreleased`, `seenAnyH2`, `unreleasedIsFirstH2` (only set true when `## Unreleased` is the first `## ` heading encountered).
   - A line is an H2 heading iff it starts with `## ` (two hashes followed by a space). H1 (`# `) lines and `### ` lines do NOT count.
   - A line is the `## Unreleased` header iff (after trimming trailing whitespace) it equals `## Unreleased` (case-sensitive — Keep-a-Changelog convention).
   - When entering `## Unreleased`: if no prior H2 has been seen, set `unreleasedIsFirstH2 = true`. Mark `inUnreleased = true`.
   - When any OTHER `## ` heading is encountered while `inUnreleased == true`, set `inUnreleased = false`.
   - While `inUnreleased == true`, count lines matching the regex `^- ` (anchored to start of line, after `bufio.Scanner` strips the newline) — these are the unreleased bullets. Bullet lines that are inside a fenced code block under `## Unreleased` should still be counted (Phase 1 prototype does not strip code fences; match its behavior verbatim — operator-readable changelogs do not nest `-` lines in code fences).
   - `LatestVersion` extraction: the FIRST `## ` heading whose remainder (after the `## ` prefix and any trailing whitespace stripped) matches `^v?\d+\.\d+\.\d+$`. Capture the full matched heading text WITH any leading `v` preserved (so `## v1.2.6` → `"v1.2.6"`; `## 1.2.6` → `"1.2.6"`). Subsequent matching version headers do NOT overwrite.
   - On scanner finish: return `ChangelogSummary{UnreleasedBullets, UnreleasedIsFirst, LatestVersion}`. If there is no `## Unreleased` header at all in the document, return `ChangelogSummary{UnreleasedBullets: 0, UnreleasedIsFirst: false, LatestVersion: <as-found>}`.
   - Empty / nil input: return zero-value `ChangelogSummary{}` (no panic).
   - This function MUST NOT return an error — it is a pure parser. Malformed input yields a best-effort summary. Imports: `bufio`, `bytes`, `regexp`, `strings`.

   Pre-compile the version regex once at package scope: `var versionHeaderRe = regexp.MustCompile(\`^v?\d+\.\d+\.\d+$\`)`.

   Add a one-line godoc note that the bash slash-command in the agent vault is the behavioural reference.

2. **Create `watcher/github-release/pkg/changelog_test.go`** as package `pkg_test` (external test package per project convention). Use the existing Ginkgo suite — `suite_test.go` already lives next to other tests; do NOT create a new suite file at this layer if one already exists. If `watcher/github-release/pkg/suite_test.go` does NOT yet exist, create it:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg_test

   import (
       "testing"
       "time"

       "github.com/onsi/ginkgo/v2"
       "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestPkg(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       gomega.RegisterFailHandler(ginkgo.Fail)
       suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       ginkgo.RunSpecs(t, "Pkg Suite", suiteConfig, reporterConfig)
   }
   ```

   Then in `changelog_test.go`:

   - `Describe("pkg.ParseChangelog", ...)` with `It` blocks for each of these scenarios. Use raw-string literals (`` ` ``) for the fixture content; remember `bufio.Scanner` operates per-line so leading newlines are fine:

     a. **canonical ordering**: `# Changelog\n\n## Unreleased\n\n- entry one\n- entry two\n\n## v1.2.3\n\n- old\n` → `UnreleasedBullets == 2`, `UnreleasedIsFirst == true`, `LatestVersion == "v1.2.3"`.
     b. **inverted ordering with mixed v-prefix**: `# Changelog\n\n## 1.2.6\n\n- old\n\n## v1.2.5\n\n- older\n\n## Unreleased\n\n- new entry\n` → `UnreleasedBullets == 1`, `UnreleasedIsFirst == false`, `LatestVersion == "1.2.6"` (first matching version header wins regardless of `v` prefix style). **This `It` MUST be named `ParseChangelog handles Unreleased at bottom with mixed v-prefix`** (spec acceptance criterion exact string).
     c. **empty Unreleased header**: `## Unreleased\n\n## v1.0.0\n\n- x\n` → `UnreleasedBullets == 0`, `UnreleasedIsFirst == true`, `LatestVersion == "v1.0.0"`.
     d. **missing Unreleased**: `## v1.0.0\n\n- x\n` → `UnreleasedBullets == 0`, `UnreleasedIsFirst == false`, `LatestVersion == "v1.0.0"`.
     e. **no versions, no unreleased**: `# Changelog\n\nIntro paragraph\n` → all zero values; `LatestVersion == ""`.
     f. **nil input**: `ParseChangelog(nil)` → zero value `ChangelogSummary{}`.
     g. **empty bytes**: `ParseChangelog([]byte(""))` → zero value `ChangelogSummary{}`.
     h. **H3 under Unreleased does not terminate counting** (Phase 1 behavior): `## Unreleased\n\n### Added\n\n- a\n- b\n\n## v1.0.0\n` → `UnreleasedBullets == 2` (because `### Added` is not an H2, so `inUnreleased` stays true). `UnreleasedIsFirst == true`. `LatestVersion == "v1.0.0"`.

   All assertions via `Expect(summary.Field).To(Equal(...))`. Do NOT compare entire structs with `Equal(ChangelogSummary{...})` — per-field assertions give clearer failure output.

3. **Implement `DeriveTaskID` in `watcher/github-release/pkg/taskid.go`** (replacing the TODO body). Keep the exported signature exactly:

   ```go
   func DeriveTaskID(owner, repo, headSHA string) uuid.UUID
   ```

   Implementation:
   - Build the canonical key string as `fmt.Sprintf("%s/%s@%s", owner, repo, headSHA)`. The `/` and `@` separators are LOAD-BEARING — they prevent boundary ambiguity between fields. Do NOT change them.
   - Return `uuid.NewSHA1(taskIDNamespace, []byte(key))`.
   - Add `"fmt"` to the imports.

   The `taskIDNamespace` value (`4f9e2c1a-7b30-4d8f-9a2e-1c5b8d4f3a90`) is FROZEN. Do not edit the literal. Changing it invalidates every existing task_identifier the controller has deduplicated against.

4. **Create `watcher/github-release/pkg/taskid_test.go`** as package `pkg_test`:

   - `It("DeriveTaskID is deterministic for identical inputs")` — call `pkg.DeriveTaskID("bborbe", "docker-utils", "d630ef3526cfc57fbdccd9ba53c5c3a02945e407")` in a loop of 10000 iterations, capture the first result, assert every subsequent result equals the first via `Expect(got).To(Equal(want))`. **Use this exact `It` text — it is a spec acceptance criterion.**
   - `It("DeriveTaskID differs when owner differs")` — `DeriveTaskID("bborbe", "x", "abc")` != `DeriveTaskID("other", "x", "abc")`.
   - `It("DeriveTaskID differs when repo differs")` — `DeriveTaskID("bborbe", "x", "abc")` != `DeriveTaskID("bborbe", "y", "abc")`.
   - `It("DeriveTaskID differs when head_sha differs")` — `DeriveTaskID("bborbe", "x", "abc")` != `DeriveTaskID("bborbe", "x", "abd")`.
   - `It("DeriveTaskID pins the bborbe/docker-utils d630ef3 namespace contract")` — compute the expected UUID inline once and pin it:
     ```go
     ns := uuid.MustParse("4f9e2c1a-7b30-4d8f-9a2e-1c5b8d4f3a90")
     expected := uuid.NewSHA1(ns, []byte("bborbe/docker-utils@d630ef3526cfc57fbdccd9ba53c5c3a02945e407"))
     Expect(pkg.DeriveTaskID("bborbe", "docker-utils", "d630ef3526cfc57fbdccd9ba53c5c3a02945e407")).To(Equal(expected))
     ```
     This locks the namespace constant + key format together — both changing in parallel could silently produce stable but wrong IDs.

5. **Run unit tests**:
   ```bash
   cd watcher/github-release && make test
   ```
   Fix any failures. Coverage for these two files should be 100% — both are pure functions.

6. **Run full precommit**:
   ```bash
   cd watcher/github-release && make precommit
   ```
   This runs format + generate + test + lint + license. The generate step is a no-op for changelog/taskid (no counterfeiter directives) but will rerun globally; the diff against `pkg/mocks/` must remain empty for this prompt to pass cleanly.

</requirements>

<constraints>
- Mirror `watcher/github-pr` Go patterns verbatim: `errors.Wrapf(ctx, err, ...)` from `github.com/bborbe/errors` for any production error path; `glog.V(N).Infof` for logs; Ginkgo v2 + Gomega; external `_test` packages.
- Frontmatter contract / `task_identifier` shape is FROZEN — DO NOT change the `taskIDNamespace` UUID literal or the `owner/repo@sha` key format. Changing either breaks controller dedup.
- No `context.Background()` outside `*_test.go` — verified by spec AC.
- No `fmt.Errorf` in production paths — use `errors.Wrapf` (note: pure-Go `ParseChangelog` returns no error, so this constraint is satisfied trivially here).
- Pre-init Prometheus counter label combinations to `.Add(0)` — N/A here (no new metrics in this prompt; `metrics.go` already does this).
- Do NOT commit — dark-factory handles git.
- Do NOT modify any file outside `watcher/github-release/pkg/changelog.go`, `watcher/github-release/pkg/taskid.go`, `watcher/github-release/pkg/changelog_test.go`, `watcher/github-release/pkg/taskid_test.go`, and (if not already present) `watcher/github-release/pkg/suite_test.go`.
- The skeleton godoc comments above each stub are load-bearing references — preserve them above your implementations.
</constraints>

<verification>
```bash
cd watcher/github-release

# Implementations exist (no TODOs in either file)
grep -c "TODO" pkg/changelog.go pkg/taskid.go
# Expected: 0 in both files (i.e. "pkg/changelog.go:0" and "pkg/taskid.go:0")

# Public signatures preserved
grep -n "^func ParseChangelog" pkg/changelog.go
grep -n "^func DeriveTaskID" pkg/taskid.go

# Frozen namespace UUID unchanged
grep -n "4f9e2c1a-7b30-4d8f-9a2e-1c5b8d4f3a90" pkg/taskid.go

# Test files exist
ls pkg/changelog_test.go pkg/taskid_test.go

# Spec-named test IDs are present verbatim
grep -F "ParseChangelog handles Unreleased at bottom with mixed v-prefix" pkg/changelog_test.go
grep -F "DeriveTaskID is deterministic for identical inputs" pkg/taskid_test.go

# Run the suite
make test

# Full precommit
make precommit
```
</verification>
