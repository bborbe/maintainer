---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-02T09:54:00Z"
generating: "2026-05-02T09:54:01Z"
prompted: "2026-05-02T10:04:18Z"
verifying: "2026-05-02T10:24:41Z"
completed: "2026-05-03T20:13:28Z"
branch: dark-factory/watcher-author-trust-filter
---

## Summary

- The GitHub PR watcher gains a pluggable trust-decision framework so untrusted PR authors can be routed to human review instead of auto-executing on the agent fleet
- The first leaf of the framework is a static GitHub-login allowlist; future leaves (collaborator check, repo allowlist, label gates) plug into the same composition without touching the watcher core
- Trusted PRs land in the vault exactly as they do today (auto-processing); untrusted PRs land as `human_review` / `todo` with a body explaining who was rejected and why
- The deployment fails to start when no trusted authors are configured — there is no silent-pass-through for misconfigured pods
- Closes the smallest possible piece of D1: a stranger opening a PR no longer triggers code execution; the rest of D1 (network policy, repo allowlist, prompt-injection guard, threat model doc) is explicitly out of scope and follows in its own specs

## Problem

The watcher today publishes a `CreateTaskCommand` for every open PR in scope, with frontmatter that immediately makes the task pickup-eligible by the agent. There is no check on who opened the PR. A fork PR from any GitHub user — including a complete stranger — kicks off the full review pipeline, which currently runs `make precommit` inside the PR's workdir. That is remote code execution as the agent's pod identity, with access to the bot PAT, Kafka, and the vault. The hole exists today in dev and would exist in prod the moment the pipeline is promoted. Closing this is the precondition for prod burn-in (goal task C2).

## Goal

After this work, every PR the watcher sees passes through a trust decision before its task is published. PRs from configured trusted authors flow through the existing fast-path unchanged. PRs from anyone else still produce a vault task — but a task that sits idle in `human_review` until a human reviews and either promotes it (`phase: in_progress`) or aborts it. The trust decision is composable: the next deliverable can add a "is repo collaborator" leaf or a "has approved-label" leaf without modifying the watcher core. The framework refuses to start when its trust configuration would leave the door open.

## Non-goals

- Implementing additional trust leaves beyond the static author allowlist (collaborator check, org-membership check, label-based check, repo allowlist) — separate specs
- NetworkPolicy egress filtering — separate D1 deliverable
- Stripping `make precommit` from the agent's review pipeline — separate D1 deliverable
- Read-only watcher PAT provisioning — separate D1 deliverable
- Threat model documentation (`docs/security-model.md`) — separate D1 deliverable
- Any change to the controller, the agent, the Kafka command schema, or the vault task structure — the watcher writes whatever frontmatter it likes; everything downstream is unchanged
- Any change to bot-allowlist filtering — that filter remains a separate, earlier-stage concern (skip entirely vs route-to-human)
- Per-repo or per-org trust configuration — a single global trusted-authors list is sufficient for the first deliverable

## Desired Behavior

