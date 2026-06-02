---
status: completed
spec: [059-changelog-rewrite-opt-in-flag]
summary: Added `release.changelogRewrite` field to `lib/maintainerconfig.ReleaseConfig` with full Ginkgo coverage (default-false, true/false, missing-field, missing-block, empty-bytes, both-true, fail-closed on non-bool string/number); created `agent/github-releaser/pkg/maintainerconfig` package with a `Fetcher` interface that mirrors the `githubchangelog` shape, returns the sentinel `ErrFileNotFound` on HTTP 404 via `stderrors.New`, re-exports the lib types/Parse, and ships with the new counterfeiter mock `MaintainerConfigFetcher`; both `make precommit` invocations exit 0
container: maintainer-changelog-rewrite-exec-230-spec-059-config-field-and-yaml-fetcher
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T18:30:00Z"
queued: "2026-06-02T18:59:48Z"
started: "2026-06-02T18:59:50Z"
completed: "2026-06-02T19:11:55Z"
branch: dark-factory/changelog-rewrite-opt-in-flag
---

<summary>
- `.maintainer.yaml` schema gains a new boolean field `release.changelogRewrite` (default `false`), shared by all maintainer bots via the existing `lib/maintainerconfig` package
- Unknown / missing / `false` / `true` values parse cleanly; non-boolean values (string, number) fail loudly with a wrapped error that the planning step will surface as `error_category=invalid_config`
- A new `pkg/maintainerconfig/fetcher.go` package in `agent/github-releaser` provides a counterfeiter-mockable `Fetcher` interface that fetches `.maintainer.yaml` from a target repo ref via the GitHub contents API, mirroring the existing `githubchangelog.Fetcher` pattern
- HTTP 404 (file absent) returns the sentinel `ErrFileNotFound`; non-2xx responses return wrapped errors; the parser contract `Parse(ctx, []byte{}) → (zero, nil)` is locked so empty-yaml-from-fetch is equivalent to file-absent for default semantics
- A new counterfeiter mock (`mocks.MaintainerConfigFetcher`) is generated alongside the interface
- Adds Ginkgo coverage for: yaml-parsing the new field true/false/missing/missing-block/empty-bytes, invalid-value error wrapping, fetcher happy path, fetcher 404 (file absent) → `ErrFileNotFound`, fetcher non-2xx → wrapped error
</summary>

<objective>
Extend `lib/maintainerconfig` with the new `Release.ChangelogRewrite` boolean field, and add a typed `Fetcher` interface in `agent/github-releaser` for retrieving `.maintainer.yaml` bytes from a target GitHub repo at a ref (HTTP 404 → sentinel `ErrFileNotFound`; non-2xx → wrapped error). This is the data-acquisition slice of spec 059; the planning-step plumbing + fail-closed + task-page audit trail live in prompt 2.
</objective>

<context>
Read `~/Documents/workspaces/maintainer-changelog-rewrite/CLAUDE.md` and `agent/github-releaser/CLAUDE.md` for project conventions.

Read these files BEFORE editing:
- `/workspace/lib/maintainerconfig/maintainerconfig.go` — the existing schema with `Release` and `PrReviewer` namespaces. Add the new field to `ReleaseConfig`.
- `/workspace/lib/maintainerconfig/maintainerconfig_test.go` — existing Ginkgo style for the parser (DescribeTable for valid documents, plain `It` for the malformed case).
- `/workspace/agent/github-releaser/pkg/githubchangelog/fetcher.go` — the existing `Fetcher` interface + `httpFetcher` implementation. Mirror its shape: counterfeiter directive at the top, `NewHTTPFetcher(token)` constructor, `newHTTPFetcherWithBase(token, apiBase)` for tests, an `httpFetcher` struct, a `contentResponse` JSON shape, and a `Fetch` method that returns wrapped errors.
- `/workspace/agent/github-releaser/pkg/githubreview/client.go` — the canonical project sentinel pattern: `stderrors "errors"` alias + `var ErrTagNotFound = stderrors.New("githubreview: tag not found")`. Mirror this exact shape for `ErrFileNotFound`.
- `/workspace/agent/github-releaser/pkg/githubauth/githubauth.go` — second example of the same sentinel pattern (`ErrAppCredentialsRequired`) for cross-reference.
- `/workspace/agent/github-releaser/mocks/fetcher.go` — the existing counterfeiter mock (`Fetcher`, `FetchStub`, `FetchReturnsOnCall`, `FetchCallCount`). Use as a template for the new mock.
- `/workspace/agent/github-releaser/pkg/steps_planning.go` — the planning step. DO NOT EDIT in this prompt; prompt 2 will wire the new fetcher into the step. This file is in context only so you understand the consumer shape.
- `/workspace/agent/github-releaser/pkg/factory/factory.go` — the existing factory. DO NOT EDIT in this prompt; prompt 2 will add the new fetcher to `CreateAgent`. The new package MUST be importable without breaking the existing `factory.go` (it has no dependencies on the new package yet).

