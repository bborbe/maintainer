---
status: completed
spec: [017-per-repo-maintenance-yaml]
summary: Created pkg/maintenance package with Loader and FileContentFetcher interfaces, added GetFileContent to GitHubClient, generated all counterfeiter mocks, added comprehensive tests achieving 84.2% coverage on GetFileContent and 96.7% on maintenance package, made gopkg.in/yaml.v3 a direct dependency; make precommit exited 0.
container: maintainer-093-spec-017-maintenance-loader
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T18:10:00Z"
queued: "2026-05-06T18:40:38Z"
started: "2026-05-06T18:40:39Z"
completed: "2026-05-06T18:46:34Z"
---

<summary>
- A new `pkg/maintenance/` package implements per-repo config loading from `.maintenance.yaml` on GitHub repos
- The loader fetches the file via the GitHub Contents API and parses the `watcher.github-build` YAML subtree
- All failure modes are handled internally: 404 is silent (common case), 5xx/malformed YAML/oversized file log WARN and fall through to empty config
- Unknown keys inside `watcher.github-build` (e.g. `priority: high`) are logged at INFO and ignored — known keys still applied
- Empty string values in the file are treated identically to absent keys — the watcher default applies
- The `GitHubClient` interface gains a `GetFileContent` method so the factory can pass the same client to the maintenance loader without a separate connection
- The loader is mockable via counterfeiter for unit tests in downstream prompts
- All existing tests continue to pass; `make test` is green in `watcher/github-build/`
</summary>

<objective>
Create the `pkg/maintenance/` package — the per-repo config loader — and extend the `GitHubClient` interface with `GetFileContent`. The loader is a self-contained, testable unit that always returns a (possibly empty) `GithubBuildConfig` and never propagates errors to its caller. This is prompt 1 of 2 for spec 017; wiring into the watcher state machine happens in prompt 2.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern, error wrapping rules.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, Counterfeiter mocks, ≥80% coverage rule.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.

**Dependency on spec 016:** This prompt assumes spec 016 (`configurable-task-frontmatter`) is already merged. `NewWatcher` in `pkg/watcher.go` now has 9 parameters ending with `assignee, taskStatus, taskPhase string`. The prompt does NOT modify `watcher.go` or `factory.go` — that happens in prompt 2.

Files to read fully before making any changes:
- `watcher/github-build/pkg/githubclient.go` — full file; understand `GitHubClient` interface (lines 33–40), concrete `*githubClient` struct, existing error-detection patterns for rate limiting (`stderrors.As` for `*gogithub.RateLimitError` and `*gogithub.AbuseRateLimitError`), and `ErrRateLimited` sentinel
- `watcher/github-build/pkg/mocks/github_client.go` — understand the counterfeiter-generated mock structure; this file will be regenerated after step 2
- `watcher/github-build/pkg/githubclient_test.go` — understand existing test patterns to mirror for `GetFileContent` tests
- `watcher/github-build/pkg/filter/filter.go` — canonical example of a subpackage under `pkg/` that defines interfaces for the parent package to use

**Key facts about go-github v62 `GetContents`:**
```go
// Repositories.GetContents signature:
func (s *RepositoriesService) GetContents(
    ctx context.Context,
    owner, repo, path string,
    opts *RepositoryContentGetOptions,
) (fileContent *RepositoryContent, directoryContent []*RepositoryContent, resp *Response, err error)

// RepositoryContentGetOptions:
type RepositoryContentGetOptions struct {
    Ref string `url:"ref,omitempty"`
}

// On the returned *RepositoryContent:
fileContent.GetContent()  // → (string, error): base64-decodes the Content field automatically
fileContent.GetSize()     // → int: file size in bytes as reported by the API

// 404 error type:
// err is a *gogithub.ErrorResponse with err.(*gogithub.ErrorResponse).Response.StatusCode == 404
// Detect via: stderrors.As(err, &ghErr) where ghErr is *gogithub.ErrorResponse
```

**Symbol verification commands to run before writing factory code:**
```bash
# Confirm GetContents method path:
grep -rn "func.*GetContents" $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/repos_contents.go 2>/dev/null | head -3

# Confirm RepositoryContentGetOptions:
grep -A 3 "RepositoryContentGetOptions" $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/repos_contents.go 2>/dev/null | head -8

# Confirm ErrorResponse and Response.StatusCode:
grep -n "type ErrorResponse\|StatusCode" $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/github.go 2>/dev/null | head -10

# Confirm gopkg.in/yaml.v3 Unmarshal:
grep -rn "func Unmarshal" $(go env GOPATH)/pkg/mod/gopkg.in/yaml.v3@*/yaml.go 2>/dev/null | head -3
```
</context>

