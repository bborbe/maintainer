---
status: completed
spec: [011-swap-execution-to-coding-pr-review]
summary: Inlined /coding:pr-review plugin content into the execution-phase prompt by reading the plugin file at runtime in checkoutExecutionStep.Run, stripping YAML frontmatter, prepending a pre-filled-arguments header, and appending a verdict-translation footer — so plugin orchestration runs as native instructions instead of being described as a prose slash command.
container: code-reviewer-071-fix-inline-coding-pr-review-into-execution-prompt
dark-factory-version: dev
created: "2026-05-02T09:00:00Z"
queued: "2026-05-02T09:02:03Z"
started: "2026-05-02T09:03:31Z"
completed: "2026-05-02T09:12:58Z"
---
<summary>
- Slash commands embedded inside structured prompts are read as documentation, not invoked. v0.21.0 wired `/coding:pr-review` into the execution wrapper but it never actually fires; Claude just runs ad-hoc `git diff` and writes a generic verdict.
- Fix is to inline the plugin command body into the execution-phase prompt so the plugin's procedure runs as native instructions in the same Claude session — no recursion, no slash dispatch.
- The plugin file is read at agent runtime (not build time) so plugin updates flow through without a rebuild.
- YAML frontmatter is stripped; pre-filled argument header replaces `<target-branch>` and mode placeholders so the agent never prompts for them.
- A short verdict-translation block is appended after the inlined body, mapping plugin bucket labels to the agent's frozen verdict schema.
- Verdict logs will start showing real sub-agent names (e.g. `go-quality-assistant`, `go-security-specialist`) once the plugin actually fires, proving the fix works end-to-end.
- The execution-phase tool allowlist already matches the plugin's `allowed-tools` (verified for drift); comment audit is updated to reflect the new wiring.
- `base_ref` injection moves from factory build-time to per-task runtime in the execution step, so each task gets its real target branch pre-filled at the top of the prompt.
- The old `execution_workflow.md` wrapper is removed; one source of truth is the inlined plugin content + the verdict translation block.
</summary>

<objective>
Make `/coding:pr-review` plugin orchestration actually run during the execution phase by inlining the plugin command body into the prompt sent to Claude, instead of describing the slash command in prose. End state: when the agent runs, it dispatches the plugin's specialist sub-agents via `Task`, produces a real Must Fix / Should Fix / Nice to Have report, then translates that report into the frozen verdict JSON schema — all in a single non-recursive Claude session.
</objective>

<context>
Read these before changing anything:
- `CLAUDE.md` (repo root) — project conventions, dark-factory rules, build commands.
- `agent/pr-reviewer/pkg/prompts/execution.go` — current `BuildExecutionInstructions(reviewMode string)`; will gain `claudeConfigDir` parameter and become fallible.
- `agent/pr-reviewer/pkg/prompts/execution_workflow.md` — current wrapper text (Steps 1–6 describing the slash command). To be deleted.
- `agent/pr-reviewer/pkg/prompts/execution_output-format.md` — frozen verdict schema. Must NOT be modified.
- `agent/pr-reviewer/pkg/factory/factory.go` — `executionTools` allowlist + `CreateAgent` signature; the comment block above `executionTools` must be updated.
- `agent/pr-reviewer/pkg/factory/factory_test.go` — `CreateAgent` test sites; signature change requires updates.
- `agent/pr-reviewer/pkg/steps_checkout_execution.go` — `checkoutExecutionStep`. **This is where `base_ref` is read from the per-task frontmatter at runtime.** The execution prompt must be assembled per-task here, not at factory init time.
- `agent/pr-reviewer/pkg/steps_checkout_execution_test.go` — existing tests for the step; must keep passing and gain a new test for missing `base_ref`.
- `agent/pr-reviewer/main.go` and `agent/pr-reviewer/cmd/run-task/main.go` — entry points calling `factory.CreateAgent`. Both already plumb `claudeConfigDir`; signature change there is a single-line edit each.
- `~/.claude/plugins/marketplaces/coding/commands/pr-review.md` — the plugin file inlined at runtime. **Read it for understanding the layout (frontmatter + body); do NOT modify it.**

