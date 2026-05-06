// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/errors"
	"github.com/golang/glog"
	"github.com/google/uuid"

	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/maintenance"
)

//counterfeiter:generate -o mocks/watcher.go --fake-name Watcher . Watcher

// Watcher polls GitHub Actions for build status changes.
type Watcher interface {
	Poll(ctx context.Context) error
}

// NewWatcher returns a Watcher that polls GitHub Actions and publishes commands.
func NewWatcher(
	githubClient GitHubClient,
	publisher CommandPublisher,
	metrics Metrics,
	repoFilter filter.RepoFilter,
	allowlist []string,
	cursorPath string,
	assignee string,
	taskStatus string,
	taskPhase string,
	maintenanceLoader maintenance.Loader,
) Watcher {
	return &buildWatcher{
		githubClient:      githubClient,
		publisher:         publisher,
		metrics:           metrics,
		repoFilter:        repoFilter,
		allowlist:         allowlist,
		cursorPath:        cursorPath,
		assignee:          assignee,
		taskStatus:        taskStatus,
		taskPhase:         taskPhase,
		maintenanceLoader: maintenanceLoader,
	}
}

type buildWatcher struct {
	githubClient      GitHubClient
	publisher         CommandPublisher
	metrics           Metrics
	repoFilter        filter.RepoFilter
	allowlist         []string
	cursorPath        string
	assignee          string
	taskStatus        string
	taskPhase         string
	maintenanceLoader maintenance.Loader
}

func (w *buildWatcher) Poll(ctx context.Context) error {
	cursor, err := LoadCursor(ctx, w.cursorPath)
	if err != nil {
		return errors.Wrapf(ctx, err, "load cursor")
	}

	for _, repoKey := range w.allowlist {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if w.repoFilter.Skip(repoKey) {
			glog.V(3).Infof("skipping repo=%s reason=filtered", repoKey)
			continue
		}

		if rateLimited := w.pollRepo(ctx, cursor, repoKey); rateLimited {
			break
		}
	}

	var redCount float64
	for _, state := range cursor.Repos {
		if state.LastKnownState == "red" {
			redCount++
		}
	}
	w.metrics.SetCurrentRedRepos(redCount)

	if err := SaveCursor(ctx, w.cursorPath, cursor); err != nil {
		glog.Warningf("cursor save failed: %v", err)
	}

	w.metrics.IncPollCycle("success")
	return nil
}

// pollRepo processes one repo. Returns true when the outer loop should break (rate-limited).
func (w *buildWatcher) pollRepo(ctx context.Context, cursor *Cursor, repoKey string) bool {
	owner, repo := splitRepoKey(repoKey)
	repoState := GetOrCreateRepoState(cursor, repoKey)

	if repoState.DefaultBranch == "" {
		branch, err := w.githubClient.GetDefaultBranch(ctx, owner, repo)
		if err != nil {
			glog.Warningf("get default branch failed repo=%s err=%v", repoKey, err)
			w.metrics.IncPollError("github_error")
			return false
		}
		repoState.DefaultBranch = branch
	}

	runs, err := w.githubClient.GetWorkflowRuns(ctx, owner, repo, repoState.DefaultBranch)
	if err != nil {
		if err == ErrRateLimited {
			w.metrics.IncPollError("rate_limited")
			return true
		}
		glog.Warningf("get workflow runs failed repo=%s err=%v", repoKey, err)
		w.metrics.IncPollError("github_error")
		return false
	}
	w.metrics.IncReposChecked()

	currState, episodeSHA, failingRuns := deriveState(runs)
	if currState == "undefined" {
		return false
	}

	w.applyStateMachine(ctx, repoKey, repoState, currState, episodeSHA, failingRuns, owner, repo)
	return false
}

