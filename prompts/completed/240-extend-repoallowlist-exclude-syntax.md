---
status: completed
spec: [070-manifest-complexity-refactor]
summary: Extended lib/repoallowlist to recognize !-prefix as exclude marker; IsAllowed and Validate now apply set-theoretic matching; all 50 Ginkgo specs pass (35 existing + 15 new); coverage 93.9%; gofmt/vet/golangci-lint clean
container: maintainer-allowlist-exclude-exec-240-extend-repoallowlist-exclude-syntax
dark-factory-version: v0.175.0
created: "2026-06-03T18:26:56Z"
queued: "2026-06-03T18:26:56Z"
started: "2026-06-03T18:26:58Z"
completed: "2026-06-03T18:33:40Z"
branch: dark-factory/extend-repoallowlist-exclusion-syntax
---

<objective>
Extend the `lib/repoallowlist` parser and matching engine to recognize a leading `!` (immediately, no whitespace) on an entry as an **exclusion** marker. The matching rule becomes set-theoretic: a target is allowed iff `(includes is empty OR any include matches the target) AND (no exclude matches the target)`. Excludes always override includes. An exclude-only allowlist means "allow everything except the excluded entries" (allow-all-except, not deny-all). The public API (`IsAllowed(allowlist []string, target string) bool`, `Validate(ctx context.Context, allowlist []string) error`) keeps frozen signatures, frozen allow-all-on-empty / empty-target-on-non-empty-list semantics, and frozen malformed-entry skip-with-WARN behavior. Every existing test case in `lib/repoallowlist/repoallowlist_test.go` continues to pass unmodified.
</objective>

<context>
Read `CLAUDE.md` for repo conventions.

Read these files BEFORE editing:
- `/workspace/specs/in-progress/061-extend-repoallowlist-exclusion-syntax.md` — the spec under implementation. § Desired Behavior, § Constraints, § Failure Modes, § Acceptance Criteria (1-12, 16, 17), § Verification are the load-bearing references. The spec's lock-in rule: a target is allowed iff `(includes empty OR any include matches target) AND (no exclude matches target)`. Order independence is part of the contract.
- `/workspace/lib/repoallowlist/repoallowlist.go` — current library. Public functions: `IsAllowed(allowlist []string, target string) bool` and `Validate(ctx context.Context, allowlist []string) error`. Internal helpers: `classifyKind(entry string) (kind string, reason string)` returns `"literal"` / `"wildcard"` and a reason on malformed. `matchWildcard(entry, target string) bool` does the wildcard matching. The malformed-entry skip path uses `glog.Errorf` with the existing format `repoallowlist: malformed entry %q: %s`. The `Validate` aggregate uses `errors.Errorf(ctx, ...)` and `errors.Join(errs...)` from `github.com/bborbe/errors`.
- `/workspace/lib/repoallowlist/repoallowlist_test.go` — current Ginkgo test suite. Two `DescribeTable` blocks (`IsAllowed` and `Validate`) and one `It` block in `Validate` for the aggregate-message case. All 30+ existing `Entry` rows and the aggregate `It` MUST continue to pass UNMODIFIED. Additive only — no deletions, no modifications of existing rows. New rows go in the same `DescribeTable` blocks.
- `/workspace/watcher/github-pr/pkg/filter/repo_allowlist_filter.go` — consumer that delegates to `repoallowlist.IsAllowed`. Reads as evidence of the delegation pattern (the library is the single source of truth for matching; consumers do not parse entries themselves). DO NOT EDIT.
- `/workspace/watcher/github-pr/main.go` line 225, `/workspace/watcher/github-build/main.go` line 87, `/workspace/agent/pr-reviewer/main.go` line 131, `/workspace/agent/pr-reviewer/cmd/run-task/main.go` line 90, `/workspace/agent/pr-reviewer/pkg/steps_checkout_execution.go` line 200 — additional `repoallowlist.IsAllowed` / `repoallowlist.Validate` callsites. All four real consumers (the spec mentions a fifth `agent/github-releaser` that does not yet exist in the tree) call into the library with zero wrapping. The library change is the only surface that needs editing in this prompt.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — interface→constructor→struct, error wrapping, package doc.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 / Gomega conventions; `DescribeTable` with `Entry` rows.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` API; never `fmt.Errorf`; never `context.Background()` in pkg/.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md` — `glog.Errorf` for malformed-entry skip; V(n) gating for any new Info-level logs.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — funlen 80, nestif 4, golines 100; banned packages; errcheck; license headers.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — coverage rules (≥ 80% on changed packages; modified code: test all changed/added paths).

