---
status: completed
spec: [011-swap-execution-to-coding-pr-review]
summary: Created scenarios/005-spec-011-plugin-pr-review.md with the full manual verification checklist for the /coding:pr-review plugin delegation execution phase, and added an Unreleased entry to CHANGELOG.md.
container: code-reviewer-069-spec-011-scenario
dark-factory-version: dev
created: "2026-05-01T10:00:00Z"
queued: "2026-05-01T09:31:59Z"
started: "2026-05-01T09:36:42Z"
completed: "2026-05-01T09:37:36Z"
branch: dark-factory/swap-execution-to-coding-pr-review
---

<summary>
- A new scenario file documents the end-to-end verification checklist for the plugin-delegation execution phase
- The scenario exercises: slash command invocation, sub-agent fan-out, verdict JSON parsing, and worktree cleanup
- It serves as the required subprocess+slash-command interface seam test referenced in the spec's acceptance criteria
- Existing scenarios 001–004 are unchanged
</summary>

<objective>
Create `scenarios/005-spec-011-plugin-pr-review.md` — a manual verification checklist that exercises the full execution phase of the `/coding:pr-review` plugin delegation end-to-end against PR #2 on the dev cluster, asserting that the slash command runs, at least one sub-agent fires, the verdict JSON parses and matches the schema, and the workdir cleanup completes.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/architecture.md` — "Agent contract" section for task frontmatter fields and the workdir layout.

Read existing scenarios for format and style:
- `scenarios/001-pr-reviewer-github-https.md`
- `scenarios/003-pr-reviewer-arbitrary-ref.md`

The scenario must cover the acceptance criteria seam from spec-011:
- (a) `/coding:pr-review` slash command runs (observable in agent logs)
- (b) at least one sub-agent fires (e.g. `go-quality-assistant` appears in logs or verdict)
- (c) verdict JSON parses and fields match schema (`verdict ∈ {approve, request_changes, comment}`, `summary` non-empty, `concerns_addressed` non-empty, each comment has `file`/`line`/`severity`/`message`)
- (d) workdir cleanup completes (no stale `/work/<task-id>` directory after run)

Also document the review-mode flag check: running with `REVIEW_MODE=full` produces more findings than `REVIEW_MODE=short`.
</context>

<requirements>
1. **Create `scenarios/005-spec-011-plugin-pr-review.md`** with this exact structure
   (fill in the content following the existing scenario style):

   ```markdown
   ---
   status: draft
   spec: 011-swap-execution-to-coding-pr-review
   ---

   # Scenario 005: execution phase delegates to /coding:pr-review plugin

   Validates that the execution phase invokes the `/coding:pr-review` slash command
   inside the prepared worktree, receives a structured findings report from specialist
   sub-agents, translates it into the verdict JSON schema, and cleans up the worktree.
   This is the required subprocess + slash-command interface seam test for spec-011.

   ## Setup
   - [ ] Scenario 001 prerequisites met (GitHub token in `GH_TOKEN`, `claude` CLI available)
   - [ ] `bborbe/coding` plugin is installed: `claude plugin list | grep coding`
   - [ ] PR #2 exists in `bborbe/code-reviewer` with a non-empty diff against `master`
   - [ ] Task file generated for PR #2:
     ```bash
     cd ~/Documents/workspaces/code-reviewer/agent/pr-reviewer/cmd/run-task
     make generate-dummy-task TASK_FILE=test5-plugin.md
     ```
     (uses default PR #2 task configuration from the Makefile)

   ## Action — standard mode (default)
   - [ ] Run the agent in `in_progress` phase:
     ```bash
     cd ~/Documents/workspaces/code-reviewer/agent/pr-reviewer/cmd/run-task
     make run-dummy-task TASK_FILE=test5-plugin.md
     ```

   ## Expected — standard mode
   - [ ] Agent log contains `/coding:pr-review` (confirms slash command was invoked)
   - [ ] Agent log contains at least one sub-agent name: `go-quality-assistant`,
         `go-security-specialist`, `go-factory-pattern-assistant`, or similar
   - [ ] `## Review` section in `test5-plugin.md` contains a valid JSON object:
     ```bash
     grep -A 5 "## Review" test5-plugin.md
     ```
   - [ ] Verdict JSON parses without error:
     ```bash
     grep -A 50 "## Review" test5-plugin.md | python3 -c "import sys,json; json.load(sys.stdin)"
     ```
   - [ ] `verdict` field is one of `approve`, `request_changes`, `comment`
   - [ ] `summary` is a non-empty string
   - [ ] `concerns_addressed` is a non-empty list (reflects sub-agent or Must Fix/Should Fix findings, not generic boilerplate from the old hand-rolled prompt)
   - [ ] Workdir cleaned up (no stale worktree after run):
     ```bash
     ls ~/.cache/code-reviewer/work/
     ```
     Worktree directory for the task should be absent (or pruned on next startup)
   - [ ] Process exits 0

   ## Action — short mode
   - [ ] Re-generate a fresh task file (or copy test5-plugin.md before the Review section is written):
     ```bash
     make generate-dummy-task TASK_FILE=test5-short.md
     ```
   - [ ] Run with short mode:
     ```bash
     REVIEW_MODE=short make run-dummy-task TASK_FILE=test5-short.md
     ```

   ## Expected — short mode
   - [ ] Agent log shows `/coding:pr-review master short`
   - [ ] No specialist sub-agent names appear in the log (short mode skips agents)
   - [ ] Verdict JSON is still valid and parses

   ## Cleanup
   - [ ] `rm test5-plugin.md test5-short.md`
   - [ ] Worktrees: removed automatically on next pod/process startup via `PruneAllWorktrees`

   ## Notes
   Last run: (not yet run — scenario created for spec-011)
   ```

2. **No Go code changes** — this prompt creates only `scenarios/005-spec-011-plugin-pr-review.md`.

3. **Verification**:
   ```bash
   ls -la scenarios/005-spec-011-plugin-pr-review.md
   head -10 scenarios/005-spec-011-plugin-pr-review.md
   ```
</requirements>

<constraints>
- Only create `scenarios/005-spec-011-plugin-pr-review.md` — do not modify any other file
- Do NOT commit — dark-factory handles git
- Follow the exact frontmatter format from existing scenarios (`status: draft`, `spec: <name>`)
- The scenario is a manual checklist (not automated) — use checkbox lists `- [ ]`
- `make precommit` is NOT required since no Go code changed (YAML/markdown only)
</constraints>

<verification>
ls -la scenarios/005-spec-011-plugin-pr-review.md

head -5 scenarios/005-spec-011-plugin-pr-review.md
# Expected: frontmatter with status: draft and spec: 011-...
</verification>
