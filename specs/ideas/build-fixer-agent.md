---
tags:
  - dark-factory
  - idea
status: idea
---

# Build-Fixer Agent

## Idea

New `agent/build-fixer/` Pattern B Job that consumes build-failure tasks (created by `watcher/github-build`), classifies the failure, and dispatches the matching dark-factory runbook (initial coverage: Go module updates). Mirrors `agent/pr-reviewer/` structure: stateless, one-shot, spawned per task.

## Why

The build watcher (spec 1) creates tasks but nothing consumes them — the auto-fix loop is half-built. The dispatcher half is what closes the goal: detector creates task → fixer classifies + dispatches → dark-factory runs the runbook → commit + tag → CI goes green → watcher closes the task (or human does, until close-on-green ships).

Most failures across bborbe repos are predictable categories: stale Go deps, golangci-lint version drift, removed APIs, vendor folder out of sync. A small classifier (read the failed run logs, match against known patterns) routes 80% of cases to the right runbook. The long tail goes to a manual-review bucket (`status: needs_input`).

## Sketch

- Layout: `agent/build-fixer/main.go` (k8s Job entry), `cmd/run-task/main.go` (local file-driven runner), `pkg/factory/`, `pkg/classifier/`, `pkg/runbooks/`. Mirror `agent/pr-reviewer/`.
- Task body shape (provided by watcher): repo, default branch, episode SHA, list of failing workflows + run URLs.
- Workflow inside the agent:
  1. Clone repo at episode SHA into ephemeral workdir.
  2. Fetch failed workflow logs via `gh run view --log-failed` for each failing workflow.
  3. Classify: pattern-match against known failure signatures (Go deps stale, lint version, etc.). Use Claude with structured output for the long tail.
  4. Dispatch:
     - Known category → invoke matching dark-factory prompt template (e.g. `dark-factory prompt approve go-deps-update --repo <repo>`).
     - Unknown → set verdict `needs_input`, populate diagnostic with classifier's best guess + log excerpts.
  5. Verdict back via Kafka (matches pr-reviewer contract).
- Initial classifier coverage:
  - Stale Go modules → `go-deps-update` runbook
  - golangci-lint version drift → `lint-version-bump` runbook (TBD)
  - Pre-commit hook version drift → `precommit-update` runbook (TBD)
  - Everything else → `needs_input`
- `assignee: build-fixer-agent` (matches what the watcher publishes).

## Risks / Open questions

- **dark-factory invocation from inside a Pattern B Job.** dark-factory runs prompts in a YOLO container; can a Pattern B Job spawn dark-factory or does dark-factory need to be invoked out-of-band? Likely: agent writes the prompt file, commits to a fix branch, opens a PR — the PR-merge then triggers the next watcher cycle. Need to clarify the dispatch boundary.
- **Classifier is the hard part.** Pattern-matching log excerpts is brittle. Claude classifying log excerpts is more robust but adds Claude API dependency to every red-repo poll. Cost-bound the classifier.
- **Runbook coverage is sparse.** `go-deps-update` exists; `lint-version-bump` and friends don't yet. Each new runbook is its own small spec. Initial release: cover Go deps only, route everything else to `needs_input`.
- **False-fix risk.** Agent commits + opens PR; the PR's CI must validate the fix before merge. Auto-merge is out of scope (human reviews the PR). Goal success criterion #5 ("False positives routed to manual-review bucket, not auto-fixed") is exactly this gate.
- **Concurrency.** Two failures on the same repo within minutes — how does the agent serialize? Probably: PriorityClass + ResourceQuota = 1 pod (matches pr-reviewer). Tasks queue.
- **Loops.** Agent fixes build → CI passes → watcher closes task → next day Dependabot bumps something else → cycle repeats. Expected and desirable.
- **What about repos where the agent's commits violate branch protection?** Repo-level config — operator opt-in.

## Related

- Builds on: `github-build-watcher-mvp`
- Companions: `github-build-watcher-close-on-green`
- Touches: new `agent/build-fixer/` module, possibly new `pkg/classifier/`, possibly new dark-factory runbook templates
- Prerequisite: each runbook category needs its own dark-factory prompt template authored before the agent can dispatch to it
