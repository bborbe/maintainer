---
status: completed
spec: [049-github-releaser-execution-phase-direct-push]
summary: Created agent/github-releaser/pkg/git package with GitOps interface, osExecGitOps implementation, 8-category error_classifier enum, and counterfeiter mock
container: maintainer-github-releaser-exec-195-spec-049-git-package
dark-factory-version: v0.173.0
created: "2026-05-28T00:00:00Z"
queued: "2026-05-28T22:17:49Z"
started: "2026-05-29T08:20:15Z"
completed: "2026-05-29T08:30:08Z"
---

<summary>
- Introduces a new `pkg/git/` package inside the github-releaser agent that wraps git shell-outs behind a tiny `GitOps` interface (Clone, Commit, Tag, Push).
- The interface is the only seam used by the future execution step — callers are decoupled from `os/exec` and can be tested with a counterfeiter mock.
- An `error_classifier` maps real git stderr fragments to a closed enum of 8 categories (auth, repo_not_found, changelog_missing, unreleased_not_found, tag_collision, protected_branch_rejected, push_non_fast_forward, unknown). This is what downstream code branches on for retry / PR-fallback decisions.
- A counterfeiter mock at `pkg/git/mocks/git_ops.go` is generated and committed.
- All git args are passed as explicit slices to `exec.CommandContext` — no shell interpolation. HTTPS auth via `https://x-access-token:${GH_TOKEN}@github.com/...` URL transformation.
- Coverage ≥ 75% on `pkg/git/` — error classifier and URL transformation are the unit-testable parts (the shell-out itself is integration territory).
</summary>

<objective>
Create the `agent/github-releaser/pkg/git/` package: a thin shell-out wrapper around `git clone / commit / tag / push` exposing a 4-method `GitOps` interface, an `error_classifier` that maps stderr to a closed enum, and a counterfeiter mock. This is the dependency that prompt 3's `ExecutionStep` will consume.

End state: `cd agent/github-releaser && go test -cover ./pkg/git/...` reports ≥ 75% coverage; the interface is frozen; the 8-category enum is exhaustively tested via DescribeTable.
</objective>

<context>
Read before writing code (repo-relative paths; container mounts repo root at `/workspace`):

- `CLAUDE.md` at repo root.
- `specs/in-progress/049-github-releaser-execution-phase-direct-push.md` — re-read Desired Behavior 1, Constraints, Failure Modes table (8 rows), Security section, and AC rows that grep `pkg/git/`.
- `agent/pr-reviewer/pkg/git/auth_failure.go` — the canonical substring-matching pattern (`gitAuthFailureSubstrings []string` + `IsGitAuthFailure(err error) bool`). Mirror the shape — NOT the substrings (the github-releaser classifier emits 8 categories, not a single boolean).
- `agent/pr-reviewer/pkg/git/repo_manager.go` lines 60-200 — canonical pattern for `exec.CommandContext` + env allowlist + stderr capture + `errors.Errorf(ctx, "git X: %s", strings.TrimSpace(stderr.String()))`. The github-releaser variant is SIMPLER — no bare-clone cache, no worktree, no UUID validation. Each method is one `exec.CommandContext` invocation.
- `agent/github-releaser/pkg/githubchangelog/fetcher.go` — counterfeiter directive style for the same agent: `//counterfeiter:generate -o mocks/fetcher.go --fake-name Fetcher . Fetcher` (note the `--fake-name Fetcher` flag pattern).
- `agent/github-releaser/pkg/changelog/changelog_test.go` lines 14-60 — canonical `DescribeTable` + `Entry` shape for table-driven tests in this codebase.
- `agent/github-releaser/pkg/pkg_suite_test.go` — Ginkgo bootstrap pattern; mirror for `pkg/git/git_suite_test.go`.

Agent-lib / vault-cli types in scope (already in go.mod):
- `github.com/bborbe/errors` — `Wrap(ctx, err, msg)`, `Wrapf(ctx, err, fmt, args...)`, `Errorf(ctx, fmt, args...)`. NO `fmt.Errorf`.
- `github.com/golang/glog` — `glog.V(2).Infof(...)` for trace, `glog.Warningf(...)` for warnings.

Coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + DescribeTable.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter directive form.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-enum-type-pattern.md` — closed-enum constants pattern.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` — `#nosec` comment justification when shelling out.

Phase-1 reference (read-only — DO NOT copy verbatim; pr-reviewer is read-only, github-releaser writes):
- pr-reviewer's `pkg/git/` shells out to git for CLONE + FETCH + WORKTREE. The github-releaser variant shells out for CLONE + COMMIT + TAG + PUSH. Pattern is the same; arg lists differ.
</context>

<requirements>

