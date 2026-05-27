---
status: approved
spec: [044-github-release-watcher-implementation]
created: "2026-05-27T20:38:37Z"
queued: "2026-05-27T20:57:47Z"
---

<summary>
- `LoadCursor` and `SaveCursor` get real implementations: JSON read/write with atomic temp-file + rename on save
- Missing cursor file returns a fresh empty cursor (cold-start is valid)
- Corrupt JSON returns an error (refuse to proceed; matches `watcher/github-build` policy)
- Tests cover round-trip, missing file, corrupt JSON, atomic write (no leftover `.tmp` after success), and the named acceptance criterion `SaveCursor + LoadCursor round-trip preserves Repos map`
- Cursor file path stays at `/data/cursor.json` for PVC compatibility with the deployment manifests
</summary>

<objective>
Replace TODO stubs in `watcher/github-release/pkg/cursor.go` with working `LoadCursor` and `SaveCursor`, and add Ginkgo unit tests covering the round-trip, cold-start, corruption, and atomic-write semantics specified in the spec failure-mode table.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`

Read these files end-to-end:
- `watcher/github-release/pkg/cursor.go` — stub + the `Cursor`/`RepoState` struct shapes you MUST preserve
- `watcher/github-release/pkg/watcher.go` — confirms how `NewCursorReader` reads the `Cursor.Repos[repoKey].LastSeenMasterSHA` field (read-side contract)

Reference implementations the spec instructs to mirror verbatim — read these in full and match the error-handling style:
- `/workspace/watcher/github-build/pkg/cursor.go` — same `Cursor.Repos map[string]*RepoState` shape; corrupt-JSON-returns-error policy; atomic temp-file + rename. This is the canonical pattern; copy its error-wrapping idioms verbatim.
- `/workspace/watcher/github-pr/pkg/cursor_test.go` — Ginkgo test layout: per-test `tmpDir` via `os.MkdirTemp`, cleanup in `AfterEach`, `filepath.Join(tmpDir, "cursor.json")`.

Note on cold-start semantics: github-build's `LoadCursor` returns the cold-start when the file is MISSING; corrupt JSON returns an ERROR. github-pr's variant tolerates corruption by logging and falling back to cold-start. The spec instructs to follow `watcher/github-build`'s stricter policy — corrupt = error.

Counterfeiter / mock note: `cursor.go` has no `//counterfeiter:generate` directive, so no mock regen is required.
</context>

<requirements>

**Execute steps in order. Run `make test` after step 4. Run `make precommit` only at the final step.**

1. **Implement `LoadCursor` in `watcher/github-release/pkg/cursor.go`** (replacing the TODO body). Keep the exported signature exactly:

   ```go
   func LoadCursor(ctx context.Context, path string) (*Cursor, error)
   ```

   Implementation (mirror `watcher/github-build/pkg/cursor.go` `LoadCursor`):
   - `data, err := os.ReadFile(path)` with the `// #nosec G304 -- path is config-controlled` comment on the same line.
   - If `os.IsNotExist(err)`: emit `glog.V(2).Infof("cursor file not found, cold-start path=%s", path)` and return `&Cursor{Repos: make(map[string]*RepoState)}, nil`.
   - On any other read error: return `nil, errors.Wrapf(ctx, err, "read cursor file path=%s", path)`.
   - `var c Cursor` then `json.Unmarshal(data, &c)`. On unmarshal error: return `nil, errors.Wrapf(ctx, err, "unmarshal cursor file path=%s", path)` (strict: refuse to proceed on corruption; do NOT silently re-initialize).
   - If `c.Repos == nil`, initialize it via `c.Repos = make(map[string]*RepoState)` (handles JSON with `"repos": null`).
   - Return `&c, nil`.

   Imports to add: `encoding/json`, `os`, `github.com/golang/glog`. Keep `context` and `github.com/bborbe/errors`.

2. **Implement `SaveCursor` in `watcher/github-release/pkg/cursor.go`** (replacing the TODO body). Keep the exported signature exactly:

   ```go
   func SaveCursor(ctx context.Context, path string, c *Cursor) error
   ```

   Implementation:
   - `data, err := json.Marshal(c)`. On error: `return errors.Wrapf(ctx, err, "marshal cursor state path=%s", path)`.
   - `if err := os.WriteFile(path+".tmp", data, 0600); err != nil` with `// #nosec G306 -- intentional 0600` comment on the same line. On error: `return errors.Wrapf(ctx, err, "write cursor tmp path=%s", path)`.
   - `if err := os.Rename(path+".tmp", path); err != nil`: `return errors.Wrapf(ctx, err, "rename cursor tmp path=%s", path)`.
   - Return `nil` on success.

   This sequence (temp-file write + rename) is the atomic-write guarantee — a torn write leaves a `.tmp` file but never a half-written cursor. Match it byte-for-byte against `watcher/github-build/pkg/cursor.go` `SaveCursor`.

