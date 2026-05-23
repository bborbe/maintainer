---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-05-23T21:06:45Z"
generating: "2026-05-23T21:06:46Z"
prompted: "2026-05-23T21:11:21Z"
branch: dark-factory/migrate-pr-watcher-to-github-app
---

## Summary

- Migrate the `watcher/github-pr` long-lived StatefulSet from PAT auth (`pr-review-of-ben` user, Teamvault `ROnG5L`) to GitHub App auth, so the user PAT can be revoked.
- Reuse the existing pr-reviewer Apps registered in spec 033: prod App `3798945` / Install `134414316` / PEM `kLoejw`; dev App `3800041` / Install `134435225` / PEM `eqKj8L`. No new App registration.
- Critical technical decision: this watcher polls every 5min forever, so it MUST use `lib/githubapp.NewClient` (returns an `*http.Client` whose transport auto-refreshes the IAT) — NOT `MintIAT`, which produces a one-shot 1-hour token that the pr-reviewer Job uses but would brick a 24/7 service after 60min.
- Keep the legacy `GH_TOKEN` env accepted as a fallback during rollout. Cleanup of the Secret entry deferred to a follow-up task after soak.
- Build watcher migration (sibling) and PAT revocation are separate tasks; this spec covers the PR watcher only.

## Problem

