// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

// PlanOutput is the typed contract for the `## Plan` JSON section the
// planning step writes for every release task. Round-trips with
// agentlib.MarshalSectionTyped + agentlib.ExtractSection[PlanOutput].
//
// Two shapes are valid:
//   - Outcome="ready"        — planning succeeded; Bump/NextVersion populated
//   - Outcome="needs_input"  — precondition failure; Reason + PreconditionFailed populated
//
// No `Details map[string]any`: concrete fields only. Future fields require
// a spec amendment.
type PlanOutput struct {
	Outcome            string   `json:"outcome"`
	Bump               string   `json:"bump,omitempty"`
	Reasoning          string   `json:"reasoning,omitempty"`
	CurrentVersion     string   `json:"current_version,omitempty"`
	NextVersion        string   `json:"next_version,omitempty"`
	NextVersionHeader  string   `json:"next_version_header,omitempty"`
	HeaderPrefixStyle  string   `json:"header_prefix_style,omitempty"`
	Bullets            []string `json:"bullets,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	PreconditionFailed string   `json:"precondition_failed,omitempty"`
}

// Outcome values for PlanOutput.Outcome.
const (
	PlanOutcomeReady      = "ready"
	PlanOutcomeNeedsInput = "needs_input"
)

// PreconditionFailed values. Keep in sync with spec 047 Desired Behavior 5.
const (
	PreconditionP1UnreleasedNotFirst = "P1_unreleased_not_first"
	PreconditionP2UnreleasedEmpty    = "P2_unreleased_empty"
	PreconditionBadCurrentVersion    = "bad_current_version"
	// PreconditionMissingFrontmatter is the PREFIX used for missing-field
	// precondition values; planning code appends the field name, e.g.
	// "missing_frontmatter_clone_url".
	PreconditionMissingFrontmatter = "missing_frontmatter_"
)
