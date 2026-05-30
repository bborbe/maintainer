---
status: completed
spec: [052-fleet-app-only-auth]
summary: Removed GH_TOKEN PAT authentication from watcher/github-pr; authenticate exclusively via GitHub App installation token
container: maintainer-fleet-app-auth-exec-208-spec-052-github-pr-watcher-app-only
dark-factory-version: v0.173.0
created: "2026-05-29T18:31:00Z"
queued: "2026-05-29T18:24:44Z"
started: "2026-05-29T18:29:44Z"
completed: "2026-05-29T18:32:08Z"
---

<summary>
- The `github-pr` watcher no longer accepts a `GH_TOKEN` PAT as an auth input — it authenticates to GitHub only via the GitHub App installation token.
- When App credentials are missing or incomplete, the watcher refuses to start with a clear error naming the App env vars (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY`) and never offers `GH_TOKEN` as an alternative.
- The watcher previously read `GH_TOKEN` directly from the environment AND declared a `GHToken` config field; both are removed.
- The GitHub App client construction (auto-refreshing installation token) and the existing partial-App-config error are unchanged.
- The legacy PAT-fallback branch and the "App wins; GH_TOKEN ignored" warning are removed.
- A test proving "App credentials absent → startup error" is added; any PAT-fallback test assertions are removed.
</summary>

<objective>
Make `watcher/github-pr` authenticate to GitHub exclusively via the GitHub App installation token, removing both the `GHToken` config field and the direct `os.Getenv("GH_TOKEN")` read, while preserving the App-client construction and the existing partial-App-config error.
</objective>

<context>
Read `CLAUDE.md` at the repo root and in `watcher/github-pr/` before editing.

Read these guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`

Read these source files before editing:
- `watcher/github-pr/main.go` — `application` struct (fields `GHToken`, `AppID`, `InstallationID`, `PEMKey`), the `getEnvInt` helper, and `resolveAuth` (which reads `os.Getenv("PEM_KEY")` / `os.Getenv("GH_TOKEN")` and `getEnvInt("APP_ID")` / `getEnvInt("INSTALLATION_ID")` directly — it does NOT use the struct fields). The call site is `httpClient, err := a.resolveAuth(ctx)` in `Run`.
- `watcher/github-pr/pkg/factory/factory.go` — `CreateGitHubAppClient(ctx, appID, installationID int64, pemKey []byte) (*http.Client, error)` (keep) and `CreateGitHubPATClient(ctx, token string) *http.Client` (this becomes dead after the PAT branch is removed — see requirement 4).
- `watcher/github-pr/pkg/factory/githubauth_test.go` — currently tests both `CreateGitHubAppClient` and `CreateGitHubPATClient`.

**Note (important nuance):** `github-pr`'s `resolveAuth` is the ONLY one of the three watchers that reads `GH_TOKEN` via `os.Getenv` AND also declares a `GHToken` struct field (spec Desired Behavior 1 calls this out explicitly). Both must go. The watcher uses `factory.CreateGitHubAppClient` which wraps `githubapp.NewClient` (auto-refreshing transport — correct for a long-lived poller; do NOT switch to `MintIAT`).

**Frozen — do NOT change:** the partial-App-config error (`appPartial && !appComplete` → "partial GitHub App config — missing %v"), the App-mode `glog.Infof` log line, and `factory.CreateGitHubAppClient`. Only the PAT *input* and the PAT-fallback branch are removed.
</context>

<requirements>

1. **`watcher/github-pr/main.go` — `application` struct:** Remove the `GHToken` field (the `env:"GH_TOKEN"` line). Keep `AppID`, `InstallationID`, `PEMKey`. Update the `AppID` usage string if it references `GH_TOKEN` (e.g. "used instead of GH_TOKEN") so it no longer mentions PAT/GH_TOKEN.

2. **`watcher/github-pr/main.go` — `resolveAuth`:** Rewrite to App-auth-or-error. The function reads from env via `getEnvInt`/`os.Getenv` today; **remove the `token := os.Getenv("GH_TOKEN")` line entirely** and remove the PAT-fallback branch (`if token != "" { ... CreateGitHubPATClient ... }`) and the "both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored" warning. KEEP the partial-App-config error block VERBATIM (`appPartial && !appComplete` → the existing `"...partial GitHub App config — missing %v; set all three or none"` string, byte-for-byte as on disk — do NOT reword or retype it) and KEEP the App-mode `glog.Infof` line and the `factory.CreateGitHubAppClient(ctx, appID, installationID, pemKey)` call. New shape:

   ```go
   func (a *application) resolveAuth(ctx context.Context) (*http.Client, error) {
       appID := getEnvInt("APP_ID")
       installationID := getEnvInt("INSTALLATION_ID")
       pemKey := []byte(os.Getenv("PEM_KEY"))

       appPartial := (appID != 0) || (installationID != 0) || (len(pemKey) != 0)
       appComplete := (appID != 0) && (installationID != 0) && (len(pemKey) != 0)

       // ┌─ FROZEN: keep the existing partial-App-config block VERBATIM ───────────┐
       // │ Do NOT retype it from this sample. The on-disk error string is          │
       // │ "...missing %v; set all three or none" — preserve it byte-for-byte      │
       // │ (a message-text change is invisible to `make precommit`). Leave the     │
       // │ `if appPartial && !appComplete { ... return ...errors.Errorf(...) }`    │
       // │ block exactly as it is in watcher/github-pr/main.go today.              │
       // └─────────────────────────────────────────────────────────────────────────┘

       if !appComplete {
           return nil, errors.Errorf(
               ctx,
               "watcher auth: GitHub App credentials not configured — set APP_ID, INSTALLATION_ID, and PEM_KEY",
           )
       }
       glog.Infof(
           "watcher auth mode=github-app app_id=%d installation_id=%d",
           appID, installationID,
       )
       return factory.CreateGitHubAppClient(ctx, appID, installationID, pemKey)
   }
   ```

   The "not configured" error string MUST contain `APP_ID` and MUST NOT contain `GH_TOKEN`.

3. **Confirm `getEnvInt` stays.** `getEnvInt` is still used by `resolveAuth` for `APP_ID` and `INSTALLATION_ID` — leave it. If after the edit `os` is still imported for `os.Getenv("PEM_KEY")` and `os.Exit`, keep the import; if goimports flags an unused import, fix it (it should still be needed).

4. **`watcher/github-pr/pkg/factory/factory.go` — remove the now-dead PAT client constructor.** After requirement 2, `CreateGitHubPATClient` has no production caller (verify: `grep -rn 'CreateGitHubPATClient' watcher/github-pr/ | grep -v _test.go`). Delete `func CreateGitHubPATClient` and, if the `golang.org/x/oauth2` import becomes unused as a result, remove it from the import block. Keep `CreateGitHubAppClient` and all other factory functions.

5. **`watcher/github-pr/pkg/factory/githubauth_test.go` — remove the PAT-client test.** Delete the `Describe("CreateGitHubPATClient", ...)` block (it tests the removed function). Keep the `Describe("CreateGitHubAppClient", ...)` block (it tests the frozen App path, including its error cases). If removing the PAT block leaves an unused import in the test file, remove it.

6. **Add an "App credentials absent → startup error" test.** The new spec needs white-box (`package main`) access because `resolveAuth` is unexported. **Verify the test-file packages first:** `main_test.go` is `package main_test` (black-box) and holds the Ginkgo `RunSpecs` bootstrap — do NOT add the spec there; `validate_test.go` is already `package main` (white-box). Add the spec either to `validate_test.go` or to a new `package main` file (e.g. `auth_resolve_test.go`) with its own `Describe(...)` (no second `RunSpecs`). The spec builds an `application{}` and calls `resolveAuth(ctx)` with `APP_ID`/`INSTALLATION_ID`/`PEM_KEY` unset, asserting the error is non-nil, `ContainSubstring("APP_ID")`, and `Not(ContainSubstring("GH_TOKEN"))`. Because `resolveAuth` reads the process environment (not struct fields), ensure those vars are unset for the assertion — **prefer `GinkgoT().Setenv(...)` (auto-restores after the spec and is parallel/`-race` safe) over manual `os.Unsetenv` + deferred restore**; if you must unset, use `GinkgoT().Setenv("APP_ID", "")` etc. (an empty value makes `getEnvInt` return 0). Use `context.Background()` for `ctx` (test code; allowed).

   For App-mode success coverage: the existing `factory/githubauth_test.go` `CreateGitHubAppClient` cases already cover the App-mode success/PEM-error path (frozen). Do NOT duplicate them. Adding the absent-creds spec above is sufficient for the new behavior.

7. **Run `make precommit` from the service directory:**

   ```bash
   cd /workspace/watcher/github-pr && make precommit
   ```

</requirements>

<constraints>
- `watcher/github-pr` is its own Go module. Build and verify ONLY in `watcher/github-pr/` with `make precommit`.
- Error construction: `github.com/bborbe/errors` context-form ONLY. No `fmt.Errorf`. No `context.Background()` in business logic (test code may use it).
- Tests: Ginkgo v2 + Gomega. `main_test.go` is `package main_test` (black-box, holds `RunSpecs`); `validate_test.go` is `package main` (white-box). The new spec reaching unexported `resolveAuth` must live in a `package main` file — see requirement 6. Verify package clauses before writing.
- Frozen App path: the partial-App-config error, the App-mode `glog.Infof` log line, and `factory.CreateGitHubAppClient` (which uses `githubapp.NewClient`, auto-refreshing — do NOT switch to `MintIAT`) must not change behavior.
- Do NOT re-add a config flag, opt-out, or env toggle to re-enable PAT auth.
- Do NOT modify k8s manifests in this prompt (handled in the stale-env-wiring prompt).
- Removed dead branches must not drop the module below its existing coverage gate.
- Do NOT commit — dark-factory handles git.
- Existing tests (other than the deleted PAT test) must still pass.
</constraints>

<verification>
```bash
# 1. Full module gate
cd /workspace/watcher/github-pr && make precommit

# 2. No GH_TOKEN env input tag
grep -rn 'env:"GH_TOKEN"' /workspace/watcher/github-pr
# Expect: zero matches (exit 1)

# 3. No GHToken identifier in non-test Go files
grep -rn 'GHToken' --include='*.go' /workspace/watcher/github-pr | grep -v _test.go
# Expect: zero matches (exit 1)

# 4. No Getenv("GH_TOKEN")
grep -rn 'Getenv("GH_TOKEN")' /workspace/watcher/github-pr
# Expect: zero matches (exit 1)

# 5. PAT-fallback machinery gone
grep -rn 'pat-fallback\|App wins\|GH_TOKEN ignored\|CreateGitHubPATClient' /workspace/watcher/github-pr
# Expect: zero matches (exit 1)

# 6. App-mode client construction preserved
grep -n 'CreateGitHubAppClient' /workspace/watcher/github-pr/main.go
# Expect: 1 match (still called from resolveAuth)

# 7. Startup error names APP_ID, not GH_TOKEN
grep -rni 'APP_ID' /workspace/watcher/github-pr/main.go
# Expect: the not-configured error string names APP_ID

# 8. Partial-config error preserved (byte-for-byte, incl. "set all three or none")
grep -n 'partial GitHub App config' /workspace/watcher/github-pr/main.go
# Expect: 1 match, still ending "...set all three or none"

# 9. No stale "instead of GH_TOKEN" AppID usage string remains
grep -rn 'instead of GH_TOKEN' /workspace/watcher/github-pr
# Expect: zero matches (exit 1)
```
</verification>
