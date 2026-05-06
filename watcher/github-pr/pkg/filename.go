// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
)

// maxFilenameHintLen is the maximum byte length of a filename_hint value.
// Hints that exceed this limit are truncated with a WARN log.
const maxFilenameHintLen = 200

// maxSlugLen is the maximum character length of the slugified PR-title segment.
const maxSlugLen = 50

// computePRFilenameHint returns the human-readable filename hint for a PR-review task.
// Format (with slug): "PR Review {provider} - {owner}-{repo} - {number} - {slug}"
// Format (empty slug): "PR Review {provider} - {owner}-{repo} - {number}"
// The returned string MUST NOT include the .md extension; the controller appends it.
func computePRFilenameHint(provider, owner, repo string, number int, title string) string {
	base := fmt.Sprintf("PR Review %s - %s-%s - %d", provider, owner, repo, number)
	slug := slugifyTitle(title)
	var hint string
	if slug == "" {
		hint = base
	} else {
		hint = base + " - " + slug
	}
	if len(hint) > maxFilenameHintLen {
		glog.Warningf(
			"filename_hint exceeds max length: len=%d max=%d — truncating",
			len(hint),
			maxFilenameHintLen,
		)
		hint = hint[:maxFilenameHintLen]
	}
	return hint
}

// slugifyTitle converts a PR title to a filesystem-safe, human-readable slug.
// Rules (applied in order):
// 1. Lowercase the entire input
// 2. Replace any character that is not [a-z0-9] with a hyphen
// 3. Collapse consecutive hyphens into a single hyphen
// 4. Trim leading and trailing hyphens
// 5. Truncate to maxSlugLen (50) characters; trim any trailing hyphen left by truncation
// Returns empty string if the result after step 4 is empty (e.g. unicode-only or whitespace-only title).
func slugifyTitle(title string) string {
	lower := strings.ToLower(title)
	var b strings.Builder
	prevHyphen := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > maxSlugLen {
		result = result[:maxSlugLen]
		result = strings.TrimRight(result, "-")
	}
	return result
}
