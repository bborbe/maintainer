---
status: approved
spec: [052-migrate-pr-reviewer-to-maintainer-yaml]
created: "2026-05-29T15:31:00Z"
queued: "2026-05-29T16:26:53Z"
---

<summary>
- Points the github-release watcher at the new shared `.maintainer.yaml` schema instead of its own private copy of the type.
- The watcher's behavior is completely unchanged — same GitHub-API fetch, same 404 / rate-limit / 1 MiB size-cap handling, same `release.autoRelease` gate. Only the Go type it parses into moves to the shared library.
- Deletes the watcher's three now-redundant local definitions (the config struct, the release sub-struct, and the private parser) so there is exactly one definition of this file's shape across the codebase.
- Regenerates the counterfeiter mock for the GitHub client so the fake's `GetMaintainerConfig` returns the shared type and all existing watcher tests compile.
- Updates the watcher's tests to reference the shared type; the assertions and scenarios they cover stay the same.
- No other watcher, agent, or the github-releaser is touched.
</summary>

<objective>
Migrate the github-release watcher's `GetMaintainerConfig` GitHub-client method to return the shared `github.com/bborbe/maintainer/lib/maintainerconfig.MaintainerConfig` type (created in prompt 1) instead of the watcher's locally-defined `MaintainerConfig`. Delete the local `MaintainerConfig`, `MaintainerReleaseConfig`, and `parseMaintainerConfig`; delegate parsing to `maintainerconfig.Parse`. Regenerate the counterfeiter mock and update tests so the module builds and behaves identically.

End state: `cd watcher/github-release && make precommit` exits 0; the watcher's 404/rate-limit/size-cap behavior is byte-for-byte unchanged; `grep -rn "parseMaintainerConfig\|MaintainerReleaseConfig" watcher/github-release/` exits 1.
</objective>

<context>
Read before writing code:

- `CLAUDE.md` at repo root — project conventions.
- `specs/in-progress/052-migrate-pr-reviewer-to-maintainer-yaml.md` — re-read Desired Behavior 4 & 6, Constraints, the Failure Modes table (the watcher-path size-cap row), and the Acceptance Criteria rows that grep `watcher/github-release/`.
- `lib/maintainerconfig/maintainerconfig.go` (created in prompt 1) — the shared type. The watcher's `MaintainerConfig.Release.AutoRelease` access path becomes `maintainerconfig.MaintainerConfig.Release.AutoRelease` — the field path is identical because the shared `Release ReleaseConfig` / `AutoRelease bool` shape mirrors the old local struct exactly. The shared parse function is `maintainerconfig.Parse(ctx, content) (MaintainerConfig, error)`.
- `watcher/github-release/pkg/githubclient.go` — the file to edit. Read in full. Key landmarks:
  - The `GitHubClient` interface (line 28) — its `GetMaintainerConfig` method (line 61) currently returns the local `MaintainerConfig`.
  - The local type definitions at lines 254-284: `MaintainerConfig` (262), `MaintainerReleaseConfig` (270), `parseMaintainerConfig` (278) — ALL THREE are deleted by this prompt.
  - The `GetMaintainerConfig` method body at lines 286-348 — the GitHub-API fetch, 404→zero-value, rate-limit→`ErrRateLimited`, >1 MiB→wrapped size error, decode, then `parseMaintainerConfig`. ONLY the final parse call changes (to `maintainerconfig.Parse`); every other branch stays.
  - The `//counterfeiter:generate -o mocks/github_client.go --fake-name GitHubClient . GitHubClient` directive (line 21).
- `watcher/github-release/pkg/doc.go` line 23 — the `//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate` line that `make generate` invokes.
- `watcher/github-release/pkg/watcher.go` line 185 + 204 — `maintainerCfg, err := w.ghClient.GetMaintainerConfig(ctx, repo)` then `AutoRelease: maintainerCfg.Release.AutoRelease`. This caller does NOT change (the field path is identical on the shared type); verify it still compiles.
- `watcher/github-release/pkg/watcher_test.go` lines 77-78, 212-213, 279-280, 325-326 — these construct `pkg.MaintainerConfig{Release: pkg.MaintainerReleaseConfig{AutoRelease: true}}`. They must change to `maintainerconfig.MaintainerConfig{Release: maintainerconfig.ReleaseConfig{AutoRelease: true}}`.
- `watcher/github-release/pkg/githubclient_test.go` lines 311-593 — the `GetMaintainerConfig` describe block. Every `pkg.MaintainerConfig{}` and `pkg.MaintainerConfig{Release: ...}` reference must change to the shared type. The HTTP-level test scenarios (404, rate-limit, oversize, malformed YAML, valid) are unchanged in INTENT — only the asserted type's import path changes.
- `watcher/github-release/go.mod` — already has `replace github.com/bborbe/maintainer/lib => ../../lib` and requires the lib module, so importing `lib/maintainerconfig` needs no go.mod change beyond what `go mod tidy` adds.

Coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` wrapping (the existing repo-context wrap stays).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter regen via `make generate`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega conventions.
</context>

<requirements>

**Run order: edit `githubclient.go` first, then run `make generate`, then fix the test files, then `make precommit` as final verification.**

1. **Edit `watcher/github-release/pkg/githubclient.go` — add the shared import.** In the import block (currently `context`, `stderrors "errors"`, `net/http`, `github.com/bborbe/errors`, `gogithub`, `gopkg.in/yaml.v3`):
   - Add `"github.com/bborbe/maintainer/lib/maintainerconfig"`.
   - REMOVE `"gopkg.in/yaml.v3"` — after deleting `parseMaintainerConfig`, this file no longer calls `yaml.Unmarshal`. (Confirm with `grep yaml watcher/github-release/pkg/githubclient.go` after the edit — must be empty. If any other code in this file still uses yaml, keep it; based on current source only `parseMaintainerConfig` uses it.)

2. **Edit the `GitHubClient` interface — change `GetMaintainerConfig`'s return type.** The method (line 61) currently reads:

   ```go
   GetMaintainerConfig(ctx context.Context, repo Repo) (MaintainerConfig, error)
   ```

   Change the return type to the shared type:

   ```go
   GetMaintainerConfig(ctx context.Context, repo Repo) (maintainerconfig.MaintainerConfig, error)
   ```

   Update the method's godoc comment block (lines 41-61) so every `MaintainerConfig` mention reads `maintainerconfig.MaintainerConfig` (the "zero-value MaintainerConfig" phrases in the Returns list). Keep the behavior description (404→zero-value, rate-limit→ErrRateLimited, oversize/malformed→wrapped error) verbatim — only the type name is qualified.

3. **Delete the three local definitions** at lines 254-284: the `MaintainerConfig` struct, the `MaintainerReleaseConfig` struct, and the `parseMaintainerConfig` function. Remove their godoc comment blocks too.

4. **Edit the `GetMaintainerConfig` method body** (lines 286-348):
   - Change the method signature return type to `(maintainerconfig.MaintainerConfig, error)`.
   - Replace every `return MaintainerConfig{}, …` with `return maintainerconfig.MaintainerConfig{}, …` (the 404 branch, the rate-limit branch, the generic-error branch, the nil-fileContent branch, the oversize branch, the decode-error branch).
   - Replace the final parse + return:

     ```go
     cfg, err := parseMaintainerConfig([]byte(decoded))
     if err != nil {
         return MaintainerConfig{}, errors.Wrapf(
             ctx,
             err,
             "parse .maintainer.yaml %s/%s",
             repo.Owner,
             repo.Name,
         )
     }
     return cfg, nil
     ```

     with:

     ```go
     cfg, err := maintainerconfig.Parse(ctx, []byte(decoded))
     if err != nil {
         return maintainerconfig.MaintainerConfig{}, errors.Wrapf(
             ctx,
             err,
             "parse .maintainer.yaml %s/%s",
             repo.Owner,
             repo.Name,
         )
     }
     return cfg, nil
     ```

   - Do NOT touch the 404 / rate-limit / nil / size-cap branches' logic — only the type name in their `return` statements. The 1 MiB cap (`fileContent.GetSize() > 1024*1024`) and its wrapped size error stay exactly as-is (spec § Failure Modes: watcher-path size-cap row preserved).

5. **Regenerate the counterfeiter mock.** From `watcher/github-release/`:

   ```bash
   make generate
   ```

   This re-runs `go generate ./...` against the `//counterfeiter:generate` directive and rewrites `watcher/github-release/pkg/mocks/github_client.go` so its `GetMaintainerConfigStub`, `getMaintainerConfigReturns`, `GetMaintainerConfigReturns`, `GetMaintainerConfigReturnsOnCall`, and the `GetMaintainerConfig` method now reference `maintainerconfig.MaintainerConfig` instead of `pkg.MaintainerConfig`. Do NOT hand-edit the mock — let counterfeiter rewrite it. If `make generate` produces no other diff, that is expected (only this one mock changes).

6. **Fix `watcher/github-release/pkg/watcher_test.go`.** Add the import `"github.com/bborbe/maintainer/lib/maintainerconfig"` and replace the four constructions:
   - `pkg.MaintainerConfig{Release: pkg.MaintainerReleaseConfig{AutoRelease: true}}` → `maintainerconfig.MaintainerConfig{Release: maintainerconfig.ReleaseConfig{AutoRelease: true}}` (lines ~213, ~280, ~326).
   - The `GetMaintainerConfigStub` at line 77-78 returns `pkg.MaintainerConfig{…}` — change its return type signature to `(maintainerconfig.MaintainerConfig, error)` and the returned value to `maintainerconfig.MaintainerConfig{…}`.
   - After editing, `grep -n "pkg.MaintainerConfig\|pkg.MaintainerReleaseConfig" watcher/github-release/pkg/watcher_test.go` must be empty.

