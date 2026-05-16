---
status: completed
spec: [029-migrate-callers-to-repoallowlist-lib-and-wildcard-rollout]
summary: 'Switched REPO_ALLOWLIST in dev.env and prod.env from enumerated literal repo lists to github.com/bborbe/* wildcard and added ## Unreleased changelog entry.'
container: maintainer-118-spec-029-env-update
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-15T20:05:00Z"
queued: "2026-05-15T20:09:41Z"
started: "2026-05-15T20:23:39Z"
completed: "2026-05-15T20:24:41Z"
branch: dark-factory/migrate-callers-to-repoallowlist-lib-and-wildcard-rollout
---

<summary>

- `dev.env` REPO_ALLOWLIST changes from `github.com/bborbe/go-skeleton` (single literal) to `github.com/bborbe/*` (org-wide wildcard)
- `prod.env` REPO_ALLOWLIST changes from a seven-entry literal list to `github.com/bborbe/*` (org-wide wildcard)
- After this change, any PR opened in any bborbe-owned repo automatically flows through the watcher → controller → agent pipeline without requiring an operator to add the repo explicitly
- The Obsidian runbook "Agent - Add Repo to PR Reviewer Allowlist" cannot be updated by this container (it lives outside the workspace at `~/Documents/Obsidian/Personal/65 Runbooks/Agent - Add Repo to PR Reviewer Allowlist.md`); the update is documented as a manual operator step in this prompt's completion report
- No code changes, no `make precommit` needed — env files only

</summary>

<objective>

Switch `REPO_ALLOWLIST` in both env files from enumerated literal repo lists to the single `github.com/bborbe/*` wildcard, completing the second half of the two-phase rollout. This prompt must only run AFTER the code migration prompt (prompt 1) has been deployed to all three services — deploying the env change before the new code is live would cause the build watcher's pre-migration startup validator to reject the wildcard and crashloop.

</objective>

<context>

Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.

**This prompt depends on prompt 1 (1-spec-029-code-migration.md) having been executed AND the new image being deployed in dev + prod clusters.**

