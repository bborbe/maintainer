# Changelog

All notable changes to this project will be documented in this file.

Please choose versions by [Semantic Versioning](http://semver.org/).

* MAJOR version when you make incompatible API changes,
* MINOR version when you add functionality in a backwards-compatible manner, and
* PATCH version when you make backwards-compatible bug fixes.

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
