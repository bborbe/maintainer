---
status: completed
spec: [010-git-checkout-review-workdir]
summary: Implemented RepoManager, WorkdirConfig, and ParseCloneURL in agent/pr-reviewer/pkg/git/ with bare-clone caching, per-task worktrees, stale-worktree pruning, and full test coverage (87%)
container: code-reviewer-065-spec-010-repo-manager
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-29T09:00:00Z"
queued: "2026-04-29T12:10:35Z"
started: "2026-04-29T12:11:57Z"
completed: "2026-04-29T12:21:22Z"
branch: dark-factory/git-checkout-review-workdir
---

<summary>
- A new `RepoManager` interface is introduced in `pkg/git/` alongside the existing `WorktreeManager`; neither interface changes the other
- A bare-clone cache is managed per repo: first task clones with `git clone --bare`; subsequent tasks run `git fetch --prune origin` on the existing bare
- A half-cloned (crashed) bare directory is detected via `git rev-parse --git-dir`, removed, and re-cloned automatically on the next task
- A worktree is created per task via `git worktree add <worktreePath> <ref>` — no separate checkout step; a pre-existing worktree at the same path is reused without mutation
- A pod-startup prune operation walks all `*.git` dirs under `reposPath` and runs `git worktree prune` on each to clear stale admin entries
- `WorkdirConfig` carries the two root paths (`ReposPath`, `WorkPath`) with no defaults — callers set them per entry point
- `clone_url` is parsed into a `<host>/<owner>/<repo>.git` relative path; inputs with `..`, shell metacharacters, or fewer than 3 path components are rejected before any disk I/O
- `ref` is passed directly to `git worktree add` as a CLI argument (never interpolated into a shell string) and validated with the existing branch-name regexp
- `task_id` is validated as a UUID pattern before being joined into a filesystem path to prevent path traversal
- Unit tests cover every path in the acceptance criteria using `GinkgoT().TempDir()` for isolation; existing `WorktreeManager` tests are untouched
</summary>

<objective>
Implement `RepoManager`, `WorkdirConfig`, and `ParseCloneURL` in `agent/pr-reviewer/pkg/git/` so that a caller can ensure a bare clone, ensure a per-task worktree, and prune stale worktree admin entries — all with fail-loud behavior and no diff-based fallback. The existing `WorktreeManager` (CLI mode) is unchanged.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface + constructor + private struct pattern, counterfeiter annotation.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega suite, `GinkgoT().TempDir()`.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `errors.Wrapf`, never `fmt.Errorf`.
Read `go-security-linting.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `#nosec G204` with reason comment for `exec.CommandContext` with validated inputs.
Read `docs/architecture.md` — "Workdir & repo storage" and "Agent contract" sections for the exact path layout and contract fields.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/git/git.go` — existing `WorktreeManager`, `isValidBranchName`, `runGit` helpers; understand the pattern to mirror
- `agent/pr-reviewer/pkg/git/git_test.go` — existing test style: `GinkgoT().TempDir()`, `initGitRepo()`, `runCmd()`
- `agent/pr-reviewer/pkg/git/git_suite_test.go` — suite setup; do NOT add a second suite file
- `agent/pr-reviewer/pkg/git/git_validation_test.go` — existing validation tests; do not duplicate

**Key path layout (from architecture.md):**
```
/repos/<host>/<owner>/<repo>.git/   # bare clone
/work/<task-id>/                    # worktree per task
```
`ParseCloneURL("https://github.com/bborbe/maintainer.git")` → `"github.com/bborbe/maintainer.git"`

**Acceptance criteria test matrix (must cover all):**
- Clone path: fresh `reposPath`, no `.git` dir → `git clone --bare` executed
- Fetch path: valid bare already present → `git fetch --prune origin` executed, no re-clone
- Half-clone recovery: directory present but `git rev-parse --git-dir` fails → remove + re-clone
- Worktree create: no worktree at `<workPath>/<taskID>` → `git worktree add` executed
- Worktree reuse: worktree dir already exists → return path without calling `git worktree add` again
- Ref pinning: worktree is created with the exact `ref` argument, not a separate checkout
- Stale-worktree pruning: `PruneAllWorktrees` runs `git worktree prune` for each `*.git` dir under `reposPath`
</context>

<requirements>
**Execute steps in this order.**

1. **Create `agent/pr-reviewer/pkg/git/workdir_config.go`**:
   ```go
   // WorkdirConfig holds the root paths for bare-clone caching and per-task worktrees.
   type WorkdirConfig struct {
       ReposPath string // root for bare clones: <ReposPath>/<host>/<owner>/<repo>.git
       WorkPath  string // root for worktrees:   <WorkPath>/<task_id>
   }
   ```
   No defaults — callers set paths per entry point.

2. **Create `agent/pr-reviewer/pkg/git/clone_url.go`**:

   Implement `ParseCloneURL(ctx context.Context, rawURL string) (string, error)` that:
   - Parses `rawURL` with `url.Parse`; rejects anything without a non-empty `Host`
   - Strips a leading `/` and a trailing `.git` from `Path` before splitting by `/`
   - Requires exactly 2 path segments after the host (i.e., `<owner>/<repo>`)
   - Rejects any segment that is empty, equals `.`, equals `..`, contains `/`, or contains any character outside `[a-zA-Z0-9._\-]`
   - Returns `"<host>/<owner>/<repo>.git"` (no leading slash)
   - Returns a descriptive error (wrapped with `errors.Errorf(ctx, ...)`) for every rejection

   The `ctx` parameter is required because `errors.Errorf` takes `ctx` as its first argument (see `pkg/prurl/prurl.go` and `pkg/git/git.go` for the convention).

   Add unit tests in a new file `agent/pr-reviewer/pkg/git/clone_url_test.go` (package `git_test`):
   - Valid: `https://github.com/bborbe/maintainer.git` → `"github.com/bborbe/maintainer.git"`
   - Valid: `https://github.com/bborbe/maintainer` (no `.git`) → `"github.com/bborbe/maintainer.git"`
   - Reject: empty string
   - Reject: `https://github.com/../foo/bar.git` (path traversal in owner)
   - Reject: `https://github.com/owner` (only one segment — missing repo)
   - Reject: `https://github.com/owner/repo/extra` (three segments — too many)
   - Reject: `https://github.com/owner/repo;rm -rf /` (shell metacharacter)

