# Verifying Specs in maintainer

Project-specific extension of the generic `dark-factory:verify-spec` workflow. When a spec moves to `verifying` (all prompts completed, code merged, autoRelease tagged), walk it through three rungs before `dark-factory spec complete`.

The principle from `~/.claude/plugins/marketplaces/dark-factory/docs/spec-verification.md` applies: **tests passing ≠ feature works**. Find live evidence at every rung.

## The three rungs

| Rung | Where | What it catches | When sufficient |
|---|---|---|---|
| 1. Local `run-once` | host, against dev Kafka via NodePort | Real Kafka schema, controller round-trip, vault path, frontmatter contract | Pure-detector specs (no in-cluster behavior change beyond image) |
| 2. Dev cluster e2e | dev k8s | Pod startup, env injection, PVC cursor, secret mount, k8s probe wiring, NetworkPolicy/DNS | Anything that depends on the StatefulSet template or in-cluster networking |
| 3. Prod cluster e2e | prod k8s | Real-traffic behavior at production scale | Specs that change throughput-sensitive paths or operator-visible behavior |

Rule of thumb: **always rung 1**. Rung 2 if anything in `k8s/` changed or if rung 1 fundamentally can't reach the behavior. Rung 3 once rung 2 looks clean — usually within minutes, not a day. **Soak is for high-risk changes only** (see below), not a default gate.

## Rung 1: local run-once

The `cmd/run-once` binary runs a single poll cycle against real dev Kafka, then exits. It exercises every code path the long-running watcher does, but with no PVC and no scheduled re-poll.

```bash
cd ~/Documents/workspaces/maintainer/watcher/github-build/cmd/run-once

# Default (bborbe/maintainer, watcher defaults):
make run-once

# Custom (exercise spec-016 overrides):
make run-once \
  REPO_ALLOWLIST=github.com/bborbe/maintainer \
  WATCHER_GITHUB_BUILD_TASK_ASSIGNEE=test-agent \
  WATCHER_GITHUB_BUILD_TASK_STATUS=backlog \
  WATCHER_GITHUB_BUILD_TASK_PHASE=planning
```

What to observe:

- stdout: `cdb_command-object-sender ... successful to develop-agent-task-v1-request with partition 0 offset NNNN` — kafka publish landed
- `kubectlquant -n dev logs agent-task-controller-0 --since=30s | grep create-task` — controller consumed
- `~/Documents/Obsidian/OpenClaw/tasks/<UUID5>.md` — vault file materialized with the expected frontmatter

Cursor warnings on `/data/cursor.json` are expected (no PVC on host) — non-fatal.

For specs that introduce a new file the watcher must read from a remote repo (e.g. `.maintenance.yaml`), put a real fixture in a public test-bed repo before running. The watcher fetches via the live GitHub Actions API; it does not respect any local filesystem layout.

## Rung 2: dev cluster e2e

Pre-conditions:

- Master is at the autoRelease tag for the spec (`git describe --tags --abbrev=0` matches the CHANGELOG entry's version)
- `maintainer-dev` worktree synced to master and pushed
- Dev secret mount valid (no GH_TOKEN rotation pending)

Deploy:

```bash
cd ~/Documents/workspaces/maintainer-dev
git pull && git merge master --no-edit && git push

cd watcher/github-build && BRANCH=dev make build upload
cd k8s && BRANCH=dev make buca

kubectlquant -n dev rollout status statefulset/maintainer-watcher-github-build --timeout=120s
```

Verify pod and override config landed:

```bash
kubectlquant -n dev get pod maintainer-watcher-github-build-0 \
  -o jsonpath='{.spec.containers[0].env}' | jq '.[] | select(.name | startswith("BUILD_"))'

kubectlquant -n dev logs maintainer-watcher-github-build-0 --tail=20 \
  | grep -E "BuildAssignee|BuildTaskStatus|BuildTaskPhase|argument_print"
```

Trigger a publish + observe the loop:

```bash
# Force cold start so a publish definitely fires
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- rm -f /data/cursor.json
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- wget -qO- http://localhost:9090/trigger
sleep 6

# Watcher published?
kubectlquant -n dev logs maintainer-watcher-github-build-0 --since=30s \
  | grep "send create-task command"

# Controller consumed without retry spam?
kubectlquant -n dev logs agent-task-controller-0 --since=30s \
  | grep -E "create-task|consume.*offset|attempt"

# Metrics increment?
kubectlquant -n dev exec maintainer-watcher-github-build-0 \
  -- wget -qO- http://localhost:9090/metrics \
  | grep -E "tasks_published_total|state_transitions_total\{|current_red_repos"
```

Verify the materialized vault task carries the spec's expected frontmatter (assignee, status, phase presence/absence — match the spec's AC literally).

