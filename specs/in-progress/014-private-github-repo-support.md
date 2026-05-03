---
status: verifying
approved: "2026-05-03T17:48:43Z"
generating: "2026-05-03T17:48:43Z"
prompted: "2026-05-03T17:56:12Z"
verifying: "2026-05-03T18:33:54Z"
branch: dark-factory/private-github-repo-support
---

## Summary

- The pr-reviewer agent gains the ability to clone and review pull requests on **private** GitHub repositories without leaking the GitHub token into the vault, the task body, the verdict, or any log line
- Pod-mode (Kafka entry) injects a github-auth-setup step that wires `gh auth setup-git` at startup using the pod's `GH_TOKEN` env var when present; subsequent `git clone` calls inherit the credential helper transparently
- Local-CLI mode (`cmd/run-task`) injects a no-op auth setup so the developer's existing `gh auth login` continues to handle credentials — `~/.gitconfig` is never mutated by the agent
- `GH_TOKEN` stays **optional** in both entry points so the agent remains usable in deployments that target bitbucket or other future hosts without a github token configured
- A clone failure on a private host where the agent could not authenticate routes the task to `phase: human_review` with a diagnostic naming the rejected `host/owner/repo` and pointing operators at the GH_TOKEN config — distinct from the existing "clone URL malformed" hard-failure path
- Closes the dev/prod gap where `bborbe/code-reviewer#3` (public) is reviewable but `bborbe/trading#110` (private) escalates to human_review with no `## Review` section because the agent cannot clone private code

## Problem

Today (2026-05-03) the pr-reviewer agent ships first-usable for **public** GitHub repositories. End-to-end review on `bborbe/code-reviewer` PR #2/#3 (public) works through dev. Private repository PRs cannot be reviewed: the agent's `repo_manager.go` shells out to `git clone --bare $URL` with no credential setup, and git on the alpine-based pod has no credential helper configured. For `bborbe/trading#110` (private) the clone fails with `could not read Username for 'https://github.com'`, the execution phase returns failure, the controller escalates the task to `phase: human_review`, and the `## Review` section never gets written. The watcher already detects private PRs (proven 2026-05-03 — prod task `b0cec7d9` was created for trading#110), and `pr-review-of-ben` has push access on the trading repo (verified via `gh api /repos/bborbe/trading`), so the only missing piece is wiring the pod's git to use the `GH_TOKEN` env var that is already present.

## Goal

After this work, the agent reviews private GitHub repositories end-to-end on the same code path that handles public repositories. In pod mode, `gh auth setup-git` runs once at startup using the pod's `GH_TOKEN` and configures git's credential helper for `github.com` operations. Subsequent `git clone` calls in the execution phase authenticate transparently. The token never appears in the vault file, in the task body, in the verdict JSON, in clone URLs persisted anywhere, or in agent logs. A pod started without `GH_TOKEN` keeps reviewing public repositories normally and surfaces a clear human_review diagnostic the first time it tries to clone a private repository — operators receive an actionable misconfiguration signal rather than a silent failure. Local-CLI mode is unaffected: the developer's pre-existing `gh auth login` continues to handle credentials.

## Non-goals

- Do NOT add bitbucket or any non-github auth wiring in this spec — the github auth setup is one piece of a future per-host scheme; bitbucket arrives in its own spec
- Do NOT make `GH_TOKEN` required in either entry point — bitbucket-only deployments must still start cleanly
- Do NOT introduce token-injected URLs (`https://x-access-token:$TOKEN@github.com/...`) anywhere in the codebase — credentials live only in git's credential helper, never in URL strings
- Do NOT mutate the developer's `~/.gitconfig` from `cmd/run-task` — the local-CLI auth path is the developer's existing setup, not the agent's responsibility
- Do NOT change the agent contract (`clone_url`, `ref`, `base_ref`, `task_id`) or the verdict schema
- Do NOT change the watcher, the controller, the Kafka command schema, or the vault task structure

## Desired Behavior

