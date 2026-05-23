---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-23T21:10:53Z"
generating: "2026-05-23T21:11:21Z"
prompted: "2026-05-23T21:15:06Z"
verifying: "2026-05-23T22:19:17Z"
branch: dark-factory/migrate-watcher-github-build-to-github-app
---

## Summary

- Migrate the `watcher/github-build` binary from PAT authentication (`pr-review-of-ben`, Teamvault `ROnG5L`) to GitHub App authentication, reusing the App identities and `lib/githubapp` package already shipped by spec 033 for the pr-reviewer agent.
- At startup, when App env vars are set, the watcher constructs an `*http.Client` via `lib/githubapp.NewClient` (auto-refreshing transport, no manual 1h expiry to manage). The client is threaded into `pkg.NewGitHubClient` whose constructor changes from `(token string)` to `(httpClient *http.Client)`. The `go-github` library accepts either form; the new shape removes the `WithAuthToken` call inside the constructor.
- **Why `NewClient` not `MintIAT`**: `MintIAT` returns a static string token that expires in 1 hour; the watcher is a long-lived StatefulSet, so a static IAT would silently break the poll cycle every hour until pod restart. `NewClient` returns a client whose RoundTripper transparently re-mints the IAT before expiry (per `lib/githubapp/githubapp.go` line 39: "refreshes the IAT every ~50 minutes"). This differs from the pr-reviewer agent, which is a one-shot Pattern B Job (<5 min runtime) and correctly uses `MintIAT`.
- Hybrid auth resolution mirrors pr-reviewer: App env wins when configured, legacy `GH_TOKEN` is accepted as a fallback (transition path), startup fails loudly only when both are unset.
- Reuses the existing Apps — prod `Ben's Pull Request Reviewer` (ID `3798945`, Install `134414316`, PEM Teamvault `kLoejw`) and dev `Ben's Pull Request Reviewer Dev` (ID `3800041`, Install `134435225`, PEM Teamvault `eqKj8L`). Operator must bump permission `Actions: Read` on BOTH Apps via the GitHub UI and accept the permission change on each Installation before deploy.
- Rolls out via rungs: main wiring → dev cluster deploy with a forced red build on `bborbe/go-skeleton` → prod cluster deploy + 24h soak. PAT retirement is explicitly deferred until the sibling `watcher/github-pr` migration also ships.

## Problem

The `watcher/github-build` pod authenticates to GitHub with the shared user PAT `pr-review-of-ben` (Teamvault `ROnG5L`). The pr-reviewer agent migrated off this PAT in spec 033; the two watchers (`github-build` and `github-pr`) are the last in-repo consumers blocking PAT retirement. As long as either watcher still depends on the user token, GitHub Trust & Safety can re-flag the account and break the watcher (workflow-run polling would start returning empty results or 401s with no clear signal), AND the PAT cannot be revoked, which leaves an over-scoped user credential in Teamvault indefinitely. Migrating the watcher to an App identity removes both risks in one step. `lib/githubapp` is already shipped (`NewClient(ctx, cfg) (*http.Client, error)` returns an auto-refreshing client suitable for long-lived pollers); the build watcher's `pkg.NewGitHubClient` constructor is the one API-surface change required (`token string` → `httpClient *http.Client`), driven by the watcher's StatefulSet lifecycle.

## Goal

After this work, the `watcher/github-build` pod authenticates to GitHub as a GitHub App in both dev and prod clusters. Workflow-run polling, job-listing, log-fetching, and repo content reads (`GetWorkflowRuns`, `GetJobsForRun`, `GetJobLog`, `GetDefaultBranch`, `GetFileContent`) all succeed under App auth against the existing repo allowlist. Pod startup logs the active auth mode (`auth mode=github-app app_id=<id> installation_id=<id>` or `auth mode=pat-fallback`). The legacy `GH_TOKEN` env continues to be accepted as a fallback so a malformed App config does not brick the watcher.

## Non-goals