3. **Create `watcher/github-release/pkg/cursor_test.go`** as package `pkg_test` (external test package). Reuse the suite file from prompt 1 (`pkg/suite_test.go`); do not create another one.

   The test file must include:

   ```go
   package pkg_test

   import (
       "context"
       "encoding/json"
       "os"
       "path/filepath"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-release/pkg"
   )
   ```

   `Describe("pkg.Cursor", ...)` with these `It` blocks (use the exact `It` text where specified — these are spec acceptance criteria):

   a. **`It("LoadCursor returns cold-start empty cursor when file is missing")`**: create a tmpDir, point `path` at a non-existent file, call `LoadCursor`, assert `err == nil`, `cursor != nil`, `cursor.Repos != nil`, `len(cursor.Repos) == 0`.

   b. **`It("LoadCursor returns error on corrupt JSON")`**: write `[]byte("not json")` to the cursor path, call `LoadCursor`, assert `err != nil`, `cursor == nil`. (Strict policy — corrupt = error, NOT silent reset.)

   c. **`It("LoadCursor handles repos: null by initializing the map")`**: write `[]byte("{\"repos\":null}")` to the cursor path, call `LoadCursor`, assert `err == nil`, `cursor.Repos != nil`, `len(cursor.Repos) == 0`. Without this guard, downstream `cursor.Repos["foo"] = ...` would nil-panic.

   d. **`It("SaveCursor + LoadCursor round-trip preserves Repos map")`** — exact spec wording: build a `&pkg.Cursor{Repos: map[string]*pkg.RepoState{"github.com/bborbe/docker-utils": {LastSeenMasterSHA: "d630ef3526cfc57fbdccd9ba53c5c3a02945e407"}, "github.com/bborbe/disk-status": {LastSeenMasterSHA: "102b3b1abcdef0000000000000000000000000a0"}}}`, save it, then `LoadCursor` it back and assert the loaded cursor equals the original via `Expect(loaded.Repos).To(HaveLen(2))` plus per-key `LastSeenMasterSHA` equality checks.

   e. **`It("SaveCursor does atomic write — no .tmp file remains after success")`**: save a cursor, then `os.Stat(path + ".tmp")` and assert `os.IsNotExist(err)` (the rename consumed the .tmp file).

   f. **`It("SaveCursor returns error when target directory does not exist")`**: call `SaveCursor` with a path under a nonexistent directory like `filepath.Join(tmpDir, "missing-dir", "cursor.json")`, assert `err != nil`. This exercises the temp-file write error wrapping path.

   Test fixture lifecycle: in `BeforeEach`, `tmpDir, err = os.MkdirTemp("", "cursor-release-*")`; in `AfterEach`, `_ = os.RemoveAll(tmpDir)` with `// #nosec G104 -- best-effort` comment.

4. **Run unit tests**:
   ```bash
   cd watcher/github-release && make test
   ```

5. **Run full precommit**:
   ```bash
   cd watcher/github-release && make precommit
   ```

</requirements>

<constraints>
- Mirror `watcher/github-pr` Go patterns verbatim: `errors.Wrapf(ctx, err, ...)` from `github.com/bborbe/errors`; `glog.V(2).Infof` for cold-start log; external `_test` package.
- Cursor file format is JSON via `encoding/json`; atomic write via `os.WriteFile(path+".tmp", 0600)` then `os.Rename`. The schema (`{"repos": {"<key>": {"last_seen_master_sha": "..."}}}`) is FROZEN — kubectl-mounted `/data/cursor.json` PVCs may already have files in this shape; changing the JSON tags would break the upgrade path.
- Corrupt JSON MUST return an error (matches `watcher/github-build` policy). Do NOT silently reset like `watcher/github-pr` does.
- No `context.Background()` in production paths. (Tests may use `context.Background()` — they are `*_test.go` files.)
- No `fmt.Errorf` in production paths — use `errors.Wrapf`.
- Do NOT commit — dark-factory handles git.
- Do NOT modify any file outside `watcher/github-release/pkg/cursor.go` and `watcher/github-release/pkg/cursor_test.go`. (If `pkg/suite_test.go` does not exist from prompt 1, create it with the same body specified there.)
- Preserve the existing godoc above `LoadCursor` and `SaveCursor` — those references to github-pr / github-build patterns help future readers.
</constraints>

<verification>
```bash
cd watcher/github-release

# No TODOs in cursor.go
grep -c "TODO" pkg/cursor.go
# Expected: 0

# Atomic write pattern present
grep -n "path+\".tmp\"" pkg/cursor.go
grep -n "os.Rename" pkg/cursor.go

# Test file exists with spec-named acceptance criterion
grep -F "SaveCursor + LoadCursor round-trip preserves Repos map" pkg/cursor_test.go

# Error wrapping used throughout
grep -n "errors.Wrapf" pkg/cursor.go
# Expected: at least 4 occurrences (read, unmarshal, write-tmp, rename)

# No fmt.Errorf in production cursor file
grep -n "fmt.Errorf" pkg/cursor.go
# Expected: no matches

# Tests pass
make test

# Full precommit
make precommit
```
</verification>