Read these coding plugin guides (in-container paths — the prompt runs inside the YOLO container):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

Verified symbols (from module source — grep-confirmed):
- `maintainerconfig.Parse(ctx, []byte) (MaintainerConfig, error)` and `maintainerconfig.MaintainerConfig{Release, PrReviewer}` from `github.com/bborbe/maintainer/lib/maintainerconfig` (already in `agent/github-releaser/go.mod` via the existing `replace github.com/bborbe/maintainer/lib => ../../lib`).
- `gopkg.in/yaml.v3` is a direct dep of `lib/maintainerconfig` — `yaml.Unmarshal` is the parsing primitive (typed-boolean unmarshalling is the spec's fail-closed boundary).
- `githubchangelog.Fetcher` interface: `Fetch(ctx context.Context, owner, repo, ref string) ([]byte, error)` — mirror this signature exactly so the planning step can keep one consistent calling convention.
- `github.com/bborbe/errors` wrap idioms: `errors.Wrapf(ctx, err, format, args...)`, `errors.Errorf(ctx, format, args...)` — every new error path must use these.
- Sentinel pattern in use across this repo: `stderrors "errors"` import alias + `var ErrXxx = stderrors.New("pkg: message")` + caller-side `errors.Is(err, ErrXxx)`. See `pkg/githubreview/client.go:11,27` and `pkg/githubauth/githubauth.go:14,25` for verbatim examples. The `github.com/bborbe/errors` package does NOT export a `Const` helper — use `stderrors.New` for sentinels.
- `github.com/maxbrunsfeld/counterfeiter/v6` annotation: `//counterfeiter:generate -o ../../mocks/<name>.go --fake-name <FakeName> . <InterfaceName>`.
</context>

<requirements>

1. **Add the `ChangelogRewrite` field to `ReleaseConfig`.** In `/workspace/lib/maintainerconfig/maintainerconfig.go`, extend the existing struct:

   ```go
   // ReleaseConfig is the `release:` namespace. AutoRelease=true is the ONLY
   // shape that lets the github-release watcher emit a release task; everything
   // else (key absent, value false, file absent) skips the repo.
   //
   // ChangelogRewrite is the spec-059 per-repo opt-in flag for the 058 LLM
   // rewrite pipeline. Default false (omit the field, set false explicitly,
   // or omit the `release:` block — all equivalent). When true, planning
   // invokes the 058 rewrite classification; when false (or absent), planning
   // short-circuits with `rewrite_needed=false` regardless of ## Unreleased
   // content — preserving the pre-058 header-rename-only behavior fleet-wide.
   // See spec 059 § Desired Behavior 1-3 and § Goal.
   type ReleaseConfig struct {
       AutoRelease      bool `yaml:"autoRelease"`
       ChangelogRewrite bool `yaml:"changelogRewrite"`
   }
   ```

   Do NOT touch `PrReviewerConfig` or any other existing field/tag. Do NOT change the `MaintainerConfig` struct. Do NOT change the package-level `Parse` function signature or its error-wrapping prefix `"unmarshal .maintainer.yaml"` (existing tests grep for that literal substring).

2. **Lock the `Parse([]byte{})` contract.** The package-level `Parse(ctx context.Context, data []byte) (MaintainerConfig, error)` MUST return `(MaintainerConfig{}, nil)` when called with an empty byte slice. This is the contract consumed by prompt 2's default mock semantics (the `Fetch` mock that returns `(nil, nil)` is equivalent to "file present, empty bytes" → zero-value config, no error). If the current implementation already satisfies this (yaml.v3's `Unmarshal(nil, &cfg)` is a no-op), no code change is needed — but add an explicit Ginkgo entry asserting it (see requirement 3 entry "empty bytes").

