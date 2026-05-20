---
status: committing
spec: [032-rename-task-status-phase-taxonomy]
summary: Bumped vault-cli to v0.64.3 in all three modules and flipped BuildTaskStatus default from 'todo' to 'next' and Phase default from 'in_progress' to 'execution' across four main.go entry points; all make precommit runs exit 0.
container: maintainer-exec-125-spec-032-dep-bump-and-defaults
dark-factory-version: v0.162.0
created: "2026-05-20T16:50:00Z"
queued: "2026-05-20T17:19:58Z"
started: "2026-05-20T17:47:59Z"
completed: "2026-05-20T17:26:42Z"
branch: dark-factory/rename-task-status-phase-taxonomy
lastFailReason: 'validate completion report: completion report status: failed'
---

<summary>
- vault-cli dependency is bumped to the version that introduces `TaskStatusNext` ("next") and `TaskPhaseExecution` ("execution") as canonical values in `watcher/github-build`, `watcher/github-pr`, and `agent/pr-reviewer` go.mod files.
- `watcher/github-build/main.go` and `watcher/github-build/cmd/run-once/main.go` now declare `BuildTaskStatus` with `default:"next"` instead of `default:"todo"`.
- `agent/pr-reviewer/main.go` and `agent/pr-reviewer/cmd/run-task/main.go` now declare the Phase flag with `default:"execution"` and updated usage string `planning | execution | ai_review`.
- All changed service modules pass `make precommit` after the dep bump and flag changes.
- If the required vault-cli version is not yet published, this prompt fails loudly with `status: failed` so the work can be retried after the vault-cli release.
</summary>

<objective>
Bump the vault-cli dependency in all maintainer modules that import it to the smallest published version that exposes `domain.TaskStatusNext` and `domain.TaskPhaseExecution`, then flip the default flag values in all affected `main.go` entry points from the legacy `"todo"` / `"in_progress"` to the new canonical `"next"` / `"execution"`.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.
Read `go-mod-dependency-fix-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — bump strategy and go mod tidy patterns.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega conventions.
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write for each code change.

Read these files fully before making any changes:
- `watcher/github-build/go.mod` — current vault-cli version (indirect, currently v0.64.0)
- `watcher/github-pr/go.mod` — current vault-cli version (indirect, currently v0.64.0)
- `agent/pr-reviewer/go.mod` — current vault-cli version (direct, currently v0.64.1)
- `watcher/github-build/main.go` — `BuildTaskStatus` field with `default:"todo"` around line 57
- `watcher/github-build/cmd/run-once/main.go` — `BuildTaskStatus` field with `default:"todo"` around line 39
- `agent/pr-reviewer/main.go` — `Phase domain.TaskPhase` with `default:"in_progress"` around line 69
- `agent/pr-reviewer/cmd/run-task/main.go` — `Phase domain.TaskPhase` with `default:"in_progress"` around line 58

**Critical background — minimum required vault-cli version is v0.64.3:**
The rename was implemented in vault-cli `master` and queued for release as **v0.64.3** (see `https://github.com/bborbe/vault-cli/blob/master/CHANGELOG.md` — the `## v0.64.3` entry introduces `TaskStatusNext`, `TaskPhaseExecution`, `IsValidTaskPhase`, and `NormalizeTaskPhase`). vault-cli v0.64.2 (the previous tag) does NOT contain these constants.

**Step 1 of this prompt is to check that vault-cli ≥ v0.64.3 is published.** If only v0.64.x tags exist, report `status: failed` immediately, do not modify any file, and message the operator to wait for the v0.64.3 release.

**Scope of go.mod files to bump** — only those that already list vault-cli:
- `agent/pr-reviewer/go.mod` (direct dep, currently v0.64.1)
- `watcher/github-build/go.mod` (indirect dep, currently v0.64.0)
- `watcher/github-pr/go.mod` (indirect dep, currently v0.64.0)
`lib/go.mod` does NOT list vault-cli — do not touch it.

