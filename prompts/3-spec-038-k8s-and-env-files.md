---
spec: [038]
status: draft
created: "2026-05-23T21:30:00Z"
---

<summary>
- `watcher/github-build/k8s/maintainer-watcher-github-build-secret.yaml` gains `APP_ID`, `INSTALLATION_ID`, and `PEM_KEY` data fields.
- `dev.env` and `prod.env` declare three new exports with exact values from the spec.
- StatefulSet wires the three new env vars from the Secret into the container.
- No code changes — YAML and env file updates only.
</summary>

<objective>
Update Kubernetes Secret manifest, `.env` files, and StatefulSet to wire the GitHub App credentials into the watcher container. The credential values come from Teamvault and are stored in Kubernetes Secrets following the existing `GH_TOKEN` pattern.
</objective>

<context>
Read before making changes:

**Existing Secret:**
```yaml
# watcher/github-build/k8s/maintainer-watcher-github-build-secret.yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: maintainer-watcher-github-build
  namespace: '{{ "NAMESPACE" | env }}'
data:
  GH_TOKEN: '{{ "WATCHER_GITHUB_BUILD_GH_TOKEN_KEY" | env | teamvaultPassword | base64 }}'
```

**Reference Secret (pr-reviewer — note `teamvaultFileBase64` for PEM):**
```yaml
# agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml
data:
  SENTRY_DSN: '{{ "SENTRY_DSN_KEY" | env | teamvaultUrl | base64 }}'
  GH_TOKEN: '{{ "AGENT_PR_REVIEWER_GH_TOKEN_KEY" | env | teamvaultPassword | base64 }}'
  ANTHROPIC_AUTH_TOKEN: '{{ "MOPmQL" | teamvaultPassword | base64 }}'
  PEM_KEY: '{{ "AGENT_PR_REVIEWER_PEM_KEY" | env | teamvaultFileBase64 }}'
```

**App identity values (from spec):**
- Dev: App ID `3800041`, Installation ID `134435225`, PEM Teamvault key `eqKj8L`
- Prod: App ID `3798945`, Installation ID `134414316`, PEM Teamvault key `kLoejw`

**Existing StatefulSet container env block (watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml):**
```yaml
containers:
  - name: service
    env:
      - name: GH_TOKEN
        valueFrom:
          secretKeyRef:
            name: maintainer-watcher-github-build
            key: GH_TOKEN
      - name: KAFKA_BROKERS
        value: '{{ "KAFKA_BROKERS" | env }}'
```

**pr-reviewer deployment env (app IDs as direct env var values):**
```yaml
APP_ID: '{{ "AGENT_PR_REVIEWER_APP_ID" | env }}'
INSTALLATION_ID: '{{ "AGENT_PR_REVIEWER_INSTALLATION_ID" | env }}'
BOT_GITHUB_LOGIN: '{{ "AGENT_PR_REVIEWER_BOT_LOGIN" | env }}'
```
Note: pr-reviewer stores App IDs as direct env vars (not Secret-wrapped). For the watcher, store APP_ID and INSTALLATION_ID in the Secret alongside GH_TOKEN (consistent with existing watcher pattern).

**PEM handling:** The `PEM_KEY` data field uses `teamvaultFileBase64` because the PEM is a binary file (RSA private key). `teamvaultFileBase64` reads the file and base64-encodes it. `APP_ID` and `INSTALLATION_ID` are plain string integers — they use `base64` template (string-to-base64, not a Teamvault lookup).

**Important:** Kubernetes Secret `data` fields always store base64-encoded values. Template rendering happens at deploy time:
- `teamvaultPassword | base64` — Teamvault password → base64
- `teamvaultFileBase64` — Teamvault file → base64 (already encoded)
- `base64` — plain string env var → base64
</context>

<requirements>
1. **Update `watcher/github-build/k8s/maintainer-watcher-github-build-secret.yaml`**:

   Replace the existing content with:
   ```yaml
   apiVersion: v1
   kind: Secret
   type: Opaque
   metadata:
     name: maintainer-watcher-github-build
     namespace: '{{ "NAMESPACE" | env }}'
   data:
     GH_TOKEN: '{{ "WATCHER_GITHUB_BUILD_GH_TOKEN_KEY" | env | teamvaultPassword | base64 }}'
     APP_ID: '{{ "WATCHER_GITHUB_BUILD_APP_ID" | env | base64 }}'
     INSTALLATION_ID: '{{ "WATCHER_GITHUB_BUILD_INSTALLATION_ID" | env | base64 }}'
     PEM_KEY: '{{ "WATCHER_GITHUB_BUILD_PEM_KEY" | env | teamvaultFileBase64 }}'
   ```

   Notes:
   - `GH_TOKEN` kept as PAT fallback (unchanged)
   - `APP_ID` and `INSTALLATION_ID` use `| base64` — plain env var string → base64 (e.g., `"3800041"` → `MzgwMDA0MQ==`)
   - `PEM_KEY` uses `teamvaultFileBase64` — Teamvault file key → binary file content → base64 (PEM never enters git)

