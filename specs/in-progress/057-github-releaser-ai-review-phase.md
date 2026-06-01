---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-31T20:33:16Z"
generating: "2026-05-31T20:33:56Z"
prompted: "2026-05-31T20:41:42Z"
verifying: "2026-05-31T21:29:46Z"
branch: dark-factory/github-releaser-ai-review-phase
---

## Summary

- The `/github-release-repo` workflow defines three phases — `planning → execution → ai_review` — but the Go agent at `agent/github-releaser/` only implements the first two. Every released task is advanced by the controller to `phase: ai_review` and then parks there forever, because no agent handles that phase.
- A concrete example sits parked today: a release task for `bborbe-claude-yolo` at commit `af4000c` shipped `v0.9.0`, the tag is on the remote, and the CHANGELOG header is rewritten on master — but the task file is stuck in `ai_review` with nothing to advance it.
- After this work, a fresh release task drives all three phases automatically to completion without human intervention between watcher emission and the task moving to `completed`. Verification failures escalate to the operator inbox instead of looping or silently succeeding.
- Scope is the new `ai_review` step (remote-only verification via the GitHub REST API — no clone), its registration in the agent factory, and the matching change to the agent's Kubernetes `Config` so the controller dispatches the new phase. No changes to the planning step, the execution step, the watcher, the slash command, the result schema, or the App's existing scopes.

## Problem

The `github-releaser` agent's `execution` step returns `NextPhase=ai_review` and writes a `## Result` section with `outcome=released`, `commit_sha`, and `tag` whenever a release succeeds. The controller honours that signal, updates the task's frontmatter to `phase: ai_review`, and emits the next dispatch event — but the agent's Kubernetes `Config` (`trigger.phases`) does not list `ai_review` and the factory only registers two phases, so nothing picks the task up. The behavior is intentional in the current manifest (a comment explicitly notes "ai_review intentionally omitted until the ## Review step is implemented (a separate spec)") and that spec is this one.

The operational consequence is two-fold. First, every successful release leaves a task file permanently in `phase: ai_review` until an operator manually marks it complete, which pollutes the in-progress queue and makes "what's actually in flight" hard to read. Second, the verification work that `ai_review` exists to perform — confirming the tag is actually present on the remote, the tag points at the commit the execution step intended, and the CHANGELOG header was actually rewritten on the default branch — is not happening at all. The execution step pushes and trusts; nothing reads back.

## Goal

After this work, a fresh `github-release` task drives all three phases to terminal state without manual intervention: `planning → execution → ai_review → completed` for a successful release, `planning → execution → ai_review → operator inbox` if the verification finds the release is not actually visible on the remote in the expected shape. The verification reads the remote via the GitHub REST API only (no clone, no local git), and the resulting verdict is recorded in the task file in the same typed-JSON style as the existing `## Plan` and `## Result` sections.

## Non-goals

- Do NOT change the `planning` or `execution` step behaviour, their inputs/outputs, or the `## Plan` / `## Result` section shapes. The new step consumes them; it does not modify them.
- Do NOT change the `/github-release-repo` slash command. Its three-phase contract is already correct; this spec brings the Go agent up to match.
- Do NOT clone the target repo to verify. The three checks are all reachable via the GitHub REST API and a clone would re-introduce the same private-repo / token-exposure surface that the existing execution step already manages — out of scope here.
- Do NOT broaden the release App's installation scope or grant new permissions. The existing Contents-read access is sufficient for every check this step performs.
- Do NOT introduce a retry loop inside the step for transient GitHub API failures. The standard context-aware error return path plus the controller's Kafka redelivery is the retry mechanism; the step itself is single-shot.
- Do NOT backfill or auto-clear release tasks already parked in `ai_review` from before this ships. The next time the parked task is re-triggered by any normal path (or operator action) the new agent will handle it; one-off backfill is out of scope.
- Do NOT add an opt-out flag, a per-task "skip ai_review" toggle, or a configurable verification ruleset — invariant; if a future consumer demands variation, that's a separate spec.
- Do NOT add a configurable knob for the GitHub API client implementation. The agent already wires an HTTP client for the planning step's CHANGELOG fetch; reuse the same auth model and HTTP shape rather than introducing a new abstraction.

## Desired Behavior