3. **Create `agent/pr-reviewer/pkg/git/repo_manager.go`**:

   ```go
   //counterfeiter:generate -o ../../mocks/repo-manager.go --fake-name RepoManager . RepoManager
   type RepoManager interface {
       // EnsureBareClone ensures a bare clone of cloneURL exists and is up to date.
       // Returns the absolute path to the bare repo.
       EnsureBareClone(ctx context.Context, cloneURL string) (string, error)
       // EnsureWorktree ensures a worktree for the given ref and taskID exists.
       // Returns the absolute path to the worktree.
       EnsureWorktree(ctx context.Context, cloneURL, ref, taskID string) (string, error)
       // PruneAllWorktrees runs `git worktree prune` on every bare repo under reposPath.
       PruneAllWorktrees(ctx context.Context) error
   }
   ```

   Constructor:
   ```go
   func NewRepoManager(cfg WorkdirConfig) RepoManager {
       return &repoManager{cfg: cfg}
   }
   ```

   **`EnsureBareClone` algorithm:**
   1. Call `ParseCloneURL(ctx, cloneURL)` → `relPath`; fail loud on error
   2. `barePath = filepath.Join(cfg.ReposPath, relPath)`
   3. If `barePath` does not exist: `os.MkdirAll(filepath.Dir(barePath), 0750)`, then `git clone --bare <cloneURL> <barePath>` — fail loud if either fails
   4. If `barePath` exists: run `git -C <barePath> rev-parse --git-dir`
      - If that fails (half-clone): `os.RemoveAll(barePath)`, then `os.MkdirAll`, then `git clone --bare` — fail loud on any step
      - If that succeeds: run `git -C <barePath> fetch --prune origin` — fail loud on error
   5. Return `barePath, nil`

   All `exec.CommandContext` calls must use `// #nosec G204 -- <justification>` and must capture stderr; wrap git errors as `errors.Errorf(ctx, "git <cmd>: %s", stderr)`.

   **`EnsureWorktree` algorithm:**
   1. Validate `ref` using the existing package-level `isValidBranchName` — its regex `^[a-zA-Z0-9._/@\-]+$` happens to also accept tag names (`v1.2.3`) and 40-hex SHAs (`abc123...`); fail loud if invalid. (Note: this couples ref validation to a function named for branches; if `isValidBranchName` is later tightened, the regression tests below will catch it.)
   2. Validate `taskID` matches `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`; fail loud if invalid
   3. Call `EnsureBareClone(ctx, cloneURL)` → `barePath`; propagate error
   4. `worktreePath = filepath.Join(cfg.WorkPath, taskID)`
   5. If `worktreePath` exists (`os.Stat` no error): return `worktreePath, nil` — **do NOT reset or re-checkout**
   6. Run `git -C <barePath> worktree add <worktreePath> <ref>` — fail loud on error
   7. Return `worktreePath, nil`

   **`PruneAllWorktrees` algorithm:**
   1. If `cfg.ReposPath` does not exist, return nil (no-op — fresh pod)
   2. Use `filepath.WalkDir` to find all entries whose name ends in `.git` and that are directories
   3. For each such `path`: run `git -C <path> worktree prune`; log a warning on error but continue (best-effort); **return `filepath.SkipDir` from the walker callback after handling a `*.git` match** so the walker does not descend into the bare repo's internals (`objects/`, `refs/`, `worktrees/`, etc.) — descent would waste time and risk false-positive matches on nested fixtures
   4. Return nil

