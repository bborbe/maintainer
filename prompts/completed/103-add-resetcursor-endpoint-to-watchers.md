---
status: completed
summary: Added /resetcursor/{repo:.+} admin endpoint to watcher/github-build protected by libhttp.NewDangerousHandlerWrapper, with Ginkgo contract tests covering slash-in-repo-key routing, successful reset, and 404 for unknown repo.
container: maintainer-103-add-resetcursor-endpoint-to-watchers
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-08T14:30:00Z"
queued: "2026-05-08T12:41:37Z"
started: "2026-05-08T12:41:38Z"
completed: "2026-05-08T12:47:27Z"
---

<summary>
- Add `/resetcursor/{repo}` admin endpoint to **maintainer-watcher-github-build only** so an operator can release the episode lock for one repo without `kubectl exec`-ing to edit `/data/cursor.json`
- Protect with `libhttp.NewDangerousHandlerWrapper` (passphrase-via-logs gate)
- Implementation: HTTP handler reads cursor.json, deletes the entry for `{repo}`, atomic-rewrites via existing `pkg.LoadCursor`/`pkg.SaveCursor`
- The next poll re-creates the entry from cold-start state (`prevState == ""`, currState == "red") and publishes a fresh create-task command
- Includes a router contract test that asserts the `{repo:.+}` regex accepts repo keys containing `/`
- **Not** adding to `watcher/github-pr` — pr-watcher's `Cursor` has no `Repos` map / no episode lock; it tracks `HeadSHAs map[string]string` for force-push detection. Different problem, separate prompt later if needed.
</summary>

<objective>
Today the only way to release a stuck red→red episode lock is to `kubectl exec` into the watcher pod and hand-edit `/data/cursor.json`. That requires shell access and is brittle. This endpoint replaces that procedure with a curl through the public admin gateway, gated by a passphrase the operator must read from pod logs.
</objective>

<context>
The build-watcher persists per-repo state in `/data/cursor.json` (NOT a kv DB — plain file). State machine in `watcher/github-build/pkg/watcher.go` (find via the `case prevState == "red" && currState == "red":` switch arm):

```go
case prevState == "red" && currState == "red":
    // Episode locked on first red; skip regardless of SHA change
```

→ once red, no new task is published until `currState == "green"` resets the lock. If the original create-task message was rejected by the downstream consumer, the lock holds forever.

`watcher/github-build/pkg/cursor.go` exports:

```go
type Cursor struct {
    Repos map[string]*RepoState `json:"repos"`
}
type RepoState struct {
    LastKnownState    string `json:"last_known_state"`    // "green" | "red" | ""
    CurrentEpisodeSHA string `json:"current_episode_sha"` // empty when green
    DefaultBranch     string `json:"default_branch"`
}

const DefaultCursorPath = "/data/cursor.json"

func LoadCursor(ctx context.Context, path string) (*Cursor, error)
func SaveCursor(ctx context.Context, path string, c *Cursor) error
```

`SaveCursor` is atomic (temp-file + rename, mode `0600`).

`libhttp.NewDangerousHandlerWrapper(handler http.Handler) http.Handler` (from `github.com/bborbe/http`):

- Generates a fresh 16-char base64url passphrase per instance, valid 5 min.
- Logs `⚠️ DANGER PASSPHRASE: <url>?passphrase=<value> (expires: ...)` on every unauthenticated hit.
- Requires `?passphrase=<value>` query — wrong/expired returns 403.
- Two-factor: HTTP access + log access both required.

`libhttp.NewErrorHandler` + `libhttp.WithErrorFunc` (canonical bborbe error-returning handler pattern, see `coding-guidelines/docs/go-json-error-handler-guide.md`):

```go
libhttp.NewErrorHandler(libhttp.WithErrorFunc(func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
    // return libhttp.WrapWithStatusCode(err, http.StatusBadRequest) for client errors
    // return wrapped error for 500s — NewErrorHandler converts to JSON response
}))
```

Concurrency: cursor file is racy with an in-flight `Poll` (Poll loads, mutates in-memory, saves at end). A reset between polls is safe; a reset during a poll loses the race to Poll's final save. Acceptable for a manual-ops endpoint — operator just retries. Do NOT add a watcher-level mutex.

The watcher service yaml carries `admin/port: '9090'` (deployed earlier today), so `https://<stage>.quant.benjamin-borbe.de/admin/maintainer-watcher-github-build/resetcursor/<owner%2Frepo>?passphrase=...` will route once this prompt deploys.

