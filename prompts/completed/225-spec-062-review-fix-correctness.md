---
status: completed
spec: ["062"]
summary: Applied five correctness and code-quality fixes to spec-056 github-releaser implementation
container: maintainer-plugin-version-bump-exec-225-spec-056-review-fix-correctness
dark-factory-version: v0.173.0
created: "2026-06-01T00:00:00Z"
queued: "2026-06-01T19:06:59Z"
started: "2026-06-01T19:07:01Z"
completed: "2026-06-01T19:09:59Z"
branch: feature/plugin-version-bump
---

<summary>
- Manifest dispatch in the release-commit step becomes exhaustive: an unknown manifest path fails closed instead of writing an empty file.
- Unquoted-version rewrite preserves whatever trailing punctuation the original line had (comma or closing brace) — no more silently appending an extra comma when the version field is the last field of an object.
- Manifest path joining uses the standard library helper so darwin and linux tests behave the same way as production.
- Two inline regular expressions move to package-level vars so they compile once per process instead of once per call.
- Two empty `if` blocks in the test file (parking unused locals) are replaced with explicit discards so linters stay quiet.
- No public API changes, no new behavior, no CHANGELOG edits — pure code-quality follow-ups to the spec-056 review.
- All existing tests must still pass; `make precommit` in `agent/github-releaser` is the gate.
</summary>

<objective>
Apply five `/coding:pr-review` correctness and code-quality fixes against the already-shipped spec-056 implementation on branch `feature/plugin-version-bump`. No new behavior, no spec changes, no CHANGELOG churn.
</objective>

<context>
Read these guides:
- /home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md (errors package usage)
- /home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md
- /home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md

Read these source files before editing — every change is anchored by function name and content, not line number:
- `/workspace/agent/github-releaser/pkg/steps_execution.go` (focus on the manifest dispatch around `if strings.HasSuffix(manifestPath, "plugin.json") { ... } else if strings.HasSuffix(manifestPath, "marketplace.json") { ... }` inside `executeDirectPush`)
- `/workspace/agent/github-releaser/pkg/plugin/manifest.go` (focus on `DetectManifests`, `BumpMarketplaceJson`, `isOpenScopeKey`, `rewriteVersionValue`)
- `/workspace/agent/github-releaser/pkg/plugin/manifest_test.go` (focus on the two `if wantErr && got != nil {}` empty blocks — one inside `DescribeTable("BumpPluginJson version-parameter boundary", ...)`, one inside `DescribeTable("BumpMarketplaceJson version-parameter boundary", ...)`)

Already-imported packages in `pkg/steps_execution.go` include `github.com/bborbe/errors` (aliased per the file) and the `git` package providing `git.ErrorCategoryUnknown`. Confirm the exact error-wrapping idiom used elsewhere in that file (e.g. `errors.Wrapf(ctx, err, "write %s", ...)`, `errors.Errorf(ctx, "...", ...)`) by reading the surrounding `executeDirectPush` body before writing the new branch.
</context>

<requirements>

## 1. `steps_execution.go` — exhaustive manifest dispatch

In `executeDirectPush` (file `/workspace/agent/github-releaser/pkg/steps_execution.go`), the loop `for _, manifestPath := range detectedManifests { ... }` currently dispatches like this:

```go
var rewrittenManifest []byte
if strings.HasSuffix(manifestPath, "plugin.json") {
    rewrittenManifest, err = plugin.BumpPluginJson(ctx, manifestContent, unprefixedVersion)
} else if strings.HasSuffix(manifestPath, "marketplace.json") {
    rewrittenManifest, err = plugin.BumpMarketplaceJson(ctx, manifestContent, unprefixedVersion)
}
if err != nil { ... }
```

If `plugin.DetectManifests` is ever extended (e.g. a new manifest type added) without updating this dispatch, both branches are skipped: `rewrittenManifest` stays `nil` and the subsequent `os.WriteFile(manifestAbsPath, rewrittenManifest, 0o644)` silently truncates the file. Make the dispatch fail-closed.

