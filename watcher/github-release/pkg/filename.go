// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import "fmt"

// ComputeTaskTitle returns the human-readable task title used by the
// CreateTaskCommand frontmatter `title`.
//
// Format (mirrors the Phase 1 prototype frontmatter output verbatim so the
// FROZEN contract per [[Agent Task File Contract]] carries):
//
//	Release <owner>/<repo> at <sha[:7]>
//
// Examples:
//   - "Release bborbe/disk-status at 102b3b1"
//   - "Release bborbe/docker-utils at d630ef3"
//
// Ground truth: Phase 1 vault file `24 Tasks/Release bborbe-docker-utils d630ef3.md`
// (frontmatter `title:` field, line 8) reads `Release bborbe/docker-utils at d630ef3`.
// The controller's title→filename slug step replaces `/` with `-` and strips ` at `
// so the on-disk filename becomes `Release bborbe-docker-utils d630ef3.md` — that's
// the controller's transform, NOT the title-field value the watcher emits.
//
// Reference: watcher/github-pr/pkg/filename.go ComputeTitle (PR-shaped variant
// with maxSlugLen, maxTitleLen, taskSuffix knobs). Release titles are short and
// deterministic — those knobs are not needed here.
func ComputeTaskTitle(release Release) string {
	return fmt.Sprintf("Release %s/%s at %s", release.Repo.Owner, release.Repo.Name, release.ShortSHA())
}
