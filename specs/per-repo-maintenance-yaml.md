---
status: draft
created: "2026-05-06T19:05:00Z"
---

# Per-repo `maintenance.yaml` overrides task frontmatter

## Summary

- After the build watcher detects a `green → red` transition for a repo, it fetches a per-repo config file (`maintenance.yaml`) from that repo's default branch
- If present, the file's `assignee`, `status`, and `phase` keys override the watcher's CLI/env defaults (set by the prerequisite spec `configurable-task-frontmatter`)
- Missing file → use defaults (no error)
- Malformed YAML or unrecognized keys → log warning, fall back to defaults (still publish; do not crash)
- The override applies ONLY to the next `CreateTaskCommand` for that episode — the file is fetched fresh per poll, no local cache, no cursor change
- Each repo can route its build-failure tasks to a different fixer (e.g. a `go-deps-update` agent vs a `python-uv-update` agent), or pin a custom initial status, without touching the watcher's deployment config

## Problem

Spec `configurable-task-frontmatter` lets the operator set ONE assignee/status/phase across the whole watcher fleet. But different repos have different fix runbooks: a Go repo wants the `go-deps-update` runbook, a Python repo wants `python-uv-update`, a docs-only repo wants `human-review` (no auto-fix at all). Today the only way to express that is to deploy multiple build-watcher StatefulSets with different `BUILD_ASSIGNEE` values and disjoint allowlists — operationally heavy.

A per-repo file in the repo itself is the natural home for repo-level routing config: the repo owner edits the file in the repo they own, the watcher reads it on the next poll. Same pattern as `dependabot.yml`, `.github/workflows/*.yml`, `.golangci.yml`.

## Goal

When the watcher is about to publish a `CreateTaskCommand` for a repo:

1. The watcher fetches `maintenance.yaml` (path TBD — see Constraints) from the repo's default branch via the existing GitHub client (`GetFileContents` or equivalent)
2. If present and parseable, the file's `assignee`, `status`, `phase` keys (each individually optional) override the corresponding watcher CLI/env defaults
3. The resulting frontmatter is what the published `CreateTaskCommand` carries
4. If absent or malformed, the watcher publishes with the watcher-level defaults; a debug-level log records the path taken

The file is read per publish event (per `green → red` transition), NOT per poll cycle. Re-publishes on the same episode do not re-fetch (and there are no re-publishes — the cursor blocks them).

## Non-goals

