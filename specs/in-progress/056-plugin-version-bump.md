---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-05-31T22:12:09Z"
generating: "2026-05-31T22:12:41Z"
prompted: "2026-05-31T22:20:38Z"
branch: dark-factory/plugin-version-bump
---

## Summary

- Today the github-releaser agent rewrites `CHANGELOG.md` and pushes an annotated tag, but it leaves Claude Code plugin manifest files (`.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`) pinned at the previous version. The released tag and CHANGELOG disagree with the manifests forever.
- Extend the release execution step so that, when those manifest files exist at the repo root, the same release commit also bumps their version fields to the new release version.
- Add a new pure-function sub-package that owns the detect-and-bump byte transform, mirroring the shape of the existing changelog sub-package.
- Widen the pre-push diff guard so it accepts exactly the set of files actually rewritten (changelog + whichever manifests existed pre-write), and fails closed on anything else.
- Behavior is unchanged for any repo without a `.claude-plugin/` directory.

## Problem

The release agent's release commit is supposed to represent "this is now version X". For Claude Code plugin repositories the agent's commit is incomplete: it ships a new tag and a new CHANGELOG heading while the plugin manifest files inside the same commit still advertise the previous version. Plugin consumers reading `marketplace.json` or `plugin.json` from the tagged commit therefore see a wrong version until somebody notices and patches by hand. This was first observed on `bborbe/coding` at tag `v0.10.0`, where both manifests remained at `0.9.12` after the release went out. Any future autoreleased plugin repo will repeat the failure.

## Goal

After this work, when the release execution step runs against a repo that contains `.claude-plugin/plugin.json` and/or `.claude-plugin/marketplace.json` at the repo root, the single release commit it produces contains the changelog rewrite **and** the version-field bumps in every detected manifest, both manifest files (if present) are written, all manifest version fields are advanced to the same unprefixed semver, the pre-push guard accepts exactly that set of files, and the tag is pushed. For repos with no `.claude-plugin/` directory the agent's behavior is byte-identical to today: commit touches `CHANGELOG.md` and nothing else.

## Non-goals

- Do NOT auto-detect whether a repo "is" a plugin to gate behavior — presence of a manifest file at the conventional path is the only signal; no allowlist, no flag.
- Do NOT add a per-feature opt-out / disable flag for plugin bumping — invariant; if a future consumer demands variation, that's a separate spec. (An escape hatch on the Goal is itself a regression.)
- Do NOT validate the plugin manifest schema beyond locating and rewriting the version field — full schema validation is out of scope.
- Do NOT support manifest formats other than the two Claude Code plugin manifests above (`package.json`, `Cargo.toml`, `pyproject.toml`, etc. are explicitly out of scope).
- Do NOT support manifests in non-root locations (e.g. `subdir/.claude-plugin/plugin.json`) — root only.
- Do NOT loosen the pre-push guard to "accept anything under `.claude-plugin/`" — the whitelist must be built from pre-write detection and compared exactly.
- Do NOT switch the manifest rewrite to a full JSON decode/encode round-trip — the rewrite must preserve byte-level structure (indentation, key order, trailing newline) so the commit diff is one line per version field.
- Do NOT add mono-repo (multiple plugins in subdirectories) support — single root manifest set only.

## Desired Behavior

