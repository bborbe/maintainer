---
status: rejected
spec: ["062"]
container: maintainer-plugin-version-bump-exec-226-spec-056-review-fix-coverage
dark-factory-version: v0.173.0
created: "2026-06-01T00:00:00Z"
queued: "2026-06-01T19:07:00Z"
started: "2026-06-01T19:10:00Z"
completed: "2026-06-01T19:13:32Z"
branch: feature/plugin-version-bump
lastFailReason: 'validate completion report: completion report status: partial'
---

<summary>
- Adds 10 missing test-coverage cases identified in PR review against spec 056.
- Tests cover edge cases for plugin-manifest detection (directory-as-file, I/O error, temp-dir cleanup).
- Tests cover edge cases for the version-line rewriter (unquoted values, unclosed quote, second `"version"` key, scope filter).
- Adds a `marketplace.json` malformed-JSON integration test mirroring the existing `plugin.json` malformed test.
- Adds documentation tests for `deriveUnprefixedVersion("")` and `sameStringSet` duplicate-element behavior.
- All additions go into existing test files; no new packages, no new files.
- Production code is NOT touched here — those changes are in sibling prompt 5 which runs first.
- Verification: `make precommit` from `agent/github-releaser/` exits 0 after both prompts apply.
</summary>

<objective>
Close the test-coverage gaps surfaced by the PR review of spec 056 (plugin-version-bump). Add Ginkgo `DescribeTable`/`Entry` rows and `It` blocks to the two existing test files so that every behavioral edge case in the manifest detection, version-line rewrite, and execution-step manifest dispatch is exercised. Production code changes (the `rewriteVersionValue` unquoted-branch fix and the marketplace.json malformed-JSON dispatch path) are owned by sibling prompt 5; this prompt only adds tests that pin those fixes in place.
</objective>

<context>
Read CLAUDE.md at the repo root for project conventions.

Read these files in full before adding tests — every assertion must align with the real production signatures and existing test idioms:

- `agent/github-releaser/pkg/plugin/manifest.go` — production code for `DetectManifests`, `BumpPluginJson`, `BumpMarketplaceJson`, `rewriteVersionValue`. Note that `rewriteVersionValue` has separate quoted and unquoted branches (`rest[0] == '"' || rest[0] == '\''` vs the else branch starting around the `// Unquoted value` comment).
- `agent/github-releaser/pkg/plugin/manifest_test.go` — existing `DescribeTable("DetectManifests", ...)`, `DescribeTable("BumpPluginJson file content", ...)`, and `DescribeTable("BumpMarketplaceJson file content", ...)`. New `Entry(...)` rows attach to these tables. Package is `plugin_test`.
- `agent/github-releaser/pkg/steps_execution.go` — `executeDirectPush` invokes `plugin.DetectManifests`, then loops over detected manifests dispatching to `plugin.BumpPluginJson` or `plugin.BumpMarketplaceJson` based on `strings.HasSuffix(manifestPath, "plugin.json")` / `"marketplace.json"`. Errors map to `git.ErrorCategoryPluginManifestInvalid`; manifest-read I/O errors map to `git.ErrorCategoryUnknown`.
- `agent/github-releaser/pkg/steps_execution_test.go` — existing `Context("plugin manifests", ...)` block; the malformed `plugin.json` test lives in an `It("plugin.json is malformed JSON …", ...)` block inside it. Also contains the existing `DescribeTable` blocks for `sameStringSet` and `deriveUnprefixedVersion` (exposed via `pkg.SameStringSetForTest` and `pkg.DeriveUnprefixedVersionForTest` from `agent/github-releaser/pkg/export_test.go`). Package is `pkg_test`.
- `agent/github-releaser/pkg/export_test.go` — confirms the two unexported helpers are exposed.

Reference docs (read for conventions, do not duplicate) — locate via the coding-plugin marketplace path inside the container; do not hardcode the mount path:

