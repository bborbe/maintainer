---
status: completed
spec: [022-pr-review-task-type-and-parked-assignee]
summary: 'Added task_type: pr-review to all emitted task commands in buildFrontmatter, buildHumanReviewFrontmatter, and publishForcePush; corrected assignee to empty string on untrusted-author paths; updated all test assertions to pin the new field values.'
container: maintainer-106-spec-022-task-type-and-parked-assignee
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-10T19:30:00Z"
queued: "2026-05-10T20:21:02Z"
started: "2026-05-10T20:21:03Z"
completed: "2026-05-10T20:24:05Z"
branch: dark-factory/pr-review-task-type-and-parked-assignee
---

<summary>
- Every task created by the github-pr watcher now carries `task_type: pr-review` in its frontmatter — operators can route by type without inspecting body or other fields.
- Tasks parked for human review (untrusted-author route) now emit `assignee: ""` (explicit empty string) instead of `"pr-reviewer-agent"` — the empty-assignee convention is the inbox-visibility signal in the cross-repo doctrine.
- Trusted force-push updates now include both `task_type: pr-review` and `assignee: pr-reviewer-agent` in the `UpdateFrontmatterCommand` — re-claiming previously-parked tasks when the head changes to trusted code.
- Untrusted force-push updates now include both `task_type: pr-review` and `assignee: ""` — keeping the task parked and unclaimed.
- All unit tests are updated to pin the new field values; no previously passing test is removed.
- The change is confined to `watcher/github-pr/pkg/watcher.go` and its test file — no other service, topic, or schema is touched.
</summary>

<objective>
Add `task_type: pr-review` to every task frontmatter and `UpdateFrontmatterCommand` payload emitted by the github-pr watcher, and correct `assignee` to `""` on the untrusted-author path (both new-task creation and force-push), so all emitted commands conform to the 2026-05-10 cross-repo doctrine: `task_type` is the routing primitive and empty `assignee` is the operator-inbox signal for tasks that need human attention.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, counterfeiter mocks, coverage ≥80%.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern.
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write for each code change.

**Files to read fully before making any changes:**

- `watcher/github-pr/pkg/watcher.go` — full file; understand `buildFrontmatter`, `buildHumanReviewFrontmatter`, and `publishForcePush` — all four sites need to change.
- `watcher/github-pr/pkg/watcher_test.go` — full file; understand every `It` block that asserts `cmd.Frontmatter` or `cmd.Updates` keys — these must be updated to pin the new fields and corrected values.

**Doctrine reference (read before implementing):**

The cross-repo doctrine values are fixed constants (do not invent alternatives):

| Situation | `task_type` | `assignee` |
|---|---|---|
| Trusted PR created | `pr-review` | `pr-reviewer-agent` |
| Untrusted PR created | `pr-review` | `""` (explicit empty string) |
| Trusted force-push update | `pr-review` | `pr-reviewer-agent` |
| Untrusted force-push update | `pr-review` | `""` (explicit empty string) |

`assignee: ""` (empty string) is intentional and must appear as an explicit key in the map — it is NOT the same as omitting the key. The operator inbox filters on `assignee == ""`.

**Note on assumption from spec:** `FrontmatterMap` preserves all fields through read-write cycles — specifying only the keys this watcher changes (`task_type`, `assignee`) in `UpdateFrontmatterCommand.Updates` does not clobber other frontmatter fields the controller wrote.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 3. Run `make precommit` only at the final step.**

1. **Update `buildFrontmatter` in `watcher/github-pr/pkg/watcher.go`**

   Add `"task_type": "pr-review"` to the returned `agentlib.TaskFrontmatter` map. The `assignee` value `"pr-reviewer-agent"` stays unchanged for this function (trusted path). After the edit the function should read:

   ```go
   func buildFrontmatter(
       pr PullRequest,
       taskIDStr, stage string,
       details PRDetails,
   ) agentlib.TaskFrontmatter {
       return agentlib.TaskFrontmatter{
           "task_type":       "pr-review",
           "assignee":        "pr-reviewer-agent",
           "phase":           "planning",
           "status":          "in_progress",
           "stage":           stage,
           "task_identifier": taskIDStr,
           "title":           pr.Title,
           "clone_url":       details.CloneURL,
           "ref":             details.HeadSHA,
           "base_ref":        details.BaseRef,
       }
   }
   ```

