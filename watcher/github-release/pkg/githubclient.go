// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"net/http"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o mocks/github_client.go --fake-name GitHubClient . GitHubClient

// GitHubClient is the upstream-source surface for the release watcher.
// All methods are scoped to a single owner; the watcher iterates per-owner.
//
// Reference: watcher/github-pr/pkg/githubclient.go (uses SearchPRs + GetPRDetails);
// watcher/github-build/pkg/githubclient.go (uses ListWorkflowRuns + GetJobInfoForRun).
type GitHubClient interface {
	// ListRepos returns non-archived repositories owned by owner.
	// Pagination is internal; the returned slice is the full set.
	ListRepos(ctx context.Context, owner string) ([]Repo, error)

	// GetMasterSHA returns the full HEAD SHA of repo's default branch.
	GetMasterSHA(ctx context.Context, repo Repo) (string, error)

	// GetChangelogContent returns the raw bytes of CHANGELOG.md at HEAD of repo's
	// default branch. Returns (nil, nil) if the file does not exist (404).
	// Other errors propagate.
	GetChangelogContent(ctx context.Context, repo Repo) ([]byte, error)

	// GetAutoReleaseConfig returns whether the repo has dark-factory autoRelease
	// enabled (.dark-factory/config.yml: autoRelease: true). Returns (false, nil)
	// if the config file does not exist (the common case — autoRelease defaults off).
	GetAutoReleaseConfig(ctx context.Context, repo Repo) (bool, error)
}

// NewGitHubClient returns the production GitHubClient backed by the given HTTP client
// (typically authenticated via GitHub App installation token).
//
// TODO: implement (mirror watcher/github-pr/pkg/githubclient.go construction shape).
func NewGitHubClient(_ *http.Client) GitHubClient {
	return &githubClient{}
}

type githubClient struct{}

// ListRepos implements GitHubClient. TODO.
func (c *githubClient) ListRepos(ctx context.Context, _ string) ([]Repo, error) {
	return nil, errors.New(ctx, "github_client: ListRepos not implemented")
}

// GetMasterSHA implements GitHubClient. TODO.
func (c *githubClient) GetMasterSHA(ctx context.Context, _ Repo) (string, error) {
	return "", errors.New(ctx, "github_client: GetMasterSHA not implemented")
}

// GetChangelogContent implements GitHubClient. TODO.
func (c *githubClient) GetChangelogContent(ctx context.Context, _ Repo) ([]byte, error) {
	return nil, errors.New(ctx, "github_client: GetChangelogContent not implemented")
}

// GetAutoReleaseConfig implements GitHubClient. TODO.
func (c *githubClient) GetAutoReleaseConfig(ctx context.Context, _ Repo) (bool, error) {
	return false, errors.New(ctx, "github_client: GetAutoReleaseConfig not implemented")
}
