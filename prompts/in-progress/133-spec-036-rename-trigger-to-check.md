---
status: executing
spec: [036-watcher-pr-rename-trigger-add-single-pr-trigger]
container: maintainer-exec-133-spec-036-rename-trigger-to-check
dark-factory-version: v0.169.0
created: "2026-05-23T21:01:00Z"
queued: "2026-05-23T21:13:13Z"
started: "2026-05-23T21:13:15Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

## Summary

- `POST /trigger` (poll) route renamed to `POST /check` in `watcher/github-pr/main.go`
- No behavior change — same handler, same poll function
- Hard cutover: no backwards-compatible alias or 308 redirect

## Objective

Rename the `/trigger` route to `/check` in the watcher HTTP server. This is the poll-equivalent endpoint. The rename is purely cosmetic — the underlying handler and behavior are identical.

## Context

Read these files before making changes:

**Route registration:**
- `/workspace/watcher/github-pr/main.go` — lines 243-254 show the HTTP server setup. Line 251 registers `router.Path("/trigger")` with `libhttp.NewBackgroundRunHandler(ctx, poll)`.

**Key snippet from main.go (verified):**
```go
func (a *application) runHTTPServer(poll run.Func) run.Func {
    return func(ctx context.Context) error {
        router := mux.NewRouter()
        router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
        router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
        router.Path("/metrics").Handler(promhttp.Handler())
        router.Path("/setloglevel/{level}").
            Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
        router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))  // <-- CHANGE THIS LINE
        glog.V(2).Infof("http server listening on %s", a.Listen)
        return libhttp.NewServer(a.Listen, router).Run(ctx)
    }
}
```

**Pattern reference:**
- `/workspace/watcher/github-build/pkg/reset_handler.go` — handler pattern using `libhttp.WrapWithStatusCode` and `libhttp.NewErrorHandler`
- `/workspace/watcher/github-pr/pkg/filter/filter.go` — existing package with BSD license header

## Requirements

1. In `/workspace/watcher/github-pr/main.go`, change line 251 from:
   ```go
   router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
   ```
   to:
   ```go
   router.Path("/check").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
   ```
   This is the only change needed in this file. The handler object and the `poll` function are identical — only the route path changes.

2. **No backwards compatibility**: Do NOT add a redirect from `/trigger` to `/check`, do NOT add an alias route, do NOT keep any `/trigger` mapping. The hard cutover is intentional per the spec.

3. Do NOT modify the cron schedule, the poll loop, or any other handler registration in the same file.

## Constraints

- BSD license header already present in main.go — do not remove or modify it
- Do NOT add any new handlers in this step — only the route rename
- Keep `libhttp.NewBackgroundRunHandler` — it is the correct handler for fire-and-forget background work
- Do not change the `poll` variable or how it is captured

## Verification

Run the following commands and confirm zero exit codes:

```bash
# Verify /check route exists (grep for the new path)
grep -n 'Path("/check")' /workspace/watcher/github-pr/main.go

# Verify /trigger route no longer exists (should have no matches)
grep -n 'Path("/trigger")' /workspace/watcher/github-pr/main.go || echo "No /trigger route found (expected)"

# Ensure the handler wiring is unchanged (same BackgroundRunHandler)
grep -n 'BackgroundRunHandler' /workspace/watcher/github-pr/main.go

# Run precommit in the watcher
cd /workspace/watcher/github-pr && make precommit
```