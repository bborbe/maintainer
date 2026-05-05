// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter

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