GitHub Trust & Safety classified the `pr-review-of-ben` user account as "Spammy" and refused reinstatement on 2026-05-23 (tickets #4391427 + #4399644), citing the ToS rule that one individual may not maintain multiple free user accounts. That PAT is now permanently at risk of revocation by GitHub. The pr-reviewer agent already migrated off this PAT in spec 033 (released `v0.25.9` → `v0.25.12`). Two services still authenticate as that user — `watcher/github-pr` (this spec) and `watcher/github-build` (separate sibling spec). Both must move to App auth before the PAT can be revoked. Until then, GitHub may pull the credential at any moment, silently breaking PR-watcher polling and stopping new pr-reviewer tasks from being created.

## Goal

After this work, the `watcher/github-pr` pod authenticates to the GitHub REST API as a GitHub App installation in both dev and prod clusters. The IAT it uses is refreshed transparently and automatically; the pod can run for arbitrarily long without restart and without credential expiry. No code path in the watcher depends on the `pr-review-of-ben` user identity. The legacy `GH_TOKEN` env stays accepted as a fallback during rollout so a malformed App config does not brick the pod.

## Non-goals

- Do NOT migrate `watcher/github-build`. Sibling task `[[Migrate Build Watcher from User PAT to GitHub App]]`, separate spec.
- Do NOT revoke the `pr-review-of-ben` PAT. Blocked on both watcher migrations; tracked as a third follow-up task.
- Do NOT register new GitHub Apps. Reuse the pr-reviewer Apps from spec 033.
- Do NOT bump App permissions. The existing `Pull requests: Read & Write` + `Metadata: Read` + `Contents: Read` is more than enough — the watcher only calls `Search.Issues` and `PullRequests.Get` (no writes to GitHub).
- Do NOT remove the `GH_TOKEN` field, the secret key, or the env wiring. Cleanup is a separate spec after soak.
- Do NOT switch the watcher to `lib/githubapp.MintIAT`. That helper produces a one-shot 1-hour token suitable for short-lived Jobs (pr-reviewer), not for a 24/7 StatefulSet. Using `MintIAT` here would silently break polling 60min after each pod start.
- Do NOT add a tunable IAT-refresh interval, App-auth feature flag, or other knobs. App-auth either works for this watcher or the rollout is reverted; there is no consumer asking for variation.

## Desired Behavior

1. When the pod starts with App-auth env vars set (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY` all populated), the watcher authenticates to GitHub as the configured App installation. Outgoing API calls (PR search, PR detail) carry an IAT minted via `lib/githubapp.NewClient`. The IAT is refreshed transparently by the underlying transport before expiry; the pod runs indefinitely without re-mint logic in the watcher itself.
2. When the App-auth env vars are absent and `GH_TOKEN` is set, the watcher authenticates with the PAT exactly as before (legacy fallback path) and logs a warning at startup naming the fallback.
3. When App-auth env vars and `GH_TOKEN` are both set, App auth wins; the PAT is ignored and a warning is logged.
4. When neither is set, the pod refuses to start with a wrapped error that names both env-var sets.
5. At startup, the pod emits a single structured log line declaring the chosen auth mode and (when App auth) the App ID and Installation ID. Operators reading `kubectl logs` can verify the mode at a glance.
6. PR search and PR detail behavior is byte-identical between PAT mode and App mode against the same set of repos. The migration does not change which PRs the watcher publishes tasks for, the cursor format, the Kafka schema, the vault filename layout, or any operator-visible config beyond the new auth env vars.
7. The dev cluster runs against dev App ID `3800041` / Installation `134435225` (scoped to `bborbe/go-skeleton` only). The prod cluster runs against prod App `3798945` / Installation `134414316` (scoped to all `bborbe/*`).
8. After at least one IAT lifetime (1 hour) of dev runtime, the pod has refreshed the IAT at least once with zero operator action and no error logs.

## Constraints

- The two App identities are frozen (registered + smoke-tested in spec 033): prod App `3798945` / Install `134414316` / PEM Teamvault `kLoejw`; dev App `3800041` / Install `134435225` / PEM Teamvault `eqKj8L`. Do not register new Apps.
- Auth path MUST be `lib/githubapp.NewClient(ctx, cfg)` (returns `*http.Client` with the auto-refreshing `ghinstallation/v2` transport). MUST NOT be `lib/githubapp.MintIAT` (single-shot 1-hour token).
- The Secret YAML committed to the repo MUST use the existing `teamvault*` template pattern (see `watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml` GH_TOKEN line). PEM bytes MUST NOT enter git.
- App ID and Installation ID are public values and MUST be wired as plain env values from `dev.env` / `prod.env`, not as Secret keys. PEM is the secret.
- Errors constructed and wrapped exclusively with `github.com/bborbe/errors`. No `fmt.Errorf`, no stdlib `errors.New`.
- Logging uses `glog`. PEM bytes and IAT bytes (beyond a short prefix) MUST NOT appear in any log line.
- BSD-style license headers on every new `.go` file.
- `CHANGELOG.md` entry under `## Unreleased`. The autoRelease tag is created by dark-factory at spec-complete time.
- Deploy ordering: dev cluster first, soak ≥1 hour to prove IAT refresh works, then prod. Reversing the order would expose prod to any latent bug.
- Reference implementation: spec 033 (`specs/in-progress/033-migrate-pr-reviewer-to-github-app.md`) and the merged code in `agent/pr-reviewer/main.go` + `lib/githubapp/`. Mirror the env-var naming and fallback semantics; only differ in `NewClient` vs `MintIAT` choice.
- See `docs/verifying-specs.md` for the rung model used in Verification.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| App-auth env unset, `GH_TOKEN` set | Pod boots in PAT fallback mode; warning logged. | None — intentional transition path. | `glog` line `watcher auth mode=pat-fallback` in pod logs. |
| App-auth env unset AND `GH_TOKEN` unset | Pod refuses to start with a wrapped error naming both env-var sets. | Operator sets one of the two auth modes. | Pod CrashLoopBackOff; error line contains `APP_ID` and `GH_TOKEN` literals. |
| App-auth env partially set (e.g. `APP_ID` only) | Pod refuses to start with an error naming the missing field. | Operator completes the env set. | Pod CrashLoopBackOff; error names the missing field. |
| `PEM_KEY` env points at a value that is not a valid RSA private key | `ghinstallation/v2` returns a parse error at first IAT mint; pod refuses to start. | Operator rotates the PEM in Teamvault and the Secret. | Pod CrashLoopBackOff; error mentions PEM parse failure. |
| `APP_ID` or `INSTALLATION_ID` wrong | First IAT mint returns 404 from GitHub; pod refuses to start. | Operator corrects the env value. | Pod CrashLoopBackOff; error contains HTTP 404 and the GitHub response body. |
| Cached IAT expires mid-pod-lifetime | `ghinstallation/v2` transparently mints a new IAT on the next outgoing request. No watcher-visible behavior change. | None — handled by the library. | Pod keeps polling without operator action; no error log line; rung-2 ≥1h soak confirms. |
| GitHub returns 401 for the IAT mid-run (revoked PEM, App deleted, etc.) | The next poll cycle logs an error from `SearchPRs` / `GetPRDetails`. The poll loop continues (cycle errors are logged, not fatal). | Operator regenerates the PEM via the App settings page, updates Teamvault + the Secret, restarts the pod. | `glog.Errorf("poll cycle error: ...")` line in pod logs containing HTTP 401. |
| GitHub rate-limits an IAT-authenticated request | Same handling as today's PAT-rate-limit case: the poll cycle error is logged; the next tick retries. No behavior change. | None — recovers on next tick. | Existing error path; no new alarm. |
| Secret rotated but pod not restarted | Pod keeps using the previously-loaded PEM (Kubernetes Secrets remount on restart only). Old PEM is still valid until revoked on the App settings page. | Operator restarts the pod after rotating; revokes old PEM only after restart confirmation. | Documented in PEM-rotation runbook (already drafted in spec 033's `docs/github-app-setup.md`). |
| Two pods run concurrently (e.g. rolling restart overlap) | Each pod obtains its own IAT independently; GitHub permits multiple live IATs per Installation. No collision. | None. | No alarm. |

## Security / Abuse Cases

- The IAT (`ghs_...`) is a bearer token with the App's full permission set. It lives only in process memory inside the watcher pod; it is NOT forwarded to any subprocess (unlike pr-reviewer, the watcher has no Claude CLI).
- The PEM is the long-lived secret. It MUST NOT enter git, MUST NOT appear in logs (beyond the prefix-only logging already implemented in `lib/githubapp`).
- App permissions are bounded to read-only repo content + read-only metadata + PR R&W (inherited from the pr-reviewer Apps). The watcher only exercises the read paths. A compromised IAT could write PR reviews/comments — same blast radius as the pr-reviewer pod, which is acceptable since both pods are deployed from the same registry to the same cluster.
- Dev App is scoped to a single repo (`bborbe/go-skeleton`) — a dev-cluster compromise cannot reach the rest of the bborbe org. This is a constraint on the App installation, not on the watcher code; verified by the dev installation already in place.
- New env vars are read-only configuration; no user-controlled input crosses a trust boundary here.

## Acceptance Criteria

Rung 1 — code wiring (host)

- [ ] `pkg.NewGitHubClient` accepts an `*http.Client` instead of a token string — evidence: `grep -n 'func NewGitHubClient' watcher/github-pr/pkg/githubclient.go` shows the signature; existing callers in `pkg/factory/factory.go` updated.
- [ ] `pkg/factory/factory.go` exposes a `CreateGitHubHTTPClient` (or equivalent) that returns an `*http.Client` chosen by the mode rules in Desired Behavior #1–#4 — evidence: `grep -n 'CreateGitHubHTTPClient\|AuthConfig' watcher/github-pr/pkg/factory/factory.go` matches; ginkgo unit tests cover App-auth happy path (env complete), PAT-fallback happy path (App env empty, GH_TOKEN set), both-set warning (App wins), and neither-set error.
- [ ] App-auth path uses `lib/githubapp.NewClient` (NOT `MintIAT`) — evidence: `grep -n 'githubapp\.NewClient\|githubapp\.MintIAT' watcher/github-pr/pkg/factory/factory.go` shows `NewClient` matching at least once and `MintIAT` matching zero times.
- [ ] `main.go` accepts new env vars `APP_ID`, `INSTALLATION_ID`, `PEM_KEY` all marked `required:"false"`; existing `GH_TOKEN` is now `required:"false"` with usage string mentioning "legacy fallback" — evidence: `grep -n 'APP_ID\|INSTALLATION_ID\|PEM_KEY\|legacy fallback' watcher/github-pr/main.go` matches; `--help` output of the binary lists the new flags.
- [ ] At pod startup, exactly one log line declares the chosen auth mode — evidence: ginkgo test asserts `watcher auth mode=github-app app_id=<id> installation_id=<id>` (App path) or `watcher auth mode=pat-fallback` (PAT path).
- [ ] No new `glog` call in the watcher logs full PEM bytes or full IAT bytes — evidence: `grep -nE 'glog\.[A-Z][a-z]+f?\(.*(PEM|ghs_)' watcher/github-pr/` returns zero matches.
- [ ] All errors use `github.com/bborbe/errors`; no `fmt.Errorf` / stdlib `errors.New` in the changed files — evidence: `git diff master -- watcher/github-pr/ | grep -E '^\+.*(fmt\.Errorf|errors\.New\()'` returns zero matches.
- [ ] All new `.go` files carry the BSD-style license header — evidence: `git diff master --name-only -- 'watcher/github-pr/**/*.go' | xargs grep -L 'BSD-style'` returns empty.
- [ ] `cd watcher/github-pr && make precommit` exits 0 — evidence: exit code 0; final output line `ready to commit`.
- [ ] `CHANGELOG.md` has a new entry under `## Unreleased` describing the migration — evidence: `grep -A2 '## Unreleased' CHANGELOG.md` shows an entry mentioning `watcher/github-pr` and GitHub App auth.

Rung 2 — k8s wiring + dev cluster deploy

- [ ] `watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml` declares a new key `PEM_KEY` populated via the existing `teamvaultFileBase64` template; `GH_TOKEN` key retained — evidence: `grep -n 'PEM_KEY\|GH_TOKEN' watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml` shows both keys.
- [ ] `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` declares container env entries `APP_ID`, `INSTALLATION_ID` (plain values from `WATCHER_GITHUB_PR_APP_ID` / `WATCHER_GITHUB_PR_INSTALLATION_ID`) and `PEM_KEY` (from the Secret) — evidence: `grep -nE 'APP_ID|INSTALLATION_ID|PEM_KEY' watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` shows all three names wired.
- [ ] `dev.env` and `prod.env` declare `WATCHER_GITHUB_PR_APP_ID`, `WATCHER_GITHUB_PR_INSTALLATION_ID`, `WATCHER_GITHUB_PR_PEM_KEY` with the correct App identities — evidence: `grep -n 'WATCHER_GITHUB_PR_APP_ID\|WATCHER_GITHUB_PR_INSTALLATION_ID\|WATCHER_GITHUB_PR_PEM_KEY' dev.env prod.env` shows `prod.env` lines `=3798945`, `=134414316`, `=kLoejw` and `dev.env` lines `=3800041`, `=134435225`, `=eqKj8L`.
- [ ] Dev cluster rolls out cleanly to the new image with App-auth env set — evidence: `kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-pr --timeout=120s` reports complete.
- [ ] Dev pod logs show `watcher auth mode=github-app app_id=3800041 installation_id=134435225` at startup — evidence: `kubectlquant -n dev logs statefulset/maintainer-watcher-github-pr | grep 'auth mode='` matches.
- [ ] Dev pod successfully completes ≥1 poll cycle against the live GitHub API under App auth — evidence: `kubectlquant -n dev logs statefulset/maintainer-watcher-github-pr | grep 'poll cycle start'` matches at least once with no subsequent `poll cycle error` line for that cycle.
- [ ] Dev pod survives ≥1 hour of runtime with no `poll cycle error` lines containing HTTP 401 — evidence: `kubectlquant -n dev logs statefulset/maintainer-watcher-github-pr --since=70m | grep -E 'poll cycle error.*401'` returns zero matches. This proves the auto-refresh transport works past the 1-hour IAT lifetime.

Rung 3 — prod cluster deploy

- [ ] After dev soaks ≥1 hour clean, prod cluster rolls out with the prod App identity — evidence: `kubectlquant -n prod rollout status statefulset/maintainer-watcher-github-pr --timeout=120s` reports complete.
- [ ] Prod pod logs show `watcher auth mode=github-app app_id=3798945 installation_id=134414316` at startup — evidence: `kubectlquant -n prod logs statefulset/maintainer-watcher-github-pr | grep 'auth mode='` matches.
- [ ] Prod pod completes ≥3 poll cycles cleanly under App auth — evidence: `kubectlquant -n prod logs statefulset/maintainer-watcher-github-pr | grep 'poll cycle start' | wc -l` ≥ 3, with zero `poll cycle error` lines containing HTTP 401 in the same window.

Scenario coverage — NO new scenario. The auth-mode-selection logic is covered by ginkgo unit tests against `lib/githubapp` with a `httptest.Server`. The end-to-end behavior is covered by rung-2 and rung-3 live cluster verification, which an in-repo scenario could not faithfully reproduce (real GitHub App, real Installation, real long-lived StatefulSet, real IAT refresh past 1h). The single behavior an E2E test could uniquely catch — IAT refresh past 1h — requires real time and a real GitHub credential exchange that no scenario harness can simulate without becoming flaky.

## Verification

Per `docs/verifying-specs.md`, this spec is rung-2-then-rung-3 (touches a deployed service AND introduces new Kubernetes Secret keys AND env-injection wiring). Execute in order.

**Rung 1 — precommit (host):**

```
cd watcher/github-pr && make precommit
```

Expected: exit 0; final output line `ready to commit`.

**Rung 2 — dev cluster:**

```
cd ~/Documents/workspaces/maintainer-dev
git pull && git merge master --no-edit && git push

cd watcher/github-pr && BRANCH=dev make build upload
cd k8s              && BRANCH=dev make buca

kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-pr --timeout=120s
kubectlquant -n dev logs statefulset/maintainer-watcher-github-pr | grep 'auth mode='
```

Expected: rollout completes; log line `watcher auth mode=github-app app_id=3800041 installation_id=134435225`.

Then wait ≥1 hour and re-check for 401 errors:

```
kubectlquant -n dev logs statefulset/maintainer-watcher-github-pr --since=70m | grep -E 'poll cycle error.*401'
```

Expected: zero matches (proves IAT auto-refresh works).

**Rung 3 — prod cluster:**

After dev soaks ≥1 hour clean, repeat the same sequence against `~/Documents/workspaces/maintainer-prod` with `BRANCH=prod` and `kubectlquant -n prod`. Confirm log line `watcher auth mode=github-app app_id=3798945 installation_id=134414316` and zero 401 errors across ≥3 prod poll cycles.

## Do-Nothing Option

If we don't ship this, the `pr-review-of-ben` PAT cannot be revoked (the watcher would lose auth the moment we revoke it). GitHub Trust & Safety has already refused reinstatement of that user account and may revoke the PAT unilaterally at any time. When that happens, the PR watcher silently stops creating tasks, the pr-reviewer agent receives no new work, and the entire review pipeline stalls until an operator notices, scrambles to provision App auth under pressure, and redeploys. The App infrastructure is already in place from spec 033 — the remaining cost is wiring the existing lib into one more binary. Not acceptable to leave this on the to-do list.

---

**Related vault notes:**

- Task: `[[Migrate PR Watcher from User PAT to GitHub App]]`
- Parent goal: `[[GitHub Code Reviewer Agent - Base]]` (closed; PAT retire deferred to these follow-ups)
- Sibling task (separate spec): `[[Migrate Build Watcher from User PAT to GitHub App]]`
- Reference spec: `specs/in-progress/033-migrate-pr-reviewer-to-github-app.md`
- Reference code: `agent/pr-reviewer/main.go`, `lib/githubapp/githubapp.go`
- App-credentials doc: `agent/pr-reviewer/docs/github-app-setup.md`
