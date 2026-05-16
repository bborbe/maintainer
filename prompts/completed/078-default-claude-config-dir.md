---
status: completed
summary: Added `default:"~/.claude"` to ClaudeConfigDir field tag in both agent/pr-reviewer/main.go and agent/pr-reviewer/cmd/run-task/main.go, and added Unreleased CHANGELOG entry.
container: code-reviewer-078-default-claude-config-dir
dark-factory-version: v0.143.0-5-g73d1db8
created: "2026-05-02T15:55:00Z"
queued: "2026-05-02T22:17:43Z"
started: "2026-05-02T22:19:40Z"
completed: "2026-05-02T22:21:32Z"
---

<summary>
- Add `default:"~/.claude"` to the `ClaudeConfigDir` argument in both `agent/pr-reviewer/main.go` and `agent/pr-reviewer/cmd/run-task/main.go`
- Eliminates the silent-failure case where the agent process runs with empty `CLAUDE_CONFIG_DIR`, the `bborbe/coding` plugin is not discoverable, and `/coding:pr-review` is silently unregistered (causing the execution-phase fallback we hit in dev on 2026-05-02)
- K8s deployments still win because the Config CRD's `env` field is set explicitly (`CLAUDE_CONFIG_DIR=/home/claude/.claude`) — env vars take precedence over arg defaults
- Local CLI usage (`make run-dummy-task`) becomes ergonomic — no env or flag needed for the common case
- Tilde expansion is handled by the consumed `agent/lib` (sibling prompt ships `Resolve()` on `ClaudeConfigDir`); literal `~/.claude` reaching env-var emission expands at the trust boundary
- This prompt lands the default ONLY; tilde expansion in lib is a sibling concern
</summary>

<objective>
Make `ClaudeConfigDir` default to `~/.claude` in both pr-reviewer entry points so consumers don't silently run without plugins when no env or flag is provided.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read before changing:
- `agent/pr-reviewer/main.go` — `application` struct; `ClaudeConfigDir` field at ~line 41
- `agent/pr-reviewer/cmd/run-task/main.go` — sibling entry point with the same arg

Key facts (verified):
- The argument framework is `bborbe/argument/v2`. `default:"X"` tag attribute sets the default value when neither flag nor env var is provided.
- **Downstream consumer audit** (verified 2026-05-02): in this repo, `ClaudeConfigDir` is only PROPAGATED, never consumed for filesystem ops. Call sites:
  - `agent/pr-reviewer/pkg/prompts/execution.go:59` — constructor param
  - `agent/pr-reviewer/pkg/steps_checkout_execution.go:23,35,130` — struct field, constructor, passed to `claudelib.ClaudeRunnerConfig{ClaudeConfigDir: s.claudeConfigDir}`
  - `agent/pr-reviewer/pkg/factory/factory.go:82,89,147` — constructor param, passed through to `ClaudeRunnerConfig`
  - All flow into `agent/lib/claude/claude-runner.go` (a separate repo, sibling change at `agent/prompts/expand-tilde-in-claudelib-paths.md`) which is the ONLY actual consumer (emits env var to subprocess)
- **Tilde expansion already shipped in `agent/lib`** via `claudelib.ClaudeConfigDir.Resolve()` (sibling work landed). This prompt only emits the literal `~/.claude` default — Claude CLI and downstream `Resolve()` handle expansion at the trust boundary.
- Today's bug (2026-05-02): K8s agent ran with empty `ClaudeConfigDir` because the Config CRD's `env: CLAUDE_CONFIG_DIR` was missing. CRD env was added in commit `051259b`. This default is defense-in-depth: if a future Config CRD forgets the env, the agent still finds the plugin via `$HOME/.claude` (HOME=/home/claude in the claude-yolo container; HOME=/Users/<user> on local CLI dev).
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Update `agent/pr-reviewer/main.go`** — add `default:"~/.claude"` to the `ClaudeConfigDir` field tag.

   Find:
   ```go
   ClaudeConfigDir claudelib.ClaudeConfigDir `required:"false" arg:"claude-config-dir" env:"CLAUDE_CONFIG_DIR" usage:"Claude Code config directory"`
   ```

   Replace with:
   ```go
   ClaudeConfigDir claudelib.ClaudeConfigDir `required:"false" arg:"claude-config-dir" env:"CLAUDE_CONFIG_DIR" usage:"Claude Code config directory" default:"~/.claude"`
   ```

2. **Update `agent/pr-reviewer/cmd/run-task/main.go`** — same change for the sibling entry point. The field has the same tag shape.

3. **Confirm there are no other consumers of the `ClaudeConfigDir` arg** in this repo:

   ```bash
   grep -rn "ClaudeConfigDir" agent/pr-reviewer --include="*.go" | grep -v _test.go | grep -v vendor
   ```

   If a third entry point exists, update its tag the same way. (As of this prompt's creation, only `main.go` and `cmd/run-task/main.go` have the arg.)

4. **Update CHANGELOG.md** — add a `## Unreleased` section above the most recent version heading (currently `## v0.23.4`) if absent, then add the bullet:

   ```markdown
   ## Unreleased

   - feat(pr-reviewer): default `ClaudeConfigDir` arg to `~/.claude` in both entry points (`main.go` and `cmd/run-task/main.go`). Defense-in-depth: prevents the silent "empty CLAUDE_CONFIG_DIR → plugin not discoverable" failure mode hit in dev on 2026-05-02. K8s deploys still take their explicit `CLAUDE_CONFIG_DIR` from the Config CRD env (env > arg default).
   ```

5. **Run `make precommit`** in `agent/pr-reviewer/`:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```

</requirements>

<constraints>
- Only edit files under `agent/pr-reviewer/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- Do NOT change the arg's name, env-var name, or `required` flag — only add `default:"~/.claude"`
- Do NOT add tilde-expansion logic to this prompt — that lives in `agent/lib` (sibling prompt). This prompt only sets the literal default.
- `make precommit` runs from `agent/pr-reviewer/`
- Existing tests must keep passing
</constraints>

<verification>
cd agent/pr-reviewer && make precommit

# Confirm both entry points have the default (fails loudly if either is missing):
grep -qE "ClaudeConfigDir.*default:\"~/.claude\"" agent/pr-reviewer/main.go \
  && grep -qE "ClaudeConfigDir.*default:\"~/.claude\"" agent/pr-reviewer/cmd/run-task/main.go \
  && echo "ok: both entry points have default"
</verification>
