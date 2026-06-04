---
status: completed
approved: "2026-05-27T21:32:27Z"
verifying: "2026-05-27T21:58:03Z"
completed: "2026-06-04T15:26:38Z"
branch: dark-factory/github-releaser-claude-bump-prompt
---

## Summary

- Pure-Go library `pkg/prompts/` for `agent/github-releaser` holding the Claude bump-classification prompt (verbatim from Phase 1 `/github-release-repo` slash command) and a typed parser for Claude's JSON response.
- Three exports: `BumpClassificationPrompt() string`, `BumpVerdict` struct, `ParseBumpVerdict(claudeOutput string) (BumpVerdict, error)`.
- The prompt text is the validated Phase 1 ruleset: ordered major → minor → patch with concrete trigger criteria (BREAKING CHANGE, `feat:`, everything else). Phase 1 classified `docker-utils v1.7.8` (minor) and `disk-status` (patch) correctly — behavior carries verbatim per [[GitHub Release Agent Phase 1 Learnings]].
- Parser tolerates "prose-around-JSON" responses (Claude sometimes wraps output in `Here is the verdict: { ... }`). Real bug from `pr-reviewer` `extractVerdict` history — same defensive parse needed here.
- Foundation spec — no IO beyond `//go:embed` of the prompt markdown file. Last of the 3 pure-Go foundation specs before the integration spec wires everything into the planning step.

## Problem

The planning step needs to (1) feed Claude a stable bump-classification prompt and (2) parse Claude's response into a typed Go struct. Inlining the prompt as a Go string literal in the step makes it hard to read, hard to version, and impossible to graduate as a separate artifact. Inlining the parser exposes the planning step to JSON-extraction edge cases (Claude wrapping JSON in prose) that Phase 1's pr-reviewer history showed are real.

A dedicated `pkg/prompts/` package keeps the LLM contract isolated, lets the prompt live as a checked-in markdown file (reviewable, diffable, embedded via `//go:embed`), and freezes the parser interface downstream consumers depend on.

## Goal

A package `github.com/bborbe/maintainer/agent/github-releaser/pkg/prompts` exporting:

```go
func BumpClassificationPrompt() string

type BumpVerdict struct {
    Bump      string `json:"bump"`      // "patch" | "minor" | "major"
    Reasoning string `json:"reasoning"` // one-sentence justification from Claude
}

func ParseBumpVerdict(claudeOutput string) (BumpVerdict, error)
```

The prompt text lives in `pkg/prompts/bump_classification.md`, embedded via `//go:embed` into the `BumpClassificationPrompt` function. End state: package compiles, `make precommit` green, prompt content checked into git as a reviewable markdown file, parser handles both plain JSON and prose-wrapped JSON, ≥ 90% coverage.

## Non-goals

- Actually calling Claude — that's `claude.NewAgentStep` in the planning step spec (downstream).
- Changing the Phase 1 prompt rules — `BumpClassificationPrompt()` returns the prompt verbatim from Phase 1. Any rule change is a separate spec.
- Other agent prompts (planning, ai_review specific guidance) — those live elsewhere or are inlined in their respective step impls.
- A general Claude-output parser — `ParseBumpVerdict` is specific to the bump verdict shape only.
- Streaming / token-by-token parsing — Claude's output is a string blob received after completion.

## Desired Behavior

1. `BumpClassificationPrompt()` returns a non-empty string — the embedded Phase 1 prompt loaded via `//go:embed` from `pkg/prompts/bump_classification.md`.
2. The returned prompt contains the literal substring `patch | minor | major` (the output-spec line Claude must honor).
3. The returned prompt contains the literal substring `BREAKING CHANGE` (the major-bump trigger) AND the literal substring `feat:` (the minor-bump trigger). These are the load-bearing classification rules.
4. The returned prompt instructs Claude to evaluate triggers in PRIORITY ORDER (major before minor before patch) — verified by literal substring `major → minor → patch` (or `major then minor then patch`) in the prompt. This closes the "feat: with BREAKING CHANGE" misclassification gap; without ordered evaluation, Claude may pick `minor` when the bullet hits multiple triggers.
5. The returned prompt instructs Claude to output JSON in a fenced block, including the literal substring `"bump":` (the JSON field name) — verified by direct grep on the prompt text.
6. `BumpVerdict` is a struct with exactly two exported fields: `Bump string` (json tag `bump`) and `Reasoning string` (json tag `reasoning`). No other fields.
7. `ParseBumpVerdict(input)` accepts a plain JSON object `{"bump":"patch","reasoning":"..."}` and returns the corresponding `BumpVerdict` with nil error. Extra unknown fields in the JSON object (e.g. `{"bump":"patch","reasoning":"x","confidence":0.9}`) are tolerated — `json.Unmarshal` ignores them and `BumpVerdict` is populated from the two known fields only. This is contract, not failure.
8. `ParseBumpVerdict` accepts a fenced JSON block embedded in prose (e.g. `Here is my verdict:\n\n\`\`\`json\n{...}\n\`\`\`\n`). The first valid JSON object containing a `"bump"` field is extracted. Three extraction strategies tried in order: (a) parse whole input as JSON, (b) extract fenced ` ```json ... ``` ` block, (c) regex for `{ ... "bump": ... }` block. First success wins. Mirrors `pr-reviewer/pkg/verdict.go` `extractVerdict` pattern.
9. `ParseBumpVerdict` returns a non-nil wrapped error (via `github.com/bborbe/errors`) when: (a) no valid JSON found, (b) `Bump` field is missing or empty, (c) `Bump` value is not in `{patch, minor, major}`, (d) `Reasoning` field is missing or empty. Error message contains literal substring `parse bump verdict` so callers can grep verdicts apart from clone/git errors.

