---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `pkg/config.go:72` recompiles a regex on every call to `resolveEnvVar`
- This function may be called multiple times per run for token resolution
- Fix: move the compiled `*regexp.Regexp` to package level so it is compiled once at program startup
</summary>

<objective>
Move the `envVarRefRegexp` compilation from inside `resolveEnvVar` to package level in `agent/pr-reviewer/pkg/config.go`, eliminating redundant regex recompilation on every call.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — private helpers, package-level compiled regex.

Files to read before making changes:
- `agent/pr-reviewer/pkg/config.go` — full file; understand `resolveEnvVar` (~line 72) and the surrounding context
- `agent/pr-reviewer/pkg/config_test.go` — full file; understand existing tests for `resolveEnvVar`
</context>

<requirements>
**Execute steps in order. Run `make test` after the fix. Run `make precommit` only at the final step.**

1. **Move `envVarRefRegexp` to package level in `agent/pr-reviewer/pkg/config.go`**

   Add at package level (after imports, before the first type/function declaration):
   ```go
   // envVarRefRegexp matches ${VAR_NAME} environment variable references.
   var envVarRefRegexp = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)\}$`)
   ```

   Remove the `regexp.MustCompile` call from inside `resolveEnvVar` and use the package-level variable:
   ```go
   func resolveEnvVar(value string) string {
       matches := envVarRefRegexp.FindStringSubmatch(value)
       if len(matches) < 2 {
           return value
       }
       envName := matches[1]
       if envVal := os.Getenv(envName); envVal != "" {
           return envVal
       }
       return value
   }
   ```

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
- Only change `agent/pr-reviewer/pkg/config.go`
- Do NOT commit — dark-factory handles git
- Use `regexp.MustCompile` at package level — this panics if the regex is invalid (which would be a development-time bug, not a runtime bug)
- Existing tests must still pass
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
