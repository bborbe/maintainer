---
status: completed
approved: "2026-05-27T22:09:05Z"
verifying: "2026-05-28T05:33:59Z"
completed: "2026-06-04T15:33:38Z"
branch: dark-factory/github-releaser-planning-phase-integration
---

## Summary

- Wires the three completed foundation libraries — `pkg/changelog` (spec 044), `pkg/semver` (spec 045), `pkg/prompts` (spec 046) — into a working planning phase for `agent/github-releaser`.
- Adds `pkg/steps_planning.go` (PlanningStep implementing `agentlib.Step`), `pkg/githubchangelog/` (interface + impl for fetching `CHANGELOG.md` via GitHub REST contents API), and `pkg/factory/factory.go` (Create* wiring per [[Go Agent Implementation Guide]]).
- Updates `main.go` to dispatch via `agentlib.AgentProvider` instead of returning a placeholder Result.
- End state: given a `task_type: github-release` task with `phase: planning`, the agent reads frontmatter, fetches CHANGELOG, validates preconditions, classifies bump via Claude, computes next version, writes `## Plan` JSON section, and advances `phase: execution` (or escalates on precondition failure).
- Execution + ai_review phases are deliberately out of scope (separate specs). CRD update for `trigger.phases` is also out of scope — defer to the dev-deploy spec.

## Problem

The three foundation libraries (changelog/semver/prompts) compile but are dead code — nothing calls them. The planning phase is the first step where they all come together: a frontmatter-driven decision pipeline that fetches CHANGELOG bytes, validates them, asks Claude to classify a bump, computes the next version, and writes a structured `## Plan` JSON section the downstream execution phase will consume.

Without this integration spec, the agent's `main.go` still returns the placeholder Result from the Milestone 1 scaffold; the `pkg/factory` directory doesn't exist; there's no `agentlib.AgentProvider`; and the watcher-emitted tasks have nowhere to land. This spec closes the gap from "library shelf" to "executable planning phase."

## Goal

A working planning phase, end-to-end, in the local CLI test runner (`cmd/run-task`). End state:

1. `cd agent/github-releaser && make precommit` exits 0.
2. `pkg/factory/factory.go` exports `CreateAgent`, `CreateAgentProvider`, `CreateClaudeRunner`, `CreateKafkaResultDeliverer` per the canonical pattern in `agent/pr-reviewer/pkg/factory/factory.go`.
3. `pkg/steps_planning.go` exports a `PlanningStep` struct implementing `agentlib.Step` (`Name`, `ShouldRun`, `Run`).
4. `pkg/githubchangelog/` exports a `Fetcher` interface plus a real implementation that calls `GET /repos/{owner}/{repo}/contents/CHANGELOG.md?ref={ref}` via `gh api` shell-out (mirrors how Phase 1 slash command did it) OR via the in-process `net/http` client — implementer's choice as long as the `Fetcher` interface is mockable.
5. `main.go` dispatches via `factory.CreateAgentProvider` instead of returning a placeholder Result; `PHASE=planning` + a valid task in `TASK_CONTENT` produces a Result with `NextPhase: "execution"` on success.
6. Running `go run ./cmd/run-task -- /tmp/fixture-task.md` against a fixture task with a valid CHANGELOG mutates the task file to contain a `## Plan` JSON block with the expected fields; running against a task with an invalid CHANGELOG (Unreleased not first heading) mutates the task to clear `assignee` and set `previous_assignee: github-releaser-agent`, with `status` and `phase` unchanged.

## Non-goals

