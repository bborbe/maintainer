---
status: completed
spec: ["062"]
summary: Integrated plugin manifest detection and bumping into executeDirectPush; modified guardCommittedFiles to accept expectedFiles parameter; added sameStringSet and deriveUnprefixedVersion helpers with unit tests
container: maintainer-plugin-version-bump-exec-221-spec-056-execute-direct-push-integration
dark-factory-version: v0.173.0
created: "2026-06-01T00:00:00Z"
queued: "2026-05-31T22:32:08Z"
started: "2026-05-31T23:16:03Z"
completed: "2026-05-31T23:19:03Z"
branch: feature/plugin-version-bump
---

<summary>
- `executeDirectPush` in `steps_execution.go` now detects plugin manifests BEFORE writing CHANGELOG.md
- Detected manifest paths are captured in a local variable
- Both manifests (if detected) are bumped to the unprefixed semver from `plan.NextVersion`
- The bump-and-write loop uses the manifest package functions; failures map to `plugin_manifest_invalid`
- The pre-push guard accepts exactly `{CHANGELOG.md}` plus the subset of detected manifests
- Commit paths are built explicitly as variadic arguments (no `git add -A`, no globs)
- Backward compatibility preserved: repos without `.claude-plugin/` behave identically to before
</summary>

<objective>
Integrate the plugin manifest package into `executeDirectPush` (in `pkg/steps_execution.go`). The manifest detection must happen BEFORE the changelog `os.WriteFile`. The detected file list must be used both for the bump-and-write loop and for constructing the guard's expected-file whitelist. The pre-push guard must accept exactly the set of files that were rewritten (changelog + detected manifests). The `GitOps` interface (method signatures, notably `Commit`) must not change.
</objective>

<context>
Read these files before implementing:
- `/workspace/agent/github-releaser/pkg/steps_execution.go` — the file to modify (executeDirectPush, guardCommittedFiles, fail method)
- `/workspace/agent/github-releaser/pkg/git/git.go` — the frozen GitOps interface (Commit signature: `Commit(ctx, workdir, message string, paths ...string)`)
- `/workspace/agent/github-releaser/pkg/git/error_classifier.go` lines 1-60 — `ErrorCategoryPluginManifestInvalid` constant added by Prompt 2
- `/workspace/agent/github-releaser/pkg/plugin/manifest.go` — the manifest package created by Prompt 1

Key facts from the spec:
- Manifest detection happens BEFORE any `os.WriteFile`
- Version string written = unprefixed semver from `plan.NextVersion` (e.g. `plan.NextVersionHeader = "## v0.10.0"` → version string `0.10.0`)
- Derive version string by stripping `## ` prefix from `plan.NextVersionHeader`, then stripping `v` prefix if present
- If `NextVersionHeader` is already `## 0.10.0` (no v prefix), derive from that (strip `## ` → `0.10.0`)
- Commit variadic paths: `changelogFileName` plus the detected manifest paths (in that order)
- Guard expected files: `append([]string{changelogFileName}, detectedManifests...)`
- `guardCommittedFiles` receives the expected file list (not the detection result) — modify the guard call or make guard accept the expected set as a parameter
- On bump error (malformed manifest, missing version, non-semver), fail with `ErrorCategoryPluginManifestInvalid`, no commit, no tag, no push
- On disk write error (ENOSPC, EACCES), fail with `ErrorCategoryUnknown`, no commit, no tag, no push
</context>

<requirements>
1. **Read** `/workspace/agent/github-releaser/pkg/steps_execution.go` fully (all ~320 lines)

2. **Add import** for the plugin package:
   ```go
   "github.com/bborbe/maintainer/agent/github-releaser/pkg/plugin"
   ```
   (Place it alphabetically after the changelog import)

3. **Modify `executeDirectPush`** to integrate manifest detection and bumping:

   a. **After the Clone succeeds** (line 170-173 area), before reading CHANGELOG.md:
   ```go
   // Detect plugin manifests BEFORE any writes.
   detectedManifests, err := plugin.DetectManifests(ctx, workdir)
   if err != nil {
       result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
           errors.Wrapf(ctx, err, "detect plugin manifests in %s", workdir))
       return "", "", result
   }
   ```

   b. **After the changelog `os.WriteFile`** succeeds (around line 194-198 area), add the manifest bump-and-write loop:
   ```go
   // Bump and write detected plugin manifests.
   for _, manifestPath := range detectedManifests {
       manifestAbsPath := filepath.Join(workdir, manifestPath)
       manifestContent, err := os.ReadFile(manifestAbsPath) // #nosec G304 -- workdir is os.TempDir-rooted
       if err != nil {
           result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Wrapf(ctx, err, "read %s", manifestAbsPath))
           return "", "", result
       }

       // Derive the unprefixed semver from plan.NextVersionHeader (e.g. "## v0.10.0" → "0.10.0")
       unprefixedVersion := deriveUnprefixedVersion(plan.NextVersionHeader)

       var rewrittenManifest []byte
       if strings.HasSuffix(manifestPath, "plugin.json") {
           rewrittenManifest, err = plugin.BumpPluginJson(ctx, manifestContent, unprefixedVersion)
       } else if strings.HasSuffix(manifestPath, "marketplace.json") {
           rewrittenManifest, err = plugin.BumpMarketplaceJson(ctx, manifestContent, unprefixedVersion)
       }
       if err != nil {
           result, _ := s.fail(ctx, md, git.ErrorCategoryPluginManifestInvalid,
               errors.Wrapf(ctx, err, "bump %s", manifestPath))
           return "", "", result
       }

       if err := os.WriteFile(manifestAbsPath, rewrittenManifest, 0o644); err != nil { // #nosec G306,G703 -- standard perms; workdir is os.TempDir-rooted
           result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Wrapf(ctx, err, "write %s", manifestAbsPath))
           return "", "", result
       }
   }
   ```

   c. **Modify the `ops.Commit` call** to pass all files to commit:
   ```go
   // Build the full commit path list: changelog + detected manifests (in that order).
   commitPaths := append([]string{changelogFileName}, detectedManifests...)
   sha, err = s.ops.Commit(ctx, workdir, "release "+tagName, commitPaths...)
   ```
   (The variadic `paths ...string` already supports this — no interface change needed)

   d. **Modify `guardCommittedFiles`** call to pass the expected file list:
   ```go
   expectedFiles := append([]string{changelogFileName}, detectedManifests...)
   if failResult := s.guardCommittedFiles(ctx, md, workdir, expectedFiles); failResult != nil {
       return "", "", failResult
   }
   ```