3. **Add Ginkgo coverage for the new field.** In `/workspace/lib/maintainerconfig/maintainerconfig_test.go`, add a new `DescribeTable` block (or extend the existing one) with at least these entries:

   - `"release.changelogRewrite: true" → ChangelogRewrite true` — input `release:\n  changelogRewrite: true\n`, expect `ReleaseConfig{AutoRelease: false, ChangelogRewrite: true}`.
   - `"release.changelogRewrite: false" → ChangelogRewrite false` — input `release:\n  changelogRewrite: false\n`, expect `ReleaseConfig{ChangelogRewrite: false}`.
   - `"release: present but no changelogRewrite field" → ChangelogRewrite false (default)` — input `release:\n  autoRelease: true\n`, expect `ReleaseConfig{AutoRelease: true, ChangelogRewrite: false}`.
   - `"no release: block" → ChangelogRewrite false` — input `prReviewer:\n  autoApprove: true\n`, expect `PrReviewerConfig{AutoApprove: true}` and `ReleaseConfig{ChangelogRewrite: false}`.
   - `"empty bytes" → zero-value config, nil error` — input `[]byte{}`, expect `MaintainerConfig{}` and `err == nil`. This entry locks the contract from requirement 2.
   - `"both autoRelease and changelogRewrite populated" → both true` — input `release:\n  autoRelease: true\n  changelogRewrite: true\n`, expect `ReleaseConfig{AutoRelease: true, ChangelogRewrite: true}`.

   Also add explicit `It` cases for the invalid-value fail-closed path. **This is the load-bearing boundary test for spec 059 AC #6/#7** (and per the prompt-style rules, the cheapest sub-test of an unmarshal-round-trip — the table entry IS the test):

   - `"release.changelogRewrite: \"yes\" (string) → wrapped error"` — input `release:\n  changelogRewrite: "yes"\n`, assert `err != nil`, assert `err.Error()` contains the literal substring `"unmarshal .maintainer.yaml"`, assert returned `cfg` is the zero value.
   - `"release.changelogRewrite: 1 (number) → wrapped error"` — input `release:\n  changelogRewrite: 1\n`, same assertions.

   `yaml.v3` will fail to unmarshal a non-bool into a `bool` field (it calls `cannot unmarshal X into Go bool`); the wrapped error from `Parse` will surface that fragment. Do NOT add custom validation logic in the lib — the type system is the validation. Document this in a code comment on `ReleaseConfig.ChangelogRewrite` so future maintainers see the contract: "non-boolean values fail at parse time; the planning step is responsible for surfacing the error as `error_category=invalid_config`."

4. **Run `go mod tidy` in `lib/`** if needed (the new field does not change imports, so this should be a no-op). Run `cd /workspace/lib && go build ./...` and `cd /workspace/lib && go test ./maintainerconfig/...` to confirm the new tests pass and the package still builds.

