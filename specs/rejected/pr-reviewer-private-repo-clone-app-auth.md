---
status: rejected
tags:
    - dark-factory
    - spec
rejected: "2026-05-31T21:07:38Z"
rejected_reason: Superseded by spec 052 which shipped App-auth wiring in agent/pr-reviewer/main.go:236-263 (MintIAT + App-vs-PAT selection). Canary trading#133 confirmed working in prod. Multiple "spec 052 review" commits (Ginkgo auth specs, run-task PEM_KEY parity, go.mod cleanup) plus follow-up fix 56d2471 are all post-fix evidence.
---

## Summary

- The `agent/pr-reviewer` Job cannot clone private GitHub repos after the legacy `GH_TOKEN` PAT was retired on 2026-05-24. Public-repo reviews still work (clone needs no auth); every private-repo PR fails with "authentication required" and exhausts the Kubernetes Job backoff limit before the inner error reaches the task file.
- The GitHub App (`ben-s-pull-request-reviewer`) is installed on all of the owner's repositories with read+write on code and PRs; the pod already mints an Installation Access Token (IAT) at startup and exposes it as `GH_TOKEN` to subprocesses, but the IAT is not reliably reaching `git clone` of private repos.
- After this work, a manual trigger of a private-repo PR (canary: `bborbe/trading#133`) results in a successful clone, a posted review, and the task transitioning to `ai_review` — without any operator setting `GH_TOKEN` and without the IAT leaking into git logs, error messages, or the on-disk clone.
- Scope is limited to the `agent/pr-reviewer` clone path. Watcher auth and the agent's review-post path are out of scope — both already work via the App-auth go-github client and are unaffected.

## Problem

The agent's PR-reviewer Job clones the target repository to a local worktree before invoking Claude to produce a review. Before 2026-05-24, the pod received a long-lived Personal Access Token via the Kubernetes secret as `GH_TOKEN`, and `gh auth setup-git` (run at pod startup) configured a credential helper that read that env var at `git clone` time. That PAT was retired so the agent now mints a short-lived GitHub-App Installation Access Token in-process and treats it as the new `GH_TOKEN`.

Since the retirement, every private-repo PR review fails. The canary is `bborbe/trading#133`: three task files (`b21e657a`, `98c8a6c2`, `302b0d5a`) show repeated Kubernetes Job backoff-limit exhaustion. Only the earliest task file surfaced the inner cause before the backoff swallowed it: `clone failed for github.com/bborbe/trading: authentication required (set GH_TOKEN and re-trigger)`. Public-repo reviews continue to succeed because `git clone` of a public repo needs no credentials at all, so the regression is invisible there. The latest canary task is now sitting in `human_review` with `current_job: ""` and no review posted. Any private repo covered by the App's "All repositories" installation is affected.

## Goal

After this work, the `agent/pr-reviewer` Job successfully clones any private repository that the configured GitHub App installation has read access to, using a freshly minted Installation Access Token for each Job run, without any operator-managed PAT and without leaking the token into any persistent or observable surface.

## Non-goals

- Do NOT change the watcher services' GitHub-App auth (already works for public-repo metadata fetch via go-github).
- Do NOT change the agent's review-post path (already works via the App-auth go-github client).
- Do NOT reinstate any long-lived PAT or any operator-managed token.
- Do NOT introduce token caching across Job runs — each Job mints a fresh IAT.
- Do NOT broaden the App's installation scope or change which repos are accessible — accessibility follows whatever the GitHub App installation already grants.
- Do NOT add a per-task opt-out flag for App auth — invariant; if a future deployment demands PAT fallback in pods, that's a separate spec.
- Do NOT add a configurable knob for the credential-injection mechanism (URL rewrite vs. credential helper vs. askpass) — the implementation may pick any approach that satisfies the constraints; the choice is not user-tunable.

## Desired Behavior

