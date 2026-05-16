---
status: completed
summary: Added /setloglevel/{level} HTTP endpoint to both watcher/github-build/main.go and watcher/github-pr/main.go, importing github.com/bborbe/log and registering log.NewSetLoglevelHandler with a 5-minute auto-reset TTL; CHANGELOG updated.
container: maintainer-102-add-setloglevel-endpoint-to-watchers
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-08T12:55:00Z"
queued: "2026-05-08T11:43:31Z"
started: "2026-05-08T11:49:11Z"
completed: "2026-05-08T11:53:38Z"
---

<summary>
- Add `/setloglevel/{level}` HTTP endpoint to both maintainer watchers (`watcher/github-build/main.go` and `watcher/github-pr/main.go`) so glog verbosity can be raised at runtime via the public admin gateway
- Default level 2, auto-resets after 5 minutes (handler ttl)
- Two-line change per file: import + route registration
- Both files already import `"time"`, both go.mod files already have `github.com/bborbe/log v1.6.12` — no go.mod edit needed
- Self-contained — no host-mounted reference repos required
</summary>

<objective>
The maintainer watchers currently silently skip events at the default `-v=2` log level (e.g., red→red episode-locked builds, filtered repos at V(3)). Bumping verbosity today requires editing the StatefulSet `-v=` arg and rolling the pod. Adding `/setloglevel/{level}` lets the operator raise verbosity for a 5-minute window via `curl -X POST https://prod.quant.benjamin-borbe.de/admin/maintainer-watcher-github-build/setloglevel/4` without restart.
</objective>

<context>
The pattern is provided by `github.com/bborbe/log`. The canonical two-line snippet to register the route is:

```go
router.Path("/setloglevel/{level}").
    Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
```

Where:
- `log` is the package `github.com/bborbe/log` (already a transitive dep at v1.6.12 in both watcher go.mod files — promote to a direct named import).
- `NewLogLevelSetter(2, 5*time.Minute)` — initial glog `-v` level is 2, raised level auto-reverts after 5 minutes.
- `ctx` is the closure-bound context already present in `runHTTPServer`.
- `time.Minute` resolves via the existing `"time"` import in both files.

Files to edit (both files have the same router-block structure):

**`watcher/github-build/main.go`** — current router block (lines 123-134):
```go
func (a *application) runHTTPServer(poll run.Func) run.Func {
    return func(ctx context.Context) error {
        router := mux.NewRouter()
        router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
        router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
        router.Path("/metrics").Handler(promhttp.Handler())
        router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
        glog.V(2).Infof("http server listening on %s", a.Listen)
        return libhttp.NewServer(a.Listen, router).Run(ctx)
    }
}
```

**`watcher/github-pr/main.go`** — current router block (lines 207-217):
```go
func (a *application) runHTTPServer(poll run.Func) run.Func {
    return func(ctx context.Context) error {
        router := mux.NewRouter()
        router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
        router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
        router.Path("/metrics").Handler(promhttp.Handler())
        router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
        glog.V(2).Infof("http server listening on %s", a.Listen)
        return libhttp.NewServer(a.Listen, router).Run(ctx)
    }
}
```

Existing imports in both files include `"time"` and the go.mod requires `github.com/bborbe/log v1.6.12` (transitive — must add a direct named import).

The watcher Service yamls already carry `admin/port: '9090'` annotations (deployed earlier today), so the gateway will route `https://<stage>.quant.benjamin-borbe.de/admin/maintainer-watcher-github-build/setloglevel/4` to the pod without further config changes.
</context>

<requirements>
**Make the following two-line addition in each main.go file. Run `make precommit` only at the final step.**

1. **Edit `watcher/github-build/main.go`**:

   a. Add `"github.com/bborbe/log"` to the named imports block (alphabetical with the other `github.com/bborbe/*` imports — e.g., right before `libsentry "github.com/bborbe/sentry"`):

   ```go
   "github.com/bborbe/errors"
   libhttp "github.com/bborbe/http"
   libkafka "github.com/bborbe/kafka"
   "github.com/bborbe/log"
   "github.com/bborbe/run"
   libsentry "github.com/bborbe/sentry"
   ```

   b. Inside `runHTTPServer`, register the route between `/metrics` and `/trigger`:

   ```go
   router.Path("/metrics").Handler(promhttp.Handler())
   router.Path("/setloglevel/{level}").
       Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
   router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
   ```

2. **Edit `watcher/github-pr/main.go`** — same change, identical placement (alphabetical import; route between `/metrics` and `/trigger`).

3. **Run `make precommit`** in the repo root:
   ```bash
   make precommit
   ```

4. **Add CHANGELOG entry** under the existing `## Unreleased` section in `CHANGELOG.md`:
   ```
   - feat(watcher): add `/setloglevel/{level}` endpoint to `maintainer-watcher-github-build` and `maintainer-watcher-github-pr` for runtime glog verbosity control (auto-resets after 5min)
   ```
</requirements>

<constraints>
- Only edit `watcher/github-build/main.go`, `watcher/github-pr/main.go`, and `CHANGELOG.md`
- Do NOT edit go.mod — `github.com/bborbe/log` is already a transitive dep at the same version as in agent repo (the `go mod tidy` step in `make precommit` will promote it to direct if needed)
- Do NOT change any other route, port, or handler
- Do NOT add other endpoints (e.g., `/resetdb`, `/resetbucket`, `/gc`, `/testloglevel`, `/sentryalert`) — only `/setloglevel/{level}`
- Do NOT alter the existing log level default (the `2` in `NewLogLevelSetter(2, ...)` matches the StatefulSet `-v=2` default)
- Do NOT alter the TTL (`5*time.Minute` is the standard auto-reset window)
- Do NOT commit — dark-factory handles git
- Use exactly the variable names already in scope: `ctx` (provided by the closure), `router`, `time` (already imported)
</constraints>

<verification>
make precommit

# Endpoint registered in both watchers (matches the symbol, not just the string)
grep -n "log.NewSetLoglevelHandler" watcher/github-build/main.go watcher/github-pr/main.go
# Expected: 2 matches (one per file)

# log import added to both
grep -n '"github.com/bborbe/log"' watcher/github-build/main.go watcher/github-pr/main.go
# Expected: 2 matches

# Import was promoted from indirect to direct in go.mod (no `// indirect` comment)
grep -n "github.com/bborbe/log v" watcher/github-build/go.mod watcher/github-pr/go.mod | grep -v "// indirect"
# Expected: 2 matches (the direct require lines, no // indirect suffix)

# Existing routes still present
grep -cE "/healthz|/readiness|/metrics|/trigger" watcher/github-build/main.go
# Expected: 4
grep -cE "/healthz|/readiness|/metrics|/trigger" watcher/github-pr/main.go
# Expected: 4

# Build passes
go build ./...

# CHANGELOG updated
grep "setloglevel" CHANGELOG.md
# Expected: 1 match in the Unreleased section
</verification>
