---
status: completed
spec: [010-git-checkout-review-workdir]
summary: 'Wired RepoManager into the execution phase: created checkoutExecutionStep that checks out the target ref as an on-disk worktree and runs Claude in that directory; narrowed executionTools to read-only git operations; updated execution_workflow.md to remove gh pr diff; updated CreateAgent to accept git.RepoManager; added REPOS_PATH/WORK_PATH env vars with startup PruneAllWorktrees to both entry points; updated factory_test.go call sites; added tests for ShouldRun and Run error branches'
container: code-reviewer-066-spec-010-execution-wiring
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-29T09:00:00Z"
queued: "2026-04-29T12:10:38Z"
started: "2026-04-29T12:21:23Z"
completed: "2026-04-29T12:27:16Z"
branch: dark-factory/git-checkout-review-workdir
---

<summary>
- The execution phase now runs inside a real on-disk git worktree; `gh pr diff` text-only inspection is removed entirely
- A custom `checkoutExecutionStep` reads `clone_url`, `ref`, and `task_id` from the task frontmatter, calls `RepoManager.EnsureWorktree`, and sets the resulting path as the Claude runner's working directory
- Checkout failure (clone, fetch, or worktree creation) fails loud and returns an error — no fallback path exists
- `factory.CreateAgent` accepts a `git.RepoManager` parameter; callers are responsible for constructing it from `git.WorkdirConfig`
- The execution phase's allowed-tools list is narrowed to read-only git operations plus `Bash(cd:*)` and `Read`/`Grep`/`Glob`; all write-back operations are excluded
- The execution workflow prompt no longer mentions `gh pr diff`; it instructs Claude to read the on-disk worktree and contains an explicit read-only guard
- K8s entry (`main.go`) adds `REPOS_PATH`/`WORK_PATH` env vars (defaults: `/repos`, `/work`), constructs `RepoManager`, and calls `PruneAllWorktrees` at startup before running the task
- `cmd/run-task` entry constructs `RepoManager` with `~/.cache/code-reviewer/repos` / `~/.cache/code-reviewer/work` defaults (shared with CLI mode)
- Planning and ai_review phases are entirely unchanged
- All existing tests continue to pass
</summary>

