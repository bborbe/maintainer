---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T22:38:58Z"
---

<summary>
- `main.go` has five bare `return err` statements at lines 134, 142, 155, 160, 170 propagating from validation functions — should use `errors.Wrap`
- `factory.go` line 133 has bare `return nil, err` from `CreateWatcher` — should use `errors.Wrap`
- `cursor.go` lines 37, 54, 57, 60 use `errors.Wrapf` without format verb (path only in message string) — should use `errors.Wrap`
- `trust.go` lines 74, 96, 111 use `errors.Wrapf` without format verb — should use `errors.Wrap`
- `ParseRepoAllowlist` in `repo_allowlist_filter.go` accepts `ctx` but discards it — dead parameter
</summary>

<objective>
Fix all bare return error violations and unused context parameters. Wrap errors at call sites where validation functions return errors. Replace `errors.Wrapf` with `errors.Wrap` where there are no format verbs.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — bborbe/errors API, never fmt.Errorf, never context.Background().

Files to read before making changes:
- `watcher/github-pr/main.go` — lines 120-180; understand validateConfig, parseMaxPRAge, parseBackfillDuration, filter.ParseRepoAllowlist
- `watcher/github-pr/pkg/factory/factory.go` — line 133; understand CreateWatcher
- `watcher/github-pr/pkg/cursor.go` — lines 30-65; understand LoadCursor, SaveCursor
- `watcher/github-pr/pkg/trust/trust.go` — lines 60-120; understand trust combinators
- `watcher/github-pr/pkg/filter/repo_allowlist_filter.go` — lines 15-35; understand ParseRepoAllowlist
</context>

<requirements>

**Execute steps in order. Run `make test` after step 5. Run `make precommit` only at the final step.**

1. **Fix bare returns in `main.go`:**

   At each validation function call that returns error, wrap:
   ```go
   // Line ~134: validateConfig
   if err := validateConfig(); err != nil {
       return errors.Wrap(ctx, err, "validate config")
   }

   // Line ~142: validateConfig(ctx)
   if err := validateConfig(ctx); err != nil {
       return errors.Wrap(ctx, err, "validate config")
   }

   // Line ~155: parseMaxPRAge
   maxPRAge, err := parseMaxPRAge()
   if err != nil {
       return errors.Wrap(ctx, err, "parse max PR age")
   }

   // Line ~160: parseBackfillDuration
   backfillDuration, err := parseBackfillDuration()
   if err != nil {
       return errors.Wrap(ctx, err, "parse backfill duration")
   }

   // Line ~170: filter.ParseRepoAllowlist
   repoAllowlist, err := filter.ParseRepoAllowlist(ctx, getEnv("REPO_ALLOWLIST", ""))
   if err != nil {
       return errors.Wrap(ctx, err, "parse repo allowlist")
   }
   ```

2. **Fix bare return in `pkg/factory/factory.go` line 133:**

   Change `return nil, err` to:
   ```go
   return nil, errors.Wrap(ctx, err, "create watcher")
   ```

3. **Fix errors.Wrapf without format verb in `pkg/cursor.go`:**

   Replace all four occurrences:
   - Line 37: `errors.Wrapf(ctx, err, "read cursor file: %s", path)` → `errors.Wrap(ctx, err, "read cursor file")`
   - Line 54: `errors.Wrapf(ctx, err, "marshal cursor state: %v", cursor)` → `errors.Wrap(ctx, err, "marshal cursor state")`
   - Line 57: `errors.Wrapf(ctx, err, "write cursor file: %s", tmpPath)` → `errors.Wrap(ctx, err, "write cursor file")`
   - Line 60: `errors.Wrapf(ctx, err, "rename cursor file: %s → %s", tmpPath, path)` → `errors.Wrap(ctx, err, "rename cursor file")`

4. **Fix errors.Wrapf without format verb in `pkg/trust/trust.go`:**

   Replace:
   - Line 74: `errors.Wrapf(ctx, err, "and trust check")` (no format args) → `errors.Wrap(ctx, err, "and trust check")`
   - Line 96: `errors.Wrapf(ctx, err, "or trust check")` (no format args) → `errors.Wrap(ctx, err, "or trust check")`
   - Line 111: `errors.Wrapf(ctx, err, "not trust check")` (no format args) → `errors.Wrap(ctx, err, "not trust check")`

5. **Fix unused ctx parameter in `pkg/filter/repo_allowlist_filter.go`:**

   Change the function signature to explicitly discard ctx:
   ```go
   func ParseRepoAllowlist(_ context.Context, raw string) ([]filter.RepoAllowlistEntry, error) {
   ```
   Or if the ctx should actually be used (propagated to any future I/O), add `ctx` usage. If truly unused, the `_` makes the intentional discard explicit.

6. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```
   Fix any compilation errors.

7. **Run `make precommit`:**
   ```bash
   cd watcher/github-pr && make precommit
   ```
</requirements>

<constraints>
- Only change `watcher/github-pr/main.go`, `watcher/github-pr/pkg/factory/factory.go`, `watcher/github-pr/pkg/cursor.go`, `watcher/github-pr/pkg/trust/trust.go`, and `watcher/github-pr/pkg/filter/repo_allowlist_filter.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Wrapf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- Do NOT use `context.Background()` — use ctx from caller
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm no bare return err in main.go:
grep -n "return err$" watcher/github-pr/main.go

# Confirm no errors.Wrapf without format args:
grep -n "errors.Wrapf.*[^*]*\"[^\"]*\"$" watcher/github-pr/pkg/cursor.go watcher/github-pr/pkg/trust/trust.go

# Confirm no unused ctx in ParseRepoAllowlist:
grep -n "context.Context" watcher/github-pr/pkg/filter/repo_allowlist_filter.go
</verification>