**Run order: do steps in sequence. Run `cd agent/github-releaser && go test ./pkg/git/...` after step 6. Run `cd agent/github-releaser && make precommit` only as the final verification.**

1. **Create `agent/github-releaser/pkg/git/git.go`** — the interface and constructor. Exact shape:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package git wraps git shell-outs behind the GitOps interface so the
   // execution step can be tested with a counterfeiter mock. Implementations
   // are responsible for assembling argv slices, capturing stderr, and
   // wrapping errors via bborbe/errors. They MUST NOT use sh -c or any
   // shell-interpolated form — argv only.
   //
   // Auth model: HTTPS clones with a GitHub token are handled by URL
   // transformation at the call site (cloneURL → https://x-access-token:<tok>@host/...).
   // The package itself takes the transformed URL — it does not know about
   // tokens directly.
   package git

   //counterfeiter:generate -o mocks/git_ops.go --fake-name GitOps . GitOps

   // GitOps is the seam between the execution step and the git binary. Four
   // methods cover the entire direct-push happy path: clone target repo,
   // commit CHANGELOG rewrite, annotated tag, push commit + tag.
   //
   // All methods are context-aware — callers can cancel mid-operation.
   // workdir is the absolute path to the checkout (created and owned by the
   // caller; the package does not manage workdir lifecycle).
   type GitOps interface {
       // Clone shells out `git clone <cloneURL> <workdir>` and checks out ref.
       // cloneURL MUST already include any auth token (the package does not
       // add credentials).
       Clone(ctx context.Context, cloneURL, ref, workdir string) error

       // Commit stages paths (relative to workdir) and creates a commit with
       // the bot identity. Returns the short SHA (7 chars) of the new commit.
       // The bot identity is set per-invocation via -c user.name / -c user.email
       // — never writes to the global gitconfig.
       Commit(ctx context.Context, workdir, message string, paths ...string) (sha string, err error)

       // Tag creates an annotated tag (git tag -a <tag> -m <message>).
       // Lightweight tags are NOT supported — annotated tags carry author
       // and date metadata.
       Tag(ctx context.Context, workdir, tag, message string) error

       // Push pushes the given refs (e.g. "HEAD", "refs/tags/v1.2.7") to origin.
       // Returns the underlying stderr-wrapped error on failure — callers
       // pass this to error_classifier to map onto the error_category enum.
       Push(ctx context.Context, workdir string, refs ...string) error
   }
   ```

   Notes:
   - Add the necessary `import` block with `"context"` once you concretize the file.
   - The `//counterfeiter:generate` directive sits ABOVE the `GitOps` type declaration (not on a separate line — directly above).
   - No `New*` constructor in this file — the constructor lives in `os_exec_git_ops.go` (step 2).

