---
status: committing
spec: [013-repo-allowlist-stage-isolation]
summary: Added REPO_ALLOWLIST env var to agent/pr-reviewer with ParseCloneURLParts refactor, allowlist helper, checkout step allowlist check, and full Ginkgo/Gomega test coverage across all new paths.
container: code-reviewer-080-spec-013-agent-allowlist
dark-factory-version: v0.147.2-1-g30ba42f
created: "2026-05-03T16:30:00Z"
queued: "2026-05-03T16:58:25Z"
started: "2026-05-03T17:04:22Z"
branch: dark-factory/repo-allowlist-stage-isolation
---

<summary>
- The PR-reviewer agent gains an optional `REPO_ALLOWLIST` env var and matching CLI flag wired through both entry points (`main.go` and `cmd/run-task/main.go`)
- A malformed allowlist entry causes a startup failure at both entry points with a clear operator-facing log naming the offending entry
- The `git.ParseCloneURL` function is refactored to expose a sibling `git.ParseCloneURLParts` function that returns `host`, `owner`, and `repo` as separate fields; existing callers of `ParseCloneURL` are unchanged (it now calls the new sibling internally)
- The checkout-execution step consults the repo allowlist before calling `EnsureWorktree` — if the allowlist is non-empty and the task's `clone_url` maps to a repo not on the list, the step returns `Status: NeedsInput` with a diagnostic naming the parsed repo and the configured allowlist size, routing the task to human review without cloning
- A `clone_url` that fails to parse remains a hard `Status: Failed` — distinct from the soft `NeedsInput` allowlist-miss path
- An empty allowlist (default `""`) skips the check entirely and preserves today's behavior bit-for-bit
- The configured allowlist size is logged at startup (count only, not contents)
- Full Ginkgo/Gomega test coverage for `ParseCloneURLParts`, the new `ParseRepoAllowlist` helper, and the checkout step's allowlist-miss and allowlist-pass paths
</summary>

<objective>
Add `REPO_ALLOWLIST` filtering to the agent PR reviewer (`agent/pr-reviewer`) so it refuses to clone repos not on the operator-configured allowlist, returning `NeedsInput` to route the task to human review. This is the agent layer of spec-013's defense-in-depth design — it is the trust boundary even when a stale or mis-routed task reaches the agent.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-error-wrapping-guide.md` from coding plugin (`~/.claude/plugins/marketplaces/coding/docs/`).
Read `go-testing-guide.md` from coding plugin.
Read `go-factory-pattern.md` from coding plugin — factory has zero business logic.

Files to read before making any changes:

- `agent/pr-reviewer/pkg/git/clone_url.go` — full file; this is where you add `CloneURLParts` struct + `ParseCloneURLParts`; refactor `ParseCloneURL` to call the new function internally
- `agent/pr-reviewer/pkg/git/clone_url_test.go` — existing tests to understand the test structure; add tests for `ParseCloneURLParts` without removing existing `ParseCloneURL` tests
- `agent/pr-reviewer/pkg/steps_checkout_execution.go` — full file; you add the allowlist check before `EnsureWorktree`; understand the `Run()` method structure
- `agent/pr-reviewer/pkg/steps_checkout_execution_test.go` — existing tests; extend with allowlist cases
- `agent/pr-reviewer/pkg/factory/factory.go` — `CreateAgent` signature; you add `repoAllowlist []string` param and thread it to `NewCheckoutExecutionStep`
- `agent/pr-reviewer/pkg/factory/runner.go` — `RunConfig` struct and `RunAgent` function; add `RepoAllowlist []string` to `RunConfig`
- `agent/pr-reviewer/pkg/factory/factory_test.go` — check if `CreateAgent` is called in tests; update call site if needed
- `agent/pr-reviewer/main.go` — full file; add `REPO_ALLOWLIST` arg, parse at startup, log count, pass to `RunConfig`
- `agent/pr-reviewer/cmd/run-task/main.go` — full file; same changes as main.go

Before writing the `NeedsInput` status constant, verify its exact name:
```bash
grep -rn "AgentStatus" $(go env GOPATH)/pkg/mod/github.com/bborbe/agent@*/lib/*.go 2>/dev/null | head -20
```
Expected to find `AgentStatusNeedsInput` (matching the `"needs_input"` verdict token from CLAUDE.md). Use whatever the grep finds — do NOT guess.