1. The agent's pod startup runs a github-auth-setup step that, when `GH_TOKEN` is non-empty, configures git's credential helper to authenticate `github.com` HTTPS clones using the token; when `GH_TOKEN` is empty the step is a no-op and pod startup succeeds
2. Subsequent `git clone --bare <https-url>` calls inside the execution phase succeed for both public and private GitHub repositories without any code change at the clone call site
3. The `cmd/run-task` local-CLI entry point performs no agent-managed auth setup and never writes to the developer's `~/.gitconfig` or any other shared config
4. A clone that fails specifically because the pod has no usable credentials for the target host returns a `NeedsInput` outcome routing the task to `phase: human_review` with a diagnostic that names the parsed `host/owner/repo` (no token), distinguishing this case from the existing hard-failure paths (malformed URL, network error, ref-not-found)
5. The `GH_TOKEN` value never appears in the task vault file, the task body, the `## Review` JSON, the verdict, agent log lines, error wrap messages, exec.Cmd args printed by go (`cmd.String()`), or any test assertion fixture
6. A pod started with `GH_TOKEN=""` continues to review public repositories successfully and produces the human_review diagnostic only on the first private-clone attempt — public processing is not affected by missing token

## Constraints

- Both entry points share the same auth-setup contract — the wiring lives in `factory.RunConfig`, the pod main injects a real github-auth-setup, the local-CLI main injects a no-op; no special-casing inside `factory.RunAgent` based on entry point
- The auth-setup contract is `Setup(ctx context.Context) error`; the pod implementation calls `gh auth setup-git` (which `~/Documents/workspaces/code-reviewer/agent/pr-reviewer/Dockerfile` already installs via `apk add github-cli`); the no-op implementation returns nil
- Auth-setup logic lives in a new package under `agent/pr-reviewer/pkg/` (not `pkg/factory/`) per `go-factory-pattern.md` — factories compose constructors, they do not own subprocess invocation
- Error wrapping uses `github.com/bborbe/errors` `Wrapf(ctx, err, ...)` — never `fmt.Errorf` and never `%w` chaining; safe identifiers only in wrap messages (`owner/repo`, never `cloneURL` if a future change ever interpolates a token)
- Logging respects `go-logging-guide.md`: the agent logs `host/owner/repo` and the configured allowlist size, never the token, never `cmd.String()`, never `cmd.Args` after any subprocess call that touches credentials
- The `gh auth setup-git` call is a single subprocess invocation; gosec G204 suppression is permitted with an explanation comment naming the binary as hardcoded `gh` and the only argument as a hardcoded `auth setup-git`
- The new auth-setup interface is mocked via counterfeiter (`go-mocking-guide.md` — never hand-written fakes); behavioral tests assert the no-op path returns nil and the real-pod path is exercised through the boundary it crosses
- The clone-failure → `NeedsInput` path uses the existing `agentlib.AgentStatusNeedsInput` semantic; the new diagnostic distinguishes "no usable credentials for host" from existing failures by parsing the git stderr output into a typed sentinel error or by checking for known auth-failure substrings produced by git itself
- `GHToken` arg shape stays `required:"false"` in both `agent/pr-reviewer/main.go` and `agent/pr-reviewer/cmd/run-task/main.go` — bitbucket-only deployments must continue to start
- Existing planning-phase `gh pr view` and `gh pr diff` calls inside the claude-yolo container are unaffected — those run with `GH_TOKEN` env-passed to the claude subprocess, a separate path from the host-side `git clone`
- Reference doc: `docs/architecture.md` "Agent contract" section — `clone_url` is the canonical identity, host-qualified parsing is shared with the repo-allowlist work

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Pod starts with `GH_TOKEN=""` and reviews a public github PR | `gh auth setup-git` is skipped (no-op); plain clone works | None needed |
| Pod starts with `GH_TOKEN=""` and reviews a private github PR | Clone fails with auth error; execution step returns `NeedsInput`; vault task routes to `phase: human_review` with diagnostic naming `host/owner/repo` and pointing operators at GH_TOKEN config | Operator sets `GH_TOKEN` in pod config and re-triggers task |
| Pod starts with `GH_TOKEN` set but invalid (revoked, wrong scope) | `gh auth setup-git` succeeds at startup (it does not validate the token); clone fails on first private attempt with a 401/403 from github; execution step returns `NeedsInput` with the underlying git error preserved in the diagnostic | Operator rotates the token and re-triggers |
| Pod starts with `GH_TOKEN` set and reviews a public repo | Clone works (authenticated, but token harmless for public repos) | None needed |
| `gh auth setup-git` itself fails at startup (e.g., gh binary missing, fs permission denied) | Pod startup fails with the wrapped error from the auth-setup step; agent never serves a request | Operator inspects the pod log and fixes the image / fs / config |
| Local-CLI run with the developer's `gh auth login` already configured | NoopAuthSetup runs (returns nil); developer's existing credential helper handles the clone | None needed |
| Local-CLI run with no developer-side gh auth at all | Plain clone fails for private repos; NoopAuthSetup is not at fault and does not pretend to handle this; the developer fixes their local environment | Developer runs `gh auth login` once on their machine |
| Bitbucket clone reaches the agent (future) | This spec does not modify the bitbucket path; clone goes through whatever bitbucket-side wiring exists or returns the existing failure mode unchanged | Out of scope; future bitbucket-auth spec |
| Two concurrent pods running the auth-setup step | `gh auth setup-git` writes to a per-pod home dir (`/home/claude/.gitconfig`); pods do not share home dirs; no contention | None needed |
| The diagnostic for the no-credentials path accidentally leaks the token | Treated as a critical bug; CHANGELOG calls out the incident; rotate token and ship a fix | Test must explicitly assert `Expect(diagnostic).NotTo(ContainSubstring(realToken))` to prevent regressions |

