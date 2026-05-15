---
status: generating
tags:
    - dark-factory
    - spec
approved: "2026-05-15T19:35:40Z"
generating: "2026-05-15T19:35:41Z"
branch: dark-factory/migrate-callers-to-repoallowlist-lib-and-wildcard-rollout
---

## Summary

- Migrate all five binaries that currently parse `REPO_ALLOWLIST` inline to import the shared `lib/repoallowlist` package landed by the sibling bootstrap spec, eliminating parser drift in a single atomic change.
- Switch `dev.env` and `prod.env` from explicit repo lists to a single `github.com/bborbe/*` wildcard entry, removing the per-repo maintenance toil that scales with both repo count and binary count.
- Update the vault runbook so its primary role flips from "how to add the next bborbe repo" to "how to add a repo outside the bborbe org".
- Ship as one atomic deploy with strict ordering: code change first (still accepts literals), then env change (introduces the wildcard) — never the reverse.
- Verifies at Rung-2 (dev cluster e2e with a live PR in a bborbe repo not previously in the literal list) and Rung-3 (prod) per `docs/verifying-specs.md`.

## Problem

`REPO_ALLOWLIST` is consumed by five separate binaries (PR reviewer agent + its local CLI, the PR watcher, the build watcher + its run-once CLI). Each has its own inline parser, all literal-only. The parent goal's success criterion #7 — "Configurable scope — env-driven repo filter; default `bborbe/*` org-wide" — cannot ship until every consumer accepts the same wildcard syntax: the PR watcher might spawn a task for a new bborbe repo, but the agent's defense-in-depth allowlist would still reject it, and the build watcher's required-non-empty startup check would refuse the wildcard outright. The sibling spec 028 has now produced a shared library that handles all three concerns (literal match, `github.com/<owner>/*` wildcard, well-formedness validation), but no caller imports it yet. Until every caller migrates in a single coordinated change, the wildcard cannot be safely turned on in any env.

## Goal

After this work, every binary that consumes `REPO_ALLOWLIST` evaluates the env value through the same shared library — no inline parsing remains anywhere in `agent/` or `watcher/`. Both `dev.env` and `prod.env` express the operator's intent with a single `github.com/bborbe/*` entry instead of an enumerated repo list. The runbook reflects the new default. A PR opened in a bborbe repo that was previously absent from the literal list triggers the full watcher → controller → agent → review pipeline with no further operator intervention.

## Non-goals

- Do NOT modify the `lib/repoallowlist` package — its API (`IsAllowed`, `Validate`) is a frozen interface from spec 028.
- Do NOT introduce prefix patterns (`github.com/bborbe/agent-*`). Only the `github.com/<owner>/*` shape ships.
- Do NOT add cross-platform support (Bitbucket, GitLab) or negative/exclusion patterns.
- Do NOT add dynamic env reload; allowlist changes still require redeploy.
- Do NOT change the optional-vs-required policy of any binary — the PR reviewer, its run-task CLI, and the PR watcher remain "empty = allow-all"; the build watcher and its run-once CLI remain "required, non-empty".

## Desired Behavior

