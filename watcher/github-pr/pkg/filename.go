// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
)

// DefaultMaxTitleLen is the default safety cap for the whole title, including segments and separators.
// Crosses Windows MAX_PATH=260 and ext4 NAME_MAX=255 with margin. Override via MAX_TITLE_LEN.
const DefaultMaxTitleLen = 200

// DefaultMaxSlugLen is the default cap for the slugified PR-title segment alone.
// Bumped from 50 to 80 (2026-05-08) — 50 cut typical PR titles mid-word. Override via MAX_SLUG_LEN.
const DefaultMaxSlugLen = 80

// computePRTitle returns the human-readable title for a PR-review task.
// maxSlug caps the slug segment alone; maxTitle is a safety cap on the full title.
// Both are passed by the caller (read from env at startup) — see watcher/github-pr/main.go.
func computePRTitle(
	provider, owner, repo string,
	number int,
	title string,
	maxSlug, maxTitle int,
) string {
	base := fmt.Sprintf("PR Review %s - %s-%s - %d", provider, owner, repo, number)
	slug := slugifyTitle(title, maxSlug)
	var t string
	if slug == "" {
		t = base
	} else {
		t = base + " - " + slug
	}
	if len(t) > maxTitle {
		glog.Warningf(
			"PR title exceeds max length: len=%d max=%d — truncating",
			len(t),
			maxTitle,
		)
		t = t[:maxTitle]
	}
	return t
}

// slugifyTitle converts a PR title to a filesystem-safe, human-readable slug.
// Rules (applied in order):
// 1. Lowercase the entire input
// 2. Replace any character that is not [a-z0-9] with a hyphen
// 3. Collapse consecutive hyphens into a single hyphen
// 4. Trim leading and trailing hyphens
// 5. Truncate to maxSlug characters; trim any trailing hyphen left by truncation
// Returns empty string if the result after step 4 is empty (e.g. unicode-only or whitespace-only title).
func slugifyTitle(title string, maxSlug int) string {
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
	if len(result) > maxSlug {
		result = result[:maxSlug]
		result = strings.TrimRight(result, "-")
	}
	return result
}
