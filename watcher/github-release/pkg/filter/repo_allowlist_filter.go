// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter

import (
	"context"
	"strings"

	repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"
)

// ParseRepoAllowlist parses a comma-separated allowlist string into a slice
// of host-qualified repo keys (e.g. "github.com/bborbe/disk-status").
// Whitespace trimmed; empty entries skipped. (nil, nil) on empty input (allow-all).
//
// Carried verbatim from watcher/github-pr — domain-agnostic.
func ParseRepoAllowlist(_ context.Context, raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var result []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			result = append(result, entry)
		}
	}
	return result, nil
}

// NewRepoAllowlistFilter returns a TaskCreationFilter that skips Releases whose
// RepoKey is not in the allowlist. An empty allowlist never skips (allow-all).
func NewRepoAllowlistFilter(allowlist []string) TaskCreationFilter {
	return &repoAllowlistFilter{allowlist: allowlist}
}

type repoAllowlistFilter struct {
	allowlist []string
}

func (f *repoAllowlistFilter) Skip(release Release) bool {
	return !repoallowlist.IsAllowed(f.allowlist, release.RepoKey)
}