5. **New package `agent/github-releaser/pkg/maintainerconfig` — package boundary.**

   Create the directory `/workspace/agent/github-releaser/pkg/maintainerconfig/` with a new file `fetcher.go` that re-exports the `lib/maintainerconfig` types and adds a typed fetcher.

   **Why a new package instead of importing `lib/maintainerconfig` directly into `steps_planning.go`?** The fetcher is a network seam owned by this agent; the schema is owned by `lib`. Separating them keeps the package boundary clean: the planning step depends on `pkg/maintainerconfig` (which composes the lib type + the network fetcher), not directly on the lib.

   **Starting point — copy and adapt, do NOT retype from scratch.** Run:
   ```
   cp /workspace/agent/github-releaser/pkg/githubchangelog/fetcher.go \
      /workspace/agent/github-releaser/pkg/maintainerconfig/fetcher.go
   ```
   Then diff-and-adapt against the target shape below (package name, file-path token in error strings `.maintainer.yaml` vs `CHANGELOG.md`, new `ErrFileNotFound` sentinel + 404 handling, lib type re-exports + `Parse` alias, counterfeiter directive pointing to the new mock). The two files MUST stay side-by-side comparable so a reviewer can verify the deliberate duplication.

   Target shape:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package maintainerconfig fetches and exposes the .maintainer.yaml
   // bytes for the github-releaser agent's planning step. The schema
   // itself lives in github.com/bborbe/maintainer/lib/maintainerconfig
   // (shared with the github-release watcher and pr-reviewer agent);
   // this package adds ONLY the network seam.
   package maintainerconfig

   import (
       "context"
       "encoding/base64"
       "encoding/json"
       stderrors "errors"
       "fmt"
       "io"
       "net/http"
       "net/url"
       "strings"
       "time"

       "github.com/bborbe/errors"
       "github.com/golang/glog"
       libmaintainerconfig "github.com/bborbe/maintainer/lib/maintainerconfig"
   )

   // Re-export the lib types so callers of this package need only one
   // import for both the schema and the fetcher.
   type (
       Config = libmaintainerconfig.MaintainerConfig
       ReleaseConfig = libmaintainerconfig.ReleaseConfig
       PrReviewerConfig = libmaintainerconfig.PrReviewerConfig
   )

   // Parse is a thin alias to lib/maintainerconfig.Parse so callers
   // don't need to import the lib directly.
   var Parse = libmaintainerconfig.Parse

   //counterfeiter:generate -o ../../mocks/maintainer_config_fetcher.go --fake-name MaintainerConfigFetcher . Fetcher

   // Fetcher reads .maintainer.yaml bytes from a remote GitHub repo at a ref.
   // Implementations MUST be safe for concurrent use. Returned bytes are the
   // raw decoded file contents (no base64, no JSON wrapper).
   //
   // HTTP 404 (file absent at the ref's tip) returns the sentinel ErrFileNotFound
   // so callers can treat the absent-file case as a clean default-valued config
   // (see spec 059 § Desired Behavior 6: missing .maintainer.yaml is treated as
   // `changelogRewrite: false`).
   type Fetcher interface {
       Fetch(ctx context.Context, owner, repo, ref string) ([]byte, error)
   }

   // ErrFileNotFound is returned by Fetch on HTTP 404. Callers use
   // errors.Is(err, ErrFileNotFound) to treat the absent-file case as
   // the default-valued config (same code path as "file absent"). Other
   // errors are NOT covered by this sentinel and must NOT be silently
   // downgraded to a default config.
   //
   // Sentinel pattern mirrors pkg/githubreview.ErrTagNotFound and
   // pkg/githubauth.ErrAppCredentialsRequired (project convention).
   var ErrFileNotFound = stderrors.New("maintainerconfig: .maintainer.yaml not found at ref")

   // NewHTTPFetcher constructs a Fetcher backed by net/http against
   // api.github.com. token is the bearer token (GitHub App IAT or PAT);
   // empty token sends no Authorization header. The internal http.Client
   // has a 15-second timeout — operations that exceed this return a wrapped
   // context-deadline-exceeded error.
   func NewHTTPFetcher(token string) Fetcher {
       return &httpFetcher{
           client:  &http.Client{Timeout: 15 * time.Second},
           token:   token,
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

   // Fetch implements Fetcher. Returns:
   //   - ErrFileNotFound on HTTP 404 (file absent at the ref's tip)
   //   - wrapped errors on:
   //       empty owner/repo/ref (caller bug; "fetch .maintainer.yaml: owner empty" etc.)
   //       request construction failure
   //       HTTP transport failure (timeout, DNS, connection reset)
   //       non-2xx non-404 response ("fetch .maintainer.yaml: status %d: %s")
   //       JSON decode failure
   //       unsupported encoding ("fetch .maintainer.yaml: unsupported encoding %q")
   //       base64 decode failure
   func (f *httpFetcher) Fetch(ctx context.Context, owner, repo, ref string) ([]byte, error) {
       if owner == "" {
           return nil, errors.Errorf(ctx, "fetch .maintainer.yaml: owner empty")
       }
       if repo == "" {
           return nil, errors.Errorf(ctx, "fetch .maintainer.yaml: repo empty")
       }
       if ref == "" {
           return nil, errors.Errorf(ctx, "fetch .maintainer.yaml: ref empty")
       }

       endpoint := fmt.Sprintf(
           "%s/repos/%s/%s/contents/.maintainer.yaml?ref=%s",
           f.apiBase, url.PathEscape(owner), url.PathEscape(repo), url.QueryEscape(ref),
       )
       req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch .maintainer.yaml: build request")
       }
       req.Header.Set("Accept", "application/vnd.github+json")
       req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
       if f.token != "" {
           req.Header.Set("Authorization", "Bearer "+f.token)
       }

       resp, err := f.client.Do(req)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch .maintainer.yaml: http %s/%s@%s", owner, repo, ref)
       }
       defer func() { _ = resp.Body.Close() }()

       body, err := io.ReadAll(resp.Body)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch .maintainer.yaml: read body")
       }

       if resp.StatusCode == http.StatusNotFound {
           glog.V(2).Infof("fetch .maintainer.yaml: GET %s/%s@%s status=404 (absent)", owner, repo, ref)
           return nil, ErrFileNotFound
       }
       if resp.StatusCode < 200 || resp.StatusCode >= 300 {
           preview := string(body)
           if len(preview) > 200 {
               preview = preview[:200]
           }
           return nil, errors.Errorf(
               ctx,
               "fetch .maintainer.yaml: status %d: %s",
               resp.StatusCode,
               preview,
           )
       }

       glog.V(2).
           Infof("fetch .maintainer.yaml: GET %s/%s@%s status=%d bytes=%d", owner, repo, ref, resp.StatusCode, len(body))

       var cr contentResponse
       if err := json.Unmarshal(body, &cr); err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch .maintainer.yaml: decode json")
       }
       if cr.Encoding != "base64" {
           return nil, errors.Errorf(
               ctx,
               "fetch .maintainer.yaml: unsupported encoding %q (want base64)",
               cr.Encoding,
           )
       }
       cleaned := strings.NewReplacer("\n", "", "\r", "").Replace(cr.Content)
       decoded, err := base64.StdEncoding.DecodeString(cleaned)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch .maintainer.yaml: base64 decode")
       }
       return decoded, nil
   }
   ```

   **Sentinel pattern is fixed: `stderrors "errors"` + `stderrors.New(...)`.** Verified against `/workspace/agent/github-releaser/pkg/githubreview/client.go:11,27` and `/workspace/agent/github-releaser/pkg/githubauth/githubauth.go:14,25`. Do NOT use `errors.Const` (that helper does not exist in `github.com/bborbe/errors`). Callers test the sentinel via `errors.Is(err, ErrFileNotFound)`.

6. **Counterfeiter mock.** Run `cd /workspace/agent/github-releaser && go generate ./...` to regenerate the mocks. Confirm `/workspace/agent/github-releaser/mocks/maintainer_config_fetcher.go` was created with a `MaintainerConfigFetcher` struct exposing `FetchStub`, `FetchArgsForCall`, `FetchReturns`, `FetchReturnsOnCall`, `FetchCallCount` — mirror the existing `mocks.Fetcher` shape.

   If `go generate ./...` does not pick up the new `//counterfeiter:generate` directive (because it lives in a brand-new file the build cache hasn't seen), run `go run github.com/maxbrunsfeld/counterfeiter/v6 -o /workspace/agent/github-releaser/mocks/maintainer_config_fetcher.go --fake-name MaintainerConfigFetcher . Fetcher` from `/workspace/agent/github-releaser/pkg/maintainerconfig/`. The Makefile's `generate` target should also work; if so, prefer that path.

