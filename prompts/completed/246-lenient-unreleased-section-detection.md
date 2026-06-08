---
status: completed
spec: [065-github-releaser-agent-lenient-unreleased]
summary: Refactored agent/github-releaser/pkg/changelog/changelog.go to use the lenient 'first non-version H2' rule for unreleased-section detection, mirroring the watcher from spec 064. Added isVersionHeader helper + versionHeaderRe regex; updated ValidateUnreleased, ExtractUnreleasedBullets, InferHeaderPrefixStyle, ExtractUnreleasedBody, ReplaceUnreleasedBody, RewriteUnreleasedHeader. ExtractSectionBody retains exact-match semantics. Added 11 Ginkgo Entry rows in a new DescribeTable 'lenient unreleased-section detection (spec 065)'. All 64 specs pass; make precommit exits 0; coverage 94%.
container: maintainer-agent-lenient-exec-246-lenient-unreleased-section-detection
dark-factory-version: v0.175.0
created: "2026-06-08T15:09:00Z"
queued: "2026-06-08T15:37:26Z"
started: "2026-06-08T15:37:28Z"
completed: "2026-06-08T15:52:09Z"
branch: dark-factory/github-releaser-agent-lenient-unreleased
---

<summary>

- The github-releaser agent's changelog parser no longer rejects author variants of the unreleased heading. `## Unreleased` (literal, current behavior) keeps working — all existing tests continue to pass unmodified
- The first `## ` heading that is NOT a version header (`vX.Y.Z` / `X.Y.Z`) is now the unreleased section, so `## unreleased`, `## Unreleased changes`, `## WIP`, `## Next`, and similar variants release correctly instead of silently failing planning
- Six exported functions (`ValidateUnreleased`, `ExtractUnreleasedBullets`, `ExtractUnreleasedBody`, `ReplaceUnreleasedBody`, `RewriteUnreleasedHeader`) plus `InferHeaderPrefixStyle` are all updated to use the lenient rule. `ExtractSectionBody` is FROZEN — it still does exact-match lookup, which preserves the post-release re-extract path used by `steps_ai_review.go`
- The on-disk heading is preserved verbatim by `ReplaceUnreleasedBody` — input `## WIP` stays `## WIP` until the separate `RewriteUnreleasedHeader` call canonicalizes it to `## vX.Y.Z`. The agent's "lenient on input, canonical on output" invariant is preserved
- One new non-exported helper `isVersionHeader(headingText string) bool` is added with a regex mirror of the watcher's helper of the same name (`^v?\d+\.\d+\.\d+$`)
- Eleven new Ginkgo `Entry` rows in a single `DescribeTable("lenient unreleased-section detection (spec 065)", ...)` cover each accepted variant, the version-header-first negative case, the empty-lenient-section case, the rewrite-to-canonical case, and the version-heading-exact-match path. All named per the spec's AC list

</summary>

<objective>

Make the github-releaser agent's changelog parser detect the unreleased section by the same structural rule the github-release watcher uses (the first H2 that is not a version header), so end-to-end release behavior is identical between watcher-detected and agent-validated changelogs. The exported function signatures stay frozen. `ExtractSectionBody` keeps its exact-match lookup — the lenient rule applies only to the "Unreleased" lookup path, so the post-release re-extract (`steps_ai_review.go` looking up `v1.2.8`) keeps working byte-identically. The on-disk rewrite is still canonical (`## vX.Y.Z`) regardless of the input variant.

</objective>

<context>

Read `/workspace/CLAUDE.md` for project conventions. Read these files fully BEFORE editing:

- `/workspace/agent/github-releaser/pkg/changelog/changelog.go` — the file under edit. The parser at lines 33-65 (`ValidateUnreleased`), lines 75-101 (`findFirstAndUnreleased`), lines 146-196 (`ExtractUnreleasedBullets`), lines 202-234 (`InferHeaderPrefixStyle`), lines 258-263 (`ExtractUnreleasedBody` wrapper), lines 281-324 (`ExtractSectionBody`), lines 341-408 (`ReplaceUnreleasedBody`), lines 428-472 (`RewriteUnreleasedHeader`). All five "Unreleased-scoped" functions gate on `parseHeading(line) == "Unreleased"` (or `headingText == "Unreleased"` for `ExtractUnreleasedBullets` / `InferHeaderPrefixStyle`). All five need to switch to the structural "first non-version H2" rule. The signature of `findFirstAndUnreleased` MAY change — it is internal.
- `/workspace/agent/github-releaser/pkg/changelog/changelog_test.go` — existing Ginkgo test file. ALL pre-existing `Entry` rows in the four `DescribeTable` blocks (`ValidateUnreleased`, `ExtractUnreleasedBullets`, `InferHeaderPrefixStyle`, `RewriteUnreleasedHeader`, `ExtractUnreleasedBody`, `ReplaceUnreleasedBody`) must continue to pass UNMODIFIED. Add the new lenient cases as a single Ginkgo `DescribeTable` block at the END of the file. Do NOT touch the existing tables.
- `/workspace/agent/github-releaser/pkg/steps_planning.go:131` — caller of `ValidateUnreleased`; the lenient change is transparent to this caller. The error message string `"Unreleased section not found."` / `"Unreleased is not the first ## section; …"` / `"Unreleased section has no bullet entries."` MUST be preserved byte-identical (existing classifier `classifyValidationFailure` parses them).
- `/workspace/agent/github-releaser/pkg/steps_ai_review.go:463` — caller of `ExtractSectionBody(ctx, content, headingText)`. This caller passes a version-string heading (e.g. `"v1.2.8"`) and depends on exact-match. Do NOT make `ExtractSectionBody` lenient. The lenient logic goes in the Unreleased-scoped wrapper / scan loop only.
- `/workspace/agent/github-releaser/pkg/git/error_classifier.go:30-32` — declares `ErrorCategoryUnreleasedNotFound` and the comment block at lines 30-32 + 47-50 + 61-65 + 128-130 explain that this category is set by the execution step, not `ClassifyError`, based on the substring `"unreleased header not found"` (lowercase) in the error message. The replacement logic in `changelog.go` MUST keep producing that substring verbatim.

For parity with the watcher's lenient helper, the canonical shape is:

```go
// /workspace/watcher/github-release/pkg/changelog.go lines 28-36
var versionHeaderRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

func isVersionHeader(heading string) (string, bool) {
    versionText := heading[3:] // strip "## "
    if versionHeaderRe.MatchString(versionText) {
        return versionText, true
    }
    return "", false
}
```

The agent's helper takes a different argument shape (the spec specifies `func isVersionHeader(headingText string) bool` — the text after `## `, NOT the full line) and returns just `bool`. The regex `^v?\d+\.\d+\.\d+$` is identical, so a single package-level `var versionHeaderRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)` in `changelog.go` matches the spec.

The current `findFirstAndUnreleased` returns `(*heading, int)` where the int is the line of the strict `## Unreleased` match. The lenient version returns the first non-version H2's line (which is the same value when the input is `## Unreleased`, so the existing test for the `nil content` / `version first` / `unreleased not first` paths all keep their line numbers). The `firstHeading` and `unreleasedLine` invariants merge into a single value when the lenient rule applies — the first H2 IS the unreleased section, so the `firstHeading.line != unreleasedLine` branch (which produces the "Unreleased is not the first ## section; …" reason) is dead under the lenient rule, but it must STAY in the code and remain triggered for the version-header-first case.

The spec's "version-header path still wins over any non-version heading appearing later" rule (DB § Desired Behavior row 2) is enforced by the structural check itself: if the first H2 is a version header, the lenient helper never opens the unreleased section, and `ValidateUnreleased` returns the version-header-first reason. The "two consecutive non-version H2s, first wins" rule (DB § row 9) is enforced by breaking the loop on the first non-version H2 — its bullets are counted, and any later non-version H2 is treated as a section boundary that closes the unreleased scan.

