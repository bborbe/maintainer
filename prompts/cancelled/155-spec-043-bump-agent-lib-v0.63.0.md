---
status: cancelled
spec: ["043"]
container: maintainer-exec-155-spec-043-bump-agent-lib-v0-63-0
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-25T00:00:00Z"
queued: "2026-05-25T18:52:04Z"
started: "2026-05-25T19:37:02Z"
completed: "2026-05-25T18:54:39Z"
lastFailReason: 'validate completion report: completion report status: failed'
cancelled: "2026-05-25T19:37:08Z"
---

<summary>
- Bumps `agent/pr-reviewer`'s dependency on `github.com/bborbe/agent/lib` from `v0.62.17` to `v0.63.0`
- Updates the matching `go.sum` checksum entries
- Adds a `chore(agent/pr-reviewer):` bullet under `## Unreleased` in the root `CHANGELOG.md` naming the lib bump
- Verifies the bumped service still builds and tests pass via `make precommit` in the changed service dir
- No `.go` source code changes; no other service is touched; `replace` blocks are preserved byte-for-byte
- Enables (but does not perform) the dev deploy that collapses pr-reviewer's per-phase pod boots into one pod on the happy path
</summary>

<objective>
Update `agent/pr-reviewer/go.mod` and `agent/pr-reviewer/go.sum` to consume `github.com/bborbe/agent/lib v0.63.0`, and document the bump in the root `CHANGELOG.md` under `## Unreleased`. The PR diff must touch exactly three files: `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum`, `CHANGELOG.md`.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` for changelog conventions.

Files to read fully before making changes:
- `agent/pr-reviewer/go.mod` — confirm the current pin is `github.com/bborbe/agent/lib v0.62.17` on a single line inside the first `require (...)` block, and that the `replace (...)` blocks (`opencontainers/runtime-spec`, `bborbe/maintainer/lib`) are present.
- `agent/pr-reviewer/go.sum` — confirm the only `agent/lib` entries are `v0.62.17` (module + `/go.mod` checksums).
- `agent/pr-reviewer/Makefile` — confirm `make precommit` is wired (it inherits `../../Makefile.precommit`).
- `CHANGELOG.md` (repo root) — confirm `## Unreleased` already exists at the top with prior `chore(agent/pr-reviewer):` and `fix(agent/pr-reviewer):` bullets above the `## v0.26.9` heading.

Key facts established from the source files:
- Current pin: `github.com/bborbe/agent/lib v0.62.17` (line 14 of `agent/pr-reviewer/go.mod` at spec time).
- Target pin: `github.com/bborbe/agent/lib v0.63.0`.
- `## Unreleased` is the first section in `CHANGELOG.md` and contains existing bullets. The next version section is `## v0.26.9`.
- `replace` directives in `agent/pr-reviewer/go.mod`:
  - `github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v1.2.0`
  - `github.com/bborbe/maintainer/lib => ../../lib`
  Both must remain byte-for-byte unchanged after the bump.

Parent context: `lib/v0.63.0` (tagged via `bborbe/agent` spec 040) collapses N pod boots (one per phase) into one pod on the happy path by looping phases inside `agentlib.Agent.Run`. This consumer bump is what makes that fix take effect for `pr-reviewer` once the new binary is deployed.
</context>

<requirements>

**Execute steps in order. Each step is a precondition for the next.**

1. **Pre-flight: verify the starting state.** Run:

   ```bash
   cd agent/pr-reviewer
   grep -c 'github.com/bborbe/agent/lib v0.62.17' go.mod
   grep -c 'github.com/bborbe/agent/lib v0.62.17' go.sum
   grep -c 'github.com/bborbe/agent/lib v0.63' go.mod
   grep -c 'github.com/bborbe/agent/lib v0.63' go.sum
   ```

   Expected: first two return `1` and `2` respectively (one go.mod require line; two go.sum lines: module + `/go.mod` checksum). Last two return `0`. If the starting state does not match (e.g. someone already bumped it, or the pin is a different version), STOP and report `status: failed` with the actual state.

