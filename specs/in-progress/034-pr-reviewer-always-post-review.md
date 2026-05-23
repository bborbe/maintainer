---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-05-23T11:11:52Z"
generating: "2026-05-23T11:12:29Z"
prompted: "2026-05-23T11:17:50Z"
---

## Summary

- Eliminate the silent-skip behavior in the PR reviewer agent's planning phase: when planning finds `concerns: []` (an empty concerns array, e.g. for a trivial PR), the agent currently advances straight to `phase: done` with zero visible artifact on the GitHub PR.
- After this work, every PR review task that reaches the planning phase produces at least one visible artifact on the PR. The empty-concerns path posts a brief `COMMENT` review (`Reviewed by <BotLogin> — no concerns flagged.`); the non-empty path proceeds through execution → ai_review unchanged.
- The vault task file always gains a `## Verdict` section after planning, naming the posted review id and the event (`COMMENT` for the LGTM case, `APPROVE` / `REQUEST_CHANGES` after the full review for non-empty concerns).
- Posts go through the existing `pkg/githubposter` REST path; no new posting code path. Builds on spec 033 (App auth) — the bot identity in the LGTM body is interpolated from the same `BOT_GITHUB_LOGIN` env that spec 033 introduced.
- No new config flag. The bot is always present on every PR — that's the invariant. Noisy repos (e.g. dependabot/renovate bumps) are already filtered out earlier by the watcher's bot-author allowlist, so they never reach the agent.

## Problem

Today the PR reviewer agent runs the planning phase, emits a JSON block with `concerns: [...]`, and only continues to execution → ai_review when concerns are non-empty. When planning returns `concerns: []` (e.g. on a trivial README comment line change), the agent advances directly to `phase: done` and never calls `pkg/githubposter`. The GitHub PR receives zero comments, zero reviews, no indication the bot ran at all.

Discovered 2026-05-23 during the spec 033 (App auth migration) R3 verification cycle on `bborbe/go-skeleton#14`. The dev pr-reviewer agent ran end-to-end cleanly, produced valid planning JSON, but left no trace on the PR. From the PR author's perspective there is no way to distinguish:

1. Bot ran and approved (no concerns).
2. Bot crashed / failed silently.
3. Bot didn't pick up the PR at all.
4. Bot is disabled for this repo.

The bot's only canonical "did this thing happen" surface — visible posts on the PR — is conditional on having concerns. The whole value of having the bot on every PR is reduced. Operator trust in the bot's coverage erodes over time because every silently skipped PR is indistinguishable from a broken bot.

## Goal

After this work, the PR reviewer agent leaves at least one visible artifact on every PR whose task reaches the planning phase. The artifact is either:

- a brief `COMMENT` review posted on the empty-concerns path, OR
- the existing full review posted on the non-empty-concerns path.

The vault task file always shows what was posted via a `## Verdict` section. No code path in the agent reaches `phase: done` after a successful planning run without first emitting a post — this is a strict invariant with no opt-out.

## Non-goals

- Do NOT change the planning prompt's judgment about what counts as a concern — only change the post-planning routing.
- Do NOT change `autoApprove` semantics — the empty-concerns case maps to `event: COMMENT`, never `APPROVE`. `autoApprove` continues to be opt-in per repo and only affects the non-empty execution path.
- Do NOT change failure-mode handling — agent crashes still skip the post by definition; this spec is strictly about successful planning paths that today silently skip.
- Do NOT touch the App auth wiring landed in spec 033 — orthogonal. This spec consumes the `BOT_GITHUB_LOGIN` env that spec 033 introduced.
- Do NOT change Kafka topics, frontmatter contracts, or watcher behavior.
- Do NOT add new task_types or scenarios.
- Do NOT add a config flag to opt out of the LGTM comment. The invariant is "bot on every PR" — no per-repo escape hatch. If a future use case demands it, that's a separate spec.

## Desired Behavior

