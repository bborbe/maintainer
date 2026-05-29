---
status: approved
created: "2026-05-30T00:00:00Z"
queued: "2026-05-29T23:11:44Z"
---

<summary>
- Fixes a live dev failure: the github-releaser agent crashes on `git clone` when the release task carries an SSH-form `clone_url` (`git@github.com:owner/repo.git`).
- Root cause: the agent authenticates with a GitHub App installation token (HTTPS only), but SSH URLs flow through unchanged — and the in-cluster alpine image has no ssh client, so the clone dies with `cannot run ssh: No such file or directory`.
- The fix normalizes any GitHub clone URL to canonical HTTPS before token injection, so the installation token is always applied and clone/push go over HTTPS.
- Handles the common forms: `git@github.com:...`, `ssh://git@github.com/...`, and already-HTTPS URLs (left untouched); unrecognized URLs pass through unchanged so they fail loudly rather than getting mangled.
- No Dockerfile change (we are NOT adding ssh), no watcher change, no new config knobs — minimal one-helper fix.
- Adds table-driven tests for the normalizer plus an end-to-end test proving an SSH `clone_url` reaches `git clone` as a token-authenticated HTTPS URL.
</summary>

<objective>
Fix the clone-URL bug in the github-releaser agent so a release task carrying an SSH-form `clone_url` (e.g. `git@github.com:bborbe/go-skeleton.git`) clones and pushes over token-authenticated HTTPS instead of failing with `cannot run ssh: No such file or directory`. The agent uses a GitHub App installation token (HTTPS auth) and the runtime image has no ssh client, so the clone URL MUST be HTTPS for the token to inject.
</objective>

<context>
Repo: module `agent/github-releaser/` (own `go.mod`) inside the maintainer repo. The container mounts the repo root; run commands from `agent/github-releaser/`.

Read before writing code:

- `CLAUDE.md` at repo root — project conventions.
- `agent/github-releaser/pkg/steps_execution.go` — the file you change. Key anchors:
  - `executeDirectPush(ctx, md, workdir, plan, cloneURL, ref)` — contains the line `authedURL := s.injectToken(cloneURL)` immediately before `s.ops.Clone(ctx, authedURL, ref, workdir)`. The normalization goes right before `injectToken`.
  - `injectToken(cloneURL string) string` — only rewrites URLs that start with `https://`; for an SSH URL it returns the input unchanged (the bug). Signature and body are verbatim; do NOT change `injectToken`.
  - `extractFrontmatter(ctx, md)` — where `cloneURL` originates (`md.Frontmatter.String("clone_url")`). No change here.
- `agent/github-releaser/pkg/steps_execution_test.go` — existing Ginkgo v2 test patterns. External test package `package pkg_test`. Imports: `agentlib "github.com/bborbe/agent/lib"`, `. "github.com/onsi/ginkgo/v2"`, `. "github.com/onsi/gomega"`, `pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"`, `gitmocks "github.com/bborbe/maintainer/agent/github-releaser/pkg/git/mocks"`. The happy-path case already uses `fakeOps.CloneStub`, `fakeOps.CommitStub`, `fakeOps.CloneArgsForCall(0)` and asserts the injected-token HTTPS URL. The `taskMD` const fixture has `clone_url: https://github.com/bborbe/example.git`; the `writeChangelog` helper writes a CHANGELOG.md into the captured workdir.
- `CHANGELOG.md` at repo root — the canonical CHANGELOG (there is NO per-agent CHANGELOG). Add one new bullet at the TOP of the `## Unreleased` block.

Coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` context-form patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + counterfeiter mocks.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter v6 mock method names (`*Stub`, `*Returns`, `*CallCount`, `*ArgsForCall`).
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — Unreleased bullet format.
</context>

<requirements>

Run order: do steps in sequence. Run `cd agent/github-releaser && go build ./...` after step 1 to catch type errors, `go test ./pkg/...` after step 2, then `make precommit` as the final step.

1. **Add the normalizer helper to `agent/github-releaser/pkg/steps_execution.go`.** Add a package-level pure function (NOT a method — no receiver, no ctx; it is a pure string transform). Place it directly above the `injectToken` method. The function:

   ```go
   // normalizeCloneURLToHTTPS converts the common GitHub clone-URL forms to
   // canonical HTTPS so the installation-token auth in injectToken always
   // applies. The github-releaser authenticates with a GitHub App installation
   // token (HTTPS only) and the runtime image has no ssh client, so an SSH
   // clone URL can never succeed — it must be rewritten to HTTPS before
   // injectToken runs.
   //
   //	git@github.com:owner/repo.git        → https://github.com/owner/repo.git
   //	ssh://git@github.com/owner/repo.git  → https://github.com/owner/repo.git
   //	https://github.com/owner/repo.git    → unchanged
   //	https://github.com/owner/repo        → unchanged (no .git is fine)
   //
   // Any form it does not recognize is returned unchanged so the failure
   // surfaces loudly downstream rather than being silently mangled.
   func normalizeCloneURLToHTTPS(raw string) string {
       const (
           scpPrefix = "git@github.com:"
           sshPrefix = "ssh://git@github.com/"
           httpsBase = "https://github.com/"
       )
       switch {
       case strings.HasPrefix(raw, scpPrefix):
           return httpsBase + strings.TrimPrefix(raw, scpPrefix)
       case strings.HasPrefix(raw, sshPrefix):
           return httpsBase + strings.TrimPrefix(raw, sshPrefix)
       default:
           return raw
       }
   }
   ```

   `strings` is already imported in this file. No new imports. No `bborbe/errors` usage (pure function, no failure path — unrecognized input is returned as-is by design).

2. **Wire the normalizer into `executeDirectPush`.** In `executeDirectPush`, change the existing line:

   ```go
   authedURL := s.injectToken(cloneURL)
   ```

   to normalize first, then inject:

   ```go
   normalizedURL := normalizeCloneURLToHTTPS(cloneURL)
   authedURL := s.injectToken(normalizedURL)
   ```

   Everything below (the `s.ops.Clone(ctx, authedURL, ref, workdir)` call and the rest of the sequence) is unchanged. Do NOT modify `injectToken`, `extractFrontmatter`, or the watcher.

3. **Add tests to `agent/github-releaser/pkg/steps_execution_test.go`.** Two additions inside the existing `Describe("ExecutionStep", ...)` block (the normalizer is package-private but reachable in this external `_test` package via the end-to-end Clone assertion; the table test covers the pure transform indirectly through the same step — see note):

   a. **End-to-end SSH → token-HTTPS composition** (this is the load-bearing regression test). Add a new `Context` that drives a full `Run` with an SSH `clone_url` and asserts the mocked `Clone` receives a token-authenticated HTTPS URL. Mirror the existing happy-path setup (`CloneStub` writes the changelog into the captured workdir; `CommitStub`/`TagReturns`/`PushReturns` succeed). Use a task fixture identical to `taskMD` but with `clone_url: git@github.com:bborbe/example.git`:

   ```go
   Context("clone_url normalization end-to-end", func() {
       const sshTaskMD = `---
status: in_progress
phase: execution
assignee: github-releaser-agent
task_type: github-release
repo: bborbe/example
clone_url: git@github.com:bborbe/example.git
ref: master
current_version: v1.2.7
task_identifier: gh-release-bborbe-example-master-ssh
---

# release task

## Plan

` + "```json" + `
{
  "outcome": "ready",
  "bump": "patch",
  "reasoning": "fix-only batch",
  "current_version": "v1.2.7",
  "next_version": "1.2.8",
  "next_version_header": "## v1.2.8",
  "header_prefix_style": "v",
  "bullets": ["fix: thing"]
}
` + "```" + `
`

       It("rewrites an SSH clone_url to token-authenticated HTTPS before Clone", func() {
           fakeOps := &gitmocks.GitOps{}
           fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
               writeChangelog(workdir)
               return nil
           }
           fakeOps.CommitReturns("abc1234", nil)
           fakeOps.TagReturns(nil)
           fakeOps.PushReturns(nil)

           step := pkg.NewExecutionStep(fakeOps, "test-token")
           md, err := agentlib.ParseMarkdown(context.Background(), sshTaskMD)
           Expect(err).NotTo(HaveOccurred())

           result, err := step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())
           Expect(result.Status).To(Equal(agentlib.AgentStatusDone))

           Expect(fakeOps.CloneCallCount()).To(Equal(1))
           _, gotCloneURL, _, _ := fakeOps.CloneArgsForCall(0)
           Expect(gotCloneURL).To(Equal("https://x-access-token:test-token@github.com/bborbe/example.git"))
       })
   })
   ```

   b. **Table-driven normalizer cases.** Because `normalizeCloneURLToHTTPS` is package-private and the test package is external (`pkg_test`), assert its behavior through the step's observable boundary (the URL handed to `Clone`). Add a `DescribeTable` that, for each input clone_url form, runs the step with an empty token (so `injectToken` is a no-op and the assertion isolates normalization) and asserts the URL `Clone` receives:

   ```go
   DescribeTable("normalizes clone_url before clone (empty token isolates normalization)",
       func(inputCloneURL, wantCloneURL string) {
           taskMD := `---
status: in_progress
phase: execution
task_identifier: gh-release-norm-table
clone_url: ` + inputCloneURL + `
ref: master
---

## Plan

` + "```json" + `
{"outcome":"ready","next_version":"1.2.8","next_version_header":"## v1.2.8"}
` + "```" + `
`
           fakeOps := &gitmocks.GitOps{}
           fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
               writeChangelog(workdir)
               return nil
           }
           fakeOps.CommitReturns("abc1234", nil)
           fakeOps.TagReturns(nil)
           fakeOps.PushReturns(nil)

           step := pkg.NewExecutionStep(fakeOps, "") // empty token → injectToken is a no-op
           md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
           Expect(err).NotTo(HaveOccurred())

           _, err = step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())

           Expect(fakeOps.CloneCallCount()).To(Equal(1))
           _, gotCloneURL, _, _ := fakeOps.CloneArgsForCall(0)
           Expect(gotCloneURL).To(Equal(wantCloneURL))
       },
       Entry("scp form", "git@github.com:owner/repo.git", "https://github.com/owner/repo.git"),
       Entry("ssh:// form", "ssh://git@github.com/owner/repo.git", "https://github.com/owner/repo.git"),
       Entry("https with .git unchanged", "https://github.com/owner/repo.git", "https://github.com/owner/repo.git"),
       Entry("https without .git unchanged", "https://github.com/owner/repo", "https://github.com/owner/repo"),
       Entry("unrecognized form unchanged", "git://example.com/owner/repo.git", "git://example.com/owner/repo.git"),
   )
   ```

   `DescribeTable` and `Entry` come from `. "github.com/onsi/ginkgo/v2"` (already dot-imported). If the import is not yet present in this test file, it is part of the ginkgo/v2 dot-import already in use — no new import line needed.

4. **Update root `CHANGELOG.md`.** Add ONE new bullet at the TOP of the `## Unreleased` block (above the current first bullet). It MUST contain the literal substring `clone_url` and describe the fix:

   ```
   - fix(agent/github-releaser): normalize SSH clone_url forms (`git@github.com:` / `ssh://`) to token-authenticated HTTPS before clone — the agent authenticates with a GitHub App installation token (HTTPS only) and the runtime image has no ssh client, so an SSH clone_url failed in-cluster with `cannot run ssh: No such file or directory`; clone + push now always go over HTTPS
   ```

5. **Final verification** — from `agent/github-releaser/`:

   ```bash
   make precommit
   ```

   Must exit 0. No `fmt.Errorf` introduced. No `context.Background()` introduced in business logic (`normalizeCloneURLToHTTPS` is a pure function with no ctx).

</requirements>

<constraints>
- Modified files only:
  - `agent/github-releaser/pkg/steps_execution.go` (add `normalizeCloneURLToHTTPS`, wire it into `executeDirectPush` before `injectToken`)
  - `agent/github-releaser/pkg/steps_execution_test.go` (add end-to-end SSH case + normalizer DescribeTable)
  - `CHANGELOG.md` at repo root (one new Unreleased bullet at the top)
- Do NOT touch the Dockerfile. The fix is HTTPS, NOT adding an ssh client.
- Do NOT change the watcher (`watcher/github-release/`). The watcher's emitted `clone_url` is accepted as-is; normalization happens agent-side.
- Do NOT change `injectToken` or `extractFrontmatter`.
- Do NOT add config knobs, flags, or thresholds. One helper + its wiring + tests.
- `normalizeCloneURLToHTTPS` is a pure string function — no `context.Context` parameter, no error return, no `bborbe/errors`.
- Errors elsewhere (none added here) use `github.com/bborbe/errors` context-form (`Wrap`/`Wrapf`/`Errorf`). NO `fmt.Errorf`. NO `context.Background()` in business logic.
- Tests: Ginkgo v2 + Gomega, external `_test` package (`package pkg_test`), counterfeiter v6 mock (`gitmocks.GitOps`).
- License header (3 lines) already present on both modified `.go` files — keep it; do NOT duplicate.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` green before AND after.
</constraints>