## Constraints

- Package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/prompts`.
- Files in `pkg/prompts/`: `prompts.go` (production code), `prompts_test.go` (Ginkgo tests), `suite_test.go` (Ginkgo bootstrap), `bump_classification.md` (embedded prompt text). No subdirectories. Tests + suite live in external package `package prompts_test`.
- Function signatures + `BumpVerdict` struct in Goal § are FROZEN. Downstream specs (planning step) depend on them. Do not rename, do not add/remove fields, do not change return types.
- `BumpClassificationPrompt()` uses `//go:embed bump_classification.md` — single embedded file, no template substitution, no runtime config.
- `ParseBumpVerdict` errors are wrapped via `github.com/bborbe/errors` (`errors.Wrapf(ctx, err, ...)` or `errors.Errorf(ctx, ...)`). Plain `fmt.Errorf` is banned per project convention. Use `context.Background()` at call site since `ParseBumpVerdict` has no ctx parameter (frozen signature; pure-Go leaf library).
- Coverage target: ≥ 90% on `pkg/prompts/`.
- Test framework: Ginkgo v2 + Gomega per [[go-testing-guide]]. External test package. `format.TruncatedDiff = false`, UTC time. `DescribeTable`/`Entry` for all 9 named cases below.
- The embedded `bump_classification.md` content MUST contain the substrings asserted by Behaviors 2-4 — verified directly by test cases that call `BumpClassificationPrompt()` and `strings.Contains` against the result.
- Stdlib only inside `prompts.go` PLUS `github.com/bborbe/errors`. No third-party prompt-templating libraries. JSON parsing via `encoding/json`. Fenced-block + regex extraction via `strings` + `regexp` stdlib only.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---|---|---|---|
| `claudeOutput` is empty string | All 3 extraction strategies fail | Return zero `BumpVerdict` + wrapped error containing `parse bump verdict: no JSON found` | Caller (planning step) treats this as a failed Claude step; controller retries phase per existing retry-cap |
| `claudeOutput` contains only prose, no JSON object | Strategies (a) and (b) fail; strategy (c) regex matches no `"bump":` block | Same `no JSON found` error | Same |
| Valid JSON but `bump` value is `"giant"` / `"patch1"` / `"Patch"` (invalid) | After unmarshal, value validation rejects | Wrapped error containing `parse bump verdict: invalid bump value` + the offending value | Same |
| Valid JSON but `bump` field is empty string `""` | Validation rejects empty | Same `invalid bump value` error | Same |
| Valid JSON but `reasoning` field missing or empty | Validation rejects | Wrapped error containing `parse bump verdict: missing reasoning` | Same |
| Malformed JSON (`{"bump": "patch"` — unclosed) | `json.Unmarshal` errors | Wrapped error containing `parse bump verdict` (with underlying syntax error) | Same |

## Do-Nothing Option

Cost of NOT building `pkg/prompts/`:

- Planning step inlines the bump prompt as a multiline Go string literal — hard to review, hard to diff, hard to graduate.
- JSON extraction logic gets re-implemented in every Claude-consuming step (planning, ai_review, future agents); the pr-reviewer codebase already has this drift between steps. A typed parser locks the contract once.
- The "prose-around-JSON" failure mode (real bug surfaced in pr-reviewer `extractVerdict`) would re-surface in the planning step if implemented inline; Phase 1 Learnings § "What carries to Phase 2 verbatim" explicitly carries this lesson forward.