Before making changes, verify the code migration has been applied across ALL 5 caller binaries (spec 029 AC #2). Missing even one would mean a partial migration that crashloops on the wildcard env value:

```bash
grep -rn "github\.com/bborbe/maintainer/lib/repoallowlist" \
  /workspace/agent/pr-reviewer/main.go \
  /workspace/agent/pr-reviewer/cmd/run-task/main.go \
  /workspace/watcher/github-pr/main.go \
  /workspace/watcher/github-build/main.go \
  /workspace/watcher/github-build/cmd/run-once/main.go
```

Expected: **at least 5 import-line matches** (one per main.go). If fewer than 5 → STOP and report `{"status":"failed","message":"code migration (prompt 1) is incomplete: <N>/5 main.go files import lib/repoallowlist. Partial migration will crashloop the build-watcher when env switches to wildcard."}`.

**This grep verifies code is MERGED, not DEPLOYED.** Operator must confirm dev + prod pods are running the new image BEFORE approving this prompt for execution. The completion report MUST flag: "DO NOT redeploy env until pods on new image confirmed."

**Files to read before making any changes:**

- `dev.env` at repo root — current REPO_ALLOWLIST line (expected: `export REPO_ALLOWLIST=github.com/bborbe/go-skeleton`)
- `prod.env` at repo root — current REPO_ALLOWLIST line (expected: `export REPO_ALLOWLIST=github.com/bborbe/maintainer,github.com/bborbe/jira-task-creator,github.com/bborbe/agent,github.com/bborbe/dark-factory,github.com/bborbe/vault-cli,github.com/bborbe/trading,github.com/bborbe/coding`)
- `CHANGELOG.md` at repo root — check for `## Unreleased` (should exist from prompt 1)

**Deploy ordering constraint (critical):**

The spec's rollout order is enforced at the infrastructure level, not in this container. This container only changes the env files. The management session is responsible for:
1. Merging prompt 1 (code change) and deploying to dev+prod first
2. Only after all three services are running the new image: running this container to commit the env change
3. Deploying the env change to dev, verifying Rung-2, then deploying to prod (Rung-3)

**Obsidian runbook update (manual step, outside this container):**

The runbook "Agent - Add Repo to PR Reviewer Allowlist" lives in the Obsidian vault at `~/Documents/Obsidian/Personal/65 Runbooks/Agent - Add Repo to PR Reviewer Allowlist.md` which is inaccessible from this YOLO container. The operator must update it manually after this container completes. The update should:
- State that `github.com/bborbe/*` is now the default
- Note that the primary use case has shifted to adding repos outside the bborbe org
- Document that per-repo additions inside `bborbe/` are still possible by adding a literal entry alongside the wildcard, separated by comma, followed by redeploy

</context>

<requirements>

Execute steps in order.

---

## Step 1 — Read all referenced files fully

Read `dev.env`, `prod.env`, and `CHANGELOG.md` before making changes. Confirm the current REPO_ALLOWLIST values match expectations. If either file has already been updated to `github.com/bborbe/*`, STOP — the change is already applied.

---

## Step 2 — Update `dev.env`

Read the full `dev.env` file first.

Change the REPO_ALLOWLIST line from:
```
export REPO_ALLOWLIST=github.com/bborbe/go-skeleton
```
to:
```
export REPO_ALLOWLIST=github.com/bborbe/*
```

Do NOT change any other line. Do NOT add comments. Do NOT reformat spacing.

---

## Step 3 — Update `prod.env`

Read the full `prod.env` file first.

Change the REPO_ALLOWLIST line from:
```
export REPO_ALLOWLIST=github.com/bborbe/maintainer,github.com/bborbe/jira-task-creator,github.com/bborbe/agent,github.com/bborbe/dark-factory,github.com/bborbe/vault-cli,github.com/bborbe/trading,github.com/bborbe/coding
```
to:
```
export REPO_ALLOWLIST=github.com/bborbe/*
```

Do NOT change any other line.

---

## Step 4 — Update CHANGELOG.md

Read `CHANGELOG.md` at repo root. Find the `## Unreleased` section (created by prompt 1). Append to the existing bullet list:

```markdown
- feat: switch REPO_ALLOWLIST in dev.env and prod.env from enumerated literal repo lists to `github.com/bborbe/*` wildcard; any bborbe-owned repo now flows through the pipeline without per-repo operator intervention
```

Do NOT replace the existing `## Unreleased` entry from prompt 1 — append only.

---

## Step 5 — Confirm no other env files need updating

Verify there are no other env files with REPO_ALLOWLIST:
```bash
grep -rn "REPO_ALLOWLIST" /workspace/*.env /workspace/common.env /workspace/local.env 2>/dev/null
```

The `common.env` and `local.env` files should NOT have REPO_ALLOWLIST (it is only in `dev.env` and `prod.env`). If REPO_ALLOWLIST appears in any other env file, update it too — but report it in the completion report.

</requirements>

<constraints>

- This prompt changes ONLY `dev.env`, `prod.env`, and `CHANGELOG.md`. No Go source files, no go.mod files, no k8s manifests.
- No `make precommit` — there is no Go code to compile. (Running precommit over the root Makefile.folder is also not needed for env-file-only changes.)
- The Obsidian runbook update is explicitly out of scope for this container. Document it as a manual step in the completion report's `## Improvements` section.
- Do NOT commit — dark-factory handles git.
- If the code migration (prompt 1) has not been applied, STOP immediately with `status: failed`. Never apply the env change before the code change.

</constraints>

<verification>

Confirm dev.env updated:
```bash
grep "REPO_ALLOWLIST" /workspace/dev.env
```
Expected: `export REPO_ALLOWLIST=github.com/bborbe/*`

Confirm prod.env updated:
```bash
grep "REPO_ALLOWLIST" /workspace/prod.env
```
Expected: `export REPO_ALLOWLIST=github.com/bborbe/*`

Confirm only env files and CHANGELOG changed:
```bash
git diff --name-only
```
Expected: only `dev.env`, `prod.env`, `CHANGELOG.md` in the diff.

Confirm no other env file has old literal list:
```bash
grep -rn "github.com/bborbe/maintainer,\|github.com/bborbe/go-skeleton" /workspace/*.env 2>/dev/null
```
Expected: zero matches.

Confirm CHANGELOG entry:
```bash
grep -n "wildcard\|bborbe/\*\|dev\.env\|prod\.env" /workspace/CHANGELOG.md | head -5
```
Expected: entry under `## Unreleased` mentioning the wildcard and env files.

</verification>