Coding plugin guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbeerrors.Errorf(ctx, "%s header not found", ...)` is the canonical form for typed-context errors. The existing file already uses this pattern; do NOT switch to `fmt.Errorf` or `errors.New`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — small public interface + private struct + `New*` constructor; counterfeiter on every interface. The new helper is private (lowercase `isVersionHeader`) and takes a primitive string, so no interface is required.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — rule `go-testing/no-stdlib-table-tests` and rule `go-testing/no-testing-t-direct`. The new cases must be Ginkgo `DescribeTable` `Entry` rows in the existing `changelog_test.go`, NOT `t.Run`-driven Go table tests. The only test entry point is `TestSuite` in `suite_test.go`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — `funlen 80`, `nestif 4`, `golines 100`. After the refactor, no function in `changelog.go` may exceed the `funlen` limit. The `ValidateUnreleased` function today is well under 80 lines and stays so; the other refactored functions are similar.

</context>

<requirements>

1. **Add a non-exported `isVersionHeader(headingText string) bool` helper and a package-level regex `var versionHeaderRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`).** Both go at the top of `changelog.go` near the existing `vPrefixRE` / `noPrefixRE` block at lines 24-27. Signature is exactly `func isVersionHeader(headingText string) bool { return versionHeaderRe.MatchString(headingText) }`. The regex shape matches the watcher's `versionHeaderRe` byte-for-byte. No other public/private symbol is added to the regex set.

2. **Replace the literal `parseHeading(line) == "Unreleased"` / `headingText == "Unreleased"` checks in `findFirstAndUnreleased` (changelog.go lines 75-101) with the structural "first non-version H2" rule.** The new shape of the function:

   ```go
   // findFirstAndUnreleased scans the content and returns the FIRST ## heading
   // (firstHeading) plus the line of the first ## heading that is NOT a version
   // header (unreleasedLine). When the first ## heading is a non-version header
   // (i.e. the lenient rule classifies it as the unreleased section),
   // firstHeading.line == unreleasedLine and the "version header first" branch
   // of ValidateUnreleased is skipped. When the first ## heading IS a version
   // header, unreleasedLine is 0 unless a later ## heading is non-version.
   func findFirstAndUnreleased(scanner *bufio.Scanner) (*heading, int) {
       var firstHeading *heading
       unreleasedLine := 0
       lineNum := 0

       for scanner.Scan() {
           lineNum++
           line := scanner.Text()
           if !isHeading(line) {
               continue
           }
           headingText := parseHeading(line)
           if firstHeading == nil {
               firstHeading = &heading{line: lineNum, text: headingText}
           }
           if !isVersionHeader(headingText) {
               unreleasedLine = lineNum
               break
           }
       }
       return firstHeading, unreleasedLine
   }
   ```

   Note the helper is renamed `findFirstAndUnreleased` (unchanged, to keep the call site at line 40 stable) but the semantic is now "first heading + first non-version heading" — the variable naming and GoDoc must reflect that. The function's EXTERNAL contract (signature `func findFirstAndUnreleased(scanner *bufio.Scanner) (*heading, int)`) is unchanged per spec § Constraints ("The `heading` struct and `findFirstAndUnreleased`'s signature are internal — they MAY change."). You MAY keep the old name or rename — but if you rename, update the call site at line 40 accordingly. The simplest choice: keep the name, update the body.

3. **Update `ValidateUnreleased` (changelog.go lines 33-65) so the lenient-detected unreleased section drives the validation result.** Specifically:

   (a) `firstHeading == nil` is the "no `## ` heading at all" case → return `(false, "Unreleased section not found.", 0)`. This is the only path that produces this reason; the version-header-only case (covered in 3b) uses a different reason per spec AC #7.

   (b) When `firstHeading` is set but `unreleasedLine` is 0 (every `## ` heading is a version header) OR `firstHeading.line != unreleasedLine` (a version header is the first H2 and a non-version H2 appears later), return the same reason string: `"Unreleased is not the first ## section; found '%s' at line %d. Move ## Unreleased above all release headings."` with `firstHeading.text` and `firstHeading.line` interpolated. This reason is byte-identical to the current message and matches both the existing "P1 fail - Unreleased not first" `Entry` row (line 27-35 in the test file: `## 1.2.6` first, `## Unreleased` later) AND the new `version_header_first_no_unreleased` `Entry` row (version header first, NO non-version H2 anywhere). Under the lenient rule, `1.2.6` matches `versionHeaderRe` (`^v?\d+\.\d+\.\d+$`) so it IS a version header per the helper — the "Unreleased is not the first ## section" reason fires for BOTH the "version-then-lenient" and "version-only" cases, differing only in whether `unreleasedLine` was set.

   (c) When `firstHeading.line == unreleasedLine > 0` (the first H2 is a non-version heading — the lenient rule accepts it as the unreleased section), continue to `hasBulletInBlock` (lines 123-134). If no bullet found, return `(false, "Unreleased section has no bullet entries.", unreleasedLine)`. If a bullet is found, return `(true, "", 0)`.

4. **Refactor `ExtractUnreleasedBullets` (changelog.go lines 146-196) to use the lenient rule.** The current code does its own scan loop and breaks on `headingText == "Unreleased"`. Replace the break condition with `!isVersionHeader(headingText)` (the FIRST non-version H2 opens the unreleased section). The bullet-counting loop (lines 177-195) is otherwise unchanged. The function still returns `nil` for absent / version-only headings and `[]string{}` for a lenient-detected unreleased section with zero bullets.

