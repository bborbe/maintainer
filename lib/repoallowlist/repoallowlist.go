// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repoallowlist

import (
	"context"
	"fmt"
	"strings"

	"github.com/bborbe/errors"
	"github.com/golang/glog"
)

// IsAllowed reports whether the target is permitted by the allowlist.
// target must be a "host/owner/repo" string (e.g. "github.com/bborbe/maintainer").
// If the allowlist is empty or nil, all targets are allowed (allow-all semantics).
// If target is empty and the allowlist is non-empty, returns false.
// Malformed or invalid wildcard entries are logged with glog.Errorf and skipped.
//
// No ctx parameter: malformed-entry errors are logged via glog and discarded;
// they never escape the function, so there is nothing for ctx to enrich.
// Validate carries the ctx since it returns the error to the caller.
func IsAllowed(allowlist []string, target string) bool {
	if len(allowlist) == 0 {
		return true
	}
	if target == "" {
		return false
	}
	for _, entry := range allowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		kind, reason := classifyKind(entry)
		if reason != "" {
			glog.Errorf("repoallowlist: malformed entry %q: %s", entry, reason)
			continue
		}
		switch kind {
		case "literal":
			if entry == target {
				return true
			}
		case "wildcard":
			if matchWildcard(entry, target) {
				return true
			}
		}
	}
	return false
}

// Validate checks all entries in the allowlist for well-formedness.
// Returns nil if the allowlist is empty/nil or all entries are valid.
// Returns an aggregate error listing every malformed entry found.
// Whitespace-only and empty entries are silently skipped (not malformed).
func Validate(ctx context.Context, allowlist []string) error {
	var errs []error
	for _, entry := range allowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, reason := classifyKind(entry); reason != "" {
			errs = append(
				errs,
				errors.Errorf(ctx, "repoallowlist: malformed entry %q: %s", entry, reason),
			)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// classifyKind inspects the already-trimmed, non-empty entry and returns
// (kind, reason) where kind is "literal" or "wildcard" on success,
// and reason is a human-readable description of the problem on failure.
func classifyKind(entry string) (kind string, reason string) {
	segments := strings.Split(entry, "/")
	if len(segments) != 3 {
		return "", fmt.Sprintf(
			"entry %q: must have exactly 3 path segments (host/owner/repo)",
			entry,
		)
	}
	if strings.Contains(segments[0], "*") || strings.Contains(segments[1], "*") {
		return "", fmt.Sprintf(
			"entry %q: wildcard '*' is only valid in the repo (third) segment",
			entry,
		)
	}
	if segments[2] == "*" {
		return "wildcard", ""
	}
	return "literal", ""
}

// matchWildcard checks if a wildcard entry (host/owner/*) matches the target.
func matchWildcard(entry, target string) bool {
	entrySegments := strings.Split(entry, "/")
	targetSegments := strings.Split(target, "/")
	if len(targetSegments) != 3 {
		return false
	}
	return entrySegments[0] == targetSegments[0] && entrySegments[1] == targetSegments[1]
}
