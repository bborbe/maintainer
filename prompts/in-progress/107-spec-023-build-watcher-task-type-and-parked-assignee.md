---
status: committing
spec: [023-build-watcher-task-type-and-parked-assignee]
summary: 'Added task_type: build-fix to all emitted task frontmatter in buildCreateTaskCommand and added translateAssignee helper that converts human to empty string; updated 5 existing test assertions and added 3 new It blocks covering human→"" translation at both watcher and per-repo maintenance override levels.'
container: maintainer-107-spec-023-build-watcher-task-type-and-parked-assignee
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-10T21:00:00Z"
queued: "2026-05-10T21:16:52Z"
started: "2026-05-10T21:16:54Z"
branch: dark-factory/build-watcher-task-type-and-parked-assignee
---

<summary>
- Every task emitted by the github-build watcher now carries `task_type: build-fix` — operators can route by task type without inspecting the task body.
- When the resolved assignee (after per-repo maintenance override coalescence) equals the literal string `human`, the emitted frontmatter carries `assignee: ""` (explicit empty string) — conforming to the cross-repo doctrine that empty assignee is the operator-inbox signal.
- Any other resolved assignee value (e.g. `build-fixer-agent`, `build-fix-planner`, `go-deps-fixer-agent`) flows through to the emitted frontmatter unchanged.
- A small private helper `translateAssignee` encapsulates the `human` → `""` rule at the emission site inside `buildCreateTaskCommand` — no change to the constructor, factory wiring, or CLI flag names.
- Unit tests cover: (a) `task_type: build-fix` in the default path, (b) watcher configured with `assignee=human` translates to `""`, (c) watcher configured with `assignee=build-fix-planner` flows through unchanged, (d) per-repo maintenance override `assignee: human` translates to `""` after resolution.
- All previously passing tests continue to pass; new assertions on `task_type` are added where appropriate without weakening existing expectations.
- Change is confined to `watcher/github-build/pkg/watcher.go` and `watcher/github-build/pkg/watcher_test.go` — no other file is touched.
</summary>

<objective>
Add `task_type: build-fix` to every task frontmatter emitted by the github-build watcher, and translate the resolved assignee from `"human"` to `""` at the emission site, so all emitted `CreateTaskCommand` payloads conform to the 2026-05-10 cross-repo doctrine: `task_type` is the routing primitive and empty `assignee` is the operator-inbox signal for parked tasks.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, counterfeiter mocks, coverage ≥80%.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern.
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write for each code change.

**Files to read fully before making any changes:**

- `watcher/github-build/pkg/watcher.go` — full file; understand `buildCreateTaskCommand` (the only emission site) and `applyStateMachine` (where `effectiveAssignee` is resolved via `coalesceString`).
- `watcher/github-build/pkg/watcher_test.go` — full file; understand every `It` block that asserts `cmd.Frontmatter` keys — these must be updated to pin `task_type` and the new `assignee` semantics.

**Doctrine reference (read before implementing):**

The cross-repo doctrine values are fixed constants (do not invent alternatives):

| Situation | `task_type` | `assignee` |
|---|---|---|
| Red build, watcher configured with any non-`human` agent name | `build-fix` | the agent name (unchanged) |
| Red build, resolved assignee equals `"human"` (from CLI/env or per-repo override) | `build-fix` | `""` (explicit empty string — key must be present) |

`assignee: ""` is intentional and must appear as an explicit key in the map — it is NOT the same as omitting the key. The operator inbox filters on `assignee == ""`.

**Single emission site:** Unlike the github-pr watcher, the github-build watcher has one emission site — `buildCreateTaskCommand`. Both changes (`task_type` addition and `human` → `""` translation) go inside that function. No changes needed to `applyStateMachine` or any other function.

**Assignee resolution order:** `effectiveAssignee = coalesceString(overrides.Assignee, w.assignee)` (in `applyStateMachine`). The `human` → `""` translation must run on the final resolved value — AFTER coalescence — inside `buildCreateTaskCommand`. This ensures the translation applies equally whether `"human"` came from the CLI flag or from a per-repo maintenance override.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 3. Run `make precommit` only at the final step.**

1. **Add `translateAssignee` helper to `watcher/github-build/pkg/watcher.go`**

   Add the following private helper function anywhere after the `coalesceString` helper (around line 410). It is the single authoritative translation point for the `human` → `""` doctrine:

   ```go
   // translateAssignee converts the magic string "human" to the empty string,
   // which the operator-inbox convention treats as "unclaimed / needs human attention".
   // Any other value is returned unchanged.
   func translateAssignee(a string) string {
       if a == "human" {
           return ""
       }
       return a
   }
   ```

