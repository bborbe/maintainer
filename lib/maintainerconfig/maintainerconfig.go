// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package maintainerconfig defines the single schema of the per-repo
// `.maintainer.yaml` trust file shared by all maintainer bots, plus a
// pure parser. Each top-level key is one bot's namespace:
//
//	release:
//	  autoRelease: true     # github-release watcher gate
//	prReviewer:
//	  autoApprove: true     # pr-reviewer agent gate
//
// Adding the next bot (build-fix, dep-pin, …) is a one-field edit to
// MaintainerConfig — every consumer imports this one type, so there is
// never a divergent copy of the file's shape. Unknown top-level keys are
// tolerated by design (yaml.Unmarshal ignores fields it does not know),
// which is the forward-compat behavior the spec mandates.
//
// Parse does NO I/O — fetching the bytes is each consumer's job (the
// watcher fetches via the GitHub API; the agent reads the cloned workDir
// on disk).
package maintainerconfig

import (
	"context"

	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"
)

// MaintainerConfig is the parsed shape of `.maintainer.yaml`. Each field is
// one bot's namespace; siblings are independent. A consumer reads only its
// own namespace and ignores the rest.
type MaintainerConfig struct {
	// Release is the github-release watcher namespace.
	Release ReleaseConfig `yaml:"release"`
	// PrReviewer is the pr-reviewer agent namespace.
	PrReviewer PrReviewerConfig `yaml:"prReviewer"`
}

// ReleaseConfig is the `release:` namespace. AutoRelease=true is the ONLY
// shape that lets the github-release watcher emit a release task; everything
// else (key absent, value false, file absent) skips the repo.
//
// ChangelogRewrite is the spec-059 per-repo opt-in flag for the 058 LLM
// rewrite pipeline. Default false (omit the field, set false explicitly,
// or omit the `release:` block — all equivalent). When true, planning
// invokes the 058 rewrite classification; when false (or absent), planning
// short-circuits with `rewrite_needed=false` regardless of ## Unreleased
// content — preserving the pre-058 header-rename-only behavior fleet-wide.
// Non-boolean values fail at parse time; the planning step is responsible
// for surfacing the error as `error_category=invalid_config`.
// See spec 059 § Desired Behavior 1-3 and § Goal.
type ReleaseConfig struct {
	AutoRelease      bool `yaml:"autoRelease"`
	ChangelogRewrite bool `yaml:"changelogRewrite"`
}

// PrReviewerConfig is the `prReviewer:` namespace. AutoApprove=true means
// "post an approving review on an approve verdict"; absence/false means
// comment-only.
type PrReviewerConfig struct {
	AutoApprove bool `yaml:"autoApprove"`
}

// Parse unmarshals a `.maintainer.yaml` document and returns the parsed
// config. Pure data extraction — no I/O. Empty input returns a zero-value
// MaintainerConfig with nil error. Malformed YAML returns a wrapped error
// (NOT a silent zero-value) so callers can fail loudly.
func Parse(ctx context.Context, content []byte) (MaintainerConfig, error) {
	var cfg MaintainerConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return MaintainerConfig{}, errors.Wrap(ctx, err, "unmarshal .maintainer.yaml")
	}
	return cfg, nil
}
