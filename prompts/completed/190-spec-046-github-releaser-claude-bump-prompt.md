---
status: completed
spec: ["046"]
summary: Created pkg/prompts with embedded bump-classification prompt and ParseBumpVerdict parser achieving 90.3% coverage
container: maintainer-github-releaser-exec-190-spec-046-github-releaser-claude-bump-prompt
dark-factory-version: v0.173.0
created: "2026-05-27T21:40:00Z"
queued: "2026-05-27T21:55:29Z"
started: "2026-05-27T21:55:31Z"
completed: "2026-05-27T21:58:03Z"
---

<summary>
- New pure-Go leaf library that holds the Claude bump-classification prompt and a typed parser for Claude's JSON verdict.
- The prompt text is an embedded, checked-in markdown file — reviewable, diffable, version-controllable.
- Three frozen exports: a function returning the prompt string, a struct for the parsed verdict, and a parser.
- Parser tolerates plain JSON, fenced JSON blocks wrapped in prose, and unknown extra JSON fields.
- Parser rejects empty input, malformed JSON, missing fields, and out-of-set bump values with wrapped errors.
- Embedded prompt encodes the validated Phase 1 priority-order classification rule (major → minor → patch).
- Comes with Ginkgo `DescribeTable` tests at ≥ 90% coverage covering all 8 named cases from the spec.
- Lifts the root CHANGELOG Unreleased section with a single new bullet.
- Foundation spec — last pure-Go leaf before the integration spec that wires planning end-to-end.
</summary>

<objective>
Create the package `github.com/bborbe/maintainer/agent/github-releaser/pkg/prompts` exporting `BumpClassificationPrompt() string`, the `BumpVerdict` struct (`Bump`, `Reasoning` fields), and `ParseBumpVerdict(claudeOutput string) (BumpVerdict, error)`. The prompt text lives in `pkg/prompts/bump_classification.md` and is loaded via `//go:embed`. End state: `cd agent/github-releaser && make precommit` exits 0, the package has ≥ 90% coverage, and the embedded prompt contains the Phase 1 priority-order rules verbatim (substrings: `patch | minor | major`, `BREAKING CHANGE`, `feat:`, `"bump":`, `major → minor → patch`).
</objective>

<context>
Read these before writing code (all paths repo-relative; container mounts repo root at `/workspace`):

- `CLAUDE.md` at repo root — project conventions.
- `agent/github-releaser/CLAUDE.md` if present.
- `agent/github-releaser/go.mod` — module path is `github.com/bborbe/maintainer/agent/github-releaser`; `github.com/bborbe/errors v1.5.13` already listed (currently indirect; a direct import promotes it on `go mod tidy`).
- `agent/github-releaser/pkg/semver/semver.go` and `agent/github-releaser/pkg/semver/suite_test.go` — sibling leaf library, completed in spec 045. Mirror its file layout, license header, error-wrapping style, and `context.Background()` usage for `bborbe/errors`.
- `agent/github-releaser/pkg/changelog/changelog.go` — another sibling leaf, completed in spec 044. Mirror package-doc-comment style.
- `agent/pr-reviewer/pkg/steps_review.go` lines 255-310 — the canonical `extractVerdict` + `lastJSONBlock` pattern this spec carries forward. Three extraction strategies in order: (1) direct unmarshal, (2) strip ` ```json ` fences, (3) last balanced `{...}` block. Reuse the same algorithm shape (not the same package — this is a new isolated leaf).
- `CHANGELOG.md` at repo root — root changelog; the new feature bullet lands at the TOP of `## Unreleased`.

