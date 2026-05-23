---
status: approved
spec: [037-migrate-pr-watcher-to-github-app]
created: "2026-05-23T21:41:43Z"
queued: "2026-05-23T21:41:43Z"
branch: fix/pr-watcher-github-app-auth-k8s-docs
---

<summary>
- `watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml`: add `PEM_KEY` Secret entry sourced via `teamvaultFileBase64` (NOT `teamvaultPassword` — that returns 0 bytes for file-type Teamvault entries, the bug that bit spec 033 mid-rollout). `GH_TOKEN` stays for now as a rollout fallback.
- `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml`: add `APP_ID` and `INSTALLATION_ID` env vars (plain integer values, not secrets) sourced from the operator's deploy shell. `PEM_KEY` flows in from the Secret via the existing pattern.
- `dev.env` and `prod.env`: add 3 new exports per stage pointing at the existing pr-reviewer App identities + Teamvault PEM keys.
- `CHANGELOG.md`: create a new `## Unreleased` heading directly above the topmost `## vX.Y.Z` version heading and add the bullet under it. (No `## Unreleased` heading currently exists — the previous one was just consumed by a recent release; the exact version doesn't matter, just position relative to the topmost version.) Bullet records the `NewClient`-not-`MintIAT` rationale (long-lived StatefulSet) so future readers see why this differs from spec 033's `MintIAT` pattern.
</summary>

<objective>
The PR watcher pod, once deployed, can pick up the GitHub App credentials from its Secret + env (no code edits). The CHANGELOG records the migration with its load-bearing technical rationale.
</objective>

<context>
**Code prerequisite:** the sibling prompt `pr-watcher-app-auth-code.md` adds the `APP_ID`, `INSTALLATION_ID`, `PEM_KEY` env consumers in `main.go`. This prompt only wires the Secret + env so those consumers receive values at deploy time.

**Reuse, don't register:** the pr-reviewer GitHub App identities (registered for spec 033) are reused as-is. No new App registration. Permissions (`Contents: Read`, `Pull requests: Read & Write`, `Metadata: Read`) are sufficient — the watcher only does `SearchPRs` + `GetPRDetails`, no writes to GitHub.

| Stage | App | App ID | Installation ID | Teamvault PEM key |
|-------|-----|--------|-----------------|-------------------|
| prod  | `Ben's Pull Request Reviewer`     | `3798945` | `134414316` | `kLoejw` |
| dev   | `Ben's Pull Request Reviewer Dev` | `3800041` | `134435225` | `eqKj8L` |

**Reference Secret pattern** (the canonical example of `teamvaultFileBase64` for PEM mounts): `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-secret.yaml:11` — `PEM_KEY: '{{ "AGENT_PR_REVIEWER_PEM_KEY" | env | teamvaultFileBase64 }}'`. Mirror this shape exactly.

**Reference env pattern:** `prod.env` already contains `export AGENT_PR_REVIEWER_PEM_KEY=kLoejw` (Teamvault key for the agent). The watcher gets its own env-var name pointing at the same Teamvault entry.

**CHANGELOG state:** there is NO `## Unreleased` heading right now — the previous one was just consumed by a recent release. The project convention is to maintain a transient `## Unreleased` staging section between commits and releases (dark-factory's auto-release flips it to `## vX.Y.Z` at tag time). This prompt must CREATE a fresh `## Unreleased` heading at the top of the per-version list, directly above the topmost `## vX.Y.Z` heading currently in the file. Find the topmost `## vX.Y.Z` heading by reading the file rather than hard-coding a version — releases ship continuously and the anchor will be stale by the time the agent runs.
</context>

<requirements>

1. **Edit `watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml`** — add the `PEM_KEY` entry next to the existing `GH_TOKEN` line, using `teamvaultFileBase64`:

   ```yaml
   GH_TOKEN: '{{ "WATCHER_GITHUB_PR_GH_TOKEN_KEY" | env | teamvaultPassword | base64 }}'
   PEM_KEY: '{{ "WATCHER_GITHUB_PR_PEM_KEY" | env | teamvaultFileBase64 }}'
   ```

   **Critical:** `teamvaultFileBase64` (single-step, already base64-encoded), NOT `teamvaultPassword | base64` — the PEM is a file-type Teamvault entry and `teamvaultPassword` returns empty for file entries.

2. **Edit `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml`** — add `APP_ID`, `INSTALLATION_ID`, and `PEM_KEY` env entries on the watcher container:

   - `APP_ID` and `INSTALLATION_ID` are plain integer values (public, not secret) — source them directly from the operator's deploy environment via the standard pattern used for `STAGE` / numeric configs in this file. Read the existing env block to find the matching pattern; it's typically either:
     ```yaml
     - name: APP_ID
       value: '{{ "WATCHER_GITHUB_PR_APP_ID" | env }}'
     - name: INSTALLATION_ID
       value: '{{ "WATCHER_GITHUB_PR_INSTALLATION_ID" | env }}'
     ```
     OR via `envFrom` if the file uses that pattern. **Match the existing convention** (e.g. how `STAGE` or `KAFKA_BROKERS` is set).
   - `PEM_KEY` comes from the Secret. Use `valueFrom.secretKeyRef` (matching how `GH_TOKEN` is already wired in the same file), pointing at the `PEM_KEY` key in `maintainer-watcher-github-pr` Secret.

3. **Edit `dev.env`** — add three exports after the existing `WATCHER_GITHUB_PR_*` block:

   ```bash
   export WATCHER_GITHUB_PR_APP_ID=3800041
   export WATCHER_GITHUB_PR_INSTALLATION_ID=134435225
   export WATCHER_GITHUB_PR_PEM_KEY=eqKj8L
   ```

4. **Edit `prod.env`** — same pattern, prod values:

   ```bash
   export WATCHER_GITHUB_PR_APP_ID=3798945
   export WATCHER_GITHUB_PR_INSTALLATION_ID=134414316
   export WATCHER_GITHUB_PR_PEM_KEY=kLoejw
   ```

5. **Edit `CHANGELOG.md`** — `## Unreleased` does NOT currently exist (consumed by a recent release). First, READ the file to find the topmost `## vX.Y.Z` heading (do NOT hard-code the version — releases ship continuously). Insert a fresh `## Unreleased` heading + blank line + the bullet directly above that topmost version heading. Shape:

   ```markdown
   ## Unreleased

   - feat(watcher/github-pr): add GitHub App auth via `APP_ID` + `INSTALLATION_ID` + `PEM_KEY` env vars (reuses existing pr-reviewer Apps `3798945` / `3800041`); uses `lib/githubapp.NewClient` (auto-refreshing IAT via `ghinstallation/v2`) because the watcher is a long-lived StatefulSet — a single `MintIAT` call would expire after 1 hour. Partial App env returns an error naming the missing field; legacy `GH_TOKEN` retained as fallback for rollout safety.

   ## v<topmost-existing-version>
   ```

   Do NOT touch any existing `## vX.Y.Z` bullet or prior version sections.

</requirements>

<constraints>
- Files edited: `watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml`, `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml`, `dev.env`, `prod.env`, `CHANGELOG.md`.
- No files created. No code files edited.
- PEM bytes must NEVER appear in git. Only Teamvault keys (which are public identifiers) live in env files / templates.
- App IDs and Installation IDs are public; safe to commit in `dev.env`/`prod.env`.
- The existing `GH_TOKEN` Secret entry stays — removing it is a separate cleanup task after a soak period proves App auth stable.
- Exactly one `## Unreleased` heading in CHANGELOG.md after the edit. (Currently zero; create exactly one.)
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
# 1. PEM_KEY uses teamvaultFileBase64 (not teamvaultPassword — spec-033 regression guard)
grep -n 'PEM_KEY.*teamvaultFileBase64' /workspace/watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml
# Expect: 1 match
grep -n 'PEM_KEY.*teamvaultPassword' /workspace/watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml
# Expect: 0 matches

# 2. Three new env vars wired in the STS
grep -nE 'name: (APP_ID|INSTALLATION_ID|PEM_KEY)' /workspace/watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml
# Expect: 3 matches

# 3. Both env files have the new exports
grep -c 'WATCHER_GITHUB_PR_APP_ID\|WATCHER_GITHUB_PR_INSTALLATION_ID\|WATCHER_GITHUB_PR_PEM_KEY' /workspace/dev.env
# Expect: 3
grep -c 'WATCHER_GITHUB_PR_APP_ID\|WATCHER_GITHUB_PR_INSTALLATION_ID\|WATCHER_GITHUB_PR_PEM_KEY' /workspace/prod.env
# Expect: 3

# 4. Exactly one ## Unreleased heading (no duplicates)
grep -c '^## Unreleased$' /workspace/CHANGELOG.md
# Expect: 1

# 5. CHANGELOG entry mentions watcher + NewClient + long-lived rationale
grep -A20 '^## Unreleased$' /workspace/CHANGELOG.md | grep -E 'watcher/github-pr.*App|NewClient|long-lived StatefulSet'
# Expect: >= 1 line matching
</verification>
