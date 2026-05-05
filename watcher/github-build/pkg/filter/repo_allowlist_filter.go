// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter

import (
	"context"
	"regexp"
	"strings"

	"github.com/bborbe/errors"
)

// repoAllowlistEntryPattern validates a single repo entry: owner/repo (two slash-delimited segments).
var repoAllowlistEntryPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`)

// ParseRepoAllowlist parses a comma-separated allowlist string into a slice of validated
// "owner/repo" keys. Empty string returns (nil, nil). Whitespace-only entries and trailing
// commas are silently dropped. Any entry not matching the required shape causes an error.
func ParseRepoAllowlist(ctx context.Context, raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var result []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !repoAllowlistEntryPattern.MatchString(entry) {
			return nil, errors.Errorf(
				ctx,
				"repo allowlist entry %q does not match required format owner/repo (pattern: ^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$)",
				entry,
			)
		}
		result = append(result, entry)
	}
	return result, nil
}

// NewRepoAllowlistFilter returns a RepoFilter that skips repos not in the allowlist.
// An empty allowlist never skips (allow-all).
func NewRepoAllowlistFilter(allowlist []string) RepoFilter {
	return &repoAllowlistFilter{allowlist: allowlist}
}

type repoAllowlistFilter struct {
	allowlist []string
}

func (f *repoAllowlistFilter) Skip(repoKey string) bool {
	if len(f.allowlist) == 0 {
		return false
	}
	for _, entry := range f.allowlist {
		if repoKey == entry {
			return false
		}
	}
	return true
}
