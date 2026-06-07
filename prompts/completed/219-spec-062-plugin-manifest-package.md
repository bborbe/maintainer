---
status: completed
spec: [062-plugin-version-bump]
container: maintainer-plugin-version-bump-exec-219-spec-056-plugin-manifest-package
dark-factory-version: v0.173.0
created: "2026-06-01T00:00:00Z"
queued: "2026-05-31T22:32:08Z"
started: "2026-05-31T22:32:09Z"
completed: "2026-05-31T23:14:15Z"
branch: feature/plugin-version-bump
---

<summary>
- New pure-function Go sub-package at `agent/github-releaser/pkg/plugin/` (mirrors `pkg/changelog/`)
- `DetectManifests(ctx, workdir)` returns the subset of `[".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"]` that exist as regular files at the repo root
- `BumpPluginJson(ctx, content, version)` rewrites only the top-level `"version"` field, preserving all other bytes
- `BumpMarketplaceJson(ctx, content, version)` rewrites `metadata.version` and every `plugins[].version`, preserving all other bytes
- Both bump functions return an error wrapped via `github.com/bborbe/errors` when input is malformed or version field is absent/non-semver
- Package has no I/O, no globals beyond compiled regex, ctx threaded only for error wrapping consistency
</summary>

<objective>
Create the pure-function plugin manifest sub-package that owns detect-and-bump byte transform for `.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json`. The package mirrors the shape of `pkg/changelog/`: pure-function, deterministic, byte-in/byte-out, no I/O, no globals beyond compiled regex tables, ctx threaded only for error wrapping consistency. Error wrapping uses `github.com/bborbe/errors` only; never `fmt.Errorf`; every wrap takes `ctx`.
</objective>

<context>
Read these files before implementing:
- `/workspace/agent/github-releaser/pkg/changelog/changelog.go` — the reference shape to mirror (pure-function, bufio.Scanner line-by-line scan, bytes.Buffer output, bborbe/errors wrapping)
- `/workspace/agent/github-releaser/pkg/git/error_classifier.go` lines 1-60 — the ErrorCategory pattern (closed enum, typed constants, two-layer-classification comment style)
- `/workspace/agent/github-releaser/pkg/changelog/changelog_test.go` — test patterns (Ginkgo v2 + Gomega, DescribeTable/Entry, golden fixture assertions)
- `/workspace/agent/github-releaser/pkg/changelog/suite_test.go` — suite file pattern

The plugin manifests have this shape:

**`.claude-plugin/plugin.json`**:
```json
{
  "name": "example",
  "version": "0.9.12",
  ...
}
```

**`.claude-plugin/marketplace.json`**:
```json
{
  "metadata": {
    "version": "0.9.12"
  },
  "plugins": [
    {"name": "plugin-a", "version": "0.9.12"},
    {"name": "plugin-b", "version": "0.9.12"}
  ]
}
```

Known manifest paths (repo-relative, always at repo root):
- `.claude-plugin/plugin.json`
- `.claude-plugin/marketplace.json`
</context>

<requirements>
1. **Create directory**: `mkdir -p /workspace/agent/github-releaser/pkg/plugin/`