// applyStateMachine applies the green/red state machine for a single repo.
func (w *buildWatcher) applyStateMachine(
	ctx context.Context,
	repoKey string,
	repoState *RepoState,
	currState, episodeSHA string,
	failingRuns []WorkflowRun,
	owner, repo string,
) {
	prevState := repoState.LastKnownState

	switch {
	case (prevState == "" || prevState == "green") && currState == "red":
		overrides := w.maintenanceLoader.LoadOverrides(ctx, owner, repo, repoState.DefaultBranch)
		effectiveAssignee := coalesceString(overrides.Assignee, w.assignee)
		effectiveStatus := coalesceString(overrides.Status, w.taskStatus)
		effectivePhase := coalesceString(overrides.Phase, w.taskPhase)
		taskID := DeriveTaskID(owner, repo, episodeSHA)
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
		if err := w.publisher.PublishCreate(ctx, cmd); err != nil {
			glog.Errorf("publish create-task failed repo=%s err=%v", repoKey, err)
			w.metrics.IncPollError("kafka_error")
			return // do NOT update cursor — next poll retries
		}
		w.metrics.IncTaskPublished()
		w.metrics.IncStateTransition("green_to_red")
		repoState.LastKnownState = "red"
		repoState.CurrentEpisodeSHA = episodeSHA

	case prevState == "red" && currState == "red":
		// Episode locked on first red; skip regardless of SHA change

	case prevState == "red" && currState == "green":
		w.metrics.IncStateTransition("red_to_green")
		repoState.LastKnownState = "green"
		repoState.CurrentEpisodeSHA = ""

	default:
		// (prevState == "" || prevState == "green") && currState == "green": no transition
	}
}

// deriveState computes the current build state for a repo from its workflow runs.
// Returns state ("green"|"red"|"undefined"), episodeSHA (only when red), and the failing runs.
func deriveState(runs []WorkflowRun) (state string, episodeSHA string, failingRuns []WorkflowRun) {
	// Group by WorkflowID, keep only the latest run per workflow (by CreatedAt desc)
	latestByWorkflow := make(map[int64]WorkflowRun)
	for _, run := range runs {
		existing, ok := latestByWorkflow[run.WorkflowID]
		if !ok || run.CreatedAt.After(existing.CreatedAt) {
			latestByWorkflow[run.WorkflowID] = run
		}
	}

	// Filter: only "failure" or "success" conclusions
	var considered []WorkflowRun
	for _, run := range latestByWorkflow {
		if run.Conclusion == "failure" || run.Conclusion == "success" {
			considered = append(considered, run)
		}
	}

	if len(considered) == 0 {
		return "undefined", "", nil
	}

	for _, run := range considered {
		if run.Conclusion == "failure" {
			failingRuns = append(failingRuns, run)
		}
	}

	if len(failingRuns) == 0 {
		return "green", "", nil
	}

	// Episode SHA = HeadSHA of the earliest (smallest CreatedAt) failing run
	sort.Slice(failingRuns, func(i, j int) bool {
		return failingRuns[i].CreatedAt.Before(failingRuns[j].CreatedAt)
	})
	episodeSHA = failingRuns[0].HeadSHA

	return "red", episodeSHA, failingRuns
}

// splitRepoKey extracts owner and repo from an allowlist entry.
// Accepts both "host/owner/repo" (3 segments — the host is dropped, matches
// ParseRepoAllowlist output) and "owner/repo" (2 segments). Anything else
// returns the original key with an empty repo so the caller can skip it.
func splitRepoKey(key string) (owner, repo string) {
	parts := strings.Split(key, "/")
	switch len(parts) {
	case 3:
		return parts[1], parts[2]
	case 2:
		return parts[0], parts[1]
	default:
		return key, ""
	}
}