5. **Refactor `InferHeaderPrefixStyle` (changelog.go lines 202-234) to skip the lenient-detected unreleased heading.** The current code at line 217 does `if headingText == "Unreleased" { continue }`. Replace with `if !isVersionHeader(headingText) { continue }` — the first non-version H2 is the lenient unreleased section, skip it and look for the first VERSION header to infer the prefix. The `vPrefixRE` / `noPrefixRE` block (lines 222-228) is unchanged. The default `"v"` return for "no historic release heading" is unchanged.

6. **Replace the body of `ExtractUnreleasedBody` (changelog.go lines 258-263) so the wrapper does its own lenient scan, NOT via `ExtractSectionBody`.** The current implementation is a one-liner `return ExtractSectionBody(ctx, content, "Unreleased")`. Replace with an inline lenient scanner that mirrors `ExtractSectionBody`'s exact shape (so its empty-content / scanner-error / not-found paths are byte-identical) but uses `!isVersionHeader(parseHeading(line))` as the heading-match condition. The error message produced when no lenient unreleased section is found MUST be `bborbeerrors.Errorf(ctx, "%s header not found", strings.ToLower("unreleased"))` which yields `"unreleased header not found"` (lowercase) — this is the substring the existing `ErrorCategoryUnreleasedNotFound` classifier matches against (per the comment at `git/error_classifier.go:30-32`). The `ctx` parameter remains for error-wrapping consistency. The line-ending normalization to `'\n'` and the leading/trailing-blank-line preservation (verbatim body) are unchanged from `ExtractSectionBody`'s current behavior.

7. **Refactor `ReplaceUnreleasedBody` (changelog.go lines 341-408) to detect the lenient unreleased heading.** The current state machine at lines 360-391 gates `state=0 → state=1` on `parseHeading(line) == "Unreleased"` (line 365). Replace with `!isVersionHeader(parseHeading(line))` (same condition as the wrapper). The verbatim preservation rule is unchanged: input `## WIP` stays `## WIP` on the line where the heading is emitted (line 366 `out.WriteString(line)`), the body swap is independent of the heading text. The error message when no lenient unreleased section is found is unchanged: `bborbeerrors.New(ctx, "unreleased header not found")` (line 396) — the substring must continue to match the classifier. The trailing-newline-less preservation logic at lines 399-405 is unchanged.

8. **Refactor `RewriteUnreleasedHeader` (changelog.go lines 428-472) to detect the lenient unreleased heading.** The current condition at line 447 is `!found && isHeading(line) && parseHeading(line) == "Unreleased"`. Replace `parseHeading(line) == "Unreleased"` with `!isVersionHeader(parseHeading(line))`. The `out.WriteString(newHeader)` at line 448 emits whatever the caller passed (typically `"## v1.2.8"`), so the on-disk heading is canonical regardless of the input variant — the spec's "lenient on input, canonical on output" invariant. The error message at line 460 is unchanged. The trailing-newline-less preservation logic at lines 463-469 is unchanged.

9. **Do NOT change `ExtractSectionBody` (changelog.go lines 281-324).** Its exact-match semantics are load-bearing for the `steps_ai_review.go:463` caller that looks up version strings like `"v1.2.8"` after a release has been cut. The `if !found && isHeading(line) && parseHeading(line) == heading` condition (line 302) stays exact-match. The error message `"%s header not found"` (lines 289, 320) stays. The "first H2 whose parsed text matches the heading" semantics is correct for a version lookup.

