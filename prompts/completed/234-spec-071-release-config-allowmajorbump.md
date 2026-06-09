---
status: completed
spec: ["071"]
summary: Added AllowMajorBump field to lib/maintainerconfig ReleaseConfig with godoc, 3 DescribeTable entries, 1 strict-parse It case, and CHANGELOG entry; all 22 specs pass.
container: maintainer-major-bump-guard-exec-234-spec-060-release-config-allowmajorbump
dark-factory-version: v0.175.0
created: "2026-06-03T15:05:00Z"
queued: "2026-06-03T14:34:34Z"
started: "2026-06-03T14:34:35Z"
completed: "2026-06-03T14:38:03Z"
branch: dark-factory/github-releaser-major-bump-guard
---

<summary>
- `.maintainer.yaml` schema gains a new boolean field `release.allowMajorBump` (default `false`), added alongside the existing `autoRelease` and `changelogRewrite` fields in the same `ReleaseConfig` struct
- A repo owner can opt in to automatic major-version releases by setting `release.allowMajorBump: true` in `.maintainer.yaml`; the github-releaser-agent will refuse to cut a `major` tag without this opt-in (or the per-run CLI override shipped in prompt 2)
- Unknown / missing / `false` / `true` values parse cleanly; non-boolean values (string, number) fail loudly via the strict parser with a wrapped error that the planning step will surface as `error_category=invalid_config`
- The lenient `Parse` path and the strict `ParseStrict` path both accept the new field — `ParseStrict` is the planning step's consumer
- Adds Ginkgo coverage for: `release.allowMajorBump: true` resolves to `AllowMajorBump true`, `release:` present but field absent resolves to `AllowMajorBump false` (default), and the strict-parse non-bool rejection path
- This is the data-shape slice of spec 060; the CLI flag plumbing, the guard decision table, the Ginkgo decision-table tests, and the README/CHANGELOG updates all depend on this prompt having shipped first
</summary>

<objective>
Extend `lib/maintainerconfig.ReleaseConfig` with a new boolean field `AllowMajorBump` (default `false`), and lock the parse contract so both the lenient `Parse` and the strict `ParseStrict` paths accept the new field cleanly. The planning step (prompt 3) will read `cfg.Release.AllowMajorBump` to decide whether a `major` bump verdict can proceed without operator override; the CLI flag (prompt 2) supplies the second lever. This prompt ships ONLY the schema — no fetcher, no factory wiring, no planning-step edits.
</objective>

<context>
Read `CLAUDE.md` and `agent/github-releaser/CLAUDE.md` for project conventions.

Read these files BEFORE editing:
- `/workspace/lib/maintainerconfig/maintainerconfig.go` — the existing schema with `MaintainerConfig { Release, PrReviewer }` namespaces. The `ReleaseConfig` struct already has `AutoRelease` and `ChangelogRewrite` fields with `yaml:` tags. Add the new field to `ReleaseConfig` alongside them.
- `/workspace/lib/maintainerconfig/maintainerconfig_test.go` — the existing Ginkgo style (DescribeTable for valid documents, plain `It` for the malformed / strict-reject cases). The spec 060 acceptance criteria name three table entries plus a strict-reject case; mirror the existing entry style (e.g. the `release.changelogRewrite: true -> ChangelogRewrite true` and `release: present but no changelogRewrite field -> ChangelogRewrite false (default)` entries are the shape template).
- `/workspace/specs/in-progress/060-github-releaser-major-bump-guard.md` — the spec under implementation. § Goal, § Non-goals, § Desired Behavior 1, § Constraints, and § Acceptance Criteria 3-7 are the load-bearing references.

Read these coding plugin guides (in-container paths — the prompt runs inside a YOLO container):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

