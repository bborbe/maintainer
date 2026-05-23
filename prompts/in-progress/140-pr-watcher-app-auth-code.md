---
status: approved
spec: [037-migrate-pr-watcher-to-github-app]
created: "2026-05-23T21:38:55Z"
queued: "2026-05-23T21:41:43Z"
branch: fix/pr-watcher-github-app-auth-code
---

<summary>
- `watcher/github-pr/pkg/githubclient.go`: `NewGitHubClient(token string)` → `NewGitHubClient(httpClient *http.Client)`. The injected client already carries auth, so drop `.WithAuthToken`.
- `watcher/github-pr/pkg/factory/factory.go`: add `AuthConfig` struct + `CreateGitHubHTTPClient(ctx, AuthConfig) (*http.Client, error)` helper. App auth via `lib/githubapp.NewClient` (auto-refreshing IAT — long-lived watcher); PAT fallback via `golang.org/x/oauth2.NewClient`; **partial App env returns an error naming the missing fields**.
- `CreateWatcher` parameter `ghToken string` → `auth AuthConfig`. Calls the new helper, runs cleanup before propagating the error.
- `watcher/github-pr/main.go`: `GHToken` becomes `required:"false"`. Add 3 new optional fields `AppID`, `InstallationID`, `PEMKey`. Update `factory.CreateWatcher` call site to pass `AuthConfig`. Pod refuses startup on misconfig (errors propagate → `os.Exit(non-zero)` → CrashLoopBackOff).
- Tests live in `watcher/github-pr/pkg/factory/`: 5 cases (App happy / PAT fallback / both-set warning / neither-set error / partial-set error names missing fields). Create `factory_suite_test.go` with ginkgo bootstrap so specs actually execute.
- INFO-level startup log line (`glog.Infof`, not `V(2)`) declares the chosen auth mode so operators see it in default `kubectl logs`.
</summary>

<objective>
The `watcher/github-pr` binary can authenticate to GitHub as the existing pr-reviewer GitHub App installation using auto-refreshing IATs, with a static-PAT fallback path that still works. Misconfiguration produces a clear, named error at startup. Tests cover all five auth-mode selection branches and pass in CI.
</objective>

