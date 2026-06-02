---
status: completed
spec: [058-manifest-complexity-refactor]
summary: Refactored BumpMarketplaceJson (gocognit 54→pass) and rewriteVersionValue (gocognit 29→pass) into scopeTracker, lineHasVersionKey, writeLine, extractExistingVersion, formatRewrittenVersion, locateVersionColon, parseVersionValuePart, parseQuotedVersionValue, parseUnquotedVersionValue — nolint count 0, all 64 tests pass, coverage 96.3% (>= 94.9% floor), public API frozen.
container: maintainer-refactor-complexity-exec-227-spec-058-manifest-complexity-refactor
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T00:00:00Z"
queued: "2026-06-02T20:07:16Z"
started: "2026-06-02T20:07:42Z"
completed: "2026-06-02T20:19:40Z"
branch: dark-factory/manifest-complexity-refactor
---

<summary>
- The two complexity-suppressed functions in `agent/github-releaser/pkg/plugin/manifest.go` get split into small helpers so the package passes `golangci-lint` cleanly without any `//nolint` directive.
- A `scopeTracker` struct carries the depth + flag state for the marketplace.json state machine, replacing four loose booleans threaded through a long loop body.
- A `lineHasVersionKey` package-level helper replaces the inline closure; a `writeLine` helper dedupes the `out.WriteString(line); out.WriteByte('\n')` pair.
- `rewriteVersionValue` is split into `extractExistingVersion` (parse) and `formatRewrittenVersion` (render) along the parse/render seam.
- Existing Ginkgo tests stay byte-identical for every covered input — only additive `Describe(...)` blocks are added in a new in-package test file (`manifest_helpers_test.go`, `package plugin`).
- The `//nolint:gocognit,gocyclo,funlen` count in `manifest.go` stays at 0; coverage on `pkg/plugin/...` stays at or above 94.9%.
- Public API of `BumpPluginJson`, `BumpMarketplaceJson`, and `rewriteVersionValue` is unchanged — `pkg/steps_execution.go` does not need to recompile to a new signature.
</summary>

<objective>
Refactor `agent/github-releaser/pkg/plugin/manifest.go` so that `BumpMarketplaceJson` and `rewriteVersionValue` fall under the project's complexity thresholds (`gocognit` ≤ 20, `funlen` ≤ 80 lines / 50 stmts, `nestif` ≤ 4) without any `//nolint` directive. Public signatures of `BumpPluginJson`, `BumpMarketplaceJson`, and `rewriteVersionValue` are frozen. Output is byte-for-byte identical to the current implementation for every input the existing test suite covers. Coverage on `pkg/plugin/...` stays ≥ 94.9% as measured by `go test -cover`.
</objective>

<context>
Read these files end-to-end before making changes:
- `/workspace/specs/in-progress/058-manifest-complexity-refactor.md` — the approved spec; non-goals + acceptance criteria are load-bearing.
- `/workspace/agent/github-releaser/pkg/plugin/manifest.go` — the only source file you will edit. 479 lines today.
- `/workspace/agent/github-releaser/pkg/plugin/manifest_test.go` — the regression net. Existing `It` blocks are FROZEN; you may NOT remove, rename, or rewrite any existing `It` block. The file stays in `package plugin_test` and continues to test the public API.
- `/workspace/agent/github-releaser/pkg/plugin/suite_test.go` — Ginkgo suite bootstrap; no changes.
- `/workspace/agent/github-releaser/pkg/steps_execution.go` — the (only) caller of `BumpPluginJson` and `BumpMarketplaceJson`. OUT OF SCOPE for this prompt. The signatures of those two functions must not change.
- `/workspace/agent/github-releaser/Makefile` — `make precommit` and `make test` entries.
- `/workspace/.golangci.yml` — linter thresholds (gocognit 20, gocyclo default 30, funlen 80 lines / 50 stmts, nestif 4).
- `/workspace/agent/github-releaser/go.mod` — module path `github.com/bborbe/maintainer/agent/github-releaser`.
- `/workspace/tools.env` — `GOLANGCI_LINT_VERSION ?= v2.11.4`; the linter is run via `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(grep '^GOLANGCI_LINT_VERSION' /workspace/tools.env | cut -d= -f2)`.