2. **Create `agent/github-releaser/pkg/git/os_exec_git_ops.go`** — the concrete implementation. Mirror `agent/pr-reviewer/pkg/git/repo_manager.go` pattern (env allowlist, stderr capture, `errors.Errorf` wrapping):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package git

   import (
       "bytes"
       "context"
       "os"
       "os/exec"
       "regexp"
       "strings"

       "github.com/bborbe/errors"
   )

   // BotIdentity holds the commit author/committer identity. Hardcoded
   // intentionally: the only consumer is github-releaser, and a single
   // value is the contract. If a future spec needs override capability,
   // that spec adds the seam; until then, parameterization is YAGNI.
   //
   // Per spec 049 § Constraints + [[GitHub Release Agent Phase 1 Learnings]],
   // this MUST match the Phase 1 slash-command identity verbatim — otherwise
   // v1.7.8's release commit history breaks attribution continuity.
   type BotIdentity struct {
       Name  string
       Email string
   }

   // DefaultBotIdentity returns the Phase 1 verbatim identity. The osExecGitOps
   // struct reads this internally on every Commit / Tag — there is no override
   // path. Exposed publicly for test assertions.
   func DefaultBotIdentity() BotIdentity {
       return BotIdentity{
           Name:  "Benjamin Borbe",
           Email: "bborbe@users.noreply.github.com",
       }
   }

   // NewOSExecGitOps returns a GitOps implementation that shells out to the
   // git binary via os/exec. Zero-arg: the bot identity is constant via
   // DefaultBotIdentity().
   func NewOSExecGitOps() GitOps {
       return &osExecGitOps{}
   }

   type osExecGitOps struct{}

   // cmdEnv returns the env allowlist for git subprocesses: HOME (for ~/.gitconfig
   // fallback) + PATH (to resolve git). Strict allowlist prevents pod-level
   // secrets from leaking. Mirrors pr-reviewer's repoManager.cmdEnv.
   func (g *osExecGitOps) cmdEnv() []string {
       return []string{
           "HOME=" + os.Getenv("HOME"),
           "PATH=" + os.Getenv("PATH"),
       }
   }

   func (g *osExecGitOps) Clone(ctx context.Context, cloneURL, ref, workdir string) error {
       // git clone --branch <ref> --depth 1 <cloneURL> <workdir>
       // --depth 1 is acceptable because we only rewrite CHANGELOG and push a single
       // commit + tag; we don't need history beyond HEAD.
       // #nosec G204 -- cloneURL constructed in caller from validated frontmatter; workdir is os.TempDir-rooted; ref validated by caller
       cmd := exec.CommandContext(ctx, "git", "clone", "--branch", ref, "--depth", "1", cloneURL, workdir)
       cmd.Env = g.cmdEnv()
       var stderr bytes.Buffer
       cmd.Stderr = &stderr
       if err := cmd.Run(); err != nil {
           return errors.Errorf(ctx, "git clone: %s", redactToken(strings.TrimSpace(stderr.String())))
       }
       return nil
   }

   func (g *osExecGitOps) Commit(
       ctx context.Context,
       workdir, message string,
       paths ...string,
   ) (string, error) {
       // git -C <workdir> add <paths...>
       if len(paths) > 0 {
           addArgs := append([]string{"-C", workdir, "add", "--"}, paths...)
           // #nosec G204 -- workdir is os.TempDir-rooted; paths come from execution step (CHANGELOG.md only)
           if out, err := exec.CommandContext(ctx, "git", addArgs...).CombinedOutput(); err != nil {
               return "", errors.Errorf(ctx, "git add: %s", strings.TrimSpace(string(out)))
           }
       }

       // git -C <workdir> -c user.name=<name> -c user.email=<email> commit -m <message>
       commitArgs := []string{
           "-C", workdir,
           "-c", "user.name=" + DefaultBotIdentity().Name,
           "-c", "user.email=" + DefaultBotIdentity().Email,
           "commit",
           "-m", message,
       }
       // #nosec G204 -- workdir is os.TempDir-rooted; identity is the bot constant; message comes from execution step
       if out, err := exec.CommandContext(ctx, "git", commitArgs...).CombinedOutput(); err != nil {
           return "", errors.Errorf(ctx, "git commit: %s", strings.TrimSpace(string(out)))
       }

       // git -C <workdir> rev-parse --short HEAD → short SHA
       // #nosec G204 -- workdir is os.TempDir-rooted; args are hardcoded
       shaBytes, err := exec.CommandContext(ctx, "git", "-C", workdir, "rev-parse", "--short", "HEAD").Output()
       if err != nil {
           return "", errors.Wrap(ctx, err, "git rev-parse HEAD")
       }
       return strings.TrimSpace(string(shaBytes)), nil
   }

   func (g *osExecGitOps) Tag(ctx context.Context, workdir, tag, message string) error {
       // git -C <workdir> -c user.name=<name> -c user.email=<email> tag -a <tag> -m <message>
       args := []string{
           "-C", workdir,
           "-c", "user.name=" + DefaultBotIdentity().Name,
           "-c", "user.email=" + DefaultBotIdentity().Email,
           "tag", "-a", tag, "-m", message,
       }
       // #nosec G204 -- workdir is os.TempDir-rooted; identity is the bot constant; tag and message come from execution step
       cmd := exec.CommandContext(ctx, "git", args...)
       cmd.Env = g.cmdEnv()
       var stderr bytes.Buffer
       cmd.Stderr = &stderr
       if err := cmd.Run(); err != nil {
           return errors.Errorf(ctx, "git tag: %s", strings.TrimSpace(stderr.String()))
       }
       return nil
   }

   func (g *osExecGitOps) Push(ctx context.Context, workdir string, refs ...string) error {
       // git -C <workdir> push --atomic origin <refs...>
       // --atomic ensures HEAD + tag land together or neither lands. Without it,
       // GitHub may accept HEAD and reject the tag (or vice versa), leaving an
       // inconsistent state on the remote.
       args := append([]string{"-C", workdir, "push", "--atomic", "origin"}, refs...)
       // No --force / --force-with-lease — non-fast-forward maps to retry, not overwrite.
       // #nosec G204 -- workdir is os.TempDir-rooted; refs are constructed by execution step from validated frontmatter ref / tag
       cmd := exec.CommandContext(ctx, "git", args...)
       cmd.Env = g.cmdEnv()
       var stderr bytes.Buffer
       cmd.Stderr = &stderr
       if err := cmd.Run(); err != nil {
           return errors.Errorf(ctx, "git push: %s", redactToken(strings.TrimSpace(stderr.String())))
       }
       return nil
   }

   // redactToken strips x-access-token:<TOK>@ patterns from stderr to prevent
   // GH_TOKEN from landing in error logs. Git can echo the URL with embedded
   // credentials on auth/clone failures (e.g.
   // "fatal: unable to access 'https://x-access-token:ghp_AAA@github.com/...'").
   // Apply to ALL Clone/Push stderr that gets wrapped into errors.
   func redactToken(s string) string {
       // Replace x-access-token:<anything-up-to-@> with x-access-token:[REDACTED]
       return tokenURLRegexp.ReplaceAllString(s, "x-access-token:[REDACTED]@")
   }

   var tokenURLRegexp = regexp.MustCompile(`x-access-token:[^@\s]+@`)
   ```

   Notes:
   - The `#nosec G204` justifications must be present — gosec flags `exec.CommandContext` calls with non-literal args and `make precommit` will fail without them.
   - `errors.Errorf` / `errors.Wrap` from `github.com/bborbe/errors`. NO `fmt.Errorf`.
   - The Commit + Tag invocations pass `user.name` / `user.email` via `-c` flags so the global gitconfig is never touched. This matches pr-reviewer's read-only stance but applied to write operations.