`watcher/github-build/main.go runHTTPServer` currently registers, in this exact order:
1. `/healthz`
2. `/readiness`
3. `/metrics`
4. `/setloglevel/{level}`
5. `/trigger`

The new route goes between `/setloglevel/{level}` and `/trigger`.

Repo layout note: `maintainer/` has multiple Go modules (`watcher/github-build/go.mod`, `watcher/github-pr/go.mod`, `agent/pr-reviewer/go.mod`). `make precommit` traverses each — that is the canonical full-tree validation. Plain `go build ./...` from repo root does NOT descend into subdir modules. All `go` invocations in this prompt are scoped to `watcher/github-build/`.
</context>

<requirements>
**Make the changes below in order. Run `make precommit` only at the final step.**

1. **Create `watcher/github-build/pkg/reset_handler.go`** with this content (use the standard file-header copyright comment from sibling pkg files):

   ```go
   package pkg

   import (
       "context"
       "net/http"

       "github.com/bborbe/errors"
       libhttp "github.com/bborbe/http"
       "github.com/golang/glog"
       "github.com/gorilla/mux"
   )

   // NewResetCursorHandler returns an HTTP handler that clears the cursor entry
   // for one repo. Repo is taken from the {repo} URL variable (e.g.
   // "github.com/bborbe/maintainer"). Releases an episode lock so the next poll
   // detects the build state from scratch and may publish a fresh create-task.
   //
   // Wrap with libhttp.NewDangerousHandlerWrapper at the call site to require a
   // passphrase — the bare handler does not enforce auth.
   //
   // Race: a concurrent Poll may overwrite the reset; operator should retry if
   // the next poll log doesn't show a state transition for the target repo.
   func NewResetCursorHandler(cursorPath string) http.Handler {
       return libhttp.NewErrorHandler(libhttp.WithErrorFunc(func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
           repoKey := mux.Vars(req)["repo"]
           if repoKey == "" {
               return libhttp.WrapWithStatusCode(errors.Errorf(ctx, "missing {repo} path variable"), http.StatusBadRequest)
           }

           cursor, err := LoadCursor(ctx, cursorPath)
           if err != nil {
               return errors.Wrapf(ctx, err, "load cursor for reset")
           }
           if _, ok := cursor.Repos[repoKey]; !ok {
               return libhttp.WrapWithStatusCode(errors.Errorf(ctx, "repo not found in cursor: %s", repoKey), http.StatusNotFound)
           }
           delete(cursor.Repos, repoKey)
           if err := SaveCursor(ctx, cursorPath, cursor); err != nil {
               return errors.Wrapf(ctx, err, "save cursor after reset")
           }
           glog.Warningf("cursor reset for repo=%s", repoKey)
           libhttp.WriteAndGlog(resp, "cursor reset for "+repoKey)
           return nil
       }))
   }
   ```

2. **Create `watcher/github-build/pkg/reset_handler_test.go`** — Ginkgo/Gomega contract test covering the three boundary concerns: gorilla/mux variable extraction with the `{repo:.+}` regex, missing-cursor-entry returns 404, successful reset removes the entry from disk and writes back via `SaveCursor`. Use a real `mux.Router` (not a faked handler) so the regex is actually exercised. Use `t.TempDir()` + write a starter cursor.json to disk → fire `httptest.NewRequest("POST", "/resetcursor/github.com/bborbe/foo", nil)` → assert: response 200, response body contains the repo key, file no longer contains the entry. Add a second test case for an unknown repo asserting 404. (Follow the existing test style in `watcher/github-build/pkg/cursor_test.go` and `watcher/github-build/pkg/watcher_test.go`.)

3. **Edit `watcher/github-build/main.go`** — register the route in `runHTTPServer` between `/setloglevel/{level}` and `/trigger`:

   ```go
   router.Path("/setloglevel/{level}").
       Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
   router.Path("/resetcursor/{repo:.+}").
       Handler(libhttp.NewDangerousHandlerWrapper(pkg.NewResetCursorHandler(pkg.DefaultCursorPath)))
   router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
   ```

   Notes:
   - `{repo:.+}` regex — repo keys contain `/` (e.g. `github.com/bborbe/maintainer`). gorilla/mux variables don't match `/` by default; the `.+` regex allows it.
   - Use `pkg.DefaultCursorPath` (already exported) — do not hardcode `"/data/cursor.json"` in main.go.
   - `pkg "github.com/bborbe/maintainer/watcher/github-build/pkg"` is already in the import block — no new import needed.

