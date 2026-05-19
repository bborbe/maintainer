// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"

	agentlib "github.com/bborbe/agent/lib"
	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

//counterfeiter:generate -o mocks/watcher.go --fake-name Watcher . Watcher

// Watcher polls GitHub and publishes task commands to Kafka.
type Watcher interface {
	Poll(ctx context.Context) error
}

// NewWatcher returns a Watcher that polls GitHub and publishes commands.
func NewWatcher(
	ghClient GitHubClient,
	createSender task.CreateCommandSender,
	cursorPath string,
	startTime libtime.DateTime,
	scope string,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	metrics Metrics,
	trustDecision trust.Trust,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
) Watcher {
	return &watcher{
		ghClient:           ghClient,
		createSender:       createSender,
		cursorPath:         cursorPath,
		startTime:          startTime,
		scope:              scope,
		taskCreationFilter: taskCreationFilter,
		stage:              stage,
		metrics:            metrics,
		trustDecision:      trustDecision,
		maxSlugLen:         maxSlugLen,
		maxTitleLen:        maxTitleLen,
		taskSuffix:         taskSuffix,
	}
}

type watcher struct {
	ghClient           GitHubClient
	createSender       task.CreateCommandSender
	cursorPath         string
	startTime          libtime.DateTime
	scope              string
	taskCreationFilter filter.TaskCreationFilter
	stage              string
	metrics            Metrics
	trustDecision      trust.Trust
	maxSlugLen         int
	maxTitleLen        int
	taskSuffix         string
}

func (w *watcher) Poll(ctx context.Context) error {
	cursorState, err := LoadCursor(ctx, w.cursorPath, w.startTime)
	if err != nil {
		return errors.Wrapf(ctx, err, "load cursor")
	}

	allPRs, abortReason := w.fetchAllPRs(ctx, cursorState.LastUpdatedAt)
	if abortReason != "" {
		w.metrics.IncPollCycle(abortReason)
		return nil
	}

	maxUpdatedAt := w.processPRs(ctx, &cursorState, allPRs)

	if maxUpdatedAt.After(cursorState.LastUpdatedAt) {
		cursorState.LastUpdatedAt = maxUpdatedAt
	}

	if err := SaveCursor(ctx, w.cursorPath, cursorState); err != nil {
		glog.Errorf("failed to save cursor err=%v", err)
	}
	w.metrics.IncPollCycle("success")
	return nil
}

// fetchAllPRs paginates GitHub search results. Returns (prs, "") on success,
// or (nil, reason) where reason is "github_error" or "rate_limited" if the caller should abort.
func (w *watcher) fetchAllPRs(
	ctx context.Context,
	since libtime.DateTime,
) ([]PullRequest, string) {
	page := 1
	var allPRs []PullRequest

	for {
		result, err := w.ghClient.SearchPRs(ctx, w.scope, since, page)
		if err != nil {
			glog.Errorf("github search failed err=%v", err)
			return nil, "github_error"
		}

		allPRs = append(allPRs, result.PullRequests...)

		if !result.HasNextPage {
			break
		}
		page = result.NextPage

		select {
		case <-ctx.Done():
			return nil, ""
		default:
		}
	}
	return allPRs, ""
}

