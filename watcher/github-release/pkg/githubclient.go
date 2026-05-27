// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	stderrors "errors"
	"net/http"

	"github.com/bborbe/errors"
	gogithub "github.com/google/go-github/v84/github"
	"gopkg.in/yaml.v3"
)

// ErrRateLimited is returned when the GitHub API responds with a rate-limit
// or abuse-rate-limit error.
var ErrRateLimited = stderrors.New("github rate limited")

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
func NewGitHubClient(httpClient *http.Client) GitHubClient {
	return &githubClient{client: gogithub.NewClient(httpClient)}
}

type githubClient struct {
	client *gogithub.Client
}

func (c *githubClient) ListRepos(ctx context.Context, owner string) ([]Repo, error) {
	user, _, err := c.client.Users.Get(ctx, owner)
	if err != nil {
		return nil, c.wrapRateLimitErr(ctx, err, "get user %s", owner)
	}
	isOrg := user.GetType() == "Organization"
	return c.listOwnerReposPaginated(ctx, owner, isOrg)
}

func (c *githubClient) listOwnerReposPaginated(
	ctx context.Context,
	owner string,
	isOrg bool,
) ([]Repo, error) {
	repos := make([]Repo, 0, 32)
	page := 1
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		repoPage, resp, err := c.fetchRepoPage(ctx, owner, isOrg, page)
		if err != nil {
			return nil, c.wrapRateLimitErr(ctx, err, "list repos for %s page=%d", owner, page)
		}
		repos = append(repos, filterRepos(repoPage)...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return repos, nil
}

func (c *githubClient) fetchRepoPage(
	ctx context.Context,
	owner string,
	isOrg bool,
	page int,
) ([]*gogithub.Repository, *gogithub.Response, error) {
	if isOrg {
		opts := &gogithub.RepositoryListByOrgOptions{
			ListOptions: gogithub.ListOptions{PerPage: 100, Page: page},
		}
		return c.client.Repositories.ListByOrg(ctx, owner, opts)
	}
	opts := &gogithub.RepositoryListByUserOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100, Page: page},
	}
	return c.client.Repositories.ListByUser(ctx, owner, opts)
}

func filterRepos(repos []*gogithub.Repository) []Repo {
	var result []Repo
	for _, repo := range repos {
		if repo.GetArchived() || repo.GetFork() {
			continue
		}
		name := repo.GetName()
		if name == "" {
			continue
		}
		result = append(result, Repo{
			Owner:         repo.GetOwner().GetLogin(),
			Name:          name,
			DefaultBranch: repo.GetDefaultBranch(),
		})
	}
	return result
}

func (c *githubClient) wrapRateLimitErr(
	ctx context.Context,
	err error,
	msg string,
	args ...interface{},
) error {
	var rl *gogithub.RateLimitError
	var arl *gogithub.AbuseRateLimitError
	if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
		return ErrRateLimited
	}
	return errors.Wrapf(ctx, err, msg, args...)
}

func (c *githubClient) GetMasterSHA(ctx context.Context, repo Repo) (string, error) {
	if repo.DefaultBranch == "" {
		return "", errors.Errorf(
			ctx,
			"repo %s/%s has empty DefaultBranch — cannot fetch HEAD SHA",
			repo.Owner,
			repo.Name,
		)
	}
	branch, _, err := c.client.Repositories.GetBranch(
		ctx,
		repo.Owner,
		repo.Name,
		repo.DefaultBranch,
		0,
	)
	if err != nil {
		return "", c.wrapRateLimitErr(
			ctx,
			err,
			"get branch %s/%s@%s",
			repo.Owner,
			repo.Name,
			repo.DefaultBranch,
		)
	}
	return branch.GetCommit().GetSHA(), nil
}

func (c *githubClient) GetChangelogContent(ctx context.Context, repo Repo) ([]byte, error) {
	opts := &gogithub.RepositoryContentGetOptions{Ref: repo.DefaultBranch}
	fileContent, _, _, err := c.client.Repositories.GetContents(
		ctx,
		repo.Owner,
		repo.Name,
		"CHANGELOG.md",
		opts,
	)
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
		return nil, errors.Wrapf(
			ctx,
			err,
			"get CHANGELOG.md %s/%s@%s",
			repo.Owner,
			repo.Name,
			repo.DefaultBranch,
		)
	}
	if fileContent == nil {
		return nil, nil
	}
	if fileContent.GetSize() > 1024*1024 {
		return nil, errors.Errorf(
			ctx,
			"CHANGELOG.md %s/%s too large: %d bytes (max 1 MiB)",
			repo.Owner,
			repo.Name,
			fileContent.GetSize(),
		)
	}
	decoded, err := fileContent.GetContent()
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "decode CHANGELOG.md %s/%s", repo.Owner, repo.Name)
	}
	return []byte(decoded), nil
}

func (c *githubClient) GetAutoReleaseConfig(ctx context.Context, repo Repo) (bool, error) {
	opts := &gogithub.RepositoryContentGetOptions{Ref: repo.DefaultBranch}
	fileContent, _, _, err := c.client.Repositories.GetContents(
		ctx,
		repo.Owner,
		repo.Name,
		".dark-factory/config.yml",
		opts,
	)
	if err != nil {
		var ghErr *gogithub.ErrorResponse
		if stderrors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusNotFound {
			return false, nil
		}
		var rl *gogithub.RateLimitError
		var arl *gogithub.AbuseRateLimitError
		if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
			return false, ErrRateLimited
		}
		return false, errors.Wrapf(
			ctx,
			err,
			"get .dark-factory/config.yml %s/%s@%s",
			repo.Owner,
			repo.Name,
			repo.DefaultBranch,
		)
	}
	if fileContent == nil {
		return false, nil
	}
	decoded, err := fileContent.GetContent()
	if err != nil {
		return false, errors.Wrapf(
			ctx,
			err,
			"decode .dark-factory/config.yml %s/%s",
			repo.Owner,
			repo.Name,
		)
	}
	var cfg darkFactoryConfig
	if err := yaml.Unmarshal([]byte(decoded), &cfg); err != nil {
		return false, errors.Wrapf(
			ctx,
			err,
			"parse .dark-factory/config.yml %s/%s",
			repo.Owner,
			repo.Name,
		)
	}
	return cfg.AutoRelease, nil
}

type darkFactoryConfig struct {
	AutoRelease bool `yaml:"autoRelease"`
}