2. **Update `buildCreateTaskCommand` in `watcher/github-build/pkg/watcher.go`**

   In `buildCreateTaskCommand` (around line 295), change the frontmatter construction from:

   ```go
   fm := agentlib.TaskFrontmatter{
       "assignee":    assignee,
       "repo":        owner + "/" + repo,
       "episode_sha": episodeSHA,
       "status":      taskStatus,
   }
   ```

   to:

   ```go
   fm := agentlib.TaskFrontmatter{
       "task_type":   "build-fix",
       "assignee":    translateAssignee(assignee),
       "repo":        owner + "/" + repo,
       "episode_sha": episodeSHA,
       "status":      taskStatus,
   }
   ```

   The `if taskPhase != "" { fm["phase"] = taskPhase }` block that follows stays unchanged.

3. **Run `make test`** from `watcher/github-build/` to identify failing assertions:

   ```bash
   cd watcher/github-build && make test
   ```

   No existing test uses `"human"` as the assignee value, so no existing `assignee` assertion will break. Tests that assert on `cmd.Frontmatter` but do not yet assert on `task_type` will still pass — they do not assert absence of extra keys. Proceed to step 4.

4. **Update `watcher/github-build/pkg/watcher_test.go` — add `task_type` assertions to existing tests**

   For each of the following existing `It` blocks, add `Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))` alongside existing `Frontmatter` assertions. Do NOT remove any existing assertion.

   a. **"treats cold start as green and publishes on first red"** (first `It` under `Describe("Poll")`):
      After the existing `Expect(cmd.Frontmatter["assignee"]).To(Equal("build-fixer-agent"))` line, add:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))
      ```

   b. **"sets assignee to build-fixer-agent in the task command"** (under `Context("assignee contract")`):
      After the existing `Expect(cmd.Frontmatter["assignee"]).To(Equal("build-fixer-agent"))` line, add:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))
      ```

   c. **"uses custom assignee and status when set"** (under `Describe("configurable frontmatter")`):
      After the existing `Expect(cmd.Frontmatter["assignee"]).To(Equal("other-agent"))` line, add:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))
      ```

   d. **"uses watcher defaults when maintenance file returns empty config"** (under `Describe("per-repo maintenance overrides")`):
      After the existing `Expect(cmd.Frontmatter["assignee"]).To(Equal("build-fixer-agent"))` line, add:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))
      ```

   e. **"overrides all three fields when the maintenance file provides them"** (under `Describe("per-repo maintenance overrides")`):
      After the existing `Expect(cmd.Frontmatter["assignee"]).To(Equal("go-deps-fixer-agent"))` line, add:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))
      ```

5. **Add new `It` blocks to `watcher/github-build/pkg/watcher_test.go` — cover the `human` → `""` translation**

   These are brand-new tests. Add them to the appropriate `Describe` blocks.

   a. **In `Describe("configurable frontmatter")`** — add after the "uses custom assignee and status when set" `It` block:

   ```go
   It("translates assignee=human to empty string in emitted frontmatter", func() {
       ghClient.GetDefaultBranchReturns("main", nil)
       ghClient.GetWorkflowRunsReturns(singleFailingRun(20, "sha-human"), nil)

       w := makeCustomWatcher([]string{"owner/repo"}, "human", "todo", "")
       Expect(w.Poll(ctx)).To(Succeed())

       Expect(createSender.SendCommandCallCount()).To(Equal(1))
       _, cmd := createSender.SendCommandArgsForCall(0)
       Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))
       Expect(cmd.Frontmatter["assignee"]).To(Equal(""))
   })

   It("passes through assignee=build-fix-planner unchanged", func() {
       ghClient.GetDefaultBranchReturns("main", nil)
       ghClient.GetWorkflowRunsReturns(singleFailingRun(21, "sha-planner"), nil)

       w := makeCustomWatcher([]string{"owner/repo"}, "build-fix-planner", "todo", "")
       Expect(w.Poll(ctx)).To(Succeed())

       Expect(createSender.SendCommandCallCount()).To(Equal(1))
       _, cmd := createSender.SendCommandArgsForCall(0)
       Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))
       Expect(cmd.Frontmatter["assignee"]).To(Equal("build-fix-planner"))
   })
   ```

   b. **In `Describe("per-repo maintenance overrides")`** — add after the "empty assignee in file falls through to watcher default" `It` block:

   ```go
   It("translates maintenance override assignee=human to empty string", func() {
       ghClient.GetDefaultBranchReturns("main", nil)
       ghClient.GetWorkflowRunsReturns(singleFailingRunMaint("sha-human-override"), nil)
       maintenanceLoader.LoadOverridesReturns(maintenance.GithubBuildConfig{
           Assignee: "human",
       })

       w := makeWatcherWithLoader([]string{"owner/repo"}, maintenanceLoader)
       Expect(w.Poll(ctx)).To(Succeed())

       Expect(createSender.SendCommandCallCount()).To(Equal(1))
       _, cmd := createSender.SendCommandArgsForCall(0)
       Expect(cmd.Frontmatter["task_type"]).To(Equal("build-fix"))
       Expect(cmd.Frontmatter["assignee"]).To(Equal(""))
   })
   ```

6. **Run `make test`** again to confirm all tests pass:

   ```bash
   cd watcher/github-build && make test
   ```

   All tests must pass before proceeding.

7. **Add CHANGELOG entry** to root `CHANGELOG.md`. If a `## Unreleased` section already exists, append to it; otherwise prepend a new `## Unreleased` section above the latest version header:

   ```markdown
   ## Unreleased

   - feat(watcher/github-build): add `task_type: build-fix` to all emitted task commands; translate `assignee=human` to `""` per 2026-05-10 cross-repo doctrine
   ```