## Security / Abuse Cases

- **The token is a write-scoped GitHub credential.** Leaking it via vault file, task body, log line, or error message constitutes a credential exposure incident. The trust boundary is the **pod filesystem**: token sits in the pod's `~/.gitconfig` written by `gh auth setup-git`, and the pod's ephemeral storage is not shared with any other component. The vault task file (committed to git) and the verdict JSON (published to Kafka and read by downstream consumers) are outside the trust boundary and must never carry the token.
- **The clone-failure diagnostic surface is the highest-risk leak point.** The diagnostic includes the parsed `host/owner/repo` and a hint about GH_TOKEN — the test suite must assert the diagnostic does not contain the literal token value, even when `GH_TOKEN` happens to overlap with the parsed identifier (e.g., a token that contains `owner/repo` substring by coincidence).
- **`exec.Command` arg lists are not sanitized.** The agent must not log `cmd.String()` or `cmd.Args` for any subprocess that touches credentials, even on failure. Since `gh auth setup-git` does not take the token as an argument (it reads `GH_TOKEN` from env), `cmd.Args` for that call is safe to log, but the convention applies repo-wide: never log subprocess args without explicit per-call review.
- **Local-CLI mutating the developer's environment.** A bug where NoopAuthSetup accidentally calls the real implementation would write the developer's `~/.gitconfig`, potentially overriding their existing credential helper. The factory wiring must be straight assignment (no conditional that could flip default to real impl), and a test must assert `cmd/run-task` factory wiring uses the noop type literal.
- **Token in task frontmatter.** The watcher publishes `clone_url` as a plain HTTPS URL (`https://github.com/owner/repo.git`) — no token interpolation. This spec preserves that contract. Any future code that constructs a token-injected URL would land outside this spec's scope and would itself be a security regression.

## Acceptance Criteria

