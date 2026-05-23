---
status: draft
spec: [036-watcher-pr-rename-trigger-add-single-pr-trigger]
created: "2026-05-23T21:02:00Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

## Summary

- `PRDetails` struct in `watcher/github-pr/pkg/githubclient.go` extended to include `AuthorLogin`, `Title`, `IsDraft`
- `GetPRDetails` implementation updated to populate the new fields
- `BuildCreateCommand` exported function added to `watcher/github-pr/pkg/watcher.go`
- New handler `trigger_handler.go` created in `watcher/github-pr/pkg/handler/`
- New factory `single_pr.go` created in `watcher/github-pr/pkg/factory/`
- Handler wired to `POST /trigger?url=<pr_url>` in `main.go`
- `GitHubClient` passed explicitly to both `CreateWatcher` and `CreateSinglePRHandler`
- Table-driven unit tests covering all error paths and both trust branches

## Objective

Create the single-PR retrigger handler that allows operators to fire a review for a specific PR by URL. The handler reuses the filter chain, trust evaluation, and task-building logic from the existing poll path.

## Context

Read these files before making changes:

**Source files (verified signatures):**
- `/workspace/watcher/github-pr/pkg/githubclient.go` — `GitHubClient` interface and `PRDetails` struct (lines 41-77)
- `/workspace/watcher/github-pr/pkg/watcher.go` — `publishCreate`, `buildFrontmatter`, `buildHumanReviewFrontmatter`, `buildTaskBody`, `buildUntrustedBody`, `computePRTitle`, `DeriveTaskID` (lines 234-372)
- `/workspace/watcher/github-pr/pkg/taskid.go` — `DeriveTaskID(owner, repo string, number int, sha string) uuid.UUID`
- `/workspace/watcher/github-pr/pkg/filter/filter.go` — `TaskCreationFilter` interface with `Skip(pr PR) bool`
- `/workspace/watcher/github-pr/pkg/factory/factory.go` — existing factory pattern
- `/workspace/watcher/github-pr/main.go` — `Run` function, `application` struct, route registration
- `/workspace/watcher/github-pr/pkg/trust/trust.go` — `Trust` interface with `IsTrusted(ctx, PR{AuthorLogin:}) (Result, error)`
- `/workspace/lib/prurl/prurl.go` — `ParsePRURL(ctx, rawURL) (*PRInfo, error)` (moved in prompt 1)

**Existing mocks in watcher/github-pr/pkg/mocks/:**
- `github_client.go` — `GitHubClient`
- `task_creation_filter.go` — `TaskCreationFilter`
- `trust.go` — `Trust`

**External mock needed from bborbe/agent:**
- `task.CreateCommandSender` — interface from `github.com/bborbe/agent/lib/command/task`. Mock is at `github.com/bborbe/agent/lib/command/task/mocks/task-create-command-sender.go` with fake name `TaskCreateCommandSender`

**Key verified signatures:**
```go
// githubclient.go
type PRDetails struct {
    HeadSHA  string
    CloneURL string
    BaseRef  string
}
func (c *githubClient) GetPRDetails(ctx context.Context, owner, repo string, number int) (PRDetails, error)

// filter.go
type TaskCreationFilter interface { Skip(pr PR) bool }

// trust.go
type Trust interface { IsTrusted(ctx context.Context, pr PR) (Result, error) }
type Result interface { Success() bool; Description() string }

// task.CreateCommandSender (external, from bborbe/agent)
type CreateCommandSender interface { SendCommand(ctx context.Context, cmd CreateCommand) error }
```

## Requirements

### Step 1: Extend PRDetails in githubclient.go

In `/workspace/watcher/github-pr/pkg/githubclient.go`, update `PRDetails` to include fields needed for filter evaluation and trust checking. Current `PRDetails` (lines 41-57):

```go
type PRDetails struct {
    HeadSHA  string
    CloneURL string
    BaseRef  string
}
```

Change to:
```go
type PRDetails struct {
    HeadSHA     string
    CloneURL    string
    BaseRef     string
    AuthorLogin string // GitHub author login; empty for deleted accounts
    Title       string // PR title
    IsDraft     bool   // draft state
}
```

