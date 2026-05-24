---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `pkg/publisher.go` is entirely empty (only license header and package declaration) — dead code that serves no purpose
- `CreateKafkaSender` in `factory.go` constructs the sender directly via `cdb.NewCommandObjectSender` without using anything from `publisher.go`
</summary>

<objective>
Delete the empty `pkg/publisher.go` file and remove any stale references.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read before making changes:
- `watcher/github-pr/pkg/publisher.go` — confirm it is empty
- `watcher/github-pr/pkg/factory/factory.go` — confirm it does not reference publisher.go
- Grep for any references: `grep -rn "publisher\|Publisher" watcher/github-pr/pkg/`
</context>

<requirements>
1. **Delete `watcher/github-pr/pkg/publisher.go`:**
   ```bash
   rm watcher/github-pr/pkg/publisher.go
   ```

2. **Confirm no references to publisher.go exist:**
   ```bash
   grep -rn "publisher" watcher/github-pr/pkg/ --include="*.go" | grep -v "_test.go" | grep -v "mocks/"
   ```
   If any production code references `publisher.go`, fix those references first.

3. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```

4. **Run `make precommit`:**
   ```bash
   cd watcher/github-pr && make precommit
   ```
</requirements>

<constraints>
- Only delete `watcher/github-pr/pkg/publisher.go`
- Do NOT commit — dark-factory handles git
- If any production code references symbols from `publisher.go`, do NOT delete until those references are removed
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm publisher.go is deleted:
ls watcher/github-pr/pkg/publisher.go 2>&1

# Confirm no references remain:
grep -rn "publisher" watcher/github-pr/pkg/*.go | grep -v "_test.go" | wc -l
</verification>
