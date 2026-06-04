---
status: completed
approved: "2026-05-27T20:46:31Z"
verifying: "2026-05-27T21:27:13Z"
completed: "2026-06-04T15:17:02Z"
branch: dark-factory/github-releaser-changelog-parser
---

## Summary

- Pure-Go library `pkg/changelog/` for `agent/github-releaser` that parses a `CHANGELOG.md` byte stream and exposes the data the planning phase needs to decide whether the watcher's emitted task is releaseable.
- Three responsibilities only: precondition validation, Unreleased-bullets extraction, and historic-header-prefix-style inference. No IO, no Claude, no git.
- Foundation for the github-releaser planning phase. Consumed by `pkg/steps_planning.go` (separate spec). This spec ships the library + tests; integration is downstream.
- Phase 1 prototype (`/github-release-repo` slash command) already validated these rules on real CHANGELOGs (`disk-status` escalation, `docker-utils v1.7.8` happy path). Behavior carries verbatim per [[GitHub Release Agent Phase 1 Learnings]] § "What carries to Phase 2 verbatim".

## Problem

The Phase 2 Go agent needs to read a target repo's `CHANGELOG.md` and decide three things before invoking Claude: (1) is the Unreleased section structurally valid for a release, (2) what bullets does it contain for classification, (3) what header-prefix style does this repo use so the next-version header matches existing conventions. Without a dedicated package these decisions get scattered across the planning step's main function, become un-mockable, and the precondition rules (which Phase 1 surfaced as critical safety boundaries) drift between watcher and agent.

A pure-Go package isolates the validation logic from agent infrastructure (Claude runner, agent/lib, Kafka), making it unit-testable with table-driven tests and making the planning step a thin orchestrator over a well-defined contract.

## Goal

A package `github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog` that exposes three pure functions consumed by `pkg/steps_planning.go`:

1. A validator that returns `(valid bool, reason string, lineNumber int)` for the P1 + P2 preconditions on `CHANGELOG.md` bytes.
2. An extractor that returns the `## Unreleased` block's `- ` bullets as a slice of strings.
3. A header-style inferrer that returns `"v"` or `""` based on the first historic release heading (with a documented default for the no-historic-release case).

End state: `pkg/changelog/changelog.go` exports these three with stable signatures, `pkg/changelog/changelog_test.go` covers them with Ginkgo `DescribeTable` covering the 8 named edge cases below, `make precommit` is green for the package.

## Non-goals

- Planning-step wiring (separate spec).
- Claude bump classification (separate spec — embedded prompt + JSON parse helper).
- Semver compute (separate spec).
- Writing `## Plan` JSON sections (responsibility of the planning step, separate spec).
- Reading CHANGELOG via the GitHub REST API (responsibility of the planning step; this package operates on `[]byte` only).
- Parsing release sections (`## vX.Y.Z` bullets) — only `## Unreleased` matters for planning.
- Mono-repo CHANGELOG support (per parent goal Non-goals).

## Desired Behavior

1. `ValidateUnreleased(content []byte) (valid bool, reason string, line int)` returns `(true, "", 0)` when CHANGELOG content has `## Unreleased` as the first `## ` heading AND that section contains at least one `- ` bullet (lines starting with `- `) between the heading and the next `## ` heading (or EOF).
2. P1 failure: when the first `## ` heading is NOT `## Unreleased`, returns `(false, "...", lineNumber)` where `lineNumber` is the 1-indexed line of the actual first `## ` heading and `reason` is the literal string `"Unreleased is not the first ## section; found '<heading-text>' at line <N>. Move ## Unreleased above all release headings."` (heading-text is the raw heading line stripped of the `## ` prefix).
3. P2 failure: when `## Unreleased` IS the first `## ` heading but its block has zero `- ` bullets, returns `(false, "Unreleased section has no bullet entries.", <unreleased-line>)`.
4. Absent-Unreleased case: when CHANGELOG contains NO `## Unreleased` heading at all, returns `(false, "Unreleased section not found.", 0)`.
5. `ExtractUnreleasedBullets(content []byte) []string` returns the slice of bullet lines (stripped of the leading `- `) between the `## Unreleased` heading and the next `## ` heading (or EOF). Empty `## Unreleased` returns `[]string{}` (non-nil empty slice). Absent `## Unreleased` returns `nil`.
6. `InferHeaderPrefixStyle(content []byte) string` returns `"v"` when the first historic release heading matches `^## v[0-9]+\.`, `""` when it matches `^## [0-9]+\.`, and `"v"` by default when no historic release exists in the file.
7. The inferrer skips `## Unreleased` and the H1 + preamble when scanning for the first historic release.
8. All three functions are pure: deterministic, no global state, no IO, no goroutines. Same input always yields same output.

