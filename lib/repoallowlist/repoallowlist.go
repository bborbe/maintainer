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
// A leading '!' on an entry (immediately after TrimSpace, with no whitespace
// between '!' and the entry body) marks the entry as an exclusion. Example:
// "!github.com/bborbe/go-skeleton" excludes go-skeleton.
//
// A target is allowed iff (includes is empty OR any include matches the
// target) AND (no exclude matches the target). Excludes always override
// includes — if both match, the target is rejected.
//
// An exclude-only allowlist (no include entries) means "allow everything
// except the excluded entries" — the canonical allow-all-except case.
//
// Example:
//
//	includes: github.com/bborbe/*
//	excludes: !github.com/bborbe/go-skeleton
//	→ allows every bborbe repo except go-skeleton.
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
	includes, excludes, sawExclude := parseAllowlist(allowlist)
	includeMatched := anyMatches(includes, target)
	excludeMatched := anyMatches(excludes, target)
	if !sawExclude {
		return includeMatched
	}
	return (len(includes) == 0 || includeMatched) && !excludeMatched
}

// parseAllowlist splits the allowlist into includes and excludes, logging
// malformed entries via glog. sawExclude reports whether any '!'-prefixed
// entry was seen (well-formed or not); this drives the IsAllowed branch
// that distinguishes "no include matched" from "no includes were present".
func parseAllowlist(allowlist []string) (includes []string, excludes []string, sawExclude bool) {
	for _, entry := range allowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		original := entry
		isExclude := false
		if entry[0] == '!' {
			isExclude = true
			entry = entry[1:]
		}
		if isExclude {
			sawExclude = true
		}
		if entry == "" {
			glog.Errorf(
				"repoallowlist: malformed entry %q: must have exactly 3 path segments (host/owner/repo)",
				original,
			)
			continue
		}
		if _, reason := classifyKind(entry); reason != "" {
			glog.Errorf("repoallowlist: malformed entry %q: %s", original, reason)
			continue
		}
		if isExclude {
			excludes = append(excludes, entry)
		} else {
			includes = append(includes, entry)
		}
	}
	return includes, excludes, sawExclude
}

// anyMatches reports whether any entry in the slice matches the target
// using the shared literal-or-wildcard dispatch.
func anyMatches(entries []string, target string) bool {
	for _, e := range entries {
		if matchesEntry(e, target) {
			return true
		}
	}
	return false
}

// Validate checks all entries in the allowlist for well-formedness.
// Returns nil if the allowlist is empty/nil or all entries are valid.
// Returns an aggregate error listing every malformed entry found.
// Whitespace-only and empty entries are silently skipped (not malformed).
// A leading '!' on an entry marks it as an exclusion; the well-formedness
// check runs on the post-'!' portion of the entry, but the aggregated
// error message names the ORIGINAL '!'-prefixed entry so the operator
// sees what they wrote.
func Validate(ctx context.Context, allowlist []string) error {
	var errs []error
	for _, entry := range allowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		original := entry
		if entry[0] == '!' {
			entry = entry[1:]
		}
		if _, reason := classifyKind(entry); reason != "" {
			errs = append(
				errs,
				errors.Errorf(ctx, "repoallowlist: malformed entry %q: %s", original, reason),
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

// matchesEntry reports whether the (already-validated) entry matches the
// target. The entry must be a non-empty, well-formed "host/owner/repo"
// or "host/owner/*" — classifyKind has already accepted it. Both literal
// (host/owner/repo) and wildcard (host/owner/*) shapes are dispatched
// identically for includes and excludes.
func matchesEntry(entry, target string) bool {
	kind, _ := classifyKind(entry)
	switch kind {
	case "literal":
		return entry == target
	case "wildcard":
		return matchWildcard(entry, target)
	}
	return false
}