Reference docs (do not inline content already covered):
- `~/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — use `github.com/bborbe/errors` for all error wrapping.
- `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2/Gomega test conventions used in this repo.
- `~/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — factory composition rules; factories stay pure (no I/O), so file reads must move to the step.
- `~/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` section format.

Key precondition (verified): `base_ref` is in the task markdown frontmatter and is read inside `checkoutExecutionStep.Run` from `md.Frontmatter`. The factory builds the step once; the step runs many times. Conclusion: building the prompt with a baked-in `base_ref` at factory time is wrong — the step must read `base_ref` from frontmatter and assemble the prompt per-Run call.
</context>

<requirements>

1. **Move prompt assembly from factory time to step run time.**

   In `agent/pr-reviewer/pkg/prompts/execution.go`, change the function signature from:
   ```go
   func BuildExecutionInstructions(reviewMode string) claudelib.Instructions
   ```
   to:
   ```go
   func BuildExecutionInstructions(
       ctx context.Context,
       claudeConfigDir claudelib.ClaudeConfigDir,
       reviewMode string,
       baseRef string,
   ) (claudelib.Instructions, error)
   ```

   Implementation steps inside `BuildExecutionInstructions`:

   a. Validate inputs early. If `baseRef == ""`, return `nil, errors.New(ctx, "base_ref is empty")`. If `reviewMode == ""`, return `nil, errors.New(ctx, "reviewMode is empty")`.

   b. Compute the plugin path:
      ```go
      pluginPath := filepath.Join(string(claudeConfigDir), "plugins", "marketplaces", "coding", "commands", "pr-review.md")
      ```
      Use `path/filepath` (not `path`).

   c. Read the file:
      ```go
      raw, err := os.ReadFile(pluginPath)
      if err != nil {
          return nil, errors.Wrapf(ctx, err, "read plugin command file path=%s", pluginPath)
      }
      ```
      A read failure is the "plugin missing" failure mode from spec-011 — propagate as a wrapped error so the step can surface it as a phase failure with a diagnostic.

   d. Strip YAML frontmatter. The plugin file starts with `---\n...\n---\n`. Implement a small helper:
      ```go
      // stripFrontmatter removes a leading YAML frontmatter block delimited by
      // "---\n" ... "\n---\n". If no leading frontmatter is present, the input
      // is returned unchanged.
      func stripFrontmatter(s string) string {
          const delim = "---\n"
          if !strings.HasPrefix(s, delim) {
              return s
          }
          rest := s[len(delim):]
          end := strings.Index(rest, "\n"+delim)
          if end < 0 {
              return s
          }
          return rest[end+len("\n"+delim):]
      }
      ```

   e. Build the pre-filled-arguments header (verbatim, with `%s` placeholders for `baseRef` and `reviewMode`):
      ```
      ## Pre-filled arguments

      The procedure below describes a `/coding:pr-review` slash command that takes
      `<target-branch>` and a mode argument. Those arguments have already been
      resolved for this run — do NOT prompt for them, do NOT re-derive them:

      - **TARGET_BRANCH**: %s
      - **mode**: %s

      Now follow the procedure below as if the slash command had been invoked with
      those arguments. The procedure references sub-agents via the `Task` tool;
      dispatch them as written.

      ---

      ```
      Use `fmt.Sprintf` to substitute `baseRef` and `reviewMode`.

   f. Build the verdict-translation footer (verbatim, no substitutions):
      ```
      ---

      ## Final step — emit verdict JSON

      After Step 7 (Manual Review) completes and the consolidated report is
      produced, ALSO emit a JSON verdict matching the agent's frozen schema (see
      `<output-format>`).

      Severity map (deterministic):
      - Must Fix finding → comment severity "critical", contributes to verdict "request_changes"
      - Should Fix finding → comment severity "major"
      - Nice to Have finding → comment severity "nit"
      - The severity "minor" is reserved for LLM judgment on findings that
        genuinely don't fit a plugin bucket; the deterministic map never emits it.

      Verdict roll-up:
      - Any Must Fix present → verdict "request_changes"
      - Else any Should Fix or Nice to Have present → verdict "comment"
      - All sections empty (or "None.") → verdict "approve"

      Each comment must pin to a real `file` and `line` from the report. If a
      finding has no coordinates, fold it into `summary` instead of emitting an
      un-pinned comment. Preserve the plugin's bucket label verbatim in the
      comment `message` for traceability.
      ```

   g. Assemble: `header + stripFrontmatter(string(raw)) + footer`. Return:
      ```go
      return claudelib.Instructions{
          {Name: "workflow", Content: assembled},
          {Name: "output-format", Content: executionOutputFormat},
      }, nil
      ```

   h. Remove the existing `//go:embed execution_workflow.md` line and the `executionWorkflow` variable. The `executionOutputFormat` embed stays.

