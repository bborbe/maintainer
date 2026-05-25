---
status: completed
summary: Extracted CreateAllowlistSnapshot to factory, removing duplicated wildcard resolution logic from main.go and cmd/run-once/main.go
container: maintainer-exec-175-review-watcher-github-build-factory-wildcard
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:25:46Z"
started: "2026-05-25T21:57:43Z"
completed: "2026-05-25T21:59:41Z"
---

<summary>
- Extract `buildAllowlistSnapshot` from `main.go` into `pkg/factory/factory.go` as `CreateAllowlistSnapshot`
- Remove duplicated wildcard resolution logic from `cmd/run-once/main.go` by using the new factory function
- Both entry points (`main.go` and `cmd/run-once/main.go`) then use the same factory call
</summary>

<objective>
Move the `buildAllowlistSnapshot` branching logic from `main.go` into `pkg/factory/factory.go` as `CreateAllowlistSnapshot`. Update both `main.go` and `cmd/run-once/main.go` to call this new factory function, eliminating the duplicated wildcard resolution code.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/main.go` lines 54-75 (`buildAllowlistSnapshot`)
- `watcher/github-build/cmd/run-once/main.go` lines 80-95 (duplicated logic)
- `watcher/github-build/pkg/factory/factory.go` (existing factory)
- `watcher/github-build/pkg/wildcard/resolved.go` (`ResolvedAllowlist`, `HasWildcard`, `RefreshInterval`)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
</context>

<requirements>
1. Add `CreateAllowlistSnapshot` to `pkg/factory/factory.go`:
   ```go
   // CreateAllowlistSnapshot returns a snapshot provider and (optionally) a background
   // refresh task for the daemon's run loop.
   // If the input allowlist contains wildcards, a ResolvedAllowlist with a refresh goroutine
   // is returned. Otherwise, a static snapshot with no background refresh is returned.
   func CreateAllowlistSnapshot(
       ghClient pkg.GitHubClient,
       repoAllowlist []string,
   ) (pkg.AllowlistSnapshot, run.Func, error) {
       if wildcard.HasWildcard(repoAllowlist) {
           expander := wildcard.NewExpander(ghClient)
           resolvedSet := wildcard.NewResolvedAllowlist(expander, repoAllowlist)
           glog.V(2).Infof(
               "wildcard_refresh_enabled entries=%d (interval=%s)",
               countWildcards(repoAllowlist), wildcard.RefreshInterval(),
           )
           return resolvedSet, func(ctx context.Context) error {
               return resolvedSet.RunRefreshLoop(ctx)
           }, nil
       }
       glog.V(2).Infof("wildcard_refresh_disabled allowlist=pure-literal")
       return pkg.NewStaticSnapshot(repoAllowlist), nil, nil
   }
   ```

2. The `countWildcards` helper should live in `pkg/factory/factory.go` (move it from `main.go` or duplicate it — since it's simple and only used by the factory, duplication is acceptable here).

3. Update `main.go` to call the factory instead of `buildAllowlistSnapshot`:
   ```go
   resolved, refreshTask, err := factory.CreateAllowlistSnapshot(ghClient, repoAllowlist)
   if err != nil {
       return errors.Wrap(ctx, err, "create allowlist snapshot")
   }
   ```

4. Update `cmd/run-once/main.go` to use the same factory function. The run-once path should still call `resolvedSet.Refresh(ctx)` synchronously before passing to the watcher (not use the refresh task).

5. Remove the `buildAllowlistSnapshot` function from `main.go` and the `countWildcards` helper if moved.

6. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

7. Run `cd watcher/github-build && go test ./...` to confirm tests pass.
</requirements>

<constraints>
- Only change files in `watcher/github-build/` (main.go, cmd/run-once/main.go, pkg/factory/factory.go)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- Factory must have zero business logic (per the factory pattern guide) — but the `if wildcard.HasWildcard` branching IS wiring logic, which is acceptable in a factory
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
