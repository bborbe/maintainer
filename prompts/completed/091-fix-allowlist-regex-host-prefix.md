---
status: completed
summary: Replaced 2-segment owner/repo regex with 3-segment host/owner/repo regex in watcher/github-build allowlist parser, created comprehensive test file mirroring PR watcher, updated stale references in filter.go/main.go/run-once/main.go, and added CHANGELOG entry.
container: maintainer-091-fix-allowlist-regex-host-prefix
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T00:15:00Z"
queued: "2026-05-05T22:26:01Z"
started: "2026-05-05T22:26:02Z"
completed: "2026-05-05T22:30:36Z"
---

<summary>
- The build watcher's allowlist parser refuses to accept host-qualified entries (e.g. `github.com/bborbe/maintainer`) at startup
- The PR watcher's parser accepts the same entries — both services share the `REPO_ALLOWLIST` env var in `dev.env` and `prod.env`
- This mismatch means the build watcher fails to start in any environment that reuses the existing shared allowlist value
- The fix aligns the build watcher's regex with the PR watcher's exactly so the same value parses in both
- Tests cover both the new accepted shape and the previously valid two-segment form (still accepted as a fallback for local CLI mode)
- A regression test ensures startup with the real `dev.env` value succeeds
</summary>

<objective>
Align `watcher/github-build/pkg/filter/ParseRepoAllowlist` with `watcher/github-pr/pkg/filter/ParseRepoAllowlist` so the build watcher accepts the existing shared `REPO_ALLOWLIST` env value (`github.com/bborbe/go-skeleton,github.com/bborbe/...`). Without this, the build watcher refuses to start when `REPO_ALLOWLIST` contains host-qualified entries.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for Ginkgo/Gomega test conventions.

Files to read fully before making any changes:
- `watcher/github-pr/pkg/filter/repo_allowlist_filter.go` — canonical pattern. Regex: `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$` (3-segment host/owner/repo). Error message references the same regex. Mirror exactly.
- `watcher/github-pr/pkg/filter/repo_allowlist_filter_test.go` — canonical test pattern (covers valid + invalid shapes, empty input, trailing commas, whitespace).
- `watcher/github-build/pkg/filter/repo_allowlist_filter.go` — current 2-segment regex (incorrect). This is what you replace.
- `watcher/github-build/pkg/filter/repo_allowlist_filter_test.go` — current tests using 2-segment values. These need to be updated.
- `dev.env` and `prod.env` (root) — confirm `REPO_ALLOWLIST` uses 3-segment `host/owner/repo` entries.
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Replace the regex and validation logic in `watcher/github-build/pkg/filter/repo_allowlist_filter.go`** so it matches the PR watcher exactly. Copy the regex pattern, the error message, and the doc comment from `watcher/github-pr/pkg/filter/repo_allowlist_filter.go` verbatim — change ONLY the package import paths if any (the file itself is in package `filter`, no rename needed).

   The new regex MUST be `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$` (note the host segment uses `.-` only, not `_`, matching the PR watcher).

   Update the doc comment for `ParseRepoAllowlist` to say it returns validated `host/owner/repo` keys (not `owner/repo`). Update the error message to reference the new regex.

   `NewRepoAllowlistFilter` and `repoAllowlistFilter` (the filter struct) need NO changes — they compare `repoKey` strings as-is. The string passed in is now host-qualified, and the watcher already passes the same host-qualified key (see `splitRepoKey` fix in the prior prompt — host stays in the cursor key + filter input, only stripped at the GitHub API call site).

2. **Update `watcher/github-build/pkg/filter/repo_allowlist_filter_test.go`** to mirror `watcher/github-pr/pkg/filter/repo_allowlist_filter_test.go`. Cases:
   - Valid: `"github.com/bborbe/go-skeleton"` → returns `["github.com/bborbe/go-skeleton"]`
   - Valid multi: `"github.com/bborbe/a,github.com/bborbe/b"` → returns both, in order
   - Valid with whitespace + trailing comma: `"github.com/bborbe/a , github.com/bborbe/b,"` → returns both, trimmed, dropped empty
   - Empty input: `""` → returns `(nil, nil)` (no error)
   - Invalid (2-segment): `"bborbe/repo"` → returns error mentioning the required format
   - Invalid (4-segment): `"a/b/c/d"` → returns error
   - Invalid (host segment with underscore — host regex is stricter): `"git_hub.com/owner/repo"` → returns error
   - `RepoAllowlistFilter.Skip`: empty allowlist → never skips; non-empty → skips entries not on list (including by exact-string match).

