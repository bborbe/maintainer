---
status: completed
summary: 'Created agent/pr-reviewer/k8s/ with seven Kubernetes manifests (Config CRD, PVC, Secret, PriorityClass, ResourceQuota dev+prod, Makefile) and added feat entry to CHANGELOG.md under ## Unreleased.'
container: code-reviewer-031-add-k8s-manifests-pr-reviewer
dark-factory-version: v0.132.0
created: "2026-04-23T00:00:00Z"
queued: "2026-04-23T17:43:52Z"
started: "2026-04-23T17:44:26Z"
completed: "2026-04-23T17:45:13Z"
---

<summary>
- Add Kubernetes manifests for the pr-reviewer Pattern B Job so it can be deployed alongside the existing `agent-claude` service.
- Mirror the seven standard files (Config CRD, PVC, Secret, PriorityClass, two ResourceQuotas, Makefile) with the pr-reviewer identity.
- PVC plus PriorityClass plus per-namespace ResourceQuota enforce one-pod-at-a-time concurrency per namespace.
- Secret adds `PR_REVIEWER_GITHUB_TOKEN` alongside existing `SENTRY_DSN` via the same teamvault templating pipeline.
- `ALLOWED_TOOLS` defaults to the PR-review tool set (`Read,Grep,Glob,Bash(git:*),Bash(gh:*),WebFetch`).
- Changelog updated so the addition is captured in the next release.
- Human handles deploy verification after the prompt completes — prompt does not call `kubectlquant` or `make buca`.
</summary>

<objective>
Create `agent/pr-reviewer/k8s/` with seven Kubernetes manifests so the service becomes deployable as a Pattern B Job on `dev` and `prod`. Stop after files are written and `CHANGELOG.md` is updated. Do not commit, do not run `make buca`, do not call `kubectlquant`.
</objective>

<context>
Read before making changes:

- `CLAUDE.md` — project conventions, Pattern B Job architecture, verdict-schema-as-contract rule, mirror-`bborbe/agent` rule.
- `~/.claude/plugins/marketplaces/coding/docs/k8s-manifest-guide.md` — manifest layout (one resource per file, `k8s/` next to code, no `---` separators).
- `~/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` conventions and conventional-commit prefixes.

The sibling repo `bborbe/agent` is NOT mounted in the container — all source templates are inlined verbatim below.

**Source templates (from `bborbe/agent/agent/claude/k8s/`, inlined verbatim):**

`agent-claude.yaml`:
```yaml
apiVersion: agent.benjamin-borbe.de/v1
kind: Config
metadata:
  name: agent-claude
  namespace: '{{ "NAMESPACE" | env }}'
spec:
  assignee: claude-agent
  priorityClassName: agent-claude
  image: docker.quant.benjamin-borbe.de:443/agent-claude
  heartbeat: 5m
  secretName: agent-claude
  volumeClaim: agent-claude
  volumeMountPath: /home/claude/.claude
  env:
    ALLOWED_TOOLS: WebSearch,WebFetch,Read,Grep
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
      ephemeral-storage: 2Gi
    limits:
      cpu: 500m
      memory: 1Gi
      ephemeral-storage: 2Gi
```

`agent-claude-pvc.yaml`:
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: agent-claude
  namespace: '{{ "NAMESPACE" | env }}'
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

`agent-claude-secret.yaml`:
```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: agent-claude
  namespace: '{{ "NAMESPACE" | env }}'
data:
  SENTRY_DSN: '{{ "SENTRY_DSN_KEY" | env | teamvaultUrl | base64 }}'
```

`priorityclass.yaml`:
```yaml
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: agent-claude
value: 500
globalDefault: false
preemptionPolicy: Never
description: "Agent claude — namespace-local concurrency via matching ResourceQuota. Never preempts."
```

`resource-quota-dev.yaml`:
```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: agent-claude
  namespace: dev
spec:
  hard:
    pods: "1"
  scopeSelector:
    matchExpressions:
      - scopeName: PriorityClass
        operator: In
        values: ["agent-claude"]
```

`resource-quota-prod.yaml`: identical to dev except `namespace: prod`.