1. When the release execution step runs and neither `.claude-plugin/plugin.json` nor `.claude-plugin/marketplace.json` exists at the repo root, the resulting release commit changes exactly `CHANGELOG.md` and the pre-push guard accepts it. (Today's behavior, preserved verbatim.)
2. When `.claude-plugin/plugin.json` exists at the repo root, the release commit also rewrites its top-level `version` field to the unprefixed semver of the release (e.g. `0.10.0`, not `v0.10.0` and not `## v0.10.0`). All other bytes of the file are byte-identical to the pre-write content (same indentation, same key order, same trailing newline, same line endings).
3. When `.claude-plugin/marketplace.json` exists at the repo root, the release commit also rewrites **both** `metadata.version` **and** the `version` field of every entry under `plugins[]` to the same unprefixed semver. All other bytes are byte-identical to the pre-write content. A marketplace containing N plugin entries has N+1 version-line changes in total.
4. When both manifests exist, both are written, both are committed in the same release commit, the pre-push guard accepts exactly `{CHANGELOG.md, .claude-plugin/plugin.json, .claude-plugin/marketplace.json}` and fails closed on any deviation.
5. The set of files passed to the commit operation is built explicitly by path (no `git add -A`, no globs). The commit operation receives `CHANGELOG.md` plus the subset of `{.claude-plugin/plugin.json, .claude-plugin/marketplace.json}` that existed pre-write, in that order.
6. The pre-push guard's expected-file whitelist is constructed from manifest detection performed **before** any write occurs in the execution step. A manifest file that appears between detection and commit is treated as an unexpected diff and the step fails closed with the existing `unexpected_diff` error category — nothing is tagged or pushed.
7. When a detected manifest is malformed (JSON parse error inside the version-field locator) or its version field is absent / not a quoted semver-shaped string, the execution step fails closed with a new error category `plugin_manifest_invalid`. No commit is made, nothing is tagged, nothing is pushed. The error message names the offending file path and the parse / location reason.
8. The version string written into every manifest version field is the unprefixed semver derived from the plan's `next_version_header` (e.g. plan header `## v0.10.0` → string `0.10.0`; plan header `## 0.10.0` → string `0.10.0`).
9. The pure-function manifest sub-package exposes a detect operation that returns the repo-relative paths of manifests that exist (subset of the two known paths, empty slice when none, never errors on absence), and a bump operation that takes the manifest content and the target version string and returns the rewritten content (errors on malformed input or missing version field). Neither function performs filesystem or network I/O.

## Constraints

- The existing direct-push trust model must be preserved: the release commit is mechanically constrained, and the pre-push guard fails closed on any unexpected file in the diff. This spec widens the whitelist; it does not loosen the guard's posture. Reference: [[Pre-Push Diff Guard Pattern]] (Obsidian vault, Personal).
- The `GitOps` interface in `pkg/git/git.go` is a frozen seam — its method signatures (notably `Commit(ctx, workdir, message, paths ...string)`) must not change. Variadic `paths` already supports passing multiple files.
- The new sub-package must mirror the shape of `pkg/changelog/`: pure-function, deterministic, byte-in/byte-out, no I/O, no globals beyond compiled regex tables, ctx threaded only for error wrapping consistency. Reference: `~/Documents/workspaces/coding/docs/go-patterns.md` (interface → constructor → struct).
- Error wrapping uses `github.com/bborbe/errors` only; never `fmt.Errorf`; every wrap takes `ctx`. Reference: `~/Documents/workspaces/coding/docs/go-error-wrapping-guide.md`.
- Tests use Ginkgo v2 + Gomega with `DescribeTable` / `Entry` for matrix cases; mocks use Counterfeiter (the existing `mocks/git_ops.go` generator directive). References: `~/Documents/workspaces/coding/docs/go-testing-guide.md`, `~/Documents/workspaces/coding/docs/go-mocking-guide.md`.
- The `ErrorCategory` enum in `pkg/git/error_classifier.go` is a **closed set** — adding `plugin_manifest_invalid` is owned by this spec and must be added as a typed constant (no string literals at the call site). The new category is **not** added to `classifierTable` (it is set directly by the execution step at the manifest-package layer, same split as `changelog_missing` and `unreleased_not_found`).
- The release-result markdown schema (`## Result` block, `outcome` / `error_category` keys) is owned by `pkg/result_output.go` — all `outcome=…` / `error_category=…` field references in ACs and Failure Modes match the writer in that file.
- The repo is a `.dark-factory.yaml` spec-flow repo. Prompts run in containers under `--set hideGit=true --set autoRelease=false` per [[Development Guide]] (Obsidian vault, Personal).
- The current working branch is `feature/plugin-version-bump` (upstream `origin/feature/plugin-version-bump`); all prompts target this branch.
- All existing tests under `agent/github-releaser/` must still pass unchanged.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility | Concurrency |
|---|---|---|---|---|---|
| Neither manifest exists at repo root | Detect returns empty slice; bump step is a no-op; commit, guard, tag, push behave exactly as today | None needed (success path) | `## Result.outcome=released`, commit touches only `CHANGELOG.md` | N/A | Per-task lock unchanged |
| One manifest exists, the other does not | Write only the existing one; commit file list includes only the present manifest; guard expects only the present manifest | None needed | Commit file set equals `{CHANGELOG.md, <present manifest>}` | N/A | Per-task lock unchanged |
| Manifest file is not valid JSON in the version-locator's scan | Fail closed with `error_category=plugin_manifest_invalid`; no `os.WriteFile` of that manifest; no commit; nothing tagged or pushed | Operator fixes the manifest by hand and re-triggers; controller retry cap applies | `## Result.outcome=failed`, `error_category=plugin_manifest_invalid`, error string contains the manifest path | Fully reversible — no commit was created | Per-task lock unchanged |
| Manifest's version field is missing or not a quoted semver-shaped string | Same as malformed: fail closed with `plugin_manifest_invalid`, no commit, no tag, no push | Same as malformed | Same as malformed | Fully reversible | Per-task lock unchanged |
| `marketplace.json` contains zero entries under `plugins[]` | Bump only `metadata.version`; commit proceeds | None needed | One line changed in `marketplace.json` (metadata only) | N/A | Per-task lock unchanged |
| `marketplace.json` contains multiple plugin entries | Bump `metadata.version` and every `plugins[].version`; commit proceeds | None needed | N+1 lines changed in `marketplace.json` for N entries | N/A | Per-task lock unchanged |
| A manifest file appears between pre-write detection and commit (e.g. another process wrote it) | Guard fails closed with `error_category=unexpected_diff`; nothing tagged or pushed | Operator investigates the unexpected writer; re-trigger | `## Result.outcome=failed`, `error_category=unexpected_diff` | Local commit exists in ephemeral workdir but workdir is removed on defer; nothing pushed | Workdir is per-task ephemeral; cross-task interference is impossible by construction |
| Disk write of a manifest fails mid-sequence (ENOSPC, EACCES) | Fail closed with `error_category=unknown` and wrapped cause naming the file path; no commit, no tag, no push | Operator addresses the disk condition; re-trigger | `## Result.outcome=failed`, `error_category=unknown`, error string names the manifest path | Fully reversible — workdir is removed on defer; the partial write is in the ephemeral workdir and was never committed | Workdir is per-task ephemeral |
| Process crashes between writing manifest(s) and commit | Workdir cleanup defer removes the workdir on next normal exit path; on hard crash the next run starts from a fresh clone | Controller re-triggers; fresh clone yields fresh detect-and-bump | Controller retry surface | Fully reversible — workdir is ephemeral and per-task | Per-task lock prevents concurrent runs on the same task |
| Plan's `next_version_header` lacks both `## ` prefix and `v` prefix (e.g. literal `0.10.0`) | The unprefixed semver derivation still yields `0.10.0`; manifests are bumped to `0.10.0`; no error | None needed | Manifests show `0.10.0` | N/A | N/A |

## Security / Abuse Cases

- **Input under attacker control:** the manifest files are part of the cloned target repo and therefore controlled by whoever can push to the upstream default branch. A crafted manifest could try to (a) cause unbounded scanning, (b) cause the rewrite to touch unintended lines, (c) trick the guard into accepting an unrelated diff.
- **Mitigations required:**
  - The version-locator scans line-by-line with bounded buffer (same scanner discipline as `pkg/changelog/`); files are bounded by the cloned repo's file size which the existing clone trust model already accepts.
  - The bump function writes **only** the version field it located; it never reformats other lines. A precise rewrite means the commit diff is the bumped lines and nothing else. The pre-push guard catches any deviation as `unexpected_diff`.
  - The pre-push guard's expected-file whitelist is built from detection that ran **before** any write — a manifest file that materializes mid-flight is treated as an unexpected diff and rejected. The whitelist is not derived from "files in `.claude-plugin/`" (which would let an attacker drop a new file into that directory and have it accepted).
  - Malformed / missing-version manifests fail closed with `plugin_manifest_invalid`; they never silently no-op.
- **Trust boundary:** the cloned repo's contents cross from "GitHub-controlled" into "agent-produced commit". The release commit is the load-bearing artifact; the guard's job is to keep that commit minimal and predictable.

## Acceptance Criteria

Evidence shape is named after each item.

- [ ] A new Go sub-package exists under `agent/github-releaser/pkg/` whose detect operation, given a workdir path, returns the subset of `[".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"]` that exist as regular files at the repo root, returns an empty slice when neither exists, and returns no error in either case — evidence: Ginkgo `DescribeTable` rows for (neither / plugin only / marketplace only / both) all pass under `make precommit` in the package directory (exit code 0).
- [ ] The same sub-package's bump operation, given `plugin.json` content and a target version string, returns content identical to the input except for the top-level `"version": "..."` line which carries the target version — evidence: Ginkgo test asserts `Expect(rewritten).To(Equal(fixtureExpected))` against a golden fixture; the diff between input and output is exactly one line.
- [ ] The same sub-package's bump operation, given `marketplace.json` content with `metadata.version` and N entries under `plugins[]`, returns content identical to the input except that `metadata.version` and every `plugins[].version` carry the target version — evidence: Ginkgo `DescribeTable` rows for N ∈ {0, 1, 3} all pass; the diff between input and output is exactly `N+1` lines (or 1 line when N=0).
- [ ] The bump operation returns an error (wrapped via `github.com/bborbe/errors`) when the input content is not valid JSON in the version-locator's scan, or when the target version field is absent, or when the located version value is not a quoted semver-shaped string — evidence: Ginkgo `DescribeTable` rows for each malformed case assert `Expect(err).To(HaveOccurred())` and `Expect(err.Error()).To(ContainSubstring("version"))`.
- [ ] A new constant `ErrorCategoryPluginManifestInvalid` with string value `"plugin_manifest_invalid"` exists in `pkg/git/error_classifier.go`, is **not** present in `classifierTable`, and is documented with the same two-layer-classification note as `ErrorCategoryUnexpectedDiff` (set directly by the execution step at the manifest-package layer, not by `ClassifyError`) — evidence: `grep -n 'ErrorCategoryPluginManifestInvalid' agent/github-releaser/pkg/git/error_classifier.go` returns at least one line in the const block and one line in a comment; `awk '/classifierTable/,/^}/' agent/github-releaser/pkg/git/error_classifier.go | grep -c 'plugin_manifest_invalid'` returns `0` (pure exit-code assertion — the new category is absent from the classifier-table literal).
- [ ] In `executeDirectPush` (in `agent/github-releaser/pkg/steps_execution.go`), the manifest detection call happens **before** the changelog `os.WriteFile`; the detected file list is captured in a local variable; that list is used both for the bump-and-write loop and for constructing the guard's expected-file whitelist — evidence: source inspection of the modified function; the detect call site appears at a line number lower than the changelog `os.WriteFile` call site; the expected-file whitelist passed (directly or indirectly) to `guardCommittedFiles` is the same slice value as the detection result plus `changelogFileName`.
- [ ] After `executeDirectPush` completes successfully against a fixture workdir containing both manifests with version `0.9.12` and a CHANGELOG with `## Unreleased`, running the same plan with `next_version_header = "## v0.10.0"`: `.claude-plugin/plugin.json` shows `"version": "0.10.0"` on the line that previously read `"version": "0.9.12"`, all other bytes of `plugin.json` are byte-identical to the pre-run file, and `.claude-plugin/marketplace.json` has every `version` line (`metadata.version` + every `plugins[].version`) rewritten to `"version": "0.10.0"` with all other bytes byte-identical to the pre-run file — evidence: Ginkgo integration test asserts file contents post-run via `Expect(actual).To(Equal(expected))` against checked-in golden fixtures at `agent/github-releaser/pkg/testdata/plugin.json.pre`, `plugin.json.post`, `marketplace.json.pre`, `marketplace.json.post`.
- [ ] After the same successful run, the mock `GitOps.CommittedFiles` returns exactly `["CHANGELOG.md", ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"]` (set equality; element order may vary), `guardCommittedFiles` returns nil, `GitOps.Tag` is invoked once, and `GitOps.Push` is invoked once — evidence: Ginkgo integration test asserts counterfeiter call-count and call-argument assertions on the mock.
- [ ] After running `executeDirectPush` against a fixture workdir with **no** `.claude-plugin/` directory, the mock `GitOps.Commit` receives exactly the variadic path argument `"CHANGELOG.md"` (and nothing else), `guardCommittedFiles` accepts a `CommittedFiles` return of `["CHANGELOG.md"]`, and the tag is pushed — evidence: Ginkgo integration test asserts mock call arguments equal `[]string{"CHANGELOG.md"}` (regression test for backward compatibility).
- [ ] After running `executeDirectPush` against a fixture workdir where the mock `GitOps.CommittedFiles` returns `["CHANGELOG.md", ".claude-plugin/plugin.json", "README.md"]` (an unexpected `README.md` slipped in), the step writes `## Result.outcome=failed` with `error_category=unexpected_diff`, `GitOps.Tag` is never invoked, and `GitOps.Push` is never invoked — evidence: Ginkgo integration test asserts mock `Tag` and `Push` call counts equal 0 and the resulting markdown's `## Result` section parses to `outcome=failed`, `error_category=unexpected_diff`.
- [ ] After running `executeDirectPush` against a fixture workdir whose `.claude-plugin/plugin.json` is malformed JSON, the step writes `## Result.outcome=failed` with `error_category=plugin_manifest_invalid`, the error message contains `.claude-plugin/plugin.json`, `GitOps.Commit` is never invoked, `GitOps.Tag` is never invoked, `GitOps.Push` is never invoked — evidence: Ginkgo integration test asserts mock `Commit`, `Tag`, `Push` call counts equal 0 and the markdown's `## Result` section parses to `outcome=failed`, `error_category=plugin_manifest_invalid`, error string contains the path substring.
- [ ] Running `make precommit` from inside `agent/github-releaser/` exits 0 — evidence: exit code 0.

**Scenario coverage:** NO new scenario. The behavior is fully reachable from a unit + integration test using the existing counterfeiter `GitOps` mock and golden manifest fixtures. There is no real `git`, no real container, no real GitHub interaction required to exercise the detect-bump-write-commit-guard path. Existing scenarios already cover the end-to-end direct-push flow.

## Verification

From the `agent/github-releaser/` directory of the worktree:

```
make precommit
```

Expected: exit code 0. The Ginkgo suite in the new sub-package and the modified execution-step suite both pass.

Per-case spot-checks (after a successful integration test run with both manifests present):

```
diff -u testdata/plugin.json.pre testdata/plugin.json.post
# expected: exactly one line changed, the "version" line, from 0.9.12 → 0.10.0
```

```
diff -u testdata/marketplace.json.pre testdata/marketplace.json.post
# expected: exactly N+1 lines changed (metadata.version + every plugins[].version), all from 0.9.12 → 0.10.0
```

## Do-Nothing Option

If we don't do this, every auto-released Claude Code plugin repo will continue to ship release commits whose plugin manifests disagree with the tag and the CHANGELOG. The mismatch will persist at every tagged commit until a human notices and patches it. The autorelease story for plugin repos is incomplete and trusts-as-evidence the manifests is unreliable. The triggering incident (`bborbe/coding` v0.10.0 shipped with manifests at `0.9.12`) demonstrates the failure is real and silent. Not acceptable for the autorelease fleet rollout.