Key facts (verified against the codebase):
- `git.ParseCloneURL` is defined at `agent/pr-reviewer/pkg/git/clone_url.go`; it returns a `string` of form `"host/owner/repo.git"`; it calls the private `splitCloneURL` and `validateCloneURLSegment` helpers which you will keep unchanged
- `checkoutExecutionStep` is in `agent/pr-reviewer/pkg/steps_checkout_execution.go`; it reads `clone_url` from `md.Frontmatter.String("clone_url")` and calls `s.repoManager.EnsureWorktree(ctx, cloneURL, ref, taskID)` after the nil checks
- `factory.CreateAgent` currently has 7 parameters; you add `repoAllowlist []string` as the 8th (at the end)
- `factory.RunConfig` is in `factory/runner.go`; add `RepoAllowlist []string` as a new field
- Both entry points (`main.go` and `cmd/run-task/main.go`) call `factory.RunAgent(ctx, factory.RunConfig{...})`; both must gain the `REPO_ALLOWLIST` arg and wire it into `RunConfig.RepoAllowlist`
- The agent module does NOT share code with the watcher module — the `ParseRepoAllowlist` helper is a local copy inside `agent/pr-reviewer/pkg/` (same regex, same behavior, independent file)
- The regex for allowlist validation is `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$` (host/owner/repo, three segments)
- This prompt touches ONLY `agent/pr-reviewer/`. The watcher (`watcher/github/`) is handled by sibling prompt 1.
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Verify the `AgentStatusNeedsInput` constant name** before writing any code:

   ```bash
   grep -rn "AgentStatus" $(go env GOPATH)/pkg/mod/github.com/bborbe/agent@*/lib/*.go 2>/dev/null | head -20
   ```

   Use the exact constant name returned by this grep. If the grep finds no match, broaden the search:
   ```bash
   grep -rn "NeedsInput\|needs_input" $(go env GOPATH)/pkg/mod/github.com/bborbe/agent@*/... 2>/dev/null | grep -i "const\|Status\|=" | head -20
   ```

   Do NOT proceed past this step without a confirmed constant name. Record the verified name; use it in every subsequent step that returns `NeedsInput`.

2. **Refactor `agent/pr-reviewer/pkg/git/clone_url.go`** to expose `CloneURLParts`.

   Add the `CloneURLParts` struct and `ParseCloneURLParts` function. Then make `ParseCloneURL` delegate to the new function so all existing callers continue to work.

   Replace the file with the refactored version. The key change: `ParseCloneURL` is now a thin wrapper:

   ```go
   // CloneURLParts holds the validated components of a git clone URL.
   type CloneURLParts struct {
       Host  string
       Owner string
       Repo  string
   }

   // ParseCloneURLParts parses a git clone URL into its host, owner, and repo
   // components. Accepts URL-form ("https://host/owner/repo.git") and SCP-form
   // SSH ("user@host:owner/repo.git"). Returns an error for malformed or unsafe
   // inputs.
   func ParseCloneURLParts(ctx context.Context, rawURL string) (*CloneURLParts, error) {
       if rawURL == "" {
           return nil, errors.Errorf(ctx, "clone URL must not be empty")
       }

       host, path, err := splitCloneURL(ctx, rawURL)
       if err != nil {
           return nil, err
       }

       path = strings.TrimPrefix(path, "/")
       path = strings.TrimSuffix(path, ".git")

       segments := strings.Split(path, "/")
       if len(segments) != 2 {
           return nil, errors.Errorf(
               ctx,
               "clone URL path must have exactly 2 segments (<owner>/<repo>), got %d: %s",
               len(segments),
               rawURL,
           )
       }

       for _, seg := range segments {
           if err := validateCloneURLSegment(ctx, seg); err != nil {
               return nil, err
           }
       }

       return &CloneURLParts{Host: host, Owner: segments[0], Repo: segments[1]}, nil
   }

   // ParseCloneURL converts a git clone URL to a relative bare-repo path:
   // "<host>/<owner>/<repo>.git". Accepts URL-form ("https://host/owner/repo.git")
   // and SCP-form SSH ("user@host:owner/repo.git"). Returns an error for malformed
   // or unsafe inputs.
   func ParseCloneURL(ctx context.Context, rawURL string) (string, error) {
       parts, err := ParseCloneURLParts(ctx, rawURL)
       if err != nil {
           return "", err
       }
       return parts.Host + "/" + parts.Owner + "/" + parts.Repo + ".git", nil
   }
   ```

   Keep the existing `cloneURLSegmentRegexp`, `scpURLRegexp`, `splitCloneURL`, and `validateCloneURLSegment` unchanged — they are used by both functions.

