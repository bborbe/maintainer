// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	stderrors "errors"
	"net/http"
	"time"

	"github.com/bborbe/errors"
	gogithub "github.com/google/go-github/v62/github"
)

// ErrRateLimited is returned when the GitHub API responds with a rate-limit or
// abuse-rate-limit error.
var ErrRateLimited = stderrors.New("github rate limited")

// WorkflowRun holds the fields the watcher needs from a GitHub Actions workflow run.
type WorkflowRun struct {
	WorkflowID   int64
	RunID        int64 // run instance ID — used by jobs API (GET /actions/runs/{id}/jobs)
	Name         string
	HeadSHA      string
	Conclusion   string
	HTMLURL      string
	CreatedAt    time.Time
	DisplayTitle string    // display_title: commit message shown in GitHub UI
	HeadBranch   string    // head_branch: branch that triggered the run
	Event        string    // event: push / pull_request / schedule / workflow_dispatch / etc.
	StartedAt    time.Time // run_started_at: when execution began (not queuing time)
	UpdatedAt    time.Time // updated_at: last status change — completion time for done runs
}

//counterfeiter:generate -o mocks/github_client.go --fake-name GitHubClient . GitHubClient

// GitHubClient abstracts the GitHub Actions API calls.
type GitHubClient interface {
	// GetWorkflowRuns returns completed workflow runs for a repo branch.
	// In-progress runs (empty Conclusion) are filtered out.
	GetWorkflowRuns(ctx context.Context, owner, repo, branch string) ([]WorkflowRun, error)

	// GetDefaultBranch returns the default branch name for a repository.
	GetDefaultBranch(ctx context.Context, owner, repo string) (string, error)

	// GetFileContent fetches the raw content of a file at the given ref.
	// Returns (nil, nil) if the file does not exist (HTTP 404 — the common case).
	// Returns (nil, ErrRateLimited) when rate-limited.
	// Returns (nil, err) for any other API error.
	GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
}

// NewGitHubClient returns a GitHubClient backed by the real GitHub API.
func NewGitHubClient(token string) GitHubClient {
	return &githubClient{
		client: gogithub.NewClient(nil).WithAuthToken(token),
	}
}

type githubClient struct {
	client *gogithub.Client
}

func (c *githubClient) GetWorkflowRuns(
	ctx context.Context,
	owner, repo, branch string,
) ([]WorkflowRun, error) {
	opts := &gogithub.ListWorkflowRunsOptions{
		Branch: branch,
		Status: "completed",
		ListOptions: gogithub.ListOptions{
			PerPage: 20,
		},
	}
	result, _, err := c.client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
	if err != nil {
		var rl *gogithub.RateLimitError
		var arl *gogithub.AbuseRateLimitError
		if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
			return nil, ErrRateLimited
		}
		return nil, errors.Wrapf(
			ctx,
			err,
			"list workflow runs %s/%s branch=%s",
			owner,
			repo,
			branch,
		)
	}

	runs := make([]WorkflowRun, 0, len(result.WorkflowRuns))
	for _, run := range result.WorkflowRuns {
		if run.GetConclusion() == "" {
			continue
		}
		var createdAt time.Time
		if run.CreatedAt != nil {
			createdAt = run.CreatedAt.Time
		}
		runs = append(runs, WorkflowRun{
			WorkflowID:   run.GetWorkflowID(),
			RunID:        run.GetID(),
			Name:         run.GetName(),
			HeadSHA:      run.GetHeadSHA(),
			Conclusion:   run.GetConclusion(),
			HTMLURL:      run.GetHTMLURL(),
			CreatedAt:    createdAt,
			DisplayTitle: run.GetDisplayTitle(),
			HeadBranch:   run.GetHeadBranch(),
			Event:        run.GetEvent(),
			StartedAt:    run.GetRunStartedAt().Time,
			UpdatedAt:    run.GetUpdatedAt().Time,
		})
	}
	return runs, nil
}

func (c *githubClient) GetDefaultBranch(
	ctx context.Context,
	owner, repo string,
) (string, error) {
	repository, _, err := c.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		var rl *gogithub.RateLimitError
		var arl *gogithub.AbuseRateLimitError
		if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
			return "", ErrRateLimited
		}
		return "", errors.Wrapf(ctx, err, "get repository %s/%s", owner, repo)
	}
	return repository.GetDefaultBranch(), nil
}

func (c *githubClient) GetFileContent(
	ctx context.Context,
	owner, repo, path, ref string,
) ([]byte, error) {
	opts := &gogithub.RepositoryContentGetOptions{Ref: ref}
	fileContent, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		var ghErr *gogithub.ErrorResponse
		if stderrors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		var rl *gogithub.RateLimitError
		var arl *gogithub.AbuseRateLimitError
		if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
			return nil, ErrRateLimited
		}
		return nil, errors.Wrapf(ctx, err, "get file content %s/%s/%s@%s", owner, repo, path, ref)
	}
	if fileContent == nil {
		return nil, nil
	}
	if fileContent.GetSize() > 1024*1024 {
		return nil, errors.Errorf(
			ctx,
			"file %s/%s/%s too large: %d bytes (max 1 MiB)",
			owner,
			repo,
			path,
			fileContent.GetSize(),
		)
	}
	decoded, err := fileContent.GetContent()
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "decode content %s/%s/%s", owner, repo, path)
	}
	return []byte(decoded), nil
}
