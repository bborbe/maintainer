---
status: draft
spec: [037-migrate-pr-watcher-to-github-app]
created: "2026-05-23T21:30:00Z"
branch: dark-factory/migrate-pr-watcher-to-github-app
---

## Summary

- Add `PEM_KEY` to the k8s Secret YAML using the `teamvaultFileBase64` template pattern
- Add `APP_ID`, `INSTALLATION_ID` (plain env), `PEM_KEY` (from Secret) to the StatefulSet YAML
- Add `WATCHER_GITHUB_PR_APP_ID`, `WATCHER_GITHUB_PR_INSTALLATION_ID`, `WATCHER_GITHUB_PR_PEM_KEY` to `dev.env` and `prod.env`
- `GH_TOKEN` Secret key and env wiring retained for legacy fallback during rollout

## Objective

Wire the new GitHub App credentials into the Kubernetes manifests and environment files. PEM bytes must use the existing `teamvaultFileBase64` template pattern and must not enter git. App ID and Installation ID are public values and are wired as plain env values from the `.env` files.

## Context

Read these files before making changes:
- `/workspace/watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml` — existing Secret pattern
- `/workspace/watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` — existing StatefulSet pattern
- `/workspace/dev.env` — dev environment variables
- `/workspace/prod.env` — prod environment variables
- `/workspace/agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml` — reference Secret with PEM (for PEM_KEY pattern only, not the full structure)

## Requirements

### 1. Update `k8s/maintainer-watcher-github-pr-secret.yaml`

Add the `PEM_KEY` key using the existing `teamvaultFileBase64` template pattern. Keep `GH_TOKEN` for legacy fallback.

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: maintainer-watcher-github-pr
  namespace: '{{ "NAMESPACE" | env }}'
data:
  GH_TOKEN: '{{ "WATCHER_GITHUB_PR_GH_TOKEN_KEY" | env | teamvaultPassword | base64 }}'
  PEM_KEY: '{{ "WATCHER_GITHUB_PR_PEM_KEY" | env | teamvaultFileBase64 }}'
```

Note: The `GH_TOKEN` key is retained for the legacy fallback path. After soak and PAT revocation (separate spec), this key will be removed.

### 2. Update `k8s/maintainer-watcher-github-pr-sts.yaml`

Add three new `env` entries to the container spec. `APP_ID` and `INSTALLATION_ID` are plain values from env vars; `PEM_KEY` comes from the Secret mount:

```yaml
env:
  # ... existing GH_TOKEN env entry (keep) ...
  - name: GH_TOKEN
    valueFrom:
      secretKeyRef:
        name: maintainer-watcher-github-pr
        key: GH_TOKEN
  - name: APP_ID
    value: '{{ "WATCHER_GITHUB_PR_APP_ID" | env }}'
  - name: INSTALLATION_ID
    value: '{{ "WATCHER_GITHUB_PR_INSTALLATION_ID" | env }}'
  - name: PEM_KEY
    valueFrom:
      secretKeyRef:
        name: maintainer-watcher-github-pr
        key: PEM_KEY
  # ... rest of existing env entries ...
```

### 3. Update `dev.env`

Add the dev App credentials:

```bash
export WATCHER_GITHUB_PR_APP_ID=3800041
export WATCHER_GITHUB_PR_INSTALLATION_ID=134435225
export WATCHER_GITHUB_PR_PEM_KEY=eqKj8L
```

### 4. Update `prod.env`

Add the prod App credentials:

```bash
export WATCHER_GITHUB_PR_APP_ID=3798945
export WATCHER_GITHUB_PR_INSTALLATION_ID=134414316
export WATCHER_GITHUB_PR_PEM_KEY=kLoejw
```

## Constraints

- PEM bytes MUST NOT enter git — use `teamvaultFileBase64` template, not inline base64
- App ID and Installation ID are public values — wire as plain env values, not Secret keys
- Do NOT remove `GH_TOKEN` from the Secret or StatefulSet — legacy fallback is kept during rollout
- `PEM_KEY` Secret key must use the existing `teamvaultFileBase64` template pattern

## Verification

```bash
grep -n 'PEM_KEY\|GH_TOKEN\|APP_ID\|INSTALLATION_ID' /workspace/watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml
grep -n 'PEM_KEY\|GH_TOKEN\|APP_ID\|INSTALLATION_ID' /workspace/watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml
grep -n 'WATCHER_GITHUB_PR_APP_ID\|WATCHER_GITHUB_PR_INSTALLATION_ID\|WATCHER_GITHUB_PR_PEM_KEY' /workspace/dev.env /workspace/prod.env
```

Expected: all new env vars and Secret keys are present with correct values. `make precommit` for the watcher passes (k8s files are not linted by `make precommit` but the `.env` files must be valid shell syntax).