7. **Ginkgo coverage for the fetcher.** Create `/workspace/agent/github-releaser/pkg/maintainerconfig/fetcher_test.go` and `/workspace/agent/github-releaser/pkg/maintainerconfig/suite_test.go` mirroring the `githubchangelog` package shape (external `_test` package, `BeforeSuite` + `RunSpecs` in the suite file, `Describe("httpFetcher", ...)` in the test file). At minimum cover these cases — they are the load-bearing boundaries per spec 059 § Acceptance Criteria #5 (no yaml, treated as default) and #6/#7 (invalid value, fail-closed):

   a. `It("happy path: 200 OK with valid base64 YAML returns decoded bytes")` — spin up an `httptest.NewServer` that returns a `contentResponse{Encoding: "base64", Content: <base64("release:\n  changelogRewrite: true\n")>}`. Build the fetcher via `newHTTPFetcherWithBase("", server.URL)`. Assert returned bytes == the raw YAML bytes; assert `err == nil`.

   b. `It("404: file absent returns ErrFileNotFound")` — server returns 404. Assert `errors.Is(err, ErrFileNotFound)` is true. Assert returned bytes is nil.

   c. `It("500: server error returns wrapped non-2xx error")` — server returns 500 with body `boom`. Assert `err != nil`, assert `errors.Is(err, ErrFileNotFound)` is false, assert `err.Error()` contains `"status 500"`, assert returned bytes is nil.

   d. `It("empty owner / repo / ref returns descriptive wrapped error")` — three small `It` cases, one per arg, asserting the message matches the documented strings (`"owner empty"`, etc.).

   e. `It("fetch bytes parse cleanly via the lib parser (round-trip test)")` — server returns `release:\n  changelogRewrite: true\n` base64-encoded. Call `f.Fetch(ctx, "bborbe", "maintainer", "master")`, then call `Parse(ctx, bytes)`, assert `cfg.Release.ChangelogRewrite == true`. This is the integration seam between the fetcher and the lib parser — a unit test on either layer in isolation would not catch a regression where the wire format diverges from the expected shape.

