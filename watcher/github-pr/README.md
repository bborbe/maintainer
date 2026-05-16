# maintainer-watcher-github-pr

Polls the GitHub Search API for open pull requests and publishes a `CreateTaskCommand` to Kafka
for each new or force-pushed PR so the `agent/pr-reviewer` picks it up automatically.

## Links

Dev:
https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/setloglevel/3
https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger

Prod:
https://prod.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/setloglevel/3
https://prod.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger

## How It Works

The watcher runs a `user:<scope>` GitHub Search query on a configurable interval. On each poll
it compares the PR's current head SHA against the value stored in the cursor; if the SHA has
changed (force-push) the PR is re-submitted as a new task. The cursor is persisted to
`/data/cursor.json` between polls so a restart does not re-trigger every known PR.

Two independent decision chains run per PR — see [`docs/watcher-decision-chains.md`](../../docs/watcher-decision-chains.md):

- **`TaskCreationFilter`** — should we create a task at all? (drafts, bots, WIP titles, age, allowlist)
- **`TrustGate`** — given a task is created, auto-process or route to `human_review`? (trusted authors)

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `GH_TOKEN` | yes | — | GitHub personal access token (read scope sufficient) |
| `KAFKA_BROKERS` | yes | — | Comma-separated Kafka broker list |
| `STAGE` | yes | — | Deployment stage (`dev` or `prod`) |
| `TRUSTED_AUTHORS` | yes | — | Comma-separated trusted GitHub logins; empty list refuses startup |
| `LISTEN` | no | `:9090` | HTTP listen address (`/healthz`, `/readiness`, `/metrics`, `/trigger`) |
| `POLL_INTERVAL` | no | `5m` | Poll interval (Go duration string) |
| `REPO_SCOPE` | no | `bborbe` | GitHub user or org to search for PRs |
| `REPO_ALLOWLIST` | no | — | Comma-separated host-qualified repo allowlist (`host/owner/repo`); empty means allow-all |
| `BOT_ALLOWLIST` | no | `dependabot[bot],renovate[bot]` | Comma-separated bot author logins to skip |
| `MAX_PR_AGE` | no | `2160h` (90d) | Skip PRs older than this; empty disables |
| `BACKFILL_DURATION` | no | `720h` (30d) | On cold start, backdate the initial cursor by this; empty disables |
| `SENTRY_DSN` | no | — | Sentry DSN for error tracking |
| `SENTRY_PROXY` | no | — | HTTP proxy URL for Sentry transport |

## HTTP Endpoints

| Path | Purpose |
|---|---|
| `/healthz` | Liveness probe (always returns 200 OK) |
| `/readiness` | Readiness probe (always returns 200 OK) |
| `/metrics` | Prometheus metrics |
| `/trigger` | Run a poll cycle in the background; returns 200 immediately |

## Development

```bash
cd watcher/github-pr
make test          # run unit tests
make generate      # regenerate counterfeiter mocks
make precommit     # format + lint + test + security checks
```

## Cursor Mechanism

The cursor at `/data/cursor.json` records the timestamp of the most-recently-seen PR update plus
a map of `task_id → head_sha`. On cold start (missing file) the cursor is initialised to the
process start time minus `BACKFILL_DURATION`, so only PRs updated within that window are
submitted. Force-push detection compares the stored head SHA for a known PR against the value
returned by the current poll; a mismatch publishes a new `CreateTaskCommand` with the new SHA.

A corrupt cursor refuses startup — see `pkg/cursor.go`.

## Relationship to pr-reviewer

This service feeds tasks into the `agent/pr-reviewer` Pattern B Job via Kafka: for every new or
force-pushed PR it publishes a `CreateTaskCommand` that the agent task controller picks up and
spawns into a per-task K8s Job. The agent runs `/coding:pr-review` and posts the verdict back to
the PR.

See [`docs/architecture.md`](../../docs/architecture.md) for the full pipeline.

## License

BSD 2-Clause License. See [LICENSE](../../LICENSE).
