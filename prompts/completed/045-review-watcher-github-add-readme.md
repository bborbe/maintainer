---
status: completed
summary: Created watcher/github/README.md with env vars, cursor mechanism, and pr-reviewer relationship; added watcher/github entry to root README Layout section; updated CHANGELOG.md with Unreleased entry.
container: code-reviewer-045-review-watcher-github-add-readme
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T14:51:25Z"
started: "2026-04-28T14:57:39Z"
completed: "2026-04-28T14:59:37Z"
---

<summary>
- watcher/github is a fully shipped service with no README.md
- Developers have no documentation of required env vars, how to run locally, or how it connects to pr-reviewer
- The root README.md does not mention watcher/github at all in its Layout section
- A README for watcher/github should document purpose, env vars, deployment, and the cursor mechanism
- The root README Layout section needs a new entry for the shipped service
</summary>

<objective>
Create `watcher/github/README.md` documenting the service purpose, all environment variables, local development commands, cursor mechanism, and relationship to the pr-reviewer agent. Add an entry for `watcher/github/` to the root `README.md` Layout section.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/main.go` (~lines 34-44): all env vars from the `application` struct tags
- `watcher/github/pkg/cursor.go` (~lines 17-62): cursor persistence details
- `README.md` (full): the root README to find the Layout section and "(future)" reference
- `agent/pr-reviewer/README.md`: style reference for a service README in this repo
</context>

<requirements>
1. **Create `watcher/github/README.md`** with these sections:

   **Title and description:**
   ```markdown
   # github-pr-watcher

   Polls the GitHub Search API for open pull requests and publishes a `CreateTaskCommand` to Kafka
   for each new PR so the `agent/pr-reviewer` picks it up automatically.
   ```

   **How it works** (2-3 sentences): GitHub Search query with `user:<scope>`, cursor persistence to `/data/cursor.json`, force-push detection via head SHA comparison.

   **Environment variables table** (sourced from `main.go` struct tags):
   | Variable | Required | Default | Description |
   |---|---|---|---|
   | `GH_TOKEN` | yes | — | GitHub personal access token (read scope sufficient) |
   | `KAFKA_BROKERS` | yes | — | Comma-separated Kafka broker addresses |
   | `STAGE` | yes | — | Deployment stage (`dev` or `prod`) |
   | `POLL_INTERVAL` | no | `5m` | Poll interval (Go duration string) |
   | `REPO_SCOPE` | no | `bborbe` | GitHub user or org to search for PRs |
   | `BOT_ALLOWLIST` | no | `dependabot[bot],renovate[bot]` | Comma-separated bot author logins to skip |
   | `LISTEN` | no | `:9090` | HTTP listen address for healthz/metrics |
   | `SENTRY_DSN` | no | — | Sentry DSN for error tracking |
   | `SENTRY_PROXY` | no | — | Optional HTTP proxy URL for Sentry transport |

   **Development commands:**
   ```bash
   cd watcher/github
   make test          # run unit tests
   make generate      # regenerate counterfeiter mocks
   make precommit     # format + lint + test + security checks
   ```

   **Cursor mechanism** (2-3 sentences): what `/data/cursor.json` stores, what happens on cold start, how force-push is detected.

   **Relationship to pr-reviewer** (1-2 sentences): this service feeds tasks into the `agent/pr-reviewer` Pattern B Job via Kafka.

   **License section:**
   ```markdown
   ## License

   BSD 2-Clause License. See [LICENSE](../../LICENSE).
   ```

2. **Update root `README.md`** Layout section (the fenced code block under `## Layout`):
   - The current tree starts with `code-reviewer/` and lists only `agent/pr-reviewer/`
   - Add a sibling entry for `watcher/github/` after the `agent/pr-reviewer/` block, e.g.:
     ```
     ├── watcher/github/            GitHub PR watcher service (own go.mod)
     │   └── pkg/                   poll loop, GitHub client, cursor, Kafka publisher
     ```
   - Optionally also add a row to the Mode/services table at the top documenting the watcher Job

3. Run `cd watcher/github && make precommit` — must exit 0 (READMEs do not need license headers; verify the precommit does not fail on the new file).
</requirements>

<constraints>
- Only change files in `watcher/github/` and the root `README.md`
- Do NOT commit — dark-factory handles git
- Do NOT modify any `.go` files
- README.md files do not need BSD license headers — only Go files do
- Keep the root README changes minimal — only fix the stale "(future)" reference
</constraints>

<verification>
ls watcher/github/README.md
# Expected: file exists

grep -n "watcher/github" README.md
# Expected: at least one line referencing watcher/github in Layout section

grep -n "GH_TOKEN\|KAFKA_BROKERS\|REPO_SCOPE\|SENTRY_PROXY" watcher/github/README.md
# Expected: matches in env var table

cd watcher/github && make precommit
</verification>