**Scope of main.go files to change:**
- `watcher/github-build/main.go` — `BuildTaskStatus string` with `default:"todo"` → `default:"next"`
- `watcher/github-build/cmd/run-once/main.go` — same field, same change
- `agent/pr-reviewer/main.go` — `Phase domain.TaskPhase`: `default:"in_progress"` → `default:"execution"` and usage `"planning | in_progress | ai_review"` → `"planning | execution | ai_review"`
- `agent/pr-reviewer/cmd/run-task/main.go` — same as above
`agent/pr-reviewer/cmd/cli/main.go` does NOT declare a Phase or BuildTaskStatus flag — do not touch it.
</context>

<requirements>
Execute steps in order. **Do not skip Step 1** — it is the go/no-go gate for the entire prompt.

---

## Step 1 — Check vault-cli ≥ v0.64.3 is published (GATE — fail fast if absent)

The minimum required vault-cli version for this prompt is **v0.64.3** — the release that introduces `TaskStatusNext`, `TaskPhaseExecution`, `IsValidTaskPhase`, and `NormalizeTaskPhase`. Before touching any file, confirm that tag exists:

```bash
curl -s "https://proxy.golang.org/github.com/bborbe/vault-cli/@v/list" | sort -V | tail -10
```

The list MUST include `v0.64.3` or higher. If the highest tag is still `v0.64.x`, the release has not been cut. **Stop immediately. Do not modify any file.** Write the completion report with:
- `"status": "failed"`
- `"message": "vault-cli v0.64.3 not yet published — highest available tag is <HIGHEST>. Re-run after vault-cli v0.64.3 is tagged. No files were modified."`

If v0.64.3+ exists, pick the highest tag as `TARGET_VERSION` (default to `v0.64.3` if no higher tag exists yet) and verify both constants are actually present at that tag:

```bash
# Replace vX.Y.Z with TARGET_VERSION
curl -sf "https://raw.githubusercontent.com/bborbe/vault-cli/vX.Y.Z/pkg/domain/task_status.go" | grep -n "TaskStatusNext"
curl -sf "https://raw.githubusercontent.com/bborbe/vault-cli/vX.Y.Z/pkg/domain/task_phase.go"  | grep -n "TaskPhaseExecution"
```

Both greps MUST return ≥1 line. If either is missing at TARGET_VERSION, stop with `status: failed` and name the missing constant.

---

## Step 2 — Identify the target vault-cli version

From Step 1, the target version is the smallest published vault-cli release that contains both `TaskStatusNext` and `TaskPhaseExecution`. Confirm this version's existence on the Go proxy:

```bash
curl -s "https://proxy.golang.org/github.com/bborbe/vault-cli/@v/vX.Y.Z.info"
```
Expected: JSON response with `"Version":"vX.Y.Z"`. Record this as TARGET_VERSION for all subsequent steps.

---

## Step 3 — Bump vault-cli in `agent/pr-reviewer/go.mod`

```bash
(cd agent/pr-reviewer && go get github.com/bborbe/vault-cli@vX.Y.Z && go mod tidy)
```

(Replace `vX.Y.Z` with TARGET_VERSION from Step 2.)

After: confirm the version updated:
```bash
grep vault-cli agent/pr-reviewer/go.mod
```
Expected: shows TARGET_VERSION as a direct dependency.

Verify the new constant is accessible:
```bash
cd agent/pr-reviewer && go doc github.com/bborbe/vault-cli/pkg/domain TaskStatusNext
```
Expected: documentation output; exit 0.

---

## Step 4 — Bump vault-cli in `watcher/github-build/go.mod`

```bash
(cd watcher/github-build && go get github.com/bborbe/vault-cli@vX.Y.Z && go mod tidy)
```

After:
```bash
grep vault-cli watcher/github-build/go.mod
```
Expected: shows TARGET_VERSION.

---

## Step 5 — Bump vault-cli in `watcher/github-pr/go.mod`

```bash
(cd watcher/github-pr && go get github.com/bborbe/vault-cli@vX.Y.Z && go mod tidy)
```

After:
```bash
grep vault-cli watcher/github-pr/go.mod
```
Expected: shows TARGET_VERSION.

---

## Step 6 — Update `watcher/github-build/main.go`

File: `watcher/github-build/main.go`

Find the `BuildTaskStatus` field (around line 57). The current struct tag contains `default:"todo"`. Change it to `default:"next"`, preserving all other spacing and tags verbatim:

Before:
```go
BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"TASK_STATUS"   usage:"Frontmatter status for published tasks"                      default:"todo"`
```

After:
```go
BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"TASK_STATUS"   usage:"Frontmatter status for published tasks"                      default:"next"`
```

No other changes to this file.

---

## Step 7 — Update `watcher/github-build/cmd/run-once/main.go`

File: `watcher/github-build/cmd/run-once/main.go`

Find the `BuildTaskStatus` field (around line 39). Same change as Step 6:

Before:
```go
BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"TASK_STATUS"   usage:"Frontmatter status for published tasks"                      default:"todo"`
```

After:
```go
BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"TASK_STATUS"   usage:"Frontmatter status for published tasks"                      default:"next"`
```

---

## Step 8 — Update `agent/pr-reviewer/main.go`

File: `agent/pr-reviewer/main.go`

Find the `Phase` field (around line 69). Change both `default` value and the `usage` string:

Before:
```go
Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | in_progress | ai_review" default:"in_progress"`
```

After:
```go
Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | execution | ai_review" default:"execution"`
```

No other changes to this file.

---

## Step 9 — Update `agent/pr-reviewer/cmd/run-task/main.go`

File: `agent/pr-reviewer/cmd/run-task/main.go`

Find the `Phase` field (around line 58). Identical change to Step 8:

Before:
```go
Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | in_progress | ai_review" default:"in_progress"`
```

After:
```go
Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | execution | ai_review" default:"execution"`
```

---

## Step 10 — Run `make test` in each changed module (fast feedback)

```bash
(cd watcher/github-build && make test)
(cd watcher/github-pr && make test)
(cd agent/pr-reviewer && make test)
```

All must pass. The most likely test failure after a dep bump is a Counterfeiter mock mismatch — the generated mock file has drifted from the bumped interface. If `make test` fails with a diff complaint about generated mock files, regenerate them:

```bash
cd <failing-module> && go generate ./...
```

Then re-run `make test` until it passes. Do not proceed to Step 10b with failing tests.

---

## Step 10b — Validate new constants cross the boundary

The new defaults `"next"` and `"execution"` are typed as `domain.TaskStatus` / `domain.TaskPhase`. Their canonical-set membership is enforced at runtime by `Validate()` (string compared against `AvailableTaskStatuses` / `AvailableTaskPhases`). Confirm the new defaults pass that boundary without needing the binary to start:

```bash
(cd agent/pr-reviewer && cat <<'EOF' > /tmp/validate_smoke.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bborbe/vault-cli/pkg/domain"
)

func main() {
	ctx := context.Background()
	if err := domain.TaskPhase("execution").Validate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "TaskPhase(execution) invalid: %v\n", err)
		os.Exit(1)
	}
	if err := domain.TaskStatus("next").Validate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "TaskStatus(next) invalid: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")
}
EOF
go run /tmp/validate_smoke.go
rm -f /tmp/validate_smoke.go)
```

Expected stdout: `OK`. Exit 0.

If `Validate()` rejects either new value, the chosen vault-cli version is wrong — STOP and report `status: failed`.

---

## Step 11 — Add CHANGELOG entry

Read `CHANGELOG.md` at the repo root. The current latest is `## v0.25.5`. If `## Unreleased` already exists, append to it; otherwise prepend above `## v0.25.5`:

```markdown
## Unreleased

- feat(watcher/github-build,agent/pr-reviewer): bump vault-cli to <TARGET_VERSION>; flip `BuildTaskStatus` default from `"todo"` to `"next"` and agent `Phase` default from `"in_progress"` to `"execution"` so newly published tasks carry the vault-cli canonical taxonomy
```

(Replace `<TARGET_VERSION>` with the actual version string.)

---

## Step 12 — Run `make precommit` in each changed module

Run each in a subshell so subsequent runs are not blocked by a prior failure:

```bash
(cd watcher/github-build && make precommit)
(cd watcher/github-pr    && make precommit)
(cd agent/pr-reviewer    && make precommit)
```