Coding-plugin guides (in-container basenames; the container mounts the marketplace at the standard path):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega, `DescribeTable`/`Entry`, external `_test` packages.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` `Wrapf`/`Errorf` usage with `context.Context`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-library-guide.md` — pure-Go library hygiene.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md` — package/function doc comments.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — root `CHANGELOG.md` entry format.

The spec lives at `specs/in-progress/046-github-releaser-claude-bump-prompt.md`. Re-read it once for the 9 behaviors before coding.

Phase 1 source for the embedded prompt content: the `/github-release-repo` slash command at `~/.claude/commands/github-release-repo.md` on the host — its "planning.4 — Claude classifies bump" rules are the verbatim source. Since the container does NOT have host paths, the prompt-markdown content is given inline below in requirement 3.
</context>

<requirements>

**Run order: do steps in sequence. Run `cd agent/github-releaser && go test ./pkg/prompts/...` after step 5. Run `cd agent/github-releaser && make precommit` only as the final verification step.**

1. **Create the package directory** `agent/github-releaser/pkg/prompts/`. It must contain exactly four files (no subdirectories, no extra files):
   - `prompts.go` — implementation
   - `prompts_test.go` — Ginkgo `DescribeTable` cases (external test package `package prompts_test`)
   - `suite_test.go` — Ginkgo suite bootstrap (external test package `package prompts_test`)
   - `bump_classification.md` — embedded prompt text (plain markdown, no frontmatter)

2. **Standard BSD copyright header** on every `.go` file (3 lines, matching `agent/github-releaser/pkg/semver/semver.go`):
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.
   ```
   The `.md` file does NOT need a license header.

3. **Write `agent/github-releaser/pkg/prompts/bump_classification.md`** with EXACTLY the content shown in the 4-backtick block below. The substrings `patch | minor | major`, `BREAKING CHANGE`, `feat:`, `"bump":`, and `major → minor → patch` are LOAD-BEARING — they are grep-asserted by the acceptance criteria. Do NOT paraphrase. Do NOT remove the arrow Unicode glyphs (U+2192, `→`).

   The outer fence below uses 4 backticks so the inner triple-backtick ```` ```json ```` example block fences cleanly. Write the file content as a plain markdown document (with the inner triple-backtick json example intact); the 4-backtick wrapper is for THIS prompt's safe display only and must NOT appear in the written file.

   ````markdown
   # Classify the next semantic-version bump

   You are classifying a software release. Given the CHANGELOG bullets for the upcoming
   release, decide whether the next version bump is `patch`, `minor`, or `major`.

   ## Rules (apply in order)

   Evaluate the bullets in priority order: major → minor → patch. The FIRST rule that
   matches wins. Do not pick a weaker bump when a stronger one applies.

   1. **major** — at least one bullet describes a BREAKING CHANGE: a removed or renamed
      public API, an incompatible behavior change, a config key removal, a database
      migration that is not backwards compatible, or any change that requires callers
      to update their code or configuration.
   2. **minor** — at least one bullet starts with `feat:` or otherwise describes a new
      additive capability (new flag, new endpoint, new exported function) that does
      NOT break existing callers.
   3. **patch** — everything else: bug fixes, refactors, doc edits, dependency bumps,
      test additions, internal cleanup.

   Note: if a bullet contains BOTH a `feat:` prefix AND the literal text `BREAKING CHANGE`,
   the correct answer is `major` — priority order is strict.

   ## Output

   Output a single JSON object inside a fenced code block. The output MUST be valid JSON
   with exactly two string fields. Do not include any prose outside the fenced block.

   The `bump` field MUST be one of: patch | minor | major.

   ```json
   {
     "bump": "patch",
     "reasoning": "one sentence justifying the classification, referencing the deciding bullet"
   }
   ```
   ````

   The file written to disk MUST end immediately after the inner triple-backtick `` ``` `` that closes the `json` example — there is no other content. The 4-backtick wrapper in this prompt is purely a display device; do not include backticks of any kind that aren't part of the inner content above.

