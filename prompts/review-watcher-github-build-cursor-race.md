---
status: draft
created: "2026-05-24T12:00:00Z"
---

<summary>
- Fix cursor file read/write race between `/resetcursor` handler and concurrent poll cycles
- Use write-to-temp + atomic rename pattern to make cursor updates idempotent
- Prevents torn reads and lost writes when reset and poll both modify cursor.json simultaneously
</summary>

<objective>
Fix the cursor file race in `pkg/reset_handler.go` and `pkg/cursor.go` by using the standard write-to-temp + atomic rename pattern. The current read-modify-write sequence in `/resetcursor` has no locking and can corrupt or lose updates when a poll cycle writes concurrently.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/pkg/reset_handler.go` lines 39-52 (LoadCursor + SaveCursor call sequence)
- `watcher/github-build/pkg/cursor.go` lines 34-67 (LoadCursor and SaveCursor implementations)
- `watcher/github-build/pkg/cursor_test.go` (existing tests, to extend)
</context>

<requirements>
1. In `pkg/cursor.go`, update `SaveCursor` to use write-to-temp + atomic rename:
   ```go
   func SaveCursor(ctx context.Context, path string, cursor *Cursor) error {
       data, err := json.Marshal(cursor)
       if err != nil {
           return errors.Wrap(ctx, err, "marshal cursor")
       }
       // Write to temp file in same directory, then rename atomically.
       // os.Rename is atomic on POSIX when src and dst are on the same filesystem.
       tmp := path + ".tmp"
       if err := os.WriteFile(tmp, data, 0600); err != nil {
           return errors.Wrap(ctx, err, "write cursor temp file")
       }
       if err := os.Rename(tmp, path); err != nil {
           return errors.Wrap(ctx, err, "rename cursor temp file")
       }
       return nil
   }
   ```

2. The `#nosec G306` annotation from the original `WriteFile` code should be moved to the new `os.WriteFile` call (same reasoning: cursor file should be owner-readable only).

3. Update `pkg/cursor_test.go` to test the `Rename` failure path. Use a mock or temp directory manipulation to trigger an error on `os.Rename`. The existing round-trip test still passes.

4. Verify `LoadCursor` in `pkg/cursor.go` does not need changes — it already reads atomically via `os.ReadFile`.

5. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.
</requirements>

<constraints>
- Only change files in `watcher/github-build/pkg/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- Keep the `#nosec G306` annotation explaining why 0600 is correct for cursor files
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