2. **Delete `agent/pr-reviewer/pkg/prompts/execution_workflow.md`.** No longer used; one source of truth is the plugin file.

3. **Update `checkoutExecutionStep` to assemble the prompt per Run.**

   In `agent/pr-reviewer/pkg/steps_checkout_execution.go`:

   a. Change the struct: replace the field `instructions claudelib.Instructions` with two new fields:
      ```go
      claudeConfigDir claudelib.ClaudeConfigDir // already exists — keep
      reviewMode      string                    // NEW
      ```
      Note: `claudeConfigDir` is already a struct field; do not duplicate.

   b. Change `NewCheckoutExecutionStep` signature: remove the `instructions claudelib.Instructions` parameter and add `reviewMode string` in its place. Update the constructor body.

   c. Inside `Run`, after the existing frontmatter reads for `clone_url`, `ref`, `task_identifier`, also read `base_ref`:
      ```go
      baseRef, _ := md.Frontmatter.String("base_ref")
      if baseRef == "" {
          return &agentlib.Result{
              Status:  agentlib.AgentStatusFailed,
              Message: "execution step: base_ref is missing from task frontmatter",
          }, nil
      }
      ```
      This must come before the `EnsureWorktree` call so a missing `base_ref` fails fast without doing IO.

   d. After the `EnsureWorktree` call succeeds and before `BuildPrompt`, build the instructions:
      ```go
      instructions, err := prompts.BuildExecutionInstructions(ctx, s.claudeConfigDir, s.reviewMode, baseRef)
      if err != nil {
          return nil, errors.Wrapf(ctx, err, "build execution instructions base_ref=%s mode=%s", baseRef, s.reviewMode)
      }
      ```
      Use the import alias `"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/prompts"`.

   e. Replace `s.instructions.String()` with `instructions.String()` in the `BuildPrompt` call.

4. **Update `factory.CreateAgent`** in `agent/pr-reviewer/pkg/factory/factory.go`:

   a. Drop the call to `prompts.BuildExecutionInstructions(reviewMode)` at line 165 — assembly moved to the step.

   b. Update `NewCheckoutExecutionStep` call to pass `reviewMode` instead of pre-built instructions:
      ```go
      executionStep := prpkg.NewCheckoutExecutionStep(
          repoManager,
          claudeConfigDir,
          agentDir,
          model,
          env,
          executionTools,
          reviewMode,
      )
      ```

   c. **Do not change the public `CreateAgent` signature.** `reviewMode` is already a parameter; `claudeConfigDir` is already a parameter. No new parameter needed at the factory boundary because the step does the runtime assembly. (This corrects the plan in the calling brief — adding `baseRef` to `CreateAgent` would be the wrong level since `baseRef` is per-task, not per-agent.)

   d. Update the comment block above `executionTools` (currently lines 52–55) to read:
      ```go
      // Sub-agent allowlist audit (spec-011 + inline-plugin fix): the inlined
      // /coding:pr-review content (assembled per-task by the execution step)
      // dispatches specialist sub-agents via Task. Each coding:* sub-agent
      // declares its own read-only tools (Read, Grep, Glob, restricted Bash;
      // no Write, Edit, curl, wget, nc). Verified by inspecting plugin agent
      // definitions under
      // ~/.claude/plugins/marketplaces/coding/agents/*.md. The tool set below
      // mirrors the plugin's `allowed-tools` frontmatter so the inlined body
      // has the same capabilities as a real /coding:pr-review invocation.
      ```

   e. Verify `executionTools` matches the plugin's `allowed-tools` frontmatter (spec calls these out: `Task`, `Bash(git diff:*)`, `Bash(git log:*)`, `Bash(git status:*)`, `Bash(git ls-files:*)`, `Bash(git fetch:*)`, `Bash(git worktree:*)`, `Bash(git branch:*)`, `Bash(rm -rf:*)`). If drift is present, fix it. If no drift, leave the tool list unchanged.

