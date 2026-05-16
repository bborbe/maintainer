---
status: completed
summary: 'Added pod-level securityContext with fsGroup: 65534 to github-pr-watcher StatefulSet and recorded fix in CHANGELOG.md under a new ## Unreleased section.'
container: code-reviewer-070-watcher-fsgroup-fix
dark-factory-version: dev
created: "2026-04-30T00:00:00Z"
queued: "2026-05-01T09:34:13Z"
started: "2026-05-01T09:37:38Z"
completed: "2026-05-01T09:38:21Z"
---

<summary>
- The github-pr-watcher pod runs as a non-root user but cannot write its cursor state file to the persistent volume
- Root cause: the pod-level securityContext is missing `fsGroup`, so the PVC mount is owned by root and the unprivileged container is denied writes
- Adding `fsGroup: 65534` at the pod spec level makes the kubelet chown the PVC mount to that GID at startup
- The fix is a single YAML insertion in the StatefulSet manifest — no Go code, no PVC spec changes
- The existing container-level securityContext (runAsNonRoot, runAsUser: 65534, readOnlyRootFilesystem, dropped capabilities) stays untouched
- After the fix, poll cycles succeed and `cursor.json` persists across pod restarts
- A CHANGELOG entry records the bug fix under the existing `## Unreleased` section at repo root
</summary>

<objective>
Fix the `permission denied` error on `/data/cursor.json` in `github-pr-watcher` by adding a pod-level `securityContext` with `fsGroup: 65534` to the StatefulSet manifest, so the PVC mount is group-owned by the same GID the container runs under.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github/k8s/github-pr-watcher-sts.yaml` — the only file edited; the insertion point is between the `affinity:` block (ends line 37) and the `containers:` key (line 38)
- `agent/pr-reviewer/k8s/agent-pr-reviewer.yaml` — reference pod-spec layout in the same repo
- `CHANGELOG.md` (repo root) — single global changelog; add an entry under `## Unreleased` (create the section if it does not exist directly under the title)

Note: there is no `watcher/github/CHANGELOG.md` — the repo uses one global changelog at the root per `CLAUDE.md` ("Single global `CHANGELOG.md` at repo root. No per-module CHANGELOG.").
</context>

<requirements>
1. Edit `watcher/github/k8s/github-pr-watcher-sts.yaml`. Insert a pod-level `securityContext` block as a sibling of `affinity` and immediately above `containers:`. The result around lines 28-40 must be exactly:

   ```yaml
       spec:
         affinity:
           nodeAffinity:
             requiredDuringSchedulingIgnoredDuringExecution:
               nodeSelectorTerms:
                 - matchExpressions:
                     - key: node_type
                       operator: In
                       values:
                         - 'agent'
         securityContext:
           fsGroup: 65534
         containers:
           - name: service
   ```

   Indentation: `securityContext:` at 6 spaces (same column as `affinity:` and `containers:`), `fsGroup: 65534` at 8 spaces. Use spaces only — no tabs. Preserve all existing blank lines and trailing whitespace conventions of the file.

2. Do NOT modify the container-level `securityContext` block (lines 58-65 in the original file). It must remain exactly:
   ```yaml
             securityContext:
               runAsNonRoot: true
               runAsUser: 65534
               allowPrivilegeEscalation: false
               readOnlyRootFilesystem: true
               capabilities:
                 drop:
                   - ALL
   ```

3. Do NOT modify any other section of the StatefulSet — `metadata`, `spec.replicas`, `spec.selector`, `spec.serviceName`, `template.metadata`, `containers[].env`, `containers[].image`, `containers[].livenessProbe`, `containers[].readinessProbe`, `containers[].resources`, `containers[].volumeMounts`, `imagePullSecrets`, `volumes`, `updateStrategy`, and `volumeClaimTemplates` (including `storageClassName: local-path`) all stay byte-identical.

4. Verify the YAML still parses with `kubectl` (client-side, no cluster needed):
   ```
   kubectl apply --dry-run=client --validate=false -f watcher/github/k8s/github-pr-watcher-sts.yaml
   ```
   Note: the file uses Go template directives like `'{{ "NAMESPACE" | env }}'`, so a strict server-side validation will fail; client-side dry-run is sufficient because templated string values are valid YAML strings.

5. Add a CHANGELOG entry under `## Unreleased` in repo-root `CHANGELOG.md`. If `## Unreleased` does not exist, create it directly below the introductory paragraph and above the topmost `## vX.Y.Z` heading. Entry text:

   ```
   - fix: add pod-level `securityContext.fsGroup: 65534` to `watcher/github/k8s/github-pr-watcher-sts.yaml` so the `datadir` PVC mount is group-owned by the non-root UID, fixing `open /data/cursor.json: permission denied` on every poll cycle
   ```

6. Do NOT run `make precommit` for this change — the YAML edit does not touch Go sources, and `precommit` would unnecessarily run the full Go pipeline. Instead, verify the change with the dry-run command in step 4 and a `grep` confirming the insertion (see `<verification>`).

7. Do NOT run `make buca` and do NOT deploy. Deployment is performed manually by the operator after the prompt completes (see post-merge verification steps in the parent task — they run separately).
</requirements>

<constraints>
- ONLY edit `watcher/github/k8s/github-pr-watcher-sts.yaml` and `CHANGELOG.md` (repo root). No other file changes.
- Do NOT change the container-level `securityContext`.
- Do NOT change the PVC `volumeClaimTemplates` (no edits to `storageClassName`, `accessModes`, or `resources.requests.storage`).
- Do NOT change any Go code, Dockerfile, or Makefile.
- Do NOT add a `runAsGroup` field — `fsGroup` alone resolves the bug; introducing `runAsGroup` is out of scope.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass (no tests touch this file, but the repo must remain green).
- Indentation must use spaces only (the existing file uses 2-space indents at each level).
</constraints>

<verification>
# 1. Confirm the pod-level securityContext block is present at the correct indentation
grep -n -A1 "^      securityContext:" watcher/github/k8s/github-pr-watcher-sts.yaml
# Expected: one match at 6-space indent followed by `        fsGroup: 65534`

# 2. Confirm the container-level securityContext is unchanged (still has runAsUser: 65534)
grep -n "runAsUser: 65534" watcher/github/k8s/github-pr-watcher-sts.yaml
# Expected: exactly one match

# 3. Confirm fsGroup appears exactly once
grep -c "fsGroup:" watcher/github/k8s/github-pr-watcher-sts.yaml
# Expected: 1

# 4. Client-side YAML/structure validation
kubectl apply --dry-run=client --validate=false -f watcher/github/k8s/github-pr-watcher-sts.yaml
# Expected: `statefulset.apps/github-pr-watcher created (dry run)` — no parse errors

# 5. Confirm CHANGELOG entry exists
grep -n "fsGroup: 65534" CHANGELOG.md
# Expected: one match in the `## Unreleased` section
</verification>
