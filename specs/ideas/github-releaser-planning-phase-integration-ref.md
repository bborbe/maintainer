---
status: approved
approved: "2026-05-27T20:11:08Z"
branch: dark-factory/github-releaser-planning-phase
---

## Summary

- Phase-1 prototype validated via `/github-release-repo` slash command + `docker-utils v1.7.8` release. Phase 2 graduates it to a Go Pattern B Job; this spec covers the **planning phase only**.
- The planning step turns a watcher-emitted `task_type: github-release` task into a structured `## Plan` JSON section that operators (or downstream execution phase) can act on without doing any git mutation.
- Mirrors the planning behavior of the Phase-1 slash command verbatim per [[GitHub Release Agent Phase 1 Learnings]]: parse `## Unreleased`, classify bump via Claude, compute next semver, infer header prefix, write `## Plan`.
- Refuses to advance the task if the CHANGELOG fails precondition checks (P1: `## Unreleased` not first heading; P2: empty Unreleased). Escalates per spec 027 (`previous_assignee` set, `assignee` cleared, `status` + `phase` untouched).
- Spec scope is intentionally tight: planning only. Execution (commit + push), PR fallback, and ai_review ship in follow-up specs.

## Problem

Today, releasing a `bborbe` fleet repo with an outstanding `## Unreleased` section is operator-driven: someone runs `/github-release-repos` or the new `/github-release-repo` slash command and watches it run. That is fine for one repo at a time and broken for the BRO-20203 ~30-repo batch driver behind [[GitHub Release Agent]]. Without an autonomous planning phase, every release also requires a human to read four bullets and decide patch/minor/major — work that Phase 1 proved Claude does correctly on real inputs.

The planning phase is the safety boundary: it produces a reviewable `## Plan` before anything mutates a target repo. If preconditions fail (malformed CHANGELOG), the agent escalates without taking action. This spec ships the boundary first; execution and verification come next.

## Goal

When the task-executor spawns `agent/github-releaser` with `PHASE=planning` and a valid `github-release` task in `TASK_CONTENT`:

- The agent fetches `CHANGELOG.md` from the target repo's `ref`, validates two preconditions, classifies the bump via Claude, computes the next version, and writes a `## Plan` section to the task body containing structured JSON the next phase can consume.
- On precondition failure the agent writes a `## Plan` section with `outcome: needs_input`, clears `assignee`, sets `previous_assignee: github-releaser-agent`, and leaves `status` + `phase` untouched so operator re-delegation resumes from `planning`.
- On success the agent advances `phase` to `execution` via the `next_phase` field on the `agentlib.Result` returned to the controller.

The end state is observable: a task that entered `phase: planning` exits either with a usable `## Plan` JSON ready for `execution`, or sits parked in the operator inbox with an actionable `needs_input` reason.

## Non-goals

- Execution phase (header rewrite, commit, push, tag) — separate spec.
- PR + auto-merge fallback path — separate spec; planning never writes git.
- ai_review phase (remote verification) — separate spec.
- Cloning the target repo. Planning reads `CHANGELOG.md` via the GitHub REST API only; cloning is execution's concern.
- Watcher-side work — the watcher slash command stays in place; this spec consumes its existing task output unchanged.
- Mono-repo support (multiple CHANGELOGs in one repo) — explicitly out per parent goal.
- Per-phase model overrides via Config CRD. Planning uses whatever `ANTHROPIC_MODEL` resolves to in env (default `sonnet`).

## Desired Behavior