4. **Add CHANGELOG entry** under the existing `## Unreleased` section in `CHANGELOG.md`:

   ```
   - feat(watcher/github-build): add `/resetcursor/{repo}` admin endpoint to release stuck episode locks. Protected by `libhttp.NewDangerousHandlerWrapper` (passphrase rotated every 5min, logged on each unauthenticated hit). Use as: `curl 'https://<stage>.quant.benjamin-borbe.de/admin/maintainer-watcher-github-build/resetcursor/github.com/bborbe/<repo>?passphrase=<from-logs>'`
   ```

5. **Build + test the changed module**:

   ```bash
   cd watcher/github-build && go build ./... && go test ./...
   ```

6. **Run `make precommit`** in repo root:

   ```bash
   make precommit
   ```
</requirements>

<constraints>
- Only edit `watcher/github-build/main.go` and `CHANGELOG.md`, and create `watcher/github-build/pkg/reset_handler.go` + `watcher/github-build/pkg/reset_handler_test.go`
- Do NOT modify `pkg/cursor.go`, `pkg/watcher.go`, the factory, or any cursor read/write logic — the new handler reuses the existing `LoadCursor`/`SaveCursor` API as-is
- Do NOT touch `watcher/github-pr/` — out of scope (different cursor model, no episode lock; separate prompt later if needed)
- Do NOT add a `sync.Mutex` to the watcher struct — the documented retry-on-race is intentional
- Do NOT add a `/resetcursor` (no-arg, whole-cursor) endpoint
- Do NOT change route order in main.go — `/healthz`, `/readiness`, `/metrics`, `/setloglevel/{level}`, `/resetcursor/{repo:.+}`, `/trigger` (in this exact order)
- Use exactly `pkg.NewResetCursorHandler` as the exported symbol name
- The mux variable name MUST be `{repo:.+}` — without `:.+` gorilla/mux rejects the slash in `github.com/bborbe/X`
- Wrap `pkg.NewResetCursorHandler(...)` in `libhttp.NewDangerousHandlerWrapper(...)` at the call site — naked registration is a security regression
- Use `pkg.DefaultCursorPath` (already exported in `pkg/cursor.go`), do NOT plumb the path through args or hardcode in main.go
- Inside the handler, return wrapped errors via `libhttp.WrapWithStatusCode(err, statusCode)` for client errors and plain wrapped errors for 5xx — do NOT use `http.Error()` directly. This matches `coding-guidelines/docs/go-json-error-handler-guide.md` and the rest of the codebase.
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
make precommit

# New handler file exists
ls watcher/github-build/pkg/reset_handler.go
# Expected: path prints, no error

# Test file exists
ls watcher/github-build/pkg/reset_handler_test.go
# Expected: path prints, no error

# Handler symbol callable from main.go (exported, right signature)
grep -n "pkg.NewResetCursorHandler" watcher/github-build/main.go
# Expected: 1 match

# Wrapped with DangerousHandlerWrapper (security gate)
grep -n "libhttp.NewDangerousHandlerWrapper(pkg.NewResetCursorHandler" watcher/github-build/main.go
# Expected: 1 match

# Mux pattern correct (must allow slash in repo key)
grep -n "/resetcursor/{repo:.+}" watcher/github-build/main.go
# Expected: 1 match

# Cursor path uses pkg.DefaultCursorPath, NOT hardcoded
grep -n "pkg.NewResetCursorHandler(pkg.DefaultCursorPath)" watcher/github-build/main.go
# Expected: 1 match
grep -n '"/data/cursor.json"' watcher/github-build/main.go
# Expected: zero matches (no hardcoded path)

# Existing endpoints still present, in expected order
grep -nE "/healthz|/readiness|/metrics|/setloglevel|/resetcursor|/trigger" watcher/github-build/main.go
# Expected: 6 lines, in that order

# Build + test the changed module specifically
cd watcher/github-build && go build ./... && go test ./...
# Expected: exit 0, all tests pass

# CHANGELOG updated
grep "resetcursor" CHANGELOG.md
# Expected: 1 match in the Unreleased section

# Handler does NOT use http.Error (must use libhttp.WrapWithStatusCode)
grep -n "http.Error" watcher/github-build/pkg/reset_handler.go
# Expected: zero matches
</verification>
