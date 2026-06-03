---
status: completed
spec: [049-github-releaser-planning-phase-integration]
summary: Created pkg/githubchangelog with Fetcher interface + httpFetcher implementation (87.2% coverage) and pkg/plan_output.go with typed PlanOutput struct and Outcome/PreconditionFailed constants
container: maintainer-github-releaser-exec-191-spec-047-githubchangelog-fetcher-and-plan-output
dark-factory-version: v0.173.0
created: "2026-05-28T00:00:00Z"
queued: "2026-05-28T05:18:37Z"
started: "2026-05-28T05:18:39Z"
completed: "2026-05-28T05:22:58Z"
---

<summary>
- Adds a CHANGELOG fetcher: a `Fetcher` interface plus a concrete implementation backed by GitHub's REST contents API.
- The fetcher is the only network boundary the planning step crosses to read a target repo's CHANGELOG bytes — kept narrow and mockable so the step can be unit-tested without HTTP.
- Adds a counterfeiter mock for `Fetcher` so the planning step tests can drive happy/escalation paths with no network.
- Adds the typed contract struct for the `## Plan` JSON section that the downstream execution phase will consume.
- All errors wrapped via `github.com/bborbe/errors`; HTTP timeouts and non-2xx responses produce wrapped errors.
- Pure plumbing — no Claude, no markdown mutation, no agentlib dependency in the fetcher package.
</summary>

<objective>
Create two new artifacts under `agent/github-releaser/`:

1. `pkg/githubchangelog/` — a sub-package exporting a `Fetcher` interface (single `Fetch(ctx, owner, repo, ref) ([]byte, error)` method), a concrete `httpFetcher` implementation that calls GitHub's REST contents API at `GET /repos/{owner}/{repo}/contents/CHANGELOG.md?ref={ref}` with base64-decoded body, and a counterfeiter mock generated into `pkg/githubchangelog/mocks/`.
2. `pkg/plan_output.go` — the typed `PlanOutput` struct used by the planning step to marshal the `## Plan` section, plus exported constants for `Outcome` and `PreconditionFailed` values.

End state: `cd agent/github-releaser && make precommit` exits 0; `pkg/githubchangelog/` has ≥ 80% coverage; `pkg/plan_output.go` has zero behavior to cover (struct only) but compiles cleanly.
</objective>

<context>
Read before writing code (all paths repo-relative; container mounts repo root at `/workspace`):