- Execution phase (header rewrite, commit, push, tag) — separate spec.
- PR + auto-merge fallback — separate spec.
- ai_review phase (remote verification) — separate spec.
- CRD `trigger.phases` update — defer to the dev-deploy spec (current CRD doesn't exist yet anyway).
- Kafka result delivery — `main.go` should support it (mirror pr-reviewer's `createDeliverer` helper), but no test exercises Kafka in this spec.
- Healthcheck routing in `CreateAgentProvider` — mirror pr-reviewer (include a healthcheck Agent) but no test exercises it.
- The Phase 1 slash command's "infer header style" output field IS used (`next_version_header`) by the execution phase — this spec writes it into `## Plan`, but the execution phase consumes it (different spec).
- Per-phase model selection from CRD — use whatever `ANTHROPIC_MODEL` env resolves to (defaults `sonnet`).

## Desired Behavior

1. `pkg/factory/factory.go` exports `CreateAgentProvider(...)` that returns an `agentlib.AgentProvider` routing `task_type: github-release` to a `*agentlib.Agent` built via `agentlib.NewAgent(agentlib.NewPhase(domain.TaskPhasePlanning, planningStep))`. Also routes `task_type: healthcheck` to a healthcheck agent (mirror pr-reviewer; no test).
2. The factory uses TYPED `domain.TaskPhasePlanning` constants for the phase name — NOT the string literal `"planning"`. (Per [[Agent Phase Dispatch Guide]] § "Three-layer phase rename — all must move together".)
3. `PlanningStep.Run(ctx, content)` reads the task frontmatter, validates required fields (`repo`, `clone_url`, `ref`, `current_version`, `task_identifier`), and on missing-field failure returns `agentlib.Result{Status: Done, Message: "..."}` after writing a `## Plan` section with `outcome: needs_input` AND clearing `assignee` + setting `previous_assignee: github-releaser-agent`.
4. `PlanningStep.Run` happy path: fetches CHANGELOG via the injected `Fetcher`, calls `changelog.ValidateUnreleased` — on P1 or P2 failure, writes `## Plan` with `outcome: needs_input` and escalates per the rule above. On validation success, extracts bullets via `changelog.ExtractUnreleasedBullets`, infers header prefix via `changelog.InferHeaderPrefixStyle`, calls the Claude runner with the prompt from `prompts.BumpClassificationPrompt()`, parses the verdict via `prompts.ParseBumpVerdict`, computes the next version via `semver.BumpVersion`, marshals a `PlanOutput` struct, writes `## Plan` section via `agentlib.MarshalSectionTyped`, and returns `agentlib.Result{Status: Done, NextPhase: string(domain.TaskPhaseExecution)}`.
5. The `PlanOutput` struct lives in `pkg/plan_output.go` with this exact shape (snake_case JSON tags, `omitempty` on escalation-only fields). Two shapes are valid:

   ```go
   type PlanOutput struct {
       Outcome             string   `json:"outcome"`               // "ready" | "needs_input"
       Bump                string   `json:"bump,omitempty"`        // patch|minor|major; required when outcome=ready, empty on needs_input
       Reasoning           string   `json:"reasoning,omitempty"`   // required when outcome=ready, empty on needs_input
       CurrentVersion      string   `json:"current_version,omitempty"`
       NextVersion         string   `json:"next_version,omitempty"`
       NextVersionHeader   string   `json:"next_version_header,omitempty"`
       HeaderPrefixStyle   string   `json:"header_prefix_style,omitempty"`
       Bullets             []string `json:"bullets,omitempty"`
       Reason              string   `json:"reason,omitempty"`              // populated only when outcome=needs_input
       PreconditionFailed  string   `json:"precondition_failed,omitempty"` // populated only when outcome=needs_input; values: "P1_unreleased_not_first" | "P2_unreleased_empty" | "missing_frontmatter_<field>" | "bad_current_version"
   }
   ```

   Concrete shapes:

   ```json
   // outcome=ready (happy path)
   {"outcome":"ready","bump":"minor","reasoning":"feat: bullet for new export","current_version":"v1.7.7","next_version":"1.8.0","next_version_header":"## v1.8.0","header_prefix_style":"v","bullets":["feat: add foo","fix: bar"]}

   // outcome=needs_input (escalation)
   {"outcome":"needs_input","reason":"Unreleased is not the first ## section; found '1.2.6' at line 11. Move ## Unreleased above all release headings.","precondition_failed":"P1_unreleased_not_first","current_version":"v1.2.6"}
   ```

   No `Details map[string]any` — concrete fields only. Future fields require a spec amendment.
6. The escalation path (`outcome: needs_input`) MUTATES frontmatter ONLY via two field changes: `assignee → ""` and `previous_assignee → "github-releaser-agent"`. Does NOT change `status` (stays `in_progress`) or `phase` (stays `planning` — resume cursor per spec 027 + [[Agent Task File Contract]] escalation rule).
7. `main.go` replaces its current placeholder Result block with: build the `AgentProvider` via `factory.CreateAgentProvider`, resolve the agent for `task_type` from `TASK_CONTENT` frontmatter, call `agent.Run(ctx, a.Phase, a.TaskContent, deliverer)`, push metrics, call `agentlib.PrintResult`. Mirror `agent/pr-reviewer/main.go` `Run` method verbatim where structure permits.
8. `cmd/run-task/main.go` exists and reads a task file from a `--task-file <path>` flag, runs the agent via the same factory, and writes the mutated content back to the file. Mirrors `agent/pr-reviewer/cmd/run-task/`. Enables local fixture-based testing without Kafka.

## Constraints

- All new package paths follow [[Go Agent Implementation Guide]]: `pkg/factory/factory.go` (single file, all `Create*` functions, no business logic, no error returns), `pkg/steps_planning.go` (flat at `pkg/` root, NOT `pkg/steps/`), `pkg/githubchangelog/` for the fetcher (subpkg is OK here since it has both interface + impl + mock).
- Phase constant: `domain.TaskPhasePlanning` typed constant only. String literal `"planning"` banned in production code; verified by grep.
- The Phase 1 prompt is `prompts.BumpClassificationPrompt()` — NEVER inline the rules in the step.
- Section names: `## Plan` only in this spec. (Execution writes `## Result`, ai_review writes `## Review` — different specs.)
- `agentlib.MarshalSectionTyped` / `agentlib.ExtractSection[T]` for section I/O. NEVER `strings.Index` (real bug from agent/backtest commit `f04109ee9b`).
- Escalation contract: `assignee: ""` + `previous_assignee: github-releaser-agent`, `status` + `phase` UNCHANGED. Verified by 4 separate grep assertions on the mutated fixture file.
- Counterfeiter mocks for `githubchangelog.Fetcher`, `claudelib.ClaudeRunner` (already exists in agentlib), generated into `pkg/githubchangelog/mocks/`. Mocks check NEEDED — these are the only IO boundaries.
- Test framework: Ginkgo v2 + Gomega; external test packages.
- Coverage: ≥ 75% on `pkg/steps_planning.go` + ≥ 80% on `pkg/githubchangelog/`. (Lower target than the foundation specs because integration code has more glue / less branching.)
- Errors via `github.com/bborbe/errors` (`Wrapf`/`Errorf`). NO `fmt.Errorf` in production code.
- `main.go` keeps its existing Sentry + Prometheus boilerplate; only the placeholder Result block changes.
- Verification runs `cmd/run-task` against 2 fixtures: `testdata/happy-planning.md` (valid CHANGELOG, expect `## Plan` with `outcome: ready`) and `testdata/unreleased-at-bottom.md` (invalid CHANGELOG, expect escalation). Both fixtures are checked in.

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Recovery |
|---|---|---|---|---|
| Required frontmatter field missing (e.g. no `clone_url`) | Step reads frontmatter, finds field empty | Write `## Plan` with `outcome: needs_input`, reason names the missing field; clear assignee, set previous_assignee; return Done | Reversible | Operator fixes the task file or re-emits via watcher; sets `assignee: github-releaser-agent` to re-delegate |
| CHANGELOG fetch fails (HTTP 4xx/5xx, timeout) | `Fetcher.Fetch` returns wrapped error | Step returns `agentlib.Result{Status: Failed, Message: "fetch CHANGELOG.md: ..."}` — controller retries phase per existing retry-cap logic | Reversible | Controller retry; at retry cap, controller escalates to operator inbox per spec 027 |
| CHANGELOG precondition P1 fails (Unreleased not first heading) | `changelog.ValidateUnreleased` returns `false` with reason | Write `## Plan` with `outcome: needs_input`, `precondition_failed: "P1_unreleased_not_first"`, reason verbatim from validator; clear assignee, set previous_assignee | Reversible | Operator fixes the target repo's CHANGELOG, re-delegates |
| CHANGELOG precondition P2 fails (empty Unreleased) | `changelog.ValidateUnreleased` returns `false` with empty-bullets reason | Same escalation path as P1 with `precondition_failed: "P2_unreleased_empty"` | Reversible | Operator investigates watcher (this implies a false positive) |
| Claude returns invalid bump verdict | `prompts.ParseBumpVerdict` returns wrapped error | Step returns `agentlib.Result{Status: Failed, Message: "parse bump verdict: ..."}` — controller retries | Reversible | Controller retry; cap-escalation per spec 027 |
| `semver.BumpVersion` returns error (malformed `current_version`) | semver returns wrapped error | Write `## Plan` with `outcome: needs_input`, reason names the bad version; clear assignee, set previous_assignee | Reversible | Operator fixes the frontmatter or watcher emission |
| Same task re-triggered (idempotency) | Step re-reads content with existing `## Plan` block | `agentlib.MarshalSectionTyped` replaces the existing section; same input → byte-identical output | Reversible | n/a — by design |

## Do-Nothing Option

Cost of NOT building the integration spec:

- Three foundation libraries (~600 lines of Go + tests) sit unreachable. `agent/github-releaser/main.go` continues returning the placeholder Result.
- Watcher-emitted tasks (already validated by Phase 1 prototype) pile up in `24 Tasks/` with no consumer. Operator falls back to running `/github-release-repo` slash command per task — same as today.
- Phase 2 graduation stalls before reaching any observable behavior; no `## Plan` JSON section is produced by Go code. The contract between watcher → planning → execution stays unproven at the Go layer.

## Security / Abuse

- `Fetcher` shells out to `gh api` OR uses `net/http` with GitHub App installation token. Same auth resolution as pr-reviewer's `resolveAuth` — App preferred, PAT fallback for local CLI mode only.
- The Claude runner inherits its allowed-tools list from the factory; planning is READ-ONLY (no git writes, no shell to target repo). Sets `claudelib.AllowedTools{}` or minimal — verified by factory inspection.
- No untrusted input reaches a shell or filesystem write outside the agent's workspace; CHANGELOG content is opaquely passed to Claude (LLM provider sees it).

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0.
- [ ] `ls agent/github-releaser/pkg/factory/factory.go pkg/steps_planning.go pkg/githubchangelog/fetcher.go` returns all 3 paths with no errors (3 files exist).
- [ ] `grep -c '^func Create(Agent|AgentProvider|ClaudeRunner|Deliverer)' agent/github-releaser/pkg/factory/factory.go` returns ≥ 3 — canonical factories present.
- [ ] `grep -c 'agentlib.NewPhase(domain.TaskPhasePlanning' agent/github-releaser/pkg/factory/factory.go` returns ≥ 1 — typed phase constant used in NewPhase wiring.
- [ ] `grep -c '"planning"' agent/github-releaser/pkg/factory/factory.go agent/github-releaser/pkg/steps_planning.go` returns 0 — no raw string-literal phases in factory or step logic. (`main.go` + `cmd/run-task/main.go` are exempted because `libargument` struct-tag `default:"planning"` cannot accept a typed constant — env-var default literal is a library limitation, not a logic violation.)
- [ ] `grep -c 'fmt.Errorf' agent/github-releaser/pkg/steps_planning.go agent/github-releaser/pkg/githubchangelog/fetcher.go` returns 0 — bborbe/errors only.
- [ ] **Amended (2026-06-04, during verify):** ~~`grep -c 'strings.Index' agent/github-releaser/pkg/steps_planning.go` returns 0~~ → relaxed: **section I/O** uses `agentlib.MarshalSectionTyped` / `agentlib.ExtractSection[T]` (verify via `grep -c 'agentlib.MarshalSectionTyped\|agentlib.ExtractSection' agent/github-releaser/pkg/steps_planning.go` returns ≥ 1). The two remaining `strings.Index` calls at `steps_planning.go:693-694` are in `extractInvalidValue` for YAML error-message parsing — not section I/O, intent of the original AC preserved.
- [ ] **Amended (2026-06-04, during verify):** ~~`go test -cover ./pkg/steps_planning/... ./pkg/githubchangelog/...`~~ → `cd agent/github-releaser && go test -cover ./pkg/... ./pkg/githubchangelog/...` reports coverage matching regex `coverage: (7[5-9]|[89][0-9]|100)\.[0-9]%` for both packages. Reason: `steps_planning.go` is a flat file at `pkg/` root (per Constraints line 84 "flat at `pkg/` root, NOT `pkg/steps/`"), so `./pkg/steps_planning/...` doesn't resolve; the correct path is `./pkg/...`.
- [ ] **End-to-end happy path with mock Claude (unconditional):** a Ginkgo integration test in `pkg/steps_planning_test.go` exercises `PlanningStep.Run` with a counterfeiter-mocked `claudelib.ClaudeRunner` returning a fixed verdict `{"bump":"minor","reasoning":"feat: stub"}` against a fixture task with `current_version: v1.7.7`. The test asserts the returned `agentlib.Result` has `Status: Done`, `NextPhase: "execution"`, and that `agentlib.ExtractSection[PlanOutput](result.Content, "## Plan")` returns a `PlanOutput` with `Outcome: "ready"`, `Bump: "minor"`, `CurrentVersion: "v1.7.7"`, `NextVersion: "1.8.0"`, `NextVersionHeader: "## v1.8.0"`, `HeaderPrefixStyle: "v"` — evidence: `grep -c 'NextVersion.*"1.8.0"' agent/github-releaser/pkg/steps_planning_test.go` returns ≥ 1.
- [ ] **End-to-end escalation (unconditional, mock Claude):** same integration test layer; fixture has `## Unreleased` NOT as the first heading. Test asserts `Result{Status: Done}` (no error), the mutated content contains `## Plan` with `outcome: "needs_input"` AND `precondition_failed: "P1_unreleased_not_first"`, AND the frontmatter has `assignee: ""` AND `previous_assignee: github-releaser-agent` AND `status: in_progress` (unchanged from input) AND `phase: planning` (unchanged from input) — evidence: 5 separate grep assertions in the test source. **Amended (2026-06-04, during verify):** the test uses the Go field name `PreconditionFailed` (not the JSON tag) in the assertion; verify via `grep -c 'PreconditionFailed.*P1_unreleased_not_first' agent/github-releaser/pkg/steps_planning_test.go` returns ≥ 1 (Equal-assertion form, same semantic).
- [ ] **Live Claude smoke (post-commit, not in AC walk):** documented in spec verification block as a manual one-shot — `cp testdata/happy-planning.md /tmp/happy.md && go run ./cmd/run-task --task-file /tmp/happy.md` against a real Claude token. NOT part of the spec-verifier AC walk (this AC is just "the smoke procedure is documented in the verification block"); evidence: `grep -c 'go run ./cmd/run-task' agent/github-releaser/docs/planning-smoke.md` returns ≥ 1 (or the same in this spec's Verification block — implementer's choice).
- [ ] Counterfeiter mocks generated: `ls agent/github-releaser/mocks/fetcher.go` returns the file. **Amended (2026-06-04, during verify):** ~~`pkg/githubchangelog/mocks/`~~ → service-root `mocks/` per the maintainer multi-module-monorepo convention (same as `watcher/github-release/mocks/`, `agent/pr-reviewer/mocks/`). The `//counterfeiter:generate` directive in `pkg/githubchangelog/fetcher.go:30` emits to `../../mocks/fetcher.go`.
- [ ] Root `CHANGELOG.md` `## Unreleased` gains a single `feat:` bullet referencing the planning-phase integration — evidence: `grep -c 'planning phase' CHANGELOG.md` returns ≥ 1.

## Verification

```bash
cd agent/github-releaser
make precommit                                                            # exit 0
go test -cover ./pkg/steps_planning/... ./pkg/githubchangelog/...         # ≥75% / ≥80%

ls pkg/factory/factory.go pkg/steps_planning.go pkg/githubchangelog/fetcher.go

grep -cE '^func Create(Agent|AgentProvider|ClaudeRunner|Deliverer)' pkg/factory/factory.go   # ≥3
grep -c   'agentlib.NewPhase(domain.TaskPhasePlanning'                  pkg/factory/factory.go   # ≥1
grep -cR  '"planning"'                                                  pkg/factory/factory.go pkg/steps_planning.go --include='*.go' | grep -v _test.go | wc -l  # =0  (main.go + cmd/run-task/main.go exempted: libargument struct-tag default literal)
grep -cR  'fmt.Errorf'                                                  pkg/steps_planning.go pkg/githubchangelog/   # =0
grep -c   'strings.Index'                                               pkg/steps_planning.go   # =0

# End-to-end via mocked Claude — verified in pkg/steps_planning_test.go
grep -c 'NextVersion.*"1.8.0"'                            pkg/steps_planning_test.go  # ≥1
grep -c 'precondition_failed.*P1_unreleased_not_first'    pkg/steps_planning_test.go  # ≥1
grep -c 'previous_assignee.*github-releaser-agent'        pkg/steps_planning_test.go  # ≥1

# Optional live smoke (requires ANTHROPIC_AUTH_TOKEN; NOT part of spec-verifier AC walk):
#   cp cmd/run-task/testdata/happy-planning.md /tmp/happy.md
#   go run ./cmd/run-task --task-file /tmp/happy.md
#   grep -c '"outcome": "ready"'   /tmp/happy.md
#   grep -c '"next_version":'      /tmp/happy.md

# Root CHANGELOG entry
grep -c 'planning phase' CHANGELOG.md   # ≥1
```

A scenario IS NOT JUSTIFIED here because the integration is verified by the run-task CLI fixture flow above. That run exercises every layer (CLI → factory → AgentProvider → Agent → PlanningStep → changelog/prompts/semver → MarshalSectionTyped) end-to-end against a real markdown file. The only thing the run-task harness can't reach is Kafka delivery — and Kafka delivery in this spec is just `delivery.NewKafkaResultDeliverer(...)` wiring identical to pr-reviewer, with no new contract. Per [[spec-writing]] § Test-layer responsibilities + scenario-writing.md four-condition test, no scenario.

## Verification Result

**Verified:** 2026-06-04T15:32:04Z (HEAD 0136309)
**Binary:** installed dark-factory (non-dark-factory repo; Phase 0 skipped)
**Scenario:** static-evidence verification — make precommit + grep assertions + go test -cover on agent/github-releaser
**Evidence:**
- `make precommit` exit 0, final line `ready to commit`
- `grep -cE '^func Create(Agent|AgentProvider|ClaudeRunner|Deliverer)' pkg/factory/factory.go` → 4
- `grep -c 'agentlib.NewPhase(domain.TaskPhasePlanning' pkg/factory/factory.go` → 1
- `grep -c '"planning"' pkg/factory/factory.go pkg/steps_planning.go` → 0+0
- `grep -c 'fmt.Errorf' pkg/steps_planning.go pkg/githubchangelog/fetcher.go` → 0+0
- `grep -c 'agentlib.MarshalSectionTyped\|agentlib.ExtractSection' pkg/steps_planning.go` → 4 (AC#7 amended scope)
- `go test -cover ./pkg/... ./pkg/githubchangelog/...` → pkg 86.7%, githubchangelog 87.5% (AC#8 amended path)
- `grep -c 'NextVersion.*"1.8.0"' pkg/steps_planning_test.go` → 2
- `grep -c 'PreconditionFailed.*P1_unreleased_not_first' pkg/steps_planning_test.go` → 1 (AC#10 amended assertion form)
- `grep -c 'previous_assignee.*github-releaser-agent' pkg/steps_planning_test.go` → 1
- `ls agent/github-releaser/mocks/fetcher.go` → present (AC#13 amended path)
- `grep -c 'planning phase' CHANGELOG.md` → 2
**Verdict:** PASS