2. **Edit `agent/pr-reviewer/go.mod`** — use the `Edit` tool with a surgical match on the version pin. Do NOT rewrite the file. Do NOT touch any other line.

   - `old_string`: `	github.com/bborbe/agent/lib v0.62.17`
   - `new_string`: `	github.com/bborbe/agent/lib v0.63.0`

   The leading whitespace is a tab (matching the rest of the `require (...)` block). The `Edit` tool will fail if the tab is missing or if the line is not unique — it should be unique because this is the only occurrence of that string in the file.

3. **Run `go mod tidy` in the service directory** to update `go.sum` checksums and prune any stale transitive entries:

   ```bash
   cd agent/pr-reviewer
   go mod tidy
   ```

   If this fails with `unknown revision v0.63.0` or any GOPROXY-related error, run `sleep 30 && go mod tidy` (literal sleep, not "wait roughly 30 seconds"). If that still fails, run `GOPROXY=direct go mod tidy`. If all three attempts fail, STOP and report `status: failed` with the tidy output (the lib tag may not yet be propagated to the module proxy).

4. **Revert any unintended `require` version bumps.** `go mod tidy` may bump transitive dependencies beyond `agent/lib`. Compare `go.mod` against the master version:

   ```bash
   cd agent/pr-reviewer
   git diff -- go.mod | grep -E '^[-+]\s+github.com/' | grep -v 'github.com/bborbe/agent/lib'
   ```

   Expected: empty output. If any other `github.com/...` line is changed in the `require` block, restore those lines to their master values (use `git checkout master -- go.mod` to view originals, then re-apply ONLY the `agent/lib v0.62.17 → v0.63.0` change). Then re-run `go mod tidy`.

   **Heuristic for which transitive bumps to accept vs revert** (the executor needs this; do not defer to the reviewer):
   - **Non-bborbe modules** (`golang.org/x/...`, `github.com/google/...`, `gopkg.in/...`, etc.) added or bumped by tidy: **accept**. These are almost certainly in `lib/v0.63.0`'s transitive closure and tidy is doing minimum-version selection correctly.
   - **`github.com/bborbe/*` modules other than `agent/lib` itself** bumped by tidy: **revert** them back to master values. Per spec Constraint #4 the only intentional target is `agent/lib`; if tidy then fails because the older bborbe version is incompatible with `lib/v0.63.0`, STOP and report `status: failed` — this is an upstream compatibility issue, not a fix-in-place case.

   For non-`github.com/bborbe/` modules (e.g. `golang.org/x/...`, `k8s.io/...`, etc.) — if tidy bumps them, accept that bump only if it is required by the `lib/v0.63.0` upgrade. If unsure, keep the master pin.

5. **Verify `replace` blocks are unchanged.** Run:

   ```bash
   cd agent/pr-reviewer
   git diff -- go.mod | awk '/^[-+].*replace/,/^[-+].*\)/' | head -40
   ```

   Expected: no `-` or `+` line touching anything inside either `replace (...)` block. If tidy rewrote a replace directive, STOP and report `status: failed` with the diff — this indicates a module-path drift in `lib/v0.63.0` and must be escalated, not silently accepted.

6. **Verify go.sum has the new checksums and no stale ones.** Run:

   ```bash
   cd agent/pr-reviewer
   grep 'github.com/bborbe/agent/lib v0.63' go.sum
   grep 'github.com/bborbe/agent/lib v0.62' go.sum
   ```

   Expected: the first grep returns at least 2 lines (module checksum + `/go.mod` checksum). The second grep returns 0 lines. If either expectation fails, re-run `go mod tidy`; if it still fails, delete `go.sum` and re-run `go mod tidy`. If that also fails, STOP and report `status: failed`.

7. **Run `go mod verify`** to confirm module integrity:

   ```bash
   cd agent/pr-reviewer
   go mod verify
   ```

   Must exit 0. If it fails, STOP and report `status: failed` with the output.

