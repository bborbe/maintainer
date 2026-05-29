---
status: approved
spec: ["052"]
created: "2026-05-29T00:00:00Z"
queued: "2026-05-29T16:22:07Z"
---

<summary>
- The github-releaser agent can now authenticate as a GitHub App installation, not just with a personal access token.
- When App credentials are configured (App ID + installation ID + a private key), the agent mints a short-lived installation token at startup and uses it for everything.
- The legacy `GH_TOKEN` personal access token still works as a fallback for local development and tests.
- When both App credentials and `GH_TOKEN` are set, App auth wins and the agent logs that `GH_TOKEN` is being ignored.
- When neither is configured, the agent refuses to start with a clear error naming the required variables — it never clones, fetches, or pushes.
- The single resolved credential (minted token or PAT) flows to both the changelog fetch and the release push, exactly as the single token does today.
- Environment variable names match the pr-reviewer agent exactly (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`, `PEM_KEY`) so cluster config maps identically.
- Both entry points (the Kafka pod entry point and the local-CLI `run-task` entry point) get the same resolution.
</summary>

<objective>
Migrate `agent/github-releaser` from PAT-only auth (`GH_TOKEN`) to GitHub App installation-token auth, mirroring the resolution already shipped in `agent/pr-reviewer`. When App credentials are present the binary mints an installation access token at startup and uses it as the single effective token for both the planning changelog fetch and the execution push; `GH_TOKEN` stays accepted as a fallback; with neither configured the binary errors before any clone. Env-var names and resolution order match pr-reviewer exactly.
</objective>

<context>
Read before writing code (repo-relative paths; the YOLO container mounts the repo root):

- `CLAUDE.md` at repo root — project conventions.
- `specs/in-progress/052-github-releaser-app-auth.md` — re-read Summary, Goal, Non-goals, Desired Behavior 1-4, Constraints, the Failure Modes table (6 rows), Security/Abuse Cases, and the Acceptance Criteria. This prompt covers all of it. The implementation note at the bottom of the spec is honored by extracting the resolution into a small testable package (see below).

- `agent/pr-reviewer/main.go` — the reference implementation. Mirror exactly:
  - The four App config struct fields (lines ~84-87): `AppID int64` env `APP_ID`, `InstallationID int64` env `INSTALLATION_ID`, `PEMKeyFile string` env `PEM_KEY_FILE`, `PEMKey string` env `PEM_KEY` with `display:"length"`.
  - `resolveAuth(ctx)` (lines ~231-280): the App-vs-PAT selection rule, the "both set → App wins, GH_TOKEN ignored" warning, minting via `githubapp.MintIAT` preferring `PEMPath` (file) over `PEM` (env content), and the "neither configured" error.
- `agent/pr-reviewer/pkg/authmode.go` — `AuthMode` enum (`AuthModeNone`/`AuthModeGitHubApp`/`AuthModePATFallback`) + `ResolveAuthMode(appID, installationID int64, pemKeyFile, ghToken string) AuthMode`. NOTE: pr-reviewer's `ResolveAuthMode` keys App mode on `pemKeyFile` only. The releaser MUST additionally accept `PEM_KEY` (env content) per spec Desired Behavior 2 — so the releaser's resolver takes BOTH pem args and selects App mode when AppID>0 AND InstallationID>0 AND (pemKeyFile set OR pemKey set). This is the documented divergence; everything else mirrors pr-reviewer.
- `agent/pr-reviewer/cmd/run-task/main.go` (lines ~117-148) — how the CLI entry point resolves auth inline (calls `ResolveAuthMode`, switches, mints via `githubapp.MintIAT`, mutates `a.GHToken`). The releaser's `cmd/run-task/main.go` is the sibling CLI entry point and gets the SAME resolution.

- `lib/githubapp/githubapp.go` — the mint helper. Verified signatures:
  - `func MintIAT(ctx context.Context, cfg Config) (string, error)` — returns the raw IAT string.
  - `type Config struct { AppID int64; InstallationID int64; PEM []byte; PEMPath string; BaseURL string }`. Exactly one of `PEM` / `PEMPath` must be set (validated inside). `BaseURL` overrides `https://api.github.com` for `httptest`-backed tests.
  - Do NOT hand-roll JWT — call `MintIAT`. (AC greps `golang-jwt` == 0 in main.go.)
- `lib/githubapp/githubapp_test.go` — the EXACT httptest IAT-endpoint test pattern to copy: a `httptest.NewServer` that responds to `POST /app/installations/<id>/access_tokens` with `{"token":"ghs_...","expires_at":"2099-01-01T00:00:00Z"}`, `generateRSAKey()` to make a valid PEM, and `Config.BaseURL: server.URL`. Reuse the `generateRSAKey()` helper verbatim and the access-tokens handler verbatim.

- `agent/github-releaser/main.go` — the Kafka entry point being migrated. Current state:
  - Single auth field `GHToken string` env `GH_TOKEN` (line ~75) with comment "PAT for now; App auth in a follow-up spec."
  - `Run(ctx, _)` calls `factory.CreateAgentProvider(a.ClaudeConfigDir, a.AgentDir, a.AnthropicModel, a.GHToken, env)` (line ~107) and `buildEnv()` (line ~134) sets `env["GH_TOKEN"] = a.GHToken` when non-empty. No `resolveAuth` exists yet.
- `agent/github-releaser/cmd/run-task/main.go` — the local-CLI entry point. Same single `GHToken` field (line ~44), inline `env` build (line ~62), and `factory.CreateAgentProvider(..., a.GHToken, env)` (line ~75).
- `agent/github-releaser/pkg/factory/factory.go` — how the single `ghToken` flows downstream. `CreateAgentProvider(..., ghToken string, env) → CreateAgent(..., ghToken, env) → githubchangelog.NewHTTPFetcher(ghToken)` (line ~107) AND `releaserpkg.NewExecutionStep(executionOps, ghToken)` (line ~111). DO NOT change any factory signature — the resolved token replaces `a.GHToken` in place, exactly as pr-reviewer mutates `a.GHToken`, so the existing wiring is unchanged.
- `agent/github-releaser/main_test.go` and `agent/github-releaser/cmd/run-task/main_test.go` — both are `package main_test` with a single "Compiles" `gexec.Build` spec + `TestSuite`. Unchanged.
- `agent/github-releaser/pkg/pkg_suite_test.go` — the Ginkgo suite-runner pattern to mirror for the new package's suite file (`RegisterFailHandler`, `GinkgoConfiguration`, 60s timeout, `RunSpecs`).
- `agent/github-releaser/go.mod` — module `github.com/bborbe/maintainer/agent/github-releaser`; has `replace github.com/bborbe/maintainer/lib => ../../lib`. It does NOT yet require `lib/githubapp`'s transitive deps (`ghinstallation/v2`). After adding the `lib/githubapp` import you MUST run `go mod tidy` in `agent/github-releaser/` to pull `github.com/bradleyfalzon/ghinstallation/v2` into go.mod/go.sum.
- `CHANGELOG.md` at repo root — add ONE new bullet at the TOP of the `## Unreleased` block.

Coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` context-form patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — Create* convention (factory unchanged here, but read for the no-business-logic rule).
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — Unreleased bullet format.
</context>

<requirements>

Run order: do the steps in sequence. After step 3 run `cd agent/github-releaser && go mod tidy && go build ./...` to catch missing-dep + type errors early. Run `cd agent/github-releaser && go test ./pkg/githubauth/...` after step 4. Run `cd agent/github-releaser && make precommit` only as the final verification step.

1. **Create `agent/github-releaser/pkg/githubauth/githubauth.go`** — a new testable package holding the auth-resolution logic. This is the clean, testable mirror of pr-reviewer's `resolveAuth` (which lives in `package main` and is not unit-testable from an external test package). Production `main.go` and `cmd/run-task/main.go` call `Resolve`.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package githubauth resolves the github-releaser agent's effective GitHub
   // credential at startup. It mirrors the pr-reviewer agent's resolution
   // order: GitHub App installation token (preferred) → GH_TOKEN PAT
   // (fallback) → startup error. Extracted into its own package so the
   // resolution outcomes are unit-testable against an httptest IAT endpoint
   // (the pattern lib/githubapp tests already use).
   package githubauth

   import (
   	"context"

   	"github.com/bborbe/errors"
   	"github.com/golang/glog"

   	githubapp "github.com/bborbe/maintainer/lib/githubapp"
   )

   // AuthMode classifies which credential type is active at pod startup.
   type AuthMode int

   const (
   	// AuthModeNone means no usable credential is configured; the caller
   	// MUST refuse to start.
   	AuthModeNone AuthMode = iota
   	// AuthModeGitHubApp means App credentials are present and an IAT will
   	// be minted.
   	AuthModeGitHubApp
   	// AuthModePATFallback means the legacy GH_TOKEN PAT is used.
   	AuthModePATFallback
   )

   // Config carries the raw credential inputs read from env/flags. Either a
   // PEM file path (PEMKeyFile) or PEM env content (PEMKey) may be supplied;
   // PEMKeyFile is preferred when both are present. BaseURL overrides the
   // GitHub API base (defaults to https://api.github.com); tests point it at
   // an httptest server.
   type Config struct {
   	AppID          int64
   	InstallationID int64
   	PEMKeyFile     string
   	PEMKey         string
   	GHToken        string
   	BaseURL        string
   }

   // ResolveAuthMode picks the credential type to use at startup.
   //   - AppID>0 AND InstallationID>0 AND (PEMKeyFile set OR PEMKey set) → AuthModeGitHubApp
   //   - else GHToken non-empty → AuthModePATFallback
   //   - else → AuthModeNone
   //
   // NOTE: unlike pr-reviewer's ResolveAuthMode (which keys App mode on the
   // PEM file path only), the releaser accepts PEM_KEY env content too, per
   // spec 052 Desired Behavior 2.
   func ResolveAuthMode(appID, installationID int64, pemKeyFile, pemKey, ghToken string) AuthMode {
   	hasPEM := pemKeyFile != "" || pemKey != ""
   	if appID > 0 && installationID > 0 && hasPEM {
   		return AuthModeGitHubApp
   	}
   	if ghToken != "" {
   		return AuthModePATFallback
   	}
   	return AuthModeNone
   }

   // Resolve returns the single effective GitHub token for the agent.
   //
   //   - App mode: mints an installation access token via lib/githubapp.MintIAT
   //     (preferring PEMKeyFile over PEMKey when both are set). When GH_TOKEN
   //     is ALSO set, logs that App wins and GH_TOKEN is ignored.
   //   - PAT fallback: returns GHToken, logging a pat-fallback warning.
   //   - None: returns a non-nil error naming the required env vars (both
   //     APP_ID and GH_TOKEN appear in the message). Returns BEFORE any clone.
   //
   // The returned token is the bearer credential wired to BOTH the planning
   // fetcher and the execution push. It is never logged in full (MintIAT logs
   // only token_prefix).
   func Resolve(ctx context.Context, cfg Config) (string, error) {
   	switch ResolveAuthMode(cfg.AppID, cfg.InstallationID, cfg.PEMKeyFile, cfg.PEMKey, cfg.GHToken) {
   	case AuthModeGitHubApp:
   		if cfg.GHToken != "" {
   			glog.Warningf(
   				"github-releaser auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
   			)
   		}
   		appCfg := githubapp.Config{
   			AppID:          cfg.AppID,
   			InstallationID: cfg.InstallationID,
   			BaseURL:        cfg.BaseURL,
   		}
   		if cfg.PEMKeyFile != "" {
   			appCfg.PEMPath = cfg.PEMKeyFile
   		} else {
   			appCfg.PEM = []byte(cfg.PEMKey)
   		}
   		iat, err := githubapp.MintIAT(ctx, appCfg)
   		if err != nil {
   			return "", errors.Wrap(ctx, err, "mint github app iat")
   		}
   		glog.V(2).Infof(
   			"github-releaser auth mode=github-app app_id=%d installation_id=%d",
   			cfg.AppID, cfg.InstallationID,
   		)
   		return iat, nil
   	case AuthModePATFallback:
   		glog.Warningf(
   			"github-releaser auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)",
   		)
   		return cfg.GHToken, nil
   	default:
   		return "", errors.Errorf(
   			ctx,
   			"github-releaser auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY_FILE (or PEM_KEY), or set GH_TOKEN",
   		)
   	}
   }
   ```

   Notes:
   - All errors via `github.com/bborbe/errors` context-form (`errors.Wrap`/`errors.Errorf`, ctx first). NO `fmt.Errorf`. NO `context.Background()` anywhere in this file.
   - The "neither" error message MUST contain both the substring `APP_ID` and the substring `GH_TOKEN` (AC grep target on the test).

2. **Create `agent/github-releaser/pkg/githubauth/githubauth_suite_test.go`** — Ginkgo suite runner. Mirror `agent/github-releaser/pkg/pkg_suite_test.go` exactly, package `githubauth_test`, suite name `"GitHubAuth Suite"`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package githubauth_test

   import (
   	"testing"
   	"time"

   	. "github.com/onsi/ginkgo/v2"
   	. "github.com/onsi/gomega"
   	"github.com/onsi/gomega/format"
   )

   func TestSuite(t *testing.T) {
   	time.Local = time.UTC
   	format.TruncatedDiff = false
   	RegisterFailHandler(Fail)
   	suiteConfig, reporterConfig := GinkgoConfiguration()
   	suiteConfig.Timeout = 60 * time.Second
   	RunSpecs(t, "GitHubAuth Suite", suiteConfig, reporterConfig)
   }
   ```

3. **Create `agent/github-releaser/pkg/githubauth/githubauth_test.go`** — external test package (`package githubauth_test`). Covers the four observable resolution outcomes from the spec ACs, using the EXACT httptest IAT-endpoint pattern from `lib/githubapp/githubapp_test.go`. Copy the `generateRSAKey()` helper and the access-tokens handler verbatim from that file.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package githubauth_test

   import (
   	"context"
   	"crypto/rand"
   	"crypto/rsa"
   	"crypto/x509"
   	"encoding/pem"
   	"net/http"
   	"net/http/httptest"

   	. "github.com/onsi/ginkgo/v2"
   	. "github.com/onsi/gomega"

   	"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubauth"
   )

   const stubIAT = "ghs_test123456789"

   // newIATServer returns an httptest server that mints stubIAT on the
   // installation access-tokens endpoint. Mirrors lib/githubapp tests.
   func newIATServer(installationID string) *httptest.Server {
   	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   		if r.URL.Path == "/app/installations/"+installationID+"/access_tokens" &&
   			r.Method == http.MethodPost {
   			w.Header().Set("Content-Type", "application/json")
   			w.WriteHeader(http.StatusOK)
   			_, _ = w.Write(
   				[]byte(`{"token":"` + stubIAT + `","expires_at":"2099-01-01T00:00:00Z"}`),
   			)
   			return
   		}
   		http.NotFound(w, r)
   	}))
   }

   var _ = Describe("Resolve", func() {
   	var server *httptest.Server

   	AfterEach(func() {
   		if server != nil {
   			server.Close()
   			server = nil
   		}
   	})

   	It("App creds set and GH_TOKEN empty → effective token is the minted IAT", func(ctx context.Context) {
   		server = newIATServer("456")
   		token, err := githubauth.Resolve(ctx, githubauth.Config{
   			AppID:          123,
   			InstallationID: 456,
   			PEMKey:         string(generateRSAKey()),
   			BaseURL:        server.URL,
   		})
   		Expect(err).NotTo(HaveOccurred())
   		Expect(token).To(Equal(stubIAT))
   	})

   	It("GH_TOKEN set and no App creds → effective token is the PAT", func(ctx context.Context) {
   		token, err := githubauth.Resolve(ctx, githubauth.Config{
   			GHToken: "pat-abc",
   		})
   		Expect(err).NotTo(HaveOccurred())
   		Expect(token).To(Equal("pat-abc"))
   	})

   	It("both App creds and GH_TOKEN set → App wins (token is the IAT, not the PAT)", func(ctx context.Context) {
   		server = newIATServer("456")
   		token, err := githubauth.Resolve(ctx, githubauth.Config{
   			AppID:          123,
   			InstallationID: 456,
   			PEMKey:         string(generateRSAKey()),
   			GHToken:        "pat-abc",
   			BaseURL:        server.URL,
   		})
   		Expect(err).NotTo(HaveOccurred())
   		Expect(token).To(Equal(stubIAT))
   		Expect(token).NotTo(Equal("pat-abc"))
   	})

   	It("neither App creds nor GH_TOKEN → error naming APP_ID and GH_TOKEN", func(ctx context.Context) {
   		token, err := githubauth.Resolve(ctx, githubauth.Config{})
   		Expect(err).To(HaveOccurred())
   		Expect(err.Error()).To(ContainSubstring("APP_ID"))
   		Expect(err.Error()).To(ContainSubstring("GH_TOKEN"))
   		Expect(token).To(BeEmpty())
   	})

   	It("App creds with PEM file path → mints IAT (PEMKeyFile preferred over PEMKey)", func(ctx context.Context) {
   		server = newIATServer("456")
   		pemPath := writeTempPEM()
   		token, err := githubauth.Resolve(ctx, githubauth.Config{
   			AppID:          123,
   			InstallationID: 456,
   			PEMKeyFile:     pemPath,
   			BaseURL:        server.URL,
   		})
   		Expect(err).NotTo(HaveOccurred())
   		Expect(token).To(Equal(stubIAT))
   	})

   	It("malformed PEM → mint error before any token returned", func(ctx context.Context) {
   		token, err := githubauth.Resolve(ctx, githubauth.Config{
   			AppID:          123,
   			InstallationID: 456,
   			PEMKey:         "not-a-valid-pem",
   		})
   		Expect(err).To(HaveOccurred())
   		Expect(token).To(BeEmpty())
   	})
   })

   // ResolveAuthMode is exercised directly to lock the App-vs-PAT decision
   // table independent of network.
   var _ = Describe("ResolveAuthMode", func() {
   	It("App when AppID+InstallationID+PEMKeyFile set", func() {
   		Expect(githubauth.ResolveAuthMode(1, 2, "/k.pem", "", "")).
   			To(Equal(githubauth.AuthModeGitHubApp))
   	})
   	It("App when AppID+InstallationID+PEMKey (env content) set", func() {
   		Expect(githubauth.ResolveAuthMode(1, 2, "", "pem-content", "")).
   			To(Equal(githubauth.AuthModeGitHubApp))
   	})
   	It("App wins when both App creds and GH_TOKEN set", func() {
   		Expect(githubauth.ResolveAuthMode(1, 2, "/k.pem", "", "pat")).
   			To(Equal(githubauth.AuthModeGitHubApp))
   	})
   	It("PAT fallback when only GH_TOKEN set", func() {
   		Expect(githubauth.ResolveAuthMode(0, 0, "", "", "pat")).
   			To(Equal(githubauth.AuthModePATFallback))
   	})
   	It("PAT fallback when App ids present but no PEM", func() {
   		Expect(githubauth.ResolveAuthMode(1, 2, "", "", "pat")).
   			To(Equal(githubauth.AuthModePATFallback))
   	})
   	It("None when nothing set", func() {
   		Expect(githubauth.ResolveAuthMode(0, 0, "", "", "")).
   			To(Equal(githubauth.AuthModeNone))
   	})
   })

   // writeTempPEM writes a valid RSA PEM to a temp file and returns its path.
   func writeTempPEM() string {
   	f, err := os.CreateTemp("", "githubauth-test-*.pem")
   	Expect(err).NotTo(HaveOccurred())
   	defer func() { _ = f.Close() }()
   	_, err = f.Write(generateRSAKey())
   	Expect(err).NotTo(HaveOccurred())
   	return f.Name()
   }

   // generateRSAKey generates a valid RSA private key for testing.
   // Copied verbatim from lib/githubapp/githubapp_test.go.
   func generateRSAKey() []byte {
   	key, err := rsa.GenerateKey(rand.Reader, 2048)
   	if err != nil {
   		panic(err)
   	}
   	block := &pem.Block{
   		Type:  "RSA PRIVATE KEY",
   		Bytes: x509.MarshalPKCS1PrivateKey(key),
   	}
   	return pem.EncodeToMemory(block)
   }
   ```

   Notes:
   - Add `"os"` to the import block (used by `writeTempPEM`). Keep imports alphabetized/grouped per the existing style.
   - The httptest server installation id in the path (`"456"`) MUST match `InstallationID: 456` in the configs — `ghinstallation/v2` builds the access-tokens URL from the installation id.
   - These tests need no Docker, no real `gh`, no cluster — the IAT endpoint is a local httptest server (the exact pattern `lib/githubapp` tests use).

4. **Modify `agent/github-releaser/main.go`** (Kafka entry point) — add the four App config fields and call `githubauth.Resolve` at the top of `Run`, mutating `a.GHToken` in place so the existing factory wiring is unchanged.

   a. Add the import `"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubauth"` to the maintainer-import group (next to the existing `factory` import).

   b. Replace the single `GHToken` field block (the field with the comment "PAT for now; App auth in a follow-up spec.") with the PAT field plus the four App fields. Use env names IDENTICAL to pr-reviewer:

   ```go
   	// GitHub token for the planning fetcher and execution push.
   	// Accepted as a fallback when App credentials are not configured.
   	GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub PAT fallback for CHANGELOG fetch and push (App auth preferred)" display:"length"`

   	// GitHub App authentication. When AppID + InstallationID + (PEMKeyFile or
   	// PEMKey) are set, the pod mints an installation access token at startup
   	// and uses it in place of GHToken (see Run() for the resolution order).
   	AppID          int64  `required:"false" arg:"app-id"          env:"APP_ID"          usage:"GitHub App ID (numeric); when set, App auth is used instead of GH_TOKEN"`
   	InstallationID int64  `required:"false" arg:"installation-id" env:"INSTALLATION_ID" usage:"GitHub App Installation ID (numeric)"`
   	PEMKeyFile     string `required:"false" arg:"pem-key-file"    env:"PEM_KEY_FILE"    usage:"Path to the GitHub App private key (PEM file mounted from k8s Secret)"`
   	PEMKey         string `required:"false" arg:"pem-key"         env:"PEM_KEY"         usage:"GitHub App private key (PEM) as env var content; mutually exclusive with PEM_KEY_FILE" display:"length"`
   ```

   c. In `Run`, immediately after the `start := ...` / `glog.V(2).Infof("%s started phase=%s", ...)` lines and BEFORE `a.createDeliverer(ctx)`, resolve auth and fail before any work on error:

   ```go
   	resolvedToken, err := githubauth.Resolve(ctx, githubauth.Config{
   		AppID:          a.AppID,
   		InstallationID: a.InstallationID,
   		PEMKeyFile:     a.PEMKeyFile,
   		PEMKey:         a.PEMKey,
   		GHToken:        a.GHToken,
   	})
   	if err != nil {
   		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
   		jobMetrics.RecordDuration(time.Since(start))
   		return err
   	}
   	a.GHToken = resolvedToken
   ```

   Leave `Config.BaseURL` unset in production (defaults to the real GitHub API). Do NOT add a BaseURL config field or env var — it exists only for tests.

   The rest of `Run` is unchanged: `buildEnv()` reads `a.GHToken` (now the resolved token) into `env["GH_TOKEN"]`, and `factory.CreateAgentProvider(..., a.GHToken, env)` receives the resolved token. Do NOT change `buildEnv`, `createDeliverer`, or any factory call.

5. **Modify `agent/github-releaser/cmd/run-task/main.go`** (local-CLI entry point — sibling entry point, mirror the same change). Verified this file calls `factory.CreateAgentProvider(..., a.GHToken, env)` at line ~75 and builds `env` inline at line ~62.

   a. Add the import `"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubauth"` to the maintainer-import group.

   b. Replace the single `GHToken` field (line ~44) with the same PAT + four App fields block shown in step 4b (identical struct-tag text).

   c. At the top of `Run`, after the `os.ReadFile(a.TaskFilePath)` block (or before the `env := map[string]string{}` build — either is fine as long as it precedes the factory call), resolve auth:

   ```go
   	resolvedToken, err := githubauth.Resolve(ctx, githubauth.Config{
   		AppID:          a.AppID,
   		InstallationID: a.InstallationID,
   		PEMKeyFile:     a.PEMKeyFile,
   		PEMKey:         a.PEMKey,
   		GHToken:        a.GHToken,
   	})
   	if err != nil {
   		return err
   	}
   	a.GHToken = resolvedToken
   ```

   Note: this file already declares `err` later in `Run` (`taskContent, err := os.ReadFile(...)`). If the resolve block is placed BEFORE the `os.ReadFile` block, use `:=` here and change the later `os.ReadFile` line's `taskContent, err :=` to `taskContent, err =` (or vice-versa) so there is exactly one `:=` declaration of `err` in the function. Ensure the file compiles — `go build ./...` will catch a redeclaration.

   The inline `env` build and the `factory.CreateAgentProvider(..., a.GHToken, env)` call are otherwise unchanged.

6. **Run `go mod tidy`** in `agent/github-releaser/` to pull `github.com/bradleyfalzon/ghinstallation/v2` (the transitive dep of `lib/githubapp.MintIAT`) into the releaser's go.mod and go.sum:

   ```bash
   cd agent/github-releaser && go mod tidy
   ```

   Confirm `github.com/bradleyfalzon/ghinstallation/v2` now appears in `agent/github-releaser/go.mod`. Do NOT add `golang-jwt` directly — it arrives transitively and is not imported by releaser code (AC greps `golang-jwt` == 0 in main.go).

7. **Update root `CHANGELOG.md`** — add ONE new bullet at the TOP of the `## Unreleased` block (above the current top `feat(watcher/github-release): add /trigger ...` bullet). The bullet MUST mention the agent and App auth:

   ```
   - feat(agent/github-releaser): migrate to GitHub App installation-token auth — mints an IAT at startup from APP_ID + INSTALLATION_ID + PEM_KEY_FILE/PEM_KEY (via lib/githubapp.MintIAT), falls back to GH_TOKEN PAT, errors before any clone when neither is set; the resolved token flows to both the changelog fetch and the release push, mirroring pr-reviewer (spec 052)
   ```

8. **Final verification** — from `agent/github-releaser/`:

   ```bash
   make precommit
   ```

   Must exit 0; all Ginkgo suites green; coverage gate satisfied.

</requirements>

<constraints>
- New files:
  - `agent/github-releaser/pkg/githubauth/githubauth.go`
  - `agent/github-releaser/pkg/githubauth/githubauth_suite_test.go`
  - `agent/github-releaser/pkg/githubauth/githubauth_test.go`
- Modified files:
  - `agent/github-releaser/main.go` (four App fields + `githubauth.Resolve` call at top of `Run`)
  - `agent/github-releaser/cmd/run-task/main.go` (same four App fields + `githubauth.Resolve` call)
  - `agent/github-releaser/go.mod` + `go.sum` (via `go mod tidy` — adds `ghinstallation/v2`)
  - `CHANGELOG.md` at repo root (one new Unreleased bullet at the top)
- Module: `agent/github-releaser/` has its own `go.mod`. Build/verify with `make precommit` in that directory ONLY.
- Env-var names IDENTICAL to pr-reviewer: `APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`, `PEM_KEY`. `PEM_KEY` carries `display:"length"` so its content is never printed by the config dumper.
- Resolution order: App (AppID>0 AND InstallationID>0 AND (PEM_KEY_FILE set OR PEM_KEY set)) wins → else GH_TOKEN PAT fallback → else error before any clone. App wins over PAT; both-set logs that GH_TOKEN is ignored.
- Mint via `lib/githubapp.MintIAT` ONLY — do NOT hand-roll JWT or IAT exchange. Prefer `PEMPath` (PEMKeyFile) over `PEM` (PEMKey) when both are present, mirroring pr-reviewer. Input validation (positive ids, PEM/PEMPath mutual exclusivity) is enforced inside `lib/githubapp` — do NOT re-validate.
- The single resolved token (minted IAT or PAT) replaces `a.GHToken` in place; the existing factory wiring (`CreateAgentProvider(..., ghToken, env) → NewHTTPFetcher` AND `NewExecutionStep`) is UNCHANGED. Do NOT introduce a second token. Do NOT change any `factory.Create*` signature.
- Error wrapping: `github.com/bborbe/errors` context-form ONLY (`errors.New/Wrap/Errorf/Wrapf`, ctx first). NEVER `fmt.Errorf`. No `context.Background()` in business logic (use the `ctx` passed into `Resolve`/`Run`).
- The minted IAT is a bearer secret. Never log the full token; `MintIAT` already logs only `token_prefix`. Releaser code logs only mode + ids (`app_id`, `installation_id`).
- Tests: Ginkgo v2 + Gomega, external `_test` package. Auth-resolution tests use `Config.BaseURL` pointed at an `httptest` IAT endpoint (the `lib/githubapp` test pattern) — no Docker, no real `gh`, no cluster.
- No releaser code change may alter the pr-reviewer agent or its auth.
- License header (3 lines) at the top of every new `.go` file.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` green before AND after.
- Out of scope (do NOT touch): k8s manifests / CRD / Secret; the pre-push diff guard or `unexpected_diff` category; the `ai_review` phase; splitting fetch-token from push-token. No scenario file (per spec — behavior is fully reachable by the unit tests above).
</constraints>

<verification>

Run from repo root unless noted.

```bash
# Build + tests + coverage (the gate)
cd agent/github-releaser && make precommit                                      # exit 0
cd agent/github-releaser && go test ./pkg/githubauth/...                        # green

# Config struct declares the four App fields with pr-reviewer env names
grep -nE 'env:"(APP_ID|INSTALLATION_ID|PEM_KEY_FILE|PEM_KEY)"' agent/github-releaser/main.go   # 4 lines
grep -n 'env:"PEM_KEY"' agent/github-releaser/main.go | grep -c 'display:"length"'             # 1

# App-mode mints via lib/githubapp; no hand-rolled JWT
grep -rn 'githubapp.MintIAT' agent/github-releaser/pkg/githubauth/githubauth.go                # ≥1
grep -nc 'golang-jwt' agent/github-releaser/main.go                                            # 0
grep -nc 'golang-jwt' agent/github-releaser/pkg/githubauth/githubauth.go                       # 0

# No fmt.Errorf / context.Background in the resolution code
grep -nc 'fmt.Errorf' agent/github-releaser/pkg/githubauth/githubauth.go                       # 0
grep -nc 'context.Background' agent/github-releaser/pkg/githubauth/githubauth.go               # 0

# Resolve called at both entry points; token mutated in place
grep -nc 'githubauth.Resolve' agent/github-releaser/main.go                                    # ≥1
grep -nc 'githubauth.Resolve' agent/github-releaser/cmd/run-task/main.go                       # ≥1

# Factory wiring unchanged (single ghToken still flows to both consumers)
grep -nc 'githubchangelog.NewHTTPFetcher(ghToken)' agent/github-releaser/pkg/factory/factory.go  # 1
grep -nc 'NewExecutionStep(executionOps, ghToken)' agent/github-releaser/pkg/factory/factory.go  # 1

# Transitive dep present after tidy
grep -nc 'ghinstallation/v2' agent/github-releaser/go.mod                                       # ≥1

# Root CHANGELOG bullet within the Unreleased section
awk '/^## Unreleased$/,/^## v/' CHANGELOG.md | grep -c 'GitHub App'                             # ≥1
```

</verification>
