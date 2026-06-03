---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-30T10:42:57Z"
generating: "2026-05-30T11:04:58Z"
prompted: "2026-05-30T11:04:58Z"
verifying: "2026-05-30T21:12:08Z"
branch: dark-factory/parallel-agent-reviews-bake-plugin-drop-pvc
---
Tags: [[Dark Factory - Spec Writing Guide]]

---

## Summary

- Let `agent/pr-reviewer` (and the identically-built `agent/github-releaser`) run **multiple task Jobs concurrently** instead of one at a time.
- Single-pod serialization had **two** historical causes, only one of which remains. (a) **Auth**: the agent used to authenticate via a Claude Code OAuth *session* token (`claude login` → `.credentials.json` on the PVC), which is single-session and cannot be used concurrently across pods. The agent now uses a stateless Anthropic-compatible API token (`ANTHROPIC_AUTH_TOKEN` from the secret) instead of OAuth — already removing that blocker: an API token works across unlimited pods, and no credentials file is needed on the PVC anymore. (b) **Plugin delivery**: the `ReadWriteOnce` PVC mounted at `/home/claude/.claude` (= `CLAUDE_CONFIG_DIR`) is now the *sole* remaining reason the agent is pinned to one pod — it is the only source of the `bborbe/coding` plugin, and RWO means only one pod can mount it. The `pods: "1"` ResourceQuota merely mirrors that hard limit.
- This spec removes blocker (b). With stateless auth already in place, baking the plugin into the image is the last step to true concurrency.
- Bake the `bborbe/coding` plugin into the agent **image** at build time so every pod has `/coding:pr-review` registered with no mounted volume; then delete the PVC, drop `volumeClaim`/`volumeMountPath` from each Config CR (keeping `CLAUDE_CONFIG_DIR`), and raise the ResourceQuota to `pods: "3"` in dev and prod.
- End state: two PRs churning at the same time both get reviewed concurrently; neither starves the other; the github-releaser agent gains the same concurrency.
- No Go behavior changes — verdict schema, auth, review-post path, and task phase system are untouched.

## Problem

The PR-reviewer is the merge gate for every maintainer/bborbe PR. It runs as stateless per-task Kubernetes Jobs. Each Job pod mounts the `agent-pr-reviewer` PVC at `/home/claude/.claude`; that PVC is `accessModes: [ReadWriteOnce]`, so the cluster will schedule at most one mounting pod at a time. A single PR that force-pushes repeatedly produces a new review task each watcher poll, and that storm monopolizes the one available slot — unrelated PRs wait for hours.

Real incident (2026-05-30): PR #24 (github-releaser clone fixes, CI-green, locally review-clean) sat unreviewed for hours while PR #25's re-trigger storm consumed every reviewer slot.

The PVC is not a deliberate throttle. Originally `/home/claude/.claude` held two things a review pod needed: the Claude Code OAuth session credentials (`.credentials.json`) and the `bborbe/coding` plugin. The OAuth session token was the harder constraint — a session login cannot be shared across concurrent pods — which made single-pod the path of least resistance. The agent has since switched to a stateless Anthropic-compatible API token (`ANTHROPIC_AUTH_TOKEN` from the `maintainer-agent-pr-reviewer` secret) instead of OAuth, so the pod no longer needs a session credential on the PVC at all — auth is now concurrency-safe by construction.

That leaves the plugin as the PVC's only remaining payload. The `bborbe/coding` plugin provides the `/coding:pr-review` slash command and specialist sub-agents; without it the execution phase falls back to a degraded inline prompt. Today the plugin reaches the pod **only** via the PVC: the Dockerfile merely `mkdir -p`s the config dir (the github-releaser Dockerfile carries an aspirational "baked into image" comment but still only runs `mkdir -p`), and the spec-008 Go startup installer (`pkg/plugins/`) is no longer present in the tree. So the PVC cannot simply be deleted — its plugin payload must first be relocated into the image. `agent/github-releaser` was cloned from the same template and carries the identical PVC + `pods: "1"` pattern, so it has the same ceiling.

## Goal

Each agent (`pr-reviewer`, `github-releaser`) runs up to three task Jobs concurrently, each in its own pod with the `/coding` plugin available from the image alone, with no shared volume and no head-of-line blocking between unrelated PRs.

## Non-goals