// buildCreateTaskCommand constructs a CreateTaskCommand for a build failure episode.
func (w *buildWatcher) buildCreateTaskCommand(
	ctx context.Context,
	taskID uuid.UUID,
	owner, repo, episodeSHA string,
	failingRuns []WorkflowRun,
	assignee, taskStatus, taskPhase string,
) WatcherCreateTaskCommand {
	firstRun := failingRuns[0]
	lines := make([]string, 0, 12+len(failingRuns))
	lines = append(lines, fmt.Sprintf("# Build Failure: %s/%s", owner, repo), "")

	if firstRun.DisplayTitle != "" {
		lines = append(lines, fmt.Sprintf("**Commit:** %s", firstRun.DisplayTitle))
	}
	if firstRun.HeadBranch != "" {
		lines = append(lines, fmt.Sprintf("**Branch:** %s", firstRun.HeadBranch))
	}
	if firstRun.Event != "" {
		lines = append(lines, fmt.Sprintf("**Event:** %s", firstRun.Event))
	}
	if !firstRun.StartedAt.IsZero() {
		lines = append(
			lines,
			fmt.Sprintf("**Started:** %s", firstRun.StartedAt.UTC().Format(time.RFC3339)),
		)
	}
	if !firstRun.UpdatedAt.IsZero() {
		lines = append(
			lines,
			fmt.Sprintf("**Finished:** %s", firstRun.UpdatedAt.UTC().Format(time.RFC3339)),
		)
	}
	if !firstRun.StartedAt.IsZero() && !firstRun.UpdatedAt.IsZero() {
		if d := formatDuration(firstRun.UpdatedAt.Sub(firstRun.StartedAt)); d != "" {
			lines = append(lines, fmt.Sprintf("**Duration:** %s", d))
		}
	}

	lines = append(lines,
		"",
		fmt.Sprintf("Episode SHA: `%s`", episodeSHA),
		"",
		"## Failing Workflows",
		"",
	)

	lines = append(lines,
		"| Workflow | Job | Failed Step | Run |",
		"|---|---|---|---|",
	)

	for _, run := range failingRuns {
		jobName, stepName := w.jobPlaceholders(ctx, owner, repo, run.RunID)
		lines = append(lines, fmt.Sprintf("| %s | %s | %s | [Run](%s) |",
			run.Name, jobName, stepName, run.HTMLURL))
	}
	body := strings.Join(lines, "\n") + "\n"

	fm := agentlib.TaskFrontmatter{
		"assignee":    assignee,
		"repo":        owner + "/" + repo,
		"episode_sha": episodeSHA,
		"status":      taskStatus,
	}
	if taskPhase != "" {
		fm["phase"] = taskPhase
	}
	return WatcherCreateTaskCommand{
		CreateTaskCommand: agentlib.CreateTaskCommand{
			TaskIdentifier: agentlib.TaskIdentifier(taskID.String()),
			Frontmatter:    fm,
			Body:           body,
		},
		FilenameHint: computeFilenameHint("github", owner, repo, episodeSHA),
	}
}

// jobPlaceholders returns (jobName, stepName) for a failing run.
// Returns ("?", "?") when the jobs API is unavailable or returns no failed jobs.
func (w *buildWatcher) jobPlaceholders(
	ctx context.Context,
	owner, repo string,
	runID int64,
) (jobName, stepName string) {
	jobName, stepName = "?", "?"
	if runID == 0 {
		return
	}
	jobs, err := w.githubClient.GetJobsForRun(ctx, owner, repo, runID)
	if err != nil {
		glog.Warningf(
			"jobs API failed run=%d repo=%s/%s err=%v — using ? placeholders",
			runID,
			owner,
			repo,
			err,
		)
		return
	}
	if len(jobs) == 0 {
		return
	}
	jobName = jobs[0].JobName
	if jobs[0].FailedStepName != "" {
		stepName = jobs[0].FailedStepName
	}
	return
}

// coalesceString returns the first non-empty string. Used to merge a
// per-repo file override (a) with the watcher-level default (b).
func coalesceString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// formatDuration formats d as a human-readable string for the task body header.
// Returns "" when d ≤ 0 so callers can omit the Duration line for zero timestamps.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	d = d.Round(time.Second)
	if d <= 0 {
		return ""
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