Reference docs (read before writing):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-linting-guide.md` — linter rules + complexity limits + `//nolint` policy.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega patterns; counterfeiter is NOT needed for this refactor (no interfaces introduced).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-architecture-patterns.md` — interface → constructor → struct → method.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — coverage rules (≥ 80% on new code; existing baseline 94.9% is the verifier's threshold here).

Concrete linter output today (proves which functions are over the threshold):
```
agent/github-releaser/pkg/plugin/manifest.go:119:1: cognitive complexity 54 of func `BumpMarketplaceJson` is high (> 20) (gocognit)
agent/github-releaser/pkg/plugin/manifest.go:312:1: cognitive complexity 29 of func `rewriteVersionValue` is high (> 20) (gocognit)
agent/github-releaser/pkg/plugin/manifest.go:44:2: Consider pre-allocating `result` (prealloc)               [out of scope]
agent/github-releaser/pkg/plugin/manifest.go:65:6: var-naming: func BumpPluginJson should be BumpPluginJSON (revive)   [out of scope]
```

Current helper inventory in `manifest.go` you may reuse as-is:
- `semverRE`, `pluginsArrayLineRE`, `isOpenScopeKeyRE` (package-level compiled regex)
- `isOpenScopeKey(trimmed string) bool`
- `extractScopeKey(trimmed string) string`
- `isVersionKeyLine(line string) bool`
- `countOpenBraces(line string) int`, `countCloseBraces(line string) int`
- `isCloseBrace(line string) bool`, `isCloseBracket(line string) bool`
- `trimLine(line string) string`, `getIndent(line string) string`
- `rewriteVersionValue(ctx, line, version, fileType) (string, error)` — frozen signature, may be split into smaller helpers but its caller-visible contract is unchanged.

Existing patterns in the file you must follow:
- Errors wrapped with `bborbeerrors.Wrapf(ctx, err, "...")`, `bborbeerrors.Wrap(ctx, err, "...")`, `bborbeerrors.Errorf(ctx, "...")`, `bborbeerrors.New(ctx, "...")` — NEVER `fmt.Errorf`, NEVER `context.Background()` in `pkg/`.
- License header on every Go file (BSD / "Copyright (c) <YEAR> Benjamin Borbe") — preserved by `make precommit` `addlicense` step.
- Helpers are unexported (lowercase) unless they need to be exercised from outside the package.
</context>

