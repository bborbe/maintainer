---
status: completed
spec: [072-force-trigger-on-github-pr-watcher]
summary: 'Added DeriveTaskIDForce helper + 3 Ginkgo tests in watcher/github-pr/pkg/, CHANGELOG ## Unreleased bullet; make precommit exits 0'
container: maintainer-trigger-force-exec-252-spec-067-derive-task-id-force-helper
dark-factory-version: v0.175.0
created: "2026-06-09T15:50:00Z"
queued: "2026-06-09T16:02:46Z"
started: "2026-06-09T16:17:06Z"
completed: "2026-06-09T16:22:08Z"
branch: dark-factory/force-trigger-on-github-pr-watcher
---

<summary>
- Adds a new exported helper `DeriveTaskIDForce` next to the existing `DeriveTaskID` in `watcher/github-pr/pkg/taskid.go`.
- The helper produces a salted task identifier by appending an extra `nonce` string to the canonical key, so two calls with the same `(owner, repo, number, sha)` but different nonces yield different UUIDs.
- Three Ginkgo unit tests in `taskid_test.go` cover: salted differs from canonical, salted is stable for the same nonce, salted differs across nonces.
- No callers are wired in this prompt — the helper is pure additive and has zero side effects.
- The existing `DeriveTaskID` and its `prWatcherNamespace` UUID are unchanged byte-for-byte; existing tests pass unmodified.
</summary>

<objective>
Add a new exported helper `DeriveTaskIDForce` to `watcher/github-pr/pkg/taskid.go` that returns a salted UUID5 derived from `(owner, repo, number, sha, nonce)`, plus Ginkgo unit tests that pin the divergence and stability invariants. This is a pure-additive change — `DeriveTaskID` and `prWatcherNamespace` are untouched.
</objective>

