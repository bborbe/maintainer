// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

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

var versionHeaderRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

func isVersionHeader(heading string) (string, bool) {
	versionText := heading[3:] // strip "## "
	if versionHeaderRe.MatchString(versionText) {
		return versionText, true
	}
	return "", false
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
func ParseChangelog(content []byte) ChangelogSummary {
	if len(content) == 0 {
		return ChangelogSummary{}
	}

	var inUnreleased bool
	var seenAnyH2 bool
	var unreleasedIsFirstH2 bool
	var unreleasedBullets int
	var latestVersion string

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()

		// Not a heading
		if !strings.HasPrefix(line, "## ") {
			if inUnreleased && strings.HasPrefix(line, "- ") {
				unreleasedBullets++
			}
			continue
		}

		// H2 heading
		heading := strings.TrimRight(line, " \t")
		if heading == "## Unreleased" {
			if !seenAnyH2 {
				unreleasedIsFirstH2 = true
			}
			inUnreleased = true
			seenAnyH2 = true
			continue
		}

		// Other heading
		inUnreleased = false
		seenAnyH2 = true
		if latestVersion == "" {
			if v, ok := isVersionHeader(heading); ok {
				latestVersion = v
			}
		}
	}

	return ChangelogSummary{
		UnreleasedBullets: unreleasedBullets,
		UnreleasedIsFirst: unreleasedIsFirstH2,
		LatestVersion:     latestVersion,
	}
}
