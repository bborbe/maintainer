---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-05-23T19:34:29Z"
generating: "2026-05-23T19:34:29Z"
prompted: "2026-05-23T19:38:49Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

## Summary

- The `maintainer-watcher-github-pr` admin surface today exposes one endpoint, `/trigger`, that polls every configured repo — name and behavior are mismatched. This spec splits the surface into two honestly-named endpoints.
- `POST /check` replaces today's `/trigger` verbatim — runs the full poll cycle. Cron-equivalent.
- `POST /trigger?url=<pr_url>` is new — fires a single PR review by URL, bypasses per-(PR, SHA) vault dedup so operators can force a re-run when an existing task is stale.
- Hard cutover, no backwards compat. Solo repo; operator updates every caller (cron, runbook, bookmark) in the same change set.
- Triggering incident (2026-05-23): re-verifying `bborbe/trading#134` after spec 035 shipped required manually editing vault frontmatter to reset `phase` + `status` + `trigger_count` because close+reopen on GitHub didn't beat the dedup. That workaround is fragile, undiscoverable, and now obsolete once `/trigger?url=` exists.

## Problem

The watcher exposes a single admin endpoint, `/trigger`, that runs the full multi-repo poll cycle — identical to the 5-minute cron. Its name implies "fire one thing"; its behavior is "fire everything". Operators reading runbooks consistently misread the name.

Worse, there is no mechanism today to force a single-PR re-run. The watcher's per-(PR, SHA) vault dedup blocks duplicate task creation, which is normally correct but becomes a trap when the existing task is stale — e.g. a `phase: done` task created by a buggy pre-fix code path. On 2026-05-23, re-verifying `bborbe/trading#134` after the spec 035 stale-phase fix shipped required manually editing the OpenClaw vault task frontmatter (`phase`, `status`, `trigger_count`) — undocumented, fragile, and burned ~10 minutes mid-cycle. Closing and reopening the PR on GitHub did not help because the dedup is keyed on (PR, SHA), not on PR state. The only safe operator action — "ignore the stale task and process this PR fresh" — has no API surface.

## Goal

After this work, the watcher's two distinct operations have two distinct, honestly-named endpoints. `POST /check` runs the multi-repo poll (the cron-equivalent). `POST /trigger?url=<pr_url>` fires a single PR review by URL, bypasses per-(PR, SHA) dedup, respects the existing filter chain (drafts, bot authors, age, repo allowlist), and returns a structured JSON response naming the materialized task. No `/trigger` (no args) endpoint survives — the rename is hard. The operator-facing workaround of editing vault frontmatter to break dedup is retired.

## Non-goals

- Do NOT keep `/trigger` (no args) as a backwards-compat alias or 308 redirect — solo repo, hard cutover. If a future consumer demands variation, that's a separate spec.
- Do NOT change the cron schedule or the poll loop's internal logic.
- Do NOT add a third "trigger by repo only" endpoint (`/check?repo=X`). Separate spec if it becomes useful.
- Do NOT change the filter chain. Drafts, bot authors, age, repo allowlist all stay; the new endpoint runs the same chain on a single PR.
- Do NOT touch `maintainer-watcher-github-build`. Independent endpoints, out of scope.
- Do NOT modify the agent task controller or executor beyond the dedup-bypass flag. The CreateTaskCommand shape gains one new field; nothing else changes.
- Do NOT add a per-feature opt-out flag for dedup-bypass — invariant on the `/trigger?url=` endpoint; if a future consumer demands variation, that's a separate spec.

## Desired Behavior

1. `watcher/github-pr/main.go` registers `router.Path("/check")` with the existing poll handler — moved verbatim from the old `/trigger` route, no behavior change.
2. `watcher/github-pr/main.go` registers `router.Path("/trigger")` with a NEW single-PR handler. The handler:
   - reads the `url` query param
   - parses it via the existing `pkg/prurl.ParsePRURL`
   - runs the parsed PR through the existing `TaskCreationFilter` chain (reused, not reimplemented)
   - on filter rejection, returns HTTP 422 with the filter's name + reason in the JSON body
   - on filter pass, publishes a `CreateTaskCommand` with the dedup-bypass flag set so the controller materializes a fresh vault task even if a same-(PR, SHA) task already exists