<context>
Read the project conventions first:
- `/workspace/CLAUDE.md` (project-wide rules)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md`

Read these source files fully before editing:
- `/workspace/watcher/github-pr/pkg/taskid.go` — the helper that ships today; you are extending this file, not replacing it.
- `/workspace/watcher/github-pr/pkg/taskid_test.go` — the Ginkgo test layout (external test package `package_test`, single `Describe` block, `Describe("Derive", ...)`).
- `/workspace/specs/in-progress/067-force-trigger-on-github-pr-watcher.md` — especially the Goal, Constraints, and the `DeriveTaskIDForce` AC entries.
</context>

<requirements>

1. **Add the helper to `watcher/github-pr/pkg/taskid.go`.** Keep the existing `DeriveTaskID` function, its doc comment, and the `prWatcherNamespace` constant untouched (Constraint: signature, namespace UUID, and input encoding are frozen).

   Add this new function below the existing `DeriveTaskID` (in the same `package pkg`):

   ```go
   // DeriveTaskIDForce returns a salted task identifier for a (PR, SHA) pair
   // plus an extra nonce. Used when an operator explicitly requests a forced
   // re-review (HTTP /trigger?force=true) so the executor can publish a
   // CreateTaskCommand with a TaskIdentifier that the controller has not
   // already seen — bypassing the dedup-skip in the agent controller.
   //
   // For the same (owner, repo, number, sha) the result is always different
   // from DeriveTaskID(...): the key includes the nonce segment "<nonce>",
   // e.g. "bborbe/maintainer#42@abc123...!1700000000000000000".
   //
   // The nonce resolution is the caller's responsibility. Callers should
   // derive it from an injected libtime.CurrentDateTimeGetter, never from
   // time.Now() in business logic.
   func DeriveTaskIDForce(owner, repo string, number int, sha, nonce string) uuid.UUID {
       key := fmt.Sprintf("%s/%s#%d@%s!%s", owner, repo, number, sha, nonce)
       return uuid.NewSHA1(prWatcherNamespace, []byte(key))
   }
   ```

   Notes:
   - Signature is exactly `func(owner, repo string, number int, sha, nonce string) uuid.UUID` per the spec.
   - The `!` separator is intentional — it is not a valid character in a GitHub SHA (`[0-9a-f]`) or in a PR number, so no collision with a real canonical key is possible.
   - Reuse the existing `prWatcherNamespace` constant — do NOT introduce a new namespace.
   - Reuse the same `fmt.Sprintf` + `uuid.NewSHA1` shape as the existing `DeriveTaskID` for visual symmetry; the only delta is the trailing `!%s` segment.

2. **Add three unit tests in `watcher/github-pr/pkg/taskid_test.go`.** The file has an outer `Describe("TaskID", func() { ... })` at line 15 with a nested `Describe("Derive", ...)` inside it. Add `Describe("DeriveForce", func() { ... })` **as a sibling to `Describe("Derive", ...)` INSIDE the outer `Describe("TaskID", ...)` block** — NOT as a new top-level `var _ = Describe(...)`. Do NOT modify the existing `Describe("Derive", ...)` block — its assertions are pinned by the spec.

   ```go
   // inside the existing Describe("TaskID", func() { ... })  block,
   // as a sibling to Describe("Derive", ...)
   Describe("DeriveForce", func() {
       It("produces a different UUID than DeriveTaskID for the same (owner, repo, number, sha)", func() {
           canonical := pkg.DeriveTaskID("bborbe", "code-reviewer", 42, "abc123def456789a")
           salted := pkg.DeriveTaskIDForce("bborbe", "code-reviewer", 42, "abc123def456789a", "nonce-1")
           Expect(salted).NotTo(Equal(canonical))
       })

       It("is stable for identical (owner, repo, number, sha, nonce) inputs", func() {
           a := pkg.DeriveTaskIDForce("bborbe", "code-reviewer", 42, "abc123def456789a", "nonce-x")
           b := pkg.DeriveTaskIDForce("bborbe", "code-reviewer", 42, "abc123def456789a", "nonce-x")
           Expect(a).To(Equal(b))
       })

       It("produces different UUIDs for the same (owner, repo, number, sha) but different nonces", func() {
           a := pkg.DeriveTaskIDForce("bborbe", "code-reviewer", 42, "abc123def456789a", "nonce-a")
           b := pkg.DeriveTaskIDForce("bborbe", "code-reviewer", 42, "abc123def456789a", "nonce-b")
           Expect(a).NotTo(Equal(b))
       })
   })
   ```

   Add this inside the existing `package_test` file. Do not change the package declaration or the existing `Describe("Derive", ...)` block.

3. **No callers.** This prompt does NOT touch the executor, the factory, the HTTP handler, the wiring in `main.go`, or any test outside `pkg/taskid_test.go`. `DeriveTaskIDForce` is a pure helper that no one calls yet — prompts 2 and 3 will wire it in.

4. **No metrics, no config fields, no clock injection in this prompt.** The helper takes a `nonce string` — the time-derived nonce is the caller's responsibility (prompt 2).

5. **Verify the negative AC.** Run `! grep -rnE '\btime\.Now\(\)' pkg/command/ pkg/taskid.go pkg/factory/` from `watcher/github-pr/`. It must exit 0 (no `time.Now()` in the changed paths).

</requirements>

<constraints>
- Existing tests in `watcher/github-pr/pkg/taskid_test.go` and `watcher/github-pr/pkg/watcher_test.go` must pass unmodified — they pin the non-force path (spec Constraint).
- `pkg.DeriveTaskID` signature, namespace UUID (`prWatcherNamespace`), and input encoding (`"<owner>/<repo>#<number>@<sha>"`) are frozen. Do NOT modify.
- Do NOT introduce a new namespace UUID — reuse `prWatcherNamespace` (it is the same value as the canonical helper's).
- Do NOT add a `time.Now()` call anywhere in `pkg/taskid.go`. The helper is a pure function over its inputs.
- Do NOT touch `watcher/github-pr/pkg/watcher.go` line ~279 (`DeriveTaskID` call site in the poll path) — spec Non-goal: poll path is independent of `force`.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
Run from `/workspace/watcher/github-pr/`:

```bash
go test ./pkg -run 'Derive' -v
grep -nE 'func DeriveTaskIDForce' pkg/taskid.go
! grep -rnE '\btime\.Now\(\)' pkg/command/ pkg/taskid.go pkg/factory/
```

Expected:
- `go test -v` shows the three new `DeriveForce` cases PASS plus the existing `Derive` cases PASS (unmodified).
- `grep` for `func DeriveTaskIDForce` returns exactly one line.
- `! grep` exits 0 (no `time.Now()` introduced in the watched paths).
- `git diff pkg/taskid.go` shows ONLY an additive change: the new function below the existing one. The existing function and `prWatcherNamespace` constant are byte-identical.
- `git diff pkg/taskid_test.go` shows ONLY the new `Describe("DeriveForce", ...)` block; the existing `Describe("Derive", ...)` block is untouched.

Then run the full module precommit for the suite this prompt touches:

```bash
cd /workspace/watcher/github-pr && make precommit
```

`make precommit` must exit 0. If only this prompt's files changed, the run is fast (no Go-cross-cutting work).
</verification>
