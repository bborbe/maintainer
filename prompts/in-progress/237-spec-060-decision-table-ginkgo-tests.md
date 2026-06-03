---
status: approved
spec: [060-github-releaser-major-bump-guard]
created: "2026-06-03T15:05:00Z"
queued: "2026-06-03T14:34:36Z"
branch: dark-factory/github-releaser-major-bump-guard
---

<summary>
- Four Ginkgo decision-table `It` cases land in `agent/github-releaser/pkg/steps_planning_test.go`, covering every row of the spec 060 guard decision table: `major + no opt-in → trip`, `major + repo opt-in → proceed`, `major + CLI flag set → proceed`, `minor + no opt-in → proceed (no-op)`
- The trip case's `## Unreleased` body contains the literal regression bullet `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` verbatim, naming the originating incident in the test source per spec 060 § Desired Behavior 8
- Every existing `pkg.NewPlanningStep(...)` call site in the test file is updated to pass the new 4-argument signature (the third argument is the maintainerConfig fetcher from spec 059, the fourth is the new `allowMajor bool`)
- The mocked `claudelib.ClaudeRunner` returns a forced `{"bump":"major","reasoning":"..."}` verdict for the trip case; no real Claude call; the test layer uses counterfeiter mocks already in `mocks/claude_runner.go` and `mocks/fetcher.go`
- The trip-case assertions lock every load-bearing invariant: `Result.Status == AgentStatusNeedsInput`, the `## Plan` JSON contains `outcome: needs_input` AND `precondition_failed: major_bump_not_allowed`, the frontmatter has `assignee: ""` AND `previous_assignee: github-releaser-agent` AND `status: in_progress` AND `phase: planning` (the FROZEN spec 047 escalation contract)
- Coverage on `agent/github-releaser/pkg/...` reaches ≥ 75% per the spec 060 § Constraints inherited target
- The existing spec 058 and spec 059 test fixtures are NOT regressed; the only diff is the new constructor argument and the four new `It` cases
</summary>

<objective>
Update the planning-step Ginkgo test file to match the new `pkg.NewPlanningStep(runner, fetcher, maintainerConfig, allowMajor)` signature from prompt 3, and add four new `It` cases that exercise the spec 060 guard decision table end-to-end through the planning step (factory wiring, frontmatter mutation, `## Plan` section marshal, NeedsInput result mapping, and the override-path `glog.V(2)` line via the counterfeiter mock). The four cases cover the trip path, both proceed paths, and the no-op minor path. The trip case's `## Unreleased` fixture must contain the literal regression bullet `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` so the originating incident is named in the test source.
</objective>

<context>
Read `CLAUDE.md` and `agent/github-releaser/CLAUDE.md` for project conventions.

Read these files BEFORE editing:
- `/workspace/agent/github-releaser/pkg/steps_planning_test.go` — the existing planning-step Ginkgo tests. Find every `pkg.NewPlanningStep(...)` call site and update each to pass the new 4-argument signature. The existing 11+ `It` cases use `pkg.NewPlanningStep(runner, fetcher, &mocks.MaintainerConfigFetcher{})` (3 args) per spec 059; after this prompt they pass `pkg.NewPlanningStep(runner, fetcher, &mocks.MaintainerConfigFetcher{}, false)` (4 args, `allowMajor=false` for all non-spec-060 tests). The spec 060 four new cases are added in a new `Context("major-bump guard (spec 060)")` block parallel to the existing `Context("changelogRewrite opt-in flag")` block.
- `/workspace/agent/github-releaser/pkg/steps_planning.go` — the planning step. Read for the new `escalation` struct shape (extended with `nextVersion`, `nextVersionHeader`, `bump`, `bullets`, `reasoning`, `allowMajorBumpConfig`, `allowMajorBumpFlag` per prompt 3) and the new `PreconditionMajorBumpNotAllowed` constant. The trip-case `It` asserts the `## Plan` JSON's `next_version` / `bullets` / `precondition_failed` / `outcome` fields.
- `/workspace/agent/github-releaser/pkg/plan_output.go` — the `PlanOutput` struct. Read for the two new `AllowMajorBumpConfig bool` and `AllowMajorBumpFlag bool` fields with their `json:"...,omitempty"` tags. The trip-case `It` parses the marshaled `## Plan` JSON and asserts `plan.AllowMajorBumpConfig == false` and `plan.AllowMajorBumpFlag == false` (both false on the trip case).
- `/workspace/agent/github-releaser/mocks/maintainer_config_fetcher.go` — the `mocks.MaintainerConfigFetcher` counterfeiter mock. The four new cases use `FetchReturns([]byte(...), nil)` to inject the desired `release.allowMajorBump` value.
- `/workspace/agent/github-releaser/mocks/claude_runner.go` — the `mocks.ClaudeRunnerMock` counterfeiter mock. The trip case forces a `{"bump":"major","reasoning":"..."}` verdict via `RunReturns(&claudelib.ClaudeResult{Result: ...}, nil)`. The other three cases use minor / patch verdicts as needed.
- `/workspace/specs/in-progress/060-github-releaser-major-bump-guard.md` — the spec under implementation. § Desired Behavior 8 names the four-row decision table; § AC 15-19 name the four `It` cases and their required assertions; § Constraints lock the FROZEN escalation contract (assignee / previous_assignee / status / phase / Status).

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — coverage ≥ 75% on `pkg/...`.