3. **Create `agent/github-releaser/pkg/git/error_classifier.go`** — closed-enum classifier:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package git

   import "strings"

   // ErrorCategory is the closed enum returned by ClassifyError. Production
   // code (the execution step) branches on these values to write
   // ## Result.error_category and to drive retry policy in the controller.
   //
   // The set is CLOSED — adding a new category requires a spec amendment.
   type ErrorCategory string

   const (
       // ErrorCategoryAuth — git server rejected credentials (401/403, missing username).
       ErrorCategoryAuth ErrorCategory = "auth"
       // ErrorCategoryRepoNotFound — clone target does not exist on the server (404).
       // NOTE: GitHub returns "Repository not found" for BOTH a typo'd repo URL
       // AND an unauthenticated request for a private repo. pr-reviewer
       // intentionally classifies this as auth; github-releaser treats it as
       // repo_not_found because the watcher allowlist + IAT auth eliminate the
       // private-repo confounder upstream. If watcher emits it, the repo is
       // public — a 404 truly means it does not exist.
       ErrorCategoryRepoNotFound ErrorCategory = "repo_not_found"
       // ErrorCategoryChangelogMissing — CHANGELOG.md is absent in the cloned repo.
       // Detected at the filesystem layer (os.ReadFile ENOENT), not via git stderr.
       ErrorCategoryChangelogMissing ErrorCategory = "changelog_missing"
       // ErrorCategoryUnreleasedNotFound — RewriteUnreleasedHeader could not find
       // the "## Unreleased" line. Detected via changelog package error, not git stderr.
       ErrorCategoryUnreleasedNotFound ErrorCategory = "unreleased_not_found"
       // ErrorCategoryTagCollision — annotated tag already exists on the remote.
       ErrorCategoryTagCollision ErrorCategory = "tag_collision"
       // ErrorCategoryProtectedBranchRejected — branch protection rejected the push.
       // Consumed by the PR-fallback spec (separate).
       ErrorCategoryProtectedBranchRejected ErrorCategory = "protected_branch_rejected"
       // ErrorCategoryPushNonFastForward — remote moved between clone and push;
       // controller retry will re-fetch.
       ErrorCategoryPushNonFastForward ErrorCategory = "push_non_fast_forward"
       // ErrorCategoryUnknown — message does not match any known fragment. Bug
       // signal: if this fires repeatedly, add a new substring to the table.
       ErrorCategoryUnknown ErrorCategory = "unknown"
   )

   // classifierEntry maps a substring fragment to a category. Order matters:
   // more-specific fragments must come first (protected-branch tokens before
   // generic push errors).
   type classifierEntry struct {
       Fragment string
       Category ErrorCategory
   }

   // classifierTable is the canonical substring→category mapping. Distinct
   // fragment per category — adding entries requires a spec amendment.
   //
   // Order rationale:
   //   - Protected-branch fragments scanned BEFORE generic push fragments
   //     ("non-fast-forward") because GitHub's GH006 message can include
   //     both "Protected branch" and "non-fast-forward" tokens in some
   //     server responses.
   //   - tag-collision fragments before auth/repo because tag failures
   //     are short-circuited at the tag step, but defensive ordering
   //     remains.
   var classifierTable = []classifierEntry{
       // Protected-branch fragments (push step).
       {Fragment: "protected branch", Category: ErrorCategoryProtectedBranchRejected},
       {Fragment: "GH006", Category: ErrorCategoryProtectedBranchRejected},
       {Fragment: "Required reviews", Category: ErrorCategoryProtectedBranchRejected},
       {Fragment: "required status checks", Category: ErrorCategoryProtectedBranchRejected},
       // Non-fast-forward (push step).
       {Fragment: "non-fast-forward", Category: ErrorCategoryPushNonFastForward},
       {Fragment: "Updates were rejected because the remote contains work", Category: ErrorCategoryPushNonFastForward},
       // Tag collision (tag step).
       {Fragment: "already exists", Category: ErrorCategoryTagCollision},
       // Repo not found (clone step).
       {Fragment: "Repository not found", Category: ErrorCategoryRepoNotFound},
       {Fragment: "returned error: 404", Category: ErrorCategoryRepoNotFound},
       // Auth (clone step).
       {Fragment: "Authentication failed", Category: ErrorCategoryAuth},
       {Fragment: "could not read Username", Category: ErrorCategoryAuth},
       {Fragment: "returned error: 403", Category: ErrorCategoryAuth},
       {Fragment: "returned error: 401", Category: ErrorCategoryAuth},
   }

   // ClassifyError maps a git stderr-wrapped error to the closed enum.
   //
   // Returns the empty-string sentinel `ErrorCategory("")` when err is nil —
   // this distinguishes "no error to classify, this was a success" from
   // ErrorCategoryUnknown ("an error occurred but no fragment matched"). The
   // execution step branches on `category != ""` to decide whether to write
   // a failure result section. Mapping nil to "unknown" instead would let a
   // missing nil-check silently emit `error_category: unknown` in a
   // successful `## Result` — a real bug we want to surface.
   //
   // changelog_missing and unreleased_not_found are NEVER returned by this
   // function — those categories are set by the execution step at the
   // filesystem / changelog-package layer, not at the git-stderr layer.
   func ClassifyError(err error) ErrorCategory {
       if err == nil {
           return ErrorCategory("")
       }
       msg := err.Error()
       for _, entry := range classifierTable {
           if strings.Contains(msg, entry.Fragment) {
               return entry.Category
           }
       }
       return ErrorCategoryUnknown
   }
   ```

   Notes:
   - The `ErrorCategory` values are CASE-SENSITIVE strings — they round-trip into the `## Result` JSON in prompt 3.
   - `ChangelogMissing` and `UnreleasedNotFound` constants are DECLARED here for the enum to be exhaustive AND to satisfy the AC grep for "≥ 8 categories", but `ClassifyError` itself never returns them — prompt 3's execution step sets those directly at the filesystem / changelog layer (per the constant doc comment).