1. Every binary that today parses `REPO_ALLOWLIST` inline (PR reviewer, its run-task CLI, the PR watcher, the build watcher, the build watcher's run-once CLI) imports the shared `lib/repoallowlist` package and routes its allowlist decision through it. No `strings.Split` over `REPO_ALLOWLIST` and no `filepath.Match`-on-allowlist code remains in any `main.go` under `agent/` or `watcher/`.
2. Each "optional" caller (PR reviewer, its run-task CLI, the PR watcher) preserves "empty allowlist means allow all" semantics by delegating to the library's predicate, which already encodes this rule.
3. Each "required, non-empty" caller (build watcher main + run-once) calls the library's validator at startup and refuses to start if the env value is empty or contains any malformed entry. The aggregate error from the validator surfaces every malformed entry in a single fail-fast message, not one at a time.
4. `dev.env` and `prod.env` each reduce their `REPO_ALLOWLIST` line to exactly `github.com/bborbe/*`. No other repo-allowlist env-var name needs editing — `REPO_ALLOWLIST` is the only one in use across the project.
5. The Obsidian runbook for adding a repo to the allowlist is updated to state that `github.com/bborbe/*` is now the default and that the runbook's primary use case has shifted to repos outside the bborbe org; per-repo additions inside `bborbe/` are documented as still possible but rarely needed.
6. A new `CHANGELOG.md` entry under `## Unreleased` describes the migration and the env-format change.
7. Rollout follows a strict ordering: the code change (still accepting literals) ships and deploys to dev first; only after dev pods run the new image with the old literal env does the env change ship. Same ordering in prod.

## Constraints

- The `lib/repoallowlist` API (module path `github.com/bborbe/maintainer/lib`, package `repoallowlist`, functions `IsAllowed` and `Validate`) is frozen by spec 028 — this spec consumes it unchanged.
- The existing optional-vs-required policy per binary is preserved: the PR reviewer, its run-task CLI, and the PR watcher still allow an empty allowlist (allow-all); the build watcher and its run-once CLI still refuse to start on empty allowlist.
- The Kafka schemas, env-var names, k8s manifests, and frontmatter contracts of every affected binary stay unchanged. The only env-value change is the literal-list → wildcard substitution.
- Errors continue to be constructed and wrapped exclusively with `github.com/bborbe/errors`; logging continues to use `glog`.
- Deploy ordering is a constraint, not a suggestion: **code first, env second**. Reversing the order would cause the build watcher's pre-migration startup validator to reject the wildcard env value and crashloop the pod.
- `CHANGELOG.md` gets an entry under `## Unreleased`. The autoRelease tag is created by dark-factory at spec-complete time, not by this spec.
- No changes inside `lib/` — that directory is owned by spec 028 and any future shared-library spec.
- See `docs/verifying-specs.md` for the rung model this spec verifies against; see `docs/architecture.md` for the watcher → controller → agent pipeline this spec exercises end-to-end.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Operator updates `dev.env` to the wildcard before the new image is deployed | Build watcher pod crashloops because its old inline parser rejects `github.com/bborbe/*` as a malformed literal. | Roll back env to the prior literal list or fast-forward the image deploy. Spec's ordering constraint exists precisely to prevent this. |
| Some binaries deploy with the new code but one or more still run the old image | Mixed parser behavior: wildcard accepted in some hops, rejected in others. Watcher might spawn tasks the agent will refuse. | Spec ships all five migrations atomically in a single PR — AC enforces this. If it happens anyway, redeploy the lagging service. |
| Env value reaches a `required, non-empty` consumer empty after migration | Validator returns a wrapped error at startup; the binary refuses to start. Pod crashloops with a clear log line. | Operator fixes the env and redeploys. This is the intended fail-fast behavior. |
| Env value contains a malformed entry (e.g. `github.com/*`, `github.com/*/foo`) | Validator returns an aggregate error naming every malformed entry. Binary refuses to start. Predicate path (in optional consumers) logs and skips, no silent allow-all fallback. | Operator fixes every named entry and redeploys. |
| A PR is opened in a bborbe repo not previously in the list, post-deploy | Watcher spawns a task; controller materializes it in the vault; agent picks it up and runs to completion. | None — this is the success path. |
| Operator wants to allow a single repo OUTSIDE the bborbe org after the migration | Add the literal entry alongside the wildcard, separated by comma; redeploy. Runbook documents this. | None — supported. |

## Security / Abuse Cases

- The wildcard widens the set of repos any of the three deployed services will act on. A bborbe-owned repo previously excluded from the literal list will now be picked up automatically. This is the intended behavior; the threat model is unchanged because the `bborbe/*` set is operator-owned. No new external trust boundary is crossed.
- A typo in the env (e.g. `github.com/borbe/*`) would silently match nothing rather than match-all — the library rejects malformed entries explicitly. Acceptable.
- The build watcher's `required, non-empty` semantics is preserved: an accidental env wipe still crashloops the pod, surfacing immediately in dev rollout status.
- No new HTTP endpoints, no new file paths, no new user-controllable input crosses a trust boundary in this spec.

## Acceptance Criteria

- [ ] `grep -rn 'strings.Split.*REPO_ALLOWLIST\|filepath.Match.*allow' agent/ watcher/` returns zero matches at repo root after the change.
- [ ] Every `main.go` under `agent/pr-reviewer/`, `agent/pr-reviewer/cmd/run-task/`, `watcher/github-pr/`, `watcher/github-build/`, and `watcher/github-build/cmd/run-once/` imports `github.com/bborbe/maintainer/lib/repoallowlist`.
- [ ] The PR reviewer, its run-task CLI, and the PR watcher still treat an empty allowlist as allow-all (via the library's predicate).
- [ ] The build watcher main and its run-once CLI call the library's validator at startup and refuse to start on empty or malformed env, surfacing every malformed entry in a single error.
- [ ] `dev.env` line for `REPO_ALLOWLIST` is exactly `github.com/bborbe/*`.
- [ ] `prod.env` line for `REPO_ALLOWLIST` is exactly `github.com/bborbe/*`.
- [ ] `CHANGELOG.md` has a new entry under `## Unreleased` describing the migration and env-format change.
- [ ] Obsidian runbook "Agent - Add Repo to PR Reviewer Allowlist" is updated: states wildcard is the new default; primary use case is now non-bborbe additions; per-repo bborbe additions documented as still possible but rarely needed.
- [ ] `make precommit` passes in each of `agent/pr-reviewer/`, `watcher/github-pr/`, `watcher/github-build/`.
- [ ] Rung-2 dev smoke (step A — parser equivalence): with the new code deployed but `dev.env` still holding the prior literal value, all three services run cleanly AND a PR opened in `bborbe/go-skeleton` (the existing dev allowlist repo) still flows end-to-end through watcher → controller → agent (regression baseline — proves the library's literal-match path matches the prior inline parser's behavior, not just that pods boot).
- [ ] Rung-2 dev smoke (step B — wildcard rollout): after editing `dev.env` to `github.com/bborbe/*` and redeploying, a PR opened in **`bborbe/coding`** (NOT in the prior dev literal list — was `go-skeleton`-only) triggers a watcher task in the OpenClaw vault AND the agent picks it up and runs to completion.
- [ ] Rung-3 prod smoke: after dev soaks ≥1 day clean, prod follows the same code-first-then-env sequence and a PR opened in **`bborbe/go-skeleton`** (NOT in the prior prod literal list of `maintainer / jira-task-creator / agent / dark-factory / vault-cli / trading / coding`) completes the watcher → controller → agent → review pipeline.

Scenario coverage: NO new scenario test. The shared library's behavior is covered by spec 028's ginkgo `DescribeTable`; per-caller wiring is covered by each service's existing unit tests; the end-to-end pipeline is covered by Rung-2 live cluster verification, which an in-repo scenario could not faithfully reproduce (real Kafka, real GitHub, real controller, real PVC). Adding a scenario would duplicate either the unit tests or the manual cluster verification without catching anything new.

## Verification

Per `docs/verifying-specs.md`, this spec is **Rung-3** (touches code in three deployed services AND changes env files). Execute in order:

**Rung 1 — precommit (host):**

```
cd agent/pr-reviewer        && make precommit
cd watcher/github-pr        && make precommit
cd watcher/github-build     && make precommit
```

**Rung 2 — dev cluster, step A (code-only, regression baseline):**

```
# Deploy new image, old env still in place
cd ~/Documents/workspaces/maintainer-dev
git pull && git merge master --no-edit && git push
cd agent/pr-reviewer        && BRANCH=dev make build upload
cd watcher/github-pr        && BRANCH=dev make build upload
cd watcher/github-build     && BRANCH=dev make build upload
cd k8s && BRANCH=dev make buca

kubectlquant -n dev rollout status statefulset/agent-pr-reviewer            --timeout=120s
kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-pr --timeout=120s
kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-build --timeout=120s
```

Expect: all three pods Running, no crashloop. The literal env still works because the library accepts literals.

**Then verify parser equivalence** (not just startup): open a trivial PR in **`bborbe/go-skeleton`** (the existing dev literal entry). PR-watcher must publish a create-task command, controller must materialize the vault task, agent must claim and complete it. This proves the library's literal-match path is equivalent to the prior inline parser — without it, step A only proves the pod boots.

**Rung 2 — dev cluster, step B (env switch + wildcard smoke test):**

1. Edit `dev.env`: replace the `REPO_ALLOWLIST` line with `export REPO_ALLOWLIST=github.com/bborbe/*`.
2. Redeploy the three services (env-only change, same images).
3. Confirm pods still Running.
4. Open a trivial PR in **`bborbe/coding`** (NOT in the prior dev literal list — dev allowlisted only `bborbe/go-skeleton`).
5. Observe: PR-watcher publishes a `create-task` command (logs); controller consumes it; a per-SHA task file appears in `~/Documents/Obsidian/OpenClaw/tasks/`; the PR-reviewer agent claims it and runs to verdict.

**Rung 3 — prod cluster:**

After dev rung-2 step B has soaked ≥1 day clean, repeat the identical code-first-then-env sequence against `~/Documents/workspaces/maintainer-prod` with `BRANCH=prod` and `kubectlquant -n prod`. Smoke-test with a PR opened in **`bborbe/go-skeleton`** — NOT in the prior prod literal list (prod allowlisted: `maintainer / jira-task-creator / agent / dark-factory / vault-cli / trading / coding`).

## Do-Nothing Option

If we don't ship this, the parent goal's success criterion #7 stays unmet indefinitely: spec 028 produced a library nobody imports. Every new bborbe repo still requires a manual env-var edit in three places (PR reviewer, PR watcher, build watcher) across two env files, and a redeploy of three services — exactly the toil the goal was meant to eliminate. The drift risk also remains permanent: future security fixes to allowlist parsing have to be replicated five times. Not acceptable; the only path forward is to migrate all five callers and flip the env in a single coordinated change.
