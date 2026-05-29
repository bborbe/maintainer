---
status: completed
spec: [045-introduce-maintainer-yaml-release-gate]
summary: 'Added .maintainer.yaml fetch+parse surface to release watcher GitHubClient: MaintainerConfig/MaintainerReleaseConfig types, parseMaintainerConfig parser, GetMaintainerConfig interface method and concrete implementation, 10 Ginkgo test cases covering all acceptance criteria branches, counterfeiter mock regenerated'
container: maintainer-yaml-exec-195-spec-045-maintainer-yaml-fetch-and-parse
dark-factory-version: v0.173.0
created: "2026-05-29T12:00:00Z"
queued: "2026-05-29T09:15:04Z"
started: "2026-05-29T09:15:12Z"
completed: "2026-05-29T09:18:48Z"
---

<summary>
- The release watcher will fetch a new repo-level config file named `.maintainer.yaml` from each scanned repo's default branch.
- A nested config type is introduced with a `release.autoRelease` boolean — future maintainer bots will hang sibling top-level keys off the same document.
- The new GitHub-client method returns a zero-value config when the file is missing (the common case) and a wrapped error when the YAML is malformed — the watcher must NOT silently treat parse failures as "off".
- Unknown top-level keys (e.g., `pr-reviewer:`, `build-fix:`) are tolerated so future bots can land without a schema migration.
- Rate-limit, 5xx, 404, oversize, and decode-failure responses are mapped to the same shapes the existing `GetChangelogContent` method already uses.
- A Ginkgo suite covers every YAML branch and every HTTP branch end-to-end via an in-process `httptest.Server`.
- No call site is rewired in this prompt — the new method is added alongside the old one; the swap happens in the follow-up prompt.
</summary>

<objective>
Add the `.maintainer.yaml` fetch+parse surface to the release watcher's GitHub client: a new `MaintainerConfig` type, a private parser, a new `GetMaintainerConfig` method on the `GitHubClient` interface and its concrete implementation, and a Ginkgo test suite that exercises every behavior listed in the spec's Acceptance Criteria. The old `GetAutoReleaseConfig` method, its parser, its tests, and the filter rewire stay UNTOUCHED in this prompt — they are removed in prompt 2 once every reader has been switched over.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read these files in full before changing anything (they pin the existing patterns the new code MUST mirror):

- `/Users/bborbe/Documents/workspaces/maintainer-yaml/specs/in-progress/045-introduce-maintainer-yaml-release-gate.md` — the spec this prompt implements.
- `/Users/bborbe/Documents/workspaces/maintainer-yaml/watcher/github-release/pkg/githubclient.go` — full file. The new method must mirror `GetChangelogContent`'s shape exactly: 404 → zero return + nil error, rate-limit → `ErrRateLimited`, oversize check (1 MiB), `GetContent()` decode, wrapped error otherwise.
- `/Users/bborbe/Documents/workspaces/maintainer-yaml/watcher/github-release/pkg/githubclient_test.go` — full file. New `Describe("GetMaintainerConfig")` block must follow the same `httptest.NewServer` + `SetBaseURL` + `Repositories.GetContents` URL-asserting style.
- `/Users/bborbe/Documents/workspaces/maintainer-yaml/watcher/github-release/pkg/githubclient_export_test.go` — explains `SetBaseURL` test hook.
- `/Users/bborbe/Documents/workspaces/maintainer-yaml/watcher/github-release/pkg/suite_test.go` — Ginkgo suite entry; do not modify.

