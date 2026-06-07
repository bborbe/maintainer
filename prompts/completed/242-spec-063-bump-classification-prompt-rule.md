---
status: completed
spec: [063-releaser-no-major-bump]
summary: Added the pre-1.0 cap rule to bump_classification.md, the new Ginkgo Describe block + DescribeTable entry in prompts_test.go, and the CHANGELOG Unreleased bullet. All 57 prompts_test.go specs pass via `make test`; prompts.go was not touched.
container: maintainer-no-major-bump-exec-242-spec-063-bump-classification-prompt-rule
dark-factory-version: v0.175.0
created: "2026-06-06T21:46:08Z"
queued: "2026-06-07T14:26:58Z"
started: "2026-06-07T14:27:00Z"
completed: "2026-06-07T14:31:20Z"
branch: dark-factory/releaser-no-major-bump
---

<summary>
- The embedded bump-classification prompt in `agent/github-releaser/pkg/prompts/bump_classification.md` gains a new rule that caps `bump: major` at `minor` whenever the upstream `current_version` is a pre-1.0 release stream (`0.x` or `v0.x`)
- The new rule is positioned after the existing `major → minor → patch` priority order so the post-1.0 decision table is unchanged; the cap fires only when the version prefix matches the pre-1.0 pattern
- The cap rule is prefix-based and explicit: `0.69.0`, `v0.69.0`, `v0.69.0-rc1`, and `0.0.0` all match; bare `0` and bare `v0` (no dot) do not — those fall through to the existing `bad_current_version` precondition rather than getting a wrong cap
- The Claude `reasoning` string for capped pre-1.0 breaking changes must mention the word "pre-1.0" so the operator audit trail on the task page names the downgrade
- The existing prompt assertions in `prompts_test.go` (`patch | minor | major`, `BREAKING CHANGE`, `feat:`, `"bump":`, `major → minor → patch`) all continue to pass — the change is purely additive inside the markdown
- A new `prompts_test.go` Describe block asserts: prompt mentions both `0.x` and `v0.x` patterns; prompt names `pre-1.0`; priority order is preserved; a new table entry exercises the verdict parser against a stubbed Claude response for a capped pre-1.0 breaking change and asserts `bump: minor` + reasoning contains "pre-1.0"
- No Go function signatures change in this prompt — `BumpClassificationPrompt()`, `BumpVerdict`, and `ParseBumpVerdict` stay byte-identical
</summary>

<objective>
Teach the github-releaser bump classifier (the embedded Claude prompt + the typed verdict parser) about the semver convention that breaking changes in a 0.x release stream are released as `minor`, not `major`. The classifier's reasoning string for a pre-1.0 breaking change must name the downgrade so operators can audit why a `refactor: rename X → Y` bullet resolved to `minor` instead of `patch` (or, equivalently, why a `BREAKING CHANGE` bullet did not become `major`).

The new rule is purely a prompt edit + a new test block. The Go `applyMajorBumpGuard` decision table, `BumpVerdict` struct, `ParseBumpVerdict` validation, and the `BumpClassificationPrompt()` exported function all stay byte-identical — the cap is enforced by Claude, not by a Go code path. The `## Current version` injection into the assembled prompt is prompt 2's job (separate file); this prompt ships only the rule text + assertions.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions, and `/workspace/agent/github-releaser/pkg/prompts/bump_classification.md` for the current ruleset. Also read the existing test file at `/workspace/agent/github-releaser/pkg/prompts/prompts_test.go` to mirror the assertion style (Ginkgo v2 + Gomega, `Describe` + `It` + `DescribeTable` + `Entry`).

The current ruleset (full file, 36 lines):

```
# Classify the next semantic-version bump

You are classifying a software release. Given the CHANGELOG bullets for the upcoming
release, decide whether the next version bump is `patch`, `minor`, or `major`.

## Rules (apply in order)

Evaluate the bullets in priority order: major → minor → patch. The FIRST rule that
matches wins. Do not pick a weaker bump when a stronger one applies.

1. **major** — at least one bullet describes a BREAKING CHANGE: a removed or renamed
   public API, an incompatible behavior change, a config key removal, a database
   migration that is not backwards compatible, or any change that requires callers
   to update their code or configuration.
2. **minor** — at least one bullet starts with `feat:` or otherwise describes a new
   additive capability (new flag, new endpoint, new exported function) that does
   NOT break existing callers.
3. **patch** — everything else: bug fixes, refactors, doc edits, dependency bumps,
   test additions, internal cleanup.

Note: if a bullet contains BOTH a `feat:` prefix AND the literal text `BREAKING CHANGE`,
the correct answer is `major` — priority order is strict.

## Output
...
```

