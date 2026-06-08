---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-06-08T09:34:12Z"
generating: "2026-06-08T09:36:26Z"
prompted: "2026-06-08T09:48:06Z"
verifying: "2026-06-08T10:11:41Z"
completed: "2026-06-08T14:08:25Z"
branch: dark-factory/github-releaser-lenient-unreleased-detection
---

## Summary

- The github-release watcher today matches the unreleased changelog section by exact string equality against `## Unreleased`. Any variant — case difference, extended wording, alternate verb — silently produces no release task.
- We will make detection lenient: the first H2 that is NOT a version header (`vX.Y.Z` / `X.Y.Z`) is treated as the unreleased section.
- The version-header path remains untouched. Authors who already write `## Unreleased` see no change.
- Failure mode shifts from silent (no release ever fires) to robust (typos, lowercase, alternate verbiage all release correctly).
- One file changes, with table-driven unit tests covering each accepted variant and the version-header negative case.

## Problem

The github-release watcher's changelog parser at `watcher/github-release/pkg/changelog.go` (line 74) compares each H2 heading via `heading == "## Unreleased"`. When an author writes `## unreleased`, `## Unreleased changes`, `## WIP`, or any other variant, the parser does not register an unreleased section. No bullets are counted, no task is published, and no release fires. There is no log line indicating "I saw an H2 that wasn't a version header and wasn't `## Unreleased`" — the operator only notices because the release pipeline silently stops. The release pipeline should not break on a typo in the heading the author types most often.

## Goal

The watcher's changelog parser detects the unreleased section by structural intent, not by literal string match. Any first H2 that is not a version header is treated as the unreleased section. The existing version-header recognition (used to compute `LatestVersion`) is unchanged. All downstream behavior (bullet counting, empty-section skip, first-heading check) continues to operate exactly as before, just keyed off the new detection rule.

## Non-goals

- Do NOT change the bullet extraction logic (`strings.HasPrefix(line, "- ")` and the count).
- Do NOT change the empty-unreleased-section skip behavior (zero bullets still emits no task).
- Do NOT add new log lines for "non-matching H2" — the lenient rule makes them unnecessary; an escape hatch on the Goal is a regression.
- Do NOT change `isVersionHeader` or the regex `versionHeaderRe`.
- Do NOT introduce any configuration flag to toggle lenient vs strict matching — invariant; if a future consumer demands strict matching, that's a separate spec.
- Do NOT touch any file outside `watcher/github-release/pkg/changelog.go`, its test, and the root `CHANGELOG.md`.

## Desired Behavior

1. A changelog whose first H2 is `## Unreleased` is parsed as before: bullets counted, `UnreleasedIsFirst=true`, task emitted if bullets > 0.
2. A changelog whose first H2 is `## unreleased` (any case variation) is parsed as the unreleased section with identical downstream behavior.
3. A changelog whose first H2 is `## Unreleased changes`, `## WIP`, `## Next`, or any other non-version-header phrase is parsed as the unreleased section with identical downstream behavior.
4. A changelog whose first H2 is `## v0.35.0` (a version header) is NOT classified as unreleased. `UnreleasedBullets=0`, `UnreleasedIsFirst=false`, `LatestVersion="0.35.0"`. No task is published.
5. A changelog where the detected unreleased section has zero `- ` bullet lines emits no task — the empty-section skip continues to apply, just keyed off the lenient detection.
6. Once the first H2 is consumed as either the unreleased section or a version header, subsequent H2 headings transition the parser out of "in unreleased" state exactly as today (no double-counting of bullets across sections).
7. A heading with trailing whitespace (`## Unreleased   ` / `## WIP	`) is treated identically to the trimmed form — `strings.TrimRight` already strips it, lenient detection then classifies as non-version → unreleased.

## Constraints

- `ChangelogSummary` struct fields and types are frozen — no additions, no removals, no renames.
- `ParseChangelog` signature is frozen: `func ParseChangelog(content []byte) ChangelogSummary`.
- `isVersionHeader` signature and regex `versionHeaderRe` are frozen.
- The empty-unreleased filter downstream of the parser continues to skip when `UnreleasedBullets == 0`.
- `make precommit` in `watcher/github-release/` must pass with no new lint warnings.
- Existing test function names and subtest case names in `watcher/github-release/pkg/changelog_test.go` that contain the literal `## Unreleased` (or assert on it) must continue to pass unmodified.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Changelog has no H2 headings at all | `UnreleasedBullets=0`, `UnreleasedIsFirst=false`, `LatestVersion=""`; no task | None — author adds a heading |
| First H2 is a version header (`## v1.2.3`), no unreleased section exists | Version captured as `LatestVersion`; no unreleased detected; no task | None — author adds an unreleased section above the version |
| First H2 is non-version (`## WIP`) but has zero bullets underneath | Detected as unreleased; `UnreleasedBullets=0`; empty-section filter skips publish | Author adds a bullet |
| Two consecutive non-version H2s (e.g. `## Unreleased` then `## Next`) | First one wins as the unreleased section; second transitions parser out of unreleased state (no bullets counted under it) | None — handled |
| H2 with trailing whitespace (`## Unreleased   `) | `strings.TrimRight` already strips trailing space/tab; detected as non-version → unreleased | None — handled |
| Empty changelog (`len(content) == 0`) | Returns zero-value `ChangelogSummary` as today | None — handled |