<verification>

Run from repo root unless noted.

```bash
# Build + tests
cd agent/github-releaser && make precommit                                       # exit 0
cd agent/github-releaser && go test ./pkg/...                                     # all green

# Normalizer present and wired before injectToken
grep -c 'func normalizeCloneURLToHTTPS(' agent/github-releaser/pkg/steps_execution.go         # =1
grep -c 'normalizeCloneURLToHTTPS(cloneURL)' agent/github-releaser/pkg/steps_execution.go     # =1
grep -c 's.injectToken(normalizedURL)' agent/github-releaser/pkg/steps_execution.go           # =1

# Tests: end-to-end SSH→token-HTTPS + table cases
grep -c 'git@github.com:bborbe/example.git' agent/github-releaser/pkg/steps_execution_test.go # ≥1
grep -c 'https://x-access-token:test-token@github.com/bborbe/example.git' agent/github-releaser/pkg/steps_execution_test.go  # ≥1
grep -c 'DescribeTable' agent/github-releaser/pkg/steps_execution_test.go                     # ≥1
grep -c 'ssh://git@github.com/owner/repo.git' agent/github-releaser/pkg/steps_execution_test.go  # ≥1

# Convention gates
grep -c 'fmt.Errorf' agent/github-releaser/pkg/steps_execution.go                             # =0
grep -c 'context.Background' agent/github-releaser/pkg/steps_execution.go                      # =0

# Dockerfile + watcher untouched (no diff)
git diff --name-only | grep -c -i dockerfile                                                   # =0
git diff --name-only | grep -c '^watcher/'                                                     # =0

# CHANGELOG bullet within Unreleased section
awk '/^## Unreleased$/,/^## v/' CHANGELOG.md | grep -c 'clone_url'                              # ≥1
```

</verification>
