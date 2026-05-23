---
status: approved
spec: [038-migrate-watcher-github-build-to-github-app]
created: "2026-05-23T21:30:00Z"
queued: "2026-05-23T21:24:11Z"
branch: dark-factory/migrate-watcher-github-build-to-github-app
---

<summary>
- `watcher/github-build/main.go` declares four new App auth env fields (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`, `PEM_KEY`) and downgrades `GH_TOKEN` to `required:"false"`.
- A hybrid auth resolver at startup determines the active mode: App env wins when configured, PAT fallback when App unset, error when neither.
- `pkg/factory.CreateWatcher` changes from `(ghToken string)` to `(httpClient *http.Client)`.
- `pkg.NewGitHubClient` changes from `(token string)` to `(httpClient *http.Client)` — wraps `gogithub.NewClient(httpClient)` directly.
- Auth mode is logged at glog V(2) on startup: `watcher/github-build auth mode=github-app app_id=<id> installation_id=<id>` or `auth mode=pat-fallback`.
- No changes to `GitHubClient` interface methods or watcher poll logic.
</summary>

<objective>
Wire the build watcher to accept GitHub App authentication via `lib/githubapp.NewClient`, thread the auto-refreshing `*http.Client` through the factory into `NewGitHubClient`, and implement the hybrid auth resolver (App wins, PAT fallback, error when neither). Keep the existing poll logic byte-identical.
</objective>

<context>
Read before making changes:

**Existing source:**
```go
// watcher/github-build/main.go lines 45-60 (application struct)
type application struct {
    SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
    SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

    Listen        string           `required:"false" arg:"listen"         env:"LISTEN"         usage:"HTTP listen address (healthz/readiness/metrics/trigger)"                            default:":9090"`
    GHToken       string           `required:"true"  arg:"gh-token"       env:"GH_TOKEN"       usage:"GitHub token (read scope sufficient)"                                                               display:"length"`
    KafkaBrokers  libkafka.Brokers `required:"true"  arg:"kafka-brokers"  env:"KAFKA_BROKERS"  usage:"Comma-separated Kafka broker list"`
    Stage         string           `required:"true"  arg:"stage"          env:"STAGE"          usage:"Deployment stage (dev|prod)"`
    PollInterval  string           `required:"false" arg:"poll-interval"  env:"POLL_INTERVAL"  usage:"Poll interval (Go duration)"                                                        default:"5m"`
    RepoAllowlist string           `required:"true"  arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); MUST be non-empty"`

    BuildAssignee   string `required:"true"  arg:"build-assignee"    env:"TASK_ASSIGNEE" usage:"Frontmatter assignee for published tasks"                    default:"build-fixer-agent"`
    BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"TASK_STATUS"   usage:"Frontmatter status for published tasks"                      default:"next"`
    BuildTaskPhase  string `required:"false" arg:"build-task-phase"  env:"TASK_PHASE"    usage:"Frontmatter phase for published tasks; empty = omit field"`
    MaxTitleLen     int    `required:"false" arg:"max-title-len"     env:"MAX_TITLE_LEN" usage:"Max length of vault task filename (whole title; safety cap)" default:"200"`
}
```

```go
// watcher/github-build/pkg/factory/factory.go lines 48-83 (CreateWatcher)
func CreateWatcher(
    ctx context.Context,
    ghToken string,
    brokers libkafka.Brokers,
    stage string,
    allowlist []string,
    cursorPath string,
    assignee string,
    taskStatus string,
    taskPhase string,
    maxTitleLen int,
) (pkg.Watcher, func(), error) {
    branch := base.Branch(stage)
    createSender, cleanup, err := CreateKafkaCreateSender(ctx, brokers, branch)
    if err != nil {
        return nil, nil, errors.Wrap(ctx, err, "create kafka create sender")
    }
    ghClient := pkg.NewGitHubClient(ghToken)
    // ...
}
```

```go
// watcher/github-build/pkg/githubclient.go lines 76-81 (NewGitHubClient)
func NewGitHubClient(token string) GitHubClient {
    return &githubClient{
        client: gogithub.NewClient(nil).WithAuthToken(token),
    }
}
```

**Reference implementation (from agent/pr-reviewer/main.go):**
```go
// resolveAuth in agent/pr-reviewer/main.go lines 232-281
func (a *application) resolveAuth(ctx context.Context) error {
    hasPEMFile := a.PEMKeyFile != ""
    hasPEMContent := a.PEMKey != ""
    useGitHubApp := a.AppID != 0 && a.InstallationID != 0 && (hasPEMFile || hasPEMContent)

    if a.GHToken != "" && useGitHubApp {
        glog.Warningf(
            "pr-reviewer auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
        )
    }

    switch {
    case useGitHubApp:
        // uses githubapp.MintIAT for one-shot token (appropriate for pr-reviewer's <5 min runtime)
    case a.GHToken != "":
        glog.Warningf("pr-reviewer auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
    default:
        return errors.Errorf(ctx, "pr-reviewer auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY_FILE (or PEM_KEY), or set GH_TOKEN")
    }
    return nil
}
```

**lib/githubapp API (from lib/githubapp/githubapp.go):**
```go
// NewClient returns *http.Client with auto-refreshing IAT transport (use for long-lived pollers)
func NewClient(ctx context.Context, cfg Config) (*http.Client, error)

// Config struct:
type Config struct {
    AppID          int64
    InstallationID int64
    PEM            []byte  // mutually exclusive with PEMPath
    PEMPath        string  // mutually exclusive with PEM
    BaseURL        string  // API base URL (defaults to https://api.github.com)
}
```

Note: The watcher is a long-lived StatefulSet (hours/days runtime) — MUST use `NewClient` (auto-refreshing transport), NOT `MintIAT` (static 1-hour token). This is different from pr-reviewer which is a one-shot job.

Cross-prompt boundary: **tests for `resolveAuth` and `tokenTransport` live in sibling prompt `2-spec-038-add-tests.md`**. Do NOT add test files in this prompt.
</context>

<requirements>
1. **Update `watcher/github-build/pkg/githubclient.go`**:

   Change `NewGitHubClient(token string)` to `NewGitHubClient(httpClient *http.Client)`:
   ```go
   func NewGitHubClient(httpClient *http.Client) GitHubClient {
       return &githubClient{
           client: gogithub.NewClient(httpClient),
       }
   }
   ```
   The `githubClient` struct and ALL interface methods remain unchanged. Only the constructor changes.

2. **Update `watcher/github-build/pkg/factory/factory.go`**:

   Change the first two parameters of `CreateWatcher`:
   ```go
   func CreateWatcher(
       ctx context.Context,
       httpClient *http.Client,  // was ghToken string
       brokers libkafka.Brokers,
       stage string,
       allowlist []string,
       cursorPath string,
       assignee string,
       taskStatus string,
       taskPhase string,
       maxTitleLen int,
   ) (pkg.Watcher, func(), error)
   ```
   Inside the function body, replace `pkg.NewGitHubClient(ghToken)` with `pkg.NewGitHubClient(httpClient)`. All other wiring is unchanged.

   Add `"net/http"` to the import block.

3. **Update `watcher/github-build/main.go`**:

   a. Add `"net/http"` and `github.com/bborbe/maintainer/lib/githubapp` to imports.

   b. In the `application` struct, change `GHToken` to `required:"false"` and add the four App fields:
   ```go
   GHToken       string `required:"false" arg:"gh-token"       env:"GH_TOKEN"       usage:"GitHub token (read scope sufficient); ignored when App auth is configured" display:"length"`
   AppID          int64  `required:"false" arg:"app-id"          env:"APP_ID"           usage:"GitHub App ID (numeric); when set, App auth is used instead of GH_TOKEN"`
   InstallationID int64  `required:"false" arg:"installation-id" env:"INSTALLATION_ID"  usage:"GitHub App Installation ID (numeric)"`
   PEMKeyFile     string `required:"false" arg:"pem-key-file"    env:"PEM_KEY_FILE"     usage:"Path to the GitHub App private key (PEM) mounted from k8s Secret"`
   PEMKey         string `required:"false" arg:"pem-key"         env:"PEM_KEY"          usage:"GitHub App private key (PEM) as env var content; mutually exclusive with PEM_KEY_FILE" display:"length"`
   ```

   c. Add a `resolveAuth` method to `application` that mirrors the pr-reviewer pattern but uses `githubapp.NewClient` (not `MintIAT`):
   ```go
   func (a *application) resolveAuth(ctx context.Context) (*http.Client, error) {
       hasPEMFile := a.PEMKeyFile != ""
       hasPEMContent := a.PEMKey != ""
       useGitHubApp := a.AppID != 0 && a.InstallationID != 0 && (hasPEMFile || hasPEMContent)

       if a.GHToken != "" && useGitHubApp {
           glog.Warningf(
               "watcher/github-build auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
           )
       }

       switch {
       case useGitHubApp:
           var pemBytes []byte
           var err error
           if hasPEMFile {
               pemBytes, err = os.ReadFile(a.PEMKeyFile)
               if err != nil {
                   return nil, errors.Wrapf(ctx, err, "read PEM_KEY_FILE %s", a.PEMKeyFile)
               }
           } else {
               pemBytes = []byte(a.PEMKey)
           }
           httpClient, err := githubapp.NewClient(ctx, githubapp.Config{
               AppID:          a.AppID,
               InstallationID: a.InstallationID,
               PEM:            pemBytes,
           })
           if err != nil {
               return nil, errors.Wrap(ctx, err, "create githubapp client")
           }
           glog.V(2).Infof(
               "watcher/github-build auth mode=github-app app_id=%d installation_id=%d",
               a.AppID, a.InstallationID,
           )
           return httpClient, nil

       case a.GHToken != "":
           glog.Warningf("watcher/github-build auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
           return &http.Client{
               Transport: &tokenTransport{token: a.GHToken},
           }, nil

       default:
           return nil, errors.Errorf(
               ctx,
               "watcher/github-build auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY_FILE (or PEM_KEY), or set GH_TOKEN",
           )
       }
   }
   ```

   d. Add the `tokenTransport` type (for PAT-fallback mode — wraps GH_TOKEN in a static-Bearer transport):
   ```go
   // tokenTransport is an http.RoundTripper that injects a static Bearer token.
   type tokenTransport struct {
       token string
   }

   func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
       req = req.Clone(req.Context())
       req.Header.Set("Authorization", "Bearer "+t.token)
       return http.DefaultTransport.RoundTrip(req)
   }
   ```

   e. In the `Run` method, replace the call to `factory.CreateWatcher`:
   ```go
   httpClient, err := a.resolveAuth(ctx)
   if err != nil {
       return errors.Wrap(ctx, err, "resolve auth")
   }
   defer httpClient.CloseIdleConnections() // best-effort cleanup

   w, cleanup, err := factory.CreateWatcher(
       ctx,
       httpClient,
       a.KafkaBrokers,
       // ... rest unchanged
   )
   ```

   f. Do NOT remove the `github.com/bborbe/run` import — it remains in use via `run.CancelOnFirstFinish` and `run.Func` (lines 110, 116, 123, 141).

4. **Extract `resolveAuth` + `tokenTransport` to a shared package — TWO entry points consume `factory.CreateWatcher`**:

   `watcher/github-build/` builds TWO binaries: the long-lived StatefulSet at `main.go` AND the local smoke-test CLI at `cmd/run-once/main.go`. Both call `factory.CreateWatcher` (verified: `grep -rn "factory.CreateWatcher" watcher/github-build/` returns matches in `main.go:88` and `cmd/run-once/main.go:60`). After step 2 changes the factory signature, BOTH binaries must produce an `*http.Client` from their `application` struct — duplicating the resolver in two places is a maintenance liability.

   Create `watcher/github-build/pkg/auth/auth.go`:
   ```go
   // Package auth resolves the GitHub auth mode (App vs PAT) for the watcher binaries.
   package auth

   import (
       "context"
       "net/http"
       "os"

       "github.com/bborbe/errors"
       "github.com/golang/glog"

       githubapp "github.com/bborbe/maintainer/lib/githubapp"
   )

   // Config is the resolver input.
   type Config struct {
       AppID          int64
       InstallationID int64
       PEMKeyFile     string
       PEMKey         string
       GHToken        string
       LogPrefix      string // e.g. "watcher/github-build" or "watcher/github-build-run-once"
   }

   // Resolve picks the active auth mode (App-when-configured, PAT-when-not,
   // error-when-neither) and returns an *http.Client suitable for go-github.
   // The App-mode client has an auto-refreshing transport (lib/githubapp.NewClient).
   // The PAT-mode client has a static-Bearer transport (tokenTransport below).
   func Resolve(ctx context.Context, cfg Config) (*http.Client, error) {
       hasPEMFile := cfg.PEMKeyFile != ""
       hasPEMContent := cfg.PEMKey != ""
       useGitHubApp := cfg.AppID != 0 && cfg.InstallationID != 0 && (hasPEMFile || hasPEMContent)

       if cfg.GHToken != "" && useGitHubApp {
           glog.Warningf("%s auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored", cfg.LogPrefix)
       }

       switch {
       case useGitHubApp:
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

       case cfg.GHToken != "":
           glog.Warningf("%s auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)", cfg.LogPrefix)
           return &http.Client{Transport: &tokenTransport{token: cfg.GHToken}}, nil

       default:
           return nil, errors.Errorf(ctx,
               "%s auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY_FILE (or PEM_KEY), or set GH_TOKEN",
               cfg.LogPrefix)
       }
   }

   // tokenTransport injects a static Bearer token. Unexported — only Resolve
   // constructs it.
   type tokenTransport struct{ token string }

   func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
       req = req.Clone(req.Context())
       req.Header.Set("Authorization", "Bearer "+t.token)
       return http.DefaultTransport.RoundTrip(req)
   }
   ```

   Then **inline `resolveAuth` / `tokenTransport` from step 3 disappear**. Replace the call site in `main.go` (step 3e) with:
   ```go
   httpClient, err := auth.Resolve(ctx, auth.Config{
       AppID: a.AppID, InstallationID: a.InstallationID,
       PEMKeyFile: a.PEMKeyFile, PEMKey: a.PEMKey,
       GHToken: a.GHToken,
       LogPrefix: "watcher/github-build",
   })
   if err != nil {
       return errors.Wrap(ctx, err, "resolve auth")
   }
   defer httpClient.CloseIdleConnections()
   ```

   **Then update `watcher/github-build/cmd/run-once/main.go` to mirror**:
   a. Downgrade its `GHToken` field to `required:"false"`.
   b. Add the same four App env fields (`AppID`, `InstallationID`, `PEMKeyFile`, `PEMKey`) to its `application` struct (same shape as step 1).
   c. Before calling `factory.CreateWatcher` at line 60, call `auth.Resolve(ctx, auth.Config{..., LogPrefix: "watcher/github-build-run-once"})` and pass the returned `httpClient` to `factory.CreateWatcher` instead of `a.GHToken`.

   Note: with this refactor, the `resolveAuth` METHOD on `application` (step 3c) and the inlined `tokenTransport` type (step 3d) are NO LONGER needed in `main.go` — they live in `pkg/auth/` instead. Strike steps 3c and 3d; both binaries' `Run` methods just call `auth.Resolve(ctx, auth.Config{...})`.

5. **Verify the test helper `NewForTest` in `watcher/github-build/pkg/githubclient_export_test.go`** — verified pre-existing signature: `NewForTest(c *gogithub.Client) *gitHubClient`. Takes a `*gogithub.Client`, not a token. No change needed.

6. **Run `cd watcher/github-build && make test`** — must pass. If tests fail due to constructor signature changes, fix the tests by passing a real `*http.Client` (for integration-style tests using httptest) or a nil/default client.

7. **Run `cd watcher/github-build && make precommit`** — must exit 0.

8. **Add changelog entry** to `CHANGELOG.md` under `## Unreleased`:
   ```
   - feat: migrate watcher/github-build from PAT to GitHub App authentication with auto-refreshing IAT transport
   ```
</requirements>

<constraints>
- MUST use `githubapp.NewClient` (returns `*http.Client` with auto-refreshing transport) — NOT `githubapp.MintIAT` (static token, unsuitable for long-lived poller)
- `context.Background()` forbidden in `factory.go`, `githubclient.go`, or the auth resolver — use the injected `ctx`
- Errors wrapped exclusively with `github.com/bborbe/errors` — no `fmt.Errorf`, no stdlib `errors.New`
- `pkg.NewGitHubClient`'s constructor signature changes from `(token string)` to `(httpClient *http.Client)` — verify all test helpers are updated
- No changes to `GitHubClient` interface or its methods
- No changes to `factory.CreateWatcher`'s external contract beyond the first parameter type
- Do NOT commit — dark-factory handles git
- Existing tests must still pass; `make precommit` must exit 0
</constraints>

<verification>
```bash
# NewGitHubClient signature
grep -n "func NewGitHubClient" watcher/github-build/pkg/githubclient.go
# Expected: "func NewGitHubClient(httpClient *http.Client) GitHubClient"

# factory.CreateWatcher signature
grep -n "func CreateWatcher" watcher/github-build/pkg/factory/factory.go
# Expected: "func CreateWatcher(ctx context.Context, httpClient *http.Client, ..."

# tokenTransport defined
grep -n "type tokenTransport struct" watcher/github-build/main.go

# resolveAuth method
grep -n "func.*resolveAuth" watcher/github-build/main.go

# Auth log lines (glog V(2))
grep -n "auth mode=" watcher/github-build/main.go
# Expected: at least two matches — "auth mode=github-app" and "auth mode=pat-fallback"

# Error message contains both APP_ID and GH_TOKEN
grep -n "APP_ID\|GH_TOKEN" watcher/github-build/main.go

# No MintIAT usage (only NewClient allowed)
grep -n "MintIAT" watcher/github-build/
# Expected: zero matches

# Tests pass
cd watcher/github-build && make test

# Full precommit
cd watcher/github-build && make precommit
```
</verification>