8. **Run `make precommit`** from `watcher/github-build/`:

   ```bash
   cd watcher/github-build && make precommit
   ```

   Must exit 0.
</requirements>

<constraints>
- **Single file under change:** `watcher/github-build/pkg/watcher.go` and its co-located test file `watcher/github-build/pkg/watcher_test.go`. No other production file is touched.
- **Frozen doctrine values** — only these exact strings are permitted (do not invent alternatives):
  - `task_type`: `"build-fix"` (string literal)
  - `assignee` (parked / `human` config): `""` (explicit empty string — the key must be present in the map, not absent)
  - `assignee` (any agent name): the name verbatim, unchanged
- `assignee: ""` for the `human` path is intentional. Do not omit the key or replace it with `"none"`, `"unassigned"`, or any other value.
- **No change to the watcher constructor signature or factory wiring.** The `assignee` parameter of `NewWatcher` keeps its existing name and semantics. Only the emission-site transformation in `buildCreateTaskCommand` changes.
- **No change to `applyStateMachine`.** The `effectiveAssignee` coalescence logic is preserved. `translateAssignee` is called inside `buildCreateTaskCommand`, not at the call site.
- All existing tests in `watcher/github-build/pkg/watcher_test.go` must continue to pass — no test is deleted. Tests are updated/extended where they assert on frontmatter keys that the spec changes.
- Do NOT commit — dark-factory handles git.
- `make precommit` runs from `watcher/github-build/`, never at repo root.
- Error wrapping: `github.com/bborbe/errors` — never `fmt.Errorf`.
- No new fields beyond `task_type` and the `assignee` translation.
- No change to verdict JSON, Kafka topic names, or any cross-service contract.
- No change to `watcher/github-pr/` or any other watcher.
</constraints>

<verification>
Run `make precommit` in a subshell so subsequent greps still resolve from repo root:
```bash
(cd watcher/github-build && make precommit)

# Confirm task_type appears in buildCreateTaskCommand:
grep -n '"task_type"' watcher/github-build/pkg/watcher.go
# Expected: 1 match inside the fm literal in buildCreateTaskCommand

# Confirm translateAssignee helper exists:
grep -n 'func translateAssignee' watcher/github-build/pkg/watcher.go
# Expected: 1 match

# Confirm translateAssignee is called in buildCreateTaskCommand:
grep -n 'translateAssignee' watcher/github-build/pkg/watcher.go
# Expected: 2 matches (declaration + call site in buildCreateTaskCommand)

# Confirm test assertions cover task_type:
grep -n 'task_type' watcher/github-build/pkg/watcher_test.go
# Expected: ≥5 occurrences

# Confirm test assertions cover assignee="" (parked path):
grep -nE '"assignee"\).*Equal\(""\)' watcher/github-build/pkg/watcher_test.go
# Expected: ≥2 matches (watcher-level human + maintenance override human)

# Confirm CHANGELOG entry at repo root:
grep -nE 'task_type.*build-fix|build-fix.*task_type' CHANGELOG.md | head -3
# Expected: one match under ## Unreleased
```

Expected: `make precommit` exits 0; all grep checks show expected matches.
</verification>