1. On completing the planning phase with `concerns: []`, the agent posts a `COMMENT` review via `pkg/githubposter` whose body is `Reviewed by <BotLogin> — no concerns flagged.` where `<BotLogin>` is interpolated from the env-injected `BOT_GITHUB_LOGIN` (default `ben-s-pull-request-reviewer[bot]` for prod, `ben-s-pull-request-reviewer-dev[bot]` for dev). The agent advances to `phase: done` only AFTER the POST succeeds.
2. On `concerns: [...]` non-empty, the existing planning → execution → ai_review → verdict-POST flow proceeds unchanged.
3. After planning, the vault task file gains a `## Verdict` section naming the posted review id and the event (`COMMENT` for the LGTM case; `APPROVE` / `REQUEST_CHANGES` after the full review for the non-empty case). The section is always present once planning completes successfully — no skip branches.
4. If the POST itself fails (network, GitHub error), the existing failure-mode wiring escalates the task to `human_review`. The agent does NOT silently swallow a post failure on the empty-concerns path.
5. The LGTM body is interpolated from `BotLogin` at runtime — no hardcoded `ben-s-pull-request-reviewer[bot]` literal anywhere in agent code or templates.
6. The `pr-post-back.md` doc is updated to document the new LGTM branch as part of the post-back contract.

## Constraints

- All errors via `github.com/bborbe/errors`. No `fmt.Errorf`. No stdlib `errors.New`.
- All logging via `github.com/golang/glog`.
- BSD-style license header on every new `.go` file.
- The existing `pkg/githubposter` REST POST path is the canonical way to post — do not introduce a new posting code path. The LGTM comment goes through the same `PrPoster` interface (`event: "COMMENT"` branch of `mapVerdictAndSummary` or an equivalent direct call path).
- The vault task file's `## Plan` section continues to be written by the planning phase; this spec adds a `## Verdict` section that names the posted artifact.
- Bot login MUST be sourced from the `BOT_GITHUB_LOGIN` env / `DefaultBotLogin` constant introduced by spec 033. No hardcoded literals.
- CHANGELOG entry under `## Unreleased`.
- Spec 033 (`033-migrate-pr-reviewer-to-github-app.md`) is the just-shipped auth spec that this spec extends. The LGTM body uses spec 033's `BotLogin` resolution; both must remain consistent.
- See `agent/pr-reviewer/docs/architecture.md` (planning → execution → ai_review three-phase decomposition) and `agent/pr-reviewer/docs/pr-post-back.md` (post-back contract) for the existing behavior this spec extends.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| Planning emits `concerns: []` | Agent POSTs LGTM comment via `PrPoster`; vault `## Verdict` section records review id + `COMMENT` event; task advances to `phase: done`. | None — happy path. | REST `GET /reviews` shows new entry; vault `## Verdict` section visible. |
| Planning emits `concerns: [...]` non-empty | Existing flow: execution → ai_review → verdict POST. Vault `## Verdict` populated after the full review. | None — existing path. | REST `GET /reviews` shows entry with `APPROVE` / `REQUEST_CHANGES`. |
| LGTM POST returns network error or GitHub 5xx | Task escalates to `human_review`; error is wrapped via `github.com/bborbe/errors` and logged. Task does NOT advance to `done`. | Operator inspects task; retries via watcher republish or manual rerun. | Vault task `phase: human_review`; `glog` error line with HTTP status; `## Diagnostics` section appended. |
| LGTM POST returns GitHub 4xx (e.g. invalid review state, PR closed) | Task escalates to `human_review`; error is wrapped and logged with response body. | Operator inspects PR state; reruns if appropriate. | Vault task `phase: human_review`; `## Diagnostics` block records HTTP status + body excerpt. |
| `BOT_GITHUB_LOGIN` env unset | LGTM body interpolates the `DefaultBotLogin` constant; identical behavior to non-empty case. | None — default applies. | LGTM body shows default login string; no error. |
| `.pr-reviewer.yaml` malformed (parse error) | Existing parser behavior preserved — fall back to defaults, log a warning. LGTM still posts on empty concerns regardless. | Operator fixes YAML. | `glog` warning line; LGTM still posts. |
| Concurrent planning runs against same PR (two pods, race) | Each pod independently POSTs an LGTM comment if both find empty concerns. GitHub permits duplicate COMMENT reviews; same as today's race for the non-empty path. | None — accepted; existing concurrency model unchanged. | Two reviews visible on PR; out-of-scope to dedupe. |
| Planning crashes before emitting JSON | Existing failure-mode handling — task escalates to `human_review`. No post attempted. | Operator inspects. | `phase: human_review`; this spec does not change this path. |
| GitHub secondary rate-limit / abuse-detection on the bot identity (HTTP 429 or 403 with retry-after) | Existing `pkg/githubposter` retry path with exponential backoff. After max retries, task escalates to `human_review`. | Operator inspects rate-limit usage; consider widening watcher's bot-author filter or temporarily throttling the watcher poll cadence. | `glog` warn line with HTTP status; retry count > 0 in metrics. |
| LGTM POST succeeds but vault task later escalates to `human_review` for a different reason | Per-review POSTs are append-only on GitHub. The LGTM stays on the PR even after the vault task's final phase changes. | None — GitHub does not support deleting reviews. Operator can dismiss + re-review if needed. | PR has both an LGTM comment AND a later `## Diagnostics` block in the vault task; review timeline shows both. |