Add a final `else` arm:

```go
} else {
    result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
        errors.Errorf(ctx, "unsupported manifest type: %s", manifestPath))
    return "", "", result
}
```

Anchor by content: locate the existing `else if strings.HasSuffix(manifestPath, "marketplace.json")` branch and append the `else` arm immediately after its closing `}`, BEFORE the existing `if err != nil { ... }` check.

Verify before saving: the file's existing error-wrapping calls use `errors.Wrapf(ctx, err, ...)` / `errors.Errorf(ctx, ...)` with the exact import alias used at the top of `steps_execution.go`. Match that alias verbatim.

## 2. `manifest.go` — preserve trailing punctuation in unquoted-value branch

In `/workspace/agent/github-releaser/pkg/plugin/manifest.go`, function `rewriteVersionValue`, the unquoted-value branch (the block following the comment `// Unquoted value` inside `if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') { ... } else { ... }`) currently ends:

```go
end := 0
for end < len(rest) && rest[end] != ',' && rest[end] != '}' && rest[end] != ' ' && rest[end] != '\t' {
    end++
}
valueStr := rest[:end]

if !semverRE.MatchString(valueStr) {
    return "", bborbeerrors.Errorf(ctx,
        fileType+" existing version field is not a semver-shaped string: %q", valueStr)
}

indent := getIndent(line)
keyPart := trimmed[:colonIdx+1]
return fmt.Sprintf("%s%s %s,", indent, keyPart, version), nil
```

The unconditional trailing `,` corrupts JSON when the version field is the last field of an object — input `"version": 0.9.12 }` becomes `"version": 0.10.0, }`. The quoted-value branch directly below already preserves trailing context via `trailing := rest[closeIdx+1:]`. Mirror that approach here.

Replace the final two statements of the unquoted branch (the `indent`/`keyPart`/`return fmt.Sprintf("%s%s %s,", ...)` portion) so that:

- `trailing` is computed as `rest[end:]` (everything after the parsed numeric value, including any whitespace, comma, or closing brace).
- The returned line is rebuilt as `fmt.Sprintf("%s%s %s%s", indent, keyPart, version, trailing)` — original punctuation preserved verbatim, no synthetic comma.

Note: `valueStr` and the `semverRE` validation step stay exactly as they are. Only the final formatted return changes.

## 3. `manifest.go` — use `filepath.Join` in `DetectManifests`

In `DetectManifests` (top of `/workspace/agent/github-releaser/pkg/plugin/manifest.go`), the loop body currently reads:

```go
for _, rel := range known {
    path := workdir + "/" + rel
    info, err := os.Stat(path)
    ...
}
```

Replace `path := workdir + "/" + rel` with `path := filepath.Join(workdir, rel)`.

Add `"path/filepath"` to the import block at the top of the file. The current imports are (verbatim from the file):

```go
import (
    "bufio"
    "bytes"
    "context"
    "fmt"
    "os"
    "regexp"
    "strings"

    bborbeerrors "github.com/bborbe/errors"
)
```

Add `"path/filepath"` in stdlib alphabetic position (between `"os"` and `"regexp"`).

## 4. `manifest.go` — promote two inline regexes to package-level vars

Below the existing `semverRE` package-level declaration:

```go
// Package-level compiled regex for semver-shaped string validation.
// Matches only the bare N.N.N pattern — no leading 'v', no suffix.
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
```

Add two new package-level vars with brief doc comments mirroring the `semverRE` style:

