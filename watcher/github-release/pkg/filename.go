// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import "fmt"

// ComputeTaskTitle returns the human-readable task title used by both the
// CreateTaskCommand frontmatter `title` and the derived vault filename.
//
// Format (mirrors the Phase 1 prototype output exactly so the contract carries):
//
//	Release <owner>-<repo> <sha[:7]>
//
// Examples:
//   - "Release bborbe-disk-status 102b3b1"
//   - "Release bborbe-docker-utils d630ef3"
//
// The controller's title→filename slug step (per agent task/controller behaviour)
// produces "Release bborbe-disk-status 102b3b1.md" in {vault}/24 Tasks/.
//
// Reference: watcher/github-pr/pkg/filename.go ComputeTitle (PR-shaped variant
// with maxSlugLen, maxTitleLen, taskSuffix knobs). Release titles are short and
// deterministic — those knobs are not needed here.
func ComputeTaskTitle(release Release) string {
	return fmt.Sprintf("Release %s-%s %s", release.Repo.Owner, release.Repo.Name, release.ShortSHA())
}
