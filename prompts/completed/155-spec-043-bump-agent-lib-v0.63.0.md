---
status: completed
spec: [043-bump-agent-pr-reviewer-to-agent-lib-v0.63.0]
summary: 'Bumped agent/pr-reviewer dependency on github.com/bborbe/agent/lib from v0.62.17 to v0.63.0, updated factory_test.go assertion to match lib v0.62.29 behavior change (needs_input no longer writes phase: human_review), documented in root CHANGELOG.md'
container: maintainer-exec-155-spec-043-bump-agent-lib-v0-63-0
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-25T00:00:00Z"
queued: "2026-05-25T19:45:24Z"
started: "2026-05-25T19:45:26Z"
completed: "2026-05-25T19:52:08Z"
---

<summary>
- Bumps `agent/pr-reviewer`'s dependency on `github.com/bborbe/agent/lib` from `v0.62.17` to `v0.63.0`
- Updates the matching `go.sum` checksum entries
- Adds a `chore(agent/pr-reviewer):` bullet under `## Unreleased` in the root `CHANGELOG.md` naming the lib bump
- Verifies the bumped service still builds and tests pass via `make precommit` in the changed service dir
- Permits surgical `*_test.go` edits ONLY for test assertions that break because the lib's CHANGELOG between v0.62.18 and v0.63.0 documents a deliberate behavior change (each edit must carry an inline CHANGELOG-version comment)
- No production `.go` source code changes; no other service is touched; `replace` blocks are preserved byte-for-byte
- Enables (but does not perform) the dev deploy that collapses pr-reviewer's per-phase pod boots into one pod on the happy path
</summary>

<objective>
Update `agent/pr-reviewer/go.mod` and `agent/pr-reviewer/go.sum` to consume `github.com/bborbe/agent/lib v0.63.0`, document the bump in the root `CHANGELOG.md` under `## Unreleased`, and — only where tests break because of an upstream behavior change documented in `lib/CHANGELOG.md` between `v0.62.18` and `v0.63.0` — update the failing test assertions to match the new behavior with an inline CHANGELOG-version comment. No production `.go` source files change.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` for changelog conventions.

Files to read fully before making changes:
- `agent/pr-reviewer/go.mod` — confirm the current pin is `github.com/bborbe/agent/lib v0.62.17` on a single line inside the first `require (...)` block, and that the `replace (...)` blocks (`opencontainers/runtime-spec`, `bborbe/maintainer/lib`) are present.
- `agent/pr-reviewer/go.sum` — confirm the only `agent/lib` entries are `v0.62.17` (module + `/go.mod` checksums).
- `agent/pr-reviewer/Makefile` — confirm `make precommit` is wired (it inherits `../../Makefile.precommit`).
- `agent/pr-reviewer/pkg/factory/factory_test.go` — note the assertion at the `Context("when result status is needs_input with Output containing frontmatter", ...)` block: `Expect(generated).To(ContainSubstring("phase: human_review"))`. This will break against `lib/v0.63.0` because v0.62.27/v0.62.29 deliberately removed the `phase: human_review` write on `AgentStatusNeedsInput` / `AgentStatusFailed`.
- `CHANGELOG.md` (repo root) — confirm `## Unreleased` already exists at the top with prior `chore(agent/pr-reviewer):` and `fix(agent/pr-reviewer):` bullets above the `## v0.26.9` heading.

Upstream CHANGELOG (the authority for "is this test breakage justified by an upstream behavior change?"):
- The lib's CHANGELOG is the local agent-repo working copy if available, OR the source-of-truth `github.com/bborbe/agent` CHANGELOG. Inside the container, the lib CHANGELOG entries can also be read from the module cache after `go mod download`: `$(go env GOMODCACHE)/github.com/bborbe/agent/lib@v0.63.0/CHANGELOG.md`. Failing that, the entries listed below are the AUTHORITATIVE excerpts at spec-amendment time (2026-05-25):

  - **v0.62.27** — "fix(lib/delivery): `applyStatusFrontmatter` no longer writes `phase: human_review` on `AgentStatusNeedsInput` or `AgentStatusFailed`; clears `assignee` and preserves existing phase instead"
  - **v0.62.28** — "docs: update `docs/task-flow-and-failure-semantics.md` to reflect spec-039 doctrine: `phase: human_review` is reserved for agent-emitted `Result.NextPhase` handoffs; controller-side failure paths leave phase unchanged and clear assignee instead"
  - **v0.62.29** — "fix(lib/delivery): stop writing `phase: human_review` on `AgentStatusNeedsInput` and `AgentStatusFailed`/default branches in `result-deliverer` and `content-generator`; phase now reflects the lifecycle stage and only `assignee` is cleared (completes spec-021 escalation doctrine)"

  These three entries collectively justify dropping any test assertion of the literal string `phase: human_review` on `AgentStatusNeedsInput` or `AgentStatusFailed` result paths.