4. **Create `agent/github-releaser/pkg/git/git_suite_test.go`** — Ginkgo bootstrap:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package git_test

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
       RunSpecs(t, "Git Suite", suiteConfig, reporterConfig)
   }
   ```

5. **Create `agent/github-releaser/pkg/git/error_classifier_test.go`** — DescribeTable that exercises ALL 8 categories with realistic stderr fragments. The 8 categories must each appear as a literal string in the test source so `grep -cE 'auth|repo_not_found|changelog_missing|unreleased_not_found|tag_collision|protected_branch_rejected|push_non_fast_forward|unknown'` returns ≥ 8:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package git_test

   import (
       "errors"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/agent/github-releaser/pkg/git"
   )

   var _ = Describe("ClassifyError", func() {
       DescribeTable("maps git stderr fragments to the closed enum",
           func(input error, expected git.ErrorCategory) {
               Expect(git.ClassifyError(input)).To(Equal(expected))
           },
           Entry("auth — Authentication failed",
               errors.New("fatal: Authentication failed for 'https://github.com/x/y.git/'"),
               git.ErrorCategoryAuth),
           Entry("auth — could not read Username",
               errors.New("fatal: could not read Username for 'https://github.com': terminal prompts disabled"),
               git.ErrorCategoryAuth),
           Entry("repo_not_found — 404 on clone",
               errors.New("remote: Repository not found.\nfatal: repository 'https://github.com/x/missing.git/' not found"),
               git.ErrorCategoryRepoNotFound),
           Entry("tag_collision — annotated tag already exists",
               errors.New("fatal: tag 'v1.2.7' already exists"),
               git.ErrorCategoryTagCollision),
           Entry("protected_branch_rejected — GH006 on push",
               errors.New("remote: error: GH006: Protected branch update failed for refs/heads/master.\nremote: error: At least 1 approving review is required by reviewers with write access."),
               git.ErrorCategoryProtectedBranchRejected),
           Entry("push_non_fast_forward — remote advanced during release",
               errors.New("! [rejected] master -> master (non-fast-forward)\nerror: failed to push some refs to 'https://github.com/x/y.git'"),
               git.ErrorCategoryPushNonFastForward),
           Entry("unknown — unrecognized message",
               errors.New("fatal: cosmic ray flipped a bit"),
               git.ErrorCategoryUnknown),
       )

       // The two filesystem/changelog categories are declared on the enum but
       // ClassifyError never returns them — they are set directly by the
       // execution step. This test documents that contract so a future
       // refactor doesn't accidentally introduce a substring match for them.
       It("never returns changelog_missing from ClassifyError", func() {
           // changelog_missing — declared on enum but emitted by execution step at fs layer
           Expect(git.ClassifyError(errors.New("CHANGELOG.md: no such file or directory"))).
               NotTo(Equal(git.ErrorCategoryChangelogMissing))
       })
       It("never returns unreleased_not_found from ClassifyError", func() {
           // unreleased_not_found — declared on enum but emitted by changelog pkg
           Expect(git.ClassifyError(errors.New("Unreleased header not found"))).
               NotTo(Equal(git.ErrorCategoryUnreleasedNotFound))
       })

       It("returns empty-string sentinel on nil — distinguishes success from 'actually-unknown stderr'", func() {
           Expect(git.ClassifyError(nil)).To(Equal(git.ErrorCategory("")))
           Expect(git.ClassifyError(nil)).NotTo(Equal(git.ErrorCategoryUnknown))
       })
   })
   ```

   Notes:
   - The literal strings `changelog_missing` and `unreleased_not_found` appear in the comments + `git.ErrorCategoryChangelogMissing` / `git.ErrorCategoryUnreleasedNotFound` references — that's how those two categories land in the AC grep.
   - `errors.New` here is the stdlib (test-only sentinel construction). The production code never uses stdlib `errors`.