`Makefile`:
```
include ../../../Makefile.variables
include ../../../Makefile.env
include ../../../Makefile.k8s
```

**Token env-var naming:** The Go binary reads the GitHub token from env var `PR_REVIEWER_GITHUB_TOKEN` (see `agent/pr-reviewer/pkg/config/config.go`, constant `DefaultGitHubToken = "PR_REVIEWER_GITHUB_TOKEN"`). The k8s Secret key name MUST match that exact string — not `GITHUB_TOKEN`.

**Registry:** Use `docker.quant.benjamin-borbe.de:443` (matches `common.env` `DOCKER_REGISTRY` and the sibling claude image). The tag is the branch — written literally as `{{branch}}` in the Config CRD so `teamvault-config-parser` substitutes it at `make buca` time.

**Target directory does not yet exist** — you will create `agent/pr-reviewer/k8s/`.
</context>

<requirements>
1. Create directory `agent/pr-reviewer/k8s/`.

2. Create `agent/pr-reviewer/k8s/agent-pr-reviewer.yaml`:
   ```yaml
   apiVersion: agent.benjamin-borbe.de/v1
   kind: Config
   metadata:
     name: agent-pr-reviewer
     namespace: '{{ "NAMESPACE" | env }}'
   spec:
     assignee: pr-reviewer-agent
     priorityClassName: agent-pr-reviewer
     image: docker.quant.benjamin-borbe.de:443/agent-pr-reviewer:{{branch}}
     heartbeat: 5m
     secretName: agent-pr-reviewer
     volumeClaim: agent-pr-reviewer
     volumeMountPath: /home/claude/.claude
     env:
       ALLOWED_TOOLS: Read,Grep,Glob,Bash(git:*),Bash(gh:*),WebFetch
     resources:
       requests:
         cpu: 500m
         memory: 1Gi
         ephemeral-storage: 2Gi
       limits:
         cpu: 500m
         memory: 1Gi
         ephemeral-storage: 2Gi
   ```

3. Create `agent/pr-reviewer/k8s/agent-pr-reviewer-pvc.yaml`:
   ```yaml
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: agent-pr-reviewer
     namespace: '{{ "NAMESPACE" | env }}'
   spec:
     accessModes:
       - ReadWriteOnce
     resources:
       requests:
         storage: 1Gi
   ```

4. Create `agent/pr-reviewer/k8s/agent-pr-reviewer-secret.yaml`:
   ```yaml
   apiVersion: v1
   kind: Secret
   type: Opaque
   metadata:
     name: agent-pr-reviewer
     namespace: '{{ "NAMESPACE" | env }}'
   data:
     SENTRY_DSN: '{{ "SENTRY_DSN_KEY" | env | teamvaultUrl | base64 }}'
     PR_REVIEWER_GITHUB_TOKEN: '{{ "AGENT_PR_REVIEWER_GH_TOKEN_KEY" | env | teamvaultUrl | base64 }}'
   ```

5. Create `agent/pr-reviewer/k8s/priorityclass.yaml`:
   ```yaml
   apiVersion: scheduling.k8s.io/v1
   kind: PriorityClass
   metadata:
     name: agent-pr-reviewer
   value: 500
   globalDefault: false
   preemptionPolicy: Never
   description: "Agent pr-reviewer — namespace-local concurrency via matching ResourceQuota. Never preempts."
   ```

6. Create `agent/pr-reviewer/k8s/resource-quota-dev.yaml`:
   ```yaml
   apiVersion: v1
   kind: ResourceQuota
   metadata:
     name: agent-pr-reviewer
     namespace: dev
   spec:
     hard:
       pods: "1"
     scopeSelector:
       matchExpressions:
         - scopeName: PriorityClass
           operator: In
           values: ["agent-pr-reviewer"]
   ```

7. Create `agent/pr-reviewer/k8s/resource-quota-prod.yaml` — identical to step 6 except `metadata.namespace: prod`.