- `CLAUDE.md` at repo root — project conventions.
- `agent/github-releaser/go.mod` — module path `github.com/bborbe/maintainer/agent/github-releaser`; `github.com/bborbe/errors` v1.5.13 already present.
- `agent/github-releaser/pkg/semver/semver.go` lines 1-50 — sibling leaf showing license header + `context.Background()` + `bborbe/errors` style.
- `agent/github-releaser/pkg/changelog/changelog.go` lines 1-30 — sibling leaf showing package doc-comment style.
- `agent/pr-reviewer/pkg/poster_types.go` — canonical counterfeiter directive form (`//counterfeiter:generate -o ../mocks/...`); note pr-reviewer's mocks live in `agent/pr-reviewer/mocks/`. This spec uses a different layout (`pkg/githubchangelog/mocks/`) because the fetcher lives in a sub-package, not the flat `pkg/`.
- `agent/pr-reviewer/pkg/pkg_suite_test.go` lines 1-10 — the `//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate` directive that drives mock regeneration; replicate the same line in this package's suite file.
- `agent/github-releaser/mocks/mocks.go` — empty placeholder file; do NOT use this directory for new mocks (different layout per this spec — mocks go inside the sub-package's `mocks/` subdir).
- `specs/in-progress/047-github-releaser-planning-phase-integration.md` — re-read the Desired Behavior, Failure Modes, and Acceptance Criteria for context. This prompt covers behaviors 1, 5 (struct only), and the fetch portion of behavior 4.

Coding-plugin guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` `Wrapf`/`Errorf` usage; `fmt.Errorf` is banned in production code in this repo.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter directive patterns + `//go:generate` placement.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega; external test packages.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-library-guide.md` — pure-Go library hygiene.

GitHub REST contents API contract (read-only, well-documented): `GET /repos/{owner}/{repo}/contents/CHANGELOG.md?ref={ref}` returns JSON like:

```json
{"name":"CHANGELOG.md","path":"CHANGELOG.md","sha":"abc","size":123,"encoding":"base64","content":"IyMgVW5y..."}
```

The `content` field is base64-encoded with newlines that must be stripped before decoding.
</context>

<requirements>

**Run order: do steps in sequence. Run `cd agent/github-releaser && go test ./pkg/githubchangelog/...` after step 6. Run `cd agent/github-releaser && make precommit` only as the final verification step.**

1. **Create the sub-package directory** `agent/github-releaser/pkg/githubchangelog/`. It must contain exactly these files at the end:
   - `fetcher.go` — interface + http implementation
   - `fetcher_test.go` — Ginkgo tests for the http implementation
   - `suite_test.go` — Ginkgo suite bootstrap with the `//go:generate` line for counterfeiter
   - `mocks/fetcher.go` — counterfeiter-generated mock (created by `go generate` in step 6)

   Plus the typed struct at `agent/github-releaser/pkg/plan_output.go`.

2. **Standard 3-line BSD copyright header** on every `.go` file. Copy verbatim from `agent/github-releaser/pkg/semver/semver.go` top three lines:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.
   ```

3. **Write `agent/github-releaser/pkg/githubchangelog/fetcher.go`** with this exact structure:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package githubchangelog fetches CHANGELOG.md byte content from a target
   // GitHub repository at a specific ref via the REST contents API. It is the
   // only network boundary the planning step crosses; kept narrow and
   // mockable.
   //
   // Auth model: a bearer token is supplied at construction time (GitHub App
   // installation access token or PAT). Empty token means anonymous (60/hr
   // rate limit — sufficient for tests, never for production).
   package githubchangelog

   import (
       "context"
       "encoding/base64"
       "encoding/json"
       "fmt"
       "io"
       "net/http"
       "strings"
       "time"

       "github.com/bborbe/errors"
   )

   //counterfeiter:generate -o mocks/fetcher.go --fake-name Fetcher . Fetcher

   // Fetcher reads CHANGELOG.md bytes from a remote GitHub repo at a ref.
   // Implementations MUST be safe for concurrent use. Returned bytes are the
   // raw decoded file contents (no base64, no JSON wrapper).
   type Fetcher interface {
       Fetch(ctx context.Context, owner, repo, ref string) ([]byte, error)
   }

   // NewHTTPFetcher constructs a Fetcher backed by net/http against
   // api.github.com. token is the bearer token (GitHub App IAT or PAT);
   // empty token sends no Authorization header. The internal http.Client
   // has a 15-second timeout — operations that exceed this return a wrapped
   // context-deadline-exceeded error.
   func NewHTTPFetcher(token string) Fetcher {
       return &httpFetcher{
           client: &http.Client{Timeout: 15 * time.Second},
           token:  token,
           apiBase: "https://api.github.com",
       }
   }

   // newHTTPFetcherWithBase is an internal constructor used by tests to
   // point the fetcher at a test server. Not exported.
   func newHTTPFetcherWithBase(token, apiBase string) Fetcher {
       return &httpFetcher{
           client:  &http.Client{Timeout: 15 * time.Second},
           token:   token,
           apiBase: apiBase,
       }
   }

   type httpFetcher struct {
       client  *http.Client
       token   string
       apiBase string
   }

   // contentResponse is the slim JSON shape we read from /repos/.../contents/.
   // Extra fields returned by GitHub are ignored.
   type contentResponse struct {
       Encoding string `json:"encoding"`
       Content  string `json:"content"`
   }

   // Fetch implements Fetcher. Returns wrapped errors on:
   //   - empty owner/repo/ref (caller bug; returns "fetch CHANGELOG.md: owner empty" etc.)
   //   - request construction failure
   //   - HTTP transport failure (timeout, DNS, connection reset)
   //   - non-2xx response (returns "fetch CHANGELOG.md: status %d: %s")
   //   - JSON decode failure
   //   - unsupported encoding (returns "fetch CHANGELOG.md: unsupported encoding %q")
   //   - base64 decode failure
   func (f *httpFetcher) Fetch(ctx context.Context, owner, repo, ref string) ([]byte, error) {
       if owner == "" {
           return nil, errors.Errorf(ctx, "fetch CHANGELOG.md: owner empty")
       }
       if repo == "" {
           return nil, errors.Errorf(ctx, "fetch CHANGELOG.md: repo empty")
       }
       if ref == "" {
           return nil, errors.Errorf(ctx, "fetch CHANGELOG.md: ref empty")
       }

       url := fmt.Sprintf(
           "%s/repos/%s/%s/contents/CHANGELOG.md?ref=%s",
           f.apiBase, owner, repo, ref,
       )
       req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch CHANGELOG.md: build request")
       }
       req.Header.Set("Accept", "application/vnd.github+json")
       req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
       if f.token != "" {
           req.Header.Set("Authorization", "Bearer "+f.token)
       }

       resp, err := f.client.Do(req)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch CHANGELOG.md: http %s/%s@%s", owner, repo, ref)
       }
       defer func() { _ = resp.Body.Close() }()

       body, err := io.ReadAll(resp.Body)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch CHANGELOG.md: read body")
       }

       if resp.StatusCode < 200 || resp.StatusCode >= 300 {
           // Truncate body for log safety.
           preview := string(body)
           if len(preview) > 200 {
               preview = preview[:200]
           }
           return nil, errors.Errorf(
               ctx,
               "fetch CHANGELOG.md: status %d: %s",
               resp.StatusCode,
               preview,
           )
       }

       var cr contentResponse
       if err := json.Unmarshal(body, &cr); err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch CHANGELOG.md: decode json")
       }
       if cr.Encoding != "base64" {
           return nil, errors.Errorf(
               ctx,
               "fetch CHANGELOG.md: unsupported encoding %q (want base64)",
               cr.Encoding,
           )
       }
       // GitHub embeds literal newlines inside the base64 string; strip them.
       cleaned := strings.NewReplacer("\n", "", "\r", "").Replace(cr.Content)
       decoded, err := base64.StdEncoding.DecodeString(cleaned)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch CHANGELOG.md: base64 decode")
       }
       return decoded, nil
   }
   ```

   Notes:
   - Every error path is wrapped with `errors.Errorf` (fresh) or `errors.Wrapf` (chained) from `github.com/bborbe/errors`. NO `fmt.Errorf`.
   - All error messages contain the literal substring `fetch CHANGELOG.md` so callers can grep fetch failures.
   - `apiBase` is configurable internally for tests (via `newHTTPFetcherWithBase`) but the exported `NewHTTPFetcher` hardcodes `https://api.github.com`.
   - `defer func() { _ = resp.Body.Close() }()` — the lint guide rejects bare `defer resp.Body.Close()` in this repo.

4. **Write `agent/github-releaser/pkg/githubchangelog/suite_test.go`** mirroring `agent/pr-reviewer/pkg/pkg_suite_test.go`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package githubchangelog_test

   //go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate

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
       RunSpecs(t, "GithubChangelog Suite", suiteConfig, reporterConfig)
   }
   ```

5. **Write `agent/github-releaser/pkg/githubchangelog/fetcher_test.go`** using `httptest.NewServer` to exercise the real `httpFetcher` against a fake GitHub. The test file is in package `githubchangelog_test` (external).

   Required test cases (each `It` or `DescribeTable` entry distinct):

   (a) **Happy path**: server returns 200 with `{"encoding":"base64","content":"IyMgVW5yZWxlYXNlZAo="}` (base64 of `## Unreleased\n`). Test calls `Fetch(ctx, "bborbe", "maintainer", "master")` and asserts the returned bytes equal `[]byte("## Unreleased\n")` and no error.

   (b) **Authorization header forwarded**: server captures the request and asserts `Authorization` header is `Bearer test-token` when fetcher is built with token `"test-token"`.

   (c) **No auth header when token empty**: with token `""`, server asserts request has NO `Authorization` header.

   (d) **URL contains owner/repo/ref**: server asserts request `URL.Path` is `/repos/foo/bar/contents/CHANGELOG.md` and query `ref` is `mybranch`.

   (e) **404 returns wrapped error**: server returns 404 with body `{"message":"Not Found"}`. Fetch returns error whose message contains `fetch CHANGELOG.md` and `status 404`.

   (f) **5xx returns wrapped error**: server returns 503. Fetch returns error containing `status 503`.

   (g) **Malformed JSON returns wrapped error**: server returns 200 with body `not-json`. Error message contains `decode json`.

   (h) **Unsupported encoding rejected**: server returns 200 with `{"encoding":"utf-8","content":"hi"}`. Error message contains `unsupported encoding "utf-8"`.

   (i) **Bad base64 returns wrapped error**: server returns 200 with `{"encoding":"base64","content":"!!!not-base64!!!"}`. Error message contains `base64 decode`.

   (j) **Empty owner / repo / ref each rejected**: three separate cases call Fetch with one empty field and assert error message contains `owner empty`, `repo empty`, or `ref empty`.

   (k) **Newlines in base64 content stripped**: server returns content like `"IyMgVW5y\nZWxlYXNlZAo="` (newline inside the base64 string). Fetch must still succeed and return `[]byte("## Unreleased\n")`.

   To point the fetcher at the test server, define an exported test hook OR add an internal test helper. RECOMMENDED: add a separate file `agent/github-releaser/pkg/githubchangelog/export_test.go` that re-exports the private constructor:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package githubchangelog

   // NewHTTPFetcherForTest constructs a fetcher pointed at a custom API base
   // URL. Exported only via export_test.go so tests can substitute httptest
   // server URLs without exposing the seam in the production API.
   var NewHTTPFetcherForTest = newHTTPFetcherWithBase
   ```

   The test file then calls `githubchangelog.NewHTTPFetcherForTest(token, server.URL)` to build a fetcher pointing at the httptest server.

6. **Generate the counterfeiter mock**:

   ```bash
   cd agent/github-releaser && go generate ./pkg/githubchangelog/...
   ```

   This produces `agent/github-releaser/pkg/githubchangelog/mocks/fetcher.go` (file name follows the `-o mocks/fetcher.go` flag in the counterfeiter directive). Verify with `ls agent/github-releaser/pkg/githubchangelog/mocks/`.

   If `go generate` fails because counterfeiter is not installed yet, the directive uses `go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate` — pinned version, no install needed. The actual mock generation lives in the suite file's `//go:generate` line plus the `//counterfeiter:generate` directive in `fetcher.go`.

