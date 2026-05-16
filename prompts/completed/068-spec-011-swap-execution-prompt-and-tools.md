---
status: completed
spec: [011-swap-execution-to-coding-pr-review]
summary: Replaced hand-rolled execution_workflow.md with plugin-delegation wrapper, wired reviewMode through BuildExecutionInstructions/CreateAgent/both entry points, updated executionTools to match plugin frontmatter, and added sub-agent audit comment.
container: code-reviewer-068-spec-011-swap-execution-prompt-and-tools
dark-factory-version: dev
created: "2026-05-01T10:00:00Z"
queued: "2026-05-01T09:31:59Z"
started: "2026-05-01T09:32:32Z"
completed: "2026-05-01T09:36:40Z"
branch: dark-factory/swap-execution-to-coding-pr-review
---

<summary>
- The hand-rolled execution-phase workflow prompt is replaced with a thin wrapper that delegates to `/coding:pr-review <base_ref> <mode>` and translates its findings into the frozen JSON verdict shape
- Severity translation is deterministic: Must Fix → `request_changes` + `critical`; Should Fix → `major` (escalates to `request_changes` when combined with Must Fix); Nice to Have → `nit` + `comment`; `minor` is reserved for LLM judgment only
- An empty-diff pre-flight check short-circuits before invoking the slash command and emits `approve` immediately
- Plugin-missing and malformed-report paths emit `comment` with diagnostic summaries; worktree-cleanup failures are logged as warnings and do not affect verdict emission
- The execution-phase `AllowedTools` is updated to exactly match the plugin command's declared tools (`Task`, `Bash(git diff:*)`, `Bash(git log:*)`, `Bash(git status:*)`, `Bash(git ls-files:*)`, `Bash(git fetch:*)`, `Bash(git worktree:*)`, `Bash(git branch:*)`, `Bash(rm -rf:*)`); unused tools (`Read`, `Grep`, `Glob`, `Bash(cd:*)`, `Bash(git show:*)`, `Bash(git clone:*)`) are removed
- A `-review-mode` CLI flag and `REVIEW_MODE` env var (values: `short | standard | full`, default `standard`) are added to both entry points and wired through the factory into the wrapper prompt
- A sub-agent allowlist audit comment is recorded in `factory.go` confirming each `coding:*` sub-agent's declared tools are read-only
- All existing tests continue to pass; `make precommit` is green
</summary>

<objective>
Replace the execution-phase hand-rolled workflow prompt with a plugin-delegation wrapper that invokes `/coding:pr-review <base_ref> <mode>` inside the prepared worktree and translates the consolidated findings report to the unchanged JSON verdict schema. Wire a deployment-scoped `ReviewMode` flag through both entry points, the factory, and the prompt builder.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `Create*` prefix, zero logic in factory functions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface + constructor + private struct.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `errors.Wrapf`, never `fmt.Errorf`.
Read `docs/architecture.md` — "Agent contract" section for `base_ref` field in task frontmatter.
Read `~/.claude/plugins/marketplaces/coding/commands/pr-review.md` — plugin invocation format, `argument-hint`, and `allowed-tools` frontmatter (the authoritative tool list).

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/factory/factory.go` — `executionTools` var, `CreateAgent` signature
- `agent/pr-reviewer/pkg/factory/factory_test.go` — `CreateAgent` call sites to update
- `agent/pr-reviewer/pkg/prompts/execution.go` — `BuildExecutionInstructions()` signature
- `agent/pr-reviewer/pkg/prompts/execution_workflow.md` — current content (to replace)
- `agent/pr-reviewer/pkg/prompts/execution_output-format.md` — frozen, do NOT change
- `agent/pr-reviewer/main.go` — application struct pattern for new fields
- `agent/pr-reviewer/cmd/run-task/main.go` — application struct pattern for new fields
- `CHANGELOG.md` — check for existing `## Unreleased` section before writing

The plugin command's `allowed-tools` frontmatter (verified from file above) is:
```
allowed-tools: Task, Bash(git diff:+), Bash(git log:+), Bash(git status:+), Bash(git ls-files:+), Bash(git fetch:+), Bash(git worktree:+), Bash(git branch:+), Bash(rm -rf:+)
```
The agent convention uses `*` where the plugin uses `+` — map 1-to-1 (e.g. `Bash(git diff:*)`).