1. The watcher exposes a single composable trust-decision concept: any number of trust leaves can be combined with `And`, `Or`, and `Not` into a compound decision that the watcher consults once per PR
2. Each trust decision returns both an outcome (trusted vs not trusted) and a human-readable description; compound decisions preserve every leaf's reason in the description so the audit trail explains why a PR was routed to human review
3. The first available leaf decides trust by membership in a configured list of GitHub login names. Match is **case-sensitive** (exact byte equality between `pr.User.Login` and configured entry — operator is responsible for matching GitHub's stored casing). An empty configured list always denies.
4. The watcher reads its trusted-author list from configuration (env var + matching CLI flag, same conventions as the existing bot allowlist); whitespace and empty entries are stripped before the list is used
5. When no trusted authors are configured, the watcher refuses to start and logs a clear, operator-facing reason (a misconfigured pod must not silently trust everyone)
6. On each new PR, the watcher consults the trust decision once before publishing the task command. Trusted PRs publish with today's frontmatter (auto-processing). Untrusted PRs publish with frontmatter that routes the task to a human-review queue and a body that explains, by name, who the author was and which trust rule rejected them
7. The same trust decision applies on the re-review path (force-push detection): a force-push by an untrusted author must not flip the task back into auto-processing
8. PRs whose author cannot be determined (defensive: missing user data in the GitHub response) are treated as untrusted
9. The watcher logs, at startup, the configured trusted-author count (a number, not the names) so operators can confirm the list was loaded; a zero-length list is a startup failure (see behavior 5), not a warning

## Constraints

- The trust framework must follow the canonical Go boolean-combinator pattern documented at `~/.claude/plugins/marketplaces/coding/docs/go-boolean-combinator-pattern.md` — slice-typed `And` / `Or`, function-style `Not`, structured result with description, full audit trail in compound descriptions, fail-safe handling of empty configurations
- All error wrapping uses `github.com/bborbe/errors` (`errors.Wrapf`, `errors.Errorf`)
- Set-of-strings helpers, where useful, use `github.com/bborbe/collection` (idiomatic for this repo)
- The Kafka command schema (`CreateTaskCommand`, frontmatter map) is frozen — the watcher writes different values into the same fields, no schema change
- The controller is not modified — it writes whatever frontmatter the watcher provides
- No new GitHub API calls — the PR's author login is already available from the existing search response
- The existing `BotAllowlist` (which skips bot-authored PRs entirely) remains unchanged and operates as a separate, earlier filter; trust evaluation happens for the human-author PRs that survive bot filtering
- The trust list is a single global list per watcher instance; per-repo configuration is out of scope
- Configuration follows the same pattern as the existing `BOT_ALLOWLIST` field (CLI flag + env var, comma-separated) for operator consistency

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Trusted-authors config empty or unset | Watcher refuses to start; logs operator-facing reason naming the missing config | Operator sets the config and restarts the pod |
| Trusted-authors entry is empty string or whitespace only | Entry is silently dropped during parse; remaining valid entries are used | None needed |
| Trust leaf returns an error | Watcher logs the error, skips that PR for this poll cycle, retries next poll; cursor is not advanced past the failed PR | Transient errors recover on next poll |
| GitHub returns a PR with no author (defensive) | PR is treated as untrusted; routed to human review with a body explaining "author unknown" | Human reviewer inspects manually |
| PR author is on the bot allowlist | Existing bot-skip behavior wins; trust evaluation does not run | None needed |
| Force-push on an untrusted PR | Re-review path re-evaluates trust; if still untrusted, the human-review state is preserved (no auto-promotion) | None needed |

## Security / Abuse Cases

- An attacker controls the PR author login by virtue of opening the PR from their own GitHub account — that login flows directly into the trust decision, so the trust decision MUST be the gate, not a downstream consumer
- A misconfigured deployment with an empty trusted-authors list would otherwise silently trust everyone; the fail-fast on empty list is the mitigation
- Whitespace or empty entries in the configured list could otherwise expand the trust set to "any author with empty login" — explicit trim-and-drop on parse closes this
- The body written for untrusted PRs contains the author login and the failing rule's description; both come from the watcher's own state and the PR metadata; no untrusted PR content (title, body, diff) is interpolated into the human-review explanation, eliminating prompt-injection from this path
- The trust decision runs once per PR and is cached only via the cursor (head-SHA tracked); a force-push triggers a re-decision so a previously-untrusted author cannot escape the gate by pushing more commits

## Acceptance Criteria

- [ ] A new trust-framework package exists in the watcher module with the full canonical boolean-combinator structure (decision interface, structured result, function adapter, slice-typed `And` and `Or`, function-style `Not`)
- [ ] The framework includes one trust leaf that decides by static GitHub-login membership; empty membership configuration always denies
- [ ] A new CLI flag and matching env var (mirroring the existing bot-allowlist convention) supplies the trusted-authors list
- [ ] The watcher refuses to start when no trusted authors are configured, with a clear operator-facing log message
- [ ] The watcher logs the configured trusted-author count (count only, not contents) at startup
- [ ] The factory composes the default trust decision from the parsed trusted-authors list and injects it into the watcher
- [ ] On the new-PR publish path, trusted PRs produce frontmatter unchanged from today's behavior
- [ ] On the new-PR publish path, untrusted PRs produce frontmatter routing to human review (`phase: human_review`, `status: todo`) and a body that names the author and includes the failing trust description
- [ ] The body for untrusted PRs uses this exact template (with `{author}` and `{reasons}` substituted at runtime):

      ```
      ## Untrusted author
      
      This PR is by GitHub user **{author}** which did not pass the trust check:
      
      - {reasons}
      
      To auto-process this PR, edit the frontmatter above:
      - `phase: in_progress`
      - `status: in_progress`
      
      To dismiss, set `status: aborted`.
      ```
- [ ] On the force-push re-review path, trust is re-evaluated and untrusted PRs do not flip back into auto-processing
- [ ] Defensive handling: a PR with no author is treated as untrusted
- [ ] Unit tests cover: each leaf in isolation, `And` algebra, `Or` algebra, `Not` algebra, nested compositions, audit-trail presence in compound descriptions, empty-configuration fail-safe
- [ ] Integration tests cover: a trusted-author PR producing today's frontmatter, an untrusted-author PR producing the human-review frontmatter and the documented body
- [ ] Existing watcher tests continue to pass
- [ ] `make precommit` passes from `watcher/github/`
- [ ] `CHANGELOG.md` has an `## Unreleased` entry summarizing the trust gate
- [ ] **Scenario coverage:** new file `scenarios/006-watcher-author-trust-filter.md` simulates an untrusted-author PR end-to-end (watcher → Kafka → controller → vault) and verifies the resulting vault task has `phase: human_review`, `status: todo`, and the documented body — and that no agent execution is triggered. (This is a new integration seam: a frontmatter value the controller has never written before. It cannot be faked at the prompt level.)

## Verification

```
cd watcher/github && make precommit
```

Manual smoke after dev deploy:

1. Build and deploy to dev (`BRANCH=dev make buca` from `watcher/github/`)
2. Open a PR from a fork by a GitHub login NOT in the configured trusted-authors list
3. Within one poll cycle, confirm a vault task appears with `phase: human_review`, `status: todo`, and a body containing the author login and the failing trust description
4. Confirm no agent pod was spawned for that task
5. Manually edit the task's frontmatter to `phase: in_progress`, `status: in_progress`; confirm the agent picks it up on the next controller cycle
6. Open a second PR from a trusted login; confirm the existing fast-path frontmatter (auto-processing) is used and the agent runs as before

## Do-Nothing Option

Not acceptable. The current pipeline auto-executes on PRs from arbitrary GitHub users. This blocks goal task C2 (prod burn-in) and is a real foothold even in dev. Of the D1 deliverables, the author-trust gate is the smallest single change that closes the "stranger triggers code execution" path; deferring it forces every other D1 deliverable (network policy, prompt-injection guard, etc.) to defend against a problem that has a much simpler fix at the source.