4. **Write `agent/github-releaser/pkg/prompts/prompts.go`** with this structure:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package prompts holds the Claude bump-classification prompt (embedded
   // from bump_classification.md) and a typed parser for Claude's JSON
   // verdict response. Pure-Go leaf library: no IO beyond //go:embed, no
   // third-party dependencies except github.com/bborbe/errors.
   //
   // The prompt text is the Phase 1 verbatim ruleset: ordered
   // major -> minor -> patch with concrete trigger criteria. The parser
   // tolerates the three real-world Claude output shapes seen in
   // pr-reviewer's extractVerdict history: plain JSON, fenced JSON in
   // prose, and JSON embedded inside arbitrary prose.
   package prompts

   import (
       "context"
       _ "embed"
       "encoding/json"
       "strings"

       "github.com/bborbe/errors"
   )

   //go:embed bump_classification.md
   var bumpClassificationPrompt string

   // BumpClassificationPrompt returns the embedded Phase 1 bump-classification
   // prompt. The string is non-empty and contains the Phase 1 priority-order
   // rules (major -> minor -> patch). Callers feed this string to a Claude
   // agent step alongside the CHANGELOG bullets to classify.
   func BumpClassificationPrompt() string {
       return bumpClassificationPrompt
   }

   // BumpVerdict is the typed shape of Claude's JSON response to the
   // bump-classification prompt. Bump is one of "patch" | "minor" | "major".
   // Reasoning is a one-sentence justification from Claude.
   type BumpVerdict struct {
       Bump      string `json:"bump"`
       Reasoning string `json:"reasoning"`
   }

   // ParseBumpVerdict extracts a BumpVerdict from Claude's raw output string.
   // Three extraction strategies are tried in order:
   //   1. Parse the trimmed input as a JSON object directly.
   //   2. Strip leading/trailing ```json fences and parse the inner block.
   //   3. Find the last balanced {...} block in the input and parse it.
   // First success wins. After successful unmarshal, the verdict is
   // validated: Bump must be one of patch|minor|major (case-sensitive),
   // Reasoning must be non-empty.
   //
   // Errors are wrapped via github.com/bborbe/errors and always contain
   // the literal substring "parse bump verdict" so callers can grep
   // verdict-parse failures apart from clone/git failures.
   func ParseBumpVerdict(claudeOutput string) (BumpVerdict, error) {
       ctx := context.Background()
       trimmed := strings.TrimSpace(claudeOutput)

       var v BumpVerdict

       // Strategy 1: direct unmarshal of trimmed input.
       if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
           return validateVerdict(ctx, v)
       }

       // Strategy 2: strip ```json fences (allow leading prose by also trying
       // after the first ```json marker; mirror pr-reviewer's TrimPrefix shape
       // for the simple case).
       stripped := strings.TrimSpace(strings.TrimSuffix(
           strings.TrimPrefix(strings.TrimPrefix(trimmed, "```json"), "```"),
           "```",
       ))
       if err := json.Unmarshal([]byte(stripped), &v); err == nil {
           return validateVerdict(ctx, v)
       }

       // Strategy 3: last balanced {...} block anywhere in the input.
       block, ok := lastJSONBlock(trimmed)
       if !ok {
           return BumpVerdict{}, errors.Errorf(ctx, "parse bump verdict: no JSON found")
       }
       if err := json.Unmarshal([]byte(block), &v); err != nil {
           return BumpVerdict{}, errors.Wrapf(ctx, err, "parse bump verdict: %s", block)
       }
       return validateVerdict(ctx, v)
   }

   // validateVerdict enforces the field-level invariants from spec 046
   // Desired Behavior 9: Bump must be in {patch, minor, major}; Reasoning
   // must be non-empty. On failure, returns a zero verdict + a wrapped
   // error containing "parse bump verdict".
   func validateVerdict(ctx context.Context, v BumpVerdict) (BumpVerdict, error) {
       switch v.Bump {
       case "patch", "minor", "major":
           // ok
       default:
           return BumpVerdict{}, errors.Errorf(ctx, "parse bump verdict: invalid bump value %q (want patch|minor|major)", v.Bump)
       }
       if strings.TrimSpace(v.Reasoning) == "" {
           return BumpVerdict{}, errors.Errorf(ctx, "parse bump verdict: missing reasoning")
       }
       return v, nil
   }

   // lastJSONBlock returns the last balanced {...} substring in s, or
   // "", false if none exists. Mirrors agent/pr-reviewer/pkg/steps_review.go
   // lastJSONBlock — kept private to this package to avoid an unwanted
   // dependency edge.
   func lastJSONBlock(s string) (string, bool) {
       end := strings.LastIndex(s, "}")
       if end < 0 {
           return "", false
       }
       depth := 0
       for i := end; i >= 0; i-- {
           switch s[i] {
           case '}':
               depth++
           case '{':
               depth--
               if depth == 0 {
                   return s[i : end+1], true
               }
           }
       }
       return "", false
   }
   ```

   Notes:
   - `_ "embed"` is the blank import that activates the `//go:embed` directive.
   - The `//go:embed bump_classification.md` line MUST sit on the line immediately above `var bumpClassificationPrompt string` — Go embed only honors directives directly preceding the var.
   - All errors use `errors.Errorf` (fresh) or `errors.Wrapf` (wrapping a downstream error) from `github.com/bborbe/errors`. NO `fmt.Errorf`.
   - `context.Background()` is acceptable because `ParseBumpVerdict`'s signature is frozen (no ctx parameter). Do NOT add a ctx parameter.