Update the `GetPRDetails` return statement (around line 156) to populate the new fields. Read the current implementation first:

```go
func (c *githubClient) GetPRDetails(ctx context.Context, owner, repo string, number int) (PRDetails, error) {
    pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, number)
    if err != nil {
        return PRDetails{}, errors.Wrapf(ctx, err, "get pull request %s/%s#%d", owner, repo, number)
    }
    return PRDetails{
        HeadSHA:  pr.GetHead().GetSHA(),
        CloneURL: pr.GetHead().GetRepo().GetCloneURL(),
        BaseRef:  pr.GetBase().GetRef(),
        AuthorLogin: pr.GetUser().GetLogin(),
        Title:       pr.GetTitle(),
        IsDraft:     pr.GetDraft(),
    }, nil
}
```

### Step 2: Export BuildCreateCommand in watcher.go

In `/workspace/watcher/github-pr/pkg/watcher.go`, add this function after the existing `publishCreate` method. Read lines 234-289 first to understand the current `publishCreate` implementation.

```go
// BuildCreateCommand builds a CreateTaskCommand for a PR given its details and trust result.
// It is used by both the poll path (via publishCreate) and the single-PR trigger handler.
func BuildCreateCommand(
    pr PullRequest,
    details PRDetails,
    taskIDStr string,
    stage string,
    maxSlugLen int,
    maxTitleLen int,
    taskSuffix string,
    trustResult trust.Result,
) task.CreateCommand {
    if trustResult.Success() {
        return task.CreateCommand{
            Title:          computePRTitle("github", pr.Owner, pr.Repo, pr.Number, details.HeadSHA, pr.Title, maxSlugLen, maxTitleLen, taskSuffix),
            TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
            Frontmatter:    buildFrontmatter(pr, taskIDStr, stage, details),
            Body:           buildTaskBody(pr),
        }
    }
    author := pr.AuthorLogin
    if author == "" {
        author = "(unknown)"
    }
    return task.CreateCommand{
        Title:          computePRTitle("github", pr.Owner, pr.Repo, pr.Number, details.HeadSHA, pr.Title, maxSlugLen, maxTitleLen, taskSuffix),
        TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
        Frontmatter:    buildHumanReviewFrontmatter(pr, taskIDStr, stage, details),
        Body:           buildUntrustedBody(author, trustResult.Description()),
    }
}
```

Ensure `trust` package is imported (line 18 of watcher.go).

### Step 3: Create the trigger handler

