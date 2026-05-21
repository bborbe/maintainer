# GitHub App Setup

Authentication for the PR reviewer agent uses a GitHub App identity. This document covers the App credentials, auth flow, and the smoke-test workflow used to validate everything end-to-end before wiring into production.

## Background

The agent previously authenticated as user `pr-review-of-ben` with a Personal Access Token. GitHub Trust & Safety flagged that user account as "Spammy", causing the REST `/reviews` endpoint to silently filter out all bot reviews — blocking auto-merge gates and external automation. GitHub Apps are first-class identities, not user accounts, and are not subject to user-level spam classification.

## App Identity

| Field | Value |
|-------|-------|
| App name | `Ben's Pull Request Reviewer` |
| App slug | `ben-s-pull-request-reviewer` |
| App settings | <https://github.com/settings/apps/ben-s-pull-request-reviewer> |
| App ID | `3798945` |
| Client ID | `Iv23liSDydoekRW3gOIk` |
| Installation ID (`bborbe`) | `134414316` |
| Bot login in API responses | `ben-s-pull-request-reviewer[bot]` |
| Private key (PEM) | Teamvault [`kLoejw`](https://teamvault.benjamin-borbe.de/secrets/kLoejw/) |

App ID and Installation ID are public values and safe to commit. The PEM is secret — store only in Teamvault and Kubernetes Secrets.

## Permissions

Minimum permissions required for the agent's capabilities:

| Capability | Permission |
|------------|------------|
| Clone PR sources (git over HTTPS) | Contents: **Read** |
| Read PR comments and reviews | Pull requests: Read |
| Write PR comments | Pull requests: **Write** |
| Post review (approve / request-changes / comment) | Pull requests: **Write** |
| Dismiss prior reviews | Pull requests: **Write** |
| Repository metadata (mandatory) | Metadata: **Read** |

All other Repository, Organization, and Account permissions remain **No access**. Webhook is disabled — the agent polls; it does not receive events.

## Auth Flow

1. Sign a short-lived JWT (≤10 min, RS256) with the App's RSA private key. Claim `iss = APP_ID`.
2. Exchange the JWT for an **installation access token** (IAT, 1 hr TTL):

   ```
   POST https://api.github.com/app/installations/{INSTALLATION_ID}/access_tokens
   Authorization: Bearer <jwt>
   Accept: application/vnd.github+json
   ```

3. Use the IAT as `Authorization: Bearer <iat>` for REST API calls.

The IAT is a drop-in replacement for `GH_TOKEN`:

- Works with the `gh` CLI (`GH_TOKEN=<iat>`)
- Works as the password for HTTPS git clones (`https://x-access-token:<iat>@github.com/...`)
- Works with REST API calls (`Authorization: token <iat>` or `Bearer <iat>`)

Production code will use `github.com/bradleyfalzon/ghinstallation/v2`, which provides an `http.RoundTripper` that mints + caches IATs transparently.

## Smoke Test: `cmd/mint-iat`

Stdlib-only validation tool. Mints a JWT, exchanges it for an IAT, then calls `GET /app` to verify the App identity and permissions. Used to confirm credentials work end-to-end before wiring into the production agent.

### Flags

| Flag | Env var | Meaning |
|------|---------|---------|
| `-app-id` | `APP_ID` | GitHub App ID (numeric) |
| `-installation-id` | `INSTALLATION_ID` | Installation ID (numeric) |
| `-pem-key` | `PEM_KEY` | PEM content (entire key string) |
| `-pem-key-file` | `PEM_KEY_FILE` | Path to PEM file |
| `-verify` | — | Call `GET /app` after mint (default `true`) |

Exactly one of `PEM_KEY` / `PEM_KEY_FILE` must be set.

### Run from Teamvault (PEM never touches disk)

```bash
TEAMVAULT_URL=https://teamvault.benjamin-borbe.de \
TEAMVAULT_USER=bborbe \
TEAMVAULT_PASSWORD='...' \
TEAMVAULT_KEY=kLoejw \
  teamvault-file | base64 -d | \
  go run ./cmd/mint-iat \
    -app-id=3798945 \
    -installation-id=134414316 \
    -pem-key-file=/dev/stdin
```

### Run from a local PEM file

```bash
go run ./cmd/mint-iat \
  -app-id=3798945 \
  -installation-id=134414316 \
  -pem-key-file=$HOME/Documents/secrets/bborbe-pr-reviewer.pem
```

### Run with PEM as env var (e.g. mounted from k8s Secret)

```bash
PEM_KEY="$(cat ~/Documents/secrets/bborbe-pr-reviewer.pem)" \
APP_ID=3798945 \
INSTALLATION_ID=134414316 \
  go run ./cmd/mint-iat
```

### Expected output

```
✓ JWT minted (len=446)
✓ IAT minted, expires at 2026-05-21T20:29:02Z
✓ GET /app: id=3798945 slug=ben-s-pull-request-reviewer name="Ben's Pull Request Reviewer" owner=bborbe
  permissions: map[contents:read metadata:read pull_requests:write]
ghs_...
```

The IAT (`ghs_...` prefix) is printed on stdout — pipe / capture as needed for subsequent API calls.

## PEM Rotation

When the PEM needs rotation (compromise or periodic):

1. <https://github.com/settings/apps/ben-s-pull-request-reviewer> → **Private keys** → **Generate a private key** (downloads new PEM)
2. Upload the new PEM to Teamvault entry `kLoejw` (replace contents)
3. Update the Kubernetes Secret in dev + prod clusters with the new PEM
4. Restart pr-reviewer pods to pick up the new key
5. After confirming the new key works, delete the old key entry on the App settings page

App ID and Installation ID remain unchanged across rotations.

## Gotchas

- **`GET /user` does not work for Apps** — Apps have no user identity. The current `poster.go::checkBotIdentity` uses `GET /user` and must be reworked to either `GET /app` or be removed.
- **Bot login has literal brackets** — `ben-s-pull-request-reviewer[bot]`. Any string-match logic referencing `pr-review-of-ben` must update.
- **IAT TTL is 1 hour** — production code must cache + refresh, not mint per call. `ghinstallation/v2` handles this transparently.
- **Required-approvals gates and App `APPROVE`** — GitHub does not explicitly document whether App reviews count toward numeric required-approvals rules. Test after migration; document findings here.
- **PEM never in git** — only Teamvault and Kubernetes Secrets.

## Migration Status

- 2026-05-21: App registered. PEM stored in Teamvault. App installed on `@bborbe`. Permissions set (Contents: Read, Pull requests: Write, Metadata: Read). `cmd/mint-iat` smoke test passing end-to-end.
- Pending: production code refactor (`pkg/githubposter`, `pkg/steps_gh_token.go`, `pkg/githubauth`), k8s manifest updates, deploy + verification, PAT user retirement.

Tracked in vault task `Migrate PR Reviewer from User PAT to GitHub App` under goal `GitHub Code Reviewer Agent - Base` (F1).
