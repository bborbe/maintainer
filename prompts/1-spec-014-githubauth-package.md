---
status: draft
spec: [014-private-github-repo-support]
created: "2026-05-03T18:00:00Z"
branch: dark-factory/private-github-repo-support
---

<summary>
- A new package `agent/pr-reviewer/pkg/githubauth/` is created with a `GitHubAuthSetup` interface (`Setup(ctx) error`), a real `GhAuthSetupGit` implementation (invokes `gh auth setup-git` when GH_TOKEN is non-empty, no-op when empty), and a `NoopAuthSetup` implementation (always returns nil)
- A counterfeiter mock for `GitHubAuthSetup` is generated at `agent/pr-reviewer/mocks/github-auth-setup.go`
- `factory.RunConfig` gains an `AuthSetup githubauth.GitHubAuthSetup` field; `RunAgent` calls `cfg.AuthSetup.Setup(ctx)` immediately after `EnsureInstalled` and before `CreateAgent`, propagating any setup error as a wrapped error
- `agent/pr-reviewer/main.go` (Kafka pod entry point) injects the real `GhAuthSetupGit` implementation, which reads `GH_TOKEN` from env; when the token is empty the setup call is a no-op so the pod still starts and reviews public repos
- `agent/pr-reviewer/cmd/run-task/main.go` (local-CLI entry point) injects `NoopAuthSetup`, never mutating `~/.gitconfig`
- The `gh auth setup-git` subprocess does not take the token as an argument (reads `GH_TOKEN` from env); `cmd.Args` are safe to log but the call site uses `gosec G204` suppression with a clear comment
- The GH_TOKEN value never appears in log lines, error wrap messages, or `cmd.String()` output — the real implementation only passes hardcoded args `["auth", "setup-git"]` to `gh`
- Unit tests cover: real impl invokes `gh auth setup-git` exactly once when token is non-empty, is not invoked when token is empty; noop always returns nil; type-literal wiring assertions confirm pod main uses the real type and local-CLI main uses the noop type
- GHToken arg shape stays `required:"false"` in both entry points; a comment is added noting git-auth setup at pod startup uses this token
- CHANGELOG `## Unreleased` entry added covering the new package and the pod-startup auth-setup behavior
</summary>

<objective>
Create the `pkg/githubauth/` package with its interface, real and no-op implementations, and counterfeiter mock. Wire the interface into `factory.RunConfig.AuthSetup` and call it in `RunAgent`. Inject the real implementation in the Kafka pod entry point and the no-op in the local-CLI entry point. This is the host-side git credential setup that allows the pod's `git clone` calls to authenticate against GitHub private repositories via `gh auth setup-git`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-factory-pattern.md` from `~/.claude/plugins/marketplaces/coding/docs/` — factories compose constructors with zero business logic.
Read `go-security-linting.md` from `~/.claude/plugins/marketplaces/coding/docs/` — gosec G204 suppression rules.
Read `go-testing-guide.md` from `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo v2/Gomega, counterfeiter mocks.
Read `go-error-wrapping-guide.md` from `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors` Wrapf/Errorf, never fmt.Errorf.
Read `go-logging-guide.md` from `~/.claude/plugins/marketplaces/coding/docs/` — never log tokens.

Files to read before making any changes:

- `agent/pr-reviewer/pkg/factory/runner.go` — `RunConfig` struct and `RunAgent` function; you add `AuthSetup` field and the Setup call between `EnsureInstalled` and `CreateAgent`
- `agent/pr-reviewer/pkg/factory/factory.go` — full file; understand `CreateAgent` signature and existing composition pattern
- `agent/pr-reviewer/main.go` — full file; understand `application` struct, `GHToken` field, and `Run()` wiring pattern; you add the real impl injection
- `agent/pr-reviewer/cmd/run-task/main.go` — full file; same pattern; you add the noop impl injection
- `agent/pr-reviewer/mocks/repo-manager.go` — read the first 30 lines to understand counterfeiter output format and package declaration
- `agent/pr-reviewer/pkg/git/repo_manager.go` — read counterfeiter annotation on line 22 (`//counterfeiter:generate -o ../../mocks/repo-manager.go --fake-name RepoManager . RepoManager`) to understand the annotation format; mirror this for the new interface