<requirements>
**Execute steps in order. Run `make test` at the end for fast feedback before `make precommit`.**

1. **Add `GetFileContent` to the `GitHubClient` interface** in `watcher/github-build/pkg/githubclient.go`.

   After the existing `GetDefaultBranch` method declaration, add:

   ```go
   // GetFileContent fetches the raw content of a file at the given ref.
   // Returns (nil, nil) if the file does not exist (HTTP 404 — the common case).
   // Returns (nil, ErrRateLimited) when rate-limited.
   // Returns (nil, err) for any other API error.
   GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
   ```

2. **Implement `GetFileContent` on the concrete `*githubClient` struct** in the same file, after the existing `GetDefaultBranch` method:

   ```go
   func (c *githubClient) GetFileContent(
       ctx context.Context,
       owner, repo, path, ref string,
   ) ([]byte, error) {
       opts := &gogithub.RepositoryContentGetOptions{Ref: ref}
       fileContent, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, path, opts)
       if err != nil {
           var ghErr *gogithub.ErrorResponse
           if stderrors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusNotFound {
               return nil, nil // file not found — 404 is the common case, not an error
           }
           var rl *gogithub.RateLimitError
           var arl *gogithub.AbuseRateLimitError
           if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
               return nil, ErrRateLimited
           }
           return nil, errors.Wrapf(ctx, err, "get file content %s/%s/%s@%s", owner, repo, path, ref)
       }
       if fileContent == nil {
           return nil, nil // directory listing returned for a path that is a dir
       }
       if fileContent.GetSize() > 1024*1024 {
           return nil, errors.Errorf(ctx, "file %s/%s/%s too large: %d bytes (max 1 MiB)", owner, repo, path, fileContent.GetSize())
       }
       decoded, err := fileContent.GetContent()
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "decode content %s/%s/%s", owner, repo, path)
       }
       return []byte(decoded), nil
   }
   ```

   Add `"net/http"` to the import block if not already present.

3. **Regenerate the `GitHubClient` mock** so it includes the new `GetFileContent` stub:

   ```bash
   cd watcher/github-build && go generate ./pkg/...
   ```

   If `go generate` fails (no `//go:generate` directive), run counterfeiter directly:

   ```bash
   cd watcher/github-build && \
     go run github.com/maxbrunsfeld/counterfeiter/v6 \
       -o pkg/mocks/github_client.go \
       --fake-name GitHubClient \
       ./pkg/. GitHubClient
   ```

   Confirm the generated file includes `GetFileContentStub`, `GetFileContentCalls`, `GetFileContentArgsForCall`, `GetFileContentReturns`.

4. **Add `net/http` import** to `watcher/github-build/pkg/githubclient.go` if not present. The file currently imports `"context"`, `stderrors "errors"`, `"time"`. Add `"net/http"`.