## Security / Abuse Cases

- LGTM body content is fully bot-controlled (a fixed format string interpolating `BotLogin`). No PR-author-controlled text is embedded — no injection risk.
- A faulty planner that mistakenly clears concerns would cause the bot to LGTM a bad PR. This risk exists today (silent skip == implicit approval); this spec makes the signal visible but does not increase the risk surface. Mitigation: planner regressions are caught by the existing ai_review meta-verdict cycle for the non-empty case, and by operator pattern-matching on LGTM-comment volume for the empty case.
- LGTM POSTs use the same IAT bearer-token path as the existing review POSTs (spec 033). No new credential handling.

## Acceptance Criteria

Each AC names its evidence shape (exit code / grep / REST response / vault file content / kubectl state).

**Rung 1 — planner change + unit tests:**

- [ ] Planning phase exit decision branches on `concerns: []` to either (a) POST LGTM comment via `PrPoster` then advance to `phase: done`, or (b) advance to `phase: execution` (existing path) — evidence: `grep -n 'concerns\|Concerns' agent/pr-reviewer/pkg/factory/factory.go agent/pr-reviewer/pkg/steps_review.go agent/pr-reviewer/pkg/<new-or-modified-planning-step>.go` shows the branching predicate; ginkgo unit test asserts both branches against an `httptest.Server` standing in for GitHub.
- [ ] LGTM body is interpolated from `BotLogin` env at call time — evidence: `grep -rn 'no concerns flagged' agent/pr-reviewer/` returns ≥1 match; `grep -rn 'ben-s-pull-request-reviewer\[bot\]' agent/pr-reviewer/pkg/ --include='*.go'` returns zero matches outside test fixtures and the spec-033 default constant.
- [ ] Ginkgo test covers the empty-concerns LGTM POST path against `httptest.Server` and asserts the request body equals `Reviewed by <BotLogin> — no concerns flagged.` with the test's injected `BotLogin` value — evidence: `cd agent/pr-reviewer && go test ./pkg/... -run 'LGTM\|EmptyConcerns' -v` exits 0 and the test name surfaces in output.
- [ ] Ginkgo test covers the non-empty-concerns path: planning advances to `phase: execution` without POSTing an LGTM comment — evidence: `httptest.Server` records zero POST requests to `/reviews` during the planning-only sub-test; ginkgo assertion `Expect(requests).To(HaveLen(0))`.
- [ ] All new errors use `github.com/bborbe/errors`; no `fmt.Errorf` / stdlib `errors.New` in modified files — evidence: `grep -E 'fmt\.Errorf|errors\.New\(' agent/pr-reviewer/pkg/<changed-files>.go` returns empty.
- [ ] All new `.go` files carry the BSD-style license header — evidence: `grep -L 'BSD-style' <new-files>` returns empty.
- [ ] `cd agent/pr-reviewer && make precommit` exits 0 — evidence: exit code 0; output line `ready to commit`.

**Rung 2 — vault contract + docs + CHANGELOG:**

- [ ] Vault task file gains a `## Verdict` section after planning completes successfully — evidence: ginkgo test reads the produced vault content and asserts `grep -n '^## Verdict' <task-output>` returns ≥1 line.
- [ ] `## Verdict` section names the review id and event (`COMMENT` for the LGTM case; `APPROVE` / `REQUEST_CHANGES` after the full review for the non-empty case) — evidence: ginkgo test asserts the literal substrings present in each branch's output.
- [ ] `agent/pr-reviewer/docs/pr-post-back.md` is updated to document the always-post LGTM branch — evidence: `grep -n 'LGTM\|no concerns flagged' agent/pr-reviewer/docs/pr-post-back.md` returns ≥1 line.
- [ ] `CHANGELOG.md` has a new entry under `## Unreleased` describing the always-post-review behavior — evidence: `grep -A3 '## Unreleased' CHANGELOG.md` shows the entry mentioning "LGTM" or "no concerns" or "always post review".
- [ ] `cd agent/pr-reviewer && make precommit` exits 0 — evidence: exit code 0; output line `ready to commit`.