1. When a release task in `phase: ai_review` is dispatched to the agent and its `## Result` section reports `outcome=released`, the agent performs three remote checks against the GitHub REST API: (a) the tag named in `## Result` exists on the target repo, (b) the tag resolves (following annotated-tag indirection if present) to the commit SHA named in `## Result`, (c) the `CHANGELOG.md` on the default branch contains a release header for that version (no `## Unreleased` placeholder remaining as the top section).
2. The agent writes a `## Review` section to the task file as a fenced JSON block with a stable shape: a boolean `approved` field, a `checks` object whose keys are the three check names and whose values are booleans, and a one-sentence `notes` field. The shape is the same on success and on failure — failure differs only in which booleans are `false` and the wording of `notes`.
3. When all three checks pass, the agent returns a result that drives the task to terminal-completed: phase advances out of `ai_review` and the task status becomes `completed`. No further dispatch is emitted for the task.
4. When any check fails, the agent's step returns `Status: failed` (no `next_phase` transition emitted). The controller's standard agent-failure escalation path applies — per the platform doctrine ([[Controller Stop Setting human_review on Agent Failure]], shipped 2026-05-25 / spec 039): the controller writes `assignee: ""`, sets `previous_assignee: github-releaser-agent`, appends a `## Failure` section with timestamp + reason, and leaves `phase` and `status` untouched (`phase` stays `ai_review`, `status` stays `in_progress`). The step also writes the `## Review` section with `approved: false` + the per-check booleans BEFORE returning `Status: failed`, so the operator inbox sees both the verdict (which checks failed and why) and the controller's `## Failure` envelope. The task does NOT transition to `phase: human_review` — that phase is reserved for "pipeline completed, human verifies result," not for "agent step failed."
5. When the task's `## Result` reports `outcome != "released"` (i.e. the execution step recorded a failure), the agent performs zero GitHub API calls and returns a result that drives the task to terminal-completed. The failure was already recorded upstream; there is nothing for `ai_review` to verify and nothing to escalate that the execution step did not already record.
6. When the `## Plan` or `## Result` section is missing or its JSON is malformed, the agent returns an error from the step (not a `## Review` verdict) so the controller's standard failure path runs. A malformed contract is not the same as a verification failure.
7. The agent's Kubernetes `Config` lists all three phases under `trigger.phases` so the controller dispatches `ai_review` events to this agent. The previously-present omission comment is gone.
8. The local CLI entry point `cmd/run-task` (the only `cmd/*` binary in `agent/github-releaser/`) invoked with `--phase ai_review` drives the new step the same way the Kafka entry point does. A single binary, one set of dependencies, two entry points.

## Constraints