// processPRs iterates over fetched PRs, publishes commands, and returns the max updated-at seen.
// It rebuilds HeadSHAs from only the current open-PR batch, pruning closed/merged PRs.
// Each (PR, SHA) pair produces at most one CreateTaskCommand across all poll cycles.
//
// Design note on cursor preservation for filter-skipped and details-fetch-error PRs:
//
// CRITICAL ASSUMPTION: the controller deduplicates incoming CreateTaskCommands by their
// task_identifier (UUID5). If the controller does NOT dedup, every transient filter toggle
// or transient GetPRDetails failure will produce a duplicate vault file on the next poll.
// VERIFY this assumption against the controller code before merging — search for the
// command consumer's idempotency check; if absent, this design must change to preserve
// per-PR cursor entries (which would require extending the cursor schema with the
// (owner, repo, number) tuple, since UUID5 is not reversible).
//
// Given the assumption holds, we accept that transient filter or fetch failures will cause
// the watcher to re-publish a CreateTaskCommand for the same (PR, SHA) on the next successful
// poll, and rely on controller dedup to make this a no-op. This matches the recovery path
// already documented in the spec failure-mode row "Watcher restart with empty cursor sees a
// PR whose head SHA already has a vault file" — same mechanism, slightly different trigger.
func (w *watcher) processPRs(
	ctx context.Context,
	cursorState *Cursor,
	allPRs []PullRequest,
) libtime.DateTime {
	since := cursorState.LastUpdatedAt
	maxUpdatedAt := since
	prDetailsCache := make(map[string]PRDetails)
	newHeadSHAs := make(map[string]string, len(allPRs))

	for _, pr := range allPRs {
		if w.taskCreationFilter.Skip(
			filter.PR{
				AuthorLogin: pr.AuthorLogin,
				IsDraft:     pr.IsDraft,
				Title:       pr.Title,
				UpdatedAt:   pr.UpdatedAt,
				RepoKey:     "github.com/" + pr.Owner + "/" + pr.Repo,
			},
		) {
			glog.V(3).Infof("skipping pr=%s/%s#%d reason=filtered", pr.Owner, pr.Repo, pr.Number)
			w.metrics.IncPRPublished("skipped")
			// Filtered PRs do not contribute entries to newHeadSHAs. If the PR was previously
			// published, its SHA-based cursor entry is pruned here and will be re-created on
			// the next successful (non-filtered) poll — controller dedup prevents a duplicate file.
			continue
		}

		details, err := w.fetchPRDetails(ctx, pr, prDetailsCache)
		if err != nil {
			glog.Errorf(
				"get pr details failed pr=%s/%s#%d err=%v",
				pr.Owner,
				pr.Repo,
				pr.Number,
				err,
			)
			// Same rationale as filtered PRs: cannot preserve old SHA-based entry without
			// knowing the SHA. Transient error → re-publish on next poll → controller deduplicates.
			continue
		}

		// Fail-closed: if head SHA is absent, skip this PR on this poll.
		if details.HeadSHA == "" {
			glog.Warningf(
				"missing head SHA for pr=%s/%s#%d, skipping",
				pr.Owner,
				pr.Repo,
				pr.Number,
			)
			continue
		}

		taskIDStr := DeriveTaskID(pr.Owner, pr.Repo, pr.Number, details.HeadSHA).String()

		if _, exists := cursorState.HeadSHAs[taskIDStr]; exists {
			// Same (PR, SHA) already spawned — no-op.
			glog.V(3).Infof(
				"no change, skipping pr=%s/%s#%d sha=%s taskID=%s",
				pr.Owner, pr.Repo, pr.Number, details.HeadSHA, taskIDStr,
			)
			newHeadSHAs[taskIDStr] = details.HeadSHA
			if pr.UpdatedAt.After(maxUpdatedAt) {
				maxUpdatedAt = pr.UpdatedAt
			}
			continue
		}

		// New (PR, SHA) pair — publish a fresh CreateTaskCommand.
		if w.publishCreate(ctx, pr, taskIDStr, details) {
			// Update cursorState in-place so duplicate PR entries in the same poll batch
			// are deduplicated without a second create publish.
			cursorState.HeadSHAs[taskIDStr] = details.HeadSHA
			newHeadSHAs[taskIDStr] = details.HeadSHA
			if pr.UpdatedAt.After(maxUpdatedAt) {
				maxUpdatedAt = pr.UpdatedAt
			}
		}
	}

	cursorState.HeadSHAs = newHeadSHAs
	return maxUpdatedAt
}

