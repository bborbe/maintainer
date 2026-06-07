---
status: draft
spec: [063-releaser-no-major-bump]
created: "2026-06-06T21:46:08Z"
branch: dark-factory/releaser-no-major-bump
---

<summary>
- Three new Ginkgo cases in `agent/github-releaser/pkg/steps_planning_test.go` lock down the spec-063 behavioral envelope: the vault-cli regression fixture, the post-1.0 unchanged behavior, and the `applyMajorBumpGuard`-untouched invariant
- The vault-cli fixture replays the 2026-06-06 incident (`current_version: 0.69.0`, one `- refactor: rename /refine-task to /plan-task` bullet, stub runner returns `{"bump":"minor","reasoning":"...pre-1.0..."}`) and asserts `outcome: ready`, `bump: minor`, `next_version: 0.70.0` — the human-shipped release shape
- The post-1.0 case asserts that a `current_version: 1.2.3` with a breaking-change bullet, plus a stub runner that returns `bump: major`, still trips the guard with `outcome: needs_input` and `precondition_failed: major_bump_not_allowed` — the spec's negative-evidence requirement
- The third case asserts the guard source has zero non-whitespace diffs by importing a pre-prompt hash of the function body and comparing; this is a regression-guard test that fails the build if a future maintainer "helpfully" extends the guard to handle pre-1.0 in Go (the spec § Non-goals explicitly forbids that)
- The test patterns mirror the existing `Context("major-bump guard (spec 060)")` block (lines 1326-1603 in the current file) so the audit-trail style is consistent with prior spec work
- Final `make precommit` gate — exit code 0 confirms AC 10
</summary>

<objective>
Add the spec-063 behavioral fixtures that prove end-to-end correctness through the planning step. The pre-1.0 fixture replays the originating vault-cli 2026-06-06 incident and asserts the release proceeds unattended with `outcome: ready` and `next_version: 0.70.0` (the human-shipped shape). The post-1.0 case asserts the guard still trips for breaking changes on `>= 1.0.0` versions. The third case pins the `applyMajorBumpGuard` source to a pre-prompt hash so a future maintainer cannot "helpfully" extend the guard to handle pre-1.0 in Go code (the spec § Non-goals explicitly forbids modifying the guard).

The test fixtures exercise the real production path (`step.Run` → `runClassification` → `resolveBumpVerdict` → `applyMajorBumpGuard` → `publishPlan`) with mocked `Fetcher`, `MaintainerConfigFetcher`, and `ClaudeRunnerMock` — the same shape used by every existing `Context` block in `steps_planning_test.go`. The production code is UNTOUCHED in this prompt — the only diffs are three new `It` cases in the test file. This is the spec's coverage-locking prompt: prompts 1 and 2 ship the rule + assembly, this prompt locks the envelope so a future refactor cannot silently regress it.

After this prompt ships, `cd /workspace/agent/github-releaser && make precommit` exits 0 (AC 10).
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions.

Read these files fully BEFORE editing:

- `/workspace/agent/github-releaser/pkg/steps_planning.go` — the production code. `applyMajorBumpGuard` (lines 451-488) is the function whose body the third test pins. Read the function verbatim so the hash in the test matches the byte content at prompt-execution time. The decision table at lines 432-438 is what the post-1.0 fixture exercises.
- `/workspace/agent/github-releaser/pkg/steps_planning_test.go` — the test file. The `Context("major-bump guard (spec 060)")` block at lines 1326-1603 is the closest analog to the new fixtures — mirror its style (trip-case CHANGELOG fixture, mocked runner that forces `bump: major`, assert `outcome: needs_input` + `precondition_failed: major_bump_not_allowed` + the FROZEN spec-047 frontmatter mutations). The `Context("bump verdict cache (re-fire after rewrite LLM transient failure)")` block at lines 595-679 shows how to use `RunArgsForCall(N)` to inspect the prompt string — useful for the third test if the hash is computed by reading the source file at test time (rather than hard-coding the bytes). The `Context("happy path")` block at lines 42-87 shows the assertion shape for `outcome: ready` + `bump: <expected>` + `next_version: <expected>`.
- `/workspace/agent/github-releaser/pkg/plan_output.go` — the `PlanOutput` struct (lines 18-105) and the constants `PlanOutcomeReady`, `PlanOutcomeNeedsInput`, `PreconditionMajorBumpNotAllowed` (lines 107-142). The new tests assert against these constants — do NOT hard-code the string literals.
- `/workspace/agent/github-releaser/pkg/steps_mocks.go` — the counterfeiter directive. The `mocks.ClaudeRunnerMock` is generated; do NOT regenerate it in this prompt.

