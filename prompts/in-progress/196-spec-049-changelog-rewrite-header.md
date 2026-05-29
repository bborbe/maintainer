---
status: approved
spec: [049-github-releaser-execution-phase-direct-push]
created: "2026-05-29T00:00:00Z"
queued: "2026-05-28T22:17:49Z"
---

<summary>
- Extends `pkg/changelog/` with one new pure function: `RewriteUnreleasedHeader(content []byte, newHeader string) ([]byte, error)`.
- The function locates the `## Unreleased` line (whitespace-tolerant, same rule as `InferHeaderPrefixStyle`) and replaces ONLY that line with `newHeader`. Bullets and other lines untouched.
- Returns a wrapped `bborbe/errors` error when `## Unreleased` is not present — caller (prompt 3) maps this to `error_category: unreleased_not_found`.
- DescribeTable covers happy path, trailing-whitespace tolerance, and the not-found error path.
- Pure-Go function — no IO, no context dependency. Coverage target ≥ 90% on the package (existing parsers already at 97.7%; adding the new function with table tests keeps it above 90%).
</summary>

<objective>
Add `RewriteUnreleasedHeader` to `agent/github-releaser/pkg/changelog/changelog.go` and exhaustive `DescribeTable` tests in `pkg/changelog/changelog_test.go`. The function is consumed by prompt 3's `ExecutionStep` to rewrite the cloned repo's CHANGELOG before commit.

End state: `cd agent/github-releaser && go test -cover ./pkg/changelog/...` reports ≥ 90% coverage; the function is grep-anchored at the top of `changelog.go` (`grep -c '^func RewriteUnreleasedHeader('` returns 1); the test file contains ≥ 3 DescribeTable entries each starting with the literal `"rewrite unreleased`.
</objective>

<context>
Read before writing code (repo-relative paths; container mounts repo root at `/workspace`):

- `CLAUDE.md` at repo root.
- `specs/in-progress/049-github-releaser-execution-phase-direct-push.md` — re-read Desired Behavior 2, Constraints "RewriteUnreleasedHeader lives in pkg/changelog/changelog.go", Acceptance Criteria row "RewriteUnreleasedHeader DescribeTable" (3 entries minimum, each entry name starting with `"rewrite unreleased`).
- `agent/github-releaser/pkg/changelog/changelog.go` — full file. Add the new function next to `InferHeaderPrefixStyle`. Reuse the existing helpers (`isHeading`, `parseHeading`, `trimTrailingWhitespace`) — do NOT re-invent string scanning. Already exports `ValidateUnreleased`, `ExtractUnreleasedBullets`, `InferHeaderPrefixStyle`. Note: existing helpers are unexported.
- `agent/github-releaser/pkg/changelog/changelog_test.go` lines 14-60 — the canonical DescribeTable pattern in this package (read once for shape, then mirror).

Helper functions already present in `changelog.go` (read once for signature; reuse):
- `isHeading(line string) bool` — line starts with `"## "`.
- `parseHeading(line string) string` — strips `"## "` prefix and trailing whitespace.
- `trimTrailingWhitespace(s string) string` — trailing space/tab/CR removed.

Module-local imports:
- `github.com/bborbe/errors` for the not-found wrapped error. NO `fmt.Errorf`.

Coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` API. **Verified at `~/go/pkg/mod/github.com/bborbe/errors@v1.5.1/`**: `errors.New(ctx, msg)` and `errors.Wrap(ctx, err, msg)` are the only forms — both require `context.Context` as the first arg. No no-context variant exists. Use `context.Background()` since `RewriteUnreleasedHeader` is a pure function with no upstream ctx. Exact pattern: `errors.New(context.Background(), "unreleased header not found")` and `errors.Wrap(context.Background(), err, "scan CHANGELOG content")`. Add `"context"` to the import block.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — DescribeTable / Entry conventions.
</context>

<requirements>

**Run order: do steps in sequence. Run `cd agent/github-releaser && go test ./pkg/changelog/...` after step 2. Run `cd agent/github-releaser && make precommit` only as the final verification.**

1. **Add `RewriteUnreleasedHeader` to `agent/github-releaser/pkg/changelog/changelog.go`** — append at the END of the file (after `trimTrailingWhitespace`). The function MUST be anchored to column 1 (start of line) so the AC grep `grep -c '^func RewriteUnreleasedHeader(' pkg/changelog/changelog.go` returns 1:

   ```go
   // RewriteUnreleasedHeader returns content with the "## Unreleased" line
   // replaced by newHeader (e.g. "## v1.2.8"). Whitespace-tolerant: trailing
   // spaces/tabs/CR on the Unreleased heading are accepted and discarded along
   // with the original line. All other lines (bullets, blank lines, other
   // headings) are preserved verbatim, including their original line endings.
   //
   // newHeader is inserted as-is. The caller is responsible for the leading
   // "## " prefix and any trailing newline normalization is left to the
   // existing content's line-ending convention (the function preserves the
   // newline that followed the original "## Unreleased" line, if any).
   //
   // Returns a wrapped bborbe/errors error if "## Unreleased" is not present.
   // The caller (execution step) maps this to error_category: unreleased_not_found.
   //
   // Pure function — no IO, deterministic. Safe for concurrent use.
   func RewriteUnreleasedHeader(content []byte, newHeader string) ([]byte, error) {
       if len(content) == 0 {
           return nil, bborbeerrors.New(context.Background(), "unreleased header not found: empty content")
       }

       scanner := bufio.NewScanner(bytes.NewReader(content))
       // The bufio default 64KB token limit is plenty for CHANGELOGs.

       var out bytes.Buffer
       found := false
       for scanner.Scan() {
           line := scanner.Text()
           if !found && isHeading(line) && parseHeading(line) == "Unreleased" {
               out.WriteString(newHeader)
               out.WriteByte('\n')
               found = true
               continue
           }
           out.WriteString(line)
           out.WriteByte('\n')
       }
       if err := scanner.Err(); err != nil {
           return nil, bborbeerrors.Wrap(context.Background(), err, "scan CHANGELOG content")
       }
       if !found {
           return nil, bborbeerrors.New(context.Background(), "unreleased header not found")
       }

       // Preserve a trailing-newline-less input: if the original content did
       // NOT end with '\n', drop the final '\n' we appended above.
       result := out.Bytes()
       if len(content) > 0 && content[len(content)-1] != '\n' && len(result) > 0 && result[len(result)-1] == '\n' {
           result = result[:len(result)-1]
       }

       return result, nil
   }
   ```

   Notes:
   - Add to imports: `"context"`, `bborbeerrors "github.com/bborbe/errors"` (alias to disambiguate from stdlib `errors`; existing file does not import either).
   - `bborbe/errors@v1.5.1` API verified: `errors.New(ctx, msg)` and `errors.Wrap(ctx, err, msg)` both require `context.Context` as first arg. No no-context variant. Use `context.Background()` since this is a pure function with no upstream ctx.
   - Reuse existing unexported helpers `isHeading` and `parseHeading` — they already handle trailing-whitespace tolerance via `trimTrailingWhitespace`. Do NOT re-implement.
   - The function signature is pure (no `ctx` parameter) on purpose — matches sibling `ValidateUnreleased`, `ExtractUnreleasedBullets`, `InferHeaderPrefixStyle` style. Internal use of `context.Background()` is only for the errors API; the function remains pure in observable behavior.
   - Doc-comment caveat: `bufio.Scanner` normalizes line endings to `\n`. `\r\n`-terminated input will round-trip as `\n`-terminated. Add this caveat to the function doc-comment ("Line endings are normalized to `\n` on rewrite.") to prevent surprise.

2. **Add DescribeTable to `agent/github-releaser/pkg/changelog/changelog_test.go`** — append a new `Describe("RewriteUnreleasedHeader", ...)` block AFTER the existing `Describe("ValidateUnreleased", ...)` block. Three entries minimum (more allowed); each entry name MUST start with the literal string `"rewrite unreleased` (lowercase r) so the AC grep `grep -c '"rewrite unreleased' pkg/changelog/changelog_test.go` returns ≥ 3:

   ```go
   var _ = Describe("RewriteUnreleasedHeader", func() {
       DescribeTable("replaces ## Unreleased line with the given header",
           func(input []byte, newHeader string, expected []byte) {
               got, err := changelog.RewriteUnreleasedHeader(input, newHeader)
               Expect(err).NotTo(HaveOccurred())
               Expect(string(got)).To(Equal(string(expected)))
           },
           Entry("rewrite unreleased — happy path replaces the heading and preserves bullets",
               []byte("# Changelog\n\n## Unreleased\n\n- feat: add foo\n- fix: bar\n\n## v1.0.0\n\n- initial\n"),
               "## v1.0.1",
               []byte("# Changelog\n\n## v1.0.1\n\n- feat: add foo\n- fix: bar\n\n## v1.0.0\n\n- initial\n")),
           Entry("rewrite unreleased — tolerates trailing whitespace on the heading line",
               []byte("# Changelog\n\n## Unreleased   \n\n- feat: bar\n\n## v0.9.0\n\n- old\n"),
               "## v0.9.1",
               []byte("# Changelog\n\n## v0.9.1\n\n- feat: bar\n\n## v0.9.0\n\n- old\n")),
           Entry("rewrite unreleased — first occurrence only when duplicate ## Unreleased present",
               []byte("## Unreleased\n\n- a\n\n## Unreleased\n\n- b\n"),
               "## v1.2.8",
               []byte("## v1.2.8\n\n- a\n\n## Unreleased\n\n- b\n")),
       )

       DescribeTable("returns a wrapped error when ## Unreleased is absent",
           func(input []byte) {
               _, err := changelog.RewriteUnreleasedHeader(input, "## v1.2.3")
               Expect(err).To(HaveOccurred())
               Expect(err.Error()).To(ContainSubstring("unreleased header not found"))
           },
           Entry("rewrite unreleased — error when no Unreleased heading present",
               []byte("# Changelog\n\n## v1.0.0\n\n- initial\n")),
           Entry("rewrite unreleased — error on empty content",
               []byte("")),
       )
   })
   ```

   Notes:
   - All five Entry names start with the literal `"rewrite unreleased` so the grep returns at least 5.
   - The "first occurrence only" entry guards against a regression where the loop might rewrite multiple headings — `RewriteUnreleasedHeader` uses a `found` boolean to short-circuit after the first match.
   - String-comparison via `Expect(string(got)).To(Equal(string(expected)))` — gives readable diffs on failure (versus `Equal([]byte{...})`).

