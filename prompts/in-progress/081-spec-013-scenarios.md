---
status: approved
spec: [013-repo-allowlist-stage-isolation]
created: "2026-05-03T16:30:00Z"
queued: "2026-05-03T16:58:25Z"
branch: dark-factory/repo-allowlist-stage-isolation
---

<summary>
- Two new scenario files are created under `scenarios/` to provide integration-level validation coverage for the repo-allowlist feature introduced by spec-013
- Scenario 012 covers the watcher filter end-to-end: an allowlisted PR produces a vault task, a non-allowlisted PR in the same org produces no vault task (watcher skips it)
- Scenario 013 covers the agent clone-refusal path: a task whose `clone_url` is outside the agent's allowlist returns `NeedsInput` without cloning, while a task with an allowlisted `clone_url` proceeds normally
- The agent scenario is verifiable locally via `cmd/run-task` (no live cluster needed) using crafted task markdown files
- Both scenarios include startup-failure sub-scenarios for malformed allowlist entries, and an empty-allowlist sub-scenario confirming backwards-compatibility
- No Go code is written in this prompt — only markdown scenario files
- No `make precommit` is needed (no Go changes); the verification command confirms both files exist and are non-empty
</summary>

<objective>
Create two integration scenario files (`scenarios/012-watcher-repo-allowlist-filter.md` and `scenarios/013-agent-repo-allowlist-clone-refusal.md`) that satisfy the scenario coverage requirement of spec-013: at least one scenario for the watcher filter end-to-end and one for the agent clone-refusal path.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read before writing scenarios:
- `scenarios/006-watcher-author-trust-filter.md` — example scenario file; mirror its structure (frontmatter, prerequisites, sub-scenarios with Action + Expected + Cleanup sections, markdown checklist format)
- `docs/architecture.md` — understand the pipeline; the watcher → Kafka → controller → vault → agent flow

Key facts:
- Existing scenarios are numbered 001–011; next available numbers are 012 and 013
- Scenario 012 = watcher filter (requires a running dev cluster)
- Scenario 013 = agent clone-refusal (verifiable locally via `cmd/run-task`)
- The watcher's `REPO_ALLOWLIST` env var holds comma-separated `host/owner/repo` entries; the watcher skips PRs not on the list (does NOT publish to Kafka)
- The agent's `REPO_ALLOWLIST` env var holds the same format; the agent returns `NeedsInput` when the task's `clone_url` parses to a repo not on the list
- After spec-013's changes: `dev.env` contains `REPO_ALLOWLIST=github.com/bborbe/code-reviewer`; verify this in the watcher scenario prerequisites
- The agent's local test runner is `agent/pr-reviewer/cmd/run-task/main.go`; it reads a task markdown file from disk via `TASK_FILE` or `--task-file`, runs the agent, and writes the result back; the `REPO_ALLOWLIST` env var controls the allowlist
- A task file for the agent scenario can be created manually in a temp dir; `clone_url`, `ref`, and `base_ref` must appear in the frontmatter
- The `make run-dummy-task` target in `agent/pr-reviewer/` may also work — check with `grep -n "run-dummy-task\|run-task" agent/pr-reviewer/Makefile` before writing the scenario
- The watcher skip metric label is `"skipped"` (from `w.metrics.IncPRPublished("skipped")` in `watcher.go`)
</context>

<requirements>

**Execute both steps. No `make precommit` needed (no Go changes).**

1. **Create `scenarios/012-watcher-repo-allowlist-filter.md`**:

```markdown
---
status: draft
spec: 013-repo-allowlist-stage-isolation
---

# Scenario 012: watcher repo-allowlist filter blocks non-allowlisted PRs

Validates the watcher layer of spec-013: the `REPO_ALLOWLIST` env var restricts
which repos the watcher publishes tasks for. An allowlisted PR produces a vault
task; a non-allowlisted PR in the same org produces no task (watcher skips it
silently).

This is a required integration-seam test for spec-013: the filter runs after
all other `TaskCreationFilter` leaves and before the Kafka publish — this path
cannot be covered by unit tests alone because the vault-materialization step
(controller) is the observable outcome.

## Prerequisites
- [ ] Dev cluster is running and healthy (`kubectl get pods -n code-reviewer`)
- [ ] Watcher is deployed to dev with `REPO_ALLOWLIST=github.com/bborbe/code-reviewer`
      (already set in `dev.env`). Confirm:
      ```bash
      kubectl get deployment github-pr-watcher -n code-reviewer \
        -o jsonpath='{.spec.template.spec.containers[0].env}' \
        | python3 -m json.tool | grep REPO_ALLOWLIST
      ```
- [ ] You can open PRs on `bborbe/code-reviewer` (the allowlisted repo) AND on a
      second repo in the same org (e.g. `bborbe/sample-project`) that is NOT on the
      allowlist — call it `non-allowlisted-repo` below
- [ ] Vault CLI is available: `vault kv list secret/code-reviewer/tasks/` returns results
- [ ] Watcher Prometheus metrics are accessible at its `/metrics` endpoint or via:
      ```bash
      kubectl port-forward svc/github-pr-watcher -n code-reviewer 9090:9090 &
      ```

## Sub-scenario A: allowlisted repo PR → vault task created

### Action
- [ ] Open a PR on `bborbe/code-reviewer` (the allowlisted repo) from the trusted
      author account (the login in `TRUSTED_AUTHORS`):
      ```bash
      # e.g. push a test branch and open a PR via gh:
      gh pr create --repo bborbe/code-reviewer --title "test: scenario 012 allowlisted PR" --body ""
      ```
- [ ] Note the PR number: `<pr-number>`
- [ ] Wait up to one poll cycle (default 5 min) for the watcher to process it

### Expected
- [ ] A vault task appears for the PR:
      ```bash
      vault kv list secret/code-reviewer/tasks/ | grep -i code-reviewer
      ```
- [ ] The vault task frontmatter has `phase: planning` and `status: in_progress`
      (trusted author fast-path):
      ```bash
      vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
        | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
      # Expected: planning in_progress
      ```
- [ ] The watcher log shows a publish event for this PR (NOT a skip):
      ```bash
      kubectl logs -n code-reviewer deployment/github-pr-watcher | grep "published CreateTaskCommand" | grep "code-reviewer"
      ```

## Sub-scenario B: non-allowlisted repo PR → no vault task

### Action
- [ ] Open a PR on `bborbe/<non-allowlisted-repo>` from any account:
      ```bash
      gh pr create --repo bborbe/<non-allowlisted-repo> --title "test: scenario 012 non-allowlisted" --body ""
      ```
- [ ] Note the PR number: `<other-pr-number>`
- [ ] Wait up to two poll cycles (≤ 10 min) for the watcher to process it

### Expected
- [ ] NO vault task appears for this PR:
      ```bash
      # Derive the expected task ID using the same hash logic as the watcher
      # (or just list all tasks and confirm none match the non-allowlisted repo title):
      vault kv list secret/code-reviewer/tasks/
      # No entry corresponding to <non-allowlisted-repo>#<other-pr-number>
      ```
- [ ] The watcher log shows a skip event for this PR:
      ```bash
      kubectl logs -n code-reviewer deployment/github-pr-watcher \
        | grep "skipping" | grep "<non-allowlisted-repo>"
      # Expected: line containing "reason=filtered"
      ```
- [ ] The Prometheus `pr_published_total{result="skipped"}` counter incremented:
      ```bash
      curl -s http://localhost:9090/metrics | grep 'pr_published_total.*skipped'
      # Expected: value > 0
      ```

## Sub-scenario C: startup failure on malformed REPO_ALLOWLIST

### Action
- [ ] Temporarily patch the watcher deployment to set a malformed allowlist:
      ```bash
      kubectl set env deployment/github-pr-watcher -n code-reviewer \
        REPO_ALLOWLIST=bborbe/code-reviewer
      # (two segments, no host — deliberately malformed)
      ```
- [ ] Wait for the pod to restart:
      ```bash
      kubectl rollout status deployment/github-pr-watcher -n code-reviewer --timeout=60s
      ```
- [ ] Check logs:
      ```bash
      kubectl logs -n code-reviewer deployment/github-pr-watcher | tail -20
      ```

### Expected
- [ ] Pod fails to start (CrashLoopBackOff or exits non-zero immediately)
- [ ] Log contains a message naming the malformed entry `bborbe/code-reviewer`
      and mentioning `host/owner/repo` format

### Cleanup
- [ ] Restore the correct allowlist:
      ```bash
      kubectl set env deployment/github-pr-watcher -n code-reviewer \
        REPO_ALLOWLIST=github.com/bborbe/code-reviewer
      kubectl rollout status deployment/github-pr-watcher -n code-reviewer --timeout=60s
      ```

## Sub-scenario D: empty REPO_ALLOWLIST → allow-all (backwards-compatibility)

### Action
- [ ] Temporarily clear the allowlist on the watcher:
      ```bash
      kubectl set env deployment/github-pr-watcher -n code-reviewer REPO_ALLOWLIST=
      ```
- [ ] Open a PR on `bborbe/<non-allowlisted-repo>` (any repo in scope via `REPO_SCOPE`)
- [ ] Wait one poll cycle

### Expected
- [ ] Watcher log shows `repo-allowlist count=0` at startup
- [ ] A vault task IS created for the non-allowlisted repo (empty allowlist = allow-all)

### Cleanup
- [ ] Restore the allowlist:
      ```bash
      kubectl set env deployment/github-pr-watcher -n code-reviewer \
        REPO_ALLOWLIST=github.com/bborbe/code-reviewer
      ```

## Cleanup
- [ ] Close or merge the test PRs opened in sub-scenarios A and B
- [ ] Confirm the watcher is healthy after all restores:
      ```bash
      kubectl get pods -n code-reviewer | grep github-pr-watcher
      ```

## Notes
Last run: (not yet run — scenario created for spec-013)
```