- `coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega conventions.
- `coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` usage (relevant only if any helper needs to wrap; this prompt's helpers do not).

This prompt MUST run AFTER sibling prompt 5 in the queue. Prompt 5 changes `rewriteVersionValue`'s unquoted branch (the bug that injects a stray `,` when the original line had no trailing comma) and adds the `marketplace.json` dispatch error-mapping; the new tests below assume those production fixes are in place. If the daemon executes this prompt before prompt 5 finishes, the unquoted-value and `marketplace.json` malformed-JSON entries are expected to fail until prompt 5 lands — that is the correct ordering signal, not a bug in this prompt.
</context>

<requirements>

All file paths below are repo-relative from the worktree root.

## 1. (Reserved — superseded by requirement 9)

The "directory-as-file" coverage gap is addressed by requirement 9's `DescribeTable` rewrite, which adds the new Entry as part of the cleanup-and-temp-dir-leak fix. Do not add a separate Entry here.

## 2. `pkg/steps_execution_test.go` — `DetectManifests` I/O error integration path

Add one `It` block inside the existing `Context("plugin manifests", func() { ... })` block, placed AFTER the existing `It("plugin.json is malformed JSON → …", ...)` block.

Goal: surface a non-`IsNotExist` error from `os.Stat` so the dispatch maps it to `git.ErrorCategoryUnknown` and never calls `Commit`/`Tag`/`Push`.

Implementation:

```go
It("DetectManifests I/O error → Result(failed, error_category=unknown); Commit/Tag/Push not called", func() {
    // chmod 0000 on Linux non-root blocks Stat of the children;
    // skip on platforms where this is unreliable (Darwin, root containers).
    if runtime.GOOS == "darwin" || os.Geteuid() == 0 {
        Skip("requires unprivileged Linux for non-IsNotExist Stat failure")
    }

    fakeOps := &gitmocks.GitOps{}
    var capturedWorkdir string
    fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
        capturedWorkdir = workdir
        writeChangelog(workdir)
        // Create .claude-plugin as a directory with mode 0000 so Stat on
        // its children returns EACCES (a non-IsNotExist error path).
        Expect(os.MkdirAll(filepath.Join(workdir, ".claude-plugin"), 0o750)).To(Succeed())
        Expect(os.Chmod(filepath.Join(workdir, ".claude-plugin"), 0o000)).To(Succeed())
        return nil
    }
    // Defer-restore the directory mode so the workdir-cleanup RemoveAll succeeds.
    DeferCleanup(func() {
        if capturedWorkdir != "" {
            _ = os.Chmod(filepath.Join(capturedWorkdir, ".claude-plugin"), 0o750)
        }
    })

    step := pkg.NewExecutionStep(fakeOps, "")
    md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
    Expect(err).NotTo(HaveOccurred())

    result, err := step.Run(context.Background(), md)
    Expect(err).NotTo(HaveOccurred())
    Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

    Expect(fakeOps.CommitCallCount()).To(Equal(0))
    Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))
    Expect(fakeOps.TagCallCount()).To(Equal(0))
    Expect(fakeOps.PushCallCount()).To(Equal(0))

    got, _ := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
    Expect(got.Outcome).To(Equal("failed"))
    Expect(string(got.ErrorCategory)).To(Equal("unknown"))
})
```

Add `"runtime"` to the import block of `pkg/steps_execution_test.go` if not already present. `taskMDPlugin` is already declared in the surrounding `Context("plugin manifests", ...)` block — reuse it. Use `DeferCleanup` (Ginkgo v2 built-in) to restore directory mode before the workdir-cleanup defer in production code runs.

## 3. `pkg/plugin/manifest_test.go` — `BumpPluginJson` unquoted-value entry

Add two `Entry` rows to the existing `DescribeTable("BumpPluginJson file content", ...)` table. Both exercise the unquoted-value branch in `rewriteVersionValue`. These tests assert prompt 5's fix to that branch (the current production code injects a stray `,` regardless of input punctuation; prompt 5 makes the rewrite preserve original punctuation).

```go
Entry("unquoted value with NO trailing comma — output keeps no comma",
    []byte(`{"name": "x", "version": 0.9.12}`),
    "0.10.0",
    []byte(`{"name": "x", "version": 0.10.0}`),
    false, ""),