Key facts (verified against the codebase):
- `factory/runner.go` `RunConfig` currently has no `AuthSetup` field — you are adding it
- `RunAgent` calls `installer.EnsureInstalled(...)` at ~line 54; the new `cfg.AuthSetup.Setup(ctx)` call goes immediately after (still before `CreateAgent` at ~line 65)
- Both `main.go` and `cmd/run-task/main.go` already import `prpkg "github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg"`
- Both entry points already have `GHToken string` with `required:"false"` and `display:"length"` — add a comment to the usage string: `"GitHub token for gh CLI auth and git credential helper at pod startup"`
- The new package lives at `agent/pr-reviewer/pkg/githubauth/` — import path: `"github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg/githubauth"`
- The counterfeiter annotation in the new package's interface file must point to `../../mocks/` relative to `agent/pr-reviewer/pkg/githubauth/`; the correct annotation is: `//counterfeiter:generate -o ../../mocks/github-auth-setup.go --fake-name GitHubAuthSetup . GitHubAuthSetup`
- `gh auth setup-git` writes a git credential helper entry for `github.com` into the pod's local git config (`/home/claude/.gitconfig`); it reads `GH_TOKEN` from the process environment, NOT from its argument list — the arg list `["auth", "setup-git"]` contains no secrets
- The `exec.CommandContext` call for `gh auth setup-git` must have gosec G204 suppressed with a comment: `// #nosec G204 -- binary is hardcoded "gh" and args are hardcoded ["auth", "setup-git"]; no user input`
- When `GH_TOKEN` is empty, the real implementation MUST return nil without invoking `gh` — this is the pod-reviews-public-repos path
- The test for the real implementation cannot exec the actual `gh` binary; instead, test via an injected command factory or by verifying the no-token path returns nil and the with-token path calls the command with the correct args. See `steps_gh_token_test.go` for a pattern that uses `httptest.NewServer` to stub the external call — but for exec.Command the idiomatic approach in this codebase is to use an `ExecFunc` injection (a `func(ctx, name, args...) error` field). Read `agent/pr-reviewer/pkg/steps_gh_token.go` to see if there is an exec-stubbing pattern already established. If there is NO such pattern, implement a minimal `ExecFunc` field on the real implementation struct (`type GhAuthSetupGit struct { ExecFunc func(ctx context.Context, name string, args ...string) error; ghToken string }`) so tests can inject a fake exec; the `New` constructor wires in the real `exec.CommandContext` wrapper. This avoids shelling out in unit tests.
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Read `agent/pr-reviewer/pkg/steps_gh_token.go`** to check if there is an exec-stubbing pattern already in use. Look for an `ExecFunc` field or similar. This determines how the real `GhAuthSetupGit` struct is tested. If no pattern exists, you will add an `ExecFunc` field (described in step 2).