Key facts established from the source files:
- Current pin: `github.com/bborbe/agent/lib v0.62.17` (line 14 of `agent/pr-reviewer/go.mod` at spec time).
- Target pin: `github.com/bborbe/agent/lib v0.63.0`.
- `## Unreleased` is the first section in `CHANGELOG.md` and contains existing bullets. The next version section is `## v0.26.9`.
- `replace` directives in `agent/pr-reviewer/go.mod`:
  - `github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v1.2.0`
  - `github.com/bborbe/maintainer/lib => ../../lib`
  Both must remain byte-for-byte unchanged after the bump.

Parent context: `lib/v0.63.0` (tagged via `bborbe/agent` spec 040) collapses N pod boots (one per phase) into one pod on the happy path by looping phases inside `agentlib.Agent.Run`. This consumer bump is what makes that fix take effect for `pr-reviewer` once the new binary is deployed.

Known failing test (verified from source at spec time):
- `agent/pr-reviewer/pkg/factory/factory_test.go` — block `Describe("Passthrough content generator wiring — failure body")` → `Context("when result status is needs_input with Output containing frontmatter")` → `It("sets phase: human_review in frontmatter and writes ## Failure")`. The assertion `Expect(generated).To(ContainSubstring("phase: human_review"))` will fail under `lib/v0.63.0` because of upstream CHANGELOG v0.62.29 (and v0.62.27). The `## Failure` body write and the message-substring assertion remain valid; only the `phase: human_review` line is the upstream-removed behavior.
- There may be additional `*_test.go` assertions in the same package that depend on the removed behavior. The procedure below covers discovery and surgical edits for all of them.
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

8. **First `make precommit` attempt** in the service directory (NOT at the repo root — repo convention forbids root precommit):

   ```bash
   cd agent/pr-reviewer
   make precommit 2>&1 | tee /tmp/precommit-1.log
   ```

   - If exit 0 → proceed to step 11 (CHANGELOG entry). No test edits needed.
   - If exit non-zero → proceed to step 9 (test-failure triage). Capture the full failure output.

9. **Triage test failures against the upstream CHANGELOG.** For each failing test:

   a. Identify the failing assertion. From the `make precommit` output, locate the test file (`*_test.go`) and the `Expect(...)` / `Fail(...)` / `t.Fatal(...)` line that triggered the failure.

   b. Determine the cause. Inspect the lib's CHANGELOG to see whether the asserted behavior is documented as deliberately removed/changed between `v0.62.18` and `v0.63.0`. Authoritative source lookup order:
      1. `$(go env GOMODCACHE)/github.com/bborbe/agent/lib@v0.63.0/CHANGELOG.md` (after `go mod download github.com/bborbe/agent/lib@v0.63.0`).
      2. If the module cache CHANGELOG is missing, use the excerpts inlined in the `<context>` block above.

   c. Classify the failure into exactly one of these buckets:
      - **Upstream-justified test break**: a lib CHANGELOG entry in the `v0.62.18 .. v0.63.0` range explicitly describes removing or changing the asserted behavior. Permitted action: edit the test assertion (see step 10).
      - **Other failure**: anything else — compile error in production code, API drift not documented in the CHANGELOG, lint/vet failure, panic in non-test code, timeout, etc. Forbidden action: do NOT edit production `.go` files or change test logic beyond what (a) demands. STOP and report `status: failed` with: (i) the failing test name and file, (ii) the assertion text, (iii) why it does not fall under "upstream-justified" (e.g. "no CHANGELOG entry between v0.62.18 and v0.63.0 mentions this symbol/behavior").

   d. The known case at spec-amendment time:
      - File: `agent/pr-reviewer/pkg/factory/factory_test.go`
      - Block: `Describe("Passthrough content generator wiring — failure body")` → `Context("when result status is needs_input with Output containing frontmatter")` → `It("sets phase: human_review in frontmatter and writes ## Failure", ...)`
      - Failing assertion: `Expect(generated).To(ContainSubstring("phase: human_review"))`
      - Justifying CHANGELOG entry: **lib v0.62.29** — "stop writing `phase: human_review` on `AgentStatusNeedsInput` and `AgentStatusFailed`/default branches in `result-deliverer` and `content-generator`". (Earlier kin: v0.62.27.)
      - Classification: **upstream-justified test break**. Permitted.

