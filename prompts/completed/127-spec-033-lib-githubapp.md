---
status: completed
spec: [033-migrate-pr-reviewer-to-github-app]
container: maintainer-pr-reviewer-app-exec-127-spec-033-lib-githubapp
dark-factory-version: v0.164.0
created: "2026-05-21T20:30:30Z"
queued: "2026-05-21T20:58:04Z"
started: "2026-05-21T20:59:46Z"
completed: "2026-05-21T21:11:03Z"
---

<summary>
- A new Go package `lib/githubapp` exists in the maintainer `lib/` module.
- The package exposes a factory that, given an App ID, Installation ID, and a PEM (string or file path), returns an `http.Client` whose transport injects a fresh installation access token (IAT) into every outgoing GitHub API request and caches/refreshes it transparently.
- The package exposes a one-shot `MintIAT` helper that returns the current IAT as a plain `ghs_...` string for callers (like `gh auth setup-git` and the Claude subprocess) that need a bearer token they can paste into `GH_TOKEN`.
- All IAT minting and JWT signing is delegated to `github.com/bradleyfalzon/ghinstallation/v2` — no hand-rolled JWT code in the production package.
- The package never logs the PEM contents or the full IAT body; only a short prefix may appear in logs.
- Ginkgo unit tests cover happy-path and error paths against a `httptest.Server` simulating the GitHub installations endpoint; coverage ≥ 80%.
</summary>

<objective>
Add a new shared package `lib/githubapp` to the maintainer `lib/` Go module that provides GitHub App authentication primitives (cached `http.Client` + one-shot IAT minter) so downstream services can authenticate to GitHub as an App instead of a user PAT. The package wraps `github.com/bradleyfalzon/ghinstallation/v2`; it does not reimplement JWT signing.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.

