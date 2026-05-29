// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"strings"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	"github.com/bborbe/errors"
	domain "github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/prompts"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/semver"
)

// AgentLogin is the GitHub-task-system identity used in escalation frontmatter
// (previous_assignee). Per spec 047 § Constraints, this MUST be
// "github-releaser-agent" — grep-asserted by acceptance criteria.
const AgentLogin = "github-releaser-agent"

// requiredFrontmatterFields are the keys read from the task's frontmatter
// before the step does any IO. Missing OR empty → outcome=needs_input
// with precondition_failed = "missing_frontmatter_<field>".
//
// Order matters for deterministic error messages: first missing field wins.
var requiredFrontmatterFields = []string{
	"repo",
	"clone_url",
	"ref",
	"current_version",
	"task_identifier",
}

// planningStep implements agentlib.Step. Fields are constructor-injected;
// no global state, no IO outside the runner and fetcher.
type planningStep struct {
	runner  claudelib.ClaudeRunner
	fetcher githubchangelog.Fetcher
}

// NewPlanningStep wires the planning step with its two IO seams: the
// Claude runner (LLM verdict) and the CHANGELOG fetcher (GitHub contents
// API).
func NewPlanningStep(runner claudelib.ClaudeRunner, fetcher githubchangelog.Fetcher) agentlib.Step {
	return &planningStep{runner: runner, fetcher: fetcher}
}

// Name implements agentlib.Step.
func (s *planningStep) Name() string { return "github-release-plan" }

// ShouldRun always returns true. The planning step is idempotent: a
// re-trigger replaces the existing ## Plan section in place. Returning
// false here would silently skip routing.
func (s *planningStep) ShouldRun(_ context.Context, _ *agentlib.Markdown) (bool, error) {
	return true, nil
}

// Run executes the planning pipeline. Six outcomes:
//  1. Missing frontmatter        → escalate (NeedsInput, ## Plan needs_input, clear assignee)
//  2. CHANGELOG fetch fails      → Failed (controller retries)
//  3. P1/P2 validation fails     → escalate
//  4. Claude verdict unparseable → Failed (controller retries)
//  5. semver.BumpVersion fails   → escalate
//  6. Happy path                 → Done, NextPhase = execution, ## Plan ready
func (s *planningStep) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
	missingField, currentVersion, repo, cloneURL, ref := s.readRequired(md)
	if missingField != "" {
		glog.V(2).Infof("planning: missing frontmatter field=%s — escalating", missingField)
		return s.escalate(ctx, md, escalation{
			reason:             "required frontmatter field missing: " + missingField,
			preconditionFailed: PreconditionMissingFrontmatter + missingField,
			currentVersion:     currentVersion,
		})
	}

	owner, name, ok := parseOwnerRepo(repo)
	if !ok {
		glog.V(2).Infof("planning: malformed repo=%q — escalating", repo)
		return s.escalate(ctx, md, escalation{
			reason:             `frontmatter "repo" must be "owner/name"; got ` + repo,
			preconditionFailed: PreconditionMissingFrontmatter + "repo",
			currentVersion:     currentVersion,
		})
	}
	_ = cloneURL // currently unused by planning; future execution step will use it

	changelogBytes, err := s.fetcher.Fetch(ctx, owner, name, ref)
	if err != nil {
		glog.V(2).Infof("planning: fetch CHANGELOG.md failed: %v", err)
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: "fetch CHANGELOG.md: " + err.Error(),
		}, nil
	}
	glog.V(2).
		Infof("planning: fetched CHANGELOG.md owner=%s name=%s ref=%s bytes=%d", owner, name, ref, len(changelogBytes))

	valid, reason, _ := changelog.ValidateUnreleased(changelogBytes)
	if !valid {
		precondition := classifyValidationFailure(reason)
		glog.V(2).
			Infof("planning: validate Unreleased failed precondition=%s reason=%q", precondition, reason)
		return s.escalate(ctx, md, escalation{
			reason:             reason,
			preconditionFailed: precondition,
			currentVersion:     currentVersion,
		})
	}

	bullets := changelog.ExtractUnreleasedBullets(changelogBytes)
	prefixStyle := changelog.InferHeaderPrefixStyle(changelogBytes)

	return s.runClassification(ctx, md, currentVersion, bullets, prefixStyle)
}