3. **Add tests for `ParseCloneURLParts`** in `agent/pr-reviewer/pkg/git/clone_url_test.go`.

   Do NOT remove existing `ParseCloneURL` tests. Append a new `Describe("ParseCloneURLParts", ...)` block covering:
   - HTTPS URL: `https://github.com/bborbe/code-reviewer.git` → `{Host: "github.com", Owner: "bborbe", Repo: "code-reviewer"}`
   - SCP SSH URL: `git@github.com:bborbe/code-reviewer.git` → `{Host: "github.com", Owner: "bborbe", Repo: "code-reviewer"}`
   - Empty string → error
   - Single-segment path → error
   - Invalid segment characters → error

4. **Create `agent/pr-reviewer/pkg/allowlist.go`** — the agent's local copy of the allowlist parse helper (same logic as the watcher, separate file, no shared dependency):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import (
       "context"
       "regexp"
       "strings"

       "github.com/bborbe/errors"
   )

   // repoAllowlistEntryPattern validates a single host-qualified repo entry.
   // Required shape: host/owner/repo (three slash-delimited segments).
   var repoAllowlistEntryPattern = regexp.MustCompile(
       `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`,
   )

   // ParseRepoAllowlist parses a comma-separated allowlist string into a slice
   // of validated host-qualified repo keys ("host/owner/repo").
   //
   // Empty string and unset env var both return (nil, nil) — allow-all.
   // Whitespace-only entries and entries from trailing commas are silently
   // dropped. Any entry that does not match the required shape causes an error.
   func ParseRepoAllowlist(ctx context.Context, raw string) ([]string, error) {
       if raw == "" {
           return nil, nil
       }
       var result []string
       for _, entry := range strings.Split(raw, ",") {
           entry = strings.TrimSpace(entry)
           if entry == "" {
               continue
           }
           if !repoAllowlistEntryPattern.MatchString(entry) {
               return nil, errors.Errorf(
                   ctx,
                   "repo allowlist entry %q does not match required format host/owner/repo (pattern: ^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$)",
                   entry,
               )
           }
           result = append(result, entry)
       }
       return result, nil
   }
   ```

5. **Create `agent/pr-reviewer/pkg/allowlist_test.go`** with Ginkgo v2 + Gomega tests. Use package `pkg_test`. Cover:
   - Empty string → nil, no error
   - Single valid entry → `[]string{"github.com/bborbe/code-reviewer"}`
   - Multiple valid entries → expected slice
   - Whitespace stripping
   - Trailing comma (empty entry) → silently dropped
   - Non-host-qualified entry (two segments: `bborbe/code-reviewer`) → error mentioning the offending entry
   - Single segment → error
   - Four segments → error

6. **Add `repoAllowlist []string` field and allowlist check to `checkoutExecutionStep`** in `agent/pr-reviewer/pkg/steps_checkout_execution.go`.

   Step 6a — add the field to the struct:
   ```go
   type checkoutExecutionStep struct {
       repoManager     git.RepoManager
       claudeConfigDir claudelib.ClaudeConfigDir
       agentDir        claudelib.AgentDir
       model           claudelib.ClaudeModel
       env             map[string]string
       allowedTools    claudelib.AllowedTools
       reviewMode      string
       repoAllowlist   []string   // new
   }
   ```

   Step 6b — add the parameter to `NewCheckoutExecutionStep` (at the end, after `reviewMode`):
   ```go
   func NewCheckoutExecutionStep(
       repoManager git.RepoManager,
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       env map[string]string,
       allowedTools claudelib.AllowedTools,
       reviewMode string,
       repoAllowlist []string,
   ) agentlib.Step {
       return &checkoutExecutionStep{
           repoManager:     repoManager,
           claudeConfigDir: claudeConfigDir,
           agentDir:        agentDir,
           model:           model,
           env:             env,
           allowedTools:    allowedTools,
           reviewMode:      reviewMode,
           repoAllowlist:   repoAllowlist,
       }
   }
   ```

   Step 6c — add the allowlist check in `Run()` **after** the nil-check block and **before** the `s.repoManager.EnsureWorktree(...)` call:

   ```go
   // Allowlist check: must run before EnsureWorktree (cloning is the trust boundary).
   if len(s.repoAllowlist) > 0 {
       parts, parseErr := git.ParseCloneURLParts(ctx, cloneURL)
       if parseErr != nil {
           // Parse failure is a hard error, distinct from the soft allowlist-miss path.
           return &agentlib.Result{
               Status:  agentlib.AgentStatusFailed,
               Message: fmt.Sprintf("execution step: failed to parse clone_url for allowlist check: %v", parseErr),
           }, nil
       }
       repoKey := parts.Host + "/" + parts.Owner + "/" + parts.Repo
       allowed := false
       for _, entry := range s.repoAllowlist {
           if entry == repoKey {
               allowed = true
               break
           }
       }
       if !allowed {
           return &agentlib.Result{
               Status: <AgentStatusNeedsInput>,
               Message: fmt.Sprintf(
                   "execution step: repo %q is not on the allowlist (%d entries); task routed to human review without clone",
                   repoKey,
                   len(s.repoAllowlist),
               ),
           }, nil
       }
   }
   ```

   Replace `<AgentStatusNeedsInput>` with the exact constant name found in step 1.

7. **Update `agent/pr-reviewer/pkg/steps_checkout_execution_test.go`** — add four new test cases to the existing suite:

   - **Allowlist empty → proceeds to EnsureWorktree** (baseline, verifies empty allowlist is allow-all)
   - **Allowlist non-empty, clone_url matches → proceeds to EnsureWorktree** (positive path)
   - **Allowlist non-empty, clone_url does NOT match → returns NeedsInput, EnsureWorktree not called** (negative path — key security invariant)
   - **Allowlist non-empty, clone_url unparseable (e.g. `not-a-url`) → returns Status: Failed (NOT NeedsInput), EnsureWorktree not called** (parse-fail path — preserves existing hard-failure behavior, distinct from soft allowlist-miss; closes the spec's parse-fail-vs-allowlist-miss-distinct-paths invariant)

   Use a task markdown with `clone_url: https://github.com/bborbe/code-reviewer.git` for the positive case, `https://github.com/bborbe/other-repo.git` for the negative case, and `not-a-url` for the parse-fail case.

   Verify: `fakeRepoManager.EnsureWorktreeCallCount()` is 0 in BOTH the negative-path and parse-fail tests.

