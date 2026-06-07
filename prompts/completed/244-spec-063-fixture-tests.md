---
status: completed
spec: [063-releaser-no-major-bump]
summary: Added spec 063 pre-1.0 cap behavioral fixtures (vault-cli 0.69.0 happy path + v1.2.3 post-1.0 guard trip) to steps_planning_test.go; both new It cases pass and make precommit exits 0
container: maintainer-no-major-bump-exec-244-spec-063-fixture-tests
dark-factory-version: v0.175.0
created: "2026-06-06T21:46:08Z"
queued: "2026-06-07T14:26:59Z"
started: "2026-06-07T14:35:34Z"
completed: "2026-06-07T14:55:58Z"
branch: dark-factory/releaser-no-major-bump
---

<summary>
- Add two behavioral fixture tests that lock down spec-063's envelope: a pre-1.0 cap (replaying the vault-cli 2026-06-06 incident, asserting the release proceeds unattended) and a post-1.0 unchanged-behavior check (asserting the guard still halts breaking changes on >=1.0.0 versions).
- The pre-1.0 fixture proves the agent now ships what a human had to ship manually on 2026-06-06.
- The post-1.0 fixture is the spec's required negative evidence — the guard remains intact for post-1.0.
- Test patterns mirror existing style in the same file, keeping the audit trail consistent.
- Final `make precommit` gate — exit code 0 confirms acceptance.
- Production code is untouched in this prompt; siblings (prompts 1 and 2) ship the rule + assembly.
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

3. **AC 9 is satisfied by spec-level diff evidence, NOT by a runtime test.** The spec's AC 9 evidence shape is `git diff HEAD -- agent/github-releaser/pkg/steps_planning.go` showing zero changed lines inside the guard body — a daemon/reviewer check, not a Go test. Do NOT add a source-pinning, SHA-hash, or AST-based runtime test for the guard body. Runtime-pinning creates noisy false positives on cosmetic edits (gofmt, golines, comment additions) and trains maintainers to bump pins without reading the diff. The two `It` cases from requirements 1 and 2 are the full scope of this prompt's test additions.

4. **Do NOT touch the production code in this prompt.** Both new tests live in `steps_planning_test.go`. The only production-code diffs in the spec 063 sequence are in prompt 1 (the prompt rule text + assertions) and prompt 2 (the prompt-assembly injection). This prompt ships ONLY test code.

5. **Do NOT add a CHANGELOG entry in this prompt.** Prompt 1 owns the `## Unreleased` block; this prompt is coverage-locking only.

6. **Run the fast `make test` first.** From repo root: `cd /workspace/agent/github-releaser && make test`. Expected: exit code 0. The new `Context("pre-1.0 cap (spec 063)", ...)` block has two new `It` cases — both must pass.

7. **Run `make precommit` as the final gate.** After `make test` exits 0, run `cd /workspace/agent/github-releaser && make precommit`. Expected: exit code 0 (AC 10). If the linter complains about line length in the long `It(...)` names, pre-wrap the literal using Go's implicit string concatenation (multiple string literals on adjacent lines).

8. **YAGNI guard.** Do NOT add new mocks or counterfeiter directives — the existing `mocks.ClaudeRunnerMock`, `mocks.Fetcher`, and `mocks.MaintainerConfigFetcher` cover both new tests. Do NOT regenerate any counterfeiter output — the existing mock files are sufficient. Do NOT modify the `factory.CreateAgent` factory — this prompt ships no production wiring.
</requirements>

<constraints>
- The change is localized to `agent/github-releaser/pkg/steps_planning_test.go` (one new `Context` block with two `It` cases). The production `steps_planning.go` is UNTOUCHED in this prompt.
- Both new tests must pass: vault-cli fixture (pre-1.0 cap) and post-1.0 unchanged behavior.
- The vault-cli fixture's expected outcome is `PlanOutcomeReady` with `NextVersion: "0.70.0"` — these are byte-exact strings; do not transform (`"0.70.0"`, not `"v0.70.0"`, not `"0.70"`).
- The post-1.0 fixture's expected outcome is `PlanOutcomeNeedsInput` with `PreconditionFailed: PreconditionMajorBumpNotAllowed` — use the constant, not the string literal.
- The new test file MUST compile cleanly with `go vet ./...` — no unused imports, no shadowed variables.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass — the new `Context` block is additive; no existing assertions are modified.
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && make test
```
Expected: exit code 0. All pre-existing tests pass. The two new `It` cases in `Context("pre-1.0 cap (spec 063)", ...)` pass:
- `vault-cli v0.69.0 + rename bullet + Claude returns minor:pre-1.0 → outcome=ready, next_version=0.70.0`
- `post-1.0 v1.2.3 + breaking-change bullet + Claude returns major: still trips guard, outcome=needs_input`

Then the final gate:
```
cd /workspace/agent/github-releaser && make precommit
```
Expected: exit code 0 (AC 10). The full precommit suite (format + generate + test + lint + license + trivy + gosec + errcheck + golangci-lint) passes.

Evidence commands the auditor will run:
- `go test -v -count=1 -run 'pre-1.0' /workspace/agent/github-releaser/pkg/` → both new `It` cases run and pass.
- `grep -c 'pre-1.0 cap (spec 063)' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → exactly 1 (the new `Context` heading).
- `grep -c 'vault-cli v0.69.0' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → exactly 1 (the vault-cli fixture's `It` name).
- `git diff HEAD -- /workspace/agent/github-releaser/pkg/steps_planning.go` → zero changed lines (this prompt is test-only). Satisfies AC 9.
- `cd /workspace/agent/github-releaser && make precommit` → exit code 0.
</verification>