- Do NOT revoke the `pr-review-of-ben` PAT (Teamvault `ROnG5L`). Deferred until BOTH `watcher/github-build` AND `watcher/github-pr` have migrated and soaked cleanly. PAT retirement is a separate spec authored after both watcher migrations complete.
- Do NOT drop the `GH_TOKEN` env from the watcher binary or its Secret in this spec — it stays accepted as a fallback. A follow-up cleanup spec removes it after PAT retirement.
- Do NOT migrate `watcher/github-pr`. Tracked as a sibling spec authored in parallel; both share the same App permission bump and both must land before PAT retirement.
- Do NOT register new Apps. Reuse the existing pr-reviewer Apps; one App per cluster covers all three pods (pr-reviewer agent, github-build watcher, github-pr watcher).
- Do NOT change the watcher's polling cadence, repo allowlist semantics, cursor file format, Kafka topic, frontmatter contract, or task-creation logic. Auth identity changes only.
- Do NOT add wildcard / multi-installation support. One App, one installation, one PEM per cluster — same model as spec 033.
- Do NOT add a tunable "disable App auth" flag. The hybrid resolver (App-when-configured, PAT-when-not, error-when-neither) IS the opt-out path; an additional toggle would just be an escape hatch on the Goal.
- Do NOT add per-watcher Apps. Single-App reuse is the explicit choice; can split later if a concrete consumer demands it, which today no one does.

## Desired Behavior

1. The watcher binary accepts four new env vars: `APP_ID` (int64), `INSTALLATION_ID` (int64), `PEM_KEY_FILE` (path to mounted PEM file) and `PEM_KEY` (PEM content as env var, mutually exclusive with `PEM_KEY_FILE`). All four are `required:"false"`.
2. The existing `GH_TOKEN` env var is downgraded to `required:"false"`. Pod startup determines the active auth mode and fails only when both App env and `GH_TOKEN` are absent.
3. When `APP_ID` and `INSTALLATION_ID` are set and at least one of `PEM_KEY_FILE` / `PEM_KEY` is set, the pod constructs an `*http.Client` via `lib/githubapp.NewClient` at startup and threads it (not a static token string) into `pkg.NewGitHubClient`. The client's transport mints and refreshes IATs transparently for the pod's lifetime.
4. When App env is unset but `GH_TOKEN` is set, the pod constructs the GitHub client using the PAT exactly as today (via a default `*http.Client` with a static-Bearer transport wrapping `GH_TOKEN`) and emits a single warning log line indicating PAT-fallback mode.
5. When both are set, App auth wins; `GH_TOKEN` is ignored and a warning is logged naming the conflict.
6. When neither is set, the pod refuses to start with a wrapped error that names both env-var sets.
7. The watcher's polling and content-fetching logic is byte-identical to today. `pkg.NewGitHubClient`'s constructor signature changes from `(token string)` to `(httpClient *http.Client)` and the body changes from `gogithub.NewClient(nil).WithAuthToken(token)` to `gogithub.NewClient(httpClient)`. All method signatures on the resulting `*gitHubClient` and all call sites in `pkg/watcher.go` are unchanged. No changes to `factory.CreateWatcher`'s external contract (it now takes a `*http.Client` instead of a string internally), no changes to cursor persistence or Kafka publishing.
8. The Kubernetes Secret `maintainer-watcher-github-build` gains a new `PEM_KEY` data field resolved at deploy time from a Teamvault file entry (same `teamvaultFileBase64` template used by the pr-reviewer Secret). The existing `GH_TOKEN` data field stays in place.
9. The dev and prod `.env` files gain three new exports: `WATCHER_GITHUB_BUILD_APP_ID`, `WATCHER_GITHUB_BUILD_INSTALLATION_ID`, and `WATCHER_GITHUB_BUILD_PEM_KEY`. Values match the existing pr-reviewer settings (dev: `3800041` / `134435225` / `eqKj8L`; prod: `3798945` / `134414316` / `kLoejw`).
10. Pod startup logs a single line at glog V(2) identifying the active auth mode, mirroring the pr-reviewer format: `watcher/github-build auth mode=github-app app_id=<id> installation_id=<id>` or `watcher/github-build auth mode=pat-fallback`.

## Constraints

