---
status: completed
spec: [044-github-releaser-changelog-parser]
summary: Created github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog with ValidateUnreleased/ExtractUnreleasedBullets/InferHeaderPrefixStyle functions, Ginkgo DescribeTable tests at 97.7% coverage, and root CHANGELOG entry
container: maintainer-github-releaser-exec-188-spec-044-github-releaser-changelog-parser
dark-factory-version: v0.173.0
created: "2026-05-27T21:30:00Z"
queued: "2026-05-27T20:59:49Z"
started: "2026-05-27T20:59:51Z"
completed: "2026-05-27T21:27:13Z"
---

<summary>
- New pure-Go library parses `CHANGELOG.md` byte streams for the github-releaser planning phase.
- Validator decides whether the `## Unreleased` section is releaseable (P1/P2 preconditions) and reports the exact line/reason on failure.
- Extractor returns the `## Unreleased` bullet entries as a string slice for downstream classification.
- Inferrer looks at the first historic release heading and reports the repo's header-prefix style (`"v"` or `""`).
- All three functions are pure: no IO, no goroutines, no global state, deterministic.
- Behavior matches the validated Phase 1 prototype verbatim (Phase 1 Learnings § "What carries to Phase 2 verbatim").
- Tests use Ginkgo v2 + Gomega `DescribeTable` for the eight named edge cases; coverage target ≥ 90%.
- Foundation for downstream `pkg/steps_planning.go` (separate spec); function signatures are frozen contracts.
</summary>

<objective>
Create the pure-Go package `github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog` exporting three functions — `ValidateUnreleased`, `ExtractUnreleasedBullets`, `InferHeaderPrefixStyle` — that operate on raw `[]byte` CHANGELOG content. Cover them with Ginkgo `DescribeTable` tests at ≥ 90% coverage. End state: `cd agent/github-releaser && make precommit` exits 0, and the eight named DescribeTable entries from the spec are present verbatim.
</objective>

<context>
Read these before writing code:

- `CLAUDE.md` at repo root (project conventions).
- `agent/github-releaser/CLAUDE.md` if present.
- `agent/github-releaser/main.go` (top-of-file package doc for context).
- `agent/github-releaser/go.mod` (module path is `github.com/bborbe/maintainer/agent/github-releaser`; Ginkgo v2 + Gomega already in deps).
- `agent/github-releaser/main_test.go` (existing Ginkgo suite bootstrap pattern: `time.Local = time.UTC`, `format.TruncatedDiff = false`, `suiteConfig.Timeout = 60 * time.Second`).
- `lib/repoallowlist/repoallowlist_test.go` (canonical `DescribeTable` style used in this repo).
- `watcher/github-build/pkg/auth/suite_test.go` (canonical `suite_test.go` pattern).
- `CHANGELOG.md` at repo root (root changelog; the new feature entry lands under `## Unreleased`).

Coding-plugin guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega conventions, external test package, `DescribeTable`/`Entry`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-library-guide.md` — pure-Go library hygiene.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md` — package/function doc comments.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — root `CHANGELOG.md` entry format.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — `make precommit` expectations.
</context>

<requirements>
1. Create directory `agent/github-releaser/pkg/changelog/`. Single-package layout, no subdirectories. Production code lives in `changelog.go`; tests in `changelog_test.go`; Ginkgo bootstrap in `suite_test.go`.

