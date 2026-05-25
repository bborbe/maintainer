---
status: approved
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:00:21Z"
---

<summary>
- Remove unused `pkg/publisher.go` file which only contains a license header and package declaration
- Remove duplicate comment block in `main.go` (lines 54-57)
- Remove unused `countWildcards` function from `main.go` if it was moved to factory
</summary>

<objective>
Clean up small code quality issues: delete the empty `pkg/publisher.go` file, merge the duplicate comment block in `main.go`, and remove any functions that are no longer needed after previous fixes.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/pkg/publisher.go` (to confirm it's empty)
- `watcher/github-build/main.go` lines 43-75 (duplicate comment and countWildcards)
</context>

<requirements>
1. Delete `watcher/github-build/pkg/publisher.go` — it contains only a license header and package declaration, no code.

2. In `main.go`, fix the duplicate comment block at lines 54-57:
   Before:
   ```go
   // buildAllowlistSnapshot creates the snapshot provider and (if wildcards are present)
   // a background refresh task for the daemon's run loop.
   // buildAllowlistSnapshot creates the snapshot provider and (if wildcards are present)
   // a background refresh task for the daemon's run loop.
   ```
   After (single comment):
   ```go
   // buildAllowlistSnapshot creates the snapshot provider and (if wildcards are present)
   // a background refresh task for the daemon's run loop.
   ```

3. If `countWildcards` was moved to `pkg/factory/factory.go` in a previous fix, remove it from `main.go`. If not moved, leave it (it's used by the remaining `buildAllowlistSnapshot` call).

4. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

5. Run `cd watcher/github-build && go test ./...` to confirm tests pass.
</requirements>

<constraints>
- Only change files in `watcher/github-build/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT remove `countWildcards` if it's still used by `buildAllowlistSnapshot` in `main.go`
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