1. A manual trigger of a private-repo PR for which the configured App installation has read access results in a successful `git clone` of that repo inside the Job pod — both for the initial bare clone and for subsequent fetches against an existing bare clone.
2. The credentials used by `git` come from an IAT minted by the pod itself at startup; no Kubernetes Secret, ConfigMap, or env var supplies a long-lived GitHub credential to the Job pod.
3. The IAT is never written to a file or git config entry that persists past the Job pod's lifetime, and never appears in `git remote -v`, `git config --list`, the bare clone's `config` file, the worktree's `config` file, the task markdown file in the vault, the pod's stdout/stderr at default verbosity, or any error message returned to the task file.
4. The IAT is never echoed in a `glog.Infof`/`glog.Errorf` log at any verbosity. Only an 8-character prefix (matching the existing `lib/githubapp/githubapp.go` convention) may appear.
5. A private-repo clone that fails because the App installation does not grant access to that specific repo produces a clearly distinct diagnostic from a generic "authentication required" message — the diagnostic names the repo and indicates the App-installation-scope cause, so the operator does not chase a credential bug.
6. The canary PR (`bborbe/trading#133`) routed through the existing admin trigger endpoint completes review posting within 10 minutes of the trigger, end-to-end.
7. The existing public-repo clone path continues to work unchanged. A clone of a public repo with no credential available proceeds anonymously and succeeds; introducing the App-auth path does not require credentials for public clones.
8. The behavior is exercised by both Job-pod and local-CLI (`cmd/run-task`) entry points consistently: when App credentials are provided to either entry point, private clones work; when neither App credentials nor any legacy fallback is provided, the agent fails fast at startup with the existing "neither App nor PAT configured" error (no change to that path).

## Constraints

- The GitHub App identifiers and PEM secret name in the existing Kubernetes manifests (`agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml`, env vars `APP_ID`, `INSTALLATION_ID`, `PEM_KEY` / `PEM_KEY_FILE`) are frozen — no manifest field renames.
- The existing `lib/githubapp` package and its `MintIAT` / `NewClient` API are frozen — this work uses them; it does not refactor them.
- The IAT lifetime is whatever GitHub returns (currently ~1h); the fix must not assume any longer lifetime. Each Job pod mints fresh on startup; no cross-Job caching, no on-disk persistence.
- The `repoallowlist` check in `checkoutExecutionStep` (spec 013) continues to run BEFORE any clone is attempted. A repo blocked by the allowlist must still produce `NeedsInput` without minting or injecting any token.
- The existing `git.IsGitAuthFailure` detector and its `NeedsInput` routing remain the failure-mode contract for genuine auth failures; only the diagnostic message text and the repo-not-installed sub-case may change.
- Existing tests under `agent/pr-reviewer/pkg/...` must keep passing without modification, except where a test currently asserts the literal string `"set GH_TOKEN and re-trigger"` — that exact string may be updated to reflect the new diagnostic.
- `errors.Errorf` / `errors.Wrapf` from `github.com/bborbe/errors` — house style.
- The fix runs inside the existing pod's filesystem layout (`/data/repos`, `/data/work`) — no new volume mounts.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility |
|---------|-----------|-------------------|----------|---------------|
| Private repo cloned for the first time; App installation grants read access | Job pod log shows `git clone --bare` returning 0; bare clone directory exists | Clone succeeds using freshly minted IAT; no credential left on disk | n/a | Reversible (next Job mints fresh) |
| Private repo whose owner/name is NOT covered by the App installation | Job pod log shows `git clone --bare` returning non-zero with 403 / "not found" from GitHub | Task result `NeedsInput` with diagnostic naming the repo and pointing at App-installation scope, NOT at "set GH_TOKEN" | Operator adds the repo to the App installation (or task is escalated to human_review) | Reversible |
| IAT mint fails at pod startup (GitHub API down, PEM unreadable, App suspended) | Pod fails `resolveAuth` before any clone is attempted; existing error path | Job exits non-zero with the existing `mint github app iat` error; no clone attempted | Operator inspects pod logs; existing path | Reversible |
| GitHub API rate-limits the IAT mint | `resolveAuth` returns the underlying ghinstallation error | Same as mint-fail above — Job exits, controller retries on Kafka redelivery | Operator waits for rate-limit window | Reversible |
| Existing bare clone on disk was created with a previous IAT now expired; new Job runs `git fetch` against it | `git fetch` returns 401/403 | Fetch uses freshly minted IAT (not the one persisted to the bare repo); succeeds | n/a — no expired token persisted to disk in the first place | Reversible |
| Two Jobs for different tasks against the same repo run concurrently in different pods | Both pods mint independent IATs | Each pod's clone uses its own IAT; no cross-pod credential sharing required | n/a | Concurrency-safe (pod-local credentials) |
| Job pod crashes mid-clone leaving a half-clone directory | Next Job sees existing dir; existing `runGitCmd("rev-parse", "--git-dir")` probe detects invalid bare repo and re-clones | Re-clone proceeds with fresh IAT | n/a — existing recovery path | Reversible |
| Public repo clone with NO App credentials configured at all (e.g. dev environment) | Pod startup fails at `resolveAuth` (existing path) | Pod exits with existing "neither App nor PAT configured" error | Operator sets App credentials | Reversible |
| Clock skew causes IAT to be rejected as not-yet-valid | `git clone` returns 401 with GitHub clock-skew message | Existing `IsGitAuthFailure` detector routes to `NeedsInput`; Kafka redelivery retriggers a fresh mint with corrected clock | Operator inspects NTP / restarts pod | Reversible |
| Bot operator inspects `git remote -v` or `cat .git/config` in a worktree after a clone | Plain-text inspection | No token present in remote URL or config — verifiable by grep | n/a | n/a |