<design-decisions>
Five concrete extractions — execute them in this order. Each helper is unexported, lives in `manifest.go`, and has its own `Describe("<IdentifierName>", ...)` block in the new in-package test file `manifest_helpers_test.go` (per the spec's acceptance criterion, the first argument of each `Describe` must be the literal Go identifier below).

**1. `scopeTracker` struct — encapsulates the marketplace state-machine state**

```go
// scopeTracker holds the depth + scope flags for the marketplace.json
// streaming state machine. Zero value is the initial state (depth 0,
// all scopes false). Mutations only happen through update() so the
// state graph lives in one place instead of as four loose booleans
// threaded through a long loop body.
type scopeTracker struct {
    depth          int
    inMetadata     bool
    inPlugin       bool
    inPluginsArray bool
}

// update advances the tracker for the next line of the stream. The
// caller still owns the bytes.Buffer — this method only mutates the
// tracker's depth + scope flags based on the current line.
func (s *scopeTracker) update(line, trimmed string)
```

Internally `update` performs, in this exact order — the order matters because flags set in step 5 depend on `oldDepth` captured before step 4 mutates `s.depth`:
1. Note `oldDepth := s.depth`.
2. If `oldDepth == 2 && isCloseBrace(line)` → `s.inMetadata = false; s.inPlugin = false`.
3. If `oldDepth >= 1 && pluginsArrayLineRE.MatchString(trimmed)` → `s.inPluginsArray = true`.
4. `s.depth = oldDepth + countOpenBraces(line) - countCloseBraces(line)`.
5. If depth increased AND `oldDepth == 1` AND `isOpenScopeKey(trimmed)` → read `scopeKey := extractScopeKey(trimmed)`; if `scopeKey == "metadata"` then `s.inMetadata = true; s.inPlugin = false; s.inPluginsArray = false`.
6. If `s.inPluginsArray && strings.HasPrefix(trimmed, "{")` AND `oldDepth` is 0, 1, or 2 → `s.inPlugin = true; s.inMetadata = false`.
7. If `oldDepth == 2 && isCloseBracket(line) && s.inPluginsArray` → `s.inPluginsArray = false; s.inPlugin = false`.
8. If `s.depth == 0` → clear all three flags.

Add an unexported accessor:

```go
// inVersionScope returns true when the tracker is inside the
// "metadata" object or inside a single plugin object — i.e. the
// scopes where a "version" key should be rewritten.
func (s *scopeTracker) inVersionScope() bool { return s.inMetadata || s.inPlugin }
```

**2. `lineHasVersionKey(trimmed string) bool` — package-level helper**

Extract the closure currently defined inside `BumpMarketplaceJson` (lines 143–156 of the current `manifest.go`) into a package-level function with the same body verbatim. The closure captures nothing; promote it.

**3. `writeLine(buf *bytes.Buffer, line string)` — output helper**

```go
// writeLine appends line + a single '\n' to buf. Used by both
// BumpPluginJson and BumpMarketplaceJson to dedupe the
// out.WriteString(line); out.WriteByte('\n') pair that appears
// on every loop iteration.
func writeLine(buf *bytes.Buffer, line string) {
    buf.WriteString(line)
    buf.WriteByte('\n')
}
```

**4. `extractExistingVersion` + `5. `formatRewrittenVersion` — split `rewriteVersionValue`**

The current `rewriteVersionValue` is 100+ lines with two distinct concerns: parse the existing version out of the line (validate it's a semver), then assemble the rewritten line. Split it along that seam:

```go
// extractExistingVersion parses the existing value of a "version" key
// in `line` and returns the components rewriteVersionValue needs to
// rebuild the line: keyPart (everything up to and including the colon),
// the validated existing value, the trailing portion of the line
// after the closing quote (or after the unquoted value), and the
// line's leading indent. ok is false if the line is malformed; the
// returned error explains why.
func extractExistingVersion(
    ctx context.Context,
    line string,
    fileType string,
) (keyPart, value, trailing, indent string, err error)
```

```go
// formatRewrittenVersion rebuilds a "version" key line with the new
// version string, preserving the original line's indent, keyPart,
// and trailing content. Assumes the components were produced by
// extractExistingVersion and are internally consistent.
func formatRewrittenVersion(indent, keyPart, version, trailing string) string
```

`rewriteVersionValue` then becomes ~15 lines: locate the version key on the trimmed line (with a fallback for `"version" :` with a space), call `extractExistingVersion`, call `formatRewrittenVersion`, return. The function signature `func rewriteVersionValue(ctx context.Context, line, version, fileType string) (string, error)` is FROZEN — do not change parameter order, return types, or names. Do not change the error messages either (the existing tests assert on substrings like "not a semver-shaped string").

**Resulting `BumpMarketplaceJson` body (post-refactor sketch — for the executor's mental model, not a verbatim paste):**

```go
func BumpMarketplaceJson(ctx context.Context, content []byte, version string) ([]byte, error) {
    // 1. semverRE check (unchanged)
    // 2. len(content) == 0 check (unchanged)
    // 3. scanner over bytes.NewReader(content) (unchanged)
    // 4. var out bytes.Buffer; var tracker scopeTracker; foundAny := false
    // 5. for scanner.Scan() loop:
    //      line := scanner.Text()
    //      trimmed := trimLine(line)
    //      tracker.update(line, trimmed)
    //      if lineHasVersionKey(trimmed) && tracker.inVersionScope() {
    //          rewritten, err := rewriteVersionValue(ctx, line, version, "marketplace.json")
    //          if err != nil { return nil, err }
    //          writeLine(&out, rewritten)
    //          foundAny = true
    //          continue
    //      }
    //      writeLine(&out, line)
    // 6. scanner.Err() check (unchanged)
    // 7. !foundAny check (unchanged)
    // 8. trailing-newline preservation (unchanged)
}
```

**Resulting `BumpPluginJson` body (post-refactor sketch — only the dedup'd write calls change):**

The function is already under thresholds; the only change is replacing `out.WriteString(line); out.WriteByte('\n')` pairs with `writeLine(&out, line)`. The signature, the pre-check, the scanner, the trailing-newline preservation, and the `found` flag stay exactly as today.

**Why these splits:**
- `scopeTracker` moves the four scope flags + depth into one named, testable unit. Each of the eight internal branches becomes one test case in the tracker's `Describe` block.
- `lineHasVersionKey` is the simplest extraction — the closure is already a pure function over `trimmed`; promoting it costs nothing and makes it independently testable.
- `writeLine` is a 2-line dedup that does NOT change behavior; it exists only so the loop body is short enough for the funlen / gocognit linters to be happy.
- `extractExistingVersion` + `formatRewrittenVersion` split `rewriteVersionValue` along the parse / render seam. Each helper is < 30 lines; each can be tested without round-tripping through the full rewrite path.

**What you MUST NOT do:**
- Do NOT change the public signatures of `BumpPluginJson`, `BumpMarketplaceJson`, or `rewriteVersionValue`.
- Do NOT introduce any `//nolint` directive — not on the new helpers, not on the existing ones, not on the loop. The point of the refactor is that the code passes the linter without suppression.
- Do NOT add new lint rules, raise thresholds, or relax existing ones in `/workspace/.golangci.yml`.
- Do NOT switch to a JSON decode/encode round-trip — keep the line-based streaming implementation.
- Do NOT add the golines `//nolint` line-affinity config tweak.
- Do NOT add behavior toggles, feature flags, or compatibility shims.
- Do NOT touch `pkg/steps_execution.go` (out of scope per spec).
- Do NOT touch `DetectManifests` (out of scope; linter complains about `prealloc` and `revive var-naming` there but those are out of scope per spec Non-goals — leave them alone).
- Do NOT rename `BumpPluginJson` → `BumpPluginJSON` (the linter's `revive var-naming` complaint is out of scope per spec Non-goals — the spec explicitly forbids renaming exported functions).
- Do NOT add a `CHANGELOG.md` entry — the spec's Acceptance Criteria do not list changelog, and the spec's Constraints do not mention it.
</design-decisions>

<requirements>
1. **Read every file in `<context>` end-to-end before writing any code.** The spec is a refactor; missing a single helper (e.g. `extractScopeKey`, `isOpenScopeKey`) breaks the rewrite path. If you skip any referenced file, document the skip in `## Improvements` with category `PROMPT`.

2. **Add the new helpers to `manifest.go` exactly as specified in `<design-decisions>` sections 1–5.** Place order (top to bottom in the file):
   - `scopeTracker` struct + `update` method + `inVersionScope` accessor — immediately above `BumpMarketplaceJson`.
   - `lineHasVersionKey` — immediately below the existing `isVersionKeyLine` helper.
   - `writeLine` — immediately below `isCloseBracket`.
   - `extractExistingVersion` and `formatRewrittenVersion` — immediately above `rewriteVersionValue`. `rewriteVersionValue` itself stays where it is, with its body rewritten to call the two new helpers.

3. **Rewrite `BumpMarketplaceJson` body to use the new helpers** per the sketch in `<design-decisions>`. The function signature `func BumpMarketplaceJson(ctx context.Context, content []byte, version string) ([]byte, error)` is FROZEN. The exported doc comment above the function is preserved verbatim. The post-refactor body must be < 50 lines and have a `gocognit` score ≤ 20. After the change, run `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(grep '^GOLANGCI_LINT_VERSION' /workspace/tools.env | cut -d= -f2) run --config /workspace/.golangci.yml ./pkg/plugin/...` to verify the score.

4. **Update `BumpPluginJson` to use `writeLine`** (the only change to that function). Replace every `out.WriteString(...); out.WriteByte('\n')` pair with `writeLine(&out, ...)`. The signature, the pre-check, the scanner, the `found` flag, and the trailing-newline preservation stay exactly as today. This is a non-behavioral dedup.

5. **Create `pkg/plugin/manifest_helpers_test.go` in `package plugin`** (NOT `package plugin_test`). This new file exercises the unexported helpers directly. Required `Describe` blocks — the first argument must be the literal Go identifier below:
   - `Describe("scopeTracker", func() { ... })` — at least 6 `It` blocks covering: enter metadata via `"metadata": {` at depth 1, enter plugin via `{` inside `inPluginsArray` at depth 2, close plugin via `}` at depth 2, close plugins array via `]` at depth 2 with `inPluginsArray` true, full exit via `}` at depth 0, the `inVersionScope` predicate (true when only `inMetadata` is set; true when only `inPlugin` is set; false when both are false).
   - `Describe("lineHasVersionKey", func() { ... })` — at least 5 `It` blocks: matches `"version": "x"` after `{`, matches `"version" : "x"` with space before colon, does NOT match inside a string value like `"description": "version: x"`, matches after a comma, returns false on a line with no version.
   - `Describe("writeLine", func() { ... })` — at least 3 `It` blocks: writes line + `\n`, writes empty line as just `\n`, two consecutive calls produce `"a\nb\n"`.
   - `Describe("extractExistingVersion", func() { ... })` — at least 4 `It` blocks: parses quoted semver, parses unquoted semver, returns error on non-semver quoted value, returns error on missing colon.
   - `Describe("formatRewrittenVersion", func() { ... })` — at least 3 `It` blocks: formats quoted value with new version, formats unquoted value with new version, preserves trailing comma.

   Use the Ginkgo v2 + Gomega APIs already imported in the existing test file (`Describe`, `It`, `Expect`, `Equal`). The file must have the standard BSD license header (use the same header as `manifest.go`; the `addlicense` step in `make precommit` will rewrite it to the current year).

6. **Verify the regression net is unchanged.** Run `cd /workspace/agent/github-releaser && go test -count=1 ./pkg/plugin/...` — every pre-existing `It` block must pass. The ONLY allowed diff in `manifest_test.go` is no diff (you do not touch that file at all). If you accidentally edited it, revert and re-do the change.

7. **Verify the nolint count is zero.** `cd /workspace/agent/github-releaser && grep -c '//nolint:\(gocognit\|gocyclo\|funlen\)' pkg/plugin/manifest.go` must output exactly `0`. If you added a `//nolint` to silence a residual complaint, you have not finished — keep splitting.

8. **Verify the package passes the linter clean against the targets.** Run `cd /workspace/agent/github-releaser && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(grep '^GOLANGCI_LINT_VERSION' /workspace/tools.env | cut -d= -f2) run --config /workspace/.golangci.yml ./pkg/plugin/...` — exit 0, no `gocognit`, `funlen`, `nestif`, or `gocyclo` complaints against `BumpMarketplaceJson`, `rewriteVersionValue`, or any new helper. (Pre-existing `prealloc` on `DetectManifests` and `revive var-naming` on `BumpPluginJson` are out of scope per spec Non-goals; they are NOT introduced by this refactor and the spec forbids fixing them.)

9. **Verify race-free.** Run `cd /workspace/agent/github-releaser && go test -race -count=1 ./pkg/plugin/...` — exit 0. The new helpers are pure functions; this is a sanity check that no shared mutable state leaked into scope.

10. **Verify the coverage floor.** Run `cd /workspace/agent/github-releaser && go test -cover -count=1 ./pkg/plugin/...` — coverage ≥ 94.9%. The new `Describe` blocks in step 5 keep the floor intact (each new helper is fully exercised by its own `Describe` block).

11. **Verify the full precommit gate.** Run `cd /workspace/agent/github-releaser && make precommit` — exit 0. If `make precommit` fails, follow the fix-loop in your session instructions: fix the failing target individually (`make lint`, `make gosec`, etc.) per the session instructions, then re-run `make precommit` once at the end.

12. **No `git commit`, no `git push`** — your session does not own git.

13. **Final check before reporting success:** confirm `pkg/steps_execution.go` is byte-identical to the version you read at the start (`git diff` shows no changes, or simply verify the function call sites `plugin.BumpPluginJson(ctx, ...)` and `plugin.BumpMarketplaceJson(ctx, ...)` still compile against the new `manifest.go`). The signatures must not have shifted.
</requirements>

<constraints>
- Public API frozen: `BumpPluginJson`, `BumpMarketplaceJson`, `rewriteVersionValue` signatures are byte-for-byte identical. `pkg/steps_execution.go` compiles without modification.
- Byte-equivalence frozen: for every input covered by the existing test suite, output bytes are character-for-character identical to the pre-refactor implementation. The test suite is the regression net.
- Coverage floor frozen: `go test -cover ./pkg/plugin/...` ≥ 94.9%.
- Helper visibility: any new helpers are unexported. The new `Describe` blocks live in `manifest_helpers_test.go` in `package plugin` (NOT `package plugin_test`).
- `//nolint` forbidden: do not add `//nolint:gocognit`, `//nolint:gocyclo`, `//nolint:funlen`, or `//nolint:nestif` anywhere. The point of the refactor is that the code passes the linter clean.
- No JSON decode/encode round-trip: the implementation stays line-based streaming.
- No new lint rules, no relaxed thresholds in `/workspace/.golangci.yml`.
- No `git commit` / `git push` — your session does not own git. The management session will commit after the prompt completes.
- Spec 056 byte-structure invariants: indentation, key order, trailing newline preservation, one-line-per-field diff shape — all preserved.
- No CHANGELOG entry — the spec's Acceptance Criteria do not list changelog; the spec's Constraints do not mention it.
- The only files you may create or modify: `pkg/plugin/manifest.go`, `pkg/plugin/manifest_helpers_test.go` (new). Do NOT touch `pkg/plugin/manifest_test.go`, `pkg/plugin/suite_test.go`, `pkg/steps_execution.go`, or any other file in the module.
</constraints>

<verification>
Run these commands from `/workspace/agent/github-releaser` and report each exit code + first 5 lines of output:

```bash
# 1. No //nolint directives in manifest.go
grep -c '//nolint:\(gocognit\|gocyclo\|funlen\)' pkg/plugin/manifest.go
# expect: stdout literal "0"

# 2. Linter clean against the plugin package
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(grep '^GOLANGCI_LINT_VERSION' /workspace/tools.env | cut -d= -f2) run --config /workspace/.golangci.yml ./pkg/plugin/...
# expect: exit 0; no gocognit / funlen / nestif / gocyclo complaints on BumpMarketplaceJson, rewriteVersionValue, or any new helper
# (pre-existing prealloc + revive complaints on DetectManifests / BumpPluginJson are out of scope and acceptable)

# 3. Linter clean against the entire pkg/ tree
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(grep '^GOLANGCI_LINT_VERSION' /workspace/tools.env | cut -d= -f2) run --config /workspace/.golangci.yml ./pkg/...
# expect: exit 0

# 4. Tests pass
go test -count=1 ./pkg/plugin/...
# expect: exit 0; all pre-existing It blocks pass byte-for-byte

# 5. Tests pass under race detector
go test -race -count=1 ./pkg/plugin/...
# expect: exit 0

# 6. Coverage floor
go test -cover -count=1 ./pkg/plugin/...
# expect: coverage ≥ 94.9%

# 7. Full precommit gate
make precommit
# expect: exit 0
```

If `make precommit` reports a non-zero exit code, fix the failing target individually (`make lint`, `make gosec`, etc.) per your session instructions and re-run `make precommit` once at the end. Do not loop `make precommit`.

Each `Describe` block added in step 5 of `<requirements>` must be discoverable via:
```bash
grep -E 'Describe\("([A-Za-z_][A-Za-z0-9_]*)"' pkg/plugin/manifest_helpers_test.go | sort -u
# expect: at least one match per new helper identifier:
#   scopeTracker, lineHasVersionKey, writeLine, extractExistingVersion, formatRewrittenVersion
```

Sanity check: the public API is frozen.
```bash
grep -nE 'func (BumpPluginJson|BumpMarketplaceJson|rewriteVersionValue)\(' pkg/plugin/manifest.go
# expect: three lines, signatures unchanged
```
</verification>