10. **Add a new Ginkgo `DescribeTable` to `changelog_test.go` covering the lenient cases.** Append at the END of the file (after the last `Describe("ReplaceUnreleasedBody", ...)` block ending around line 349). The `DescribeTable` description string MUST be exactly `"lenient unreleased-section detection (spec 065)"` — the verification step uses `-ginkgo.focus` on this string. The block uses these named `Entry` rows (each one is a separate spec AC; the test name mirrors the AC name verbatim — snake_case for fixture name, mixed-case for description):

    - `Entry("literal_Unreleased", ...)` — fixture `# Changelog\n\n## Unreleased\n\n- feat: x\n\n## v1.2.8\n`; asserts `ValidateUnreleased(content)` returns `(true, "", 0)` AND `ExtractUnreleasedBullets(content)` returns `[]string{"feat: x"}`. (AC: spec line "literal_Unreleased asserts ValidateUnreleased returns (true, "", 0) and ExtractUnreleasedBullets returns the fixture's bullets in order".)
    - `Entry("lowercase_unreleased", ...)` — fixture with `## unreleased` (lowercase) + bullets; asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ExtractUnreleasedBody(ctx, content)` returns the body. Use the `DescribeTable` body shape with two assertions: `v, r, l := changelog.ValidateUnreleased(content); Expect(v).To(BeTrue()); body, err := changelog.ExtractUnreleasedBody(ctx, content); Expect(err).NotTo(HaveOccurred()); Expect(body).To(ContainSubstring("- feat: x"))`.
    - `Entry("extended_Unreleased_changes", ...)` — fixture `# Changelog\n\n## Unreleased changes\n\n- feat: x\n\n## v1.2.8\n`; asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ExtractUnreleasedBullets` returns `[]string{"feat: x"}`.
    - `Entry("WIP_heading", ...)` — fixture `# Changelog\n\n## WIP\n\n- feat: x\n- fix: y\n`; asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ReplaceUnreleasedBody(ctx, content, "- chore: clean\n")` returns a `[]byte` that contains the literal substring `"## WIP\n"` (the heading line is preserved verbatim, NOT renamed to `## Unreleased` or `## vX.Y.Z`). Use a byte-prefix / byte-contains assertion on the returned `[]byte`.
    - `Entry("Next_heading", ...)` — fixture `# Changelog\n\n## Next\n\n- feat: z\n`; asserts `ValidateUnreleased(content)` returns `(true, "", 0)` AND `ExtractUnreleasedBullets(content)` returns `[]string{"feat: z"}` (parity with `literal_Unreleased` / `extended_Unreleased_changes` — proves the lenient rule extracts bullets under any non-version H2, not just validates).
    - `Entry("version_header_first_no_unreleased", ...)` — fixture `# Changelog\n\n## v0.35.0\n\n- shipped\n`; asserts `ValidateUnreleased(content)` returns `(false, "Unreleased is not the first ## section; found 'v0.35.0' at line 3. Move ## Unreleased above all release headings.", 3)`. The reason string and the line number (3) are byte-identical to the spec AC.
    - `Entry("empty_lenient_section", ...)` — fixture `# Changelog\n\n## WIP\n\n## v0.35.0\n\n- shipped\n`; asserts `ValidateUnreleased(content)` returns `(false, "Unreleased section has no bullet entries.", 3)`. Line 3 is the line of `## WIP`.
    - `Entry("two_non_version_h2s_first_wins", ...)` — fixture `# Changelog\n\n## Unreleased\n\n- a\n- b\n\n## Next\n\n- c\n- d\n\n## v0.35.0\n`; asserts `ExtractUnreleasedBullets(content)` returns `[]string{"a", "b"}` (NOT `{"a", "b", "c", "d"}` — the `## Next` heading closes the unreleased scan).
    - `Entry("rewrite_lowercase_to_canonical", ...)` — fixture `## unreleased\n\n- feat: x\n`; calls `RewriteUnreleasedHeader(ctx, content, "## v0.73.0")` and asserts the result's first 11 bytes are `[]byte("## v0.73.0\n")` (NOT `## unreleased`). Use `Expect(string(got[:11])).To(Equal("## v0.73.0\n"))` or `Expect(string(got)).To(HavePrefix("## v0.73.0\n"))`.
    - `Entry("extract_section_body_version_exact", ...)` — fixture `## Unreleased\n\n- feat: x\n\n## v1.2.8\n\n- old\n`; calls `ExtractSectionBody(ctx, content, "v1.2.8")` and asserts the result contains `- old` (the v1.2.8 body, NOT the Unreleased body). This proves the lenient rule did NOT bleed into the version-heading lookup path.
    - `Entry("infer_prefix_style_with_lenient_unreleased", ...)` — fixture `## WIP\n\n- feat: x\n\n## v1.2.8\n`; asserts `InferHeaderPrefixStyle(content)` returns `"v"` (the lenient `## WIP` heading is skipped, the first version header is `## v1.2.8` which matches `vPrefixRE`).

    The `DescribeTable` body is your choice — use a small local `type lenientCase struct{ content []byte }` and assert each row inline. The `Entry` description strings above are the names that `-ginkgo.focus` matches against (snake_case for the AC name, mixed-case inside the string for human readability — match exactly what the spec's AC list says: `literal_Unreleased`, `lowercase_unreleased`, `extended_Unreleased_changes`, `WIP_heading`, `Next_heading`, `version_header_first_no_unreleased`, `empty_lenient_section`, `two_non_version_h2s_first_wins`, plus the three extra rows).

11. **All pre-existing tests in `changelog_test.go` whose fixture contains a `## Unreleased` (literal) heading must continue to pass UNMODIFIED.** The `## Unreleased` literal continues to work because (a) it is not a version header (it doesn't match `^v?\d+\.\d+\.\d+$`), so (b) the new lenient rule classifies it as the unreleased section. This protects the 6 `Entry` rows at lines 24-26 (literal), 50 (trailing whitespace), 84-90 (extracts bullets), 91-93 (empty returns empty slice), 97-103 (first occurrence wins), 110-112 (leading whitespace bullet) in `ValidateUnreleased` / `ExtractUnreleasedBullets`; the 4 rows at lines 159-194 in `RewriteUnreleasedHeader`; the 9 rows at lines 207-253 in `ExtractUnreleasedBody`; the 7 rows at lines 268-348 in `ReplaceUnreleasedBody`; the 8 rows at lines 122-149 in `InferHeaderPrefixStyle`. Total ~34 rows with literal `## Unreleased`.

    **EXCEPTION — one pre-existing test in `ValidateUnreleased` MUST be updated** because its fixture does NOT contain a literal `## Unreleased`. The `Entry("no Unreleased section", ...)` at lines 36-38 has fixture `# Changelog\n\n## v1.0.0\n\n- initial\n` (version header only, no non-version H2). Under the lenient rule, this fixture produces `(false, "Unreleased is not the first ## section; found 'v1.0.0' at line 3. Move ## Unreleased above all release headings.", 3)` — NOT the historical `"Unreleased section not found."`. Update ONLY this single `Entry` row's expected `reason` and `line` values to the new strings above. The `Entry` description name `"no Unreleased section"` is preserved (the test case is still "the file has no unreleased section" — just with a more specific reason now). All OTHER pre-existing rows stay byte-identical. Run `git diff pkg/changelog/changelog_test.go` after editing — the only changes must be: (a) the new `DescribeTable` block at the end of the file; (b) the one-line update to the `no Unreleased section` row's expected values.

12. **No new log lines, no new config flag, no new public type or interface.** Per spec § Non-goals: the lenient rule makes a "non-matching H2" log unnecessary (an escape hatch on the Goal is a regression), and there is no configuration knob to toggle lenient vs strict (invariant). The change is local to `changelog.go` and `changelog_test.go`.

13. **Verify coverage.** Run `go test -coverprofile=/tmp/cover.out ./pkg/changelog/... && go tool cover -func=/tmp/cover.out | grep changelog.go` — the changed functions must remain at ≥80% statement coverage (coding guidelines). New test cases are inline-string fixtures — no I/O, no mocks. The `isVersionHeader` helper is a one-line regex match; the line is covered transitively by every lenient-case assertion.

</requirements>

<constraints>

- Spec § Constraints (FROZEN signatures):
  - `ValidateUnreleased(content []byte) (valid bool, reason string, line int)`
  - `ExtractUnreleasedBullets(content []byte) []string`
  - `ExtractUnreleasedBody(ctx context.Context, content []byte) (string, error)`
  - `ExtractSectionBody(ctx context.Context, content []byte, heading string) (string, error)` — exact-match; do NOT relax.
  - `ReplaceUnreleasedBody(ctx context.Context, content []byte, newBody string) ([]byte, error)`
  - `RewriteUnreleasedHeader(ctx context.Context, content []byte, newHeader string) ([]byte, error)`
  - `InferHeaderPrefixStyle(content []byte) string`
- Spec § Constraints: error message substring `"unreleased header not found"` (lowercase) MUST be preserved verbatim in `ExtractUnreleasedBody` / `ReplaceUnreleasedBody` / `RewriteUnreleasedHeader` so the existing `ErrorCategoryUnreleasedNotFound` classifier in `git/error_classifier.go:30-32` continues to match.
- Spec § Constraints: `ExtractSectionBody` retains exact-match semantics when called with a heading argument that is NOT the literal `"Unreleased"`. Only `ExtractUnreleasedBody`'s wrapper layer becomes lenient. This preserves the post-release re-extract path in `steps_ai_review.go:463`.
- Spec § Non-goals: do NOT touch `watcher/github-release/`, do NOT change the bump classifier prompt, do NOT add a config flag, do NOT change the Pattern B Job contract, do NOT remove `ErrorCategoryUnreleasedNotFound`, do NOT change `RewriteUnreleasedHeader`'s output format.
- Do NOT commit — dark-factory handles git.
- Do NOT change the existing test cases in `changelog_test.go` (lines 16-349) — they must pass unmodified. New cases are added as a new `DescribeTable` block at the end.
- Coding plugin rule `go-testing/no-stdlib-table-tests` (MUST) — use Ginkgo `DescribeTable` + `Entry`, not `t.Run` table tests.
- Coding plugin rule `go-testing/no-testing-t-direct` (MUST) — do NOT add a `func TestLenientUnreleased(t *testing.T)`; the Ginkgo suite's `TestSuite` in `suite_test.go` is the only test entry point.
- Coding plugin rule `go-precommit/funlen-80` (MUST) — no function in `changelog.go` may exceed 80 lines after the refactor.
- Use `bborbeerrors.Errorf(ctx, ...)` / `bborbeerrors.Wrap(ctx, ...)` / `bborbeerrors.New(ctx, ...)` from `github.com/bborbe/errors` (in-repo precedent: already imported in `changelog.go:19` and used throughout). Do NOT use `fmt.Errorf` or `errors.New`.
- The Go module is `github.com/bborbe/maintainer/agent/github-releaser`; run `make precommit` from inside `agent/github-releaser/`, never at repo root.
- `go.mod` uses module mode (not vendor) — `make buca` regenerates `vendor/` before `docker build`; dark-factory prompt work does not touch `vendor/`.

</constraints>

<verification>

Run from `/workspace/agent/github-releaser/`:

1. `make precommit` — must exit 0. (Runs format + generate + test + lint + license.)
2. `go test ./pkg/changelog -v -ginkgo.v -ginkgo.focus="lenient unreleased-section detection \(spec 065\)"` — must show all 11 new `Entry` rows passing with their snake_case names visible. (The parens in the `DescribeTable` description string are regex metacharacters in Ginkgo focus — escape with `\(` `\)`.) Expected `Entry` names in the output: `literal_Unreleased`, `lowercase_unreleased`, `extended_Unreleased_changes`, `WIP_heading`, `Next_heading`, `version_header_first_no_unreleased`, `empty_lenient_section`, `two_non_version_h2s_first_wins`, `rewrite_lowercase_to_canonical`, `extract_section_body_version_exact`, `infer_prefix_style_with_lenient_unreleased`. Exit code 0.
3. `go test ./pkg/changelog -v -ginkgo.v` — must show all PRE-EXISTING `Entry` rows still passing with their updated expected values: the 8 `ValidateUnreleased` rows (the `no Unreleased section` row at lines 36-38 has its `reason` and `line` updated per req 11), the 7 `ExtractUnreleasedBullets` rows, the 9 `InferHeaderPrefixStyle` rows, the 6 `RewriteUnreleasedHeader` rows, the 12 `ExtractUnreleasedBody` rows, the 9 `ReplaceUnreleasedBody` rows. The `## Unreleased` literal continues to work — only the one non-literal fixture's expected values change.
4. `go test -coverprofile=/tmp/cover.out ./pkg/changelog/... && go tool cover -func=/tmp/cover.out | grep changelog.go` — each changed function must show ≥80.0% (coding guidelines; `changelog.go` as a whole should remain at or above its current coverage). `isVersionHeader` is exercised transitively by every lenient-case assertion.
5. `grep -n 'headingText == "Unreleased"' pkg/changelog/changelog.go && echo "FAIL: strict literal still present" || echo "OK: strict literal removed"` — must print `OK: strict literal removed`.
6. `grep -n 'parseHeading(line) == "Unreleased"' pkg/changelog/changelog.go && echo "FAIL: strict literal still present" || echo "OK: strict literal removed"` — must print `OK: strict literal removed`.
7. `grep -n 'func isVersionHeader' pkg/changelog/changelog.go` — must return exactly one line.
8. `git diff --stat pkg/changelog/changelog_test.go` — must show insertions (the new `DescribeTable` block) and (at most) one modification (the `no Unreleased section` row's expected values per req 11). Use `git diff pkg/changelog/changelog_test.go` to visually confirm the only changed pre-existing row is `no Unreleased section` and that the change is to its `false, "..."` arguments only.

Expected exit codes: 0 for steps 1-4, 1 for steps 5-6 (no match = OK), 0 for step 7, 0 for step 8 with the change pattern described above.

</verification>

<success_criteria>

- `make precommit` in `agent/github-releaser/` exits 0 (AC #1)
- All 11 new Ginkgo `Entry` rows in the `lenient unreleased-section detection (spec 065)` `DescribeTable` PASS (AC #2: `literal_Unreleased`, `lowercase_unreleased`, `extended_Unreleased_changes`, `WIP_heading`, `Next_heading`, `version_header_first_no_unreleased`, `empty_lenient_section`, `two_non_version_h2s_first_wins`; plus the three implementation-coverage rows `rewrite_lowercase_to_canonical`, `extract_section_body_version_exact`, `infer_prefix_style_with_lenient_unreleased`)
- Test `literal_Unreleased` asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ExtractUnreleasedBullets` returns the fixture's bullets in order (AC #3)
- Test `lowercase_unreleased` asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ExtractUnreleasedBody` returns the fixture body (AC #4)
- Test `extended_Unreleased_changes` asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ExtractUnreleasedBullets` returns the bullets (AC #5)
- Test `WIP_heading` asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ReplaceUnreleasedBody` preserves the `## WIP` heading line verbatim while replacing the body (AC #6)
- Test `version_header_first_no_unreleased` asserts `ValidateUnreleased` returns `(false, "Unreleased is not the first ## section; found 'v0.35.0' at line 3. Move ## Unreleased above all release headings.", 3)` (AC #7)
- Test `empty_lenient_section` asserts `ValidateUnreleased` returns `(false, "Unreleased section has no bullet entries.", 3)` (AC #8)
- Test `two_non_version_h2s_first_wins` asserts `ExtractUnreleasedBullets` returns ONLY the bullets between `## Unreleased` and `## Next` (AC #9)
- Test `rewrite_lowercase_to_canonical` asserts the result starts with `## v0.73.0\n` (NOT `## unreleased`) (AC #10)
- Test `extract_section_body_version_exact` asserts `ExtractSectionBody(ctx, content, "v1.2.8")` still returns the v1.2.8 body (AC #11)
- `grep -n 'headingText == "Unreleased"' pkg/changelog/changelog.go` returns no matches (AC #12)
- `grep -n 'parseHeading(line) == "Unreleased"' pkg/changelog/changelog.go` returns no matches (AC #13)
- `grep -n 'func isVersionHeader' pkg/changelog/changelog.go` returns exactly one match (AC #14)
- All pre-existing tests in `agent/github-releaser/pkg/changelog/changelog_test.go` whose fixture contains a literal `## Unreleased` continue to pass without source modification. The one exception — the `no Unreleased section` row at lines 36-38 — has its expected `reason` and `line` updated per req 11 to match the new lenient reason message. `git diff` on the test file shows: (a) the new `DescribeTable` block (additions), (b) the one-line update to the `no Unreleased section` row, and (c) nothing else. (AC #15)
- Rung 2 (dev k8s deploy + re-fire vault-cli task) and Rung 3 (prod cutover) are out-of-band of prompt execution per the spec; the orchestrator (calling command) handles them after the prompt is approved and merged

</success_criteria>

<reference>

- Spec: `/workspace/specs/in-progress/065-github-releaser-agent-lenient-unreleased.md` — full constraints, Desired Behaviors, Failure Modes, and Acceptance Criteria
- File under edit: `/workspace/agent/github-releaser/pkg/changelog/changelog.go` — 6 exported functions + 1 internal helper refactored
- Existing tests (must pass unmodified): `/workspace/agent/github-releaser/pkg/changelog/changelog_test.go` — 51 pre-existing `Entry` rows
- Caller of `ValidateUnreleased`: `/workspace/agent/github-releaser/pkg/steps_planning.go:131`
- Caller of `ExtractSectionBody` (exact-match path): `/workspace/agent/github-releaser/pkg/steps_ai_review.go:463`
- Error classifier that matches the `"unreleased header not found"` substring: `/workspace/agent/github-releaser/pkg/git/error_classifier.go:30-32`
- Watcher lenient parser (for parity reference): `/workspace/watcher/github-release/pkg/changelog.go:28-36` (regex shape), `/workspace/watcher/github-release/pkg/changelog_test.go`
- Sibling spec 064 prompt (template + style reference): `/workspace/prompts/completed/245-spec-064-lenient-unreleased-detection.md`
- Coding plugin docs (in-container paths):
  - `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
  - `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`
  - `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
  - `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md`
  - `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

</reference>