- [ ] A new package `agent/pr-reviewer/pkg/githubauth/` defines an interface with method `Setup(ctx context.Context) error`, two implementations (a real `gh-auth-setup-git`-based one and a no-op), and a counterfeiter mock for the interface
- [ ] `agent/pr-reviewer/pkg/factory/runner.go` `RunConfig` carries a field of the new auth-setup interface; `factory.RunAgent` calls `cfg.AuthSetup.Setup(ctx)` immediately after `PluginInstaller.EnsureInstalled` and before `CreateAgent`; failure of the setup call propagates up as a wrapped error
- [ ] `agent/pr-reviewer/main.go` injects the real implementation into `RunConfig.AuthSetup`; the implementation reads `GH_TOKEN` from env and is a no-op when the token is empty (does not invoke `gh`)
- [ ] `agent/pr-reviewer/cmd/run-task/main.go` injects the no-op implementation into `RunConfig.AuthSetup`
- [ ] `GHToken` arg shape in both entry points stays `required:"false"`; the comment is updated to mention git-auth setup at pod startup; no other arg shape changes
- [ ] The execution step (`pkg/steps_checkout_execution.go`) detects clone failures whose error matches the github-no-credentials condition and returns `agentlib.AgentStatusNeedsInput` with a diagnostic naming the parsed `host/owner/repo` and a configured-allowlist-size-style hint about GH_TOKEN; existing failure paths (malformed URL, ref not found, generic network error) keep their current `Status: Failed` semantics
- [ ] Unit tests cover: the real auth-setup invokes `gh auth setup-git` exactly once when the token is non-empty and is not invoked when the token is empty; the no-op auth-setup always returns nil; factory wiring asserts the pod main injects the real impl and the local-CLI main injects the no-op (type-literal assertion or equivalent); clone-failure-to-`NeedsInput` translation in the execution step; the diagnostic emitted on the no-credentials path does not contain the literal token value (asserted with a recognizable fake token, including a token whose substring overlaps `owner/repo`)
- [ ] Integration: `make run-dummy-task` against a public PR continues to work in local-CLI mode (no regression)
- [ ] After dev deploy: triggering trading PR #110 (private, `bborbe/trading`) via the watcher results in a vault task whose `## Review` section contains a verdict from `/coding:pr-review` with at least one specialist sub-agent fan-out
- [ ] After prod deploy: re-triggering the existing prod task `b0cec7d9` for trading PR #110 produces a populated `## Review` section instead of escalating to human_review
- [ ] Pod logs grep for the literal token string returns zero hits across both the success path and every failure path (verified by an explicit test that exercises each failure mode with a recognizable fake token)
- [ ] **Scenario coverage:** the change introduces a new auth-boundary integration seam (host-side `git clone` reaches `github.com` with a credential helper configured by `gh auth setup-git` at pod startup). A new or extended scenario under `scenarios/` exercises (a) trigger a private PR, (b) confirm vault task progresses to `phase: done` with `## Review` populated, (c) confirm pod log contains zero hits for the token literal. A second scenario covers the no-token path: trigger a private PR with `GH_TOKEN=""`, confirm `phase: human_review` with the diagnostic name visible in the task body
- [ ] CHANGELOG `## Unreleased` entry covers the agent's auth-setup change, the new package, and the operator-visible behavior shift on private repos

## Verification

```
cd agent/pr-reviewer && make precommit
```

Manual smoke after dev deploy:

1. Build and deploy `agent/pr-reviewer` to dev with `GH_TOKEN` set in the pod env (it already is — used by planning's `gh pr view`)
2. Confirm pod startup log line indicates auth-setup completed (count-only style: e.g. `agent-pr-reviewer git auth: gh setup-git complete`)
3. Trigger trading PR #110 via the dev watcher; observe vault task progress to `phase: done` and `## Review` section populated with a verdict
4. Grep dev pod logs for the literal token value (the secret string itself, not the env var name `GH_TOKEN`) — assert zero hits
5. Open a fresh public PR in `bborbe/code-reviewer` and confirm it still completes end-to-end (no regression)
6. Manually patch a dev pod's `GH_TOKEN` env var to empty; trigger any private PR; confirm task escalates to `phase: human_review` with diagnostic naming the rejected `host/owner/repo`; restore env

After prod deploy, repeat steps 3-5 against prod and re-trigger the existing trading PR #110 prod task `b0cec7d9`.

## Do-Nothing Option

The agent stays usable for public repositories only. Trading PRs (private), the actual production review surface for the operator's own work, can never be reviewed by the agent — every trading PR escalates to human_review with no `## Review`. The post-back-to-GitHub future capability (Task A4) is moot until private-repo support exists, since the operator's main private-repo target is the trading codebase. The cost of the do-nothing option compounds: every additional trading-side feature, fix, or deploy ships without the specialist sub-agent review the agent was built to provide. The token is already in the pod env; the only thing standing between the agent and useful prod traffic is the credential-helper wiring this spec lands.
