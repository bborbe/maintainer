# Changelog

All notable changes to this project will be documented in this file.

Please choose versions by [Semantic Versioning](http://semver.org/).

* MAJOR version when you make incompatible API changes,
* MINOR version when you add functionality in a backwards-compatible manner, and
* PATCH version when you make backwards-compatible bug fixes.

## v0.39.0

- feat(watcher/github-release): `POST /trigger?force=true` now bypasses the SHA-unchanged dedup filter for exactly one poll cycle so operators can re-run the watcher even when every repo's head SHA matches the recorded cursor. All other filters (allowlist, empty-unreleased, auto-release) still run; cursor save semantics are unchanged. Absent or unparseable `?force` values resolve to `false` (today's behaviour). Closes the asymmetry with the sibling github-pr watcher's force flag (spec 069). (spec 071)
- feat(lib): Add `GithubBuildV1SchemaID` (`Group: "maintainer"`, `Kind: "githubbuild"`, `Version: "v1"`) to the CDBSchemaIDs registry, serializing to `maintainer-githubbuild-v1` (spec 068)
- feat(watcher/github-build): Add `TriggerBuildCheckCommand` payload (`Scope` + `Force` reserved-unread fields), `TriggerBuildCheckCommandSender` with counterfeiter mock, and in-memory `pkg.MemDB` offset store for the /trigger CQRS split (spec 068)
- feat: Add `TriggerBuildCheckCommandExecutor` invoking shared `pkg.Watcher.Poll` on consumed `TriggerBuildCheckCommand`s (spec 068)
- refactor: Move /trigger HTTP handler from `pkg/trigger_handler.go` (in-process `chan struct{}` signal) to `pkg/handler/trigger_handler.go` (CQRS publish + 202); handler now depends only on `command.TriggerBuildCheckCommandSender` (spec 068)
- refactor: Move Kafka sync producer lifecycle to `factory.CreateSyncProducer` and have `factory.CreateWatcher` return the producer for reuse across senders; main.go wires create-task and trigger-build-check senders from a single sync producer (spec 068)
- feat: Wire github-build /trigger through Kafka command consumer — third run.Func, factory.CreateCommandConsumer (cdb.RunCommandConsumerTxDefault + lib.GithubBuildV1SchemaID), shared Watcher, MemDB offset store; factory AST control-flow assertion + integration clean-shutdown test (spec 068)

## v0.38.0

- feat(watcher/github-release): Add `TriggerReleaseCheckCommand` payload (`Scope` + `Force` reserved-unread fields), `TriggerReleaseCheckCommandSender` with counterfeiter mock, and in-memory `pkg.MemDB` offset store for the /trigger CQRS split (spec 067)
- feat: Add TriggerReleaseCheckCommandExecutor invoking shared pkg.Watcher.Poll on consumed commands
- feat: Add thin HTTP /trigger handler that publishes TriggerReleaseCheckCommand to Kafka and returns 202
- feat: Wire github-release /trigger through Kafka command consumer — third run.Func, factory.CreateCommandConsumer, shared Watcher, MemDB offset store
- test: Add clean-shutdown and crash-recovery integration tests for the wired command consumer
- fix(agent/github-releaser): drop the dead `expectedSHA` parameter from `s.fail` (always `""` on the failure path; doc-comment now states the failure-path post-check is superseded-only — the local commit/tag step never produces a SHA that could match the remote tag, so the `released` branch is by construction unreachable). Use `string(domain.TaskPhaseAIReview)` for the `NextPhase` on the verdict-upgrade return (consistency with line 177). Wrap both `LsRemote` call sites in a 30s `context.WithTimeout` so a stalled GitHub / DNS hang / TCP backoff cannot block the agent indefinitely. Add `url_helpers_test.go` covering `normalizeCloneURLToHTTPS` (SCP / SSH / HTTPS / unknown forms) and `injectToken` (token-prefix, empty-token, non-HTTPS guard, no-existing-userinfo precondition). Rewrite the post-check re-fire idempotency test to assert on typed `ResolutionOutput` extracted from the post-state markdown (the prior test asserted on a freshly-parsed `md2` that never had a `## Resolution` block — trivially-true). Replace the byte-comparison in the status=completed terminal-idempotency test with a typed-struct comparison (survives benign agentlib re-encoding). Replace the test-only `intToStr` helper with `strconv.Itoa` (spec 064 pr-review round-2)
- fix(agent/github-releaser): post-check verdict upgrade now flips the execution step's `s.fail` return from `AgentStatusFailed` to `AgentStatusDone` (NextPhase=ai_review) when `md.Frontmatter["status"]` was rewritten to `completed`. Without this the controller saw `Failed` even after the `## Resolution` block recorded `released`/`superseded` — a retry-storm that re-cloned/committed/tagged every cycle while the idempotency guard kept the verdict stable. Per-spec "Recovery: None needed" only holds when the returned Status is coherent with the upgraded frontmatter. Also: add a `taskID == "" || authedURL == ""` early-return at the top of `postCheck` so the validatePlan / extractFrontmatter early-fail sites don't waste an `LsRemote` round-trip with a malformed auth URL (`verdict=no-op-missing-context`) (spec 064 pr-review MAJOR + NIT 1)
- test(agent/github-releaser): add `status=aborted` idempotency test for the execution-step post-check (twin of the existing `status=completed` case); log `verdict=no-op-remote-empty` from `checkReviewOverride` so operators can distinguish "remote returned empty" from "function never called" (spec 064 pr-review follow-ups)
- feat(agent/github-releaser): ai_review step now records a `## Review Warning` block and closes the task as `completed` (frontmatter `status: completed` / `phase: done`) when a review rejection coincides with a confirmed remote tag at the agent's expected SHA (or at a different SHA — superseded mirror). The review verdict (the `## Review` section with `Approved=false`) is preserved durably on the task body for the operator audit trail. When the remote is empty or the LsRemote query errors, the existing `human_review` path stands unchanged. The new branch is a sub-decision on the `!approved` path inside `Run`; the `result.Outcome != "released"` short-circuit and the `Approved=true` happy path are unchanged. The factory wiring is unchanged (spec 064)
- feat(agent/github-releaser): post-check tail on the execution step — after `## Result` is written (success or failure), the agent shells out `git ls-remote refs/tags/<planned-version>` and uses the response to decide the terminal verdict. Remote shows tag at expected SHA → upgrade to `released` + `## Resolution` block + `status: completed` / `phase: done`. Remote shows tag at different SHA → upgrade to `superseded` + `## Resolution` block + `status: completed` / `phase: done`. Remote empty → no-op. `ls-remote` error → no-op (error logged, never downgrades a verdict). The post-check is idempotent: `status ∈ {completed, aborted}` → return immediately at the first statement. One structured log line per invocation via `glog.V(2)` naming `task_id`, `planned_version`, `observed_remote_sha`, and `verdict` from `{released, superseded, no-op-remote-empty, no-op-remote-error, no-op-already-terminal}`. All existing `s.fail` call sites participate via a widened signature (the verdict change is internal to the agent — no Kafka envelope, no agent-lib API changes). Closes the false-negative class of bug where a successful GitHub release is recorded as `failed` because the agent only consulted local state (spec 064)
- feat(agent/github-releaser): add `git.GitOps.LsRemote(ctx, cloneURL, ref, tag) (sha string, err error)` — read-only `git ls-remote refs/tags/<tag>` query that returns the dereferenced commit SHA for annotated tags (the `^{}` line) and the tag-object SHA for lightweight tags; returns `("", nil)` when the tag is absent on the remote. Counterfeiter mock regenerated, unit tests cover annotated / lightweight / empty / subcommand-error / argv-only / token-redaction fixtures. The post-check verdict-upgrade behavior lands in a follow-up prompt (spec 064 prompt 2). No agent behavior change in this seam
## v0.37.0

- feat(watcher/github-pr): split /trigger into CQRS pair — HTTP handler validates the PR URL and publishes a `TriggerPRReviewCommand` to Kafka (returns 202), an in-pod command consumer (third `run.Func` alongside the poll loop and HTTP server) runs the GitHub fetch + filter + trust + downstream `CreateTaskCommand` publish. Pod crashes mid-trigger survive via Kafka redelivery (downstream task_id is derived and idempotent). HTTP wire shape changes from `200 + {status,task_id,repo,pr_number,head_sha}` to `202 + {status,url}`; filter-skip and trust-reject become silent in the HTTP response (visible in `github_pr_published{result="skipped"|"kafka_error"|"trust_error"}` metrics). The `/admin/trigger` mount path and the `GithubPRReviewV1SchemaID` are unchanged.
- test(watcher/github-pr): add `TriggerPRReviewCommand` operation constant, sender, executor, byte-identical payload parity, crash-recovery, panicking-GitHub-client, clean-shutdown, and end-to-end command flow tests (spec 066)

## v0.36.0

- feat(lib): add `CDBSchemaIDs` registry with `GithubPRReviewV1SchemaID` (`maintainer/githubprreview/v1`) and `GithubReleaserV1SchemaID` (`maintainer/githubreleaser/v1`) — first maintainer-owned CQRS schemas. Aggregate slice is consumed by `trading/strimzi/topic-controller/pkg/topics.go` (separate PR) to provision the matching Kafka topics. Adds `github.com/bborbe/cqrs v0.5.3` dep. No behavior change yet — schema definitions only; commands + senders + handlers follow in sibling PRs.

## v0.35.2

- fix(agent/github-releaser): lenient unreleased-section detection — the changelog parser now treats the first H2 that is not a version header (`vX.Y.Z` / `X.Y.Z`) as the unreleased section, so `## unreleased`, `## Unreleased changes`, `## WIP`, `## Next`, and similar author variants release correctly. Mirrors the watcher lenient rule from spec 064 so end-to-end release behavior is identical. The on-disk heading is still canonicalized to `## vX.Y.Z` by `RewriteUnreleasedHeader` (lenient on input, canonical on output). `ExtractSectionBody` retains exact-match semantics for version-string lookups used by `steps_ai_review.go` (spec 065)

## v0.35.1

- fix(watcher/github-release): lenient unreleased-section detection — the first H2 that is not a version header (vX.Y.Z / X.Y.Z) is now treated as the unreleased section, so "## unreleased", "## Unreleased changes", "## WIP", and similar variants release correctly instead of silently producing no task (spec 064)

## v0.35.0

- feat(agent/github-releaser): teach the bump classifier about the pre-1.0 cap — pre-1.0 projects (current_version starting with `0.` or `v0.`) now have `major` capped at `minor`; the LLM records the downgrade in its `reasoning` string. Post-1.0 behavior is unchanged. The Go `applyMajorBumpGuard` stays as the durable safety net (spec 063)

## v0.34.0

- feat(lib/repoallowlist): allow `!`-prefix entries as exclusions — a target is allowed iff `(includes empty OR any include matches) AND (no exclude matches)`. Excludes always override includes; an exclude-only allowlist means allow-all-except. Existing `IsAllowed` / `Validate` signatures unchanged; consumer services pick up the new semantics with zero code change (spec 061)

## v0.33.1

- fix(agent/pr-reviewer): verdict now decides the GitHub review event — `approve` always maps to `APPROVE` and `request-changes` always maps to `REQUEST_CHANGES`, regardless of the per-repo `autoApprove` config. The `autoApprove` flag remains as a config field/YAML key for operator tooling; it no longer downgrades a verdict. The verifier's fresh-review allow-list at the call site in `pkg/steps_review.go` drops `COMMENTED` so a stale `COMMENTED` review at the head SHA is correctly treated as a non-match. Branch protection's "approving review" requirement is now satisfied on docs-only PRs with `autoApprove=false`.

## v0.33.0