3. Success response shape (HTTP 200): JSON `{"task_id": "<uuid>", "kafka_offset": <int>, "repo": "<owner/repo>", "pr_number": <int>, "head_sha": "<sha>"}`.
4. Failure response shape: JSON `{"error": "<reason>", "filter": "<filter name if filter rejection>", "pr_url": "<url echoed if present>"}`. HTTP status: 400 (bad/missing URL), 422 (filter rejection), 502 (Kafka publish failure).
5. `CreateTaskCommand` schema gains a `force_bypass_dedup` field (or named equivalent — implementer picks the name; default false). Implementation decision (scoped to the implementer): if dedup is enforced server-side in the controller, the flag is checked there at vault-task materialization; if dedup is purely a vault-file-presence check inside the watcher before publish, the watcher skips that check when serving `/trigger?url=` and publishes the command directly. Either path is acceptable; pick the smaller diff after reading the existing dedup code.
6. All existing callers of the old `/trigger` (poll) endpoint are updated to `/check` in the same change set. Includes any Kubernetes CronJob manifests under `~/Documents/workspaces/maintainer/k8s/` and the runbook in `~/Documents/Obsidian/Personal/65 Runbooks/`.
7. The runbook documents both endpoints with example `curl` commands and the response shape.

## Constraints

- All errors via `github.com/bborbe/errors`. No `fmt.Errorf`. No stdlib `errors.New`.
- All logging via `github.com/golang/glog`.
- BSD-style license header on every new/modified `.go` file.
- HTTP handlers use `libhttp.NewBackgroundRunHandler` or the existing handler pattern in the watcher — no new HTTP plumbing approach.
- PR URL parsing uses `pkg/prurl.ParsePRURL` — reuse, do not reimplement.
- Filter chain is the existing `TaskCreationFilter` in `watcher/github-pr/pkg/` — reuse, do not reimplement.
- `CreateTaskCommand` schema change must be backwards compatible at the wire level (new field defaults to false; existing publishers/consumers see no behavior change).
- CHANGELOG entry under `## Unreleased`.
- Both endpoints sit behind the admin gateway (`https://<stage>.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/...`) which already enforces Google OAuth. No new auth surface.
- Existing knowledge to reference:
  - Precedent admin-trigger handler: `agent/task/executor/pkg/handler/healthcheck_trigger_handler.go` (sibling repo) — reference for shape, do not bind to it.
  - Spec 026 (per-(PR, SHA) task creation) — defines the dedup invariant the new flag deliberately overrides.
  - Spec 027 (post-verdict to GitHub PR) — confirms the bot's reviews are append-only on GitHub, so dual reviews from a race are harmless.
  - Spec 035 (stale-phase fix) — the triggering incident for this spec; the manual vault-edit workaround is what `/trigger?url=` replaces.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| `POST /trigger` without `url` query param | HTTP 400; JSON body `{"error": "url query parameter required"}` | Operator adds `?url=<pr_url>` | `curl -i` first line `HTTP/* 400` + structured body |
| `POST /trigger?url=<not-a-pr-url>` | HTTP 400; JSON body cites parse error from `prurl.ParsePRURL` | Operator corrects URL | 400 + error string in body |
| `POST /trigger?url=<filtered-PR>` (draft, bot author, non-allowlisted repo, too old) | HTTP 422; JSON body names the filter that blocked + filter's reasoning | Operator removes the filter condition OR accepts that the PR is intentionally filtered | 422 + filter name; `glog` line records filter rejection |
| `POST /trigger?url=<valid PR>` but Kafka publish fails | HTTP 502; JSON body cites Kafka error | Operator inspects Kafka cluster health (admin gateway, cluster logs) | 502 + `glog` error line |
| `POST /trigger?url=<valid PR>` happy path | HTTP 200; JSON body with `task_id` + `kafka_offset`; CreateTaskCommand published with `force_bypass_dedup=true`; controller materializes a fresh vault task even if same-(PR, SHA) task exists | None — happy path | Response body contains UUID; vault task file shows fresh `current_job` after next agent run |
| Two operators hit `/trigger?url=<same PR>` concurrently | Both publish CreateTaskCommands with `force_bypass_dedup=true`; both materialize. Accept dual reviews — bot reviews are append-only (spec 027) and a double LGTM is harmless. Rare in practice. | None — accepted | Two task UUIDs returned; controller logs show two materializations |
| Old consumer reads CreateTaskCommand with new `force_bypass_dedup` field (schema drift during partial rollout) | Field is ignored; consumer behaves as if `false` (dedup enforced normally). No crash, no malformed parse — the Kafka command is JSON-encoded and unknown fields are tolerated. | None — backwards compatible at the wire level by design (Constraint line 59). | Pre-update consumer logs no change; new field appears in Kafka inspector but is not acted on. After consumer redeploy, the field takes effect. |
| Mid-request crash (pod OOMKill, restart) after URL parse but before Kafka publish | No command published; operator retries `POST /trigger?url=...` | Operator re-issues request | No new task in vault; no Kafka offset returned (connection drops) |
| `POST /check` (no args) | Runs the full multi-repo poll cycle — behavior identical to today's `/trigger` no-arg path | None | Same `glog` poll-cycle log lines as today |