Read these coding guides in `~/.claude/plugins/marketplaces/coding/docs/` (the YOLO container's claude config dir; resolved at runtime by the agent):
- `go-factory-pattern.md` — factory function rules (pure composition, no I/O, no `context.Background()`)
- `go-testing-guide.md` — Ginkgo/Gomega conventions for the project
- `go-mocking-guide.md` — Counterfeiter mock generation, if internal interfaces are added
- `test-pyramid-triggers.md` — which test types to write per code change (default unit; add integration only when crossing a real out-of-process boundary)

If those paths are not visible in-container, fall back to `/coding:check-guides "go factory pattern testing mocking"`.

Read these files in the maintainer repo before making changes:
- `lib/repoallowlist/repoallowlist.go` — precedent for a new shared `lib/<pkg>/` subpackage; mirror its style and license-header pattern
- `lib/repoallowlist/repoallowlist_test.go` — Ginkgo test layout precedent
- `lib/repoallowlist/repoallowlist_suite_test.go` — suite bootstrap precedent
- `lib/go.mod` — current module path `github.com/bborbe/maintainer/lib`; current deps include `github.com/bborbe/errors`, `glog`, `ginkgo/v2`, `gomega`
- `lib/Makefile` — `make precommit` target used for verification
- `agent/pr-reviewer/cmd/mint-iat/main.go` — stdlib smoke-test reference; **the algorithm is correct, but the production package must use `ghinstallation/v2`, not reimplement JWT signing**
- `agent/pr-reviewer/docs/github-app-setup.md` — auth flow, App identity, permissions table; the package's behavior must match this document
- `agent/pr-reviewer/pkg/githubposter/poster.go` lines 21–30, 80–137, 389–416 — see how the existing `httpClient` + `ghToken` pair is consumed today; the new `http.Client` from `lib/githubapp` must be a drop-in replacement, and the IAT returned by the one-shot helper must work as a bearer token via either `Authorization: token <iat>` or `Authorization: Bearer <iat>` (both accepted by GitHub for IATs)

**Library docs**: <https://pkg.go.dev/github.com/bradleyfalzon/ghinstallation/v2> — read the `NewKeyFromFile`, `New`, and `Transport.Token` entry points. Note that `Transport` implements `http.RoundTripper` and `Token(ctx)` returns the current IAT string (used by our one-shot helper).
</context>

<requirements>
Execute steps in order. Each step is independently runnable; do not skip ahead.

---

## Step 1 — Add `ghinstallation/v2` dependency to `lib/go.mod`

```bash
(cd lib && go get github.com/bradleyfalzon/ghinstallation/v2@latest && go mod tidy)
```

Verify:

```bash
grep -n 'ghinstallation' lib/go.mod
```

Expected: at least one line referencing `github.com/bradleyfalzon/ghinstallation/v2` as a direct require. Transitive deps (`golang-jwt`, `google/go-github`) appear in `lib/go.sum` after `go mod tidy`.

---

## Step 2 — Create the package skeleton

Create directory: `lib/githubapp/`

Create file: `lib/githubapp/githubapp.go` with the BSD-style header (mirror `lib/repoallowlist/repoallowlist.go` lines 1–3 verbatim) and `package githubapp` declaration.

Create file: `lib/githubapp/githubapp_suite_test.go` mirroring `lib/repoallowlist/repoallowlist_suite_test.go` verbatim (only the package name changes to `githubapp_test`).

---

## Step 3 — Define the public API

In `lib/githubapp/githubapp.go`, define exactly these exported symbols:

```go
// Config carries the inputs needed to authenticate as a GitHub App installation.
//
// AppID and InstallationID are public values (visible in the App settings page
// and the installation URL respectively) and are safe to commit. PEM (or PEMPath)
// is the long-lived secret and MUST come from a Kubernetes Secret mount, never
// from a checked-in file.
//
// Exactly one of PEM or PEMPath must be non-empty; passing both is a
// configuration error.
type Config struct {
    AppID          int64
    InstallationID int64
    PEM            []byte // PEM content; mutually exclusive with PEMPath
    PEMPath        string // path to PEM file; mutually exclusive with PEM
}

// NewClient returns an *http.Client whose RoundTripper authenticates every
// outgoing request as the given App installation using a cached IAT.
//
// The first call mints a JWT, exchanges it for an IAT, and caches the IAT
// for ~50 minutes; subsequent calls reuse the cached IAT and refresh it
// transparently before expiry.
//
// Returns an error if the config is invalid (both PEM and PEMPath set, or
// neither set; AppID or InstallationID zero) or if the PEM cannot be parsed.
func NewClient(ctx context.Context, cfg Config) (*http.Client, error)

// MintIAT returns a current installation access token as a plain string
// (e.g. "ghs_...") suitable for use as a bearer credential in subprocess
// env (GH_TOKEN), `gh auth setup-git`, or any other caller that needs the
// raw token rather than an authenticated http.Client.
//
// The returned token is valid for up to 1 hour from GitHub's perspective.
// Callers that need long-lived authentication should use NewClient instead;
// callers that need to refresh a one-shot string token should call MintIAT
// again — the underlying ghinstallation/v2 transport caches across calls.
//
// Returns an error if the config is invalid or the IAT exchange fails.
func MintIAT(ctx context.Context, cfg Config) (string, error)
```

**Implementation rules:**

- Use `github.com/bradleyfalzon/ghinstallation/v2`. The internal flow is:
  1. Read PEM bytes (from `cfg.PEM` directly, or `os.ReadFile(cfg.PEMPath)` if path was provided). Use `filepath.Clean` on the path before passing to `os.ReadFile` to satisfy gosec G304 (same pattern as `agent/pr-reviewer/cmd/mint-iat/main.go::resolvePEM`).
  2. Build the `*ghinstallation.Transport` via `ghinstallation.New(http.DefaultTransport, cfg.AppID, cfg.InstallationID, pemBytes)`.
  3. For `NewClient`: wrap the transport in `&http.Client{Transport: transport}`.
  4. For `MintIAT`: call `transport.Token(ctx)` and return the string.
- All errors MUST be wrapped via `github.com/bborbe/errors` using `errors.Wrap` / `errors.Wrapf` / `errors.Errorf`. Do NOT use `fmt.Errorf` or stdlib `errors.New`.
- Logging: import `github.com/golang/glog`. At V(2) log a one-line startup message naming `app_id` and `installation_id` — explicitly do NOT log PEM bytes, PEM length, or the IAT body. If a debug breadcrumb of the IAT is genuinely useful, log only the first 8 characters of the token followed by `...` (this mirrors how the existing maintainer code redacts secrets — see `agent/pr-reviewer/main.go` `display:"length"` flags).
- All exported symbols MUST have a godoc comment that starts with the symbol name.
- BSD-style license header on every new `.go` file (verbatim three lines from `lib/repoallowlist/repoallowlist.go`).

---

## Step 4 — Validate the config

Add a private validator. Either an unexported function `validate(cfg Config) error` or a method `func (c Config) validate() error` — pick whichever fits the package style. The check enforces:

- `cfg.AppID > 0` else error `"github app id must be positive, got %d"`.
- `cfg.InstallationID > 0` else error `"github app installation id must be positive, got %d"`.
- Exactly one of `cfg.PEM` and `cfg.PEMPath` is set; else error `"exactly one of PEM or PEMPath must be set"`.

`NewClient` and `MintIAT` MUST both call this validator before any I/O.

---

## Step 5 — Ginkgo unit tests with httptest

Create `lib/githubapp/githubapp_test.go` with Ginkgo specs. Cover at minimum:

1. **Happy path — `NewClient`**: spin up a `httptest.Server` whose `/app/installations/{id}/access_tokens` returns `{"token":"ghs_test","expires_at":"2099-01-01T00:00:00Z"}` and whose `/repos/...` returns 200. Set `ghinstallation/v2`'s API base URL to the test server (the library exposes `BaseURL` on the transport — set it after construction). Make a real HTTP call via the returned client and assert the request carried `Authorization: token ghs_test` (or `Bearer ghs_test`, either is acceptable per GitHub).
2. **Happy path — `MintIAT`**: same fake server; assert the returned string equals `ghs_test`.
3. **Config validation — both PEM and PEMPath set**: both `NewClient` and `MintIAT` return a non-nil error containing the literal `"exactly one"`.
4. **Config validation — neither PEM nor PEMPath set**: both return a non-nil error containing the literal `"exactly one"`.
5. **Config validation — AppID = 0**: both return a non-nil error containing the literal `"app id"`.
6. **Config validation — InstallationID = 0**: both return a non-nil error containing the literal `"installation id"`.
7. **PEMPath unreadable**: pass a nonexistent path; both return a non-nil wrapped error mentioning the path.
8. **PEM malformed**: pass random non-PEM bytes; both return a non-nil error from the underlying library, surfaced through `errors.Wrap`.

Use a valid test RSA private key generated in-suite (`rsa.GenerateKey(rand.Reader, 2048)` then `x509.MarshalPKCS1PrivateKey` + `pem.Encode`) — do NOT check in a PEM file under `lib/githubapp/testdata/`. Keep the suite self-contained.

For the tests that exercise the httptest server, set `transport.BaseURL` to the `httptest.Server`'s URL right after `ghinstallation.New(...)` returns — `BaseURL` is a public `string` field on `*ghinstallation.Transport` in v2 (as of late 2025). If the API has shifted in a newer release, fall back to godoc; do not invent a new accessor.

For the `Authorization` header assertion in test #1, use a regex to avoid substring-noise false positives:

```go
Expect(req.Header.Get("Authorization")).To(MatchRegexp(`^(token|Bearer) ghs_test$`))
```

**Coverage target: ≥ 80%.** After tests are passing, run `cd lib && go test -cover ./githubapp/...` and report the coverage percentage; if < 80%, add tests for whichever branches are missed.

---

## Step 6 — Run module-local precommit

```bash
cd lib && make precommit
```

Must exit 0. The relevant targets that will run: format, generate (no generated code in this package — should no-op), test (must pass with the new package), lint (no new warnings), errcheck, gosec, vulncheck, license-header check.

If gosec G304 fires on the `os.ReadFile(cfg.PEMPath)` call, use `filepath.Clean` on the path immediately before the read (mirror the `agent/pr-reviewer/cmd/mint-iat/main.go::resolvePEM` pattern — already accepted by repo lint config).

---

## Step 7 — Add CHANGELOG entry

Read `CHANGELOG.md` at the repo root. The current top entry is `## v0.25.7` or similar (check the actual file). If a `## Unreleased` section already exists, append to it; otherwise prepend a new `## Unreleased` section above the most recent released version.

Add one line under `## Unreleased`:

```markdown
- feat(lib): add `lib/githubapp` shared package — `NewClient` + `MintIAT` for GitHub App installation access token minting via `ghinstallation/v2`; consumed by spec 033 pr-reviewer auth migration
```
</requirements>

<constraints>
- IAT minting MUST go through `github.com/bradleyfalzon/ghinstallation/v2`. Do not reimplement JWT signing in production code. The stdlib `cmd/mint-iat` smoke-test tool stays where it is for credential verification drills; it does not move into `lib/githubapp`.
- The new module path stays `github.com/bborbe/maintainer/lib`, package `githubapp`. Mirrors the precedent of `lib/repoallowlist` (spec 028).
- All errors via `github.com/bborbe/errors`. No `fmt.Errorf`. No stdlib `errors.New`.
- All logging via `github.com/golang/glog`. No `log.*`, no `fmt.Print*`.
- BSD-style license header on every new `.go` file (verbatim three lines from `lib/repoallowlist/repoallowlist.go`).
- Factory functions are pure composition: no I/O at construction time, no `context.Background()` — accept the ambient `ctx` from the caller.
- No logging of PEM bytes anywhere. No logging of the full IAT body; at most an 8-char prefix.
- Coverage ≥ 80% via Ginkgo + httptest. No real GitHub network calls in the suite — that would couple tests to live App credentials and is unacceptable.
- Existing `lib/repoallowlist` tests must still pass; nothing in this prompt touches that package.
- Do NOT commit — dark-factory handles git.
- Do NOT add files to `lib/mocks/` unless this package introduces an internal interface that needs a Counterfeiter mock. If you do, follow `go-mocking-guide.md`.
</constraints>

<verification>
```bash
# 1. Package exists with expected files
ls lib/githubapp/*.go
# Expected: at least lib/githubapp/githubapp.go and lib/githubapp/githubapp_test.go and lib/githubapp/githubapp_suite_test.go

# 2. Exported symbols are present and use the agreed signatures
grep -n 'func NewClient\|func MintIAT\|type Config' lib/githubapp/githubapp.go
# Expected: at least three matches

# 3. ghinstallation dependency is in lib/go.mod (direct require)
grep -n 'ghinstallation/v2' lib/go.mod
# Expected: at least one match

# 4. No fmt.Errorf or stdlib errors.New
grep -E 'fmt\.Errorf|errors\.New' lib/githubapp/*.go
# Expected: empty output (any matches are violations)

# 5. No PEM/IAT bytes in logs (only prefix allowed)
grep -n 'glog\.' lib/githubapp/*.go | grep -i 'pem\|ghs_' | grep -v 'prefix\|first 8\|\[:8\]'
# Expected: empty output

# 6. License headers
grep -L 'BSD-style' lib/githubapp/*.go
# Expected: empty output (every .go file has the header)

# 7. Test coverage ≥ 80%
cd lib && go test -cover ./githubapp/...
# Expected: a line like `coverage: 8X.X% of statements` (X.X >= 0.0); exit code 0

# 8. Module-local precommit
cd lib && make precommit
# Expected: exit code 0; final line `ready to commit`

# 9. CHANGELOG updated
grep -A2 '## Unreleased' CHANGELOG.md | head -5
# Expected: a line mentioning lib/githubapp under Unreleased
```
</verification>