5. **Create `watcher/github-build/pkg/maintenance/` package**:

   **`watcher/github-build/pkg/maintenance/doc.go`:**
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package maintenance loads per-repo config from .maintenance.yaml on GitHub.
   package maintenance
   ```

   **`watcher/github-build/pkg/maintenance/loader.go`:**

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package maintenance

   import (
       "context"

       "github.com/golang/glog"
       "gopkg.in/yaml.v3"
   )

   // FileContentFetcher fetches raw file bytes from a GitHub repository.
   // Matches the GetFileContent method signature on pkg.GitHubClient.
   //
   //counterfeiter:generate -o ../mocks/file_content_fetcher.go --fake-name FileContentFetcher . FileContentFetcher
   type FileContentFetcher interface {
       GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
   }

   // GithubBuildConfig holds the watcher.github-build subtree of .maintenance.yaml.
   // All fields are optional; empty string means "no override — use watcher default".
   type GithubBuildConfig struct {
       Assignee string
       Status   string
       Phase    string
   }

   // Loader fetches per-repo override config for the build watcher.
   //
   //counterfeiter:generate -o ../mocks/maintenance_loader.go --fake-name MaintenanceLoader . Loader
   type Loader interface {
       // LoadOverrides fetches .maintenance.yaml from the repo's default branch and
       // returns the watcher.github-build subtree. Never returns an error — all
       // failures are logged and result in an empty GithubBuildConfig (fall through
       // to watcher defaults). Empty string fields mean "no override".
       LoadOverrides(ctx context.Context, owner, repo, defaultBranch string) GithubBuildConfig
   }

   // NewLoader returns a Loader backed by the given FileContentFetcher.
   func NewLoader(fetcher FileContentFetcher) Loader {
       return &loaderImpl{fetcher: fetcher}
   }

   type loaderImpl struct {
       fetcher FileContentFetcher
   }

   // rawConfig is the full .maintenance.yaml structure used for YAML unmarshalling.
   type rawConfig struct {
       Watcher map[string]map[string]interface{} `yaml:"watcher"`
   }

   func (l *loaderImpl) LoadOverrides(ctx context.Context, owner, repo, defaultBranch string) GithubBuildConfig {
       filePath := ".maintenance.yaml"
       content, err := l.fetcher.GetFileContent(ctx, owner, repo, filePath, defaultBranch)
       if err != nil {
           glog.Warningf("maintenance loader: fetch failed owner=%s repo=%s err=%v", owner, repo, err)
           return GithubBuildConfig{}
       }
       if content == nil {
           // 404 — file absent is the common case; no log
           return GithubBuildConfig{}
       }

       var raw rawConfig
       if err := yaml.Unmarshal(content, &raw); err != nil {
           glog.Warningf("maintenance loader: malformed YAML owner=%s repo=%s path=%s err=%v", owner, repo, filePath, err)
           return GithubBuildConfig{}
       }

       watcherSection := raw.Watcher
       if watcherSection == nil {
           return GithubBuildConfig{}
       }
       buildSection, ok := watcherSection["github-build"]
       if !ok || buildSection == nil {
           return GithubBuildConfig{}
       }

       // Log INFO for unknown keys; extract known keys.
       known := map[string]bool{"assignee": true, "status": true, "phase": true}
       for k := range buildSection {
           if !known[k] {
               glog.Infof("maintenance loader: ignored unknown key watcher.github-build.%s in %s/%s/%s", k, owner, repo, filePath)
           }
       }

       cfg := GithubBuildConfig{}
       if v, ok := buildSection["assignee"].(string); ok {
           cfg.Assignee = v
       }
       if v, ok := buildSection["status"].(string); ok {
           cfg.Status = v
       }
       if v, ok := buildSection["phase"].(string); ok {
           cfg.Phase = v
       }
       return cfg
   }
   ```

