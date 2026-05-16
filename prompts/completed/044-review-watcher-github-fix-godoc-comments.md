---
status: completed
summary: Fixed six GoDoc comments in watcher/github/pkg to start with the declared item name, and created pkg/doc.go with a package-level documentation comment.
container: code-reviewer-044-review-watcher-github-fix-godoc-comments
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T14:51:25Z"
started: "2026-04-28T14:54:21Z"
completed: "2026-04-28T14:57:37Z"
---

<summary>
- Six GoDoc comments do not start with the name of the item they document
- go doc renders these incorrectly — "State holds..." instead of "Cursor holds..."
- GoDoc convention requires the first word of the comment to be the item name
- Additionally the pkg package has no package-level documentation comment (missing doc.go)
- All fixes are pure documentation changes with no behavioral impact
- Files affected: cursor.go, filter.go, publisher.go, taskid.go, and new doc.go
</summary>

<objective>
Fix six GoDoc comments that do not start with the declared item name, and create `watcher/github/pkg/doc.go` with a package-level documentation comment. All changes are documentation only — no logic changes.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/cursor.go` (~lines 20, 26, 49): `Cursor` struct comment says "State holds...", `LoadCursor` comment says "Load reads...", `SaveCursor` comment says "Save persists..."
- `watcher/github/pkg/filter.go` (~line 17): `ShouldSkipPR` comment says "ShouldSkip returns..."
- `watcher/github/pkg/publisher.go` (~line 26): `NewCommandPublisher` comment says "New returns..."
- `watcher/github/pkg/taskid.go` (~line 17): `DeriveTaskID` comment says "Derive returns..."
</context>

<requirements>
1. **`watcher/github/pkg/cursor.go`** — fix three comments:
   - Line ~20: Change `// State holds the watcher's persisted cursor.` → `// Cursor holds the watcher's persisted poll state, including the last-seen update time and a map of task-identifier to head SHA for force-push detection.`
   - Line ~26: Change `// Load reads cursor state from path.` → `// LoadCursor reads cursor state from path.` (keep the second line unchanged)
   - Line ~49: Change `// Save persists cursor state to path atomically via a temp file + rename.` → `// SaveCursor persists cursor state to path atomically via a temp file + rename.`

2. **`watcher/github/pkg/filter.go`** — fix one comment:
   - Line ~17: Change `// ShouldSkip returns true if the PR should be filtered out (draft or bot-authored).` → `// ShouldSkipPR returns true if the PR should be filtered out because it is a draft or bot-authored.`

3. **`watcher/github/pkg/publisher.go`** — fix one comment:
   - Line ~26: Change `// New returns a CommandPublisher backed by the given CommandObjectSender.` → `// NewCommandPublisher returns a CommandPublisher backed by the given CommandObjectSender.`

4. **`watcher/github/pkg/taskid.go`** — fix one comment:
   - Line ~17: Change `// Derive returns a deterministic task identifier for a PR.` → `// DeriveTaskID returns a deterministic task identifier for a PR.` (keep the second line unchanged)

5. **Create `watcher/github/pkg/doc.go`**:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package pkg provides the core domain types and logic for the github-pr-watcher service:
   // GitHub API integration, pull-request filtering, cursor persistence, and
   // Kafka command publishing.
   package pkg
   ```

6. Run `cd watcher/github && make precommit` — must exit 0 (addlicense will verify the new file has a proper header).
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Do NOT change any logic — only comment text
- Existing tests must still pass
- The `doc.go` file must have the BSD copyright header (same format as all other files in the package)
</constraints>

<verification>
cd watcher/github && grep -nE "^// (State holds|Load reads|Save persists|ShouldSkip returns|New returns|Derive returns)" pkg/cursor.go pkg/filter.go pkg/publisher.go pkg/taskid.go
# Expected: no matches (all fixed)

cd watcher/github && head -5 pkg/doc.go
# Expected: copyright header present

cd watcher/github && make precommit
</verification>