5. **Update `factory_test.go`.** The existing two `CreateAgent` `It` blocks already pass `reviewMode` — they pass `"standard"`. No signature change at the factory level (per requirement 4c), so these tests should continue to pass without edits. Verify by running tests; if anything broke, fix the call sites to pass the same arguments in the same order.

6. **Update `steps_checkout_execution_test.go`.**

   a. The `BeforeEach` constructor call passes `claudelib.Instructions{}` as the last argument. Replace that with the new `reviewMode` string parameter (use `"standard"`):
      ```go
      step = pkg.NewCheckoutExecutionStep(
          repoManager,
          "",                       // claudeConfigDir (unused — tests fail before plugin read)
          "agent",
          "sonnet",
          map[string]string{},
          claudelib.AllowedTools{"Read"},
          "standard",
      )
      ```

   b. Add a new `Context` block under `Describe("Run")` for the new failure mode:
      ```go
      Context("when base_ref is missing from frontmatter", func() {
          It("returns AgentStatusFailed without propagating error", func() {
              md, err := agentlib.ParseMarkdown(
                  ctx,
                  "---\nclone_url: https://github.com/example/repo.git\nref: main\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
              )
              Expect(err).NotTo(HaveOccurred())
              result, err := step.Run(ctx, md)
              Expect(err).NotTo(HaveOccurred())
              Expect(result).NotTo(BeNil())
              Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
              Expect(result.Message).To(ContainSubstring("base_ref"))
          })
      })
      ```
      This test must run before any test that would hit the plugin-read path — `base_ref` is checked first.

   c. Existing tests for missing `clone_url`, missing `ref`, and `EnsureWorktree` errors must keep passing. They all exit before `BuildExecutionInstructions` is called, so they don't need a real plugin file.

7. **Add a unit test for `BuildExecutionInstructions`** at `agent/pr-reviewer/pkg/prompts/execution_test.go` (new file, package `prompts_test`, Ginkgo style consistent with other `*_test.go` in this repo — see `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`):

   a. Set up a temp dir with a fake plugin file at `<tmpdir>/plugins/marketplaces/coding/commands/pr-review.md`. Use Ginkgo `BeforeEach`/`AfterEach` and `os.MkdirTemp` / `os.RemoveAll`.

   b. **Happy path test.** Write a fake plugin file body:
      ```
      ---
      description: Test plugin
      allowed-tools: Task
      ---
      # PR Review

      Procedure body line 1.
      Procedure body line 2.
      ```
      Call `BuildExecutionInstructions(ctx, claudelib.ClaudeConfigDir(tmpdir), "standard", "main")`. Assert:
      - No error.
      - Returned `Instructions` has 2 entries: `workflow` and `output-format`.
      - `workflow` content contains `"TARGET_BRANCH**: main"` (pre-filled args block).
      - `workflow` content contains `"mode**: standard"`.
      - `workflow` content contains `"Procedure body line 1."` (inlined body).
      - `workflow` content does NOT contain `"description: Test plugin"` (frontmatter stripped).
      - `workflow` content contains `"Final step — emit verdict JSON"` (footer appended).
      - `workflow` content contains `"Severity map"` and `"Verdict roll-up"` (footer detail).

   c. **Frontmatter-stripping test.** Use a fake plugin file that has frontmatter; assert the YAML keys (`description:`, `allowed-tools:`) are absent from the assembled output and the body content is present.

   d. **No-frontmatter test.** Plugin file body without leading `---`; assert assembled content contains the body unchanged.

   e. **Plugin missing test.** Pass a `claudeConfigDir` whose plugin path does not exist. Assert `BuildExecutionInstructions` returns a non-nil error and that the error message contains `"read plugin command file"` (the wrap prefix).

   f. **Empty `baseRef` test.** Assert returns error containing `"base_ref"`.

   g. **Empty `reviewMode` test.** Assert returns error containing `"reviewMode"` or `"mode"`.