8. **Run `make precommit` in the service directory** (NOT at the repo root — repo convention forbids root precommit):

   ```bash
   cd agent/pr-reviewer
   make precommit
   ```

   Must exit 0. This covers `go build`, `go vet`, `go test`, and any repo-specific lint/format gates. If it fails, the failure must be reported verbatim — do NOT attempt to "fix" a test failure by editing `.go` source under `agent/pr-reviewer/` (this spec forbids `.go` edits). If the failure is a `lib/v0.63.0` API drift, STOP and report `status: failed` so the human reviewer can escalate to the agent repo.

9. **Update `CHANGELOG.md` at the repo root.** Use the `Edit` tool with a surgical anchor — do NOT rewrite the file. Append a new bullet under the existing `## Unreleased` section. The existing first bullet under `## Unreleased` (at spec time) starts with `- fix(agent/pr-reviewer): every pr-review step now publishes its routing decision`. Use the `## Unreleased` header line as the anchor and prepend the new bullet immediately below it:

   - `old_string`:
     ```
     ## Unreleased

     - fix(agent/pr-reviewer): every pr-review step now publishes
     ```
   - `new_string`:
     ```
     ## Unreleased

     - chore(agent/pr-reviewer): bump `github.com/bborbe/agent/lib` from v0.62.17 to v0.63.0 to collapse multi-phase pod boots into one pod on the happy path (lib spec 040); pr-reviewer's 3-phase chain now runs in a single pod boot once the new binary is deployed.

     - fix(agent/pr-reviewer): every pr-review step now publishes
     ```

   If the existing first bullet's text has changed since spec time (i.e. the `old_string` above is no longer unique or no longer matches), use a different unique anchor that still places the new bullet INSIDE the `## Unreleased` section (above the `## v0.26.9` heading and below the `## Unreleased` heading). Never read+rewrite the whole CHANGELOG.

10. **Final verification of the three-file constraint.** Run:

    ```bash
    cd /workspace
    git diff --name-only
    ```

    (Adapt path to the actual working directory — the container path may differ from the host path.) Expected: exactly three lines, in any order:
    ```
    CHANGELOG.md
    agent/pr-reviewer/go.mod
    agent/pr-reviewer/go.sum
    ```

    If any other file appears in the diff, revert it (`git checkout -- <file>`) unless it is a genuinely-required transitive change forced by `lib/v0.63.0` (in which case STOP and report — this spec says only three files change).

11. **Verify no `.go` source under `agent/pr-reviewer/` is in the diff:**

    ```bash
    git diff --name-only | grep -E '^agent/pr-reviewer/.*\.go$'
    ```

    Expected: empty output. If any `.go` file appears, revert it.

</requirements>

<constraints>
- ONLY modify `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum`, and root `CHANGELOG.md`. No other files.
- Do NOT modify any `.go` file under `agent/pr-reviewer/`. `go mod tidy` does not touch `.go` files, but if any `.go` file appears in the diff for any reason, revert it.
- Do NOT modify `replace` blocks in `agent/pr-reviewer/go.mod`. If `go mod tidy` rewrites a replace directive, treat as failure and STOP.
- Do NOT bump any other top-level `require` version beyond `github.com/bborbe/agent/lib`. If tidy proposes additional `github.com/bborbe/*` bumps, revert them. For non-`github.com/bborbe/` transitive bumps, keep them only if required by `lib/v0.63.0`.
- Do NOT run `make precommit` at the repo root. Run it ONLY from `agent/pr-reviewer/`. Repo convention: root precommit is hard-blocked.
- Do NOT touch `~/Documents/workspaces/maintainer-dev/` or `~/Documents/workspaces/maintainer-prod/`. All work happens in the development worktree.
- Do NOT run `make buca` or any deploy command. The dev deploy is a manual operator step outside this PR.
- Do NOT introduce any feature flag, env var, or per-deploy opt-out for "old vs new" lib behavior. The bump is unconditional.
- Do NOT bump any other `agent/lib` consumer (e.g. `agent/claude`, `agent/code`, `agent/gemini`) if they exist in this repo. Each consumer bump is its own PR.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass under `lib/v0.63.0`. If a test fails, do NOT edit the test or the source under test to make it pass — STOP and report.
- The `chore(agent/pr-reviewer):` CHANGELOG bullet must explicitly name the version `v0.63.0` so the bump is greppable.
</constraints>

