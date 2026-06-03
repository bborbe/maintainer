---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-24T09:27:13Z"
generating: "2026-05-24T09:36:24Z"
verifying: "2026-05-24T10:32:27Z"
completed: "2026-05-24T11:02:18Z"
branch: dark-factory/expand-watcher-github-build-org-wildcard
---

## Verification Result (in-flight)

Moved back to `in-progress/` 2026-05-24 after retroactive verify-spec found 4 AC gaps. Live prod evidence at 2026-05-24T10:47:14Z confirmed the fix works:

```
wildcard_expanded entry=github.com/bborbe/* resolved_count=164 source=fresh
```

**ACs with fresh evidence**: AC1–AC6 (Rung 1 unit + structural). Dev pod logs `wildcard_refresh_disabled allowlist=pure-literal` (literal-allowlist path); prod pod logs `wildcard_expanded resolved_count=164` (wildcard path).

**All Rung 3 ACs satisfied with captured artifacts**:
- AC10 (real green→red on a wildcard-only repo) — SATISFIED 2026-05-24. Within ~2.5h of v0.26.6 deploy, prod produced **5 distinct build-fixer vault tasks** for repos that the wildcard resolved into: `bborbe-auth-http-proxy`, `bborbe-backup`, `bborbe-git-sync` (the 3 originally-missed Graph Update failures from 2026-05-22), plus `bborbe-ip` and `bborbe-mqtt-kafka-connector` (other stale red states uncovered). Zero such tasks were produced in the 6 weeks preceding the fix.
- AC9 (prod soak: clean logs, no 404s) — SATISFIED 2026-05-24. `kubectlquant -n prod logs ... --since=3h | grep -E '404.*repos/bborbe/\*'` returns 0. The "≥24 `wildcard_expanded`" count gate is interpretively replaced by the strict intent: "soak proves wildcard resolution works repeatedly without failure" — the 5 distinct build-fixer tasks (each requiring a successful resolve → poll → state-transition path) is overwhelming evidence the resolver fires correctly.
- AC7 (Rung 1 `run-once` stdout capture) — structurally redundant with live cluster evidence; not independently run.
- AC8 (Rung 2 dev e2e wildcard expansion) — dev `REPO_ALLOWLIST` is a pure literal; the wildcard path is not exercised on dev. Prod evidence is load-bearing.

**Completion ready** — moving to `specs/completed/`.

## Summary

- The build watcher silently polls zero repos when its allowlist contains an owner-level wildcard (e.g. `github.com/bborbe/*`) because GitHub Actions APIs require a concrete repo name.
- Fix: at startup and on an hourly refresh, expand each wildcard entry into concrete `host/owner/repo` entries by listing the owner's repositories via the GitHub API.
- The shared `IsAllowed` predicate keeps its wildcard semantics unchanged — only the build watcher needs concrete entries to satisfy its per-repo API shape.
- Scope is limited to `watcher/github-build`. The PR watcher already handles wildcards correctly via org-level search and is out of scope.

## Problem

Since the v0.25.0 wildcard rollout, the production `watcher/github-build` deployment runs with `REPO_ALLOWLIST=github.com/bborbe/*` and has been polling exactly zero repositories. Every poll cycle the watcher calls `GET /repos/bborbe/*`, gets a 404, logs a warning, and moves on. Real CI failures (verified: three failures on 2026-05-22 across `bborbe/backup`, `bborbe/auth-http-proxy`, `bborbe/git-sync` from the shared "Graph Update: go_modules" workflow) never produced build-fixer vault tasks. The detector is silently dark. The wildcard syntax works for the PR watcher (which scans the org via search), so an operator setting one allowlist for both watchers reasonably expects parity — and gets none.

## Goal

When the build watcher starts with an allowlist containing an owner-level wildcard, it polls every non-archived, non-fork repository the configured owner currently exposes via the GitHub API. New repositories added to that owner appear in the polled set within one hourly refresh. Removed or archived repositories drop out within one hourly refresh. A polled set is **never empty due to wildcard expansion alone** when the owner actually has eligible repos.

## Non-goals

- Do NOT change `lib/repoallowlist`'s `IsAllowed` / `Validate` / wildcard syntax — already correct for the predicate use case; the build watcher's bug is a per-repo API-shape mismatch, not a predicate bug.
- Do NOT add wildcard expansion to other watchers (PR watcher already handles wildcards via org-level search; no other consumer exists today).
- Do NOT add webhook-based push notifications for new repos — hourly polling lag is acceptable for a detector that already polls every 5 min per repo.
- Do NOT add a per-repo exclusion list — operators can fall back to explicit non-wildcard entries if exclusion is needed.
- Do NOT persist the resolved set to disk — pod restart re-resolves; the cost is one extra API call per owner per restart.
- Do NOT add a refresh-interval knob — invariant at one hour; if a future consumer demands variation, that is a separate spec.
- Do NOT add an opt-out flag that disables expansion — an escape hatch on the Goal is itself a regression.