- The two App identities and their Installation IDs are frozen by spec 033. Do not register new Apps, do not change Installation IDs. (Prod: App `3798945`, Install `134414316`, PEM Teamvault `kLoejw`. Dev: App `3800041`, Install `134435225`, PEM Teamvault `eqKj8L`.)
- The four GitHub REST endpoints the build watcher hits — `Actions.ListRepositoryWorkflowRuns`, `Actions.ListWorkflowJobs`, `Actions.GetWorkflowJobLogs`, `Repositories.Get`, `Repositories.GetContents` — require App permissions `Actions: Read`, `Contents: Read`, `Metadata: Read`. The existing pr-reviewer Apps grant `Contents: Read` and `Metadata: Read` already; they do NOT yet grant `Actions: Read`. The operator MUST bump `Actions: Read` on both Apps via the GitHub UI and accept the permission change on each Installation before the new pods roll out. This is a prerequisite, not a code change.
- The pr-reviewer Apps' Installation scope (dev: `bborbe/go-skeleton` only; prod: all `bborbe/*`) is the upper bound for the build watcher's reachable repos. The watcher's `REPO_ALLOWLIST` MUST stay a subset; out-of-scope repos will return 404 on `Repositories.Get`. The dev cluster currently allows only `github.com/bborbe/go-skeleton`, which is in scope; the prod allowlist is already `bborbe/*`, which is in scope.
- App authentication MUST go through `lib/githubapp.NewClient(ctx, githubapp.Config)` (returns `*http.Client` with auto-refreshing transport). Do NOT use `lib/githubapp.MintIAT` — it produces a static 1-hour token unsuitable for a long-lived poller. Do not call `github.com/bradleyfalzon/ghinstallation/v2` directly from the watcher; the lib is the seam.
- Errors are constructed and wrapped exclusively with `github.com/bborbe/errors`. No `fmt.Errorf`. No stdlib `errors.New`.
- Logging uses `glog`.
- BSD-style license headers on every new `.go` file.
- `CHANGELOG.md` entry under `## Unreleased`.
- PEMs MUST NOT enter git. Teamvault and Kubernetes Secrets only. The Secret YAML uses the existing `teamvaultFileBase64` template pattern.
- Deploy ordering: dev cluster first, soak ≥1 real green→red transition, then prod with ≥24h soak. Reversing the order would expose prod to any latent bug.
- See spec `specs/in-progress/033-migrate-pr-reviewer-to-github-app.md` for the canonical App identity table, permissions list, and PEM rotation procedure. This spec consumes that work; do not duplicate the App registration details here.
- See `agent/pr-reviewer/docs/github-app-setup.md` for the App reference doc.
- See `docs/verifying-specs.md` for the rung model.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| App-auth env vars unset, legacy `GH_TOKEN` set | Pod boots in PAT fallback mode; warning logged at startup. | None — intentional transition path. | `glog` warning line `watcher/github-build auth mode=pat-fallback` in pod logs. |
| App-auth env vars unset AND legacy `GH_TOKEN` unset | Pod refuses to start; wrapped error names both env-var sets. | Operator sets one auth mode. | Pod CrashLoopBackOff; error line `watcher/github-build auth: neither App nor PAT configured` in `kubectl logs`. |
| `PEM_KEY_FILE` path set but file missing or unreadable | Pod refuses to start with a wrapped error naming the path. | Operator fixes the Secret mount or the env path. | Pod CrashLoopBackOff; error mentions PEM path. |
| `PEM_KEY` content set but malformed (not a valid RSA private key) | `lib/githubapp` returns a parse error at first IAT mint; pod refuses to start. | Operator rotates PEM in Teamvault and the Secret. | Pod CrashLoopBackOff; error mentions PEM parse. |
| App ID or Installation ID wrong | First IAT mint returns 404 from GitHub; pod refuses to start. | Operator corrects the env vars. | Pod CrashLoopBackOff; error contains HTTP 404 and GitHub response body. |
| **Operator deploys before bumping `Actions: Read` on the App** | First `Actions.ListRepositoryWorkflowRuns` call returns HTTP 403 with body `Resource not accessible by integration`. Poll cycle logs the error and continues; no tasks are produced for this poll. | Operator bumps permission on the App, accepts the change on the Installation, restarts pods. | Poll-cycle error logs contain `403` and `not accessible by integration`; no new build-fixer tasks observed despite known-red workflow runs. |
| Cached IAT expires mid-pod-lifetime | `lib/githubapp.NewClient` returns a client whose `ghinstallation/v2` transport refreshes the IAT every ~50 min automatically, before the 1-hour expiry. No application-level recovery needed. | None — transparent to the watcher. | A poll-cycle log containing 401 from GitHub would indicate the refresh failed (regression); negative evidence via Rung 3/4 ACs (`grep 401\|403` returns zero matches). |
| GitHub Trust & Safety flags the App itself (hypothetical) | Polling would return 401 / empty results. | Out of scope — fall back to a different App identity. | Manual: operator notices missing build-fixer tasks for a known-red workflow. |
| Pod runs concurrently with the pr-reviewer agent and the github-pr watcher under the same Installation | Each pod obtains its own IAT independently. GitHub allows multiple live IATs per Installation; nominal API load is well below per-installation rate limits (15k req/h for Apps). | None — supported by the Installation model. | No alarm; poll cycles complete without rate-limit errors. |
| Secret rotated but pods not restarted | Pods keep using the old PEM from the mounted file until pod restart (Kubernetes Secrets remount on pod restart only). | Operator restarts pods after rotating; revokes old PEM only after restart confirmation. | Documented in `docs/github-app-setup.md` PEM Rotation section. |
| `Repositories.GetContents` called for a repo outside the Installation scope | GitHub returns 404. Watcher's existing code maps 404 to `(nil, nil)` (file not found). The poll cycle proceeds as if `.github/maintainer.yaml` does not exist. | None — semantics match today. | None — behaves indistinguishably from the "file does not exist" branch. |