7. **Write `agent/github-releaser/pkg/plan_output.go`** — typed struct + constants used by the planning step. EXACT shape from spec 047 Desired Behavior 5:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   // PlanOutput is the typed contract for the `## Plan` JSON section the
   // planning step writes for every release task. Round-trips with
   // agentlib.MarshalSectionTyped + agentlib.ExtractSection[PlanOutput].
   //
   // Two shapes are valid:
   //   - Outcome="ready"        — planning succeeded; Bump/NextVersion populated
   //   - Outcome="needs_input"  — precondition failure; Reason + PreconditionFailed populated
   //
   // No `Details map[string]any`: concrete fields only. Future fields require
   // a spec amendment.
   type PlanOutput struct {
       Outcome            string   `json:"outcome"`
       Bump               string   `json:"bump,omitempty"`
       Reasoning          string   `json:"reasoning,omitempty"`
       CurrentVersion     string   `json:"current_version,omitempty"`
       NextVersion        string   `json:"next_version,omitempty"`
       NextVersionHeader  string   `json:"next_version_header,omitempty"`
       HeaderPrefixStyle  string   `json:"header_prefix_style,omitempty"`
       Bullets            []string `json:"bullets,omitempty"`
       Reason             string   `json:"reason,omitempty"`
       PreconditionFailed string   `json:"precondition_failed,omitempty"`
   }

   // Outcome values for PlanOutput.Outcome.
   const (
       PlanOutcomeReady       = "ready"
       PlanOutcomeNeedsInput  = "needs_input"
   )

   // PreconditionFailed values. Keep in sync with spec 047 Desired Behavior 5.
   const (
       PreconditionP1UnreleasedNotFirst = "P1_unreleased_not_first"
       PreconditionP2UnreleasedEmpty    = "P2_unreleased_empty"
       PreconditionBadCurrentVersion    = "bad_current_version"
       // PreconditionMissingFrontmatter is the PREFIX used for missing-field
       // precondition values; planning code appends the field name, e.g.
       // "missing_frontmatter_clone_url".
       PreconditionMissingFrontmatter = "missing_frontmatter_"
   )
   ```

   Notes:
   - Package is `pkg` (NOT `plan_output`) — this file lives at the flat `pkg/` root next to where `steps_planning.go` will live (prompt 2). This is the FIRST flat-`package pkg` file in `agent/github-releaser/pkg/`.
   - JSON tags are snake_case verbatim from the spec.
   - `Outcome` has NO `omitempty` — it is always written so consumers can branch.
   - `omitempty` on all other fields enables the two-shape contract from the spec (escalation drops happy-path fields; happy path drops Reason/PreconditionFailed).

7a. **Write `agent/github-releaser/pkg/plan_output_test.go`** — JSON round-trip tests covering the snake_case-tag contract that downstream specs depend on. The tags are the serialization boundary; a typo (`"nextversion"` vs `"next_version"`) compiles and unit-test-as-struct-equality passes but breaks `agentlib.ExtractSection[PlanOutput]` at runtime in prompt 2.

   Create `pkg/plan_output_test.go` (external test package `package pkg_test`) and `pkg/pkg_suite_test.go` for the Ginkgo bootstrap (mirror the suite_test pattern from `pkg/semver/suite_test.go`). Tests:

   ```go
   package pkg_test

   import (
       "encoding/json"
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
   )

   var _ = Describe("PlanOutput JSON contract", func() {
       It("round-trips happy-path (outcome=ready) with all fields", func() {
           in := pkg.PlanOutput{
               Outcome:           pkg.PlanOutcomeReady,
               Bump:              "minor",
               Reasoning:         "feat: stub",
               CurrentVersion:    "v1.7.7",
               NextVersion:       "1.8.0",
               NextVersionHeader: "## v1.8.0",
               HeaderPrefixStyle: "v",
               Bullets:           []string{"feat: stub"},
           }
           b, err := json.Marshal(in)
           Expect(err).NotTo(HaveOccurred())
           // Snake-case tags survive marshaling
           Expect(string(b)).To(ContainSubstring(`"outcome":"ready"`))
           Expect(string(b)).To(ContainSubstring(`"bump":"minor"`))
           Expect(string(b)).To(ContainSubstring(`"current_version":"v1.7.7"`))
           Expect(string(b)).To(ContainSubstring(`"next_version":"1.8.0"`))
           Expect(string(b)).To(ContainSubstring(`"next_version_header":"## v1.8.0"`))
           Expect(string(b)).To(ContainSubstring(`"header_prefix_style":"v"`))
           // Escalation fields omitted
           Expect(string(b)).NotTo(ContainSubstring(`"reason"`))
           Expect(string(b)).NotTo(ContainSubstring(`"precondition_failed"`))
           // Round-trip
           var out pkg.PlanOutput
           Expect(json.Unmarshal(b, &out)).To(Succeed())
           Expect(out).To(Equal(in))
       })

       It("omits happy-path fields on outcome=needs_input", func() {
           in := pkg.PlanOutput{
               Outcome:            pkg.PlanOutcomeNeedsInput,
               Reason:             "Unreleased is not the first ## section",
               PreconditionFailed: pkg.PreconditionP1UnreleasedNotFirst,
               CurrentVersion:     "v1.2.6",
           }
           b, err := json.Marshal(in)
           Expect(err).NotTo(HaveOccurred())
           Expect(string(b)).To(ContainSubstring(`"outcome":"needs_input"`))
           Expect(string(b)).To(ContainSubstring(`"reason":"Unreleased is not the first ## section"`))
           Expect(string(b)).To(ContainSubstring(`"precondition_failed":"P1_unreleased_not_first"`))
           Expect(string(b)).To(ContainSubstring(`"current_version":"v1.2.6"`))
           // Happy-path-only fields absent
           Expect(string(b)).NotTo(ContainSubstring(`"bump"`))
           Expect(string(b)).NotTo(ContainSubstring(`"reasoning"`))
           Expect(string(b)).NotTo(ContainSubstring(`"next_version"`))
           Expect(string(b)).NotTo(ContainSubstring(`"next_version_header"`))
           Expect(string(b)).NotTo(ContainSubstring(`"header_prefix_style"`))
           Expect(string(b)).NotTo(ContainSubstring(`"bullets"`))
       })
   })
   ```

   Add the verification grep at the end:
   ```bash
   grep -c '"next_version":"1.8.0"' agent/github-releaser/pkg/plan_output_test.go   # ≥1
   grep -c '"P1_unreleased_not_first"' agent/github-releaser/pkg/plan_output_test.go # ≥1
   ```

8. **Coverage ≥ 80% on `pkg/githubchangelog/`**: after writing tests, run:

   ```bash
   cd agent/github-releaser && go test -cover ./pkg/githubchangelog/...
   ```

   The 11 test cases above naturally exercise every error path. If coverage falls under 80%, add a case for the request-construction error path (e.g. invalid `apiBase`).

9. **Final verification**: from `agent/github-releaser/`:

   ```bash
   cd agent/github-releaser && make precommit
   ```

   Must exit 0. Existing tests for `pkg/changelog/`, `pkg/semver/`, `pkg/prompts/` must still pass — none of their files are touched.

</requirements>

<constraints>
- Module path: `github.com/bborbe/maintainer/agent/github-releaser`. NEW package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog`. NEW file path: `agent/github-releaser/pkg/plan_output.go` (package `pkg` — flat, not a sub-package).
- `Fetcher` interface signature FROZEN per spec 047 Desired Behavior 4:
  - `Fetch(ctx context.Context, owner, repo, ref string) ([]byte, error)`