Sub-agent allowlist audit (do this before writing the comment in step 6):
```bash
grep -r "allowed-tools\|tools:" \
  ~/.claude/plugins/marketplaces/coding/agents/ 2>/dev/null | head -60
```
Confirm each `coding:*` sub-agent declares only read-only tools (Read, Grep, Glob, restricted Bash for analysis — no Write, Edit, curl, wget, nc).
</context>

<requirements>
**Execute steps in this order. Run `make precommit` only in step 9.**

1. **Replace `agent/pr-reviewer/pkg/prompts/execution_workflow.md`** with the following exact content:

   ```markdown
   You are the EXECUTION phase of a 3-phase PR review agent.

   Your job: delegate the code review to the `/coding:pr-review` plugin command and
   translate its consolidated findings into the verdict JSON.

   **READ-ONLY GUARD**: This working tree is a temporary on-disk checkout.
   Do NOT run `git commit`, `git push`, `git reset`, `git checkout -b`, or any
   command that modifies git history or pushes to a remote.
   You may run `git fetch`, `git worktree`, `git diff`, `git log`, `git status`,
   `git ls-files`, and `git branch` as required by the plugin command.

   ## Step 1 — Read task context

   From the task content, identify:
   - `base_ref` — the target branch to diff against (e.g. `master`, `main`)
   - `## Plan` — focus areas, files, and concerns from the planning phase

   If `base_ref` is empty or missing, return
   `{"verdict":"failed","summary":"base_ref is missing from task frontmatter","comments":[],"concerns_addressed":[]}`.

   If `## Plan` is missing or unparseable, return `needs_input`.

   ## Step 2 — Empty-diff short-circuit

   Run:
   ```bash
   git diff origin/<base_ref>...HEAD --name-only
   ```

   If the output is empty (no changed files), emit this verdict immediately and stop:
   ```json
   {"verdict":"approve","summary":"no changes to review","comments":[],"concerns_addressed":[]}
   ```

   ## Step 3 — Invoke the plugin

   Run the PR-review slash command from within this working directory:

   ```
   /coding:pr-review <base_ref> {{REVIEW_MODE}}
   ```

   Where `<base_ref>` is the value from step 1.

   The command will:
   - Create a temporary worktree in `/tmp/`
   - Run automated checks (`make precommit`, LICENSE detection)
   - Fan out to specialist sub-agents (go-quality, go-security, go-factory-pattern,
     go-test-coverage, etc.) with each sub-agent using its own declared tools
   - Produce a consolidated findings report with Must Fix / Should Fix / Nice to Have
   - Clean up the temporary worktree

   Wait for the command to complete fully before proceeding to step 4.

   ## Step 4 — Handle failure modes

   **Plugin not registered** (slash command `/coding:pr-review` is not found):
   Emit:
   ```json
   {"verdict":"comment","summary":"Plugin /coding:pr-review is not registered. Escalating to human review. Verify plugin install at pod startup.","comments":[],"concerns_addressed":[]}
   ```

   **Malformed report** (plugin output missing all three Must Fix / Should Fix / Nice to Have
   section headers):
   Emit:
   ```json
   {"verdict":"comment","summary":"Plugin produced a malformed report (required section headers missing). Raw output (first 500 chars): <truncated>","comments":[],"concerns_addressed":[]}
   ```

   **`make precommit` fails inside the worktree** (reported by the plugin):
   Record as a `critical`-severity finding. Raise verdict to `request_changes`.

   **Sub-agent partial failure** (one agent errors or times out):
   Include findings from agents that completed. Note the missing agent(s) in `summary`.

   **Worktree cleanup failure** (plugin logs a cleanup warning):
   Log the warning; do not let cleanup failure affect verdict emission.

   ## Step 5 — Translate findings

   The plugin produces a consolidated report with three sections (always present,
   may be "None."):

   ```
   ## Must Fix
   ...
   ## Should Fix
   ...
   ## Nice to Have
   ...
   ```

   **Deterministic severity map:**
   - Must Fix finding → comment `severity: "critical"`
   - Should Fix finding → comment `severity: "major"`
   - Nice to Have finding → comment `severity: "nit"`
   - `minor` is reserved for LLM judgment on findings that do not fit any plugin
     bucket; the map above never emits `minor` for plugin-bucketed findings

   **Verdict roll-up:**
   - Any Must Fix finding present → `verdict: "request_changes"`
   - No Must Fix but any Should Fix or Nice to Have → `verdict: "comment"`
   - All sections empty (or all "None.") → `verdict: "approve"`

   **Per-comment rules:**
   - Pin each finding to a real `file` and `line` drawn from the plugin report
   - If a finding has no file/line coordinates, fold it into `summary` rather than
     emitting an un-pinned comment
   - Preserve the plugin's exact bucket label verbatim in `message` for traceability
     (e.g. "[Must Fix] missing error handling in pkg/foo.go")

   ## Step 6 — concerns_addressed

   For each concern raised in `## Plan`:
   - If the code addresses it cleanly → note "addressed by code at <file>:<line>"
   - If a comment was raised for it → note "raised as comment at <file>:<line>"
   - List every concern regardless of outcome

   ## Output

   Final response MUST be a single JSON object matching `<output-format>`.
   ```

2. **Update `agent/pr-reviewer/pkg/prompts/execution.go`**:

   Change `BuildExecutionInstructions` to accept a `reviewMode string` parameter and
   inject it into the workflow text:

   ```go
   import (
       _ "embed"
       "strings"

       claudelib "github.com/bborbe/agent/lib/claude"
   )

   // BuildExecutionInstructions assembles the execution-phase prompt from embedded
   // modules, injecting the configured review mode into the slash-command invocation.
   func BuildExecutionInstructions(reviewMode string) claudelib.Instructions {
       workflow := strings.ReplaceAll(executionWorkflow, "{{REVIEW_MODE}}", reviewMode)
       return claudelib.Instructions{
           {Name: "workflow", Content: workflow},
           {Name: "output-format", Content: executionOutputFormat},
       }
   }
   ```

   Do NOT change `execution_output-format.md` or the embed directives.

3. **Update `executionTools` in `agent/pr-reviewer/pkg/factory/factory.go`**:

   Replace the current `executionTools` var with the exact set from the plugin's
   `allowed-tools` frontmatter (using `*` instead of `+`):

   ```go
   executionTools = claudelib.AllowedTools{
       "Task",
       "Bash(git diff:*)",
       "Bash(git log:*)",
       "Bash(git status:*)",
       "Bash(git ls-files:*)",
       "Bash(git fetch:*)",
       "Bash(git worktree:*)",
       "Bash(git branch:*)",
       "Bash(rm -rf:*)",
   }
   ```

   Removed tools (not in plugin frontmatter, now unused): `"Read"`, `"Grep"`, `"Glob"`,
   `"Bash(cd:*)"`, `"Bash(git show:*)"`, `"Bash(git clone:*)"`.

4. **Add `reviewMode string` parameter to `CreateAgent` in `factory.go`**:

   New signature:
   ```go
   func CreateAgent(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir        claudelib.AgentDir,
       model           claudelib.ClaudeModel,
       ghToken         string,
       env             map[string]string,
       repoManager     git.RepoManager,
       reviewMode      string,
   ) AgentRunner {
   ```

   Inside the body, update the `prompts.BuildExecutionInstructions()` call to pass the mode:
   ```go
   prompts.BuildExecutionInstructions(reviewMode),
   ```

5. **Add sub-agent allowlist audit comment in `factory.go`**:

   Directly above the `executionTools` var declaration, add:
   ```go
   // Sub-agent allowlist audit (spec-011): each coding:* sub-agent dispatched by
   // /coding:pr-review declares read-only tools only (Read, Grep, Glob, restricted
   // Bash for analysis). No Write, Edit, curl, wget, or network-exfil primitives.
   // Verified by inspecting ~/.claude/plugins/marketplaces/coding/agents/*.md.
   ```

6. **Add `ReviewMode` field to `agent/pr-reviewer/main.go`**:

   Add after the `WorkPath` field in the application struct:
   ```go
   // Review depth passed to /coding:pr-review (short | standard | full)
   ReviewMode string `required:"false" arg:"review-mode" env:"REVIEW_MODE" usage:"Review depth: short | standard | full" default:"standard"`
   ```

   Update the `factory.CreateAgent` call to pass `a.ReviewMode` as the last argument:
   ```go
   agent := factory.CreateAgent(
       a.ClaudeConfigDir,
       a.AgentDir,
       a.Model,
       a.GHToken,
       env,
       repoManager,
       a.ReviewMode,
   )
   ```

7. **Add `ReviewMode` field to `agent/pr-reviewer/cmd/run-task/main.go`**:

   Add after the `WorkPath` field in the application struct:
   ```go
   // Review depth passed to /coding:pr-review (short | standard | full)
   ReviewMode string `required:"false" arg:"review-mode" env:"REVIEW_MODE" usage:"Review depth: short | standard | full" default:"standard"`
   ```

   Update the `factory.CreateAgent` call to pass `a.ReviewMode` as the last argument:
   ```go
   agent := factory.CreateAgent(
       a.ClaudeConfigDir,
       a.AgentDir,
       a.Model,
       a.GHToken,
       env,
       repoManager,
       a.ReviewMode,
   )
   ```

8. **Update `agent/pr-reviewer/pkg/factory/factory_test.go`**:

   Both `It(...)` blocks call `factory.CreateAgent` with 6 args. Add `"standard"` as the
   seventh argument to each call site:
   ```go
   agent := factory.CreateAgent(
       "",
       "agent",
       "sonnet",
       "",
       map[string]string{},
       repoManager,
       "standard",
   )
   ```
   Do the same for the second `It(...)` block (token set in env).

9. **Update `CHANGELOG.md`**:

   Check whether `## Unreleased` already exists. If not, add it after the first
   `# Changelog` lines, before `## v0.19.1`. Add this entry:
   ```markdown
   ## Unreleased

   - feat: replace hand-rolled execution-phase prompt with `/coding:pr-review` plugin delegation; add `-review-mode` flag (short|standard|full, default standard); update `executionTools` to match plugin's declared tool requirements
   ```

10. **Compile and run tests**:
    ```bash
    cd agent/pr-reviewer && go build ./...
    cd agent/pr-reviewer && make test
    ```

11. **Run final validation**:
    ```bash
    cd agent/pr-reviewer && make precommit
    ```
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- Do NOT modify `execution_output-format.md` — the verdict schema is frozen
- Do NOT modify `steps_checkout_execution.go` — its constructor signature (`instructions claudelib.Instructions`) is unchanged; only the instructions content changes via `BuildExecutionInstructions`
- Do NOT modify planning or ai_review prompts, tools, or steps
- Do NOT modify `cmd/cli/main.go` — it uses `DockerReviewer`, not the agent pipeline
- Error wrapping uses `github.com/bborbe/errors` — never `fmt.Errorf`
- The `{{REVIEW_MODE}}` placeholder in `execution_workflow.md` must be substituted via `strings.ReplaceAll` in `BuildExecutionInstructions`, not at runtime in `checkoutExecutionStep.Run()`
- The `Bash(rm -rf:*)` pattern is deliberately broad (not narrowed to `/tmp/pr-review-*`) — this matches the plugin frontmatter exactly and is intentional per spec security section
- `make precommit` runs from `agent/pr-reviewer/`, never from repo root
- Existing tests (including `steps_checkout_execution_test.go`) must still pass without modification — the step's constructor signature is unchanged
</constraints>

<verification>
cd agent/pr-reviewer && go build ./...

cd agent/pr-reviewer && make test

cd agent/pr-reviewer && make precommit

# Confirm the placeholder is injected:
grep "REVIEW_MODE" agent/pr-reviewer/pkg/prompts/execution_workflow.md
grep "REVIEW_MODE" agent/pr-reviewer/pkg/prompts/execution.go

# Confirm executionTools updated:
grep -A 12 "executionTools" agent/pr-reviewer/pkg/factory/factory.go

# Confirm ReviewMode field in both entry points:
grep "ReviewMode\|review-mode\|REVIEW_MODE" agent/pr-reviewer/main.go
grep "ReviewMode\|review-mode\|REVIEW_MODE" agent/pr-reviewer/cmd/run-task/main.go
</verification>