- Autoscaling or dynamic concurrency — a fixed `pods: "3"` per agent is sufficient for now.
- Reworking the watcher's re-trigger-on-every-SHA behavior — the churn is legitimate; parallelism is the fix.
- Per-PR review deduplication.
- Pinning the plugin to a specific version — the image tracks marketplace latest at build time, frozen for the life of that image tag (acceptable; rebuild to refresh). Operational change: plugin updates now ship via image rebuild + redeploy, no longer via a `claude plugin update` at pod boot off the PVC cache.
- Changing the verdict schema, GitHub-App auth, clone path, or review-post path.
- Restoring or re-designing the spec-008 Go plugin installer — out of scope; the image bake replaces the need for a runtime installer. (If a runtime installer is later reintroduced it must not hard-fail when github.com is unreachable, since the plugin is now image-resident — but that is a separate spec.)
- Migrating any other agent beyond `pr-reviewer` and `github-releaser`.

## Desired Behavior

1. A freshly built `pr-reviewer` image, run with **no** volume mounted at `/home/claude/.claude`, has the `bborbe/coding` plugin installed: `claude plugin list` lists `coding`, and `/coding:pr-review` is a registered slash command.
2. The same holds for a freshly built `github-releaser` image.
3. With the PVC removed, two PR-review task Jobs triggered close together both run **concurrently** (two pods both reach `Running`), each produces its verdict independently, and neither blocks the other.
4. When one PR re-triggers repeatedly while another PR's review is queued, the second PR's review starts without waiting for the storm to subside (no head-of-line blocking) — up to the `pods: "3"` ceiling.
5. The execution phase uses the `/coding:pr-review` command (not the degraded inline-prompt fallback) on every pod, with no PVC present.
6. Removing the PVC leaves no dangling reference: neither Config CR names a `volumeClaim` or `volumeMountPath`, and no PVC manifest remains in either agent's `k8s/` directory.
7. `CLAUDE_CONFIG_DIR: /home/claude/.claude` remains set in each Config CR and resolves to the image-resident plugin directory.
8. A fourth concurrent task for the same agent queues (pending) rather than failing, because the ResourceQuota caps the agent at three pods.

## Constraints

- **Plugin facts** (durable `claude` CLI conventions, from `docs/claude-plugin-cli.md`): the marketplace slug is `bborbe/coding`; install form is `claude plugin install coding`; the enabled-plugin identifier is `coding@coding`; the marketplace alias is the last path segment of the slug (`coding`). The bake mechanism is the implementer's choice (e.g. `claude plugin marketplace add bborbe/coding && claude plugin install coding` at build, or cloning the marketplace repo into `$CLAUDE_CONFIG_DIR/plugins/marketplaces/coding/` plus writing `settings.json` with `enabledPlugins: {"coding@coding": true}`) provided the result satisfies Desired Behavior 1–2 with no network access at pod runtime.
- The bake runs in the final image build under `HOME=/home/claude`; it must write to `/home/claude/.claude` so the plugin is present at the path `CLAUDE_CONFIG_DIR` points to.
- The build reaches github.com at **build** time only; pod **runtime** must not require github.com to load the plugin. A build that cannot reach the marketplace must fail the build (not silently produce a pluginless image).
- Keep the existing multi-stage Dockerfile structure (golang build stage → alpine deps stage → final). Do not change the Go build, the `@anthropic-ai/claude-code` install, or the entrypoint.
- The Config CR field names (`volumeClaim`, `volumeMountPath`, env block) are controller-interpreted — only remove the two volume fields; do not rename or restructure other fields.
- **`volumeClaim`/`volumeMountPath` are already optional in the executor** (verified in `bborbe/agent`): the job spawner returns early with no volume when `VolumeClaim == ""` (`task/executor/pkg/spawner/job_spawner.go:188`), the CRD struct marks both `omitempty` with no required-validation (`.../v1/types.go:64`), validation only rejects the half-set combo (`types.go:156`), and the test `job_spawner_test.go:414` ("has no volumes when VolumeClaim is empty") asserts a clean no-PVC pod. So dropping both fields needs **no executor or controller change** — the resulting pod has empty `Volumes`/`VolumeMounts`.
- ResourceQuota `scopeSelector` (PriorityClass `maintainer-agent-pr-reviewer` / `maintainer-agent-github-releaser`) stays unchanged; only the `pods` hard limit changes `"1"` → `"3"` in both `resource-quota-dev.yaml` and `resource-quota-prod.yaml` for each agent.
- Each pod requests `cpu: 500m` / `memory: 1Gi`; three concurrent pods per agent = 1.5 CPU / 3Gi peak per agent per namespace — confirm this fits the namespace's overall quota before raising.
- Apply the identical change set to both `agent/pr-reviewer` and `agent/github-releaser` — Dockerfile bake, PVC deletion, Config CR edit, quota bump (dev + prod).
- **Rollout ordering**: deploy the plugin-baked (pluginless-PVC) image *before or together with* the quota bump — never raise `pods: "3"` ahead of the new image. Three old-image pods would contend for the RWO PVC and two would stick in `ContainerCreating`.
- `make precommit` must pass in any service dir whose Go code is touched (expected: none — this is Dockerfile + k8s yaml only).

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---------|-----------|-------------------|----------|
| Marketplace unreachable at image build | `docker build` exits non-zero at the bake step | Build fails loudly — no pluginless image is produced | Retry build when github.com reachable |
| Pod starts with no PVC and image-resident plugin present | Execution-phase log shows `/coding:pr-review` invoked | `/coding:pr-review` registered; execution uses it | n/a |
| Pod starts with no PVC and plugin absent from image | Pre-deploy: AC#1/#2 build probe fails. Post-deploy: execution-phase log shows the inline-prompt fallback marker (grep the log for the fallback prompt signature) | Execution phase falls back to inline prompt (degraded, not a crash) | Rebuild image with working bake; redeploy |
| **Quota bumped to `pods: 3` before the new pluginless-PVC image is deployed** | `kubectlquant -n <ns> get pods -l agent=pr-reviewer-agent` shows 2 pods stuck `ContainerCreating`, events report `Multi-Attach error` / `volume is already used by pod` on the RWO PVC | Old-image pods still mount the RWO PVC; only one can attach, the other two wedge | Roll quota back to `1`, or complete the image rollout first; confirm pods reach `Running`. (Prevented by the rollout-ordering constraint.) |
| 4th concurrent task for one agent while 3 pods already Running | `kubectlquant get pods` shows the 4th pod `Pending` | Job stays Pending until a slot frees (ResourceQuota) | Automatic once a pod finishes |
| Old PVC still bound after Config CR drops the mount | `kubectlquant get pvc` still lists the PVC after rollout | PVC becomes unused; pods schedule freely | Delete the PVC object from the cluster — confirm `kubectlquant -n <ns> get pvc <name>` returns NotFound |
| Two pods for the same repo run concurrently | Two pods `Running` under selector `-l agent=pr-reviewer-agent` for the same repo | Each has its own image-resident plugin + its own clone; no shared writable state | n/a (no shared volume by design) |

