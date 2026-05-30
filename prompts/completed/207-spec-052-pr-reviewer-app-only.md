---
status: completed
spec: [052-fleet-app-only-auth]
summary: Removed GH_TOKEN PAT input from pr-reviewer; both entry points now authenticate exclusively via GitHub App installation token, forwarded to the agent subprocess as resolvedToken
container: maintainer-fleet-app-auth-exec-207-spec-052-pr-reviewer-app-only
dark-factory-version: v0.173.0
created: "2026-05-29T18:30:00Z"
queued: "2026-05-29T18:24:44Z"
started: "2026-05-29T18:25:27Z"
completed: "2026-05-29T18:29:43Z"
---

<summary>
- The `pr-reviewer` agent no longer accepts a `GH_TOKEN` PAT as an auth input — it authenticates to GitHub only via the GitHub App installation token.
- When App credentials are missing or incomplete, the agent refuses to start with a clear error that names the App env vars (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`/`PEM_KEY`) and never offers `GH_TOKEN` as an alternative.
- The minted installation token still reaches the agent's subprocess exactly as before: the `gh` CLI, the git credential helper, the repo manager, and the agent provider all keep receiving the resolved token, so private-repo clones keep working.
- The GitHub App minting behavior itself (JWT exchange, IAT minting) is unchanged.
- The legacy PAT-fallback code paths and the "App wins; GH_TOKEN ignored" warning are removed from both the Kafka entry point and the local-CLI entry point.
- Tests that asserted PAT-fallback behavior are removed; a test proving "App credentials absent → startup error" is added.
</summary>

<objective>
Make `agent/pr-reviewer` (both the Kafka pod binary and the local-CLI `cmd/run-task` binary) authenticate to GitHub exclusively via the GitHub App installation token, removing the dormant `GH_TOKEN` PAT *input* while preserving the forwarding of the *resolved* (minted) installation token to the agent's subprocess.
</objective>

<context>
Read `CLAUDE.md` at the repo root and in `agent/pr-reviewer/` for project conventions before editing.

Read these guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` context-form usage.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega, external `_test` packages.

Read these source files before editing (they hold the exact current shapes):
- `agent/pr-reviewer/main.go` — the Kafka entry point. `resolveAuth` (currently mints an IAT into `a.GHToken`), `dispatchAgent` (forwards `a.GHToken`), and the `application` struct (fields `GHToken`, `AppID`, `InstallationID`, `PEMKeyFile`, `PEMKey`).
- `agent/pr-reviewer/cmd/run-task/main.go` — the local-CLI entry point. Has its own `application` struct and an inline auth switch using `prpkg.ResolveAuthMode` + `githubapp.MintIAT`, also overloading `a.GHToken` as the IAT carrier.
- `agent/pr-reviewer/pkg/authmode.go` — `ResolveAuthMode(appID, installationID int64, pemKeyFile, ghToken string) AuthMode` with `AuthModeNone`/`AuthModeGitHubApp`/`AuthModePATFallback`.
- `agent/pr-reviewer/pkg/authmode_test.go` — table test that exercises the PAT-fallback branches.
- `agent/pr-reviewer/pkg/factory/runner.go` — `RunConfig.GHToken` is consumed for: `git.NewRepoManager(workdirCfg, cfg.GHToken)`, `env["GH_TOKEN"] = cfg.GHToken`, `CreatePrPoster(cfg.GHToken, ...)`, `CreateReviewVerifier(cfg.GHToken, ...)`, `CreateAgent(..., cfg.GHToken, ...)`. **This is the resolved-token consumer — it MUST keep receiving the minted token.**
- `agent/pr-reviewer/main_test.go` — current suite (only a "Compiles" spec).

**Critical understanding of the `a.GHToken` overload (spec Desired Behavior 4):** Today `a.GHToken` serves TWO roles — (1) the PAT *input* env field, and (2) the *carrier* for the minted IAT (`a.GHToken = iat`). Role (1) must be removed; role (2) (the resolved token that gets forwarded to the subprocess) MUST survive. The chosen approach below introduces a distinct resolved-token carrier so forwarding is preserved.

**Frozen — do NOT change:** `lib/githubapp.MintIAT` usage and signature, the App-mode `glog.V(2)` log line, the partial-App-config handling that already exists, and the downstream consumption of the resolved token (`runner.go`, `git.NewRepoManager`, `githubauth.NewGhAuthSetupGit`, `factory.CreateAgentProvider`). Only the PAT *input* and the PAT-fallback branch are removed.

Note: `githubapp.MintIAT` signature (verified in `lib/githubapp`): `MintIAT(ctx context.Context, cfg githubapp.Config) (string, error)`; `Config{AppID int64; InstallationID int64; PEM []byte; PEMPath string}`.
</context>

<requirements>

1. **`agent/pr-reviewer/main.go` — `application` struct:** Remove the `GHToken` field entirely (the line tagged `env:"GH_TOKEN"`). Keep `AppID`, `InstallationID`, `PEMKeyFile`, `PEMKey`, `BotLogin`. Update the doc comment block above the removed field / above `AppID` so it no longer says "the legacy GHToken env stays accepted as a fallback"; it should state that App auth is the only auth path. **Also update the `AppID` field's `usage:` struct tag if it says "App auth is used instead of GH_TOKEN" (it does today) — reword so it does not mention `GH_TOKEN` (e.g. `usage:"GitHub App ID (numeric); required for App auth"`); a lingering "instead of GH_TOKEN" misstates behavior post-change (spec Desired Behavior 6).** Add a new unexported field on the struct to carry the resolved (minted) installation token, e.g.:

   ```go
   // resolvedToken holds the GitHub App installation token minted in resolveAuth.
   // It is NOT a config input (no env tag) — it is the value forwarded to the
   // agent subprocess (gh CLI, git credential helper, repo manager, agent provider).
   resolvedToken string
   ```

   Place it as a plain struct field (no struct tag) so libargument ignores it.

2. **`agent/pr-reviewer/main.go` — `resolveAuth`:** Rewrite so it is App-auth-or-error. Remove the PAT-fallback `case a.GHToken != "":` branch and the "both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored" warning. The minting logic (PEMKeyFile vs PEMKey → `githubapp.MintIAT`) is unchanged; assign the minted IAT to `a.resolvedToken` (NOT `a.GHToken`). The App-mode `glog.V(2)` log line stays unchanged. The "neither configured" default branch becomes the "App credentials absent" error. New shape:

   ```go
   func (a *application) resolveAuth(ctx context.Context) error {
       hasPEMFile := a.PEMKeyFile != ""
       hasPEMContent := a.PEMKey != ""
       useGitHubApp := a.AppID != 0 && a.InstallationID != 0 && (hasPEMFile || hasPEMContent)
       if !useGitHubApp {
           return errors.Errorf(
               ctx,
               "pr-reviewer auth: GitHub App credentials not configured — set APP_ID, INSTALLATION_ID, and PEM_KEY_FILE (or PEM_KEY)",
           )
       }
       var iat string
       var err error
       if hasPEMFile {
           iat, err = githubapp.MintIAT(ctx, githubapp.Config{
               AppID:          a.AppID,
               InstallationID: a.InstallationID,
               PEMPath:        a.PEMKeyFile,
           })
       } else {
           iat, err = githubapp.MintIAT(ctx, githubapp.Config{
               AppID:          a.AppID,
               InstallationID: a.InstallationID,
               PEM:            []byte(a.PEMKey),
           })
       }
       if err != nil {
           return errors.Wrap(ctx, err, "mint github app iat")
       }
       a.resolvedToken = iat
       glog.V(2).Infof(
           "pr-reviewer auth mode=github-app app_id=%d installation_id=%d",
           a.AppID, a.InstallationID,
       )
       return nil
   }
   ```

   The error string MUST contain the substring `APP_ID` and MUST NOT contain `GH_TOKEN`.

3. **`agent/pr-reviewer/main.go` — forward the resolved token (NOT a removed input).** Everywhere the code currently reads `a.GHToken`, replace with `a.resolvedToken`. Specifically:
   - In `Run`, the `factory.RunConfig` literal: `GHToken: a.GHToken` → `GHToken: a.resolvedToken`.
   - In `Run`, `AuthSetup: githubauth.NewGhAuthSetupGit(a.GHToken)` → `githubauth.NewGhAuthSetupGit(a.resolvedToken)`.
   - In `dispatchAgent`: the `if a.GHToken != "" { env["GH_TOKEN"] = a.GHToken }` block → `if a.resolvedToken != "" { env["GH_TOKEN"] = a.resolvedToken }`. **Keep the `env["GH_TOKEN"]` OUTPUT key — this is the subprocess env var, not an input.**
   - In `dispatchAgent`: `git.NewRepoManager(git.WorkdirConfig{...}, a.GHToken)` → `..., a.resolvedToken)`.
   - In `dispatchAgent`: `factory.CreateAgentProvider(..., a.GHToken, ...)` → `..., a.resolvedToken, ...)`.

   After this step, `grep -n 'a\.GHToken' agent/pr-reviewer/main.go` MUST return zero matches; `env["GH_TOKEN"]` assignment from `a.resolvedToken` MUST remain.

4. **`agent/pr-reviewer/cmd/run-task/main.go` — `application` struct:** Remove the `GHToken` field (the `env:"GH_TOKEN"` line). Keep `AppID`, `InstallationID`, `PEMKeyFile`, `BotLogin`. Update the doc comment so it no longer mentions a GHToken fallback, **and update the `AppID` field's `usage:` tag if it says "App auth is used instead of GH_TOKEN" — same reword as step 1.** Add an unexported `resolvedToken string` field (no struct tag), same pattern as step 1.

5. **`agent/pr-reviewer/cmd/run-task/main.go` — auth resolution in `Run`:** Replace the `prpkg.ResolveAuthMode(...)` switch with App-auth-or-error logic. `cmd/run-task` only supports `PEMKeyFile` (it has no `PEMKey` field). Remove the `AuthModePATFallback` case and the "App wins; GH_TOKEN ignored" warning. New shape:

   ```go
   useGitHubApp := a.AppID != 0 && a.InstallationID != 0 && a.PEMKeyFile != ""
   if !useGitHubApp {
       return errors.Errorf(
           ctx,
           "pr-reviewer auth: GitHub App credentials not configured — set APP_ID, INSTALLATION_ID, and PEM_KEY_FILE",
       )
   }
   iat, err := githubapp.MintIAT(ctx, githubapp.Config{
       AppID:          a.AppID,
       InstallationID: a.InstallationID,
       PEMPath:        a.PEMKeyFile,
   })
   if err != nil {
       return errors.Wrap(ctx, err, "mint github app iat")
   }
   a.resolvedToken = iat
   glog.V(2).Infof(
       "pr-reviewer auth mode=github-app app_id=%d installation_id=%d",
       a.AppID, a.InstallationID,
   )
   ```

   Then in the `factory.RunConfig` literal, replace `GHToken: a.GHToken` with `GHToken: a.resolvedToken`. `cmd/run-task` uses `githubauth.NewNoopAuthSetup()` (unchanged — it does not forward a token to gh; the developer's own `gh auth login` handles it). The error string MUST contain `APP_ID` and MUST NOT contain `GH_TOKEN`.

6. **`agent/pr-reviewer/pkg/authmode.go` — delete the PAT-fallback enum machinery.** After step 5 there is no remaining caller of `ResolveAuthMode` (verify: `grep -rn 'ResolveAuthMode' agent/pr-reviewer/`). Delete the file `agent/pr-reviewer/pkg/authmode.go` and its test `agent/pr-reviewer/pkg/authmode_test.go` (the test asserts `AuthModePATFallback` behavior, a removed path). If after deletion any other file references `AuthMode`, `AuthModeNone`, `AuthModeGitHubApp`, or `AuthModePATFallback`, that reference must also be removed — but verify first; the grep above is the source of truth.

7. **Keep the resolved-token forwarding chain in `pkg/factory/runner.go` untouched.** `RunConfig.GHToken` and all its consumers (`git.NewRepoManager`, `env["GH_TOKEN"]`, `CreatePrPoster`, `CreateReviewVerifier`, `CreateAgent`) stay exactly as-is. The field is named `GHToken` for historical reasons but now carries the resolved installation token; do NOT rename it (out of scope, and renaming a public field ripples needlessly). Leave `runner.go`, `pkg/githubauth/`, `pkg/git/repo_manager.go`, `pkg/github/client.go`, and `pkg/steps_gh_token.go` unchanged — they consume the *resolved* token / *output* env, not the removed input.

8. **Add an "App credentials absent → startup error" test for the Kafka entry point.** Add a spec that constructs an `application{}` with no App fields set and calls `resolveAuth(context.Background())`, asserting the returned error is non-nil and its message `ContainSubstring("APP_ID")` and `Not(ContainSubstring("GH_TOKEN"))`. Use `context.Background()` for `ctx` (test code; allowed). This spec is fully hermetic (it errors before any GitHub call) and covers the new "App-or-error" branch.

   **Do NOT add a `resolveAuth` App-mode *success* test here.** Unlike the watchers (which use `githubapp.NewClient`, an auto-refreshing transport that constructs without a network call), pr-reviewer's `resolveAuth` calls `lib/githubapp.MintIAT`, which performs a **live installation-token mint over HTTP** (`transport.Token(ctx)` against `api.github.com`). A locally-generated RSA PEM passes key parsing but the mint then fails (network/401) in a hermetic test, and `resolveAuth` exposes no `BaseURL` seam to inject an `httptest.Server` (adding one would touch the frozen App path — out of scope). The App-mode mint success path is already covered by `lib/githubapp`'s own httptest-backed `MintIAT` tests (frozen). **For spec AC #11 evidence (per-service App-mode success test):** cite the frozen `lib/githubapp` `MintIAT` httptest suite as the App-mode success coverage for pr-reviewer, and note in the implementation output that a `resolveAuth`-level success test is intentionally omitted (no injectable mint endpoint; would require modifying the frozen App path).

   **Test-package placement (verify before writing):** `resolveAuth` is unexported, so the absent-creds spec needs white-box access (`package main`). The existing `agent/pr-reviewer/main_test.go` is `package main_test` (black-box) — it CANNOT reach unexported symbols, so do NOT add the spec there. Instead create a new file `agent/pr-reviewer/auth_resolve_test.go` declared `package main`, holding its own `Describe(...)`/`It(...)` blocks. Go permits both `main` and `main_test` test files in one directory; the Ginkgo bootstrap (`RunSpecs` in `TestSuite`) already lives in `main_test.go`, so do NOT add a second `RunSpecs` — the new `package main` file contributes specs to the same suite run. (Confirm `main_test.go`'s actual package clause first; if it is in fact `package main`, you may add the spec there directly — but the white-box requirement is the invariant, not the file.)

9. **Verify the local-CLI entry point.** `cmd/run-task` has no Ginkgo suite of its own for `resolveAuth` today. Do NOT add a new suite/test file there unless `make precommit` coverage gate requires it. If the coverage gate drops below the module's existing threshold after removing `authmode_test.go`, add a minimal absent-creds test for `cmd/run-task`'s `Run` (or restore coverage by other means) — but only if the gate actually fails.

10. **Run `make precommit` from the service directory:**

    ```bash
    cd /workspace/agent/pr-reviewer && make precommit
    ```

</requirements>

<constraints>
- `github.com/bborbe/maintainer` is a multi-module mono-repo; `agent/pr-reviewer` is its own Go module. Build and verify ONLY in `agent/pr-reviewer/` with `make precommit`.
- Error construction: `github.com/bborbe/errors` context-form (`errors.Errorf(ctx, …)`, `errors.Wrap(ctx, …)`) ONLY. No `fmt.Errorf`. No `context.Background()` in business logic (test code may use it).
- Tests: Ginkgo v2 + Gomega. The existing `main_test.go` is `package main_test` (black-box) and keeps its single Ginkgo `RunSpecs` bootstrap. The new white-box spec that reaches unexported `resolveAuth`/`resolvedToken` goes in a separate `package main` file (`auth_resolve_test.go`) — see requirement 8. Verify the actual package clause before writing.
- The GitHub App auth path is frozen: `githubapp.MintIAT` usage, the App-mode `glog.V(2)` log line, and the resolved token value must not change behavior.
- Do NOT re-add a config flag, opt-out, or env toggle to re-enable PAT auth.
- Do NOT modify k8s manifests in THIS prompt (the pr-reviewer k8s yaml already omits a live `GH_TOKEN` env — its only `GH_TOKEN` reference is a historical comment, handled in the watcher/stale-env prompt is out of scope here; leave it).
- Do NOT rename `RunConfig.GHToken` — it carries the resolved token now; renaming is out of scope.
- Removed dead branches must not drop the module below its existing coverage gate.
- Do NOT commit — dark-factory handles git.
- Existing tests (other than the deleted PAT-fallback tests) must still pass.
</constraints>

<verification>
```bash
# 1. Full module gate
cd /workspace/agent/pr-reviewer && make precommit

# 2. No GH_TOKEN env input tag remains
grep -rn 'env:"GH_TOKEN"' /workspace/agent/pr-reviewer
# Expect: zero matches (exit 1)

# 3. No GHToken identifier in non-test Go files (RunConfig.GHToken is in runner.go — see note)
grep -rn 'GHToken' --include='*.go' /workspace/agent/pr-reviewer | grep -v _test.go
# Expect: ONLY the ~6 lines in pkg/factory/runner.go (the `RunConfig.GHToken` field decl +
# its `cfg.GHToken` forwarding consumers — the resolved-token carrier, intentionally kept).
# There must be ZERO `a.GHToken` in main.go or cmd/run-task/main.go, and ZERO env:"GH_TOKEN".
# (This grep is expected to return a NON-empty list — that is correct, unlike greps 2/4/5.)

# 3b. No stale "instead of GH_TOKEN" AppID usage string remains
grep -rn 'instead of GH_TOKEN' /workspace/agent/pr-reviewer
# Expect: zero matches (exit 1)

# 4. No a.GHToken in either entry point
grep -rn 'a\.GHToken' /workspace/agent/pr-reviewer/main.go /workspace/agent/pr-reviewer/cmd/run-task/main.go
# Expect: zero matches (exit 1)

# 5. PAT-fallback machinery gone
grep -rn 'pat-fallback\|App wins\|GH_TOKEN ignored\|AuthModePATFallback\|ResolveAuthMode' /workspace/agent/pr-reviewer
# Expect: zero matches (exit 1)

# 6. Resolved-token forwarding survives: env["GH_TOKEN"] output still set from resolved token
grep -n 'env\["GH_TOKEN"\]' /workspace/agent/pr-reviewer/main.go
# Expect: 1 match, assigned from a.resolvedToken

# 7. Resolved token reaches gh-auth-setup, repo manager, and agent provider in dispatchAgent
grep -n 'NewGhAuthSetupGit\|NewRepoManager\|CreateAgentProvider' /workspace/agent/pr-reviewer/main.go
# Expect: each present and passed a.resolvedToken (verify by reading the lines)

# 8. Startup error names APP_ID, not GH_TOKEN
grep -rni 'APP_ID' /workspace/agent/pr-reviewer/main.go /workspace/agent/pr-reviewer/cmd/run-task/main.go
# Expect: the auth-not-configured error string names APP_ID

# 9. authmode.go and its test are deleted
ls /workspace/agent/pr-reviewer/pkg/authmode.go /workspace/agent/pr-reviewer/pkg/authmode_test.go 2>&1
# Expect: both "No such file"
```
</verification>