2. **Create `agent/pr-reviewer/pkg/githubauth/github_auth_setup.go`**:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package githubauth

   import (
       "context"
       "os/exec"

       "github.com/bborbe/errors"
       "github.com/golang/glog"
   )

   // GitHubAuthSetup configures git credential helpers for GitHub at pod startup.
   // The pod implementation invokes `gh auth setup-git`; the local-CLI noop
   // returns nil without touching any config file.
   //
   //counterfeiter:generate -o ../../mocks/github-auth-setup.go --fake-name GitHubAuthSetup . GitHubAuthSetup
   type GitHubAuthSetup interface {
       Setup(ctx context.Context) error
   }

   // NewGhAuthSetupGit returns a GitHubAuthSetup that invokes `gh auth setup-git`
   // when ghToken is non-empty. When ghToken is empty the Setup call is a no-op
   // so pods that target non-GitHub hosts start cleanly without a token.
   func NewGhAuthSetupGit(ghToken string) GitHubAuthSetup {
       return &ghAuthSetupGit{
           ghToken:  ghToken,
           execFunc: defaultExecFunc,
       }
   }

   // ghAuthSetupGit is the real implementation; execFunc is injectable for testing.
   type ghAuthSetupGit struct {
       ghToken  string
       execFunc func(ctx context.Context, name string, args ...string) error
   }

   func (g *ghAuthSetupGit) Setup(ctx context.Context) error {
       if g.ghToken == "" {
           glog.V(2).Infof("github-auth-setup: GH_TOKEN not set, skipping gh auth setup-git")
           return nil
       }
       glog.V(2).Infof("github-auth-setup: running gh auth setup-git")
       if err := g.execFunc(ctx, "gh", "auth", "setup-git"); err != nil {
           return errors.Wrap(ctx, err, "gh auth setup-git failed")
       }
       glog.V(2).Infof("github-auth-setup: gh auth setup-git complete")
       return nil
   }

   // defaultExecFunc is the production exec.CommandContext wrapper.
   func defaultExecFunc(ctx context.Context, name string, args ...string) error {
       // #nosec G204 -- binary is hardcoded "gh" and args are hardcoded ["auth", "setup-git"]; no user input
       cmd := exec.CommandContext(ctx, name, args...)
       if out, err := cmd.CombinedOutput(); err != nil {
           return errors.Errorf(ctx, "%s %v: %s", name, args, out)
       }
       return nil
   }

   // NewNoopAuthSetup returns a GitHubAuthSetup that always returns nil.
   // Used by cmd/run-task so the developer's existing gh auth login continues
   // to handle credentials; ~/.gitconfig is never mutated by the agent.
   func NewNoopAuthSetup() GitHubAuthSetup {
       return &noopAuthSetup{}
   }

   type noopAuthSetup struct{}

   func (n *noopAuthSetup) Setup(_ context.Context) error { return nil }
   ```

3. **Create `agent/pr-reviewer/pkg/githubauth/github_auth_setup_test.go`** with Ginkgo v2 + Gomega:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package githubauth_test

   import (
       "context"
       "errors"
       "testing"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg/githubauth"
   )

   func TestGitHubAuth(t *testing.T) {
       RegisterFailHandler(Fail)
       RunSpecs(t, "GitHubAuth Suite")
   }

   var _ = Describe("GhAuthSetupGit", func() {
       var (
           ctx       context.Context
           callCount int
           lastName  string
           lastArgs  []string
           fakeExec  func(ctx context.Context, name string, args ...string) error
       )

       BeforeEach(func() {
           ctx = context.Background()
           callCount = 0
           fakeExec = func(_ context.Context, name string, args ...string) error {
               callCount++
               lastName = name
               lastArgs = args
               return nil
           }
       })

       It("does not invoke gh when GH_TOKEN is empty", func() {
           setup := githubauth.NewGhAuthSetupGitWithExecFunc("", fakeExec)
           err := setup.Setup(ctx)
           Expect(err).NotTo(HaveOccurred())
           Expect(callCount).To(Equal(0))
       })

       It("invokes gh auth setup-git exactly once when GH_TOKEN is non-empty", func() {
           setup := githubauth.NewGhAuthSetupGitWithExecFunc("fake-token", fakeExec)
           err := setup.Setup(ctx)
           Expect(err).NotTo(HaveOccurred())
           Expect(callCount).To(Equal(1))
           Expect(lastName).To(Equal("gh"))
           Expect(lastArgs).To(Equal([]string{"auth", "setup-git"}))
       })

       It("propagates exec error when gh fails", func() {
           fakeExec = func(_ context.Context, _ string, _ ...string) error {
               return errors.New("gh not found")
           }
           setup := githubauth.NewGhAuthSetupGitWithExecFunc("some-token", fakeExec)
           err := setup.Setup(ctx)
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("gh auth setup-git failed"))
       })

       It("does not include the token value in any argument", func() {
           const fakeToken = "ghp_SUPERSECRET123"
           fakeExec = func(_ context.Context, name string, args ...string) error {
               Expect(name).NotTo(ContainSubstring(fakeToken))
               for _, a := range args {
                   Expect(a).NotTo(ContainSubstring(fakeToken))
               }
               return nil
           }
           setup := githubauth.NewGhAuthSetupGitWithExecFunc(fakeToken, fakeExec)
           Expect(setup.Setup(ctx)).To(Succeed())
       })
   })

   var _ = Describe("NoopAuthSetup", func() {
       It("always returns nil", func() {
           setup := githubauth.NewNoopAuthSetup()
           Expect(setup.Setup(context.Background())).To(Succeed())
       })
   })
   ```