5. **Write `agent/github-releaser/pkg/prompts/suite_test.go`** mirroring `agent/github-releaser/pkg/semver/suite_test.go`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package prompts_test

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
       RunSpecs(t, "Prompts Suite", suiteConfig, reporterConfig)
   }
   ```

6. **Write `agent/github-releaser/pkg/prompts/prompts_test.go`** with:

   (a) A `Describe("BumpClassificationPrompt", ...)` block (or `It` block) that calls `prompts.BumpClassificationPrompt()` and asserts the result contains EACH of these literal substrings (one Expect per substring; descriptive `It` names):
   - `patch | minor | major`
   - `BREAKING CHANGE`
   - `feat:`
   - `"bump":`
   - `major → minor → patch`

   And asserts the result is not empty (`Expect(p).NotTo(BeEmpty())`).

   (b) A `DescribeTable("ParseBumpVerdict", ...)` block containing EXACTLY the 8 named `Entry` rows below. Entry descriptions must be the literal strings in quotes — they are grep-asserted by the acceptance criteria (must each appear exactly once):

   ```go
   var _ = DescribeTable("ParseBumpVerdict",
       func(input, wantBump, wantReasoning, wantErrSubstr string) {
           verdict, err := prompts.ParseBumpVerdict(input)
           if wantErrSubstr == "" {
               Expect(err).NotTo(HaveOccurred())
               Expect(verdict.Bump).To(Equal(wantBump))
               Expect(verdict.Reasoning).To(Equal(wantReasoning))
           } else {
               Expect(err).To(HaveOccurred())
               Expect(err.Error()).To(ContainSubstring("parse bump verdict"))
               Expect(err.Error()).To(ContainSubstring(wantErrSubstr))
               Expect(verdict).To(Equal(prompts.BumpVerdict{}))
           }
       },
       Entry("plain JSON parsed",
           `{"bump":"patch","reasoning":"bug fix only"}`,
           "patch", "bug fix only", ""),
       Entry("fenced JSON block extracted from prose",
           "Here is my verdict:\n\n```json\n{\"bump\":\"minor\",\"reasoning\":\"new feat: foo\"}\n```\n",
           "minor", "new feat: foo", ""),
       Entry("plain JSON with extra fields tolerated",
           `{"bump":"major","reasoning":"removed API","confidence":0.9}`,
           "major", "removed API", ""),
       Entry("empty input errors",
           ``,
           "", "", "no JSON found"),
       Entry("invalid bump value errors",
           `{"bump":"giant","reasoning":"x"}`,
           "", "", "invalid bump value"),
       Entry("missing reasoning errors",
           `{"bump":"patch","reasoning":""}`,
           "", "", "missing reasoning"),
       Entry("malformed JSON errors",
           `{"bump": "patch"`,
           "", "", "no JSON found"),
       Entry("prose only no JSON errors",
           `Claude says: the answer is patch but I am not formatting JSON.`,
           "", "", "no JSON found"),
   )
   ```

   Notes:
   - The `"malformed JSON errors"` entry input `{"bump": "patch"` has no closing `}`. Strategies 1 + 2 fail JSON parsing; strategy 3 (`lastJSONBlock`) finds no balanced object and returns false. The error therefore takes the `no JSON found` branch. The `wantErrSubstr` for this row is `"no JSON found"` — tight, single-truth assertion.
   - The `"prose only no JSON errors"` input intentionally lacks any `{` or `}` so strategy 3's `lastJSONBlock` returns `false`.
   - Entries are case-sensitive; descriptions must match the spec verbatim.

7. **Coverage ≥ 90%**: after writing tests, run

   ```bash
   cd agent/github-releaser && go test -cover ./pkg/prompts/...
   ```

   The 8 entries plus the BumpClassificationPrompt assertions naturally exercise all three strategies, both validation branches, and the embed read. If coverage falls below 90%, add 1-2 extra `Entry` rows (NOT grep-asserted) — candidates:
   - `Entry("uppercase bump rejected", `{"bump":"Patch","reasoning":"x"}`, "", "", "invalid bump value")` — case-sensitivity check
   - `Entry("strategy 3 finds JSON inside long prose", "intro... {\"bump\":\"patch\",\"reasoning\":\"r\"} outro...", "patch", "r", "")` — hits strategy 3's success path
   - `Entry("JSON with surrounding whitespace", "  \n {\"bump\":\"minor\",\"reasoning\":\"r\"}  \n  ", "minor", "r", "")` — hits the `TrimSpace` path

   Add only as many extras as needed to clear 90%. The 8 spec-named entries are MANDATORY.

8. **Update root `CHANGELOG.md`** Unreleased section. The current top of `## Unreleased` reads:

   ```
   ## Unreleased

   - feat(agent/github-releaser): add pkg/semver with BumpVersion(current, bump) for Phase 1 → Phase 2 version arithmetic (spec 045)
   - feat(agent/github-releaser): add pkg/changelog parser library — pure-Go ValidateUnreleased/ExtractUnreleasedBullets/InferHeaderPrefixStyle for planning step (spec 044)
   - feat(agent/github-releaser): scaffold Pattern B Job skeleton — Milestone 1 of Phase 2 graduation of the github-releaser agent
   ```

   ADD exactly ONE new bullet at the TOP of the Unreleased block (newest at top), so the new state begins:

   ```
   ## Unreleased

   - feat(agent/github-releaser): add pkg/prompts with embedded bump-classification prompt and ParseBumpVerdict parser for the planning step (spec 046)
   - feat(agent/github-releaser): add pkg/semver ...
   ...
   ```

   The new bullet MUST contain the literal substring `pkg/prompts` (acceptance-criteria grep target).