8. **Update `agent/pr-reviewer/pkg/factory/runner.go`** — add `RepoAllowlist []string` to `RunConfig`:

   ```go
   type RunConfig struct {
       ClaudeConfigDir claudelib.ClaudeConfigDir
       AgentDir        claudelib.AgentDir
       Model           claudelib.ClaudeModel
       GHToken         string
       ReposPath       string
       WorkPath        string
       ReviewMode      string
       RepoAllowlist   []string   // new: host-qualified repos the agent may clone
       Phase           domain.TaskPhase
       TaskContent     string
       Deliverer       agentlib.ResultDeliverer
   }
   ```

   In `RunAgent`, pass `cfg.RepoAllowlist` to `CreateAgent`:
   ```go
   agent := CreateAgent(
       cfg.ClaudeConfigDir,
       cfg.AgentDir,
       cfg.Model,
       cfg.GHToken,
       env,
       repoManager,
       cfg.ReviewMode,
       cfg.RepoAllowlist,   // new
   )
   ```

9. **Update `agent/pr-reviewer/pkg/factory/factory.go`** — add `repoAllowlist []string` parameter to `CreateAgent` (at the end, after `reviewMode`):

   ```go
   func CreateAgent(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       ghToken string,
       env map[string]string,
       repoManager git.RepoManager,
       reviewMode string,
       repoAllowlist []string,
   ) AgentRunner {
   ```

   Inside `CreateAgent`, update the `NewCheckoutExecutionStep` call to pass `repoAllowlist` as the last argument:
   ```go
   executionStep := prpkg.NewCheckoutExecutionStep(
       repoManager,
       claudeConfigDir,
       agentDir,
       model,
       env,
       executionTools,
       reviewMode,
       repoAllowlist,   // new
   )
   ```

   Check `agent/pr-reviewer/pkg/factory/factory_test.go` — if it calls `CreateAgent` directly, update the call site with the new parameter (`nil` or an empty `[]string{}` is fine for tests not covering the allowlist).