2. **Update `dev.env`**:

   Add these three lines after the existing `WATCHER_GITHUB_BUILD_*` entries (line ~16-19):
   ```bash
   export WATCHER_GITHUB_BUILD_APP_ID=3800041
   export WATCHER_GITHUB_BUILD_INSTALLATION_ID=134435225
   export WATCHER_GITHUB_BUILD_PEM_KEY=eqKj8L
   ```

3. **Update `prod.env`**:

   Add these three lines:
   ```bash
   export WATCHER_GITHUB_BUILD_APP_ID=3798945
   export WATCHER_GITHUB_BUILD_INSTALLATION_ID=134414316
   export WATCHER_GITHUB_BUILD_PEM_KEY=kLoejw
   ```

4. **Update `watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml`**:

   In the container's `env` list, after the existing `GH_TOKEN` block, add:
   ```yaml
   - name: APP_ID
     valueFrom:
       secretKeyRef:
         name: maintainer-watcher-github-build
         key: APP_ID
   - name: INSTALLATION_ID
     valueFrom:
       secretKeyRef:
         name: maintainer-watcher-github-build
         key: INSTALLATION_ID
   - name: PEM_KEY
     valueFrom:
       secretKeyRef:
         name: maintainer-watcher-github-build
         key: PEM_KEY
   ```

   The order in the STS matches the Secret key order (GH_TOKEN, APP_ID, INSTALLATION_ID, PEM_KEY). Place the new blocks after GH_TOKEN for consistency.

5. **Verify all changes**:
   ```bash
   grep -n 'PEM_KEY\|APP_ID\|INSTALLATION_ID' watcher/github-build/k8s/maintainer-watcher-github-build-secret.yaml
   grep -n 'WATCHER_GITHUB_BUILD_APP_ID\|WATCHER_GITHUB_BUILD_INSTALLATION_ID\|WATCHER_GITHUB_BUILD_PEM_KEY' dev.env prod.env
   grep -n 'APP_ID\|INSTALLATION_ID\|PEM_KEY' watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml
   ```

6. **Build verification** (YAML changes do not affect Go, but run for safety):
   ```bash
   cd watcher/github-build && go build ./...
   ```
</requirements>

<constraints>
- `PEM_KEY` MUST use `teamvaultFileBase64` — PEM bytes never enter git
- `APP_ID` and `INSTALLATION_ID` use plain `base64` — they are integer strings, not Teamvault secrets
- `.env` values match the spec exactly: dev `3800041`/`134435225`/`eqKj8L`, prod `3798945`/`134414316`/`kLoejw`
- `GH_TOKEN` stays in the Secret — PAT fallback remains available
- Do NOT commit — dark-factory handles git
- No code changes in this prompt
</constraints>

<verification>
```bash
# Secret has all four keys
grep -n 'GH_TOKEN\|APP_ID\|INSTALLATION_ID\|PEM_KEY' watcher/github-build/k8s/maintainer-watcher-github-build-secret.yaml
# Expected: 4 matches (one per key)

# dev.env has three new exports
grep -E 'WATCHER_GITHUB_BUILD_(APP_ID|INSTALLATION_ID|PEM_KEY)' dev.env
# Expected: 3 lines, values 3800041 / 134435225 / eqKj8L

# prod.env has three new exports
grep -E 'WATCHER_GITHUB_BUILD_(APP_ID|INSTALLATION_ID|PEM_KEY)' prod.env
# Expected: 3 lines, values 3798945 / 134414316 / kLoejw

# StatefulSet wires all three from Secret
grep -n 'APP_ID\|INSTALLATION_ID\|PEM_KEY' watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml
# Expected: 3 secretKeyRef entries (APP_ID, INSTALLATION_ID, PEM_KEY)

# Build passes
cd watcher/github-build && go build ./...
```
</verification>