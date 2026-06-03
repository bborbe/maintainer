---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-29T17:59:25Z"
generating: "2026-05-29T18:10:20Z"
prompted: "2026-05-29T18:10:20Z"
verifying: "2026-05-29T19:40:15Z"
completed: "2026-05-30T11:15:52Z"
branch: dark-factory/fleet-app-only-auth
---

## Summary

- On 2026-05-24 the `GH_TOKEN` PAT auth path was retired fleet-wide: every maintainer service in the cluster authenticates to GitHub via a GitHub App installation token (Config CRs set `APP_ID` / `INSTALLATION_ID` / `PEM_KEY`, never `GH_TOKEN`). The PAT *input* fallback now lives only as dormant code — alive in local dev, dead in prod.
- The `agent/github-releaser` agent was already made strictly App-only (it can push to master, so it earned the tightest auth surface first). This work brings the remaining 4 maintainer services to the same App-only model for fleet uniformity and a smaller auth surface everywhere.
- The 4 services are: `agent/pr-reviewer`, `watcher/github-pr`, `watcher/github-build`, `watcher/github-release`. Each gets its `GH_TOKEN` PAT *input* removed; the GitHub App path stays exactly as-is.
- This is low-risk dead-code removal: the removed branch is never exercised in production. The one subtlety is `pr-reviewer`, which reuses the resolved token (the minted App installation token) to authenticate the `gh` CLI and git credential helper in its subprocess — that resolved-token forwarding MUST survive; only the PAT *input credential* is removed.
- After this work, all 4 services refuse to start when App credentials are absent (clear startup error naming the App env vars), and no service accepts or reads a `GH_TOKEN` input.

## Problem

Four maintainer services still carry a `GH_TOKEN` personal-access-token (PAT) input as a "legacy fallback" auth path. Since the fleet-wide PAT retirement on 2026-05-24, no deployed instance sets `GH_TOKEN` — the fallback is dead code in production but remains an accepted input. A dormant credential input widens the auth surface (a misconfiguration or a leaked PAT could silently re-enable a weaker, longer-lived, broader-scoped credential), and it leaves the fleet inconsistent now that the push-capable releaser agent is already App-only. Removing the dormant PAT input makes every service App-auth-or-refuse-to-start, matching the releaser and shrinking the credential surface uniformly.

## Goal

