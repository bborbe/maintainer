---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `SinglePRTriggerHandler` uses raw `http.Handler`/`ServeHTTP` instead of `libhttp.WithError` — no panic recovery, inconsistent JSON error responses, manual `writeError` scattered throughout
- The handler mixes seven distinct concerns (URL parsing, GitHub API fetch, filter evaluation, trust evaluation, command building, Kafka publish, HTTP response writing) in one 65-line method
- The factory `CreateSinglePRHandler` does not follow the factory naming convention — it should be `CreateSinglePRTriggerHandler`
</summary>

<objective>
Refactor `SinglePRTriggerHandler` to use `libhttp.WithError` pattern: return errors naturally instead of calling `writeError`, let the wrapper handle panic recovery and JSON error marshalling. Rename factory to match `Create*Handler` convention.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-http-handler-refactoring-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — handler in pkg/handler/, factory in pkg/factory/, no inline handlers.

Files to read before making changes:
- `watcher/github-pr/pkg/handler/trigger_handler.go` — full file; understand ServeHTTP, writeError, writeSuccess
- `watcher/github-pr/pkg/factory/single_pr.go` — full file; understand CreateSinglePRHandler
- `watcher/github-pr/main.go` — lines 228-250; understand call site

Grep-verify `libhttp.WithError` exists before writing any code:
```bash
grep -rn "WithError\|WithErrorFunc" $(go env GOPATH)/pkg/mod/github.com/bborbe/http@*/delivery/ 2>/dev/null | head -20
```
</context>

<requirements>

**Execute steps in order. Run `make test` after step 3. Run `make precommit` only at the final step.**

1. **Refactor `SinglePRTriggerHandler` interface in `trigger_handler.go`:**

   Change from raw `http.Handler` to `libhttp.WithError`:
   ```go
   type SinglePRTriggerHandler interface {
       // Handle processes a PR URL and fires a review task.
       // Returns an error that is automatically JSON-marshalled as {"error":"..."} by the wrapper.
       Handle(ctx context.Context, resp http.ResponseWriter, req *http.Request) error
   }
   ```

   Update the struct to not embed `http.Handler`:
   ```go
   type singlePRTriggerHandler struct {
       ghClient               GitHubClient
       createSender           task.CreateCommandSender
       taskCreationFilter     filter.TaskCreationFilter
       trustDecision          trust.Trust
       stage                  string
       maxSlugLen             int
       maxTitleLen            int
       taskSuffix             string
   }
   ```

2. **Refactor `ServeHTTP` into `Handle` in `trigger_handler.go`:**

   Convert the method to return `error` instead of calling `writeError`/`writeSuccess`:
   ```go
   func (h *singlePRTriggerHandler) Handle(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
       urlStr := req.URL.Query().Get("url")
       if urlStr == "" {
           return errors.Errorf(ctx, "url query parameter is required")
       }

       prURL, err := prurl.ParsePRURL(urlStr)
       if err != nil {
           return errors.Wrap(ctx, err, "parse PR URL")
       }
       if prURL.Platform != "github" {
           return errors.Errorf(ctx, "only github platform is supported, got %s", prURL.Platform)
       }

       prDetails, err := h.ghClient.GetPRDetails(ctx, prURL.Owner, prURL.Repo, prURL.Number)
       if err != nil {
           return errors.Wrap(ctx, err, "get PR details")
       }

       if h.taskCreationFilter.Skip(filter.PR{
           Title:     prDetails.Title,
           AuthorLogin: prDetails.AuthorLogin,
           UpdatedAt: prDetails.UpdatedAt,
       }) {
           return errors.Errorf(ctx, "PR skipped by filter")
       }

       trustResult, err := h.trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: prDetails.AuthorLogin})
       if err != nil {
           return errors.Wrap(ctx, err, "check trust")
       }

       taskID := taskid.DeriveTaskID(prDetails)
       taskIDStr := taskid.FormatTaskID(taskID)

       cmd := BuildCreateCommand(prDetails, trustResult, h.stage, h.maxSlugLen, h.maxTitleLen, h.taskSuffix, taskIDStr)
       if err := h.createSender.SendCommand(ctx, cmd); err != nil {
           return errors.Wrap(ctx, err, "send create task command")
       }

       resp.Header().Set("Content-Type", "application/json")
       resp.WriteHeader(http.StatusOK)
       json.NewEncoder(resp).Encode(map[string]string{
           "status":  "ok",
           "task_id": taskIDStr,
       })
       return nil
   }
   ```

   Remove the `writeError` and `writeSuccess` helper methods entirely.

   The `determineRejectingFilter` helper can be removed — it is no longer needed since we return errors directly.

3. **Rename `CreateSinglePRHandler` to `CreateSinglePRTriggerHandler` in `factory/single_pr.go`:**

   ```go
   // CreateSinglePRTriggerHandler wires a handler that fires a single-PR review by URL.
   func CreateSinglePRTriggerHandler(
       ...
   ) handler.SinglePRTriggerHandler {
   ```

4. **Update call site in `main.go` line ~228:**
   Change `factory.CreateSinglePRHandler` → `factory.CreateSinglePRTriggerHandler`.

5. **Update tests in `trigger_handler_test.go`:**

   Change mock calls from `ServeHTTP` to `Handle`:
   - Replace `handler.ServeHTTP(resp, req)` with `handler.Handle(ctx, resp, req)`
   - Update assertions to use the new response format

6. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```
   Fix any compilation errors.

7. **Run `make precommit`:**
   ```bash
   cd watcher/github-pr && make precommit
   ```
</requirements>

<constraints>
- Only change `watcher/github-pr/pkg/handler/trigger_handler.go`, `watcher/github-pr/pkg/factory/single_pr.go`, `watcher/github-pr/main.go`, and `watcher/github-pr/pkg/handler/trigger_handler_test.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Wrapf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- The new `Handle` method must return `error` — caller (libhttp wrapper) handles JSON encoding and panic recovery
- Do NOT use `writeError` or `writeSuccess` — remove them
- Coverage ≥80% for changed packages
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm Handle method returns error:
grep -n "func.*Handle.*error" watcher/github-pr/pkg/handler/trigger_handler.go

# Confirm no writeError/writeSuccess:
grep -n "writeError\|writeSuccess" watcher/github-pr/pkg/handler/trigger_handler.go

# Confirm factory renamed:
grep -n "CreateSinglePRTriggerHandler" watcher/github-pr/pkg/factory/single_pr.go watcher/github-pr/main.go
</verification>