2. **Update `buildHumanReviewFrontmatter` in `watcher/github-pr/pkg/watcher.go`**

   Add `"task_type": "pr-review"` AND change `"assignee"` from `"pr-reviewer-agent"` to `""`. After the edit:

   ```go
   func buildHumanReviewFrontmatter(
       pr PullRequest,
       taskIDStr, stage string,
       details PRDetails,
   ) agentlib.TaskFrontmatter {
       return agentlib.TaskFrontmatter{
           "task_type":       "pr-review",
           "assignee":        "",
           "phase":           "human_review",
           "status":          "todo",
           "stage":           stage,
           "task_identifier": taskIDStr,
           "title":           pr.Title,
           "clone_url":       details.CloneURL,
           "ref":             details.HeadSHA,
           "base_ref":        details.BaseRef,
       }
   }
   ```

3. **Update `publishForcePush` in `watcher/github-pr/pkg/watcher.go`**

   Both the trusted and untrusted `updates` maps must include `"task_type"` and `"assignee"`. After the edit, the two arms of the `if trustResult.Success()` block should read:

   ```go
   if trustResult.Success() {
       updates = agentlib.TaskFrontmatter{
           "task_type":     "pr-review",
           "assignee":      "pr-reviewer-agent",
           "phase":         "planning",
           "status":        "in_progress",
           "trigger_count": 0,
       }
       bodySection = &task.BodySection{Heading: heading, Section: heading + "\n"}
   } else {
       if author == "" {
           author = "(unknown)"
       }
       glog.V(2).Infof("untrusted force-push author=%q trust=%s pr=%s", author, trustResult.Description(), pr.HTMLURL)
       updates = agentlib.TaskFrontmatter{
           "task_type":     "pr-review",
           "assignee":      "",
           "phase":         "human_review",
           "status":        "todo",
           "trigger_count": 0,
       }
       section := heading + "\n" + buildUntrustedBody(author, trustResult.Description())
       bodySection = &task.BodySection{Heading: heading, Section: section}
   }
   ```

4. **Run `make test`** from `watcher/github-pr/`:

   ```bash
   cd watcher/github-pr && make test
   ```

   Some existing test assertions will now fail because they assert `assignee: "pr-reviewer-agent"` on paths that now return `""`, or don't assert `task_type` yet. Identify every failing assertion before proceeding to step 5.

5. **Update `watcher/github-pr/pkg/watcher_test.go`**

   Update every `It` block that asserts on `cmd.Frontmatter` or `cmd.Updates` to pin the new field values. Apply the following changes:

   a. **"New PR (no existing cursor entry)" test** — trusted path. Add `task_type` assertion after the existing `assignee` check:
      ```go
      Expect(cmd.Frontmatter["assignee"]).To(Equal("pr-reviewer-agent"))
      Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
      ```

   b. **"buildFrontmatter fields" test** — trusted path. Add `task_type` assertion alongside the other field checks:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
      ```
      The existing `Expect(cmd.Frontmatter["assignee"]).To(Equal("pr-reviewer-agent"))` stays unchanged.

   c. **"Trusted-author new PR" test** — add assertions for both new fields:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
      Expect(cmd.Frontmatter["assignee"]).To(Equal("pr-reviewer-agent"))
      ```

   d. **"Untrusted-author new PR" test** — add assertions for `task_type` and pin `assignee` to the corrected empty value:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
      Expect(cmd.Frontmatter["assignee"]).To(Equal(""))
      ```

   e. **"Force-push (existing entry, different SHA)" test** — trusted force-push. Add assertions:
      ```go
      Expect(cmd.Updates["task_type"]).To(Equal("pr-review"))
      Expect(cmd.Updates["assignee"]).To(Equal("pr-reviewer-agent"))
      ```

   f. **"UpdateFrontmatterCommand fields" test** — trusted force-push. Add:
      ```go
      Expect(cmd.Updates["task_type"]).To(Equal("pr-review"))
      Expect(cmd.Updates["assignee"]).To(Equal("pr-reviewer-agent"))
      ```

   g. **"Untrusted-author force-push" test** — add assertions for both fields:
      ```go
      Expect(cmd.Updates["task_type"]).To(Equal("pr-review"))
      Expect(cmd.Updates["assignee"]).To(Equal(""))
      ```

   h. **"PR with missing AuthorLogin (defensive)" test** — untrusted path (empty author → treated as untrusted). Add:
      ```go
      Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
      Expect(cmd.Frontmatter["assignee"]).To(Equal(""))
      ```

6. **Run `make test`** again to confirm all tests pass:

   ```bash
   cd watcher/github-pr && make test
   ```

   All tests must pass before proceeding.

7. **Add CHANGELOG entry** to root `CHANGELOG.md`. If a `## Unreleased` section already exists, append the entry to it. Otherwise prepend a new `## Unreleased` section above the latest version header:

   ```markdown
   ## Unreleased

   - feat(watcher/github-pr): add `task_type: pr-review` to all emitted task commands; set `assignee: ""` on untrusted-author create and force-push paths per 2026-05-10 cross-repo doctrine
   ```

