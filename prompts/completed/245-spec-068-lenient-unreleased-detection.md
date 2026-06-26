---
status: completed
spec: [068-cqrs-trigger-github-build]
summary: Replaced literal '## Unreleased' match in ParseChangelog with structural H2-classification (first non-version H2 is the unreleased section); added 8 Ginkgo Entry rows; inserted '## Unreleased' fix bullet in root CHANGELOG.
container: maintainer-lenient-unreleased-exec-245-spec-064-lenient-unreleased-detection
dark-factory-version: v0.175.0
created: "2026-06-08T09:38:20Z"
queued: "2026-06-08T10:00:49Z"
started: "2026-06-08T10:00:51Z"
completed: "2026-06-08T10:11:41Z"
branch: dark-factory/github-releaser-lenient-unreleased-detection
---

<summary>

- The github-release watcher's changelog parser no longer matches the unreleased section by literal `heading == "## Unreleased"`. The first H2 that is NOT a version header (`vX.Y.Z` / `X.Y.Z`) is now treated as the unreleased section
- `## Unreleased` (literal, current behavior) keeps working — existing test cases all still pass unmodified
- Common author variants — `## unreleased` (lowercase), `## Unreleased changes` (extended), `## WIP`, `## Next` — now release correctly instead of silently producing no task
- The version-header detection (`isVersionHeader` + regex `versionHeaderRe`) is byte-identical, and the `ChangelogSummary` struct / `ParseChangelog` signature are frozen
- Eight new Ginkgo `Entry` rows in a single `DescribeTable("lenient unreleased detection (spec 064)", ...)` cover each accepted variant, the version-header negative case, the version-header-then-WIP negative case, the empty-unreleased filter, and trailing-whitespace trimming — all asserted inline in the table source
- A single `fix:` bullet is added to root `CHANGELOG.md` under `## Unreleased` (no new version, no new release)

</summary>

<objective>

Make the github-release watcher's changelog parser detect the unreleased section by structural intent (the first H2 that is not a version header) rather than by literal string match against `## Unreleased`. The version-header recognition and the public `ChangelogSummary` / `ParseChangelog` surface are frozen. The change is localized to `watcher/github-release/pkg/changelog.go` plus new table-driven test cases in `watcher/github-release/pkg/changelog_test.go`, and a single CHANGELOG bullet.

</objective>

<context>

Read `/workspace/CLAUDE.md` for project conventions. Read these files fully BEFORE editing:

- `/workspace/watcher/github-release/pkg/changelog.go` — the file under edit. The current logic is at lines 49-98. The line that must change is line 74: `if heading == "## Unreleased" {`. This block plus the trailing `if latestVersion == "" { if v, ok := isVersionHeader(heading); ok { latestVersion = v } }` block at lines 86-90 must be re-ordered so the new flow is: (a) H2 arrives; (b) if it's a version header, capture `latestVersion` and stay out of unreleased; (c) otherwise it's the unreleased section — set `inUnreleased = true` and `unreleasedIsFirstH2` if `!seenAnyH2`.
- `/workspace/watcher/github-release/pkg/changelog_test.go` — existing Ginkgo test file. All 8 existing `It(...)` blocks at lines 15-117 must continue to pass UNMODIFIED. Add the new cases as a single Ginkgo `DescribeTable` block — see go-testing-guide rule `go-testing/no-stdlib-table-tests` below.
- `/workspace/watcher/github-release/pkg/suite_test.go` — Ginkgo driver. Test entry is `TestPkg` (not `TestParseChangelog`); see "Verification command deviation" at the bottom of this prompt.
- `/workspace/CHANGELOG.md` — there is currently NO `## Unreleased` section. The preamble ends at line 9 (the last bullet about PATCH). The most recent release `## v0.35.0` starts at line 11. You must INSERT a new `## Unreleased` H2 at line 10 (between preamble and `## v0.35.0`) and add the new bullet underneath it. Layout after edit: line 9 = preamble end, line 10 = blank, line 11 = `## Unreleased`, line 12 = blank, line 13 = `- fix: ...`, line 14 = blank, line 15 = `## v0.35.0` (previously line 11). Do NOT touch any other line.

The current parser, verbatim (lines 49-98 of `changelog.go`):

