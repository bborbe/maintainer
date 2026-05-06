---
status: completed
spec: [020-richer-build-task-context]
container: maintainer-099-spec-020-include-logs
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T21:00:00Z"
queued: "2026-05-06T20:54:21Z"
started: "2026-05-06T21:09:06Z"
completed: "2026-05-06T21:20:42Z"
branch: dark-factory/richer-build-task-context
---

<summary>
- Per-repo opt-in flag `include_logs: true` in `.maintenance.yaml` enables an `## Error` section in the task body with the last 30 lines (≤ 4 KB) of the primary failing job's log
- `IncludeLogs bool` is added to `GithubBuildConfig` in the maintenance package; the YAML parser extracts `include_logs` from the `watcher.github-build` subtree
- `GetJobLog` is added to the `GitHubClient` interface: fetches the job log via GitHub's redirect URL then reads the plain-text log via HTTP; rejects > 1 MiB payloads before truncation
- `buildCreateTaskCommand` receives an `includeLogs bool` parameter; `applyStateMachine` extracts the value from maintenance overrides
- A `redactLogSnippet` helper applies five exact regex patterns before the snippet enters the body
- Log fetch failures (any reason) and > 1 MiB payloads produce a WARN log and omit `## Error` — the publish always succeeds with the rich context already present
- Unit tests verify each of the five redaction patterns individually, log truncation, and both include_logs=true and include_logs=false paths
- `docs/build-watcher.md` gains an `## include_logs` section documenting the opt-in, schema, redaction, and failure modes
- CHANGELOG entry under `## Unreleased`
</summary>

<objective>
Add per-repo opt-in log snippets to build-failure task bodies. When `include_logs: true` appears in a repo's `.maintenance.yaml` under `watcher.github-build`, the watcher fetches the primary failing job's log, applies regex redaction of known secret patterns, caps it at 30 lines or 4 KB, and emits it as a fenced code block in an `## Error` section. All failure modes degrade silently — the publish always completes, with or without the log.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — private helpers, interface patterns.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, DescribeTable, coverage ≥80%.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.
Read `go-security-linting.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `#nosec` usage, `http.DefaultClient` vs custom client.

**Dependency check — run before making any changes:**

```bash
# Confirm prompt 2 is complete (GetJobsForRun present and ctx in buildCreateTaskCommand):
grep -n "GetJobsForRun" watcher/github-build/pkg/githubclient.go
grep -n "ctx context.Context" watcher/github-build/pkg/watcher.go | grep "buildCreateTaskCommand"
```
If `GetJobsForRun` is absent from the interface OR `buildCreateTaskCommand` does not take `ctx context.Context` as its first parameter, STOP and report `status: failed` with reason "spec-020 prompt 2 not yet executed".

**Files to read fully before making any changes:**
- `watcher/github-build/pkg/maintenance/loader.go` — full file; understand `GithubBuildConfig` struct (fields: `Assignee`, `Status`, `Phase`), the `rawConfig` unmarshalling structure, the `known` map, and how YAML fields are extracted
- `watcher/github-build/pkg/githubclient.go` — full file; understand `GitHubClient` interface, existing error detection patterns, `ErrRateLimited` sentinel
- `watcher/github-build/pkg/watcher.go` — full file; understand `buildCreateTaskCommand` body (the table loop and the area after the table where `body` is assembled); understand `applyStateMachine` where maintenance overrides are extracted
- `watcher/github-build/pkg/watcher_internal_test.go` — full file; follow `DescribeTable` pattern for new redaction unit tests
- `watcher/github-build/pkg/mocks/github_client.go` — understand mock structure; will be regenerated
- `docs/build-watcher.md` — full file; understand existing sections to know where to append new content

**Verify go-github v62 job logs API before writing any code:**

```bash
# Confirm GetWorkflowJobLogs method signature:
grep -n "func.*Actions.*GetWorkflowJobLogs" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_jobs.go 2>/dev/null | head -5

# Confirm return types (expect *url.URL, *Response, error):
grep -A 5 "func.*GetWorkflowJobLogs" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_jobs.go 2>/dev/null | head -10
```

If `GetWorkflowJobLogs` does not return a `*url.URL`, adapt the implementation to what the grep finds and document the deviation in `## Improvements`.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 7. Run `make precommit` only at the final step.**