<objective>
Wire the `RepoManager` (implemented in prompt 1 of this spec) into the execution phase of the agent. The execution phase's Go step ensures the worktree exists, then runs Claude in that directory. Update `execution_workflow.md` to reflect on-disk review. Update entry points to supply `WorkdirConfig` and run startup pruning.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `Create*` prefix, zero logic in factory functions, factory accepts deps as params.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface + constructor + private struct.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `errors.Wrapf`, never `fmt.Errorf`.
Read `docs/architecture.md` — "Agent contract" (frontmatter fields `clone_url`, `ref`, `task_id`) and "Workdir & repo storage" sections.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/git/repo_manager.go` — `RepoManager` interface, `WorkdirConfig` struct (from prompt 1)
- `agent/pr-reviewer/pkg/git/git.go` — existing git package style
- `agent/pr-reviewer/pkg/factory/factory.go` — `CreateAgent`, `executionTools`, `CreateClaudeRunner`
- `agent/pr-reviewer/pkg/steps_review.go` — pattern for a custom step: `Name()`, `ShouldRun()`, `Run()` signature
- `agent/pr-reviewer/pkg/prompts/execution.go` — `BuildExecutionInstructions` and embedded files
- `agent/pr-reviewer/pkg/prompts/execution_workflow.md` — current execution prompt (to replace)
- `agent/pr-reviewer/main.go` — K8s entry point structure
- `agent/pr-reviewer/cmd/run-task/main.go` — run-task entry point structure

**Grep-verify before writing:**
- Frontmatter access on `*agentlib.Markdown`:
  ```bash
  grep -rn "Frontmatter\|GetFrontmatter\|FrontmatterValue\|Get(" \
    $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/...go 2>/dev/null | head -40
  ```
  Find the actual method/function name and signature for reading frontmatter fields like `clone_url`, `ref`, `task_identifier` from the task markdown — use ONLY what grep finds.

- `claudelib.ClaudeRunnerConfig.WorkingDirectory` field type:
  ```bash
  grep -A 10 "type ClaudeRunnerConfig struct" \
    $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/claude/*.go 2>/dev/null
  ```
  Confirm the field name and type (may be `claudelib.AgentDir` or `string`).

- `claudelib.BuildPrompt` signature:
  ```bash
  grep -n "func BuildPrompt" \
    $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/claude/*.go 2>/dev/null
  ```

- `agentlib.AgentStatusFailed`, `agentlib.AgentStatusDone` constants (for `Run` return values):
  ```bash
  grep -n "AgentStatusFailed\|AgentStatusDone" \
    $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/*.go 2>/dev/null | head -20
  ```

- `agentlib.Markdown.ReplaceSection` / `agentlib.Section`:
  ```bash
  grep -n "ReplaceSection\|type Section" \
    $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/*.go 2>/dev/null | head -20
  ```
</context>

<requirements>
**Execute steps in this order. Do NOT run `make precommit` until step 8.**

1. **Create `agent/pr-reviewer/pkg/steps_checkout_execution.go`** (package `pkg`):

   This is a custom `agentlib.Step` that replaces `claudelib.NewAgentStep` for the execution phase. The step:
   - Extracts `clone_url`, `ref`, and `task_id` (field is named `task_identifier` in frontmatter per architecture.md) from the task markdown frontmatter
   - Calls `repoManager.EnsureWorktree(ctx, cloneURL, ref, taskID)` → `worktreePath`
   - Creates a fresh `claudelib.ClaudeRunner` using the verified `WorkingDirectory` field type (from grep above), passing `worktreePath` as the working directory
   - Builds and runs the prompt using `claudelib.BuildPrompt`
   - Writes the result under `## Review` via `md.ReplaceSection`
   - On `EnsureWorktree` error: fail loud — `return nil, errors.Wrapf(ctx, err, "ensure worktree clone_url=%s ref=%s task_id=%s", cloneURL, ref, taskID)` (no fallback)
   - On `runner.Run` error: return `&agentlib.Result{Status: agentlib.AgentStatusFailed, Message: ...}, nil` (same pattern as `reviewStep`)

   Struct:
   ```go
   type checkoutExecutionStep struct {
       repoManager    git.RepoManager
       claudeConfigDir claudelib.ClaudeConfigDir
       agentDir        claudelib.AgentDir
       model           claudelib.ClaudeModel
       env             map[string]string
       allowedTools    claudelib.AllowedTools
       instructions    claudelib.Instructions
   }
   ```

   Constructor:
   ```go
   func NewCheckoutExecutionStep(
       repoManager    git.RepoManager,
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir        claudelib.AgentDir,
       model           claudelib.ClaudeModel,
       env             map[string]string,
       allowedTools    claudelib.AllowedTools,
       instructions    claudelib.Instructions,
   ) agentlib.Step {
       return &checkoutExecutionStep{...}
   }
   ```

   `Name()`: return `"pr-execute"`

   `ShouldRun(_ context.Context, md *agentlib.Markdown) (bool, error)`:
   - Return `false` if `## Review` section already exists (idempotent), otherwise `true`
   - Mirror the `reviewStep.ShouldRun` pattern

   `Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error)`:
   - Extract frontmatter fields (use the method found by grep — do NOT guess)
   - Validate `clone_url` and `ref` are non-empty; if either is missing, return `AgentStatusFailed` with a clear message
   - Call `repoManager.EnsureWorktree` — fail loud on error (propagate as error return, not failed result)
   - **Create runner inline (do NOT call `factory.CreateClaudeRunner` — that would create an import cycle since `factory` imports `pkg`).** Instantiate via `claudelib.NewClaudeRunner(claudelib.ClaudeRunnerConfig{...})` directly, passing `claudelib.AgentDir(worktreePath)` as the working directory. Verify the exact `ClaudeRunnerConfig` field names by grepping the `claudelib` package; mirror the field set used by `factory.CreateClaudeRunner`.
   - Marshal task content: `taskContent, err := md.Marshal(ctx)`
   - Build prompt: `claudelib.BuildPrompt(s.instructions.String(), nil, taskContent)`
   - Run: `runResult, runErr := runner.Run(ctx, prompt)`
   - On `runErr`: return `&agentlib.Result{Status: agentlib.AgentStatusFailed, Message: fmt.Sprintf("execution claude run failed: %v", runErr)}, nil`
   - Write section: `md.ReplaceSection(agentlib.Section{Heading: "## Review", Body: runResult.Result})`
   - Return: `&agentlib.Result{Status: agentlib.AgentStatusDone, NextPhase: "ai_review"}, nil`

2. **Update `executionTools` in `agent/pr-reviewer/pkg/factory/factory.go`**:

   Replace the current broad `"Bash(git:*)"` and `gh pr diff` entries with the narrowed read-only set from spec Desired Behavior #6:
   ```go
   executionTools = claudelib.AllowedTools{
       "Read", "Grep", "Glob",
       "Bash(cd:*)",
       "Bash(git diff:*)",
       "Bash(git log:*)",
       "Bash(git show:*)",
       "Bash(git clone:*)",
       "Bash(git fetch:*)",
       "Bash(git worktree:*)",
   }
   ```
   Excluded (by omission): `git commit`, `git push`, `git checkout -b`, `git branch`, `git reset`, `gh pr diff`, `gh pr view`, `WebFetch`. Note: `Bash(git worktree:*)` is intentionally wide — it permits `git worktree add` (the only sanctioned write, sandboxed to `/work/<task_id>`) plus the read-only `worktree list` / `worktree prune`.

3. **Update `CreateAgent` in `agent/pr-reviewer/pkg/factory/factory.go`**:

   Add `repoManager git.RepoManager` as the last parameter:
   ```go
   func CreateAgent(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir        claudelib.AgentDir,
       model           claudelib.ClaudeModel,
       ghToken         string,
       env             map[string]string,
       repoManager     git.RepoManager,
   ) AgentRunner {
   ```

   Replace the `executionStep` line from:
   ```go
   executionStep := claudelib.NewAgentStep(claudelib.AgentStepConfig{...})
   ```
   to:
   ```go
   executionStep := prpkg.NewCheckoutExecutionStep(
       repoManager,
       claudeConfigDir,
       agentDir,
       model,
       env,
       executionTools,
       prompts.BuildExecutionInstructions(),
   )
   ```

   Planning and reviewStep lines remain **unchanged**.

   Add import for `"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"` if not already present.

3.5. **Update `agent/pr-reviewer/pkg/factory/factory_test.go`**:

   The two `It(...)` blocks in this file call `factory.CreateAgent` with the old 5-argument signature and will fail to compile after step 3. Update both call sites to pass `nil` (typed as `git.RepoManager`) for the new `repoManager` parameter:
   ```go
   var repoManager git.RepoManager // nil — interface check is type-only
   agent := factory.CreateAgent(claudeConfigDir, agentDir, model, env, repoManager)
   ```
   The existing assertions only check that `agent` is non-nil, which still holds. Add the `git` import.

4. **Update `agent/pr-reviewer/main.go`**:

   a. Add two new application fields after `Model`:
   ```go
   ReposPath string `required:"false" arg:"repos-path" env:"REPOS_PATH" usage:"Root path for bare-clone cache" default:"/repos"`
   WorkPath  string `required:"false" arg:"work-path"  env:"WORK_PATH"  usage:"Root path for per-task worktrees" default:"/work"`
   ```

   b. In `Run()`, before the plugin installer block, build the repo manager and run startup prune:
   ```go
   workdirCfg := git.WorkdirConfig{
       ReposPath: a.ReposPath,
       WorkPath:  a.WorkPath,
   }
   repoManager := git.NewRepoManager(workdirCfg)
   if err := repoManager.PruneAllWorktrees(ctx); err != nil {
       glog.Warningf("startup worktree prune: %v", err)
   }
   ```
   (PruneAllWorktrees is best-effort — log warning, do not abort startup on prune failure.)

   c. Pass `repoManager` to `factory.CreateAgent`:
   ```go
   agent := factory.CreateAgent(a.ClaudeConfigDir, a.AgentDir, a.Model, a.GHToken, env, repoManager)
   ```

   Add import `"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"`.

5. **Update `agent/pr-reviewer/cmd/run-task/main.go`**:

   a. Add two new application fields after `Model`:
   ```go
   ReposPath string `required:"false" arg:"repos-path" env:"REPOS_PATH" usage:"Root path for bare-clone cache (default: ~/.cache/code-reviewer/repos)"`
   WorkPath  string `required:"false" arg:"work-path"  env:"WORK_PATH"  usage:"Root path for per-task worktrees (default: ~/.cache/code-reviewer/work)"`
   ```

   b. In `Run()`, before `factory.CreateAgent`, resolve home-dir defaults and build the repo manager:
   ```go
   reposPath := a.ReposPath
   workPath := a.WorkPath
   if reposPath == "" || workPath == "" {
       home, err := os.UserHomeDir()
       if err != nil {
           return errors.Wrap(ctx, err, "resolve user home dir")
       }
       if reposPath == "" {
           reposPath = filepath.Join(home, ".cache", "code-reviewer", "repos")
       }
       if workPath == "" {
           workPath = filepath.Join(home, ".cache", "code-reviewer", "work")
       }
   }
   workdirCfg := git.WorkdirConfig{
       ReposPath: reposPath,
       WorkPath:  workPath,
   }
   repoManager := git.NewRepoManager(workdirCfg)
   ```

   c. Pass `repoManager` to `factory.CreateAgent`:
   ```go
   agent := factory.CreateAgent(a.ClaudeConfigDir, a.AgentDir, a.Model, a.GHToken, env, repoManager)
   ```

   Add imports: `"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"`, `"path/filepath"`, and `"os"` if not already imported (`os.UserHomeDir` is in `os`).

6. **Update `agent/pr-reviewer/pkg/prompts/execution_workflow.md`** — replace the entire file:

   ```markdown
   You are the EXECUTION phase of a 3-phase PR review agent.

   Your job: produce the actual code review by inspecting the on-disk working tree
   prepared for this task. The plan from the previous phase is in the `## Plan` section.

   **READ-ONLY GUARD: This working tree is a temporary checkout of an existing ref.
   Do NOT run `git commit`, `git push`, `git checkout -b`, `git branch`, `git reset`,
   or any other command that modifies history or pushes to a remote. The worktree
   is for reading only.**

   ## Steps

   1. Read `## Plan` — focus areas, files, and pre-flagged concerns.
   2. For each file in `files_changed`, read the file from the working tree using
      the `Read` tool, and inspect the diff using `git diff <base_ref>...HEAD <file>`.
   3. For each concern in `## Plan`, check whether the code is sound:
      - Address it (mention what mitigates the concern), OR
      - Confirm it is a real issue and write a comment for it.
   4. Identify additional issues not flagged in the plan if you find them
      while reading the code.
   5. Choose an overall verdict:
      - `approve` — no critical or major issues
      - `request_changes` — at least one critical or major issue
      - `comment` — only minor / nit comments

   ## Rules

   - Read-only inspection of the working tree. Do NOT post anything to the PR yet —
     posting happens after the ai_review phase verifies your output.
   - Comments must reference real files and real line numbers from the diff.
     If you cannot pin a comment to a line, omit it.
   - Severity calibration:
     - `critical` — bug that breaks functionality, security hole, data loss
     - `major` — incorrect behavior under common conditions, performance
       regression, broken tests
     - `minor` — questionable design, missing tests for edge case
     - `nit` — style, naming, comment phrasing
   - If `## Plan` is missing or unparseable, return `needs_input`.
   - If file reads or git commands fail, return `failed`.
   - Final response MUST be a single JSON object matching `<output-format>`.
   ```

7. **Add a test for `checkoutExecutionStep.ShouldRun`** in `agent/pr-reviewer/pkg/steps_checkout_execution_test.go` (package `pkg_test`):
   - Construct a `checkoutExecutionStep` with a mock `RepoManager` (counterfeiter mock — add `//counterfeiter:generate` directive if mock doesn't exist yet, or reuse existing `mocks.RepoManager` if created by prompt 1)
   - `ShouldRun` with no `## Review` section → returns `true`
   - `ShouldRun` with `## Review` section present → returns `false`
   - `Run()` with empty `clone_url` in frontmatter → returns `Result{Status: AgentStatusFailed, Message: ...}` (no error)
   - `Run()` with empty `ref` in frontmatter → returns `Result{Status: AgentStatusFailed, Message: ...}` (no error)
   - `Run()` with `repoManager.EnsureWorktreeReturns(...)` set to error → returns wrapped error (fail loud, NOT failed result)
   - `Run()` happy path with mocked `RepoManager` and a fake/in-memory runner is acceptable; if mocking the runner is impractical, at minimum exercise the error branches above using the counterfeiter `RepoManager` mock from prompt 1 (`mocks/repo-manager.go`)

8. **Compile and test**:
   ```bash
   cd agent/pr-reviewer && go build ./...
   cd agent/pr-reviewer && make test
   ```
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Do NOT modify `cmd/cli/main.go` — that entry point uses `DockerReviewer` (not the agent pipeline) and is out of scope for this spec (see spec §Desired Behavior #7)
- Do NOT add a diff-based fallback anywhere in the execution step — checkout failure propagates as an error return (no `gh pr diff` fallback)
- Planning and ai_review phases remain unchanged; do not touch `planningStep`, `reviewStep`, or `planningTools`/`reviewTools`
- Error wrapping uses `github.com/bborbe/errors` — never `fmt.Errorf`; `errors.Wrapf(ctx, err, "message")`
- Grep-verify every `agentlib.*` and `claudelib.*` symbol before writing struct literals or calling methods — do not guess field names
- If the frontmatter accessor method is not found by grep, report `status: failed` with the grep output — do NOT invent a method
- `make precommit` runs from `agent/pr-reviewer/`, never from repo root
- Existing tests must pass — `factory_test.go` is updated explicitly in step 3.5 to pass `nil` for the new `repoManager` parameter
</constraints>

<verification>
cd agent/pr-reviewer && go build ./...

cd agent/pr-reviewer && make test

cd agent/pr-reviewer && make precommit
</verification>