## Rung 3: prod cluster e2e

Once rung 2 looks clean. Same shape as rung 2 but `maintainer-prod` worktree, `BRANCH=prod`, and `kubectlquant -n prod`. Reference: `[[git-rest - Deploy New Version]]` runbook for the dev→prod promotion pattern.

After prod cutover, observe one full poll cycle (the watcher's default 5m interval is enough) and confirm the expected log line / vault file / metric tick appears. That IS the rung 3 evidence — not a clock-based wait.

**When to add a soak gate**: only for changes where the failure mode is silent or slow-burning — security-critical changes, capital-at-risk paths, anything where a regression wouldn't surface in the first poll cycle. For routine code changes (refactors, new env vars, new endpoints, dependency bumps, even auth migrations) skip the soak. The Rung 3 evidence command (a grep for the expected log line, a check for the vault file, a metric scrape) is the gate. If the change works on first poll, the spec is verified; waiting 24h adds no signal.

## Closing the spec

After all relevant rungs pass:

```bash
dark-factory spec complete <id>
```

Update the matching task page in Obsidian to `completed` with the rung evidence in the Definition of Done section.

If verification fails on any rung, do NOT mark complete. Either:

- File a follow-up bug spec with the failing reproduction (preferred)
- Or write a fix prompt that closes the gap and re-run the rung that failed

## Rung selection by spec type

| Spec touches | Run rung 1 | Run rung 2 | Run rung 3 |
|---|---|---|---|
| Pure code path under `pkg/` (no k8s, no env, no remote fetch) | yes | optional | no |
| New CLI arg / env var | yes | yes (verify env injection) | yes, once rung 2 clean |
| New k8s manifest / StatefulSet template change | rung 1 doesn't catch this | yes | yes, once rung 2 clean |
| New remote API call (GitHub contents, etc.) | yes | yes (verify TokenScope + NetworkPolicy) | yes, once rung 2 clean |
| New Prometheus metric | rung 2 (real Prometheus scrapes the metric) | n/a (unless prod-only labels) |
| Pure refactor / doc change | optional | no | no |
| Security-critical / capital-at-risk / silent-failure-prone | yes | yes | yes + soak gate (see Rung 3 §"When to add a soak gate") |

If unsure: rung 1 always; rung 2 if any of `k8s/`, `dev.env`, `prod.env`, `Dockerfile`, or `Makefile` changed; rung 3 once rung 2 is clean and the rung-3 evidence command returns the expected result. No clock-based wait by default.

## Anti-patterns

- **"`make precommit` passed, marking complete."** Tests prove what the author thought. The dev cluster proves what production sees. The two diverge regularly.
- **Skipping rung 1 because rung 2 is "more thorough".** Rung 1 is faster, deterministic, and exercises Kafka schema + controller round-trip — the same boundaries rung 2 hits, with 100× shorter feedback loop.
- **Marking the spec complete the same minute the rollout finishes.** The pod is `Running` long before it has done a full poll cycle. Wait one cycle.
- **Using `gh auth token` (your personal token) for in-cluster verification.** The cluster pulls from teamvault. Local rung 1 uses your token; rung 2/3 use the secret. They are different tokens with different scopes.
- **Adding a "24h soak" AC to a routine spec.** Soak is for failure modes that don't surface in the first poll cycle (silent auth drift, slow-burn memory leaks, capital-at-risk). For everything else, the Rung 3 evidence command IS the gate. If the change works on first poll, it's verified — a clock-based wait adds no signal. Specs 038 and 039 both wrote "Rung 4 = 24h prod soak" ACs that turned out to be over-engineering; the live evidence on first poll cycle was the actual proof.

## See also

- Generic verification: `~/.claude/plugins/marketplaces/dark-factory/docs/spec-verification.md`
- Bug-spec verification (stricter): `~/.claude/plugins/marketplaces/dark-factory/docs/bug-workflow.md`
- Pipeline overview: `docs/architecture.md`
- Episode-SHA semantics for build-watcher specs: `docs/build-watcher.md`
- Deploy procedure (dev → prod): `[[git-rest - Deploy New Version]]` (Obsidian Personal vault, runbook)