1. **Add `IncludeLogs bool` to `GithubBuildConfig` in `watcher/github-build/pkg/maintenance/loader.go`**

   Update the struct:

   ```go
   type GithubBuildConfig struct {
       Assignee    string
       Status      string
       Phase       string
       IncludeLogs bool // include_logs: true enables the ## Error log snippet opt-in
   }
   ```

   In `LoadOverrides`, update the `known` map to include `"include_logs"`:

   ```go
   known := map[string]bool{
       "assignee":     true,
       "status":       true,
       "phase":        true,
       "include_logs": true,
   }
   ```

   After the existing `if v, ok := buildSection["phase"].(string); ok { ... }` block, add:

   ```go
   if v, ok := buildSection["include_logs"].(bool); ok {
       cfg.IncludeLogs = v
   }
   ```

   No mock regeneration needed — `GithubBuildConfig` is a value type returned by `LoadOverrides`; existing tests that pass `maintenance.GithubBuildConfig{}` still compile with `IncludeLogs` defaulting to `false`.

2. **Add `GetJobLog` to the `GitHubClient` interface and implement it in `watcher/github-build/pkg/githubclient.go`**

   Interface declaration (add after `GetJobsForRun`):

   ```go
   // GetJobLog fetches the plain-text log for a workflow job by following GitHub's
   // redirect to an Azure storage URL. Returns (nil, nil) when the URL is unavailable.
   // Returns (nil, err) for a non-nil error where the log should be omitted.
   // Rejects payloads > 1 MiB before truncation (returns (nil, err)).
   GetJobLog(ctx context.Context, owner, repo string, jobID int64) ([]byte, error)
   ```

   Concrete implementation on `*githubClient` (add after `GetJobsForRun`):

   ```go
   func (c *githubClient) GetJobLog(
       ctx context.Context,
       owner, repo string,
       jobID int64,
   ) ([]byte, error) {
       logURL, _, err := c.client.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 0)
       if err != nil {
           var rl *gogithub.RateLimitError
           var arl *gogithub.AbuseRateLimitError
           if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
               return nil, ErrRateLimited
           }
           return nil, errors.Wrapf(ctx, err, "get job log URL owner=%s repo=%s job=%d", owner, repo, jobID)
       }
       if logURL == nil {
           return nil, nil
       }
       req, err := http.NewRequestWithContext(ctx, http.MethodGet, logURL.String(), nil)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "create log request job=%d", jobID)
       }
       resp, err := http.DefaultClient.Do(req) // #nosec G107 — URL comes from GitHub API, not user input
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "fetch log job=%d", jobID)
       }
       defer resp.Body.Close()

       const maxBytes = 1024 * 1024 // 1 MiB
       // Read one extra byte to detect >1 MiB without reading the entire payload:
       data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "read log body job=%d", jobID)
       }
       if len(data) > maxBytes {
           return nil, errors.Errorf(ctx, "log payload exceeds 1 MiB for job=%d (got %d bytes) — treating as suspicious", jobID, len(data))
       }
       return data, nil
   }
   ```

   Add `"io"` and `"net/http"` to the import block of `githubclient.go` if not already present.

3. **Regenerate the `GitHubClient` mock** (interface gained `GetJobLog`):

   ```bash
   cd watcher/github-build && go generate ./pkg/...
   ```

   If `go generate` does not trigger, run counterfeiter directly:

   ```bash
   cd watcher/github-build && \
     go run github.com/maxbrunsfeld/counterfeiter/v6 \
       -o pkg/mocks/github_client.go \
       --fake-name GitHubClient \
       ./pkg/. GitHubClient
   ```

   Confirm `pkg/mocks/github_client.go` includes `GetJobLogStub`, `GetJobLogCallCount`, `GetJobLogArgsForCall`, `GetJobLogReturns`.