Verified symbols (from module source — grep-confirmed):
- `pkg.NewPlanningStep(runner, fetcher, maintainerConfig, allowMajor) agentlib.Step` — the new 4-arg signature (per prompt 3). Every test fixture must be updated to pass `false` as the fourth arg unless the case specifically tests the override path.
- `mocks.MaintainerConfigFetcher.FetchReturns(bytes, err)` — sets the mock's return values. `FetchReturns([]byte("release:\n  allowMajorBump: true\n"), nil)` injects the spec-060 YAML opt-in; `FetchReturns(nil, nil)` yields the default-mock semantics (prompt 1's locked contract `Parse(ctx, []byte{}) → (zero, nil)` → `allowMajorBump=false`).
- `mocks.ClaudeRunnerMock.RunReturns(&claudelib.ClaudeResult{Result: "..."}, nil)` — forces the LLM verdict. For the spec 060 cases: `{"bump":"major","reasoning":"BREAKING CHANGE detected"}` for the trip / repo-opt-in / flag-opt-in cases, and `{"bump":"minor","reasoning":"feat: stub"}` for the no-op minor case.
- `agentlib.ExtractSection[pkg.PlanOutput](ctx, md, "## Plan")` — parses the `## Plan` JSON for assertions. The existing tests use this pattern (see `steps_planning_test.go:73-86` for the happy-path template).
- `md.Frontmatter.String("key")` — reads a frontmatter field. The trip case asserts `assignee==""`, `previous_assignee=="github-releaser-agent"`, `status=="in_progress"`, `phase=="planning"`.
- `pkg.PlanOutcomeNeedsInput = "needs_input"` and `pkg.PreconditionMajorBumpNotAllowed = "major_bump_not_allowed"` — the outcome + precondition values the trip case asserts.
- `agentlib.AgentStatusNeedsInput`, `agentlib.AgentStatusDone` — the two status values asserted on the trip and proceed paths.
- `domain.TaskPhaseExecution = "execution"` — the `NextPhase` value asserted on the proceed paths. `result.NextPhase == "execution"` for the proceed cases; `result.NextPhase == ""` (or empty string) for the trip case (the `escalate` helper does not set `NextPhase`).
- `pkg.NewPlanningStep` is also called from `pkg/factory/factory.go`'s `CreateAgent` (production wiring). The factory call site is already updated in prompt 3; this prompt only touches the test file.
</context>

<requirements>

1. **Update every existing `pkg.NewPlanningStep(...)` call site in the test file.** Find them via:

   ```
   grep -n 'pkg.NewPlanningStep' /workspace/agent/github-releaser/pkg/steps_planning_test.go
   ```

   Every existing call site passes 3 arguments (`runner, fetcher, &mocks.MaintainerConfigFetcher{}` or `runner, fetcher, withChangelogRewriteTrue()`). After this prompt, every call site passes 4 arguments with the fourth being `false`:

   ```go
   step := pkg.NewPlanningStep(
       fakeRunner,
       fakeFetcher,
       &mocks.MaintainerConfigFetcher{},
       false, // spec 060: per-run allowMajor; false for all non-spec-060 tests
   )
   ```

   For tests that previously used `withChangelogRewriteTrue()` (the spec 059 helper that returns a mock with `release.changelogRewrite: true` set), the same change applies — pass `false` as the fourth argument. The spec 059 mock helper's return value is unchanged.

   Add a brief comment on the first updated call site only (so a future maintainer sees the convention): `// spec 060: per-run allowMajor; false unless the test exercises the override path.`