Create `/workspace/watcher/github-pr/pkg/handler/trigger_handler.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    agentlib "github.com/bborbe/agent/lib"
    task "github.com/bborbe/agent/lib/command/task"
    "github.com/bborbe/errors"
    libtime "github.com/bborbe/time"
    "github.com/golang/glog"

    "github.com/bborbe/maintainer/lib/prurl"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// successResponse is the JSON body on HTTP 200.
type successResponse struct {
    TaskID   string `json:"task_id"`
    Repo     string `json:"repo"`
    PRNumber int    `json:"pr_number"`
    HeadSHA  string `json:"head_sha"`
}

// errorResponse is the JSON body on HTTP 4xx/5xx.
type errorResponse struct {
    Error  string `json:"error"`
    Filter string `json:"filter,omitempty"`
    PRURL  string `json:"pr_url,omitempty"`
}

// SinglePRTriggerHandler handles POST /trigger?url=<pr_url>
//counterfeiter:generate -o ../mocks/single_pr_trigger_handler.go --fake-name SinglePRTriggerHandler . SinglePRTriggerHandler

type SinglePRTriggerHandler interface {
    ServeHTTP(resp http.ResponseWriter, req *http.Request)
}

// NewSinglePRTriggerHandler returns a handler that fires a single PR review by URL.
// The filter and trustDecision are passed in (reused from the poll path) — not created here.
func NewSinglePRTriggerHandler(
    ghClient pkg.GitHubClient,
    createSender task.CreateCommandSender,
    taskCreationFilter filter.TaskCreationFilter,
    trustDecision trust.Trust,
    stage string,
    maxSlugLen int,
    maxTitleLen int,
    taskSuffix string,
) SinglePRTriggerHandler {
    return &singlePRTriggerHandler{
        ghClient:           ghClient,
        createSender:       createSender,
        taskCreationFilter: taskCreationFilter,
        trustDecision:      trustDecision,
        stage:              stage,
        maxSlugLen:         maxSlugLen,
        maxTitleLen:        maxTitleLen,
        taskSuffix:         taskSuffix,
    }
}

type singlePRTriggerHandler struct {
    ghClient           pkg.GitHubClient
    createSender       task.CreateCommandSender
    taskCreationFilter filter.TaskCreationFilter
    trustDecision      trust.Trust
    stage              string
    maxSlugLen         int
    maxTitleLen        int
    taskSuffix         string
}

func (h *singlePRTriggerHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
    ctx := req.Context()
    rawURL := req.URL.Query().Get("url")
    if rawURL == "" {
        h.writeError(resp, http.StatusBadRequest, "url query parameter required", "", "")
        return
    }

    prInfo, err := prurl.ParsePRURL(ctx, rawURL)
    if err != nil {
        h.writeError(resp, http.StatusBadRequest, err.Error(), "", rawURL)
        return
    }
    if prInfo.Platform != prurl.PlatformGitHub {
        h.writeError(resp, http.StatusBadRequest,
            fmt.Sprintf("unsupported platform: %s (only github supported)", prInfo.Platform),
            "", rawURL)
        return
    }

    // Fetch full PR details (includes author, title, draft state needed for filters)
    details, err := h.ghClient.GetPRDetails(ctx, prInfo.Owner, prInfo.Repo, prInfo.Number)
    if err != nil {
        h.writeError(resp, http.StatusBadGateway,
            fmt.Sprintf("github fetch failed: %v", err), "", rawURL)
        return
    }

    // Build filter input — use details for all fields needed by filter chain
    repoKey := "github.com/" + prInfo.Owner + "/" + prInfo.Repo
    filterPR := filter.PR{
        AuthorLogin: details.AuthorLogin,
        IsDraft:     details.IsDraft,
        Title:       details.Title,
        UpdatedAt:   libtime.DateTime{}, // zero time passes age filter (no cursor dependency)
        RepoKey:     repoKey,
    }

    if h.taskCreationFilter.Skip(filterPR) {
        filterName := h.determineRejectingFilter(filterPR)
        glog.V(2).Infof("trigger: PR filtered by %s pr=%s", filterName, rawURL)
        h.writeError(resp, http.StatusUnprocessableEntity,
            fmt.Sprintf("PR filtered by %s", filterName), filterName, rawURL)
        return
    }

    // Trust evaluation
    trustResult, err := h.trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: details.AuthorLogin})
    if err != nil {
        h.writeError(resp, http.StatusBadGateway,
            fmt.Sprintf("trust evaluation failed: %v", err), "", rawURL)
        return
    }

    // Build PR struct for command creation
    pr := pkg.PullRequest{
        Number:      prInfo.Number,
        Owner:       prInfo.Owner,
        Repo:        prInfo.Repo,
        Title:       details.Title,
        AuthorLogin: details.AuthorLogin,
        HTMLURL:     rawURL,
        IsDraft:     details.IsDraft,
    }

    taskIDStr := pkg.DeriveTaskID(prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA).String()

    cmd := pkg.BuildCreateCommand(pr, details, taskIDStr, h.stage, h.maxSlugLen, h.maxTitleLen, h.taskSuffix, trustResult)

    if err := h.createSender.SendCommand(ctx, cmd); err != nil {
        h.writeError(resp, http.StatusBadGateway,
            fmt.Sprintf("kafka publish failed: %v", err), "", rawURL)
        return
    }

    glog.V(2).Infof("trigger: published task_id=%s pr=%s/%s#%d sha=%s", taskIDStr, prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA)
    resp.Header().Set("Content-Type", "application/json")
    resp.WriteHeader(http.StatusOK)
    json.NewEncoder(resp).Encode(successResponse{
        TaskID:   taskIDStr,
        Repo:     prInfo.Owner + "/" + prInfo.Repo,
        PRNumber: prInfo.Number,
        HeadSHA:  details.HeadSHA,
    })
}

// determineRejectingFilter identifies which filter rejected the PR.
// Called when taskCreationFilter.Skip returned true.
func (h *singlePRTriggerHandler) determineRejectingFilter(pr filter.PR) string {
    if filter.NewDraftFilter().Skip(pr) {
        return "DraftFilter"
    }
    if filter.NewBotAuthorFilter(nil).Skip(pr) {
        return "BotAuthorFilter"
    }
    if filter.NewWIPTitleFilter().Skip(pr) {
        return "WIPTitleFilter"
    }
    return "TaskCreationFilter"
}

func (h *singlePRTriggerHandler) writeError(resp http.ResponseWriter, status int, errMsg, filterName, prURL string) {
    glog.Errorf("trigger error status=%d error=%s filter=%s pr_url=%s", status, errMsg, filterName, prURL)
    resp.Header().Set("Content-Type", "application/json")
    resp.WriteHeader(status)
    json.NewEncoder(resp).Encode(errorResponse{
        Error:  errMsg,
        Filter: filterName,
        PRURL:  prURL,
    })
}
```