## Security / Abuse Cases

- **Token exfiltration via persistent state**: the IAT must not be written to any file that outlives the Job pod (bare clone config, worktree config, vault task file, on-pod log file, exported metric label). The bare clone and worktree are on `/data/repos` and `/data/work` — both pod-local, but the worktree is also where Claude operates; an attacker-controlled PR diff that prompts the model to read `.git/config` must find no credential there.
- **Token leakage via logs and error messages**: the IAT must not appear in `glog` output at any verbosity. The existing `prefix8` convention from `lib/githubapp/githubapp.go` (8-character prefix only) is the upper bound. The existing "underlying git error intentionally NOT included in the diagnostic" guard in `steps_checkout_execution.go` remains — git stderr is logged only at `glog.V(2)` and is sanitized of any URL containing a token.
- **Cross-repo access via the installation**: the App's installation determines which repos can be cloned. The pod must not have any mechanism that lets a malicious task expand that scope (e.g. an attacker-controlled `clone_url` cannot escalate access to a repo the App doesn't have).
- **`clone_url` is attacker-influenced**: the URL comes through the task frontmatter and ultimately from the webhook payload / admin trigger. The existing `ParseCloneURLParts` validation and `repoallowlist` check remain the first line of defense. Token injection into the URL (if used) must happen AFTER the allowlist check, never before, so a blocked URL never sees a token.
- **Token injection cannot hang or retry forever**: IAT mint has a bounded timeout via the existing ghinstallation transport; clone uses the existing context with the Job's deadline. No new unbounded retry loops.
- **Subprocess env inheritance**: if the implementation chooses to pass the IAT via env to `git`/`gh` subprocesses, it must not export the token into the broader pod env in a way that the Claude subprocess running on the same pod sees it as `GH_TOKEN` (which would let prompt injection in a malicious PR diff exfiltrate it). The existing pattern (env passed only to Claude via `factory.RunAgent`) is acceptable; widening that surface is not.

## Acceptance Criteria