1. With `PHASE=planning` and a task carrying `task_type: github-release`, `assignee: github-releaser-agent`, `status: in_progress`, `phase: planning` plus required domain fields (`repo`, `clone_url`, `ref`, `current_version`, `task_identifier`), the agent runs the planning workflow and produces a `## Plan` section.
2. The agent fetches CHANGELOG.md via `GET /repos/{owner}/{repo}/contents/CHANGELOG.md?ref={ref}` using the configured GitHub auth (App token preferred, PAT fallback — same resolution order as pr-reviewer's `main.go` resolveAuth).
3. Precondition P1: the first `## ` heading in CHANGELOG.md (skipping `# Changelog` H1 and preamble) MUST be `## Unreleased`. Any other first heading triggers escalation with a `## Plan` outcome of `needs_input` whose reason names the offending heading and its line number.
4. Precondition P2: the bullets under `## Unreleased` (lines between the heading and the next `## ` heading or EOF, filtered to lines starting with `- `) MUST be ≥ 1. Empty Unreleased triggers escalation with reason "watcher emitted task for empty Unreleased — likely watcher bug, investigate."
5. Claude is asked to classify the bullet list as `patch | minor | major` using a prompt embedded in `pkg/prompts/`. The prompt is the verbatim text from Phase 1's `/github-release-repo` slash command: rules in order major → minor → patch, output is JSON `{bump, reasoning}`.
6. The agent computes `next_version` as `current_version + bump` with edge case `v0.0.0 → v0.1.0` regardless of bump (first release default).
7. The agent infers `header_prefix_style` by scanning existing `## v?X.Y.Z` headings in CHANGELOG.md; the first historic release heading sets the style (`"v"` or `""`). If no historic release exists, defaults to `"v"` (canonical Keep-a-Changelog style).
8. The agent writes a `## Plan` section via `agentlib.MarshalSectionTyped` with a typed `PlanOutput` struct: `{outcome: "ready", bump, reasoning, current_version, next_version, next_version_header, header_prefix_style, bullets[]}`.
9. On success the agent returns `agentlib.Result{Status: Done, NextPhase: "execution"}` so the controller advances `phase` to `execution` on commit.
10. On precondition failure the agent writes a `## Plan` section with `{outcome: "needs_input", reason, precondition_failed, details}` and returns `agentlib.Result{Status: Done, NextPhase: ""}`. The controller commits the section; a separate writer mutation clears `assignee` and sets `previous_assignee` per spec 027 (same path the existing `task/controller` `PublishFailure` writer uses on `needs_input`).

## Constraints

- Pattern B Job contract: read `TASK_CONTENT` and `PHASE` envs (canonical phase values per [[Agent Phase Dispatch Guide]]: `planning` not `in_progress`); never read or write the vault directly.
- `pkg/factory/factory.go` is a single file, all `Create*` functions, no business logic, no `error` return — per [[Go Agent Implementation Guide]] § factory.go conventions.
- Planning step uses `agentlib.NewPhase(domain.TaskPhasePlanning, planningStep)` with the typed phase constant. String literals are banned (CRD `trigger.phases` is literal-match; mismatch silently drops tasks per [[Agent Phase Dispatch Guide]] § Operational Gotcha).
- The Claude bump-classification prompt is embedded via `//go:embed` and ships verbatim from the Phase 1 slash command. Any prompt change requires a separate spec.
- The `PlanOutput` struct lives in `pkg/changelog/` (or equivalent shared location) and is the contract the execution-phase spec consumes verbatim. Field names + JSON tags are part of this spec's contract.
- The CHANGELOG-fetch path goes through a `GitHubChangelogFetcher` interface (counterfeiter-mockable) so tests don't hit the real GitHub API.
- The validation logic (P1 + P2) and the header-style inference live in `pkg/changelog/` and are unit-tested with `DescribeTable` covering: `## Unreleased` first, `## Unreleased` not first (rejected), no `## Unreleased` (rejected), empty bullets (rejected), historic v-prefix style, historic no-prefix style, no historic releases.
- Escalation MUST set `previous_assignee: github-releaser-agent` and clear `assignee: ""`. It MUST NOT mutate `status` or `phase`. This is the production rule per [[Agent Task File Contract]] § Escalation rule; failure to honor it breaks the operator-inbox filter.
- The agent NEVER clones the target repo in the planning phase. Cloning is execution's concern. Planning's only outbound dependency is the GitHub REST contents API.

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Recovery |
|---|---|---|---|---|
| CHANGELOG.md missing (404) | `GET /contents/CHANGELOG.md?ref={ref}` returns 404 | Write `## Plan` with `outcome: needs_input`, reason "CHANGELOG.md not found at ref X"; clear assignee, set previous_assignee | Reversible | Operator creates CHANGELOG.md on master and re-delegates by setting `assignee: github-releaser-agent` |
| Precondition P1 fails (Unreleased not first heading) | First `## ` heading in CHANGELOG ≠ `## Unreleased` | Write `## Plan` with `outcome: needs_input`, reason names the offending heading + line; clear assignee + set previous_assignee | Reversible | Operator reorders CHANGELOG (moves `## Unreleased` above the first historic release), commits to master, re-delegates by setting `assignee: github-releaser-agent` |
| Precondition P2 fails (empty Unreleased) | Zero `- ` bullets between `## Unreleased` and next `## ` | Write `## Plan` with `outcome: needs_input`, reason "watcher emitted task for empty Unreleased"; clear assignee + set previous_assignee | Reversible | Operator investigates watcher (false positive); deletes empty `## Unreleased`; watcher won't re-emit (dedup); task lives in operator inbox until manually marked aborted |
| GitHub API rate-limited (429 or 403 with rate-limit headers) | HTTP 429 or `x-ratelimit-remaining: 0` | Return `agentlib.Result{Status: Failed}` — controller retries phase per existing retry-cap logic | Reversible | Wait for rate-limit reset; controller re-triggers same phase |
| Claude classification returns malformed JSON | `json.Unmarshal` fails on stdout | Return `agentlib.Result{Status: Failed}` with parsed error; controller retries phase | Reversible | Controller retry; at retry cap, escalates to `human_review` per spec 027 |
| Claude classification returns `bump` value outside `{patch, minor, major}` | Decoded struct field validation fails | Return `agentlib.Result{Status: Failed}` with reason; controller retries phase | Reversible | Same as malformed JSON path |
| `current_version` frontmatter is unparseable (not `v0.0.0` or `vX.Y.Z`) | semver parse error | Write `## Plan` with `outcome: needs_input`, reason names the bad value; clear assignee + set previous_assignee | Reversible | Operator inspects task; usually this is a watcher emission bug — fix watcher, re-emit |
| Concurrent re-trigger (controller redelivery of same Kafka message) | Same `task_identifier` seen twice | Idempotent: the planning step's writes are full-section replacements via `MarshalSectionTyped`. Re-running produces the same output. | Reversible | No action — by design |

## Do-Nothing Option

Cost of NOT building the planning phase:

- BRO-20203 fleet release (~30 repos) stays manual: operator runs `/github-release-repo` per repo, ~5 min/repo × 30 = 2.5h of toil per fleet pass.
- Phase 2 graduation stalls: with no planning phase implemented in Go, the rest of the agent pipeline can't be wired up. The Phase 1 slash command keeps doing all the work, which is fine for low volume but contradicts the [[GitHub Release Agent]] goal of an autonomous pipeline.
- The validated Phase 1 contract (`## Plan` JSON shape, precondition rules, escalation pattern, header-prefix inference) loses momentum the longer it sits in a slash command without graduation. Phase 2 inheritance is verbatim today; that may not hold after weeks of slash-command iteration.

## Security / Abuse

- The agent receives `TASK_CONTENT` from the trusted task-executor; no user-supplied input bypasses the watcher's `repo: bborbe/<name>` allowlist match.
- The agent reads CHANGELOG.md via GitHub's REST contents API using a GitHub App installation token (mint at startup via `lib/githubapp.MintIAT`, same path as pr-reviewer). PAT (`GH_TOKEN` env) is the documented fallback for local CLI mode only.
- The agent NEVER writes to GitHub in the planning phase. If a future bug routes write traffic here, the agent's `allowed-tools` scope (planning is read-only) will reject the call at the Claude CLI boundary.
- The Claude prompt receives the CHANGELOG bullet list (which is fetched from a public-or-org-private GitHub repo per the allowlist). No secrets are passed to Claude. The LLM provider's data policy applies.
- The vault task file content (after section write) is published to Kafka via the task/controller's `agent-task-v1-event` topic; Kafka topology and access controls are project-level concerns out of scope for this spec.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0 — evidence: exit code 0.
- [ ] `cat agent/github-releaser/pkg/factory/factory.go | grep -E '^func Create(Agent|AgentProvider|ClaudeRunner|Deliverer)\b'` returns ≥4 lines, confirming the canonical Create-functions exist per [[Go Agent Implementation Guide]].
- [ ] `grep -rn 'agentlib.NewPhase(domain.TaskPhasePlanning' agent/github-releaser/pkg/` returns ≥1 line — typed phase constant, not string literal, per [[Agent Phase Dispatch Guide]].
- [ ] `grep -rn '"planning"' agent/github-releaser/pkg/ agent/github-releaser/main.go | grep -v '_test.go' | grep -v '// '` returns 0 lines — no raw string-literal phases in production code (test fixtures + comments allowed).
- [ ] `grep -rn 'github-releaser-agent' agent/github-releaser/pkg/ agent/github-releaser/main.go | grep -v '_test.go'` returns ≥1 line — the escalation path sets the canonical `previous_assignee` value.
- [ ] Running `go run ./agent/github-releaser/cmd/run-task -- /tmp/happy-planning.md` against a fixture task (copied from `cmd/run-task/testdata/happy-planning.md`) whose CHANGELOG bullets are `[feat: add foo, fix: bar]` and `current_version: v1.7.7` produces a modified `/tmp/happy-planning.md` whose `## Plan` JSON has EXACTLY: `"outcome": "ready"`, `"bump": "minor"`, `"current_version": "v1.7.7"`, `"next_version": "v1.8.0"`, `"next_version_header": "## v1.8.0"`, `"header_prefix_style": "v"` — verified by 6 separate `grep -c` checks each returning 1.
- [ ] Running the same `cmd/run-task` against a fixture (copied from `cmd/run-task/testdata/unreleased-at-bottom.md`) whose target repo's CHANGELOG has `## Unreleased` NOT as the first heading produces a modified `/tmp/unreleased-at-bottom.md` whose `## Plan` has `outcome=needs_input` AND whose frontmatter has `assignee: ""` AND `previous_assignee: github-releaser-agent` AND `status: in_progress` (unchanged from input) AND `phase: planning` (unchanged from input) — verified via 5 separate grep checks on the result file.
- [ ] Ginkgo unit-test coverage of `pkg/changelog/` ≥ 80%, of `pkg/semver/` ≥ 80% — evidence: `go test ./pkg/changelog/... ./pkg/semver/... -cover` reports `coverage: ≥80.0%` for each package.
- [ ] `pkg/changelog/` tests use `DescribeTable` / `Entry` for: Unreleased-first (valid), Unreleased-not-first (P1 fail with line number), no-Unreleased (P1 fail), empty-bullets (P2 fail), v-prefix-historic, no-prefix-historic, no-historic-release-defaults-to-v — verified via `grep -c 'Entry(' pkg/changelog/*_test.go` returns ≥7.
- [ ] Compilation smoke test passes: `cd agent/github-releaser && go test -run TestSuite -v ./...` reports `Compiles` test PASS within 60s.
- [ ] CHANGELOG.md `## Unreleased` section gains an entry referencing this spec (per [[changelog-guide]] conventional prefix `feat:`).

## Verification

```bash
cd ~/Documents/workspaces/maintainer-github-releaser/agent/github-releaser
make precommit                                                    # exit 0
go test -cover ./pkg/changelog/... ./pkg/semver/...               # ≥80% each

# Happy path — copy fixture to /tmp so reruns are deterministic (run-task mutates in place)
cp ./cmd/run-task/testdata/happy-planning.md /tmp/happy-planning.md
go run ./cmd/run-task -- /tmp/happy-planning.md
grep -c '"outcome": "ready"'           /tmp/happy-planning.md     # =1
grep -c '"bump": "minor"'              /tmp/happy-planning.md     # =1
grep -c '"current_version": "v1.7.7"'  /tmp/happy-planning.md     # =1
grep -c '"next_version": "v1.8.0"'     /tmp/happy-planning.md     # =1
grep -c '"next_version_header": "## v1.8.0"' /tmp/happy-planning.md # =1
grep -c '"header_prefix_style": "v"'   /tmp/happy-planning.md     # =1

# Escalation path — copy fixture to /tmp
cp ./cmd/run-task/testdata/unreleased-at-bottom.md /tmp/unreleased-at-bottom.md
go run ./cmd/run-task -- /tmp/unreleased-at-bottom.md
grep -c '"outcome": "needs_input"'                  /tmp/unreleased-at-bottom.md # =1
grep -c '^assignee: ""'                             /tmp/unreleased-at-bottom.md # =1
grep -c 'previous_assignee: github-releaser-agent'  /tmp/unreleased-at-bottom.md # =1
grep -c '^phase: planning'                          /tmp/unreleased-at-bottom.md # =1 (unchanged)
grep -c '^status: in_progress'                      /tmp/unreleased-at-bottom.md # =1 (unchanged)
```

Reference task fixtures live under `cmd/run-task/testdata/`. They are checked-in markdown files mirroring the Phase 1 watcher's emitted task contract — see [[Agent Task File Contract]] for the schema.

No scenario is justified at this rung: precondition logic + classification round-trip are fully unit-testable; integration is the run-task fixture flow above. Per [[spec-writing]] § Test-layer responsibilities, only behavior the unit + integration layers genuinely cannot reach earns a scenario. This spec doesn't qualify.