10. **Surgical test edits for upstream-justified breaks.** For each failure classified as upstream-justified in step 9c:

    a. Use the `Edit` tool. Do NOT rewrite the whole file. Match the smallest unique snippet that contains the broken assertion.

    b. Edit policy:
       - DELETE the assertion line that depends on the upstream-removed behavior (e.g. the `Expect(generated).To(ContainSubstring("phase: human_review"))` line).
       - Do NOT change the surrounding assertions that remain valid (e.g. `ContainSubstring("## Failure")`, `ContainSubstring(<message>)`).
       - If removing the line empties the `It(...)` block, also update the `It` description to match what is now actually being asserted (e.g. `"writes ## Failure with the message"`).
       - Add an inline Go comment IMMEDIATELY ABOVE the deleted-or-modified region naming the upstream CHANGELOG version that justifies the change. Format exactly:
         ```go
         // updated for lib v0.62.29: needs_input no longer writes phase: human_review in passthrough content generator (see github.com/bborbe/agent/lib CHANGELOG v0.62.27 / v0.62.29)
         ```
         Substitute the actual CHANGELOG version(s) that justify the edit if a different test is involved. The comment is mandatory — it is the auditable trail proving the test edit is intentional and traceable.

    c. Concrete edit for the known case (use this exact `Edit` call shape):

       - File: `agent/pr-reviewer/pkg/factory/factory_test.go`
       - `old_string`:
         ```go
       Context("when result status is needs_input with Output containing frontmatter", func() {
           It("sets phase: human_review in frontmatter and writes ## Failure", func() {
               result := agentlib.AgentResultInfo{
                   Status:  agentlib.AgentStatusNeedsInput,
                   Message: "GH_TOKEN unauthorized (HTTP 401)",
                   Output:  "---\nstatus: in_progress\nphase: planning\n---\n",
               }
               generated, err := gen.Generate(ctx, "", result)
               Expect(err).NotTo(HaveOccurred())
               Expect(generated).To(ContainSubstring("## Failure"))
               Expect(generated).To(ContainSubstring("GH_TOKEN unauthorized (HTTP 401)"))
               Expect(generated).To(ContainSubstring("phase: human_review"))
           })
       })
         ```
       - `new_string`:
         ```go
       // updated for lib v0.62.29: needs_input no longer writes phase: human_review in passthrough content generator (see github.com/bborbe/agent/lib CHANGELOG v0.62.27 / v0.62.29)
       Context("when result status is needs_input with Output containing frontmatter", func() {
           It("writes ## Failure with the message and preserves existing phase", func() {
               result := agentlib.AgentResultInfo{
                   Status:  agentlib.AgentStatusNeedsInput,
                   Message: "GH_TOKEN unauthorized (HTTP 401)",
                   Output:  "---\nstatus: in_progress\nphase: planning\n---\n",
               }
               generated, err := gen.Generate(ctx, "", result)
               Expect(err).NotTo(HaveOccurred())
               Expect(generated).To(ContainSubstring("## Failure"))
               Expect(generated).To(ContainSubstring("GH_TOKEN unauthorized (HTTP 401)"))
           })
       })
         ```
       Indentation note: the snippet above uses tabs (Go convention). Use tabs in the Edit tool input. If your tooling shows the literal whitespace as spaces above, convert to tabs before issuing the Edit call. If the Edit fails on `old_string` not being unique or not matching whitespace, re-read the file with the Read tool and adjust the `old_string` to the exact bytes on disk before retrying.

    d. After all upstream-justified test edits are in place, **re-run `make precommit`**:

       ```bash
       cd agent/pr-reviewer
       make precommit 2>&1 | tee /tmp/precommit-2.log
       ```

       - If exit 0 → proceed to step 11.
       - If exit non-zero → STOP and report `status: failed`. Include in the report: (i) the diff of test edits made so far, (ii) the new failure output, (iii) the reason this remaining failure is NOT upstream-justified (i.e. no CHANGELOG entry between v0.62.18 and v0.63.0 covers it). Do NOT iterate further test edits to chase a green build — a second-round failure indicates either incomplete CHANGELOG coverage, a production-code break in the lib (escalate to the agent repo), or a test asserting an undocumented behavior that nobody promised. None of those are this prompt's problem to silently paper over.