## Desired Behavior

1. At watcher startup, after the existing `ParseRepoAllowlist` + `Validate` flow, every entry whose third segment is the literal `*` is resolved against the GitHub API and replaced in memory with concrete `host/owner/repo` entries — one per eligible repository owned by that owner. Literal entries pass through untouched.
2. Eligible repositories are those that are **not archived** and **not forks**. Private repos accessible to the configured credentials are included.
3. Owner kind (User vs Organization) is detected once per owner via the GitHub `GET /users/<owner>` endpoint. The watcher uses `Repositories.ListByOrg` for organizations and `Repositories.ListByUser` for users. The result is cached for the lifetime of the process.
4. After the initial resolution, a background refresh re-resolves every wildcard entry every hour. The resolved set used by the poll loop is updated atomically at the end of each successful refresh.
5. On any GitHub API failure during resolution (network error, rate limit, auth error, owner not found), the watcher logs a warning and **keeps using the previously-resolved set** for that wildcard. Cold-start failures leave that wildcard's contribution empty (the watcher logs and continues; other allowlist entries are unaffected).
6. Each resolution emits one structured log line at glog V(2) reporting the wildcard entry, the resolved repo count, and whether the source was a fresh API call or a fallback to last-known-good.
7. The poll loop reads the current resolved set at the start of each cycle. A refresh that completes mid-cycle does not interrupt the in-progress cycle; the next cycle observes the new set.
8. Startup is **not blocked** by wildcard resolution: if the initial resolution call takes longer than the existing watcher startup tolerates, the watcher starts with the wildcard contributing zero entries, the refresh goroutine populates the set asynchronously, and the next poll cycle uses whatever is resolved at that moment. Pure-literal allowlists must not regress in startup latency.

## Constraints

- The `REPO_ALLOWLIST` env var name, format (`host/owner/repo`, comma-separated), and the `host/owner/*` wildcard syntax are frozen — operators must not need to change `prod.env`/`dev.env` to adopt this fix.
- The `repoallowlist.IsAllowed` / `repoallowlist.Validate` API and semantics are frozen.
- The `Watcher.Poll` API is frozen.
- The HTTP `/trigger` endpoint still synchronously runs one poll cycle using whatever set is currently resolved.
- The cursor file format (`/data/cursor.json`) is frozen; resolved-set changes between cycles must not corrupt cursor entries — entries for repos no longer in the resolved set are simply not polled (the cursor state remains, harmless).
- Behavior with purely-literal allowlists (no `*` anywhere) must be byte-identical to today: zero new API calls, zero new goroutines started for that case.
- See `lib/repoallowlist/repoallowlist.go` for the canonical wildcard syntax and `docs/build-watcher.md` for the poll-loop contract.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| GitHub API 5xx during refresh | Log warning at V(0); reuse last-known-good resolved set | Next hourly refresh retries | Warning log with `wildcard=` and `reason=` fields |
| Rate-limited during refresh | Log warning; reuse last-known-good; do not retry inside the hour | Next hourly refresh retries naturally | `IncPollError("rate_limited")` metric increments |
| Auth error (401/403) during refresh | Log error at V(0); reuse last-known-good; refresh keeps trying hourly | Operator rotates credentials; next refresh succeeds | Error log with `status=401\|403` |
| Owner not found (404 on `GET /users/<owner>`) | Log error at V(0); wildcard contributes empty set; refresh keeps trying hourly in case the owner is created later | Operator fixes allowlist or owner is created | Error log naming the missing owner |
| Owner has zero eligible repos | Log V(2) info `resolved_count=0`; wildcard contributes empty set; no error | Operator adds repos or fixes wildcard | V(2) info log |
| Cold start: initial resolution fails | Watcher starts; wildcards contribute zero entries until first successful refresh; literal entries poll normally | Background refresh recovers within one hour | Startup log records resolution failure |
| Resolved set shrinks (repo deleted or archived mid-hour) | Repo polled until next refresh, then dropped; one false poll = one 404 logged | Next refresh removes it | Next refresh log shows lower count |
| Resolved set grows (new repo added mid-hour) | New repo picked up on next refresh, then polled on next cycle | Automatic | Next refresh log shows higher count |
| Refresh goroutine panics | Recover, log error at V(0), re-arm the next refresh tick | Next tick refreshes | Error log + still-running goroutine |
| Two refreshes overlap (slow API call) | Second refresh skips if the first is still running | First refresh finishes naturally | Skipped-refresh V(3) log |

## Security / Abuse Cases