## Security / Abuse Cases

- The IAT (`ghs_...`) is a bearer token with the App's full permission set (`Actions: Read`, `Contents: Read`, `Metadata: Read`, plus whatever pr-reviewer needs — `Pull requests: Write`). The build watcher itself only exercises the read scopes. A compromised pod IAT is bounded to read repo contents/actions and post nothing (the watcher never writes). This is strictly less powerful than today's PAT, which carries the user's full repo scope.
- The PEM is the long-lived secret. NEVER enters git, NEVER appears in logs. `lib/githubapp` already enforces this; the watcher MUST NOT log PEM bytes or IAT bytes beyond a prefix.
- Installation scope bounds the blast radius: dev IAT can only touch `bborbe/go-skeleton`; prod IAT can touch all `bborbe/*` repos. No org-admin, no account scopes.
- The hybrid auth resolver does not validate that `PEM_KEY_FILE` points inside the Secret mount — an attacker who can write the env could point at an attacker-controlled file. They would already have full pod control; no new attack surface.

## Acceptance Criteria

### Rung 1 — main wiring (`watcher/github-build/main.go`)

- [ ] `watcher/github-build/main.go` declares the four new fields `AppID int64`, `InstallationID int64`, `PEMKeyFile string`, `PEMKey string` with `required:"false"` and the env names `APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`, `PEM_KEY` — evidence: `grep -nE 'APP_ID|INSTALLATION_ID|PEM_KEY_FILE|PEM_KEY' watcher/github-build/main.go` matches four lines; `./maintainer-watcher-github-build --help 2>&1 | grep -E 'app-id\|installation-id\|pem-key-file\|pem-key'` returns four matches.
- [ ] The existing `GHToken` field is `required:"false"` — evidence: `grep -n 'GHToken' watcher/github-build/main.go` shows `required:"false"`; previous `required:"true"` literal is gone.
- [ ] At pod startup, when App env is configured, the binary calls `lib/githubapp.NewClient` and threads the returned `*http.Client` into `factory.CreateWatcher` — evidence: `grep -n 'githubapp.NewClient' watcher/github-build/main.go` matches; `grep -n 'MintIAT' watcher/github-build/` returns zero matches; unit test asserts a non-nil `*http.Client` is threaded into `factory.CreateWatcher`; on a real start with App env set, glog line `watcher/github-build auth mode=github-app app_id=<id> installation_id=<id>` appears.
- [ ] When App env is unset and `GH_TOKEN` is set, startup logs `watcher/github-build auth mode=pat-fallback` and proceeds — evidence: ginkgo test asserts the log line; exit code 0 from a smoke run.
- [ ] When both App env and `GH_TOKEN` are set, App wins and a single warning log line names the conflict — evidence: ginkgo test asserts the warning string contains the literal `both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored` (matches the pr-reviewer wording at `agent/pr-reviewer/main.go` line ~241) and that the App-refreshing `*http.Client` is what reaches the factory.
- [ ] When neither is configured, the binary refuses to start with a wrapped error that contains both literals `APP_ID` and `GH_TOKEN` — evidence: ginkgo test asserts non-nil error; error message contains both literals.
- [ ] In PAT-fallback mode, the binary wraps `GH_TOKEN` in a static-Bearer `*http.Client` and threads it into `factory.CreateWatcher` — evidence: `grep -nE 'oauth2\.StaticTokenSource|Authorization.*Bearer' watcher/github-build/main.go` returns ≥1 match in the PAT branch; unit test asserts the captured outbound `Authorization` header equals `Bearer <GH_TOKEN>`.
- [ ] No literal `pr-review-of-ben` appears in `watcher/github-build/` — evidence: `grep -rn 'pr-review-of-ben' watcher/github-build/` returns zero matches.
- [ ] `cd watcher/github-build && make precommit` exits 0 — evidence: exit code 0; output line `ready to commit`.
- [ ] `CHANGELOG.md` has a new entry under `## Unreleased` describing the App-auth migration of the build watcher — evidence: `grep -A2 '## Unreleased' CHANGELOG.md` shows the new entry mentioning `watcher/github-build`.