11. **Update `CHANGELOG.md` at the repo root.** Use the `Edit` tool with a surgical anchor — do NOT rewrite the file. Insert a new bullet at the top of the existing `## Unreleased` section, immediately below the `## Unreleased` heading and above the current first bullet.

    Recommended anchor strategy:
    - Read the first ~30 lines of `CHANGELOG.md` to find the literal text of the current first bullet under `## Unreleased`.
    - Use `Edit` with `old_string` consisting of the `## Unreleased` header + the blank line + the first ~80 chars of the current first bullet, and `new_string` inserting the new `chore(agent/pr-reviewer):` bullet between the header and the existing first bullet.

    Bullet text (exact):
    ```
    - chore(agent/pr-reviewer): bump `github.com/bborbe/agent/lib` from v0.62.17 to v0.63.0 to collapse multi-phase pod boots into one pod on the happy path (lib spec 040); pr-reviewer's 3-phase chain now runs in a single pod boot once the new binary is deployed.
    ```

    If test edits were made in step 10, append a second sentence to the bullet naming the test-assertion update, e.g.:
    ```
    - chore(agent/pr-reviewer): bump `github.com/bborbe/agent/lib` from v0.62.17 to v0.63.0 to collapse multi-phase pod boots into one pod on the happy path (lib spec 040); pr-reviewer's 3-phase chain now runs in a single pod boot once the new binary is deployed. Test assertions updated to match lib v0.62.27/v0.62.29 behavior change (`needs_input` / `failed` no longer write `phase: human_review`).
    ```

    Never read+rewrite the whole CHANGELOG. Use `Edit` only.

12. **Final verification of the bounded-diff constraint.** Run from repo root:

    ```bash
    git diff --name-only | sort
    ```

    Expected: exactly the three "core" files PLUS any `*_test.go` files whose edits are justified by step 10. Allowed set:
    - `CHANGELOG.md`
    - `agent/pr-reviewer/go.mod`
    - `agent/pr-reviewer/go.sum`
    - zero or more `agent/pr-reviewer/**/*_test.go` files (each carrying the inline CHANGELOG-version comment from step 10c)

    If any file outside this allowed set appears, revert it (`git checkout -- <file>`). If reverting would re-break precommit, STOP and report `status: failed` — that means a production source file was touched and this prompt forbids that.

13. **Verify no production `.go` source under `agent/pr-reviewer/` is in the diff:**

    ```bash
    git diff --name-only | grep -E '^agent/pr-reviewer/.*\.go$' | grep -v '_test\.go$'
    ```

    Expected: empty output. If any non-test `.go` file appears, revert it. If that re-breaks precommit, STOP and report `status: failed`.

14. **Verify every changed `*_test.go` file carries the inline CHANGELOG-version comment:**

    ```bash
    for f in $(git diff --name-only | grep -E '^agent/pr-reviewer/.*_test\.go$'); do
        if ! grep -E 'updated for lib v0\.6[23]\.[0-9]+' "$f"; then
            echo "MISSING COMMENT: $f"
        fi
    done
    ```

    Expected: no `MISSING COMMENT:` lines. If any test file is in the diff without the inline comment, add the comment via `Edit` per the format in step 10b before declaring done.

</requirements>