### Step 4: Create the factory

Create `/workspace/watcher/github-pr/pkg/factory/single_pr.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
    "context"

    task "github.com/bborbe/agent/lib/command/task"
    "github.com/bborbe/errors"

    "github.com/bborbe/maintainer/watcher/github-pr/pkg"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// CreateSinglePRHandler wires a handler that fires a single-PR review by URL.
func CreateSinglePRHandler(
    ctx context.Context,
    ghClient pkg.GitHubClient,
    createSender task.CreateCommandSender,
    taskCreationFilter filter.TaskCreationFilter,
    trustDecision trust.Trust,
    stage string,
    maxSlugLen int,
    maxTitleLen int,
    taskSuffix string,
) (handler.SinglePRTriggerHandler, error) {
    if ghClient == nil {
        return nil, errors.Errorf(ctx, "ghClient is required")
    }
    if createSender == nil {
        return nil, errors.Errorf(ctx, "createSender is required")
    }
    if taskCreationFilter == nil {
        return nil, errors.Errorf(ctx, "taskCreationFilter is required")
    }
    if trustDecision == nil {
        return nil, errors.Errorf(ctx, "trustDecision is required")
    }
    return handler.NewSinglePRTriggerHandler(
        ghClient,
        createSender,
        taskCreationFilter,
        trustDecision,
        stage,
        maxSlugLen,
        maxTitleLen,
        taskSuffix,
    ), nil
}
```

### Step 5: Update CreateWatcher factory to accept ghClient