Verified symbols (from module source — grep-confirmed):
- `maintainerconfig.Parse(ctx, []byte) (MaintainerConfig, error)` and `maintainerconfig.ParseStrict(ctx, []byte) (MaintainerConfig, error)` from `github.com/bborbe/maintainer/lib/maintainerconfig`. Both delegate to `parseInternal` with a `strict bool`; the new field is consumed by both paths unchanged.
- `ReleaseConfig` is `type ReleaseConfig struct { AutoRelease bool; ChangelogRewrite bool }` with yaml tags `yaml:"autoRelease"` and `yaml:"changelogRewrite"`. Add a third field `AllowMajorBump bool \`yaml:"allowMajorBump"\`` so the tag mirrors the spec's docs and AC evidence (`grep -c 'yaml:"allowMajorBump"'` returns 1).
- `gopkg.in/yaml.v3` is a direct dep of `lib/maintainerconfig`. yaml.v3 rejects non-bool values for a typed-`bool` field with the same `cannot unmarshal X into Go bool` fragment that the existing `changelogRewrite` field relies on — no custom validation needed in the lib.
- `github.com/bborbe/errors` is the only error-handling import; the planning step (prompt 3) will surface the parse error via `errors.Wrapf(ctx, err, "parse .maintainer.yaml")`. The lib's `parseInternal` already wraps with `errors.Wrap(ctx, err, "unmarshal .maintainer.yaml")` on the `dec.Decode` error path. No new error wrapping is required in this prompt.
</context>

<requirements>

