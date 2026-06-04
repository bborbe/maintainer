---
status: completed
approved: "2026-05-27T20:32:51Z"
generating: "2026-05-27T22:07:25Z"
prompted: "2026-05-27T22:07:25Z"
verifying: "2026-05-27T22:10:15Z"
completed: "2026-06-04T15:09:17Z"
branch: dark-factory/github-release-watcher-implementation
---

# github-release-watcher-implementation

## Summary

- Fill in the `watcher/github-release/` Go skeleton so it produces real Kafka `CreateTaskCommand` events for repos with non-empty `## Unreleased` entries
- Same task contract Phase 1 slash-command prototype validated end-to-end on `bborbe/docker-utils v1.7.8` — frontmatter shape, deterministic task_identifier, body conventions all carry verbatim
- No new capabilities beyond Phase 1; this spec is pure Stage-2 productionization per [[Agent Development Workflow]] § Stage 2
- Skeleton already lives on `feat/github-release-watcher` (commits `1bc199b` through `14fe3c8`) with TODO stubs, type signatures, and godoc pointing at the existing `watcher/github-pr` analogues — each implementation prompt is translation work, not greenfield design
- Sister spec `github-release-watcher-deploy` (separate) covers the post-deploy / rung-2 evidence that the watcher emits a real task against the dev cluster

## Problem

The Phase 1 slash-command prototype proved the watcher↔agent task contract end-to-end but requires the operator to remember to run `/github-unreleased-repo-watcher` manually. Across the ~30-repo bborbe fleet opted out of dark-factory's `autoRelease: true` path (the autoRelease daemon hits branch-protection rejections on every protected master), manual scanning is unreliable: releases sit on `## Unreleased` for days, the BRO-20203 lib-migration follow-up batch can't fire-and-forget, and operator attention is spent on a deterministic mechanical scan. Translating the validated prototype to a long-running Go service that polls + emits per cycle removes the operator from the loop while keeping every contract decision Phase 1 made.

## Goal

A compiled, tested `watcher/github-release/` Go service whose `make precommit` is clean, whose unit tests cover the four pure-Go modules (changelog parser, task ID, cursor, filter chain) with ≥80% coverage, whose `Watcher.Poll` cycle correctly ties source query → filter → publish → cursor save together under a mocked `GitHubClient`, and whose `TaskPublisher.BuildCreateCommand` output for any given `(owner, repo, head_sha)` is bit-for-bit semantically equivalent to the Phase 1 slash-command output for the same inputs.

## Non-goals

- Detecting which repos to release (the watcher emits one task per affected repo — the agent decides whether to release)
- PR + auto-merge fallback (agent territory per [[Agent Task File Contract]])
- AI bump classification (agent territory)
- Mono-repo handling (single root `CHANGELOG.md` only)
- Kafka result-delivery on the watcher side (watcher is producer-only; agent emits results)
- Dev cluster deployment + e2e evidence — that's `github-release-watcher-deploy` (sister spec)
- Multi-org support (single `OWNER` env per deployment; multi-org = multiple deployments)
- Counterfeiter mock regen workflow change — uses existing `make generate` target

## Desired Behavior

