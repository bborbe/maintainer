---
status: committing
summary: Updated splitRepoKey to accept three-segment host/owner/repo allowlist entries (stripping host for GitHub API calls while preserving host in cursor key), added regression tests covering API args, cursor key shape, and task body content, added internal table test for splitRepoKey with all edge cases, and updated CHANGELOG.md.
container: maintainer-090-fix-split-repo-key-host-prefix
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T00:00:00Z"
queued: "2026-05-05T22:05:59Z"
started: "2026-05-05T22:06:01Z"
---

<summary>
- The build watcher accepts allowlist entries like `github.com/bborbe/maintainer` (host-qualified, three slash-separated segments)
- Internally it splits each entry to extract the GitHub `owner` and `repo` for API calls
- A mismatch between the allowlist's three-segment format and the splitter's two-segment expectation made every prod GitHub API call use the wrong owner/repo (host gets passed as owner)
- The splitter is updated to drop the host segment when present, leaving the two-segment legacy form intact
- A regression test covers the host-prefixed input path so this can't slip again
- The cursor key, task body, and DeriveTaskID input use the canonical `owner/repo` form (host stripped) so vault task IDs stay stable across host changes
</summary>

<objective>
Fix the splitRepoKey bug introduced when spec-015 wired `ParseRepoAllowlist` (three-segment `host/owner/repo`) into the watcher loop where `splitRepoKey` only handled two-segment `owner/repo`. With the bug, `GetDefaultBranch(ctx, "github.com", "bborbe/maintainer")` is called instead of `(ctx, "bborbe", "maintainer")` — every GitHub API call 404s in prod.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `docs/build-watcher.md` for episode-SHA semantics + state machine (so the cursor-key change preserves the contract).
Read `~/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for error wrapping conventions.
Read `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for Ginkgo/Gomega test conventions.

Files to read before making any changes:
- `watcher/github-build/pkg/watcher.go` — full file. `splitRepoKey` is at the bottom (~line 213). Note the call sites at line 97 (`pollRepo`) and inside the cursor map keying at line 98 (`GetOrCreateRepoState(cursor, repoKey)`).
- `watcher/github-build/pkg/watcher_test.go` — full file. Tests currently use the two-segment form `"owner/repo"` everywhere.
- `watcher/github-build/pkg/filter/repo_allowlist_filter.go` — `ParseRepoAllowlist` validates the three-segment `host/owner/repo` regex; this is the prod input shape.
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Update `splitRepoKey` in `watcher/github-build/pkg/watcher.go`** to accept both formats:
   - Three segments (`host/owner/repo`): drop the host, return last two — this is the prod path produced by `ParseRepoAllowlist`
   - Two segments (`owner/repo`): unchanged — preserves existing tests + CLI/local-mode usage
   - Anything else: return `(key, "")` unchanged so the caller logs and skips

   Replacement:
   ```go
   // splitRepoKey extracts owner and repo from an allowlist entry.
   // Accepts both "host/owner/repo" (3 segments — the host is dropped, matches
   // ParseRepoAllowlist output) and "owner/repo" (2 segments). Anything else
   // returns the original key with an empty repo so the caller can skip it.
   func splitRepoKey(key string) (owner, repo string) {
   	parts := strings.Split(key, "/")
   	switch len(parts) {
   	case 3:
   		return parts[1], parts[2]
   	case 2:
   		return parts[0], parts[1]
   	default:
   		return key, ""
   	}
   }
   ```

2. **Decide cursor key form for host-prefixed inputs.** The cursor map (`cursor.Repos`) is currently keyed by whatever `repoKey` the loop iterates. For three-segment inputs that means the cursor file would store entries like `"github.com/bborbe/maintainer"`. That works (no breakage) but couples the cursor to the host. Keep the host in the cursor key — do NOT strip it for the cursor — so re-deploys with a different host value (e.g. `github.com` vs an enterprise host) start a fresh episode rather than silently colliding with existing state. This is intentional and matches the PR watcher's `RepoKey` shape (the PR watcher uses the same host-qualified key).

   No code change for this step — it documents the existing behavior. The change in step 1 only affects what `splitRepoKey` returns to the GitHub API call sites, NOT the cursor key.