- `PlanOutput` struct shape FROZEN per spec 047 Desired Behavior 5 — exact field names, JSON tags, and `omitempty` placement. NO `Details map[string]any`.
- Errors via `github.com/bborbe/errors` (`Wrapf`/`Errorf`). `fmt.Errorf` is BANNED in `pkg/githubchangelog/fetcher.go`. Acceptance grep: `grep -c 'fmt.Errorf' agent/github-releaser/pkg/githubchangelog/fetcher.go` returns 0.
- Every error returned by `Fetch` MUST contain the literal substring `fetch CHANGELOG.md`.
- HTTP client timeout: 15 seconds. Hardcoded in `NewHTTPFetcher`. Per spec 047 Failure Modes row "CHANGELOG fetch fails (HTTP 4xx/5xx, timeout)".
- Counterfeiter mock: generated into `pkg/githubchangelog/mocks/fetcher.go` via `//counterfeiter:generate -o mocks/fetcher.go --fake-name Fetcher . Fetcher` directive AND `//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate` in `suite_test.go`. Pin to `v6.12.2` (matches `agent/pr-reviewer/pkg/pkg_suite_test.go`).
- Test framework: Ginkgo v2 + Gomega; external test package `package githubchangelog_test`.
- Coverage target: ≥ 80% on `pkg/githubchangelog/`.
- Security per spec § Security: bearer token forwarded in `Authorization` header only. No token logging (the error wraps include `%s/%s@%s` but never the token). No shell-out — this implementation uses `net/http`, NOT `gh api`. (The spec offers either; we pick `net/http` because it's mockable via `httptest` and avoids depending on `gh` being in the agent container's PATH.)
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` is green before AND after (run as the precommit step).
- All `make precommit` invocations run from `agent/github-releaser/`, never from the repo root.
- License header (3 lines) required at the top of every `.go` file.
- No `strings.Index` in this prompt's code — not relevant here, but the rule applies project-wide.
- `pkg/githubchangelog/` is a SUB-package; do NOT put files in `agent/github-releaser/mocks/` (that's the flat-pkg mock dir from prompt 188's scaffold — empty placeholder only).
</constraints>

<verification>

Run from the repo root unless noted.

```bash
# Build + tests pass + coverage ≥ 80%
cd agent/github-releaser && make precommit                                # exit 0
cd agent/github-releaser && go test -cover ./pkg/githubchangelog/...      # ≥ 80%