Reference guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` context-form rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 conventions, external `_test` package, `httptest` patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter regeneration (relevant for prompt 2, not this one).
</context>

<requirements>

1. **Add the new config types to `watcher/github-release/pkg/githubclient.go`** (append after the existing `darkFactoryConfig` struct, do not touch it). Use these exact declarations:

   ```go
   // MaintainerConfig is the parsed shape of `.maintainer.yaml` at the root of a
   // target repo. It is the trust gate that opts the repo into maintainer-bot
   // behaviors. v1 has a single `release` namespace; future maintainer bots
   // (`pr-reviewer`, `build-fix`, `dep-pin`, …) land as sibling top-level keys.
   //
   // Unknown top-level keys are tolerated by design — `yaml.Unmarshal` into
   // this struct silently ignores fields it does not recognise, which is the
   // forward-compat behavior the spec mandates.
   type MaintainerConfig struct {
       Release MaintainerReleaseConfig `yaml:"release"`
   }

   // MaintainerReleaseConfig is the `release:` namespace of `.maintainer.yaml`.
   // AutoRelease=true is the ONLY shape that lets the github-release watcher
   // emit a release task; everything else (key absent, value false, file
   // absent) skips the repo.
   type MaintainerReleaseConfig struct {
       AutoRelease bool `yaml:"autoRelease"`
   }
   ```

2. **Add the private parser in the same file**, after the new types:

   ```go
   // parseMaintainerConfig unmarshals a .maintainer.yaml document and returns
   // the parsed config. Pure data extraction — no I/O. Empty input returns a
   // zero-value MaintainerConfig with nil error (matches the "file empty"
   // happy-path branch of GetMaintainerConfig).
   func parseMaintainerConfig(content []byte) (MaintainerConfig, error) {
       var cfg MaintainerConfig
       if err := yaml.Unmarshal(content, &cfg); err != nil {
           return MaintainerConfig{}, err //nolint:wrapcheck // caller wraps with repo context
       }
       return cfg, nil
   }
   ```

3. **Add `GetMaintainerConfig` to the `GitHubClient` interface** in `watcher/github-release/pkg/githubclient.go`. Append it INSIDE the interface declaration, AFTER `GetChangelogContent`, BEFORE the existing `GetAutoReleaseConfig`. Do not remove `GetAutoReleaseConfig` in this prompt. Use this signature and godoc verbatim:

   ```go
   // GetMaintainerConfig returns the parsed `.maintainer.yaml` document at
   // HEAD of repo's default branch. The file is the trust gate for maintainer
   // bots; a repo without it is treated as "not opted in" (zero-value config,
   // nil error). This is the common case — the file is rare.
   //
   // Returns:
   //   - (parsed config, nil) on a valid YAML document (including empty input
   //     and documents with the `release:` key absent — both yield zero-value).
   //   - (zero-value MaintainerConfig, nil) on HTTP 404 (file absent).
   //   - (zero-value MaintainerConfig, ErrRateLimited) on primary or abuse
   //     rate-limit responses.
   //   - (zero-value MaintainerConfig, wrapped error) on every other failure
   //     including network errors, 5xx responses, oversize files (>1 MiB),
   //     base64 decode failures, and YAML parse failures. Malformed YAML
   //     must NOT be silently treated as `autoRelease: false`.
   //
   // The 1 MiB cap is enforced via the API-reported Size before decoding
   // (cheap upstream rejection). A post-decode re-check is not added because
   // base64 encoding can only inflate, never deflate — a Size under 1 MiB
   // cannot decode to over 1 MiB.
   GetMaintainerConfig(ctx context.Context, repo Repo) (MaintainerConfig, error)
   ```

4. **Implement `(c *githubClient) GetMaintainerConfig`** in the same file, immediately AFTER the existing `GetAutoReleaseConfig` method. The body MUST mirror `GetChangelogContent`'s control flow — same 404 check, same rate-limit mapping, oversize check on `fileContent.GetSize()` (pre-decode ONLY — base64 encoding inflates by ~33%, never deflates, so an under-1-MiB raw size cannot decode to over 1 MiB; the post-decode re-check `GetChangelogContent` carries is YAGNI for this gate), `GetContent()` decode, then call `parseMaintainerConfig`. Use these exact wrap messages so the test can assert on substrings:

   - Top-level GetContents error wrap: `errors.Wrapf(ctx, err, "get .maintainer.yaml %s/%s@%s", repo.Owner, repo.Name, repo.DefaultBranch)`.
   - Pre-decode oversize: `errors.Errorf(ctx, ".maintainer.yaml %s/%s too large: %d bytes (max 1 MiB)", repo.Owner, repo.Name, fileContent.GetSize())`.
   - Decode failure: `errors.Wrapf(ctx, err, "decode .maintainer.yaml %s/%s", repo.Owner, repo.Name)`.
   - Parse failure: `errors.Wrapf(ctx, err, "parse .maintainer.yaml %s/%s", repo.Owner, repo.Name)`.

   Path passed to `client.Repositories.GetContents`: the literal string `".maintainer.yaml"` (no leading slash, no `.dark-factory/` prefix). Ref: `&gogithub.RepositoryContentGetOptions{Ref: repo.DefaultBranch}`.

   Zero-value returns on the happy nil-content branch (where `fileContent == nil`): `return MaintainerConfig{}, nil` — matches `GetChangelogContent`'s `return nil, nil` shape adapted for the new return type.

   All error returns of the form `return X, err` must use `return MaintainerConfig{}, err` — never a partially-populated config on error.

5. **Add a new `Describe("GetMaintainerConfig")` block** to `watcher/github-release/pkg/githubclient_test.go`. Append it INSIDE the existing `var _ = Describe("pkg.GitHubClient", ...)` block, AFTER the existing `Describe("GetAutoReleaseConfig")` block. Cover ALL of the following `It` blocks — each scenario is one named `It` so the `-v` test output enumerates the AC matrix:

   - `It("returns zero-value config and nil error on HTTP 404")` — `httptest.Server` asserts `r.URL.Path == "/repos/bborbe/x/contents/.maintainer.yaml"`, returns 404. Expect `err == nil`, returned config equal to `pkg.MaintainerConfig{}`.
   - `It("returns zero-value config and nil error when file is empty")` — server returns 200 with `"content":""` and `"size":0`. Expect `err == nil`, config is zero-value.
   - `It("returns zero-value Release.AutoRelease when release key is absent")` — YAML body `"pr-reviewer:\n  enabled: true\n"`. Expect `err == nil`, `cfg.Release.AutoRelease == false`.
   - `It("returns AutoRelease=false when release.autoRelease is explicitly false")` — YAML body `"release:\n  autoRelease: false\n"`. Expect `err == nil`, `cfg.Release.AutoRelease == false`.
   - `It("returns AutoRelease=true when release.autoRelease is true")` — YAML body `"release:\n  autoRelease: true\n"`. Expect `err == nil`, `cfg.Release.AutoRelease == true`.
   - `It("surfaces wrapped error on malformed YAML")` — YAML body `"{invalid"` (unbalanced brace, same fixture style the old test used). Expect `err != nil`, error message contains the substring `"parse .maintainer.yaml"`, and config is zero-value. Do NOT assert `BeFalse()` on the boolean — assert the returned config is the zero value `pkg.MaintainerConfig{}`. The point of this test is that parse failure is loud, not silent.
   - `It("ignores unknown top-level keys")` — YAML body `"pr-reviewer:\n  enabled: true\nbuild-fix:\n  channel: stable\nrelease:\n  autoRelease: true\n"`. Expect `err == nil`, `cfg.Release.AutoRelease == true`.
   - `It("returns ErrRateLimited on rate-limit response")` — server returns 403 + `X-RateLimit-Remaining: 0` + `X-RateLimit-Reset: 9999999999` + body `{"message":"API rate limit exceeded"}`. Assert `Expect(err).To(MatchError(pkg.ErrRateLimited))` and returned config equals `pkg.MaintainerConfig{}`.
   - `It("returns wrapped error on HTTP 500 response")` — server returns 500 + body `{"message":"server error"}`. Expect `err != nil`, error message contains `"get .maintainer.yaml"`, returned config is zero-value. Covers the spec failure-modes "GitHub 5xx on the fetch" row.
   - `It("returns wrapped error on oversize response")` — server returns JSON with `"size":2000000` and empty `"content"`. Expect `err != nil`, error message contains `"too large"`, returned config is zero-value. Exercises the pre-decode 1 MiB cap.

   Use `base64.StdEncoding.EncodeToString([]byte(yamlContent))` to compute the content field, matching the existing `GetAutoReleaseConfig` tests. The handler response shape is `{"name":"maintainer.yaml","path":".maintainer.yaml","size":<len>,"encoding":"base64","content":"<encoded>"}`.

6. **Do NOT modify any of these in this prompt** (they belong to prompt 2):
   - `watcher/github-release/pkg/filter/auto_release_filter.go`
   - `watcher/github-release/pkg/filter/auto_release_filter_test.go`
   - `watcher/github-release/pkg/filter/filter.go`
   - `watcher/github-release/pkg/filter/filter_test.go`
   - `watcher/github-release/pkg/watcher.go`
   - `watcher/github-release/pkg/watcher_test.go`
   - `watcher/github-release/pkg/release.go`
   - `watcher/github-release/pkg/factory/factory.go`
   - `watcher/github-release/pkg/metrics.go`
   - `watcher/github-release/README.md`
   - `docs/watcher-decision-chains.md`
   - The old `GetAutoReleaseConfig` method, its godoc, the `parseAutoReleaseConfig` helper, the `darkFactoryConfig` struct, or their tests in `githubclient_test.go`.

7. **Regenerate the counterfeiter mock for `GitHubClient`** so the new method becomes mockable. Run from `watcher/github-release/`:

   ```
   make generate
   ```

   This regenerates `watcher/github-release/pkg/mocks/github_client.go`. After regeneration, the mock file MUST contain BOTH `GetAutoReleaseConfigStub` (because the old interface method still exists in this prompt) AND `GetMaintainerConfigStub` (because the new method was just added). Do not hand-edit the mock; if `make generate` does not produce both, fix the interface declaration and rerun.

8. **Verify the new code compiles and the new tests pass** before declaring this prompt done. Run from `watcher/github-release/`:

   ```
   go test -run TestPkg -v ./pkg/... 2>&1 | grep -E "GetMaintainerConfig|GetAutoReleaseConfig|FAIL|PASS"
   go test -cover ./pkg/...
   make precommit
   ```

   Every `It` block listed in requirement 5 must appear in the `-v` output as a passing line. `make precommit` must exit 0. The `go test -cover ./pkg/...` output: the `./pkg/` package coverage line must report ≥ 80.0%. The existing `GetAutoReleaseConfig` tests must still pass because the old method is still wired — if they break, you have accidentally modified prompt-2 territory.

</requirements>

<constraints>
- All code lives under `watcher/github-release/`. No other watcher, no other agent, no shared lib package is modified.
- Error wrapping uses `github.com/bborbe/errors` context-form only: `errors.New(ctx, msg)`, `errors.Wrap(ctx, err, msg)`, `errors.Wrapf(ctx, err, fmt, args...)`, `errors.Errorf(ctx, fmt, args...)`. NEVER `fmt.Errorf` on production paths. The package is already imported in `githubclient.go` as `"github.com/bborbe/errors"`.
- Tests use Ginkgo v2 + Gomega in an external `_test` package (`package pkg_test`).
- Mocks are regenerated via the existing `make generate` target (counterfeiter v6). No new tooling.
- The fetch path mirrors the existing `GetChangelogContent` shape exactly: `Repositories.GetContents` with `Ref: repo.DefaultBranch`, 404 mapping to a zero-value happy-path return, rate-limit mapping to `ErrRateLimited`, 1 MiB cap enforced both on `fileContent.GetSize()` and on decoded content length.
- No `time.Now()` in business logic; if the implementation needs time (it does not for this prompt), use `github.com/bborbe/time` `libtime`.
- The github-releaser agent never reads `.maintainer.yaml`. This prompt only adds the watcher-side fetch surface.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run from `watcher/github-release/`:

```
make precommit
```

Must exit 0. Additionally:

```
grep -n "GetMaintainerConfig" pkg/githubclient.go
```

Expected: at least two matches — interface declaration line and concrete method declaration line.

```
grep -n "MaintainerConfig\|MaintainerReleaseConfig\|parseMaintainerConfig" pkg/githubclient.go
```

Expected: matches for the two type declarations, the parser declaration, and the interface + method signatures.

```
grep -n "GetMaintainerConfigStub" pkg/mocks/github_client.go
```

Expected: at least one match (mock regenerated).

```
grep -n "GetAutoReleaseConfig" pkg/githubclient.go pkg/mocks/github_client.go
```

Expected: still matches — the old method is NOT removed in this prompt. If the grep returns no matches, you have over-reached into prompt-2 work.

```
go test -run TestPkg -v ./pkg/... 2>&1 | grep "GetMaintainerConfig"
```

Expected: ten lines, one per `It` block listed in requirement 5.
</verification>