## Security / Abuse Cases

- Both endpoints sit behind the admin gateway, which enforces Google OAuth. No new auth surface.
- `url` query param is operator-controlled. The parser (`prurl.ParsePRURL`) already validates URL shape and decomposes into structured fields before any downstream call — no shell interpolation, no path traversal, no SSRF (the URL is parsed for owner/repo/PR-number and used to construct a CreateTaskCommand; the watcher does not fetch the URL).
- `force_bypass_dedup=true` lets an authenticated operator double-spawn a review on the same PR. Acceptable because (a) operator is authenticated via admin gateway, (b) the bot's reviews are append-only on GitHub (spec 027) so double LGTMs are harmless, (c) double-spawn is the explicit point of the endpoint — this is the workaround for the stale-task trap that prompted the spec.
- No new outbound network surface. The handler publishes to the existing Kafka topic and returns; no new HTTP egress.

## Acceptance Criteria

Rung 1 — code + tests:

- [ ] `watcher/github-pr/main.go` registers `router.Path("/check")` wired to the existing poll handler — evidence: `grep -n 'router.Path("/check")' watcher/github-pr/main.go` returns ≥1 match.
- [ ] `watcher/github-pr/main.go` registers `router.Path("/trigger")` wired to the new single-PR handler — evidence: `grep -n 'router.Path("/trigger")' watcher/github-pr/main.go` returns ≥1 match AND the poll handler reference (`libhttp.NewBackgroundRunHandler.*poll` or equivalent) appears on the `/check` line, NOT the `/trigger` line.
- [ ] No `/trigger` route survives mapped to the poll handler — evidence: `grep -nE 'Path."/trigger".*poll|Path."/trigger".*NewBackgroundRunHandler' watcher/github-pr/main.go` returns zero matches.
- [ ] New handler implementation lives in `watcher/github-pr/pkg/` and is covered by table-driven unit tests with ≥80% coverage — evidence: `cd watcher/github-pr && go test -cover ./pkg/...` reports ≥80% on the new file; sibling `*_test.go` has cases for: missing url, invalid url, each filter-rejection branch, valid PR happy path, Kafka publish failure.
- [ ] `CreateTaskCommand` schema gains a `force_bypass_dedup` (or named equivalent) field, default `false`, AND the new `/trigger?url=` handler actually sets it to `true` on publish, AND the enforcement site branches on it — evidence: all three gates pass: (a) `grep -rn 'force_bypass_dedup\|ForceBypassDedup\|BypassDedup' agent/ watcher/ --include='*.go'` matches in producer (watcher) and consumer (controller/executor); (b) `grep -rnE 'ForceBypassDedup\s*:\s*true|"force_bypass_dedup"\s*:\s*true' watcher/github-pr/pkg/ --include='*.go'` returns ≥1 line in the new handler (flag actually set, not just declared); (c) `grep -rnE 'if[^/]*ForceBypassDedup|if[^/]*force_bypass_dedup' agent/ watcher/ --include='*.go'` returns ≥1 line (consumer branches on the flag).
- [ ] Dedup-bypass path is unit-tested at whichever site enforces it (watcher pre-publish OR controller materialization). Test asserts that when the flag is true, materialization proceeds even when a same-(PR, SHA) task already exists — evidence: test file present with assertion `force_bypass_dedup=true` → fresh task materialized despite existing task fixture.
- [ ] `cd watcher/github-pr && make precommit` exits 0 — evidence: exit code 0.
- [ ] `CHANGELOG.md` `## Unreleased` entry describes the rename + new endpoint — evidence: `grep -A5 '## Unreleased' CHANGELOG.md` shows entry mentioning `/check` and `/trigger?url=`.

Rung 2 — dev cluster + verification on a real PR:

- [ ] **Post-Deploy (Rung-2):** dev cluster runs the new image — evidence: `cd watcher/github-pr && BRANCH=dev make buca` (or `/make-buca`) succeeds and `kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-pr --timeout=120s` reports complete.
  - deploy_check: `kubectlquant -n dev describe statefulset maintainer-watcher-github-pr | grep 'Image:'` shows the new image tag.
  - deploy_target: dev cluster, statefulset `maintainer-watcher-github-pr`.