- The `## Plan` and `## Result` section shapes (`PlanOutput`, `ResultOutput` in `agent/github-releaser/pkg/`) are frozen. The new step reads them; it does not modify them and does not add new fields.
- The phase identifier registered in the factory MUST be the typed constant `domain.TaskPhaseAIReview` from `github.com/bborbe/vault-cli/pkg/domain`, never a raw string literal. The Kubernetes `trigger.phases` value is matched literal-against the agent's registered phase set; a string-vs-constant mismatch silently drops tasks.
- The factory function (`CreateAgent` in `agent/github-releaser/pkg/factory/factory.go`) stays pure: no error return, no `switch` on configuration, no conditional registration of phases. Adding the third phase is a single additional `agentlib.NewPhase(...)` argument and one additional constructor call.
- The existing release App's permissions (Contents: read, plus the write needed by the execution step) are the upper bound — the new step uses Contents: read only and adds no scope requirement.
- The CHANGELOG fetch MUST NOT hardcode `main` as the default branch. The target repo's actual default branch is resolved at runtime (the `repos/{repo}/contents/CHANGELOG.md` GitHub API endpoint already defaults to the repo's default branch when no `?ref=` is passed — relying on that behavior is correct and idiomatic). Tag-resolution (`git/refs/tags/{tag}`, `git/tags/{sha}`) is branch-independent.
- The step MUST NOT return a result that mutates `assignee`, `previous_assignee`, `status`, or writes a `## Failure` section directly. Those mutations are the **controller's** responsibility on `Status: failed` paths per the platform doctrine ([[Controller Stop Setting human_review on Agent Failure]] / spec 039). The step's only outputs are: (a) the `## Review` section, (b) the returned `agentlib.Step` result with `Status: done | failed` and (on success) the terminal-completed phase signal.
- The step MUST NOT return `next_phase: human_review` on verification failure. Per the same doctrine, `human_review` is reserved for "pipeline completed, human verifies result" (an explicit verdict-from-success path), not for "step failed." `Status: failed` is the correct signal; the controller does the rest.
- Token handling uses the same env-derived bearer-token pattern the planning step's `githubchangelog.NewHTTPFetcher` already uses. No new secret name, no new env var, no new Kubernetes manifest field for credentials.
- `github.com/bborbe/errors` everywhere — `errors.Wrapf(ctx, err, ...)` / `errors.Errorf(ctx, ...)`. No bare `return err`, no `fmt.Errorf`.
- `glog.V(2).Infof` for phase transitions and outbound HTTP; `glog.Warningf` for non-fatal anomalies; never log a bearer token at any verbosity (existing 8-character-prefix convention applies if any token surfaces in a log line).
- Tests live in an external `pkg_test` package using Ginkgo/Gomega, with `BeforeEach`/`JustBeforeEach`/`It` and AAA structure; mocks are Counterfeiter-generated via `//counterfeiter:generate` directives, with generated files under `agent/github-releaser/mocks/`.
- Existing tests under `agent/github-releaser/pkg/...` and `agent/github-releaser/pkg/factory/...` must continue to pass unchanged. The factory test that asserts the registered phase count is the canonical place to assert "exactly three phases are registered".
- The Definition of Done that governs implementation completeness is `docs/dod.md` in this repository. The spec defers to it; it does not duplicate criteria.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility |
|---------|-----------|-------------------|----------|---------------|
| All three remote checks pass | Step receives 200 from each API call; values match | `## Review` with `approved: true`; task transitions to `completed`; no further dispatch | n/a | n/a |
| Tag does not exist on remote (404 from `git/refs/tags/{tag}`) | Step sees 404 on first check | Step writes `## Review` with `approved: false` and `tag_exists: false`, then returns `Status: failed`; controller unassigns (clears `assignee`, sets `previous_assignee: github-releaser-agent`, appends `## Failure` section); `phase` stays `ai_review`, `status` stays `in_progress` (NOT `human_review`) | Operator inspects and either re-runs the release or marks the task aborted | Reversible |
| Annotated tag points at a commit other than the SHA in `## Result` | Step follows tag → wrapped commit and compares; mismatch | Step writes `## Review` with `approved: false` and `tag_at_expected_sha: false`, returns `Status: failed`; controller applies the same unassign + `## Failure` envelope as above | Operator investigates (most likely a race with manual tag movement) | Reversible |
| Lightweight tag points at a commit other than the SHA in `## Result` | Step sees `object.type == "commit"` and compares directly; mismatch | Same as annotated mismatch | Same | Reversible |
| `CHANGELOG.md` on default branch still has `## Unreleased` as top section / lacks the new version header | Step fetches contents, base64-decodes, scans top of file | Step writes `## Review` with `approved: false` and `changelog_header_rewritten: false`, returns `Status: failed`; controller applies the unassign + `## Failure` envelope | Operator investigates (most likely a merge/race) | Reversible |
| `## Result` reports `outcome != "released"` (e.g. `failed`) | Step parses `## Result` JSON before any API call | Zero API calls; `## Review` with `approved: true` and `notes` indicating "execution did not release; nothing to verify"; step returns `Status: done` + terminal-completed signal so task transitions to `completed`. (The execution step's failure was already escalated upstream via the same controller doctrine — no second escalation here.) | n/a | n/a |
| `## Plan` or `## Result` section missing | Section extraction returns nil | Step returns wrapped error; controller's standard failure path runs; task stays in `ai_review` for retry/operator | Operator inspects task file shape | Reversible |
| `## Plan` or `## Result` JSON malformed | Section extraction returns decode error | Step returns wrapped error (same path as missing) | Same | Reversible |
| GitHub API returns 5xx or times out on any check | HTTP client returns non-2xx / context-deadline error | Step returns wrapped error; controller retries via Kafka redelivery (existing path); no `## Review` written this attempt | Operator waits for transient outage to clear, or escalates if persistent | Reversible |
| GitHub API rate-limits the agent (403 with rate-limit headers) | Step sees 403 with `X-RateLimit-Remaining: 0` | Step returns wrapped error indicating rate limit; controller redelivery handles next-window retry. No `## Review` this attempt. | Operator waits for rate window; or reduces release frequency | Reversible |
| Two `ai_review` dispatches arrive concurrently for the same task | Controller's per-task lock serializes them | Only one runs at a time; the second sees the `## Review` section already present and produces the same verdict (idempotent overwrite) | n/a | Concurrency-safe via controller lock |
| Default branch name is not `main` (e.g. `master`) | CHANGELOG fetch uses repo's actual default branch | Check still passes — fetch resolves the default branch via the API (do not hardcode `main`) | n/a | n/a |
| Mid-step crash after writing `## Review` but before returning to controller | Pod restarts; next dispatch re-runs the step | Step re-runs the same three checks (idempotent); writes the same `## Review` (overwrites); returns same verdict | n/a | Reversible |

## Security / Abuse Cases

- **Token in logs:** the bearer token used for the three API calls must not appear in any `glog` output at any verbosity, nor in any returned error message. The existing 8-character prefix convention from `lib/githubapp` is the upper bound for any deliberate surfacing.
- **Attacker-controlled `repo` / `tag` / `commit_sha`:** these values come from the task file, which is populated by upstream phases — but a malicious or corrupted upstream payload could craft a `repo` like `owner/../../etc` or a tag with shell-meta characters. The HTTP layer uses `url.PathEscape` / `url.QueryEscape` for every path segment and query value (same pattern as `githubchangelog`). No values flow into a shell command.
- **Cross-repo access via the App installation:** an `ai_review` call against a repo the App installation does not cover returns 404, which the step interprets as "tag does not exist" → escalation. That is the correct outcome — the operator inbox is the right place for "this release did not actually land in a place I can see."
- **No unbounded retries / no hangs:** the HTTP client uses a bounded timeout (same as `githubchangelog`); the step performs at most three sequential API calls and returns. No `for` loop on transient failure; the controller's redelivery is the only retry surface.
- **Idempotent overwrites:** re-running `ai_review` on a task that already has a `## Review` section overwrites it with the result of the fresh checks. There is no append, no version history, no per-attempt suffix.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0 — evidence: exit code.
- [ ] `agent/github-releaser/pkg/factory/factory.go` `CreateAgent` registers exactly three phases (`planning`, `execution`, `ai_review`). NOTE: `agent/lib` v0.63.11 does NOT expose `Agent.Phases()` — `findPhase` is unexported (`agent_agent.go:126`). Direct phase-name assertion in a Go test is therefore not possible without modifying agent-lib (out of scope). Evidence: (1) **primary** — `grep -nE 'agentlib\.NewPhase\(' agent/github-releaser/pkg/factory/factory.go | wc -l` returns `3` AND `grep -E 'domain\.TaskPhaseAIReview' agent/github-releaser/pkg/factory/factory.go` returns at least one match (proves the typed constant is in source); (2) **corroborating** — the existing factory test (`CreateAgent` returns non-nil `*agentlib.Agent` and the three-phase construction does not panic) continues to pass, since `NewAgent` and `NewPhase` would panic on any miswiring.
- [ ] A unit test exercises the happy path (all three checks pass) and asserts the returned result drives the task to terminal-completed — evidence: `result.Status == agentlib.AgentStatusDone` AND `result.NextPhase == "done"` (per `agent/lib v0.63.x` agent_agent.go:91 — the literal `"done"` string is the terminal-completed signal).
- [ ] The agent's Kubernetes `Config` lists all three phases under `trigger.phases` with no omission comment — evidence: `grep -A4 'phases:' agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml` shows `planning`, `execution`, `ai_review` and `grep -n 'intentionally omitted' agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml` returns no match.
- [ ] A unit test verifies the tag-missing case (mock returns 404 on `git/refs/tags/{tag}`) produces `approved: false`, `checks.tag_exists: false` in the written `## Review` section AND the step returns `Status: failed` (no `next_phase` transition) — evidence: test asserts the `## Review` JSON contents in the returned markdown AND asserts `result.Status == agentlib.AgentStatusFailed` AND asserts the result does NOT carry a `next_phase` of `human_review` (or any other phase). The step explicitly does NOT mutate `assignee` / `previous_assignee` / `status` / write a `## Failure` section — those are the controller's responsibility on the `failed` path per spec 039.
- [ ] A unit test verifies the commit-SHA-mismatch case for an annotated tag (`object.type == "tag"` → follow-up `git/tags/{sha}` returns a different commit SHA than `## Result.commit_sha`) produces `approved: false`, `checks.tag_at_expected_sha: false`, and `Status: failed` — evidence: test assertions on `## Review` JSON contents AND on the returned step result's `Status` field.
- [ ] A unit test verifies the commit-SHA-mismatch case for a lightweight tag (`object.type == "commit"`, SHA mismatch) produces the same `approved: false` / `tag_at_expected_sha: false` / `Status: failed` — evidence: test assertion on `## Review` JSON contents AND on `result.Status`.
- [ ] A unit test verifies the changelog-header-missing case (mocked CHANGELOG fetch returns content whose top section is still `## Unreleased`) produces `approved: false`, `checks.changelog_header_rewritten: false`, and `Status: failed` — evidence: test assertion on `## Review` JSON contents AND on `result.Status`.
- [ ] A unit test verifies the step does NOT add a `## Failure` section to the markdown on any verification-failure path (controller's job, not the step's) — evidence: parse the returned markdown after each failure case and assert no `## Failure` heading is present.
- [ ] A unit test verifies the short-circuit case (`## Result.outcome != "released"`) makes zero HTTP calls and returns a terminal-completed result — evidence: test asserts the GitHub client mock recorded zero invocations AND the returned result is terminal-completed.
- [ ] A unit test verifies malformed-`## Plan` or malformed-`## Result` JSON produces a wrapped error (not a `## Review` verdict) — evidence: test asserts the error chain contains the wrap message and that no `## Review` section was added to the markdown.
- [ ] A unit test verifies the bearer token never appears in returned error strings (mock the HTTP transport to fail and assert the resulting wrapped error does not contain the token value) — evidence: test asserts `strings.Contains(err.Error(), token) == false`.
- [ ] A manual smoke run via the local CLI entry point against the parked `Release bborbe-claude-yolo af4000c.md` task (or an equivalent test-task file with the same shape) produces a `## Review` section in the task file with `approved: true`, all three `checks` keys set to `true`, and a terminal-completed result printed by the CLI — evidence: file diff on the task file showing the new `## Review` section + CLI stdout line indicating completion.
- [ ] A manual smoke run on a tampered task file (same starting state but `commit_sha` in `## Result` altered to a non-existent SHA) produces a `## Review` section with `approved: false`, `tag_at_expected_sha: false`, and the CLI's printed agent result shows `Status: failed` — evidence: file diff showing the `## Review` JSON contents + CLI stdout line showing `Status: failed`. (The local-CLI path does NOT exercise the controller's unassign / `## Failure` / `previous_assignee` mutation; that's platform behavior tested separately.)

**Scenario coverage:** NO new dark-factory scenario. The behavior is reachable by (a) unit tests against the new step with a stubbed GitHub HTTP client (counterfeiter mock on the API seam) and (b) the local-CLI smoke run against a real parked task file. A scenario would require a real release cycle (real tag push, real CHANGELOG rewrite, real CRD dispatch) for which the manual smoke run already provides operator-level verification at lower cost.

## Verification

```
cd agent/github-releaser && make precommit
```

Behavioral verification:

1. `grep -nE 'agentlib\.NewPhase\(' agent/github-releaser/pkg/factory/factory.go` returns three lines — one each for `TaskPhasePlanning`, `TaskPhaseExecution`, `TaskPhaseAIReview`.
2. `grep -nE 'planning|execution|ai_review|intentionally omitted' agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml` shows all three phases listed under `trigger.phases` and zero "intentionally omitted" comment lines.
3. Local smoke run (no Kafka, no cluster) against a copy of the parked release task file, invoking the agent's local CLI entry point with `ai_review` as the phase to drive — outputs a `## Review` section in the file with `approved: true` and `checks` all `true`, plus a CLI-stdout completion line. The file diff is the evidence; the stdout line is corroborating.
4. Repeat (3) with a tampered task file where the `commit_sha` in `## Result` is altered to a non-existent SHA — outputs a `## Review` section with `approved: false` and `tag_at_expected_sha: false`, and the CLI's printed agent result shows `Status: failed`. The local-CLI smoke run does NOT exercise the controller's unassign / `## Failure` / `previous_assignee` mutation — that is platform behavior tested separately by spec 039's live-verify path; the agent step's contract here is "return `failed` with `## Review` written," and that's what we verify locally.

## Do-Nothing Option

Unacceptable. Every successful release currently parks the task in `phase: ai_review` indefinitely; the in-progress queue accumulates one orphaned task per release and the operator must manually mark each completed. More importantly, the verification work that `ai_review` exists to perform is not happening — a release that silently fails to push the tag, or whose CHANGELOG rewrite is lost in a merge race, would not be caught. The do-nothing alternative would be to delete `ai_review` from the slash-command contract and have `execution` return a terminal result, which trades observability for simplicity and rolls back the Phase 1 design intent. Implementing the missing step is strictly less work than re-justifying its absence.
