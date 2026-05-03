---
status: generating
tags:
    - dark-factory
    - spec
approved: "2026-05-03T15:27:22Z"
generating: "2026-05-03T16:23:22Z"
branch: dark-factory/repo-allowlist-stage-isolation
---

## Summary

- The watcher and the agent each gain an optional `REPO_ALLOWLIST` env var that filters PR processing to a configured set of host-qualified repositories
- **Empty allowlist (default `""`, also covers unset env) is allow-all** — preserves today's behavior bit-for-bit in both services. Non-empty enforces membership at two layers (watcher emits no task; agent refuses to clone)
- The two-layer design is defense-in-depth: the watcher layer is the primary stage-isolation fix, the agent layer is the safety net for stale or misconfigured tasks
- Format is host-qualified (`host/owner/repo`) so the same mechanism extends to Bitbucket without ambiguity
- Closes the dev/prod collision: with `dev.env` and `prod.env` configured to non-overlapping repo sets, the two stages can no longer derive the same task ID for the same PR and trample each other's vault files

## Problem

The GitHub PR watcher and the agent PR reviewer both run in `dev` and `prod` namespaces. Both stages' watchers use the same `REPO_SCOPE=bborbe`, so both observe the same PRs. The task ID is a deterministic hash of `(owner, repo, number)` with no stage component, so both watchers emit the same `CreateTaskCommand` for the same PR. Both controllers then write to the same vault path — dev wins, prod retries with HTTP 500 indefinitely. The end-to-end milestone proven on dev today (2026-05-03) is blocked from prod burn-in not by a functional bug but by this stage-isolation gap. Two `REPO_ALLOWLIST` lines are already present in `dev.env` and `prod.env` but no code reads them.

## Goal

After this work, each stage can be configured with a non-overlapping set of host-qualified repositories. The watcher publishes a task only for PRs whose repo is on its allowlist; the agent clones only for tasks whose `clone_url` is on its allowlist. With dev set to one repo and prod set to another, the two stages cannot collide on a task ID for the same PR, and a stale task that somehow reached the wrong stage cannot cause that stage to clone code it was not authorized to clone. An empty allowlist (default) is a deliberate "everything in scope" mode that preserves today's behavior unchanged.

## Non-goals

- Do NOT change `REPO_SCOPE` semantics, validation, or the `user:%s` GitHub search-query format — `REPO_SCOPE` continues to bound what the watcher fetches; `REPO_ALLOWLIST` filters what survives
- Do NOT introduce a shared library between the watcher module and the agent module for the allowlist parser/filter — the helper is small enough that each service owns its own copy
- Do NOT remove or rename existing tests — additive coverage only
- Do NOT add per-repo or per-branch policy beyond simple membership — composition with other filters (author trust, label gates) is out of scope
- Do NOT alter the Kafka command schema, the controller, or the vault task structure
- Do NOT change Bitbucket-side code in this spec — the format is chosen to extend cleanly, but the Bitbucket watcher is not in scope here

## Desired Behavior

1. Both services accept an optional comma-separated `REPO_ALLOWLIST` configuration via env var and matching CLI flag (same convention as existing config in each service)
2. Each entry is a host-qualified triple `host/owner/repo` (example: `github.com/bborbe/code-reviewer`); whitespace and empty entries are stripped on parse
3. An empty (or unset) allowlist is **allow-all** in both services — every PR matched by `REPO_SCOPE` (watcher) and every task that reaches the agent processes as it does today, bit-for-bit unchanged
4. A non-empty allowlist with a malformed entry (missing slash, wrong shape, trailing comma producing an empty tail after trim) is a startup failure with a clear operator-facing reason
5. The watcher consults its allowlist after existing pre-task filters and skips emitting a task command for any PR whose `host/owner/repo` is not on the list; skipped PRs increment the existing skip metric and produce no Kafka publish
6. The agent derives `host`, `owner`, and `repo` from the task's `clone_url`, consults its allowlist before cloning, and refuses to clone any task whose parsed `host/owner/repo` is not on the list; refusal returns a `NeedsInput` outcome with a diagnostic naming the parsed repo and the configured allowlist size, routing the task to a human-review queue rather than failing it
7. The agent treats a `clone_url` that fails to parse as a hard failure (existing behavior preserved), distinct from the soft "not on allowlist" outcome

## Constraints

- The Kafka command schema and the vault frontmatter shape are frozen — this change adds no fields, only filters
- Error wrapping in both services uses `github.com/bborbe/errors`
- The watcher and the agent live in separate Go modules (`watcher/github/go.mod`, `agent/pr-reviewer/go.mod`); no new shared dependency is introduced for this change
- The allowlist entry format must be `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$` (host segment plus owner segment plus repo segment, slash-delimited); validation is identical in both services
- Existing callers of the current clone-URL parser (today returns a concatenated string) must continue to work unchanged; the agent's new derivation of `host`/`owner`/`repo` is additive (the prompt decides whether to refactor in place or add a sibling function)
- Each service's allowlist is independent — they do not have to agree and they do not share state at runtime; defense-in-depth means each layer can refuse independently
- The watcher's existing pre-task filter chain (bot allowlist, draft skip, author trust, dedupe by head SHA) is unchanged — the repo allowlist becomes one additional filter, applied after the existing chain
- The existing `REPO_SCOPE` env var, its CLI flag, and its validation are unchanged
- The two `REPO_ALLOWLIST` lines already present in `dev.env` and `prod.env` are non-host-qualified and must be updated to the host-qualified form as part of this work
- Reference doc: `docs/architecture.md` describes the agent contract (provider-agnostic, ref-agnostic) — the allowlist key chosen here (host-qualified) preserves that property when Bitbucket is added later

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Allowlist entry malformed (missing slash, wrong shape) | Service refuses to start with operator-facing log naming the offending entry | Operator fixes config and restarts |
| Allowlist contains whitespace or empty entries | Entries are silently dropped during parse; remaining valid entries are used | None needed |
| Allowlist empty / unset | Both services preserve today's behavior (no filtering); operators see "0 entries" at startup | None needed |
| Watcher allowlist accidentally configured identically across stages | Stage-collision returns; controller log shows 500 retries on the losing stage | Operator splits the allowlists; collision is operator-visible, not silent |
| Agent receives a task with `clone_url` parsed to repo not on allowlist | Agent returns `NeedsInput` with diagnostic; task is routed to human review without a clone | Operator inspects, either expands allowlist or aborts the task |
| Agent receives a task with `clone_url` that fails to parse | Existing failure path preserved (`Status: Failed` with parse error) — distinct from the soft allowlist-miss path | None needed |
| Watcher allowlist filters out a PR that the agent allowlist would accept (or vice versa) | Each layer is independent; whichever filters first wins; this is the expected defense-in-depth posture | None needed |