7. **Fix `watcher/github-release/pkg/githubclient_test.go`.** Add the import `"github.com/bborbe/maintainer/lib/maintainerconfig"` and replace every `pkg.MaintainerConfig{}` / `pkg.MaintainerConfig{Release: …}` with `maintainerconfig.MaintainerConfig{}` / `maintainerconfig.MaintainerConfig{Release: maintainerconfig.ReleaseConfig{…}}` (the `cfg).To(Equal(pkg.MaintainerConfig{}))` assertions at lines ~333, ~359, ~484, ~541, ~566, ~593 and any `Release:`-bearing expectation). The HTTP-fixture scenarios (404, rate-limit, >1 MiB, malformed YAML, valid `release.autoRelease: true`) are UNCHANGED in behavior — only the asserted Go type's package qualifier changes. Confirm a test still asserts that malformed YAML produces a wrapped error (spec § Failure Modes) and that >1 MiB produces a wrapped size error.
   - After editing, `grep -n "pkg.MaintainerConfig\|pkg.MaintainerReleaseConfig" watcher/github-release/pkg/githubclient_test.go` must be empty.

8. **Tidy modules.** From `watcher/github-release/`:

   ```bash
   go mod tidy
   ```

   so the direct dependency on the lib module's `maintainerconfig` package is recorded.

9. **Final verification** — from `watcher/github-release/`:

   ```bash
   make precommit
   ```

   Must exit 0. The watcher builds against the shared type; all existing tests pass.

</requirements>

<constraints>
- `GetMaintainerConfig` now returns `github.com/bborbe/maintainer/lib/maintainerconfig.MaintainerConfig`. (Spec § Desired Behavior 4.)
- The watcher's local `MaintainerConfig`, `MaintainerReleaseConfig`, and `parseMaintainerConfig` are DELETED — `grep -rn "parseMaintainerConfig\|MaintainerReleaseConfig" watcher/github-release/` must exit 1. (Spec § Acceptance Criteria.)
- The 404 / rate-limit (`ErrRateLimited`) / 1 MiB size-cap / decode behavior is preserved EXACTLY — this prompt changes the parsed-into Go type, nothing else. (Spec § Desired Behavior 4 & 6, § Non-goals: "Changing the github-release watcher's gate behavior … Only the Go type it parses into moves.")
- Counterfeiter mock regenerated via the existing `make generate` (counterfeiter v6, pinned in `doc.go`). Do NOT hand-edit `mocks/github_client.go`. (Spec § Constraints.)
- Error wrapping stays `github.com/bborbe/errors` — the repo-context `errors.Wrapf(ctx, err, "parse .maintainer.yaml %s/%s", …)` wrap on top of the lib's wrap is intentional. `fmt.Errorf` BANNED. (Spec § Constraints.)
- Tests stay Ginkgo v2 + Gomega, external `_test` package. (Spec § Constraints.)
- The github-releaser agent and every other watcher/agent are NOT touched. (Spec § Constraints.)
- Coverage on `watcher/github-release/pkg/...` stays ≥80%. (Spec § Constraints.)
- License header preserved on `githubclient.go`.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd watcher/github-release && make test` green after the change.
</constraints>

<verification>

Run from repo root unless noted.

```bash
# Build + tests
cd watcher/github-release && make precommit                          # exit 0
cd watcher/github-release && go test -cover ./pkg/...                # >= 80%

# Local types removed
grep -rn "parseMaintainerConfig\|MaintainerReleaseConfig" watcher/github-release/   # exits 1 / empty

# Shared lib imported in the client
grep -n "maintainerconfig" watcher/github-release/pkg/githubclient.go               # shows the import + qualified return type

# yaml no longer imported in the client (parser moved to lib)
grep -c "gopkg.in/yaml.v3" watcher/github-release/pkg/githubclient.go               # =0

# Tests reference the shared type, not the deleted local one
grep -n "pkg.MaintainerConfig\|pkg.MaintainerReleaseConfig" watcher/github-release/pkg/   # empty

# Mock returns the shared type
grep -c "maintainerconfig.MaintainerConfig" watcher/github-release/pkg/mocks/github_client.go   # >= 1

# Caller field path still compiles (unchanged)
grep -n "maintainerCfg.Release.AutoRelease" watcher/github-release/pkg/watcher.go    # present
```

</verification>