4. **Add `redactLogSnippet` helper to `watcher/github-build/pkg/watcher.go`**

   Add this function after `formatDuration` at the bottom of `watcher.go`. The function is intentionally free of early returns so each pattern is always applied in the documented order.

   First, add `"regexp"` to the import block if not present.

   ```go
   // redactLogSnippet applies regex redaction to remove known secret patterns from a
   // CI log snippet before it enters the task body.
   //
   // ORDER MATTERS — apply specific patterns BEFORE the generic hex catch-all so token
   // shapes that happen to be hex (e.g. github tokens are alphanumeric so unaffected;
   // but a future bearer-token shape that's hex-only would be caught by step 5 with a
   // generic [REDACTED] marker, losing the "Bearer " prefix). Reordering here is a bug.
   //
   // Pattern 5 (40+-char hex) WILL redact the episode SHA if it appears verbatim in
   // log output. Acceptable: the SHA is already shown in plain text in the body header,
   // so the operator hasn't lost recoverable context. False positives < leaked tokens.
   func redactLogSnippet(s string) string {
       // 1. GitHub tokens: gho_, ghp_, ghs_, ghu_ followed by ≥16 alphanumerics
       s = regexp.MustCompile(`gh[opsu]_[a-zA-Z0-9]{16,}`).ReplaceAllString(s, "[REDACTED]")

       // 2. Bearer auth headers: "Bearer " followed by ≥16 token chars
       s = regexp.MustCompile(`Bearer\s+[A-Za-z0-9._-]{16,}`).ReplaceAllString(s, "Bearer [REDACTED]")

       // 3. AWS access key IDs: AKIA followed by 16 uppercase alphanumerics
       s = regexp.MustCompile(`AKIA[0-9A-Z]{16}`).ReplaceAllString(s, "[REDACTED]")

       // 4. AWS secret access keys: keep the key= prefix, redact the 40-char base64 secret
       s = regexp.MustCompile(`(aws_secret_access_key[\s=:]+["']?)[A-Za-z0-9/+]{40}["']?`).
           ReplaceAllString(s, "${1}[REDACTED]")

       // 5. Long opaque hex strings (≥40 chars) — generic auth hashes catch-all.
       //    Will also match the episode SHA if present in log output — acceptable per spec.
       //    MUST run last so the specific patterns above (1-4) match their tokens first.
       s = regexp.MustCompile(`\b[a-f0-9]{40,}\b`).ReplaceAllString(s, "[REDACTED]")

       return s
   }
   ```

5. **Add `lastNLinesUpTo4KB` helper to `watcher/github-build/pkg/watcher.go`**

   Add after `redactLogSnippet`:

   ```go
   // lastNLinesUpTo4KB returns the last n lines of s, further capped at maxBytes bytes.
   // Applied AFTER redaction to limit what enters the task body.
   func lastNLinesUpTo4KB(s string, n int) string {
       const maxBytes = 4096
       // Trim trailing newline to avoid a phantom empty last line
       s = strings.TrimRight(s, "\n")
       lines := strings.Split(s, "\n")
       if len(lines) > n {
           lines = lines[len(lines)-n:]
       }
       snippet := strings.Join(lines, "\n")
       if len(snippet) > maxBytes {
           // Keep the tail: truncate from the start, then trim to the next line boundary
           snippet = snippet[len(snippet)-maxBytes:]
           if idx := strings.Index(snippet, "\n"); idx >= 0 && idx < len(snippet)-1 {
               snippet = snippet[idx+1:]
           }
       }
       return snippet
   }
   ```

6. **Update `buildCreateTaskCommand` in `watcher/github-build/pkg/watcher.go`**

   a. Add `includeLogs bool` as the last parameter:

   ```go
   func (w *buildWatcher) buildCreateTaskCommand(
       ctx context.Context,
       taskID uuid.UUID,
       owner, repo, episodeSHA string,
       failingRuns []WorkflowRun,
       assignee, taskStatus, taskPhase string,
       includeLogs bool,
   ) WatcherCreateTaskCommand {
   ```

   b. In the body-building section, after the table loop (after all rows are appended), add the log section. The primary run for log fetching is `failingRuns[0]` (the run that established the episode SHA). The jobs info from that run is already fetched during the table loop. To avoid a second jobs API call, save the job info from `failingRuns[0]` during the loop:

   Modify the table-building loop to save job info for the first run:

   ```go
   var primaryJobID int64  // job ID for the primary failing run (failingRuns[0] log fetch)
   for i, run := range failingRuns {
       jobName := "?"
       stepName := "?"
       if run.RunID != 0 {
           jobs, err := w.githubClient.GetJobsForRun(ctx, owner, repo, run.RunID)
           if err != nil {
               glog.Warningf("jobs API failed run=%d repo=%s/%s err=%v — using ? placeholders", run.RunID, owner, repo, err)
           } else if len(jobs) > 0 {
               jobName = jobs[0].JobName
               if jobs[0].FailedStepName != "" {
                   stepName = jobs[0].FailedStepName
               }
               if i == 0 {
                   primaryJobID = jobs[0].JobID
               }
           }
       }
       lines = append(lines, fmt.Sprintf("| %s | %s | %s | [Run](%s) |",
           run.Name, jobName, stepName, run.HTMLURL))
   }
   ```

   c. After the loop, add the `## Error` section when `includeLogs` and a job ID is available:

   ```go
   if includeLogs && primaryJobID != 0 {
       logData, err := w.githubClient.GetJobLog(ctx, owner, repo, primaryJobID)
       if err != nil {
           glog.Warningf("log fetch failed repo=%s/%s job=%d err=%v — omitting ## Error section", owner, repo, primaryJobID, err)
       } else if logData != nil {
           redacted := redactLogSnippet(string(logData))
           snippet := lastNLinesUpTo4KB(redacted, 30)
           if snippet != "" {
               lines = append(lines,
                   "",
                   "## Error",
                   "",
                   "```",
                   snippet,
                   "```",
               )
           }
       }
   }
   ```

   The `## Error` section MUST appear after the table and before the `body` string join.