## Acceptance Criteria

- [ ] `cd watcher/github-release && make precommit` exits 0 — evidence: exit code 0
- [ ] `go test ./pkg -run TestParseChangelog -v` passes; table includes named cases `literal_Unreleased`, `lowercase_unreleased`, `extended_Unreleased_changes`, `WIP_heading`, `version_header_first_no_unreleased`, `empty_unreleased_section`, `trailing_whitespace_heading` — evidence: test output shows each named case PASS
- [ ] Test case `literal_Unreleased` asserts `UnreleasedBullets > 0`, `UnreleasedIsFirst == true`, `LatestVersion` matches the second H2 — evidence: assertion in test source
- [ ] Test case `lowercase_unreleased` (fixture: `## unreleased` + bullets) asserts `UnreleasedBullets == <fixture bullet count>`, `UnreleasedIsFirst == true` — evidence: assertion in test source
- [ ] Test case `extended_Unreleased_changes` (fixture: `## Unreleased changes` + bullets) asserts `UnreleasedBullets == <fixture bullet count>`, `UnreleasedIsFirst == true` — evidence: assertion in test source
- [ ] Test case `WIP_heading` (fixture: `## WIP` + bullets) asserts `UnreleasedBullets == <fixture bullet count>`, `UnreleasedIsFirst == true` — evidence: assertion in test source
- [ ] Test case `version_header_first_no_unreleased` (fixture: `## v0.35.0` + bullets) asserts `UnreleasedBullets == 0`, `UnreleasedIsFirst == false`, `LatestVersion == "0.35.0"` — evidence: assertion in test source
- [ ] Test case `empty_unreleased_section` (fixture: `## WIP` followed immediately by `## v0.35.0`) asserts `UnreleasedBullets == 0`, `LatestVersion == "0.35.0"` — evidence: assertion in test source
- [ ] `grep -n 'heading == "## Unreleased"' watcher/github-release/pkg/changelog.go` returns no matches — evidence: exit code 1 (no match)
- [ ] Root `CHANGELOG.md` contains a single new bullet under `## Unreleased` describing lenient detection — evidence: `grep -A 20 '^## Unreleased' CHANGELOG.md` shows the bullet on a line starting with `- `

## Verification

```
cd watcher/github-release
make precommit
go test ./pkg -run TestParseChangelog -v
grep -n 'heading == "## Unreleased"' pkg/changelog.go && echo "FAIL: literal still present" || echo "OK: literal removed"
cd ../..
grep -A 20 '^## Unreleased' CHANGELOG.md
```

Expected:
- `make precommit` exits 0
- `go test` shows each named subtest PASS
- The grep for the literal returns no match (exit 1), prints `OK: literal removed`
- The CHANGELOG grep shows the new bullet

**Verification rungs (per `docs/verifying-specs.md`):**
- Rung 1 (unit tests via `make precommit`) is the primary and sufficient surface. The change is pure parsing logic with no I/O, no Kafka, no remote calls.
- Rung 2 (dev k8s deploy + e2e) is NOT required for this spec. No k8s manifest, env, Dockerfile, or remote-fetch path changes.
- Rung 3 (prod) — the post-merge github-releaser auto-release is itself the production proof: observe the next `## Unreleased` → `## vX.Y.Z+1` release after merge. No separate prod scenario.

## Do-Nothing Option

Leave the literal match in place. The release pipeline continues to silently fail whenever an author varies the heading. Each failure costs operator attention to diagnose (no release, no log line). The cost of the fix is one function change plus tests; the cost of the do-nothing is recurring silent breakage. Not acceptable.

## Verification Result

**Verified:** 2026-06-08T13:55:33Z (HEAD 5be2a11)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory (v0.175.0)
**Scenario:** Re-ran `## Verification` block from spec against HEAD: `make precommit` in `watcher/github-release/`, Ginkgo focus on "spec 064", literal-removal grep, CHANGELOG bullet provenance via merge commit.
**Evidence:**
- `cd watcher/github-release && make precommit` exits 0; coverage 82.5%.
- `go test ./pkg -v -ginkgo.focus="spec 064"` runs 10 of 72 specs, all PASS — includes all 7 AC-named cases (`literal_Unreleased`, `lowercase_unreleased`, `extended_Unreleased_changes`, `WIP_heading`, `version_header_first_no_unreleased`, `empty_unreleased_section`, `trailing_whitespace_heading`) plus `version_header_first_then_wip`, `version_first_then_wip_with_bullets`, `second_non_version_h2_after_unreleased`.
- `grep -n 'heading == "## Unreleased"' watcher/github-release/pkg/changelog.go` returns no match (exit 1) — literal removed.
- `git show 189a76d:CHANGELOG.md` shows the `## Unreleased` bullet at merge time; bullet text persists at line 13 of HEAD CHANGELOG.md under `## v0.35.1` after auto-release `5be2a11`.
- Rung 3 production proof: github-releaser autonomously cut v0.35.1 from the literal `## Unreleased` bullet introduced by PR #47 — daemon-of-self auto-release. Lenient-branch live-fire in prod awaits a future bborbe-repo PR with a typo'd heading; spec § Verification Rungs explicitly accepted the auto-release-of-self as the production proof.
**Verdict:** PASS