6. **Create `agent/github-releaser/pkg/git/os_exec_git_ops_test.go`** — unit tests for `DefaultBotIdentity`, constructor, AND a **boundary-crossing integration test** against a real local git repo. The integration test is the only place that proves `-c user.name`/`-c user.email` flags are wired correctly through the actual `git` binary; mocked tests in prompt 3 do NOT exercise this.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package git_test

   import (
       "context"
       "os"
       "os/exec"
       "path/filepath"
       "strings"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/agent/github-releaser/pkg/git"
   )

   var _ = Describe("DefaultBotIdentity", func() {
       It("returns Phase 1 verbatim identity", func() {
           id := git.DefaultBotIdentity()
           Expect(id.Name).To(Equal("Benjamin Borbe"))
           Expect(id.Email).To(Equal("bborbe@users.noreply.github.com"))
       })
   })

   var _ = Describe("NewOSExecGitOps", func() {
       It("returns a non-nil GitOps", func() {
           ops := git.NewOSExecGitOps()
           Expect(ops).NotTo(BeNil())
       })
   })

   // Boundary-crossing integration tests — exercise the real `git` binary
   // against a local repo. These prove the -c user.name / -c user.email
   // identity injection works through the actual shell-out, not just via mocks.
   // Skip on systems without git (CI containers have it; macOS dev workstations
   // have it). Use a per-test tempdir for isolation.
   var _ = Describe("osExecGitOps boundary contracts", func() {
       var (
           ctx     context.Context
           workdir string
           ops     git.GitOps
       )

       BeforeEach(func() {
           if _, err := exec.LookPath("git"); err != nil {
               Skip("git binary not available")
           }
           ctx = context.Background()
           var err error
           workdir, err = os.MkdirTemp("", "github-releaser-git-test-*")
           Expect(err).NotTo(HaveOccurred())
           // Initialize a local repo so Commit/Tag have something to operate on.
           init := exec.Command("git", "-C", workdir, "init", "-b", "master")
           Expect(init.Run()).To(Succeed())
           // Seed an initial commit so `git commit` against CHANGELOG.md has a parent.
           Expect(os.WriteFile(filepath.Join(workdir, "CHANGELOG.md"), []byte("# Changelog\n\n## Unreleased\n\n- feat: stub\n"), 0o644)).To(Succeed())
           ops = git.NewOSExecGitOps()
       })

       AfterEach(func() {
           os.RemoveAll(workdir)
       })

       It("Commit attributes the commit to DefaultBotIdentity via -c flags", func() {
           sha, err := ops.Commit(ctx, workdir, "release v1.2.8", "CHANGELOG.md")
           Expect(err).NotTo(HaveOccurred())
           Expect(sha).NotTo(BeEmpty())

           // Inspect the commit's author/committer name+email.
           out, runErr := exec.Command("git", "-C", workdir, "log", "-1", "--format=%an <%ae>|%cn <%ce>").CombinedOutput()
           Expect(runErr).NotTo(HaveOccurred())
           expect := "Benjamin Borbe <bborbe@users.noreply.github.com>|Benjamin Borbe <bborbe@users.noreply.github.com>"
           Expect(strings.TrimSpace(string(out))).To(Equal(expect))
       })

       It("Tag creates an annotated tag attributed to the bot identity", func() {
           _, err := ops.Commit(ctx, workdir, "release v1.2.8", "CHANGELOG.md")
           Expect(err).NotTo(HaveOccurred())
           Expect(ops.Tag(ctx, workdir, "v1.2.8", "release v1.2.8")).To(Succeed())

           // `taggername` is only set on annotated tags (not lightweight) — proves
           // we used `git tag -a` AND the -c flags were honored.
           out, runErr := exec.Command("git", "-C", workdir, "tag", "-l", "--format=%(taggername) <%(taggeremail)>", "v1.2.8").CombinedOutput()
           Expect(runErr).NotTo(HaveOccurred())
           Expect(strings.TrimSpace(string(out))).To(ContainSubstring("Benjamin Borbe"))
           Expect(strings.TrimSpace(string(out))).To(ContainSubstring("bborbe@users.noreply.github.com"))
       })

       It("Push --atomic to a local bare remote lands HEAD and tag together", func() {
           // Make commit + tag locally.
           _, err := ops.Commit(ctx, workdir, "release v1.2.8", "CHANGELOG.md")
           Expect(err).NotTo(HaveOccurred())
           Expect(ops.Tag(ctx, workdir, "v1.2.8", "release v1.2.8")).To(Succeed())

           // Set up a local bare remote and wire it as origin.
           remote, err := os.MkdirTemp("", "github-releaser-bare-*")
           Expect(err).NotTo(HaveOccurred())
           defer os.RemoveAll(remote)
           Expect(exec.Command("git", "init", "--bare", remote).Run()).To(Succeed())
           Expect(exec.Command("git", "-C", workdir, "remote", "add", "origin", remote).Run()).To(Succeed())

           Expect(ops.Push(ctx, workdir, "HEAD:master", "refs/tags/v1.2.8")).To(Succeed())

           // Verify both refs landed on the bare remote.
           headOut, headErr := exec.Command("git", "-C", remote, "rev-parse", "master").CombinedOutput()
           Expect(headErr).NotTo(HaveOccurred())
           Expect(strings.TrimSpace(string(headOut))).NotTo(BeEmpty())
           tagOut, tagErr := exec.Command("git", "-C", remote, "rev-parse", "v1.2.8").CombinedOutput()
           Expect(tagErr).NotTo(HaveOccurred())
           Expect(strings.TrimSpace(string(tagOut))).NotTo(BeEmpty())
       })
   })

   var _ = Describe("redactToken", func() {
       It("strips x-access-token credentials from stderr-like strings", func() {
           in := "fatal: unable to access 'https://x-access-token:ghp_AAA@github.com/owner/repo/': repository not found"
           out := git.RedactTokenForTest(in)
           Expect(out).NotTo(ContainSubstring("ghp_AAA"))
           Expect(out).To(ContainSubstring("x-access-token:[REDACTED]@"))
       })
   })
   ```

   Notes:
   - The boundary tests gate on `exec.LookPath("git")` — CI containers have git; the test skips gracefully if git is absent.
   - `RedactTokenForTest` is an `export_test.go`-style accessor that exposes the private `redactToken` to the test package without leaking it into the production API. Add `var RedactTokenForTest = redactToken` to a new `export_test.go` in `pkg/git/`.
   - The `NewOSExecGitOps()` signature is now zero-arg (per audit recommendation: drop the `BotIdentity` parameter; the identity is a hardcoded constant inside the struct). Update the constructor + struct accordingly.

7. **Generate the counterfeiter mock**:

   ```bash
   cd agent/github-releaser && go generate ./pkg/git/...
   ```

   This produces `agent/github-releaser/pkg/git/mocks/git_ops.go`. If `go generate` does not produce it (counterfeiter not pinned in module), invoke directly:

   ```bash
   cd agent/github-releaser && go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -o pkg/git/mocks/git_ops.go --fake-name GitOps ./pkg/git GitOps
   ```

   Verify file exists: `ls agent/github-releaser/pkg/git/mocks/git_ops.go`.

8. **Coverage check** — from `agent/github-releaser/`:

   ```bash
   go test -cover ./pkg/git/...
   ```

   Must report ≥ 75%. The classifier table walk + 8 entries + identity + constructor + DescribeTable hit every branch except the actual `exec.CommandContext` paths (those are not unit-testable offline and are exempt per the spec § Constraints coverage rationale). If coverage drops below 75%, add one more table entry to `error_classifier_test.go` (e.g. another protected-branch phrasing).

9. **Final verification** — from `agent/github-releaser/`:

   ```bash
   make precommit
   ```

   Must exit 0. No `fmt.Errorf` in `pkg/git/`. No `sh -c` anywhere. Counterfeiter mock file present.

</requirements>

<constraints>
- New package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/git`.
- Layout (exactly these files in `pkg/git/`): `git.go`, `os_exec_git_ops.go`, `error_classifier.go`, `export_test.go`, `git_suite_test.go`, `error_classifier_test.go`, `os_exec_git_ops_test.go`, plus `mocks/git_ops.go` (generated). `export_test.go` exposes the private `redactToken` helper to the test package via `var RedactTokenForTest = redactToken` — contents are a single line under the `package git` declaration.
- `GitOps` interface signature FROZEN — exact methods Clone/Commit/Tag/Push per spec § Desired Behavior 1 and the AC grep `grep -cE 'Clone\(.*\).*error|Commit\(.*\).*\(string, error\)|Tag\(.*\).*error|Push\(.*\).*error' pkg/git/git.go` ≥ 4.
- `error_category` enum is CLOSED — the 8 typed constants `ErrorCategoryAuth`, `ErrorCategoryRepoNotFound`, `ErrorCategoryChangelogMissing`, `ErrorCategoryUnreleasedNotFound`, `ErrorCategoryTagCollision`, `ErrorCategoryProtectedBranchRejected`, `ErrorCategoryPushNonFastForward`, `ErrorCategoryUnknown` are exported. String values match the spec verbatim: `"auth"`, `"repo_not_found"`, etc.
- Bot identity: `Benjamin Borbe` + `bborbe@users.noreply.github.com`. Verbatim from Phase 1 — spec § Constraints.
- All git commands via `exec.CommandContext` with explicit arg slices. NO `sh -c`, NO shell interpolation. Per spec § Security.
- Annotated tags only (`git tag -a -m`). Lightweight tags banned. Per spec § Constraints.
- Errors via `github.com/bborbe/errors` (`Wrap`/`Wrapf`/`Errorf`). `fmt.Errorf` is BANNED in production code.
- Counterfeiter directive: `//counterfeiter:generate -o mocks/git_ops.go --fake-name GitOps . GitOps` — sits directly above the `GitOps` type declaration.
- Counterfeiter pinned to `v6.12.2` in the `//go:generate` line in `git_suite_test.go`.
- Coverage ≥ 75% on `pkg/git/` — classifier + identity + constructor are the unit-testable parts; the shell-out methods are integration territory (tested via mocked GitOps in prompt 3).
- `#nosec G204` justifications on every `exec.CommandContext` call — gosec will fail precommit otherwise.
- No write access to the global gitconfig — `user.name` / `user.email` passed per-invocation via `-c` flags.
- License header (3 lines) at the top of every `.go` file.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` is green before AND after.
</constraints>

<verification>

Run from repo root unless noted.

```bash
# Build + tests + coverage
cd agent/github-releaser && make precommit                           # exit 0
cd agent/github-releaser && go test -cover ./pkg/git/...             # ≥ 75%

