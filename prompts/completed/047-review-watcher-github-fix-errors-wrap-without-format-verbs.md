---
status: completed
summary: Replaced six errors.Wrapf calls in pkg/publisher.go and two in pkg/factory/factory.go with errors.Wrap where format strings contain no % verbs.
container: code-reviewer-047-review-watcher-github-fix-errors-wrap-without-format-verbs
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T14:52:29Z"
started: "2026-04-28T15:02:04Z"
completed: "2026-04-28T15:04:25Z"
---

<summary>
- Eight calls use errors.Wrapf with a plain string message that has no format verbs
- The project guide states errors.Wrapf is only for messages with % format parameters
- When no format verbs are present, errors.Wrap (without the f) is the correct call
- All eight occurrences are in two files: pkg/publisher.go and pkg/factory/factory.go
- This is a mechanical find-and-replace with no behavioral change
- Tests are unaffected since both functions wrap errors the same way
</summary>

<objective>
Replace all `errors.Wrapf(ctx, err, "plain string")` calls that contain no `%` format verbs with `errors.Wrap(ctx, err, "plain string")`. This follows the project's error wrapping guide: `Wrapf` is only for messages with format parameters.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/publisher.go` (~lines 38, 42, 53, 57, 65, 69): six `errors.Wrapf` calls with plain string messages
- `watcher/github/pkg/factory/factory.go` (~lines 33, 58): two `errors.Wrapf` calls with plain string messages
</context>

<requirements>
1. In `watcher/github/pkg/publisher.go`, replace the following six calls:
   - Line ~38: `errors.Wrapf(ctx, err, "marshal create-task command")` → `errors.Wrap(ctx, err, "marshal create-task command")`
   - Line ~42: `errors.Wrapf(ctx, err, "publish create-task")` → `errors.Wrap(ctx, err, "publish create-task")`
   - Line ~53: `errors.Wrapf(ctx, err, "marshal update-frontmatter command")` → `errors.Wrap(ctx, err, "marshal update-frontmatter command")`
   - Line ~57: `errors.Wrapf(ctx, err, "publish update-frontmatter")` → `errors.Wrap(ctx, err, "publish update-frontmatter")`
   - Line ~65: `errors.Wrapf(ctx, err, "marshal command to json")` → `errors.Wrap(ctx, err, "marshal command to json")`
   - Line ~69: `errors.Wrapf(ctx, err, "parse event from json")` → `errors.Wrap(ctx, err, "parse event from json")`

2. In `watcher/github/pkg/factory/factory.go`, replace the following two calls:
   - Line ~33: `errors.Wrapf(ctx, err, "create sync producer")` → `errors.Wrap(ctx, err, "create sync producer")`
   - Line ~58: `errors.Wrapf(ctx, err, "create kafka publisher")` → `errors.Wrap(ctx, err, "create kafka publisher")`

3. Verify no unintentional changes: `errors.Wrapf` calls that DO use format verbs (e.g., `"search github prs scope=%s"`, `"get pull request %s/%s#%d"`, `"read cursor file path=%s"`) must remain as `errors.Wrapf`.

4. Run `cd watcher/github && make test` — must pass.

5. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Only replace `errors.Wrapf` → `errors.Wrap` where the format string has no `%` characters
- Do NOT touch any `errors.Wrapf` calls that contain format verbs like `%s`, `%d`, `%v`
- No behavioral change — this is purely a naming convention fix
</constraints>

<verification>
cd watcher/github && grep -n "errors\.Wrapf" pkg/publisher.go pkg/factory/factory.go
# Expected: zero matches (all replaced)

cd watcher/github && grep -n "errors\.Wrap\b" pkg/publisher.go pkg/factory/factory.go
# Expected: eight matches total (six in publisher.go, two in factory.go)

cd watcher/github && make precommit
</verification>
