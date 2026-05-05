// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/bborbe/errors"
	gogithub "github.com/google/go-github/v62/github"
)

// ErrRateLimited is returned when the GitHub API responds with a rate-limit or
// abuse-rate-limit error.
var ErrRateLimited = stderrors.New("github rate limited")

// WorkflowRun holds the fields the watcher needs from a GitHub Actions workflow run.
type WorkflowRun struct {
	WorkflowID int64
	Name       string
	HeadSHA    string
	Conclusion string
	HTMLURL    string
	CreatedAt  time.Time
}

//counterfeiter:generate -o mocks/github_client.go --fake-name GitHubClient . GitHubClient

// GitHubClient abstracts the GitHub Actions API calls.
type GitHubClient interface {
	// GetWorkflowRuns returns completed workflow runs for a repo branch.
	// In-progress runs (empty Conclusion) are filtered out.
	GetWorkflowRuns(ctx context.Context, owner, repo, branch string) ([]WorkflowRun, error)

	// GetDefaultBranch returns the default branch name for a repository.
	GetDefaultBranch(ctx context.Context, owner, repo string) (string, error)
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
			WorkflowID: run.GetWorkflowID(),
			Name:       run.GetName(),
			HeadSHA:    run.GetHeadSHA(),
			Conclusion: run.GetConclusion(),
			HTMLURL:    run.GetHTMLURL(),
			CreatedAt:  createdAt,
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
