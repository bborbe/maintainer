---
status: completed
spec: ["047"]
summary: Implemented PlanningStep for github-releaser agent with full test coverage
container: maintainer-github-releaser-exec-192-spec-047-planning-step
dark-factory-version: v0.173.0
created: "2026-05-28T00:00:00Z"
queued: "2026-05-28T05:18:37Z"
started: "2026-05-28T05:22:59Z"
completed: "2026-05-28T05:29:08Z"
---

<summary>
- Adds the PlanningStep: the first executable step in the github-releaser agent's planning phase.
- Reads task frontmatter, fetches the target repo's CHANGELOG via the Fetcher from prompt 1, validates Unreleased preconditions, asks Claude to classify the bump, computes the next version, writes a typed `## Plan` JSON section, and advances to the execution phase.
- On precondition failure (missing frontmatter field, P1 not-first heading, P2 empty Unreleased, bad current_version): writes `## Plan` with `outcome: needs_input`, clears `assignee`, sets `previous_assignee: github-releaser-agent`, returns Done — keeps `status`/`phase` unchanged for operator-triggered re-delegation.
- Integration test layer drives the full step with a counterfeiter-mocked Claude runner and the mocked Fetcher from prompt 1 — exercises happy path AND escalation end-to-end without network.
- All errors via `github.com/bborbe/errors`; section I/O via `agentlib.MarshalSectionTyped` / `agentlib.ExtractSection` — never raw `strings.Index`.
- Coverage ≥ 75% on `pkg/steps_planning.go`.
</summary>

<objective>
Implement `agent/github-releaser/pkg/steps_planning.go` — a `PlanningStep` struct exposing `Name`, `ShouldRun`, `Run` per `agentlib.Step`. The step wires together three already-built foundation libraries (`pkg/changelog`, `pkg/semver`, `pkg/prompts`), the new Fetcher from prompt 1, and a Claude runner injected from the factory (prompt 3).

End state: `cd agent/github-releaser && make precommit` exits 0; ≥ 75% coverage on `pkg/steps_planning.go`; Ginkgo integration tests in `pkg/steps_planning_test.go` cover the happy path (Result Done + NextPhase execution + `## Plan` with `outcome: ready`) and the P1 escalation path (Result Done + frontmatter mutation: `assignee` empty, `previous_assignee: github-releaser-agent`, `status` and `phase` unchanged).
</objective>

<context>
Read before writing code (all paths repo-relative; container mounts repo root at `/workspace`):