2. **Add a new `Context("major-bump guard (spec 060)", ...)` block.** Place it AFTER the existing `Context("changelogRewrite opt-in flag", ...)` block (so the spec history reads in chronological order: 058 → 059 → 060). The new block contains FOUR `It` cases, each of which MUST satisfy the load-bearing boundary tests for spec 060 § Desired Behavior 8 / § AC 15-19.

   Common fixture scaffolding (define once at the top of the new Context, reuse in each `It`):

   ```go
   const taskMD = "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-bborbe-maintainer-master-spec060\n---\n\n# release task\n"
   ```

   The trip-case fixture's `## Unreleased` body MUST contain the literal regression bullet `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` verbatim (per spec 060 § Desired Behavior 8: "The `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` bullet is used verbatim as the input for at least the trip case so the regression is named in the test source"). The fixture:

   ```go
   // Trip-case CHANGELOG: contains the literal regression bullet from
   // the originating incident (spec 060 § Problem). A prefix-only
   // classifier would mark this `refactor:` as patch; the spec-060
   // guard ensures the operator gets a NeedsInput regardless of what
   // the classifier returns. Mocked ClaudeRunner forces bump=major
   // so the trip case is deterministic — independent of the
   // prefix-only classifier rules.
   tripChangelog := []byte(
       "## Unreleased\n\n" +
           "- refactor(lib): rename TaskTypeClaude → TaskTypeLLM\n\n" +
           "## v1.7.7\n\n- old\n",
   )
   ```

   The acceptance criterion `grep -c 'refactor(lib): rename TaskTypeClaude' /workspace/agent/github-releaser/pkg/steps_planning_test.go` returns ≥ 1 — the bullet MUST appear verbatim in the test source.

