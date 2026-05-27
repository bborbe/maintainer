// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package filter implements the TaskCreationFilter chain — predicates
// that decide whether a vault task should be created for a Release.
//
// See [[Watcher Writing Guide]] § Required components #4 (TaskCreationFilter chain)
// for the chain semantics. The release watcher's filters:
//
//  1. RepoAllowlistFilter — scope guard (allowlist via env)
//  2. EmptyUnreleasedFilter — skip if ## Unreleased has zero bullets
//  3. AutoReleaseFilter — skip if .dark-factory/config.yml: autoRelease: true
//  4. SHAUnchangedFilter — skip if cursor already recorded this master HEAD
package filter

//counterfeiter:generate -o ../mocks/task_creation_filter.go --fake-name TaskCreationFilter . TaskCreationFilter

// Release is the filter-evaluation input.
// Mirrors pkg.Release as a local type to avoid an import cycle (pkg imports
// filter; filter cannot import pkg).
type Release struct {
	RepoKey           string // "github.com/owner/name" — for RepoAllowlistFilter
	HeadSHA           string // full SHA — for SHAUnchangedFilter
	UnreleasedBullets int    // for EmptyUnreleasedFilter
	AutoRelease       bool   // for AutoReleaseFilter
}

// TaskCreationFilter decides whether a single Release should be skipped
// (no vault task created). Implementations return true to skip.
type TaskCreationFilter interface {
	// Skip returns true if the Release should be excluded from task creation.
	Skip(release Release) bool
}

// TaskCreationFilterFunc adapts a function to the TaskCreationFilter interface.
type TaskCreationFilterFunc func(release Release) bool

// Skip implements TaskCreationFilter for the function adapter.
func (f TaskCreationFilterFunc) Skip(release Release) bool {
	return f(release)
}

// TaskCreationFilters is a slice composite: skip if ANY member votes skip.
// An empty slice never skips (no filters configured = process every Release).
type TaskCreationFilters []TaskCreationFilter

// Skip returns true if any contained filter votes skip. Short-circuit on first hit.
func (fs TaskCreationFilters) Skip(release Release) bool {
	for _, f := range fs {
		if f.Skip(release) {
			return true
		}
	}
	return false
}