7. **Update `applyStateMachine` in `watcher/github-build/pkg/watcher.go`** to extract `IncludeLogs` from overrides and pass it to `buildCreateTaskCommand`:

   Locate the call to `buildCreateTaskCommand` inside the `green→red` branch of `applyStateMachine`. Currently (after prompt 2):
   ```go
   cmd := w.buildCreateTaskCommand(
       ctx,
       taskID,
       owner,
       repo,
       episodeSHA,
       failingRuns,
       effectiveAssignee,
       effectiveStatus,
       effectivePhase,
   )
   ```

   Change to:
   ```go
   cmd := w.buildCreateTaskCommand(
       ctx,
       taskID,
       owner,
       repo,
       episodeSHA,
       failingRuns,
       effectiveAssignee,
       effectiveStatus,
       effectivePhase,
       overrides.IncludeLogs,
   )
   ```

   `overrides` is already declared above this line (from the existing `overrides := w.maintenanceLoader.LoadOverrides(...)` call). `overrides.IncludeLogs` is `false` by default when the field is absent from `.maintenance.yaml` — no `coalesceString` equivalent needed since the Go zero value for `bool` IS the correct default.

8. **Run `make test`** to verify:

   ```bash
   cd watcher/github-build && make test
   ```

   If `buildCreateTaskCommand` internal tests in `watcher_internal_test.go` call it directly, update the call to include `false` as the last argument. Fix any compile errors before proceeding.

