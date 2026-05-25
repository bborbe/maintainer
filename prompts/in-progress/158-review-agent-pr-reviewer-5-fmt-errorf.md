---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T21:00:21Z"
---

<summary>
- `cmd/cli/main.go:56, 96, 191` use `fmt.Errorf` for error construction
- Project convention requires `github.com/bborbe/errors.Errorf` with context
- The file already imports `bborbe/errors` and uses it correctly elsewhere — fix the 3 inconsistent calls
</summary>

<objective>
Replace 3 `fmt.Errorf` calls in `agent/pr-reviewer/cmd/cli/main.go` with `errors.Errorf(ctx, ...)` to match project error-handling conventions.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors` API, banned `fmt.Errorf`.

Files to read before making changes:
- `agent/pr-reviewer/cmd/cli/main.go` — full file; find the 3 `fmt.Errorf` usages at lines 56, 96, and 191
</context>

<requirements>
**Execute steps in order. Run `make test` after the fix. Run `make precommit` only at the final step.**

1. **Replace `fmt.Errorf` with `errors.Errorf` in `agent/pr-reviewer/cmd/cli/main.go`**

   a. Line 56 (in `run()` function — usage error):
   ```go
   // Before:
   return fmt.Errorf("usage: %s", a.Cmd.UsageString())

   // After:
   return errors.Errorf(ctx, "usage: %s", a.Cmd.UsageString())
   ```

   b. Line 96 (in `runGitHub()` — unsupported platform):
   ```go
   // Before:
   return fmt.Errorf("unsupported platform: %s", prInfo.Platform)

   // After:
   return errors.Errorf(ctx, "unsupported platform: %s", prInfo.Platform)
   ```

   c. Line 191 (in `runBitbucket()` — missing token):
   ```go
   // Before:
   return fmt.Errorf("BITBUCKET_TOKEN not set")

   // After:
   return errors.Errorf(ctx, "BITBUCKET_TOKEN not set")
   ```

   Note: Ensure `ctx` is in scope at each call site. All three functions (`run()`, `runGitHub()`, `runBitbucket()`) accept `ctx context.Context` as their first parameter.

2. **Run `make test`** to verify:

   ```bash
   cd agent/pr-reviewer && make test
   ```

3. **Run `make precommit`** for final validation:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only change `agent/pr-reviewer/cmd/cli/main.go`
- Do NOT commit — dark-factory handles git
- All 3 `fmt.Errorf` calls must be replaced — the file already imports `github.com/bborbe/errors` correctly
- Pass `ctx` as first argument to `errors.Errorf`
- Existing tests must still pass
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