8. **Acceptance gate — `make precommit` exits 0 in `lib/`.** Run `cd /workspace/lib && make precommit` and confirm exit code 0. Investigate and fix any failures. `make precommit` runs `go generate ./...`; no new counterfeiter directives live in `lib/`, so this should be a no-op for mock generation (only the parser tests should run).

9. **Acceptance gate — `make test` exits 0 in `agent/github-releaser/`.** Run `cd /workspace/agent/github-releaser && make test` and confirm exit code 0. The full `make precommit` (lint + gosec + trivy) is run once at the end of prompt 2 — prompt 1's scope is data acquisition only and the faster `make test` loop catches the parser + fetcher regressions first. If `make test` fails, fix and re-run; do not skip.
</requirements>

<constraints>
- The new `ChangelogRewrite` field MUST default to `false` on any of: empty bytes, missing `release:` block, missing `changelogRewrite` field, explicit `false`. This is the load-bearing rollout-safety invariant per spec 059 § Constraints — verify by reading the Ginkgo test entries.
- `Parse(ctx, []byte{})` MUST return `(MaintainerConfig{}, nil)`. Prompt 2's default mock semantics depend on this contract.
- The fetcher MUST treat HTTP 404 as `ErrFileNotFound` (sentinel error declared via `stderrors.New`), NOT as a generic non-2xx error. This is the only "absent file" path; any other 4xx/5xx must surface as a wrapped error. The planning step (prompt 2) maps `ErrFileNotFound` to a zero-value config via `errors.Is`.
- The fetcher MUST NOT have any business logic for parsing the YAML — it returns raw decoded bytes. Parsing is the lib's job. This keeps the package boundary clean and lets the lib stay a pure parser.
- The fetcher MUST mirror the existing `githubchangelog.Fetcher` shape exactly: same constructor naming (`NewHTTPFetcher(token)`), same `newHTTPFetcherWithBase(token, apiBase)` test helper, same `httpFetcher` struct, same 15-second client timeout, same error message format. This is so a reviewer can compare the two files side-by-side and see the deliberate duplication.
- The lib package is consumed by ALL maintainer bots (release watcher, pr-reviewer, …). Do NOT add agent-specific fields outside the existing `ReleaseConfig` / `PrReviewerConfig` namespaces.
- The new `pkg/maintainerconfig` package in `agent/github-releaser` re-exports the lib types as type aliases (not as new named types). The planning step will import this package and use `Config`, `ReleaseConfig`, `Parse` directly — not the lib.
- Do NOT add a per-PR override, env-var override, or runtime reload — spec 059 § Non-goals explicitly forbids all three. The flag is read from `.maintainer.yaml` only.
- Do NOT add Prometheus metrics, debug logging, or other observability beyond the existing `glog.V(2).Infof` pattern. Spec 059 does not call for new metrics.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
```
cd /workspace/lib && make precommit
```
Expected: exit code 0; new `ChangelogRewrite` Ginkgo entries pass; existing entries still pass.

```
cd /workspace/agent/github-releaser && make test
```
Expected: exit code 0; new fetcher Ginkgo entries in `pkg/maintainerconfig/fetcher_test.go` pass; existing tests in `pkg/githubchangelog/` and `pkg/steps_planning_test.go` still pass.

Evidence commands the auditor will run:
- `grep -n "ChangelogRewrite" /workspace/lib/maintainerconfig/maintainerconfig.go` → exactly ONE struct field declaration, with the documented yaml tag.
- `grep -n "ChangelogRewrite" /workspace/lib/maintainerconfig/maintainerconfig_test.go` → at least one `Entry(...)` line per (true / false / missing-field / missing-block / empty-bytes / both-true) case.
- `ls /workspace/agent/github-releaser/mocks/maintainer_config_fetcher.go` → file exists.
- `grep -n "counterfeiter:generate" /workspace/agent/github-releaser/pkg/maintainerconfig/fetcher.go` → exactly ONE directive pointing to the new mock.
- `grep -n "ErrFileNotFound" /workspace/agent/github-releaser/pkg/maintainerconfig/fetcher.go` → sentinel declared via `stderrors.New`; used on the 404 path.
- `grep -n "stderrors" /workspace/agent/github-releaser/pkg/maintainerconfig/fetcher.go` → import alias present (project convention; matches `pkg/githubreview/client.go`).
- `grep -rn "release.changelogRewrite" /workspace/specs/in-progress/059-changelog-rewrite-opt-in-flag.md` → the spec's source of truth; cross-referenced.
</verification>
</output>
