---
status: completed
spec: [060-github-releaser-major-bump-guard]
summary: 'Implemented spec 060 major-bump guard in github-releaser planning step: extended PlanOutput with AllowMajorBumpConfig/AllowMajorBumpFlag fields and PreconditionMajorBumpNotAllowed constant, threaded allowMajor through NewPlanningStep/factory.CreateAgent/entry points, merged resolveChangelogRewrite into resolveMaintainerConfig, inserted the major-bump guard in runClassification with both audit log lines, and updated factory_test.go; go build ./... exits 0'
container: maintainer-major-bump-guard-exec-236-spec-060-guard-logic-planoutput-fields
dark-factory-version: v0.175.0
created: "2026-06-03T15:05:00Z"
queued: "2026-06-03T14:34:36Z"
started: "2026-06-03T14:42:26Z"
completed: "2026-06-03T14:52:52Z"
branch: dark-factory/github-releaser-major-bump-guard
---

<summary>
- The planning step's `runClassification` gains a guard between `semver.BumpVersion` and `publishPlan` that blocks a `major` verdict unless EITHER `cfg.Release.AllowMajorBump == true` OR the per-run CLI override `AllowMajor` is set
- When the guard trips, the step writes `## Plan` with `outcome: needs_input`, `precondition_failed: major_bump_not_allowed`, the classifier's reasoning string verbatim, the cited bullets, the resolved values of `allow_major_bump_config` and `allow_major_bump_flag` so the operator can see which lever to flip, and applies the FROZEN spec-047 escalation contract (clear assignee, set previous_assignee, status + phase UNCHANGED) and returns `Status: NeedsInput`
- `PlanOutput` gains two new optional fields (`AllowMajorBumpConfig bool` with `json:"allow_major_bump_config,omitempty"` and `AllowMajorBumpFlag bool` with `json:"allow_major_bump_flag,omitempty"`) and a new precondition constant `PreconditionMajorBumpNotAllowed = "major_bump_not_allowed"` (placed alongside the existing P1/P2/missing-frontmatter/bad-current-version constants in `plan_output.go`)
- `NewPlanningStep` signature gains a fifth parameter `allowMajor bool`; the factory `CreateAgent` accepts a sixth parameter and threads the value down; both `main.go` entry points pass `a.AllowMajor` to `CreateAgent`
- When the override path runs (major + flag=true), a `glog.V(2)` line containing the literal `--allow-major override` is emitted so kubectl-logs greps surface operator overrides; on the trip case, a `glog.V(2)` line containing the literal `major bump not allowed` is emitted
- The execution and ai_review phases are untouched; the guard fires before `NextPhase: execution` is returned
</summary>

<objective>
Implement the spec 060 major-bump guard in the planning step: extend `PlanOutput` with the two new audit-trail fields and the new precondition constant; extend `NewPlanningStep` to accept the per-run `allowMajor bool`; insert the guard decision-table check between `semver.BumpVersion` and `publishPlan`; update the factory and both `main.go` entry points to thread the new value through. The guard is the only behavior change — every other planning-step path is preserved.
</objective>

<context>
Read `CLAUDE.md` and `agent/github-releaser/CLAUDE.md` for project conventions.