- Fields beyond `assignee`, `status`, `phase` (priority, labels, body customization, schedule overrides) — file format is forward-compatible; future fields can be added in follow-up specs
- Per-repo config in any location other than the repo itself (e.g. centralized YAML in the watcher's deployment) — defeats the "repo owner controls their own routing" principle
- Caching the file across polls — the file is small (<1 KB), GitHub's contents API is cheap, and re-publishes are blocked by the cursor anyway, so re-fetching at the rate of `green → red` transitions is fine
- Resolving `maintenance.yaml` from a non-default branch (e.g. the failing PR's branch) — the file lives at HEAD of default, period
- Schema validation against a JSON schema or strict struct unmarshaling — the parser tolerates unknown keys and logs them at WARN, but doesn't fail

## Desired Behavior

1. Repo `bborbe/maintainer` adds `maintenance.yaml` with `assignee: go-deps-fixer-agent` to its default branch. Next `green → red` transition publishes a task with `assignee: go-deps-fixer-agent`. Cursor + episode SHA logic unchanged.
2. Repo without `maintenance.yaml` continues to receive watcher-default frontmatter (e.g. `assignee: build-fixer-agent`).
3. Repo with `maintenance.yaml` containing only `assignee: foo` (no `status`, no `phase`) overrides only `assignee`; `status` and `phase` come from watcher defaults.
4. Malformed YAML in `maintenance.yaml` → watcher logs WARN with file path + parse error, publishes with watcher defaults. NO failure mode where the publish is dropped because of a malformed config file.
5. GitHub API 404 on the maintenance file (= "no maintenance.yaml at HEAD") → silent fall-through to defaults; no warning log (404 is the common case).
6. GitHub API 5xx fetching the file → WARN log, publish with defaults (don't block the publish on a transient registry error).

## Constraints

- **Path**: `maintenance.yaml` at repo root (NOT `.github/maintenance.yaml`) — this is the maintainer-platform's own config file, not a GitHub-platform file. Convention follows other root-level repo configs (`Dockerfile`, `Makefile`, `CHANGELOG.md`) rather than `.github/` (which GitHub uses for its own integrations). `.github/` would imply the file is for GitHub Actions; this file is for the maintainer watcher.
- **Schema** (all fields optional):
  ```yaml
  assignee: <string>   # overrides BUILD_ASSIGNEE
  status: <string>     # overrides BUILD_TASK_STATUS
  phase: <string>      # overrides BUILD_TASK_PHASE; empty string = explicit "no phase"
  ```
- **Override precedence**: `maintenance.yaml` > watcher CLI/env defaults > hardcoded fallback (the hardcoded fallback only matters if the operator omitted both the file and the env var, which the args' `default` tags prevent)
- **Empty values in the file**: `assignee: ""` in YAML is treated as "no override; use default". Distinguishing "absent key" from "empty value" adds complexity for no operator benefit — both mean "fall through".
- **Phase semantics**: `phase: ""` in the file means "set the published frontmatter's `phase` to empty (= omit)". This matches the watcher-default behavior when `BUILD_TASK_PHASE` is unset. For explicit override to a non-empty phase, set `phase: planning` (or whatever).
- **Read-only**: the watcher MUST NOT write or modify `maintenance.yaml`. It only reads.
- **GitHub API path**: `GET /repos/{owner}/{repo}/contents/maintenance.yaml?ref={default_branch}` — go-github's `Repositories.GetContents` returns the base64-decoded content
- **Timeout / cancellation**: the fetch uses the same `ctx` as the rest of the poll cycle; no separate timeout (the surrounding poll cycle's deadline applies)
- **Caching**: NO local cache; refetched per publish event. If the GitHub API rate limit becomes a concern, revisit in a follow-up
- **Backwards compat**: a repo that doesn't add `maintenance.yaml` sees zero behavior change. The whole feature is opt-in per repo.
- **Testability**: the maintenance loader takes the GitHub client interface as a dependency, returns a typed `MaintenanceConfig` struct (or zero-value if absent/error). Mockable via counterfeiter.
- **Error wrapping**: `github.com/bborbe/errors`, never `fmt.Errorf`

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `maintenance.yaml` absent (404) | Use defaults; no log | none |
| `maintenance.yaml` malformed YAML | WARN log with parse error + file path; publish with defaults | repo owner fixes the file |
| `maintenance.yaml` valid YAML, unknown key (e.g. `priority: high`) | INFO log "ignored unknown key"; apply known keys; publish | repo owner removes/aligns with documented schema |
| `maintenance.yaml` has `assignee: ""` | Treated as absent; defaults applied for that field | by design |
| GitHub API 5xx fetching file | WARN log; publish with defaults | transient — next poll retries |
| GitHub API rate-limited fetching file | WARN log; publish with defaults; counter `poll_errors_total{reason="rate_limited"}` increments | rate limit recovery |
| File >1 MB | Reject as malformed; WARN log; publish with defaults | repo owner shrinks file (this should never happen for a config file but is a sanity bound) |

## Security / Abuse

- **Read-only API access**: the watcher's GH token has `Contents: Read`; reading `maintenance.yaml` is within the existing scope. No permission expansion.
- **Untrusted repo content**: the maintenance file is content from a third-party repo (anyone in the allowlist). The parsed values flow into vault frontmatter, which the controller and downstream agents consume. **Risk**: a compromised allowlisted repo could set `assignee: arbitrary-string` and route tasks to a non-existent agent (DoS — task sits unconsumed) or to an existing agent that handles a different category (misroute — wrong fixer runs). **Mitigation**:
  - The allowlist is operator-controlled — only trusted repos can publish in the first place
  - Misroute blast radius: the wrong fixer reads its own input and either rejects or no-ops; it does not execute arbitrary code from the maintenance file
  - **Optional hardening (deferred)**: validate `assignee` against a closed set in watcher CLI/env (e.g. `--allowed-assignees=build-fixer-agent,go-deps-fixer-agent`); reject overrides not in the set. NOT in scope for v1; flag if the threat model changes.

## Acceptance Criteria

- [ ] New file `watcher/github-build/pkg/maintenance.go` with `MaintenanceConfig` struct (3 optional string fields) + `LoadMaintenanceConfig(ctx, ghClient, owner, repo, defaultBranch) (MaintenanceConfig, error)` function
- [ ] Loader returns zero-value config + nil error when GitHub returns 404
- [ ] Loader returns zero-value config + nil error when YAML is malformed (logs WARN)
- [ ] Loader returns zero-value config + wrapped error when GitHub returns 5xx
- [ ] Watcher's `applyStateMachine` (or `pollRepo`) calls the loader before `buildCreateTaskCommand`, passes resulting `MaintenanceConfig` into the command builder
- [ ] `buildCreateTaskCommand` applies override precedence: `MaintenanceConfig.Assignee` if non-empty, else default; same for status; same for phase
- [ ] `phase` field in frontmatter omitted when both maintenance file and default are empty
- [ ] Unit test: maintenance file with all three fields → all three override
- [ ] Unit test: maintenance file with one field → only that field overrides; other two from defaults
- [ ] Unit test: 404 → defaults applied
- [ ] Unit test: malformed YAML → defaults applied + WARN logged
- [ ] Unit test: empty string in file → treated as absent (default applies)
- [ ] Integration test (mockable via counterfeit `GitHubClient`): full poll cycle, watcher publishes with `assignee` overridden by maintenance file
- [ ] Add `maintenance.yaml` example to `docs/build-watcher.md` documenting the schema, override precedence, location
- [ ] Add `maintenance.yaml` to `bborbe/maintainer` itself as a real-world smoke test target (set `assignee: build-fixer-agent` to match default — verifies the loader path runs without changing behavior)
- [ ] CHANGELOG entry under `## Unreleased`
- [ ] `make precommit` clean

## Verification

```bash
cd watcher/github-build && make precommit
```

Real-world smoke test (after deploy to dev):

```bash
# 1. Add maintenance.yaml to bborbe/maintainer master:
echo 'assignee: go-deps-fixer-agent' > /tmp/maintenance.yaml
gh api -X PUT /repos/bborbe/maintainer/contents/maintenance.yaml \
  --input - <<EOF
{"message":"add maintenance.yaml","content":"$(base64 < /tmp/maintenance.yaml)"}
EOF

# 2. Wipe cursor + trigger watcher
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- rm /data/cursor.json
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- wget -qO- http://localhost:9090/trigger

# 3. Verify the new vault task has assignee=go-deps-fixer-agent (not build-fixer-agent):
sleep 5 && grep "^assignee:" ~/Documents/Obsidian/OpenClaw/tasks/710db30e*.md
# expected: assignee: go-deps-fixer-agent

# 4. Cleanup: remove the file from the repo
gh api -X DELETE /repos/bborbe/maintainer/contents/maintenance.yaml \
  -f message="remove maintenance.yaml" -f sha=$(gh api /repos/bborbe/maintainer/contents/maintenance.yaml --jq .sha)
```

## Do-Nothing Option

Leave per-repo routing as a deployment concern (run multiple watcher StatefulSets with disjoint allowlists, each with its own `BUILD_ASSIGNEE`). Cost: operational complexity scales linearly with the number of routing variants. Five repos that each want a different fixer = five watchers. With this spec, one watcher serves all repos and each repo declares its own routing — same operator effort whether you have 5 routes or 50.

## Related

- Builds on: `configurable-task-frontmatter` (provides the watcher-level defaults this spec falls back to)
- Companion: `specs/ideas/build-fixer-agent.md` (the consumer that reads `assignee` and dispatches accordingly)
- Pattern source: `dependabot.yml`, `.golangci.yml`, `.goreleaser.yaml` — repo-rooted config files for cross-repo tooling