4. **Add `NewGhAuthSetupGitWithExecFunc` to `github_auth_setup.go`** — a test-only constructor that accepts an injected exec function (exported for `_test` package access):

   ```go
   // NewGhAuthSetupGitWithExecFunc constructs a GhAuthSetupGit with an injected
   // exec function for testing. Do not use in production code.
   func NewGhAuthSetupGitWithExecFunc(
       ghToken string,
       execFunc func(ctx context.Context, name string, args ...string) error,
   ) GitHubAuthSetup {
       return &ghAuthSetupGit{ghToken: ghToken, execFunc: execFunc}
   }
   ```

5. **Run `go generate ./pkg/githubauth/...`** in `agent/pr-reviewer/` to generate the counterfeiter mock:

   ```bash
   cd agent/pr-reviewer && go generate ./pkg/githubauth/...
   ```

   If this fails, run counterfeiter directly:
   ```bash
   cd agent/pr-reviewer && go run github.com/maxbrunsfeld/counterfeiter/v6 -o mocks/github-auth-setup.go --fake-name GitHubAuthSetup ./pkg/githubauth/. GitHubAuthSetup
   ```

   Verify `mocks/github-auth-setup.go` was created.

6. **Add `AuthSetup githubauth.GitHubAuthSetup` to `RunConfig`** in `agent/pr-reviewer/pkg/factory/runner.go`:

   After the `RepoAllowlist []string` field, add:
   ```go
   AuthSetup githubauth.GitHubAuthSetup // pod: real gh-auth-setup; local-CLI: noop
   ```

   Add the import:
   ```go
   "github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg/githubauth"
   ```

7. **Call `cfg.AuthSetup.Setup(ctx)` in `RunAgent`** in `agent/pr-reviewer/pkg/factory/runner.go`:

   After the `installer.EnsureInstalled(...)` block (which ends around line 58), add:

   ```go
   if err := cfg.AuthSetup.Setup(ctx); err != nil {
       return nil, errors.Wrap(ctx, err, "github auth setup failed")
   }
   ```

   This must appear BEFORE the `CreateAgent(...)` call.

8. **Add wiring test for `RunConfig.AuthSetup` type-literal assertions** in `agent/pr-reviewer/pkg/factory/factory_test.go`:

   Read the existing test file first to understand its structure. Add a new `Describe("RunConfig.AuthSetup wiring")` block (or append `It(...)` cases to an existing appropriate block) that:
   - Constructs a `factory.RunConfig` with `AuthSetup: githubauth.NewGhAuthSetupGit("fake-token")` and asserts the field is non-nil and is of the expected concrete type using `Expect(cfg.AuthSetup).To(BeAssignableToTypeOf(&githubauth.GhAuthSetupGit{}))` — but note `ghAuthSetupGit` is unexported; instead assert `Expect(cfg.AuthSetup).NotTo(BeNil())` and `Expect(cfg.AuthSetup).NotTo(BeAssignableToTypeOf(githubauth.NewNoopAuthSetup()))` to confirm it is NOT the noop type
   - Asserts that `githubauth.NewNoopAuthSetup()` is a different type than `githubauth.NewGhAuthSetupGit("x")`

   The purpose: guard against a future refactor accidentally wiring the wrong type.

