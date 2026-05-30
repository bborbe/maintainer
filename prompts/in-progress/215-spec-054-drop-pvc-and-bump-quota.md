---
status: approved
spec: ["054"]
created: "2026-05-30T12:45:00Z"
queued: "2026-05-30T18:59:51Z"
---

<summary>
- Removes the shared one-pod-at-a-time storage claim from both agents so multiple review/release pods can run at once.
- Deletes the two PVC manifest files from the repo.
- Removes the two volume fields from each agent's Config resource while keeping the plugin config-dir setting intact.
- Raises each agent's pod limit from one to three in both the dev and prod quota files.
- Updates the two docs (`docs/architecture.md`, `README.md`) that still describe the removed PVC / OAuth-seed model.
- No Go code and no container image changes here — this prompt is k8s YAML edits + file deletions + the two named docs. Depends on the image bake (prompt 1); cluster rollout order is enforced by the spec's verification ladder, not a prompt.
</summary>

<objective>
Remove the `ReadWriteOnce` PVC that pins each agent to a single pod, raise the per-agent pod quota to three in dev and prod, and update the docs that still describe the PVC. After this prompt the manifests and docs describe pluginless-PVC, three-pod-capable agents; the actual cluster rollout (in the correct order) is operator-driven spec verification, not this prompt.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before editing (paths are repo-relative; the container starts at the repo root). The spec's inlined examples are paraphrased; the REAL files differ in field shape and names, so read them:
- `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml` (Config CR)
- `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-pvc.yaml`
- `agent/pr-reviewer/k8s/resource-quota-dev.yaml`
- `agent/pr-reviewer/k8s/resource-quota-prod.yaml`
- `agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml` (Config CR)
- `agent/github-releaser/k8s/maintainer-agent-github-releaser-pvc.yaml`
- `agent/github-releaser/k8s/resource-quota-dev.yaml`
- `agent/github-releaser/k8s/resource-quota-prod.yaml`
- `docs/architecture.md` (storage-tiers table) and `README.md` (Prerequisites / Deployment section)

VERIFIED current state of the pr-reviewer Config CR (relevant lines — note `env` is a YAML MAP, not a name/value list, and there is NO `task`/`maxConcurrent`/`ANTHROPIC_AUTH_TOKEN` field in the CR; the auth token arrives at runtime from the Secret via envFrom):
```yaml
spec:
  ...
  secretName: maintainer-agent-pr-reviewer
  volumeClaim: agent-pr-reviewer
  volumeMountPath: /home/claude/.claude
  env:
    # Tell Claude Code where the config + plugins live (matches volumeMountPath).
    # Without this, the agent runs without /coding plugin loaded → /coding:pr-review
    # slash command not registered → execution phase falls back to inline prompt.
    CLAUDE_CONFIG_DIR: /home/claude/.claude
    REPO_ALLOWLIST: '{{ "REPO_ALLOWLIST" | env }}'
    ...
```
The github-releaser CR is structurally identical; its `volumeClaim` value is `agent-github-releaser` and its comment text differs (see requirement 4).

Verified: `volumeClaim`/`volumeMountPath` are already optional in the executor (bborbe/agent) — the job spawner returns early with no volume when `VolumeClaim == ""`, the CRD marks both `omitempty` with no required-validation, and validation only rejects the half-set combo. So dropping BOTH fields needs no executor/controller change; the resulting pod has empty `Volumes`/`VolumeMounts`. Dropping only one would trip the half-set validation, so both must go together.

Verified current ResourceQuota (all four files share this shape; only `name`, `namespace`, and the scopeSelector value differ per agent):
```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: maintainer-agent-pr-reviewer
  namespace: dev
spec:
  hard:
    pods: "1"
  scopeSelector:
    matchExpressions:
      - scopeName: PriorityClass
        operator: In
        values: ["maintainer-agent-pr-reviewer"]
```
</context>

<requirements>
1. Delete the two PVC manifest files from the repo:
   - `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-pvc.yaml`
   - `agent/github-releaser/k8s/maintainer-agent-github-releaser-pvc.yaml`

2. Edit `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml`: under `spec`, delete EXACTLY these two lines and nothing else:
   ```yaml
     volumeClaim: agent-pr-reviewer
     volumeMountPath: /home/claude/.claude
   ```
   Keep everything else byte-for-byte: the `image:` line, `secretName:`, the entire `env:` MAP (especially `CLAUDE_CONFIG_DIR: /home/claude/.claude`), the `trigger:` block, and `resources:`. Do NOT rename or restructure any other field — these field names are controller-interpreted. Do NOT convert `env` from a map to a list.

3. Edit `agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml`: under `spec`, delete EXACTLY these two lines:
   ```yaml
     volumeClaim: agent-github-releaser
     volumeMountPath: /home/claude/.claude
   ```
   Keep everything else (image, secretName, env map including `CLAUDE_CONFIG_DIR: /home/claude/.claude`, trigger, resources) unchanged.

4. Update the now-stale comment above `CLAUDE_CONFIG_DIR` in BOTH Config CRs so it no longer claims the plugin comes from a mounted volume — the plugin is now baked into the image (prompt 1).
   - **pr-reviewer** — replace these exact three lines:
     ```yaml
         # Tell Claude Code where the config + plugins live (matches volumeMountPath).
         # Without this, the agent runs without /coding plugin loaded → /coding:pr-review
         # slash command not registered → execution phase falls back to inline prompt.
     ```
     with:
     ```yaml
         # Tell Claude Code where the image-resident config + plugins live (the /coding
         # plugin is baked into the image at this path; no volume is mounted). Without
         # this, /coding:pr-review is not registered → execution falls back to inline prompt.
     ```
   - **github-releaser** — replace these exact four lines:
     ```yaml
         # Tell Claude Code where config + plugins live (matches volumeMountPath). The
         # planning phase invokes Claude with an embedded bump-classification prompt
         # (pkg/prompts), so it does not depend on a /coding slash command, but the
         # config dir is still where the claude CLI resolves its settings.
     ```
     with:
     ```yaml
         # Tell Claude Code where the image-resident config + plugins live (the /coding
         # plugin is baked into the image at this path; no volume is mounted). The
         # planning phase invokes Claude with an embedded bump-classification prompt
         # (pkg/prompts), so it does not depend on a /coding slash command, but the
         # config dir is still where the claude CLI resolves its settings.
     ```