**Rung 3 — dev cluster deploy + verify on real PR:**

- [ ] **Post-Deploy (Rung-3):** The pr-reviewer container is rebuilt and uploaded for `dev` — evidence: `/make-buca agent/pr-reviewer dev` reports success; new image digest visible in `kubectlquant -n dev describe statefulset agent-pr-reviewer`.
  - deploy_check: `kubectlquant -n dev describe statefulset agent-pr-reviewer | grep 'Image:'` shows the just-built `:dev` image tag.
  - deploy_target: dev cluster, statefulset `agent-pr-reviewer`.
- [ ] **Post-Deploy (Rung-3):** The pr-reviewer pod rolls out cleanly in dev — evidence: `kubectlquant -n dev rollout status statefulset/agent-pr-reviewer --timeout=120s` reports complete.
  - deploy_check: `kubectlquant -n dev get pods -l app=agent-pr-reviewer` shows ready pods at the new image.
  - deploy_target: dev cluster, namespace `dev`.
- [ ] **Post-Deploy (Rung-3):** A trivial PR opened on `bborbe/go-skeleton` (e.g. one-line README change) receives an LGTM `COMMENT` review within 10 minutes of the bot picking up the task — evidence: PR comments tab on `https://github.com/bborbe/go-skeleton/pull/<N>` shows the review; the body matches `Reviewed by ben-s-pull-request-reviewer-dev[bot] — no concerns flagged.`.
  - deploy_check: spawned Job logs (`kubectlquant -n dev logs job/pr-reviewer-agent-<uuid>-<ts>`) show `auth mode=github-app` and a `pkg/githubposter` POST entry.
  - deploy_target: dev cluster Job spawned via the watcher → controller → executor pipeline.
- [ ] **Post-Deploy (Rung-3):** The vault task file's `## Verdict` section names the review id and the `COMMENT` event — evidence: `grep -n '^## Verdict' ~/Documents/Obsidian/OpenClaw/tasks/<task-file>.md` returns ≥1 line; the section body includes both the numeric review id and the string `COMMENT`.
  - deploy_check: same vault task file's frontmatter shows `phase: done` + `trigger_count: 1`.
  - deploy_target: OpenClaw vault (`~/Documents/Obsidian/OpenClaw/tasks/`), written by the dev pr-reviewer pod via the controller's git-rest path.
- [ ] **Post-Deploy (Rung-3):** REST API returns the new review as `ben-s-pull-request-reviewer-dev[bot]` with `state=COMMENTED` — evidence: `curl -s https://api.github.com/repos/bborbe/go-skeleton/pulls/<N>/reviews | jq '.[] | {user: .user.login, state}'` lists an entry with `user == "ben-s-pull-request-reviewer-dev[bot]"` and `state == "COMMENTED"`.
  - deploy_check: same `curl` confirms `body` matches the LGTM template (substring `no concerns flagged`).
  - deploy_target: GitHub REST API for `bborbe/go-skeleton`.
- [ ] **Post-Deploy (Rung-3):** During a ≥1-day dev soak, no task that reaches planning advances to `phase: done` without a posted review (LGTM or full) — evidence: `find ~/Documents/Obsidian/OpenClaw/tasks -name 'PR Review *- dev.md' -newermt '1 day ago' | xargs grep -L '^## Verdict'` returns empty.
  - deploy_check: same find re-run at end of soak; output must remain empty.
  - deploy_target: OpenClaw vault tasks created during the dev soak window.

**Rung 4 — prod cluster deploy:**

- [ ] **Post-Deploy (Rung-4):** After dev soaks ≥1 day with at least one clean PR cycle on each branch (empty-concerns LGTM and non-empty full review), the prod cluster rolls out — evidence: `/make-buca agent/pr-reviewer prod` reports success; `kubectlquant -n prod rollout status statefulset/agent-pr-reviewer --timeout=120s` reports complete.
  - deploy_check: `kubectlquant -n prod describe statefulset agent-pr-reviewer | grep 'Image:'` shows the just-built `:prod` image tag.
  - deploy_target: prod cluster, statefulset `agent-pr-reviewer`.