### Rung 2 — Kubernetes Secret + env wiring

- [ ] `watcher/github-build/k8s/maintainer-watcher-github-build-secret.yaml` declares a `PEM_KEY` data field that resolves via `teamvaultFileBase64` against env `WATCHER_GITHUB_BUILD_PEM_KEY` — evidence: `grep -n 'PEM_KEY' watcher/github-build/k8s/maintainer-watcher-github-build-secret.yaml` matches the template line; format mirrors `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml` line 11.
- [ ] `dev.env` and `prod.env` each declare `WATCHER_GITHUB_BUILD_APP_ID`, `WATCHER_GITHUB_BUILD_INSTALLATION_ID`, and `WATCHER_GITHUB_BUILD_PEM_KEY` with the values: dev `3800041` / `134435225` / `eqKj8L`, prod `3798945` / `134414316` / `kLoejw` — evidence: `grep -nE 'WATCHER_GITHUB_BUILD_(APP_ID|INSTALLATION_ID|PEM_KEY)' dev.env prod.env` matches six lines with the correct values.
- [ ] The build watcher's StatefulSet / Deployment manifest passes the new env vars from the Secret and Configmap and mounts `PEM_KEY` content as a file (or as env, matching whatever pattern `agent/pr-reviewer` uses) — evidence: `grep -nE 'APP_ID|INSTALLATION_ID|PEM_KEY' watcher/github-build/k8s/*.yaml` matches the env wiring.

### Rung 3 — dev cluster deploy + e2e

- [ ] **Operator prerequisite — `Actions: Read` accepted on BOTH Apps' Installations** — evidence: operator-attested note in the spec verification log (timestamp + initials) confirming the permission was bumped and the Installation accepted on dev App `3800041` AND prod App `3798945`. Without this, Rung 3 #5 will fail with a 403.
- [ ] The dev cluster's `maintainer-watcher-github-build` Secret carries a `PEM_KEY` field after redeploy — evidence: `kubectlquant -n dev get secret maintainer-watcher-github-build -o json | jq '.data | keys'` includes `PEM_KEY`.
- [ ] The build watcher StatefulSet in dev rolls out cleanly to the new image with App-auth env set — evidence: `kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-build --timeout=120s` reports complete (or equivalent kind if Deployment).
- [ ] Pod logs show `watcher/github-build auth mode=github-app app_id=3800041 installation_id=134435225` at startup — evidence: `kubectlquant -n dev logs statefulset/maintainer-watcher-github-build | grep 'auth mode='` matches.
- [ ] A real commit pushed to `bborbe/go-skeleton`'s default branch that fails CI produces a build-fixer task in the configured task vault within ≤2 poll intervals — evidence: a new task file under the operator's vault root (today: `~/Documents/Obsidian/Personal/24 Tasks/`) with frontmatter `assignee: build-fixer-agent` and a body referencing the failing workflow run's URL; pod logs show the green→red transition and the published `CreateTaskCommand`.
- [ ] No 401 / 403 errors appear in pod logs during a 30-minute observation window after Rung 3 #5 succeeds — evidence: `kubectlquant -n dev logs statefulset/maintainer-watcher-github-build --since=30m | grep -E '401|403'` returns zero matches.

### Rung 4 — prod cluster deploy + 24h soak

- [ ] After dev soaks ≥1 real green→red transition and Rung 3 ACs pass, the prod cluster rolls out — evidence: `kubectlquant -n prod rollout status statefulset/maintainer-watcher-github-build --timeout=120s` reports complete.
- [ ] Pod logs show `watcher/github-build auth mode=github-app app_id=3798945 installation_id=134414316` at startup — evidence: `kubectlquant -n prod logs statefulset/maintainer-watcher-github-build | grep 'auth mode='` matches.
- [ ] During a 24h soak following prod rollout, at least one real green→red transition on a `bborbe/*` repo produces a build-fixer task with App auth in logs — evidence: pod logs over the soak window show ≥1 published `CreateTaskCommand` correlated with a `Actions.ListRepositoryWorkflowRuns` response containing a `failure` conclusion; the corresponding vault task exists.
- [ ] No 401 / 403 errors appear in prod pod logs during the 24h soak — evidence: `kubectlquant -n prod logs statefulset/maintainer-watcher-github-build --since=24h | grep -E '401|403'` returns zero matches.

