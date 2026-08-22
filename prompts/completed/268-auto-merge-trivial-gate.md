---
status: completed
summary: Added autoMerge.trivial per-repo opt-in gate to maintainerconfig schema with AutoMergeConfig type, package-doc line, Ginkgo tests (true/absent/non-bool strict), README table+example, and CHANGELOG feat bullet
execution_id: maintainer-auto-merge-exec-268-auto-merge-trivial-gate
dark-factory-version: dev
created: "2026-08-22T20:21:09Z"
queued: "2026-08-22T20:21:09Z"
started: "2026-08-22T20:21:33Z"
completed: "2026-08-22T20:24:00Z"
---

# Add autoMerge.trivial per-repo gate

<summary>
- Repos can opt in to trivial-PR auto-merge via `.maintainer.yaml` (`autoMerge.trivial: true`)
- The shared per-repo config schema gains the new namespace so the separate PR-watcher service can read it
- Repos without the flag (or without the file) default to `false` — no repo auto-merges without explicit consent
- Existing namespaces (`release`, `prReviewer`, `goUpdate`) parse exactly as before
- New namespace flows through both lenient (`Parse`) and strict (`ParseStrict`) paths
</summary>

<objective>
Add the `autoMerge.trivial` per-repo opt-in gate to the shared `maintainerconfig` schema so the github-pr-watcher trivial-classifier (separate repo) can gate auto-merge on repo consent — default `false`, never defaulted-on.
</objective>

<context>
Read `maintainerconfig/maintainerconfig.go` — the single shared schema of `.maintainer.yaml`, one top-level namespace per bot. Follow the existing `GoUpdateConfig` pattern (struct field + type + `yaml` tags + package-doc line). Read `maintainerconfig/maintainerconfig_test.go` — the `DescribeTable("valid documents", ...)` pattern for parse rows and the strict-parse error assertions.
</context>

<requirements>
1. In `maintainerconfig/maintainerconfig.go`:
   - Add a field `AutoMerge AutoMergeConfig yaml:"autoMerge"` to the `MaintainerConfig` struct (after `GoUpdate`).
   - Define `AutoMergeConfig` with a single field `Trivial bool yaml:"trivial"` (default `false`), with a doc comment matching the `GoUpdateConfig` style — opt-in, not opt-out; absent reads `false`.
   - Extend the package doc comment with a line for the new namespace, mirroring the existing lines, e.g. `autoMerge:\n  trivial: true     # github-pr-watcher trivial auto-merge gate`.
   - Do NOT modify `knownNamespaces()` — it is reflection-driven and picks up the new tag automatically.
2. In `maintainerconfig/maintainerconfig_test.go` (external test package, Ginkgo/Gomega — follow the existing suite):
   - Add a valid-documents `DescribeTable` row with description `autoMerge.trivial: true -> Trivial true` asserting `expected.Trivial` is `true`.
   - Add a valid-documents row with description `autoMerge absent -> Trivial false` asserting `expected.Trivial` is `false` (default).
   - Add a strict-parse failure row with description `autoMerge.trivial: non-bool -> strict error` asserting a wrapped error whose message contains `unmarshal .maintainer.yaml` — match the existing strict-error assertion style used for `allowMajorBump` / `changelogRewrite` / `allowFork` (the "non-bool" cases in `maintainerconfig_test.go`; there is no non-bool error test for `goUpdate`). Do NOT assert a field-path literal such as `autoMerge.trivial` inside the error message; `yaml.v3` does not emit field paths.
3. In `CHANGELOG.md`: add a single `feat:` bullet under `## Unreleased` (create the `## Unreleased` section if it does not exist) naming the new field — e.g. `feat: add autoMerge.trivial per-repo opt-in gate for trivial-PR auto-merge`.
4. In `README.md`: add `autoMerge.trivial` to the `.maintainer.yaml` schema table (around line 46) and to the example block (around lines 69-75), matching the existing `prReviewer.autoApprove` / `release.autoRelease` entries. The pre-existing missing `goUpdate.autoUpdate` may be backfilled in the same edit.
</requirements>

<constraints>
- Use `github.com/bborbe/errors` for error wrapping — never `fmt.Errorf`.
- Preserve existing behavior: `release`, `prReviewer`, `goUpdate` parse identically; empty-file → zero-value contract unchanged; unknown top-level namespaces still ignored (forward-compat).
- No new dependencies; do not run `go mod vendor`; do not use `-mod=vendor` in any test/verification command.
- External test package + Ginkgo/Gomega — follow the existing suite pattern; no new files beyond edits to `maintainerconfig.go` / `maintainerconfig_test.go` / `CHANGELOG.md` / `README.md`.
- Repo-relative paths only; no absolute paths.
</constraints>

<verification>
make precommit
go test ./maintainerconfig/...

grep -c 'AutoMerge AutoMergeConfig' maintainerconfig/maintainerconfig.go
grep -c 'yaml:"autoMerge"' maintainerconfig/maintainerconfig.go
grep -c 'Trivial bool' maintainerconfig/maintainerconfig.go
grep -c 'yaml:"trivial"' maintainerconfig/maintainerconfig.go
grep -c 'autoMerge.trivial: true -> Trivial true' maintainerconfig/maintainerconfig_test.go
grep -c 'autoMerge absent -> Trivial false' maintainerconfig/maintainerconfig_test.go
grep -c 'autoMerge.trivial: non-bool -> strict error' maintainerconfig/maintainerconfig_test.go
grep -c 'autoMerge' CHANGELOG.md
grep -c 'autoMerge' README.md
grep -c 'fmt\.Errorf' maintainerconfig/maintainerconfig.go
</verification>