10. **Add `REPO_ALLOWLIST` to `agent/pr-reviewer/main.go`**:

    Step 10a — add field to `application` struct (after the `GHToken` field):
    ```go
    RepoAllowlist string `required:"false" arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); empty means allow-all"`
    ```

    Step 10b — parse in `Run()`, before calling `factory.RunAgent`. Use the package alias for `agent/pr-reviewer/pkg`:
    ```go
    repoAllowlist, err := prpkg.ParseRepoAllowlist(ctx, a.RepoAllowlist)
    if err != nil {
        return err
    }
    glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))
    ```

    Check the existing imports — `main.go` does NOT currently import `agent/pr-reviewer/pkg` directly; you will need to add:
    ```go
    prpkg "github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg"
    ```

    Step 10c — wire `repoAllowlist` into `RunConfig`:
    ```go
    result, err := factory.RunAgent(ctx, factory.RunConfig{
        ClaudeConfigDir: a.ClaudeConfigDir,
        AgentDir:        a.AgentDir,
        Model:           a.Model,
        GHToken:         a.GHToken,
        ReposPath:       a.ReposPath,
        WorkPath:        a.WorkPath,
        ReviewMode:      a.ReviewMode,
        RepoAllowlist:   repoAllowlist,   // new
        Phase:           a.Phase,
        TaskContent:     a.TaskContent,
        Deliverer:       deliverer,
    })
    ```

11. **Add `REPO_ALLOWLIST` to `agent/pr-reviewer/cmd/run-task/main.go`** — mirror the same changes as main.go:
    - Add `RepoAllowlist string` field with the same struct tag
    - Import `prpkg "github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg"`
    - Parse and log at startup (same code)
    - Wire into `RunConfig` (same field)

12. **Update `CHANGELOG.md`** — add a bullet to the existing `## Unreleased` section (which sibling prompt 1 already created; if it doesn't exist yet, create it):

    ```markdown
    - feat(pr-reviewer): add `REPO_ALLOWLIST` env var (comma-separated `host/owner/repo` entries) that blocks cloning repos not on the configured list. Non-allowlisted tasks return `NeedsInput` and are routed to human review without cloning. Empty allowlist is allow-all. Extends `git.ParseCloneURL` with a `ParseCloneURLParts` sibling that exposes host/owner/repo as separate fields.
    ```

13. **Run `make precommit`** in `agent/pr-reviewer/`:

    ```bash
    cd agent/pr-reviewer && make precommit
    ```

</requirements>

<constraints>
- Only edit files under `agent/pr-reviewer/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- Do NOT introduce a shared library between watcher and agent modules — `ParseRepoAllowlist` is an agent-local copy (the watcher has its own copy in `watcher/github/pkg/filter/`)
- Do NOT change the Kafka command schema, vault frontmatter, or CQRS structure
- `ParseCloneURL` must continue to return the same string as before (just delegates to `ParseCloneURLParts` now); all existing callers (`git.RepoManager`) continue to work unchanged
- The `clone_url` parse-failure path (`Status: Failed`) MUST be distinct from the allowlist-miss path (`Status: NeedsInput`); do NOT merge these two paths
- Empty `repoAllowlist` (nil or zero-length) MUST skip the allowlist check entirely — `len(s.repoAllowlist) == 0` is the guard condition
- The allowlist check happens BEFORE `EnsureWorktree` — no network call if the repo is not on the list
- `NeedsInput` diagnostic MUST name the parsed `host/owner/repo` key and the configured allowlist size (count only, no entry contents)
- Use `github.com/bborbe/errors` (`errors.Wrapf`, `errors.Errorf`); never `fmt.Errorf`
- The regex for allowlist validation is exactly `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`
- Whitespace entries and empty entries (trailing comma) are silently dropped in `ParseRepoAllowlist`
- Malformed entry in `REPO_ALLOWLIST` causes startup failure (the error propagates from `ParseRepoAllowlist` in `Run()`)
- `make precommit` runs from `agent/pr-reviewer/`, never at repo root
- Existing tests must pass without modification
</constraints>

<verification>
cd agent/pr-reviewer && make precommit

# Confirm CloneURLParts and ParseCloneURLParts:
grep -n "CloneURLParts\|ParseCloneURLParts" agent/pr-reviewer/pkg/git/clone_url.go

# Confirm ParseRepoAllowlist in agent pkg:
ls agent/pr-reviewer/pkg/allowlist.go agent/pr-reviewer/pkg/allowlist_test.go

# Confirm allowlist check in checkout step (NeedsInput must appear):
grep -n "repoAllowlist\|NeedsInput\|allowlist" agent/pr-reviewer/pkg/steps_checkout_execution.go

# Confirm wired through RunConfig:
grep -n "RepoAllowlist" agent/pr-reviewer/pkg/factory/runner.go

# Confirm both entry points have the arg:
grep -n "REPO_ALLOWLIST\|RepoAllowlist\|ParseRepoAllowlist" agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go

# Confirm EnsureWorktree is NOT called in the negative test path:
grep -n "EnsureWorktreeCallCount\|NeedsInput" agent/pr-reviewer/pkg/steps_checkout_execution_test.go

# Confirm CHANGELOG updated:
grep -n "REPO_ALLOWLIST\|pr-reviewer.*allowlist\|allowlist.*pr-reviewer" CHANGELOG.md
</verification>
