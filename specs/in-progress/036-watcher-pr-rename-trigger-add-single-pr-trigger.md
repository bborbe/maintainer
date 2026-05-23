---
status: approved
tags:
    - dark-factory
    - spec
approved: "2026-05-23T20:50:50Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

## Summary

- The `maintainer-watcher-github-pr` admin surface exposes one endpoint, `/trigger`, that polls every configured repo — name and behavior are mismatched.
- `POST /check` replaces today's `/trigger` verbatim — runs the full poll cycle. Cron-equivalent.
- `POST /trigger?url=<pr_url>` is new — fires a single PR review by URL by routing around the in-watcher per-(PR, SHA) presence check.
- Hard cutover, no backwards compat. Solo repo; operator updates every caller in the same change set.
- **Known limit:** the new handler re-publishes a CreateTaskCommand with the deterministic task_id (UUID5 of repo#PR@sha). If the agent-task-controller already materialized that task in the vault, materialization is idempotent and no fresh task spawns. For that case, the operator still falls back to vault-frontmatter reset. Fixing controller-side dedup-bypass requires a CreateTaskCommand schema change in the `bborbe/agent` repo and is deliberately out of scope here — file a follow-up spec when the cost of that fallback becomes painful again.

## Problem

The watcher exposes a single admin endpoint, `/trigger`, that runs the full multi-repo poll cycle — identical to the 5-minute cron. Its name implies "fire one thing"; its behavior is "fire everything". Operators misread it.

There is no operator-facing path to retrigger a single PR. Closing+reopening on GitHub does not help because the watcher's vault-presence dedup keys on (PR, SHA), not on PR state. Today's only workaround is editing OpenClaw vault task frontmatter (`phase`, `status`, `trigger_count`) by hand — fragile, undocumented in any runbook, and consistently burns ~10 min mid-cycle.

## Goal

After this work, the watcher's two distinct operations have two distinct, honestly-named endpoints. `POST /check` runs the multi-repo poll (the cron-equivalent). `POST /trigger?url=<pr_url>` fires a single PR review by URL, runs the existing filter chain (drafts, bot authors, age, repo allowlist), and returns structured JSON. No `/trigger` (no-arg) endpoint survives.

## Non-goals

- Do NOT keep `/trigger` (no-arg) as a backwards-compat alias or 308 redirect.
- Do NOT change the cron schedule or the poll loop.
- Do NOT add a third "trigger by repo only" endpoint.
- Do NOT change the filter chain.
- Do NOT touch `maintainer-watcher-github-build`.
- Do NOT modify `CreateTaskCommand` or any code in `github.com/bborbe/agent`. The controller-side dedup-bypass is a separate, cross-repo concern; live with the partial bypass for now.

## Desired Behavior

1. **Extract `ParsePRURL` to a shared lib.** Move `agent/pr-reviewer/pkg/prurl.go` + `agent/pr-reviewer/pkg/prurl_test.go` → `lib/prurl/prurl.go` + `lib/prurl/prurl_test.go` (package `prurl`). Update existing `agent/pr-reviewer/pkg` callers to import `github.com/bborbe/maintainer/lib/prurl` instead of the in-package symbol. Both binaries (`agent/pr-reviewer` and `watcher/github-pr`) then import the same shared package — no duplication, no cross-binary internal-package import. Matches the project's existing shared-lib pattern (`lib/githubapp`, `lib/githubposter`, `lib/repoallowlist`).
2. `watcher/github-pr/main.go` registers `router.Path("/check")` wired to the existing poll handler — moved verbatim from the old `/trigger` route, no behavior change.
3. `watcher/github-pr/main.go` registers `router.Path("/trigger")` wired to a NEW single-PR handler. The handler:
   - reads the `url` query param
   - parses it via `lib/prurl.ParsePRURL` (the extracted shared parser from step 1)
   - fetches the real PR via the existing `GitHubClient` (so `UpdatedAt`, `HeadSHA`, draft state, author come from GitHub, not faked)
   - runs the PR through the existing `TaskCreationFilter` chain (reused as-is)
   - on filter rejection, returns HTTP 422 with the filter name + reason in the JSON body
   - on filter pass, calls the existing `publishCreate`-equivalent code path so the resulting CreateTaskCommand is byte-identical in shape to one produced by the poll path (same `buildFrontmatter` / `buildTaskBody` / `computePRTitle` / `DeriveTaskID` — reuse, do not duplicate)
4. Success response (HTTP 200): JSON `{"task_id": "<uuid>", "repo": "<owner/repo>", "pr_number": <int>, "head_sha": "<sha>"}`.
5. Failure response: JSON `{"error": "<reason>", "filter": "<filter name if filter rejection>", "pr_url": "<url echoed if present>"}`. Status: 400 (bad/missing URL), 422 (filter rejection), 502 (Kafka publish or GitHub fetch failure).
6. All existing callers of the old `/trigger` (poll) endpoint update to `/check` in the same change set: any k8s CronJob manifests under `~/Documents/workspaces/maintainer/k8s/`, runbooks under `~/Documents/Obsidian/Personal/65 Runbooks/`.
7. Runbook documents both endpoints with example `curl` and the response shape, plus the known limit (re-publish is idempotent at controller; vault-edit fallback still needed for already-materialized stuck tasks).

## Constraints

- All errors via `github.com/bborbe/errors`. No `fmt.Errorf`. No stdlib `errors.New`.
- All logging via `github.com/golang/glog`.
- BSD license header on every new/modified `.go` file.
- HTTP error responses use the existing project pattern in `watcher/github-build/pkg/reset_handler.go` (`libhttp.WrapWithStatusCode` / `NewJSONErrorHandler`) — do NOT hand-roll JSON encoding. Canonical guide: `~/Documents/workspaces/coding/docs/go-json-error-handler-guide.md`.
- Handler factory uses the project's standard shape (factory returns `http.Handler`). Canonical guide: `~/Documents/workspaces/coding/docs/go-http-handler-refactoring-guide.md`.
- Time injection: any time deps come through `CurrentDateTimeGetter` from the constructor; no `libtime.NewCurrentDateTime().Now()` inside the handler body. Canonical guide: `~/Documents/workspaces/coding/docs/go-time-injection-guide.md`.
- PR URL parsing uses `lib/prurl.ParsePRURL` (the shared parser from Desired Behavior step 1) — reuse.
- Filter chain reuses the existing `filter.TaskCreationFilter` interface — pass the existing chain into the new handler via constructor; do not rebuild.
- Reuse `pkg.publishCreate` / `buildFrontmatter` / `buildTaskBody` / `computePRTitle` / `DeriveTaskID`. If the existing `publishCreate` is bound too tightly to `processPRs`, extract a shared helper (e.g. `BuildAndPublishCreateCommand(ctx, pr, trustResult, ...)`) used by both the poll path and the new handler — do not duplicate the construction logic.
- Trust evaluation: the existing watcher branches on `trustResult` (trusted vs untrusted PR) when shaping the CreateTaskCommand. The new handler MUST run the same trust check on the fetched PR and produce the same shaped command for the same input — no silent bypass of the trust-based human-review routing.
- CHANGELOG entry under any unreleased heading (`## Unreleased`, `## [Unreleased]`, or `## Next`); if none exists, create `## Unreleased`.
- Both endpoints sit behind the admin gateway (Google OAuth). No new auth surface.
- Existing knowledge to reference:
  - Spec 026 (per-(PR, SHA) task creation) — defines the dedup invariant the new endpoint deliberately routes around at the watcher layer.
  - Spec 027 (post-verdict to GitHub PR) — bot reviews are append-only on GitHub, so dual reviews from a race are harmless.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| `POST /trigger` without `url` | HTTP 400; JSON `{"error": "url query parameter required"}` | Operator adds `?url=<pr_url>` | `curl -i` first line `HTTP/* 400` |
| `POST /trigger?url=<not-a-pr-url>` | HTTP 400; JSON body cites parse error from `prurl.ParsePRURL` | Operator corrects URL | 400 + error string in body |
| `POST /trigger?url=<pr>` where GitHub fetch fails (PR not found, GitHub 5xx) | HTTP 502; JSON cites GitHub error | Operator retries or inspects GitHub status | 502 + `glog` line |
| `POST /trigger?url=<filtered-PR>` (draft, bot author, non-allowlisted, too old) | HTTP 422; JSON body names the filter | Operator accepts the PR is intentionally filtered (filter-chain changes are out of scope for this spec — Non-goal line 32) | 422 + filter name; `glog` records rejection |
| `POST /trigger?url=<pr>` but Kafka publish fails | HTTP 502; JSON cites Kafka error | Operator inspects Kafka cluster | 502 + `glog` error |
| `POST /trigger?url=<pr>` happy path AND no prior task for this (PR, SHA) | HTTP 200 + JSON `task_id`; controller materializes fresh vault task; agent reviews | None | Response UUID; vault file appears within ~60s |
| `POST /trigger?url=<pr>` happy path BUT a task already exists for this (PR, SHA) in the vault | HTTP 200 + JSON `task_id` (same as existing); CreateTaskCommand published but controller's create-if-not-exists is idempotent → no fresh agent Job spawns | Operator resets vault frontmatter (existing workaround) OR pushes a new commit so SHA changes | Response UUID matches existing task file; no new pod in `kubectl get pods` |
| Two operators hit `/trigger?url=<same PR>` concurrently | Same task_id from both; controller materialization is idempotent; at most one agent Job spawns | None | Two HTTP 200s, same `task_id` |
| Mid-request crash between filter pass and Kafka publish | No command published | Operator re-issues request | No new task, no Kafka offset |
| `POST /check` | Runs the full multi-repo poll cycle — identical to today's `/trigger` no-arg behavior | None | Same `glog` poll-cycle lines as today |

## Security / Abuse Cases

- Both endpoints behind admin gateway (Google OAuth). No new auth surface.
- `url` is operator-controlled; `prurl.ParsePRURL` validates shape before any downstream call. No shell interpolation, no path traversal, no SSRF (the URL is parsed into structured fields; the watcher does not fetch the URL — only the parsed `owner/repo/number` is sent to the GitHub API).
- New endpoint cannot bypass the filter chain; rejection still returns 422.
- No new outbound network surface.

## Acceptance Criteria

Rung 1 — code + tests:

- [ ] `lib/prurl/prurl.go` + `lib/prurl/prurl_test.go` exist and the in-package `prurl.go` at `agent/pr-reviewer/pkg/` no longer exists — evidence: `test -f lib/prurl/prurl.go && test -f lib/prurl/prurl_test.go && ! test -f agent/pr-reviewer/pkg/prurl.go` exits 0.
- [ ] All callers reference the shared package — evidence: `grep -rn 'github.com/bborbe/maintainer/lib/prurl' agent/ watcher/ --include='*.go'` returns ≥2 matches (at least one in `agent/pr-reviewer`, at least one in `watcher/github-pr`) AND `grep -rn '"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/prurl"' agent/ watcher/ --include='*.go'` returns zero.
- [ ] `watcher/github-pr/main.go` registers `router.Path("/check")` wired to the existing poll handler — evidence: `grep -nE 'Path\("/check"\)' watcher/github-pr/main.go` returns ≥1 match, AND a follow-up `grep -A3 'Path\("/check"\)' watcher/github-pr/main.go | grep -E 'NewBackgroundRunHandler|poll'` returns ≥1 match (handle gofmt-split lines).
- [ ] `watcher/github-pr/main.go` registers `router.Path("/trigger")` wired to the new single-PR handler — evidence: `grep -nE 'Path\("/trigger"\)' watcher/github-pr/main.go` returns ≥1 match AND `grep -A3 'Path\("/trigger"\)' watcher/github-pr/main.go | grep -E 'NewBackgroundRunHandler|poll'` returns zero (the /trigger route does NOT call the poll handler).
- [ ] No `/trigger` route survives mapped to the poll handler — covered by the gate above.
- [ ] New handler in `watcher/github-pr/pkg/handler/` with table-driven unit tests covering: missing url, invalid url, GitHub fetch failure, each filter-rejection branch, valid PR happy path, Kafka publish failure. Coverage on the new file ≥80% — evidence: `cd watcher/github-pr && go test -cover ./pkg/handler/...` reports ≥80%.
- [ ] New handler builds CreateTaskCommand via the **same** helpers as the poll path (`computePRTitle`, `DeriveTaskID`, `buildFrontmatter`/`buildHumanReviewFrontmatter`, `buildTaskBody`/`buildUntrustedBody`) — evidence: `grep -nE 'computePRTitle|DeriveTaskID|buildFrontmatter|buildTaskBody' watcher/github-pr/pkg/handler/*.go` returns matches.
- [ ] Trust-branching preserved: a test asserts an untrusted-author PR goes through `buildHumanReviewFrontmatter` + `buildUntrustedBody` via the new handler, matching the poll-path shape.
- [ ] Boundary contract test: a representative PR title (containing `/`, `:`, `?`) passes `task.CreateCommand.Validate(ctx)` after going through the handler's build pipeline.
- [ ] `cd watcher/github-pr && make precommit` exits 0.
- [ ] `cd agent/pr-reviewer && make precommit` exits 0 (verifies the prurl move did not break the agent).
- [ ] CHANGELOG entry under an unreleased heading describes both the rename, the new endpoint, AND the `lib/prurl` extraction — evidence: `grep -A10 -E '^## (Unreleased|\[Unreleased\]|Next)' CHANGELOG.md` shows entries mentioning `/check`, `/trigger?url=`, and `lib/prurl`.

Rung 2 — dev cluster:

- [ ] **Post-Deploy (Rung-2):** dev cluster runs the new image — `cd watcher/github-pr && BRANCH=dev make buca` succeeds; `kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-pr --timeout=120s` reports complete.
  - deploy_check: `kubectlquant -n dev describe statefulset maintainer-watcher-github-pr | grep 'Image:'` shows the new image tag matching the just-built `develop-*` tag.
  - deploy_target: dev cluster statefulset `maintainer-watcher-github-pr`.
- [ ] **Post-Deploy (Rung-2):** `POST https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/check` triggers the poll cycle — HTTP 200; pod logs show poll-cycle lines.
  - deploy_check: `kubectlquant -n dev logs statefulset/maintainer-watcher-github-pr --since=2m` shows the standard poll-cycle log lines.
  - deploy_target: dev cluster watcher pod.
- [ ] **Post-Deploy (Rung-2):** `POST .../trigger?url=https://github.com/bborbe/go-skeleton/pull/<N>` against an open PR with NO existing vault task returns HTTP 200 + `task_id`; vault task materializes within 60s.
  - deploy_check: response JSON contains `task_id`; `ls ~/Documents/Obsidian/OpenClaw/tasks/*go-skeleton*<N>*.md` shows the fresh task file within 60s.
  - deploy_target: dev cluster watcher + OpenClaw vault.
- [ ] **Post-Deploy (Rung-2):** `POST .../trigger` without `url` returns HTTP 400.
  - deploy_check: `curl -i -X POST .../trigger` first line is `HTTP/* 400` and body is the documented JSON error shape.
  - deploy_target: dev cluster watcher.
- [ ] **Post-Deploy (Rung-2):** `POST .../trigger?url=<filtered-PR>` (draft or non-allowlisted) returns HTTP 422 with the filter name in the body.
  - deploy_check: response body JSON contains the `filter` field naming the rejecting filter.
  - deploy_target: dev cluster watcher.

Rung 3 — prod cluster:

- [ ] **Post-Deploy (Rung-3):** prod cluster rolls out — `kubectlquant -n prod rollout status statefulset/maintainer-watcher-github-pr --timeout=120s` reports complete.
  - deploy_check: `kubectlquant -n prod describe statefulset maintainer-watcher-github-pr | grep 'Image:'` shows the new image tag.
  - deploy_target: prod cluster statefulset `maintainer-watcher-github-pr`.
- [ ] **Post-Deploy (Rung-3):** `POST https://prod.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger?url=<open prod PR with no prior task>` returns HTTP 200 + `task_id`; vault task materializes; bot posts a review.
  - deploy_check: response JSON contains `task_id`; `gh api '/repos/<owner>/<repo>/pulls/<N>/reviews' | jq '.[].submitted_at'` shows a new review with `submitted_at` after the cutover timestamp.
  - deploy_target: prod cluster watcher + target GitHub repo.

Rung 4 — caller cleanup:

- [ ] All in-repo callers of the old `/trigger` (poll) endpoint updated to `/check` — evidence: `grep -rn '/admin/maintainer-watcher-github-pr/trigger' ~/Documents/workspaces/maintainer/` returns matches ONLY for the new single-PR endpoint (those include `?url=`); no naked `/trigger` references for the poll behavior remain in the maintainer repo.
- [ ] **Host-side (manual, not container-verifiable):** Obsidian runbook callers updated — evidence: `grep -rn '/admin/maintainer-watcher-github-pr/trigger' ~/Documents/Obsidian/` (run on the host, not in the dark-factory container) returns matches ONLY where `?url=` follows; any naked `/trigger` references for the poll behavior get rewritten to `/check`. Note: this AC is host-only because the Obsidian vault is not mounted in the container.
- [ ] Runbook updated documenting both endpoints with example `curl` AND the known limit ("if vault task already materialized, fall back to vault-frontmatter reset") — evidence: `grep -rn '/admin/maintainer-watcher-github-pr/' ~/Documents/Obsidian/Personal/65\ Runbooks/` (host-side) returns ≥2 lines covering both endpoints.

**Scenario coverage:** NO new scenario. Unit tests cover happy path + each filter branch + each error path. Live cluster verification in Rungs 2 + 3 covers integration. A scenario file would duplicate either layer.

## Verification

```
cd watcher/github-pr && make precommit
```

Then deploy to dev via `BRANCH=dev make buca`, hit both endpoints against a fresh `bborbe/go-skeleton` PR with no prior vault task, verify task UUID in response + fresh vault task materialized. Deploy to prod, repeat against a fresh prod PR.

## Do-Nothing Option

Without this fix the `/trigger` name keeps misleading operators forever and the only path to "retrigger this PR" stays "manually edit OpenClaw vault frontmatter". The work is bounded (one rename + one new handler reusing existing builders + tests + runbook). The deliberate dedup-bypass cut-out keeps the change inside this repo and avoids the cross-repo `bborbe/agent` schema change that killed the first attempt — that schema change can be its own spec when the partial bypass proves insufficient.