Entry("unquoted value WITH trailing comma — output keeps comma",
    []byte(`{"name": "x", "version": 0.9.12, "other": 1}`),
    "0.10.0",
    []byte(`{"name": "x", "version": 0.10.0, "other": 1}`),
    false, ""),
```

These both go through `rewriteVersionValue`'s `// Unquoted value` branch. The assertion is byte-exact on the returned content (no extra `,`, no stripped `,`).

## 4. `pkg/plugin/manifest_test.go` — `BumpPluginJson` unclosed-quote entry

Add one `Entry` row to the same `DescribeTable("BumpPluginJson file content", ...)` table to drive the `closeIdx == -1` path inside `rewriteVersionValue`:

```go
Entry("unclosed quote on version value — error mentions version field",
    []byte(`{"version": "0.9.12}`),
    "0.10.0",
    nil,
    true, "(not a semver|version)"),
```

The expected production error from `rewriteVersionValue` reads `plugin.json existing version field is not a semver-shaped string: ""` (matches `not a semver` via regex). The regex above also accepts `version` so it stays robust if prompt 5 renames the message.

## 5. `pkg/plugin/manifest_test.go` — `BumpPluginJson` second-`"version"`-key entry

Add one `Entry` row to the same `DescribeTable("BumpPluginJson file content", ...)` table documenting the first-match contract — only the top-level `"version"` is rewritten; a nested `"version"` is left byte-identical to input:

```go
Entry("second nested version key is left untouched",
    []byte(`{
  "version": "0.9.12",
  "extras": {
    "version": "0.9.12"
  }
}`),
    "0.10.0",
    []byte(`{
  "version": "0.10.0",
  "extras": {
    "version": "0.9.12"
  }
}`),
    false, ""),
```

The `BumpPluginJson` loop sets `found = true` on the first hit and stops rewriting, so the nested line is byte-identical. The assertion proves it.

## 6. `pkg/steps_execution_test.go` — `deriveUnprefixedVersion("")` table entry

Add one `Entry` row to the existing `DescribeTable("strips ## prefix and v prefix", ...)` table inside `Describe("deriveUnprefixedVersion", ...)`:

```go
Entry("empty string → empty", "", ""),
```

`deriveUnprefixedVersion` is two `strings.TrimPrefix` calls so `""` returns `""`. Documents the fail-safe path.

## 7. `pkg/steps_execution_test.go` — `marketplace.json` malformed integration test

Add one `It` block inside the existing `Context("plugin manifests", func() { ... })` block, placed AFTER the existing `It("plugin.json is malformed JSON → …", ...)` block.

Mirrors the existing plugin.json malformed test exactly, but for marketplace.json:

```go
It("marketplace.json is malformed JSON → Result(failed, error_category=plugin_manifest_invalid); Commit NOT called; Tag NOT called; Push NOT called", func() {
    fakeOps := &gitmocks.GitOps{}
    fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
        writeChangelog(workdir)
        Expect(os.MkdirAll(filepath.Join(workdir, ".claude-plugin"), 0o750)).To(Succeed())
        malformedMarketplace := []byte(`{"metadata": {"version": }}`)
        Expect(os.WriteFile(filepath.Join(workdir, ".claude-plugin", "marketplace.json"), malformedMarketplace, 0o600)).To(Succeed())
        return nil
    }
    fakeOps.TagReturns(nil)
    fakeOps.PushReturns(nil)

    step := pkg.NewExecutionStep(fakeOps, "")
    md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
    Expect(err).NotTo(HaveOccurred())

    result, err := step.Run(context.Background(), md)
    Expect(err).NotTo(HaveOccurred())
    Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

    Expect(fakeOps.CommitCallCount()).To(Equal(0))
    Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))
    Expect(fakeOps.TagCallCount()).To(Equal(0))
    Expect(fakeOps.PushCallCount()).To(Equal(0))

    got, _ := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
    Expect(got.Outcome).To(Equal("failed"))
    Expect(string(got.ErrorCategory)).To(Equal("plugin_manifest_invalid"))
    Expect(got.Error).To(ContainSubstring(".claude-plugin/marketplace.json"))
    Expect(got.Tag).To(BeEmpty())
    Expect(got.CommitSHA).To(BeEmpty())
})
```

`taskMDPlugin` is already in scope. Reuse `writeChangelog`.

## 8. `pkg/plugin/manifest_test.go` — `BumpMarketplaceJson` out-of-scope `"version"` entry

Add one `Entry` row to the existing `DescribeTable("BumpMarketplaceJson file content", ...)` table. Asserts that a top-level (depth-1) `"version"` key that is NOT inside `metadata` or `plugins[]` is NOT rewritten — only `metadata.version` and `plugins[].version` are:

```go
Entry("top-level version outside metadata/plugins is NOT rewritten",
    []byte(`{
  "name": "x",
  "version": "0.0.1",
  "metadata": {
    "version": "0.9.12"
  },
  "plugins": [
    {"name": "a", "version": "0.9.12"}
  ]
}`),
    "0.10.0",
    []byte(`{
  "name": "x",
  "version": "0.0.1",
  "metadata": {
    "version": "0.10.0"
  },
  "plugins": [
    {"name": "a", "version": "0.10.0"}
  ]
}`),
    false, ""),