## Security / Abuse

- The prompt is checked into git as `bump_classification.md` — visible to anyone with repo read access. No secrets, no credentials in the prompt.
- Claude's output is parsed but never executed as code; the parser only extracts a typed struct. No injection surface.
- Maximum adversarial input is a malicious CHANGELOG bullet attempting prompt-injection (e.g. `- bump: major (override)`). The prompt's structure (explicit JSON-only output instruction) and the parser's strict field validation defend against this; even if Claude is tricked, the parser would reject an off-shape response.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0 — evidence: exit code 0.
- [ ] `ls agent/github-releaser/pkg/prompts/` returns exactly `prompts.go`, `prompts_test.go`, `suite_test.go`, `bump_classification.md` — evidence: `ls | wc -l` returns 4.
- [ ] `grep -c '^func BumpClassificationPrompt()' agent/github-releaser/pkg/prompts/prompts.go` returns 1.
- [ ] `grep -c '^type BumpVerdict struct' agent/github-releaser/pkg/prompts/prompts.go` returns 1.
- [ ] `grep -c '^func ParseBumpVerdict(' agent/github-releaser/pkg/prompts/prompts.go` returns 1.
- [ ] `grep -c '//go:embed bump_classification.md' agent/github-releaser/pkg/prompts/prompts.go` returns 1.
- [ ] `cd agent/github-releaser && go test -cover ./pkg/prompts/...` reports `coverage: ≥90.0%` — evidence: stdout matches regex `coverage: (9[0-9]|100)\.[0-9]%`.
- [ ] DescribeTable case **plain JSON parsed** exists — evidence: `grep -c '"plain JSON parsed"' pkg/prompts/prompts_test.go` returns 1.
- [ ] DescribeTable case **fenced JSON block extracted from prose** exists — evidence: `grep -c '"fenced JSON block extracted from prose"' pkg/prompts/prompts_test.go` returns 1.
- [ ] DescribeTable case **plain JSON with extra fields tolerated** exists — evidence: `grep -c '"plain JSON with extra fields tolerated"' pkg/prompts/prompts_test.go` returns 1.
- [ ] DescribeTable case **empty input errors** exists — evidence: `grep -c '"empty input errors"' pkg/prompts/prompts_test.go` returns 1.
- [ ] DescribeTable case **invalid bump value errors** exists — evidence: `grep -c '"invalid bump value errors"' pkg/prompts/prompts_test.go` returns 1.
- [ ] DescribeTable case **missing reasoning errors** exists — evidence: `grep -c '"missing reasoning errors"' pkg/prompts/prompts_test.go` returns 1.
- [ ] DescribeTable case **malformed JSON errors** exists — evidence: `grep -c '"malformed JSON errors"' pkg/prompts/prompts_test.go` returns 1.
- [ ] DescribeTable case **prose only no JSON errors** exists — evidence: `grep -c '"prose only no JSON errors"' pkg/prompts/prompts_test.go` returns 1.
- [ ] Embedded prompt contains rule triggers — evidence: a test calls `BumpClassificationPrompt()` and asserts the result contains literal substrings `"patch | minor | major"`, `"BREAKING CHANGE"`, `"feat:"`, `"\"bump\":"`. `grep -c 'BumpClassificationPrompt' pkg/prompts/prompts_test.go` returns ≥ 1.
- [ ] Embedded prompt enforces priority-order evaluation — evidence: `grep -cE 'major.*minor.*patch' pkg/prompts/bump_classification.md` returns ≥ 1 (the prompt instructs Claude to evaluate `major → minor → patch` in priority order).
- [ ] Errors use `github.com/bborbe/errors`, not `fmt.Errorf` — evidence: `grep -c 'fmt.Errorf' pkg/prompts/prompts.go` returns 0; `grep -cE 'errors\.(Wrap|Errorf)' pkg/prompts/prompts.go` returns ≥ 1.
- [ ] Error messages contain `parse bump verdict` — evidence: `grep -c 'parse bump verdict' pkg/prompts/prompts.go` returns ≥ 1.
- [ ] Root `CHANGELOG.md` `## Unreleased` section gains a single `feat:` bullet referencing `pkg/prompts` — evidence: `grep -c 'pkg/prompts' CHANGELOG.md` returns ≥ 1.

## Verification