4. **Generate counterfeiter mock**:
   ```bash
   cd agent/pr-reviewer && go generate ./pkg/git/...
   ```
   Verify `mocks/repo-manager.go` is created.

5. **Create `agent/pr-reviewer/pkg/git/repo_manager_test.go`** (package `git_test`):

   Use `GinkgoT().TempDir()` for all directories. Set up a real local git bare repo using `initBareRepo()` helper (create a bare repo, commit one file) — model after the helpers in `git_test.go`.

   Cover all items in the acceptance criteria test matrix from `<context>`:
   - **Clone path**: `ReposPath` is an empty temp dir → call `EnsureBareClone` → confirm barePath exists and `git -C barePath rev-parse --git-dir` succeeds
   - **Fetch path**: call `EnsureBareClone` twice → confirm the bare exists after both calls and no error on second call
   - **Half-clone recovery**: create `barePath` dir with garbage content → call `EnsureBareClone` → confirm dir is replaced with a valid bare repo
   - **Worktree create**: call `EnsureWorktree` → confirm worktreePath exists and contains the checked-out ref
   - **Worktree reuse**: call `EnsureWorktree` twice with the same taskID → second call returns same path without error; confirm no double-add
   - **Ref pinning (branch)**: `EnsureWorktree` with a branch name → confirm worktree HEAD matches that branch
   - **Ref pinning (tag)**: `EnsureWorktree` with a tag name like `v1.2.3` → confirm worktree HEAD points at that tag (locks in `isValidBranchName` accepting tags)
   - **Ref pinning (SHA)**: `EnsureWorktree` with a 40-hex commit SHA → confirm worktree HEAD is at that commit (locks in `isValidBranchName` accepting SHAs)
   - **Stale-worktree pruning**: run `git -C barePath worktree add /tmp/stale-wt-test <ref>`, then manually delete `/tmp/stale-wt-test` to create a stale admin entry, then call `PruneAllWorktrees` → confirm `git -C barePath worktree list` no longer shows the stale entry
   - **ParseCloneURL errors** → `EnsureBareClone` fails loud with a descriptive error (test one bad URL shape)
   - **Invalid taskID** → `EnsureWorktree` fails loud before touching disk
   - **Invalid ref** → `EnsureWorktree` fails loud before touching disk

6. **Run tests and verify**:
   ```bash
   cd agent/pr-reviewer && go test ./pkg/git/...
   cd agent/pr-reviewer && go test -coverprofile=/tmp/cover.out ./pkg/git/... \
     && go tool cover -func=/tmp/cover.out | grep repo_manager
   ```
   Coverage for `repo_manager.go` must be ≥80%.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/pkg/git/` and `agent/pr-reviewer/mocks/`
- Do NOT commit — dark-factory handles git
- Do NOT modify `git.go` or any existing `*_test.go` file — only add new files
- Do NOT add a second Ginkgo suite file; append to the existing `git_suite_test.go` only if it needs new imports
- Error wrapping uses `github.com/bborbe/errors` — never `fmt.Errorf`, never bare `return err`; use `errors.Errorf` or `errors.Wrapf` with `ctx` as first argument
- All `exec.CommandContext` calls with variable inputs must carry `// #nosec G204 -- <reason>` explaining why each variable argument is safe (validated by preceding check)
- `ref` is passed as a CLI argument to `exec.Command`, never interpolated into a shell string
- `task_id` is joined into a path only after UUID-regexp validation
- No diff-based fallback exists anywhere in this file — all failures return errors
- Existing tests (`WorktreeManager` suite) must still pass
- `make precommit` runs from `agent/pr-reviewer/`, never from repo root
</constraints>

<verification>
cd agent/pr-reviewer && go test ./pkg/git/...

cd agent/pr-reviewer && go test -coverprofile=/tmp/cover.out ./pkg/git/... \
  && go tool cover -func=/tmp/cover.out | grep -E "repo_manager|clone_url"

cd agent/pr-reviewer && make precommit
</verification>
