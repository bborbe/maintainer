---
tags:
  - dark-factory
  - idea
status: idea
---

# Build-Fixer Agent

## Idea

New `agent/build-fixer/` Pattern B Job that consumes build-failure tasks (created by `watcher/github-build`) and produces exactly one artifact: a `kind: bug` spec file in the failing repo's `specs/ideas/`, committed and pushed on a branch. That's the entire job. Dark-factory running in the target repo takes over from there (triage → approve → prompts → fix → PR).

Mirrors `agent/pr-reviewer/` structure: stateless, one-shot, spawned per task.

## Why

The build watcher creates tasks but nothing consumes them — the auto-fix loop is half-built. Earlier sketches had the fixer agent classify the failure, pick a runbook, write a prompt, open a PR. That conflates planning (what's broken?) with execution (how do we fix it?) and reinvents what dark-factory already does well.

The leaner split:

- **Build-fixer agent** = bug filer. Read logs, write a `kind: bug` spec with reproduction + expected/actual, commit it. Done.
- **Dark-factory** (already running in each target repo) = execution substrate. Triage the spec, approve, generate fix prompts, run them, open PR.

This makes build-fixer trivially small — no classifier library, no runbook coverage matrix, no PR machinery. It's "log-reader → spec-writer." And it composes: sentry-bug-analyser, flake-quarantiner, updater-agent all become the same shape (read signal → file bug spec → done).

See `~/Documents/workspaces/dark-factory/docs/bug-workflow.md` for the bug-spec contract.

## Sketch

- Layout: `agent/build-fixer/main.go` (k8s Job entry), `cmd/run-task/main.go` (local file-driven runner), `pkg/factory/`, `pkg/specwriter/`. Mirror `agent/pr-reviewer/`.
- Task body shape (provided by watcher): repo, default branch, episode SHA, list of failing workflows + run URLs.
- Workflow inside the agent:
  1. Clone repo at episode SHA into ephemeral workdir.
  2. Fetch failed workflow logs via `gh run view --log-failed` for each failing workflow.
  3. Generate a `kind: bug` spec via Claude:
     - Frontmatter: `status: idea`, `kind: bug`
     - `Reproduction`: failing workflow + run URL + episode SHA + smallest log excerpt that exhibits the failure
     - `Expected vs Actual`: green CI on default branch (cite `.github/workflows/<name>.yml`) vs the observed failure
     - `Why this is a bug`: cite the workflow file or repo conventions
     - No fix details — that's the prompts' job after approval (per bug-workflow.md)
  4. Write to `specs/ideas/bug-build-failure-<short-slug>.md` in the cloned workdir.
  5. Commit on a new branch `build-fixer/<episode-sha-short>`, push.
  6. Verdict back via Kafka with branch name + spec path (matches pr-reviewer contract).
- `assignee: build-fixer-agent` (matches what the watcher publishes).
- No PR — the spec lands on a branch; whoever runs dark-factory in that repo picks it up (or a separate watcher could open the PR; out of scope here).

## Risks / Open questions

- **Does the target repo run dark-factory?** Build-fixer assumes yes. Repos without dark-factory get a bug spec sitting on a branch with no consumer. Operator opt-in via `.maintenance.yaml` flag (`build_fixer.enabled: true`).
- **Spec quality from logs.** Claude generating a bug spec from raw `gh run view` output may produce vague reproductions. Mitigation: structured prompt with the bug-workflow.md checklist baked in; reject specs missing mandatory sections (re-prompt once, then escalate `needs_input`).
- **Branch vs PR boundary.** Agent pushes a branch but doesn't open a PR — keeps the agent's scope tight, but means a human (or another watcher) opens the PR. Alternative: agent opens a draft PR labelled `build-fixer:auto-filed`. Decide before promotion.
- **Spec filename collisions.** Two failing episodes on the same repo → two bug specs. Slug must include enough context (episode SHA short + failing workflow name) to avoid clobbering.
- **What if the bug is already filed?** Agent should scan `specs/ideas/` for an existing `bug-build-failure-*` spec referencing the same episode SHA or workflow, and skip (verdict `already_filed`).
- **Concurrency.** Two failures on the same repo within minutes — agent serializes via PriorityClass + ResourceQuota = 1 pod (matches pr-reviewer). Tasks queue.
- **Branch protection on default branch is irrelevant** — agent never touches default. It pushes to `build-fixer/*` branches only.
- **Loops.** Bug spec lands → dark-factory fixes → PR merges → build green → watcher closes task → next day a different failure files a different bug spec. Expected and desirable.

## Related

- Builds on: `github-build-watcher-mvp` (shipped)
- Companions: `github-build-watcher-close-on-green` (closes the task once the bug's fix lands)
- Contract: `~/Documents/workspaces/dark-factory/docs/bug-workflow.md` (spec frontmatter + sections)
- Touches: new `agent/build-fixer/` module, no new dark-factory runbooks (uses bug-spec flow)
- Prerequisite: target repos must run dark-factory daemon for the bug specs to get consumed