9. **Add redaction unit tests in `watcher/github-build/pkg/watcher_internal_test.go`**

   Append after the `formatDuration` tests:

   ```go
   var _ = Describe("redactLogSnippet", func() {
       DescribeTable("redacts known secret patterns",
           func(input, wantContain, wantNotContain string) {
               result := redactLogSnippet(input)
               if wantContain != "" {
                   Expect(result).To(ContainSubstring(wantContain))
               }
               if wantNotContain != "" {
                   Expect(result).NotTo(ContainSubstring(wantNotContain))
               }
           },
           Entry("GitHub PAT (ghp_)",
               "token=ghp_ABCDEFGHIJKLMNOPabcde",
               "[REDACTED]", "ghp_ABCDEFGHIJKLMNOPabcde"),
           Entry("GitHub OAuth token (gho_)",
               "Authorization: gho_ABCDEFGHIJKLMNOPqrstu",
               "[REDACTED]", "gho_ABCDEFGHIJKLMNOPqrstu"),
           Entry("Bearer header",
               "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc",
               "Bearer [REDACTED]", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
           Entry("AWS access key ID",
               "access_key=AKIAIOSFODNN7EXAMPLE1",
               "[REDACTED]", "AKIAIOSFODNN7EXAMPLE1"),
           Entry("AWS secret access key",
               "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
               "aws_secret_access_key = [REDACTED]", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
           Entry("long hex string (≥40 hex chars)",
               "token: da39a3ee5e6b4b0d3255bfef95601890afd80709 ok",
               "[REDACTED]", "da39a3ee5e6b4b0d3255bfef95601890afd80709"),
           Entry("safe short hex string not redacted",
               "short: abc123",
               "short: abc123", ""),
           Entry("non-secret text passes through unchanged",
               "INFO: build succeeded in 42s",
               "INFO: build succeeded in 42s", ""),
       )
   })

   var _ = Describe("lastNLinesUpTo4KB", func() {
       It("returns last N lines when fewer than N lines exist", func() {
           Expect(lastNLinesUpTo4KB("a\nb\nc", 10)).To(Equal("a\nb\nc"))
       })

       It("returns exactly the last N lines when more exist", func() {
           input := strings.Join([]string{"1", "2", "3", "4", "5"}, "\n")
           result := lastNLinesUpTo4KB(input, 3)
           Expect(result).To(Equal("3\n4\n5"))
       })

       It("caps at 4096 bytes when last N lines exceed 4 KB", func() {
           // Build a string where the last 30 lines sum to > 4096 bytes
           longLine := strings.Repeat("x", 200) // 200 bytes per line
           lines := make([]string, 40)
           for i := range lines {
               lines[i] = longLine
           }
           input := strings.Join(lines, "\n")
           result := lastNLinesUpTo4KB(input, 30)
           Expect(len(result)).To(BeNumerically("<=", 4096))
       })

       It("handles empty input", func() {
           Expect(lastNLinesUpTo4KB("", 30)).To(Equal(""))
       })
   })
   ```

   Add `"strings"` import to `watcher_internal_test.go` if not already present.

