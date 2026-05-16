---
status: completed
spec: [014-private-github-repo-support]
summary: 'Added git auth-failure detection: IsGitAuthFailure helper in pkg/git/auth_failure.go, pre-parse of clone_url in Run(), NeedsInput return on auth failures with host/owner/repo diagnostic and GH_TOKEN hint, token non-leakage enforced, all tests passing, make precommit exit 0.'
container: code-reviewer-083-spec-014-clone-auth-failure
dark-factory-version: v0.147.2-1-g30ba42f
created: "2026-05-03T18:00:00Z"
queued: "2026-05-03T18:14:47Z"
started: "2026-05-03T18:27:00Z"
completed: "2026-05-03T18:31:27Z"
branch: dark-factory/private-github-repo-support
---

<summary>
- When `repoManager.EnsureWorktree` fails because git could not authenticate to the remote host (auth-failure substrings from git stderr: `"could not read Username"`, `"Authentication failed"`, `"Repository not found"`, `"403"`, `"401"`), the checkout execution step now returns `AgentStatusNeedsInput` instead of propagating the error
- The `NeedsInput` diagnostic names the parsed `host/owner/repo` from the `clone_url` frontmatter field and hints at the `GH_TOKEN` configuration — never including the token value itself
- A helper function `IsGitAuthFailure(err error) bool` (in `agent/pr-reviewer/pkg/git/`) is added to centralize the auth-failure substring check, keeping the detection logic testable in isolation
- Existing failure paths are preserved: a malformed `clone_url` (unparseable) still returns `AgentStatusFailed` (hard failure); a non-auth network error (e.g., DNS failure) still propagates as an error; only the subset of errors matching the auth-failure substrings is routed to `NeedsInput`
- Unit tests cover: auth-failure error string → `NeedsInput` result; each known git auth-failure substring triggers the conversion (including `"Repository not found"` which GitHub returns for unauthenticated access to private repos and accepts a known false-positive on typo'd public repos); non-auth error string → error propagation (unchanged); the diagnostic on the `NeedsInput` path contains `host/owner/repo` and `GH_TOKEN` hint; token non-leakage is verified by injecting a distinctive fake token (e.g. `"FAKE_TOKEN_DO_NOT_LEAK_xyz123"`) into the underlying clone error and asserting it does NOT appear in `result.Message`
- The no-clone invariant holds: when `NeedsInput` is returned from the auth-failure path, `EnsureWorktree` was called but failed before producing a clone — the task is routed to human review without a filesystem clone artifact
- CHANGELOG `## Unreleased` entry added covering the auth-failure-to-NeedsInput translation and the operator-visible behavior change on private repos
</summary>

<objective>
Add the clone-failure → `NeedsInput` translation to `checkoutExecutionStep.Run()` in `agent/pr-reviewer/pkg/steps_checkout_execution.go`. When `repoManager.EnsureWorktree` fails with a git auth-failure error (no usable credentials for the remote host), return `AgentStatusNeedsInput` with a diagnostic naming `host/owner/repo` and pointing operators at `GH_TOKEN`. This is the observable signal that distinguishes "private repo, no token" from "network down" or "malformed URL" — it routes the task to human review and gives operators an actionable fix hint.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-error-wrapping-guide.md` from `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never fmt.Errorf.
Read `go-testing-guide.md` from `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo v2/Gomega, external test packages.

Files to read before making any changes:

- `agent/pr-reviewer/pkg/steps_checkout_execution.go` — full file; understand the `Run()` method; the `EnsureWorktree` call at ~line 99 and the existing allowlist check pattern at ~line 95; you modify the error handling after `EnsureWorktree`
- `agent/pr-reviewer/pkg/steps_checkout_execution_test.go` — full file; understand existing test cases and the fakeRepoManager setup; you add auth-failure test cases
- `agent/pr-reviewer/pkg/git/repo_manager.go` — read `cloneBare` method (~line 77); note that git auth failures produce stderr like `"could not read Username for 'https://github.com': terminal prompts disabled"` or `"remote: Repository not found."` or `"The requested URL returned error: 403"`; the error returned by `cloneBare` is `errors.Errorf(ctx, "git clone --bare: %s", stderr.String())` — the auth-failure substring is embedded in the error message string
- `agent/pr-reviewer/pkg/git/clone_url.go` — `ParseCloneURLParts` and `CloneURLParts` struct (Host, Owner, Repo fields); you use this to extract `host/owner/repo` for the diagnostic when the clone fails

Key facts (verified against the codebase):
- `checkoutExecutionStep.Run()` currently calls `errors.Wrapf(ctx, err, "ensure worktree clone_url=%s ref=%s task_id=%s", ...)` when `EnsureWorktree` fails; this propagates as a fatal error — you change this block
- The `checkAllowlist` helper already calls `git.ParseCloneURLParts` — use the same function for the diagnostic in the auth-failure path (parse `cloneURL` before calling `EnsureWorktree`, store the result, use it in the error handler)
- `agentlib.AgentStatusNeedsInput` is already used in `checkAllowlist`; use the same constant
- The auth-failure detection must be based on the error message string because `repoManager.EnsureWorktree` does not return typed sentinel errors — use `strings.Contains(err.Error(), ...)` checks
- The known auth-failure substrings from git on alpine (github.com): `"could not read Username"`, `"Authentication failed"`, `"Repository not found"`, `"returned error: 403"`, `"returned error: 401"`; these cover the three failure modes from the spec's failure table (no token, invalid/revoked token, and the "repository not found" message GitHub returns for private repos when unauthenticated)
- `IsGitAuthFailure` belongs in `agent/pr-reviewer/pkg/git/` (e.g., `agent/pr-reviewer/pkg/git/auth_failure.go`) so it is co-located with the git package and testable in isolation; `steps_checkout_execution.go` calls `git.IsGitAuthFailure(err)` 
- Pre-parsing `clone_url` into `CloneURLParts` BEFORE the `EnsureWorktree` call avoids a second parse in the error handler; store `parts, parseErr := git.ParseCloneURLParts(ctx, cloneURL)` after the frontmatter nil checks and use `parts` in both the `checkAllowlist` call and the auth-failure diagnostic. If `parseErr != nil` at this pre-parse stage, return `AgentStatusFailed` immediately (the clone_url is unparseable — same hard-failure behavior as before)
- NOTE: the allowlist check (`s.checkAllowlist`) currently also calls `git.ParseCloneURLParts` internally. After this prompt, the pre-parse makes that internal parse redundant. However, do NOT refactor `checkAllowlist` to accept pre-parsed parts — leave that as a follow-up. Simply add the pre-parse before the allowlist call in `Run()` and keep `checkAllowlist` unchanged.
- The diagnostic format for the `NeedsInput` result is a fixed template that does NOT embed `err.Error()`: `fmt.Sprintf("execution step: clone failed for %s: authentication required (set GH_TOKEN and re-trigger)", repoKey)` where `repoKey = parts.Host+"/"+parts.Owner+"/"+parts.Repo`. The underlying git error is intentionally NOT included in the diagnostic — it is logged at glog `v(2)` so operators can dig into pod logs, but the task's `## Review` / `Message` field stays clean and free of any risk that git ever echoed credential-bearing strings
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Verify `AgentStatusNeedsInput` is already imported** in `steps_checkout_execution.go` (it is used by `checkAllowlist`). No new import needed for the constant.

2. **Create `agent/pr-reviewer/pkg/git/auth_failure.go`**:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package git

   import "strings"

   // gitAuthFailureSubstrings are the error message fragments that git (and GitHub)
   // produce when authentication fails on an HTTPS clone. The agent detects these
   // to distinguish "no usable credentials" from generic network or config errors.
   //
   // TRADE-OFF: substring matching is pattern-based and brittle — git or gh CLI
   // upgrades may rephrase these strings, silently breaking detection. This is an
   // interim solution until a typed sentinel error (or structured error code) is
   // plumbed through the git package. See spec 014 for the follow-up.
   //
   // Notably, "Repository not found" is intentionally classified as auth failure:
   // GitHub returns this exact message when an unauthenticated client requests a
   // private repository. The known false-positive is a typo'd public repo URL —
   // an operator who mistypes a public repo name will see this routed to
   // NeedsInput as "auth required" rather than as a hard failure. This is an
   // acceptable trade-off because the diagnostic still names host/owner/repo and
   // the operator can verify the URL when re-triggering.
   var gitAuthFailureSubstrings = []string{
       "could not read Username",
       "Authentication failed",
       "Repository not found", // private repos w/o creds get this exact message; see comment above
       "returned error: 403",
       "returned error: 401",
   }

   // IsGitAuthFailure reports whether err looks like a git authentication failure
   // on an HTTPS remote. Returns false for nil.
   func IsGitAuthFailure(err error) bool {
       if err == nil {
           return false
       }
       msg := err.Error()
       for _, sub := range gitAuthFailureSubstrings {
           if strings.Contains(msg, sub) {
               return true
           }
       }
       return false
   }
   ```

3. **Create `agent/pr-reviewer/pkg/git/auth_failure_test.go`**:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package git_test

   import (
       "errors"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"
   )

   var _ = Describe("IsGitAuthFailure", func() {
       DescribeTable("returns true for known auth-failure substrings",
           func(msg string) {
               Expect(git.IsGitAuthFailure(errors.New(msg))).To(BeTrue())
           },
           Entry("no username prompt disabled", "git clone --bare: fatal: could not read Username for 'https://github.com': terminal prompts disabled"),
           Entry("authentication failed", "git clone --bare: remote: Authentication failed for 'https://github.com/bborbe/trading.git/'"),
           Entry("repository not found", "git clone --bare: remote: Repository not found."),
           Entry("403 error", "git clone --bare: The requested URL returned error: 403"),
           Entry("401 error", "git clone --bare: The requested URL returned error: 401"),
       )

       DescribeTable("returns false for non-auth errors",
           func(msg string) {
               Expect(git.IsGitAuthFailure(errors.New(msg))).To(BeFalse())
           },
           Entry("DNS failure", "git clone --bare: unable to access 'https://github.com/bborbe/foo.git/': Could not resolve host: github.com"),
           Entry("connection refused", "git clone --bare: fatal: unable to connect to github.com"),
           Entry("ref not found", "git clone --bare: error: pathspec 'no-such-ref' did not match any file"),
           Entry("generic error", "some other error"),
       )

       It("returns false for nil error", func() {
           Expect(git.IsGitAuthFailure(nil)).To(BeFalse())
       })
   })
   ```

4. **Refactor `checkoutExecutionStep.Run()` in `agent/pr-reviewer/pkg/steps_checkout_execution.go`**:

   Step 4a — before the `s.checkAllowlist` call, add a pre-parse of `clone_url` into `CloneURLParts`:

   Replace the block after the `if baseRef == ""` check:
   ```go
   // Pre-parse clone_url to extract host/owner/repo for allowlist and
   // auth-failure diagnostics. A parse failure is a hard error — the URL is
   // malformed and no clone can proceed.
   parts, parseErr := git.ParseCloneURLParts(ctx, cloneURL)
   if parseErr != nil {
       return &agentlib.Result{
           Status:  agentlib.AgentStatusFailed,
           Message: fmt.Sprintf("execution step: failed to parse clone_url: %v", parseErr),
       }, nil
   }
   repoKey := parts.Host + "/" + parts.Owner + "/" + parts.Repo
   ```

   Step 4b — replace the `s.checkAllowlist(ctx, cloneURL)` call with one that still uses `cloneURL`:
   ```go
   if result := s.checkAllowlist(ctx, cloneURL); result != nil {
       return result, nil
   }
   ```
   (No change here — `checkAllowlist` continues to call `ParseCloneURLParts` internally. The pre-parse in step 4a is separate, used only for the auth-failure diagnostic below.)

   Step 4c — replace the `EnsureWorktree` error handling block:

   **Before (current code):**
   ```go
   worktreePath, err := s.repoManager.EnsureWorktree(ctx, cloneURL, ref, taskID)
   if err != nil {
       return nil, errors.Wrapf(
           ctx,
           err,
           "ensure worktree clone_url=%s ref=%s task_id=%s",
           cloneURL,
           ref,
           taskID,
       )
   }
   ```

   **After:**
   ```go
   worktreePath, err := s.repoManager.EnsureWorktree(ctx, cloneURL, ref, taskID)
   if err != nil {
       if git.IsGitAuthFailure(err) {
           // Underlying git error is intentionally NOT included in the diagnostic
           // (it could in theory echo credential-bearing strings). Operators dig
           // into pod logs at glog v(2) for the raw git stderr.
           glog.V(2).Infof("clone auth failure repo=%s ref=%s task_id=%s err=%v", repoKey, ref, taskID, err)
           return &agentlib.Result{
               Status: agentlib.AgentStatusNeedsInput,
               Message: fmt.Sprintf(
                   "execution step: clone failed for %s: authentication required (set GH_TOKEN and re-trigger)",
                   repoKey,
               ),
           }, nil
       }
       return nil, errors.Wrapf(
           ctx,
           err,
           "ensure worktree repo=%s ref=%s task_id=%s",
           repoKey,
           ref,
           taskID,
       )
   }
   ```

   Note: the error wrap message now uses `repoKey` (safe `host/owner/repo`) instead of `cloneURL` (could be an HTTPS URL that in theory could contain a token in future code). This is a security hardening — per the spec: "safe identifiers only in wrap messages". The auth-failure diagnostic is a fixed template containing only `repoKey` — `err.Error()` is NOT embedded, so any credential-bearing string git might echo cannot leak into the task's `## Review` / `Message` field. The raw git error is preserved at glog `v(2)` for operator debugging.

5. **Add tests to `agent/pr-reviewer/pkg/steps_checkout_execution_test.go`**:

   Add the following test cases inside the existing `Describe("Run")` block. Read the existing test structure first to find the right insertion point (after the existing `EnsureWorktree` error cases if any, or as new `Context` blocks before the success path).

   ```go
   Context("when EnsureWorktree fails with a git auth-failure error", func() {
       BeforeEach(func() {
           repoManager.EnsureWorktreeReturns(
               "",
               fmt.Errorf("git clone --bare: fatal: could not read Username for 'https://github.com': terminal prompts disabled"),
           )
       })

       It("returns AgentStatusNeedsInput", func() {
           md, err := agentlib.ParseMarkdown(ctx, "---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n")
           Expect(err).NotTo(HaveOccurred())
           result, err := step.Run(ctx, md)
           Expect(err).NotTo(HaveOccurred())
           Expect(result).NotTo(BeNil())
           Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
       })

       It("diagnostic names host/owner/repo", func() {
           md, err := agentlib.ParseMarkdown(ctx, "---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n")
           Expect(err).NotTo(HaveOccurred())
           result, _ := step.Run(ctx, md)
           Expect(result.Message).To(ContainSubstring("github.com/bborbe/trading"))
       })

       It("diagnostic contains GH_TOKEN hint", func() {
           md, err := agentlib.ParseMarkdown(ctx, "---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n")
           Expect(err).NotTo(HaveOccurred())
           result, _ := step.Run(ctx, md)
           Expect(result.Message).To(ContainSubstring("GH_TOKEN"))
       })

       It("diagnostic does NOT leak the underlying git error (token non-leakage)", func() {
           // Inject a distinctive fake token into the underlying clone error.
           // The diagnostic uses a fixed template and must not echo err.Error(),
           // so the fake token must NOT appear in result.Message.
           const fakeToken = "FAKE_TOKEN_DO_NOT_LEAK_xyz123"
           repoManager.EnsureWorktreeReturns(
               "",
               fmt.Errorf("git clone --bare: fatal: could not read Username for 'https://%s@github.com': terminal prompts disabled", fakeToken),
           )
           md, err := agentlib.ParseMarkdown(ctx, "---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n")
           Expect(err).NotTo(HaveOccurred())
           result, runErr := step.Run(ctx, md)
           Expect(runErr).NotTo(HaveOccurred())
           Expect(result).NotTo(BeNil())
           Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
           Expect(result.Message).NotTo(ContainSubstring(fakeToken))
       })
   })

   Context("when EnsureWorktree fails with 'Repository not found'", func() {
       // GitHub returns this exact string for unauthenticated requests to private
       // repos. Intentionally classified as auth failure; known false-positive on
       // typo'd public repo URLs (operator can verify URL when re-triggering).
       BeforeEach(func() {
           repoManager.EnsureWorktreeReturns(
               "",
               fmt.Errorf("git clone --bare: remote: Repository not found.\nfatal: repository 'https://github.com/bborbe/private.git/' not found"),
           )
       })

       It("returns AgentStatusNeedsInput", func() {
           md, err := agentlib.ParseMarkdown(ctx, "---\nclone_url: https://github.com/bborbe/private.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n")
           Expect(err).NotTo(HaveOccurred())
           result, runErr := step.Run(ctx, md)
           Expect(runErr).NotTo(HaveOccurred())
           Expect(result).NotTo(BeNil())
           Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
           Expect(result.Message).To(ContainSubstring("github.com/bborbe/private"))
           Expect(result.Message).To(ContainSubstring("GH_TOKEN"))
       })
   })

   Context("when EnsureWorktree fails with a non-auth error", func() {
       BeforeEach(func() {
           repoManager.EnsureWorktreeReturns(
               "",
               fmt.Errorf("git clone --bare: unable to access 'https://github.com/bborbe/foo.git/': Could not resolve host: github.com"),
           )
       })

       It("propagates the error (not NeedsInput)", func() {
           md, err := agentlib.ParseMarkdown(ctx, "---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n")
           Expect(err).NotTo(HaveOccurred())
           result, runErr := step.Run(ctx, md)
           Expect(runErr).To(HaveOccurred())
           Expect(result).To(BeNil())
       })
   })
   ```

6. **Update `CHANGELOG.md`** — append to the existing `## Unreleased` section (created by sibling prompt 1; if not yet present, create it):

   ```markdown
   - feat(pr-reviewer): translate git auth-failure clone errors to `AgentStatusNeedsInput`, routing private-repo tasks to human review with a diagnostic naming `host/owner/repo` and a `GH_TOKEN` config hint. Adds `git.IsGitAuthFailure` helper covering known GitHub auth-failure substrings.
   ```

7. **Run `make precommit`** in `agent/pr-reviewer/`:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```

</requirements>

<constraints>
- Only edit files under `agent/pr-reviewer/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- The `NeedsInput` path MUST only fire when `git.IsGitAuthFailure(err) == true`; all other `EnsureWorktree` errors continue to propagate as wrapped errors (fatal)
- The diagnostic for the auth-failure `NeedsInput` path MUST contain `host/owner/repo` (from `ParseCloneURLParts`) and MUST contain the string `"GH_TOKEN"`
- The diagnostic MUST be a fixed template — it MUST NOT embed `err.Error()` from the underlying clone failure. The raw git error is logged at glog `v(2)` only. This is verified by a test that injects a distinctive fake token into the underlying clone error and asserts the token does NOT appear in `result.Message`.
- `IsGitAuthFailure` lives in `agent/pr-reviewer/pkg/git/` package, not in `pkg/` — keep it co-located with the git error production code
- Error wrapping for the non-auth path uses `repoKey` (safe identifier `host/owner/repo`) instead of `cloneURL` in the wrap message — security hardening per spec constraints
- Use `github.com/bborbe/errors` (`errors.Wrapf`); never `fmt.Errorf` for new error-wrapping calls
- The existing `checkAllowlist` method is NOT changed — it continues to call `ParseCloneURLParts` internally; the pre-parse in `Run()` is an addition, not a replacement
- `make precommit` runs from `agent/pr-reviewer/`, never at repo root
- Existing tests must pass without modification
</constraints>

<verification>
cd agent/pr-reviewer && make precommit

# Confirm new git auth failure file:
ls agent/pr-reviewer/pkg/git/auth_failure.go agent/pr-reviewer/pkg/git/auth_failure_test.go

# Confirm IsGitAuthFailure is called in the checkout step:
grep -n "IsGitAuthFailure" agent/pr-reviewer/pkg/steps_checkout_execution.go

# Confirm NeedsInput is returned on auth failure:
grep -n "AgentStatusNeedsInput\|authentication required (set GH_TOKEN" agent/pr-reviewer/pkg/steps_checkout_execution.go

# Confirm repoKey (not cloneURL) is in the wrap message:
grep -n "repo=" agent/pr-reviewer/pkg/steps_checkout_execution.go
# Expected: repo=%s with repoKey, not cloneURL

# Confirm test cases for auth failure added:
grep -n "auth-failure\|NeedsInput\|could not read Username" agent/pr-reviewer/pkg/steps_checkout_execution_test.go

# Confirm CHANGELOG updated:
grep -n "IsGitAuthFailure\|auth-failure\|private repo" CHANGELOG.md
</verification>