2. **Create `/workspace/agent/github-releaser/pkg/plugin/manifest.go`** with:
   - Package doc: `// Package plugin provides pure-Go functions for detecting Claude Code plugin manifests and bumping their version fields.`
   - Package-level compiled regex: `var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)`
   - `DetectManifests(ctx context.Context, workdir string) ([]string, error)`:
     - Checks existence of `.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json` using `os.Stat` (not `os.Open`)
     - Returns a slice of repo-relative paths that exist as regular files
     - Returns empty nil slice when neither exists
     - Returns `nil` error for absence (existence detection is not an error condition)
     - Wraps errors with `errors.Wrapf(ctx, err, "detect manifests in %s", workdir)`
   - `BumpPluginJson(ctx context.Context, content []byte, version string) ([]byte, error)`:
     - **Validate the `version` PARAMETER FIRST** against `semverRE`; if it doesn't match, return `errors.Errorf(ctx, "plugin.json bump rejected: version parameter %q is not a semver-shaped string", version)`. This is the boundary check that prevents callers from writing `v0.10.0` (with prefix) or `0.10.0-rc1` into the file.
     - Scans line-by-line with `bufio.Scanner` (same scanner discipline as `pkg/changelog/`)
     - Locates the top-level `"version"` key (a line whose unquoted form matches `^\s*"version"\s*:`)
     - Replaces the VALUE portion of that line (after `": "`, up to `,` or `}`) with the quoted version string
     - All other bytes are byte-identical to input (same indentation, same key order, same trailing newline)
     - Returns `errors.Wrapf(ctx, err, "bump plugin.json")` on scanner error
     - Returns `errors.Errorf(ctx, "plugin.json version field not found")` when no `"version"` line is found
     - Returns `errors.Errorf(ctx, "plugin.json existing version field is not a semver-shaped string: %q", value)` when the value already in the file is not quoted semver (must match `^\d+\.\d+\.\d+$` after stripping quotes)
     - Preserves trailing newline: if input did not end with `\n`, the output does not either
   - `BumpMarketplaceJson(ctx context.Context, content []byte, version string) ([]byte, error)`:
     - **Validate the `version` PARAMETER FIRST** against `semverRE`; if it doesn't match, return `errors.Errorf(ctx, "marketplace.json bump rejected: version parameter %q is not a semver-shaped string", version)`. Same boundary check as `BumpPluginJson`.
     - Line-by-line scanner pattern (same discipline as BumpPluginJson)
     - Rewrites `metadata.version` (the value after `"metadata": {` and `"version":`)
     - Rewrites every `plugins[].version` (value of `"version"` after an entry's `{` opens)
     - Tracks JSON brace depth and the most-recently-opened parent key to know which `"version"` is in scope (depth++ on `{`, depth-- on `}`; remember whether the current object's parent was `"metadata"` or under `"plugins"`). Any `"version"` whose lexical context is **not** directly inside the `metadata` object OR directly inside an entry of the `plugins` array is left untouched (e.g. a `"version"` nested deeper under `"plugins[].metadata.version"` is out of scope).
     - All other bytes byte-identical to input
     - Returns `errors.Wrapf(ctx, err, "bump marketplace.json")` on scanner error
     - Returns `errors.Errorf(ctx, "marketplace.json version field not found")` when no version line is found
     - Returns `errors.Errorf(ctx, "marketplace.json existing metadata.version is not a semver-shaped string: %q", value)` when the existing metadata version fails
     - Returns `errors.Errorf(ctx, "marketplace.json existing plugins[%d].version is not a semver-shaped string: %q", idx, value)` when an existing plugin version fails
     - Preserves trailing newline same as BumpPluginJson

3. **Create `/workspace/agent/github-releaser/pkg/plugin/manifest_test.go`** with:
   - `package plugin_test` (external test package)
   - Ginkgo v2 test suite using `DescribeTable`/`Entry` for all matrix cases
   - Suite file boilerplate matching `pkg/changelog/suite_test.go`

4. **Tests for DetectManifests**:
   - `DescribeTable("DetectManifests"...)` with entries:
     - `Entry("neither exists → returns nil", workdir with no .claude-plugin/ dir, wants nil slice)`
     - `Entry("plugin.json only → returns [plugin.json]", workdir with only plugin.json, wants ["plugin.json"])`
     - `Entry("marketplace.json only → returns [marketplace.json]", workdir with only marketplace.json, wants ["marketplace.json])`
     - `Entry("both exist → returns both", workdir with both, wants ["plugin.json", "marketplace.json"])`
   - Create temp workdirs with `os.MkdirAll` and `os.WriteFile` inside BeforeEach

5. **Tests for BumpPluginJson** — split into TWO tables:

   **5a. Input-version-parameter boundary table** (the regex this package exists to guard — must reject prefixed / suffixed forms BEFORE touching file content):
   - `DescribeTable("BumpPluginJson version-parameter boundary", ...)` with valid `plugin.json` fixture as input bytes; `Entry`:
     - `"0.10.0"` → no error (happy)
     - `"1.2.8"` → no error
     - `"v0.10.0"` → error, message contains `"version parameter"` (the leading `v` is the most common mistake; this is the boundary the regex exists to enforce)
     - `"0.10.0-rc1"` → error
     - `"0.10"` → error
     - `"latest"` → error
     - `""` → error
     - `"00.10.0"` → no error (`^\d+\.\d+\.\d+$` accepts leading zeros — explicitly OK; matches semver-string shape, not strict semver)

   **5b. File-content table** (existing happy/sad cases):
     - Happy path: input with `"version": "0.9.12"` + version `"0.10.0"` → output identical except `"0.10.0"` on that line. Assert output line count equals input line count and `Expect(rewritten).To(Equal(expected))` against a checked-in golden byte fixture.
     - `"version"` line not found in file → `err != nil`, error message matches `MatchRegexp("version.*not found")`
     - File's existing version value is non-semver (e.g. `"latest"`) → `err != nil`, error contains "existing version"
     - Malformed JSON (e.g. `"version": ` line with no value at all) → `err != nil`, error matches `MatchRegexp("(not found|not a semver)")` (accepts either interpretation — the line either fails the value-extraction regex OR is treated as missing)
     - Empty content → `err != nil`, error contains "not found"
     - Trailing newline preserved when input ends with `\n`; trailing newline absent when input does not — assert both

6. **Tests for BumpMarketplaceJson** — also split into TWO tables:

   **6a. Input-version-parameter boundary table** — same set as 5a: `0.10.0` / `1.2.8` valid; `v0.10.0` / `0.10.0-rc1` / `0.10` / `latest` / `""` all error with message containing `"version parameter"`.

   **6b. File-content table** (existing happy/sad cases):
     - N=0 plugins: bumps `metadata.version` only, exactly 1 line changed
     - N=1 plugin: bumps both, exactly 2 lines changed
     - N=3 plugins: bumps all, exactly 4 lines changed
     - `metadata.version` not found → `err != nil`
     - `plugins[].version` non-semver → `err != nil`, error contains index
     - Malformed → `err != nil`
     - Empty content → `err != nil`

7. **Do NOT add a `//go:generate` counterfeiter directive** — this package has no interface to mock and the directive would be a no-op that pollutes `go generate ./...`. Counterfeiter is added only when a package introduces an interface to mock.

8. **Create `/workspace/agent/github-releaser/pkg/plugin/suite_test.go`** matching the pattern from `pkg/changelog/suite_test.go`:
   ```go
   package plugin_test

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Plugin Suite", suiteConfig, reporterConfig)
   }
   ```
</requirements>

<constraints>
- Mirror `pkg/changelog/` shape exactly — pure-function, no I/O in transform ops, ctx for error wrapping
- Never `fmt.Errorf` — use `errors.Errorf` or `errors.Wrapf` with `ctx`
- Never `context.Background()` in package code
- Bump functions must preserve byte-level structure (indentation, key order, trailing newline)
- Never decode/encode full JSON — use line-by-line scanner
- Ginkgo `DescribeTable`/`Entry` for all matrix cases; never stdlib `t.Run` tables
- External test package (`package plugin_test`)
</constraints>

<verification>
From `agent/github-releaser/`, run:
```
make precommit
```
This runs `go test ./pkg/plugin/...` which executes the Ginkgo suite. All tests must pass.
</verification>