2. **Create `scenarios/013-agent-repo-allowlist-clone-refusal.md`**:

```markdown
---
status: draft
spec: 013-repo-allowlist-stage-isolation
---

# Scenario 013: agent repo-allowlist blocks clone of non-allowlisted task

Validates the agent layer of spec-013: when `REPO_ALLOWLIST` is set and a task's
`clone_url` parses to a repo not on the list, the agent returns `status: needs_input`
without cloning. This is the defense-in-depth layer — it fires even if a stale or
mis-routed task somehow reaches the wrong agent.

This scenario is verifiable **locally** using `cmd/run-task` — no live cluster needed.

## Prerequisites
- [ ] `agent/pr-reviewer` is built and sibling prompts 1 + 2 have been deployed:
      ```bash
      cd agent/pr-reviewer && go build ./cmd/run-task/
      ```
- [ ] Vault / Kafka NOT required — `cmd/run-task` uses file I/O only
- [ ] A temp directory for test task files: `mkdir -p /tmp/scenario-013`

## Sub-scenario A: clone_url outside allowlist → NeedsInput, no clone

### Setup
- [ ] Create a test task file whose `clone_url` is outside the allowlist:
      ```bash
      cat > /tmp/scenario-013/out-of-scope-task.md << 'EOF'
      ---
      clone_url: https://github.com/bborbe/other-repo.git
      ref: abc1234567890abcdef
      base_ref: master
      task_identifier: scenario-013-test-001
      phase: in_progress
      status: in_progress
      ---

      # PR Review: scenario 013 test — non-allowlisted repo
      EOF
      ```

### Action
- [ ] Run the agent against this task with the allowlist restricted to a different repo, AND pin REPOS_PATH/WORK_PATH to scenario-local temp dirs so the no-clone assertion is hermetic:
      ```bash
      mkdir -p /tmp/scenario-013/repos /tmp/scenario-013/work
      REPO_ALLOWLIST=github.com/bborbe/code-reviewer \
      REPOS_PATH=/tmp/scenario-013/repos \
      WORK_PATH=/tmp/scenario-013/work \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-013/out-of-scope-task.md \
      ./agent/pr-reviewer/cmd/run-task/run-task
      ```

### Expected
- [ ] Agent exits with output containing `"status":"needs_input"`:
      ```bash
      cat /tmp/scenario-013/out-of-scope-task.md | grep -A2 '"status"'
      # Expected: "needs_input"
      ```
      Or check the agent's stdout JSON directly.
- [ ] The diagnostic message names the parsed repo key `github.com/bborbe/other-repo`
- [ ] The diagnostic message names the configured allowlist size (1 entry)
- [ ] NO git clone was performed — the scenario-local repos and work dirs stay empty:
      ```bash
      [ -z "$(ls -A /tmp/scenario-013/repos)" ] && echo "ok: repos empty"
      [ -z "$(ls -A /tmp/scenario-013/work)"  ] && echo "ok: work empty"
      ```

## Sub-scenario B: clone_url on allowlist → proceeds past the allowlist check

### Setup
- [ ] Create a test task file whose `clone_url` IS on the allowlist:
      ```bash
      cat > /tmp/scenario-013/in-scope-task.md << 'EOF'
      ---
      clone_url: https://github.com/bborbe/code-reviewer.git
      ref: master
      base_ref: master
      task_identifier: scenario-013-test-002
      phase: in_progress
      status: in_progress
      ---

      # PR Review: scenario 013 test — allowlisted repo
      EOF
      ```

### Action
- [ ] Run the agent against this task with the matching allowlist (no GH token needed —
      the test will fail at clone or Claude, but must NOT fail at the allowlist check):
      ```bash
      REPO_ALLOWLIST=github.com/bborbe/code-reviewer \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-013/in-scope-task.md \
      ./agent/pr-reviewer/cmd/run-task/run-task 2>&1 | head -20
      ```

### Expected
- [ ] Output does NOT contain `"needs_input"` (allowlist check passed)
- [ ] Agent proceeds to the clone/checkout phase (may fail later due to missing GH token
      or network access — that is expected and out of scope for this scenario)
- [ ] Log does NOT contain `"not on the allowlist"` or similar allowlist-reject message

## Sub-scenario C: startup failure on malformed REPO_ALLOWLIST

### Action
- [ ] Run `cmd/run-task` with a malformed (two-segment) allowlist:
      ```bash
      REPO_ALLOWLIST=bborbe/code-reviewer \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-013/out-of-scope-task.md \
      ./agent/pr-reviewer/cmd/run-task/run-task 2>&1 | head -10
      ```

### Expected
- [ ] Agent exits non-zero immediately (startup failure before any PR processing)
- [ ] Output contains a message naming the malformed entry `bborbe/code-reviewer`
      and mentioning the required `host/owner/repo` format

## Sub-scenario D: empty REPO_ALLOWLIST → allow-all (backwards-compatibility)

### Action
- [ ] Run the agent with no allowlist set (or explicitly empty):
      ```bash
      REPO_ALLOWLIST= \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-013/out-of-scope-task.md \
      ./agent/pr-reviewer/cmd/run-task/run-task 2>&1 | head -20
      ```

### Expected
- [ ] Agent startup log shows `repo-allowlist count=0`
- [ ] Agent does NOT return `needs_input` from the allowlist check (empty = allow-all)
- [ ] Agent proceeds to clone/checkout (may fail for other reasons — expected)

## Cleanup
- [ ] Remove temp task files: `rm -rf /tmp/scenario-013`

## Notes
Last run: (not yet run — scenario created for spec-013)
```