```go
func ParseChangelog(content []byte) ChangelogSummary {
	if len(content) == 0 {
		return ChangelogSummary{}
	}

	var inUnreleased bool
	var seenAnyH2 bool
	var unreleasedIsFirstH2 bool
	var unreleasedBullets int
	var latestVersion string

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()

		// Not a heading
		if !strings.HasPrefix(line, "## ") {
			if inUnreleased && strings.HasPrefix(line, "- ") {
				unreleasedBullets++
			}
			continue
		}

		// H2 heading
		heading := strings.TrimRight(line, " \t")
		if heading == "## Unreleased" {
			if !seenAnyH2 {
				unreleasedIsFirstH2 = true
			}
			inUnreleased = true
			seenAnyH2 = true
			continue
		}

		// Other heading
		inUnreleased = false
		seenAnyH2 = true
		if latestVersion == "" {
			if v, ok := isVersionHeader(heading); ok {
				latestVersion = v
			}
		}
	}

	return ChangelogSummary{
		UnreleasedBullets: unreleasedBullets,
		UnreleasedIsFirst: unreleasedIsFirstH2,
		LatestVersion:     latestVersion,
	}
}
```

The new flow replaces the `if heading == "## Unreleased" { ... }` block at line 74-81 with a structural test: first try `isVersionHeader(heading)`; if that matches, capture `latestVersion` and do NOT set `inUnreleased`; if it does NOT match, treat the heading as the unreleased section. The order matters — `isVersionHeader` must run before the "everything else is unreleased" branch, otherwise `## v1.2.3` would be classified as unreleased. `strings.TrimRight(line, " \t")` (line 73) is unchanged — trailing whitespace is already stripped.

The new shape of the H2-handling branch (replacing lines 62-78 — the literal-match block AND the "other heading" block):

```go
// H2 heading — classify structurally: version-header OR unreleased.
heading := strings.TrimRight(line, " \t")
isFirstH2 := !seenAnyH2   // snapshot BEFORE setting seenAnyH2
seenAnyH2 = true
if v, ok := isVersionHeader(heading); ok {
    if latestVersion == "" {
        latestVersion = v
    }
    inUnreleased = false
    continue
}
// Non-version H2 → unreleased section (lenient: any phrase counts).
if isFirstH2 {
    unreleasedIsFirstH2 = true
}
inUnreleased = true
```