## Security / Abuse Cases

- The agent layer is the trust boundary: a misconfigured or stale task that names a `clone_url` outside the agent's allowlist must NOT result in a clone, because cloning runs the PR's code in the agent's pod identity
- An attacker cannot induce the agent to clone an arbitrary repo by injecting a task into the vault — the controller-to-agent path passes through the agent's allowlist check before any network call
- The `clone_url` value originates outside the agent's process (controller frontmatter); it must be parsed and validated, not interpolated, and the parse-failure path must be distinct from the allowlist-miss path so the diagnostic is unambiguous
- The watcher layer alone is not a security boundary against stale tasks (a task the watcher emitted before its allowlist was tightened is still on the bus); the agent layer is what closes that gap
- The allowlist itself is operator-controlled config; whitespace and empty entries are dropped on parse so a misconfigured pod cannot accidentally widen scope to "match anything"

## Acceptance Criteria

- [ ] Watcher: `REPO_ALLOWLIST` env var and matching CLI flag are wired through the watcher's existing config plumbing, validated at startup, parsed into an allowlist helper, and consulted as a filter in the new-PR processing path after existing filters
- [ ] Watcher: PRs not on the allowlist increment the existing "skipped" metric and do not produce a Kafka publish
- [ ] Watcher: empty allowlist preserves today's behavior; existing watcher tests pass without modification
- [ ] Watcher: non-empty allowlist behavior is covered by tests on both positive (in list) and negative (not in list) paths
- [ ] Agent: `REPO_ALLOWLIST` env var and matching CLI flag are wired through both entry points (the K8s Job entry and the local `run-task` entry) and plumbed through the run config into the checkout step
- [ ] Agent: the clone-URL parser exposes `host`, `owner`, and `repo` as separate fields (or via a sibling function) without breaking existing callers
- [ ] Agent: when allowlist is non-empty and the parsed `host/owner/repo` is not on it, the checkout step returns `Status: NeedsInput` with a diagnostic that names the parsed repo and the configured allowlist size, and does NOT clone
- [ ] Agent: when `clone_url` fails to parse, existing failure behavior is preserved (distinct from the allowlist-miss path)
- [ ] Agent: empty allowlist preserves today's behavior; existing agent tests pass without modification
- [ ] Agent: non-empty allowlist behavior is covered by tests on both positive (in list) and negative (not in list) paths
- [ ] Both services: malformed allowlist entry is a startup failure with a clear operator-facing log
- [ ] Both services: configured allowlist size is logged at startup (count only, not contents)
- [ ] `dev.env` is updated to the host-qualified form for the dev test-bed repo
- [ ] `prod.env` is updated to the host-qualified form for the prod target repo
- [ ] `CHANGELOG.md` has an `## Unreleased` entry covering both services
- [ ] **Scenario coverage:** because this change introduces a stage-isolation behavior at two integration seams (watcher → Kafka and controller → agent clone), at least one scenario covers the watcher filter end-to-end (allowlisted PR produces a task, non-allowlisted PR produces none), and either an additional scenario or an expansion of an existing pr-reviewer scenario covers the agent's clone-refusal path. New or extended scenario files live under `scenarios/` and follow the existing numbering.

## Verification

```
cd watcher/github && make precommit
cd agent/pr-reviewer && make precommit
```

Manual smoke after dev deploy:

1. Build and deploy watcher and agent to dev with `dev.env` pointing the allowlist at the dev test-bed repo only
2. Build and deploy to prod with `prod.env` pointing at a different repo
3. Open a PR in the dev allowlisted repo; confirm dev controller writes the vault file, prod controller log shows no attempt for that task
4. Open a PR in the prod allowlisted repo; confirm prod controller writes the vault file, dev controller log shows no attempt
5. Inject (via test harness) a stale task whose `clone_url` is outside the agent's allowlist; confirm the agent returns `NeedsInput` and does not clone
6. Confirm both services log the configured allowlist size at startup

## Do-Nothing Option

Not acceptable for prod burn-in. Without a per-stage repo filter, dev and prod will continue to derive the same task IDs for the same PRs, the same vault files will be contended, and prod will keep silently losing to dev. The collision is observable today (HTTP 500 retries on prod controller for PR #3) and blocks the next milestone. The allowlist is the smallest change that closes the collision while also adding a defense-in-depth layer at the agent — both pieces are needed; doing only the watcher side leaves stale tasks dangerous, and doing only the agent side leaves the dev/prod write contention in place.
