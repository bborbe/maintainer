// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import "github.com/bborbe/maintainer/agent/github-releaser/pkg/git"

// ResultOutput is the typed contract for the `## Result` JSON section the
// execution step writes for every release task. Round-trips with
// agentlib.MarshalSectionTyped + agentlib.ExtractSection[ResultOutput].
//
// Two shapes are valid:
//   - Outcome="released" — direct-push succeeded; CommitSHA + Tag populated; ErrorCategory empty
//   - Outcome="failed"   — any failure; ErrorCategory + Error populated; CommitSHA + Tag empty
//
// Future fields require a spec amendment.
type ResultOutput struct {
	Outcome       string            `json:"outcome"`
	Path          string            `json:"path"`
	CommitSHA     string            `json:"commit_sha,omitempty"`
	Tag           string            `json:"tag,omitempty"`
	ErrorCategory git.ErrorCategory `json:"error_category,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// Outcome values for ResultOutput.Outcome.
const (
	ResultOutcomeReleased = "released"
	ResultOutcomeFailed   = "failed"
)

// Path values for ResultOutput.Path. Only one value today; the PR-fallback
// spec will add a second (`"pr-merge"`).
const ResultPathDirectPush = "direct-push"