<verification>
# 1. go.mod has exactly one v0.63.0 line and zero v0.62 lines for agent/lib:
grep -c 'github.com/bborbe/agent/lib v0.63.0' agent/pr-reviewer/go.mod
# Expected: 1
grep -c 'github.com/bborbe/agent/lib v0.62' agent/pr-reviewer/go.mod
# Expected: 0

# 2. go.sum has v0.63.0 entries and no v0.62 entries for agent/lib:
grep -c 'github.com/bborbe/agent/lib v0.63' agent/pr-reviewer/go.sum
# Expected: 2 (module + /go.mod checksum)
grep -c 'github.com/bborbe/agent/lib v0.62' agent/pr-reviewer/go.sum
# Expected: 0

# 3. Module integrity:
cd agent/pr-reviewer && go mod verify
# Expected: exit 0, "all modules verified"

# 4. Service builds and tests pass:
cd agent/pr-reviewer && make precommit
# Expected: exit 0

# 5. Exactly three files changed (run from repo root):
git diff --name-only | sort
# Expected (exactly three lines):
#   CHANGELOG.md
#   agent/pr-reviewer/go.mod
#   agent/pr-reviewer/go.sum

# 6. No .go file in the diff:
git diff --name-only | grep -E '^agent/pr-reviewer/.*\.go$' | wc -l
# Expected: 0

# 7. replace blocks unchanged:
git diff -- agent/pr-reviewer/go.mod | awk '/replace \(/,/^\)/' | grep -E '^[-+]' | wc -l
# Expected: 0

# 8. CHANGELOG has the new bullet under ## Unreleased, naming v0.63.0:
awk '/^## Unreleased/{flag=1;next} /^## /{flag=0} flag' CHANGELOG.md | grep -E '^- chore\(agent/pr-reviewer\):' | grep -c 'v0.63.0'
# Expected: >=1

# 9. Other github.com/bborbe/ require pins unchanged:
git diff -- agent/pr-reviewer/go.mod | grep -E '^[-+]\s+github.com/bborbe/' | grep -v 'github.com/bborbe/agent/lib' | wc -l
# Expected: 0
</verification>

<!--
REVIEWER NOTES (for /audit-prompt — surfaced because non-interactive mode):

1. Step 4 (revert unintended bumps) has a soft judgment call: "keep [non-bborbe transitive bumps] only if required by lib/v0.63.0". The agent cannot reliably know what lib/v0.63.0 "requires" without inspecting that lib's go.mod. Practical heuristic for the executing agent: if `go mod tidy` adds/changes a non-bborbe module after the lib bump, that module is almost certainly in lib/v0.63.0's transitive closure — accept the change. If it's a bborbe module the agent did not explicitly target, revert. The spec's Constraint #4 reinforces "only github.com/bborbe/agent/lib is the intentional target".

2. Step 9 (CHANGELOG edit) anchors on the current first Unreleased bullet ("every pr-review step now publishes..."). If new spec runs land before this one and shift the first bullet, the Edit will fail and the agent needs to pick a fresh anchor. The fallback instruction is included.

3. The spec's AC has a `grep -c 'github.com/bborbe/agent/lib v0.63' agent/pr-reviewer/go.sum returns >=1`. The verification block here asserts `== 2` because tidy always writes both the module checksum and the /go.mod checksum. If a future Go toolchain changes this convention, relax to `>=1`.

4. Non-interactive mode resolution: the spec is fully self-contained — no clarifying questions surfaced. Branch name comes from spec frontmatter (`dark-factory/bump-agent-pr-reviewer-to-agent-lib-v0-63-0`) and is owned by dark-factory CLI, not this prompt.
-->
