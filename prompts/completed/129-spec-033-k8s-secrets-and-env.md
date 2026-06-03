---
status: completed
spec: [033-migrate-pr-reviewer-to-github-app]
container: maintainer-pr-reviewer-app-exec-129-spec-033-k8s-secrets-and-env
dark-factory-version: v0.164.0
created: "2026-05-21T20:30:30Z"
queued: "2026-05-21T20:58:04Z"
started: "2026-05-21T21:25:40Z"
completed: "2026-05-21T21:38:08Z"
---

<summary>
- The pr-reviewer Kubernetes Secret carries the GitHub App private key (PEM) as a mounted file, sourced via the existing teamvault-resolved-at-deploy template pattern.
- The pr-reviewer Config CR (the agent task controller's resource that defines pod env + secrets) exposes the new App-auth env vars (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`, `BOT_LOGIN`) so the binary sees them at startup.
- Two cluster targets are wired: dev (App ID 3800041, Installation 134435225, PEM Teamvault `eqKj8L`, bot login `ben-s-pull-request-reviewer-dev[bot]`) and prod (App ID 3798945, Installation 134414316, PEM Teamvault `kLoejw`, bot login `ben-s-pull-request-reviewer[bot]`).
- The legacy `GH_TOKEN` Secret key stays in place as a fallback during transition — removing it is a follow-up cleanup.
- No new Helm chart, no new CRD; only the existing `agent/pr-reviewer/k8s/` manifests are edited.
- The PEM never enters git; the manifest references a Teamvault file via the existing `teamvaultFile` template filter (same pattern as other secrets in this repo).
</summary>

<objective>
Update the pr-reviewer Kubernetes manifests so that, when deployed to dev or prod, the pod has a mounted PEM file at a stable path and the new App-auth env vars (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`, `BOT_LOGIN`) point the binary at the right App. The legacy `GH_TOKEN` Secret entry stays in place as a fallback during transition.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.

Read these files in the maintainer repo before making changes:
- `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml` — existing Secret YAML; uses Go templates with `teamvaultPassword` and env-var-driven Teamvault keys (e.g. `'{{ "AGENT_PR_REVIEWER_GH_TOKEN_KEY" | env | teamvaultPassword | base64 }}'`). The deploy pipeline resolves these templates at apply time via `teamvault-config-parser`.
- `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml` — the Config CR (`kind: Config`, group `agent.benjamin-borbe.de/v1`); defines `secretName`, the `env:` map injected into the pod, etc. The CRD itself is defined in the external `~/Documents/workspaces/agent/` repo (the agent task controller). To verify the schema for any file-mount fields (e.g. `secretFileMounts`), inspect `~/Documents/workspaces/agent/` if available locally; otherwise default to Option B in Step 3 (env-string PEM) which sidesteps the schema-uncertainty entirely.
- `agent/pr-reviewer/Makefile` (parent dir) — defines `make build`, `make upload`. `agent/pr-reviewer/k8s/Makefile` includes `Makefile.k8s` which defines `buca` (= `apply` only; build + upload live in the parent Makefile).
- `Makefile.k8s` (repo root) — the canonical deploy machinery; uses `teamvault-config-parser` (NOT `go-template-renderer`) to resolve templates before piping into `kubectlquant apply -f -`.
- `dev.env` and `prod.env` (repo root) — operator-sourced env files that already define `AGENT_PR_REVIEWER_GH_TOKEN_KEY=ROnG5L` etc. These are committed (they hold Teamvault entry keys, NOT secret values). New `AGENT_PR_REVIEWER_APP_ID`, `AGENT_PR_REVIEWER_INSTALLATION_ID`, `AGENT_PR_REVIEWER_BOT_LOGIN`, and `AGENT_PR_REVIEWER_PEM_KEY` entries belong here.
- `agent/pr-reviewer/docs/github-app-setup.md` — App identity reference; the env-var names this prompt uses MUST match the prompt-2 binary changes (already coordinated).
- Existing manifests only use `teamvaultPassword` and `teamvaultUrl` filters. **`teamvaultFile` is not used in this repo** — use `teamvaultPassword` for the PEM (it returns the raw secret value as a string, which for a Teamvault file-type entry is the full multi-line PEM content).

**Env-var contract (must match Prompt 2's binary changes):**

| Env var | Prod value | Dev value |
|---------|------------|-----------|
| `APP_ID` | `3798945` | `3800041` |
| `INSTALLATION_ID` | `134414316` | `134435225` |
| `PEM_KEY_FILE` | `/etc/github-app/pem` (or the chosen mount path) | same path |
| `BOT_LOGIN` | `ben-s-pull-request-reviewer[bot]` | `ben-s-pull-request-reviewer-dev[bot]` |
| `GH_TOKEN` | unchanged (legacy PAT fallback) | unchanged |

The PEM path itself can be anything stable; pick `/etc/github-app/pem` unless an existing convention in this repo prefers a different mount root.

**Teamvault entry keys:**

- Dev PEM: Teamvault key `eqKj8L` (entry name: `Github App - bborbe-pr-reviewer - private key - dev`)
- Prod PEM: Teamvault key `kLoejw` (entry name: `Github App - bborbe-pr-reviewer - private key`)

App ID, Installation ID, and Bot login are public values — they go directly in the Config CR's `env:` map (or in a Secret as ordinary string keys if mounting them via the Secret is the project convention). The PEM is the only secret-secret.
</context>

<requirements>
Execute steps in order.

---

## Step 1 — Inspect existing Secret + Config CR patterns

```bash
cat agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml
cat agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml
grep -rln 'teamvaultPassword\|teamvaultUrl\|teamvaultFile' agent/ watcher/ 2>/dev/null
```

Expected: every match uses `teamvaultPassword` or `teamvaultUrl` (or, more rarely, no matches). `teamvaultFile` does not appear in this repo. Use **`teamvaultPassword`** for the PEM — Teamvault file-type entries return their full content (including newlines) as the password value.

Confirm how the Config CR injects env vars by reading the existing manifest:
1. The Config CR's `spec.env:` map carries literal env vars (with `{{ ... | env }}` templating for deploy-time resolution).
2. The `spec.secretName:` reference makes Secret keys available to the pod via the controller's plumbing — read `~/Documents/workspaces/agent/` (the controller repo) if locally available to confirm whether keys mount as files or env vars by default; otherwise assume env-var injection by Secret key (the common default).

The choice between Option A (file mount) and Option B (env-string PEM) in Step 3 depends on this.

---

## Step 2 — Add the PEM key to the Secret

Edit `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml`. Add a new key `PEM` (or `GITHUB_APP_PEM` — pick a name that matches the existing convention; if other manifests prefix env-style keys for secret derivation, follow that):

```yaml
data:
  # ... existing keys ...
  PEM: '{{ "AGENT_PR_REVIEWER_PEM_KEY" | env | teamvaultPassword | base64 }}'
```

The env var `AGENT_PR_REVIEWER_PEM_KEY` is read at deploy time and resolved to the Teamvault entry key:

- For dev deploys: `AGENT_PR_REVIEWER_PEM_KEY=eqKj8L`
- For prod deploys: `AGENT_PR_REVIEWER_PEM_KEY=kLoejw`

(The actual export of `AGENT_PR_REVIEWER_PEM_KEY=<key>` happens in the deploy environment — same pattern as the existing `AGENT_PR_REVIEWER_GH_TOKEN_KEY` env on the line right above. Do not commit env values; only commit the template.)

---

## Step 3 — Mount the PEM as a file inside the pod

If the Config CR (`agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml`) supports a `secretVolumeMounts:` or equivalent block, add an entry that mounts the Secret's `PEM` key as a file at `/etc/github-app/pem`.

If the existing CR shape only supports env-var injection (no file mounts of Secret keys), use Option B below.

**Option A — file mount (preferred):**

Add to the Config CR `spec:` block:

```yaml
secretFileMounts:
  - secretKey: PEM
    mountPath: /etc/github-app/pem
```

(The exact field name is determined by the Config CR's schema — check the controller's CRD definition or other Configs in this repo to find the canonical field. If the CRD lacks a file-mount feature, fall back to Option B.)

**Option B — env-var with PEM string content (fallback):**

If the Config CR cannot mount Secret keys as files, use the PEM string directly via env. Change the binary's Step-2 contract: instead of `PEM_KEY_FILE`, point at `PEM_KEY` (PEM content env var). This requires editing `agent/pr-reviewer/main.go` from Prompt 2's plan to read `PEM_KEY` (string) in addition to `PEM_KEY_FILE` (path). The `lib/githubapp.Config` already supports both via its `PEM` and `PEMPath` fields.

If Option B is the chosen path, document the decision in `agent/pr-reviewer/docs/github-app-setup.md` and update Prompt 2's `BotLogin` env-table comment in this file to match. The end state: the binary accepts EITHER `PEM_KEY` (string) OR `PEM_KEY_FILE` (path); the k8s Config CR sets one of them.

---

## Step 4 — Add the App-auth env vars to the Config CR's `env:` map

Edit `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml`. In the `env:` map (currently includes `CLAUDE_CONFIG_DIR`, `REPO_ALLOWLIST`, `ANTHROPIC_BASE_URL`, etc.), add the four new vars using deploy-time env templates:

```yaml
env:
  # ... existing env entries ...
  APP_ID: '{{ "AGENT_PR_REVIEWER_APP_ID" | env }}'
  INSTALLATION_ID: '{{ "AGENT_PR_REVIEWER_INSTALLATION_ID" | env }}'
  BOT_LOGIN: '{{ "AGENT_PR_REVIEWER_BOT_LOGIN" | env }}'
  PEM_KEY_FILE: /etc/github-app/pem   # OR PEM_KEY via Secret, per Option A vs B in Step 3
```

The env templates (`AGENT_PR_REVIEWER_APP_ID`, `AGENT_PR_REVIEWER_INSTALLATION_ID`, `AGENT_PR_REVIEWER_BOT_LOGIN`, `AGENT_PR_REVIEWER_PEM_KEY`) are sourced from `dev.env` / `prod.env` at the repo root (committed files; they hold Teamvault entry keys and public App IDs, NOT secret values).

Append to `dev.env`:

```bash
export AGENT_PR_REVIEWER_APP_ID=3800041
export AGENT_PR_REVIEWER_INSTALLATION_ID=134435225
export AGENT_PR_REVIEWER_BOT_LOGIN="ben-s-pull-request-reviewer-dev[bot]"
export AGENT_PR_REVIEWER_PEM_KEY=eqKj8L
```

Append to `prod.env`:

```bash
export AGENT_PR_REVIEWER_APP_ID=3798945
export AGENT_PR_REVIEWER_INSTALLATION_ID=134414316
export AGENT_PR_REVIEWER_BOT_LOGIN="ben-s-pull-request-reviewer[bot]"
export AGENT_PR_REVIEWER_PEM_KEY=kLoejw
```

Matches the existing pattern (`export AGENT_PR_REVIEWER_GH_TOKEN_KEY=ROnG5L` already in those files).

---

## Step 5 — Document the deploy-time env vars

In `agent/pr-reviewer/docs/github-app-setup.md`, add a new section near the Migration Status block titled "Deploy-time environment variables" listing the four `AGENT_PR_REVIEWER_*` env vars and the prod / dev values. State that they are set in the operator's deploy shell (typically `~/.zshrc` or per-cluster `.envrc`) before running `BRANCH=dev make buca` or `BRANCH=prod make buca`.

This makes the operator's required exports discoverable from the canonical doc instead of buried in the manifest template comments.

---

## Step 6 — Validate the manifests parse

The actual deploy renderer is `teamvault-config-parser` (see `Makefile.k8s` at the repo root). It requires Teamvault auth which is unlikely available in the YOLO container — **skip the full render** and only check YAML syntactic validity:

```bash
for f in agent/pr-reviewer/k8s/*.yaml; do
  python3 -c "import yaml; yaml.safe_load(open('$f'))" || echo "INVALID: $f"
done
```

Templated values inside quoted strings (`'{{ "X" | env }}'`) parse as plain YAML strings, so both the Secret and the Config CR should parse cleanly. Any `INVALID:` line indicates a syntax error introduced by the edits in Steps 2 and 4.

---

## Step 7 — Update CHANGELOG

Append one line to the existing `## Unreleased` section:

```markdown
- feat(agent/pr-reviewer): wire k8s Secret + Config CR for GitHub App auth (PEM mount + APP_ID/INSTALLATION_ID/PEM_KEY_FILE/BOT_LOGIN env vars); dev uses `eqKj8L` + App 3800041, prod uses `kLoejw` + App 3798945; legacy `GH_TOKEN` Secret key retained as fallback (spec 033)
```

---

## Step 8 — Run module-local precommit

```bash
cd agent/pr-reviewer && make precommit
```

Most precommit targets don't touch YAML; this is a smoke test that nothing else regressed. Must exit 0.
</requirements>

<constraints>
- PEM MUST NOT enter git. Only Teamvault and Kubernetes Secrets. The Secret YAML committed to the repo uses the existing teamvault-resolved-at-deploy pattern (template filter `teamvaultFile` or `teamvaultPassword`); plaintext PEM is forbidden.
- The legacy `GH_TOKEN` Secret key stays in place. Removing it is out of scope for this prompt.
- Use the existing template-filter convention in this repo (`teamvaultFile` or `teamvaultPassword`) — do not invent new filters and do not introduce a different secret backend.
- The bot login is configurable via `BOT_LOGIN` env, sourced from a deploy-time `AGENT_PR_REVIEWER_BOT_LOGIN` env var, distinct per cluster.
- The dev App's repo scope (only `bborbe/go-skeleton`) is a GitHub-side constraint enforced by the App installation. Do NOT widen the dev scope; do NOT widen the prod scope either (it is already org-wide).
- Do NOT change Kafka topics, frontmatter contracts, prompt content, or any review-rendering logic. Auth identity changes only.
- Do NOT touch `lib/` or `agent/pr-reviewer/pkg/` or `agent/pr-reviewer/main.go` — those are owned by Prompts 1 and 2. The only exception: if Step 3 forces Option B (env-string PEM instead of file mount), edit Prompt 2's main-binary handling to read `PEM_KEY` env in addition to `PEM_KEY_FILE`; otherwise leave the binary alone.
- Do NOT commit — dark-factory handles git.
- Do NOT deploy. Rung-3 (dev cluster apply) and Rung-4 (prod cluster apply) are operator tasks executed manually per `docs/verifying-specs.md` — they are explicitly outside the YOLO container's reach.
</constraints>

<verification>
```bash
# 1. PEM key added to Secret manifest
grep -n 'PEM\|teamvaultFile\|teamvaultPassword' agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml
# Expected: a new line containing PEM as a key with a teamvault template

# 2. App-auth env vars wired in Config CR
grep -n 'APP_ID\|INSTALLATION_ID\|PEM_KEY_FILE\|BOT_LOGIN' agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml
# Expected: at least one line per var (4 total)

# 3. Deploy-time env-var references use the AGENT_PR_REVIEWER_ prefix
grep -rn 'AGENT_PR_REVIEWER_APP_ID\|AGENT_PR_REVIEWER_INSTALLATION_ID\|AGENT_PR_REVIEWER_BOT_LOGIN\|AGENT_PR_REVIEWER_PEM_KEY' agent/pr-reviewer/k8s/
# Expected: matches in either the Secret or the Config CR

# 3b. dev.env and prod.env updated with the new exports
grep -n 'AGENT_PR_REVIEWER_APP_ID\|AGENT_PR_REVIEWER_PEM_KEY' dev.env prod.env
# Expected: 2+ matches per file (APP_ID + PEM_KEY at minimum)

# 4. Plaintext PEM is NOT in any manifest
grep -rE 'BEGIN (RSA )?PRIVATE KEY' agent/pr-reviewer/k8s/
# Expected: 0 matches (PEM stays in Teamvault, never in git)

# 5. Config CR + Secret YAML both parse
for f in agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml; do
  python3 -c "import yaml; yaml.safe_load(open('$f'))" || echo "INVALID: $f"
done
# Expected: no `INVALID:` lines

# 6. Legacy GH_TOKEN entry still present (fallback retained)
grep -n 'GH_TOKEN' agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml
# Expected: at least 1 match

# 7. Docs updated with deploy-time env var list
grep -n 'AGENT_PR_REVIEWER_APP_ID\|Deploy-time' agent/pr-reviewer/docs/github-app-setup.md
# Expected: at least 1 match

# 8. CHANGELOG updated
grep -A6 '## Unreleased' CHANGELOG.md | head -10
# Expected: a line mentioning k8s Secret + Config CR wiring

# 9. Module-local precommit smoke
cd agent/pr-reviewer && make precommit
# Expected: exit 0; final line `ready to commit`
```
</verification>
