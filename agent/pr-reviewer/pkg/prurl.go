// Copyright (c) 2025 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/bborbe/errors"
)

// Platform represents the type of PR platform.
type Platform string

const (
	// PlatformGitHub represents GitHub.
	PlatformGitHub Platform = "github"
	// PlatformBitbucket represents Bitbucket Server.
	PlatformBitbucket Platform = "bitbucket"
)

// PRInfo contains parsed information from a pull request URL.
type PRInfo struct {
	Platform Platform
	Host     string
	Owner    string // GitHub owner or Bitbucket project (used as owner equivalent)
	Project  string // Bitbucket project (same as Owner for Bitbucket, empty for GitHub)
	Repo     string
	Number   int
	RepoURL  string
}

// ParsePRURL parses a PR URL and extracts platform, host, owner/project, repo, PR number, and repo URL.
// Supports:
//   - GitHub: https://github.com/{owner}/{repo}/pull/{number}
//   - Bitbucket Server: https://{host}/projects/{project}/repos/{repo}/pull-requests/{number}[/overview]
//
// Returns error for unrecognized URL patterns or non-HTTPS URLs.
func ParsePRURL(ctx context.Context, rawURL string) (*PRInfo, error) {
	if rawURL == "" {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	if u.Scheme != "https" {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	// Detect platform by host
	if u.Host == "github.com" {
		return parseGitHub(ctx, rawURL, u)
	}

	// Try Bitbucket Server pattern (any non-github.com host with /projects/ path)
	if strings.Contains(u.Path, "/projects/") {
		return parseBitbucket(ctx, rawURL, u)
	}

	return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
}

// parseGitHub parses a GitHub PR URL.
// Expected format: https://github.com/{owner}/{repo}/pull/{number}
func parseGitHub(ctx context.Context, rawURL string, u *url.URL) (*PRInfo, error) {
	// Parse path: /{owner}/{repo}/pull/{number}
	path := strings.TrimSuffix(u.Path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(parts) != 4 {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	owner := parts[0]
	repo := parts[1]
	pullKeyword := parts[2]
	numberStr := parts[3]

	if owner == "" || repo == "" {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	if pullKeyword != "pull" {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	repoURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)

	return &PRInfo{
		Platform: PlatformGitHub,
		Host:     "github.com",
		Owner:    owner,
		Project:  "", // GitHub doesn't use projects
		Repo:     repo,
		Number:   number,
		RepoURL:  repoURL,
	}, nil
}

// parseBitbucket parses a Bitbucket Server PR URL.
// Expected format: https://{host}/projects/{project}/repos/{repo}/pull-requests/{number}[/overview]
func parseBitbucket(ctx context.Context, rawURL string, u *url.URL) (*PRInfo, error) {
	// Parse path: /projects/{project}/repos/{repo}/pull-requests/{number}[/overview]
	path := strings.TrimSuffix(u.Path, "/")
	path = strings.TrimSuffix(path, "/overview") // Remove optional /overview suffix
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Expected: [projects, {project}, repos, {repo}, pull-requests, {number}]
	if len(parts) != 6 {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	if parts[0] != "projects" {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	project := parts[1]

	if parts[2] != "repos" {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	repo := parts[3]

	if parts[4] != "pull-requests" {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	numberStr := parts[5]

	if project == "" || repo == "" {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return nil, errors.Errorf(ctx, "unsupported URL format: %s", rawURL)
	}

	repoURL := fmt.Sprintf("https://%s/projects/%s/repos/%s", u.Host, project, repo)

	return &PRInfo{
		Platform: PlatformBitbucket,
		Host:     u.Host,
		Owner:    project, // Use project as owner equivalent
		Project:  project,
		Repo:     repo,
		Number:   number,
		RepoURL:  repoURL,
	}, nil
}
