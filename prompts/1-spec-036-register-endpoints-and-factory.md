---
status: draft
spec: [036-watcher-pr-rename-trigger-add-single-pr-trigger]
created: "2026-05-23T19:45:00Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

<summary>
- `POST /check` replaces the existing `/trigger` poll route — same handler, new path
- `POST /trigger?url=<pr_url>` is wired as a new route requiring the `url` query parameter
- The old naked `/trigger` (poll) route no longer exists — hard cutover
- Factory updated to also export the `createSender` so `runHTTPServer` can pass it to the new handler
</summary>

<objective>
Wire two HTTP routes in `watcher/github-pr/main.go`: `POST /check` for the multi-repo poll cycle (moved from `/trigger`) and `POST /trigger` for the new single-PR URL trigger (requires `url` query param). Also export the `createSender` from the factory so it can be passed to the new handler.
</objective>

<context>
Read CLAUDE.md for project conventions.

**Read fully before implementing:**
- `watcher/github-pr/main.go` — `runHTTPServer` method (lines 243-254)
- `watcher/github-pr/pkg/factory/factory.go` — `CreateWatcher` returns only `(pkg.Watcher, func(), error)`; the `createSender` is internal
- `watcher/github-pr/pkg/watcher.go` — `Watcher` interface (`Poll` method); the watcher struct holds the `createSender` internally

**Key discovery:** The existing `CreateWatcher` in `factory.go` creates both the `createSender task.CreateCommandSender` and the `ghClient GitHubClient` internally and returns only the `Watcher` interface. For the single-trigger handler to receive the `createSender`, we need to either:
1. Extend `CreateWatcher` to also return the sender
2. Create a separate factory for the handler

Option 1 is the smaller diff: modify `CreateWatcher` to return `(pkg.Watcher, task.CreateCommandSender, func(), error)`. The watcher struct already holds the sender internally, so we can also expose it via a method on the watcher or via a second return value.

**Decision:** Modify `CreateWatcher` to return the `createSender` as a second return value. This requires updating:
- `factory.go`: `CreateWatcher` signature + body
- `main.go`: receive the sender from `CreateWatcher` + pass it to the handler
</context>

<requirements>

1. **Update `watcher/github-pr/pkg/factory/factory.go`**

   a. Change `CreateWatcher` return signature from `(pkg.Watcher, func(), error)` to `(pkg.Watcher, task.CreateCommandSender, func(), error)`:

   ```go
   func CreateWatcher(
       ctx context.Context,
       ghToken string,
       brokers libkafka.Brokers,
       stage string,
       repoScope string,
       taskCreationFilter filter.TaskCreationFilter,
       startTime libtime.DateTime,
       trustedAuthors []string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
   ) (pkg.Watcher, task.CreateCommandSender, func(), error)
   ```

   b. Update the return statement to include `createSender`:

   ```go
   return w, createSender, cleanup, nil
   ```

   c. Update the call site in `CreateKafkaSender` to return the sender so `CreateWatcher` can forward it.

2. **Update `watcher/github-pr/main.go`**

   a. In `Run` (line ~189), receive the `createSender` from `CreateWatcher`:

   ```go
   w, createSender, cleanup, err := factory.CreateWatcher(
       ctx,
       a.GHToken,
       a.KafkaBrokers,
       a.Stage,
       a.RepoScope,
       taskCreationFilter,
       startTime,
       trustedAuthors,
       a.MaxSlugLen,
       a.MaxTitleLen,
       a.TaskSuffix,
   )
   ```

   b. In `runHTTPServer`, add a second parameter for the `createSender`:

   ```go
   func (a *application) runHTTPServer(poll run.Func, createSender task.CreateCommandSender) run.Func {
       return func(ctx context.Context) error {
           router := mux.NewRouter()
           router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
           router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
           router.Path("/metrics").Handler(promhttp.Handler())
           router.Path("/setloglevel/{level}").
               Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
           router.Path("/check").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
           router.Path("/trigger").Handler(handler.NewSingleTriggerHandler(
               w.(*pkg.watcher).ghClient,
               createSender,
               taskCreationFilter,
               a.Stage,
               a.MaxSlugLen,
               a.MaxTitleLen,
               a.TaskSuffix,
           ))
           glog.V(2).Infof("http server listening on %s", a.Listen)
           return libhttp.NewServer(a.Listen, router).Run(ctx)
       }
   }
   ```

   Note: `w.(*pkg.watcher)` is a type assertion to access the internal fields. If the watcher struct fields are not accessible this way, use an alternative:
   - Option A: Add a `GHClient() GitHubClient` method to the `Watcher` interface
   - Option B: Construct the handler in `Run` before calling `runHTTPServer` and pass it as a pre-built handler

   The simplest approach: add `GHClient() GitHubClient` to the `Watcher` interface and implement it on the watcher struct. This keeps the interface minimal.

   c. Update the call to `runHTTPServer` in `Run`:

   ```go
   return run.CancelOnFirstFinish(ctx,
       a.runPollLoop(pollOnce, pollInterval),
       a.runHTTPServer(pollOnce, createSender),
   )
   ```

3. **Update `watcher/github-pr/pkg/watcher.go`**

   a. Add `GHClient() GitHubClient` to the `Watcher` interface:

   ```go
   type Watcher interface {
       GHClient() GitHubClient
       Poll(ctx context.Context) error
   }
   ```

   b. Implement it on the watcher struct:

   ```go
   func (w *watcher) GHClient() GitHubClient {
       return w.ghClient
   }
   ```

4. **Create `watcher/github-pr/pkg/handler/` directory and stub file**

   Create the directory:
   ```bash
   mkdir -p watcher/github-pr/pkg/handler
   ```

   Create `watcher/github-pr/pkg/handler/doc.go`:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package handler contains the HTTP request handlers for the watcher.
   package handler
   ```

5. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```
   If tests fail due to interface/return signature changes, fix them.

6. **Verify routes:**
   ```bash
   grep -n 'Path("/check")\|Path("/trigger")' watcher/github-pr/main.go
   ```
   Expected: two matches.

</requirements>

<constraints>
- Only edit files under `watcher/github-pr/` for this prompt
- Do NOT commit — dark-factory handles git
- The poll handler behavior is unchanged — only the route path moves from `/trigger` to `/check`
- Error wrapping: `github.com/bborbe/errors` — never `fmt.Errorf`
- `CreateWatcher` returns `(pkg.Watcher, task.CreateCommandSender, func(), error)`
- `Watcher` interface gains a `GHClient() GitHubClient` method
- Route paths: `/check` (poll), `/trigger` (single-PR, requires `url` query param)
- The `runHTTPServer` parameter list grows to accept `createSender`
</constraints>

<verification>
grep -n 'Path("/check")\|Path("/trigger")' watcher/github-pr/main.go
# Expected: two lines — one /check, one /trigger

grep -n "GHClient\(\)" watcher/github-pr/pkg/watcher.go
# Expected: interface declaration + method implementation

cd watcher/github-pr && make test
# Expected: exit 0
</verification>