This is the canonical shape. Use it verbatim (variable names + ordering matter for the unit tests' assertions on `UnreleasedIsFirst`).

Coding plugin guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2/Gomega `DescribeTable` + `Entry` for the new cases; rule `go-testing/no-stdlib-table-tests` explicitly forbids `t.Run`-driven table tests in a Ginkgo suite, and rule `go-testing/no-testing-t-direct` forbids adding a `func TestParseChangelog(t *testing.T)` to this package. The new cases must be Ginkgo `DescribeTable` `Entry` rows, NOT a `func TestParseChangelog` Go test.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` must use a conventional prefix (`fix:` is correct here — this is a bug fix to silent-failure behavior, not a new feature).

</context>

<requirements>

1. **Replace the literal `## Unreleased` match in `ParseChangelog` with structural detection.** In `/workspace/watcher/github-release/pkg/changelog.go`, replace BOTH the `if heading == "## Unreleased" { ... }` block (currently lines 62-69) AND the "Other heading" block immediately following it (currently lines 71-78) with the canonical pseudocode shown in `<context>` above. The replacement consolidates the two blocks into one classifier that runs `isVersionHeader` first; the non-version path is the lenient unreleased branch. The new code must satisfy ALL of:

   (a) **Version-header path:** `isVersionHeader(heading)` returns `ok=true` → if `latestVersion == ""`, capture `v` into `latestVersion` (first-version-wins). Set `inUnreleased = false`. Set `seenAnyH2 = true`. `continue` past the bullet-counting block.

   (b) **Lenient unreleased path:** `isVersionHeader(heading)` returns `ok=false` → snapshot `isFirstH2 := !seenAnyH2` BEFORE setting `seenAnyH2 = true`. Then set `seenAnyH2 = true`, set `unreleasedIsFirstH2 = true` only if `isFirstH2` is true, and set `inUnreleased = true`. Fall through (no `continue` needed — the loop iteration ends naturally after this block).

   (c) A `## v0.35.0` H2 that is the FIRST H2 in the file produces `UnreleasedIsFirst=false`, `UnreleasedBullets=0`, `LatestVersion="0.35.0"`. Test `version_header_first_no_unreleased` covers this.

   (d) A `## v0.35.0` followed by `## WIP` (version-header first, non-version second) produces `UnreleasedIsFirst=false` (because `seenAnyH2` was already true when WIP arrived), `UnreleasedBullets=0` because WIP has no bullets, and `LatestVersion="0.35.0"`. Test `version_header_first_then_wip` (req 5) covers this.

2. **Do NOT touch `isVersionHeader` or `versionHeaderRe`.** Lines 28-36 stay byte-identical. The regex `versionHeaderRe` and the helper signature are frozen per spec § Constraints.

3. **Do NOT change the `ChangelogSummary` struct, the `ParseChangelog` signature, or the `seenAnyH2` / `unreleasedIsFirstH2` / `unreleasedBullets` / `latestVersion` / `inUnreleased` local variable set.** Only the H2-classification branch at lines 74-81 is edited. The struct, signature, and locals are frozen per spec § Constraints.

4. **Add a new Ginkgo `DescribeTable` to `changelog_test.go` for the lenient detection variants.** Append the new `DescribeTable` block at the END of the existing `Describe("pkg.ParseChangelog", func() { ... })` (do NOT add a sibling `Describe`, do NOT convert the existing `It` blocks to `Entry` rows — leave lines 15-117 byte-identical). The `DescribeTable` description string MUST be exactly `"lenient unreleased detection (spec 064)"` — verify step 2 below uses `-ginkgo.focus` on this string. The block uses seven named `Entry` rows here (req 5 adds the eighth). Use exactly these names (snake_case, matching the spec):

   - `Entry("literal_Unreleased", fixtureLiteral, want{Bullets: 1, IsFirst: true, Latest: "v1.2.3"})`
   - `Entry("lowercase_unreleased", fixtureLowercase, want{Bullets: 2, IsFirst: true, Latest: "v1.2.3"})`
   - `Entry("extended_Unreleased_changes", fixtureExtended, want{Bullets: 1, IsFirst: true, Latest: "v1.2.3"})`
   - `Entry("WIP_heading", fixtureWIP, want{Bullets: 2, IsFirst: true, Latest: "v1.2.3"})`
   - `Entry("version_header_first_no_unreleased", fixtureVersionFirst, want{Bullets: 0, IsFirst: false, Latest: "v0.35.0"})`
   - `Entry("empty_unreleased_section", fixtureEmpty, want{Bullets: 0, IsFirst: true, Latest: "v0.35.0"})`
   - `Entry("trailing_whitespace_heading", fixtureTrailingWS, want{Bullets: 1, IsFirst: true, Latest: "v1.2.3"})`

   Where the fixtures are inline string literals of the form:

   ```go
   fixtureLiteral := `# Changelog

   ## Unreleased

   - new entry

   ## v1.2.3

   - old
   `

   fixtureLowercase := `# Changelog

   ## unreleased

   - entry one
   - entry two

   ## v1.2.3
   `

   fixtureExtended := `# Changelog

   ## Unreleased changes

   - one

   ## v1.2.3
   `

   fixtureWIP := `# Changelog

   ## WIP

   - alpha
   - beta

   ## v1.2.3
   `

   fixtureVersionFirst := `# Changelog

   ## v0.35.0

   - shipped
   `

   fixtureEmpty := `# Changelog

   ## WIP

   ## v0.35.0

   - shipped
   `

   fixtureTrailingWS := "# Changelog\n\n## WIP\t\n\n- one\n\n## v1.2.3\n"
   ```

   The `want` struct and the `DescribeTable` body shape are your choice — use a small local `type want struct{ Bullets int; IsFirst bool; Latest string }` and assert with `Expect(summary.UnreleasedBullets).To(Equal(w.Bullets))` etc.

5. **Add an eighth `Entry` row to the SAME `DescribeTable` asserting the version-header path still wins when a non-version H2 follows.** Add this row inline in the table built in req 4 (not a separate `It` or `Describe`). Name it `version_header_first_then_wip`. Fixture inline:

   ```go
   fixtureVersionThenWIP := `# Changelog

   ## v0.35.0

   - shipped

   ## WIP

   - should not count
   `
   ```

   `Entry("version_header_first_then_wip", fixtureVersionThenWIP, want{Bullets: 0, IsFirst: false, Latest: "v0.35.0"})`. This proves the spec § Failure Modes row "First H2 is a version header, no unreleased section exists" still works after the lenient rule.

6. **Insert a `## Unreleased` section into root `/workspace/CHANGELOG.md` and add a single `fix:` bullet.** The CHANGELOG currently has NO `## Unreleased` heading — the preamble ends at line 9 and `## v0.35.0` starts at line 11. Insert a new `## Unreleased` H2 with a single bullet between them, so the file becomes:

   ```
   ... (lines 1-9 preamble unchanged) ...

   ## Unreleased

   - fix(watcher/github-release): lenient unreleased-section detection — the first H2 that is not a version header (vX.Y.Z / X.Y.Z) is now treated as the unreleased section, so "## unreleased", "## Unreleased changes", "## WIP", and similar variants release correctly instead of silently producing no task (spec 064)

   ## v0.35.0
   ... (rest unchanged) ...
   ```

   Do NOT touch the preamble, do NOT modify any existing `## vX.Y.Z` section, do NOT add anything above the preamble. The bullet text wording above is fine — only polish for length, do not change the core meaning.

7. **No new log lines, no new config flag.** Per spec § Non-goals: the lenient rule makes a "non-matching H2" log unnecessary (an escape hatch on the Goal is a regression), and there is no configuration knob to toggle lenient vs strict (invariant).

8. **All 8 existing `It(...)` blocks in `changelog_test.go` must continue to pass UNMODIFIED.** The `## Unreleased` literal continues to work because (a) it is not a version header, so (b) the new lenient rule classifies it as the unreleased section. Run `go test ./pkg -v -ginkgo.v` (the `-ginkgo.v` flag is required for individual `It` descriptions to surface in the output) and confirm all 8 existing test descriptions still print PASS.

9. **Verify coverage.** Run `go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out | grep changelog.go` — the changed `ParseChangelog` function must remain at 100% statement coverage. New test cases are inline-string fixtures — no I/O, no mocks. (`go.mod` uses module mode, not vendor — see `main_test.go:19` `gexec.Build` call for confirmation.)

</requirements>

<constraints>

- Spec § Constraints: `ChangelogSummary` fields and types are frozen; `ParseChangelog` signature `func ParseChangelog(content []byte) ChangelogSummary` is frozen; `isVersionHeader` and `versionHeaderRe` are frozen; the empty-unreleased filter downstream continues to skip when `UnreleasedBullets == 0`.
- Spec § Non-goals: do NOT change bullet extraction, do NOT add new log lines, do NOT change `isVersionHeader` or the regex, do NOT add a config flag, do NOT touch any file outside `watcher/github-release/pkg/changelog.go`, its test, and the root `CHANGELOG.md`.
- Do NOT commit — dark-factory handles git.
- Do NOT change the existing test cases in `changelog_test.go` (lines 15-117) — they must pass unmodified.
- Coding plugin rule `go-testing/no-stdlib-table-tests` (MUST) — use Ginkgo `DescribeTable` + `Entry`, not `t.Run` table tests.
- Coding plugin rule `go-testing/no-testing-t-direct` (MUST) — do NOT add a `func TestParseChangelog(t *testing.T)`; the Ginkgo suite's `TestPkg` is the only test entry point. This is a deviation from the spec's literal AC `go test ./pkg -run TestParseChangelog -v` — the equivalent verification command is `go test ./pkg -v -ginkgo.focus="lenient unreleased detection (spec 064)"` (see "Verification command deviation" at the bottom of this prompt).
- The Go module is `github.com/bborbe/maintainer/watcher/github-release`; run `make precommit` from inside `watcher/github-release/`, never at repo root.
- `go.mod` uses `mod=mod` (not vendor) — see the `gexec.Build` call in `main_test.go` for confirmation.

</constraints>

<verification>

Run from `/workspace/watcher/github-release/`:

1. `make precommit` — must exit 0.
2. `go test ./pkg -v -ginkgo.v -ginkgo.focus="lenient unreleased detection \(spec 064\)"` — must show all 8 new `Entry` rows passing with their snake_case names visible. (The parens in the `DescribeTable` description string are regex metacharacters in Ginkgo focus — escape with `\(` `\)`.) The expected exit code is 0 and the output must include each Entry name: `literal_Unreleased`, `lowercase_unreleased`, `extended_Unreleased_changes`, `WIP_heading`, `version_header_first_no_unreleased`, `empty_unreleased_section`, `trailing_whitespace_heading`, `version_header_first_then_wip`.
3. `go test ./pkg -v -ginkgo.v` — must show all 8 existing `It` blocks plus the 8 new `Entry` rows passing.
4. `go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out | grep changelog.go` — `ParseChangelog` must show `100.0%`.
5. `grep -n 'heading == "## Unreleased"' pkg/changelog.go && echo "FAIL: literal still present" || echo "OK: literal removed"` — must print `OK: literal removed`.
6. `cd /workspace && grep -A 5 '^## Unreleased' CHANGELOG.md` — must print the `## Unreleased` heading followed by a `- fix(watcher/github-release): lenient unreleased-section detection` bullet on a subsequent line.

</verification>

## Verification command deviation

The spec's Acceptance Criterion `go test ./pkg -run TestParseChangelog -v` does NOT match this package — the only Go test function in the package is `TestPkg` (the Ginkgo driver in `suite_test.go`), and the package's `Describe` / `It` blocks are not reachable via `go test -run TestParseChangelog`. Adding a `func TestParseChangelog(t *testing.T)` would violate the `go-testing/no-testing-t-direct` rule. The semantically equivalent verification is `go test ./pkg -v -ginkgo.focus="lenient unreleased detection (spec 064)"` (verify step 2 above), which runs only the new `DescribeTable` and reports each `Entry` row's PASS/FAIL. Verify step 3 runs the full suite to confirm the existing 8 `It` blocks still pass. Flag this deviation to the user in your completion report so they know the spec AC command was not used verbatim.
