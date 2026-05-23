---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-21T20:16:14Z"
generating: "2026-05-21T20:30:30Z"
prompted: "2026-05-21T20:49:03Z"
verifying: "2026-05-21T21:38:29Z"
branch: dark-factory/spec-033
---

## Summary

- Migrate the `agent/pr-reviewer` binary from PAT-on-user authentication (`pr-review-of-ben`) to GitHub App authentication so PR reviews stop being silently filtered by GitHub Trust & Safety.
- Introduce a new shared package `lib/githubapp` that mints + caches installation access tokens (IATs) via `github.com/bradleyfalzon/ghinstallation/v2`. Exposes an `http.RoundTripper` / `http.Client` factory plus a one-shot "mint at startup" helper for components (Claude CLI subprocess, `gh auth setup-git`) that need a plain bearer string in `GH_TOKEN`.
- Two Apps already exist and have been smoke-tested end-to-end (Phase A + B, commit `c1a105d`, `cmd/mint-iat`): prod `Ben's Pull Request Reviewer` (App ID `3798945`, Install `134414316`, scope all bborbe/* repos) and dev `Ben's Pull Request Reviewer Dev` (App ID `3800041`, Install `134435225`, scope `bborbe/go-skeleton` only). PEMs live in Teamvault (`kLoejw` prod, `eqKj8L` dev).
- Wire the new lib through `agent/pr-reviewer/main.go`: pod startup mints an IAT and threads it through the existing `GHToken` field so `githubposter`, `githubauth`, and the Claude subprocess all keep working unchanged. The legacy `GH_TOKEN` env stays accepted as a fallback during transition.
- Rolls out via rungs: lib bootstrap → main wiring → dev cluster deploy with real PR → prod cluster deploy with real PR → retire `pr-review-of-ben` PAT. PR Watcher and Build Watcher migrations are out of scope (separate specs in goal F1, sub-tasks F2 / F3).

## Problem

The PR reviewer agent authenticates as the GitHub user `pr-review-of-ben` via a Personal Access Token. GitHub Trust & Safety flagged that user account as "Spammy", and as a consequence the REST `GET /pulls/N/reviews` endpoint silently filters out every review the bot posts — they appear in the GitHub UI but are invisible to API consumers. Auto-merge gates and external automation that depend on the REST API can no longer see the agent's verdicts, defeating the purpose of the reviewer. Mitigation by appealing the flag is not durable: the same classifier can re-flag the user at any time, and the underlying signal (a user account whose only activity is rapid PR reviews) genuinely looks bot-like to GitHub.

GitHub Apps are first-class identities, not user accounts. They are not subject to user-level spam classification and their reviews remain visible via the REST API. Two Apps have already been registered, installed, permissioned, and smoke-tested end-to-end against `bborbe/maintainer#2` (review id `4340102652` visible via `GET /reviews` as `ben-s-pull-request-reviewer[bot]`) — see commit `c1a105d` and `agent/pr-reviewer/cmd/mint-iat`. The remaining work is to wire the App authentication into the production agent binary, deploy, verify, and retire the PAT user.

## Goal

After this work, the PR reviewer agent authenticates to GitHub as a GitHub App in both dev and prod clusters. Reviews posted by the agent are visible via the REST `GET /pulls/N/reviews` endpoint as `ben-s-pull-request-reviewer[bot]` (prod) or `ben-s-pull-request-reviewer-dev[bot]` (dev). No code path in the agent depends on the `pr-review-of-ben` user identity. A new shared package `lib/githubapp` provides the IAT-minting machinery so the same code can be reused by the PR Watcher and Build Watcher migrations that follow in sibling specs.

## Non-goals

- Do NOT migrate the PR Watcher (`watcher/github-pr`). Tracked separately as F2 `[[Migrate PR Watcher from User PAT to GitHub App]]`.
- Do NOT migrate the Build Watcher (`watcher/github-build`). Tracked separately as F3 `[[Migrate Build Watcher from User PAT to GitHub App]]`.
- Do NOT remove the legacy `GH_TOKEN` env. It stays accepted as a fallback so a malformed App config does not brick the pod during transition; cleanup is a follow-up spec.
- Do NOT remove the `cmd/mint-iat` smoke-test tool. It remains useful for ongoing credential verification and PEM rotation drills.
- Passive observation of whether App `APPROVE` reviews count toward GitHub's numeric required-approvals branch-protection rules is in scope (record the dev-cluster finding in `docs/github-app-setup.md`); active experiments — creating new branch-protection rules, modifying existing ones, or building workarounds — are out of scope. AC `Rung-3 #8` is operator-attested observation only and passes regardless of finding direction.
- Do NOT change Kafka topics, frontmatter contracts, prompt content, or any review-rendering logic. Auth identity changes only.
- Do NOT add wildcard / multi-installation support. One App, one installation, one PEM per cluster.

## Desired Behavior

1. The maintainer repo contains a new package `lib/githubapp` inside the existing `lib/` Go module. It exposes a factory that, given an App ID, Installation ID, and PEM (string or file path), returns an `http.Client` whose `RoundTripper` injects a fresh IAT into every outgoing GitHub API request and caches/refreshes that IAT transparently.
2. The same package exposes a one-shot "mint IAT now" helper that returns a plain token string (`ghs_...`) for use as a `GH_TOKEN` value in subprocess env and `gh auth setup-git`. Pod startup calls this once.
3. The PR reviewer pod, at startup, reads new env vars (App ID, Installation ID, PEM source) and mints an IAT. The minted IAT is placed into the existing `a.GHToken` field so downstream code (`githubposter`, `githubauth`, `factory.RunAgent`, Claude subprocess `GH_TOKEN`) requires no behavioral change beyond accepting a `ghs_...` token in place of a `ghp_...` PAT.
4. If the App-auth env vars are unset AND the legacy `GH_TOKEN` env is set, the pod uses the legacy `GH_TOKEN` exactly as before (PAT fallback). This is the transition path; it logs a warning at startup.
5. If both App-auth env vars and legacy `GH_TOKEN` are set, App auth wins; the legacy token is ignored and a warning is logged.
6. If neither is set, the pod refuses to start with a clear error message naming both options.
7. The bot login (`ben-s-pull-request-reviewer[bot]` prod, `ben-s-pull-request-reviewer-dev[bot]` dev) is configurable via env, not hard-coded. The existing `botLogin` value flowing into `githubposter.NewPrPoster` is sourced from this env, defaulting to the prod App slug.
8. The identity-check in `githubposter.checkBotIdentity` is reworked: `GET /user` does not work for Apps. Replace with `GET /app` (returns the App's identity) when running under App auth, or drop the check entirely if the IAT itself is considered sufficient identity proof. The decision is captured in the spec verification; either approach satisfies the desired behavior so long as no `pr-review-of-ben` literal remains and no pod crashloops on the new auth.
9. The `pkg/steps_gh_token.go` preflight continues to call `GET /rate_limit` and remains valid for both PATs and IATs (both authenticate the same way to that endpoint). User-facing error messages are reworded so they no longer mention "rotate teamvault entry" as if the token were a PAT — they mention "App credentials" / "IAT minting" in the App-auth case.
10. Two Kubernetes Secrets are created (dev cluster + prod cluster), each containing its App's PEM. Secrets are mounted as files into the pr-reviewer pod; env vars point at the mounted path.
11. The Obsidian runbook at `65 Runbooks/GitHub - Manage Bot Accounts.md` is updated to document App-auth as the active identity for the PR reviewer and to describe PEM rotation in terms of the procedure already drafted in `agent/pr-reviewer/docs/github-app-setup.md`.

## Constraints

- The two App identities are fixed: prod App ID `3798945`, prod Installation ID `134414316`, prod PEM Teamvault `kLoejw`; dev App ID `3800041`, dev Installation ID `134435225`, dev PEM Teamvault `eqKj8L`. These are frozen interfaces — do not register new Apps.
- The new module path stays `github.com/bborbe/maintainer/lib`, package `githubapp`. Mirrors the precedent of `lib/repoallowlist` (spec 028).
- IAT minting MUST go through `github.com/bradleyfalzon/ghinstallation/v2`. Do not reimplement JWT signing in production code. The stdlib-only implementation in `cmd/mint-iat` stays as a smoke-test reference but does not move into `lib/githubapp`.
- Errors are constructed and wrapped exclusively with `github.com/bborbe/errors`. No `fmt.Errorf`. No stdlib `errors.New`.
- Logging uses `glog`.
- BSD-style license headers on every new `.go` file, matching the rest of the maintainer repo.
- `CHANGELOG.md` entry under `## Unreleased`. The autoRelease tag is created by dark-factory at spec-complete time.
- PEMs MUST NOT enter git. Only Teamvault and Kubernetes Secrets. The Secret YAML committed to the repo MUST use the existing teamvault-resolved-at-deploy pattern (see other Secrets in `k8s/`), never plaintext.
- Deploy ordering: dev cluster first, soak ≥1 day with at least one real PR cycle, then prod. Reversing the order would expose prod to any latent bug.
- See `agent/pr-reviewer/docs/github-app-setup.md` for the App identity table, auth flow, permissions list, and PEM rotation procedure. That document is the canonical reference for the credentials this spec consumes.
- See `docs/verifying-specs.md` for the rung model.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| App-auth env vars unset, legacy `GH_TOKEN` set | Pod boots in PAT fallback mode; warning logged at startup. | None — intentional transition path. | `glog` warning line `pr-reviewer auth mode=pat-fallback` visible in pod logs. |
| App-auth env vars unset AND legacy `GH_TOKEN` unset | Pod refuses to start; clear error names both env-var sets. | Operator sets one of the two auth modes. | Pod CrashLoopBackOff; error line `pr-reviewer auth: neither App nor PAT configured` in `kubectl logs`. |
| PEM file path set but file missing or unreadable | Pod refuses to start with a wrapped error naming the path. | Operator fixes the Secret mount or the env path. | Pod CrashLoopBackOff; error mentions PEM path. |
| PEM file present but malformed (not a valid RSA private key) | `ghinstallation/v2` returns a parse error at first IAT mint; pod refuses to start. | Operator rotates the PEM in Teamvault and the Secret. | Pod CrashLoopBackOff; error mentions PEM parse. |
| App ID or Installation ID wrong | First IAT mint returns 404 from GitHub; pod refuses to start. | Operator corrects the env vars. | Pod CrashLoopBackOff; error contains HTTP 404 and GitHub response body. |
| IAT minting succeeds at startup but the cached IAT expires mid-pod-lifetime | `ghinstallation/v2` transparently refreshes the IAT on the next request. No agent-visible behavior change. | None — handled by the library. | No alarm; metrics in `lib/githubapp` (if added) show refresh count. |
| GitHub returns 401 for the IAT on a single request mid-run | `pkg/steps_gh_token.go` returns `needsInput` with reworded message naming App credentials. Task escalates to human review. | Operator regenerates the PEM via the App settings page, updates Teamvault and the Secret, restarts pods. | Vault task moves to `needs_input`; phase `human_review`. |
| `GET /user` is called on a code path that wasn't reworked | GitHub returns 403 / 404 for an App-auth IAT. Step fails. | Bug — rework the call to `GET /app` or drop it. | Pod logs an error naming `GET /user`; verification step in this spec catches it. |
| GitHub Trust & Safety flags the App itself (hypothetical) | Reviews would again become invisible. | Out of scope — fall back to a different App identity. | Manual: operator notices missing reviews in the REST API. |
| Two pr-reviewer pods run concurrently and both call GitHub | Each pod obtains its own IAT independently; GitHub permits multiple live IATs per Installation; no rate-limit collision observed under nominal review load. | None — supported by the Installation model. | No alarm; review POSTs succeed from both pods. |
| Secret rotated but pods not restarted | Pods keep using the old PEM from the mounted file (Kubernetes Secrets remount on restart only) until restart. Old PEM is still valid until revoked on the App settings page. | Operator restarts pods after rotating; revokes old PEM only after restart confirmation. | Procedure documented in `docs/github-app-setup.md` PEM Rotation section; runbook update is part of DoD. |

## Security / Abuse Cases

- The IAT (`ghs_...`) is a bearer token with the App's full permission set. It is forwarded to the Claude CLI subprocess via `GH_TOKEN` env — same threat model as today's PAT.
- The PEM is the long-lived secret. It NEVER enters git, NEVER appears in logs (the existing `githubauth` package already suppresses subprocess output for this reason; the new `lib/githubapp` MUST do the same — no logging of PEM bytes, no logging of IAT bytes beyond a prefix).
- App permissions are minimized per the table in `docs/github-app-setup.md`: Contents Read, Pull requests Write, Metadata Read. No org, account, or admin scopes. A compromised IAT can read repo contents and write PR reviews/comments — bounded to the installed repos (all `bborbe/*` for prod, `bborbe/go-skeleton` only for dev).
- The dev App is scoped to a single repo so a dev cluster compromise cannot reach the rest of the bborbe org. This is a constraint, not a suggestion — the dev App's installation MUST stay narrow.
- The bot-login string is now configurable via env. An attacker who can write the env could spoof identity checks, but they would already have full pod control — no new attack surface.

## Acceptance Criteria

Rung 1 — `lib/githubapp` package

- [ ] A new package `lib/githubapp` exists in the `lib/` Go module — evidence: `ls lib/githubapp/*.go` returns ≥1 file; `grep -n 'package githubapp' lib/githubapp/*.go` matches.
- [ ] The package exposes a factory returning an `http.Client` whose transport injects a cached IAT for GitHub API requests — evidence: `grep -n 'func New' lib/githubapp/*.go` shows the exported constructor; ginkgo test covers a happy-path round-trip against a `httptest.Server` simulating the GitHub `app/installations/.../access_tokens` endpoint.
- [ ] The package exposes a one-shot "mint IAT now" helper returning a `ghs_...` string — evidence: `grep -n 'func MintIAT\|func MintInstallationToken' lib/githubapp/*.go` matches; ginkgo test covers a happy-path mint against a `httptest.Server`.
- [ ] Production code uses `github.com/bradleyfalzon/ghinstallation/v2`; the dependency appears in `lib/go.mod` — evidence: `grep ghinstallation lib/go.mod` matches.
- [ ] No PEM bytes or IAT bytes longer than a prefix appear in any `glog` call in `lib/githubapp` — evidence: `grep -n 'glog\.' lib/githubapp/*.go | grep -i 'pem\|ghs_'` returns zero matches.
- [ ] Package coverage ≥ 80% — evidence: `cd lib && go test -cover ./githubapp/...` reports `coverage: >= 80.0% of statements`.
- [ ] All `.go` files carry the BSD-style license header — evidence: `grep -L 'BSD-style' lib/githubapp/*.go` returns empty.
- [ ] All errors use `github.com/bborbe/errors`; no `fmt.Errorf` / stdlib `errors.New` in the new package — evidence: `grep -E 'fmt\.Errorf|errors\.New' lib/githubapp/*.go` returns empty.
- [ ] `cd lib && make precommit` exits 0 — evidence: exit code 0; output line `ready to commit`.

Rung 2 — `agent/pr-reviewer` main wiring

- [ ] `agent/pr-reviewer/main.go` accepts new env vars for App ID, Installation ID, PEM path / PEM content, and Bot login — evidence: `grep -n 'APP_ID\|INSTALLATION_ID\|PEM' agent/pr-reviewer/main.go` matches; `--help` output of the binary lists the new flags.
- [ ] At pod startup, if App-auth env is configured, the agent mints an IAT via `lib/githubapp` and assigns it to `a.GHToken` BEFORE any downstream consumer reads that field — evidence: unit test or integration test asserts ordering; `glog` line `pr-reviewer auth mode=github-app app_id=<id> installation_id=<id>` appears in pod logs.
- [ ] If App-auth env is unset and `GH_TOKEN` is set, the agent logs `pr-reviewer auth mode=pat-fallback` and proceeds — evidence: ginkgo test asserts the log line.
- [ ] If neither is set, the agent refuses to start with a wrapped error naming both — evidence: ginkgo test asserts non-nil error; error message contains both `APP_ID` and `GH_TOKEN` literals.
- [ ] `pkg/githubposter::checkBotIdentity` no longer calls `GET /user` unconditionally — evidence: `grep -n 'api.github.com/user"' agent/pr-reviewer/pkg/githubposter/*.go` returns zero matches (either replaced with `GET /app` or the check is removed entirely; both are acceptable per Desired Behavior #8).
- [ ] The hard-coded `pr-review-of-ben` literal no longer appears anywhere under `agent/pr-reviewer/` — evidence: `grep -rn 'pr-review-of-ben' agent/pr-reviewer/` returns zero matches.
- [ ] The `botLogin` value flowing into `githubposter.NewPrPoster` is sourced from env, with a default of `ben-s-pull-request-reviewer[bot]` — evidence: `grep -n 'BOT_LOGIN' agent/pr-reviewer/main.go` matches; default-value test asserts the prod App slug.
- [ ] `pkg/steps_gh_token.go` error messages no longer say "rotate teamvault entry" in PAT-only language; when in App-auth mode they reference App credentials — evidence: `grep -n 'App credentials\|App auth\|rotate the PEM' agent/pr-reviewer/pkg/steps_gh_token.go` matches at least one line.
- [ ] `cd agent/pr-reviewer && make precommit` exits 0 — evidence: exit code 0; output line `ready to commit`.
- [ ] `CHANGELOG.md` has a new entry under `## Unreleased` describing the App-auth migration — evidence: `grep -A2 '## Unreleased' CHANGELOG.md` shows the new entry.

Rung 3 — dev cluster deploy

- [ ] A Kubernetes Secret holding the dev App PEM exists in the dev cluster's `dev` namespace — evidence: `kubectlquant -n dev get secret pr-reviewer-github-app` returns a Secret with key `pem`.
- [ ] The pr-reviewer StatefulSet in dev mounts the Secret as a file and sets the new env vars — evidence: `kubectlquant -n dev get statefulset agent-pr-reviewer -o yaml` shows the volume mount + env vars.
- [ ] The pr-reviewer pod in dev rolls out cleanly to the new image with App-auth env set — evidence: `kubectlquant -n dev rollout status statefulset/agent-pr-reviewer --timeout=120s` reports complete.
- [ ] Pod logs show `pr-reviewer auth mode=github-app app_id=3800041 installation_id=134435225` at startup — evidence: `kubectlquant -n dev logs statefulset/agent-pr-reviewer | grep 'auth mode='` matches.
- [ ] A real PR opened on `bborbe/go-skeleton` triggers a watcher task that the agent picks up and runs to verdict — evidence: per-SHA task file in `~/Documents/Obsidian/OpenClaw/tasks/` reaches status `completed`.
- [ ] The agent's review appears in the GitHub UI on that PR — evidence: visible on `https://github.com/bborbe/go-skeleton/pull/<N>`.
- [ ] The same review is visible via the REST API as `ben-s-pull-request-reviewer-dev[bot]` — evidence: `curl -s https://api.github.com/repos/bborbe/go-skeleton/pulls/<N>/reviews | jq '.[].user.login'` returns at least one entry equal to `ben-s-pull-request-reviewer-dev[bot]`.
- [ ] Whether the App's APPROVE counts toward branch-protection required-approvals rules is observed and documented in `docs/github-app-setup.md` — evidence: a new "Required Approvals Behavior" section in that file, with the dev-cluster observation recorded (this is a discovery, not a gating constraint).

Rung 4 — prod cluster deploy

- [ ] After dev soaks ≥1 day with at least one successful PR cycle, the prod cluster rolls out with the prod App PEM — evidence: `kubectlquant -n prod rollout status statefulset/agent-pr-reviewer --timeout=120s` reports complete.
- [ ] Pod logs show `pr-reviewer auth mode=github-app app_id=3798945 installation_id=134414316` at startup — evidence: `kubectlquant -n prod logs statefulset/agent-pr-reviewer | grep 'auth mode='` matches.
- [ ] A real PR opened on any `bborbe/*` repo serviced by prod triggers a successful end-to-end review cycle — evidence: per-SHA task file in `~/Documents/Obsidian/OpenClaw/tasks/` reaches status `completed`.
- [ ] The agent's review is visible via the REST API as `ben-s-pull-request-reviewer[bot]` — evidence: `curl -s https://api.github.com/repos/bborbe/<repo>/pulls/<N>/reviews | jq '.[].user.login'` returns at least one entry equal to `ben-s-pull-request-reviewer[bot]`.

Rung 5 — PAT retirement

- [ ] After ≥3 prod PR cycles complete cleanly under App auth, the `pr-review-of-ben` PAT is revoked at `https://github.com/settings/tokens` — evidence: a manual confirmation note added to `docs/github-app-setup.md` Migration Status section with date and operator initials.
- [ ] The Obsidian runbook `65 Runbooks/GitHub - Manage Bot Accounts.md` is updated to mark `pr-review-of-ben` as retired and to document App auth as the active identity — evidence: `grep -n 'retired\|App auth' "~/Documents/Obsidian/Personal/65 Runbooks/GitHub - Manage Bot Accounts.md"` matches the updates.

Scenario coverage — NO new scenario. The new `lib/githubapp` package is covered by ginkgo unit tests against a `httptest.Server` (real GitHub network calls would couple the test suite to live App credentials, which is unacceptable). The end-to-end behavior is covered by Rung-3 and Rung-4 live cluster verification, which an in-repo scenario could not faithfully reproduce (real GitHub App, real Installation, real PR, real Kafka, real controller, real PVC). Adding a scenario would duplicate either the unit tests or the manual cluster verification without catching anything new.

## Verification

Per `docs/verifying-specs.md`, this spec is **Rung-4** (touches a deployed service AND introduces a new Kubernetes Secret AND requires dev-then-prod ordering). Execute in order:

**Rung 1 — precommit (host):**

```
cd lib && make precommit
```

**Rung 2 — precommit (host):**

```
cd agent/pr-reviewer && make precommit
```

**Rung 3 — dev cluster:**

```
# Apply Secret (dev cluster, dev namespace) — Secret YAML must use the existing
# teamvault-resolved-at-deploy pattern; PEM source is Teamvault entry eqKj8L.
# (Exact apply mechanics are inherited from k8s/ patterns; consult an existing
# Secret manifest in k8s/ for the resolved-at-deploy template.)

# Deploy new image
cd ~/Documents/workspaces/maintainer-dev
git pull && git merge master --no-edit && git push
cd agent/pr-reviewer && BRANCH=dev make build upload
cd k8s              && BRANCH=dev make buca

kubectlquant -n dev rollout status statefulset/agent-pr-reviewer --timeout=120s
kubectlquant -n dev logs statefulset/agent-pr-reviewer | grep 'auth mode='
```

Then open a trivial PR in `bborbe/go-skeleton`, wait for the watcher → controller → agent pipeline, then verify the review appears via the REST API:

```
curl -s https://api.github.com/repos/bborbe/go-skeleton/pulls/<N>/reviews \
  | jq '.[] | {user: .user.login, state: .state, commit_id: .commit_id}'
```

Expected: at least one entry with `user == "ben-s-pull-request-reviewer-dev[bot]"`.

**Rung 4 — prod cluster:**

After dev soaks ≥1 day with at least one clean PR cycle, repeat the same sequence against `~/Documents/workspaces/maintainer-prod` with `BRANCH=prod` and `kubectlquant -n prod`. Smoke-test with a PR opened in any `bborbe/*` repo serviced by prod. Confirm via REST API that the review's `user.login` is `ben-s-pull-request-reviewer[bot]`.

**Rung 5 — PAT retirement:**

After ≥3 clean prod PR cycles, revoke the `pr-review-of-ben` PAT at <https://github.com/settings/tokens>. Update `docs/github-app-setup.md` Migration Status section. Update `65 Runbooks/GitHub - Manage Bot Accounts.md`.

## Do-Nothing Option

If we don't ship this, the PR reviewer agent continues to post reviews that are invisible to the REST API. Any auto-merge gate, external CI check, or third-party automation that reads `GET /pulls/N/reviews` sees no reviews at all — defeating the purpose of the agent. Appealing the spam flag on `pr-review-of-ben` is non-durable: the same heuristic will re-flag the user the moment review activity resumes. The Apps are already registered, installed, permissioned, and smoke-tested end-to-end (commit `c1a105d`); the remaining cost is wiring the existing infrastructure into the production binary. Not doing this work strands the smoke-test investment and leaves the reviewer in a degraded state indefinitely. Not acceptable.

---

**Related vault notes:**

- Task: `[[Migrate PR Reviewer from User PAT to GitHub App]]`
- Goal: `[[GitHub Code Reviewer Agent - Base]]` (F1)
- Sibling tasks (future specs): `[[Migrate PR Watcher from User PAT to GitHub App]]` (F2), `[[Migrate Build Watcher from User PAT to GitHub App]]` (F3)
- Reference doc: `agent/pr-reviewer/docs/github-app-setup.md`
- Smoke-test foundation: commit `c1a105d` (`cmd/mint-iat`, Phase A + B verified 2026-05-21)