10. **Add integration tests for include_logs in `watcher/github-build/pkg/watcher_test.go`**

    Append a new `Describe("include_logs opt-in", ...)` block:

    ```go
    Describe("include_logs opt-in", func() {
        var maintenanceLoaderWithLogs *mocks.MaintenanceLoader
        var runID int64 = 77
        var jobID int64 = 99

        makeWatcherWithLogs := func(includeLogs bool) pkg.Watcher {
            maintenanceLoaderWithLogs = new(mocks.MaintenanceLoader)
            maintenanceLoaderWithLogs.LoadOverridesReturns(maintenance.GithubBuildConfig{
                IncludeLogs: includeLogs,
            })
            return pkg.NewWatcher(
                ghClient,
                publisher,
                metrics,
                filter.RepoFilters{},
                []string{"owner/repo"},
                cursorPath,
                "build-fixer-agent",
                "todo",
                "",
                maintenanceLoaderWithLogs,
            )
        }

        singleFailingRunWithJobID := func(sha string) []pkg.WorkflowRun {
            return []pkg.WorkflowRun{
                {
                    WorkflowID: 1,
                    RunID:      runID,
                    Name:       "CI",
                    HeadSHA:    sha,
                    Conclusion: "failure",
                    HTMLURL:    "https://github.com/owner/repo/actions/runs/77",
                    CreatedAt:  time.Now(),
                },
            }
        }

        BeforeEach(func() {
            ghClient.GetDefaultBranchReturns("main", nil)
            ghClient.GetJobsForRunReturns([]pkg.WorkflowJobInfo{
                {JobID: jobID, JobName: "build", FailedStepName: "Run tests"},
            }, nil)
        })

        It("emits ## Error section with redacted snippet when include_logs=true", func() {
            ghClient.GetWorkflowRunsReturns(singleFailingRunWithJobID("sha-logs"), nil)
            // Log contains a GitHub token that should be redacted
            logContent := "step 1: ok\nstep 2: token=ghp_ABCDEFGHIJKLMNOPqrstu\nstep 3: FAILED"
            ghClient.GetJobLogReturns([]byte(logContent), nil)

            w := makeWatcherWithLogs(true)
            Expect(w.Poll(ctx)).To(Succeed())

            Expect(publisher.PublishCreateCallCount()).To(Equal(1))
            _, cmd := publisher.PublishCreateArgsForCall(0)
            Expect(cmd.Body).To(ContainSubstring("## Error"))
            Expect(cmd.Body).To(ContainSubstring("```"))
            // Token must be redacted
            Expect(cmd.Body).NotTo(ContainSubstring("ghp_ABCDEFGHIJKLMNOPqrstu"))
            Expect(cmd.Body).To(ContainSubstring("[REDACTED]"))
            // Log content (sans token) must be present
            Expect(cmd.Body).To(ContainSubstring("step 3: FAILED"))
        })

        It("omits ## Error section when include_logs=false (default)", func() {
            ghClient.GetWorkflowRunsReturns(singleFailingRunWithJobID("sha-nologs"), nil)

            w := makeWatcherWithLogs(false)
            Expect(w.Poll(ctx)).To(Succeed())

            Expect(publisher.PublishCreateCallCount()).To(Equal(1))
            _, cmd := publisher.PublishCreateArgsForCall(0)
            Expect(cmd.Body).NotTo(ContainSubstring("## Error"))
            // GetJobLog must NOT be called when include_logs=false
            Expect(ghClient.GetJobLogCallCount()).To(Equal(0))
        })

        It("omits ## Error section when log fetch fails; publish still succeeds", func() {
            ghClient.GetWorkflowRunsReturns(singleFailingRunWithJobID("sha-logfail"), nil)
            ghClient.GetJobLogReturns(nil, os.ErrNotExist)

            w := makeWatcherWithLogs(true)
            Expect(w.Poll(ctx)).To(Succeed())

            Expect(publisher.PublishCreateCallCount()).To(Equal(1))
            _, cmd := publisher.PublishCreateArgsForCall(0)
            // Body still has the table — just no ## Error section
            Expect(cmd.Body).To(ContainSubstring("## Failing Workflows"))
            Expect(cmd.Body).NotTo(ContainSubstring("## Error"))
        })

        It("omits ## Error when jobs API fails (no jobID to log-fetch with)", func() {
            ghClient.GetWorkflowRunsReturns(singleFailingRunWithJobID("sha-nojob"), nil)
            // jobs API failure → primaryJobID stays 0 → log fetch skipped
            ghClient.GetJobsForRunReturns(nil, os.ErrNotExist)
            ghClient.GetJobLogReturns(nil, nil)

            w := makeWatcherWithLogs(true)
            Expect(w.Poll(ctx)).To(Succeed())

            Expect(publisher.PublishCreateCallCount()).To(Equal(1))
            _, cmd := publisher.PublishCreateArgsForCall(0)
            Expect(cmd.Body).NotTo(ContainSubstring("## Error"))
            Expect(ghClient.GetJobLogCallCount()).To(Equal(0))
        })
    })
    ```

11. **Add maintenance loader test for `include_logs` in `watcher/github-build/pkg/maintenance/loader_test.go`**

    Append a new test case in the existing `Describe("Loader", ...)` after the last existing `Context`:

    ```go
    Context("valid YAML — include_logs: true", func() {
        It("returns IncludeLogs=true", func() {
            content := []byte(`watcher:
  github-build:
    assignee: go-deps-fixer-agent
    include_logs: true
`)
            fetcher.GetFileContentReturns(content, nil)
            cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
            Expect(cfg.IncludeLogs).To(BeTrue())
            Expect(cfg.Assignee).To(Equal("go-deps-fixer-agent"))
        })
    })

    Context("valid YAML — include_logs absent (default false)", func() {
        It("returns IncludeLogs=false", func() {
            content := []byte(`watcher:
  github-build:
    assignee: build-fixer-agent
`)
            fetcher.GetFileContentReturns(content, nil)
            cfg := loader.LoadOverrides(ctx, "owner", "repo", "main")
            Expect(cfg.IncludeLogs).To(BeFalse())
        })
    })
    ```

12. **Update `docs/build-watcher.md`** — append a new section after the `## Per-Repo Configuration (.maintenance.yaml)` section:

    ````markdown
    ## Log Snippets (`include_logs`)

    Opt-in per repo by adding to `.maintenance.yaml`:

    ```yaml
    watcher:
      github-build:
        include_logs: true
    ```

    When enabled, each build-failure task body gains an `## Error` section with the last
    30 lines (≤ 4 KB) of the primary failing job's log, fenced as a code block and redacted.

    **Default:** `false`. Repos that do not set this flag see no `## Error` section.

    ### Redaction

    The following patterns are stripped before the snippet enters the task body:

    | Pattern | Replaces with |
    |---|---|
    | `gh[opsu]_[a-zA-Z0-9]{16,}` | `[REDACTED]` |
    | `Bearer\s+[A-Za-z0-9._-]{16,}` | `Bearer [REDACTED]` |
    | `AKIA[0-9A-Z]{16}` | `[REDACTED]` |
    | `aws_secret_access_key[\s=:]+["']?[A-Za-z0-9/+]{40}["']?` | keeps prefix, redacts secret |
    | `\b[a-f0-9]{40,}\b` | `[REDACTED]` (catches generic auth hashes; false-positive on commit SHAs is acceptable — SHAs already appear in the header) |

    **Residual risk:** Regex cannot catch every token shape. Operators should audit their
    CI logs for secret leakage before enabling `include_logs: true` on a repo.

    ### Size limits

    | Limit | Value | When measured |
    |---|---|---|
    | Raw log payload | 1 MiB max | Before redaction — payloads larger than this are rejected as suspicious and the `## Error` section is omitted |
    | Line cap | last 30 lines | After redaction |
    | Byte cap | 4 KB | After redaction and line cap — the tail is kept when both limits apply |

    ### Failure modes

    | Trigger | Behavior |
    |---|---|
    | Log fetch error (any reason) | WARN log; `## Error` section omitted; publish proceeds |
    | Log size > 1 MiB | WARN log; `## Error` section omitted; publish proceeds |
    | Jobs API failed (no `jobID`) | Log fetch skipped; `## Error` omitted; publish proceeds |
    | `include_logs: false` or absent | No log fetch attempted; `## Error` omitted |
    ````