5. Bump all FOUR ResourceQuota files: change `pods: "1"` to `pods: "3"` (string-quoted "3", matching existing quoting). Change ONLY the `pods` value — leave `scopeSelector`, `name`, `namespace`, `apiVersion` untouched:
   - `agent/pr-reviewer/k8s/resource-quota-dev.yaml`
   - `agent/pr-reviewer/k8s/resource-quota-prod.yaml`
   - `agent/github-releaser/k8s/resource-quota-dev.yaml`
   - `agent/github-releaser/k8s/resource-quota-prod.yaml`

6. Update the two docs that still describe the removed PVC / OAuth-seed model:
   - `docs/architecture.md` — the "Storage tiers" table has a `.claude/ config` row whose "Backing today" cell reads `existing PVC agent-pr-reviewer (preserved across code-reviewer → maintainer rename to avoid OAuth re-seed)`. Rewrite that cell to say the `/coding` plugin is now **baked into the image** at `/home/claude/.claude` (no PVC); persistence is "image-resident (rebuilt per release)". Do not alter the unrelated `/work` and `/repos` rows.
   - `README.md` — in the Prerequisites/Deployment section, the bullet `PVC agent-pr-reviewer seeded with a valid .claude/ config (copy from agent-claude PVC or run one-time claude login ...)` is obsolete. Replace it with a bullet stating the `/coding` plugin is baked into the image at build time (no PVC seed, no `claude login`), and update any nearby line that says the agent is capped at 1 concurrent pod / matches the ReadWriteOnce PVC to say `pods: "3"` concurrent with no PVC constraint. Also drop the `PVC` entry from the `k8s/` directory/artifact legend (the line listing `Config CRD, Secret, PVC, PriorityClass, ResourceQuota, Makefile`). Do NOT touch the unrelated `AGENT_PR_REVIEWER_GH_TOKEN_KEY` line — it is pre-existing PAT-vs-App-auth drift, out of scope for this change.

7. Do NOT add any new manifest (no new volume, no emptyDir, no replacement PVC). Do NOT touch `priorityclass.yaml`, the `-secret.yaml` files, or the `Makefile` files. Do NOT add a refresh-interval knob, concurrency-tuning field, or any field the spec did not request.
</requirements>

<constraints>
- The Config CR field names (`volumeClaim`, `volumeMountPath`, env block) are controller-interpreted — only remove the two volume fields; do NOT rename or restructure other fields. `env` stays a YAML map.
- `CLAUDE_CONFIG_DIR: /home/claude/.claude` MUST remain set in each Config CR (it resolves to the image-resident plugin dir baked in prompt 1).
- Both volume fields must be removed together (removing only one trips the executor's half-set validation).
- ResourceQuota `scopeSelector` (PriorityClass) stays unchanged; only the `pods` hard limit changes "1" → "3".
- Each pod requests cpu 500m / memory 1Gi; three concurrent pods per agent = 1.5 CPU / 3Gi peak per agent per namespace. Namespace-quota headroom is confirmed during operator-driven spec verification BEFORE applying — this prompt only edits the repo files.
- Scope is k8s YAML plus the two named docs (`docs/architecture.md`, `README.md`) — do NOT change Go code or Dockerfiles or any other file.
- Do NOT commit — dark-factory handles git.
- No Go changes expected.
</constraints>

<verification>
Run from the repo root (the container starts there):

```
# PVC manifests gone:
ls agent/pr-reviewer/k8s/*pvc* agent/github-releaser/k8s/*pvc*        # expect: no such file (both error)

# no dangling volume references anywhere in k8s:
grep -rn 'volumeClaim\|volumeMountPath' agent/pr-reviewer/k8s agent/github-releaser/k8s   # expect: 0 matches

# CLAUDE_CONFIG_DIR still present once per agent:
grep -rn 'CLAUDE_CONFIG_DIR' agent/pr-reviewer/k8s agent/github-releaser/k8s              # expect: 1 line each

# stale "matches volumeMountPath" comment gone from both CRs:
grep -rn 'matches volumeMountPath' agent/*/k8s/*.yaml                 # expect: 0 matches

# quota bumped in all four files:
grep -rn 'pods:' agent/*/k8s/resource-quota-*.yaml                    # expect: "3" in all four lines

# stale PVC references removed from docs, new wording present:
grep -niE 'seeded with|ReadWriteOnce PVC|OAuth re-seed|claude login' README.md docs/architecture.md   # expect: 0
grep -n 'PVC' README.md                                                                                # expect: 0 (legend entry dropped)
grep -niq 'baked into the image' README.md docs/architecture.md && echo "DOCS UPDATED"                # expect: DOCS UPDATED
```

Validate the edited YAML still parses (template placeholders like `'{{ "NAMESPACE" | env }}'` are quoted strings, so safe_load handles them):
```
for f in agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml agent/*/k8s/resource-quota-*.yaml; do python3 -c "import sys,yaml; list(yaml.safe_load_all(open('$f')))" && echo "OK $f"; done
```

No Go changes expected; run `make precommit` only in a touched Go service dir (expected: N/A).
</verification>