- [ ] **Post-Deploy (Rung-2):** `POST https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/check` triggers the poll cycle — evidence: HTTP 200; `kubectlquant -n dev logs statefulset/maintainer-watcher-github-pr --since=5m` shows poll-cycle log lines.
  - deploy_check: poll log lines visible.
  - deploy_target: dev cluster watcher pod.
- [ ] **Post-Deploy (Rung-2):** `POST https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger?url=https://github.com/bborbe/go-skeleton/pull/<N>` returns HTTP 200 with `task_id` in JSON body; a fresh vault task is materialized within 60s even when an existing same-(PR, SHA) `phase: done` task is present — evidence: `curl -X POST ...` returns 200 + JSON with `task_id`; `ls ~/Documents/Obsidian/OpenClaw/tasks/*go-skeleton*<N>*` after 60s shows a fresh `current_job` field and `trigger_count >= 1`.
  - deploy_check: pod logs show the new handler firing.
  - deploy_target: dev cluster watcher pod + OpenClaw vault.
- [ ] **Post-Deploy (Rung-2):** `POST .../trigger` without `url` returns HTTP 400 — evidence: `curl -i -X POST .../trigger` first line is `HTTP/* 400`.
- [ ] **Post-Deploy (Rung-2):** `POST .../trigger?url=<filtered-PR>` (use a known draft PR or non-allowlisted repo) returns HTTP 422 with the filter name in the body — evidence: response body JSON contains the filter name field.

Rung 3 — prod cluster (after dev verifies):

- [ ] **Post-Deploy (Rung-3):** prod cluster rolls out — evidence: `kubectlquant -n prod rollout status statefulset/maintainer-watcher-github-pr --timeout=120s` reports complete.
  - deploy_check: new image tag visible in `kubectlquant -n prod describe statefulset maintainer-watcher-github-pr | grep 'Image:'`.
  - deploy_target: prod cluster watcher.
- [ ] **Post-Deploy (Rung-3):** `POST https://prod.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger?url=https://github.com/bborbe/trading/pull/134` returns HTTP 200 + `task_id`; a fresh vault task materializes; bot posts a fresh review on the PR — evidence: `curl ... | jq .task_id` returns a UUID; `gh api '/repos/bborbe/trading/pulls/134/reviews' | jq '.[].submitted_at'` shows a new review with `submitted_at` after the cutover timestamp.
  - deploy_target: prod cluster + bborbe/trading repo.

Rung 4 — caller cleanup:

- [ ] All callers of the old `/trigger` (poll) endpoint updated to `/check` — evidence: `grep -rn '/admin/maintainer-watcher-github-pr/trigger' ~/Documents/Obsidian/ ~/Documents/workspaces/maintainer/` returns matches ONLY for the new single-PR endpoint (those include `?url=`); no naked `/trigger` references for the poll behavior remain. Kubernetes CronJob manifests (if any in `~/Documents/workspaces/maintainer/k8s/`) updated.
- [ ] Runbook updated documenting both endpoints — evidence: `grep -rn '/admin/maintainer-watcher-github-pr/' ~/Documents/Obsidian/Personal/65\ Runbooks/` returns ≥2 lines covering both `/check` and `/trigger?url=` with example `curl`.

**Scenario coverage:** NO new scenario. Unit tests in Rung 1 cover happy path + each filter branch + each error path. Live cluster verification in Rungs 2 + 3 covers the integration against the real Kafka + agent controller. A scenario file would duplicate either layer.

## Verification

```
cd watcher/github-pr && make precommit
```

Then deploy to dev via `BRANCH=dev make buca`, hit both endpoints with a real PR (e.g. an open PR on `bborbe/go-skeleton`), verify task UUID in response + fresh vault task materialized. Then deploy to prod and re-verify against `bborbe/trading#134`.

## Do-Nothing Option

Without this fix, the watcher endpoint surface continues to mislead operators ("/trigger fires one thing — wait, no, it polls everything"), every existing runbook reference to `/trigger` keeps that misleading name baked in, and the only way to retrigger a stuck PR remains manually editing OpenClaw vault frontmatter — fragile, undiscoverable, undocumented in any runbook. Every future spec-verification cycle that hits a stale task wastes 5-10 minutes on vault-editing rather than verifying the actual fix; the cost compounds. The scope is small (one rename + one new handler + one schema field + tests + runbook update) and bounded; the cost of skipping is recurring forever.