2. Add the standard BSD copyright header (matching other files in this repo, e.g. `agent/github-releaser/main_test.go`) to all three new files:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.
   ```

3. **`agent/github-releaser/pkg/changelog/changelog.go`** — production code. Package `changelog`. Add a package doc comment summarizing the three responsibilities (precondition validation, Unreleased-bullets extraction, historic-header-prefix inference). Export EXACTLY these three function signatures verbatim — downstream specs depend on them:

   ```go
   func ValidateUnreleased(content []byte) (valid bool, reason string, line int)
   func ExtractUnreleasedBullets(content []byte) []string
   func InferHeaderPrefixStyle(content []byte) string
   ```

4. **`ValidateUnreleased` behavior** (per spec Desired Behavior items 1-4):
   - Scan lines with `bufio.Scanner` over `bytes.NewReader(content)` (preserves raw bytes; treat malformed UTF-8 opaquely — do NOT normalize).
   - Track 1-indexed line numbers.
   - A "`## ` heading" is any line whose trimmed content starts with `## ` (exactly two `#` then a space). Trailing whitespace on heading lines must be tolerated: trim trailing whitespace before comparing the heading text. Leading whitespace before `##` is NOT a heading.
   - "Is `## Unreleased`" check: after stripping leading `## ` and trailing whitespace, the remaining text equals `Unreleased` exactly (case-sensitive).
   - Algorithm:
     a. First pass: locate the line number of the FIRST `## ` heading and capture its raw heading-text (the text after `## `, trim-right whitespace). Also locate the line number of the FIRST `## Unreleased` heading anywhere in the file.
     b. If NO `## ` heading exists at all OR no `## Unreleased` exists → return `(false, "Unreleased section not found.", 0)`.
     c. If the first `## ` heading is NOT `## Unreleased` → return `(false, fmt.Sprintf("Unreleased is not the first ## section; found '%s' at line %d. Move ## Unreleased above all release headings.", headingText, firstHeadingLine), firstHeadingLine)`. The reason string MUST match the spec exactly, including the period after "headings" and the single quotes around the heading text.
     d. If the first `## ` heading IS `## Unreleased`, scan from the line AFTER the Unreleased heading until either the next `## ` heading or EOF. Count lines that begin with the literal prefix `- ` (hyphen, space). Lines beginning with `*` or `+` are NOT bullets. If at least one `- ` bullet is found → return `(true, "", 0)`.
     e. Otherwise (Unreleased block empty of `- ` bullets) → return `(false, "Unreleased section has no bullet entries.", unreleasedLine)` where `unreleasedLine` is the 1-indexed line number of the `## Unreleased` heading.

5. **`ExtractUnreleasedBullets` behavior** (per spec Desired Behavior item 5):
   - Locate the FIRST `## Unreleased` heading (same whitespace tolerance as `ValidateUnreleased`). Subsequent `## Unreleased` occurrences are ignored.
   - If no `## Unreleased` exists → return `nil`.
   - Otherwise, scan from the next line until the next `## ` heading or EOF. For each line that begins with the literal prefix `- `, strip the `- ` prefix (exactly two characters) and append the remainder to the result slice. Do NOT trim further whitespace from the bullet body — preserve it verbatim.
   - If `## Unreleased` exists but the block has zero `- ` bullets → return a NON-NIL empty slice: `[]string{}` (use `result := []string{}` initialization, not `var result []string`). The non-nil empty distinction matters for `len(result) == 0 && result != nil` tests.

6. **`InferHeaderPrefixStyle` behavior** (per spec Desired Behavior items 6-7):
   - Scan for the first `## ` heading whose heading-text (after `## ` prefix, trim-right) is NOT `Unreleased`. This is "the first historic release heading."
   - If that heading-text matches the regex `^v[0-9]+\.` → return `"v"`.
   - If it matches `^[0-9]+\.` → return `""`.
   - If no historic release heading exists (file contains only `## Unreleased`, or no `## ` headings at all, or no `## ` heading matches either regex) → return `"v"` (documented default).
   - Use `regexp.MustCompile` at package level (`var` block) — do NOT compile inside the function on each call. Pure-function contract still holds: package-level compiled regexes are read-only and deterministic.

7. **Purity contract** (per spec Desired Behavior item 8): no IO (no `os.*`, no `net/*`), no goroutines, no global mutable state, no time-dependent calls. Same input → same output. The only stdlib packages needed should be `bufio`, `bytes`, `fmt`, `regexp`, `strings`.

8. **`agent/github-releaser/pkg/changelog/suite_test.go`** — Ginkgo bootstrap. External test package `package changelog_test`. Copy the pattern from `watcher/github-build/pkg/auth/suite_test.go` verbatim, changing the suite name to `"Changelog Suite"`:
   ```go
   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Changelog Suite", suiteConfig, reporterConfig)
   }
   ```

9. **`agent/github-releaser/pkg/changelog/changelog_test.go`** — Ginkgo `DescribeTable` tests. External test package `package changelog_test`. Import path for the package under test: `"github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog"`.