3. **Add regression tests in `watcher/github-build/pkg/watcher_test.go`.** Mirror the existing two-segment test setup but use the three-segment form for one new context. Add ONE new context block in the existing Ginkgo suite (do not duplicate the whole file). At minimum cover:
   - **Host-prefixed allowlist entry → GitHub API called with stripped owner/repo.** Construct a fake `GitHubClient` (existing counterfeit), pass `[]string{"github.com/owner/repo"}` as the allowlist, and assert the captured `GetDefaultBranch` call args are `("owner", "repo")`, NOT `("github.com", "owner/repo")`.
   - **Host-prefixed allowlist entry → cursor key keeps the host.** Same setup, after a green→red transition assert `cursor.Repos["github.com/owner/repo"]` exists (NOT `cursor.Repos["owner/repo"]`).
   - **Host-prefixed allowlist entry → task body uses the stripped form.** Assert `cmd.Frontmatter["repo"]` equals `"owner/repo"` (NOT `"github.com/owner/repo"`) and the markdown body title is `# Build Failure: owner/repo`.

4. **Add a unit test for `splitRepoKey` itself.** Create or extend a table test in the same `watcher_test.go` (or a new `watcher_internal_test.go` if testing the unexported function requires package `pkg` instead of `pkg_test` — pick whichever the existing file structure permits). Cases:
   - `"github.com/owner/repo"` → `("owner", "repo")`
   - `"owner/repo"` → `("owner", "repo")`
   - `"single"` → `("single", "")`
   - `""` → `("", "")`
   - `"a/b/c/d"` → `("a/b/c/d", "")` (4 segments — invalid, return unchanged)

5. **Verify the fix doesn't break existing tests.** All existing tests using `"owner/repo"` (two-segment) MUST still pass — the switch case for `len == 2` preserves the legacy behavior.

6. **Update CHANGELOG.md** under `## Unreleased`. Append a single bullet:
   ```
   - fix(watcher/github-build): splitRepoKey now strips the host prefix from `host/owner/repo` allowlist entries so GitHub API calls use the correct `owner` and `repo` (regression from spec-015)
   ```

7. **Run `make precommit`** in `watcher/github-build/`:
   ```bash
   cd watcher/github-build && make precommit
   ```
</requirements>

<constraints>
- Only edit `watcher/github-build/pkg/watcher.go`, `watcher/github-build/pkg/watcher_test.go` (and optionally `watcher_internal_test.go`), and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- The cursor key MUST keep the host prefix when allowlist entries are host-qualified — do NOT strip it from the cursor map key
- The GitHub API call sites (`GetDefaultBranch`, `GetWorkflowRuns`) MUST receive the stripped two-segment owner + repo
- The task body title (`# Build Failure: <owner>/<repo>`) and `Frontmatter["repo"]` MUST use the stripped form (no host)
- `DeriveTaskID(owner, repo, episodeSHA)` MUST receive the stripped form so task IDs are stable regardless of host (UUID5 collision avoidance is fine — different repos still get different IDs because owner+repo differ)
- Existing tests using two-segment `"owner/repo"` MUST still pass unchanged
- Error wrapping uses `github.com/bborbe/errors`; never `fmt.Errorf`
- Test additions follow Ginkgo v2 + Gomega patterns from `go-testing-guide.md`
- `make precommit` runs from `watcher/github-build/`, never at repo root
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm splitRepoKey handles 3 segments:
grep -A 12 "func splitRepoKey" watcher/github-build/pkg/watcher.go
# Expected: switch on len(parts), case 3 returns parts[1], parts[2]

# Confirm regression test exists:
grep -n "github.com/owner/repo\|host-prefixed\|three-segment" watcher/github-build/pkg/watcher_test.go
# Expected: at least one match

# Confirm splitRepoKey table test exists:
grep -n "splitRepoKey\|TestSplitRepoKey" watcher/github-build/pkg/watcher_test.go watcher/github-build/pkg/*internal*test.go 2>/dev/null
# Expected: at least one match

# Confirm CHANGELOG entry:
grep -n "splitRepoKey\|host prefix" CHANGELOG.md
# Expected: one match under ## Unreleased
</verification>