4. **Modify `guardCommittedFiles`** to accept an `expectedFiles []string` parameter. The comparison is set-equality (order-independent — `git diff-tree` output ordering is alphabetical, not insertion-order). The helper **must NOT mutate its inputs** (the caller passes `expectedFiles` into the error message on the failure path, so an in-place sort would corrupt that message):
   ```go
   func (s *executionStep) guardCommittedFiles(
       ctx context.Context,
       md *agentlib.Markdown,
       workdir string,
       expectedFiles []string,
   ) *agentlib.Result {
       files, err := s.ops.CommittedFiles(ctx, workdir)
       if err != nil {
           result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Wrap(ctx, err, "inspect committed files"))
           return result
       }
       if !sameStringSet(files, expectedFiles) {
           result, _ := s.fail(ctx, md, git.ErrorCategoryUnexpectedDiff,
               errors.Errorf(ctx,
                   "release commit must change only %v, got %v", expectedFiles, files))
           return result
       }
       return nil
   }

   // sameStringSet reports whether a and b contain the same elements,
   // ignoring order. It never mutates its inputs (callers reuse them
   // — e.g. expectedFiles is rendered into the failure message).
   func sameStringSet(a, b []string) bool {
       if len(a) != len(b) {
           return false
       }
       ac := slices.Clone(a)
       bc := slices.Clone(b)
       slices.Sort(ac)
       slices.Sort(bc)
       return slices.Equal(ac, bc)
   }
   ```
   Add `"slices"` to imports (used for `Clone`, `Sort`, `Equal`).

5. **Add the `deriveUnprefixedVersion` helper** to `steps_execution.go`:
   ```go
   // deriveUnprefixedVersion strips "## " prefix and "v" prefix from
   // a version header to produce the unprefixed semver string.
   // "## v0.10.0" → "0.10.0"
   // "## 0.10.0" → "0.10.0"
   // "0.10.0" → "0.10.0"
   func deriveUnprefixedVersion(header string) string {
       header = strings.TrimPrefix(header, "## ")
       header = strings.TrimPrefix(header, "v")
       return header
   }
   ```

6. **Imports**: `"slices"` (for the helper above). No `"sort"` import needed — the modern `slices` API replaces it.

7. **Audit all existing callers of `guardCommittedFiles`** — there is only one call site (inside `executeDirectPush`). The new parameter is required so update it.

8. **Verify no other functions** in `steps_execution.go` call `guardCommittedFiles` — grep the file to confirm.

9. **Deterministic commit-path order at the call site**: build `commitPaths` as `append([]string{changelogFileName}, detectedManifests...)` so the order is reproducible (changelog first, detected manifests next in the order `DetectManifests` returned them — `plugin.json` before `marketplace.json` per Prompt 1's slice order). The guard's `sameStringSet` is order-independent, but the *call-site* order is fixed so the commit-paths argument list is stable across runs.

10. **Add a small unit test** for `sameStringSet` in a new or existing test file alongside `steps_execution_test.go` — `DescribeTable` rows: equal-same-order / equal-different-order / different-length / element-mismatch / nil-vs-nil / empty-vs-empty / one-empty. Each `Entry` calls `sameStringSet(a, b)` and asserts the boolean result. Additionally, in one entry assert that the inputs are NOT mutated after the call (`Expect(a).To(Equal(originalA))` / same for `b`) — this locks in the no-mutation contract.

11. **Run `make test`** after each meaningful change.
</requirements>

<constraints>
- `GitOps` interface signatures must not change — variadic `paths ...string` already supports multiple files
- Manifest detection must happen BEFORE any `os.WriteFile` (detection happens at the filesystem layer, before changelog rewrite)
- Pre-push guard must fail closed on any deviation from the expected set
- `ErrorCategoryPluginManifestInvalid` is set directly by the execution step (not via `ClassifyError`)
- Disk write errors (ENOSPC, EACCES) use `ErrorCategoryUnknown`
- Use `github.com/bborbe/errors` only; never `fmt.Errorf`
- Never `context.Background()` in package code
</constraints>

<verification>
From `agent/github-releaser/`, run:
```
make precommit
```
All tests must pass. The Ginkgo suite in `pkg/` verifies the manifest integration and guard behavior.

Additionally, verify the SYNTACTIC order of operations in `executeDirectPush` (line-number anchors go stale; the contract is the *order of statements* in the function body):
1. `plugin.DetectManifests(...)` call must syntactically precede the first `os.ReadFile(changelogPath)` in the function body
2. `plugin.DetectManifests(...)` call must syntactically precede the first `os.WriteFile(changelogPath, ...)` in the function body
3. `s.guardCommittedFiles(...)` call must syntactically precede the `s.ops.Tag(...)` call
4. The `expectedFiles` slice passed to `s.guardCommittedFiles` must equal `append([]string{changelogFileName}, detectedManifests...)` (changelog first, then the manifests in the order returned by `DetectManifests`)
</verification>