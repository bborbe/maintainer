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
	updateFrontmatterSender task.UpdateFrontmatterCommandSender,
	cursorPath string,
	startTime libtime.DateTime,
	scope string,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	metrics Metrics,
	trustDecision trust.Trust,
) Watcher {
	return &watcher{
		ghClient:                ghClient,
		createSender:            createSender,
		updateFrontmatterSender: updateFrontmatterSender,
		cursorPath:              cursorPath,
		startTime:               startTime,
		scope:                   scope,
		taskCreationFilter:      taskCreationFilter,
		stage:                   stage,
		metrics:                 metrics,
		trustDecision:           trustDecision,
	}
}

type watcher struct {
	ghClient                GitHubClient
	createSender            task.CreateCommandSender
	updateFrontmatterSender task.UpdateFrontmatterCommandSender
	cursorPath              string
	startTime               libtime.DateTime
	scope                   string
	taskCreationFilter      filter.TaskCreationFilter
	stage                   string
	metrics                 Metrics
	trustDecision           trust.Trust
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
		taskIDStr := DeriveTaskID(pr.Owner, pr.Repo, pr.Number).String()

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
			if known, ok := cursorState.HeadSHAs[taskIDStr]; ok {
				newHeadSHAs[taskIDStr] = known
			}
			continue
		}

		details, err := w.fetchPRDetails(ctx, pr, taskIDStr, prDetailsCache)
		if err != nil {
			glog.Errorf(
				"get pr details failed pr=%s/%s#%d err=%v",
				pr.Owner,
				pr.Repo,
				pr.Number,
				err,
			)
			if known, ok := cursorState.HeadSHAs[taskIDStr]; ok {
				newHeadSHAs[taskIDStr] = known
			}
			continue
		}

		if w.handlePR(ctx, cursorState, pr, taskIDStr, details) {
			newHeadSHAs[taskIDStr] = details.HeadSHA
			if pr.UpdatedAt.After(maxUpdatedAt) {
				maxUpdatedAt = pr.UpdatedAt
			}
		}
	}

	cursorState.HeadSHAs = newHeadSHAs
	return maxUpdatedAt
}

// handlePR processes a single PR: publishes commands, updates cursor SHA.
// Returns true if the PR was processed successfully (cursor should advance).
func (w *watcher) handlePR(
	ctx context.Context,
	cursorState *Cursor,
	pr PullRequest,
	taskIDStr string,
	details PRDetails,
) bool {
	knownSHA, exists := cursorState.HeadSHAs[taskIDStr]
	switch {
	case !exists:
		return w.publishCreate(ctx, cursorState, pr, taskIDStr, details)
	case knownSHA != details.HeadSHA:
		return w.publishForcePush(ctx, cursorState, pr, taskIDStr, knownSHA, details.HeadSHA)
	default:
		glog.V(3).
			Infof("no change, skipping pr=%s/%s#%d taskID=%s", pr.Owner, pr.Repo, pr.Number, taskIDStr)
		return true
	}
}

func (w *watcher) publishCreate(
	ctx context.Context,
	cursorState *Cursor,
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
			Title:          computePRTitle("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
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
			Title:          computePRTitle("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
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
	cursorState.HeadSHAs[taskIDStr] = details.HeadSHA
	glog.V(2).Infof("published CreateTaskCommand pr=%s/%s#%d taskID=%s trusted=%t",
		pr.Owner, pr.Repo, pr.Number, taskIDStr, trustResult.Success())
	w.metrics.IncPRPublished("create")
	return true
}

func (w *watcher) publishForcePush(
	ctx context.Context,
	cursorState *Cursor,
	pr PullRequest,
	taskIDStr, oldSHA, newSHA string,
) bool {
	author := pr.AuthorLogin

	trustResult, err := w.trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: author})
	if err != nil {
		glog.Errorf("trust check failed pr=%s err=%v", pr.HTMLURL, err)
		w.metrics.IncPRPublished("error")
		return false
	}

	heading := fmt.Sprintf("## Outdated by force-push %s", oldSHA)

	var updates agentlib.TaskFrontmatter
	var bodySection *task.BodySection

	if trustResult.Success() {
		updates = agentlib.TaskFrontmatter{
			"task_type":     "pr-review",
			"assignee":      "pr-reviewer-agent",
			"phase":         "planning",
			"status":        "in_progress",
			"trigger_count": 0,
		}
		bodySection = &task.BodySection{Heading: heading, Section: heading + "\n"}
	} else {
		if author == "" {
			author = "(unknown)"
		}
		glog.V(2).Infof("untrusted force-push author=%q trust=%s pr=%s", author, trustResult.Description(), pr.HTMLURL)
		updates = agentlib.TaskFrontmatter{
			"task_type":     "pr-review",
			"assignee":      "",
			"phase":         "human_review",
			"status":        "todo",
			"trigger_count": 0,
		}
		section := heading + "\n" + buildUntrustedBody(author, trustResult.Description())
		bodySection = &task.BodySection{Heading: heading, Section: section}
	}

	cmd := task.UpdateFrontmatterCommand{
		TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
		Updates:        updates,
		Body:           bodySection,
	}
	if err := w.updateFrontmatterSender.SendCommand(ctx, cmd); err != nil {
		glog.Errorf("publish update-frontmatter failed pr=%s err=%v", pr.HTMLURL, err)
		w.metrics.IncPRPublished("error")
		return false
	}
	cursorState.HeadSHAs[taskIDStr] = newSHA
	glog.V(2).Infof("published UpdateFrontmatterCommand pr=%s/%s#%d taskID=%s trusted=%t",
		pr.Owner, pr.Repo, pr.Number, taskIDStr, trustResult.Success())
	w.metrics.IncPRPublished("update_frontmatter")
	return true
}

func (w *watcher) fetchPRDetails(
	ctx context.Context,
	pr PullRequest,
	taskIDStr string,
	cache map[string]PRDetails,
) (PRDetails, error) {
	if details, ok := cache[taskIDStr]; ok {
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
	cache[taskIDStr] = details
	return details, nil
}

func buildTaskBody(pr PullRequest) string {
	return fmt.Sprintf("# PR Review: %s\n\n%s\n", pr.Title, pr.HTMLURL)
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