9. **Final verification**: run from `agent/github-releaser/` (NOT repo root):

   ```bash
   cd agent/github-releaser && make precommit
   ```

   It must exit 0. If linters complain (e.g. import ordering, missing license header, unused import), fix the underlying issue. Do NOT use `--no-verify`, do NOT modify `Makefile.precommit`, do NOT add `//nolint` directives.

</requirements>

<constraints>
- Package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/prompts`. Single directory, four files only: `prompts.go`, `prompts_test.go`, `suite_test.go`, `bump_classification.md`. No subdirectories. No `testdata/`.
- Function signatures + `BumpVerdict` struct are FROZEN — verbatim from spec 046 Goal section:
  - `func BumpClassificationPrompt() string`
  - `type BumpVerdict struct { Bump string \`json:"bump"\`; Reasoning string \`json:"reasoning"\` }`
  - `func ParseBumpVerdict(claudeOutput string) (BumpVerdict, error)`
  Do NOT rename, do NOT add/remove fields, do NOT change return types, do NOT add a `context.Context` parameter to `ParseBumpVerdict`.
- `BumpClassificationPrompt()` uses `//go:embed bump_classification.md` — single embedded file, no template substitution, no runtime config.
- The embedded `bump_classification.md` MUST contain these literal substrings (grep-asserted on the .md file):
  - `patch | minor | major`
  - `BREAKING CHANGE`
  - `feat:`
  - `"bump":`
  - `major → minor → patch` (with Unicode arrow U+2192)
- Errors MUST be wrapped via `github.com/bborbe/errors` (`Wrapf` for downstream errors, `Errorf` for fresh ones). Plain `fmt.Errorf` is BANNED in `prompts.go`. Acceptance grep: `grep -c 'fmt.Errorf' pkg/prompts/prompts.go` must return 0; `grep -cE 'errors\.(Wrap|Errorf)' pkg/prompts/prompts.go` must return ≥ 1.
- All error messages from `ParseBumpVerdict` MUST contain the literal substring `parse bump verdict` so callers can grep verdicts apart from clone/git failures.
- `ParseBumpVerdict` uses `context.Background()` internally because the signature has no ctx parameter (frozen). Same pattern as `pkg/semver/semver.go`.
- Test framework: Ginkgo v2 + Gomega. External test package (`package prompts_test`). Use `DescribeTable` / `Entry` for the 8 named cases — no hand-rolled `[]struct{...}` loops.
- Suite file sets `format.TruncatedDiff = false` and `time.Local = time.UTC`.
- Stdlib only inside `prompts.go` PLUS `github.com/bborbe/errors`. Allowed stdlib: `context`, `embed` (blank import), `encoding/json`, `strings`. No `regexp` is necessary; the balanced-brace scanner does the job. No third-party prompt-templating libraries.
- Coverage target: ≥ 90% on `pkg/prompts/`.
- Failure modes from spec § Failure Modes table — each row maps to one or more test Entry rows above:
  - empty input → `"empty input errors"` (`no JSON found`)
  - prose-only input → `"prose only no JSON errors"` (`no JSON found`)
  - invalid bump value (`"giant"`, etc.) → `"invalid bump value errors"`
  - empty bump value → covered by the `"invalid bump value errors"` switch default (empty string is not in the allow-set)
  - missing/empty reasoning → `"missing reasoning errors"`
  - malformed JSON → `"malformed JSON errors"`
- Security: per spec § Security, the prompt is checked into git (no secrets), Claude output is parsed but never executed, parser strict-validates fields. No injection surface added by this prompt.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` is green before and after.
- All `make precommit` invocations run from `agent/github-releaser/`, never from the repo root.
- License header (3 lines) required at the top of every `.go` file — copy from `pkg/semver/semver.go`.
</constraints>

<verification>

Run from the repo root unless noted.

```bash
# Package builds + tests pass + coverage ≥ 90%
cd agent/github-releaser && make precommit
cd agent/github-releaser && go test -cover ./pkg/prompts/...

# File layout (= 4 files exactly)
ls agent/github-releaser/pkg/prompts/ | sort
ls agent/github-releaser/pkg/prompts/ | wc -l                                   # =4

# Frozen exports exist exactly once
grep -c '^func BumpClassificationPrompt()'  agent/github-releaser/pkg/prompts/prompts.go   # =1
grep -c '^type BumpVerdict struct'          agent/github-releaser/pkg/prompts/prompts.go   # =1
grep -c '^func ParseBumpVerdict('           agent/github-releaser/pkg/prompts/prompts.go   # =1
grep -c '//go:embed bump_classification.md' agent/github-releaser/pkg/prompts/prompts.go   # =1

# Error-wrapping convention (bborbe/errors only)
grep -c 'fmt.Errorf'                        agent/github-releaser/pkg/prompts/prompts.go   # =0
grep -cE 'errors\.(Wrap|Errorf)'            agent/github-releaser/pkg/prompts/prompts.go   # ≥1
grep -c 'parse bump verdict'                agent/github-releaser/pkg/prompts/prompts.go   # ≥1

# All 8 spec-named DescribeTable entries present
grep -c '"plain JSON parsed"'                       agent/github-releaser/pkg/prompts/prompts_test.go   # =1
grep -c '"fenced JSON block extracted from prose"'  agent/github-releaser/pkg/prompts/prompts_test.go   # =1
grep -c '"plain JSON with extra fields tolerated"'  agent/github-releaser/pkg/prompts/prompts_test.go   # =1
grep -c '"empty input errors"'                      agent/github-releaser/pkg/prompts/prompts_test.go   # =1
grep -c '"invalid bump value errors"'               agent/github-releaser/pkg/prompts/prompts_test.go   # =1
grep -c '"missing reasoning errors"'                agent/github-releaser/pkg/prompts/prompts_test.go   # =1
grep -c '"malformed JSON errors"'                   agent/github-releaser/pkg/prompts/prompts_test.go   # =1
grep -c '"prose only no JSON errors"'               agent/github-releaser/pkg/prompts/prompts_test.go   # =1

# Embedded prompt content contract
grep -c 'BumpClassificationPrompt'                  agent/github-releaser/pkg/prompts/prompts_test.go         # ≥1
grep -c 'patch | minor | major'                     agent/github-releaser/pkg/prompts/bump_classification.md  # ≥1
grep -c 'BREAKING CHANGE'                           agent/github-releaser/pkg/prompts/bump_classification.md  # ≥1
grep -c 'feat:'                                     agent/github-releaser/pkg/prompts/bump_classification.md  # ≥1
grep -c '"bump":'                                   agent/github-releaser/pkg/prompts/bump_classification.md  # ≥1
grep -cE 'major.*minor.*patch'                      agent/github-releaser/pkg/prompts/bump_classification.md  # ≥1

# Root CHANGELOG mentions pkg/prompts
grep -c 'pkg/prompts' CHANGELOG.md   # ≥1

# Existing tests still pass at the agent level
cd agent/github-releaser && make test
```

</verification>