8. **Update `CHANGELOG.md`.** Read the existing file first. Add a new `## Unreleased` section above the most recent versioned entry, or append to it if one already exists. Entry text:
   ```
   ### Fixed
   - **pr-reviewer**: inline `/coding:pr-review` plugin content into the execution-phase
     prompt so plugin orchestration actually fires. Previously the wrapper described the
     slash command in prose, but Claude reads it as documentation and never invokes the
     plugin — slash commands don't trigger from inside a multi-section structured prompt.
     The plugin file is now read at runtime, frontmatter stripped, and arguments pre-filled
     before being concatenated with a verdict-translation footer.
   ```
   Follow the format used elsewhere in `CHANGELOG.md` (read it first to match conventions).

</requirements>

<constraints>
- Only edit files under `agent/pr-reviewer/` and `CHANGELOG.md`. No changes outside this scope.
- Do NOT modify `~/.claude/plugins/marketplaces/coding/commands/pr-review.md` or any other plugin file.
- Do NOT modify `agent/pr-reviewer/pkg/prompts/execution_output-format.md` — verdict schema is frozen.
- Do NOT remove the `-review-mode` flag or the `REVIEW_MODE` env var (added in v0.21.0).
- Do NOT add a recursive `claude` invocation. Plugin content runs as native instructions in the same Claude session.
- Use `github.com/bborbe/errors` for all error wrapping. NEVER `fmt.Errorf` for error construction (see `go-error-wrapping-guide.md`). `fmt.Sprintf` for string formatting (e.g., the pre-filled-args header) is fine.
- Build/test from `agent/pr-reviewer/` only. Never run `make` at repo root.
- Existing tests must keep passing.
- Factory functions stay pure — no I/O. The plugin file read happens inside the step's `Run`, not in the factory.
- Do NOT commit — dark-factory handles git.
- Follow Ginkgo v2/Gomega test conventions; counterfeiter mocks live in `mocks/`.
</constraints>

<verification>
Run from `agent/pr-reviewer/`:
```bash
cd agent/pr-reviewer && make precommit
```
Must pass cleanly (format + generate + test + lint + license).

Sanity checks:
1. `agent/pr-reviewer/pkg/prompts/execution_workflow.md` no longer exists.
2. `grep -rn "executionWorkflow" agent/pr-reviewer/` returns no hits.
3. `grep -n "BuildExecutionInstructions" agent/pr-reviewer/pkg/factory/factory.go` returns no hits (call moved to the step).
4. `grep -n "BuildExecutionInstructions" agent/pr-reviewer/pkg/steps_checkout_execution.go` returns exactly one hit (the call site inside `Run`).
5. New test file `agent/pr-reviewer/pkg/prompts/execution_test.go` exists and is exercised by `make test`.
6. `CHANGELOG.md` has an `## Unreleased` section describing the fix.

Manual verification (post-deploy, not part of this prompt — for reference):
- `cd agent/pr-reviewer/cmd/run-task && make generate-dummy-task TASK_FILE=verify-fix.md && make run-dummy-task TASK_FILE=verify-fix.md`
- Verdict log should contain real sub-agent names (e.g. `go-quality-assistant`, `go-security-specialist`) — proving the plugin actually fired.
- The verdict's `concerns_addressed` should reference plugin section labels (`Must Fix` / `Should Fix` / `Nice to Have`).
</verification>
