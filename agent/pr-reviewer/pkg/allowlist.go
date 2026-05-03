// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"regexp"
	"strings"

	"github.com/bborbe/errors"
)

// repoAllowlistEntryPattern validates a single host-qualified repo entry.
// Required shape: host/owner/repo (three slash-delimited segments).
var repoAllowlistEntryPattern = regexp.MustCompile(
	`^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`,
)

// ParseRepoAllowlist parses a comma-separated allowlist string into a slice
// of validated host-qualified repo keys ("host/owner/repo").
//
// Empty string and unset env var both return (nil, nil) — allow-all.
// Whitespace-only entries and entries from trailing commas are silently
// dropped. Any entry that does not match the required shape causes an error.
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
				"repo allowlist entry %q does not match required format host/owner/repo (pattern: ^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$)",
				entry,
			)
		}
		result = append(result, entry)
	}
	return result, nil
}