The spec's AC list for this prompt:

> AC 5: The cached-verdict short-circuit in `resolveBumpVerdict` is unchanged — evidence: `git diff HEAD -- agent/github-releaser/pkg/steps_planning.go` shows the only changes inside `resolveBumpVerdict` are within the `cachedBump == ""` branch (the cache-hit early return is untouched). Verifier confirms via reading the diff context lines around the existing `if cachedBump != ""` block.
>
> AC 6: A new `prompts_test.go` table entry asserts a fixture pre-1.0 breaking-change scenario yields `bump: minor` — evidence: the test invokes `ParseBumpVerdict` (or a new helper) on a stubbed Claude response and asserts `verdict.Bump == "minor"` and `verdict.Reasoning` contains the substring `pre-1.0`. Test passes (`go test -run Pre10 ./agent/github-releaser/pkg/prompts/...` exit 0). [NOTE: this AC was partially satisfied in prompt 1's `Entry("pre-1.0 breaking change capped to minor (spec 063)", ...)` — this prompt's end-to-end fixture completes the AC by exercising the planning step, not just the parser.]
>
> AC 7: A planning-step test replays the vault-cli fixture (current_version `0.69.0`, one bullet `- refactor: rename /refine-task to /plan-task`) using a stub runner that returns `{"bump":"minor","reasoning":"breaking change capped to minor due to pre-1.0 stream"}` and asserts the resulting `## Plan` outcome is `ready`, `bump` is `minor`, `next_version` is `0.70.0` — evidence: the test asserts `outcome: ready` and `next_version: 0.70.0` in the published markdown.
>
> AC 8: A planning-step test confirms a post-1.0 input (current_version `1.2.3`, breaking-change bullet) with stub runner returning `bump: major` still trips the guard — evidence: published markdown contains `outcome: needs_input` and `precondition_failed: major_bump_not_allowed`.
>
> AC 9: `applyMajorBumpGuard` source has no diff outside whitespace/comments — evidence: `git diff HEAD -- agent/github-releaser/pkg/steps_planning.go` shows zero changed lines inside the `func (s *planningStep) applyMajorBumpGuard` body (only the prompt-assembly call site upstream changes).
>
> AC 10: `make precommit` in the changed module exits 0 — evidence: exit code 0.

AC 5, 6, 7, 8, 9 are all about diff and test evidence. AC 10 is the make-precommit gate. The three new tests in this prompt collectively cover AC 6 (parser), AC 7 (vault-cli fixture), AC 8 (post-1.0 unchanged), and AC 9 (guard source pinned). AC 5 is verified by `git diff` — the third test makes the AC machine-checkable so a future refactor that accidentally reorders the cache-hit block fails the test.

Coding plugin guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2/Gomega style, `Context` + `It` shape, `BeforeEach` / `AfterEach` for setup.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — coverage rules: ≥80% on changed packages; the new tests add coverage to the existing already-tested functions so no new untested code is introduced.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — the `make precommit` targets: format + generate + test + lint + license. The full precommit is slow; use `make test` for fast iteration and run `make precommit` only at the end.
</context>

<requirements>

