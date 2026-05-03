---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-01T09:02:12Z"
generating: "2026-05-01T09:02:16Z"
prompted: "2026-05-01T09:11:58Z"
verifying: "2026-05-01T09:37:36Z"
completed: "2026-05-03T20:02:53Z"
branch: dark-factory/swap-execution-to-coding-pr-review
---

## Summary

- Replace the agent's hand-rolled execution-phase prompt with a thin wrapper that invokes the `bborbe/coding` plugin's `/coding:pr-review <base_ref>` slash command from inside the prepared review workdir.
- The plugin runs its own automated checks and dispatches a fan-out of specialist sub-agents (go-quality, go-security, go-architecture, go-test-coverage, etc.), producing a severity-bucketed findings report.
- The wrapper's only job is to point cwd at the workdir, hand off to the slash command with the right target branch, and translate the resulting report back into the unchanged JSON verdict shape so the existing verdict parser keeps working.
- Net effect: review quality moves from "single-pass eyeball over a diff string" to "ten specialist sub-agents over real source files," fulfilling the goal's #1 success criterion ("plugin-quality reviews"). Verdict schema, downstream parser, planning prompt, and ai_review prompt all stay intact.
- Failure paths (plugin missing, agent timeout, worktree cleanup error, sub-agent finds out-of-rubric criticality) all degrade gracefully — never a crash.

## Problem

Today's execution-phase prompt is a hand-rolled review workflow that reads files and a diff and then asks the LLM to spot issues in a single pass with no specialist review lens. It cannot match the depth of the `bborbe/coding` plugin's `/coding:pr-review` command, which fans out to ten or more domain-specialist sub-agents (Go quality, Go security, Go test coverage, Go architecture, factory-pattern compliance, HTTP-handler organization, license compliance, README quality, shellcheck, and more), runs project-aware automated checks (`make precommit`, license detection), and emits a structured Must-Fix / Should-Fix / Nice-to-Have report. We are reinventing — poorly — what the plugin already does, and every PR reviewed today gets a shallower verdict than it would if the plugin were driving. With Tasks A1 (plugin install) and A2a (workdir clone+checkout) already shipped, the only thing standing between us and plugin-quality reviews is the execution prompt itself.

## Goal

After this work, the execution phase of the PR-reviewer agent invokes the `/coding:pr-review` slash command inside the prepared workdir, receives the plugin's structured findings report, and emits the same JSON verdict shape it has always emitted. Reviews carry the depth of the plugin's specialist sub-agent fan-out without any change to the verdict contract, the verdict parser, or downstream consumers (controller, vault writer, eventual PR poster). The hand-rolled workflow prompt is gone.

## Non-goals

- Changing the JSON verdict shape, field names, or semantics
- Modifying the verdict parser or any downstream consumer
- Rewriting the planning-phase prompt or the ai_review-phase prompt (they may simplify if naturally affected, but no behavioral change required)
- Implementing PR-posting (`gh pr review --approve|--request-changes`) — that is Task A4
- Adding plugin-install logic at runtime — A1 already handles install at pod start
- Adding workdir clone/checkout logic — A2a already handles it
- Pinning the plugin version or sub-agent set — we track whatever the installed plugin provides
- Falling back to the old hand-rolled prompt when the plugin is unavailable (we escalate to human_review instead)
- Bitbucket parity for the plugin path (Bitbucket has its own slash command; out of scope here)

## Desired Behavior

1. The execution phase, when given a prepared review workdir and a base ref, invokes the `bborbe/coding` plugin's PR-review slash command with the base ref as its target-branch argument and the configured review mode (`short | standard | full`, default `standard`, configurable per-deployment via CLI flag `-review-mode` or env var `REVIEW_MODE`).
2. The slash command runs from inside the review workdir — never against the main agent checkout — so all sub-agent reads, automated checks, and worktree operations happen against the PR's code.
3. The plugin's consolidated findings report (Must Fix / Should Fix / Nice to Have buckets, automated-check results, sub-agent findings) is translated into the existing JSON verdict shape: `verdict`, `summary`, `comments[]`, `concerns_addressed[]`.
4. Severity translation is deterministic. Verdict roll-up: any Must-Fix finding raises verdict to `request_changes`; otherwise Should-Fix or Nice-to-Have findings without Must-Fix yield `comment`; a clean report yields `approve`. Per-comment severity uses a fixed 3-to-3 bucket map: Must-Fix → `critical`, Should-Fix → `major`, Nice-to-Have → `nit`. The `minor` severity is reserved for the LLM's judgment in edge cases that genuinely don't fit a plugin bucket; the deterministic mapping never emits `minor`.
5. Each comment in the verdict pins to a real file path and line number drawn from the plugin's report; findings the plugin cannot pin to a line are folded into the `summary` rather than emitted as comments without coordinates.
6. `concerns_addressed` continues to reflect each concern raised in the planning phase, marked either as addressed-by-the-code or as raised-as-a-comment, regardless of whether the underlying check came from the plugin or from a planning-flagged concern.
7. An empty diff is recognized before the slash command runs and short-circuits to `verdict: "approve"` with a `summary` of "no changes to review" and an empty `comments[]`, matching the plugin's own no-changes behavior.
8. The agent's tool allowlist for the execution phase grants every Bash invocation the slash command needs (worktree create/remove, fetch, ls-files, status, branch query, recursive cleanup) plus the sub-agent dispatch primitive (`Task`), so the slash command runs end-to-end without permission denials.