The new rule inserts after the "Note:" paragraph and before the "## Output" heading. It names the prefix match (`0.*` / `v0.*`) and the downgrade semantics ("strongest allowed bump is `minor`") and the audit-trail requirement ("reasoning string must mention `pre-1.0`"). The wording is the load-bearing contract for the LLM; tighten it, do not paraphrase.

The spec defines the prefix as **literal prefix-based**: the value must start with `0.` or `v0.` exactly. `0.69.0`, `v0.69.0`, `v0.69.0-rc1`, and `0.0.0` all match. `0` and `v0` (no dot) are treated as malformed — they do NOT match the cap pattern, and the existing `bad_current_version` precondition in the planning step fires instead. The rule text in the markdown must state this so Claude can follow it.

The `## Output` section already documents the JSON shape (`bump` ∈ `patch | minor | major`, `reasoning` non-empty). The new rule must NOT change that schema — `major` is still a legal verdict for post-1.0 inputs; the rule just caps it for pre-1.0 inputs.

Existing test assertions in `prompts_test.go` that MUST still pass (line numbers from the current file):

- `BumpClassificationPrompt returns non-empty string` (line 17)
- `contains patch | minor | major` (line 22)
- `contains BREAKING CHANGE` (line 27)
- `contains feat:` (line 32)
- `contains bump field` (line 37)
- `contains major → minor → patch priority order` (line 42)

The spec's Acceptance Criteria (AC 1-3) for this prompt are:

> AC 1: `BumpClassificationPrompt()` embedded text contains a rule capping pre-1.0 inputs at `minor` — evidence: `grep -nE 'pre-1\.0|0\.\*|v0\.\*' agent/github-releaser/pkg/prompts/bump_classification.md` returns ≥1 line.
>
> AC 2: The pre-1.0 cap rule names both `0.x` and `v0.x` patterns — evidence: prompt contains both literal forms (grep returns each).
>
> AC 3: The prompt's existing major → minor → patch priority order text is still present — evidence: existing test `contains major → minor → patch priority order` still passes (`go test ./agent/github-releaser/pkg/prompts/...` exit 0).