1. **Add the vault-cli fixture (pre-1.0, breaking change, expected `minor` bump to `0.70.0`).** In `/workspace/agent/github-releaser/pkg/steps_planning_test.go`, append a new `Context("pre-1.0 cap (spec 063)")` block inside the outermost `Describe("steps_planning", func() { ... })`. The block, mirroring the `Context("happy path")` shape (lines 42-87):

   ```go
   Context("pre-1.0 cap (spec 063)", func() {
       // Vault-cli 2026-06-06 regression: /refine-task → /plan-task rename
       // at v0.69.0 halted at planning with major_bump_not_allowed. The
       // spec-063 fix teaches the classifier to cap pre-1.0 breaking
       // changes at minor. This fixture replays the exact incident and
       // asserts the release proceeds unattended to outcome=ready with
       // next_version=0.70.0 (the human-shipped shape).
       vaultCliChangelog := []byte(
           "## Unreleased\n\n" +
               "- refactor: rename /refine-task to /plan-task\n\n" +
               "## v0.69.0\n\n- old\n",
       )

       It(
           "vault-cli v0.69.0 + rename bullet + Claude returns minor:pre-1.0 → outcome=ready, next_version=0.70.0",
           func() {
               fakeFetcher := &mocks.Fetcher{}
               fakeFetcher.FetchReturns(vaultCliChangelog, nil)

               fakeRunner := &mocks.ClaudeRunnerMock{}
               fakeRunner.RunReturns(
                   &claudelib.ClaudeResult{
                       Result: `{"bump":"minor","reasoning":"breaking change capped to minor due to pre-1.0 stream (current_version 0.69.0)"}`,
                   },
                   nil,
               )

               step := pkg.NewPlanningStep(
                   fakeRunner,
                   fakeFetcher,
                   &mocks.MaintainerConfigFetcher{},
                   false,
               )

               taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/vault-cli\nclone_url: https://github.com/bborbe/vault-cli.git\nref: master\ncurrent_version: 0.69.0\ntask_identifier: gh-release-bborbe-vault-cli-001\n---\n\n# release task\n"

               md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
               Expect(err).NotTo(HaveOccurred())

               result, err := step.Run(context.Background(), md)
               Expect(err).NotTo(HaveOccurred())

               // Status/NextPhase: planning succeeded, advance to execution.
               Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
               Expect(result.NextPhase).To(Equal("execution"))

               // ## Plan JSON content: outcome=ready, bump=minor, next_version=0.70.0.
               plan, err := agentlib.ExtractSection[pkg.PlanOutput](
                   context.Background(), md, "## Plan",
               )
               Expect(err).NotTo(HaveOccurred())
               Expect(plan.Outcome).To(Equal(pkg.PlanOutcomeReady))
               Expect(plan.Bump).To(Equal("minor"))
               Expect(plan.CurrentVersion).To(Equal("0.69.0"))
               Expect(plan.NextVersion).To(Equal("0.70.0"))
               Expect(plan.NextVersionHeader).To(Equal("## v0.70.0"))
               Expect(plan.HeaderPrefixStyle).To(Equal("v"))
               Expect(plan.PreconditionFailed).To(BeEmpty())

               // FROZEN spec-047 escalation contract is NOT triggered on
               // the happy path: status/phase unchanged, assignee is
               // untouched (the planning step does not mutate the
               // frontmatter on the success path; the controller's
               // status→frontmatter switch handles phase advance).
               gotStatus, _ := md.Frontmatter.String("status")
               Expect(gotStatus).To(Equal("in_progress"))
               gotPhase, _ := md.Frontmatter.String("phase")
               Expect(gotPhase).To(Equal("planning"))
           },
       )
   })
   ```

   The fixture's `current_version: 0.69.0` (no `v` prefix) is the spec's exact input — the spec § Desired Behavior 4 says the literal prefix-based match handles both `0.` and `v0.`. The fixture's CHANGELOG has `## v0.69.0` (with prefix) so `InferHeaderPrefixStyle` returns `"v"` and `NextVersionHeader` is composed as `## v0.70.0`. The bump math (`BumpVersion("0.69.0", "minor")`) returns the numeric `"0.70.0"` regardless of input prefix. The `next_version: 0.70.0` proves the bump math is right; the spec AC 7 grep `grep -E '^next_version: 0.70.0$' <task>` returns 1 line.