3. **Run package tests** — from `agent/github-releaser/`:

   ```bash
   go test -cover ./pkg/changelog/...
   ```

   Must report ≥ 90% coverage. The existing parsers were at 97.7%; the new function and 5 entries push the total branch count up but every branch is hit:
   - happy path (heading found) — entries 1, 2, 3
   - heading not found, non-empty content — entry 4
   - empty content — entry 5
   - "first occurrence only" path (found=true, continue) — entry 3
   - trailing-newline strip — implicitly via entries that have no trailing newline in input (none currently — if coverage flags the strip branch, add a sixth entry where input has no trailing newline).

   If coverage drops below 90%, add an entry like:
   ```go
   Entry("rewrite unreleased — input without trailing newline keeps no-trailing-newline output",
       []byte("## Unreleased\n\n- feat: x"),
       "## v0.1.1",
       []byte("## v0.1.1\n\n- feat: x")),
   ```

4. **Final verification** — from `agent/github-releaser/`:

   ```bash
   make precommit
   ```

   Must exit 0. No `fmt.Errorf` introduced in `pkg/changelog/changelog.go`.

</requirements>

<constraints>
- Modify `agent/github-releaser/pkg/changelog/changelog.go` and `agent/github-releaser/pkg/changelog/changelog_test.go` only. No new files in `pkg/changelog/`.
- Function signature FROZEN: `func RewriteUnreleasedHeader(content []byte, newHeader string) ([]byte, error)`.
- Function is anchored at column 1 so `grep -c '^func RewriteUnreleasedHeader(' pkg/changelog/changelog.go` returns 1.
- Function is PURE — no IO, no context parameter, no goroutines, no global state mutation. Matches sibling pure functions in the same file.
- Whitespace tolerance: trailing spaces/tabs/CR on `## Unreleased` line MUST be accepted (use existing `parseHeading` helper which already calls `trimTrailingWhitespace`).
- Returns wrapped `bborbe/errors` error on "not found" — the wrapping message MUST contain the literal substring `unreleased header not found` so the test can assert on it AND so the caller (execution step in prompt 3) can verify the substring before mapping to `ErrorCategoryUnreleasedNotFound`.
- First-occurrence-only semantics: if two `## Unreleased` headings exist (malformed input), rewrite ONLY the first one. Guard via a `found` boolean.
- DescribeTable in tests: minimum 3 entries with names starting `"rewrite unreleased` (5 in the spec above; spec AC requires ≥ 3).
- Coverage target ≥ 90% on `pkg/changelog/` total. Existing 97.7% has headroom — keep it above 90% with the new entries.
- Errors via `github.com/bborbe/errors` (`New` / `Wrap` / `Errorf` — whichever form the package exports). `fmt.Errorf` is BANNED.
- License header (3 lines) at the top of any modified file is already present — no new file means no new header.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` is green before AND after.
</constraints>

<verification>

Run from repo root unless noted.

```bash
# Build + tests + coverage
cd agent/github-releaser && make precommit                              # exit 0
cd agent/github-releaser && go test -cover ./pkg/changelog/...          # ≥ 90%

# Signature frozen
grep -c '^func RewriteUnreleasedHeader(' agent/github-releaser/pkg/changelog/changelog.go    # =1

# DescribeTable entries present
grep -c '"rewrite unreleased' agent/github-releaser/pkg/changelog/changelog_test.go          # ≥3

# Error wrapping convention
grep -c 'fmt.Errorf' agent/github-releaser/pkg/changelog/changelog.go                        # =0

# Error message substring callers depend on
grep -c 'unreleased header not found' agent/github-releaser/pkg/changelog/changelog.go       # ≥1
```

</verification>