Read these files BEFORE editing:
- `/workspace/agent/github-releaser/pkg/plan_output.go` — the existing `PlanOutput` struct. Add the two new fields (`AllowMajorBumpConfig bool` and `AllowMajorBumpFlag bool`) with their json tags, and the new `PreconditionMajorBumpNotAllowed = "major_bump_not_allowed"` constant in the existing precondition `const ( ... )` block. Do NOT remove or rename any existing field. The existing `ChangelogRewrite *bool` field with `omitempty` is the closest semantic precedent for "audit-trail resolution value" — mirror its godoc style.
- `/workspace/agent/github-releaser/pkg/steps_planning.go` — the planning step. The current `Run` calls `s.runClassification(...)` which in turn calls `semver.BumpVersion`, then `resolveRewriteVerdict`, then `publishPlan`. Insert the new guard check between the `semver.BumpVersion` call (line 304) and the `resolveRewriteVerdict` call (line 314) — or, equivalently, between `runClassification`'s `BumpVersion` block and its `RewriteVerdict` block. The check has access to `verdict.Bump`, `nextNumeric`, `currentVersion`, `bullets`, `prefixStyle`, `originalBody`, and (via new struct fields) the resolved `allowMajorBumpConfig` and `allowMajorFlag`. The `s.resolveChangelogRewrite` method (added in spec 059 prompt 2) is the right place to ALSO resolve `allowMajorBumpConfig` from the parsed config — extend it to return the second value, or add a parallel `resolveAllowMajorBumpConfig` helper. The cleanest path: extend `resolveChangelogRewrite` to also return `cfg.Release.AllowMajorBump` (or refactor to a single `resolveMaintainerConfig` that returns a struct of all flag values), then add the new `allowMajor bool` field on `planningStep` directly.
- `/workspace/agent/github-releaser/pkg/steps_planning_test.go` — DO NOT EDIT in this prompt. The Ginkgo decision-table tests (four cases: trip, repo-opt-in, flag-opt-in, minor no-op) ship in prompt 4. The existing tests will fail to compile after this prompt's `NewPlanningStep` signature change; that's expected — prompt 4 updates them. The build at this prompt's `go build ./...` step will succeed because the test file is excluded by the Go build system.
- `/workspace/agent/github-releaser/pkg/factory/factory.go` — the factory's `CreateAgent` function. Change the signature to accept a sixth `allowMajor bool` parameter and pass it to the new `NewPlanningStep` call. Update `CreateAgentProvider` to also accept the parameter and pass it through.
- `/workspace/agent/github-releaser/main.go` — the Kafka entry. Update the `factory.CreateAgent(...)` call to pass `a.AllowMajor`. The `pkg.BuildEnv(...)` call already forwards `a.AllowMajor` to the env map (per prompt 2), but the factory call needs an explicit argument too — the value flows into `NewPlanningStep` as a constructor parameter, not via the env map.
- `/workspace/agent/github-releaser/cmd/run-task/main.go` — the local-CLI entry. Same as above — pass `a.AllowMajor` to `factory.CreateAgent(...)`.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

