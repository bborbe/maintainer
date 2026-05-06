// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"strings"

	"github.com/golang/glog"
)

// maxFilenameHintLen is the maximum byte length of a filename_hint value.
// Hints that exceed this limit are truncated with a WARN log to prevent filesystem aliasing.
const maxFilenameHintLen = 200

// computeFilenameHint returns the human-readable filename hint for a build-failure task.
// Format: "Build Failure {provider} - {slugifySegment(owner)}-{slugifySegment(repo)} - {sha7}"
// The returned string MUST NOT include the .md extension; the controller appends it.
func computeFilenameHint(provider, owner, repo, episodeSHA string) string {
	sha7 := episodeSHA
	if len(sha7) > 7 {
		sha7 = sha7[:7]
	}
	ownerRepo := slugifySegment(owner) + "-" + slugifySegment(repo)
	hint := "Build Failure " + provider + " - " + ownerRepo + " - " + sha7
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

// slugifySegment converts s to a filesystem-safe lowercase segment.
// Non-[a-z0-9] characters (including uppercase letters) are replaced with hyphens;
// leading and trailing hyphens are stripped.
func slugifySegment(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