3. **`It("major bump trips guard when neither opt-in present", ...)`** — the trip case. Mock the maintainerConfigFetcher to return YAML WITHOUT the opt-in (default mock returns `(nil, nil)` which yields `allowMajorBump=false` per spec 059's locked contract). Force ClaudeRunner to return `{"bump":"major","reasoning":"BREAKING CHANGE: rename TaskTypeClaude to TaskTypeLLM"}`. Construct the step with `allowMajor=false`. Required assertions (FIVE separate Gomega expectations per spec 060 § AC 15):

   a. `result.Status == agentlib.AgentStatusNeedsInput`.

   b. The mutated `## Plan` JSON (via `agentlib.ExtractSection[pkg.PlanOutput]`) has `plan.Outcome == "needs_input"`.

   c. The mutated `## Plan` JSON has `plan.PreconditionFailed == "major_bump_not_allowed"`.

   d. The mutated `## Plan` JSON has `plan.AllowMajorBumpConfig == false` AND `plan.AllowMajorBumpFlag == false` (the audit-trail fields on the trip case are populated with the resolved values so the operator can see which lever to flip).

   e. The frontmatter mutations match the FROZEN spec 047 contract: `md.Frontmatter.String("assignee") == ""`, `md.Frontmatter.String("previous_assignee") == "github-releaser-agent"`, `md.Frontmatter.String("status") == "in_progress"`, `md.Frontmatter.String("phase") == "planning"`.

   Optional bonus assertion (not required by AC but useful for the regression bullet): `plan.Reason` contains the substring `major bump not allowed` (the trip-case log line's prefix; mirrors the `Reason` substring the spec requires).

4. **`It("major bump proceeds when repo opt-in true", ...)`** — the repo-opt-in proceed case. Mock `maintainerConfigFetcher.FetchReturns([]byte("release:\n  allowMajorBump: true\n"), nil)`. Force ClaudeRunner to return `{"bump":"major","reasoning":"..."}`. Construct the step with `allowMajor=false`. Required assertions (per spec 060 § AC 16):

   a. `result.Status == agentlib.AgentStatusDone`.

   b. `result.NextPhase == "execution"`.

   c. The mutated `## Plan` JSON has `plan.Outcome == "ready"` (the existing `PlanOutcomeReady` value).

   d. The mutated `## Plan` JSON has `plan.Bump == "major"` and `plan.AllowMajorBumpConfig == true` (the resolved flag value flows to the audit trail; `plan.AllowMajorBumpFlag == false` because the CLI override was not used).

5. **`It("major bump proceeds when CLI flag set", ...)`** — the override-proceed case. Default-mock the maintainerConfigFetcher (returns `(nil, nil)` → `allowMajorBump=false`). Force ClaudeRunner to return `{"bump":"major","reasoning":"..."}`. Construct the step with `allowMajor=true`. Required assertions (per spec 060 § AC 17):

   a. `result.Status == agentlib.AgentStatusDone`.

   b. `result.NextPhase == "execution"`.

   c. The mutated `## Plan` JSON has `plan.Outcome == "ready"`.

   d. The mutated `## Plan` JSON has `plan.AllowMajorBumpConfig == false` AND `plan.AllowMajorBumpFlag == true` (the CLI override IS used; both fields are populated on the happy path for audit).

6. **`It("minor bump unaffected by guard", ...)`** — the no-op case. Default-mock the maintainerConfigFetcher. Force ClaudeRunner to return `{"bump":"minor","reasoning":"feat: stub"}`. Construct the step with `allowMajor=false`. Required assertions (per spec 060 § AC 18):

   a. `result.Status == agentlib.AgentStatusDone`.

   b. `result.NextPhase == "execution"`.

   c. The mutated `## Plan` JSON has `plan.Outcome == "ready"`, `plan.Bump == "minor"`, `plan.NextVersion == "1.8.0"`.

   d. (Implicit invariant) the maintainerConfigFetcher was called at most once — the guard path does not short-circuit, so the normal planning step ran.

7. **Counterfeiter mocks are already in place.** Both `mocks.MaintainerConfigFetcher` (spec 059 prompt 1) and `mocks.ClaudeRunnerMock` (existing) are generated; no new `//counterfeiter:generate` directive is needed. If a new mock fails to resolve, run `cd /workspace/agent/github-releaser && go generate ./...` first.

8. **Coverage gate — `go test -cover ./pkg/...` reports coverage matching the regex `coverage: ([7-9][0-9]|100)\.?[0-9]*%` on the planning-step package.** Run `cd /workspace/agent/github-releaser && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/... && go tool cover -func=/tmp/cover.out | tail -1` and confirm the last line matches the spec's coverage regex. The spec 060 § Constraints inherits spec 049's ≥ 75% target. The four new cases plus the existing 11+ cases should land the file well above 75% on `steps_planning.go` (which has the new guard logic).

   If coverage is below 75%, identify the uncovered branches in the new guard logic (most likely: the override-proceed case's `glog.V(2)` line, or the `escalation` struct's new fields) and add minimal targeted assertions to cover them. The simplest coverage boost for the override-proceed log line: assert the result is Done (which the case already does — the `glog.V(2)` line is purely observability and doesn't affect the path).

9. **Acceptance gate — `make test` exits 0 in `agent/github-releaser/`.** Run `cd /workspace/agent/github-releaser && make test` and confirm exit code 0. The new four `It` cases pass; every updated existing `It` case (with the new 4-arg `NewPlanningStep` signature) still passes. Investigate and fix any failures. Do NOT run `make precommit` in this prompt — the test suite is the load-bearing boundary for spec 060, and `make precommit` adds lint / gosec / trivy on top of test, which is slow. The full precommit is the final gate at the end of prompt 5's flow.

10. **Cross-prompt dependency declaration.** This prompt depends on prompts 1, 2, and 3 having shipped (the schema, the CLI flag, the guard logic). Prompt 5 (README + CHANGELOG) depends on this prompt's tests passing — the CHANGELOG entry cites the Ginkgo test count as evidence.
</requirements>

<constraints>
- The trip case MUST use the existing `escalate` helper (via the planning step's `runClassification` path) — do NOT hand-roll a parallel frontmatter mutation. The FROZEN spec 047 contract is the same as the existing P1/P2/missing-frontmatter escalation paths.
- The trip case's `## Unreleased` body MUST contain the literal bullet `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` (with the `→` arrow character). This is the originating-incident regression name from spec 060 § Problem. The acceptance criterion `grep -c 'refactor(lib): rename TaskTypeClaude' agent/github-releaser/pkg/steps_planning_test.go` returns ≥ 1.
- The four new `It` case names MUST match the spec's AC substrings so the AC grep evidence commands work:
  - `It("major bump trips guard when neither opt-in present", ...)` — `grep -c 'major bump trips guard when neither opt-in present' agent/github-releaser/pkg/steps_planning_test.go` returns 1.
  - `It("major bump proceeds when repo opt-in true", ...)` — `grep -c 'major bump proceeds when repo opt-in true' agent/github-releaser/pkg/steps_planning_test.go` returns 1.
  - `It("major bump proceeds when CLI flag set", ...)` — `grep -c 'major bump proceeds when CLI flag set' agent/github-releaser/pkg/steps_planning_test.go` returns 1.
  - `It("minor bump unaffected by guard", ...)` — `grep -c 'minor bump unaffected by guard' agent/github-releaser/pkg/steps_planning_test.go` returns 1.
- The mocked ClaudeRunner verdict for the trip case MUST be a `major` verdict with reasoning that contains the substring `BREAKING CHANGE` (so the trip case's `plan.Reason` value contains a meaningful citation; the spec's AC 11 says the trip's `reason` field contains the classifier reasoning verbatim). Do NOT use a generic `{"bump":"major","reasoning":"x"}` stub — use a reasoning string that includes both `BREAKING CHANGE` AND a citation of the offending bullet, e.g. `BREAKING CHANGE: refactor(lib) renames TaskTypeClaude → TaskTypeLLM`.
- Every existing `pkg.NewPlanningStep(...)` call site in the test file MUST be updated to pass 4 arguments. Forgetting a site produces a compile error that breaks the whole package's tests. Use `grep -n 'pkg.NewPlanningStep' agent/github-releaser/pkg/steps_planning_test.go` to enumerate sites; the count is the call-site count.
- The default fourth-argument value is `false` for every non-spec-060 test (the spec 059 tests, the spec 058 tests, the integration tests). Only the `major bump proceeds when CLI flag set` case uses `true`.
- The four new cases use the existing `mocks.ClaudeRunnerMock.RunReturns(...)` pattern (single-call) — the trip / proceed-with-rewrite flows use `RunReturnsOnCall(0, ...)` and `RunReturnsOnCall(1, ...)` only when a second LLM call is expected. The spec 060 cases do NOT exercise the rewrite pipeline (they mock the maintainerConfigFetcher with the default mock, which yields `changelogRewrite=false`, which skips the rewrite call), so a single `RunReturns(...)` is correct.
- The trip case's `nextVersion` assertion (in `plan.NextVersion` and `plan.NextVersionHeader`) is optional — the spec's required assertions are the FIVE listed in requirement 3. Asserting `plan.NextVersion == "2.0.0"` and `plan.NextVersionHeader == "## v2.0.0"` is a useful bonus that proves the trip case computes the would-be release, but is NOT required by AC 15.
- Do NOT add Prometheus metrics, debug logging, or other observability beyond the existing `glog.V(2).Infof` pattern.
- Do NOT commit — dark-factory handles git.
- Coverage on `agent/github-releaser/pkg/...` MUST be ≥ 75% after this prompt (spec 060 inherits spec 049's target).
- Existing tests MUST still pass after the constructor-signature update.
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && make test
```
Expected: exit code 0; the four new `It` cases pass; every updated existing `It` case still passes; coverage on `pkg/...` matches `coverage: ([7-9][0-9]|100)\.?[0-9]*%` regex.

Evidence commands the auditor will run:
- `grep -c 'major bump trips guard when neither opt-in present' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → 1.
- `grep -c 'major bump proceeds when repo opt-in true' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → 1.
- `grep -c 'major bump proceeds when CLI flag set' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → 1.
- `grep -c 'minor bump unaffected by guard' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → 1.
- `grep -c 'refactor(lib): rename TaskTypeClaude' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → ≥ 1 (the regression-bullet fixture).
- `grep -c 'pkg.NewPlanningStep' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → ≥ 11 (every existing test fixture now passes 4 arguments; the four new cases add 4 more).
- `cd /workspace/agent/github-releaser && go test -cover ./pkg/... 2>&1 | grep -E 'coverage: ([7-9][0-9]|100)\.?[0-9]*%'` → at least one match.
- `cd /workspace/agent/github-releaser && make test` → exit code 0.
</verification>