```bash
cd agent/github-releaser
make precommit                                            # exit 0
go test -cover ./pkg/prompts/...                          # coverage ≥ 90%
ls pkg/prompts/ | sort                                    # 4 files: bump_classification.md prompts.go prompts_test.go suite_test.go

# Signatures + frozen contract
grep -c '^func BumpClassificationPrompt()'  pkg/prompts/prompts.go   # =1
grep -c '^type BumpVerdict struct'          pkg/prompts/prompts.go   # =1
grep -c '^func ParseBumpVerdict('           pkg/prompts/prompts.go   # =1
grep -c '//go:embed bump_classification.md' pkg/prompts/prompts.go   # =1

# Error wrapping
grep -c 'fmt.Errorf' pkg/prompts/prompts.go                # =0
grep -cE 'errors\.(Wrap|Errorf)' pkg/prompts/prompts.go    # ≥1
grep -c 'parse bump verdict' pkg/prompts/prompts.go        # ≥1

# 8 mandatory DescribeTable cases
grep -c '"plain JSON parsed"'                       pkg/prompts/prompts_test.go  # =1
grep -c '"fenced JSON block extracted from prose"'  pkg/prompts/prompts_test.go  # =1
grep -c '"plain JSON with extra fields tolerated"'  pkg/prompts/prompts_test.go  # =1
grep -c '"empty input errors"'                      pkg/prompts/prompts_test.go  # =1
grep -c '"invalid bump value errors"'               pkg/prompts/prompts_test.go  # =1
grep -c '"missing reasoning errors"'                pkg/prompts/prompts_test.go  # =1
grep -c '"malformed JSON errors"'                   pkg/prompts/prompts_test.go  # =1
grep -c '"prose only no JSON errors"'               pkg/prompts/prompts_test.go  # =1

# Prompt content contract (embedded markdown)
grep -c 'BumpClassificationPrompt'                  pkg/prompts/prompts_test.go  # ≥1
grep -c 'patch | minor | major'                     pkg/prompts/bump_classification.md  # ≥1
grep -c 'BREAKING CHANGE'                           pkg/prompts/bump_classification.md  # ≥1
grep -c 'feat:'                                     pkg/prompts/bump_classification.md  # ≥1
grep -c '"bump":'                                   pkg/prompts/bump_classification.md  # ≥1
grep -cE 'major.*minor.*patch'                      pkg/prompts/bump_classification.md  # ≥1  (priority-order)

# Root CHANGELOG
grep -c 'pkg/prompts' CHANGELOG.md                                                # ≥1
```

No scenario justified — pure-Go library with `//go:embed`, fully covered by unit tests + table-driven cases. Per [[spec-writing]] § Test-layer responsibilities.

## Verification Result

**Verified:** 2026-06-04T15:25:54Z (HEAD 753f3f5)
**Binary:** installed dark-factory (target repo is bborbe/maintainer, not dark-factory)
**Scenario:** Filesystem + `make precommit` + `go test -cover` against master; original 4 files added by commit cff6cd0 on 2026-05-27 (matches `verifying` timestamp).
**Evidence:**
- `make precommit` in `agent/github-releaser` exits 0 (trivy clean, addlicense ran, "ready to commit")
- `go test -cover ./pkg/prompts/...` → `coverage: 90.7% of statements`
- All 4 spec-mandated files present (`prompts.go`, `prompts_test.go`, `suite_test.go`, `bump_classification.md`); 3 extra files in dir from downstream completed spec 058 (`changelog_*.md`, `changelog-quality-guide.md`)
- Signature greps: `BumpClassificationPrompt()=1`, `BumpVerdict struct=1`, `ParseBumpVerdict(=1`, `//go:embed bump_classification.md=1`
- Prompt content: `patch | minor | major=1`, `BREAKING CHANGE=2`, `feat:=2`, `"bump":=1`, priority-order `major.*minor.*patch=1`
- Error wrapping: `fmt.Errorf=0`, `errors.(Wrap|Errorf)=14`, `parse bump verdict=6`
- All 8 DescribeTable Entry names present (some appear ≥2× due to second DescribeTable for error-substring assertions — intent satisfied)
- Root `CHANGELOG.md` contains 5 references to `pkg/prompts`
- Commit cff6cd0 ("Classify the next semantic-version bump", 2026-05-27 23:58 +0200) introduced exactly the 4 files (281 insertions)
**Verdict:** PASS

Notes on strict-literal drift: ACs #2 and several DescribeTable count ACs (#9-11, #13, #15) specified exact integer matches that no longer hold because downstream completed spec 058 added 3 prompt files to the same directory and grew the test suite with additional Entries reusing the same case names. Spec 048's deliverables are intact and serve as the foundation; the drift is additive growth from accepted downstream work, not regression.