All four services authenticate to GitHub exclusively via the GitHub App installation token. None of them declares, reads, accepts, or forwards a `GH_TOKEN` PAT *input*. When the App credentials are absent or incomplete, each service fails fast at startup with a clear error that names the App environment variables (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY` / `PEM_KEY_FILE`) and does not mention `GH_TOKEN` as an alternative. The existing GitHub App minting/resolution behavior is unchanged, and `pr-reviewer` continues to forward its minted installation token to the `gh` CLI / git credential helper subprocess.

## Non-goals

- Do NOT touch `agent/github-releaser` — it is already App-only and is the reference model, not a target. (Note: in this worktree's branch base the releaser's App-only change is not yet merged; treat the *intent* — App-auth-or-error, drop the PAT input, keep forwarding the resolved token — as the pattern, not the releaser's current on-disk code.)
- Do NOT change the GitHub App auth path itself (JWT exchange, IAT minting, `lib/githubapp` usage, partial-App-config error handling). The App path is frozen.
- Do NOT change how the *resolved* token is consumed downstream: `pr-reviewer`'s forwarding of the minted token to its subprocess (env var, git credential helper, repo manager, agent provider) stays exactly as-is; the watchers' use of the resolved `*http.Client` for their GitHub API client stays exactly as-is.
- Do NOT add a config flag, opt-out, or env toggle to re-enable PAT auth — invariant; if a future consumer demands PAT auth again, that is a separate spec.
- Do NOT create, rotate, or reconfigure any GitHub App or installation.
- Do NOT modify k8s manifests or Config CRs to *add or change* auth env (they already omit `GH_TOKEN`). Removing a now-orphaned `GH_TOKEN` env declaration, if one is found, is in scope only as a flagged note (see Desired Behavior 6); manifest infra changes beyond that are out of scope.

## Assumptions

- All deployed instances of the 4 services already set `APP_ID` / `INSTALLATION_ID` / `PEM_KEY` (or `PEM_KEY_FILE`) and do NOT set `GH_TOKEN` — established by the fleet-wide PAT retirement on 2026-05-24. Removing the PAT input therefore changes no production code path.
- The GitHub App installation token minted by each service today is functionally sufficient everywhere the PAT was previously accepted (API client auth, and `pr-reviewer`'s subprocess `gh`/git-credential forwarding) — already true, since prod has run App-only since the retirement.

## Desired Behavior

1. **No `GH_TOKEN` input remains in any of the 4 services.** After the work, no binary (including the `cmd/run-task` and `cmd/run-once` siblings) declares an argument/config field bound to `env:"GH_TOKEN"`, and no code reads `GH_TOKEN` from the environment (e.g. `os.Getenv("GH_TOKEN")`) as an auth input. (`watcher/github-pr` currently both declares a `GHToken` arg field AND reads `os.Getenv("GH_TOKEN")` directly in its resolver — both must go.)

2. **Each auth resolver becomes App-auth-or-error.** In every resolution path the PAT-fallback branch is removed, along with the "both App credentials and `GH_TOKEN` are set — App wins; `GH_TOKEN` ignored" warning (it is unreachable once PAT input is gone). New resolution: App credentials present and complete → mint/use the installation token; App credentials absent or incomplete → return a startup error before any GitHub work begins. (The existing *partial-App-config* error — some-but-not-all App fields set — is part of the frozen App path and stays.)

3. **The App auth path is byte-for-byte preserved in behavior.** Each service still mints/uses the installation token the same way it does today (`agent/pr-reviewer` via `lib/githubapp.MintIAT` inside its `resolveAuth`; `watcher/github-build` via its `pkg/auth.Resolve`; `watcher/github-release` via its `pkg/auth.ResolveGitHubClient`; `watcher/github-pr` via its inline `resolveAuth` building an App client). The App-mode log line and the resulting authenticated client/token are unchanged.

4. **`pr-reviewer` still forwards the resolved token to its subprocess.** The minted installation token continues to reach the `gh` CLI auth + git credential helper in the claude-yolo container, and the repo manager / agent provider, exactly as today. Removing the PAT *input* must not break the *resolved-token* forwarding. (Implementation note for the executor: `pr-reviewer` currently reuses the same `GHToken` field as both the PAT input AND the carrier for the minted IAT — `a.GHToken = iat`. Removing the field requires introducing a distinct resolved-token carrier so forwarding survives; agent decides the exact shape at impl time, but the forwarded value MUST be the minted installation token.)

5. **Startup error messages name the App env vars, not `GH_TOKEN`.** The "neither configured" / "auth not configured" startup error in each service references the App environment variables (`APP_ID`, `INSTALLATION_ID`, and `PEM_KEY` and/or `PEM_KEY_FILE` per that service's existing support) and does NOT offer `GH_TOKEN` as an alternative.

6. **Stale `GH_TOKEN` env wiring is surfaced.** Any Config CR, k8s manifest, or `*.env` file that still declares a now-unused `GH_TOKEN` for one of these 4 services is identified and reported (as a note in the implementation output and, if trivial and isolated, removed). A lingering comment that merely *mentions* `GH_TOKEN` (e.g. a doc comment) is not a live env declaration and need not be removed, but should be updated if it now misstates behavior.

7. **PAT-fallback tests are removed; App-mode and absent-creds tests remain green.** Unit tests asserting PAT-fallback behavior or "both App+PAT set" behavior are removed (those code paths no longer exist). Tests covering App-mode success and "App credentials absent → startup error" exist and pass for each service.

## Constraints

- `github.com/bborbe/maintainer` is a multi-module mono-repo; each of the 4 services is its own Go module with its own `go.mod`. Build and verify each independently with `make precommit` in that service's directory (the per-service `Makefile` includes the shared `Makefile.precommit`).
- Error construction uses `github.com/bborbe/errors` context-form (`errors.Errorf(ctx, …)`, `errors.Wrap(ctx, …)`); NO `fmt.Errorf`; NO `context.Background()` in business logic.
- Tests use Ginkgo v2 + Gomega, external `_test` packages, counterfeiter v6 for fakes.
- Per-service coverage gate is whatever each module's existing `make precommit` enforces; removed dead branches must not drop a module below its existing gate (removing dead code and its tests typically raises or holds coverage).
- The GitHub App auth path (frozen): `lib/githubapp` minting, partial-App-config error, App-mode log line, and the resolved `*http.Client` / token value must not change behavior.
- `pr-reviewer`'s subprocess env builder, git credential helper wiring (`githubauth.NewGhAuthSetupGit`), repo manager construction, and agent provider construction must continue to receive the minted installation token.
- See `docs/architecture.md` and `docs/build-watcher.md` for service layout and the per-module build model.

## Failure Modes

| Trigger | Expected behavior | Recovery | Reversibility |
|---------|-------------------|----------|---------------|
| App credentials absent at startup (no `APP_ID`/`INSTALLATION_ID`/`PEM_KEY`) | Service returns a startup error naming the App env vars and exits non-zero before any GitHub call | Operator sets the App env vars (already set in all deployed instances) | n/a (startup-only) |
| App credentials partially set (some but not all three) | Existing partial-App-config error fires (unchanged behavior) | Operator sets the missing App field(s) | n/a (startup-only) |
| Operator sets a stray `GH_TOKEN` env (post-change) | Ignored entirely — no field reads it; if App creds present, App auth proceeds; if absent, startup error fires (no silent PAT fallback) | None needed; behavior is App-or-error regardless of `GH_TOKEN` | n/a |
| `pr-reviewer` minting succeeds but resolved-token forwarding regressed by the change | Caught pre-merge: `pr-reviewer` subprocess loses `gh`/git auth (private clone fails). Prevented by Desired Behavior 4 + its AC | Re-instate forwarding of the minted token; do not re-add the PAT input | reversible (code) |
| A removed test was the only coverage for an App-mode branch | `make precommit` coverage gate fails in that module | Add/keep an App-mode test before removing the PAT test | reversible (code) |

## Security / Abuse Cases

- This change *shrinks* the trust surface: it removes an accepted long-lived broad-scoped credential input (`GH_TOKEN` PAT) in favor of the short-lived narrowly-scoped App installation token only.
- After the change, a leaked or misconfigured `GH_TOKEN` env value cannot re-enable a weaker auth path — no code reads it.
- The minted installation token still flows into `pr-reviewer`'s subprocess (necessary for private-repo clone). That forwarding is unchanged; no new path exposes the token to logs or error messages (the App path already uses `display:"length"` on credential fields and logs only App ID / installation ID).
- No new user-controlled input or trust boundary is introduced.

## Acceptance Criteria

Run all greps from the worktree root `/Users/bborbe/Documents/workspaces/maintainer-fleet-app-auth`.

- [ ] No `env:"GH_TOKEN"` struct tag remains in any of the 4 services — evidence: `grep -rn 'env:"GH_TOKEN"' agent/pr-reviewer watcher/github-pr watcher/github-build watcher/github-release` returns zero matches (exit 1).
- [ ] No `GHToken` identifier remains in any of the 4 services' non-test Go files — evidence: `grep -rn 'GHToken' --include='*.go' agent/pr-reviewer watcher/github-pr watcher/github-build watcher/github-release | grep -v _test.go` returns zero matches (exit 1).
- [ ] No code reads `GH_TOKEN` from the environment as an auth input in the 4 services — evidence: `grep -rn 'Getenv("GH_TOKEN")' agent/pr-reviewer watcher/github-pr watcher/github-build watcher/github-release` returns zero matches (exit 1).
- [ ] No PAT-fallback branch or "App wins; GH_TOKEN ignored" warning remains — evidence: `grep -rn 'pat-fallback\|App wins\|GH_TOKEN ignored\|PATFallback\|AuthModePATFallback' agent/pr-reviewer watcher/github-pr watcher/github-build watcher/github-release` returns zero matches (exit 1).
- [ ] `pr-reviewer` still forwards the minted installation token to its subprocess — evidence: the env-builder still sets the `GH_TOKEN` *output* env for the subprocess from the resolved token, AND `NewGhAuthSetupGit` / repo manager / agent provider still receive the resolved token. Verify by reading `agent/pr-reviewer/main.go` `dispatchAgent`: `env["GH_TOKEN"]` is assigned from the resolved-token carrier (not from a removed input field), and the same carrier is passed to `NewGhAuthSetupGit`, `git.NewRepoManager`, and `factory.CreateAgentProvider` — evidence: file content shows these three call sites pass the resolved token.
- [ ] Each service's "auth not configured" startup error names the App env vars and not `GH_TOKEN` — evidence: `grep -rni 'APP_ID' <service>/main.go` finds the App env var named in the service's startup/error path for each of the 4 services (the exact phrasing differs across the 3 resolver shapes — match `APP_ID` case-insensitively in the error string, NOT a fixed literal like "set APP_ID"). That no `GH_TOKEN` *input* survives the auth/error path is already proven by AC #1 (`env:"GH_TOKEN"` → zero matches) and AC #3 (`Getenv("GH_TOKEN")` → zero matches); the only surviving `GH_TOKEN` reference anywhere is the resolved-IAT *output* forwarding in `pr-reviewer` (`env["GH_TOKEN"]=`), which the forwarding AC above positively verifies stays. (No fragile `grep -iv` exclusion is used — input-absence and output-survival are each pinned by a dedicated AC.)
- [ ] `make precommit` exits 0 in `agent/pr-reviewer` — evidence: exit code 0.
- [ ] `make precommit` exits 0 in `watcher/github-pr` — evidence: exit code 0.
- [ ] `make precommit` exits 0 in `watcher/github-build` — evidence: exit code 0.
- [ ] `make precommit` exits 0 in `watcher/github-release` — evidence: exit code 0.
- [ ] An App-mode success test and an "App credentials absent → startup error" test exist and pass for each of the 4 services — evidence: per-service `make precommit` (above) reports the relevant test names as passing; the test files contain no remaining PAT-fallback assertions (covered by the `GHToken`/`pat-fallback` greps including `_test.go` returning only the intended App-mode/absent-creds tests).
- [ ] Stale `GH_TOKEN` env wiring for these 4 services is reported — evidence: implementation output lists the result of `grep -rn 'GH_TOKEN' --include='*.env' --include='*.yaml' --include='*.yml' .` scoped to the 4 services, with each hit classified as "live env declaration (removed)" or "comment/doc mention (left or corrected)".

**Scenario coverage — NO new scenario.** This is dead-code removal verified by unit tests + per-service `make precommit` + symbol greps. The one load-bearing behavior (`pr-reviewer` resolved-token forwarding to the subprocess) is covered by the existing `pr-reviewer` unit/integration tests and AC #5's file-content check; no real-Docker / real-`gh` E2E is required to catch a forwarding regression because the env-builder assignment is unit-observable.

## Verification

Per service, from the worktree root:

```
cd agent/pr-reviewer && make precommit
cd watcher/github-pr && make precommit
cd watcher/github-build && make precommit
cd watcher/github-release && make precommit
```

Symbol greps (all expected to return zero auth-input matches; run from worktree root):

```
grep -rn 'env:"GH_TOKEN"' agent/pr-reviewer watcher/github-pr watcher/github-build watcher/github-release
grep -rn 'GHToken' --include='*.go' agent/pr-reviewer watcher/github-pr watcher/github-build watcher/github-release | grep -v _test.go
grep -rn 'Getenv("GH_TOKEN")' agent/pr-reviewer watcher/github-pr watcher/github-build watcher/github-release
grep -rn 'pat-fallback\|App wins\|GH_TOKEN ignored' agent/pr-reviewer watcher/github-pr watcher/github-build watcher/github-release
```

## Do-Nothing Option

Acceptable in the narrow sense that the PAT path is already dead in production — nothing breaks today if we skip this. But the fleet stays inconsistent (releaser App-only, the other four not), and a dormant broad-scope credential input lingers as a latent re-enablement and leakage risk. Given the change is mechanical, low-risk, and uniform across four services, the cost of doing it is small and the standing auth-surface reduction is durable.
