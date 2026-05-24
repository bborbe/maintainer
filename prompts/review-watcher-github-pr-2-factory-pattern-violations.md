---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `CreateGitHubHTTPClient` has multi-branch auth-mode conditionals, logging, and returns `error` — all prohibited by zero-business-logic factory rule
- `CreateKafkaSender` returns `(_, cleanup, error)` — cleanup closure belongs in `main.go` via `defer`, not in factory return value
- `CreateWatcher` returns `error` (propagated from inner factory) and recomputes `trustDecision` that caller already constructed
- `CreateSinglePRHandler` returns `error`, calls the dispatching `CreateGitHubHTTPClient`, and validates nil inputs with four `if` blocks
- `AuthConfig` struct lives in factory package but is shared across factory functions
</summary>

<objective>
Refactor factory functions to follow zero-business-logic rule: no conditionals, no error returns, no logging, no I/O. Move cleanup lifecycle to main.go. Move auth-mode dispatch to main.go. Pass ready-to-use dependencies instead of raw config structs.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — zero logic, no error return, Create* prefix.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/` — inject interfaces, never call package functions directly.

Files to read before making changes:
- `watcher/github-pr/pkg/factory/factory.go` — full file; understand all 3 factory functions
- `watcher/github-pr/pkg/factory/single_pr.go` — full file; understand CreateSinglePRHandler
- `watcher/github-pr/main.go` — lines 104-250; understand current wiring in Run()
- `watcher/github-pr/pkg/watcher.go` — lines 59-72; understand watcher struct fields
</context>

<requirements>

**Execute steps in order. Run `make test` after step 4. Run `make precommit` only at the final step.**

1. **Split `CreateGitHubHTTPClient` into two pure pass-through factories in `factory.go`:**

   Replace the current `CreateGitHubHTTPClient` function with:
   ```go
   // CreateGitHubAppClient creates an HTTP client authenticated as a GitHub App installation.
   func CreateGitHubAppClient(
       ctx context.Context,
       appID int64,
       installationID int64,
       pemKey []byte,
   ) (*http.Client, error) {
       cfg := githubapp.Config{
           AppID:          appID,
           InstallationID: installationID,
           PrivateKey:     pemKey,
       }
       return githubapp.NewClient(ctx, cfg)
   }

   // CreateGitHubPATClient creates an HTTP client authenticated with a personal access token.
   func CreateGitHubPATClient(ctx context.Context, token string) *http.Client {
       ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
       return oauth2.NewClient(ctx, ts)
   }
   ```

   Remove the `AuthConfig` struct and all auth-mode conditionals from `CreateGitHubHTTPClient`. Delete `CreateGitHubHTTPClient` entirely. Update imports: add `"github.com/google/go-github/v62/github"` (if not already present for go-github), add `"golang.org/x/oauth2"` (for oauth2).

2. **Refactor `CreateKafkaSender` in `factory.go`:**

   Change signature to accept a `SyncProducer` instead of creating one:
   ```go
   func CreateKafkaSender(syncProducer libkafka.SyncProducer, branch base.Branch) task.CreateCommandSender {
       sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
       return task.NewCreateCommandSender(sender)
   }
   ```

   Remove the `cleanup func()` return value and the `syncProducer.Close()` logic. Move cleanup to `main.go` via `defer`. Remove error return.

3. **Refactor `CreateWatcher` in `factory.go`:**

   Change signature to accept `*http.Client` instead of `AuthConfig`, and `trust.Trust` instead of `[]string`:
   ```go
   func CreateWatcher(
       ctx context.Context,
       httpClient *http.Client,
       createSender task.CreateCommandSender,
       cursorPath string,
       startTime libtime.DateTime,
       scope string,
       taskCreationFilter filter.TaskCreationFilter,
       stage string,
       metrics Metrics,
       trustDecision trust.Trust,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
   ) pkg.Watcher {
       ghClient := pkg.NewGitHubClient(httpClient)
       return pkg.NewWatcher(
           ghClient,
           createSender,
           cursorPath,
           startTime,
           scope,
           taskCreationFilter,
           stage,
           metrics,
           trustDecision,
           maxSlugLen,
           maxTitleLen,
           taskSuffix,
       )
   }
   ```

   Remove the `trustedAuthors []string` parameter and the inline `trustDecision` construction. Remove error return. Remove call to `CreateGitHubHTTPClient` — caller passes the `*http.Client`.

4. **Update `CreateSinglePRHandler` in `factory/single_pr.go`:**

   Change signature to accept `*http.Client` instead of `AuthConfig`, and `trust.Trust` instead of `[]string`. Remove error return. Remove nil validations (move to main.go caller):
   ```go
   func CreateSinglePRHandler(
       httpClient *http.Client,
       createSender task.CreateCommandSender,
       taskCreationFilter filter.TaskCreationFilter,
       trustDecision trust.Trust,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
   ) handler.SinglePRTriggerHandler {
       ghClient := pkg.NewGitHubClient(httpClient)
       return handler.NewSinglePRTriggerHandler(
           ghClient,
           createSender,
           taskCreationFilter,
           trustDecision,
           stage,
           maxSlugLen,
           maxTitleLen,
           taskSuffix,
       )
   }
   ```

   Update imports. Remove the `errors.Errorf` nil checks — caller is responsible for passing non-nil values.

5. **Update `main.go` wiring in `Run()`:**

   The auth-mode dispatch logic moves from factory to `main.go`. Find the existing `CreateGitHubHTTPClient` call and replace with:
   ```go
   var httpClient *http.Client
   var err error

   // Determine auth mode
   appID := getEnvInt("APP_ID")
   installationID := getEnvInt("INSTALLATION_ID")
   pemKey := []byte(getEnv("PEM_KEY", ""))
   token := getEnv("GH_TOKEN", "")

   if appID != 0 && installationID != 0 && len(pemKey) != 0 {
       glog.Infof("using GitHub App authentication")
       httpClient, err = factory.CreateGitHubAppClient(ctx, appID, installationID, pemKey)
       if err != nil {
           return errors.Wrap(ctx, err, "create GitHub App client")
       }
   } else if token != "" {
       glog.Infof("using GitHub PAT authentication")
       httpClient = factory.CreateGitHubPATClient(ctx, token)
   } else {
       return errors.Errorf(ctx, "neither GH_TOKEN nor APP_ID/INSTALLATION_ID/PEM_KEY provided")
   }
   ```

   For `CreateKafkaSender`: create the sync producer in `main.go` and pass it to the factory:
   ```go
   syncProducer, err := libkafka.NewSyncProducerWithName(ctx, brokers, "maintainer-watcher-github-pr")
   if err != nil {
       return errors.Wrap(ctx, err, "create sync producer")
   }
   defer func() {
       if err := syncProducer.Close(); err != nil {
           glog.Warningf("close kafka sync producer: %v", err)
       }
   }()
   createSender := factory.CreateKafkaSender(syncProducer, branch)
   ```

   For `CreateWatcher`: pass the `trustDecision` that was already constructed in `main.go` before calling the factory. Pass `httpClient` instead of `AuthConfig`.

   For `CreateSinglePRHandler`: pass `httpClient` and `trustDecision` directly.

   If needed, add helper `getEnvInt(name string) int64` that parses `strconv.ParseInt(getEnv(name, "0"), 10, 64)`.

6. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```
   Fix any compilation errors (likely from changed function signatures in main.go and tests).

7. **Run `make precommit`:**
   ```bash
   cd watcher/github-pr && make precommit
   ```
</requirements>

<constraints>
- Only change `watcher/github-pr/pkg/factory/factory.go`, `watcher/github-pr/pkg/factory/single_pr.go`, and `watcher/github-pr/main.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass — update any test that calls the old factory signatures
- Use `errors.Wrap`/`errors.Wrapf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- Auth-mode dispatch and partial-config validation MUST live in `main.go`, not in factories
- Cleanup lifecycle (`syncProducer.Close()`) MUST be in `main.go` via `defer`, not in factory return
- Factories MUST return exactly what they construct — no error, no cleanup closure
- Coverage ≥80% for changed packages
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm no factory returns error:
grep -n "func Create.*error" watcher/github-pr/pkg/factory/*.go

# Confirm no cleanup closure in factory returns:
grep -n "cleanup.*func" watcher/github-pr/pkg/factory/*.go

# Confirm auth dispatch in main.go:
grep -n "CreateGitHubAppClient\|CreateGitHubPATClient" watcher/github-pr/main.go

# Confirm SyncProducer lifecycle in main.go:
grep -n "NewSyncProducerWithName\|syncProducer.Close" watcher/github-pr/main.go
</verification>