Verified symbols (from module source — grep-confirmed):
- `prompts.BumpVerdict{Bump string, Reasoning string}` from `agent/github-releaser/pkg/prompts/prompts.go:61`. The guard reads `verdict.Bump == "major"` to trigger; other values pass through.
- `prompts.RewriteVerdict{RewriteNeeded bool, RewrittenUnreleased string, Reasoning string}` — unchanged. The guard runs BEFORE `resolveRewriteVerdict`, so the rewrite flag is irrelevant to the trip case.
- `semver.BumpVersion(ctx, currentVersion, bump string) (nextNumeric string, err error)` — called at `steps_planning.go:304`. The guard runs AFTER this call so it has `nextNumeric` to populate `## Plan.NextVersion` on the trip case.
- `agentlib.Markdown` — `md.Frontmatter["assignee"] = ""` and `md.Frontmatter["previous_assignee"] = AgentLogin` are the FROZEN escalation contract from spec 047. Use the existing `escalate` helper (which already does both) for the trip case; do NOT hand-roll a parallel frontmatter mutation.
- `escalate` helper at `steps_planning.go:470` — populates `PlanOutput{Outcome: NeedsInput, Reason, PreconditionFailed, CurrentVersion}` and mutates frontmatter. The trip case MUST use this helper, not a new parallel one. The `escalation` struct can carry the new `NextVersion` / `Bullets` / `Reasoning` fields OR the trip case can call `escalate` and then separately extend the `PlanOutput` with a follow-up `MarshalSectionTyped` call. The cleaner path: extend the existing `escalate` helper to accept a richer `escalation` struct that can carry `nextVersion` / `nextVersionHeader` / `bullets` / `reasoning` / `allowMajorBumpConfig` / `allowMajorBumpFlag`. See requirement 3.
- `agentlib.AgentStatusNeedsInput`, `agentlib.AgentStatusDone`, `agentlib.AgentStatusFailed` — the three status values the planning step can return. The trip case returns `AgentStatusNeedsInput` (NOT `Failed`, NOT `Done`).
- `domain.TaskPhaseExecution` — the existing `NextPhase: string(domain.TaskPhaseExecution)` literal in `publishPlan`. The trip case does NOT advance the phase; it returns `NextPhase: ""` (implicit, via the `escalate` helper which doesn't set NextPhase).
- `glog.V(2).Infof("...major bump not allowed...", ...)` — the trip-case log line. The substring `major bump not allowed` is FIXED (the spec's AC `grep -c 'major_bump_not_allowed'` is for the constant; the log line uses the spaces form).
- `glog.V(2).Infof("...--allow-major override...")` — the override-path log line. The substring `--allow-major override` is FIXED.
- `mocks.MaintainerConfigFetcher` is generated by spec 059 prompt 1. The existing tests use the default mock (returns `(nil, nil)`); the new prompt 4 tests will wire a custom `FetchReturns(...)` to exercise the trip case.
</context>

<requirements>

1. **Extend `PlanOutput` and add the new precondition constant.** In `/workspace/agent/github-releaser/pkg/plan_output.go`, ADD (do NOT replace) the following:

   a. Two new struct fields. Insert them after the existing `ConfigFetchWarning` field, before the `ErrorCategory` field, so the audit-trail block of the struct stays grouped:

   ```go
   // AllowMajorBumpConfig records the spec-060 per-repo YAML opt-in flag
   // value resolved at planning entry. Populated on outcome=needs_input
   // (trip case) so the operator can see which lever to flip. NOT
   // populated on outcome=ready (the happy path does not need an audit
   // trail) — omitempty removes the token from the JSON.
   AllowMajorBumpConfig bool `json:"allow_major_bump_config,omitempty"`

   // AllowMajorBumpFlag records the spec-060 per-run CLI override value
   // resolved at planning entry. Same shape and contract as
   // AllowMajorBumpConfig — populated on the trip case only, omitted
   // from JSON on the happy path.
   AllowMajorBumpFlag bool `json:"allow_major_bump_flag,omitempty"`
   ```

   Do NOT change the JSON tag names — the spec's AC `grep -c 'AllowMajorBumpConfig\s\+bool' agent/github-releaser/pkg/plan_output.go` returns 1 and `grep -c 'AllowMajorBumpFlag\s\+bool' agent/github-releaser/pkg/plan_output.go` returns 1.

   b. One new precondition constant. Add it to the existing `const ( ... )` block alongside `PreconditionP1UnreleasedNotFirst`, `PreconditionP2UnreleasedEmpty`, `PreconditionBadCurrentVersion`, and `PreconditionMissingFrontmatter`:

   ```go
   // PreconditionMajorBumpNotAllowed is set on the trip case of the
   // spec-060 major-bump guard. Trip condition: bump=major AND
   // cfg.Release.AllowMajorBump==false AND the per-run CLI override
   // is unset. The escalation block carries the classifier's reasoning
   // and the resolved values of both opt-in levers so the operator
   // can see which lever to flip.
   PreconditionMajorBumpNotAllowed = "major_bump_not_allowed"
   ```

   The acceptance criterion `grep -c 'PreconditionMajorBumpNotAllowed' agent/github-releaser/pkg/steps_planning.go` returns ≥ 1 — the constant is consumed there. The spec's AC `grep -c 'major_bump_not_allowed' agent/github-releaser/pkg/steps_planning.go` returns ≥ 1 — the string literal is referenced in the trip-case log line and in the `Reason` substring.

2. **Extend `planningStep` with the new `allowMajor` field.** In `/workspace/agent/github-releaser/pkg/steps_planning.go`, ADD a new struct field and update the constructor:

   ```go
   type planningStep struct {
       runner           claudelib.ClaudeRunner
       fetcher          githubchangelog.Fetcher
       maintainerConfig maintainerconfig.Fetcher
       allowMajor       bool
   }

   // NewPlanningStep wires the planning step with its four IO seams:
   //   - the Claude runner (LLM verdict for bump + rewrite)
   //   - the CHANGELOG.md fetcher (GitHub contents API)
   //   - the .maintainer.yaml fetcher (GitHub contents API, spec 059)
   //   - the spec-060 per-run override: when true, the major-bump guard
   //     is bypassed; equivalent to cfg.Release.AllowMajorBump==true.
   func NewPlanningStep(
       runner claudelib.ClaudeRunner,
       fetcher githubchangelog.Fetcher,
       maintainerConfig maintainerconfig.Fetcher,
       allowMajor bool,
   ) agentlib.Step {
       return &planningStep{
           runner:           runner,
           fetcher:          fetcher,
           maintainerConfig: maintainerConfig,
           allowMajor:       allowMajor,
       }
   }
   ```

   **Signature change is BREAKING.** Every `pkg.NewPlanningStep(...)` call site in test fixtures (`pkg/steps_planning_test.go`) and the factory call site in `pkg/factory/factory.go` will fail to compile. They are updated in this prompt (factory) and in prompt 4 (tests). Do NOT add a new constructor alongside the old one — Go has no default parameters; the prompt must update every call site atomically.

3. **Extend the `escalate` helper to carry the new fields.** The existing `escalate(ctx, md, escalation{...})` helper takes a struct with three fields (`reason`, `preconditionFailed`, `currentVersion`). The trip case needs to populate `nextVersion`, `nextVersionHeader`, `bump`, `bullets`, `reasoning`, `allowMajorBumpConfig`, and `allowMajorBumpFlag` on the `PlanOutput` so the operator sees the would-be release on the task page. Extend the `escalation` struct and the helper:

   ```go
   type escalation struct {
       reason             string
       preconditionFailed string
       currentVersion     string
       // Spec 060 trip-case fields. All optional — P1/P2 escalation
       // paths pass zero values; the major-bump guard trip case
       // populates all of them so the operator sees the would-be
       // release shape on the task page.
       nextVersion         string
       nextVersionHeader   string
       bump                string
       bullets             []string
       reasoning           string
       allowMajorBumpConfig bool
       allowMajorBumpFlag   bool
   }
   ```

   And update the `PlanOutput` assembly inside `escalate` to include the new fields when they're non-zero (the `omitempty` tags handle the rest — zero values produce no JSON tokens). Mirror the existing `publishPlan` field population: `Bump`, `NextVersion`, `NextVersionHeader`, `Bullets`, `Reasoning`, `AllowMajorBumpConfig`, `AllowMajorBumpFlag` go onto the `PlanOutput` literal. `Reason` is already populated from `e.reason`.

4. **Extend `resolveChangelogRewrite` to also return the spec-060 flag.** The cleanest path: rename the existing helper to `resolveMaintainerConfig` and have it return a struct (or just extend the existing multi-return). Simplest shape: return `(allowMajorBumpConfig bool, fetchWarning string, err error)`. The `changelogRewrite` value is still needed by the caller for the spec 059 decision table — read both fields from the parsed `cfg` inside the helper.

   ```go
   func (s *planningStep) resolveMaintainerConfig(
       ctx context.Context,
       owner, name, ref string,
   ) (changelogRewrite bool, allowMajorBump bool, fetchWarning string, err error)
   ```

   The semantics are the same as the existing `resolveChangelogRewrite` (per spec 059): `ErrFileNotFound` → both flags false, no error; transport error → both flags false, warning populated, no error; parse error → wrapped error. Update the call site in `Run` accordingly.

   If the rename is too invasive, the equivalent path is to add a parallel `resolveAllowMajorBump(ctx, owner, name, ref) (bool, error)` helper that calls `s.maintainerConfig.Fetch` again — but that double-fetches the config and breaks the spec-059 "flag-read-once" invariant. The merged helper is the correct shape.

5. **Insert the major-bump guard into `runClassification`.** Between the existing `semver.BumpVersion` block (around line 304) and the existing `resolveRewriteVerdict` call (around line 314), insert:

   ```go
   if verdict.Bump == "major" && !allowMajorBumpConfig && !s.allowMajor {
       glog.V(2).Infof(
           "planning: major bump not allowed: bump=major, allowMajorBumpConfig=%t, allowMajorFlag=%t, reasoning=%q",
           allowMajorBumpConfig, s.allowMajor, verdict.Reasoning,
       )
       prefixStyle := changelog.InferHeaderPrefixStyle(changelogBytes) // already computed earlier — pass it down
       header := "## " + prefixStyle + nextNumeric
       return s.escalate(ctx, md, escalation{
           reason:               "major bump not allowed: " + verdict.Reasoning,
           preconditionFailed:   PreconditionMajorBumpNotAllowed,
           currentVersion:       currentVersion,
           nextVersion:          nextNumeric,
           nextVersionHeader:    header,
           bump:                 verdict.Bump,
           bullets:              bullets,
           reasoning:            verdict.Reasoning,
           allowMajorBumpConfig: allowMajorBumpConfig,
           allowMajorBumpFlag:   s.allowMajor,
       })
   }
   ```

   The `changelogBytes` is already in scope (it's the result of `s.fetcher.Fetch(...)` at line 115); pass it through `runClassification` as a new parameter OR re-derive `prefixStyle` from the already-passed `prefixStyle string` (preferred — `runClassification` already receives `prefixStyle` as a parameter, so just reuse it; do not re-derive).

   Refactored: the guard does NOT need `changelogBytes`; it uses the already-passed `prefixStyle` parameter. Update the `escalate` call to:

   ```go
   header := "## " + prefixStyle + nextNumeric
   ```

   The `prefixStyle` is the existing parameter on `runClassification`; no new parameter threading needed.

6. **Emit the override-path log line when the major + flag case runs.** When the guard's trip condition does NOT fire because the CLI override was set (`s.allowMajor == true` and `verdict.Bump == "major"`), emit a `glog.V(2)` line so operator overrides are auditable in `kubectl logs`. Insert immediately AFTER the guard check (i.e. between the guard and the existing `resolveRewriteVerdict` call):

   ```go
   if verdict.Bump == "major" && s.allowMajor && !allowMajorBumpConfig {
       glog.V(2).Infof("planning: --allow-major override accepted for major bump")
   }
   ```

   The substring `--allow-major override` is FIXED (the spec mentions it in § Desired Behavior 6 and § Failure Modes row 3). The guard condition is exclusive — if `allowMajorBumpConfig` is also true, the operator did NOT need the override; the log line is for the override-only path so a future grep for the audit surface is unambiguous.

7. **Update `factory.CreateAgent` to thread the new parameter.** In `/workspace/agent/github-releaser/pkg/factory/factory.go`, change `CreateAgent` and `CreateAgentProvider` to accept a sixth `allowMajor bool` parameter and pass it to `NewPlanningStep`:

   ```go
   func CreateAgent(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       ghToken string,
       env map[string]string,
       allowMajor bool,
   ) *agentlib.Agent {
       // ...
       planningStep := releaserpkg.NewPlanningStep(
           planningRunner,
           fetcher,
           maintainerConfigFetcher,
           allowMajor,
       )
       // ...
   }
   ```

   And:

   ```go
   func CreateAgentProvider(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       ghToken string,
       env map[string]string,
       allowMajor bool,
   ) agentlib.AgentProvider {
       domainAgent := CreateAgent(claudeConfigDir, agentDir, model, ghToken, env, allowMajor)
       // ...
   }
   ```

8. **Update the two `main.go` call sites to pass `a.AllowMajor`.** In `/workspace/agent/github-releaser/main.go`, change the `factory.CreateAgentProvider(...)` call (lines 131-137) to pass `a.AllowMajor` as the sixth argument. In `/workspace/agent/github-releaser/cmd/run-task/main.go`, change the `factory.CreateAgentProvider(...)` call (lines 86-92) to pass `a.AllowMajor` as the sixth argument. The `factory.CreateAgent` call inside `CreateAgentProvider` is automatically updated (requirement 7).

9. **Sibling entry-point check (Go).** Before running `go build`, run:

   ```
   grep -rn 'pkg.NewPlanningStep\|NewPlanningStep(\|factory.CreateAgent\|factory.CreateAgentProvider' /workspace/agent/github-releaser/
   ```

   Expected hits (after this prompt's edits):
   - `agent/github-releaser/pkg/factory/factory.go` — `CreateAgent` calls `NewPlanningStep(..., allowMajor)` (4 args); `CreateAgentProvider` calls `CreateAgent(..., allowMajor)` (6 args).
   - `agent/github-releaser/main.go` — `factory.CreateAgentProvider(..., a.AllowMajor)`.
   - `agent/github-releaser/cmd/run-task/main.go` — same.
   - `agent/github-releaser/pkg/steps_planning_test.go` — every `pkg.NewPlanningStep(...)` call site. These will still pass 3 arguments in the existing tests (because this prompt doesn't update them — prompt 4 does), so the build will FAIL on the test file. Run `go build ./...` (not `go test ./...` or `make test`) to verify only the production binaries build cleanly. The test file's compile errors are EXPECTED at this prompt's gate; prompt 4 fixes them.

   If `pkg/steps_execution.go` or `pkg/steps_ai_review.go` or any other file references `NewPlanningStep` or `CreateAgent`, the new parameter must be plumbed there too — re-scope this prompt.

10. **Acceptance gate — `go build ./...` exits 0 in `agent/github-releaser`.** Run `cd /workspace/agent/github-releaser && go build ./...` and confirm exit code 0. The test file `pkg/steps_planning_test.go` will still pass 3 arguments to `NewPlanningStep` — its compile errors are EXPECTED and will be fixed by prompt 4. Do NOT run `go test ./...` or `make test` in this prompt; prompt 4 ships the test updates. Do NOT run `make precommit` — the full precommit runs the test suite which would fail.

11. **Cross-prompt dependency declaration.** This prompt depends on prompts 1 and 2 having shipped (the `cfg.Release.AllowMajorBump` symbol from prompt 1 and the `a.AllowMajor` field from prompt 2). Prompt 4 depends on this prompt having shipped (the `NewPlanningStep` 4-arg signature and the new `PlanOutput` fields). Prompt 5 depends on this prompt having shipped (the README/CHANGELOG references the new field and flag).
</requirements>

<constraints>
- The guard fires ONLY when `verdict.Bump == "major" && !allowMajorBumpConfig && !s.allowMajor`. All other combinations proceed. The decision table is FROZEN per spec 060 § Desired Behavior 3:
  - `patch` (any) → proceed
  - `minor` (any) → proceed
  - `major` + `allowMajorBumpConfig==true` → proceed
  - `major` + `allowMajorBumpConfig==false` + `s.allowMajor==true` → proceed (with override log)
  - `major` + `allowMajorBumpConfig==false` + `s.allowMajor==false` → TRIP
- The trip case MUST use the existing `escalate` helper — do NOT hand-roll a parallel frontmatter mutation. The FROZEN spec 047 contract is: `assignee: ""`, `previous_assignee: github-releaser-agent`, `status` + `phase` UNCHANGED, `Result.Status: AgentStatusNeedsInput`. The existing `escalate` already does all of this; the new fields flow through the extended `escalation` struct from requirement 3.
- The trip case returns `Status: AgentStatusNeedsInput` (NOT `Failed`, NOT `Done`). The deliverer maps `NeedsInput` to "no auto-retry, no advance, operator re-delegates by re-setting assignee" — same wiring as the existing P1/P2/missing-frontmatter escalation paths.
- The `PlanOutput.AllowMajorBumpConfig` and `PlanOutput.AllowMajorBumpFlag` fields MUST use `omitempty` so the happy-path JSON does NOT carry these tokens. The trip case populates both; the happy path leaves both zero-valued. The spec's failure-mode row 3 says the override-path's `glog.V(2)` is the audit surface, NOT a `## Plan` field — so the trip case carries the fields for the operator's benefit, but the override-proceed case does NOT.
- The override-path `glog.V(2)` line is emitted ONLY when the operator actually used the override (i.e. `verdict.Bump == "major" && s.allowMajor && !allowMajorBumpConfig`). When both opt-ins are true, the override is inert — do NOT log.
- The new `allowMajor` field on `planningStep` is a plain `bool`, NOT a `*bool`. Same shape as the resolved `cfg.Release.AllowMajorBump` value (a plain `bool` returned from `resolveMaintainerConfig`).
- The `NewPlanningStep` signature change is BREAKING and is updated atomically in this prompt. Every test fixture's `pkg.NewPlanningStep(...)` call site is updated in prompt 4.
- The `factory.CreateAgent` and `factory.CreateAgentProvider` signature changes are BREAKING and updated atomically in this prompt. Every entry-point call site (`main.go` and `cmd/run-task/main.go`) is updated in this prompt.
- The guard MUST run AFTER `semver.BumpVersion` so the escalation block can name the would-be `next_version`. The guard MUST run BEFORE `resolveRewriteVerdict` so a `major` trip does NOT spend an LLM call on the rewrite classification.
- Do NOT change the bump classifier or its prompt. Spec 060 § Non-goals explicitly forbids it.
- Do NOT add a `release.allowMinorBump` or `release.allowPatchBump` knob. Spec 060 § Non-goals explicitly forbids the symmetric knobs.
- Do NOT add an "auto-downgrade major to minor" fallback. Spec 060 § Non-goals explicitly forbids the silent downgrade.
- Do NOT add a `force-major-via-task-frontmatter` path. Spec 060 § Non-goals explicitly forbids the task-content override.
- Do NOT add Prometheus metrics, debug logging, or other observability beyond the existing `glog.V(2).Infof` pattern. The two new log lines (`major bump not allowed` on trip; `--allow-major override` on override-proceed) are the canonical audit surface.
- Do NOT commit — dark-factory handles git.
- Existing tests will fail to compile at this prompt's gate — that is EXPECTED, prompt 4 updates them.
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && go build ./...
```
Expected: exit code 0; both `application` structs compile with the new factory call; both `BuildEnv` call sites still pass 5 arguments; `NewPlanningStep` accepts 4 arguments at every production call site; `factory.CreateAgent` and `factory.CreateAgentProvider` accept 6 arguments.

Evidence commands the auditor will run:
- `grep -c 'AllowMajorBumpConfig\s\+bool' /workspace/agent/github-releaser/pkg/plan_output.go` → 1.
- `grep -c 'AllowMajorBumpFlag\s\+bool' /workspace/agent/github-releaser/pkg/plan_output.go` → 1.
- `grep -c 'PreconditionMajorBumpNotAllowed' /workspace/agent/github-releaser/pkg/steps_planning.go` → ≥ 1 (the constant is referenced; also imported via the `pkg.PreconditionMajorBumpNotAllowed` symbol if exposed).
- `grep -c 'major_bump_not_allowed' /workspace/agent/github-releaser/pkg/steps_planning.go` → ≥ 1 (the string literal in the trip-case log or in the `escalation.reason` substring).
- `grep -c '\-\-allow-major override' /workspace/agent/github-releaser/pkg/steps_planning.go` → 1 (the override-path log line).
- `grep -n 'func NewPlanningStep' /workspace/agent/github-releaser/pkg/steps_planning.go` → signature shows 4 parameters: `(runner, fetcher, maintainerConfig, allowMajor)`.
- `grep -n 'func CreateAgent' /workspace/agent/github-releaser/pkg/factory/factory.go` → signature shows 6 parameters: `(claudeConfigDir, agentDir, model, ghToken, env, allowMajor)`.
- `grep -rn 'factory.CreateAgentProvider' /workspace/agent/github-releaser/` → both call sites pass 6 arguments.
- `git diff master -- agent/github-releaser/pkg/steps_planning.go agent/github-releaser/pkg/plan_output.go agent/github-releaser/pkg/factory/factory.go agent/github-releaser/main.go agent/github-releaser/cmd/run-task/main.go | grep -c '^+.*fmt\.Errorf'` → 0 (no `fmt.Errorf` introduced; the touched files use `github.com/bborbe/errors` only).
- `cd /workspace/agent/github-releaser && go build ./...` → exit code 0.
</verification>
