---
status: completed
spec: ["062"]
summary: Added ErrorCategoryPluginManifestInvalid constant to pkg/git/error_classifier.go with two-layer-classification comment, plus corresponding test assertions in error_classifier_test.go
container: maintainer-plugin-version-bump-exec-220-spec-056-error-category
dark-factory-version: v0.173.0
created: "2026-06-01T00:00:00Z"
queued: "2026-05-31T22:32:08Z"
started: "2026-05-31T23:14:16Z"
completed: "2026-05-31T23:16:02Z"
branch: feature/plugin-version-bump
---

<summary>
- New `ErrorCategoryPluginManifestInvalid` constant added to `pkg/git/error_classifier.go` with string value `"plugin_manifest_invalid"`
- The constant is NOT added to `classifierTable` (it is set directly by the execution step at the manifest-package layer)
- Two-layer-classification comment added following the pattern of `ErrorCategoryUnexpectedDiff`
- Source inspection assertions added to the existing test file
</summary>

<objective>
Add the `plugin_manifest_invalid` error category constant to the closed `ErrorCategory` enum in `pkg/git/error_classifier.go`. The new category follows the same two-layer-classification pattern as `ErrorCategoryUnexpectedDiff`, `ErrorCategoryChangelogMissing`, and `ErrorCategoryUnreleasedNotFound` — it is set directly by the execution step at the manifest-package layer (not by `ClassifyError`) and therefore must NOT appear in `classifierTable`.
</objective>

<context>
Read these files before implementing:
- `/workspace/agent/github-releaser/pkg/git/error_classifier.go` — the ErrorCategory enum and classifierTable; the new constant is added to the const block
- `/workspace/agent/github-releaser/pkg/git/error_classifier_test.go` — the existing test file; append new `It` blocks to the existing `Describe("ClassifyError", ...)` block (follow the pattern already established for `changelog_missing` and `unreleased_not_found`)
- `/workspace/agent/github-releaser/pkg/git/git_suite_test.go` — already exists; do NOT create a suite file

The `ErrorCategory` enum is a **closed set**. The new constant mirrors `ErrorCategoryUnexpectedDiff` in shape (single-line `// ErrorCategory...` header → blank → multi-line block comment explaining when it fires AND why it is NOT in classifierTable (two-layer-classification reasoning) → const definition). See `<requirements>` section 3 for the exact comment content.
</context>

<requirements>
1. **Read** `/workspace/agent/github-releaser/pkg/git/error_classifier.go`

2. **Add the new constant** to the `const (...)` block in `error_classifier.go`, between `ErrorCategoryUnexpectedDiff` and `ErrorCategoryPushNonFastForward`. Place it after `ErrorCategoryUnexpectedDiff` since they share the same two-layer-classification pattern.

3. **Insert the constant** after the existing `ErrorCategoryUnexpectedDiff` block (they share the same two-layer-classification pattern, so they should sit next to each other). Use exactly this comment + constant:
   ```go
   // ErrorCategoryPluginManifestInvalid — plugin manifest (.claude-plugin/plugin.json or
   // .claude-plugin/marketplace.json) is malformed (JSON parse error inside the version-locator
   // scan) or its version field is absent or not a quoted semver-shaped string. Detected at
   // the plugin manifest package layer (manifest.go bump operation), not git stderr.
   //
   // TWO-LAYER CLASSIFICATION: this category is set DIRECTLY by the execution step
   // (steps_execution.go), NOT by ClassifyError. ClassifyError maps git *stderr* onto
   // categories; plugin_manifest_invalid is a *semantic* assertion on the manifest content
   // (the bump operation failed, git never ran), so there is no stderr fragment to match.
   // Same split as changelog_missing / unreleased_not_found / unexpected_diff, which are
   // set at the filesystem / changelog-package / guard layer respectively. Do NOT add
   // a classifierTable entry for it — it would never fire (no matching stderr) and would
   // imply the wrong layer owns the check.
   ErrorCategoryPluginManifestInvalid ErrorCategory = "plugin_manifest_invalid"
   ```

4. **Do NOT add a `classifierTable` entry** for `plugin_manifest_invalid`. The `classifierTable` is for git stderr fragments only.

5. **Open** `/workspace/agent/github-releaser/pkg/git/error_classifier_test.go` (the file already exists; do NOT create or overwrite it).

6. **Append two new `It` blocks INSIDE the existing `Describe("ClassifyError", ...)` block in `error_classifier_test.go`**, immediately AFTER the existing `It("never returns unreleased_not_found from ClassifyError", ...)` block and BEFORE the final `It("returns empty-string sentinel on nil", ...)` block. Mirror the existing `never returns` / string-value contract style exactly (the test file is `package git_test`, so all references use the `git.` prefix; `classifierTable` is unexported and intentionally NOT exercised from here — the contract is asserted by the public `ClassifyError` boundary, the same way `changelog_missing` and `unreleased_not_found` are):
   ```go
   It("never returns plugin_manifest_invalid from ClassifyError", func() {
       // plugin_manifest_invalid — declared on enum but emitted by execution step
       // at the plugin manifest package layer; never reaches ClassifyError because
       // git never runs when the bump fails.
       Expect(git.ClassifyError(errors.New("plugin.json version field not found"))).
           NotTo(Equal(git.ErrorCategoryPluginManifestInvalid))
   })
   It("ErrorCategoryPluginManifestInvalid has string value plugin_manifest_invalid", func() {
       Expect(string(git.ErrorCategoryPluginManifestInvalid)).To(Equal("plugin_manifest_invalid"))
   })
   ```
   The suite file `git_suite_test.go` already exists; do NOT add or modify it.
</requirements>

<constraints>
- The `ErrorCategory` enum is a closed set — adding a new category requires a spec amendment
- `classifierTable` must NOT contain `plugin_manifest_invalid` — it is set directly by the execution step, not by `ClassifyError`
- Two-layer-classification comment must mirror `ErrorCategoryUnexpectedDiff` in structure
- Use `github.com/bborbe/errors` for any new error wrapping (though this constant definition alone needs no wrapping)
</constraints>

<verification>
From `agent/github-releaser/`, run:
```
make precommit
```
This runs tests for `pkg/git/...`. All tests must pass, including the new `ErrorCategoryPluginManifestInvalid` assertions.

Additionally, verify the constant is absent from `classifierTable`:
```
awk '/classifierTable/,/^}/' agent/github-releaser/pkg/git/error_classifier.go | grep -c 'plugin_manifest_invalid'
```
Must return `0`.
</verification>