```

Production code in `BumpMarketplaceJson` only rewrites when `inMetadata || inPlugin` is true (see the `inScope` check). The top-level `"version": "0.0.1"` line satisfies `lineHasVersionKey` but neither `inMetadata` nor `inPlugin` is true at that depth → the line is written through verbatim. Test asserts the byte-exact equality.

## 9. `pkg/plugin/manifest_test.go` — `DetectManifests` temp-dir cleanup

The existing `DescribeTable("DetectManifests", ...)` rows pass their workdir via an inline `func() string { dir, _ := os.MkdirTemp("", "...") ... return dir }()` which is evaluated at table-construction time (suite-init) and leaks the temp dir on every run.

Refactor the table to use `GinkgoT().TempDir()` (auto-cleans per test) instead of `os.MkdirTemp`. Convert the table from positional arg passing to a per-row closure that the table function evaluates at run time. Concrete change:

Old shape (4 inline-evaluated workdir args):
```go
DescribeTable("DetectManifests",
    func(workdir string, want []string) {
        got, err := plugin.DetectManifests(context.Background(), workdir)
        Expect(err).NotTo(HaveOccurred())
        Expect(got).To(Equal(want))
    },
    Entry("neither exists → returns nil slice",
        func() string { dir, _ := os.MkdirTemp("", "detect-neither"); return dir }(),
        nil),
    // ... etc
)
```

New shape (closure builds workdir at Entry-run time, GinkgoT().TempDir() auto-cleans):
```go
DescribeTable("DetectManifests",
    func(setup func(dir string), want []string) {
        dir := GinkgoT().TempDir()
        setup(dir)
        got, err := plugin.DetectManifests(context.Background(), dir)
        Expect(err).NotTo(HaveOccurred())
        Expect(got).To(Equal(want))
    },
    Entry("neither exists → returns nil slice",
        func(dir string) {},
        nil),
    Entry("plugin.json only → returns [plugin.json]",
        func(dir string) {
            Expect(os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o750)).To(Succeed())
            Expect(os.WriteFile(filepath.Join(dir, ".claude-plugin", "plugin.json"),
                []byte(`{"name":"test","version":"0.1.0"}`), 0o600)).To(Succeed())
        },
        []string{".claude-plugin/plugin.json"}),
    Entry("marketplace.json only → returns [marketplace.json]",
        func(dir string) {
            Expect(os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o750)).To(Succeed())
            Expect(os.WriteFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"),
                []byte(`{"metadata":{"version":"0.1.0"},"plugins":[]}`), 0o600)).To(Succeed())
        },
        []string{".claude-plugin/marketplace.json"}),
    Entry("both exist → returns both",
        func(dir string) {
            Expect(os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o750)).To(Succeed())
            Expect(os.WriteFile(filepath.Join(dir, ".claude-plugin", "plugin.json"),
                []byte(`{"name":"test","version":"0.1.0"}`), 0o600)).To(Succeed())
            Expect(os.WriteFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"),
                []byte(`{"metadata":{"version":"0.1.0"},"plugins":[]}`), 0o600)).To(Succeed())
        },
        []string{".claude-plugin/marketplace.json", ".claude-plugin/plugin.json"}),
    Entry("plugin.json exists as a directory → omitted from result",
        func(dir string) {
            Expect(os.MkdirAll(filepath.Join(dir, ".claude-plugin", "plugin.json"), 0o750)).To(Succeed())
        },
        nil),
)
```

Notes on the rewrite:

- `GinkgoT().TempDir()` is removed automatically after each Entry runs — no leak.
- Use `0o750` for directories and `0o600` for files. This matches `writeChangelog` in `steps_execution_test.go` and the project-wide gosec posture. Do not preserve the old `0o755` / `0o644` modes.
- The "both exist" Entry MUST assert the result `[]string{".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"}` — `plugin.json` first, then `marketplace.json` — because `DetectManifests` iterates over `known = [".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"]` in that exact order. Match the production iteration order verbatim.
- **Important: this refactor SUPERSEDES requirement 1.** The new "plugin.json exists as a directory → omitted from result" Entry is included here (last Entry in the rewritten `DescribeTable`). Do NOT also add it via requirement 1 — that would duplicate. If the file already had the Entry added by requirement 1, replace it; do not append a second copy.

## 10. `pkg/steps_execution_test.go` — `sameStringSet` duplicate-elements coverage

Add two `Entry` rows to the existing `DescribeTable("order-independent set equality", ...)` table inside `Describe("sameStringSet", ...)`:

```go
Entry("identical duplicates → true", []string{"a", "a"}, []string{"a", "a"}, true),
Entry("duplicate vs distinct, same length → false", []string{"a", "a"}, []string{"a", "b"}, false),
```

Documents that `sameStringSet`'s sort-then-compare semantics treat duplicate elements as set-equal only when both slices have the same multiset. (`["a","a"]` vs `["a","b"]` sorts to `["a","a"]` vs `["a","b"]` → `slices.Equal` is false.)

</requirements>

<constraints>

- Use Ginkgo v2 + Gomega with `DescribeTable`/`Entry` for matrix cases; never stdlib `t.Run` tables.
- Tests are in `package plugin_test` (`pkg/plugin/manifest_test.go`) and `package pkg_test` (`pkg/steps_execution_test.go`) — match the existing files; do NOT switch to internal `package plugin` or `package pkg` test packages.
- Use `github.com/bborbe/errors` for any error wrapping in helpers (tests here only assert error messages — they do not produce errors).
- Do NOT modify any production code in this prompt. The `rewriteVersionValue` unquoted-branch fix and the `marketplace.json` malformed-JSON dispatch are owned by sibling prompt 5. If a new test fails because the prompt-5 fix is not yet in, that is the expected ordering signal — the daemon executes prompts in order.
- Do NOT add new test files. All additions go into the two existing files named in `<requirements>`.
- Do NOT introduce new top-level `Describe` or `Context` blocks unless a requirement explicitly says so. New `Entry` rows attach to the named existing tables; new `It` blocks live inside the named existing `Context("plugin manifests", …)` block.
- Branch is already `feature/plugin-version-bump`. Do NOT switch branches. Do NOT commit — dark-factory handles git.
- Pre-existing failure in `pkg/git/os_exec_git_ops_test.go:222` (pre-push-hook test) is unrelated to this prompt; do NOT attempt to fix it here.

</constraints>

<verification>

Repo-relative from the worktree root:

```
cd agent/github-releaser && make precommit
```

Exit code 0. All tests pass (after prompt 5 has also been applied, which the daemon does first because it is queued before this prompt).

Spot-checks (also repo-relative):

```
cd agent/github-releaser && go test ./pkg/plugin/... -count=1
cd agent/github-releaser && go test ./pkg/... -count=1 -timeout 60s
```

Both must exit 0.

If `make precommit` reports failure ONLY in `pkg/git/os_exec_git_ops_test.go:222` (pre-push-hook test), that failure is pre-existing and out of scope for this prompt.

</verification>
