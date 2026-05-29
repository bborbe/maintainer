// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package prompts holds the Claude bump-classification prompt (embedded
// from bump_classification.md) and a typed parser for Claude's JSON
// verdict response. Pure-Go leaf library: no IO beyond //go:embed, no
// third-party dependencies except github.com/bborbe/errors.
//
// The prompt text is the Phase 1 verbatim ruleset: ordered
// major -> minor -> patch with concrete trigger criteria. The parser
// tolerates the three real-world Claude output shapes seen in
// pr-reviewer's extractVerdict history: plain JSON, fenced JSON in
// prose, and JSON embedded inside arbitrary prose.
package prompts

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/bborbe/errors"
)

//go:embed bump_classification.md
var bumpClassificationPrompt string

// BumpClassificationPrompt returns the embedded Phase 1 bump-classification
// prompt. The string is non-empty and contains the Phase 1 priority-order
// rules (major -> minor -> patch). Callers feed this string to a Claude
// agent step alongside the CHANGELOG bullets to classify.
func BumpClassificationPrompt() string {
	return bumpClassificationPrompt
}

// BumpVerdict is the typed shape of Claude's JSON response to the
// bump-classification prompt. Bump is one of "patch" | "minor" | "major".
// Reasoning is a one-sentence justification from Claude.
type BumpVerdict struct {
	Bump      string `json:"bump"`
	Reasoning string `json:"reasoning"`
}

// ParseBumpVerdict extracts a BumpVerdict from Claude's raw output string.
// Three extraction strategies are tried in order:
//  1. Parse the trimmed input as a JSON object directly.
//  2. Strip leading/trailing ```json fences and parse the inner block.
//  3. Find the last balanced {...} block in the input and parse it.
//
// First success wins. After successful unmarshal, the verdict is
// validated: Bump must be one of patch|minor|major (case-sensitive),
// Reasoning must be non-empty.
//
// Errors are wrapped via github.com/bborbe/errors and always contain
// the literal substring "parse bump verdict" so callers can grep
// verdict-parse failures apart from clone/git failures.
func ParseBumpVerdict(ctx context.Context, claudeOutput string) (BumpVerdict, error) {
	trimmed := strings.TrimSpace(claudeOutput)

	var v BumpVerdict

	// Strategy 1: Parse the trimmed input as a JSON object directly.
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
		return validateVerdict(ctx, v)
	}

	// Strategy 2: Strip ```json fences (allow leading prose by also trying
	// after the first ```json marker; mirror pr-reviewer's TrimPrefix shape
	// for the simple case).
	stripped := strings.TrimSpace(strings.TrimSuffix(
		strings.TrimPrefix(strings.TrimPrefix(trimmed, "```json"), "```"),
		"```",
	))
	if err := json.Unmarshal([]byte(stripped), &v); err == nil {
		return validateVerdict(ctx, v)
	}

	// Strategy 3: Find the last balanced {...} block in the input.
	block, ok := lastJSONBlock(trimmed)
	if !ok {
		return BumpVerdict{}, errors.Errorf(ctx, "parse bump verdict: no JSON found")
	}
	if err := json.Unmarshal([]byte(block), &v); err != nil {
		return BumpVerdict{}, errors.Wrapf(ctx, err, "parse bump verdict: %s", block)
	}
	return validateVerdict(ctx, v)
}

// validateVerdict enforces the field-level invariants from spec 046
// Desired Behavior 9: Bump must be in {patch, minor, major}; Reasoning
// must be non-empty. On failure, returns a zero verdict + a wrapped
// error containing "parse bump verdict".
func validateVerdict(ctx context.Context, v BumpVerdict) (BumpVerdict, error) {
	switch v.Bump {
	case "patch", "minor", "major":
		// ok
	default:
		return BumpVerdict{}, errors.Errorf(
			ctx,
			"parse bump verdict: invalid bump value %q (want patch|minor|major)",
			v.Bump,
		)
	}
	if strings.TrimSpace(v.Reasoning) == "" {
		return BumpVerdict{}, errors.Errorf(ctx, "parse bump verdict: missing reasoning")
	}
	return v, nil
}

// lastJSONBlock returns the last balanced {...} substring in s, or
// "", false if none exists. Mirrors agent/pr-reviewer/pkg/steps_review.go
// lastJSONBlock — kept private to this package to avoid an unwanted
// dependency edge.
func lastJSONBlock(s string) (string, bool) {
	end := strings.LastIndex(s, "}")
	if end < 0 {
		return "", false
	}
	depth := 0
	for i := end; i >= 0; i-- {
		switch s[i] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				return s[i : end+1], true
			}
		}
	}
	return "", false
}