### Scenario coverage — NO new scenario

The resolver logic is covered by ginkgo unit tests in `watcher/github-build/`. The end-to-end behavior is covered by Rung 3 (dev cluster, real PR-like commit, real GitHub, real Kafka) and Rung 4 (prod soak). A scenario test would have to fake either GitHub or the App credentials and would not catch the only interesting failure modes (permission bump not accepted, App not installed on the target repo). Live cluster verification is the right granularity.

## Verification

Per `docs/verifying-specs.md`, this spec is **Rung-4** (touches a deployed service AND modifies a Kubernetes Secret AND requires dev-then-prod ordering with a soak). Execute in order:

**Rung 1 — precommit (host):**

```
cd watcher/github-build && make precommit
```

**Rung 2 — manifest review (host):**

```
grep -nE 'PEM_KEY|APP_ID|INSTALLATION_ID' watcher/github-build/k8s/*.yaml
grep -nE 'WATCHER_GITHUB_BUILD_(APP_ID|INSTALLATION_ID|PEM_KEY)' dev.env prod.env
```

Expected: env wiring present in Secret + Deployment/StatefulSet manifests; six matching lines across `dev.env` + `prod.env`.

**Rung 3 — dev cluster:**

```
# Prereq (operator): bump `Actions: Read` on Apps 3798945 and 3800041 via GitHub UI;
# accept the permission change on each Installation. Record timestamp + initials in
# the spec verification log.

cd ~/Documents/workspaces/maintainer-dev
git pull && git merge master --no-edit && git push
cd watcher/github-build && BRANCH=dev make build upload
cd k8s                  && BRANCH=dev make buca

kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-build --timeout=120s
kubectlquant -n dev logs statefulset/maintainer-watcher-github-build | grep 'auth mode='
```

Then push a commit to `bborbe/go-skeleton`'s default branch that fails CI. Within two poll intervals (10 min default), confirm:

```
# A new task file appears in the operator's vault root
# (today: $VAULT_ROOT = ~/Documents/Obsidian/Personal)
ls -lt "$VAULT_ROOT/24 Tasks/" | head

# Pod logs show the green→red transition + CreateTaskCommand publish
kubectlquant -n dev logs statefulset/maintainer-watcher-github-build --since=15m \
  | grep -E 'green.*red|CreateTaskCommand|auth mode='
```

**Rung 4 — prod cluster:**

After dev soaks ≥1 real green→red transition, repeat the sequence against `~/Documents/workspaces/maintainer-prod` with `BRANCH=prod` and `kubectlquant -n prod`. Leave the pod running for ≥24h and confirm at the end:

```
kubectlquant -n prod logs statefulset/maintainer-watcher-github-build --since=24h \
  | grep -E '401|403'   # expect: zero matches
kubectlquant -n prod logs statefulset/maintainer-watcher-github-build --since=24h \
  | grep -E 'CreateTaskCommand'   # expect: at least one
```

## Do-Nothing Option

If we don't ship this, the `pr-review-of-ben` PAT cannot be revoked, because the build watcher is one of the last two consumers blocking retirement. The watcher continues to depend on a user account that GitHub Trust & Safety has previously flagged as spammy — re-flagging would silently degrade workflow-run polling (401s with no clear signal, missing build-fixer tasks for known-red builds). The infrastructure to fix this (`lib/githubapp`, the registered Apps, the Teamvault PEM entries, the Secret template) is already shipped by spec 033; the remaining work is hours of wiring. Not doing this work strands that investment and leaves the watcher dependent on a credential we want to retire. Not acceptable.

---

**Related notes:**

- Sibling spec (parallel): migrate `watcher/github-pr` from PAT to GitHub App — both must merge before PAT retirement.
- Predecessor: `specs/in-progress/033-migrate-pr-reviewer-to-github-app.md` — established `lib/githubapp` and the App identities reused here.
- Reference doc: `agent/pr-reviewer/docs/github-app-setup.md`.
- Vault task: `[[Migrate Build Watcher from User PAT to GitHub App]]` (goal F3 of `[[GitHub Code Reviewer Agent - Base]]`).