# Files exist
ls agent/github-releaser/pkg/git/git.go                              # exists
ls agent/github-releaser/pkg/git/os_exec_git_ops.go                  # exists
ls agent/github-releaser/pkg/git/error_classifier.go                 # exists
ls agent/github-releaser/pkg/git/git_suite_test.go                   # exists
ls agent/github-releaser/pkg/git/error_classifier_test.go            # exists
ls agent/github-releaser/pkg/git/mocks/git_ops.go                    # exists (counterfeiter output)

# Interface frozen
grep -c '^type GitOps interface' agent/github-releaser/pkg/git/git.go                                              # =1
grep -cE 'Clone\(.*\).*error|Commit\(.*\).*\(string, error\)|Tag\(.*\).*error|Push\(.*\).*error' agent/github-releaser/pkg/git/git.go   # ≥4

# Bot identity verbatim from Phase 1
grep -c 'bborbe@users.noreply.github.com' agent/github-releaser/pkg/git/os_exec_git_ops.go     # ≥1
grep -c 'Benjamin Borbe' agent/github-releaser/pkg/git/os_exec_git_ops.go                      # ≥1

# Annotated tags only
grep -c '"tag", "-a"' agent/github-releaser/pkg/git/os_exec_git_ops.go                         # ≥1

# Error-wrapping convention
grep -c 'fmt.Errorf' agent/github-releaser/pkg/git/                                            # =0
grep -c '"sh"' agent/github-releaser/pkg/git/                                                   # =0
grep -c 'sh -c' agent/github-releaser/pkg/git/                                                  # =0

# 8-category enum exhaustively tested
grep -cE 'auth|repo_not_found|changelog_missing|unreleased_not_found|tag_collision|protected_branch_rejected|push_non_fast_forward|unknown' agent/github-releaser/pkg/git/error_classifier_test.go   # ≥8

# Counterfeiter directive present and well-formed
grep -c 'counterfeiter:generate -o mocks/git_ops.go --fake-name GitOps' agent/github-releaser/pkg/git/git.go   # =1
```

</verification>