Verified symbols (from module source — grep-confirmed):
- `errors.Errorf(ctx context.Context, format string, args ...interface{}) error` from `github.com/bborbe/errors@v1.5.13` — used in current `Validate`. The same helper wraps exclude-entry errors in this prompt.
- `errors.Join(errs ...error) error` from `github.com/bborbe/errors@v1.5.13` (`errors_join.go:7`) — used in current `Validate` for the aggregate. The same helper aggregates exclude-entry errors in this prompt. The returned error's `Error()` is `[\n` + each err + `\n]` + `]` (newlines + brackets), which the existing `It("Validate returns aggregate error mentioning each malformed entry", ...)` already relies on.
- `glog.Errorf(format string, args ...interface{})` from `github.com/golang/glog@v1.2.5` — used in current `IsAllowed` for malformed-entry skip. Same log format string `repoallowlist: malformed entry %q: %s` is preserved for both include and exclude paths.
- `lib/repoallowlist` is imported as `repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"` by the consumers — no import path change in this prompt.
- `func IsAllowed(allowlist []string, target string) bool` and `func Validate(ctx context.Context, allowlist []string) error` — FROZEN signatures, FROZEN return types. No ctx parameter on `IsAllowed` (the malformed-entry path discards via `glog`).
</context>

<requirements>

1. **Parser change — recognize `!`-prefix as exclude.** In `/workspace/lib/repoallowlist/repoallowlist.go`, change the entry-classification path so a leading `!` (immediately after `strings.TrimSpace`, with no whitespace between `!` and the entry body) marks the entry as an **exclude**; all other entries remain **includes**. The current loop at lines 32-52 trims and calls `classifyKind(entry)` once. The new path:

   a. After `entry = strings.TrimSpace(entry)` and the empty-entry `continue`, examine the first byte of `entry`. If `entry[0] == '!'`, strip the leading `!` to get `body := entry[1:]` and set an `isExclude bool` flag. Whitespace at the start of the original entry is already trimmed by step 33, so the `!` check operates on the trimmed string.

   b. The well-formedness check runs on `body` (the post-`!` portion), NOT on the original `entry`. So `classifyKind(body)` is called for both include and exclude entries. If the body is empty (`entry == "!"` or `entry == "!  "` which trims to `!` then strips to `""`), the malformed-entry path logs the original `entry` (with the `!`) and the reason `entry "!": must have exactly 3 path segments (host/owner/repo)`, and skips.

   c. After classification, the matching phase is split: includes feed the existing "any match?" loop, excludes feed a separate "any exclude match?" loop. The final `IsAllowed` return is `(includeEmpty || includeMatched) && !excludeMatched`. The existing return-false-when-target-empty and return-true-when-allowlist-empty short-circuits are preserved BEFORE the loop runs.

   Suggested internal structure (the executor may refactor for clarity; the behavioral contract is fixed):

   ```go
   func IsAllowed(allowlist []string, target string) bool {
       if len(allowlist) == 0 {
           return true
       }
       if target == "" {
           return false
       }
       var includes []string
       var excludes []string
       for _, entry := range allowlist {
           entry = strings.TrimSpace(entry)
           if entry == "" {
               continue
           }
           original := entry // capture before stripping for log fidelity
           isExclude := false
           if entry[0] == '!' {
               isExclude = true
               entry = entry[1:]
           }
           if entry == "" {
               // "!" or "!  " — empty body after stripping
               glog.Errorf("repoallowlist: malformed entry %q: must have exactly 3 path segments (host/owner/repo)", original)
               continue
           }
           kind, reason := classifyKind(entry)
           if reason != "" {
               glog.Errorf("repoallowlist: malformed entry %q: %s", original, reason)
               continue
           }
           if isExclude {
               excludes = append(excludes, entry)
           } else {
               includes = append(includes, entry)
           }
       }
       includeMatched := false
       for _, e := range includes {
           if matchesEntry(e, target) {
               includeMatched = true
               break
           }
       }
       excludeMatched := false
       for _, e := range excludes {
           if matchesEntry(e, target) {
               excludeMatched = true
               break
           }
       }
       return (len(includes) == 0 || includeMatched) && !excludeMatched
   }
   ```

   The malformed-entry log MUST use the ORIGINAL entry string (with the `!` prefix for excludes) so the operator's input value is named verbatim — log the entry AS THE OPERATOR WROTE IT, not the stripped body. Capture the original string in a local variable before any stripping. Acceptance: every new Ginkgo `Entry` row whose input contains `!` and is well-formed produces a `glog` log line that names the original `!`-prefixed entry on malformed input.

   Introduce a small helper `matchesEntry(entry, target string) bool` that does the literal/wildcard match using the existing `classifyKind` + `matchWildcard` pair. Refactoring into a helper is REQUIRED — the two matching loops must share the same code path so order independence and exact-match semantics are guaranteed (no chance of an include loop doing one thing and an exclude loop doing another). The helper signature: `func matchesEntry(entry, target string) bool` — both arguments non-empty, `entry` already validated by `classifyKind`.