Each must exit 0. If a module fails:
1. Identify the specific failing target from the output (e.g. `make lint`, `make gosec`, `make errcheck`).
2. Fix the issue.
3. Re-run only that target: `cd <module> && make lint` (not the full `make precommit`).
4. Repeat until that target passes.
5. Re-run full `make precommit` for that module once all individual targets pass.

Report the exit code for each module's `make precommit` in the completion report.
</requirements>

<constraints>
- **Gate on Step 1**: if `TaskStatusNext` or `TaskPhaseExecution` are absent from the chosen vault-cli version, report `status: failed` immediately. Do not modify any file.
- **`lib/go.mod` is out of scope** — it does not import vault-cli; do not bump or touch it.
- **`agent/pr-reviewer/cmd/cli/main.go` is out of scope** — it does not declare a Phase or BuildTaskStatus flag.
- **Pin all three modules to the same TARGET_VERSION** — if they land on different versions, that violates the spec constraint.
- **No `fmt.Errorf`** — use `errors.Errorf`/`errors.Wrapf` from `github.com/bborbe/errors` for any new error wrapping.
- **`make precommit` per module, never at repo root** — the root Makefile delegates via `Makefile.folder`; running it there is wasteful and may mask per-module failures.
- **Do NOT commit** — dark-factory handles git.
- **Counterfeiter mocks** — if `go generate ./...` produces a diff after the dep bump, commit that diff as part of this prompt's changes (it is expected churn from interface changes).
- **`watcher/github-pr` frontmatter string literals** (`"status": "in_progress"`, `"status": "todo"` in `buildFrontmatter`) are plain strings in a map, not `domain.TaskStatus`-typed — they are NOT validated by vault-cli's `Validate()` and do not need to change in this prompt.
</constraints>

<verification>
```bash
# 1. Confirm all three modules on the same vault-cli version (TARGET_VERSION)
grep -n 'github.com/bborbe/vault-cli' watcher/github-build/go.mod watcher/github-pr/go.mod agent/pr-reviewer/go.mod
# Expected: three lines, all showing TARGET_VERSION

# 2. Confirm BuildTaskStatus default flipped — no stale "todo" defaults
grep -n 'default:"todo"' watcher/github-build/main.go watcher/github-build/cmd/run-once/main.go
# Expected: 0 matches

grep -n 'default:"next"' watcher/github-build/main.go watcher/github-build/cmd/run-once/main.go
# Expected: 1 match per file (exactly 2 total)

# 3. Confirm Phase default and usage flipped — no stale "in_progress" struct tags
grep -n 'default:"in_progress"' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go
# Expected: 0 matches

grep -n 'default:"execution"' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go
# Expected: 1 match per file (exactly 2 total)

# 4. Confirm usage strings updated
grep -n 'planning | in_progress | ai_review' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go
# Expected: 0 matches

grep -n 'planning | execution | ai_review' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go
# Expected: 1 match per file (exactly 2 total)

# 5. Broad sweep — no surviving flag-default "todo"/"in_progress" in domain-typed flags
grep -rn 'default:"todo"\|default:"in_progress"' watcher/ agent/ --include='*.go' --exclude-dir=vendor
# Visual inspection: any remaining hits should NOT be TaskStatus/TaskPhase-typed flags.
# (GitHub build-status or PR-review-state string fields are out of scope.)

# 6. Confirm CHANGELOG entry
grep -n 'dep.*bump.*vault\|vault.*bump\|next.*execution\|execution.*next' CHANGELOG.md | head -3
# Expected: one match under ## Unreleased

# 7. Final precommit per module
(cd watcher/github-build && make precommit)
(cd watcher/github-pr    && make precommit)
(cd agent/pr-reviewer    && make precommit)
# Expected: exit 0 in each

# 8. Smoke-check binary help (no execution needed in CI — just compilation + grep)
(cd watcher/github-build && go build -o /tmp/gb-bin . && /tmp/gb-bin --help 2>&1 | grep 'build-task-status' || true)
(cd agent/pr-reviewer    && go build -o /tmp/pr-bin  . && /tmp/pr-bin  --help 2>&1 | grep -A1 'phase' || true)
# Expected: first binary shows "next" in usage or default; second shows "execution" and "planning | execution | ai_review"
```
</verification>