3. **Add a startup-shape regression test** that exercises the realistic `dev.env` value:
   - Input: `"github.com/bborbe/go-skeleton,github.com/bborbe/jira-task-creator"` (the actual `dev.env` value)
   - Expected: parse succeeds, returns 2 entries

4. **Update adjacent stale `owner/repo` references** so the user-facing surface is consistent. None are functional bugs — they are stale comments/usage strings/error messages from the prior 2-segment regime:

   a. **`watcher/github-build/pkg/filter/filter.go:17`** — `RepoFilter` interface comment currently reads `// repoKey = "owner/repo"`. Change to `// repoKey = "host/owner/repo"`.

   b. **`watcher/github-build/main.go:61`** — error string currently says `"REPO_ALLOWLIST must be non-empty: set at least one owner/repo entry"`. Change to `"... set at least one host/owner/repo entry"`.

   c. **`watcher/github-build/cmd/run-once/main.go:35`** — `usage:` tag currently says `"Comma-separated owner/repo allowlist; MUST be non-empty"`. Change to `"Comma-separated host-qualified repo allowlist (host/owner/repo); MUST be non-empty"` to match `main.go:45`.

   d. **`watcher/github-build/cmd/run-once/main.go:46`** — error string currently says `"... set at least one owner/repo entry"`. Change to `"... set at least one host/owner/repo entry"`.

5. **Run `make precommit`** in `watcher/github-build/`:
   ```bash
   cd watcher/github-build && make precommit
   ```

6. **Update CHANGELOG.md** under `## Unreleased`. Append a single bullet:
   ```
   - fix(watcher/github-build): allowlist parser accepts host-qualified `host/owner/repo` entries (matches PR watcher; build watcher would previously refuse startup against the shared `REPO_ALLOWLIST` env value)
   ```
</requirements>

<constraints>
- Only edit `watcher/github-build/pkg/filter/repo_allowlist_filter.go`, `watcher/github-build/pkg/filter/repo_allowlist_filter_test.go`, `watcher/github-build/pkg/filter/filter.go`, `watcher/github-build/main.go`, `watcher/github-build/cmd/run-once/main.go`, and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- The new regex MUST match the PR watcher's exactly: `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`
- The error message MUST reference `host/owner/repo` (not `owner/repo`)
- `NewRepoAllowlistFilter` signature and `repoAllowlistFilter.Skip` behavior MUST be unchanged (still string-equality on the host-qualified key — the watcher's `splitRepoKey` fix already handles host stripping at the API boundary)
- Existing watcher tests using two-segment `"owner/repo"` allowlist entries (in `watcher/github-build/pkg/watcher_test.go`) bypass `ParseRepoAllowlist` by calling `NewWatcher` directly with the slice — they MUST still pass. Do NOT modify those tests in this prompt.
- Error wrapping uses `github.com/bborbe/errors`; never `fmt.Errorf`
- Test additions follow Ginkgo v2 + Gomega patterns
- `make precommit` runs from `watcher/github-build/`, never at repo root
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm the regex matches the PR watcher's:
grep "MustCompile" watcher/github-build/pkg/filter/repo_allowlist_filter.go
# Expected: `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`

# Confirm the error message references host/owner/repo:
grep "host/owner/repo" watcher/github-build/pkg/filter/repo_allowlist_filter.go

# Confirm dev.env-shape regression test exists:
grep -n "go-skeleton\|jira-task-creator" watcher/github-build/pkg/filter/repo_allowlist_filter_test.go
# Expected: at least one match

# Confirm 2-segment input now rejected:
grep -n "bborbe/repo\|two-segment" watcher/github-build/pkg/filter/repo_allowlist_filter_test.go

# Confirm PR + build watcher regexes are byte-identical:
diff <(grep -A1 "MustCompile" watcher/github-pr/pkg/filter/repo_allowlist_filter.go | grep -o "\^.*\$") \
     <(grep -A1 "MustCompile" watcher/github-build/pkg/filter/repo_allowlist_filter.go | grep -o "\^.*\$")
# Expected: empty diff

# Confirm adjacent stale references updated:
grep -n "owner/repo" watcher/github-build/main.go watcher/github-build/cmd/run-once/main.go watcher/github-build/pkg/filter/filter.go
# Expected: no bare "owner/repo" without "host/" prefix in those files (matches inside "host/owner/repo" are fine)

# Confirm CHANGELOG entry:
grep -n "host-qualified\|REPO_ALLOWLIST" CHANGELOG.md | tail -3
</verification>