- refactor(agent/github-releaser): extract the spec-060 major-bump guard decision table from `runClassification` into a private `applyMajorBumpGuard` helper, and the rewrite-and-publish tail into a private `resolveRewriteAndPublish` helper. `runClassification` drops from 92 non-comment lines (over the 80-line `funlen` threshold) to 64. Decision table, escalation contract, glog lines, and `PlanOutput` field emissions are preserved bit-identically
- feat(agent/github-releaser): planning step now blocks `bump=major` verdicts unless EITHER `.maintainer.yaml` has `release.allowMajorBump: true` (per-repo opt-in) OR the agent is invoked with `--allow-major` (per-run CLI override, env `ALLOW_MAJOR`). Trip condition (`major` + neither opt-in) writes `## Plan(outcome=needs_input, precondition_failed=major_bump_not_allowed)` with `assignee` cleared and `previous_assignee=github-releaser-agent` per the spec-047 escalation contract, returns `Status=NeedsInput` (no auto-retry, no advance), and emits `glog.V(2) "major bump not allowed"`. The override path emits `glog.V(2) "--allow-major override"` so `kubectl logs` greps surface operator overrides. Decision table: `patch`/`minor` always proceed; `major` + repo-opt-in proceeds silently; `major` + CLI-override proceeds with the override audit line; `major` + neither escalates. `PlanOutput` gains `allow_major_bump_config` and `allow_major_bump_flag` (both `omitempty`); `PreconditionMajorBumpNotAllowed` constant added alongside the existing P1/P2/missing-frontmatter/bad-current-version values. `NewPlanningStep` and `factory.CreateAgent`/`CreateAgentProvider` signatures gained the new `allowMajor bool` parameter; both `main.go` entry points thread `a.AllowMajor` through
- feat(lib): add `release.allowMajorBump` boolean field to `ReleaseConfig` in `lib/maintainerconfig` — spec-060 per-repo opt-in for automatic major-version releases; default `false` (omit the field, set `false` explicitly, or omit the `release:` block — all equivalent). Non-boolean values fail at parse time via the type system. The planning step (github-releaser) reads `cfg.Release.AllowMajorBump` to gate `bump=major` verdicts; the second lever is the `--allow-major` CLI flag (env `ALLOW_MAJOR`) on the agent binary. Ginkgo table coverage for true/missing-field/missing-block plus the strict-parse non-bool rejection path
- test(agent/github-releaser): four new Ginkgo cases in `pkg/steps_planning_test.go` under `Context("major-bump guard (spec 060)")` cover the spec-060 decision table end-to-end through the planning step — `major bump trips guard when neither opt-in present` (NeedsInput, FROZEN spec-047 frontmatter mutations: `assignee=""` / `previous_assignee=github-releaser-agent` / `status=in_progress` / `phase=planning`, plus `## Plan` carrying `outcome=needs_input` AND `precondition_failed=major_bump_not_allowed`), `major bump proceeds when repo opt-in true` (Done / `NextPhase=execution` / `outcome=ready` with `bump=major`), `major bump proceeds when CLI flag set` (Done / `NextPhase=execution` / `outcome=ready` with the new `allowMajor=true` constructor argument), and `minor bump unaffected by guard` (Done / `NextPhase=execution` / `outcome=ready` / `next_version=1.8.0`). The trip case's `## Unreleased` fixture contains the literal regression bullet `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` so the originating incident is named in the test source. All 29 pre-existing call sites in the file were updated to the new 4-argument `pkg.NewPlanningStep(runner, fetcher, maintainerConfig, allowMajor)` signature (default `false`); the `factory.CreateAgent` compile-time assertion at the bottom of the file was updated to match. Planning-step coverage stays at 90.7% (well above spec-049's 75% target)
- docs: README adds a `## github-releaser` section under the existing top-level structure, with a `### Major-bump guard (spec 060)` sub-section documenting `release.allowMajorBump` (per-repo YAML opt-in, default `false`, one-line YAML example), the `--allow-major` CLI flag with `ALLOW_MAJOR` env name (per-run override), one-paragraph rationale pointing at the false-negative class of bug where a prefix-based bump classifier mis-categorizes a breaking change, and the two operator re-delegation paths (commit YAML opt-in + re-set assignee, OR re-fire the Job with `--allow-major=true` / `ALLOW_MAJOR=true`)
- feat(agent/github-releaser): planning phase now blocks `bump=major` verdicts without an explicit opt-in — `release.allowMajorBump: true` in the target repo's `.maintainer.yaml` (durable, per-repo) or `--allow-major` / `ALLOW_MAJOR=true` on the agent binary (transient, per-run). Without either, a `major` verdict returns `Status=NeedsInput` with `precondition_failed=major_bump_not_allowed` so the operator can confirm before tag + push. Guards the false-negative class of bug where a prefix-based classifier mis-categorizes a breaking change (spec 060)

## v0.32.1

- chore: bump `github.com/bborbe/agent/lib` v0.63.11 → v0.65.0 across all services (agent/pr-reviewer, agent/github-releaser, watcher/github-pr, watcher/github-release, watcher/github-build) to pick up `envparse.RedactForLog` — pr-reviewer subprocess env logs now show `ANTHROPIC_AUTH_TOKEN=***` and `GH_TOKEN=***` instead of the literal values; closes the 2026-06-03 prod-leak surface