9. **Update `agent/pr-reviewer/main.go`** — inject the real implementation:

   Step 9a — add import: `"github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg/githubauth"`

   Step 9b — update the `GHToken` field comment in `application` struct:
   ```go
   // GitHub token forwarded to the Claude CLI subprocess as GH_TOKEN for gh auth.
   // Also used by the real GitHubAuthSetup to configure git credential helper at pod startup.
   GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub token for gh CLI auth and git credential helper at pod startup" display:"length"`
   ```

   Step 9c — in `Run()`, before `factory.RunAgent(...)`, construct and pass the real auth setup:
   ```go
   authSetup := githubauth.NewGhAuthSetupGit(a.GHToken)
   ```

   Step 9d — wire into `RunConfig`:
   ```go
   result, err := factory.RunAgent(ctx, factory.RunConfig{
       ...
       AuthSetup:     authSetup,
       ...
   })
   ```

10. **Update `agent/pr-reviewer/cmd/run-task/main.go`** — inject the noop:

    Step 10a — add import: `"github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg/githubauth"`

    Step 10b — update the `GHToken` field comment (same style as step 9b but note this is the noop path):
    ```go
    // GitHub token forwarded to the Claude CLI subprocess as GH_TOKEN for gh auth.
    // cmd/run-task uses NoopAuthSetup — the developer's existing gh auth login handles git credentials.
    GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub token for gh CLI auth" display:"length"`
    ```

    Step 10c — in `Run()`, construct and pass the noop:
    ```go
    authSetup := githubauth.NewNoopAuthSetup()
    ```

    Step 10d — wire into `RunConfig`:
    ```go
    result, err := factory.RunAgent(ctx, factory.RunConfig{
        ...
        AuthSetup:     authSetup,
        ...
    })
    ```

11. **Update `CHANGELOG.md`** — create `## Unreleased` section above the most recent `## vX.Y.Z`:

    ```markdown
    ## Unreleased

    - feat(pr-reviewer): add `pkg/githubauth` package with `GitHubAuthSetup` interface, real `GhAuthSetupGit` implementation (runs `gh auth setup-git` at pod startup when `GH_TOKEN` is set), and `NoopAuthSetup` (used by `cmd/run-task`). Wire through `factory.RunConfig.AuthSetup` so pods authenticate git against GitHub private repos; local-CLI mode is unaffected.
    ```

12. **Run `make precommit`** in `agent/pr-reviewer/`:

    ```bash
    cd agent/pr-reviewer && make precommit
    ```

</requirements>

<constraints>
- Only edit files under `agent/pr-reviewer/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- The auth-setup contract is `Setup(ctx context.Context) error` — no other methods on the interface
- `GH_TOKEN` value MUST NOT appear in any subprocess argument, any log line, any error message, or any wrapped error string — the real implementation reads it from the process environment (set at pod startup from the K8s secret), not from its own field
- `gh auth setup-git` is the only subprocess the real implementation invokes; it takes no user-controlled arguments; `cmd.Args` for this call is `["gh", "auth", "setup-git"]`
- Use gosec G204 suppression comment `// #nosec G204 -- binary is hardcoded "gh" and args are hardcoded ["auth", "setup-git"]; no user input` on the exec.Command call
- `GHToken` field stays `required:"false"` in both entry points — bitbucket-only deployments must start cleanly
- Error wrapping uses `github.com/bborbe/errors` (`errors.Wrap`, `errors.Wrapf`, `errors.Errorf`); never `fmt.Errorf`
- Logging uses `glog.V(2).Infof(...)` — never logs the token value, never logs `cmd.String()` or `cmd.Args`
- The counterfeiter annotation must follow the exact format from `repo_manager.go`: `//counterfeiter:generate -o ../../mocks/github-auth-setup.go --fake-name GitHubAuthSetup . GitHubAuthSetup`
- `make precommit` runs from `agent/pr-reviewer/`, never at repo root
- Existing tests must pass without modification
</constraints>

<verification>
cd agent/pr-reviewer && make precommit

# Confirm new package files:
ls agent/pr-reviewer/pkg/githubauth/github_auth_setup.go agent/pr-reviewer/pkg/githubauth/github_auth_setup_test.go

# Confirm counterfeiter mock was generated:
ls agent/pr-reviewer/mocks/github-auth-setup.go

# Confirm AuthSetup field in RunConfig:
grep -n "AuthSetup" agent/pr-reviewer/pkg/factory/runner.go

# Confirm Setup call in RunAgent:
grep -n "AuthSetup.Setup\|auth setup" agent/pr-reviewer/pkg/factory/runner.go

# Confirm real impl in main.go, noop in cmd/run-task/main.go:
grep -n "NewGhAuthSetupGit\|NewNoopAuthSetup" agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go

# Confirm token never in args (safety check):
grep -n "ghToken\|GHToken\|GH_TOKEN" agent/pr-reviewer/pkg/githubauth/github_auth_setup.go
# Expected: only the field assignment and the empty-check; not in any exec args

# Confirm CHANGELOG updated:
grep -n "githubauth\|GhAuthSetupGit\|git credential helper" CHANGELOG.md
</verification>