8. **Run `make precommit`** from `watcher/github-pr/`:

   ```bash
   cd watcher/github-pr && make precommit
   ```

   Must exit 0.
</requirements>

<constraints>
- **Single file under change:** `watcher/github-pr/pkg/watcher.go` and its co-located test file `watcher/github-pr/pkg/watcher_test.go`. No other production files touched.
- **Frozen doctrine values** — only these exact strings are permitted (do not invent alternatives):
  - `task_type`: `"pr-review"` (string literal)
  - `assignee` (trusted): `"pr-reviewer-agent"` (string literal)
  - `assignee` (parked/untrusted): `""` (explicit empty string — the key must be present in the map, not absent)
- `assignee: ""` in the untrusted path is intentional. Do not omit the key or replace it with `"none"`, `"unassigned"`, or any other value.
- All existing tests in `watcher/github-pr/pkg/watcher_test.go` must continue to pass — no test is deleted. Tests are updated where they assert on frontmatter keys that the spec changes.
- Do NOT commit — dark-factory handles git.
- `make precommit` runs from `watcher/github-pr/`, never at repo root.
- Error wrapping: `github.com/bborbe/errors` — never `fmt.Errorf`.
- Coverage ≥80% for changed packages.
- No change to verdict JSON, Kafka topic names, or any cross-service contract.
- No new fields beyond `task_type` and `assignee` adjustments.
- No changes to `buildUntrustedBody`, `buildTaskBody`, or any other function not listed in requirements.
- No changes to `watcher/github-build/` or any other watcher.
</constraints>

<verification>
Run `make precommit` in a subshell so subsequent greps still resolve from repo root:
```bash
(cd watcher/github-pr && make precommit)

# Confirm task_type appears at all four sites in watcher.go:
grep -n '"task_type"' watcher/github-pr/pkg/watcher.go
# Expected: 4 matches (buildFrontmatter, buildHumanReviewFrontmatter, trusted updates, untrusted updates)

# Confirm buildHumanReviewFrontmatter uses empty assignee:
grep -A 15 'func buildHumanReviewFrontmatter' watcher/github-pr/pkg/watcher.go | grep '"assignee"'
# Expected: "assignee":        "",

# Confirm trusted publishForcePush updates include correct assignee:
grep -A 10 'trustResult.Success()' watcher/github-pr/pkg/watcher.go | grep '"assignee"'
# Expected: "assignee":      "pr-reviewer-agent",

# Confirm both empty-assignee parked sites exist:
grep -nE '"assignee":\s*"",' watcher/github-pr/pkg/watcher.go
# Expected: at least 2 matches (buildHumanReviewFrontmatter + untrusted publishForcePush)

# Confirm test assertions cover all four sites:
grep -nE 'task_type' watcher/github-pr/pkg/watcher_test.go
# Expected: ≥4 occurrences covering trusted create, untrusted create, trusted force-push, untrusted force-push

grep -nE '"assignee"' watcher/github-pr/pkg/watcher_test.go
# Expected: assertions in both trusted (="pr-reviewer-agent") and untrusted (="") branches

# Confirm CHANGELOG entry at repo root:
grep -nE 'task_type.*pr-review|pr-review.*task_type' CHANGELOG.md | head -3
# Expected: one match under ## Unreleased
```

Expected: `make precommit` exits 0; all grep checks show expected matches.
</verification>