## v0.32.0
- feat(agent/pr-reviewer): install `@ast-grep/cli` in the alpine image so the `bborbe/coding` plugin's new dispatcher (Step 4a invokes the ast-grep-runner agent) can resolve the `sg` / `ast-grep` binary. Without it, the reviewer's Claude run loops on `sg --version` checks until the job hits `activeDeadlineSeconds` (observed on bborbe/coding#34). Mirrors the `claude-yolo` PR #8 fix that closed the same gap for the dark-factory image. `ARG ASTGREP_VERSION=latest` so we can pin a version later without changing the install line shape

## v0.31.0

- feat(agent/github-releaser): add `pkg/maintainerconfig` package — fetches `.maintainer.yaml` bytes from a target GitHub repo at a ref via the contents API, mirrors the `githubchangelog.Fetcher` shape, returns the sentinel `ErrFileNotFound` (declared via `stderrors.New`, project convention) on HTTP 404 so callers can treat the absent-file case as a default-valued config. Re-exports `lib/maintainerconfig.{Config,ReleaseConfig,PrReviewerConfig,Parse}` so the planning step needs only one import. New counterfeiter mock `mocks.MaintainerConfigFetcher`; Ginkgo coverage for happy path, 404 → `ErrFileNotFound`, 500 → wrapped non-2xx error, empty owner/repo/ref, malformed JSON, unsupported encoding, bad base64, and the round-trip fetch → `Parse` integration seam
- feat(lib): add `release.changelogRewrite` boolean field to `ReleaseConfig` in `lib/maintainerconfig` — spec-059 per-repo opt-in for the 058 LLM rewrite pipeline. Default false (omit the field, set false explicitly, or omit the `release:` block — all equivalent). Non-boolean values fail at parse time via the type system. `Parse(ctx, []byte{})` continues to return `(MaintainerConfig{}, nil)`. Ginkgo table coverage for true/false/missing-field/missing-block/empty-bytes/both-true and `It` cases for the fail-closed path on string/number values
- feat(agent/github-releaser): planning step now reads `release.changelogRewrite` from `.maintainer.yaml` at the target ref's tip via a new `pkg/maintainerconfig` fetcher; when false (default — file absent, field absent, or explicit false) the planning LLM is NOT invoked for the rewrite call and the resulting `## Plan` carries `rewrite_needed=false`; when true the existing 058 rewrite pipeline runs unchanged. Non-boolean values for `release.changelogRewrite` fail closed at planning entry (`outcome=failed`, `error_category=invalid_config` on `## Plan`, task ends in `human_review`, no commit/tag/push). The resolved flag value is recorded on `## Plan` for audit. Adds Ginkgo coverage for all value cases plus the `human_review` fail-closed path and flag-read-once semantics
- feat(agent/github-releaser): ai-review step now performs a semantic faithfulness check (Claude LLM compares the planning-captured `## Unreleased` body against the final `## vX.Y.Z` body in the local clone), a local diff check (release commit must touch only `CHANGELOG.md` + detected plugin manifests), and gates the network push on both. On success it pushes the local commit + tag via `git.GitOps.Push` and returns `Done`; on any check failure or push error it writes `## Review` with a `FailedChecks` list naming the offending checks (TagExists, TagAtExpectedSHA, ChangelogHeaderRewritten, Faithfulness, UnexpectedFileChange) and exits to `human_review`. `NewAIReviewStep` signature gained `claudelib.ClaudeRunner` and `git.GitOps` parameters; `factory.CreateAgent` builds the new dependencies (read-only `aiReviewTools` Claude runner + the same GitOps used for execution). `ReviewOutput` now carries `Overall` (pass|fail|unknown), `PerEntry` (flat list flattening the LLM's per_entry+extras), `UnexpectedFiles`, and `FailedChecks`. Faithfulness LLM unavailability is surfaced as `Overall=unknown`, not as a per-entry verdict
- feat(agent/github-releaser): add `changelog.ExtractSectionBody(ctx, content, heading)` pure helper generalizing `ExtractUnreleasedBody` — both functions share the unexported scan loop; `ExtractUnreleasedBody` is now a thin wrapper that pins the heading to `"Unreleased"`. ai-review uses the new helper to extract the `## vX.Y.Z` body for the faithfulness comparison
- feat(agent/github-releaser): add `prompts.ChangelogFaithfulnessPrompt()` (embedded from `pkg/prompts/changelog_faithfulness.md`) plus `prompts.FaithfulnessLLMResponse` / `prompts.FaithfulnessEntry` types and `prompts.ParseFaithfulnessResponse` (same three-strategy JSON extraction as `ParseBumpVerdict` / `ParseRewriteVerdict`; validates `overall ∈ {pass, fail}`, `per_entry[i].verdict ∈ {present, silent-drop}`, `extras[i].verdict == hallucinated`); Ginkgo table coverage for plain / fenced / bad-verdict / missing-overall / extra-fields cases
- refactor(agent/github-releaser): ai-review structural checks (TagExists, TagAtExpectedSHA, ChangelogHeaderRewritten) no longer early-return on a check failure — they accumulate into a `FailedChecks` list so the human reviewer sees the full set of issues. Sentinel `ErrTagNotFound` is still handled cleanly (recorded, not wrapped as a transient retry). Transient transport errors (5xx, etc.) still return wrapped errors for controller retry, the same as before
- test(agent/github-releaser): new `It` cases in `pkg/steps_ai_review_test.go` — faithful-rewrite happy path (push happens, NextPhase=done), silent-drop / hallucinated / unexpected-file-change failures (no push, NextPhase=human_review, `## Review` captures the offending entries), structural-check independence (TagExists, TagAtExpectedSHA, ChangelogHeaderRewritten each fail in isolation), LLM unavailability (Overall=unknown), push failure and concurrent-push (tag already exists on upstream) both end in `human_review` with a `push failed` note, and the integration-seam mapping test that flattens the LLM's per_entry+extras into a single `PerEntry` list with extras tagged `Verdict=hallucinated`
- test(agent/github-releaser): extend `pkg/prompts/prompts_test.go` with `ChangelogFaithfulnessPrompt` content checks (semantic faithfulness, silent-drop, hallucinated, per_entry/extras/overall schema) and `ParseFaithfulnessResponse` table entries
- test(agent/github-releaser): execution step now leaves the workdir on disk for ai-review to read on success; the `pkg/steps_execution_test.go` happy-path and pre-existing tests updated to reflect the new `Workdir`/`LocalTag` ownership boundary
- feat(agent/github-releaser): execution step now writes the `## Unreleased` body using `plan.RewrittenUnreleased` (when `plan.RewriteNeeded=true`) before renaming the header, in a single atomic commit covering both changes. Push has moved out of execution into the (next) ai-review phase; the local clone + annotated tag now survive `Run`'s return so ai-review can read them. `ResultOutput` gains `Workdir` and `LocalTag` fields; failure paths still remove the workdir via defer
- feat(agent/github-releaser): add `changelog.ReplaceUnreleasedBody` pure helper that swaps the body of the `## Unreleased` section (every line after the heading up to the next `## ` heading or EOF) with a supplied string, plus table tests in `pkg/changelog/changelog_test.go` covering typical replacement, empty body, missing `## Unreleased`, and trailing-newline edge cases
- refactor(agent/github-releaser): rename `executeDirectPush` to `executeLocalRelease` (the name "direct push" was misleading once the push moved out). `ResultPathDirectPush = "direct-push"` is kept for back-compat with persisted task pages
- feat(agent/github-releaser): planning step now captures the original `## Unreleased` body verbatim and emits a rewrite verdict (rewrite_needed + optional cleaned body) into the `## Plan` JSON, with the Changelog Quality Guide embedded via `//go:embed` and a second focused Claude call. Already-clean changelogs pass through with `rewrite_needed=false`; noisy bodies (raw `git log` lines, missing prefixes, ten-line `chore: bump` dumps) are cleaned into prefix-conformant bullets by the planning LLM using the embedded guide as the ruleset
- feat(agent/github-releaser): add `changelog.ExtractUnreleasedBody` pure helper returning the verbatim body of the `## Unreleased` section, plus table tests in `pkg/changelog/changelog_test.go`
- feat(agent/github-releaser): add `prompts.RewriteVerdict` type and `ParseRewriteVerdict` parser using the same three-strategy extraction as `ParseBumpVerdict` (plain JSON, fenced ```json, last balanced block); Ginkgo coverage for plain / fenced / empty / missing-reasoning / malformed / extra-fields cases
- test(agent/github-releaser): five new `It` cases in `pkg/steps_planning_test.go` under `Context("rewrite decision")` — clean → false, noisy git-log dump → true, missing-prefix, chore-dump fold, verbatim capture. The verbatim-capture test asserts the security-relevant invariant that `OriginalUnreleased` is byte-equal to the slice ai-review will read
- test(agent/github-releaser): add `Context("happy-path workdir + no-push")`, `Context("rewrite_needed")` (true + false), and `Context("re-fire idempotency")` blocks in `pkg/steps_execution_test.go`; existing push-failure / push-assertion specs updated to assert `PushCallCount()==0`; new `pkg/result_output_test.go` covers `Workdir`/`LocalTag` JSON tag stability and `omitempty`
- fix(agent/github-releaser): ai-review `verifyTagAtExpectedCommit` and `verifyChangelogHeaderRewritten` now fail closed on transient GitHub API errors — set the corresponding check boolean to false AND append the failed-check name to `FailedChecks` instead of silently passing through a wrapped error that the caller only logged. The `runStructuralChecks` Warningf wrappers around those helpers are now removed as unreachable. `verifyTagExists` retains its existing transport-error → controller-retry path (intentional asymmetry). New `Context("transport-error fail-closed")` Ginkgo specs cover both helpers; new `Context("rollupVerdict: LLM unknown + structural failure")` proves `Overall=unknown` overrides even when a structural check also failed (both names surface in `FailedChecks`)
- test(agent/github-releaser): new `Describe("rewrite_needed branch")` in `pkg/steps_execution_test.go` with a happy-path spec (captures post-Commit CHANGELOG bytes via a closure variable; asserts the verbatim rewritten body lands under `## v1.0.0` with no leftover `## Unreleased` heading) and an error-mapping spec (no `## Unreleased` heading → `Status=Failed`, `error_category=unreleased_not_found`, `Commit` not invoked)
- test(agent/github-releaser): new `Context("plugin-manifest branch in unexpected-file-change check")` in `pkg/steps_ai_review_test.go` — Case A applied (plugin.DetectManifests CAN return a non-nil error at `pkg/plugin/manifest.go:52` on non-IsNotExist Stat failures). Spec 3a seeds a real on-disk workdir with a valid `.claude-plugin/plugin.json` and asserts the committed manifest is in the expected set; spec 3b forces `DetectManifests` to error (chmod `.claude-plugin/` to 0000 → EACCES) so the ai-review check falls back to the changelog-only expected set and an extra committed `plugin.json` surfaces as `UnexpectedFileChange=true` with `FailedChecks` containing `CheckUnexpectedFileChange`
- test(agent/github-releaser): new `It("empty 200 OK body with no encoding field rejected")` in `pkg/maintainerconfig/fetcher_test.go` — sends a 200 OK with `{}` and asserts the wrapped error message contains `unsupported encoding ""`
- refactor(agent/github-releaser): ai-review `Push` failed-check name is now a typed constant `pkg.CheckPush = "Push"` (matching the other Check* constants). The `finishApproved` push-failure path appends `CheckPush` instead of the bare string `"Push"`
- refactor(agent/github-releaser): `setupWorkdir` godoc clarified — does NOT create the directory, just removes any stale copy and returns the path; `ops.Clone` is what actually creates it
- refactor(agent/github-releaser): 15-second HTTP timeout in `pkg/maintainerconfig/fetcher.go` promoted to a named `fetchTimeout` package constant; used in both `NewHTTPFetcher` and `newHTTPFetcherWithBase`
- refactor(agent/github-releaser): `factory.aiReviewTools` package-level `var` moved into `CreateAgent` as a local (mirrors `executionOps` lifecycle)
- refactor(agent/github-releaser): `pkg/maintainerconfig/fetcher.go` — counterfeiter directive moved to sit directly above the `Fetcher` interface declaration (project convention; godoc now above the directive)
- fix(agent/github-releaser): ai-review `checkUnexpectedFileChange` now fails closed on a `CommittedFiles` error — sets `checks.UnexpectedFileChange=true` and appends `CheckUnexpectedFileChange` to `FailedChecks` instead of silently passing through `nil`. Mirrors the `verifyTagAtExpectedCommit` / `verifyChangelogHeaderRewritten` transport-error pattern. A transient git blip can no longer leave the unexpected-file-change check passing. New `It("CommittedFiles error sets UnexpectedFileChange=true and appends CheckUnexpectedFileChange")` regression spec
- fix(agent/github-releaser): `.maintainer.yaml` HTTP response body bounded to 1 MiB via `http.MaxBytesReader` — `maxConfigBodyBytes = 1 << 20` constant. A misconfigured or hostile upstream can no longer exhaust agent memory. `httpFetcher.readBody` wraps the response body before `io.ReadAll`; new `It("oversize body rejected with cap in error")` spec covers the trip
- fix(agent/github-releaser): non-2xx response body redacted in `httpFetcher.checkStatus` error string and any associated V(2) log — replaced the 200-char body preview with the status code plus a short SHA-256 fingerprint (`body_sha256_prefix=<8 hex chars>`) and the byte count. Operator-internal paths / partial stack traces from GitHub 5xx or proxy bodies can no longer leak through. New `It("500 response with sensitive body returns redacted error")` asserts the four `Not[Contain]` invariants
- feat(agent/github-releaser): bump-verdict cache in `## Plan` — on a re-fire (e.g. transient rewrite-LLM failure), the planning step reads the prior `## Plan` section, reuses the `Bump` and `Reasoning` verbatim, and skips the bump LLM call. Cache lookup is non-fatal (fresh task page has no plan yet). The rewrite-failure path now publishes a partial `## Plan` (with the bump verdict populated) so the re-fire cache can find it. `RunCallCount` falls from 2 to 1 on re-fire. New `Context("bump verdict cache")` Ginkgo spec models the production round-trip via `(*Markdown).Marshal` + `agentlib.ParseMarkdown`
- feat(agent/github-releaser): surface non-404 `.maintainer.yaml` transport fetch failures on `## Plan` via new `ConfigFetchWarning` field (`json:"config_fetch_warning,omitempty"`) — operators can grep the task page to confirm whether a repo that opted into rewrite was silently downgraded. `glog.Warningf` retained for log-stream observability. Three new `It` specs cover transport-warning surfacing, happy-path empty, and 404-empty (legitimate-absent)
- fix(lib): `lib/maintainerconfig.Parse` now uses `yaml.NewDecoder` with `KnownFields(true)` — typos like `changelogRwrite` or `prRevierer` fail loudly with a wrapped `unmarshal .maintainer.yaml` error instead of producing a silent default-false config. The empty-bytes contract (`Parse(ctx, []byte{})` → `(MaintainerConfig{}, nil)`) is preserved by an explicit short-circuit. Three new `It` specs (`unknown top-level field rejected`, `typo in nested release field rejected`, `typo in top-level prReviewer key rejected`) replace the old forward-compat "unknown top-level key ignored" tolerance spec. The package godoc no longer asserts the "tolerated by design" behavior

## v0.30.0

- feat(agent/github-releaser): release commit now bumps `.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json` version fields alongside the CHANGELOG rewrite when those manifests exist at repo root — fixes the silent drift where Claude Code plugin repos (e.g. `bborbe/coding`) shipped release tags whose manifest versions disagreed with the CHANGELOG. Pre-push guard whitelist widened dynamically to the set of files actually touched; fails closed on anything else.
- feat(agent/github-releaser): add `plugin_manifest_invalid` error category for malformed plugin manifests (JSON parse error or missing/non-semver version field)
- test(agent/github-releaser): add integration tests for manifest bumping in executeDirectPush covering both manifests present, one manifest only, no manifests (backward compatibility), unexpected_diff guard, and malformed plugin.json guard
- test(agent/github-releaser): add edge-case coverage for DetectManifests (temp-dir cleanup via GinkgoT().TempDir, directory-as-file skip), BumpPluginJson (unquoted version values with/without trailing comma, unclosed quote error, second nested version key left untouched), BumpMarketplaceJson (top-level version outside metadata/plugins scope), deriveUnprefixedVersion(""), sameStringSet duplicate-element behavior, DetectManifests I/O error mapping to error_category=unknown, and marketplace.json malformed JSON mapping to error_category=plugin_manifest_invalid

## v0.29.1

- fix(agent/github-releaser): ai_review tag-SHA comparison now accepts short-vs-full SHA equivalence. Execution step writes Result.CommitSHA via `git rev-parse --short HEAD` (7 chars); GitHub API returns 40-char full SHA. Naive `==` was false-positive on every release (caught by canary on parked `Release bborbe-claude-yolo af4000c` immediately after prod deploy). Fix uses bidirectional `strings.HasPrefix`; regression test covers short-vs-full both directions plus a non-matching short prefix.

## v0.29.0

- feat(agent/pr-reviewer): when ai_review returns verdict=fail with hallucinations, dismiss bot's prior APPROVED/CHANGES_REQUESTED review at head SHA via GitHub REST and post a COMMENT citing each hallucination; route to human_review regardless of dismissal outcome

- test(agent/github-releaser): add comprehensive unit tests for ai_review step covering all acceptance criteria (happy path, tag-missing/404, annotated/lightweight tag SHA mismatch, CHANGELOG ## Unreleased header, Result.outcome short-circuit, malformed/missing Result section, missing frontmatter repo, token-in-error guard, no-##-Failure assertion, Name/ShouldRun)

- feat(agent/github-releaser): wire ai_review phase into CreateAgent alongside planning and execution phases, completing the three-phase release agent
- feat(agent/github-releaser): Add githubreview client implementing AIReviewClient interface with TagExists, ResolveTagCommit, and FetchChangelog methods for ai_review step verification
- feat(agent/github-releaser): Add ai_review step with three verification checks (tag exists, tag at expected SHA, CHANGELOG header rewritten) and ReviewOutput section

## v0.28.1

- refactor(mocks): consolidate every module's counterfeiter mocks into a single `<module>/mocks/` directory next to its `go.mod`, matching `go-mocking-guide.md` and the existing `agent/pr-reviewer/mocks/` reference. Removes five stray nested `pkg/mocks/` dirs in `agent/github-releaser`, `watcher/github-build`, `watcher/github-pr`, and `watcher/github-release` (21 files moved). Rewrites every `//counterfeiter:generate -o` directive to `../mocks/` (from `pkg/`) or `../../mocks/` (from `pkg/<sub>/`). Adds missing `//go:generate counterfeiter -generate` to `watcher/github-build/pkg/maintenance/suite_test.go` and `watcher/github-pr/pkg/handler/suite_test.go` (without those, `make generate`'s `rm -rf mocks` wiped subpkg mocks with no regen step). All four affected modules' `make precommit` green.

## v0.28.0

- feat(agent/github-releaser): pre-push guard — release fails closed if the commit changed anything other than `CHANGELOG.md` (defense-in-depth on the direct-push trust model)

## v0.27.1

- fix(agent/pr-reviewer): no longer posts a false CHANGES_REQUESTED on approved PRs whose review carries a long `comments` array

## v0.27.0

- feat(agent/github-releaser): new agent that cuts a release directly on master — rewrites `## Unreleased` → next version, commits, tags, pushes; semver bump classified from the CHANGELOG content
- feat(watcher/github-release): scans repos for non-empty `## Unreleased` and triggers the github-releaser agent
- feat: opt repos into auto-release via `.maintainer.yaml: release.autoRelease: true` (also where `prReviewer.autoApprove` now lives — replaces `.pr-reviewer.yaml`)
- feat(watcher/github-release): operator endpoints — `/trigger` forces a poll, `/resetcursor/{repo}` re-emits a stuck release, `/setcursor/{repo}?sha=` pins the last-seen master SHA (no PVC editing required)
- feat: App-only GitHub auth across all agents + watchers — drops `GH_TOKEN` PAT input fleet-wide; tightens the auth surface on push-capable agents
- feat: `/coding:pr-review` available in every pod (plugin baked into the image — no PVC mount)
- fix(agent/pr-reviewer, watcher/github-build): pods no longer crash-loop at startup (regression in `argument/v2.Print` on unexported struct fields)
- fix(agent/github-releaser): handle SSH-form `clone_url` (rewritten to HTTPS — runtime image has no ssh client) and clone the target's default branch instead of the trigger ref
- fix(agent/github-releaser): planning escalation keeps task in `planning` instead of auto-completing

## v0.26.39

- feat(watcher/github-release): initial Phase 1 build — scans configured repos on a cursor-tracked poll loop and emits one release task per repo whose `## Unreleased` is non-empty
- feat: opt repos into auto-release via `.maintainer.yaml: release.autoRelease: true` (replaces `.dark-factory/config.yml`; semantics flipped to positive opt-in)

## v0.26.38

- refactor(watcher/github-pr): extract TaskPublisher interface and taskPublisher struct from watcher to clarify publish/trust ownership; bundle stage/maxSlugLen/maxTitleLen/taskSuffix into TaskConfig value type

## v0.26.37

- fix(watcher/github-build): wrap errors following ParseRepoAllowlist calls in main.go and cmd/run-once/main.go

## v0.26.36

- refactor(pr-reviewer): move Kafka SyncProducer lifecycle from factory to main.go; CreateDeliverer now accepts a connected SyncProducer for pure factory wiring

## v0.26.35

- test(watcher/github-pr): add unit tests for BuildCreateCommand covering trusted/untrusted author branches, empty author login, title sanitization, and maxTitleLen truncation
- test(watcher/github-pr): add nil-check tests for CreateSinglePRTriggerHandler factory; add panic guards for nil httpClient, createSender, taskCreationFilter, and trustDecision
- test(watcher/github-pr): add pkg/factory/single_pr_test.go with panic assertions for all nil parameters

## v0.26.34

- fix(watcher/github-pr): wrap errors at validation call sites in main.go to avoid bare returns
- fix(watcher/github-pr): replace errors.Wrapf with errors.Wrap in cursor.go and trust.go where no format args present
- fix(watcher/github-pr): explicitly discard unused ctx parameter in ParseRepoAllowlist

## v0.26.33

- feat(watcher/github-pr): record IncPRPublished metrics for /trigger endpoint outcomes; distinguish trust_error and kafka_error from generic error

## v0.26.32

- refactor(watcher/github-pr): refactor SinglePRTriggerHandler to use libhttp.WithError interface pattern; return errors naturally instead of calling writeError/writeSuccess

## v0.26.31

- refactor(watcher/github-pr): split CreateGitHubHTTPClient into CreateGitHubAppClient and CreateGitHubPATClient with zero-business-logic factories; move auth-mode dispatch to main.go
- refactor(watcher/github-pr): refactor CreateKafkaSender to accept SyncProducer instead of creating one; move cleanup to main.go via defer
- refactor(watcher/github-pr): refactor CreateWatcher and CreateSinglePRHandler to accept concrete dependencies (*http.Client, trust.Trust) instead of raw config structs

## v0.26.30

- test(watcher/github-build): add unit tests for runPollLoop error handling path and countWildcards function

## v0.26.29

- test(watcher/github-build): add unit tests for GetJobsForRun covering successful response with failed jobs, no failed jobs, HTTP error, and rate limit scenarios

## v0.26.28

- refactor(watcher/github-build): extract CreateAllowlistSnapshot to factory eliminating duplicated wildcard resolution logic in main.go and cmd/run-once/main.go

## v0.26.27

- fix(watcher/github-build): use write-to-temp + atomic rename in SaveCursor to prevent cursor file corruption from concurrent read-modify-write races

## v0.26.26

- test(agent/pr-reviewer): add unit tests for ExpandHome, normalizeURL, classifyError, eventToState, truncateBody, isGitHubPRURL, hasAnyPRURL, writePlanningVerdict, appendVerifyDiagnostic

## v0.26.25

- test(agent/pr-reviewer): standardize Ginkgo suite setup across all test suites — add `//go:generate` directive to factory, GinkgoConfiguration with timeout to all suites, and rename non-standard test functions to `TestSuite`

## v0.26.24

- refactor(agent/pr-reviewer): inject libtime.CurrentDateTimeGetter into step structs and githubposter components replacing direct time.Now() calls

## v0.26.23

- refactor(watcher/github-pr): remove empty pkg/publisher.go and its export test file

## v0.26.22

- test(watcher/github-pr): standardize Ginkgo suite setup with UTC timezone, untruncated diffs, and 60s timeout

## v0.26.21

- fix(watcher/github-pr): add context cancellation checks to Poll, fetchAllPRs, and processPRs to enable prompt shutdown when context is cancelled

## v0.26.20

- refactor(watcher/github-build): remove unused context.Context parameter from ParseRepoAllowlist

## v0.26.19

- fix(watcher/github-build): use context.Background() for /trigger handler to prevent requests during graceful shutdown from being dropped

## v0.26.18

- fix(watcher/github-build): add panic-recover wrapper around wildcard refresh loop closure in buildAllowlistSnapshot to prevent panics outside safeRefresh from killing the CancelOnFirstFinish task set

## v0.26.17

- fix(watcher/github-build): add context cancellation checks inside pollRepo before GetDefaultBranch, GetWorkflowRuns, and each fetchJobInfoForRun call to prevent SIGTERM from being deferred until after all API calls complete

## v0.26.16

- fix(watcher/github-build): add context cancellation check in listOwnerReposPaginated to stop pagination immediately when context is cancelled

## v0.26.15

- fix(watcher/github-build): narrow redactOpaqueHexRE from 40+ to exactly 40 hex chars to reduce false positives; add #nosec G101 to AWS secret key redaction regex

## v0.26.14

- chore(watcher/github-build): delete empty pkg/publisher.go; fix duplicate comment in main.go

## v0.26.13

- fix(agent/pr-reviewer): add 15-second timeout to http.Client used by PrPoster and ReviewVerifier to prevent indefinite hangs on stalled GitHub API connections

## v0.26.12

- fix(agent/pr-reviewer/cmd/cli): replace fmt.Errorf with errors.Errorf in main.go to follow project error-handling conventions

## v0.26.11

- perf(agent/pr-reviewer): move envVarRefRegexp compilation to package level in config.go — eliminates redundant regex recompilation on every resolveEnvVar call

## v0.26.10

- chore(agent/pr-reviewer): bump `github.com/bborbe/agent/lib` from v0.62.17 to v0.63.0 to collapse multi-phase pod boots into one pod on the happy path (lib spec 040); pr-reviewer's 3-phase chain now runs in a single pod boot once the new binary is deployed. Test assertions updated to match lib v0.62.27/v0.62.29 behavior change (`needs_input` / `failed` no longer write `phase: human_review`).

- fix(agent/pr-reviewer): every pr-review step now publishes its routing decision on retrigger instead of silently skipping. Previously `planningStep`, `checkoutExecutionStep`, and `reviewStep` returned `ShouldRun=false` when their output section (`## Plan` / `## Review` / `## Verdict`) already existed in the body. On a retrigger (controller resets `trigger_count` but leaves the body intact), the step was skipped entirely — including the `NextPhase` decision — so the `tokenCheck` step's `Done + NextPhase=""` became the last delivered result and the task short-circuited to `phase: done` without any review running. Surfaced on `bborbe/trading#136` (2026-05-25). Fix: each step's `ShouldRun` always returns `true`; the `## <heading> already present` check moves into `Run`, which then publishes the same `NextPhase` it would have published on a fresh run (planning re-parses concerns from the existing plan; execution and ai-review just advance the phase).

- fix(agent/pr-reviewer): export GH_TOKEN into every git subprocess env (clone, fetch, worktree add, worktree prune) so the credential helper installed by `gh auth setup-git` (`gh auth git-credential`) can authenticate HTTPS operations. Without this, git inherited the pod env (no GH_TOKEN), the helper returned nothing, and clone failed with `authentication required (set GH_TOKEN and re-trigger)` even after PR #11 fixed the gh-auth-setup-git step itself. Allowlist env to `{HOME, PATH, GH_TOKEN}` to keep unrelated pod secrets out of git and any helper it shells out to.

- fix(agent/pr-reviewer): export the minted GitHub App IAT as `GH_TOKEN` in the `gh auth setup-git` subprocess env — without this, gh inherits the pod env (no token) and fails with `You are not logged into any GitHub hosts` even though the IAT was minted successfully. Surfaced by the previous diagnostic-capture fix; reproduced on `bborbe/agent#3` and `bborbe/trading#135` in prod
- fix(agent/pr-reviewer): capture combined stdout+stderr from `gh auth setup-git` and include the scrubbed bounded tail (last 4 KiB, GH_TOKEN value replaced with `***`) in the wrapped error so operators can diagnose pod-startup auth failures via the OpenClaw task `## Failure` body — previously the gh output was dropped entirely and only `gh auth setup-git failed` surfaced
- fix(agent/pr-reviewer): publish a `Status: Failed` result via the deliverer when `RunAgent` aborts on auth-setup failure so the passthrough content generator splices the wrapped error into the task `## Failure` section — previously the pod exited non-zero, k8s retried until backoffLimit, and only `Job has reached the specified backoff limit` reached the OpenClaw task body

## v0.26.9

- fix(watcher/github-build): suppress noisy stack trace when Dependabot internal graph-update workflows (`Graph Update:` or `Dependabot Updates`) are filtered out — these runs must not affect the red/green state machine

## v0.26.8

- chore: Rename `WATCHER_GITHUB_PR_TASK_SUFFIX` to `TASK_SUFFIX` in watcher/github-pr to match build watcher unified naming — breaking change, operator must update env files at deploy time

## v0.26.7

- feat(watcher/github-build): add `TASK_SUFFIX` env var to disambiguate build-failure task filenames per stage, preventing dev/prod filename collisions in the shared vault

## v0.26.6

- fix(watcher/github-build): expand owner-level wildcard allowlist entries (e.g. `github.com/bborbe/*`) into concrete repos at startup and refresh hourly — closes the silent-zero-polls bug introduced by the v0.25.0 wildcard rollout (spec 039)

## v0.26.5

- chore(agent/pr-reviewer): add `glog.V(2)` logging to every planning-step return site so routing decisions (LGTM short-circuit, execution advance, human_review escalation, POST failures) are visible in pod logs; mirrors the existing `steps_review.go` pattern

## v0.26.4

- feat(watcher/github-pr): add GitHub App auth via `APP_ID` + `INSTALLATION_ID` + `PEM_KEY` env vars (reuses existing pr-reviewer Apps `3798945` / `3800041`); uses `lib/githubapp.NewClient` (auto-refreshing IAT via `ghinstallation/v2`) because the watcher is a long-lived StatefulSet — a single `MintIAT` call would expire after 1 hour. Partial App env returns an error naming the missing field; legacy `GH_TOKEN` retained as fallback for rollout safety.

## v0.26.3

- feat(watcher/github-pr): migrate from PAT to GitHub App authentication with auto-refreshing IAT transport; supports APP_ID + INSTALLATION_ID + PEM_KEY env vars; static-PAT fallback via GH_TOKEN still works; partial App config produces a named error at startup naming the missing fields

## v0.26.2

- chore(watcher/github-build): wire GitHub App credentials (APP_ID, INSTALLATION_ID, PEM_KEY) into watcher container via Kubernetes Secret and StatefulSet env vars (spec 038)

## v0.26.1

- test: add Ginkgo v2 unit tests for auth resolver in `watcher/github-build/pkg/auth/auth_test.go` covering PAT fallback, conflict warning, refusal, and missing PEMKeyFile (spec 038)

## v0.26.0

- feat: migrate watcher/github-build from PAT to GitHub App authentication with auto-refreshing IAT transport

## v0.25.15

- refactor(lib): extract `ParsePRURL` from `agent/pr-reviewer/pkg/prurl.go` to shared `lib/prurl/prurl.go` so both `agent/pr-reviewer` and `watcher/github-pr` import the same parser (spec 036)
- refactor(watcher/github-pr): rename admin endpoint `/trigger` (multi-repo poll) to `/check` — name now reflects behavior; hard cutover, no backwards-compat alias (spec 036)
- feat(watcher/github-pr): add `POST /trigger?url=<pr_url>` admin endpoint to fire a single-PR review by URL; reuses the existing filter chain and trust evaluation; known limit — if a vault task already exists for the same (PR, SHA) the operator must still reset vault frontmatter or push a new commit (spec 036)

## v0.25.14

- refactor(agent/pr-reviewer,lib): move `prurl` package from `agent/pr-reviewer/pkg/prurl` to `lib/prurl`; update all callers to import `github.com/bborbe/maintainer/lib/prurl`

## v0.25.13

- refactor(watcher/github-pr): rename `/trigger` HTTP route to `/check`

## v0.25.12

- fix(agent/pr-reviewer): drop `checkBotIdentity` pre-flight call to `GET /app`; GitHub's `/app` endpoint requires the App-level JWT but the agent only holds the Installation Access Token, so every call returned 401 `"A JSON web token could not be decoded"` and blocked every review POST; bot identity is now trusted from the `BotLogin` env var (operator-configured), removing the broken self-check entirely

## v0.25.11

- fix(agent/pr-reviewer): planner now advances non-empty-concerns tasks with `NextPhase: "execution"` (via `domain.TaskPhaseExecution`) instead of the stale `"in_progress"` literal that spec 032 renamed; factory + k8s Config CR `trigger.phases` + planner unit test all moved to the canonical value; restores the spec 034 F2 invariant for the non-empty-concerns branch (spec 035)

## v0.25.10

- feat(agent/pr-reviewer): planning phase now posts an LGTM COMMENT review when concerns are empty, eliminating the silent-skip path; every PR that reaches planning produces at least one visible artifact; vault task gains `## Verdict` section naming the posted review id and event

## v0.25.9

- feat(agent/pr-reviewer): add GitHub App auth via new `APP_ID` / `INSTALLATION_ID` / `PEM_KEY_FILE` / `BOT_GITHUB_LOGIN` env vars; legacy `GH_TOKEN` retained as fallback; bot login `ben-s-pull-request-reviewer[bot]` (prod) / `ben-s-pull-request-reviewer-dev[bot]` (dev); `pr-review-of-ben` literal eradicated (spec 033)
- feat(agent/pr-reviewer): wire k8s Secret + Config CR for GitHub App auth (PEM mount + APP_ID/INSTALLATION_ID/PEM_KEY_FILE/BOT_LOGIN env vars); dev uses `eqKj8L` + App 3800041, prod uses `kLoejw` + App 3798945; legacy `GH_TOKEN` Secret key retained as fallback (spec 033)

## v0.25.8

- feat(lib): add `lib/githubapp` shared package — `NewClient` + `MintIAT` for GitHub App installation access token minting via `ghinstallation/v2`; consumed by spec 033 pr-reviewer auth migration

## v0.25.7

- test(agent/pr-reviewer): add `NormalizeTaskStatus("todo")` → `TaskStatusNext` and `NormalizeTaskPhase("in_progress")` → `TaskPhaseExecution` alias round-trip tests to document and guard vault-cli's legacy alias contract (spec 032)

## v0.25.6

- feat(watcher/github-build,agent/pr-reviewer): bump vault-cli to v0.64.3; flip `BuildTaskStatus` default from `"todo"` to `"next"` and agent `Phase` default from `"in_progress"` to `"execution"` so newly published tasks carry the vault-cli canonical taxonomy

## v0.25.5

- feat(agent/pr-reviewer): route claude CLI to Anthropic-compatible alt-provider via dedicated `AnthropicBaseURL`/`AnthropicAuthToken`/`AnthropicModel` fields on the application struct (mapped to `ANTHROPIC_BASE_URL`/`ANTHROPIC_AUTH_TOKEN`/`ANTHROPIC_MODEL` env vars). The renamed `AnthropicModel` field drives both the `--model` CLI flag and the `ANTHROPIC_MODEL` env var on the claude subprocess — single source of truth replaces the prior `MODEL`/`ANTHROPIC_MODEL` two-knob configuration. Applied to both Kafka entry point (`agent/pr-reviewer/main.go`) and local CLI entry point (`agent/pr-reviewer/cmd/run-task/main.go`); `pkg/factory.RunConfig` extended with the same fields and merges them into the claude subprocess env in `RunAgent`.
- feat(agent/pr-reviewer): `k8s/maintainer-agent-pr-reviewer.yaml` adds `ANTHROPIC_BASE_URL=https://api.minimax.io/anthropic` + `ANTHROPIC_MODEL=MiniMax-M2.7-highspeed` to `spec.env`; `k8s/maintainer-agent-pr-reviewer-secret.yaml` adds `ANTHROPIC_AUTH_TOKEN` sourced from teamvault `MOPmQL`. Enables MiniMax routing for dev canary as part of `[[Switch Agent API Provider]]` work.

## v0.25.4

- feat(watcher/github-pr): add `WATCHER_GITHUB_PR_TASK_SUFFIX` env var; non-empty value is appended as ` - <suffix>` to PR task filenames so dev and prod watchers writing into the same vault produce distinct filenames (dev=`dev` → ` - dev`; prod empty → unchanged). Fixes YAML merge-conflict markers in OpenClaw task files when two watchers poll overlapping repos.

## v0.25.3

- fix(agent/pr-reviewer): invert SHA filter in `listBotReviews` from `==` to `!=`; dismissal now removes only prior-commit reviews and never the current-head review; add Dismissal Contract invariant comment and doc section (spec 031)

## v0.25.2

- fix(agent/pr-reviewer): verdict parser normalises spelling drift (request_changes → request-changes, case variants); deletes markdown-heading fallback heuristic; any non-approve or absent JSON verdict fails closed to request-changes

## v0.25.1

- feat(watcher/github-pr,watcher/github-build): vault task bodies now include a clickable GitHub repo link. github-build's H1 becomes a link to https://github.com/{owner}/{repo}; github-pr adds a **Repo:** line under the existing PR-URL line. Operators triaging tasks no longer need to URL-type to reach the repo top-level.

## v0.25.0

- feat: switch REPO_ALLOWLIST in dev.env and prod.env from enumerated literal repo lists to `github.com/bborbe/*` wildcard; any bborbe-owned repo now flows through the pipeline without per-repo operator intervention

## v0.24.0

- feat: migrate all five REPO_ALLOWLIST callers to shared `lib/repoallowlist` package; replace inline regex parsers with `IsAllowed` predicate (supporting `github.com/<owner>/*` wildcard) and `Validate` validator (aggregate error for required callers); add `replace github.com/bborbe/maintainer/lib => ../../lib` to three service go.mod files

## v0.23.43

- feat(lib): bootstrap new shared Go module at `lib/` (module path `github.com/bborbe/maintainer/lib`); add `repoallowlist` package with `IsAllowed` predicate and `Validate` validator supporting literal matching, `github.com/<owner>/*` wildcard, and allow-all semantics for empty/nil allowlists

## v0.23.42

- feat(agent/pr-reviewer): add post-verification to `ai_review` phase — after `## Verdict` is written, `reviewStep` calls `ReviewVerifier.VerifyReview` (GET `/pulls/{n}/reviews`) to confirm the execution-phase review persisted on GitHub; failure appends `ai_review verify:` diagnostic line and returns `AgentStatusFailed`; skips when `## Review` absent or last diagnostics block has `class: permanent`/`class: unknown`; moves `ReviewVerifier`, `VerifyRequest`, `VerifyResult` from `pkg/githubposter` to `pkg` to break import cycle; adds `CreateReviewVerifier` factory (spec 027 prompt 3/3)

## v0.23.41

- feat(agent/pr-reviewer): wire `PrPoster` into `checkoutExecutionStep` — after Claude writes `## Review` (vault-first), calls `PrPoster.Post`, appends a `## Diagnostics` block (append-only per-run, success one-liner or fenced-YAML failure), and routes to `ai_review` on success or `human_review` on posting failure; adds `CreatePrPoster` factory, updates `CreateAgent`/`CreateAgentProvider`; moves shared types (`PrPoster`, `PostRequest`, `PostResult`, `ErrorClass`) from `pkg/githubposter` to `pkg` to break import cycle (spec 027 prompt 2/3)

## v0.23.40

- feat(agent/pr-reviewer): add `pkg/githubposter/` — GitHub REST API client for posting PR reviews. Implements bot-identity self-check, `.pr-reviewer.yaml` autoApprove config, prior-review dismissal, POST review, verify-after-POST (catches phantom POSTs), and per-call retry policy (one retry max for transient errors; no retry for permanent). Used by `in_progress` and `ai_review` phases in subsequent prompts. (spec 027 prompt 1/3)
- chore(agent/pr-reviewer): wrap phase-output JSON in fenced ```json blocks across `planning_output-format.md`, `execution_output-format.md`, `review_output-format.md` so the `## Plan` / `## Review` / `## Verdict` sections render readably in Obsidian; downstream parsers already accept fenced JSON (no parser change)

## v0.23.39

- feat(watcher/github-pr): per-(PR, SHA) spawn model — each push produces a new task file identified by the head commit SHA; the old task file is never mutated; removes `publishForcePush` and the `## Outdated by force-push` mutation path; `DeriveTaskID` now encodes full SHA in UUID5 key; `computePRTitle` adds `sha[:8]` segment between PR number and slug

## v0.23.38

- feat(agent/pr-reviewer): collapse verdict from three values to two — every review now ends with approve or request-changes; Should Fix findings escalate to request-changes (was comment); empty or unparseable agent output defaults to request-changes (fail-closed); comment constant removed, compiler-enforced

## v0.23.37

- feat(agent/pr-reviewer): per-task-type dispatch via factory.CreateAgentProvider — healthcheck task type now routes to a dedicated liveness agent built from lib/healthcheck; unknown task_type values fail fast via lib.AgentProvider.Get; bumps agent/lib v0.62.5 → v0.62.16
- feat(agent/pr-reviewer): add `healthcheck` to `taskTypes` list alongside `pr-review` + `oauth-probe` — prepares for healthcheck dispatch (rename of `oauth-probe`); no behavior change yet

## v0.23.36

- feat(agent/pr-reviewer): wire `JobMetrics` from `github.com/bborbe/agent/lib/metrics@v0.62.5` into `Run()` — constructs a fresh registry + pusher at startup, defers `PushContext` for end-of-run metric delivery, records run outcome and duration at every return path; adds `PUSHGATEWAY_URL` (default `http://pushgateway:9090`) and `TASK_TYPE` (default `unknown`) env fields; bumps `agent/lib` from `v0.62.4` to `v0.62.5`

## v0.23.35

- fix(pr-reviewer): bump `github.com/bborbe/agent/lib` v0.57.0 → v0.61.0 so `passthroughContentGenerator` writes a `## Failure` body section on BOTH `status: failed` AND `status: needs_input` results. Fixes 2026-05-12 incident on PR `bborbe/trading#122` where a Claude CLI 401 left the task page with no failure reason, forcing operators to race the agent pod's TTL cleanup to grab `kubectl logs`. Adds a factory-level regression test guarding the version pin against future accidental downgrade.

## v0.23.34

- feat(watcher/github-build): add `task_type: build-fix` to all emitted task commands; translate `assignee=human` to `""` per 2026-05-10 cross-repo doctrine

## v0.23.33

- feat(watcher/github-pr,watcher/github-build): make filename length caps configurable via env vars `MAX_SLUG_LEN` (default `80`, was `50` const) and `MAX_TITLE_LEN` (default `200`, unchanged). Bump of slug default from 50→80 preserves typical PR-title information that previously truncated mid-word. Watchers fail-loud at startup if either value is ≤0 or if MAX_SLUG_LEN >= MAX_TITLE_LEN. github-build only honors MAX_TITLE_LEN (no slug in build-failure filenames).
- chore(test): make `-race` flag opt-in via `RACE` Makefile variable (default `true` preserves local behaviour). CI sets `RACE=false` to sidestep ubuntu-latest+go1.26.3 segfault under `-race` in `agent/pr-reviewer` (run 25558544578). Race detection still on for local dev + can be re-enabled in CI by removing the env block when the runner issue is resolved.
- feat(watcher/github-pr): add `task_type: pr-review` to all emitted task commands; set `assignee: ""` on untrusted-author create and force-push paths per 2026-05-10 cross-repo doctrine

## v0.23.32

- refactor(watcher): rename internal `filename_hint` terminology to `title` across both watchers — function names (`computePRTitle`, `computeBuildTitle`), constants (`maxTitleLen`), comments, log messages, and stale doc sections updated; wire format unchanged (already on `Title` field per 0.31.x); contract tests preserved verbatim

## v0.23.31

- feat(watcher/github-build): add `/resetcursor/{repo}` admin endpoint to release stuck episode locks. Protected by `libhttp.NewDangerousHandlerWrapper` (passphrase rotated every 5min, logged on each unauthenticated hit). Use as: `curl 'https://<stage>.quant.benjamin-borbe.de/admin/maintainer-watcher-github-build/resetcursor/github.com/bborbe/<repo>?passphrase=<from-logs>'`

## v0.23.30

- feat(watcher): add `/setloglevel/{level}` endpoint to `maintainer-watcher-github-build` and `maintainer-watcher-github-pr` for runtime glog verbosity control (auto-resets after 5min)
- refactor(watcher/github-build): migrate create-task publish path to task.CreateCommandSender from agent/lib/command/task — removes WatcherCreateTaskCommand wrapper and hand-rolled kafkaPublisher; slug result now populates Title field; bumps agent/lib to v0.58.0

## v0.23.29

- refactor(watcher/github-pr): migrate create-task and update-frontmatter publish paths to task.CreateCommandSender / task.UpdateFrontmatterCommandSender from agent/lib/command/task — removes WatcherCreateTaskCommand wrapper; slug result now populates Title field; bumps agent/lib to v0.58.0

## v0.23.28

- feat(watcher/github-build): add include_logs opt-in — repos with watcher.github-build.include_logs: true in .maintenance.yaml receive an ## Error section in the task body containing the last 30 lines (≤ 4 KB) of the primary failing job's log, redacted for GitHub tokens, Bearer headers, AWS keys, and long hex strings; log fetch failures degrade silently without blocking the publish

## v0.23.27

- feat(watcher/github-build): replace failing-workflows bullet list with a Markdown table — columns: Workflow / Job / Failed Step / Run; failed-step names fetched via one jobs API call per failing run; degraded gracefully (shows ?) when the jobs API is unavailable

## v0.23.26

- feat(watcher/github-build): enrich task body header — commit subject, branch, event, started/finished timestamps, and elapsed duration now appear in every build-failure task body using fields already present in the workflow-run API response (zero extra API calls); all fields are optional and omitted gracefully when not populated

## v0.23.25

- feat(watcher/github-pr): emit filename_hint in CreateTaskCommand — PR-review vault tasks will land at "PR Review github - {owner}-{repo} - {number} - {slug}.md" once the companion bborbe/agent controller PR lands; existing controllers silently ignore the new field

## v0.23.24

- feat(watcher/github-build): emit filename_hint in CreateTaskCommand — build-failure vault tasks will land at "Build Failure github - {owner}-{repo} - {sha7}.md" once the companion bborbe/agent controller PR lands; existing controllers silently ignore the new field

## v0.23.23

- chore(watcher/github-build): rename pod-side env vars from `WATCHER_GITHUB_BUILD_TASK_*` to short `TASK_ASSIGNEE` / `TASK_STATUS` / `TASK_PHASE` (the StatefulSet template now maps long deploy-side names → short pod-side names; long names remain in `dev.env`/`prod.env` so the cross-service file stays unambiguous)
- chore(env): rename teamvault-key env vars to mirror source layout (`GH_BUILD_WATCHER_TOKEN_KEY` → `WATCHER_GITHUB_BUILD_GH_TOKEN_KEY`, `GH_PR_WATCHER_TOKEN_KEY` → `WATCHER_GITHUB_PR_GH_TOKEN_KEY`, `PR_REVIEWER_GITHUB_TOKEN_KEY` → `AGENT_PR_REVIEWER_GH_TOKEN_KEY`); secret manifests updated accordingly
- feat(env): expose build-watcher task-frontmatter overrides in `dev.env`/`prod.env` (`WATCHER_GITHUB_BUILD_TASK_ASSIGNEE=bborbe`, `WATCHER_GITHUB_BUILD_TASK_STATUS=in_progress`, `WATCHER_GITHUB_BUILD_TASK_PHASE=todo`) so the dev/prod fleet routes build-failure tasks to a human until the build-fixer agent ships
- feat(watcher/github-build): self-monitor — `bborbe/maintainer` added to the dev `REPO_ALLOWLIST` and a default `.maintenance.yaml` committed at repo root
- docs(maintainer): new `docs/verifying-specs.md` codifies the three-rung spec verification ladder (local `cmd/run-once` → dev k8s e2e → prod k8s e2e); CLAUDE.md links it as a BLOCKING rule before `dark-factory spec complete`
- docs(watcher/github-build): README env-vars table uses the short pod-side names with an explainer about the long→short deploy mapping

## v0.23.22

- feat(watcher/github-build): per-repo .maintenance.yaml overrides — build watcher reads watcher.github-build.{assignee,status,phase} from the repo's root on each green→red transition; missing file, malformed YAML, and API errors fall through silently to watcher defaults (WATCHER_GITHUB_BUILD_TASK_ASSIGNEE / WATCHER_GITHUB_BUILD_TASK_STATUS / WATCHER_GITHUB_BUILD_TASK_PHASE)

## v0.23.21

- feat(watcher/github-build): add `pkg/maintenance` package with `Loader` interface and `loaderImpl` that fetches `.maintenance.yaml` from a repo's default branch and returns a `GithubBuildConfig` with optional `Assignee`, `Status`, and `Phase` overrides; 404 is silent, all other failures log WARN and fall through to empty config
- feat(watcher/github-build): extend `GitHubClient` interface with `GetFileContent` method; implement on `*githubClient` using `Repositories.GetContents` with silent 404, rate-limit sentinel, and 1 MiB size guard

## v0.23.20

- feat(watcher/github-build): add WATCHER_GITHUB_BUILD_TASK_ASSIGNEE, WATCHER_GITHUB_BUILD_TASK_STATUS, WATCHER_GITHUB_BUILD_TASK_PHASE env vars so operators can override published task frontmatter at deploy time without a code change; empty WATCHER_GITHUB_BUILD_TASK_PHASE omits the phase key entirely

## v0.23.19

- fix(watcher/github-build): allowlist parser accepts host-qualified `host/owner/repo` entries (matches PR watcher; build watcher would previously refuse startup against the shared `REPO_ALLOWLIST` env value)

## v0.23.18

- fix(watcher/github-build): splitRepoKey now strips the host prefix from `host/owner/repo` allowlist entries so GitHub API calls use the correct `owner` and `repo` (regression from spec-015)

## v0.23.17

- feat(watcher/github-build): add k8s manifests (StatefulSet, Secret, Service, Makefile) for `maintainer-watcher-github-build` mirroring the PR watcher layout with inline PVC for cursor persistence
- docs(build-watcher): add `docs/build-watcher.md` documenting episode-SHA semantics, state machine, per-repo granularity rationale, red/green derivation rules, cold-start flood behavior, and v1 deviation from spec-015
- test(scenarios): add scenario 016 covering build watcher end-to-end — detect, idempotency, recover, and new episode on distinct SHA

## v0.23.16

- feat(watcher/github-build): new service polls GitHub Actions API for failed CI workflow runs on default branches; publishes `CreateTaskCommand` to Kafka on `green → red` transitions with deterministic UUID5 task ID (`assignee: build-fixer-agent`); re-polls are idempotent (same episode SHA = same task ID); `red → green` clears state without publishing closure (follow-up spec)

## v0.23.15

- feat(watcher/github-build): implement core state machine, cursor persistence, filter chain, and Prometheus metrics — `Poll()` converts GitHub Actions state into `CreateTaskCommand` Kafka messages with green/red episode tracking, idempotent publish, and atomic cursor writes

## v0.23.14

- feat(watcher/github-build): implement GitHub Actions API client (`GitHubClient` interface with `GetWorkflowRuns` and `GetDefaultBranch`), task ID derivation (`DeriveTaskID` with build-watcher-specific UUID namespace), and Kafka publisher (`CommandPublisher.PublishCreate`) with counterfeiter mocks and Ginkgo/Gomega test coverage ≥80%

## v0.23.13

- feat(watcher): scaffold `watcher/github-build/` module with Go module, Makefile, Dockerfile, pkg/doc.go, main.go (stub Run), and main_test.go; establishes env-var schema (GH_TOKEN, KAFKA_BROKERS, STAGE, POLL_INTERVAL, REPO_ALLOWLIST required) for the GitHub Actions build watcher service

- chore: rename repo `code-reviewer` → `maintainer`; module paths `github.com/bborbe/code-reviewer/...` → `github.com/bborbe/maintainer/...`; rename `watcher/github/` → `watcher/github-pr/` to disambiguate from upcoming `watcher/github-build/`. User-facing strings (`~/.code-reviewer.yaml`, `/tmp/code-reviewer-*` temp dirs, CLI usage prints) deferred to follow-up commit.
- chore(k8s): rename agent-pr-reviewer image, Config, Secret, PriorityClass, and ResourceQuota to `maintainer-agent-pr-reviewer`. Manifest filenames renamed to match. PVC `agent-pr-reviewer` and Config `volumeClaim` reference preserved (avoids `.claude/` OAuth re-seed). Config `assignee: pr-reviewer-agent` preserved (task contract). `Makefile.SERVICE` and `pkg/factory.serviceName` updated to match new image name.
- chore(cli): rename CLI binary `code-reviewer` → `pr-reviewer`. Move config to `~/.config/maintainer/pr-reviewer.yaml` (was `~/.code-reviewer.yaml`); cache to `~/.cache/maintainer/pr-reviewer/{repos,work}` (was `~/.cache/code-reviewer/...`); temp clone dirs to `/tmp/pr-reviewer-<repo>-pr-N` and prompt temp files to `pr-reviewer-prompt-*.md`. Hard cutover — users must `mv ~/.code-reviewer.yaml ~/.config/maintainer/pr-reviewer.yaml`. Updates `cmd/run-task/Makefile` smoke-test default repo to `bborbe/maintainer`.
- docs: sweep README.md (root + `agent/pr-reviewer/`), CLAUDE.md, `docs/architecture.md`, all active scenarios, and active specs to reflect repo + binary + path renames. In-code package and command doc-comments updated to reference `maintainer-agent-pr-reviewer` (matches deployed image / Kafka client_id). Scenario fix: `kubectl -n code-reviewer` → `kubectl -n dev` (namespace was already `dev` per `dev.env`; old scenarios were stale). Vault paths `secret/code-reviewer/tasks/` updated to `secret/maintainer/tasks/` — operational migration of the actual Vault data is a separate manual step.
- chore(watcher): rename `github-pr-watcher` → `maintainer-watcher-github-pr` to follow the `maintainer-{path}` convention. Image, StatefulSet/Service/Secret names, k8s manifest filenames, Makefile `SERVICE`, Kafka `client_id` / `Initiator`, package + command doc-comments, log line, watcher README, and scenarios all updated. StatefulSet rename loses the cursor PVC (`datadir-github-pr-watcher-0`) — watcher cold-starts with `BACKFILL_DURATION=720h` (30 days) on next deploy. Deterministic task IDs (`hash(owner/repo#N)`) prevent duplicate task creation during backfill. Old StatefulSet/Service/Secret/PVC must be deleted manually after the new resources are healthy.

## v0.23.12

- feat(k8s): wire `REPO_ALLOWLIST` env var through both `agent/pr-reviewer` Config CRD and `watcher/github` StatefulSet manifests so the value flows from `dev.env` / `prod.env` into pod env at deploy time. Without these manifest entries the watcher and agent never saw the host-qualified allowlist their code already reads.
- fix(test): replace hardcoded `/bin/true` and `/bin/false` paths with bare `true` / `false` in `pkg/githubauth` test so `make precommit` works on macOS hosts (where `/bin/true` is absent — only `/usr/bin/true` exists).
- test(scenario): add scenario 014 covering private GitHub repo PR review end-to-end (private clone via `gh auth setup-git`, no token leak in pod logs, public-repo regression check).
- chore(env): update `dev.env` to `REPO_ALLOWLIST=github.com/bborbe/go-skeleton` (single test-bed repo) and `prod.env` to `REPO_ALLOWLIST=github.com/bborbe/code-reviewer,github.com/bborbe/jira-task-creator` (host-qualified production scope including private jira-task-creator).

## v0.23.11

- feat(pr-reviewer): translate git auth-failure clone errors to `AgentStatusNeedsInput`, routing private-repo tasks to human review with a diagnostic naming `host/owner/repo` and a `GH_TOKEN` config hint. Adds `git.IsGitAuthFailure` helper covering known GitHub auth-failure substrings.

## v0.23.10

- feat(pr-reviewer): add `pkg/githubauth` package with `GitHubAuthSetup` interface, real `GhAuthSetupGit` implementation (runs `gh auth setup-git` at pod startup when `GH_TOKEN` is set), and `NoopAuthSetup` (used by `cmd/run-task`). Wire through `factory.RunConfig.AuthSetup` so pods authenticate git against GitHub private repos; local-CLI mode is unaffected.

## v0.23.9

- feat(pr-reviewer): add `REPO_ALLOWLIST` env var (comma-separated `host/owner/repo` entries) that blocks cloning repos not on the configured list. Non-allowlisted tasks return `NeedsInput` and are routed to human review without cloning. Empty allowlist is allow-all. Extends `git.ParseCloneURL` with a `ParseCloneURLParts` sibling that exposes host/owner/repo as separate fields.

## v0.23.8

- feat(watcher): add `REPO_ALLOWLIST` env var (comma-separated `host/owner/repo` entries) that restricts task creation to configured repos. Empty allowlist is allow-all (preserves today's behavior). Malformed entries cause startup failure with a clear log. Adds `RepoAllowlistFilter` leaf to the `TaskCreationFilter` chain. Updated `dev.env` and `prod.env` to host-qualified form (`github.com/bborbe/code-reviewer`).

## v0.23.7

- fix(github-pr-watcher): write `clone_url`, `ref`, `base_ref` to task frontmatter on PR creation so the agent's execution phase has the fields it needs to clone and diff. Without them every watcher-triggered review failed at `execution step: clone_url is missing from task frontmatter`, escalating to `phase: human_review` with no `## Review` section ever written. Adds a new `GetPRDetails` GitHub API method (replacing `GetHeadSHA`) that returns head SHA + clone URL + base ref in one PullRequests.Get call.

## v0.23.6

- refactor(pr-reviewer): extract shared startup orchestration into `factory.RunAgent` so both entry points (`main.go` for Kafka pod, `cmd/run-task/main.go` for local CLI) install the `bborbe/coding` plugin and prune stale worktrees identically. Closes the silent-degradation gap where local smoke runs skipped `PluginInstaller` and produced reviews without specialist sub-agent dispatch.

## v0.23.5

- feat(pr-reviewer): default `ClaudeConfigDir` arg to `~/.claude` in both entry points (`main.go` and `cmd/run-task/main.go`). Defense-in-depth: prevents the silent "empty CLAUDE_CONFIG_DIR → plugin not discoverable" failure mode hit in dev on 2026-05-02. K8s deploys still take their explicit `CLAUDE_CONFIG_DIR` from the Config CRD env (env > arg default).

## v0.23.4

- feat(watcher): add `BACKFILL_DURATION` env var (default 30 days) that backdates the initial cursor on cold start. First deploy now picks up PRs updated within the configured window instead of returning zero PRs until organic activity arrives. Once `cursor.json` exists, the env var is ignored.

## v0.23.3

- feat(watcher): add `WIPTitleFilter` (skip PRs with `WIP:` / `WIP ` title prefix) and `AgeFilter` (skip PRs older than `MAX_PR_AGE`, default 90 days). Both extend the `TaskCreationFilter` chain. Configurable via `MAX_PR_AGE` env var (libtime extended duration; empty disables age filter, negative rejected at startup).

## v0.23.2

- refactor(watcher): introduce composable `TaskCreationFilter` chain in `watcher/github/pkg/filter/` (interface + `DraftFilter`/`BotAuthorFilter` leaves + slice composite). Replaces the single `ShouldSkipPR` function. No behavior change. Adds `docs/watcher-decision-chains.md` documenting the split between TaskCreationFilter and TrustGate.

## v0.23.1

- docs: add scenario 006 manual verification checklist for spec-012 watcher author-trust gate

## v0.23.0

- feat: add trusted-authors trust gate to github-pr-watcher; untrusted PR authors are routed to human_review instead of auto-processing; watcher refuses to start without a non-empty TRUSTED_AUTHORS list

## v0.22.0

- feat: add `watcher/github/pkg/trust` boolean-combinator package with `Trust` interface, `And`/`Or`/`Not` combinators, `NewAuthorAllowlist` leaf, and `ParseTrustedAuthors` filter helper

## v0.21.3

### Fixed
- **pr-reviewer**: inline `/coding:pr-review` plugin content into the execution-phase
  prompt so plugin orchestration actually fires. Previously the wrapper described the
  slash command in prose, but Claude reads it as documentation and never invokes the
  plugin — slash commands don't trigger from inside a multi-section structured prompt.
  The plugin file is now read at runtime, frontmatter stripped, and arguments pre-filled
  before being concatenated with a verdict-translation footer.

## v0.21.2

- fix: add pod-level `securityContext.fsGroup: 65534` to `watcher/github/k8s/github-pr-watcher-sts.yaml` so the `datadir` PVC mount is group-owned by the non-root UID, fixing `open /data/cursor.json: permission denied` on every poll cycle

## v0.21.1

- docs: add scenario 005 manual verification checklist for `/coding:pr-review` plugin delegation end-to-end (slash command invocation, sub-agent fan-out, verdict JSON schema, workdir cleanup)

## v0.21.0

- feat: replace hand-rolled execution-phase prompt with `/coding:pr-review` plugin delegation; add `-review-mode` flag (short|standard|full, default standard); update `executionTools` to match plugin's declared tool requirements

## v0.19.1

- chore: raise `ephemeral-storage` from `2Gi` to `5Gi` in both requests and limits for agent-pr-reviewer K8s Config CR to accommodate full-size git clones on overlayfs

## v0.19.0

- feat: wire `RepoManager` into the execution phase — `checkoutExecutionStep` checks out the target ref as an on-disk worktree and runs Claude in that directory; update `CreateAgent` to accept `git.RepoManager`; add `REPOS_PATH`/`WORK_PATH` env vars to K8s and run-task entry points with startup `PruneAllWorktrees`; narrow `executionTools` to read-only git operations; replace `gh pr diff` in `execution_workflow.md` with on-disk worktree inspection instructions

## v0.18.0

- feat: add `RepoManager` interface with bare-clone caching, per-task worktrees, and stale-worktree pruning in `agent/pr-reviewer/pkg/git/`; add `ParseCloneURL` and `WorkdirConfig` supporting the same package

## v0.17.4

- refactor: flatten `agent/pr-reviewer/pkg/` by collapsing single-consumer subpackages (`config`, `prurl`, `review`, `verdict`, `version`, `steps`) into flat files in `pkg/`, and merging `pkg/prompts/{execution,planning,review}/` into a single `pkg/prompts/` package

## v0.17.3

- test: add Ginkgo/Gomega tests for `reviewStep.Name`, `ShouldRun`, and `Run` in `agent/pr-reviewer/pkg/steps`, covering all four `Run` branches (runner error, unparseable output, verdict pass, verdict fail); coverage reaches 93.3%

## v0.17.2

- test: add coverage for `publishForcePush` Kafka failure, `fetchHeadSHA` error and cache-hit paths in `watcher/github`

## v0.17.1

- chore: add container securityContext to `watcher/github` StatefulSet — runAsNonRoot, runAsUser 65534, allowPrivilegeEscalation false, readOnlyRootFilesystem true, drop ALL capabilities; add emptyDir /tmp volume for runtime scratch space

## v0.17.0

- feat: add Prometheus metrics to `watcher/github` — poll cycle counter (`github_pr_watcher_poll_cycles_total`) and PR-processed counter (`github_pr_watcher_prs_total`) with pre-initialized label values; inject `Metrics` interface into `Watcher` via constructor

## v0.16.25

- refactor: extract routing logic from `factory.CreateClaudeRunner` and `factory.CreateDeliverer` into `main.go`; return `AgentRunner` interface from `CreateAgent`; inject `libtime.CurrentDateTimeGetter` from caller

## v0.16.24

- fix: rebuild `HeadSHAs` from current open-PR batch each poll cycle to prevent unbounded growth from closed/merged PRs

## v0.16.23

- fix: propagate `LoadCursor` error from `Poll` instead of swallowing it, pass `*Cursor` to `processPRs`/`handlePR`/`publishCreate`/`publishForcePush` to make HeadSHAs mutation explicit in function signatures

## v0.16.22

- refactor: replace per-call `make(chan base.RequestID, 1)` + `base.NewCommandCreator` in `buildCommandObject` with a long-lived `commandCreator` field on `kafkaPublisher`, initialized once via `base.RequestIDChannel(ctx)` in `NewCommandPublisher`

## v0.16.21

- refactor: move `ParseBotAllowlist` from `watcher/github/pkg/factory` to `watcher/github/pkg`, log `syncProducer.Close()` error instead of discarding it, remove unused `pollInterval` parameter from `CreateWatcher`
- test: add `ParseBotAllowlist` test cases to `watcher/github/pkg/filter_test.go`

## v0.16.20

- refactor: replace `time.Time` with `libtime.DateTime` in `watcher/github` struct fields and function signatures, inject `libtime.CurrentDateTimeGetter` in `main.go` instead of calling `time.Now()` directly

## v0.16.19

- fix: add 30s timeout to Bitbucket HTTP client to prevent slow-server goroutine exhaustion
- fix: upgrade non-loopback `http://` hosts to `https://` in `buildURL` to prevent cleartext credential transmission
- refactor: replace all `fmt.Errorf` calls in `agent/pr-reviewer/pkg/bitbucket/client.go` with `errors.Errorf`/`errors.Wrapf` from `github.com/bborbe/errors`

## v0.16.18

- fix: validate branch names before `git checkout` in `CreateClone` to prevent argument injection via hyphen-prefixed or traversal branch names
- refactor: replace all `fmt.Errorf` calls in `agent/pr-reviewer/pkg/git/git.go` with `errors.Errorf`/`errors.Wrapf` from `github.com/bborbe/errors`

## v0.16.17

- refactor: replace `oauth2.StaticTokenSource` + `context.Background()` with `gogithub.NewClient(nil).WithAuthToken(token)` in `watcher/github/pkg/githubclient.go`

## v0.16.16

- fix: validate `REPO_SCOPE` env var against `^[a-zA-Z0-9_.-]+$` at startup in `watcher/github` to prevent query injection via malformed scope values

## v0.16.15

- refactor: migrate `fmt.Errorf` to `errors.Wrapf/Errorf(ctx, ...)` in `pkg/review`, `pkg/github/client.go`, `pkg/config/config.go`, and `pkg/steps/review.go`; replace `log.Printf` warning with `glog.Warningf`; thread `ctx` through `validateConfig`, `FindRepo`, `extractVerdict`, and `lastJSONBlock`

## v0.16.14

- refactor: add `ctx context.Context` to `prurl.Parse` and internal helpers, replace all `fmt.Errorf` with `errors.Errorf(ctx, ...)` for context-tagged stack trace errors

## v0.16.13

- refactor: replace `errors.Wrapf` with `errors.Wrap` where format string has no `%` verbs in watcher/github/pkg and watcher/github/pkg/factory

## v0.16.12

- refactor: remove deprecated `ParsePRURL`/`PRInfo` from `pkg/github` and dead `FindRepoPath` method from `pkg/config`

## v0.16.11

- docs: add watcher/github/README.md documenting env vars, cursor mechanism, and relationship to pr-reviewer; update root README Layout section

## v0.16.10

- docs: fix six GoDoc comments in watcher/github/pkg to start with the declared item name; add package-level doc.go

## v0.16.9

- chore: add tools.go to watcher/github to pin tool dependencies and prevent go mod tidy from dropping them

## v0.16.8

- fix: watcher/github pkg suite uses GinkgoConfiguration() with 60s timeout; replace time.Now() with fixed date in test fixtures for determinism

## v0.16.7

- fix(agent/pr-reviewer): update five Ginkgo suite files to use four-argument `RunSpecs` with `GinkgoConfiguration()` and 60-second timeout so suites respect Ginkgo configuration flags

## v0.16.6

- fix(watcher/github): drop runtime rate-limit pre-check (`rateSafeThreshold`) — the threshold (10) was set assuming REST API (5000/hr) but applied to Search API (10/min), causing every poll cycle to abort after the first call. Tokens are for use; broken-token validity is checked separately by `make verify-gh-token`. On 403 the search call returns an error, the cycle aborts, next 5-min tick retries.
- feat(agent/pr-reviewer): `make verify-gh-token` now prints per-bucket usage (core/search/graphql/code_search) with reset countdown — exposes that Search API is 10/min not 5000/hr (root cause of the watcher bug above).

## v0.16.5

- feat(watcher/github): add admin `/trigger` HTTP endpoint — fires an out-of-band poll cycle on demand via `libhttp.NewBackgroundRunHandler` (async, ParallelSkipper-deduped). Refactors poll logic into a shared `pollOnce run.Func` reused by cron loop + handler.

## v0.16.4

- refactor(agent/pr-reviewer): swap local `pkg/plugins` for `agent/lib/claude.PluginInstaller` (lib/v0.56.0). Drops 4 files; behavior unchanged. Phase 2 of EnsurePluginsInstaller task.

## v0.16.3

- chore: generate fix prompts from full code review of watcher/github — context.Background() in constructor, libtime migration, Prometheus metrics, error wrapping, factory cleanup, test coverage gaps, scope injection validation, K8s security context, and more

## v0.16.2

- chore: generate fix prompts from full code review of agent/pr-reviewer — security hardening (HTTP timeout, branch validation), error wrapping migration, factory pattern compliance, test quality, and dead code cleanup

## v0.16.1

- refactor(watcher/github): flatten `pkg/` per `coding/docs/go-composition.md` — collapsed `pkg/{cursor,filter,githubclient,publisher,taskid,watcher}/` subpackages into a single `pkg/` with one file per former subpackage; renamed colliding identifiers (`State`→`Cursor`, `Load/Save`→`LoadCursor/SaveCursor`, `ShouldSkip`→`ShouldSkipPR`, `Derive`→`DeriveTaskID`, `publisher.New`→`NewCommandPublisher`); consolidated all counterfeiter mocks into `pkg/mocks/`
- refactor(watcher/github): replace Deployment + standalone PVC with StatefulSet + embedded `volumeClaimTemplates` per trading converter pattern; add headless Service; add livenessProbe/readinessProbe + prometheus annotations + keel auto-deploy annotations + node affinity + imagePullSecrets; drop unused PriorityClass + ResourceQuota (not the convention for stateful services in this org)
- feat(watcher/github): add HTTP server (`/healthz`, `/readiness`, `/metrics`) running concurrently with the poll loop via `bborbe/run.CancelOnFirstFinish`; new `LISTEN` env var (default `:9090`)

## v0.16.0

- feat: add github-pr-watcher service (watcher/github/) — polls GitHub Search API and publishes CreateTaskCommand/UpdateFrontmatterCommand to Kafka for automatic PR review triggering
- feat: add k8s manifests for github-pr-watcher (Deployment, PVC, Secret, ResourceQuota dev+prod)

## v0.15.3

- feat(watcher/github): implement full poll cycle — cursor persistence, Kafka command publishing (`CreateTaskCommand`/`UpdateFrontmatterCommand`), force-push detection, rate-limit backoff, and wired main.go tick loop

## v0.15.2

- feat(watcher/github): add GitHub API layer — `GitHubClient` interface with `SearchPRs`/`GetHeadSHA`, `filter.ShouldSkip`, and `taskid.Derive` deterministic task ID derivation using UUID v5

## v0.15.1

- feat(watcher/github): add github-pr-watcher service scaffold (go.mod, Makefile, Dockerfile, main.go skeleton)

## v0.15.0

- feat: add plugin installer library (`pkg/plugins/`) ensuring Claude Code plugins are installed before task handling
- feat: wire plugin installer into agent-pr-reviewer startup — ensures `bborbe/coding` plugin is present on every pod boot
- docs: add `docs/claude-plugin-cli.md` documenting claude plugin CLI derivation rules

## v0.14.3

- feat(pr-reviewer): add `pkg/plugins` package with `Installer` interface and `NewExecCommander` for managing Claude Code plugins (install/update via `claude plugin` CLI)

## v0.14.2

- feat(pr-reviewer): preflight GH_TOKEN check as step 0 in every phase. New `pkg/steps/gh_token.go` hits GitHub's `/rate_limit` endpoint (free, doesn't count against the limit) before each Claude call. Routes failures explicitly:
  - empty token → `needs_input` → `human_review` (non-retryable)
  - HTTP 401 → `needs_input` → `human_review` (non-retryable, with truncated GH error body)
  - rate limit < 1000/hr → `needs_input` → `human_review` (token degraded to anonymous, e.g. revoked or scope-stripped)
  - remaining quota < 10 → `failed` → controller retries after backoff
  - network error / non-200 → `failed` → controller retries
  - healthy PAT → `done + ContinueToNext` (the actual Claude step runs next)
- Catches the exact failure mode that wasted 3 jobs in the v0.14.1 e2e smoke test: a teamvault-stored token that authenticates as user but rate-limits as anonymous. The agent now stops at preflight (~200ms, 1 HTTP call) with an actionable message instead of running through 3 phases of confusing "rate limit exceeded" errors from inside the LLM.
- 9 table-driven tests in `pkg/steps/gh_token_test.go` covering all branches via `httptest.Server`.
- New `make verify-gh-token` target in `agent/pr-reviewer/Makefile` — same check from the command line, useful before deploying.

## v0.14.1

- fix(pr-reviewer): tolerate prose around JSON in ai_review verdict. Caught during local smoke against PR #2: Claude prefixed the verdict JSON with explanatory prose despite the prompt asking for raw JSON only, causing `json.Unmarshal` to fail and incorrectly route to `human_review`. New `extractVerdict` walks the LLM response in 3 stages — direct unmarshal, fence-stripped unmarshal, last-balanced-`{...}`-block extraction — covered by 11 table-driven test cases in `pkg/steps/review_test.go`.

## v0.14.0

- feat(pr-reviewer): per-phase decomposition. Replace single shared Claude step with 3 distinct steps:
  - `planning` — read-only diff inspection (`git diff`, `gh pr view/diff`); writes `## Plan` JSON (files, scope, focus areas, concerns)
  - `in_progress` — read + cross-file inspection; reads `## Plan`, writes `## Review` JSON (verdict, summary, comments, concerns_addressed)
  - `ai_review` — minimal read-only fresh-context verifier; reads `## Plan` + `## Review`, writes `## Verdict` JSON; conditional next-phase routing — `verdict=pass` → `done`, anything else → `human_review`
- New per-phase prompt modules under `pkg/prompts/{planning,execution,review}/` with workflow.md + output-format.md; old generic prompt removed
- New `pkg/steps/review.go` — custom AgentStep that parses verdict JSON to drive conditional NextPhase
- Per-phase tool scopes in factory: planning + review are read-only; execution gets broader git/gh access; none can post comments (posting stays out-of-band after human approves verdict)

## v0.13.0

- refactor(pr-reviewer): migrate to agent framework (lib v0.54.0). Drop `claudelib.TaskRunner` / `NewResultDelivererAdapter` / `FallbackContentGenerator`; use `lib.NewAgent` + `claude.NewAgentStep` shared across 3 phases (planning, in_progress, ai_review) with `## Review` output section. Factory exposes `CreateAgent` + `CreateDeliverer` matching the canonical `agent/claude` shape. main.go gains typed `Phase` field; both entry points (Kafka main.go + cmd/run-task) updated.

## v0.12.3

- chore: bump github.com/bborbe/agent/lib to v0.53.1 (route-failures: failed status → phase: human_review + ## Failure section; UpdateFrontmatterCommand for spawn/failure notifications)

## v0.12.2

- Rework root README to document three modes (CLI, local task runner, k8s Job agent); add k8s deploy, prerequisites, trigger instructions, debug commands, full repo layout

## v0.12.1

- Replace os.Getenv passthrough in factory with typed GHToken argument on both main.go entries — factory receives ghToken string and wires GH_TOKEN into ClaudeRunnerConfig.Env only when non-empty

## v0.12.0

- Specialize pr-reviewer factory: hardcode AllowedTools + GH_TOKEN passthrough and move prompts.BuildInstructions() inside; drop AllowedToolsRaw/EnvContextRaw/ClaudeEnvRaw CLI args and parseKeyValuePairs helper from both main.go entries
- Type TaskID field as agentlib.TaskIdentifier directly (no string conversion)
- Rename Secret data key `PR_REVIEWER_GITHUB_TOKEN` → `GH_TOKEN` so gh CLI picks it up natively; drop `ALLOWED_TOOLS` from Config CRD env
- Add `make apply` target; `make buca` passes DOCKER_REGISTRY from env
- Add github-cli + git to container image; shrink image via `apk del npm` + `npm cache clean`
- Harden Makefile.env: error on invalid BRANCH, outdent conditional includes (Make parsed as recipe)

## v0.11.0

- feat: add k8s manifests for pr-reviewer (Config CRD, PVC, Secret, PriorityClass, ResourceQuota dev+prod, Makefile)

## v0.10.0

- Add `agent/pr-reviewer/main.go` + `main_test.go` — k8s job entry point mirroring `bborbe/agent/agent/claude/main.go` (task-content via env, optional Kafka result delivery on `TASK_ID`, configurable `AllowedTools`/`EnvContext`/`ClaudeEnv`)
- Add `agent/pr-reviewer/pkg/factory/factory.go` — wires `TaskRunner`, `ClaudeRunner`, `SyncProducer`, and `KafkaResultDeliverer` (verbatim claude factory, `serviceName = agent-pr-reviewer`)
- Add `agent/pr-reviewer/pkg/prompts/` with embedded `workflow.md` + `output-format.md` via `//go:embed`
- Rewrite `agent/pr-reviewer/cmd/run-task/main.go` as claude-style local runner: reads task file, uses `FileResultDeliverer` to write result back, configurable allowed-tools
- Update `cmd/run-task/Makefile` claude-style with `ALLOWED_TOOLS=Read,Grep,Glob,Bash(git:*),Bash(gh:*),WebFetch`; generates/runs dummy PR-review task
- Add `agent/pr-reviewer/agent/.claude/CLAUDE.md` — headless PR reviewer guardrails (no internal network, no state mutation, JSON-only output)
- Simplify `agent/pr-reviewer/Makefile` to use shared `Makefile.variables` + `Makefile.precommit` includes (reduced from ~100 to 14 lines); keeps own `install`/`run` targets with `VERSION`/`LDFLAGS`
- Fix `Makefile.precommit` `goimports-reviser` project-name from `github.com/bborbe/agent` to `github.com/bborbe/code-reviewer`
- Fix root `Makefile.folder` `DIRS` discovery to match bborbe/agent (`find */* -maxdepth 0`) so it targets service dirs (`agent/pr-reviewer`) instead of recursing into `cmd/run-task`
- Add `agent/pr-reviewer/Dockerfile` mirroring `bborbe/agent/agent/claude/Dockerfile` (multi-stage build → `/main` + `claude-code` CLI)
- Add `agent/pr-reviewer/.gitignore` (`.update-logs/`, `.claude/`, `CLAUDE.md`, `.mcp-*`, `cover.out`)
- Add `agent/pr-reviewer/README.md` describing service layout, entry points (local CLI vs planned k8s job), verdict contract
- Add root `common.env`, `default.env`, `dev.env`, `prod.env` (copied from bborbe/agent) so shared `Makefile.env`/`Makefile.docker` work
- Promote `bborbe/agent/lib`, `cqrs`, `kafka`, `sentry`, `service`, `time`, `golang/glog` to direct deps in `agent/pr-reviewer/go.mod`; bump `golang.org/x/vuln` to v1.2.0

## v0.9.0

- Transform repo to multi-module layout (bborbe/agent pattern): service at `agent/pr-reviewer/` with own `go.mod`, binary entry point at `cmd/run-task/main.go`; root has no `go.mod`
- Root `Makefile` delegates `precommit`/`test`/`lint` to service dirs via `Makefile.folder` (auto-discovers Makefiles at any depth)
- Copy shared Makefile includes from bborbe/agent: `Makefile.docker`, `Makefile.env`, `Makefile.k8s`, `Makefile.precommit`, `Makefile.variables`
- Update module path to `github.com/bborbe/code-reviewer/agent/pr-reviewer`; rewrite all imports and LDFLAGS
- Binary renamed `pr-reviewer` → `run-task` (matches Pattern B Job convention)
- `.golangci.yml`, `.osv-scanner.toml`, `.trivyignore` stay at repo root; service Makefile references via `../../`

## v0.8.0

- Rename module path from `github.com/bborbe/pr-reviewer` to `github.com/bborbe/code-reviewer` (repo renamed to cover broader scope)
- Update all imports, Makefile ldflags, prompts, specs, and docs to new module path
- Upgrade `github.com/go-git/go-git/v5` from v5.17.2 to v5.18.0 (security fix)
- Remove stale OSV ignore entries for GO-2026-4923, GHSA-6jwv-w5xf-7j27, GHSA-xmrv-pmrh-hhx2

## v0.7.4

- Update golangci-lint to v2.11.4
- Update osv-scanner to v2.3.5
- Update gosec to v2.25.0
- Update multiple indirect dependencies
- Bump Go toolchain to 1.26.2

## v0.7.3

- Update dependencies (docker, containerd, prometheus, otel, golang.org/x)
- Upgrade go-git to v5.17.2
- Upgrade moby/buildkit to v0.29.0
- Remove stale exclude and replace directives from go.mod

## v0.7.2

- upgrade golangci-lint from v1 to v2
- standardize Makefile: multiline trivy
- update .golangci.yml to v2 format
- setup dark-factory config

## v0.7.1

- go mod update

## v0.7.0

- Add --version flag to print build-time version and exit

## v0.6.0

- Use YOLO_OUTPUT=print for raw text output instead of stream-json extraction
- Update default container image to claude-yolo v0.2.0

## v0.5.9

- Fix Docker executor to use YOLO_PROMPT_FILE and YOLO_MODEL env vars (matching dark-factory pattern)
- Extract review result from stream-formatter output via --- DONE --- marker

## v0.5.8

- Replace git worktree with git clone --local for Docker-compatible standalone repos

## v0.5.7

- Remove useDocker toggle, always use Docker (claude-yolo) for reviews
- Remove host-based claudeReviewer

## v0.5.6

- Add configurable autoApprove field (default false) to guard approve API calls
- Refactor submitGitHubReview and submitBitbucketReview to respect autoApprove setting

## v0.5.5

- Add Docker-based review executor using claude-yolo container
- Add useDocker and containerImage config fields
- Mount ~/.claude-yolo as Claude config inside container

## v0.5.4

- Strip JSON verdict block from review text before posting as PR comment

## v0.5.3

- Add JSON verdict parser (parseJSONVerdict) with fallback to heuristic section scanning
- Add StripJSONVerdict to remove verdict block from review output
- Support JSON verdict inside markdown code fences

## v0.5.2

- Fix verdict parser treating markdown horizontal rules (---) as must-fix content

## v0.5.1

- Fix Bitbucket needs-work verdict by replacing broken /profile endpoint with configurable username
- Remove GetProfile from Bitbucket client (404 on Bitbucket Data Center)
- Add bitbucket.username config field for needs-work participant API
- Fix /pr-review vendor/node_modules exclusion for nested directories

## v0.5.0

- Use /pr-review command with target branch for diff-scoped reviews instead of /code-review
- Fetch both source and target branch from GitHub and Bitbucket PR APIs
- Rename GetPRBranch to GetPRBranches returning source and target branch pair

## v0.4.1

- Add progress logging for long-running operations (fetch, worktree, review, post)
- Move worktrees to /tmp to avoid polluting repo directory
- Add robust stale worktree cleanup with fallback to force-remove

## v0.4.0

- Add Bitbucket Server support: parse PR URLs, fetch branch, post comments via REST API
- Add platform-agnostic URL parser (pkg/prurl) supporting GitHub and Bitbucket Server
- Add Bitbucket API client (pkg/bitbucket) with Bearer token auth and error handling
- Add Bitbucket token configuration with BITBUCKET_TOKEN env var default
- Route GitHub and Bitbucket URLs to respective clients in main workflow

## v0.3.0

- Wire verdict-based review submission into main workflow
- Add --comment-only flag to skip verdict and post as plain comment
- Log detected verdict and reason to stderr

## v0.2.0

- Add verdict parser for review output analysis (approve/request-changes/comment)
- Add SubmitReview to GitHub Client for structured review submission via gh CLI
- Add verbose version display and token debug logging
- Add build-time version injection via pkg/version
- Clean up default GitHub token constant

## v0.1.1

- Fix LICENSE year from 2016 to 2025
- Fix README license type from BSD 3-Clause to BSD 2-Clause
- Update README token example to PR_REVIEWER_GITHUB_TOKEN
- Add CLAUDE.md to .gitignore
- Default github token to ${PR_REVIEWER_GITHUB_TOKEN} env var

## v0.1.0

- Initial project setup