2. **Add the post-1.0 unchanged-behavior fixture.** Append a second `It` inside the same `Context("pre-1.0 cap (spec 063)", ...)` block. The fixture mirrors the existing `Context("major-bump guard (spec 060)")` "major bump trips guard" case (lines 1340-1403) but uses a `current_version: 1.2.3` and a different repo so the audit trail is unambiguous:

   ```go
   It(
       "post-1.0 v1.2.3 + breaking-change bullet + Claude returns major: still trips guard, outcome=needs_input",
       func() {
           post1Changelog := []byte(
               "## Unreleased\n\n" +
                   "- refactor(lib): rename TaskTypeClaude → TaskTypeLLM\n\n" +
                   "## v1.2.3\n\n- old\n",
           )
           fakeFetcher := &mocks.Fetcher{}
           fakeFetcher.FetchReturns(post1Changelog, nil)

           fakeRunner := &mocks.ClaudeRunnerMock{}
           fakeRunner.RunReturns(
               &claudelib.ClaudeResult{
                   Result: `{"bump":"major","reasoning":"BREAKING CHANGE: refactor(lib) renames TaskTypeClaude → TaskTypeLLM"}`,
               },
               nil,
               // Note: in production the post-1.0 path would also have
               // Claude return major (because the cap rule does NOT apply
               // to 1.x — major is still legal). The guard still trips
               // because allowMajorBumpConfig and allowMajorFlag are both
               // false in this fixture. The reasoning here is
               // deliberately post-1.0 to make the test name unambiguous.
           )

           step := pkg.NewPlanningStep(
               fakeRunner,
               fakeFetcher,
               &mocks.MaintainerConfigFetcher{},
               false,
           )

           taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/post-1-0-lib\nclone_url: https://github.com/bborbe/post-1-0-lib.git\nref: master\ncurrent_version: v1.2.3\ntask_identifier: gh-release-bborbe-post-1-0-001\n---\n\n# release task\n"

           md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
           Expect(err).NotTo(HaveOccurred())

           result, err := step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())

           // Status: NeedsInput (the spec-060 trip contract).
           Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))

           // ## Plan JSON: outcome + precondition + audit-trail flags.
           plan, err := agentlib.ExtractSection[pkg.PlanOutput](
               context.Background(), md, "## Plan",
           )
           Expect(err).NotTo(HaveOccurred())
           Expect(plan.Outcome).To(Equal(pkg.PlanOutcomeNeedsInput))
           Expect(plan.PreconditionFailed).To(Equal(pkg.PreconditionMajorBumpNotAllowed))

           // FROZEN spec-047 frontmatter mutations.
           gotAssignee, _ := md.Frontmatter.String("assignee")
           Expect(gotAssignee).To(Equal(""))
           gotPrevAssignee, _ := md.Frontmatter.String("previous_assignee")
           Expect(gotPrevAssignee).To(Equal("github-releaser-agent"))
           gotStatus, _ := md.Frontmatter.String("status")
           Expect(gotStatus).To(Equal("in_progress"))
           gotPhase, _ := md.Frontmatter.String("phase")
           Expect(gotPhase).To(Equal("planning"))
       },
   )
   ```

   The test name is long but the audit trail is the load-bearing part. The fixture proves AC 8 — the post-1.0 path is unchanged.

