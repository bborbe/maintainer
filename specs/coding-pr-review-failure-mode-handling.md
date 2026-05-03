---
status: idea
tags:
    - dark-factory
    - spec
---

## Summary

- Add explicit failure-mode handling to the agent's `/coding:pr-review` wrapper for two edge cases that spec 011 deferred under the adopt-before-enhance principle.
- **Empty diff**: when `git diff <base>...HEAD` returns no changes, short-circuit to `verdict: "approve"` with `summary: "no changes to review"` and empty `comments[]`, BEFORE invoking the plugin. Avoids spawning a sub-agent fan-out for a guaranteed no-op.
- **Plugin missing at runtime**: when `/coding:pr-review` is unregistered in the agent's `CLAUDE_CONFIG_DIR` (residual edge case if the plugin was uninstalled mid-pod-lifetime, despite spec 008's `PluginInstaller.EnsureInstalled` guard at boot), emit `verdict: "comment"` with a diagnostic summary naming the missing command and route the task to `human_review` rather than producing a silently-degraded inline review.
- Both behaviors were ACs in spec 011 (#99, #100) but were deferred to keep the wave shippable. Today's silent-degradation diagnosis (2026-05-03 — config-dir state issue, not plugin-runtime-missing) showed the residual gap is rare but real.

## Problem

Spec 011 shipped the `/coding:pr-review` wrapper but deferred two failure-mode behaviors:

1. **Empty diff path** — when the agent receives a task whose `clone_url`/`ref`/`base_ref` triple produces an empty diff (e.g. PR with all commits squashed away, or base/head SHA collide), the current wrapper still spawns the full `/coding:pr-review` sub-agent fan-out which inevitably produces a "nothing to review" verdict. Wasteful and slow. Spec 011's #99 wanted a short-circuit before the fan-out fires.

2. **Plugin missing at runtime** — when the slash command `/coding:pr-review` is unregistered in the agent's `CLAUDE_CONFIG_DIR`, the wrapper today produces a silently-degraded inline review (the model just diffs the files itself, no sub-agent dispatch, no severity bucketing, no plugin-quality output). Today's diagnosis (2026-05-03) caught this in dev when `~/.claude-agent` had only the marketplace files but the plugin was never `claude plugin install`'ed. Spec 008's `PluginInstaller.EnsureInstalled` is the production guard at pod boot, but if the plugin is uninstalled mid-pod-lifetime or the config-dir state drifts, the residual silent-degradation reappears. Spec 011's #100 wanted an explicit `verdict: "comment"` + diagnostic summary in this case.

## Goal

After this work, the agent's execution phase distinguishes three execution paths and responds appropriately:

1. **Empty diff** — short-circuit before plugin invocation: `verdict: "approve"`, `summary: "no changes to review"`, `comments: []`. Cheap and fast.
2. **Plugin available, diff non-empty** — current happy path unchanged: dispatch to `/coding:pr-review`, translate report to verdict JSON.
3. **Plugin unregistered at runtime** — refuse silently-degrading; emit `verdict: "comment"` with `summary` naming the missing slash command and instruction to install. Routes to `human_review` per existing kafkaResultDeliverer mapping.

## Non-goals

- Do NOT change the existing happy-path wrapper — only add pre/post guards.
- Do NOT introduce runtime plugin-install logic in the agent (that lives in spec 008's `PluginInstaller`, called at pod boot).
- Do NOT change the verdict schema or severity buckets — uses existing values.
- Do NOT add empty-diff filtering at the watcher layer — that's a separate optimization.

## Desired Behavior

1. Before invoking the plugin, the wrapper runs `git diff --quiet <base>...HEAD` (or equivalent) inside the worktree; exit code 0 (no changes) triggers the empty-diff short-circuit.
2. The empty-diff short-circuit emits a verdict that satisfies the existing schema and downstream parser; no special-case handling needed in the deliverer.
3. Before dispatching to `/coding:pr-review`, the wrapper checks whether the slash command is registered (via a known-good probe — implementation detail for the prompt) and, if absent, emits the diagnostic verdict and exits before any specialist sub-agent is dispatched.
4. The diagnostic verdict for plugin-missing names the slash command and the configured `CLAUDE_CONFIG_DIR` so an operator can diagnose immediately.
5. Both new paths are unit-tested at the `checkoutExecutionStep.Run` level: empty-diff input → `verdict: approve`; plugin-missing → `verdict: comment` with diagnostic.

## Constraints

- Verdict schema, downstream parser, and severity buckets stay frozen.
- Pre-clone allowlist check (spec 013) runs BEFORE the empty-diff and plugin-missing checks; allowlist refusal still produces `Status: NeedsInput` as today.
- Order of checks inside `checkoutExecutionStep.Run`: parse `clone_url` → allowlist check → clone/worktree → empty-diff probe → plugin-presence probe → dispatch.
- Failure paths are mutually exclusive: a task that hits empty-diff returns `Done` with `verdict: approve`; a task that hits plugin-missing returns `Done` with `verdict: comment`; no path returns `Failed` for these conditions (parse-fail still returns `Failed` per spec 013).
- `errors.Errorf`/`errors.Wrapf` from `github.com/bborbe/errors`.
- Existing tests must keep passing.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `git diff` between base and head returns no changes | Short-circuit: `verdict: approve`, `summary: "no changes to review"`, `comments: []`. No plugin invocation. | None needed. |
| Slash command `/coding:pr-review` not registered in `CLAUDE_CONFIG_DIR` at runtime | Diagnostic verdict: `verdict: comment`, `summary: "/coding:pr-review not registered in CLAUDE_CONFIG_DIR=<path>"`, `comments: []`. Task routes to `human_review`. | Operator inspects pod config-dir state, re-runs `PluginInstaller`. |
| `git diff --quiet` errors (e.g. ref doesn't exist) | Existing `Failed` path preserved (parse-fail-style). | Operator inspects task ref. |

## Acceptance Criteria

- [ ] `checkoutExecutionStep.Run` runs an empty-diff probe before plugin dispatch; non-empty diff falls through to today's plugin path.
- [ ] Empty-diff probe sets `verdict: approve, summary: "no changes to review", comments: []` and returns `Status: Done` with `NextPhase: ai_review`.
- [ ] `checkoutExecutionStep.Run` runs a plugin-presence probe before dispatch; absent → `verdict: comment` with diagnostic naming the slash command and `CLAUDE_CONFIG_DIR`.
- [ ] Unit tests cover both new paths in `agent/pr-reviewer/pkg/steps_checkout_execution_test.go`.
- [ ] Existing happy-path tests pass without modification.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Behavioral test (manual):
1. Create a task whose `clone_url`/`ref`/`base_ref` produces an empty diff (e.g. `ref == base_ref`); run `make run-dummy-task`; verdict is `approve` with summary "no changes to review"; no sub-agents dispatched.
2. Uninstall `coding` plugin from `CLAUDE_CONFIG_DIR`; run `make run-dummy-task` against any PR; verdict is `comment` with diagnostic naming the missing slash command; no silent inline-review degradation.

## Do-Nothing Option

Acceptable for a while. Spec 008's `PluginInstaller.EnsureInstalled` already prevents the plugin-missing case at pod boot, and empty diffs rarely reach the agent because the watcher filters drafts and closed PRs. The cost is occasional silent degradation when config-dir state drifts (caught manually today). The fix is small but not urgent — track until evidence accumulates that either edge case fires in real usage.