2. **Validate change — accept `!`-prefix entries.** In `/workspace/lib/repoallowlist/repoallowlist.go`, change the `Validate` function so:

   a. After `entry = strings.TrimSpace(entry)` and the empty-entry `continue`, examine the first byte. If `entry[0] == '!'`, strip the leading `!` to get the body. The well-formedness check runs on the body. The aggregated error message names the ORIGINAL `!`-prefixed entry (so `Validate(ctx, []string{"!github.com/bborbe"})` returns an error whose `Error()` contains the literal string `!github.com/bborbe` AND the substring `must have exactly 3 path segments`).

   b. The empty-body case (`"!"` or `"!  "` after trim) is malformed — body after strip is empty, `classifyKind("")` returns the existing "must have exactly 3 path segments" reason. The aggregated error names the original `!` entry (the body-empty case still produces a 3-segments error, not a custom "empty body" reason; the existing reason text is sufficient and matches the spec's AC 11 substring requirement).

   c. The double-bang case (`"!!host/owner/repo"`) is malformed: after strip, body is `!host/owner/repo`, `classifyKind` returns the existing "must have exactly 3 path segments" reason (4 segments). The aggregated error names the original `!!host/owner/repo` entry.

   d. `Validate` keeps returning aggregated errors via `errors.Join(errs...)` from `github.com/bborbe/errors`. The error string for each malformed entry uses the existing format `repoallowlist: malformed entry %q: %s` from `errors.Errorf(ctx, ...)`.

3. **Package doc — document the `!`-prefix syntax in the `IsAllowed` doc comment.** The current doc comment is at lines 16-24. Add three new lines to the doc comment, in the existing prose style (full sentences, behavior-focused), explaining:

   a. A leading `!` on an entry marks it as an exclusion. Example: `!github.com/bborbe/go-skeleton` excludes go-skeleton.

   b. A target is allowed iff `(includes is empty OR any include matches the target) AND (no exclude matches the target)`. Excludes always override includes.

   c. An exclude-only allowlist (no include entries) means "allow everything except the excluded entries" — the canonical allow-all-except case.

   Add a usage example near the bottom of the doc comment:

   ```go
   //   includes: github.com/bborbe/*
   //   excludes: !github.com/bborbe/go-skeleton
   //   → allows every bborbe repo except go-skeleton.
   ```

   Acceptance: `grep -n '!' lib/repoallowlist/repoallowlist.go` shows the new doc lines naming the `!`-prefix and the matching rule.

4. **Helper refactor — share the matching path.** Extract a `matchesEntry(entry, target string) bool` helper that classifies the entry (`classifyKind(entry)` is assumed to have already returned `""` reason — the helper does NOT re-validate) and dispatches to literal-equal or wildcard-host+owner-match. This refactor is required so the include loop and the exclude loop in `IsAllowed` use IDENTICAL matching logic — order independence is a load-bearing guarantee of the spec, and two parallel literal-or-wildcard if/else blocks in the two loops would be a footgun. The helper is package-private (lowercase) since only `IsAllowed` calls it. Place it immediately after `matchWildcard` at the bottom of the file.

5. **Additive tests — extend `repoallowlist_test.go` with the spec's 12 ACs.** The existing 30+ `Entry` rows and 1 `It` block stay UNTOUCHED. Add a new `Entry` for each spec AC (1-12) in the appropriate `DescribeTable`:

   a. **IsAllowed table — append these rows** (the existing rows stay first; append at the end of the entry list, separated by a blank line and a comment header like `// Exclude syntax (spec 061)`):

   - `IsAllowed([]string{"github.com/bborbe/*", "!github.com/bborbe/go-skeleton"}, "github.com/bborbe/go-skeleton", false)` — AC 1.
   - `IsAllowed([]string{"github.com/bborbe/*", "!github.com/bborbe/go-skeleton"}, "github.com/bborbe/maintainer", true)` — AC 2.
   - `IsAllowed([]string{"!github.com/bborbe/go-skeleton"}, "github.com/bborbe/maintainer", true)` — AC 3 (exclude-only is allow-all-except).
   - `IsAllowed([]string{"!github.com/bborbe/go-skeleton"}, "github.com/bborbe/go-skeleton", false)` — AC 4.
   - `IsAllowed([]string{"!github.com/bborbe/*"}, "github.com/bborbe/anything", false)` — AC 5 (wildcard exclude).
   - `IsAllowed([]string{"!github.com/bborbe/*"}, "github.com/other/anything", true)` — AC 6 (wildcard exclude does not over-reach).

   b. **Order independence — add a dedicated `It("order independence: include-then-exclude equals exclude-then-include", ...)`** in the `IsAllowed` `Describe` block (alongside the existing `DescribeTable`, NOT inside it). The `It` builds the two orderings, iterates over `["github.com/bborbe/go-skeleton", "github.com/bborbe/maintainer", "github.com/other/repo"]`, and asserts `IsAllowed(orderingA, target) == IsAllowed(orderingB, target)` for every target. This covers AC 7.

   c. **Malformed exclude skip — append these rows to the IsAllowed table** (also under the `// Exclude syntax (spec 061)` block):

   - `IsAllowed([]string{"!github.com/bborbe", "github.com/bborbe/*"}, "github.com/bborbe/maintainer", true)` — AC 8: malformed `!github.com/bborbe` (only 2 segments) is skipped; the remaining `github.com/bborbe/*` matches; the function returns `true`. The entry also asserts that a `glog` log line is emitted — see the new `It` block below.
   - `IsAllowed([]string{"!github.com/*/repo", "github.com/bborbe/maintainer"}, "github.com/bborbe/maintainer", true)` — malformed `!github.com/*/repo` (wildcard in owner) is skipped; the literal include matches; returns `true`.
   - `IsAllowed([]string{"!*/bborbe/repo", "github.com/bborbe/maintainer"}, "github.com/bborbe/maintainer", true)` — malformed `!*/bborbe/repo` (wildcard in host) is skipped; the literal include matches; returns `true`.

   The malformed-skip log line (spec AC 8) is operator-visible only and not asserted in the test — the load-bearing contract for AC 8 is that the function returns the correct decision based on the remaining well-formed entries (`IsAllowed` returns `true` because the surviving include matches). Do NOT add a glog-capture `It` block; the three new malformed-skip `Entry` rows above (under `// Exclude syntax (spec 061)`) cover the function-return assertion fully. The log line is documented in the code path (the `glog.Errorf` call) and verified at integration time, not in unit tests.

   d. **Validate table — append these rows** (under a `// Exclude syntax (spec 061)` comment block, keeping existing rows first):

   - `Validate(ctx, []string{"!github.com/bborbe/go-skeleton"}, false)` — AC 9 (no error).
   - `Validate(ctx, []string{"!github.com/bborbe"}, true)` — AC 10 (error; `It` companion below asserts the substring).
   - `Validate(ctx, []string{"!"}, true)` — AC 11 (error).

   e. **Validate aggregate error message — add a dedicated `It("Validate aggregate error for malformed exclude entries names the entry and reason", ...)`** (alongside the existing aggregate-error `It`). The `It` calls `Validate(ctx, []string{"!github.com/bborbe"})` and asserts:
   - `err != nil`.
   - `err.Error()` contains the substring `!github.com/bborbe` (the original entry, not the stripped body).
   - `err.Error()` contains the substring `must have exactly 3 path segments`.

   This locks the spec's AC 10 evidence.

   f. **Existing rows pass unmodified — `git diff lib/repoallowlist/repoallowlist_test.go` shows ONLY additions, no deletions or modifications of existing `Entry` rows.** Verify with `git diff` after editing: the diff must be purely additive in the test file.

6. **No new dependencies.** The `import` block of `repoallowlist.go` stays exactly as it is (lines 7-14: `context`, `fmt`, `strings`, `github.com/bborbe/errors`, `github.com/golang/glog`). The `bborbe/errors` `Errorf` and `Join` are already imported; the new `Validate` exclude path uses them. No new imports.

7. **No new logging, no new error returns.** The malformed-exclude log uses the EXISTING `glog.Errorf("repoallowlist: malformed entry %q: %s", ...)` call — no new log format, no new log level, no `glog.V(n)` Info-level line. The `IsAllowed` signature is FROZEN — no new error return, no ctx parameter.

8. **License header — preserve the existing header.** The file's first 4 lines are the BSD-style license header (lines 1-3 plus the blank line at 4). Keep verbatim. Do not add a new header to `repoallowlist_test.go` — the existing header (lines 1-3 + blank at 4) stays.

9. **Acceptance gate — `make precommit` exits 0 in `lib/repoallowlist/`.** After all edits:

   ```
   cd /workspace/lib/repoallowlist && make precommit
   ```

   Expected: exit code 0. The precommit runs format + generate + test + lint + gosec + license; it MUST pass. Investigate and fix any failures. The spec's AC 16 names this command explicitly.

   Also run `cd /workspace/lib/repoallowlist && go vet ./...` and confirm exit code 0 (spec AC 17). The vet check is covered by precommit but called out for explicitness.

10. **Test coverage on `lib/repoallowlist` reaches ≥ 80% on statements.** Spec-inherited constraint from the project's `definition-of-done.md`. Run `cd /workspace/lib && go test -coverprofile=/tmp/cover.out -mod=vendor ./repoallowlist/... && go tool cover -func=/tmp/cover.out` and confirm `total: ≥ 80.0%`. The new exclude paths (parser + matching + Validate) are added code; they MUST have coverage. Investigate and fix any uncovered lines.

11. **Cross-prompt dependency declaration.** This prompt is the first in the spec 061 chain. Prompt 2 (the docs ripple prompt) depends on the library having shipped — its README examples and CHANGELOG entry name the new `!`-prefix syntax. If this prompt fails to ship, prompt 2's verification will fail and the auditor will flag the cross-prompt regression. This prompt does NOT touch any consumer code under `watcher/` or `agent/` (other than the test file in `lib/repoallowlist/`) — the spec locks consumer code as unchanged, and prompt 2 is the only place that proves the no-diff contract via the spec's grep commands.
</requirements>

<constraints>
- `IsAllowed` and `Validate` signatures are FROZEN: `func IsAllowed(allowlist []string, target string) bool` and `func Validate(ctx context.Context, allowlist []string) error`. No new parameter, no new return value, no error return added to `IsAllowed`.
- The allow-all-on-empty / empty-target-on-non-empty-list short-circuits are FROZEN — they run BEFORE the entry-classification loop, exactly as today. Do not move them inside the loop.
- Malformed-entry behavior at `IsAllowed` time is FROZEN — log via `glog.Errorf` and skip, do NOT return an error. The same skip-with-WARN semantics apply to malformed EXCLUDE entries.
- The malformed-entry log format is FROZEN — `glog.Errorf("repoallowlist: malformed entry %q: %s", originalEntry, reason)`. The first arg is the ORIGINAL entry string (with the `!` prefix for excludes), not the post-strip body. Operators see what they wrote, not what the parser saw after stripping.
- `Validate` returns aggregated errors via `errors.Join(errs...)` from `github.com/bborbe/errors`. The aggregate format is `[\n` + each `Error()` + `\n]` + `]`. The existing `It("Validate returns aggregate error mentioning each malformed entry", ...)` already locks this format; do not change it.
- `classifyKind` is the single source of truth for "is this entry well-formed and is it literal or wildcard". The new exclude path reuses `classifyKind` on the post-`!` body, NOT a parallel validator. The double-bang case (`!!host/owner/repo`) falls out naturally: post-strip body has 4 segments, `classifyKind` returns the existing "must have exactly 3 path segments" reason.
- The new `matchesEntry` helper does NOT re-validate the entry — it ASSUMES the entry has already been classified by `classifyKind` with an empty reason. Validation lives in the parser path; the matching path is pure dispatch.
- No new dependencies on packages outside the current import set of `repoallowlist.go` (`context`, `fmt`, `strings`, `github.com/bborbe/errors`, `github.com/golang/glog`).
- No new logging at Info or V(n) levels. The malformed-entry skip is the only log surface, and it stays at `glog.Errorf`.
- Every existing test case in `lib/repoallowlist/repoallowlist_test.go` continues to pass without modification. The test file diff is purely additive.
- The five consumer services (`watcher/github-release`, `watcher/github-pr`, `watcher/github-build`, `agent/pr-reviewer`, `agent/github-releaser`) are NOT edited in this prompt. The spec's lock-in: any diff under those paths in this PR is a spec violation. The library change is sufficient — verified by prompt 2's grep proof.
- Do NOT commit — dark-factory handles git.
- Do NOT touch any file under `watcher/` or `agent/` (other than reading them for context). Do NOT touch `CHANGELOG.md`, `README.md`, or any consumer README.
- Do NOT add a `REPO_BLOCKLIST` env var, a `TASK_SUFFIX` knob, or any other new configuration surface. The `!`-prefix in `REPO_ALLOWLIST` is the only new knob.
</constraints>

<verification>
```
cd /workspace/lib/repoallowlist && make precommit
```
Expected: exit code 0; lint passes (funlen 80, nestif 4, golines 100); all Ginkgo tests pass (the existing 30+ `Entry` rows and the aggregate-error `It` PLUS the new spec-061 rows for ACs 1-12); gosec clean; license headers valid; trivy clean.

```
cd /workspace/lib/repoallowlist && go vet ./...
```
Expected: exit code 0 (spec AC 17, called out for explicitness even though precommit covers it).

```
cd /workspace/lib && go test -coverprofile=/tmp/cover.out ./repoallowlist/... && go tool cover -func=/tmp/cover.out | tail -5
```
Expected: `total:` line shows ≥ 80.0% statement coverage on `lib/repoallowlist/...`. The new exclude paths (parser + matching + Validate) MUST be covered.

Evidence commands the auditor will run (spec 061 § AC 1-12, 16, 17):
- `cd /workspace/lib/repoallowlist && go test -v ./...` → all Ginkgo specs pass, including the new order-independence `It`, the malformed-exclude-skip cases, and the new Validate-aggregate-error `It` that asserts both the entry substring AND the reason substring.
- `grep -n 'must have exactly 3 path segments' /workspace/lib/repoallowlist/repoallowlist.go` → still present (the existing `classifyKind` reason is reused for exclude bodies).
- `grep -n '!' /workspace/lib/repoallowlist/repoallowlist.go` → shows the new doc lines naming the `!`-prefix and the matching rule, plus the parser code that handles the prefix.
- `git diff lib/repoallowlist/repoallowlist_test.go` → purely additive; existing `Entry` rows and `It` blocks unchanged.
- `git diff --stat lib/` → only `lib/repoallowlist/repoallowlist.go` and `lib/repoallowlist/repoallowlist_test.go` are touched; no other files under `lib/` are modified.
- `cd /workspace/lib/repoallowlist && make precommit` → exit code 0.
- `cd /workspace/lib/repoallowlist && go vet ./...` → exit code 0.
</verification>
