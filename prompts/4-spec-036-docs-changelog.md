---
status: draft
spec: [036-watcher-pr-rename-trigger-add-single-pr-trigger]
created: "2026-05-23T19:45:03Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

<summary>
- `watcher/github-pr/README.md` updated: `/trigger` docs replaced with `/check` (poll) and `/trigger?url=` (single PR) endpoints
- `/trigger?url=` includes example `curl` command and response JSON shape
- `CHANGELOG.md` gets a new `## Unreleased` entry describing both changes: the rename and the new single-PR endpoint
- No code changes in this prompt
</summary>

<objective>
Update documentation to reflect the two distinct endpoints introduced by this spec. The README gets example `curl` commands and response shapes. The CHANGELOG gets an entry under `## Unreleased`.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read `watcher/github-pr/README.md` — fully (already read). The current endpoints table lists `/trigger` as "Run a poll cycle in the background". This needs to be split into `/check` and `/trigger?url=`.

Read root `CHANGELOG.md` — check if `## Unreleased` section exists. Previous prompts add entries there.

**Documentation pattern:** The README already has the admin gateway URLs for dev/prod. The new `/trigger?url=` endpoint follows the same pattern.
</context>

<requirements>

1. **Update `watcher/github-pr/README.md`**

   a. **Update the Links section** — add the new `/check` endpoint URLs alongside the existing ones:

   ```
   Dev:
   https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/check
   https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger?url=https://github.com/owner/repo/pull/123

   Prod:
   https://prod.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/check
   https://prod.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger?url=https://github.com/owner/repo/pull/123
   ```

   b. **Update the HTTP Endpoints table** — replace the single `/trigger` row with two rows:

   | Path | Purpose |
   |---|---|
   | `/healthz` | Liveness probe (always returns 200 OK) |
   | `/readiness` | Readiness probe (always returns 200 OK) |
   | `/metrics` | Prometheus metrics |
   | `/check` | Run a poll cycle in the background; returns 200 immediately |
   | `/trigger?url=<pr_url>` | Trigger a single PR review by URL; bypasses dedup; returns task details |

   c. **Add endpoint details section** after the HTTP Endpoints table:

   ```
   ### /check

   Runs the full multi-repo poll cycle — identical to the cron job. Use this to manually trigger a poll.

   ```bash
   curl -X POST https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/check
   ```

   Response: `200 OK` (no body)

   ### /trigger?url=

   Triggers a single PR review, bypassing the per-(PR, SHA) dedup. Use this to force a re-run for a PR that has a stale task.

   ```bash
   curl -X POST "https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger?url=https://github.com/owner/repo/pull/123"
   ```

   Success response (HTTP 200):
   ```json
   {"task_id":"<uuid>","kafka_offset":123,"repo":"owner/repo","pr_number":123,"head_sha":"abc123..."}
   ```

   Error responses:
   - HTTP 400: missing or invalid URL (`{"error": "url query parameter required"}`)
   - HTTP 422: PR filtered by policy (draft, bot author, non-allowlisted repo, too old) (`{"error": "...", "filter": "...", "pr_url": "..."}`)
   - HTTP 502: Kafka publish failure (`{"error": "..."}`)
   ```

2. **Update root `CHANGELOG.md`**

   a. Check if `## Unreleased` section exists. If not, create it:

   ```markdown
   ## Unreleased

   - feat(watcher/github-pr): add `POST /trigger?url=` endpoint that triggers a single PR review by URL and bypasses the per-(PR, SHA) dedup for stale-task recovery
   - refactor(watcher/github-pr): rename `/trigger` poll endpoint to `/check` for clarity

   ## v0.25.11
   ...
   ```

   b. If `## Unreleased` already exists, append the two entries to it.

   The prefix for the rename is `refactor:` (existing code behavior preserved). The prefix for the new endpoint is `feat:` (new behavior).

3. **Verify the README changes:**

   ```bash
   grep -n "/check\|/trigger" watcher/github-pr/README.md
   ```
   Expected: `/check` appears in the links section and endpoints table; `/trigger?url=` appears in the links section, endpoints table, and new details section.

</requirements>

<constraints>
- Only edit `watcher/github-pr/README.md` and root `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- BSD license header NOT needed on markdown files
- Use the existing CHANGELOG entry format from previous entries
- Example URLs in README: use `owner/repo` and `123` as placeholder values
- Do NOT document the internal `forceBypassDedup` field — it's an implementation detail
</constraints>

<verification>
grep -n "/check\|/trigger?url=" watcher/github-pr/README.md
# Expected: /check appears at least 2x; /trigger?url= appears at least 3x (links, table, details)

grep -n "Unreleased" CHANGELOG.md
# Expected: section exists with the two entries

cd watcher/github-pr && make test
# Verify README changes don't break any tests
</verification>