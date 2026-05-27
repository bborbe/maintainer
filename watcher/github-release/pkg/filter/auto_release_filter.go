// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter

// NewAutoReleaseFilter skips Releases whose repo has dark-factory autoRelease
// enabled (`.dark-factory/config.yml: autoRelease: true`).
//
// Rationale: existing dark-factory autoRelease daemon handles those repos via
// post-merge rename + tag. github-releaser-agent is for repos that opted OUT of
// autoRelease (typically because branch protection blocks the existing daemon's
// post-merge commit). Dual-emission would race the daemons against each other.
//
// The Release.AutoRelease bool is sourced once by GitHubClient.GetAutoReleaseConfig.
func NewAutoReleaseFilter() TaskCreationFilter {
	return TaskCreationFilterFunc(func(release Release) bool {
		return release.AutoRelease
	})
}