Coding guidance (mirror existing project patterns; don't import external docs):

- Ginkgo v2 / Gomega test style — mirror the existing `Describe` + `It` + `DescribeTable` shape already present in `/workspace/agent/github-releaser/pkg/prompts/prompts_test.go`.
- CHANGELOG entry — add a single `test:` prefixed bullet under `## Unreleased` (create the heading if absent).
</context>

<requirements>

1. **Add the pre-1.0 cap rule to `bump_classification.md`.** Insert a new rule section immediately AFTER the existing "Note: if a bullet contains BOTH a `feat:` prefix AND the literal text `BREAKING CHANGE`, the correct answer is `major` — priority order is strict." paragraph and BEFORE the "## Output" heading. Use exactly the wording below — the exact phrasing is the contract the LLM follows; do not paraphrase. Keep the existing `## Rules (apply in order)` heading and numbered list intact (the new section is a separate sub-heading so the priority order is unchanged for post-1.0 inputs):

   ```markdown
   ## Pre-1.0 cap

   If the release is on a pre-1.0 stream — meaning the `current_version` you are given
   starts with the literal prefix `0.` or `v0.` (for example `0.69.0`, `v0.69.0`,
   `v0.69.0-rc1`, or `0.0.0`) — you MUST NOT return `bump: major`. The strongest
   allowed bump is `minor`: a breaking-change bullet resolves to `minor` (not
   `major`) and your `reasoning` string MUST mention `pre-1.0` so the operator can
   audit the downgrade.

   The prefix is literal and exact: `0.` and `v0.` are the only patterns that trigger
   this cap. A bare `0` or `v0` (no dot) does NOT match — treat those as malformed
   input and follow the existing priority order. The post-1.0 priority order above
   (major → minor → patch) is unchanged for `current_version` of `1.*`, `v1.*`, or
   higher.
   ```

   Acceptance evidence: the file contains the literal substrings `pre-1.0`, `0.`, and `v0.` (the spec's AC 1 grep is `grep -nE 'pre-1\.0|0\.\*|v0\.\*' ...`; that grep pattern requires backslash-escaped dots because it scans for `0.*` / `v0.*` glob-style patterns, but the actual rule text contains the literal `0.` and `v0.` with trailing dots — both forms match under that grep).

2. **Do NOT touch the `## Output` section.** The JSON shape (`bump` ∈ `patch | minor | major`, `reasoning` non-empty) stays byte-identical. `major` is still a legal verdict for post-1.0 inputs — the cap is a runtime policy applied to the LLM, not a type-level constraint on the schema.

3. **Do NOT touch `prompts.go`.** The `BumpClassificationPrompt()` function, the `BumpVerdict` struct, `ParseBumpVerdict`, and `validateVerdict` all stay byte-identical. The cap is policy in the prompt text, not enforcement in Go.

4. **Add Ginkgo assertions for the new rule.** In `/workspace/agent/github-releaser/pkg/prompts/prompts_test.go`, append a new `Describe` block immediately after the existing `var _ = Describe("BumpClassificationPrompt", func() { ... })` block (which currently ends at line 46). The new block, mirroring the existing `It(...)` style:

   ```go
   var _ = Describe("BumpClassificationPrompt pre-1.0 cap (spec 063)", func() {
       It("names pre-1.0 in the rule text", func() {
           p := prompts.BumpClassificationPrompt()
           Expect(p).To(ContainSubstring("pre-1.0"))
       })

       It("names the 0.x prefix pattern", func() {
           p := prompts.BumpClassificationPrompt()
           Expect(p).To(ContainSubstring("0."))
       })

       It("names the v0.x prefix pattern", func() {
           p := prompts.BumpClassificationPrompt()
           Expect(p).To(ContainSubstring("v0."))
       })

       It("states major is forbidden for pre-1.0", func() {
           p := prompts.BumpClassificationPrompt()
           Expect(p).To(ContainSubstring("MUST NOT return `bump: major`"))
       })

       It("states minor is the strongest allowed bump", func() {
           p := prompts.BumpClassificationPrompt()
           Expect(p).To(ContainSubstring("strongest allowed bump is `minor`"))
       })

       It("states reasoning must mention pre-1.0 for audit trail", func() {
           p := prompts.BumpClassificationPrompt()
           Expect(p).To(ContainSubstring("reasoning"))
           Expect(p).To(ContainSubstring("`pre-1.0`"))
       })

       It("preserves the major → minor → patch priority order", func() {
           p := prompts.BumpClassificationPrompt()
           Expect(p).To(ContainSubstring("major → minor → patch"))
       })
   })
   ```

   All seven `It` cases must pass. AC 1 + AC 2 + AC 3 are satisfied by this block.

5. **Add a Ginkgo table entry that exercises the verdict parser against a capped pre-1.0 response.** The fixture simulates Claude following the new rule on a pre-1.0 breaking change: bump `minor`, reasoning that names the downgrade. Append ONE new `Entry(...)` to the existing `DescribeTable("ParseBumpVerdict", ...)` block (the block currently ends at line 90). The new entry, mirroring the existing "plain JSON parsed" entry shape:

   ```go
   Entry("pre-1.0 breaking change capped to minor (spec 063)",
       `{"bump":"minor","reasoning":"breaking change capped to minor due to pre-1.0 stream (current_version 0.69.0)"}`,
       "minor", "breaking change capped to minor due to pre-1.0 stream (current_version 0.69.0)", ""),
   ```

   The parser contract is unchanged — this entry only proves that a verdict shaped like the one Claude is now expected to produce on a pre-1.0 breaking change round-trips cleanly through `ParseBumpVerdict`. AC 6 is partially satisfied here; the planning-step end-to-end fixture is in prompt 3.

6. **Do NOT add a `BumpClassificationPromptFor(currentVersion)` helper in this prompt.** The spec *permits* such a helper but the choice belongs to prompt 2 (`spec-063-inject-current-version.md`), which decides whether to add a helper or assemble the prompt inline. This prompt scope: prompt-text edit + assertions only. The lib package stays a pure `//go:embed` + `BumpClassificationPrompt()` accessor.

7. **Add a `## Unreleased` entry to `CHANGELOG.md`.** Append a single bullet to the existing `## Unreleased` block (create the block if absent; the current `CHANGELOG.md` has its most recent `## v0.34.0` at the top — `## Unreleased` is missing). The entry:

   ```
   - feat(agent/github-releaser): teach the bump classifier about the pre-1.0 cap — pre-1.0 projects (current_version starting with `0.` or `v0.`) now have `major` capped at `minor`; the LLM records the downgrade in its `reasoning` string. Post-1.0 behavior is unchanged. The Go `applyMajorBumpGuard` stays as the durable safety net (spec 063)
   ```

8. **Run `make test` in the changed module.** From repo root: `cd /workspace/agent/github-releaser && make test`. Expected: exit code 0; all pre-existing assertions still pass; all 7 new `It` cases in the new `Describe` block pass; the new `DescribeTable` entry passes. Do NOT run `make precommit` in this prompt — prompt 3 covers that gate for the full spec.

9. **YAGNI guard.** Do NOT add a `BumpClassificationPromptFor(currentVersion string) string` helper to `prompts.go`; the spec forbids it. Do NOT add a CLI flag, env var, or per-repo config to opt out of the cap; the spec § Non-goals forbids it. Do NOT touch the `BumpVerdict` schema or `ParseBumpVerdict` validation rules; the spec § Constraints forbids it. Do NOT change the existing `## Rules (apply in order)` heading or reorder the three numbered rules; the post-1.0 priority order must be byte-identical.
</requirements>

<constraints>
- `BumpClassificationPrompt()` exported function and the `//go:embed bump_classification.md` directive MUST stay byte-identical (no signature change, no helper added, no file path change).
- `BumpVerdict` struct, `ParseBumpVerdict` validation, and `validateVerdict` MUST stay byte-identical — no new fields, no new validation branches, no error-message changes.
- The existing `## Rules (apply in order)` heading and the three numbered rules (major, minor, patch) MUST stay in the same order with the same wording. The new pre-1.0 cap rule is a separate sub-section inserted AFTER the "Note:" paragraph and BEFORE the `## Output` heading.
- The `## Output` section (JSON shape, the literal `patch | minor | major` string, the fenced JSON example) MUST stay byte-identical.
- All six existing `It(...)` cases in the `BumpClassificationPrompt` Describe block MUST continue to pass unmodified.
- The `prompts.go` file MUST NOT import any new package; the only existing import (`github.com/bborbe/errors`) is unchanged.
- Do NOT add a CLI flag, env var, per-repo config, or per-PR override to the prompts lib or the agent binary in this prompt. Spec 063 § Non-goals forbids all of these.
- Do NOT touch `agent/github-releaser/pkg/steps_planning.go` in this prompt — the prompt-assembly change is prompt 2's job. The `## Current version` section injection belongs to prompt 2; this prompt ships only the rule text + assertions.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass — the change is purely additive (one new `Describe` block + one new `DescribeTable` entry).
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && go test ./pkg/prompts/... -v -count=1
```
Expected: exit code 0. All pre-existing assertions in `prompts_test.go` still pass. The 7 new `It` cases in the new `Describe("BumpClassificationPrompt pre-1.0 cap (spec 063)", ...)` block pass. The new `Entry("pre-1.0 breaking change capped to minor (spec 063)", ...)` row in the `DescribeTable("ParseBumpVerdict", ...)` block passes.

Evidence commands the auditor will run:
- `grep -nE 'pre-1\.0|0\.\*|v0\.\*' /workspace/agent/github-releaser/pkg/prompts/bump_classification.md` → ≥ 1 line (the spec's AC 1 grep).
- `grep -n '0\.' /workspace/agent/github-releaser/pkg/prompts/bump_classification.md` → contains the `0.` literal (AC 2 first half).
- `grep -n 'v0\.' /workspace/agent/github-releaser/pkg/prompts/bump_classification.md` → contains the `v0.` literal (AC 2 second half).
- `grep -n 'major → minor → patch' /workspace/agent/github-releaser/pkg/prompts/bump_classification.md` → still 1 line (AC 3 sanity — the priority order text is preserved).
- `cd /workspace/agent/github-releaser && go test ./pkg/prompts/...` → exit code 0.
- `git diff --stat HEAD -- /workspace/agent/github-releaser/pkg/prompts/` shows exactly two files touched: `bump_classification.md` (added the rule section) and `prompts_test.go` (added one Describe block + one DescribeTable entry). `prompts.go` is UNTOUCHED.
</verification>