- [ ] A unit or package-level test exercises `repoManager.EnsureBareClone` against a private-repo URL with App credentials wired through; the test passes — evidence: `make test` exit code 0 in `agent/pr-reviewer/` AND the new test name appears in `make test` output.
- [ ] A unit test verifies that after a successful clone, the bare repo's `config` file does NOT contain the substring `x-access-token` or any 30+ character token-like string — evidence: test assertion on file contents; `grep -c 'x-access-token' <barePath>/config` returns 0.
- [ ] A unit test verifies that after a successful worktree creation, `git -C <worktreePath> remote get-url origin` output does NOT contain a token — evidence: test assertion; command output checked for absence of `x-access-token` and absence of any `ghs_` / `ghp_` / `gho_` / `ghu_` / `ghr_` / `github_pat_` prefix.
- [ ] A unit test verifies that when the App installation does not grant access to the requested repo (simulated 403 from GitHub on clone), the diagnostic message names the repo AND mentions installation scope, AND does NOT contain the literal string `"set GH_TOKEN and re-trigger"` — evidence: test assertion on returned `agentlib.Result.Message`.
- [ ] A unit test verifies that with no App credentials and no fallback configured, `resolveAuth` (or its equivalent) returns the existing "neither App nor PAT configured" error and no clone is attempted — evidence: test exit / returned error string.
- [ ] `make precommit` exits 0 in `agent/pr-reviewer/` — evidence: exit code.
- [ ] After deploying the fix to the dev pod, a manual trigger of `bborbe/trading#133` via the admin endpoint (`/admin/maintainer-watcher-github-pr/trigger?url=https://github.com/bborbe/trading/pull/133`) results in: (a) HTTP 200 with a task_id, (b) the Job pod reaching `kubectl get pod ... -o jsonpath='{.status.phase}' = Succeeded` within 10 minutes (NOT `Failed` / `BackoffLimitExceeded`), (c) a review comment appearing on the PR by the `ben-s-pull-request-reviewer` bot account, (d) the resulting task file in the vault transitioning to `phase: ai_review`. Evidence: PR comment URL + task file `phase` field + Job pod status.
- [ ] After the same manual trigger, `kubectlquant -n dev logs <pod>` does NOT show any string matching `ghs_[A-Za-z0-9_]{20,}` or `ghp_[A-Za-z0-9_]{20,}` at default verbosity — evidence: grep returns 0 matches.
- [ ] After the same manual trigger, `kubectlquant -n dev exec <pod> -- cat /data/repos/github.com/bborbe/trading.git/config` (run while the pod still exists, if a debug-shell sidecar is feasible) shows no token — evidence: grep returns 0 matches. If the pod is gone by the time the verifier looks, this AC is satisfied by the equivalent unit test asserting the same property.

**Scenario coverage:** NO new dark-factory scenario. The behavior is reachable by (a) unit tests against the clone path with a stubbed git transport / temporary local repo, and (b) the existing manual canary trigger against `bborbe/trading#133` (operator-level verification, not E2E test infrastructure). A scenario test would require a real private GitHub repo and a real App installation in CI, which is disproportionate for a single failure mode covered by the manual canary.

## Verification

```
cd ~/Documents/workspaces/maintainer/agent/pr-reviewer && make precommit
```

Behavioral verification (manual, post-deploy to dev):

1. Confirm the agent's dev pod is running the new image: `kubectlquant -n dev describe pod -l app=maintainer-agent-pr-reviewer | grep Image:` shows the post-fix tag.
2. Trigger the canary: `curl -s 'https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger?url=https://github.com/bborbe/trading/pull/133'` — expect HTTP 200 with a JSON body containing `task_id`.
3. Within 10 minutes, the Job pod for that task_id reaches `Succeeded` (not `Failed` / backoff-exhausted): `kubectlquant -n dev get jobs -l task_id=<task_id>` shows `COMPLETIONS 1/1`.
4. The PR shows a new review comment from the `ben-s-pull-request-reviewer` bot: visible at `https://github.com/bborbe/trading/pull/133`.
5. The corresponding task file in `~/Documents/Obsidian/OpenClaw/tasks/PR Review github - bborbe-trading - 133 - <task_id>*.md` has frontmatter `phase: ai_review` and contains a `## Review` section with the bot's verdict + summary.
6. `kubectlquant -n dev logs <pod>` greps clean for any token-shaped string: `kubectlquant -n dev logs <pod> | grep -E 'gh[sopur]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}'` returns nothing.

## Do-Nothing Option

Unacceptable. Private-repo PR reviews are the primary daily-use case for this agent — `bborbe/trading` alone produces multiple PRs per day. The do-nothing fallback would be to re-introduce a long-lived PAT, which inverts the 2026-05-24 retirement work and re-introduces a credential the operator must rotate manually. Doing nothing keeps the canary task stuck in `human_review` indefinitely and silently breaks every future private repo the App installation grants access to.
