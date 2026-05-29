---
status: approved
spec: ["052"]
created: "2026-05-29T18:32:00Z"
queued: "2026-05-29T18:24:44Z"
---

<summary>
- The `github-build` and `github-release` watchers no longer accept a `GH_TOKEN` PAT as an auth input — both authenticate to GitHub only via the GitHub App installation token.
- When App credentials are missing or incomplete, each watcher refuses to start with a clear error naming the App env vars and never offers `GH_TOKEN` as an alternative.
- Both services have a long-lived binary AND a `cmd/run-once` smoke-test binary; the `GH_TOKEN` input is removed from both binaries of each service.
- The shared auth resolver in each service (`pkg/auth`) drops its PAT-fallback branch and the "App wins; GH_TOKEN ignored" warning.
- The GitHub App client construction (auto-refreshing installation token) and `github-release`'s existing partial-App-config error are unchanged.
- PAT-fallback test assertions are removed; "App credentials absent → startup error" coverage exists and passes for each service.
</summary>

<objective>
Make `watcher/github-build` and `watcher/github-release` (each: long-lived binary + `cmd/run-once` binary) authenticate to GitHub exclusively via the GitHub App installation token, removing the `GH_TOKEN` PAT input and the PAT-fallback branch from each service's shared `pkg/auth` resolver, while preserving App-client construction and `github-release`'s partial-config error.
</objective>

<context>
Read `CLAUDE.md` at the repo root and in each service directory before editing.