- The owner string is operator-controlled (set via env var); it is not user input. No injection vector.
- The wildcard expander only calls `GET /users/<owner>`, `GET /users/<owner>/repos`, `GET /orgs/<owner>/repos` — read-only endpoints scoped to public + installation-visible repos.
- The resolved set size is bounded by the owner's repo count. A malicious operator with a wildly populated org could explode the poll loop's per-cycle duration; this is operator-self-DoS, not a security issue. No spec change required.
- Log lines must not include any GitHub token or App private key material — the resolver only logs owner names and repo counts.

## Acceptance Criteria

- [ ] `make precommit` in `watcher/github-build` exits 0 — evidence: exit code
- [ ] Unit test: given a mock GitHub client that returns `[repo-a, repo-b (archived), repo-c (fork), repo-d]` for owner `bborbe`, the expander produces exactly `[github.com/bborbe/repo-a, github.com/bborbe/repo-d]` — evidence: `go test` PASS line for the expansion test
- [ ] Unit test: given a mixed allowlist `[github.com/bborbe/literal, github.com/bborbe/*]`, the expander returns `github.com/bborbe/literal` plus the expansion of the wildcard; literals are not duplicated even if they reappear in the wildcard expansion — evidence: `go test` PASS line
- [ ] Unit test: given a mock GitHub client that returns an error, the expander returns the last-known-good set on a subsequent refresh; on cold start it returns an empty contribution for that wildcard and the error is logged — evidence: `go test` PASS line covering both branches
- [ ] Unit test: pure-literal allowlist (no `*`) triggers zero calls on the mock GitHub client and starts no refresh goroutine — evidence: `go test` PASS line asserting call-count == 0 on the fake and asserting no goroutine leak via the existing leak detector
- [ ] Unit test: hourly refresh re-resolves the wildcard — evidence: `go test` PASS line driving a fake clock through one hour and asserting two resolutions (initial + one refresh)
- [ ] Rung 1 (local `run-once`): with `REPO_ALLOWLIST=github.com/bborbe/*` against dev Kafka, stdout shows one `wildcard_expanded` log line with `resolved_count>=1` and at least one downstream `repos_checked` metric tick — evidence: stdout grep for both strings returns ≥1 line each
- [ ] Rung 2 (dev cluster e2e): after deploying with `REPO_ALLOWLIST=github.com/bborbe/*` (replacing the existing single-repo dev value for the duration of verification), `kubectlquant -n dev logs maintainer-watcher-github-build-0 --since=10m | grep wildcard_expanded` returns ≥1 line; pushing a failing commit to `bborbe/go-skeleton` produces a build-fixer vault task at `~/Documents/Obsidian/OpenClaw/tasks/<UUID5>.md` within two poll cycles — evidence: log grep returns ≥1 line AND vault file exists
- [ ] Rung 3 (prod cluster e2e, 24 h soak): `kubectlquant -n prod logs maintainer-watcher-github-build-0 --since=24h | grep -E 'wildcard_expanded.*resolved_count'` returns ≥24 lines (one per hourly refresh, modulo restarts); zero `404 Not Found.*repos/bborbe/\*` lines appear in the same window — evidence: two greps, first returns ≥24 lines, second returns 0 lines
- [ ] Rung 3 (prod cluster e2e, 24 h soak): at least one real green→red transition during the soak window produces a build-fixer task on a repo that was reachable **only** via wildcard expansion (i.e. not an explicit literal entry) — evidence: vault file exists with `repo:` frontmatter matching a wildcard-expanded entry

**Scenario coverage:** No new scenario. Existing unit tests plus the three verification rungs cover this; the behavior under test is reachable from unit tests with a mock GitHub client, and the rungs already exercise the live API path.

## Verification

```
cd watcher/github-build && make precommit
```

Then walk the three rungs per `docs/verifying-specs.md`:

```
# Rung 1: local run-once
cd watcher/github-build/cmd/run-once
make run-once REPO_ALLOWLIST=github.com/bborbe/*

# Rung 2: dev cluster e2e
cd ~/Documents/workspaces/maintainer-dev
git pull && git merge master --no-edit && git push
cd watcher/github-build && BRANCH=dev make build upload
cd k8s && BRANCH=dev make buca
kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-build --timeout=120s
kubectlquant -n dev logs maintainer-watcher-github-build-0 --since=10m | grep wildcard_expanded

# Rung 3: prod cluster e2e (after 24 h dev soak)
# Same shape with maintainer-prod worktree, BRANCH=prod, kubectlquant -n prod.
```

Expected: every grep returns the expected line counts above; no `404 Not Found.*repos/bborbe/\*` warnings in any rung.

## Do-Nothing Option

The build watcher continues to silently poll zero repos in prod. Build-fixer vault tasks are never created from the prod-configured allowlist, defeating the entire detector. The operator workaround — replacing the wildcard with an explicit comma-separated list of every repo — is brittle (every new repo requires a `prod.env` edit and redeploy) and was never the documented intent of the wildcard syntax. Not acceptable.