func (s *planningStep) runClassification(
	ctx context.Context,
	md *agentlib.Markdown,
	currentVersion string,
	bullets []string,
	prefixStyle string,
) (*agentlib.Result, error) {
	userMsg := strings.Join(bullets, "\n")
	fullPrompt := prompts.BumpClassificationPrompt() + "\n\n## Bullets to classify\n\n" + userMsg

	runResult, err := s.runner.Run(ctx, fullPrompt)
	if err != nil {
		glog.V(2).Infof("planning: claude runner failed: %v", err)
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: "claude run: " + err.Error(),
		}, nil
	}

	verdict, err := prompts.ParseBumpVerdict(ctx, runResult.Result)
	if err != nil {
		glog.V(2).Infof("planning: parse verdict failed: %v", err)
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: "parse bump verdict: " + err.Error(),
		}, nil
	}

	nextNumeric, err := semver.BumpVersion(ctx, currentVersion, verdict.Bump)
	if err != nil {
		glog.V(2).Infof("planning: bump version failed: %v", err)
		return s.escalate(ctx, md, escalation{
			reason:             err.Error(),
			preconditionFailed: PreconditionBadCurrentVersion,
			currentVersion:     currentVersion,
		})
	}

	header := "## " + prefixStyle + nextNumeric
	output := PlanOutput{
		Outcome:           PlanOutcomeReady,
		Bump:              verdict.Bump,
		Reasoning:         verdict.Reasoning,
		CurrentVersion:    currentVersion,
		NextVersion:       nextNumeric,
		NextVersionHeader: header,
		HeaderPrefixStyle: prefixStyle,
		Bullets:           bullets,
	}
	section, err := agentlib.MarshalSectionTyped(ctx, "## Plan", output)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "marshal ## Plan section")
	}
	md.ReplaceSection(section)

	return &agentlib.Result{
		Status:    agentlib.AgentStatusDone,
		NextPhase: string(domain.TaskPhaseExecution),
	}, nil
}

// escalation captures the fields the escalate path needs to assemble the
// needs_input PlanOutput. Keeping it as a value type makes the call sites
// explicit and prevents missing-field bugs.
type escalation struct {
	reason             string
	preconditionFailed string
	currentVersion     string
}

// escalate writes a ## Plan(needs_input) section, clears `assignee`,
// sets `previous_assignee: github-releaser-agent`, and returns
// NeedsInput. status + phase are LEFT UNCHANGED — per spec 047
// § Constraints and [[Agent Task File Contract]] escalation rule.
//
// Returning AgentStatusNeedsInput (NOT Done) is critical: the framework
// deliverer switch (FileResultDeliverer / KafkaResultDeliverer) maps
// NeedsInput to "status: in_progress, assignee cleared, phase preserved"
// — exactly the escalation contract. Returning Done with empty NextPhase
// instead auto-advances to "phase: done, status: completed" (bug 048).
// The controller does not retry NeedsInput; the human operator
// re-delegates by re-setting assignee.
func (s *planningStep) escalate(
	ctx context.Context,
	md *agentlib.Markdown,
	e escalation,
) (*agentlib.Result, error) {
	output := PlanOutput{
		Outcome:            PlanOutcomeNeedsInput,
		Reason:             e.reason,
		PreconditionFailed: e.preconditionFailed,
		CurrentVersion:     e.currentVersion,
	}
	section, err := agentlib.MarshalSectionTyped(ctx, "## Plan", output)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "marshal ## Plan section (needs_input)")
	}
	md.ReplaceSection(section)

	// Frontmatter mutation: clear assignee, set previous_assignee.
	// Use direct map writes; TaskFrontmatter is map[string]interface{}.
	md.Frontmatter["assignee"] = ""
	md.Frontmatter["previous_assignee"] = AgentLogin

	return &agentlib.Result{
		Status:  agentlib.AgentStatusNeedsInput,
		Message: e.reason,
	}, nil
}

// readRequired pulls the five required frontmatter fields. Returns the
// first missing field's name (or "" if all present), plus the resolved
// values for current_version, repo, clone_url, ref. Empty string counts
// as missing.
func (s *planningStep) readRequired(
	md *agentlib.Markdown,
) (missing, currentVersion, repo, cloneURL, ref string) {
	values := map[string]string{}
	for _, key := range requiredFrontmatterFields {
		v, _ := md.Frontmatter.String(key)
		if strings.TrimSpace(v) == "" {
			return key, values["current_version"], values["repo"], values["clone_url"], values["ref"]
		}
		values[key] = v
	}
	return "", values["current_version"], values["repo"], values["clone_url"], values["ref"]
}

// parseOwnerRepo splits an "owner/name" string. Empty or no-slash input
// returns ok=false.
func parseOwnerRepo(s string) (owner, name string, ok bool) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// classifyValidationFailure maps the validator's reason string to the
// typed PreconditionFailed value. The reason strings are produced by
// changelog.ValidateUnreleased in pkg/changelog/changelog.go.
func classifyValidationFailure(reason string) string {
	switch {
	case strings.Contains(reason, "is not the first ## section"):
		return PreconditionP1UnreleasedNotFirst
	case strings.Contains(reason, "no bullet entries"),
		strings.Contains(reason, "not found"):
		return PreconditionP2UnreleasedEmpty
	default:
		return PreconditionP2UnreleasedEmpty
	}
}