## Constraints

- Package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog`.
- Single file `pkg/changelog/changelog.go` for production code + `pkg/changelog/changelog_test.go` for tests + `pkg/changelog/suite_test.go` for the Ginkgo bootstrap. No subdirectories.
- Function signatures listed in Desired Behavior are frozen — downstream specs depend on them.
- Test framework: Ginkgo v2 + Gomega per [[go-testing-guide]]. External test package (`package changelog_test`). `format.TruncatedDiff = false`, UTC time.
- Use `DescribeTable` / `Entry` for the 8 named cases below — no hand-rolled `[]struct` loops.
- Error wrapping: not applicable (no errors returned — preconditions surface via `(valid, reason, line)` triple, not Go errors).
- Coverage target: ≥ 90% on `pkg/changelog/` (small pure-Go pkg; high coverage is cheap and warranted).
- Counterfeiter mocks: not needed (no interfaces in this spec; it's leaf logic).

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---|---|---|---|
| `content == nil` or `len(content) == 0` | First-pass scan finds no `## ` headings | `ValidateUnreleased` returns `(false, "Unreleased section not found.", 0)`; `ExtractUnreleasedBullets` returns `nil`; `InferHeaderPrefixStyle` returns `"v"` (default) | Caller (planning step) escalates to operator inbox with `needs_input` |
| CHANGELOG has malformed UTF-8 in bullets | n/a — `bufio.Scanner` over `[]byte` with default split function preserves bytes; functions return whatever lines contain | Treat bytes opaquely; do not attempt UTF-8 normalization | Caller's downstream Claude classifier sees the bytes as-is; if Claude rejects, that's Claude's failure mode |
| Heading lines with trailing whitespace (`## Unreleased   `) | Whitespace-trim the heading line content before comparison | Treated as `## Unreleased` (match) | n/a — pass |
| Bullet lines using `*` or `+` markers instead of `- ` | Strict `^- ` prefix check; lines starting with `*` or `+` are NOT bullets | P2 failure if no `- ` bullets exist; downstream operator must normalize the CHANGELOG | Operator edits CHANGELOG to use `- ` bullets; re-delegates |
| `## Unreleased` heading repeated twice in file | Only the FIRST `## Unreleased` block is parsed; subsequent ones are ignored | Same as single-occurrence | This is malformed CHANGELOG hygiene; operator should fix but the package tolerates it |

## Do-Nothing Option

Cost of NOT building `pkg/changelog/`:

- Planning step ends up with inline precondition logic, untestable in isolation, harder to audit.
- Header-prefix inference scattered across planning + execution steps risks divergence (planning infers one way, execution writes a different style).
- Spec 044 (now archived) tried to bundle these rules into the planning spec; that produced an 11-behavior spec the auditor missed flagging as too heavy and the prompt-creator spent 11+ min grounding before being killed. Smaller foundation spec → faster prompt-gen → cleaner integration spec.

## Security / Abuse

