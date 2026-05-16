---
status: completed
summary: Added validateRepoScope helper and package-level repoScopePattern to watcher/github/main.go, with validation called at startup before CreateWatcher, and Ginkgo DescribeTable tests covering valid and invalid inputs.
container: code-reviewer-050-review-watcher-github-fix-scope-query-injection
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T15:19:27Z"
started: "2026-04-28T15:47:08Z"
completed: "2026-04-28T15:50:04Z"
---

<summary>
- The REPO_SCOPE env var is interpolated directly into the GitHub Search API query string
- A malformed or adversarial value (e.g. "bborbe is:issue") alters the query semantics
- This allows the watcher to process non-PR issues and publish spurious task commands to Kafka
- Validation must happen at startup before the poll loop starts
- A simple allowlist regex (alphanumerics, hyphens, dots) covers all valid GitHub user/org names
- The validation should return an error that kills the pod at startup with a clear message
</summary>

<objective>
Validate the `REPO_SCOPE` environment variable against a GitHub user/org name allowlist pattern (`^[a-zA-Z0-9_.-]+$`) at application startup in `main.go`. If the value does not match, return an error so the pod fails to start rather than silently accepting the malformed input.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/main.go` (~lines 43, 47-78): `RepoScope` field definition and `Run` method where validation should be inserted
- `watcher/github/pkg/githubclient.go` (~lines 75-79): the `fmt.Sprintf` that interpolates `scope` into the search query
- `watcher/github/main_test.go`: check if there are any integration tests that set `REPO_SCOPE` and would need updating
</context>

<requirements>
1. Add a `validateRepoScope` helper in `watcher/github/main.go`. The `github.com/bborbe/errors` API requires `ctx` for both `New` and `Errorf` — thread it through:

   ```go
   var repoScopePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

   func validateRepoScope(ctx context.Context, scope string) error {
       if !repoScopePattern.MatchString(scope) {
           return errors.Errorf(ctx, "repo scope %q must match ^[a-zA-Z0-9_.-]+$", scope)
       }
       return nil
   }
   ```

2. Call `validateRepoScope` at the start of `application.Run` in `watcher/github/main.go`, before `factory.CreateWatcher`:

   ```go
   if err := validateRepoScope(ctx, a.RepoScope); err != nil {
       return err
   }
   ```

3. Add `"regexp"` to the imports in `main.go`. Ensure the import alias for the bborbe errors lib is `"github.com/bborbe/errors"` (not stdlib `errors`).

4. Add a Ginkgo/Gomega test for `validateRepoScope` in `watcher/github/main_test.go` (matching the existing test style there) using `DescribeTable` / `Entry`:
   - Valid inputs: `"bborbe"`, `"my-org"`, `"org.name"`, `"org_name"`, `"Org123"`
   - Invalid inputs: `"user is:issue"` (space), `"user;drop"` (semicolon), `""` (empty), `"user+more"` (plus)

5. Run `cd watcher/github && make test` — must pass.

6. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Validation must fail-fast at startup (in `Run`, before `CreateWatcher`) — not at poll time
- The regex must be a package-level compiled `var` (never `regexp.MustCompile` inside a function body)
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
cd watcher/github && grep -n "validateRepoScope\|repoScopePattern" main.go
# Expected: two matches (declaration + call)

cd watcher/github && make precommit
</verification>