Update `/workspace/watcher/github-pr/pkg/factory/factory.go` to change `CreateWatcher` signature from `ghToken string` to `ghClient pkg.GitHubClient`. Read the current signature first (lines 49-62):

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
) (pkg.Watcher, func(), error) {
```

Change to accept `ghClient pkg.GitHubClient` instead of `ghToken string`:

```go
func CreateWatcher(
    ctx context.Context,
    ghClient pkg.GitHubClient,
    brokers libkafka.Brokers,
    stage string,
    repoScope string,
    taskCreationFilter filter.TaskCreationFilter,
    startTime libtime.DateTime,
    trustedAuthors []string,
    maxSlugLen int,
    maxTitleLen int,
    taskSuffix string,
) (pkg.Watcher, func(), error) {
```

And update line 70 (the `NewWatcher` call) to remove `pkg.NewGitHubClient(ghToken)` and use `ghClient` directly:

```go
w := pkg.NewWatcher(
    ghClient,  // was: pkg.NewGitHubClient(ghToken)
    createSender,
    // ... rest unchanged
)
```

### Step 6: Wire the handler in main.go

Update `/workspace/watcher/github-pr/main.go`:

1. In the `Run` function, create the GitHubClient once and pass it to both factories:

   Around line 189, change:
   ```go
   w, cleanup, err := factory.CreateWatcher(
       ctx,
       a.GHToken,  // remove this
       // ... rest
   )
   ```
   To:
   ```go
   ghClient := pkg.NewGitHubClient(a.GHToken)
   w, cleanup, err := factory.CreateWatcher(
       ctx,
       ghClient,  // pass explicit client
       a.KafkaBrokers,
       // ... rest
   )
   ```

2. Add `TriggerHandler http.Handler` field to the `application` struct (around line 104):
   ```go
   type application struct {
       // ... existing fields ...
       TriggerHandler http.Handler
   }
   ```

3. After the `CreateWatcher` call (around line 205), create the trigger handler:
   ```go
   triggerHandler, err := factory.CreateSinglePRHandler(
       ctx,
       ghClient,
       createSender,
       taskCreationFilter,
       trustDecision,
       a.Stage,
       a.MaxSlugLen,
       a.MaxTitleLen,
       a.TaskSuffix,
   )
   if err != nil {
       return errors.Wrap(ctx, err, "create single-PR trigger handler")
   }
   a.TriggerHandler = triggerHandler
   ```

4. In `runHTTPServer` (around line 251), add the trigger route:
   ```go
   router.Path("/trigger").Handler(a.TriggerHandler)
   ```

   Note: The trigger handler is NOT a `BackgroundRunHandler` — it writes its own HTTP response. Use the raw handler.

### Step 7: Add unit tests

Create `/workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler_test

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/bborbe/agent/lib/command/task"
    "github.com/bborbe/agent/lib/command/task/mocks"
    "github.com/bborbe/errors"

    "github.com/bborbe/maintainer/watcher/github-pr/pkg"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
    "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

var _ = Describe("TriggerHandler", func() {
    var (
        ghClient           *mocks.GitHubClient
        createSender       *mocks.TaskCreateCommandSender
        taskCreationFilter *mocks.TaskCreationFilter
        trustDecision      *mocks.Trust
        h                  http.Handler
    )

    BeforeEach(func() {
        ghClient = new(mocks.GitHubClient)
        createSender = new(mocks.TaskCreateCommandSender)
        taskCreationFilter = new(mocks.TaskCreationFilter)
        trustDecision = new(mocks.Trust)

        taskCreationFilter.SkipReturns(false)
        trustDecision.IsTrustedReturns(trust.NewResult(true, "trusted"), nil)

        h = handler.NewSinglePRTriggerHandler(
            ghClient,
            createSender,
            taskCreationFilter,
            trustDecision,
            "dev",
            80, 200, "",
        )
    })

    describeTable("error cases", func(rawURL string, expectedStatus int) {
        req := httptest.NewRequest("POST", "/trigger?"+rawURL, nil)
        resp := httptest.NewRecorder()
        h.ServeHTTP(resp, req)
        Expect(resp.Code).To(Equal(expectedStatus))
        var body map[string]string
        json.Unmarshal(resp.Body.Bytes(), &body)
        if expectedStatus != http.StatusOK {
            Expect(body["pr_url"]).ToNot(BeEmpty())
        }
    },
        entry("missing url returns 400", "foo=bar", http.StatusBadRequest),
        entry("empty url returns 400", "url=", http.StatusBadRequest),
        entry("invalid url returns 400", "url=not-a-url", http.StatusBadRequest),
        entry("non-github platform returns 400", "url=https://bitbucket.org/owner/repo/pull-requests/1", http.StatusBadRequest),
    )

    Context("GitHub fetch failure", func() {
        BeforeEach(func() {
            ghClient.GetPRDetailsReturns(pkg.PRDetails{}, errors.New("network error"))
        })

        It("returns 502", func() {
            req := httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/1", nil)
            resp := httptest.NewRecorder()
            h.ServeHTTP(resp, req)
            Expect(resp.Code).To(Equal(http.StatusBadGateway))
            var body map[string]string
            json.Unmarshal(resp.Body.Bytes(), &body)
            Expect(body["error"]).To(ContainSubstring("github fetch failed"))
        })
    })

    Context("filter rejection", func() {
        BeforeEach(func() {
            ghClient.GetPRDetailsReturns(pkg.PRDetails{
                HeadSHA:     "abc123",
                CloneURL:    "https://github.com/bborbe/repo.git",
                BaseRef:     "main",
                AuthorLogin: "dependabot[bot]",
                Title:       "Bump foo from 1.0 to 2.0",
                IsDraft:     false,
            }, nil)
        })

        Context("draft filter rejects", func() {
            BeforeEach(func() {
                taskCreationFilter.SkipStub = func(pr filter.PR) bool {
                    return pr.AuthorLogin == "dependabot[bot]"
                }
            })

            It("returns 422 with filter name", func() {
                req := httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/1", nil)
                resp := httptest.NewRecorder()
                h.ServeHTTP(resp, req)
                Expect(resp.Code).To(Equal(http.StatusUnprocessableEntity))
                var body map[string]string
                json.Unmarshal(resp.Body.Bytes(), &body)
                Expect(body["filter"]).ToNot(BeEmpty())
            })
        })
    })

    Context("Kafka publish failure", func() {
        BeforeEach(func() {
            ghClient.GetPRDetailsReturns(pkg.PRDetails{
                HeadSHA:  "abc123",
                CloneURL: "https://github.com/bborbe/repo.git",
                BaseRef:  "main",
            }, nil)
            createSender.SendCommandReturns(errors.New("kafka error"))
        })

        It("returns 502", func() {
            req := httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/1", nil)
            resp := httptest.NewRecorder()
            h.ServeHTTP(resp, req)
            Expect(resp.Code).To(Equal(http.StatusBadGateway))
            var body map[string]string
            json.Unmarshal(resp.Body.Bytes(), &body)
            Expect(body["error"]).To(ContainSubstring("kafka publish failed"))
        })
    })

    Context("happy path", func() {
        BeforeEach(func() {
            ghClient.GetPRDetailsReturns(pkg.PRDetails{
                HeadSHA:  "abc123",
                CloneURL: "https://github.com/bborbe/repo.git",
                BaseRef:  "main",
            }, nil)
        })

        It("returns 200 with task_id", func() {
            req := httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42", nil)
            resp := httptest.NewRecorder()
            h.ServeHTTP(resp, req)
            Expect(resp.Code).To(Equal(http.StatusOK))
            var body map[string]interface{}
            json.Unmarshal(resp.Body.Bytes(), &body)
            Expect(body["task_id"]).ToNot(BeEmpty())
            Expect(body["repo"]).To(Equal("bborbe/repo"))
            Expect(body["pr_number"]).To(Equal(float64(42)))
            Expect(body["head_sha"]).To(Equal("abc123"))
        })

        It("calls createSender with correct task", func() {
            req := httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42", nil)
            resp := httptest.NewRecorder()
            h.ServeHTTP(resp, req)
            Expect(createSender.SendCommandCallCount()).To(Equal(1))
        })
    })

    Context("trust-branching: untrusted author", func() {
        var sentCmd task.CreateCommand

        BeforeEach(func() {
            ghClient.GetPRDetailsReturns(pkg.PRDetails{
                HeadSHA:     "abc123",
                CloneURL:    "https://github.com/bborbe/repo.git",
                BaseRef:     "main",
                AuthorLogin: "unknown-user",
                Title:       "Fix bug",
                IsDraft:     false,
            }, nil)
            trustDecision.IsTrustedReturns(trust.NewResult(false, "author not in allowlist"), nil)
            createSender.SendCommandStub = func(ctx context.Context, cmd task.CreateCommand) error {
                sentCmd = cmd
                return nil
            }
        })

        It("routes to human_review frontmatter (phase=human_review, status=todo)", func() {
            req := httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42", nil)
            resp := httptest.NewRecorder()
            h.ServeHTTP(resp, req)
            Expect(resp.Code).To(Equal(http.StatusOK))
            Expect(sentCmd.Frontmatter["phase"]).To(Equal("human_review"))
            Expect(sentCmd.Frontmatter["status"]).To(Equal("todo"))
        })
    })

    Context("trust-branching: trusted author", func() {
        var sentCmd task.CreateCommand

        BeforeEach(func() {
            ghClient.GetPRDetailsReturns(pkg.PRDetails{
                HeadSHA:     "abc123",
                CloneURL:    "https://github.com/bborbe/repo.git",
                BaseRef:     "main",
                AuthorLogin: "bborbe",
                Title:       "Feature: add support",
                IsDraft:     false,
            }, nil)
            trustDecision.IsTrustedReturns(trust.NewResult(true, "trusted"), nil)
            createSender.SendCommandStub = func(ctx context.Context, cmd task.CreateCommand) error {
                sentCmd = cmd
                return nil
            }
        })

        It("routes to in_progress frontmatter (phase=planning, status=in_progress)", func() {
            req := httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42", nil)
            resp := httptest.NewRecorder()
            h.ServeHTTP(resp, req)
            Expect(resp.Code).To(Equal(http.StatusOK))
            Expect(sentCmd.Frontmatter["phase"]).To(Equal("planning"))
            Expect(sentCmd.Frontmatter["status"]).To(Equal("in_progress"))
        })
    })
})
```

Note: The mock `mocks.TaskCreateCommandSender` is the counterfeiter-generated mock from `github.com/bborbe/agent/lib/command/task/mocks/task-create-command-sender.go` with fake name `TaskCreateCommandSender`. Import it as `mocks` from the task package. Verify the exact import path by reading the mock file first if needed.

### Step 8: Generate mocks

Run `cd /workspace/watcher/github-pr && go generate ./...` to generate the `SinglePRTriggerHandler` mock from the `//counterfeiter:generate` directive in the handler file.

## Constraints

- Do NOT duplicate `computePRTitle`, `DeriveTaskID`, `buildFrontmatter`, `buildTaskBody`, `buildHumanReviewFrontmatter`, `buildUntrustedBody` — use `BuildCreateCommand`
- Filter chain is passed in (constructor), not rebuilt in the handler
- Trust evaluation runs on the fetched PR, same as poll path
- Do NOT use `BackgroundRunHandler` for the trigger route — the handler writes its own HTTP response
- All errors via `errors.Errorf` or `errors.Wrapf` — no stdlib `fmt.Errorf`
- BSD license header on every new file
- Do NOT use `libtime.NewCurrentDateTime().Now()` inside the handler — time comes from the passed-in filter/trust objects
- The trigger handler does NOT bypass the vault dedup — it publishes the same CreateTaskCommand as the poll path; controller dedup handles already-materialized tasks

## Verification

```bash
# Files exist
test -f /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
test -f /workspace/watcher/github-pr/pkg/factory/single_pr.go
test -f /workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go

# PRDetails has new fields
grep -n 'AuthorLogin\|Title\|IsDraft' /workspace/watcher/github-pr/pkg/githubclient.go

# BuildCreateCommand is exported
grep -n 'func BuildCreateCommand' /workspace/watcher/github-pr/pkg/watcher.go

# Route registration: /trigger is the new handler, not BackgroundRunHandler
grep -n 'Path("/trigger")' /workspace/watcher/github-pr/main.go
# Verify it's NOT BackgroundRunHandler
grep -A1 'Path("/trigger")' /workspace/watcher/github-pr/main.go | grep -v BackgroundRunHandler

# /check route still exists
grep -n 'Path("/check")' /workspace/watcher/github-pr/main.go

# Generate mocks
cd /workspace/watcher/github-pr && go generate ./...

# Run tests with coverage
cd /workspace/watcher/github-pr && go test -cover ./pkg/handler/...

# Check coverage >=80% on trigger_handler.go
cd /workspace/watcher/github-pr && go test -coverprofile=/tmp/cover.out ./pkg/handler/... && go tool cover -func=/tmp/cover.out

# Boundary test: CreateCommand.Validate passes with PR titles containing special chars
# (add test case with Title: "feat: add /path?query support" — verifies no shell interpolation, no path traversal in title)

# Make precommit
cd /workspace/watcher/github-pr && make precommit
```