3. **Add the `applyMajorBumpGuard` source-pinning test.** Append a third `It` inside the same `Context("pre-1.0 cap (spec 063)", ...)` block. The test reads the production source file at test time, locates the `func (s *planningStep) applyMajorBumpGuard` function body, computes a SHA-256 of the body, and compares against a constant. This makes AC 9 machine-checkable: if a future maintainer adds a "pre-1.0" branch to the guard, the test fails.

   ```go
   It(
       "applyMajorBumpGuard source body is byte-identical to spec 063 baseline (no in-Go pre-1.0 logic)",
       func() {
           // The guard function must NOT be modified to handle pre-1.0
           // inputs — that is prompt 1's job (LLM-side rule). If a
           // future maintainer adds a `if version is pre-1.0` branch
           // to the guard, this test fails. The hash below pins the
           // body to the spec 063 baseline.
           //
           // To regenerate the hash after an INTENTIONAL guard change
           // (which would itself require a spec amendment per the
           // existing spec 060 § Desired Behavior 3 freeze), update
           // the literal below. Do NOT regenerate as part of this
           // prompt — the body must not change.
           const expectedHash = "PLACEHOLDER_HASH_TO_BE_FILLED_AT_EXEC_TIME"

           srcBytes, err := os.ReadFile("steps_planning.go")
           // The test file lives at pkg/steps_planning_test.go; the
           // production source is at the sibling pkg/steps_planning.go.
           // `go test ./pkg/...` runs with the working directory set
           // to pkg/, so the relative path is just the filename.
           Expect(err).NotTo(HaveOccurred())

           body, ok := extractApplyMajorBumpGuardBody(string(srcBytes))
           Expect(ok).To(BeTrue(), "applyMajorBumpGuard body not found in steps_planning.go")

           h := sha256.Sum256([]byte(body))
           actualHash := hex.EncodeToString(h[:])
           Expect(actualHash).To(Equal(expectedHash))
       },
   )
   ```

   **Implementation note for the executor**: the placeholder `expectedHash = "PLACEHOLDER_HASH_TO_BE_FILLED_AT_EXEC_TIME"` is intentionally wrong — the executor MUST replace it with the real SHA-256 of the current function body BEFORE running the test. The replacement procedure:

   a. Read `/workspace/agent/github-releaser/pkg/steps_planning.go` and locate the `func (s *planningStep) applyMajorBumpGuard(` declaration. Capture the bytes from that line to the closing `}` of the function (the FIRST top-level `}` at column 1 that follows the declaration). Do NOT include the doc-comment lines above the declaration in the body — only the `func (s *planningStep) applyMajorBumpGuard(...)` line through the matching closing `}`.

   b. Compute `sha256.Sum256([]byte(body))` in a one-shot helper. Use the Go standard library `crypto/sha256` and `encoding/hex` (the test file will need to import both). Print the hex digest via `fmt.Printf("DEBUG_HASH=%s\n", hex.EncodeToString(h[:]))` during a temporary scratch run of `go test -v`, capture the output, and replace the placeholder literal with the captured digest.

   c. After replacing the placeholder, run `go test -run applyMajorBumpGuard ./pkg/...` to confirm the test passes against the unchanged source. The test SHOULD pass — the source is the spec 063 baseline.

   The test's load-bearing property: if a future prompt modifies the guard (e.g., adds a pre-1.0 branch, a new opt-out, a new glog line, a new precondition value), the hash changes, the literal mismatches, and the test fails with a clear error message naming the spec 063 invariant.

   The executor must ALSO add a helper function `extractApplyMajorBumpGuardBody(src string) (string, bool)` somewhere in the test file (or in a new helper file `pkg/extract_helpers_test.go` if the test file is approaching the 2000-line threshold — it currently sits at 1791 lines, well below the limit). The helper signature: input is the full source file contents, output is the function body string (from the `func (s *planningStep) applyMajorBumpGuard(` line through the matching closing `}` at column 1), and a bool indicating whether the function was found. The implementation scans for the function-name token then walks braces to find the matching close. This is a pure-string test helper, NOT production code.

4. **Do NOT touch the production code in this prompt.** All three new tests live in `steps_planning_test.go` (and the new helper function for the third test). The only production-code diffs in the spec 063 sequence are in prompt 1 (the prompt rule text + assertions) and prompt 2 (the prompt-assembly injection). This prompt ships ONLY test code.

5. **Do NOT add a CHANGELOG entry in this prompt.** Prompt 1 owns the `## Unreleased` block; this prompt is coverage-locking only.

6. **Run the fast `make test` first.** From repo root: `cd /workspace/agent/github-releaser && make test`. Expected: exit code 0. Investigate any failures. The new `Context("pre-1.0 cap (spec 063)", ...)` block has three new `It` cases — all must pass.

7. **Run `make precommit` as the final gate.** After `make test` exits 0, run `cd /workspace/agent/github-releaser && make precommit`. Expected: exit code 0 (AC 10). The precommit suite is slow (format + generate + test + lint + license + trivy + gosec + errcheck + golangci-lint); the new test code is plain Ginkgo v2 + Gomega and should not trip any linter. If the linter complains about line length in the long `It(...)` names, see the `go-precommit.md` rules (`golines 100`); the longest test name is 122 characters and must be split across multiple string-literal lines using Go's implicit string concatenation.

8. **YAGNI guard.** Do NOT add the `extractApplyMajorBumpGuardBody` helper to the production `steps_planning.go` file — it is a test-only helper, declare it in the test file or a sibling `_test.go` file. Do NOT use a regex to extract the function body — brace-counting is the only correct approach. Do NOT add new mocks or counterfeiter directives — the existing `mocks.ClaudeRunnerMock`, `mocks.Fetcher`, and `mocks.MaintainerConfigFetcher` cover all three new tests. Do NOT regenerate any counterfeiter output — the existing mock files are sufficient. Do NOT modify the `factory.CreateAgent` factory — this prompt ships no production wiring.
</requirements>