10. **Required `DescribeTable` entries** — these eight entry titles MUST appear verbatim (the acceptance grep is exact-match on these strings). Organize them across `DescribeTable` blocks per function (one block per function is fine, or one combined block — your call as long as the entry titles match):

    For `ValidateUnreleased`:
    - `"P1 valid - Unreleased first"` — CHANGELOG where `## Unreleased` is the first `## ` heading and has at least one `- ` bullet; assert `valid == true`, `reason == ""`, `line == 0`.
    - `"P1 fail - Unreleased not first"` — CHANGELOG where `## 1.2.6` appears at line 11 BEFORE `## Unreleased`; assert `valid == false`, `reason == "Unreleased is not the first ## section; found '1.2.6' at line 11. Move ## Unreleased above all release headings."`, `line == 11`. The literal substring `found '1.2.6' at line 11` must appear in the test source (the acceptance grep checks this).
    - `"no Unreleased section"` — CHANGELOG with `## v1.0.0` but NO `## Unreleased`; assert `valid == false`, `reason == "Unreleased section not found."`, `line == 0`.
    - `"P2 fail - empty Unreleased"` — CHANGELOG where `## Unreleased` is the first `## ` heading but its block contains no `- ` bullets (e.g. blank lines, or prose without bullets, or only `*`/`+` markers); assert `valid == false`, `reason == "Unreleased section has no bullet entries."`, `line == <line of ## Unreleased>`.
    - `"trailing whitespace heading tolerated"` — CHANGELOG with `## Unreleased   ` (trailing spaces) as the first heading, with one `- ` bullet; assert `valid == true`.

    For `InferHeaderPrefixStyle`:
    - `"v-prefix historic"` — CHANGELOG with `## Unreleased` then `## v1.2.3`; assert returns `"v"`.
    - `"no-prefix historic"` — CHANGELOG with `## Unreleased` then `## 1.2.3`; assert returns `""`.
    - `"no historic release defaults to v"` — CHANGELOG with only `## Unreleased` (no historic release); assert returns `"v"`.

    You SHOULD also add ExtractUnreleasedBullets cases (these don't need exact titles — acceptance grep doesn't check them, but coverage target ≥ 90% requires them):
    - Bullets extracted in order from a multi-bullet Unreleased block.
    - Empty Unreleased returns non-nil empty slice (`[]string{}`, not `nil`).
    - Absent `## Unreleased` returns `nil`.
    - Repeated `## Unreleased` heading: only first block is parsed.

11. **Test data hygiene**: use multi-line raw string literals (backtick strings) for CHANGELOG fixtures in the `Entry(...)` calls. Keep fixtures inline — do NOT create a `testdata/` directory. Example fixture pattern:
    ```go
    Entry("P1 valid - Unreleased first",
        []byte("# Changelog\n\n## Unreleased\n\n- feat: add foo\n\n## v1.0.0\n\n- initial\n"),
        true, "", 0),
    ```

12. **Coverage target ≥ 90%**: after writing tests, run `cd agent/github-releaser && go test -cover ./pkg/changelog/...` and verify stdout contains a `coverage: 9[0-9]\.[0-9]%` match (anything ≥ 90.0%). If coverage is below 90%, add additional cases (e.g. for `ExtractUnreleasedBullets` or boundary scans) until it passes.

13. **Failure modes from spec must be reflected in tests**:
    - `nil` or empty `content`: `ValidateUnreleased` returns `(false, "Unreleased section not found.", 0)`, `ExtractUnreleasedBullets` returns `nil`, `InferHeaderPrefixStyle` returns `"v"`. Add an Entry for each.
    - Bullet lines using `*` or `+` markers: treated as non-bullets → P2 failure if no `- ` bullets exist. Add as a sub-case of `"P2 fail - empty Unreleased"` or a separate entry.
    - Repeated `## Unreleased` heading: only the FIRST block is parsed. Add an ExtractUnreleasedBullets test case for this.

14. **Root `CHANGELOG.md` entry** (acceptance criterion): APPEND a single bullet under the existing `## Unreleased` heading in `CHANGELOG.md` at the repo root. The heading is already present (added in commit `cf71e76`) and contains the prior Milestone 1 entry — your job is to ADD ONE MORE BULLET to the existing block, NOT to recreate the heading. Do NOT create or restore `agent/github-releaser/CHANGELOG.md` (project rule: single global CHANGELOG.md at repo root, no per-module CHANGELOG; the per-module file was removed in `cf71e76`).

    Bullet text to add (exact):
    ```
    - feat(agent/github-releaser): add pkg/changelog parser library — pure-Go ValidateUnreleased/ExtractUnreleasedBullets/InferHeaderPrefixStyle for planning step (spec 044)
    ```