<constraints>
- ONLY modify `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum`, root `CHANGELOG.md`, and `*_test.go` files under `agent/pr-reviewer/` that fail under `lib/v0.63.0` due to an upstream behavior change documented in the lib CHANGELOG between `v0.62.18` and `v0.63.0`. No other files.
- **No production `.go` edits.** `*_test.go` edits are permitted only to match an upstream behavior change with an inline `// updated for lib vX.Y.Z: ...` comment. Any change to a non-`_test.go` Go file under `agent/pr-reviewer/` is forbidden — if `go mod tidy` somehow touches one, revert it.
- Do NOT modify `replace` blocks in `agent/pr-reviewer/go.mod`. If `go mod tidy` rewrites a replace directive, treat as failure and STOP.
- Do NOT bump any other top-level `require` version beyond `github.com/bborbe/agent/lib`. If tidy proposes additional `github.com/bborbe/*` bumps, revert them. For non-`github.com/bborbe/` transitive bumps, keep them only if required by `lib/v0.63.0`.
- Do NOT "fix" a failing test by adding new assertions, changing the System Under Test, or rewriting the test logic. The only permitted test edit is: REMOVE (or relax) an assertion that depends on a behavior the lib's CHANGELOG explicitly documents as removed/changed in the v0.62.18..v0.63.0 range, AND add the inline CHANGELOG-version comment naming the version that justifies the removal.
- Do NOT run `make precommit` at the repo root. Run it ONLY from `agent/pr-reviewer/`. Repo convention: root precommit is hard-blocked.
- Do NOT touch `~/Documents/workspaces/maintainer-dev/` or `~/Documents/workspaces/maintainer-prod/`. All work happens in the development worktree.
- Do NOT run `make buca` or any deploy command. The dev deploy is a manual operator step outside this PR.
- Do NOT introduce any feature flag, env var, or per-deploy opt-out for "old vs new" lib behavior. The bump is unconditional.
- Do NOT bump any other `agent/lib` consumer (e.g. `agent/claude`, `agent/code`, `agent/gemini`) if they exist in this repo. Each consumer bump is its own PR.
- Do NOT commit — dark-factory handles git.
- If `make precommit` fails after the test-edit pass for ANY reason other than an upstream-CHANGELOG-justified test break, STOP and report `status: failed`. Do not iterate.
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
# Expected: >=2 (module + /go.mod checksum)
grep -c 'github.com/bborbe/agent/lib v0.62' agent/pr-reviewer/go.sum
# Expected: 0

# 3. Module integrity:
cd agent/pr-reviewer && go mod verify
# Expected: exit 0, "all modules verified"

# 4. Service builds and tests pass:
cd agent/pr-reviewer && make precommit
# Expected: exit 0

# 5. Diff is bounded to the allowed set (run from repo root):
git diff --name-only | sort
# Expected: only files matching this regex set:
#   CHANGELOG.md
#   agent/pr-reviewer/go.mod
#   agent/pr-reviewer/go.sum
#   agent/pr-reviewer/.*_test\.go      (zero or more, each with the inline CHANGELOG comment)

# 6. No production .go file in the diff:
git diff --name-only | grep -E '^agent/pr-reviewer/.*\.go$' | grep -v '_test\.go$' | wc -l
# Expected: 0

# 7. Every changed *_test.go file carries the inline CHANGELOG-version comment:
for f in $(git diff --name-only | grep -E '^agent/pr-reviewer/.*_test\.go$'); do
    grep -E 'updated for lib v0\.6[23]\.[0-9]+' "$f" >/dev/null || echo "MISSING COMMENT: $f"
done
# Expected: no output

# 8. replace blocks unchanged:
git diff -- agent/pr-reviewer/go.mod | awk '/replace \(/,/^\)/' | grep -E '^[-+]' | wc -l
# Expected: 0

# 9. CHANGELOG has the new bullet under ## Unreleased, naming v0.63.0:
awk '/^## Unreleased/{flag=1;next} /^## /{flag=0} flag' CHANGELOG.md | grep -E '^- chore\(agent/pr-reviewer\):' | grep -c 'v0.63.0'
# Expected: >=1

# 10. Other github.com/bborbe/ require pins unchanged:
git diff -- agent/pr-reviewer/go.mod | grep -E '^[-+]\s+github.com/bborbe/' | grep -v 'github.com/bborbe/agent/lib' | wc -l
# Expected: 0

# 11. Over-deletion guard: surviving assertions still present in factory_test.go (catches
# the case where the agent deletes more than just the upstream-removed assertion).
# These two substrings are independent of the phase: human_review line and must remain.
grep -c 'ContainSubstring("## Failure")' agent/pr-reviewer/pkg/factory/factory_test.go
# Expected: 3 (master count — must remain unchanged; over-deletion would drop this)
grep -c 'GH_TOKEN unauthorized (HTTP 401)' agent/pr-reviewer/pkg/factory/factory_test.go
# Expected: 4 (master count — must remain unchanged; over-deletion would drop this)
</verification>