The package operates on `[]byte` only — no file IO, no network, no shell. No attack surface. The only adversarial input is malformed CHANGELOG bytes (long lines, deeply-nested markdown, binary data); the package's tolerance for malformed input is documented in Failure Modes.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0 — evidence: exit code 0.
- [ ] `ls agent/github-releaser/pkg/changelog/` returns `changelog.go`, `changelog_test.go`, `suite_test.go` — evidence: 3 files present (`ls | wc -l` = 3).
- [ ] `grep -c '^func ValidateUnreleased(' agent/github-releaser/pkg/changelog/changelog.go` returns 1.
- [ ] `grep -c '^func ExtractUnreleasedBullets(' agent/github-releaser/pkg/changelog/changelog.go` returns 1.
- [ ] `grep -c '^func InferHeaderPrefixStyle(' agent/github-releaser/pkg/changelog/changelog.go` returns 1.
- [ ] `cd agent/github-releaser && go test -cover ./pkg/changelog/...` reports `coverage: ≥90.0%` — evidence: stdout matches `coverage: 9[0-9]\.[0-9]%`.
- [ ] DescribeTable case **P1-valid (Unreleased first)** exists — evidence: `grep -c '"P1 valid - Unreleased first"' pkg/changelog/changelog_test.go` returns 1.
- [ ] DescribeTable case **P1-fail (Unreleased not first heading)** exists — evidence: `grep -c '"P1 fail - Unreleased not first"' pkg/changelog/changelog_test.go` returns 1.
- [ ] DescribeTable case **no-Unreleased section at all** exists — evidence: `grep -c '"no Unreleased section"' pkg/changelog/changelog_test.go` returns 1.
- [ ] DescribeTable case **P2-fail (empty Unreleased)** exists — evidence: `grep -c '"P2 fail - empty Unreleased"' pkg/changelog/changelog_test.go` returns 1.
- [ ] DescribeTable case **v-prefix-historic** exists — evidence: `grep -c '"v-prefix historic"' pkg/changelog/changelog_test.go` returns 1.
- [ ] DescribeTable case **no-prefix-historic** exists — evidence: `grep -c '"no-prefix historic"' pkg/changelog/changelog_test.go` returns 1.
- [ ] DescribeTable case **no-historic-release-defaults-to-v** exists — evidence: `grep -c '"no historic release defaults to v"' pkg/changelog/changelog_test.go` returns 1.
- [ ] DescribeTable case **trailing-whitespace-heading-tolerated** exists — evidence: `grep -c '"trailing whitespace heading tolerated"' pkg/changelog/changelog_test.go` returns 1.
- [ ] P1-fail entry asserts the exact reason string: when CHANGELOG has `## 1.2.6` at line 11 before `## Unreleased`, the test asserts `reason == "Unreleased is not the first ## section; found '1.2.6' at line 11. Move ## Unreleased above all release headings."` — evidence: `grep -c "found '1.2.6' at line 11" pkg/changelog/changelog_test.go` returns 1.
- [ ] `CHANGELOG.md` (root) `## Unreleased` section gains a `feat:` entry referencing this spec per [[changelog-guide]].

## Verification

```bash
cd ~/Documents/workspaces/maintainer-github-releaser/agent/github-releaser
make precommit                                                                # exit 0
go test -cover ./pkg/changelog/...                                            # coverage ≥90%
ls pkg/changelog/ | sort                                                      # changelog.go, changelog_test.go, suite_test.go
grep -c '^func ValidateUnreleased('       pkg/changelog/changelog.go         # =1
grep -c '^func ExtractUnreleasedBullets(' pkg/changelog/changelog.go         # =1
grep -c '^func InferHeaderPrefixStyle('   pkg/changelog/changelog.go         # =1
grep -c '"P1 valid - Unreleased first"'             pkg/changelog/changelog_test.go  # =1
grep -c '"P1 fail - Unreleased not first"'          pkg/changelog/changelog_test.go  # =1
grep -c '"no Unreleased section"'                   pkg/changelog/changelog_test.go  # =1
grep -c '"P2 fail - empty Unreleased"'              pkg/changelog/changelog_test.go  # =1
grep -c '"v-prefix historic"'                       pkg/changelog/changelog_test.go  # =1
grep -c '"no-prefix historic"'                      pkg/changelog/changelog_test.go  # =1
grep -c '"no historic release defaults to v"'       pkg/changelog/changelog_test.go  # =1
grep -c '"trailing whitespace heading tolerated"'   pkg/changelog/changelog_test.go  # =1
grep -c "found '1.2.6' at line 11"                  pkg/changelog/changelog_test.go  # =1
```

No scenario justified at this rung — this is leaf-library logic fully covered by unit tests + table-driven cases. Per [[spec-writing]] § Test-layer responsibilities, scenarios are for behavior the unit + integration layers genuinely cannot reach; this spec is unit-layer by construction.

## Verification Result

**Verified:** 2026-06-04T15:16:33Z (HEAD d93847f)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory v0.175.0
**Scenario:** Walked each AC against `agent/github-releaser/pkg/changelog/` on maintainer master.
**Evidence:**
- `cd agent/github-releaser && make precommit` → `ready to commit`, EXIT=0
- `ls pkg/changelog/` → `changelog.go`, `changelog_test.go`, `suite_test.go` (3 files)
- `grep -c '^func ValidateUnreleased('` = 1; `ExtractUnreleasedBullets` = 1; `InferHeaderPrefixStyle` = 1
- `go test -cover ./pkg/changelog/...` → `coverage: 95.7% of statements` (≥90%)
- All 8 DescribeTable case names grep = 1 each; `"found '1.2.6' at line 11"` grep = 1
- Root CHANGELOG.md `## Unreleased` feat entry referencing spec was shipped in commit 76e0f86 (`feat(agent/github-releaser): add pkg/changelog parser library — pure-Go ValidateUnreleased/ExtractUnreleasedBullets/InferHeaderPrefixStyle for planning step (spec 044)`) — spec 044 was renumbered to 046 in 54de50b; the entry was later rolled into v0.27.0's summary bullets per the release pipeline
**Verdict:** PASS
