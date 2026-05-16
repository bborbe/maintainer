---
status: completed
summary: Added isValidBranchName validation to CreateClone to block argument injection via malformed branch names, and migrated all fmt.Errorf calls in git.go to github.com/bborbe/errors with full context propagation.
container: code-reviewer-052-review-agent-pr-reviewer-security-and-error-wrapping-git
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T12:00:00Z"
queued: "2026-04-28T15:19:27Z"
started: "2026-04-28T15:52:39Z"
completed: "2026-04-28T15:56:35Z"
---

<summary>
- Branch names sourced from GitHub/Bitbucket API responses are passed directly to `git checkout` without validation, enabling argument injection via attacker-controlled branch names starting with `-` or containing `..`
- The `exec.Command` invocation blocks classic shell injection, but git flag confusion (e.g. `--upload-pack=...`) is still reachable via branches that start with a hyphen
- Several error constructors in the git package use `fmt.Errorf` instead of the project-standard context-aware error wrapping, losing stack traces on failure
- Both concerns are in the same file; fixing them together avoids a second editing pass
</summary>

<objective>
Add branch-name validation before any `git checkout` call to prevent argument injection, and migrate all `fmt.Errorf` calls in the git package to `github.com/bborbe/errors` so failures carry full context. After this fix malformed branch names are rejected before touching git, and all git errors are wrapped with context.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/git/git.go` — `CreateClone` (~line 56) which contains an inline `git checkout` invocation (~line 85–96), `validateRepoPath` (~line 121), all `fmt.Errorf` calls at lines 82, 94, 96, 125, 132, 138, 163. Note: there is no separate `Checkout` method — the checkout step is inline within `CreateClone`.
- `agent/pr-reviewer/pkg/git/git_test.go` — existing test patterns; note that success-path tests for `Clone` require a real `git` binary
- `agent/pr-reviewer/pkg/git/git_suite_test.go` — suite setup
</context>

<requirements>
1. **Add an unexported `isValidBranchName(branch string) bool` function** in `git.go`:
   ```go
   func isValidBranchName(b string) bool {
       if b == "" {
           return false
       }
       if strings.HasPrefix(b, "-") {
           return false
       }
       if strings.Contains(b, "..") {
           return false
       }
       // Allow alphanumeric, hyphen, underscore, forward slash, dot (single), at
       return regexp.MustCompile(`^[a-zA-Z0-9._/@\-]+$`).MatchString(b)
   }
   ```
   Import `regexp` and `strings` as needed.

2. **Call `isValidBranchName` at the top of `CreateClone`** (before any subprocess invocation, ~line 56). The `branch` parameter is the value to validate. Return early if invalid:
   ```go
   if !isValidBranchName(branch) {
       return "", errors.Errorf(ctx, "invalid branch name: %s", branch)
   }
   ```

3. **Replace all `fmt.Errorf` calls in `git.go` with `errors.Wrapf` / `errors.Errorf`** from `github.com/bborbe/errors`. The `ctx` parameter is available in all methods that currently use `fmt.Errorf` (~lines 82, 94, 96, 125, 132, 138, 163):
   - `fmt.Errorf("...: %w", err)` → `errors.Wrapf(ctx, err, "...")`
   - `fmt.Errorf("...")` without a causal error → `errors.Errorf(ctx, "...")`

4. **Add tests for `isValidBranchName`** in `git_test.go` (or a new `git_validation_test.go` in the same package):
   ```
   - empty string → false
   - starts with "-" → false
   - contains ".." → false
   - "feature/my-branch" → true
   - "main" → true
   - "--upload-pack=cmd" → false
   - "branch/../../../etc/passwd" → false
   ```
   Use `DescribeTable` / `Entry` following the project's Ginkgo/Gomega test conventions.

5. **Add a test for the branch-validation guard in `CreateClone`**: pass a branch name starting with `-` and assert a non-nil error is returned before any git subprocess is invoked.

6. **Run `cd agent/pr-reviewer && make test`** — must pass.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` (always pass `ctx` as first arg) — never `fmt.Errorf`, never bare `return err`
- Code changes MUST add or update tests for changed paths — paths covered before the fix must remain covered, new paths must be tested
- The regexp pattern must be conservative (allowlist, not denylist) — prefer false-negatives over false-positives for branch names
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