1. **Add the `AllowMajorBump` field to `ReleaseConfig`.** In `/workspace/lib/maintainerconfig/maintainerconfig.go`, extend the existing struct. Insert the new field immediately AFTER `ChangelogRewrite` (it is a third release-policy opt-in flag and reads naturally in that position). The exact field declaration:

   ```go
   // AllowMajorBump is the spec-060 per-repo opt-in for automatic major-version
   // releases. Default false (omit the field, set false explicitly, or omit the
   // `release:` block — all equivalent). When false, the github-releaser-agent
   // planning phase will TRIP (Status=NeedsInput, ## Plan outcome=needs_input,
   // precondition_failed=major_bump_not_allowed) on any classifier verdict of
   // `bump=major`, forcing a human ack before tag + push. When true, a major
   // verdict proceeds to execution as before. The second lever is the
   // `--allow-major` CLI flag on the agent binary (env `ALLOW_MJOR`) — either
   // source is sufficient. Non-boolean values fail at parse time; the planning
   // step is responsible for surfacing the error as `error_category=invalid_config`.
   // See spec 060 § Desired Behavior 1 and § Goal.
   AllowMajorBump bool `yaml:"allowMajorBump"`
   ```

   Do NOT touch `PrReviewerConfig` or any other existing field/tag. Do NOT change the `MaintainerConfig` struct. Do NOT change the package-level `Parse` / `ParseStrict` function signatures or their error-wrapping prefix `"unmarshal .maintainer.yaml"` (existing tests grep for that literal substring). Do NOT add a CLI flag or env-var override to the lib (the lib is a pure schema + parser; CLI plumbing is prompt 2's job, owned by the agent binary).

2. **Update the `ReleaseConfig` godoc.** The existing struct godoc explains `AutoRelease` and `ChangelogRewrite`. Extend it with a paragraph naming `AllowMajorBump` and the spec cross-reference, mirroring the existing prose style. Do not rewrite the whole comment — just add a paragraph that names the new field's purpose, default, and the trip/override semantics. This keeps the doc-comment a load-bearing surface for future maintainers.

3. **Add Ginkgo coverage for the new field.** In `/workspace/lib/maintainerconfig/maintainerconfig_test.go`, add THREE new entries to the existing `DescribeTable("valid documents", ...)` block (do NOT add a new DescribeTable — extend the existing one to keep the parser tests in one place). Mirror the exact entry-name style of the existing `release.changelogRewrite: true -> ChangelogRewrite true` and `release: present but no changelogRewrite field -> ChangelogRewrite false (default)` entries. The new entries, in order:

   a. Entry named exactly `release.allowMajorBump: true -> AllowMajorBump true` — input `release:\n  allowMajorBump: true\n`, expect `maintainerconfig.MaintainerConfig{Release: maintainerconfig.ReleaseConfig{AutoRelease: false, ChangelogRewrite: false, AllowMajorBump: true}}`. The acceptance criterion `grep -c 'release.allowMajorBump: true -> AllowMajorBump true' lib/maintainerconfig/maintainerconfig_test.go` returns 1 — keep the entry name verbatim so the AC grep matches.

   b. Entry named exactly `release: present but no allowMajorBump field -> AllowMajorBump false (default)` — input `release:\n  autoRelease: true\n`, expect `maintainerconfig.Maintainerconfig{Release: maintainerconfig.ReleaseConfig{AutoRelease: true, ChangelogRewrite: false, AllowMajorBump: false}}`. The acceptance criterion `grep -c 'present but no allowMajorBump field' lib/maintainerconfig/maintainerconfig_test.go` returns 1 — keep the substring `present but no allowMajorBump field` verbatim.

   c. Entry named exactly `no release: block -> AllowMajorBump false` — input `prReviewer:\n  autoApprove: true\n`, expect `maintainerconfig.MaintainerConfig{PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true}, Release: maintainerconfig.ReleaseConfig{AllowMajorBump: false}}`. Mirrors the existing `no release: block -> ChangelogRewrite false` entry to lock the "absent `release:` block" default path. (This entry is the spec's "Field absent, file absent, value false — all equivalent" guarantee.)

4. **Add Ginkgo coverage for the strict-parse rejection of non-bool `allowMajorBump`.** Add a new `It(...)` case in the existing `var _ = Describe("Parse", func() {...})` block (alongside the existing `release.changelogRewrite: non-bool string value -> wrapped error` and `release.changelogRewrite: number value -> wrapped error` cases). The new case, using the same shape:

   ```go
   It("release.allowMajorBump: non-bool -> strict error", func() {
       cfg, err := maintainerconfig.ParseStrict(
           ctx,
           []byte("release:\n  allowMajorBump: \"yes\"\n"),
       )
       Expect(err).To(HaveOccurred())
       Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
       Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
   })
   ```

   The acceptance criterion `grep -c 'allowMajorBump: non-bool -> strict error' lib/maintainerconfig/maintainerconfig_test.go` returns 1 — keep the substring `allowMajorBump: non-bool -> strict error` verbatim in the `It(...)` name. Use `ParseStrict` (not `Parse`) because the spec is explicit that the planning step consumes the strict path. Mirrors the existing `It("ParseStrict rejects typo in nested release field", ...)` test (which uses `ParseStrict` for the `changelogRwrite` typo case) so the strict-only behavior of the new field is locked.

5. **Do NOT add tests for the lenient `Parse` non-bool path.** Spec 059 already proved that yaml.v3's `cannot unmarshal X into bool` fragment surfaces through the same `Parse` -> `errors.Wrap` path for `changelogRewrite`. The same path applies to `allowMajorBump` by symmetry — adding a redundant test would be pure duplication. If a future maintainer breaks the lenient-parse non-bool path, the existing `changelogRewrite: non-bool string value -> wrapped error` test will fail and surface the regression.

6. **Acceptance gate — `go test ./...` exits 0 in `lib/`.** Run `cd /workspace/lib && go test ./maintainerconfig/...` and confirm exit code 0. Investigate and fix any failures. `go mod tidy` is a no-op (no new imports). Do NOT run `make precommit` in this prompt — that runs the full lint/gosec/trivy suite and is slow; the parser tests are the load-bearing boundary for this prompt and the faster `go test` loop catches the regressions first. Prompt 2's precommit run will cover the cross-package integration.

7. **Cross-prompt dependency declaration.** This prompt ships only the schema. Prompt 2 (CLI flag plumbing) and prompt 3 (guard logic + PlanOutput fields) both depend on the `ReleaseConfig.AllowMajorBump` symbol being available. Prompt 4 (Ginkgo decision-table tests) and prompt 5 (README + CHANGELOG) build on prompts 2-3. If the build at any later prompt fails with `cfg.Release.AllowMajorBump undefined`, the daemon should re-queue prompt 1 — do not stub the field or define a placeholder.
</requirements>

<constraints>
- The new `AllowMajorBump` field MUST default to `false` on any of: empty bytes, missing `release:` block, missing `allowMajorBump` field, explicit `false`. This is the load-bearing rollout-safety invariant per spec 060 § Constraints — verify by reading the Ginkgo test entries.
- The `yaml:"allowMajorBump"` tag name is FIXED; the spec's AC `grep -c 'yaml:"allowMajorBump"' lib/maintainerconfig/maintainerconfig.go` returns 1. Do not rename to `yaml:"allowMajor"` or any other form.
- The new `ReleaseConfig.AllowMajorBump` is read by the planning step (prompt 3) via `cfg.Release.AllowMajorBump` — the field name is also FIXED. The spec's AC `grep -c 'AllowMajorBump' lib/maintainerconfig/maintainerconfig.go` returns ≥ 1.
- Non-boolean values fail at parse time via the type system (yaml.v3's `cannot unmarshal X into Go bool` rejection surfaces through `parseInternal` -> `errors.Wrap(ctx, err, "unmarshal .maintainer.yaml")`). The planning step (prompt 3) is responsible for surfacing the error as `error_category=invalid_config`; do NOT add custom validation logic in the lib.
- The lib package is consumed by ALL maintainer bots (release watcher, pr-reviewer, …). Do NOT add agent-specific fields outside the existing `ReleaseConfig` / `PrReviewerConfig` namespaces.
- Do NOT add a CLI flag, env-var override, or per-PR/per-task override at the lib layer. The lib is a pure schema + parser; the CLI flag ships in prompt 2 owned by the agent binary.
- Do NOT add a `release.allowMinorBump` or `release.allowPatchBump` knob. Spec 060 § Non-goals explicitly forbids the symmetric knobs — the guard is specific to `major` only.
- Do NOT add Prometheus metrics, debug logging, or other observability in the lib. Lib code is pure schema + parse.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass (the three new `DescribeTable` entries plus the new `It` case must not break the existing 11+ `Entry(...)` lines or the existing 4 `It(...)` cases).
</constraints>

<verification>
```
cd /workspace/lib && go test ./maintainerconfig/...
```
Expected: exit code 0; the three new `DescribeTable` entries pass; the new `It("release.allowMajorBump: non-bool -> strict error", ...)` case passes; the existing 11+ `Entry(...)` lines and 4 `It(...)` cases still pass.

Evidence commands the auditor will run:
- `grep -n 'AllowMajorBump' /workspace/lib/maintainerconfig/maintainerconfig.go` → exactly ONE struct field declaration, with the documented yaml tag and a doc-comment paragraph.
- `grep -n 'yaml:"allowMajorBump"' /workspace/lib/maintainerconfig/maintainerconfig.go` → exactly ONE occurrence (the struct field's yaml tag).
- `grep -c 'release.allowMajorBump: true -> AllowMajorBump true' /workspace/lib/maintainerconfig/maintainerconfig_test.go` → 1.
- `grep -c 'present but no allowMajorBump field' /workspace/lib/maintainerconfig/maintainerconfig_test.go` → 1.
- `grep -c 'allowMajorBump: non-bool -> strict error' /workspace/lib/maintainerconfig/maintainerconfig_test.go` → 1.
- `grep -c 'AllowMajorBump' /workspace/lib/maintainerconfig/maintainerconfig_test.go` → ≥ 3 (the three new entries plus the strict-reject `It` case reference the field).
- `git diff master -- lib/maintainerconfig/maintainerconfig.go | grep -c '^+.*fmt\.Errorf'` → 0 (no `fmt.Errorf` introduced; the lib uses `github.com/bborbe/errors` only).
- `cd /workspace/lib && go test ./maintainerconfig/...` → exit code 0.
</verification>