- `CLAUDE.md` at repo root — project conventions.
- `specs/in-progress/047-github-releaser-planning-phase-integration.md` — re-read Desired Behavior 3-6, Failure Modes table (7 rows), and Acceptance Criteria. This prompt covers behaviors 3, 4, 6, and the test layer for ACs 9-10.
- `agent/github-releaser/pkg/changelog/changelog.go` lines 25-200 — already exports `ValidateUnreleased(content []byte) (valid bool, reason string, line int)`, `ExtractUnreleasedBullets(content []byte) []string`, `InferHeaderPrefixStyle(content []byte) string`. EXACT signatures — do not re-invent.
- `agent/github-releaser/pkg/semver/semver.go` lines 22-50 — already exports `BumpVersion(current string, bump string) (string, error)`. Returns the numeric version (no `v` prefix); caller prepends prefix.
- `agent/github-releaser/pkg/prompts/prompts.go` — already exports `BumpClassificationPrompt() string`, `type BumpVerdict struct { Bump string; Reasoning string }`, and `ParseBumpVerdict(claudeOutput string) (BumpVerdict, error)`. Errors from `ParseBumpVerdict` contain `parse bump verdict`.
- `agent/github-releaser/pkg/plan_output.go` — produced by prompt 1; exports `PlanOutput` struct + constants `PlanOutcomeReady`, `PlanOutcomeNeedsInput`, `PreconditionP1UnreleasedNotFirst`, `PreconditionP2UnreleasedEmpty`, `PreconditionBadCurrentVersion`, `PreconditionMissingFrontmatter`.
- `agent/github-releaser/pkg/githubchangelog/fetcher.go` — produced by prompt 1; exports `Fetcher` interface with `Fetch(ctx, owner, repo, ref) ([]byte, error)`. Mock at `pkg/githubchangelog/mocks/fetcher.go` (type `Fetcher`).
- `agent/pr-reviewer/pkg/steps_planning.go` lines 30-115 — canonical step struct + constructor + `Name`/`ShouldRun`/`Run` shape. Mirror the layout (NOT the LGTM-routing logic — that's pr-reviewer-specific).
- `agent/pr-reviewer/pkg/steps_planning.go` lines 83-115 — shows how to call `s.runner.Run(ctx, prompt)` and read `runResult.Result` (which is the Claude stdout). The Claude runner interface is `claudelib.ClaudeRunner.Run(ctx, prompt) (*claudelib.ClaudeResult, error)` and `ClaudeResult.Result` is the string output.
- `agent/pr-reviewer/pkg/steps_mocks.go` line 7 — counterfeiter directive for the Claude runner mock pattern: `//counterfeiter:generate -o ../mocks/claude-runner.go --fake-name ClaudeRunnerMock github.com/bborbe/agent/lib/claude.ClaudeRunner`. Replicate this in a new `agent/github-releaser/pkg/steps_mocks.go` so the github-releaser pkg test suite has access to a ClaudeRunner mock.

Agent-lib API contract (already imported transitively; `agent/github-releaser/go.mod` has `github.com/bborbe/agent/lib v0.63.11`):

- `agentlib.Step` interface (`github.com/bborbe/agent/lib`): `Name() string`, `ShouldRun(ctx, *Markdown) (bool, error)`, `Run(ctx, *Markdown) (*Result, error)`.
- `agentlib.Result` struct: `Status AgentStatus`, `NextPhase string`, `Message string`, `ContinueToNext bool`. Body changes flow through markdown mutation, NOT Result.
- `agentlib.MarshalSectionTyped[T any](ctx, heading string, value T) (Section, error)` — produces a `Section{Heading, Body}` with `Body` = `` ```json\n{...}\n``` `` fence.
- `agentlib.ExtractSection[T any](ctx, *Markdown, heading string) (*T, error)` — reverse of above; used in tests.
- `md.ReplaceSection(Section)` — idempotent in-place update.
- `md.Frontmatter.String(key) (string, bool)` — read a string frontmatter field; `ok=false` when absent.
- `md.Frontmatter[key] = value` — direct map mutation (TaskFrontmatter is `map[string]interface{}`).
- `domain.TaskPhaseExecution` and `domain.TaskPhasePlanning` from `github.com/bborbe/vault-cli/pkg/domain` — typed constants, NEVER use the string literal `"planning"` or `"execution"` in production code (spec § Constraints).

Phase 1 prompt access: `prompts.BumpClassificationPrompt()` returns the embedded markdown rules. Concatenate with the bullets for the actual Claude call. The shape for the user message portion is your choice — recommend `BumpClassificationPrompt() + "\n\n## Bullets to classify\n\n" + strings.Join(bullets, "\n")`.

Coding-plugin guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` `Wrapf`/`Errorf` usage.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega, external `_test` packages, `DescribeTable`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter directive patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — context for how the step is wired in prompt 3 (NOT what this prompt builds — just for awareness of injection seams).
</context>

<requirements>

**Run order: do steps in sequence. Run `cd agent/github-releaser && go test ./pkg/...` after step 6. Run `cd agent/github-releaser && make precommit` only as the final verification step.**

1. **Create `agent/github-releaser/pkg/steps_mocks.go`** — single-line counterfeiter directive file that exposes the Claude runner mock to the test suite. Mirrors `agent/pr-reviewer/pkg/steps_mocks.go`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   //counterfeiter:generate -o ../mocks/claude-runner.go --fake-name ClaudeRunnerMock github.com/bborbe/agent/lib/claude.ClaudeRunner
   ```

   Notes: the existing `agent/github-releaser/mocks/mocks.go` placeholder file is in the same package `mocks` — this directive output `../mocks/claude-runner.go` joins it.

2. **Create `agent/github-releaser/pkg/pkg_suite_test.go`** — bootstrap for the flat pkg test suite. Mirrors `agent/pr-reviewer/pkg/pkg_suite_test.go`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg_test

   //go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Pkg Suite", suiteConfig, reporterConfig)
   }
   ```

3. **Generate Claude runner mock**:

   ```bash
   cd agent/github-releaser && go generate ./pkg/...
   ```

   Produces `agent/github-releaser/mocks/claude-runner.go` (joins the package `mocks` namespace alongside the existing placeholder `mocks.go`).

4. **Write `agent/github-releaser/pkg/steps_planning.go`** with this structure:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import (
       "context"
       "strings"

       agentlib "github.com/bborbe/agent/lib"
       claudelib "github.com/bborbe/agent/lib/claude"
       "github.com/bborbe/errors"
       domain "github.com/bborbe/vault-cli/pkg/domain"
       "github.com/golang/glog"

       "github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog"
       "github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog"
       "github.com/bborbe/maintainer/agent/github-releaser/pkg/prompts"
       "github.com/bborbe/maintainer/agent/github-releaser/pkg/semver"
   )

   // AgentLogin is the GitHub-task-system identity used in escalation frontmatter
   // (previous_assignee). Per spec 047 § Constraints, this MUST be
   // "github-releaser-agent" — grep-asserted by acceptance criteria.
   const AgentLogin = "github-releaser-agent"

   // requiredFrontmatterFields are the keys read from the task's frontmatter
   // before the step does any IO. Missing OR empty → outcome=needs_input
   // with precondition_failed = "missing_frontmatter_<field>".
   //
   // Order matters for deterministic error messages: first missing field wins.
   var requiredFrontmatterFields = []string{
       "repo",
       "clone_url",
       "ref",
       "current_version",
       "task_identifier",
   }

   // planningStep implements agentlib.Step. Fields are constructor-injected;
   // no global state, no IO outside the runner and fetcher.
   type planningStep struct {
       runner  claudelib.ClaudeRunner
       fetcher githubchangelog.Fetcher
   }

   // NewPlanningStep wires the planning step with its two IO seams: the
   // Claude runner (LLM verdict) and the CHANGELOG fetcher (GitHub contents
   // API).
   func NewPlanningStep(runner claudelib.ClaudeRunner, fetcher githubchangelog.Fetcher) agentlib.Step {
       return &planningStep{runner: runner, fetcher: fetcher}
   }

   // Name implements agentlib.Step.
   func (s *planningStep) Name() string { return "github-release-plan" }

   // ShouldRun always returns true. The planning step is idempotent: a
   // re-trigger replaces the existing ## Plan section in place. Returning
   // false here would silently skip routing.
   func (s *planningStep) ShouldRun(_ context.Context, _ *agentlib.Markdown) (bool, error) {
       return true, nil
   }

   // Run executes the planning pipeline. Five outcomes:
   //   1. Missing frontmatter        → escalate (Done, ## Plan needs_input,  clear assignee)
   //   2. CHANGELOG fetch fails      → Failed (controller retries)
   //   3. P1/P2 validation fails     → escalate
   //   4. Claude verdict unparseable → Failed (controller retries)
   //   5. semver.BumpVersion fails   → escalate
   //   6. Happy path                 → Done, NextPhase = execution, ## Plan ready
   func (s *planningStep) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
       // (1) Frontmatter validation — escalate path.
       missingField, currentVersion, repo, cloneURL, ref := s.readRequired(md)
       if missingField != "" {
           glog.V(2).Infof("planning: missing frontmatter field=%s — escalating", missingField)
           return s.escalate(ctx, md, escalation{
               reason:             "required frontmatter field missing: " + missingField,
               preconditionFailed: PreconditionMissingFrontmatter + missingField,
               currentVersion:     currentVersion,
           })
       }

       owner, name, ok := parseOwnerRepo(repo)
       if !ok {
           // repo frontmatter present but not "owner/name" — treat as
           // missing-field escalation with a descriptive precondition.
           glog.V(2).Infof("planning: malformed repo=%q — escalating", repo)
           return s.escalate(ctx, md, escalation{
               reason:             `frontmatter "repo" must be "owner/name"; got ` + repo,
               preconditionFailed: PreconditionMissingFrontmatter + "repo",
               currentVersion:     currentVersion,
           })
       }
       _ = cloneURL // currently unused by planning; future execution step will use it

       // (2) Fetch CHANGELOG.md from the target repo at ref. Network errors
       // produce a Failed result so the controller retries; do not escalate
       // on transient failure.
       changelogBytes, err := s.fetcher.Fetch(ctx, owner, name, ref)
       if err != nil {
           glog.V(2).Infof("planning: fetch CHANGELOG.md failed: %v", err)
           return &agentlib.Result{
               Status:  agentlib.AgentStatusFailed,
               Message: "fetch CHANGELOG.md: " + err.Error(),
           }, nil
       }

       // (3) Validate Unreleased preconditions — P1/P2 → escalate.
       valid, reason, _ := changelog.ValidateUnreleased(changelogBytes)
       if !valid {
           precondition := classifyValidationFailure(reason)
           glog.V(2).Infof("planning: validate Unreleased failed precondition=%s reason=%q", precondition, reason)
           return s.escalate(ctx, md, escalation{
               reason:             reason,
               preconditionFailed: precondition,
               currentVersion:     currentVersion,
           })
       }

       bullets := changelog.ExtractUnreleasedBullets(changelogBytes)
       prefixStyle := changelog.InferHeaderPrefixStyle(changelogBytes)

       // (4) Ask Claude to classify the bump.
       userMsg := strings.Join(bullets, "\n")
       fullPrompt := prompts.BumpClassificationPrompt() + "\n\n## Bullets to classify\n\n" + userMsg
       runResult, err := s.runner.Run(ctx, fullPrompt)
       if err != nil {
           glog.V(2).Infof("planning: claude runner failed: %v", err)
           return &agentlib.Result{
               Status:  agentlib.AgentStatusFailed,
               Message: "claude run: " + err.Error(),
           }, nil
       }
       verdict, err := prompts.ParseBumpVerdict(runResult.Result)
       if err != nil {
           glog.V(2).Infof("planning: parse verdict failed: %v", err)
           return &agentlib.Result{
               Status:  agentlib.AgentStatusFailed,
               Message: "parse bump verdict: " + err.Error(),
           }, nil
       }

       // (5) Compute next version — semver errors are escalation, not retry:
       // a malformed current_version cannot be retried by the controller, it
       // needs operator/watcher intervention.
       nextNumeric, err := semver.BumpVersion(currentVersion, verdict.Bump)
       if err != nil {
           glog.V(2).Infof("planning: bump version failed: %v", err)
           return s.escalate(ctx, md, escalation{
               reason:             err.Error(),
               preconditionFailed: PreconditionBadCurrentVersion,
               currentVersion:     currentVersion,
           })
       }

       // (6) Happy path — write ## Plan(ready), advance to execution.
       header := "## " + prefixStyle + nextNumeric
       output := PlanOutput{
           Outcome:           PlanOutcomeReady,
           Bump:              verdict.Bump,
           Reasoning:         verdict.Reasoning,
           CurrentVersion:    currentVersion,
           NextVersion:       nextNumeric,
           NextVersionHeader: header,
           HeaderPrefixStyle: prefixStyle,
           Bullets:           bullets,
       }
       section, err := agentlib.MarshalSectionTyped(ctx, "## Plan", output)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "marshal ## Plan section")
       }
       md.ReplaceSection(section)

       return &agentlib.Result{
           Status:    agentlib.AgentStatusDone,
           NextPhase: string(domain.TaskPhaseExecution),
       }, nil
   }

   // escalation captures the fields the escalate path needs to assemble the
   // needs_input PlanOutput. Keeping it as a value type makes the call sites
   // explicit and prevents missing-field bugs.
   type escalation struct {
       reason             string
       preconditionFailed string
       currentVersion     string
   }

   // escalate writes a ## Plan(needs_input) section, clears `assignee`,
   // sets `previous_assignee: github-releaser-agent`, and returns Done.
   // status + phase are LEFT UNCHANGED — per spec 047 § Constraints and
   // [[Agent Task File Contract]] escalation rule.
   //
   // Returning Done (NOT Failed/NeedsInput) is deliberate: the step succeeded
   // at producing a verdict (the verdict is "needs operator input"). The
   // controller does not retry a Done result; the human operator re-delegates
   // by re-setting assignee.
   func (s *planningStep) escalate(
       ctx context.Context,
       md *agentlib.Markdown,
       e escalation,
   ) (*agentlib.Result, error) {
       output := PlanOutput{
           Outcome:            PlanOutcomeNeedsInput,
           Reason:             e.reason,
           PreconditionFailed: e.preconditionFailed,
           CurrentVersion:     e.currentVersion,
       }
       section, err := agentlib.MarshalSectionTyped(ctx, "## Plan", output)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "marshal ## Plan section (needs_input)")
       }
       md.ReplaceSection(section)

       // Frontmatter mutation: clear assignee, set previous_assignee.
       // Use direct map writes; TaskFrontmatter is map[string]interface{}.
       md.Frontmatter["assignee"] = ""
       md.Frontmatter["previous_assignee"] = AgentLogin

       return &agentlib.Result{
           Status:  agentlib.AgentStatusDone,
           Message: e.reason,
       }, nil
   }

   // readRequired pulls the five required frontmatter fields. Returns the
   // first missing field's name (or "" if all present), plus the resolved
   // values for current_version, repo, clone_url, ref. Empty string counts
   // as missing.
   func (s *planningStep) readRequired(md *agentlib.Markdown) (missing, currentVersion, repo, cloneURL, ref string) {
       values := map[string]string{}
       for _, key := range requiredFrontmatterFields {
           v, _ := md.Frontmatter.String(key)
           if strings.TrimSpace(v) == "" {
               return key, values["current_version"], values["repo"], values["clone_url"], values["ref"]
           }
           values[key] = v
       }
       return "", values["current_version"], values["repo"], values["clone_url"], values["ref"]
   }

   // parseOwnerRepo splits an "owner/name" string. Empty or no-slash input
   // returns ok=false.
   func parseOwnerRepo(s string) (owner, name string, ok bool) {
       parts := strings.SplitN(s, "/", 2)
       if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
           return "", "", false
       }
       return parts[0], parts[1], true
   }

   // classifyValidationFailure maps the validator's reason string to the
   // typed PreconditionFailed value. The reason strings are produced by
   // changelog.ValidateUnreleased in pkg/changelog/changelog.go.
   func classifyValidationFailure(reason string) string {
       switch {
       case strings.Contains(reason, "is not the first ## section"):
           return PreconditionP1UnreleasedNotFirst
       case strings.Contains(reason, "no bullet entries"),
           strings.Contains(reason, "not found"):
           return PreconditionP2UnreleasedEmpty
       default:
           return PreconditionP2UnreleasedEmpty
       }
   }
   ```

   Notes:
   - Imports include `domain "github.com/bborbe/vault-cli/pkg/domain"` for `domain.TaskPhaseExecution` (already in the snippet's import block above).
   - The snippet above uses `PreconditionMissingFrontmatter` (bare identifier — same `package pkg`, exported constant from `pkg/plan_output.go` shipped by prompt 1).
   - All errors via `bborbe/errors`. NO `fmt.Errorf`. NO `strings.Index` for section work — use `agentlib.MarshalSectionTyped` / `md.ReplaceSection`.
   - `string(domain.TaskPhaseExecution)` cast because `Result.NextPhase` is a plain string field (see `agentlib.Result` definition).
   - `cloneURL` is read but currently unused — kept in the return for the execution-phase spec (separate). The `_ = cloneURL` discard avoids unused-variable failures.

5. **Write `agent/github-releaser/pkg/steps_planning_test.go`** — external test package (`package pkg_test`). Ginkgo + Gomega + counterfeiter mocks.

   Imports:

   ```go
   import (
       "context"

       agentlib "github.com/bborbe/agent/lib"
       claudelib "github.com/bborbe/agent/lib/claude"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
       githubchangelogmocks "github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog/mocks"
       agentmocks "github.com/bborbe/maintainer/agent/github-releaser/mocks"
   )
   ```

   (The mock type names match the counterfeiter directives: `agentmocks.ClaudeRunnerMock` for the Claude runner, `githubchangelogmocks.Fetcher` for the fetcher. Verify by reading the generated files.)

   Required test cases:

   **(A) Happy path — outcome=ready, NextPhase=execution**

   Test name: `It("ready path: emits ## Plan with outcome=ready and NextPhase=execution", func() { ... })`

   Setup:

   ```go
   fakeFetcher := &githubchangelogmocks.Fetcher{}
   fakeFetcher.FetchReturns([]byte("## Unreleased\n\n- feat: add foo\n- fix: bar\n\n## v1.7.7\n\n- old\n"), nil)

   fakeRunner := &agentmocks.ClaudeRunnerMock{}
   fakeRunner.RunReturns(&claudelib.ClaudeResult{
       Result: `{"bump":"minor","reasoning":"feat: stub"}`,
   }, nil)

   step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

   taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-bborbe-maintainer-master-001\n---\n\n# release task\n"

   md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
   Expect(err).NotTo(HaveOccurred())

   result, err := step.Run(context.Background(), md)
   Expect(err).NotTo(HaveOccurred())
   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
   Expect(result.NextPhase).To(Equal("execution"))

   plan, err := agentlib.ExtractSection[pkg.PlanOutput](context.Background(), md, "## Plan")
   Expect(err).NotTo(HaveOccurred())
   Expect(plan.Outcome).To(Equal("ready"))
   Expect(plan.Bump).To(Equal("minor"))
   Expect(plan.CurrentVersion).To(Equal("v1.7.7"))
   Expect(plan.NextVersion).To(Equal("1.8.0"))
   Expect(plan.NextVersionHeader).To(Equal("## v1.8.0"))
   Expect(plan.HeaderPrefixStyle).To(Equal("v"))
   Expect(plan.Bullets).To(ContainElements("feat: add foo", "fix: bar"))
   ```

   This test exercises: fetcher mock → changelog.ValidateUnreleased → bullets extraction → Claude mock returns verdict → ParseBumpVerdict → semver.BumpVersion("v1.7.7", "minor") → "1.8.0" → header "## v1.8.0" → MarshalSectionTyped → ExtractSection round-trip.

   The `grep -c 'NextVersion.*"1.8.0"'` AC requires the literal `"1.8.0"` text in this test file — the `Expect(plan.NextVersion).To(Equal("1.8.0"))` line satisfies it.

   **(B) Escalation — P1 not-first heading**

   Test name: `It("P1 escalation: ## Unreleased not first → outcome=needs_input + assignee cleared", func() { ... })`

   Setup uses a CHANGELOG where `## v1.2.6` appears BEFORE `## Unreleased`:

   ```go
   badChangelog := []byte("# Changelog\n\nIntro text.\n\n## v1.2.6\n\n- old release\n\n## Unreleased\n\n- new bullet\n")
   fakeFetcher := &githubchangelogmocks.Fetcher{}
   fakeFetcher.FetchReturns(badChangelog, nil)
   fakeRunner := &agentmocks.ClaudeRunnerMock{} // not called on escalation
   step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

   // ... same task frontmatter as case A but current_version: v1.2.6 ...

   result, err := step.Run(context.Background(), md)
   Expect(err).NotTo(HaveOccurred())
   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
   // NextPhase empty — caller stays in planning per spec 047 Desired Behavior 6.
   Expect(result.NextPhase).To(BeEmpty())

   // Fetcher called, Claude NOT called (escalation short-circuits before claude).
   Expect(fakeRunner.RunCallCount()).To(Equal(0))

   plan, err := agentlib.ExtractSection[pkg.PlanOutput](context.Background(), md, "## Plan")
   Expect(err).NotTo(HaveOccurred())
   Expect(plan.Outcome).To(Equal("needs_input"))
   Expect(plan.PreconditionFailed).To(Equal("P1_unreleased_not_first"))
   Expect(plan.Reason).To(ContainSubstring("not the first ## section"))

   // Frontmatter mutations:
   gotAssignee, _ := md.Frontmatter.String("assignee")
   Expect(gotAssignee).To(Equal(""))
   gotPrevAssignee, _ := md.Frontmatter.String("previous_assignee")
   Expect(gotPrevAssignee).To(Equal("github-releaser-agent"))
   gotStatus, _ := md.Frontmatter.String("status")
   Expect(gotStatus).To(Equal("in_progress"))
   gotPhase, _ := md.Frontmatter.String("phase")
   Expect(gotPhase).To(Equal("planning"))
   ```

   These 5 frontmatter assertions are the load-bearing AC checks (`grep -c 'previous_assignee.*github-releaser-agent'` ≥ 1 etc.).

   **(C) Escalation — missing required frontmatter field**

   Test name: `It("missing clone_url → outcome=needs_input + precondition_failed=missing_frontmatter_clone_url", func() { ... })`

   Setup omits `clone_url` from frontmatter; fetcher must NOT be called:

   ```go
   fakeFetcher := &githubchangelogmocks.Fetcher{}
   fakeRunner := &agentmocks.ClaudeRunnerMock{}
   step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

   taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-001\n---\n"
   // clone_url intentionally missing

   md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
   result, err := step.Run(context.Background(), md)
   Expect(err).NotTo(HaveOccurred())
   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
   Expect(fakeFetcher.FetchCallCount()).To(Equal(0))

   plan, _ := agentlib.ExtractSection[pkg.PlanOutput](context.Background(), md, "## Plan")
   Expect(plan.Outcome).To(Equal("needs_input"))
   Expect(plan.PreconditionFailed).To(Equal("missing_frontmatter_clone_url"))
   ```

   **(D) Failed — fetch error returns Status=Failed (controller retries)**

   Test name: `It("fetcher transport error → Status=Failed", func() { ... })`

   ```go
   fakeFetcher := &githubchangelogmocks.Fetcher{}
   fakeFetcher.FetchReturns(nil, errors.New("dial tcp: connection refused"))
   // ... full frontmatter ...
   result, _ := step.Run(context.Background(), md)
   Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
   Expect(result.Message).To(ContainSubstring("fetch CHANGELOG.md"))
   ```

   Import `"errors"` (stdlib) for `errors.New` in tests OR build a sentinel error via `bborbe/errors.New`.

   **(E) Failed — claude verdict unparseable returns Status=Failed**

   Test name: `It("claude returns malformed JSON → Status=Failed", func() { ... })`

   ```go
   fakeFetcher.FetchReturns([]byte("## Unreleased\n\n- feat: x\n"), nil)
   fakeRunner.RunReturns(&claudelib.ClaudeResult{Result: "not-json-at-all"}, nil)
   result, _ := step.Run(context.Background(), md)
   Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
   Expect(result.Message).To(ContainSubstring("parse bump verdict"))
   ```

   **(F) Escalation — bad current_version → outcome=needs_input + precondition_failed=bad_current_version**

   Test name: `It("malformed current_version → outcome=needs_input + precondition_failed=bad_current_version", func() { ... })`

   ```go
   fakeFetcher.FetchReturns([]byte("## Unreleased\n\n- feat: x\n"), nil)
   fakeRunner.RunReturns(&claudelib.ClaudeResult{Result: `{"bump":"minor","reasoning":"x"}`}, nil)
   // frontmatter current_version: "garbage"
   result, _ := step.Run(context.Background(), md)
   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
   plan, _ := agentlib.ExtractSection[pkg.PlanOutput](context.Background(), md, "## Plan")
   Expect(plan.Outcome).To(Equal("needs_input"))
   Expect(plan.PreconditionFailed).To(Equal("bad_current_version"))
   ```

   **(G) Idempotency — re-run replaces existing ## Plan in place**

   Test name: `It("idempotent: re-running with existing ## Plan replaces it", func() { ... })`

   ```go
   // Prime the markdown with a stale ## Plan section.
   taskMD := /* full frontmatter */ + "\n## Plan\n\n```json\n{\"outcome\":\"stale\"}\n```\n"
   // Then run the step normally; assert there's exactly ONE ## Plan section in md.Sections
   // after Run, and its content reflects the FRESH outcome (not "stale").
   var planCount int
   for _, sec := range md.Sections {
       if sec.Heading == "## Plan" {
           planCount++
       }
   }
   Expect(planCount).To(Equal(1))
   ```

   This covers Failure Modes table row "Same task re-triggered (idempotency)".

6. **Coverage ≥ 75% on `pkg/steps_planning.go`**:

   ```bash
   cd agent/github-releaser && go test -cover ./pkg/...
   ```

   Cases A-G above hit: happy path, P1 escalation, missing frontmatter, fetch error, claude parse error, bad current_version escalation, idempotent replace. That covers every branch in Run + escalate + readRequired + classifyValidationFailure + parseOwnerRepo. If coverage drops below 75%, add a case for P2 escalation (empty Unreleased): `changelog := []byte("## Unreleased\n\n## v1.0.0\n\n- old\n")` and assert `PreconditionFailed == "P2_unreleased_empty"`.

7. **Final verification**: from `agent/github-releaser/`:

   ```bash
   cd agent/github-releaser && make precommit
   ```

   Must exit 0. No `strings.Index` in `pkg/steps_planning.go`. No `fmt.Errorf` in `pkg/steps_planning.go`. No raw `"planning"` / `"execution"` string literals in `pkg/steps_planning.go` (use typed constants).

</requirements>

<constraints>
- File path: `agent/github-releaser/pkg/steps_planning.go` (flat at `pkg/` root, NOT `pkg/steps/`).
- Step interface: `agentlib.Step` from `github.com/bborbe/agent/lib`. Three methods: `Name() string`, `ShouldRun(ctx, *Markdown) (bool, error)`, `Run(ctx, *Markdown) (*Result, error)`.
- Constructor signature FROZEN: `func NewPlanningStep(runner claudelib.ClaudeRunner, fetcher githubchangelog.Fetcher) agentlib.Step`.
- `Name()` returns the literal `"github-release-plan"`.
- Step name `AgentLogin` constant MUST equal `"github-releaser-agent"` (spec § Constraints, grep-asserted by AC).
- Phase constants typed: use `domain.TaskPhaseExecution` from `github.com/bborbe/vault-cli/pkg/domain`. NEVER write the string literal `"execution"` or `"planning"` in `pkg/steps_planning.go`. Grep AC: `grep -c '"planning"\|"execution"' pkg/steps_planning.go` returns 0.
- Section I/O via `agentlib.MarshalSectionTyped` + `md.ReplaceSection`. NEVER `strings.Index` for sections.
- Errors via `github.com/bborbe/errors` (`Wrapf`/`Errorf`). `fmt.Errorf` is BANNED.
- Escalation contract per spec § Constraints AND § Desired Behavior 6:
  - `assignee` → `""`
  - `previous_assignee` → `"github-releaser-agent"`
  - `status` UNCHANGED
  - `phase` UNCHANGED
  - Result: `Status: Done`, `NextPhase: ""`, no error.
- Fetch failure returns `Status: Failed` (controller retries). NOT escalation. Per Failure Modes row 2.
- semver.BumpVersion failure returns escalation with `precondition_failed: "bad_current_version"`. Per Failure Modes row 6.
- Counterfeiter directive: replicate `agent/pr-reviewer/pkg/steps_mocks.go` form. Pin counterfeiter to `v6.12.2` in the suite file's `//go:generate` line.
- Test framework: Ginkgo v2 + Gomega; external test package `package pkg_test`.
- Coverage target: ≥ 75% on `pkg/steps_planning.go`.
- Test imports: `pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"`, `githubchangelogmocks "github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog/mocks"`, `agentmocks "github.com/bborbe/maintainer/agent/github-releaser/mocks"`.
- ClaudeRunner mock TYPE NAME: `ClaudeRunnerMock` (from the counterfeiter `--fake-name` flag — match pr-reviewer's pattern verbatim).
- Fetcher mock TYPE NAME: `Fetcher` (from `--fake-name Fetcher` in `pkg/githubchangelog/fetcher.go` directive from prompt 1).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` is green before AND after.
- License header (3 lines) at the top of every `.go` file.
</constraints>

<verification>

Run from the repo root unless noted.

```bash
# Build + tests pass + coverage ≥ 75%
cd agent/github-releaser && make precommit                              # exit 0
cd agent/github-releaser && go test -cover ./pkg/...                    # steps_planning.go ≥ 75%

# Files exist
ls agent/github-releaser/pkg/steps_planning.go                          # exists
ls agent/github-releaser/pkg/steps_planning_test.go                     # exists
ls agent/github-releaser/pkg/steps_mocks.go                             # exists
ls agent/github-releaser/pkg/pkg_suite_test.go                          # exists
ls agent/github-releaser/mocks/claude-runner.go                         # exists (counterfeiter output)

# Frozen step constructor
grep -c '^func NewPlanningStep(' agent/github-releaser/pkg/steps_planning.go                       # =1
grep -c 'func .* Name() string'  agent/github-releaser/pkg/steps_planning.go                       # ≥1
grep -c 'func .* ShouldRun(' agent/github-releaser/pkg/steps_planning.go                           # ≥1
grep -c 'func .* Run(' agent/github-releaser/pkg/steps_planning.go                                 # ≥1
grep -c 'github-release-plan' agent/github-releaser/pkg/steps_planning.go                          # ≥1
grep -c 'github-releaser-agent' agent/github-releaser/pkg/steps_planning.go                        # ≥1

# Error-wrapping convention (bborbe/errors only)
grep -c 'fmt.Errorf' agent/github-releaser/pkg/steps_planning.go                                   # =0
grep -c 'strings.Index' agent/github-releaser/pkg/steps_planning.go                                # =0
grep -cE '"(planning|execution)"' agent/github-releaser/pkg/steps_planning.go                      # =0
grep -c 'domain.TaskPhaseExecution' agent/github-releaser/pkg/steps_planning.go                    # ≥1

# Section I/O via agentlib helpers (not raw string scanning)
grep -c 'agentlib.MarshalSectionTyped' agent/github-releaser/pkg/steps_planning.go                 # ≥1

# Integration test ACs (from spec 047)
grep -c 'NextVersion.*"1.8.0"' agent/github-releaser/pkg/steps_planning_test.go                    # ≥1
grep -c 'precondition_failed\|PreconditionFailed.*P1_unreleased_not_first' agent/github-releaser/pkg/steps_planning_test.go  # ≥1
grep -c 'previous_assignee.*github-releaser-agent' agent/github-releaser/pkg/steps_planning_test.go  # ≥1
grep -c '"in_progress"' agent/github-releaser/pkg/steps_planning_test.go                           # ≥1
grep -c '"planning"' agent/github-releaser/pkg/steps_planning_test.go                              # ≥1

# Make targets green
cd agent/github-releaser && make test
```

</verification>