6. **Create `watcher/github-build/pkg/maintenance/suite_test.go`**:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package maintenance_test

   import (
       "testing"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   func TestMaintenance(t *testing.T) {
       RegisterFailHandler(Fail)
       RunSpecs(t, "Maintenance Suite")
   }
   ```

7. **Create `watcher/github-build/pkg/maintenance/loader_test.go`**:

   Use counterfeiter mock `*mocks.FileContentFetcher` for `FileContentFetcher`.
   Import path for mock: `github.com/bborbe/maintainer/watcher/github-build/pkg/mocks`.

   Cover all failure modes from the spec failure table AND happy paths:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package maintenance_test

   import (
       "context"
       stderrors "errors"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-build/pkg/maintenance"
       "github.com/bborbe/maintainer/watcher/github-build/pkg/mocks"
   )

   var _ = Describe("Loader", func() {
       var (
           ctx     context.Context
           fetcher *mocks.FileContentFetcher
           loader  maintenance.Loader
       )

       BeforeEach(func() {
           ctx = context.Background()
           fetcher = new(mocks.FileContentFetcher)
           loader = maintenance.NewLoader(fetcher)
       })

       Context("file not found (404)", func() {
           It("returns empty config silently", func() {
               fetcher.GetFileContentReturns(nil, nil) // 404 → nil, nil
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg).To(Equal(maintenance.GithubBuildConfig{}))
               Expect(fetcher.GetFileContentCallCount()).To(Equal(1))
           })
       })

       Context("API error (5xx or other)", func() {
           It("returns empty config and does not panic", func() {
               fetcher.GetFileContentReturns(nil, stderrors.New("http 500 internal server error"))
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg).To(Equal(maintenance.GithubBuildConfig{}))
           })
       })

       Context("file too large (returned as error by GetFileContent)", func() {
           It("returns empty config", func() {
               fetcher.GetFileContentReturns(nil, stderrors.New("file owner/repo/.maintenance.yaml too large: 1048577 bytes (max 1 MiB)"))
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg).To(Equal(maintenance.GithubBuildConfig{}))
           })
       })

       Context("malformed YAML", func() {
           It("returns empty config", func() {
               fetcher.GetFileContentReturns([]byte("watcher:\n  github-build:\n    assignee: [invalid yaml"), nil)
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg).To(Equal(maintenance.GithubBuildConfig{}))
           })
       })

       Context("valid YAML — no watcher.github-build subtree", func() {
           It("returns empty config (subtree isolation)", func() {
               content := []byte(`watcher:
  github-pr:
    assignee: pr-reviewer-agent
`)
               fetcher.GetFileContentReturns(content, nil)
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg).To(Equal(maintenance.GithubBuildConfig{}))
           })
       })

       Context("valid YAML — all three keys set", func() {
           It("returns all three overrides", func() {
               content := []byte(`watcher:
  github-build:
    assignee: go-deps-fixer-agent
    status: backlog
    phase: planning
`)
               fetcher.GetFileContentReturns(content, nil)
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg.Assignee).To(Equal("go-deps-fixer-agent"))
               Expect(cfg.Status).To(Equal("backlog"))
               Expect(cfg.Phase).To(Equal("planning"))
           })
       })

       Context("valid YAML — only assignee set", func() {
           It("returns assignee override; status and phase are empty string", func() {
               content := []byte(`watcher:
  github-build:
    assignee: go-deps-fixer-agent
`)
               fetcher.GetFileContentReturns(content, nil)
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg.Assignee).To(Equal("go-deps-fixer-agent"))
               Expect(cfg.Status).To(Equal(""))
               Expect(cfg.Phase).To(Equal(""))
           })
       })

       Context("valid YAML — assignee is empty string", func() {
           It("treats empty string as absent (returns empty Assignee)", func() {
               content := []byte(`watcher:
  github-build:
    assignee: ""
`)
               fetcher.GetFileContentReturns(content, nil)
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg.Assignee).To(Equal(""))
           })
       })

       Context("valid YAML — unknown key in watcher.github-build", func() {
           It("applies known keys and ignores unknown ones without error", func() {
               content := []byte(`watcher:
  github-build:
    assignee: go-deps-fixer-agent
    priority: high
`)
               fetcher.GetFileContentReturns(content, nil)
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg.Assignee).To(Equal("go-deps-fixer-agent"))
               // No panic; unknown "priority" key is ignored
           })
       })

       Context("fetch passes the correct ref to GetFileContent", func() {
           It("uses the defaultBranch as the ref", func() {
               fetcher.GetFileContentReturns(nil, nil)
               loader.LoadOverrides(ctx, "myorg", "myrepo", "develop")
               _, calledOwner, calledRepo, calledPath, calledRef := fetcher.GetFileContentArgsForCall(0)
               Expect(calledOwner).To(Equal("myorg"))
               Expect(calledRepo).To(Equal("myrepo"))
               Expect(calledPath).To(Equal(".maintenance.yaml"))
               Expect(calledRef).To(Equal("develop"))
           })
       })

       Context("GitHub fetch returns ErrRateLimited", func() {
           It("returns empty config without erroring (caller continues with defaults)", func() {
               fetcher.GetFileContentReturns(nil, pkg.ErrRateLimited)
               cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
               Expect(cfg.Assignee).To(BeEmpty())
               Expect(cfg.Status).To(BeEmpty())
               Expect(cfg.Phase).To(BeEmpty())
               Expect(cfg.PhaseSet).To(BeFalse())
               // WARN log emitted; no panic; publish proceeds with defaults
           })
       })
   })
   ```

8. **Generate counterfeiter mocks** for the two new interfaces:

   ```bash
   cd watcher/github-build && go generate ./pkg/maintenance/...
   ```

   If `go generate` is not configured, run counterfeiter directly for each:

   ```bash
   cd watcher/github-build && \
     go run github.com/maxbrunsfeld/counterfeiter/v6 \
       -o pkg/mocks/file_content_fetcher.go \
       --fake-name FileContentFetcher \
       ./pkg/maintenance/. FileContentFetcher

   cd watcher/github-build && \
     go run github.com/maxbrunsfeld/counterfeiter/v6 \
       -o pkg/mocks/maintenance_loader.go \
       --fake-name MaintenanceLoader \
       ./pkg/maintenance/. Loader
   ```

   Confirm `pkg/mocks/file_content_fetcher.go` and `pkg/mocks/maintenance_loader.go` are created.

9. **Add `gopkg.in/yaml.v3` to direct dependencies** (it is currently indirect in go.mod):

   ```bash
   cd watcher/github-build && go get gopkg.in/yaml.v3 && go mod tidy
   ```

10. **Run `make test`** to verify everything compiles and passes:

    ```bash
    cd watcher/github-build && make test
    ```

    Fix any compile errors before proceeding to `make precommit`.

11. **Run `make precommit`** in `watcher/github-build/`:

    ```bash
    cd watcher/github-build && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/`; do NOT touch CHANGELOG.md or docs/ in this prompt (prompt 2 handles those)
- Do NOT commit — dark-factory handles git
- Do NOT wire the loader into `watcher.go`, `factory.go`, or `main.go` — that is prompt 2's responsibility
- `GetFileContent` must return `(nil, nil)` for HTTP 404 — silent, not an error — because 404 is the common case (most repos won't have `.maintenance.yaml`)
- `GetFileContent` must return `(nil, ErrRateLimited)` for rate-limit errors (reuse the existing sentinel from `pkg/githubclient.go`)
- `GetFileContent` must reject files > 1 MiB by returning `(nil, error)` — NOT by panicking or silently returning truncated content
- `LoadOverrides` MUST never return an error — all failure modes are handled internally with `glog.Warningf` (or `glog.Infof` for unknown keys) and a `GithubBuildConfig{}` return
- Empty string values in YAML (`assignee: ""`) MUST return an empty string in `GithubBuildConfig.Assignee` — the watcher (prompt 2) is responsible for "empty override = fall through to default" logic
- `LoadOverrides` MUST NOT log anything for 404 (file absent = normal; log would be noise on every publish for repos without the file)
- The `FileContentFetcher` interface MUST be in `pkg/maintenance/` (not added to `pkg.GitHubClient`); BUT `pkg.GitHubClient` MUST also include `GetFileContent` so the factory (prompt 2) can pass `ghClient` as `FileContentFetcher` via Go structural typing
- `Loader` mock MUST be at `pkg/mocks/maintenance_loader.go` with fake name `MaintenanceLoader`
- `FileContentFetcher` mock MUST be at `pkg/mocks/file_content_fetcher.go` with fake name `FileContentFetcher`
- YAML library MUST be `gopkg.in/yaml.v3` (already in go.mod as indirect; make direct via `go get`)
- Error wrapping: `github.com/bborbe/errors`; never `fmt.Errorf`
- `make precommit` runs from `watcher/github-build/`, never at repo root
- All existing tests (cursor, filter, watcher, githubclient) must still pass
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm GetFileContent added to GitHubClient interface:
grep -n "GetFileContent" watcher/github-build/pkg/githubclient.go
# Expected: at least 2 matches — interface declaration + concrete method

# Confirm 404 returns (nil, nil):
grep -A 5 "StatusNotFound" watcher/github-build/pkg/githubclient.go
# Expected: return nil, nil

# Confirm size check returns error (not panic):
grep -n "1024.*1024\|too large" watcher/github-build/pkg/githubclient.go
# Expected: at least one match — the size guard

# Confirm maintenance package exists:
ls watcher/github-build/pkg/maintenance/
# Expected: doc.go, loader.go, loader_test.go, suite_test.go

# Confirm Loader interface and FileContentFetcher interface:
grep -n "type Loader interface\|type FileContentFetcher interface\|LoadOverrides\|GetFileContent" \
  watcher/github-build/pkg/maintenance/loader.go
# Expected: both interface declarations + method signatures

# Confirm mocks generated:
ls watcher/github-build/pkg/mocks/
# Expected: includes file_content_fetcher.go and maintenance_loader.go

# Confirm GithubBuildConfig struct fields:
grep -n "Assignee\|Status\|Phase" watcher/github-build/pkg/maintenance/loader.go
# Expected: struct fields + extraction in LoadOverrides

# Confirm silent 404 (no glog.Warning on nil content):
grep -n "Warningf\|Infof" watcher/github-build/pkg/maintenance/loader.go
# Expected: Warningf only in error/malformed paths; no Warningf in the nil-content path

# Confirm unknown-key INFO log:
grep -n "ignored unknown key\|Infof" watcher/github-build/pkg/maintenance/loader.go
# Expected: glog.Infof for unknown keys

# Confirm gopkg.in/yaml.v3 is a direct dep in go.mod:
grep "gopkg.in/yaml.v3" watcher/github-build/go.mod
# Expected: line without "// indirect"

# Confirm GitHubClient mock includes GetFileContent stub:
grep -n "GetFileContent" watcher/github-build/pkg/mocks/github_client.go
# Expected: GetFileContentStub, GetFileContentCallCount, etc.
</verification>