- [ ] **Post-Deploy (Rung-4):** A real PR opened on any `bborbe/*` repo serviced by prod that produces `concerns: []` receives an LGTM `COMMENT` review — evidence: `curl -s https://api.github.com/repos/bborbe/<repo>/pulls/<N>/reviews | jq '.[].user.login'` lists `ben-s-pull-request-reviewer[bot]` with state `COMMENTED`.
  - deploy_check: spawned Job logs (`kubectlquant -n prod logs job/pr-reviewer-agent-<uuid>-<ts>`) show `auth mode=github-app app_id=3798945`.
  - deploy_target: prod cluster Job spawned via the watcher pipeline.
- [ ] `CHANGELOG.md` `## Unreleased` entry is finalized and the spec is closed — evidence: dark-factory `spec complete` marks the spec `completed`; CHANGELOG entry remains present.

**Scenario coverage — NO new scenario.** The httptest-based ginkgo tests in Rung 1 and Rung 2 cover the observable contract (LGTM POST body, `## Verdict` section content, flag-driven branching) without requiring real GitHub credentials. The live cluster verification in Rung 3 and Rung 4 catches integration-level failures (App auth, real PRs, real Kafka). Adding a `scenarios/` file would duplicate either the unit tests or the live cluster smoke without catching anything new.

## Verification

Per `docs/verifying-specs.md`, this spec is **Rung-4** (touches a deployed service AND requires dev-then-prod ordering). Execute in order:

**Rung 1 + 2 — precommit (host):**

```
cd agent/pr-reviewer && make precommit
```

**Rung 3 — dev cluster:**

```
# Deploy new image to dev (canonical worktree path; per [[Make Buca - Deploy Service]] runbook)
cd ~/Documents/workspaces/maintainer-dev
git fetch
git merge origin/master --no-edit
git push
/make-buca agent/pr-reviewer dev

kubectlquant -n dev rollout status statefulset/agent-pr-reviewer --timeout=120s
```

Then open a trivial PR in `bborbe/go-skeleton` (one-line README change), wait for the watcher → controller → agent pipeline, then verify:

```
curl -s https://api.github.com/repos/bborbe/go-skeleton/pulls/<N>/reviews \
  | jq '.[] | {user: .user.login, state, body: .body[:80]}'
```

Expected: at least one entry with `user == "ben-s-pull-request-reviewer-dev[bot]"`, `state == "COMMENTED"`, and `body` matching the LGTM template.

Also verify the vault task file shows the `## Verdict` section:

```
grep -n -A3 '^## Verdict' ~/Documents/Obsidian/OpenClaw/tasks/<task-file>.md
```

Expected: section present, body names the review id and `COMMENT` event.

**Rung 4 — prod cluster:**

After dev soaks ≥1 day with at least one PR cycle in each branch (empty-concerns LGTM + non-empty full review), repeat against `~/Documents/workspaces/maintainer-prod` with `BRANCH=prod`. Confirm via REST that the LGTM review's `user.login` is `ben-s-pull-request-reviewer[bot]` and `state` is `COMMENTED`.

## Do-Nothing Option

If we don't ship this, the PR reviewer agent continues to silently skip every trivial PR. PR authors cannot tell if the bot is alive without checking other signals (vault task files, k8s logs, etc.) — defeating the purpose of having a bot on every PR. Operator trust in the bot's coverage erodes over time because every silently skipped PR looks indistinguishable from a broken bot. The fix is small and bounded (one branch in the planner exit decision, one extra POST, one doc update, one CHANGELOG line). Not acceptable to leave as-is.

---

**Related vault notes:**

- Task: `[[PR Reviewer Always Posts a Review]]`
- Goal: `[[GitHub Code Reviewer Agent - Base]]` (F2)
- Prior spec: `033-migrate-pr-reviewer-to-github-app.md` (just-shipped App auth migration that introduced `BOT_GITHUB_LOGIN` env, which this spec consumes)
- Reference docs: `agent/pr-reviewer/docs/architecture.md` (three-phase decomposition), `agent/pr-reviewer/docs/pr-post-back.md` (post-back contract — updated by this spec)
