---
tags:
  - dark-factory
  - idea
status: idea
---

# Close Build-Failure Tasks on Green Recovery

## Idea

When `watcher/github-build` detects a `red → green` transition, publish a task-completion command (or directly mark the open task `completed`) so the vault task closes itself when the build is fixed — whether the fix came from the build-fixer agent, a human commit, or a Dependabot merge.

## Why

Without auto-close, build-failure tasks accumulate. Every red episode creates a new task; nothing removes the stale ones once the underlying build recovers. Manual triage burden moves from "find failed builds" to "close stale tasks" — a different chore, same chore. The signal source already knows when the build recovers (the watcher sees green); plumbing that knowledge into task closure makes the lifecycle self-healing.

The MVP spec (`github-build-watcher-mvp`) ships without this on purpose: it's a clean slice of work and the auto-close requires a closure command that may not exist in the task-controller yet. This idea isolates the closure work so the MVP is independently shippable.

## Sketch

- Watcher's state machine adds a `red → green` transition handler that publishes a closure event with the same `task_id` derived from `episode_sha`.
- Closure shape — three options:
  1. **`CompleteTaskCommand` Kafka command** — task-controller transitions the task to `completed` (matches existing command-pattern; requires controller to support the command if it doesn't already).
  2. **Direct vault file edit by watcher** — watcher writes `status: completed` to the task frontmatter (couples watcher to vault layout — bad).
  3. **Synthetic agent run** — watcher publishes a minimal `CreateTaskCommand` for an "auto-close" assignee that just marks the original task done (overkill).
- Recommend (1). Verify task-controller has or can add `CompleteTaskCommand`.
- Body of completion event includes "build recovered at SHA `<recovery_sha>`" for audit.

## Risks / Open questions

- **Does the task-controller accept a `CompleteTaskCommand` today?** If yes, this idea is small. If no, the controller change becomes the gating work — possibly a separate spec in `bborbe/agent`.
- **What if a human starts working on the task (status: in_progress) and the build recovers via Dependabot?** Auto-close would yank the task out from under them. Mitigation: only close if status is `todo` or `backlog`; leave `in_progress` alone.
- **Force-push erases the failing commit — is that "green" or "stale red"?** Watcher sees green (no failed runs on default branch HEAD). Auto-close is correct.
- **Race: red → green → red transitions within one poll interval.** Watcher only sees the latest state. The intermediate red is invisible. Acceptable — the latest red would create a fresh task anyway with a different episode SHA.
- **Should closure publish a Kafka event or call a controller HTTP API?** Kafka matches the existing command pattern; HTTP would be a new integration shape. Stick with Kafka.

## Related

- Builds on: `github-build-watcher-mvp` (this idea is non-trivial only after MVP ships)
- Touches: `watcher/github-build/pkg/watcher.go` (transition handler), possibly `bborbe/agent` (controller `CompleteTaskCommand` if missing)
- Prerequisite check: read `bborbe/agent` repo to confirm completion-command support