Read these guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`

Read these source files before editing. The two services have DIFFERENT resolver shapes — do NOT assume they are identical:

**github-build (resolver: `pkg/auth/auth.go` `Resolve(ctx, Config) (*http.Client, error)`):**
- `watcher/github-build/pkg/auth/auth.go` — `Config{AppID, InstallationID int64; PEMKeyFile, PEMKey, GHToken, LogPrefix string}`. Logic: `useGitHubApp := AppID!=0 && InstallationID!=0 && (PEMKeyFile!="" || PEMKey!="")`; warning if both App+GHToken; then `switch { case useGitHubApp: ...githubapp.NewClient... ; case GHToken!="": ...tokenTransport... ; default: error }`. **This resolver has NO partial-App-config error** — partial App config silently falls through to PAT/error. After removing the PAT branch, partial App config will hit the "not configured" error, which is acceptable (the spec freezes only the partial-config error *where one already exists*; github-build has none to preserve).
- `watcher/github-build/pkg/auth/auth_test.go` — has "PAT fallback mode", "Conflict mode" (both-set warning), "Refusal mode" (neither), "Missing PEMKeyFile" specs. The PAT-fallback and Conflict specs assert removed behavior.
- `watcher/github-build/main.go` — `application` struct fields `GHToken`, `AppID`, `InstallationID`, `PEMKeyFile`, `PEMKey`; calls `auth.Resolve(ctx, auth.Config{... GHToken: a.GHToken, LogPrefix: "watcher/github-build"})` in `Run`.
- `watcher/github-build/cmd/run-once/main.go` — `Application` struct with the same fields; calls `auth.Resolve(ctx, auth.Config{... GHToken: a.GHToken, LogPrefix: "watcher/github-build-run-once"})`.
- `watcher/github-build/cmd/run-once/main_test.go` — sets `GHToken: "fake-token"` in a test fixture `Application`.

**github-release (resolver: `pkg/auth/auth.go` `ResolveGitHubClient(ctx, Credentials) (*http.Client, error)`):**
- `watcher/github-release/pkg/auth/auth.go` — `Credentials{AppID, InstallationID int64; PEMKey []byte; Token string}`. Logic: `appPartial`/`appComplete` (PEMKey via `len`); **partial-config error EXISTS here** ("partial GitHub App config — missing %v") → KEEP it; then `if appComplete { warn-if-Token... githubapp.NewClient... }`; then `if creds.Token != "" { ...oauth2 PAT... }`; then "neither" error. Uses `golang.org/x/oauth2` for the PAT path.
- `watcher/github-release/pkg/auth/auth_test.go` — has "neither" + partial-config specs (KEEP), an "App-backed client" success spec (KEEP), a malformed-PEM spec (KEEP), and "PAT-backed client when only Token set" + "App wins when both App credentials and Token set" specs (REMOVE — removed behavior). Uses a `generateTestPEM()` helper.
- `watcher/github-release/main.go` — `application` struct fields `AppID`, `InstallationID`, `PEMKey`, `GHToken`; calls `auth.ResolveGitHubClient(ctx, auth.Credentials{AppID, InstallationID, PEMKey: []byte(a.PEMKey), Token: a.GHToken})`.
- `watcher/github-release/cmd/run-once/main.go` — `Application` struct with the same fields; calls `auth.ResolveGitHubClient(ctx, auth.Credentials{... Token: a.GHToken})`.
- `watcher/github-release/cmd/run-once/main_test.go` — sets `GHToken: "fake-token"` in a test fixture.

**Frozen across both:** the App-mode log line, `githubapp.NewClient` usage (auto-refreshing — do NOT switch to `MintIAT`), and `github-release`'s partial-App-config error.
</context>

<requirements>

## Part A — watcher/github-build

1. **`watcher/github-build/pkg/auth/auth.go` — `Config` + `Resolve`:** Remove the `GHToken string` field from `Config`. In `Resolve`, remove the `if cfg.GHToken != "" && useGitHubApp { glog.Warningf("...App wins; GH_TOKEN ignored") }` block and the `case cfg.GHToken != "":` PAT branch (the `tokenTransport` client). Make the `default` branch fire whenever App is not complete. New shape:

   ```go
   func Resolve(ctx context.Context, cfg Config) (*http.Client, error) {
       hasPEMFile := cfg.PEMKeyFile != ""
       hasPEMContent := cfg.PEMKey != ""
       useGitHubApp := cfg.AppID != 0 && cfg.InstallationID != 0 && (hasPEMFile || hasPEMContent)
       if !useGitHubApp {
           return nil, errors.Errorf(
               ctx,
               "%s auth: GitHub App credentials not configured — set APP_ID, INSTALLATION_ID, and PEM_KEY_FILE (or PEM_KEY)",
               cfg.LogPrefix,
           )
       }
       appCfg := githubapp.Config{AppID: cfg.AppID, InstallationID: cfg.InstallationID}
       if hasPEMFile {
           pemBytes, err := os.ReadFile(cfg.PEMKeyFile)
           if err != nil {
               return nil, errors.Wrapf(ctx, err, "read PEM file %s", cfg.PEMKeyFile)
           }
           appCfg.PEM = pemBytes
       } else {
           appCfg.PEM = []byte(cfg.PEMKey)
       }
       httpClient, err := githubapp.NewClient(ctx, appCfg)
       if err != nil {
           return nil, errors.Wrap(ctx, err, "create githubapp client")
       }
       glog.V(2).Infof("%s auth mode=github-app app_id=%d installation_id=%d",
           cfg.LogPrefix, cfg.AppID, cfg.InstallationID)
       return httpClient, nil
   }
   ```

   Then **delete the `tokenTransport` type and its `RoundTrip` method** (now unreferenced). The error string MUST contain `APP_ID` and MUST NOT contain `GH_TOKEN`. Remove any import that becomes unused after dropping `tokenTransport`.

2. **`watcher/github-build/main.go`:** Remove the `GHToken` field from the `application` struct (the `env:"GH_TOKEN"` line). In the `auth.Resolve(ctx, auth.Config{...})` call, remove the `GHToken: a.GHToken` line. Keep `AppID`, `InstallationID`, `PEMKeyFile`, `PEMKey`, `LogPrefix`. **Also reword the `AppID` field's `usage:` struct tag — it currently says "GitHub App ID (numeric); when set, App auth is used instead of GH_TOKEN", which now misstates behavior. Change it so it does not mention `GH_TOKEN` (e.g. `usage:"GitHub App ID (numeric); required for App auth"`).** (Note: no verification grep elsewhere catches this string — it is not a `GHToken` identifier nor a `pat-fallback` token — so it must be fixed deliberately per spec Desired Behavior 6.)

3. **`watcher/github-build/cmd/run-once/main.go`:** Same as step 2 for the `Application` struct and its `auth.Resolve(ctx, auth.Config{...})` call (`LogPrefix: "watcher/github-build-run-once"`). Remove the `GHToken` field and the `GHToken: a.GHToken` line, **and reword the same `AppID` "instead of GH_TOKEN" `usage:` string here too.**

4. **`watcher/github-build/pkg/auth/auth_test.go`:** Remove the `Describe("PAT fallback mode", ...)` and `Describe("Conflict mode", ...)` blocks (both assert removed behavior). KEEP "Refusal mode" (neither configured → error) — verify its assertion still holds; it currently expects the error to contain `APP_ID` and `GH_TOKEN`. Update it: the new error contains `APP_ID` but NOT `GH_TOKEN`, so change the `ContainSubstring("GH_TOKEN")` assertion to `Not(ContainSubstring("GH_TOKEN"))` (or remove that line and keep the `APP_ID` assertion). KEEP "Missing PEMKeyFile" (App-complete path with bad PEM file → error). **Add a positive App-mode success spec — `watcher/github-build` currently has NONE (its specs are PAT-fallback / Conflict / Refusal / Missing-PEMKeyFile only), and after deleting the PAT + Conflict specs the App success branch (`githubapp.NewClient`) would have zero direct coverage, which spec Failure-Mode row 5 forbids. This spec is mandatory, not conditional:** `auth.Resolve(ctx, auth.Config{AppID: 1, InstallationID: 2, PEMKey: <valid in-test RSA PEM>, LogPrefix: "test"})` returns a non-nil client and no error. Generate the PEM in-test with `rsa.GenerateKey(rand.Reader, 2048)` + `x509.MarshalPKCS1PrivateKey` + `pem.EncodeToMemory` (block type `RSA PRIVATE KEY`).

5. **`watcher/github-build/cmd/run-once/main_test.go`:** Remove the `GHToken: "fake-token"` line from the test fixture `Application` (the field no longer exists). The fixture's other fields stay. If the test relied on PAT auth succeeding, ensure it now supplies App creds or does not exercise the auth path (read the test — `auth.Resolve` is mocked via the `CreateWatcher` factory field, so the fixture likely never calls real auth; just drop the dead field).

## Part B — watcher/github-release

6. **`watcher/github-release/pkg/auth/auth.go` — `Credentials` + `ResolveGitHubClient`:** Remove the `Token string` field from `Credentials`. In `ResolveGitHubClient`, KEEP the `appPartial && !appComplete` partial-config error block (frozen). Remove the `if creds.Token != "" { glog.Warningf("...App wins; GH_TOKEN ignored") }` line inside the `appComplete` branch, and remove the entire `if creds.Token != "" { ...oauth2 PAT... }` fallback block. Replace the final "neither App nor PAT configured" error with an App-only error. New tail shape (the partial-config block above stays unchanged):

   ```go
   if appComplete {
       glog.Infof(
           "watcher auth mode=github-app app_id=%d installation_id=%d",
           creds.AppID, creds.InstallationID,
       )
       client, err := githubapp.NewClient(ctx, githubapp.Config{
           AppID:          creds.AppID,
           InstallationID: creds.InstallationID,
           PEM:            creds.PEMKey,
       })
       if err != nil {
           return nil, errors.Wrap(ctx, err, "create github app client")
       }
       return client, nil
   }
   return nil, errors.Errorf(
       ctx,
       "watcher auth: GitHub App credentials not configured — set APP_ID, INSTALLATION_ID, and PEM_KEY",
   )
   ```

   The error string MUST contain `APP_ID` and MUST NOT contain `GH_TOKEN`. Remove the `golang.org/x/oauth2` import (now unused after dropping the PAT branch). Update the function doc comment so it no longer describes the "Only Token set → PAT fallback" rule.

7. **`watcher/github-release/main.go`:** Remove the `GHToken` field from the `application` struct. In the `auth.ResolveGitHubClient(ctx, auth.Credentials{...})` call, remove the `Token: a.GHToken` line. Keep `AppID`, `InstallationID`, `PEMKey`.

8. **`watcher/github-release/cmd/run-once/main.go`:** Same as step 7 for the `Application` struct and its `auth.ResolveGitHubClient` call. Remove the `GHToken` field and the `Token: a.GHToken` line.

9. **`watcher/github-release/pkg/auth/auth_test.go`:** Remove the `It("returns PAT-backed client when only Token set", ...)` and `It("App wins when both App credentials and Token set", ...)` specs (removed behavior). KEEP: the "neither App nor PAT configured" spec (verify its `ContainSubstring("neither App nor PAT configured")` still matches — if you changed the error wording in step 6, update the assertion to `ContainSubstring("not configured")` and `ContainSubstring("APP_ID")`), the three partial-config specs (frozen), the "App-backed client when all three App fields set" success spec (frozen), and the "malformed PEM" spec (frozen). The `generateTestPEM()` helper stays (still used by the App-success and App-wins... — App-success only after removing App-wins). Ensure no unused import or unused helper remains.

10. **`watcher/github-release/cmd/run-once/main_test.go`:** Remove the `GHToken: "fake-token"` line from the test fixture (the field no longer exists). Same reasoning as step 5.

## Both services

11. **Run `make precommit` from each service directory:**

    ```bash
    cd /workspace/watcher/github-build && make precommit
    cd /workspace/watcher/github-release && make precommit
    ```

</requirements>

<constraints>
- `watcher/github-build` and `watcher/github-release` are SEPARATE Go modules. Build and verify each independently with `make precommit` in its own directory.
- Error construction: `github.com/bborbe/errors` context-form ONLY. No `fmt.Errorf`. No `context.Background()` in business logic (test code may use it).
- Tests: Ginkgo v2 + Gomega, external `_test` packages.
- Frozen: App-mode log line, `githubapp.NewClient` usage (auto-refreshing — NOT `MintIAT`), and `github-release`'s partial-App-config error. `github-build` has no partial-config error to preserve (its partial config now hits the "not configured" error — acceptable).
- Do NOT re-add a config flag, opt-out, or env toggle to re-enable PAT auth.
- Do NOT modify k8s manifests in this prompt (handled in the stale-env-wiring prompt).
- Removed dead branches must not drop either module below its existing coverage gate.
- Do NOT commit — dark-factory handles git.
- Existing tests (other than the deleted PAT tests) must still pass.
</constraints>

<verification>
```bash
# 1. Per-module gates
cd /workspace/watcher/github-build && make precommit
cd /workspace/watcher/github-release && make precommit