# Files exist
ls agent/github-releaser/pkg/githubchangelog/fetcher.go                    # exists
ls agent/github-releaser/pkg/githubchangelog/fetcher_test.go               # exists
ls agent/github-releaser/pkg/githubchangelog/suite_test.go                 # exists
ls agent/github-releaser/pkg/githubchangelog/mocks/fetcher.go              # exists (counterfeiter output)
ls agent/github-releaser/pkg/plan_output.go                                # exists

# Frozen interface + struct
grep -c '^type Fetcher interface'                            agent/github-releaser/pkg/githubchangelog/fetcher.go   # =1
grep -c 'Fetch(ctx context.Context, owner, repo, ref string) (\[\]byte, error)'  agent/github-releaser/pkg/githubchangelog/fetcher.go   # =1
grep -c '^func NewHTTPFetcher('                              agent/github-releaser/pkg/githubchangelog/fetcher.go   # =1
grep -c '^type PlanOutput struct'                            agent/github-releaser/pkg/plan_output.go               # =1
grep -c 'PlanOutcomeReady'                                   agent/github-releaser/pkg/plan_output.go               # ≥1
grep -c 'PlanOutcomeNeedsInput'                              agent/github-releaser/pkg/plan_output.go               # ≥1
grep -c 'PreconditionP1UnreleasedNotFirst'                   agent/github-releaser/pkg/plan_output.go               # ≥1

# Error-wrapping convention (bborbe/errors only)
grep -c 'fmt.Errorf'                                         agent/github-releaser/pkg/githubchangelog/fetcher.go   # =0
grep -cE 'errors\.(Wrap|Errorf)'                             agent/github-releaser/pkg/githubchangelog/fetcher.go   # ≥1
grep -c 'fetch CHANGELOG.md'                                 agent/github-releaser/pkg/githubchangelog/fetcher.go   # ≥1

# Counterfeiter directive present
grep -c '//counterfeiter:generate'                           agent/github-releaser/pkg/githubchangelog/fetcher.go   # =1
grep -c 'counterfeiter/v6@v6.12.2'                           agent/github-releaser/pkg/githubchangelog/suite_test.go   # =1

# Make targets green
cd agent/github-releaser && make test
```

</verification>
