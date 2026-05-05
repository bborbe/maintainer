// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"sort"
	"strings"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/errors"
	"github.com/golang/glog"
	"github.com/google/uuid"

	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
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
) Watcher {
	return &buildWatcher{
		githubClient: githubClient,
		publisher:    publisher,
		metrics:      metrics,
		repoFilter:   repoFilter,
		allowlist:    allowlist,
		cursorPath:   cursorPath,
	}
}

type buildWatcher struct {
	githubClient GitHubClient
	publisher    CommandPublisher
	metrics      Metrics
	repoFilter   filter.RepoFilter
	allowlist    []string
	cursorPath   string
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
		taskID := DeriveTaskID(owner, repo, episodeSHA)
		cmd := buildCreateTaskCommand(taskID, owner, repo, episodeSHA, failingRuns)
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
func buildCreateTaskCommand(
	taskID uuid.UUID,
	owner, repo, episodeSHA string,
	failingRuns []WorkflowRun,
) agentlib.CreateTaskCommand {
	lines := make([]string, 0, 6+len(failingRuns))
	lines = append(lines,
		fmt.Sprintf("# Build Failure: %s/%s", owner, repo),
		"",
		fmt.Sprintf("Episode SHA: `%s`", episodeSHA),
		"",
		"## Failing Workflows",
		"",
	)
	for _, run := range failingRuns {
		lines = append(lines, fmt.Sprintf("- [%s](%s)", run.Name, run.HTMLURL))
	}
	body := strings.Join(lines, "\n") + "\n"

	return agentlib.CreateTaskCommand{
		TaskIdentifier: agentlib.TaskIdentifier(taskID.String()),
		Frontmatter: agentlib.TaskFrontmatter{
			"assignee":    "build-fixer-agent",
			"repo":        owner + "/" + repo,
			"episode_sha": episodeSHA,
			"status":      "todo",
		},
		Body: body,
	}
}