<context>
**Why:** the `pr-review-of-ben` PAT is shared by 3 services (pr-reviewer ✅ migrated in PR #6 + #9, watcher/github-pr 🔄 this prompt, watcher/github-build — sibling spec 038). Until both watchers migrate, the PAT cannot be revoked. GitHub Support refused account reinstatement on ToS grounds (one individual = one free user account), so Apps are the only viable path.

**Critical technical decision:** the watcher is a long-lived `StatefulSet` (polls every 5 min forever). IATs expire after 1 hour. So this prompt MUST use `lib/githubapp.NewClient` (returns `*http.Client` with auto-refreshing IAT via `ghinstallation/v2`), NOT `lib/githubapp.MintIAT` (single-shot, used by short-lived `pr-reviewer` Job). The verification step asserts this with a zero-match grep on `MintIAT`.

**Read before editing:**
- `agent/pr-reviewer/main.go` — sibling implementation; lines 82–87 show App env vars, lines 230–266 show auth-mode resolution. **Note divergence**: pr-reviewer uses `PEMKeyFile string` (path). This watcher uses `PEMKey string` (raw PEM env). Don't blindly copy.
- `agent/pr-reviewer/pkg/authmode.go` — `ResolveAuthMode` reference. Mirror the structure but adapt for `PEMKey` raw-bytes field and add the partial-set error path.
- `lib/githubapp/githubapp.go` — `Config{AppID, InstallationID, PEM []byte}` + `NewClient(ctx, cfg)` signature.
- `watcher/github-pr/pkg/githubclient.go:80` — current `NewGitHubClient(token string)`.
- `watcher/github-pr/pkg/factory/factory.go:50` — current `CreateWatcher(ctx, ghToken string, ...)` with single caller of `NewGitHubClient` at line 70.
- `watcher/github-pr/main.go:104-122` — `application` struct; `GHToken required:"true"` at line 109. Caller of `CreateWatcher` at lines 189-201.
- `watcher/github-pr/pkg/suite_test.go` — ginkgo bootstrap reference (lines 5-25) for the new `factory_suite_test.go`.

**Historical reference:** rejected dark-factory spec `037-migrate-pr-watcher-to-github-app` (in `specs/rejected/`) — its Failure Modes table and Constraints section are the source of truth for behaviors enforced below.
</context>

<requirements>

1. **Update `watcher/github-pr/pkg/githubclient.go`** — change `NewGitHubClient` to accept an authenticated `*http.Client`:

   ```go
   import "net/http"

   // NewGitHubClient returns a GitHubClient backed by the real GitHub API.
   // The httpClient must already carry authentication (either App auth via
   // lib/githubapp.NewClient, or static-PAT via oauth2.NewClient).
   func NewGitHubClient(httpClient *http.Client) GitHubClient {
       return &githubClient{
           client: gogithub.NewClient(httpClient),
       }
   }
   ```

   The existing call site is `pkg/factory/factory.go:70` (the only one — verify via `rg 'NewGitHubClient\(' watcher/github-pr/`). The export-test helper `NewForTest` in `githubclient_export_test.go` takes `*gogithub.Client` and is unaffected.

2. **Update `watcher/github-pr/pkg/factory/factory.go`** — add the `AuthConfig` struct and `CreateGitHubHTTPClient` helper:

   ```go
   import (
       "net/http"
       "golang.org/x/oauth2"
       githubapp "github.com/bborbe/maintainer/lib/githubapp"
   )

   // AuthConfig selects GitHub auth mode. When AppID + InstallationID + PEMKey
   // are all set, App auth is used with auto-refreshing IATs (required because
   // the watcher is long-lived; a one-shot MintIAT would expire after 1 hour).
   // When only GHToken is set, a static-PAT oauth2 client is returned as a
   // fallback. Partial App-env config returns an error naming the missing
   // fields so operators see the misconfig in kubectl logs immediately.
   type AuthConfig struct {
       AppID          int64
       InstallationID int64
       PEMKey         string // PEM content (env), not a file path
       GHToken        string
   }

   // CreateGitHubHTTPClient returns an authenticated *http.Client.
   func CreateGitHubHTTPClient(ctx context.Context, cfg AuthConfig) (*http.Client, error) {
       appPartial := (cfg.AppID != 0) || (cfg.InstallationID != 0) || (cfg.PEMKey != "")
       appComplete := (cfg.AppID != 0) && (cfg.InstallationID != 0) && (cfg.PEMKey != "")
       if appPartial && !appComplete {
           var missing []string
           if cfg.AppID == 0 {
               missing = append(missing, "APP_ID")
           }
           if cfg.InstallationID == 0 {
               missing = append(missing, "INSTALLATION_ID")
           }
           if cfg.PEMKey == "" {
               missing = append(missing, "PEM_KEY")
           }
           return nil, errors.Errorf(
               ctx,
               "watcher auth: partial GitHub App config — missing %v; set all three or none",
               missing,
           )
       }
       if appComplete {
           if cfg.GHToken != "" {
               glog.Warningf(
                   "watcher auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
               )
           }
           httpClient, err := githubapp.NewClient(ctx, githubapp.Config{
               AppID:          cfg.AppID,
               InstallationID: cfg.InstallationID,
               PEM:            []byte(cfg.PEMKey),
           })
           if err != nil {
               return nil, errors.Wrap(ctx, err, "github app client")
           }
           glog.Infof(
               "watcher auth mode=github-app app_id=%d installation_id=%d",
               cfg.AppID, cfg.InstallationID,
           )
           return httpClient, nil
       }
       if cfg.GHToken != "" {
           glog.Warningf("watcher auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
           ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GHToken})
           return oauth2.NewClient(ctx, ts), nil
       }
       return nil, errors.Errorf(
           ctx,
           "watcher auth: neither App nor PAT configured — set APP_ID + INSTALLATION_ID + PEM_KEY, or set GH_TOKEN",
       )
   }
   ```

   Change `CreateWatcher` signature: replace `ghToken string` (parameter position 2) with `auth AuthConfig`. Body change at line 70: replace `ghClient := pkg.NewGitHubClient(ghToken)` with:

   ```go
   httpClient, err := CreateGitHubHTTPClient(ctx, auth)
   if err != nil {
       cleanup()
       return nil, nil, err
   }
   ghClient := pkg.NewGitHubClient(httpClient)
   ```

   (Note: `cleanup()` from `CreateKafkaSender` must run before the early return so the Kafka producer doesn't leak.)

3. **Update `watcher/github-pr/main.go`** — change `GHToken` to optional + add 3 App fields + update `CreateWatcher` call site:

   - Line 109: `GHToken string … required:"true"` → `required:"false"`, update usage to "GitHub PAT (legacy fallback when App credentials are not set)".
   - Add 3 fields under `GHToken` (mirror pr-reviewer's struct tags exactly for consistency):

     ```go
     AppID          int64  `required:"false" arg:"app-id"          env:"APP_ID"          usage:"GitHub App ID (numeric); when set with InstallationID + PEMKey, App auth is used instead of GH_TOKEN"`
     InstallationID int64  `required:"false" arg:"installation-id" env:"INSTALLATION_ID" usage:"GitHub App Installation ID (numeric)"`
     PEMKey         string `required:"false" arg:"pem-key"         env:"PEM_KEY"         usage:"GitHub App private key (PEM content from k8s Secret envFrom)" display:"length"`
     ```

   - Update the `factory.CreateWatcher` call at line 189: replace the `a.GHToken` argument with `factory.AuthConfig{AppID: a.AppID, InstallationID: a.InstallationID, PEMKey: a.PEMKey, GHToken: a.GHToken}`. The argument list is now `(ctx, factory.AuthConfig{...}, a.KafkaBrokers, a.Stage, ...)`.

   Do NOT add any other startup logging — `CreateGitHubHTTPClient` already emits the auth-mode log line.

4. **Create `watcher/github-pr/pkg/factory/factory_suite_test.go`** — ginkgo bootstrap (without this, specs in the new `factory_test` package will not run):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package factory_test

   import (
       "testing"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   func TestFactory(t *testing.T) {
       RegisterFailHandler(Fail)
       RunSpecs(t, "Factory Suite")
   }
   ```

5. **Create `watcher/github-pr/pkg/factory/githubauth_test.go`** — table-driven Ginkgo tests for `CreateGitHubHTTPClient`. Test cases (use `DescribeTable` / `Entry`):

   - **App happy path:** `AuthConfig{AppID: 1, InstallationID: 2, PEMKey: <valid PEM>}` returns a non-nil `*http.Client`, no error. Generate the PEM in-test with `rsa.GenerateKey(rand.Reader, 2048)` + `x509.MarshalPKCS1PrivateKey` + `pem.EncodeToMemory` — `ghinstallation/v2` rejects bogus PEM strings at parse time.
   - **PAT fallback:** `AuthConfig{GHToken: "ghp_test"}` returns a non-nil `*http.Client`, no error.
   - **Both set (App wins):** `AuthConfig{AppID: 1, InstallationID: 2, PEMKey: <valid PEM>, GHToken: "ghp_test"}` returns a non-nil `*http.Client`, no error.
   - **Neither set:** `AuthConfig{}` returns nil + error containing "neither App nor PAT".
   - **Partial App config — only AppID:** `AuthConfig{AppID: 1}` returns nil + error containing "INSTALLATION_ID" and "PEM_KEY".
   - **Partial App config — missing PEM:** `AuthConfig{AppID: 1, InstallationID: 2}` returns nil + error containing "PEM_KEY".

   Use `context.Background()` for `ctx`. Package is `factory_test` (external test package). Assertion library: gomega.

6. **Run from the service dir:**

   ```bash
   cd /workspace/watcher/github-pr && make precommit
   ```

</requirements>

<constraints>
- Files edited: `pkg/githubclient.go`, `pkg/factory/factory.go`, `main.go`.
- Files created: `pkg/factory/factory_suite_test.go`, `pkg/factory/githubauth_test.go`.
- BSD-style license header on every new `.go` file (copy from `pkg/factory/factory.go`).
- Error wrapping: `github.com/bborbe/errors` exclusively — never `fmt.Errorf`.
- **`lib/githubapp.NewClient` only — never `MintIAT`.** Watcher is long-lived (StatefulSet).
- PAT fallback uses `golang.org/x/oauth2.NewClient` (which IS authenticated). Do NOT use `gogithub.NewClient(nil).WithAuthToken(t).Client()` — that returns an unauthenticated `http.DefaultClient` and produces silent 401s.
- Partial App env config returns an error naming the missing fields. Silently downgrading to PAT (or to "neither") is forbidden by the spec failure-modes contract.
- Startup auth-mode log is at `glog.Infof` (default level) so it shows up in `kubectl logs` without `-v`.
- No `panic`, no `glog.Fatal`. Misconfig propagates as an error from `application.Run` → `os.Exit(non-zero)` → CrashLoopBackOff.
- Do NOT touch the K8s manifests, `dev.env`, `prod.env`, or `CHANGELOG.md` — those ship in the sibling prompt.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
# 1. Compile + lint + tests
cd /workspace/watcher/github-pr && make precommit

# 2. NewClient (not MintIAT) — the load-bearing invariant
grep -c 'githubapp\.NewClient' /workspace/watcher/github-pr/pkg/factory/factory.go
# Expect: >= 1
grep -c 'githubapp\.MintIAT' /workspace/watcher/github-pr/pkg/factory/factory.go
# Expect: 0

# 3. Signature changed
grep -n 'func NewGitHubClient' /workspace/watcher/github-pr/pkg/githubclient.go
# Expect: "func NewGitHubClient(httpClient *http.Client) GitHubClient"

# 4. AuthConfig + helper present
grep -n 'type AuthConfig struct\|func CreateGitHubHTTPClient' /workspace/watcher/github-pr/pkg/factory/factory.go
# Expect: 2 matches

# 5. Three App env fields in main.go
grep -nE 'env:"(APP_ID|INSTALLATION_ID|PEM_KEY)"' /workspace/watcher/github-pr/main.go
# Expect: 3 matches

# 6. INFO-level startup log (not V(2))
grep -n 'glog.Infof.*auth mode=github-app' /workspace/watcher/github-pr/pkg/factory/factory.go
# Expect: 1 match

# 7. Suite bootstrap exists so the new tests actually execute
ls /workspace/watcher/github-pr/pkg/factory/factory_suite_test.go
# Expect: file present

# 8. Partial-set error path is tested
grep -c 'Partial\|partial.*App\|INSTALLATION_ID.*missing\|missing.*INSTALLATION_ID' /workspace/watcher/github-pr/pkg/factory/githubauth_test.go
# Expect: >= 1
</verification>
