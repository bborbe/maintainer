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
//	goUpdate:
//	  autoUpdate: true      # github-update-go-watcher gate
//
// Adding the next bot (build-fix, dep-pin, …) is a one-field edit to
// MaintainerConfig — every consumer imports this one type, so there is
// never a divergent copy of the file's shape.
//
// Typos in a KNOWN namespace are REJECTED by ParseStrict (`changelogRwrite`
// inside `release:`), because a high-trust .maintainer.yaml is load-bearing for
// release gating and a typo must fail loudly rather than produce a silent
// default-false config.
//
// UNKNOWN top-level namespaces are IGNORED, even by ParseStrict. This is
// forward compatibility, and it is not optional: one schema is read by several
// independently-deployed binaries, so a repo adopting a new bot's namespace
// must not break the bots that have not been rebuilt yet.
//
// This package previously rejected unknown top-level keys too, on the
// assumption that "add the field, then deploy the bot" left only a brief
// incompatible window. It does not. The window lasts until every consumer is
// rebuilt AND redeployed, and until then the failure is severe and quiet:
// on 2026-08-16, adding `goUpdate:` to two repos made the deployed
// github-releaser-agent fail its planning step with
// `field goUpdate not found`, which cleared the task's assignee and wedged the
// release. No tag, no retry, no alert — the repo simply stopped releasing.
//
// Parse does NO I/O — fetching the bytes is each consumer's job (the
// watcher fetches via the GitHub API; the agent reads the cloned workDir
// on disk).
package maintainerconfig