# 2. No GH_TOKEN env input tag in either service
grep -rn 'env:"GH_TOKEN"' /workspace/watcher/github-build /workspace/watcher/github-release
# Expect: zero matches (exit 1)

# 3. No GHToken / Token-as-input identifier in non-test Go files
grep -rn 'GHToken' --include='*.go' /workspace/watcher/github-build /workspace/watcher/github-release | grep -v _test.go
# Expect: zero matches (exit 1)

# 4. PAT-fallback machinery gone
grep -rn 'pat-fallback\|App wins\|GH_TOKEN ignored\|tokenTransport' /workspace/watcher/github-build /workspace/watcher/github-release
# Expect: zero matches (exit 1)

# 5. App-mode construction preserved (githubapp.NewClient, not MintIAT)
grep -rn 'githubapp.NewClient' /workspace/watcher/github-build/pkg/auth/auth.go /workspace/watcher/github-release/pkg/auth/auth.go
# Expect: 1 match each
grep -rn 'MintIAT' /workspace/watcher/github-build /workspace/watcher/github-release
# Expect: zero matches (exit 1)

# 6. Startup errors name APP_ID, not GH_TOKEN
grep -rni 'APP_ID' /workspace/watcher/github-build/pkg/auth/auth.go /workspace/watcher/github-release/pkg/auth/auth.go
# Expect: the not-configured error string names APP_ID in each

# 7. github-release partial-config error preserved
grep -n 'partial GitHub App config' /workspace/watcher/github-release/pkg/auth/auth.go
# Expect: 1 match

# 8. No stale "instead of GH_TOKEN" AppID usage string remains (github-build, both binaries)
grep -rn 'instead of GH_TOKEN' /workspace/watcher/github-build /workspace/watcher/github-release
# Expect: zero matches (exit 1)
```
</verification>