1. **CHANGELOG parsing** returns the correct `ChangelogSummary` for representative inputs: canonical Keep-a-Changelog ordering (`## Unreleased` first), inverted ordering (`## Unreleased` at bottom), empty Unreleased (header without bullets), missing `## Unreleased`, mixed header prefix style (`## 1.2.6` vs `## v1.2.6`).
2. **Task identifier** is a deterministic UUID5 derived from `(owner, repo, head_sha)`; same inputs always yield the same UUID. The namespace is a fixed constant — changing it breaks controller dedup, so it MUST stay stable across releases.
3. **Cursor persistence** round-trips `Cursor.Repos[repoKey].LastSeenMasterSHA` via atomic temp-file write + rename; missing file returns a fresh empty cursor (cold-start is valid); corrupt JSON returns an error (refuses to proceed; matches `watcher/github-build` policy).
4. **Filter chain** skips releases per the four predicates: `EmptyUnreleasedFilter` (UnreleasedBullets == 0), `AutoReleaseFilter` (AutoRelease == true), `SHAUnchangedFilter` (cursor entry equals current HeadSHA), `RepoAllowlistFilter` (carried verbatim — RepoKey not in allowlist; empty allowlist = allow-all).
5. **GitHub client** wraps `*http.Client` (App-auth path from `factory.CreateGitHubAppClient`, PAT fallback) and implements four methods: `ListRepos(owner)`, `GetMasterSHA(repo)`, `GetChangelogContent(repo)` (returns `(nil, nil)` on 404), `GetAutoReleaseConfig(repo)` (returns `(false, nil)` on 404). 5xx surfaces as wrapped error; rate-limit response (403 with `X-RateLimit-Remaining: 0`) surfaces as a distinguishable wrapped error so the watcher can label its metric correctly.
6. **TaskPublisher.BuildCreateCommand** produces a `task.CreateCommand` whose `Frontmatter` matches the contract per [[Agent Task File Contract]]: `task_type: github-release`, `assignee: github-releaser-agent`, `phase: planning`, `status: in_progress`, `stage` (from config), `task_identifier` (UUID5), `title` (`Release <owner>-<repo> <sha[:7]>` — dash separator, NO `/`. The `agent/lib` `CreateCommand` validator rejects `/` in titles; rung-1 verification 2026-05-28 surfaced this. The Phase 1 vault file's `title: Release bborbe/docker-utils at d630ef3` is NOT evidence of the production contract — it was written directly by the slash command which bypassed the validator.), `repo` (`owner/name`), `clone_url` (`git@github.com:owner/name.git`), `ref` (full HeadSHA), `current_version`. Body is the operator-readable header only (title + version + HEAD + changelog URL + repo link) — no `## Unreleased` bullets embedded.
7. **Watcher.Poll** runs one cycle: load cursor (cold-start safe) → `ListRepos(owner)` → for each repo (sequential default; per-repo parallelism — agent decides at impl time; default sequential) gather `(HeadSHA, ChangelogContent → ChangelogSummary, AutoReleaseConfig)` → assemble `filter.Release` → apply chain → on non-skip call `TaskPublisher.PublishCreate` → on successful publish update `cursor.Repos[repoKey].LastSeenMasterSHA` → save cursor → emit `IncPollCycle("success")`. On any GitHub 5xx during repo listing or per-repo fetch the cycle aborts WITHOUT cursor save and emits `IncPollCycle("github_error")` or `IncPollCycle("rate_limited")` accordingly.

## Constraints

- Mirror `watcher/github-pr` Go patterns verbatim where they exist: `errors.Wrapf(ctx, err, ...)` for error wrapping (never `fmt.Errorf` for production paths), `glog.V(N).Infof` for structured logs, counterfeiter-generated mocks in `mocks/`, Ginkgo v2 + Gomega for tests, external `_test` packages
- Frontmatter contract is FROZEN per [[Agent Task File Contract]] and Phase 1 evidence (`24 Tasks/Release bborbe-docker-utils d630ef3.md`). Any deviation breaks contract parity and produces tasks the agent cannot consume
- `task_identifier` namespace UUID stays stable across releases — changing the constant generates new UUIDs for the same `(owner, repo, head_sha)` and breaks controller dedup
- `lib/repoallowlist` carried verbatim — no domain logic change in `repo_allowlist_filter.go`
- Cursor file format is JSON via `encoding/json`; atomic write via `os.WriteFile(path+".tmp", 0600)` then `os.Rename`. Stays compatible with existing `/data/cursor.json` PVC mount path
- Mocks regenerate via existing `make generate` target — no new tooling
- No `context.Background()` in production paths — context flows from `Run(ctx)` per `coding-guidelines/go-context-cancellation-in-loops.md`
- Pre-init Prometheus counter label combinations to `.Add(0)` per `coding-guidelines/go-prometheus-metrics-guide.md` so labels appear before first event
- No new capabilities beyond Phase 1 prototype (Stage 2 anti-pattern guard per [[Agent Development Workflow]])

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---|---|---|---|
| GitHub returns 5xx during `ListRepos` | github_error metric label | Abort cycle, no cursor save | Next cycle (default 10 min) retries from same cursor |
| GitHub returns 403 + `X-RateLimit-Remaining: 0` | rate_limited metric label | Abort cycle, no cursor save | Wait for `X-RateLimit-Reset`; next cycle resumes |
| Per-repo `GetMasterSHA` or `GetChangelogContent` errors transiently | `glog.V(2).Infof("repo dropped from cycle: owner=%s repo=%s err=%v", ...)` log line emitted — operator greps this to confirm the prune happened. `IncFilterSkipped` is NOT emitted (this is not a filter skip — it's a fetch failure) | That repo's cursor entry is pruned this cycle; controller dedup makes re-emit a no-op | Next successful cycle re-fetches and re-publishes |
| Per-repo `GetChangelogContent` returns 404 (no CHANGELOG.md) | normal path | `ChangelogSummary{UnreleasedBullets: 0}` → `EmptyUnreleasedFilter` skips | None needed (deterministic skip) |
| Per-repo `GetAutoReleaseConfig` returns 404 (no `.dark-factory/config.yml`) | normal path | `AutoRelease: false` → `AutoReleaseFilter` does NOT skip | None needed |
| Cursor file corrupt JSON | `LoadCursor` returns error | `Poll` returns error; caller logs + non-zero exit on next cycle attempt (matches `watcher/github-build` policy) | Operator deletes the corrupt file (accept cold-start) or restores from PVC backup |
| Kafka `SendCommand` failure | `IncPublished("error")` metric | Skip cursor update for that repo; cycle continues | Next cycle re-publishes (controller dedup makes idempotent at `task_identifier`) |
| Cursor `SaveCursor` failure post-publish | log warn line, no metric | Cycle returns nil — tasks already published, cursor write is best-effort | Next cycle may re-publish same `(repo, head_sha)`; controller dedup absorbs |

## Do-Nothing Option

Keep the Phase 1 slash-command pair as the only release mechanism. Cost:
- Operator must remember to run `/github-unreleased-repo-watcher` periodically (no autonomous scan)
- BRO-20203 follow-up batch (~30 repos opted out of `autoRelease: true`) cannot fire-and-forget
- Time cost per scan: ~3 minutes operator attention × ~weekly cadence × 52 = ~2.5 hours/year, plus drift cost from forgotten scans (releases delayed by days/weeks)
- The slash-command pipeline has already validated the contract — the question isn't "should we automate?" but "do we automate now or later?" Deferring means accepting operator toil indefinitely.

## Security / Abuse

- GitHub auth via App installation token (existing `watcher/github-pr` pattern, App ID + Installation ID + PEM via k8s Secret `envFrom`). No user-input-to-shell surface.
- CHANGELOG content is the only untrusted input. Passed through to task body header verbatim (no shell interpolation, no template engine). Body is operator-readable markdown only — agent does not parse it.
- `task_identifier` is UUID5 derived from public inputs (owner, repo, head_sha — all visible in git history). No secret material in the identifier.
- Cursor file at `/data/cursor.json` contains only repo names + SHAs. No secrets.
- Defense-in-depth `REPO_ALLOWLIST` env (carried verbatim from `watcher/github-pr`) refuses to publish tasks for repos outside the configured scope, even if `ListRepos` returns more.

## Acceptance Criteria

- [ ] `cd watcher/github-release && make precommit` exits 0
- [ ] `grep -rn "TODO" watcher/github-release --include='*.go'` returns 0 lines (all stubs implemented)
- [ ] Test files exist for each pure-Go module: `ls watcher/github-release/pkg/changelog_test.go watcher/github-release/pkg/taskid_test.go watcher/github-release/pkg/cursor_test.go watcher/github-release/pkg/filter/empty_unreleased_filter_test.go watcher/github-release/pkg/filter/auto_release_filter_test.go watcher/github-release/pkg/filter/sha_unchanged_filter_test.go` exits 0. Coverage threshold deliberately omitted — the 6 named-test ACs below (filter behaviors, taskid determinism, changelog inverted-ordering, cursor round-trip, BuildCreateCommand table row, Poll mock-driven cycle) exercise every pure-Go branch; mechanical coverage % adds Goodhart pressure without strengthening behavior.
- [ ] `cd watcher/github-release && make generate && git diff --exit-code mocks` exits 0 (mock regen is deterministic, no unexpected diff). All counterfeiter `-o` directives resolve to the service-root `mocks/` directory — same convention as `watcher/github-pr/mocks/`, `agent/pr-reviewer/mocks/`, etc. in this multi-module monorepo. **Amendment (2026-06-04, during verify):** original AC referred to `pkg/mocks` — spec typo; actual placement is `watcher/github-release/mocks/`. The mocks-license-header drift on regen (`addlicense` skips counterfeiter files) is a repo-wide concern that affects every service's `mocks/`, not specific to this spec — tracked separately if needed.
- [ ] Ginkgo `It` named `BuildCreateCommand produces frontmatter task_type github-release for bborbe/docker-utils d630ef3` passes — verifies a known table row matches the Phase 1 frozen output
- [ ] Ginkgo `It`s named `EmptyUnreleasedFilter skips when UnreleasedBullets is 0`, `AutoReleaseFilter passes when AutoRelease is true`, `SHAUnchangedFilter skips when LastSeenSHA equals HeadSHA`, `SHAUnchangedFilter emits when LastSeenSHA differs from HeadSHA` all pass. **Amendment (2026-06-04, during verify):** original AC said `AutoReleaseFilter skips when AutoRelease is true` — wrong semantic. The github-releaser bot is gated on positive opt-in (`.maintainer.yaml: release.autoRelease: true`), so the filter `passes` when AutoRelease is true (lets the repo through to publish). The skip-on-true wording in Desired Behavior #4 was also inverted; the implementation correctly uses the opt-in semantic.
- [ ] Ginkgo `It` named `DeriveTaskID is deterministic for identical inputs` passes — same `(owner, repo, head_sha)` produces identical UUID across 10k invocations
- [ ] Ginkgo `It` named `ParseChangelog handles Unreleased at bottom with mixed v-prefix` passes — exercises the `disk-status`-style inverted ordering Phase 1 surfaced (see [[GitHub Release Agent Phase 1 Learnings]] finding #4)
- [ ] Ginkgo `It` named `SaveCursor + LoadCursor round-trip preserves Repos map` passes
- [ ] Ginkgo `It` named `Poll publishes one task per non-skipped repo and saves cursor` passes against a counterfeiter-mocked `GitHubClient` (2 repos: one publishes, one filtered)
- [ ] No `context.Background()` in `watcher/github-release/**/*.go` outside `*_test.go`, EXCEPT in service entry points (`func main()`) passing the root context into `service.Main(...)` — the identical exception lives in `watcher/github-pr/main.go` and `watcher/github-build/main.go` (canonical service-entry-point pattern). Check: every `grep -rn "context.Background()" watcher/github-release --include='*.go' | grep -v _test.go` hit lives in a `main.go` file and matches `service.Main(context.Background()`. **Amendment (2026-06-04, during verify):** original AC said "exactly one line in `main.go`". The implementation added a second canonical service entry point at `cmd/run-once/main.go` (manual-trigger binary for testing — same pattern as other maintainer watchers) — both call `service.Main(context.Background(), ...)`. AC widened to "every match is a canonical service entry point" instead of pinning a single file.
- [ ] No `fmt.Errorf` in production paths — `grep -rn "fmt.Errorf" watcher/github-release --include='*.go' | grep -v _test.go` returns 0 lines (use `errors.Wrapf`)
- [ ] Prometheus counter `github_release_watcher_published_total{status="create"}` exists with `.Add(0)` initialization at package init — `grep -A2 'publishedTotal.WithLabelValues' pkg/metrics.go` shows pre-initialization

## Verification

```bash
cd watcher/github-release
make precommit
```

`make precommit` runs format + generate + test + lint + license — covers every AC above except the named-Ginkgo-test assertions, which `make precommit`'s test stage enforces transitively (Ginkgo fails the suite if any `It` fails).

## Related

- `[[Build github-release watcher]]` — parent Phase 2 task page (vault)
- `[[Watcher Writing Guide]]` — six-required-components checklist this spec satisfies
- `[[Agent Task File Contract]]` — frontmatter + body shape this watcher MUST emit
- `[[GitHub Release Agent Phase 1 Learnings]]` — what carries to Phase 2 verbatim (finding #6: single-pod multi-phase loop is agent territory; watcher does NOT have phases — its single Poll cycle is the unit of work)
- `[[Agent Development Workflow]]` § Stage 2 — "no new capabilities" guardrail
- Reference implementations:
  - `~/Documents/workspaces/maintainer/watcher/github-pr/` — PR-scan analogue (time-based cursor, full PR-review filter chain — copy patterns, drop PR-specifics)
  - `~/Documents/workspaces/maintainer/watcher/github-build/` — per-repo state-machine cursor analogue
- `github-release-watcher-deploy` (sister spec — to be written after this one's prompts complete) — covers dev deploy + rung-2 e2e evidence

## Verification Result

**Verified:** 2026-06-04T15:08:13Z (HEAD d27cf97)
**Binary:** installed `dark-factory` (spec targets `github.com/bborbe/maintainer/watcher/github-release`, not dark-factory itself)
**Scenario:** `cd watcher/github-release && make precommit` + 7 targeted Ginkgo focus runs against current HEAD
**Evidence:**
- `make precommit` → `ready to commit` (lint 0 issues, vet clean, osv-scanner clean, trivy 0 vuln/secret, addlicense applied)
- Ginkgo focus runs (all 1 Passed | 0 Failed against their respective focus strings): `BuildCreateCommand produces frontmatter task_type github-release for bborbe/docker-utils d630ef3`; `EmptyUnreleasedFilter skips when UnreleasedBullets is 0`, `AutoReleaseFilter passes when AutoRelease is true`, `SHAUnchangedFilter skips when LastSeenSHA equals HeadSHA`, `SHAUnchangedFilter emits when LastSeenSHA differs from HeadSHA` (4 Passed | 0 Failed); `DeriveTaskID is deterministic for identical inputs`; `ParseChangelog handles Unreleased at bottom with mixed v-prefix`; `SaveCursor + LoadCursor round-trip preserves Repos map`; `Poll publishes one task per non-skipped repo and saves cursor`
- `grep -rn TODO watcher/github-release --include='*.go'` → exit 1 (0 lines); `grep fmt.Errorf | grep -v _test.go` → exit 1 (0 lines)
- `grep context.Background() | grep -v _test.go` → 2 hits, both `func main()` calling `service.Main(context.Background(), ...)` (`main.go` + `cmd/run-once/main.go` — canonical service entry points per amended AC11)
- `pkg/metrics.go:43,68-69` → `published_total{status}` registered with `.Add(0)` init for `create` + `error`; namespace `github_release_watcher` → metric name `github_release_watcher_published_total`
- Mocks regen: counterfeiter writes to module-root `watcher/github-release/mocks/` (7 files); substantive content stable; license-header drift declared out-of-scope per AC4 amendment and healed by `make precommit`'s `addlicense` step
**Verdict:** PASS