func (w *watcher) publishCreate(
	ctx context.Context,
	pr PullRequest,
	taskIDStr string,
	details PRDetails,
) bool {
	author := pr.AuthorLogin

	trustResult, err := w.trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: author})
	if err != nil {
		glog.Errorf("trust check failed pr=%s err=%v", pr.HTMLURL, err)
		w.metrics.IncPRPublished("error")
		return false
	}

	var cmd task.CreateCommand
	if trustResult.Success() {
		cmd = task.CreateCommand{
			Title: computePRTitle(
				"github",
				pr.Owner,
				pr.Repo,
				pr.Number,
				details.HeadSHA,
				pr.Title,
				w.maxSlugLen,
				w.maxTitleLen,
				w.taskSuffix,
			),
			TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
			Frontmatter:    buildFrontmatter(pr, taskIDStr, w.stage, details),
			Body:           buildTaskBody(pr),
		}
	} else {
		if author == "" {
			author = "(unknown)"
		}
		glog.V(2).Infof("untrusted author=%q trust=%s pr=%s", author, trustResult.Description(), pr.HTMLURL)
		cmd = task.CreateCommand{
			Title:          computePRTitle("github", pr.Owner, pr.Repo, pr.Number, details.HeadSHA, pr.Title, w.maxSlugLen, w.maxTitleLen, w.taskSuffix),
			TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
			Frontmatter:    buildHumanReviewFrontmatter(pr, taskIDStr, w.stage, details),
			Body:           buildUntrustedBody(author, trustResult.Description()),
		}
	}

	if err := w.createSender.SendCommand(ctx, cmd); err != nil {
		glog.Errorf("publish create-task failed pr=%s err=%v", pr.HTMLURL, err)
		w.metrics.IncPRPublished("error")
		return false
	}
	glog.V(2).Infof("published CreateTaskCommand pr=%s/%s#%d sha=%s taskID=%s trusted=%t",
		pr.Owner, pr.Repo, pr.Number, details.HeadSHA, taskIDStr, trustResult.Success())
	w.metrics.IncPRPublished("create")
	return true
}

func (w *watcher) fetchPRDetails(
	ctx context.Context,
	pr PullRequest,
	cache map[string]PRDetails,
) (PRDetails, error) {
	cacheKey := fmt.Sprintf("%s/%s#%d", pr.Owner, pr.Repo, pr.Number)
	if details, ok := cache[cacheKey]; ok {
		return details, nil
	}
	details, err := w.ghClient.GetPRDetails(ctx, pr.Owner, pr.Repo, pr.Number)
	if err != nil {
		return PRDetails{}, errors.Wrapf(
			ctx,
			err,
			"get pr details pr=%s/%s#%d",
			pr.Owner,
			pr.Repo,
			pr.Number,
		)
	}
	cache[cacheKey] = details
	return details, nil
}

func buildTaskBody(pr PullRequest) string {
	repoLink := fmt.Sprintf("https://github.com/%s/%s", pr.Owner, pr.Repo)
	return fmt.Sprintf(
		"# PR Review: %s\n\n%s\n\n**Repo:** [%s/%s](%s)\n",
		pr.Title,
		pr.HTMLURL,
		pr.Owner,
		pr.Repo,
		repoLink,
	)
}

func buildFrontmatter(
	pr PullRequest,
	taskIDStr, stage string,
	details PRDetails,
) agentlib.TaskFrontmatter {
	return agentlib.TaskFrontmatter{
		"task_type":       "pr-review",
		"assignee":        "pr-reviewer-agent",
		"phase":           "planning",
		"status":          "in_progress",
		"stage":           stage,
		"task_identifier": taskIDStr,
		"title":           pr.Title,
		"clone_url":       details.CloneURL,
		"ref":             details.HeadSHA,
		"base_ref":        details.BaseRef,
	}
}

func buildHumanReviewFrontmatter(
	pr PullRequest,
	taskIDStr, stage string,
	details PRDetails,
) agentlib.TaskFrontmatter {
	return agentlib.TaskFrontmatter{
		"task_type":       "pr-review",
		"assignee":        "",
		"phase":           "human_review",
		"status":          "todo",
		"stage":           stage,
		"task_identifier": taskIDStr,
		"title":           pr.Title,
		"clone_url":       details.CloneURL,
		"ref":             details.HeadSHA,
		"base_ref":        details.BaseRef,
	}
}

func buildUntrustedBody(author, reasons string) string {
	return fmt.Sprintf(
		"## Untrusted author\n\nThis PR is by GitHub user **%s** which did not pass the trust check:\n\n- %s\n\nTo auto-process this PR, edit the frontmatter above:\n- `phase: in_progress`\n- `status: in_progress`\n\nTo dismiss, set `status: aborted`.\n",
		author,
		reasons,
	)
}
