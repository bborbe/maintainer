---
status: approved
approved: "2026-05-06T20:25:29Z"
branch: dark-factory/richer-build-task-context
---

## Summary

- Today's build-failure vault task body has only the repo name, episode SHA, and a list of failing-workflow links — operators must click through to GitHub to see what actually broke
- Add structured context the watcher can pull from the GitHub Actions API: commit subject, author/event, started/finished/duration, and the **failed step name** for each failing job
- All fields here are sourced from existing API responses (`runs` endpoint already used; `jobs` endpoint adds one cheap call per failing run) — no `/logs` zip, no token-leak surface
- A separate opt-in flag `include_logs: true` in `.maintenance.yaml` enables an additional log-snippet section with redaction; default OFF until the redaction story is proven safe per repo
- 100% watcher-side change — the body markdown is already constructed by the watcher's `buildCreateTaskCommand`; no controller / cross-repo coordination

## Problem

Operator opens [a build-failure vault task](obsidian://open?vault=OpenClaw&file=tasks%2FBuild%20Failure%20github%20-%20bborbe-maintainer%20-%205886450) and sees:

```markdown
# Build Failure: bborbe/maintainer
Episode SHA: `5886450...`
## Failing Workflows
- [CI](.../runs/25456589225)
```

To answer "what broke?" they have to follow the link, navigate jobs, find the failing step, scroll through 100s of log lines. Friction multiplies as more repos onboard. Most failure categories (lint, test segfault, dep-update, compile error) are determinable from a 1-line failed-step name + a 30-line log snippet — turning a 60-second click-through into a 3-second skim.

## Goal

After this work ships, every build-failure task body answers three questions without the operator leaving Obsidian:

1. **What commit broke it** (subject + author + event + duration)
2. **Where did it break** (which workflow → which job → which step)
3. **What did it say** (last N lines of the failed step's log, redacted, opt-in per repo)

## Desired Behavior

1. Task body header includes commit subject, branch, event, started/finished timestamps, and elapsed duration — all from fields already present in the `WorkflowRun` API response (zero extra API calls)
2. A `## Failing Workflows` table replaces today's bullet list, columns: workflow → job → failed step → run link
3. Failed-step names come from a single `GET /repos/{owner}/{repo}/actions/runs/{id}/jobs` call per failing run (small JSON payload; no logs, no zip)
4. When `.maintenance.yaml`'s `watcher.github-build.include_logs: true` is set for a repo, an `## Error` section appears in the body containing the last 30 lines of the failed step's log, fenced as a code block, with sensitive-pattern redaction applied
5. Default `include_logs: false` — repos that haven't opted in see no log snippet (matches today's behavior + adds the table + header context)
6. Log fetch failures (rate limit, 5xx, timeout) → omit the `## Error` section, log a WARN, do NOT block the publish — the body still ships with the rich context that didn't need logs

## Non-goals

- Changing the filename — separate spec `human-readable-filenames-for-build-tasks`
- Fetching commit message body (only the subject line of `head_commit.message` / the `display_title` field is included; longer commit bodies bloat the task)
- Linking to specific log line numbers — runs are large; a snippet is enough
- Cross-job failure aggregation across multiple failing workflows — list each workflow as a row in the table; the operator scans them all
- Persisting fetched data outside the vault task body — no separate cache, refetch each publish (publishes are rare per the cursor's episode-locking)
- Including PR data (only build/CI is in scope; PR-review tasks are a separate watcher)

## Constraints

- ALL new fields MUST be optional in the body — if any single API call fails, the publish proceeds with whatever context was successfully fetched
- The `jobs` API call MUST be issued at most ONCE per failing run per publish (don't re-fetch if multiple steps in the same job failed; one run = one jobs call)
- `include_logs: true` MUST be honored from `.maintenance.yaml` (per-repo opt-in via spec-017 mechanism); the watcher CLI/env defaults this to `false` and there is NO CLI/env override that turns it on globally — operator-side opt-in is per-repo only
- Raw log payload MUST be rejected as suspicious if larger than 1 MB BEFORE truncation (likely an unbounded script dump; failure mode covered below); on rejection, omit `## Error` section and WARN log
- Log snippet MUST be capped at the last 30 lines OR 4 KB, whichever is smaller — measured AFTER redaction
- Redaction MUST run a regex pass that strips, in this exact set (no "TBD" addenda — extend via a follow-up spec if a future leak shape is observed):
  - GitHub tokens: `gh[opsu]_[a-zA-Z0-9]{16,}`
  - Bearer headers: `Bearer\s+[A-Za-z0-9._-]{16,}` → `Bearer [REDACTED]`
  - AWS access key IDs: `AKIA[0-9A-Z]{16}`
  - AWS secret access keys (heuristic — base64-ish 40 chars after `aws_secret_access_key` literal in stdout): `aws_secret_access_key[\s=:]+["']?[A-Za-z0-9/+]{40}["']?` → keep the prefix, redact the secret
  - Long opaque hex strings: `\b[a-f0-9]{40,}\b` → `[REDACTED]` (catches generic auth hashes; false-positive on commit SHAs is acceptable since the body already shows the commit SHA in plain header)
- The redaction regex set above is the FULL set for v1 — no "TBD" patterns. Future leak shapes ship as a follow-up spec, not a code change against this one.
- Body output MUST be deterministic for the same `(run_id, jobs response, log content)` inputs — no `fetched_at` timestamp embedded in the body, no random IDs (re-publishes for the same episode SHA never happen at runtime per spec-015's cursor lock, but determinism is required for testability and for golden-file body assertions)
- Watcher uses the same `ctx` for the new API calls as the rest of the poll cycle (no `context.Background()`)
- All errors wrapped via `github.com/bborbe/errors`; never `fmt.Errorf`; never `bare return err`
- Existing `buildCreateTaskCommand` tests MUST be updated; new tests cover: free-field rendering, jobs API success + failure, log opt-in true + false, redaction patterns

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `runs` API rate-limited | Already handled by spec-015 (`ErrRateLimited` aborts the poll cycle); no change | unchanged |
| `jobs` API rate-limited for one run | Skip the failed-step column for that row (show `?` or omit); WARN log; publish proceeds | next poll retries |
| `jobs` API 5xx | Same as rate-limited — degraded body, publish proceeds | transient |
| `logs` fetch fails (any reason) when `include_logs: true` | Omit `## Error` section; WARN log; publish proceeds | n/a — body is still richer than today |
| Log size > 1 MB | Reject as suspicious (likely a script dumping unbounded output); WARN; omit `## Error` | repo-side fix |
| Redaction regex matches false-positive (e.g. a legitimate hex hash in test output) | Redacted as `[REDACTED]` | acceptable — small UX cost vs leak risk |
| `.maintenance.yaml` malformed (already covered by spec-017 fall-through) | Use defaults (`include_logs: false`); existing WARN | unchanged |
| Two failing workflows on the same run | Table has one row per workflow; jobs/steps under each — single jobs API call covers all of them | unchanged |

## Security / Abuse

- **Log content is the only new data path that crosses a security boundary.** Logs commonly include tokens that leaked into stdout during CI (debug output, env var dumps, `set -x` traces). The vault repo is git-versioned and may be pushed to a remote — leaked tokens become permanent.
- **Mitigations**:
  - `include_logs` defaults to `false`; opt-in is per-repo via `.maintenance.yaml`, so the operator explicitly takes responsibility for one repo at a time after auditing its CI for secret leakage
  - Regex redaction pass strips known token shapes before the snippet enters the body
  - 4 KB / 30-line cap limits exposure window
  - Snippet is the LAST N lines — most leak-prone debug output happens at the start of CI (env dumps); the failure tail is more likely to be the actual error
- **Residual risk**: regex can't catch every token shape; an operator-misconfigured CI step could still dump unrecognized secrets. The opt-in default means this is not exposure-by-default. Not auto-redacting commit SHAs is intentional (false-positive) — cap the worst-case at "operator sees `[REDACTED]` instead of a SHA in the snippet".
- **Token-bearing logs scope**: only the build-watcher's GitHub token reads logs; that token is already read-only on the same repo set, so the watcher cannot escalate by reading logs.

## Acceptance Criteria

- [ ] Task body header includes commit subject (`display_title`), branch, event, started/finished timestamps, duration — observable on a test publish for `bborbe/maintainer`
- [ ] Task body has a `## Failing Workflows` table (replacing today's bullet list) with columns: workflow / job / failed step / run link
- [ ] Failed step names come from one `jobs` API call per failing run (verified by counting outbound API calls in a fixture test)
- [ ] When `.maintenance.yaml` has `watcher.github-build.include_logs: true`, the body has an `## Error` section with the last 30 lines (≤ 4 KB) of the failed step's log, fenced
- [ ] When the flag is absent or `false`, NO `## Error` section appears
- [ ] Redaction strips ALL patterns from the Constraints set: `gh[opsu]_*`, `Bearer ...`, `AKIA*`, `aws_secret_access_key=*`, and 40+-char hex strings — verified by unit test fixtures, one per pattern
- [ ] All non-fatal API failures (jobs 5xx, logs fetch error, rate limit on the jobs endpoint) result in a degraded body but the publish itself succeeds
- [ ] No publish path is ever blocked by a missing optional field
- [ ] CHANGELOG entry under `## Unreleased`
- [ ] `make precommit` clean
- [ ] Rung-2 verification per `docs/verifying-specs.md`: dev-cluster pod produces a vault file with the new fields populated

## Verification

```bash
cd watcher/github-build && make precommit
```

Live verification (rung 2):

```bash
# Default (no log section):
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- rm -f /data/cursor.json
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- wget -qO- http://localhost:9090/trigger

# Confirm body has commit subject + table + step name, NO ## Error section:
cd ~/Documents/Obsidian/OpenClaw && git pull --quiet
grep -E "Title:|## Failing Workflows|Failed Step|## Error" "tasks/Build Failure github"*

# Opt-in for one repo: add to bborbe/maintainer .maintenance.yaml then re-trigger:
#   watcher:
#     github-build:
#       include_logs: true
# After the publish, the body should include `## Error` with redacted log snippet.
```

## Do-Nothing Option

Leave the body as a bare list of run-URL bullets. Cost: every triage requires a click-through to GitHub Actions UI. Acceptable when the watcher only sees a few repos; degrades quickly as the auto-fix loop scales. The richer body is purely additive — failures in fetching the new context degrade gracefully, so there is no regression risk to the existing minimal body.

## Related

- Companion spec (filename): `human-readable-filenames-for-build-tasks.md`
- Builds on: spec-015 (build watcher MVP), spec-017 (`.maintenance.yaml` per-repo overrides — supplies `include_logs`)
- Pipeline overview: `docs/architecture.md`
- Episode-SHA semantics: `docs/build-watcher.md`
- Verification ladder: `docs/verifying-specs.md`
