// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

// ChangelogSummary is the parsed result of reading a CHANGELOG.md.
//
// Populated by ParseChangelog. The fields drive:
//   - UnreleasedBullets → empty_unreleased_filter (skip if 0)
//   - UnreleasedIsFirst → planning-phase precondition (the agent enforces this;
//     watcher does not skip on it — the agent's escalation path is better
//     placement for that operator feedback)
//   - LatestVersion → Release.CurrentVersion (semver base for next bump)
type ChangelogSummary struct {
	UnreleasedBullets int    // count of "^- " lines under "## Unreleased"
	UnreleasedIsFirst bool   // true if "## Unreleased" is the first "## " heading
	LatestVersion     string // first "## vX.Y.Z" or "## X.Y.Z" header found; "" if none
}

// ParseChangelog parses a CHANGELOG.md byte slice into a ChangelogSummary.
//
// Behaviour:
//   - Counts "^- " bullet lines directly under "## Unreleased" until next "## " heading
//   - Determines whether "## Unreleased" is the first "## " heading (preamble lines like
//     "# Changelog" don't count — only "## " level)
//   - Extracts the first "## X.Y.Z" or "## vX.Y.Z" heading as LatestVersion
//
// Implementation reference: the /github-unreleased-repo-watcher slash command (Phase 1
// prototype) does the equivalent in bash via awk + grep. The Go port lives here so it
// is unit-testable.
//
// TODO: implement.
func ParseChangelog(_ []byte) ChangelogSummary {
	return ChangelogSummary{}
}