```go
// Package-level compiled regex for the "plugins": [ array-opening line.
var pluginsArrayLineRE = regexp.MustCompile(`^\s*"plugins"\s*:\s*\[`)

// Package-level compiled regex for a named-object scope opener such as `"metadata": {`.
var isOpenScopeKeyRE = regexp.MustCompile(`^\s*"[^"]+"\s*:\s*\{`)
```

Then remove the inline declarations:

- Inside `BumpMarketplaceJson`, delete the line:
  ```go
  pluginsArrayLineRE := regexp.MustCompile(`^\s*"plugins"\s*:\s*\[`)
  ```
  (and its surrounding comment `// Regex to detect "plugins": [ (plugins array opening line)`). The existing call `pluginsArrayLineRE.MatchString(trimmed)` later in the function continues to work because the name now resolves to the package-level var.

- In `isOpenScopeKey`, replace the body:
  ```go
  func isOpenScopeKey(trimmed string) bool {
      matched, _ := regexp.MatchString(`^\s*"[^"]+"\s*:\s*\{`, trimmed)
      return matched
  }
  ```
  with:
  ```go
  func isOpenScopeKey(trimmed string) bool {
      return isOpenScopeKeyRE.MatchString(trimmed)
  }
  ```

After both removals, verify `regexp` is still used in the file (it is — `semverRE` keeps it). Do not remove the `regexp` import.

## 5. `manifest_test.go` — remove **all four** empty `if` blocks

In `/workspace/agent/github-releaser/pkg/plugin/manifest_test.go`, four `DescribeTable` callbacks each contain an empty `if` parking statement to silence "declared but not used" on a `got` variable. All four appear at the tail of their respective callback bodies. There are two polarities:

- Two `if wantErr && got != nil {}` blocks (currently at approximately lines 75 and 167 — anchor by content, not line number)
- Two `if !wantErr && got == nil {}` blocks (currently at approximately lines 101 and 192)

Replace **every** empty-block pattern (and any preceding `// Prevent unused variable…` comment) with a single explicit discard line:

```go
_ = got
```

`_ = got` works for both polarities: it silences "declared but not used" regardless of whether `wantErr` is true or false. After the fix, the file contains exactly four `_ = got` lines (one per former empty block) and zero empty `if` blocks of either polarity.

Do not reorder the `DescribeTable` callback logic or change any `Entry(...)` rows.

</requirements>

<constraints>
- Use `github.com/bborbe/errors` only; never `fmt.Errorf`; always pass `ctx` to error constructors.
- Do not change any public function signatures (`DetectManifests`, `BumpPluginJson`, `BumpMarketplaceJson` keep their current signatures).
- Do not modify `CHANGELOG.md` — this is a fix iteration on existing Unreleased bullets, not a new feature.
- Do not commit — dark-factory handles git. Branch is already `feature/plugin-version-bump`; commits land there.
- All existing tests must still pass.
- Do not introduce new dependencies. The only new import is the stdlib `"path/filepath"` in `manifest.go`.
- Do not delete or rename existing tests; the empty `if` cleanup only swaps an empty block for `_ = got`.
</constraints>

<verification>
Run from `/workspace/agent/github-releaser`:

```
cd /workspace/agent/github-releaser && make precommit
```

`make precommit` must pass. The local environment may report one pre-existing failure in `pkg/git/os_exec_git_ops_test.go:222` originating from a user-side pre-push hook — that failure is unrelated to this prompt's scope and is not the responsibility of this fix.

Spot-check after the precommit:

1. `grep -n 'unsupported manifest type' pkg/steps_execution.go` — exactly one match in the new `else` arm.
2. `grep -n 'pluginsArrayLineRE = regexp.MustCompile' pkg/plugin/manifest.go` — exactly one match, at package scope (no inline match inside `BumpMarketplaceJson`).
3. `grep -n 'isOpenScopeKeyRE = regexp.MustCompile' pkg/plugin/manifest.go` — exactly one match, at package scope.
4. `grep -n 'filepath.Join(workdir, rel)' pkg/plugin/manifest.go` — exactly one match, inside `DetectManifests`.
5. `grep -nE '(if wantErr && got != nil|if !wantErr && got == nil) \{\s*\}' pkg/plugin/manifest_test.go` — zero matches (both polarities removed).
6. `grep -c '_ = got' pkg/plugin/manifest_test.go` — exactly `4` matches.
7. `grep -n 'fmt.Sprintf("%s%s %s,"' pkg/plugin/manifest.go` — zero matches (the unconditional trailing-comma format string is gone).
</verification>
