# github-pr-watcher

Polls the GitHub Search API for open pull requests and publishes a `CreateTaskCommand` to Kafka
for each new PR so the `agent/pr-reviewer` picks it up automatically.

## How It Works

The watcher runs a `user:<scope>` GitHub Search query on a configurable interval. On each poll it
compares the PR's current head SHA against the value stored in the cursor; if the SHA has changed
(force-push) the PR is re-submitted as a new task. The cursor is persisted to `/data/cursor.json`
between polls so that a restart does not re-trigger every known PR.

## Environment Variables

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

## Development

```bash
cd watcher/github
make test          # run unit tests
make generate      # regenerate counterfeiter mocks
make precommit     # format + lint + test + security checks
```

## Cursor Mechanism

The cursor is stored at `/data/cursor.json` and contains the timestamp of the most-recently-seen
PR update and a map of task identifier → head SHA. On a cold start (file missing or corrupt) the
cursor is initialised to the process start time, so only PRs updated after the first deployment are
submitted. Force-push detection works by comparing the stored head SHA for a known PR against the
value returned by the current poll; a mismatch triggers a new `CreateTaskCommand`.

## Relationship to pr-reviewer

This service feeds tasks into the `agent/pr-reviewer` Pattern B Job via Kafka: for every new or
force-pushed PR it publishes a `CreateTaskCommand` that the agent task controller picks up and
forwards to the pr-reviewer job.

## License

BSD 2-Clause License. See [LICENSE](../../LICENSE).