13. **Add CHANGELOG entry** to root `CHANGELOG.md` under `## Unreleased`:

    ```
    - feat(watcher/github-build): add include_logs opt-in — repos with watcher.github-build.include_logs: true in .maintenance.yaml receive an ## Error section in the task body containing the last 30 lines (≤ 4 KB) of the primary failing job's log, redacted for GitHub tokens, Bearer headers, AWS keys, and long hex strings; log fetch failures degrade silently without blocking the publish
    ```

14. **Run `make precommit`** from `watcher/github-build/`:

    ```bash
    cd watcher/github-build && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/pkg/`, `watcher/github-build/pkg/maintenance/`, `docs/build-watcher.md`, and root `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- **Dependency on spec-020 prompt 2:** If `GetJobsForRun` is missing or `buildCreateTaskCommand` does not take `ctx`, STOP and report `status: failed`
- `include_logs` MUST default to `false` — Go's zero value for `bool` IS the correct default; no `coalesceString`-style merge needed
- `include_logs: true` MUST be honored ONLY from `.maintenance.yaml` via the maintenance loader; there is NO CLI/env override that turns it on globally — `overrides.IncludeLogs` (not any watcher struct field) is the sole gate
- `GetJobLog` MUST reject payloads > 1 MiB BEFORE truncation — read 1 MiB + 1 byte, check length, reject if > 1 MiB
- `redactLogSnippet` MUST apply all 5 patterns listed in the spec Constraints — no additions, no omissions, no `TBD` patterns
- `lastNLinesUpTo4KB` MUST enforce BOTH limits simultaneously: last 30 lines, then cap at 4 KB (keeping the tail)
- `GetJobLog` is called AT MOST ONCE per publish, using `primaryJobID` from `failingRuns[0]`'s job info — no second jobs API call
- When `includeLogs == false` OR `primaryJobID == 0`, `GetJobLog` MUST NOT be called (zero unnecessary API calls)
- Log fetch failures MUST produce a `glog.Warningf` log and MUST NOT return an error from `buildCreateTaskCommand` — the publish always completes
- `## Error` section MUST appear AFTER the `## Failing Workflows` table in the body
- The `## Error` code block MUST use triple-backtick fencing (` ``` `) — no language specifier
- Mock regeneration is REQUIRED after adding `GetJobLog` to the interface
- Error wrapping in production code: `github.com/bborbe/errors` — never `fmt.Errorf`; `#nosec G107` comment required on the `http.DefaultClient.Do` call with the URL from GitHub API (explains the source-is-trusted justification)
- `make precommit` runs from `watcher/github-build/`, never at repo root
- Coverage ≥80% for changed packages
- All existing tests must still pass; `buildCreateTaskCommand` gains one new `bool` parameter at the end — update any internal test that calls it directly by adding `false` as the last argument
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm IncludeLogs field added to GithubBuildConfig:
grep -n "IncludeLogs" watcher/github-build/pkg/maintenance/loader.go
# Expected: struct field + YAML extraction + known map entry

# Confirm include_logs in known map:
grep -n '"include_logs"' watcher/github-build/pkg/maintenance/loader.go
# Expected: one match in the known map

# Confirm GetJobLog in interface:
grep -n "GetJobLog" watcher/github-build/pkg/githubclient.go
# Expected: interface declaration + concrete implementation

# Confirm 1 MiB rejection:
grep -n "maxBytes\|1024.*1024\|len(data).*maxBytes" watcher/github-build/pkg/githubclient.go
# Expected: size check in GetJobLog

# Confirm #nosec on http.DefaultClient.Do:
grep -n "nosec\|DefaultClient" watcher/github-build/pkg/githubclient.go
# Expected: #nosec G107 comment on the Do call

# Confirm mock regenerated with GetJobLog:
grep -n "GetJobLog" watcher/github-build/pkg/mocks/github_client.go
# Expected: GetJobLogStub, GetJobLogCallCount, GetJobLogReturns

# Confirm redactLogSnippet and lastNLinesUpTo4KB helpers:
grep -n "func redactLogSnippet\|func lastNLinesUpTo4KB" watcher/github-build/pkg/watcher.go
# Expected: both functions present

# Confirm all 5 redaction patterns present:
grep -n "gh\[opsu\]\|Bearer\\\\s\|AKIA\[0-9\|aws_secret_access_key\|a-f0-9\]{40" watcher/github-build/pkg/watcher.go
# Expected: 5 regex strings (one per pattern)

# Confirm includeLogs parameter in buildCreateTaskCommand:
grep -A 11 "func.*buildWatcher.*buildCreateTaskCommand" watcher/github-build/pkg/watcher.go
# Expected: last param is includeLogs bool

# Confirm GetJobLog NOT called when includeLogs=false:
grep -n "includeLogs.*&&.*primaryJobID\|primaryJobID.*&&.*includeLogs" watcher/github-build/pkg/watcher.go
# Expected: guard condition before GetJobLog call

# Confirm ## Error appears after the table:
grep -n '"## Error"' watcher/github-build/pkg/watcher.go
# Expected: in the body-building section after the table loop

# Confirm overrides.IncludeLogs passed to buildCreateTaskCommand:
grep -n "overrides.IncludeLogs\|IncludeLogs" watcher/github-build/pkg/watcher.go
# Expected: extracted from overrides + passed to buildCreateTaskCommand

# Confirm redaction tests in internal test file:
grep -n "redactLogSnippet\|lastNLinesUpTo4KB" watcher/github-build/pkg/watcher_internal_test.go
# Expected: DescribeTable blocks for both

# Confirm all 5 patterns tested:
grep -n "ghp_\|Bearer\|AKIA\|aws_secret_access_key\|da39a3ee" watcher/github-build/pkg/watcher_internal_test.go
# Expected: at least 5 test entries (one per pattern)

# Confirm include_logs tests in maintenance loader test:
grep -n "include_logs\|IncludeLogs" watcher/github-build/pkg/maintenance/loader_test.go
# Expected: at least 2 test cases

# Confirm integration tests in watcher_test.go:
grep -n "include_logs\|GetJobLog\|## Error" watcher/github-build/pkg/watcher_test.go
# Expected: new Describe block with assertions

# Confirm docs updated:
grep -n "include_logs\|## Error\|Redaction" docs/build-watcher.md
# Expected: new section with redaction table

# Confirm CHANGELOG entry:
grep -n "include_logs\|## Error\|redact" CHANGELOG.md
# Expected: one match under ## Unreleased
</verification>