## Security / Abuse Cases

- The plugin is operator-controlled and baked from a known marketplace slug (`bborbe/coding`) at build time — no untrusted plugin source is introduced at runtime, and pods no longer need github.com access to obtain plugins (reduced runtime network surface).
- Removing the shared RWO PVC eliminates a shared writable surface between review pods; a malicious PR diff processed in one pod can no longer leave artifacts on a volume that a later pod mounts.
- No new credentials, secrets, or mounts are introduced. The GitHub-App auth path and its no-token-on-disk guarantees are untouched.
- Raising `pods: "3"` is bounded by the ResourceQuota; it cannot exhaust the namespace beyond the declared quota.

## Acceptance Criteria

- [ ] `agent/pr-reviewer/Dockerfile` bakes the `coding` plugin; running the built image with no volume mounted and the runtime env pinned, `claude plugin list` output contains `coding` — evidence: `docker run --rm -e HOME=/home/claude -e CLAUDE_CONFIG_DIR=/home/claude/.claude <image> claude plugin list` shows `coding`. (Env pinned so the probe matches pod runtime, not the build shell's defaults.)
- [ ] `/coding:pr-review` is **registered** (not merely listed) in the built pr-reviewer image with no PVC — evidence: a deterministic in-container check on the literal path — `docker run --rm <image> sh -c 'ls /home/claude/.claude/plugins/marketplaces/coding/commands/pr-review*'` exits 0 — AND a dev execution-phase log shows `/coding:pr-review` invoked rather than the inline-prompt fallback. (Guards the most likely failure: a build that bakes to the wrong path or with a `settings.json`/`CLAUDE_CONFIG_DIR` mismatch — `claude plugin list` can pass while the command stays unregistered.)
- [ ] Same two criteria hold for the `agent/github-releaser` image.
- [ ] `maintainer-agent-pr-reviewer-pvc.yaml` and `maintainer-agent-github-releaser-pvc.yaml` are deleted from the repo — evidence: files absent under each `k8s/`.
- [ ] Neither Config CR (`maintainer-agent-pr-reviewer.yaml`, `maintainer-agent-github-releaser.yaml`) contains `volumeClaim` or `volumeMountPath`; both still contain `CLAUDE_CONFIG_DIR: /home/claude/.claude` — evidence: `grep` returns 0 for the volume fields, 1 for `CLAUDE_CONFIG_DIR`.
- [ ] `resource-quota-{dev,prod}.yaml` for both agents set `pods: "3"` — evidence: `grep 'pods:' ` shows `"3"` in all four files.
- [ ] **Behavioral (dev)**: two PR-review tasks triggered within the same minute both reach pod phase `Running` simultaneously (two pods), both post verdicts; neither waits for the other — evidence: `kubectlquant -n dev get pods -l agent=pr-reviewer-agent` shows ≥2 `Running` at one moment, plus both PRs receive bot reviews. This reproduces the PR #24-vs-#25 scenario.
- [ ] Namespace quota headroom confirmed before bumping `pods: "3"` — evidence: `kubectlquant -n dev describe resourcequota` shows the agent's 3-pod / 1.5-CPU / 3Gi peak fits under the namespace ceiling (repeat for prod before prod rollout).
- [ ] Existing plugin scenarios re-confirmed against the pluginless-PVC image — re-run dark-factory scenarios `005-spec-011-plugin-pr-review` and `007-spec-011-plugin-pr-review-short-mode` (or their dev equivalent) after the bake to prove `/coding:pr-review` still resolves from the image, not the (now-removed) PVC — evidence: both scenarios pass against the deployed dev image.
- [ ] github-releaser concurrency is verified **by parity** (identical build + manifest pattern), not by a separate concurrent-release observation — this is a deliberate scope choice (releases are rare), recorded here so the gap is intentional, not an oversight.
- [ ] `make precommit` passes in any touched Go service dir (expected: no Go changes) — evidence: exit 0, or N/A noted.

## Verification

Image bake (per agent, before deploy):

```
# from the agent dir, build and inspect the plugin without any mount
docker build -t parallel-test-pr-reviewer agent/pr-reviewer
docker run --rm -e HOME=/home/claude -e CLAUDE_CONFIG_DIR=/home/claude/.claude parallel-test-pr-reviewer claude plugin list   # expect: coding listed
docker run --rm parallel-test-pr-reviewer sh -c "find /home/claude/.claude/plugins -name 'pr-review*' | grep -q ."  # expect: exit 0 (path discovered empirically — the install nests deeper than a literal commands/ guess)
```

Manifest hygiene:

```
cd ~/Documents/workspaces/maintainer-parallel-reviews
ls agent/pr-reviewer/k8s/*pvc* agent/github-releaser/k8s/*pvc*        # expect: no such file
grep -rn 'volumeClaim\|volumeMountPath' agent/pr-reviewer/k8s agent/github-releaser/k8s   # expect: 0 matches
grep -rn 'CLAUDE_CONFIG_DIR' agent/pr-reviewer/k8s agent/github-releaser/k8s              # expect: 1 each
grep -rn 'pods:' agent/*/k8s/resource-quota-*.yaml                    # expect: "3" in all four
```

Behavioral (dev, post-deploy — operator-driven, follow `docs/verifying-specs.md` rung-2; this is task/spec verification work, NOT a dark-factory prompt). Rollout order is load-bearing: deploy the plugin-baked image and apply the volume-stripped Config CRs FIRST, then bump the quota to `pods: "3"`, then delete the now-unused PVC objects — never raise the quota ahead of the new image (three old-image pods would contend for the RWO PVC and wedge in `ContainerCreating`).

```
# after the ordered rollout, delete the now-unused PVC objects (real metadata.name values,
# NOT the maintainer-agent-* filenames):
kubectlquant -n dev delete pvc agent-pr-reviewer agent-github-releaser
kubectlquant -n dev get pvc agent-pr-reviewer agent-github-releaser   # expect: NotFound

# trigger two PRs within the same minute, then poll for the concurrency window
# (the overlap is brief — a single get can miss it):
for i in $(seq 1 30); do
  n=$(kubectlquant -n dev get pods -l agent=pr-reviewer-agent --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
  echo "running=$n"; [ "$n" -ge 2 ] && { echo "CONCURRENCY OK"; break; }
  sleep 2
done
```

Confirm both PRs receive a `ben-s-pull-request-reviewer` review without one starving the other. Prod follows by parity after dev is green.

## Do-Nothing Option

The reviewer stays serialized behind one RWO PVC. Any single force-pushing PR continues to monopolize the only reviewer slot and starve every other PR's review for hours — the recurring PR #24-vs-#25 incident. Merge latency across the whole maintainer/bborbe fleet stays hostage to the noisiest PR, and github-releaser inherits the same ceiling. The cost of inaction grows with PR volume.