15. **Verification step**: run from `agent/github-releaser/` (NOT repo root):
    ```bash
    cd agent/github-releaser
    make precommit
    go test -cover ./pkg/changelog/...
    ls pkg/changelog/ | sort
    grep -c '^func ValidateUnreleased('       pkg/changelog/changelog.go
    grep -c '^func ExtractUnreleasedBullets(' pkg/changelog/changelog.go
    grep -c '^func InferHeaderPrefixStyle('   pkg/changelog/changelog.go
    grep -c '"P1 valid - Unreleased first"'             pkg/changelog/changelog_test.go
    grep -c '"P1 fail - Unreleased not first"'          pkg/changelog/changelog_test.go
    grep -c '"no Unreleased section"'                   pkg/changelog/changelog_test.go
    grep -c '"P2 fail - empty Unreleased"'              pkg/changelog/changelog_test.go
    grep -c '"v-prefix historic"'                       pkg/changelog/changelog_test.go
    grep -c '"no-prefix historic"'                      pkg/changelog/changelog_test.go
    grep -c '"no historic release defaults to v"'       pkg/changelog/changelog_test.go
    grep -c '"trailing whitespace heading tolerated"'   pkg/changelog/changelog_test.go
    grep -c "found '1.2.6' at line 11"                  pkg/changelog/changelog_test.go
    ```
    Every `grep -c` MUST return exactly `1`; `make precommit` MUST exit 0; coverage MUST be ≥ 90.0%.
</requirements>

<constraints>
- Package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog`.
- Single directory, three files: `changelog.go`, `changelog_test.go`, `suite_test.go`. No subdirectories. No `testdata/`.
- Function signatures listed in requirement 3 are FROZEN. Downstream specs depend on them. Do not rename, do not add/remove parameters, do not change return types.
- Test framework: Ginkgo v2 + Gomega. External test package (`package changelog_test`). Use `DescribeTable` / `Entry` for the eight named cases — no hand-rolled `[]struct{...}` loops.
- `format.TruncatedDiff = false`, `time.Local = time.UTC` in `suite_test.go`.
- No errors returned by any of the three functions — preconditions surface via `(valid, reason, line)` triple, not Go errors. Do NOT add an `error` return.
- No counterfeiter mocks — no interfaces in this spec.
- Coverage target ≥ 90% on `pkg/changelog/`.
- Stdlib only inside `changelog.go` — no third-party imports needed (bufio, bytes, fmt, regexp, strings). If you reach for anything else, reconsider.
- Treat malformed UTF-8 opaquely — do NOT normalize bytes. `bufio.Scanner` defaults are fine.
- Do NOT commit — dark-factory handles git.
- Existing tests in `agent/github-releaser/` must still pass.
- Run verification from `agent/github-releaser/`, not from repo root.
</constraints>

<verification>
Run from the github-releaser agent directory:

```bash
cd agent/github-releaser
make precommit                                                                  # exit 0
go test -cover ./pkg/changelog/...                                              # coverage: 9X.X%
ls pkg/changelog/ | sort                                                        # changelog.go, changelog_test.go, suite_test.go
grep -c '^func ValidateUnreleased('       pkg/changelog/changelog.go           # =1
grep -c '^func ExtractUnreleasedBullets(' pkg/changelog/changelog.go           # =1
grep -c '^func InferHeaderPrefixStyle('   pkg/changelog/changelog.go           # =1
grep -c '"P1 valid - Unreleased first"'             pkg/changelog/changelog_test.go   # =1
grep -c '"P1 fail - Unreleased not first"'          pkg/changelog/changelog_test.go   # =1
grep -c '"no Unreleased section"'                   pkg/changelog/changelog_test.go   # =1
grep -c '"P2 fail - empty Unreleased"'              pkg/changelog/changelog_test.go   # =1
grep -c '"v-prefix historic"'                       pkg/changelog/changelog_test.go   # =1
grep -c '"no-prefix historic"'                      pkg/changelog/changelog_test.go   # =1
grep -c '"no historic release defaults to v"'       pkg/changelog/changelog_test.go   # =1
grep -c '"trailing whitespace heading tolerated"'   pkg/changelog/changelog_test.go   # =1
grep -c "found '1.2.6' at line 11"                  pkg/changelog/changelog_test.go   # =1
```

All `grep -c` MUST return exactly `1`. `make precommit` MUST exit 0. Coverage MUST report ≥ 90.0%.

Also verify the root `CHANGELOG.md` Unreleased entry (run from repo root):
```bash
grep -c 'feat(agent/github-releaser): add pkg/changelog parser library' CHANGELOG.md   # =1
```
</verification>