</requirements>

<constraints>
- Only create files under `scenarios/` — do NOT edit any Go source files or CHANGELOG.md in this prompt
- Do NOT commit — dark-factory handles git
- Scenario files are markdown only; no `make precommit` is needed
- The scenario numbers 012 and 013 are the next available numbers after the existing 011 highest scenario
- Do NOT alter existing scenario files
- The `cmd/run-task` binary path in the scenario assumes it has been built with `go build ./cmd/run-task/` from `agent/pr-reviewer/` — adjust the path in the scenario if the Makefile exposes a `make build` target that places the binary elsewhere
- Before writing the scenario, check `agent/pr-reviewer/Makefile` for any `run-task` or `run-dummy-task` targets:
  ```bash
  grep -n "run-task\|run-dummy\|build" agent/pr-reviewer/Makefile | head -20
  ```
  If a convenience target exists, reference it in the scenario instead of the raw binary path
</constraints>

<verification>
# Confirm both scenario files were created and are non-empty:
ls -la scenarios/012-watcher-repo-allowlist-filter.md scenarios/013-agent-repo-allowlist-clone-refusal.md

# Confirm they have the correct frontmatter spec field:
grep "spec:" scenarios/012-watcher-repo-allowlist-filter.md scenarios/013-agent-repo-allowlist-clone-refusal.md

# Confirm they have sub-scenario A and B sections:
grep "Sub-scenario" scenarios/012-watcher-repo-allowlist-filter.md scenarios/013-agent-repo-allowlist-clone-refusal.md
</verification>
