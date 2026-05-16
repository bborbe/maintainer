---
status: completed
summary: Added container securityContext (runAsNonRoot, runAsUser 65534, allowPrivilegeEscalation false, readOnlyRootFilesystem true, drop ALL) and emptyDir /tmp volume to watcher/github StatefulSet.
container: code-reviewer-061-review-watcher-github-add-k8s-pod-security-context
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T15:31:17Z"
started: "2026-04-28T18:25:46Z"
completed: "2026-04-28T18:26:25Z"
---

<summary>
- The StatefulSet has no securityContext at pod or container level
- Without it the container runs as root (UID 0) by default
- Running as root is not required — the binary only writes to the /data PVC mount
- Kubernetes best practice requires runAsNonRoot, allowPrivilegeEscalation: false, and dropping all capabilities
- The /data PVC volume mount is unaffected by a read-only root filesystem
- This is a principle-of-least-privilege fix with no runtime behavior change
</summary>

<objective>
Add a `securityContext` block to the StatefulSet container spec in `watcher/github/k8s/github-pr-watcher-sts.yaml` that disables privilege escalation, drops all capabilities, runs as a non-root user, and sets a read-only root filesystem.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/k8s/github-pr-watcher-sts.yaml` (full): current container spec without securityContext
</context>

<requirements>
1. In `watcher/github/k8s/github-pr-watcher-sts.yaml`, add a `securityContext` block to the container spec (inside the `containers[0]` entry, at the same indentation level as `name`, `image`, `env`):

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

   Place this block directly after the `imagePullPolicy: Always` line. Anchor by content (find that exact line) — do not rely on a line number.

2. Add an `emptyDir` volume mounted at `/tmp` to support any runtime temp-file writes (TLS cert bundles, libgit2 scratch space, sentry queue) under `readOnlyRootFilesystem: true`:
   - Add to `volumes:`: `- name: tmp` with `emptyDir: {}`
   - Add to container `volumeMounts:`: `- name: tmp` mounted at `mountPath: /tmp`

3. Verify that no other volumeMounts write to the root filesystem:
   - The only application mount is `/data` (the PVC) — this is a volume, not the root filesystem, so `readOnlyRootFilesystem: true` is safe.
   - The binary `/main` is baked into the image at build time and only needs to be executed, not written.

3. Do NOT add a pod-level `securityContext` (the container-level is sufficient for these settings).

4. Verify the YAML is valid — no `---` separators, single `apiVersion` per file.

5. This prompt changes only YAML — no Go code changes, no `make precommit` needed. Run the sanity check instead:
   ```bash
   grep -c "^apiVersion:" watcher/github/k8s/github-pr-watcher-sts.yaml
   # Expected: 1

   grep "runAsNonRoot\|allowPrivilegeEscalation\|readOnlyRootFilesystem" watcher/github/k8s/github-pr-watcher-sts.yaml
   # Expected: three matches
   ```
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Do NOT modify any `.go` files
- Do NOT run `kubectl`, `make buca`, or any deploy command
- One resource per YAML file — preserve the existing single-document structure
- `runAsUser: 65534` is the `nobody` user — standard non-root UID for containers with no user management
</constraints>

<verification>
grep -n "runAsNonRoot\|allowPrivilegeEscalation\|readOnlyRootFilesystem\|runAsUser\|capabilities" watcher/github/k8s/github-pr-watcher-sts.yaml
# Expected: 5 matches

grep -c "^apiVersion:" watcher/github/k8s/github-pr-watcher-sts.yaml
# Expected: 1
</verification>