## Constraints

- **Frozen verdict schema**: `verdict ∈ {approve, request_changes, comment}`, `summary` string, `comments[]` of `{file, line, severity, message}`, `concerns_addressed[]` of strings. Field names, types, and value vocabularies do not change. The downstream verdict parser is the contract — its existing tests must keep passing untouched.
- **Frozen sibling prompts**: planning and ai_review prompts are out of scope. They may receive trivial edits (e.g. wording cleanup) only if naturally required by this swap; behavior must not change.
- **Restricted runtime environment**: the agent runs as a K8s Job with a tightly scoped Bash allowlist. The execution phase's allowlist must be extended to cover exactly what `/coding:pr-review` needs and nothing more — `git fetch`, `git worktree`, `git ls-files`, `git status`, `git branch`, plus `rm -rf` for worktree cleanup, plus the `Task` tool for sub-agent dispatch. The required set is documented in the plugin command's frontmatter (`coding/commands/pr-review.md`) and that document is the authoritative source for the patterns this spec must allow.
- **No runtime plugin install**: A1 (Spec 008) already installs the `bborbe/coding` plugin at pod boot via `lib/claude.PluginInstaller`. The execution phase does not install, refresh, or check the plugin — it assumes presence and escalates if absent.
- **No runtime clone/checkout**: A2a (Spec 010) already prepares the review workdir with the correct head ref checked out before the execution phase runs. The execution phase does not clone or checkout; it only operates inside the prepared workdir.
- **Read-only against the PR repo**: the execution phase must not commit, push, create branches, reset, or otherwise mutate the PR's git history. It may create and remove a `/tmp/pr-review-*` worktree (the slash command does this internally) but must not write to the workdir's tracked files.
- **Plugin report format is the contract surface**: the wrapper depends on the consolidated-report shape produced by `coding/commands/pr-review.md` Step 5 (Must Fix / Should Fix / Nice to Have headers, always present). If that contract drifts upstream, this spec's translation must be re-validated.
- **Report-to-JSON translation happens in-prompt, not in Go**: the wrapper prompt instructs the LLM to read the slash command's markdown report and emit the verdict JSON at the end of the execution phase. No Go-side report parser is added. Rationale: matches the existing flow (already proven in run-task smoke tests), avoids forcing the plugin into a structured-output mode it does not currently provide, and remains robust to non-breaking changes in the plugin's report wording. Trade-off acknowledged: parsing is non-deterministic; flakiness should surface as the "malformed report" failure mode (see table) rather than as silent verdict drift.
- **Per-deployment review mode**: review depth (`short | standard | full`) is a deployment-level concern (cost/speed trade-off per cluster), not a per-PR decision. The mode is wired through main.go's argument struct (existing pattern: `-model`, `-branch`, `-claude-config-dir`) as `-review-mode` with env-var fallback `REVIEW_MODE`, default `standard`. Dev clusters can opt into `full` for deeper audits without code changes; prod stays `standard` for predictable cost. Per-PR overrides are explicitly out of scope.
- **Sub-agent allowlist propagation**: when the slash command spawns specialist sub-agents via `Task`, each child agent uses its OWN `tools:` declaration from its agent-definition file — the parent's Bash allowlist does not propagate. Spec verification therefore must inspect each invoked `coding:*` sub-agent's declared tools (read-only Read/Grep/Glob/restricted Bash; no Write/Edit; no exfil primitives like curl/wget/nc). This is a verification step, not a runtime enforcement — the runtime enforcer is each child's agent-definition file.
- **Existing factory wiring**: the per-phase `executionTools` allowlist lives in `agent/pr-reviewer/pkg/factory/factory.go`. The set of patterns added there is the durable interface contract with the slash command — changes to that set are deployment-visible and must be tested.
- **Existing knowledge to reference**: `docs/claude-plugin-cli.md` (plugin CLI conventions, established by Spec 008) and `coding/commands/pr-review.md` (the slash command's contract). The spec does not duplicate their content; it references them.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `/coding:pr-review` slash command not registered (plugin missing despite A1) | Execution phase emits a `verdict: "comment"` with a diagnostic `summary` naming the missing command, escalates the task to `human_review` | Operator inspects pod, fixes plugin install, re-triggers task |
| Slash command exceeds K8s Job `activeDeadlineSeconds` | Job dies; controller observes failed phase; partial output (if any) is logged; verdict file is not written | Operator bumps Job timeout or scopes review to fewer sub-agents |
| Plugin's `make precommit` step fails inside the workdir | Failure is recorded in the verdict's `summary` and surfaced as a Must-Fix-equivalent finding; verdict becomes `request_changes` | Normal review feedback loop — author fixes precommit, re-pushes |
| Sub-agent fan-out partial failure (e.g. one agent times out) | Successful sub-agents' findings are still translated; missing agents are noted in `summary`; verdict is computed from what came back | Operator inspects logs; if persistent, narrows agent set or files plugin issue |
| Worktree cleanup fails after review (`/tmp/pr-review-*` lingers) | Logged as a warning; verdict is still emitted; pod cleanup at job-end removes the directory anyway | Automatic on next pod recycle |
| Slash command finds a critical issue that does not fit the existing severity rubric | Translated to `severity: "critical"` and verdict `request_changes`; the plugin's own bucket label is preserved verbatim in the comment `message` for traceability | Reviewer sees the unmapped category in plain text |
| Plugin report has findings but no file/line coordinates | Findings are folded into `summary` rather than emitted as un-pinned comments; verdict is still computed from severity | Author reads `summary`; future plugin versions may add coordinates |
| Empty diff (no changes between head and base) | Short-circuit before invoking the slash command: `verdict: "approve"`, `summary: "no changes to review"`, empty `comments[]` | Normal pass-through |
| Plugin emits malformed report (missing required headers) | Wrapper emits `verdict: "comment"` with a diagnostic `summary` describing the parse failure; raw report is logged | Operator inspects logs; if persistent, investigates plugin upstream |

## Security / Abuse Cases

- **PR content is untrusted input.** The slash command reads files from the PR's checked-out workdir (operator-controlled repo set, but PR author can put arbitrary file contents in any tracked file). The wrapper must not interpolate file contents into shell commands; the slash command's existing pattern of passing paths as exec arguments to `git` and `make` already enforces this and must not be undermined.
- **Bash allowlist is the trust boundary.** The added patterns (`git fetch`, `git worktree`, `git ls-files`, `git status`, `git branch`, `rm -rf`) must remain prefix-scoped — `Bash(git fetch:*)`, not `Bash(git:*)` — so the slash command cannot escalate to history-mutating git operations (`git push`, `git commit`, `git reset`). The `rm -rf` pattern is granted as `Bash(rm -rf:*)` (matching `coding/commands/pr-review.md` frontmatter exactly), deliberately not narrowed to `/tmp/pr-review-*`. Rationale: blast radius is the K8s pod's ephemeral storage — the agent has no writable mounts, no persistent volumes, and no filesystem outside `/tmp` and the workdir. A worst-case rogue `rm -rf /` only kills the pod, which the Job recycles anyway. A tighter pattern would be security theater unless A2b (PVC cache) lands and adds writable mounts; reconsider then.
- **`Task` tool broadens trust surface.** Each spawned sub-agent uses its own `tools:` declaration from its agent-definition file (parent's allowlist does not propagate). Verification step: inspect each invoked `coding:*` sub-agent's declaration to confirm read-only tooling appropriate for review (Read, Grep, Glob, restricted Bash for analysis only — no Write, no Edit, no curl/wget/nc/network exfil primitives). This audit is a one-time spec-verification task, not a runtime check; the runtime enforcement is each child's agent-definition file.
- **Plugin-supplied prompts are operator-trusted, not user-trusted.** The `bborbe/coding` plugin is installed from a known marketplace by the operator at pod boot; we assume its prompt content is trusted. PR authors cannot influence plugin content.
- **Worktree creation can fill `/tmp`.** A pathological PR with a huge tree can blow out `/tmp` during worktree creation. The Job's ephemeral storage limits constrain this; the wrapper does not need to add its own quota check, but worktree-create failures must surface as `verdict: "comment"` with a diagnostic, not as a crash.
- **No new outbound network surface.** The slash command does not introduce HTTP egress; all its work is local file inspection plus sub-agent dispatch. Existing pod network policy is sufficient.

## Acceptance Criteria

- [x] The hand-rolled execution-phase workflow prompt is removed (or rewritten end-to-end) and replaced by a wrapper prompt whose semantic core is delegation to `/coding:pr-review <base_ref>` plus report-to-JSON translation.
- [x] The wrapper invokes the slash command with the base ref drawn from task context, runs it inside the prepared workdir, and never against the main agent checkout.
- [x] The verdict JSON shape (`verdict`, `summary`, `comments[]`, `concerns_addressed[]`) is unchanged. The existing verdict-parser tests pass without modification.
- [x] Severity translation is documented in the wrapper prompt and is deterministic: Must-Fix → critical / Should-Fix → major / Nice-to-Have → nit.
- [x] The factory's execution-phase `AllowedTools` set is extended to include the patterns the slash command requires (`Task`, `Bash(git fetch:*)`, `Bash(git worktree:*)`, `Bash(git ls-files:*)`, `Bash(git status:*)`, `Bash(git branch:*)`, `Bash(rm -rf:*)`) and not more.
- [x] A `-review-mode` CLI flag and `REVIEW_MODE` env var are added to `main.go`'s argument struct. Default `standard`.
- [x] Sub-agent allowlist audit is documented (per Constraint): comment in `factory.go` confirms that each `coding:*` sub-agent's declared `tools:` was inspected and is read-only/no-exfil.
- [x] ~~Empty-diff path short-circuits before invoking the slash command and emits `verdict: "approve"` with `summary` "no changes to review."~~ **Deferred to follow-up spec** — adopt-before-enhance: the empty-diff path is rare in practice (the watcher already filters drafts/closed PRs before they reach the agent) and the plugin's own no-changes behavior is acceptable as today's default. Tracked separately for explicit handling.
- [x] ~~Plugin-missing path emits `verdict: "comment"` with a diagnostic `summary` and does not crash the agent.~~ **Deferred to follow-up spec** — adopt-before-enhance: spec 008's `PluginInstaller.EnsureInstalled` is the production guard against plugin absence; today's silent-degradation diagnosis (2026-05-03) was a config-dir state issue, not a plugin-runtime-missing failure. Explicit diagnostic-comment handling for the residual edge case (plugin uninstall mid-run) is tracked separately. Worktree-cleanup failure handling stays in scope and is a warning today.
- [x] `make precommit` passes in `agent/pr-reviewer/` (verified 2026-05-03 after fixing macOS-incompatible `/bin/true` test path).
- [x] **Local smoke test**: today's `make run-dummy-task` against trading PR #40 produced a verdict whose `concerns_addressed` references real findings from named sub-agents (`coding:go-quality-assistant`, `coding:go-test-coverage-assistant`, `coding:go-architecture-assistant`, `coding:simple-bash-runner`). Verdict JSON parses, severity-bucketed findings (3× critical, 1× major, 2× nit). Artifact: `agent/pr-reviewer/cmd/run-task/test-trading-40.md`.
- [x] **Scenario coverage**: scenario `005-spec-011-plugin-pr-review.md` exists (created 2026-05-03). Full e2e proven by today's local smoke against trading PR #40 — slash command ran, 4 sub-agents fired, verdict JSON parsed and matched schema, workdir cleanup completed.
- [x] After dev deploy: today's PR #2 (task `a5b7903a`, 2026-05-03 14:09:55Z) and PR #3 (task `61fc8314`, 14:47:17Z) end-to-end via watcher → controller → executor → agent → vault `## Review` populated with verdict. Closing acceptance — plugin-quality reviews become observable in the vault. Note: trivial PRs in code-reviewer don't fan out to specialists by design (model judges); the substantive Go-changes proof point is trading PR #40 above.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Local smoke test against PR #2:

```
cd agent/pr-reviewer/cmd/run-task && make run-dummy-task
```

Inspect the resulting verdict JSON: confirm it parses, `verdict ∈ {approve, request_changes, comment}`, and `concerns_addressed` reflects sub-agent findings (not the old prompt's generic concern echo).

After deploy to dev:

```
# Trigger one PR via the watcher; observe controller materialize the vault task
kubectlquant -n dev logs <agent-pr-reviewer-job-pod> | grep -E "coding:pr-review|sub-agent"

# Read the verdict written to the vault task and confirm it contains
# specialist findings (e.g. "go-quality:", "go-security:") rather than
# the old hand-rolled prompt's wording.
```

## Do-Nothing Option

Stay with the hand-rolled execution prompt. The agent continues to produce mechanically-correct but shallow reviews — single-pass LLM eyeballing of a diff string with no specialist lens, no project-aware automated checks, no severity buckets. The goal's #1 success criterion ("plugin-quality reviews") cannot be met, blocking the wave. We also continue paying maintenance cost on a prompt that duplicates — worse — what `bborbe/coding` already provides for free; every upstream improvement in the plugin's sub-agent set passes us by until we make this swap.

A weaker alternative — keep the hand-rolled prompt as a "fallback" branch when the plugin is missing — adds branching complexity to the wrapper, doubles the surface we test, and undermines A1's contract that the plugin is reliably installed at pod boot. Cleaner to escalate to `human_review` on plugin absence (a surfaced operator failure) than to silently degrade to a worse review path.
