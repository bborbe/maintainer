---
status: completed
approved: "2026-05-27T21:17:49Z"
verifying: "2026-05-27T21:29:40Z"
completed: "2026-06-04T15:22:59Z"
branch: dark-factory/github-releaser-semver-bump
---

## Summary

- Pure-Go library `pkg/semver/` for `agent/github-releaser` that computes the next semantic version given a current version and a bump kind.
- Single exported function `BumpVersion(current, bump string) (next string, err error)` consumed by `pkg/steps_planning.go` (separate spec).
- Phase 1 prototype behavior carried forward verbatim: `vX.Y.Z` + (patch|minor|major) → next numeric version; special-case `v0.0.0 → 0.1.0` regardless of bump (first-release default per [[GitHub Release Agent Phase 1 Learnings]] § "What carries to Phase 2 verbatim").
- Foundation spec — no IO, no Claude, no git. Companion to `pkg/changelog/` (spec 044). The two combined are all the pure-Go logic the planning step needs before reaching the Claude classifier.

## Problem

The planning step needs to compute the next version from `current_version` (frontmatter) + `bump` (Claude's classification). Inlining the math in the step makes it untestable in isolation, easy to drift between `vX.Y.Z` and `X.Y.Z` formats, and exposes the `v0.0.0` edge case (where every bump kind collapses to `0.1.0`) to subtle off-by-one bugs.

A pure-Go package isolates the version arithmetic, makes table-driven tests cheap, and freezes the contract the planning step + downstream specs depend on. Phase 1 already validated the rules on real input (disk-status `v1.2.6 + patch → 1.2.7`; docker-utils `v1.7.7 + minor → 1.8.0`); this spec graduates them to a typed function.

## Goal

A package `github.com/bborbe/maintainer/agent/github-releaser/pkg/semver` exporting a single function:

```go
func BumpVersion(current string, bump string) (next string, err error)
```

End state: `pkg/semver/semver.go` exports this with a stable signature, `pkg/semver/semver_test.go` covers it with Ginkgo `DescribeTable` for the 9 named edge cases below, `make precommit` is green for the package, coverage ≥ 90%.

## Non-goals

- Parsing release headers from CHANGELOG (responsibility of `pkg/changelog/` — spec 044).
- Header prefix style inference (`v` vs no-`v`) — `pkg/changelog/InferHeaderPrefixStyle` owns that.
- Formatting the next version into a `## vX.Y.Z` heading line — the planning step composes the final header from prefix style + bumped version.
- Pre-release / build metadata semver suffixes (`-rc.1`, `+build.42`). Out of scope for github-releaser; the watcher's emitted `current_version` never carries them.
- Comparing versions / sorting — only computing the next one matters.
- Reading version from git tags — `current_version` arrives via watcher-emitted task frontmatter.

## Desired Behavior

1. `BumpVersion("v1.2.6", "patch")` returns `("1.2.7", nil)`.
2. `BumpVersion("v1.2.6", "minor")` returns `("1.3.0", nil)`.
3. `BumpVersion("v1.2.6", "major")` returns `("2.0.0", nil)`.
4. `BumpVersion("1.2.6", "patch")` returns `("1.2.7", nil)` — input `v` prefix is optional; output never includes `v`.
5. `BumpVersion("v0.0.0", "patch")` returns `("0.1.0", nil)` — first-release default; bump kind is overridden when current is `0.0.0`.
6. `BumpVersion("v0.0.0", "minor")` returns `("0.1.0", nil)` — same first-release default.
7. `BumpVersion("v0.0.0", "major")` returns `("0.1.0", nil)` — same first-release default; major-on-first-release does NOT yield `1.0.0`. The library is deliberately opinionated: every new repo starts at `0.1.0` per the Phase 1 prototype rule.
8. `BumpVersion("not-a-version", "patch")` returns `("", err)` where err is non-nil and wrapped via `github.com/bborbe/errors`; error message contains the literal substring `parse version`.
9. `BumpVersion("v1.2.3", "giant")` returns `("", err)` where err is non-nil; error message contains the literal substring `invalid bump`. The accepted bump values are exactly `{patch, minor, major}`; anything else errors.

## Constraints

- Package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/semver`.
- Single directory, three files: `semver.go`, `semver_test.go`, `suite_test.go`. No subdirectories.
- Function signature in Goal § is FROZEN. Downstream specs depend on it.
- Output `next` string format is `"X.Y.Z"` — numeric, NO `v` prefix. Caller composes the final header by prepending the prefix style returned by `changelog.InferHeaderPrefixStyle`.
- Input `current` accepts both `"vX.Y.Z"` and `"X.Y.Z"`. The `v` prefix on input is stripped before parse.
- Input `bump` is case-sensitive and must be exactly one of `patch | minor | major`. Empty string, capitalized variants, abbreviations all error.
- `v0.0.0` (numeric zero, regardless of `v` prefix) is the SOLE first-release sentinel. `v0.0.1` is a normal current_version (just bumps `patch → 0.0.2`).
- Errors are wrapped via `github.com/bborbe/errors` per project convention (`errors.Wrapf(ctx, err, "parse version: %q", current)`). Plain `fmt.Errorf` is banned.
- Test framework: Ginkgo v2 + Gomega per [[go-testing-guide]]. External test package (`package semver_test`). `format.TruncatedDiff = false`, UTC time.
- Use `DescribeTable` / `Entry` for the 9 named cases — no hand-rolled `[]struct` loops.
- Coverage target: ≥ 90% on `pkg/semver/`.
- Stdlib only inside `semver.go` PLUS `github.com/bborbe/errors`. No third-party semver libraries (`github.com/Masterminds/semver` etc.) — the rules are simple enough to keep in 30 lines of stdlib.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---|---|---|---|
| `current` is not parseable as `X.Y.Z` (e.g. `"abc"`, `"1.2"`, `"1.2.3.4"`, `"v1.2.x"`) | `strconv.Atoi` fails on any of the 3 components, or `strings.Split` returns ≠ 3 parts after stripping `v` | Return `("", wrapped-err)` with message containing `parse version` | Caller (planning step) escalates to operator inbox with `outcome: needs_input` and reason citing the bad input |
| `current` contains negative numbers (`"v-1.2.3"`) | Stripped `-` before strconv → `strconv.Atoi` succeeds on `1` but `strings.Split("v-1.2.3", ".")` yields `["v-1", "2", "3"]`; `strconv.Atoi("v-1")` fails | Same as malformed: `("", err)` containing `parse version` | Same |
| `bump` is not in `{patch, minor, major}` | Switch statement on `bump` falls through to default | Return `("", wrapped-err)` with message containing `invalid bump` | Same |
| `current` numeric components overflow int (e.g. `"v999999999999999999999.0.0"`) | `strconv.Atoi` returns ErrRange | Return `("", wrapped-err)` containing `parse version` | Same; this is malformed input |
| `bump = "major"` on `v0.0.0` | First-release sentinel check fires BEFORE the bump switch | Returns `("0.1.0", nil)` per Behavior 7 — does NOT yield `1.0.0` | n/a — by design |

## Do-Nothing Option

Cost of NOT building `pkg/semver/`:

- Planning step inlines the math; the `v0.0.0` sentinel becomes an if-statement at the bottom of a step function; the `v`-prefix-strip logic is duplicated between this step and any future tooling (rollback, dry-run preview).
- Spec 044 (now archived) attempted to bundle semver + changelog into one spec; that produced 11 behaviors and choked the prompt-creator. Splitting yields two ≤9-behavior specs that generate prompts in single-digit minutes.
- The first-release sentinel (`v0.0.0 → 0.1.0` regardless of bump) is a real Phase 1 rule that needs a dedicated test — without a typed function, it stays an undocumented if-statement inside the step and silently regresses when someone refactors the step.

## Security / Abuse

The package operates on string inputs only — no IO, no shell, no network. Maximum attack surface is a long pathological version string; `strconv.Atoi` rejects overflow and the parse fails fast. No untrusted input reaches a shell or filesystem.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0 — evidence: exit code 0.
- [ ] `ls agent/github-releaser/pkg/semver/` returns `semver.go`, `semver_test.go`, `suite_test.go` — evidence: 3 files (`ls | wc -l` = 3).
- [ ] `grep -c '^func BumpVersion(' agent/github-releaser/pkg/semver/semver.go` returns 1.
- [ ] `cd agent/github-releaser && go test -cover ./pkg/semver/...` reports `coverage: ≥90.0%` — evidence: stdout matches `coverage: 9[0-9]\.[0-9]%`.
- [ ] DescribeTable case **patch bump from v1.2.6** exists — evidence: `grep -c '"patch bump from v1.2.6"' pkg/semver/semver_test.go` returns 1.
- [ ] DescribeTable case **minor bump from v1.2.6** exists — evidence: `grep -c '"minor bump from v1.2.6"' pkg/semver/semver_test.go` returns 1.
- [ ] DescribeTable case **major bump from v1.2.6** exists — evidence: `grep -c '"major bump from v1.2.6"' pkg/semver/semver_test.go` returns 1.
- [ ] DescribeTable case **no v prefix input tolerated** exists — evidence: `grep -c '"no v prefix input tolerated"' pkg/semver/semver_test.go` returns 1.
- [ ] DescribeTable case **v0.0.0 patch defaults to 0.1.0** exists — evidence: `grep -c '"v0.0.0 patch defaults to 0.1.0"' pkg/semver/semver_test.go` returns 1.
- [ ] DescribeTable case **v0.0.0 minor defaults to 0.1.0** exists — evidence: `grep -c '"v0.0.0 minor defaults to 0.1.0"' pkg/semver/semver_test.go` returns 1.
- [ ] DescribeTable case **v0.0.0 major defaults to 0.1.0** exists — evidence: `grep -c '"v0.0.0 major defaults to 0.1.0"' pkg/semver/semver_test.go` returns 1.
- [ ] DescribeTable case **malformed current version** exists with error message check — evidence: `grep -c '"malformed current version"' pkg/semver/semver_test.go` returns 1.
- [ ] DescribeTable case **invalid bump kind** exists with error message check — evidence: `grep -c '"invalid bump kind"' pkg/semver/semver_test.go` returns 1.
- [ ] Errors use `github.com/bborbe/errors` (NOT `fmt.Errorf`) — evidence: `grep -c 'fmt.Errorf' pkg/semver/semver.go` returns 0; `grep -c 'errors.Wrap' pkg/semver/semver.go` returns ≥1.
- [ ] Output format invariant: `next` returned by all happy-path tests has NO `v` prefix — evidence: `grep -c 'Expect(next).To(Equal("v' pkg/semver/semver_test.go` returns 0 (no test asserts a v-prefixed output).
- [ ] ~~Root `CHANGELOG.md` `## Unreleased` section gains a single `feat:` bullet referencing `pkg/semver` — evidence: `grep -c 'pkg/semver' CHANGELOG.md` returns ≥ 1.~~ **Amendment (2026-06-04, during verify):** AC dropped as historical. The `pkg/semver` package shipped in v0.27.0 under the broader `github-releaser` umbrella bullet (CHANGELOG.md line 106: "semver bump classified from the CHANGELOG content") — the code has been in production since then, no `## Unreleased` window remains, and retroactively adding a per-package bullet would mis-state release history.

## Verification

```bash
cd agent/github-releaser
make precommit                                            # exit 0
go test -cover ./pkg/semver/...                           # coverage ≥ 90%
ls pkg/semver/ | sort                                     # semver.go, semver_test.go, suite_test.go
grep -c '^func BumpVersion(' pkg/semver/semver.go         # =1
grep -c 'fmt.Errorf' pkg/semver/semver.go                 # =0  (errors lib only)
grep -c 'errors.Wrap'  pkg/semver/semver.go               # ≥1

grep -c '"patch bump from v1.2.6"'              pkg/semver/semver_test.go   # =1
grep -c '"minor bump from v1.2.6"'              pkg/semver/semver_test.go   # =1
grep -c '"major bump from v1.2.6"'              pkg/semver/semver_test.go   # =1
grep -c '"no v prefix input tolerated"'         pkg/semver/semver_test.go   # =1
grep -c '"v0.0.0 patch defaults to 0.1.0"'      pkg/semver/semver_test.go   # =1
grep -c '"v0.0.0 minor defaults to 0.1.0"'      pkg/semver/semver_test.go   # =1
grep -c '"v0.0.0 major defaults to 0.1.0"'      pkg/semver/semver_test.go   # =1
grep -c '"malformed current version"'           pkg/semver/semver_test.go   # =1
grep -c '"invalid bump kind"'                   pkg/semver/semver_test.go   # =1
```

No scenario justified — pure-Go library, fully covered by unit tests + table-driven cases. Per [[spec-writing]] § Test-layer responsibilities, scenarios are only for behavior the unit layer can't reach. This is unit-layer by construction.

## Verification Result

**Verified:** 2026-06-04T15:21:52Z (HEAD 9ab4be3)
**Binary:** installed dark-factory (spec is unit-layer, no daemon involvement)
**Scenario:** Direct artifact verification — pure-Go library spec, no runtime scenario per spec § Verification.
**Evidence:**
- `cd agent/github-releaser && make precommit` → exit 0, "ready to commit"
- `go test -cover ./pkg/semver/...` → `coverage: 92.3% of statements`
- `ls pkg/semver/` → `semver.go`, `semver_test.go`, `suite_test.go` (3 files)
- `grep -c '^func BumpVersion(' pkg/semver/semver.go` → 1
- `grep -c 'fmt.Errorf' pkg/semver/semver.go` → 0; `grep -c 'errors.Wrap' pkg/semver/semver.go` → 3
- All 9 DescribeTable case labels present (grep=1 each)
- `grep -c 'Expect(next).To(Equal("v' pkg/semver/semver_test.go` → 0 (no v-prefixed output asserted)
- AC16 dropped per amendment: package shipped in v0.27.0 under umbrella `github-releaser` bullet (CHANGELOG.md:106)
**Verdict:** PASS