8. Create `agent/pr-reviewer/k8s/Makefile`:
   ```
   include ../../../Makefile.variables
   include ../../../Makefile.env
   include ../../../Makefile.k8s
   ```
   Exactly three lines. No targets, no trailing content.

9. Update `CHANGELOG.md` at repo root:
   - Ensure a `## Unreleased` section exists at the top (create directly above the most recent `## vX.Y.Z` heading if missing).
   - Add bullet under `## Unreleased`: `- feat: add k8s manifests for pr-reviewer (Config CRD, PVC, Secret, PriorityClass, ResourceQuota dev+prod, Makefile)`
   - Do not alter existing version sections.

10. Sanity self-check (do NOT `kubectlquant`, do NOT `make buca`):
    - `ls agent/pr-reviewer/k8s/` must list exactly: `Makefile`, `agent-pr-reviewer-pvc.yaml`, `agent-pr-reviewer-secret.yaml`, `agent-pr-reviewer.yaml`, `priorityclass.yaml`, `resource-quota-dev.yaml`, `resource-quota-prod.yaml`.
    - `grep -r "agent-claude" agent/pr-reviewer/k8s/` must return no matches (empty output).
    - Each YAML file contains exactly one `apiVersion:` line (no `---` separators, no multi-document YAML).

11. Do not commit, do not stage, do not push. Leave all new files untracked and `CHANGELOG.md` modified for human review.
</requirements>

<constraints>
- Pattern B Job only (stateless, one-shot). Do NOT invent Deployments, StatefulSets, Services, Ingress, or CronJobs.
- One resource per YAML file. No `---` separators, no multi-document YAML.
- Templating syntax byte-for-byte identical to the inlined source: `'{{ "NAMESPACE" | env }}'` (single-quoted), secret pipeline `'{{ "KEY_NAME" | env | teamvaultUrl | base64 }}'`.
- Image string must be exactly `docker.quant.benjamin-borbe.de:443/agent-pr-reviewer:{{branch}}`. The `{{branch}}` placeholder is literal — `teamvault-config-parser` substitutes it from the `BRANCH` env var at `make buca` time.
- PVC mount path stays `/home/claude/.claude` (the container user is still `claude` — do not rename to `pr-reviewer`).
- ResourceQuota `pods: "1"` must be a quoted string (unquoted parses as int and can fail server-side validation).
- Secret key for GitHub token MUST be `PR_REVIEWER_GITHUB_TOKEN` — this matches the Go constant `DefaultGitHubToken` in `agent/pr-reviewer/pkg/config/config.go`. Renaming it breaks auth.
- Do NOT run `kubectlquant`, `kubectl`, `make buca`, `make apply`, or any deploy-adjacent command. Human handles deploy verification.
- Do NOT commit. Do NOT stage. Do NOT push.
- Do NOT edit `Makefile.variables`, `Makefile.env`, `Makefile.k8s`, or any other root Makefile. Do NOT create a per-module `CHANGELOG.md`.
</constraints>

<verification>
From repo root:

1. File presence — must list exactly seven entries:
   ```
   ls agent/pr-reviewer/k8s/
   ```
   Expected: `Makefile  agent-pr-reviewer-pvc.yaml  agent-pr-reviewer-secret.yaml  agent-pr-reviewer.yaml  priorityclass.yaml  resource-quota-dev.yaml  resource-quota-prod.yaml`

2. No stale names:
   ```
   grep -r "agent-claude" agent/pr-reviewer/k8s/
   ```
   Must return no matches (empty output, exit code 1).

3. Token env-var name matches Go code:
   ```
   grep -n "PR_REVIEWER_GITHUB_TOKEN" agent/pr-reviewer/k8s/agent-pr-reviewer-secret.yaml
   ```
   Must match.

4. Changelog entry present under `## Unreleased`:
   ```
   grep -n "add k8s manifests for pr-reviewer" CHANGELOG.md
   ```
   Must return one line under `## Unreleased` (not under any `## vX.Y.Z`).

5. Working tree clean of commits:
   ```
   git status
   ```
   Must show seven new untracked files under `agent/pr-reviewer/k8s/` and `CHANGELOG.md` as modified. No new commits.
</verification>