import (
	"bytes"
	"context"
	"reflect"
	"strings"

	"github.com/bborbe/errors"
	"github.com/golang/glog"
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
	// GoUpdate is the github-update-go-watcher namespace.
	GoUpdate GoUpdateConfig `yaml:"goUpdate"`
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
//
// AllowMajorBump is the spec-060 per-repo opt-in for automatic major-version
// releases. Default false (omit the field, set false explicitly, or omit
// the `release:` block — all equivalent). When false, the
// github-releaser-agent planning phase TRIPS (Status=NeedsInput, ## Plan
// outcome=needs_input, precondition_failed=major_bump_not_allowed) on any
// classifier verdict of `bump=major`, forcing a human ack before tag +
// push. When true, a major verdict proceeds to execution as before. The
// second lever is the `--allow-major` CLI flag (env `ALLOW_MAJOR`) — either
// source is sufficient. Non-boolean values fail at parse time; the
// planning step is responsible for surfacing the error as
// `error_category=invalid_config`. See spec 060 § Desired Behavior 1 and
// § Goal.
//
// AllowFork only has meaning when the repo carrying this config is itself a
// fork. The github-release-watcher currently drops forked repos during repo
// listing, upstream of this config being read at all, so a fork with
// `autoRelease: true` never releases. AllowFork is the fix's per-repo half:
// once the watcher stops filtering forks, it will additionally require
// AllowFork=true before treating a fork as release-eligible. Default false
// (field absent, or `.maintainer.yaml` absent) so a `.maintainer.yaml`
// INHERITED from forking a repo that already sets `autoRelease: true` does
// not silently start auto-tagging the fork — the fork owner must opt in
// explicitly. Non-boolean values fail at parse time like the other fields.
type ReleaseConfig struct {
	AutoRelease      bool `yaml:"autoRelease"`
	ChangelogRewrite bool `yaml:"changelogRewrite"`
	AllowMajorBump   bool `yaml:"allowMajorBump"`
	// AllowFork opts a forked repo into auto-release eligibility. See the
	// field-group doc comment above for why this defaults closed.
	AllowFork bool `yaml:"allowFork"`
}

// PrReviewerConfig is the `prReviewer:` namespace. AutoApprove=true means
// "post an approving review on an approve verdict"; absence/false means
// comment-only.
type PrReviewerConfig struct {
	AutoApprove bool `yaml:"autoApprove"`
}

// GoUpdateConfig is the `goUpdate:` namespace. AutoUpdate=true is the
// per-repo consent flag the github-update-go-watcher gates on before
// opening a Go-version-bump PR — the same trust-gate shape as
// ReleaseConfig.AutoRelease. A repo with no `.maintainer.yaml`, no
// `goUpdate:` block, or `autoUpdate` absent/false all read as false
// (opt-in, not opt-out).
type GoUpdateConfig struct {
	AutoUpdate bool `yaml:"autoUpdate"`
}

// Parse unmarshals a `.maintainer.yaml` document leniently (unknown fields
// are silently ignored). Pure data extraction — no I/O. Empty input returns
// a zero-value MaintainerConfig with nil error. Malformed YAML returns a
// wrapped error (NOT a silent zero-value) so callers can fail loudly.
//
// Fleet-tolerant by design: the github-release watcher reads `.maintainer.yaml`
// from every repo to gate auto-release, and a typo'd key in one repo must NOT
// break the watcher for the rest of the fleet. Use ParseStrict instead when
// the caller wants typos to fail closed (e.g. the github-releaser planning
// step — see spec 059).
func Parse(ctx context.Context, content []byte) (MaintainerConfig, error) {
	return parseInternal(ctx, content, false)
}

// ParseStrict unmarshals a `.maintainer.yaml` document with `KnownFields(true)`
// applied to the namespaces this binary knows, so an unrecognized key INSIDE a
// known namespace produces a wrapped error. Use this when the caller wants
// typos like `changelogRwrite` to fail loudly (e.g. the github-releaser
// planning step where a silent zero-value would disable the rewrite pipeline
// without operator signal).
//
// Unknown top-level namespaces are ignored rather than rejected — see the
// package doc for why that is required, and for the failure it prevents. The
// cost is that a misspelled namespace is indistinguishable from a newer one;
// both are ignored, and both are logged at WARNING.
//
// The lib's lenient Parse remains the default for fleet readers (watcher).
func ParseStrict(ctx context.Context, content []byte) (MaintainerConfig, error) {
	return parseInternal(ctx, content, true)
}

func parseInternal(
	ctx context.Context,
	content []byte,
	strict bool,
) (MaintainerConfig, error) {
	var cfg MaintainerConfig
	if len(content) == 0 {
		// Preserve the existing "empty bytes -> zero-value, nil" contract
		// (asserted by maintainerconfig_test.go lines 29-31 and 87-89).
		// yaml.NewDecoder(empty).Decode would return io.EOF; the explicit
		// short-circuit keeps the contract crisp.
		return cfg, nil
	}
	if strict {
		// Drop namespaces this binary does not know about BEFORE the strict
		// decode. KnownFields(true) cannot distinguish "typo inside release:"
		// from "namespace added by a newer schema", and conflating those makes
		// every additive schema change a fleet-wide outage. Filtering first
		// keeps typos inside known namespaces fatal, which is the property
		// ParseStrict exists for.
		filtered, err := dropUnknownNamespaces(ctx, content)
		if err != nil {
			return MaintainerConfig{}, err
		}
		content = filtered
	}
	dec := yaml.NewDecoder(bytes.NewReader(content))
	if strict {
		dec.KnownFields(true)
	}
	if err := dec.Decode(&cfg); err != nil {
		return MaintainerConfig{}, errors.Wrap(ctx, err, "unmarshal .maintainer.yaml")
	}
	return cfg, nil
}

// dropUnknownNamespaces removes top-level keys that MaintainerConfig does not
// declare, so a document written against a newer schema still parses here.
// Values are round-tripped as yaml.Node, which preserves the nested content
// verbatim for the strict decode that follows.
func dropUnknownNamespaces(ctx context.Context, content []byte) ([]byte, error) {
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, errors.Wrap(ctx, err, "unmarshal .maintainer.yaml")
	}
	known := knownNamespaces()
	for key := range raw {
		if _, ok := known[key]; ok {
			continue
		}
		// Warning, not V(2). This is the one downside of tolerating unknown
		// namespaces: a misspelled one (`prRevierer:`) is now indistinguishable
		// from a genuinely newer one, so neither fails the parse. Logging it
		// loudly is what keeps a typo discoverable. Volume is low — only the
		// strict path filters, and that runs once per release, not per fleet
		// scan (the watcher uses lenient Parse, which never reaches here).
		glog.Warningf(
			"ignoring unknown .maintainer.yaml namespace %q — either a newer schema than this binary, or a typo",
			key,
		)
		delete(raw, key)
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "marshal filtered .maintainer.yaml")
	}
	return data, nil
}

// knownNamespaces reads the yaml tags off MaintainerConfig rather than
// hardcoding a list, so adding a namespace stays a one-field edit.
func knownNamespaces() map[string]struct{} {
	t := reflect.TypeOf(MaintainerConfig{})
	out := make(map[string]struct{}, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}