<constraints>
- The change is localized to `agent/github-releaser/pkg/steps_planning_test.go` (one new `Context` block with three `It` cases + a small helper function) and possibly a new `pkg/extract_helpers_test.go` if the helper makes the main test file exceed the 2000-line file-size guideline. The production `steps_planning.go` is UNTOUCHED in this prompt.
- The three new tests must pass: vault-cli fixture, post-1.0 unchanged, and applyMajorBumpGuard source-pinning.
- The vault-cli fixture's expected outcome is `PlanOutcomeReady` with `NextVersion: "0.70.0"` — these are byte-exact strings; do not transform (`"0.70.0"`, not `"v0.70.0"`, not `"0.70"`).
- The post-1.0 fixture's expected outcome is `PlanOutcomeNeedsInput` with `PreconditionFailed: PreconditionMajorBumpNotAllowed` — use the constant, not the string literal.
- The `applyMajorBumpGuard` source-pinning test MUST use SHA-256 (the `crypto/sha256` + `encoding/hex` standard library combo). Do NOT use MD5 or any weaker hash. Do NOT use a custom hash function.
- The `extractApplyMajorBumpGuardBody` helper MUST handle nested braces correctly (the guard body itself contains no nested functions, but the brace-counting must be robust to future changes that DO add nested blocks). A simple `for i := startIdx; i < len(src); i++` loop with a depth counter is the right shape.
- The `expectedHash` constant in the source-pinning test MUST be the real SHA-256 of the spec-063-baseline function body, computed at test-execution time. The placeholder `"PLACEHOLDER_HASH_TO_BE_FILLED_AT_EXEC_TIME"` is a guide for the executor — the executor MUST replace it before running the test.
- The new test file (or the modified main test file) MUST compile cleanly with `go vet ./...` — no unused imports, no shadowed variables.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass — the new `Context` block is additive; no existing assertions are modified.
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && make test
```
Expected: exit code 0. All pre-existing tests pass (29 call sites in `steps_planning_test.go`, all 5 prior Context blocks, all prompt 1 + 2 tests). The three new `It` cases in `Context("pre-1.0 cap (spec 063)", ...)` pass:
- `vault-cli v0.69.0 + rename bullet + Claude returns minor:pre-1.0 → outcome=ready, next_version=0.70.0`
- `post-1.0 v1.2.3 + breaking-change bullet + Claude returns major: still trips guard, outcome=needs_input`
- `applyMajorBumpGuard source body is byte-identical to spec 063 baseline (no in-Go pre-1.0 logic)`

Then the final gate:
```
cd /workspace/agent/github-releaser && make precommit
```
Expected: exit code 0 (AC 10). The full precommit suite (format + generate + test + lint + license + trivy + gosec + errcheck + golangci-lint) passes.

Evidence commands the auditor will run:
- `go test -v -count=1 -run 'pre-1.0' /workspace/agent/github-releaser/pkg/` → the three new `It` cases run and pass.
- `grep -c 'pre-1.0 cap (spec 063)' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → exactly 1 (the new `Context` heading).
- `grep -c 'vault-cli v0.69.0' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → exactly 1 (the vault-cli fixture's `It` name).
- `grep -c 'applyMajorBumpGuard source body is byte-identical' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → exactly 1 (the source-pinning `It` name).
- `git diff HEAD -- /workspace/agent/github-releaser/pkg/steps_planning.go` → zero changed lines (this prompt is test-only).
- `git diff HEAD -- /workspace/agent/github-releaser/pkg/steps_planning.go | grep -A 200 'applyMajorBumpGuard' | head -5` → the function body is unchanged from master.
- `cd /workspace/agent/github-releaser && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/... && go tool cover -func=/tmp/cover.out | grep -E 'steps_planning\.go|total'` → coverage on `steps_planning.go` stays at or above 90.7% (the pre-prompt baseline). The new tests are exercising already-tested code paths so the coverage number is the regression-guard, not a growth target.
- `cd /workspace/agent/github-releaser && make precommit` → exit code 0.
</verification>
