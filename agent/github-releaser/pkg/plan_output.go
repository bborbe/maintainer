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

	// OriginalUnreleased is the raw ## Unreleased body (verbatim, line-endings
	// preserved) captured at planning time. ai-review reads this from the task
	// page — never re-derives it from the repo — so an attacker who modifies
	// the repo between planning and review cannot mask drift.
	OriginalUnreleased string `json:"original_unreleased,omitempty"`

	// RewriteNeeded is true when the planning LLM judged the original body
	// does not conform to the Changelog Quality Guide and produced a cleaned
	// body in RewrittenUnreleased. When false, execution renames the header
	// only and leaves the body untouched.
	//
	// omitempty is deliberately NOT applied so a `false` decision is always
	// written explicitly — ai-review needs to distinguish "not decided" from
	// "decided no".
	RewriteNeeded bool `json:"rewrite_needed"`

	// RewrittenUnreleased is the cleaned body. Populated only when
	// RewriteNeeded is true. Execution replaces the ## Unreleased body with
	// this text before renaming the header.
	RewrittenUnreleased string `json:"rewritten_unreleased,omitempty"`
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
