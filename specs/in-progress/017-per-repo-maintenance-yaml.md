---
status: verifying
approved: "2026-05-06T17:46:03Z"
generating: "2026-05-06T17:48:00Z"
prompted: "2026-05-06T17:59:14Z"
verifying: "2026-05-06T18:50:30Z"
branch: dark-factory/per-repo-maintenance-yaml
---

## Summary

- After the build watcher detects a `green → red` transition for a repo, it fetches a per-repo config file (`.maintenance.yaml`) from that repo's default branch
- The file is hierarchically scoped: each maintainer service has its own subtree mirroring the source layout (`watcher.github-build`, `watcher.github-pr`, future `agent.build-fixer`, etc.). The build watcher reads ONLY the `watcher.github-build` subtree; other services ignore it
- Inside the build watcher's subtree, the file's `assignee`, `status`, and `phase` keys override the watcher's CLI/env defaults (set by the prerequisite spec `configurable-task-frontmatter`)
- Missing file, missing subtree, or missing keys → use defaults (no error)
- Malformed YAML or unrecognized keys → log warning, fall back to defaults (still publish; do not crash)
- The override applies ONLY to the next `CreateTaskCommand` for that episode — the file is fetched fresh per publish, no local cache, no cursor change
- One file per repo serves all maintainer services (build watcher today; PR watcher, fixer agent, others later)

## Problem

Spec `configurable-task-frontmatter` lets the operator set ONE assignee/status/phase across the whole watcher fleet. But different repos have different fix runbooks: a Go repo wants the `go-deps-update` runbook, a Python repo wants `python-uv-update`, a docs-only repo wants `human-review` (no auto-fix at all). Today the only way to express that is to deploy multiple build-watcher StatefulSets with different `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE` values and disjoint allowlists — operationally heavy.

A per-repo file in the repo itself is the natural home for repo-level routing config: the repo owner edits the file in the repo they own, the watcher reads it on the next poll. Same pattern as `dependabot.yml`, `.github/workflows/*.yml`, `.golangci.yml`.

## Goal

After this work ships, repos express per-service config via a root-level `.maintenance.yaml` whose schema mirrors the maintainer source tree:

```yaml
watcher:
  github-build:
    assignee: go-deps-fixer-agent
    status: todo
    phase: planning
```

The build watcher honors its `watcher.github-build` subtree on every `green → red` publish — `assignee`, `status`, and `phase` from the file override the watcher's CLI/env defaults; missing file, missing subtree, or missing keys fall through silently to defaults. Repo owners can change routing for any maintainer service without touching deployment config. The same file remains useful as more maintainer services land — each gets its own subtree.

## Non-goals

- Fields beyond `assignee`, `status`, `phase` (priority, labels, body customization, schedule overrides) — file format is forward-compatible; future fields can be added in follow-up specs
- Per-repo config in any location other than the repo itself (e.g. centralized YAML in the watcher's deployment) — defeats the "repo owner controls their own routing" principle
- Caching the file across polls — the file is small (<1 KB), GitHub's contents API is cheap, and re-publishes are blocked by the cursor anyway, so re-fetching at the rate of `green → red` transitions is fine
- Resolving `.maintenance.yaml` from a non-default branch (e.g. the failing PR's branch) — the file lives at HEAD of default, period
- Schema validation against a JSON schema or strict struct unmarshaling — the parser tolerates unknown keys and logs them at WARN, but doesn't fail

## Desired Behavior

1. Repo `bborbe/maintainer` adds `.maintenance.yaml` with `watcher.github-build.assignee: go-deps-fixer-agent` to its default branch. Next `green → red` transition publishes a task with `assignee: go-deps-fixer-agent`. Cursor + episode SHA logic unchanged.
2. Repo without `.maintenance.yaml` continues to receive watcher-default frontmatter (e.g. `assignee: build-fixer-agent`).
3. Repo with `.maintenance.yaml` containing only `watcher.github-build.assignee: foo` (no `status`, no `phase`) overrides only `assignee`; `status` and `phase` come from watcher defaults.
4. Repo with `.maintenance.yaml` that has the file but no `watcher.github-build` subtree (e.g. only `watcher.github-pr.*` populated) → build watcher uses its defaults; no error.
5. Malformed YAML in `.maintenance.yaml` → watcher logs WARN with file path + parse error, publishes with watcher defaults. NO failure mode where the publish is dropped because of a malformed config file.
6. GitHub API 404 on the maintenance file (= "no .maintenance.yaml at HEAD") → silent fall-through to defaults; no warning log (404 is the common case).
7. GitHub API 5xx fetching the file → WARN log, publish with defaults (don't block the publish on a transient registry error).

## Constraints

- **Path**: `.maintenance.yaml` at repo root (NOT `.github/.maintenance.yaml`) — this is the maintainer-platform's own config file, not a GitHub-platform file. Convention follows other root-level repo configs (`Dockerfile`, `Makefile`, `CHANGELOG.md`, `.dark-factory.yaml`) rather than `.github/` (which GitHub uses for its own integrations).
- **Schema** — hierarchical, mirrors the maintainer source tree. Build watcher reads ONLY `watcher.github-build`. All keys optional at every level:
  ```yaml
  watcher:
    github-build:
      assignee: <string>   # overrides WATCHER_GITHUB_BUILD_TASK_ASSIGNEE
      status: <string>     # overrides WATCHER_GITHUB_BUILD_TASK_STATUS
      phase: <string>      # overrides WATCHER_GITHUB_BUILD_TASK_PHASE; empty string = explicit "no phase"
    # Future: watcher.github-pr (PR watcher reads its own subtree)
  # Future: agent.build-fixer.* (fixer agent reads its own subtree)
  ```
- **Subtree isolation**: each maintainer service reads ONLY its own subtree. The build watcher MUST NOT read or react to keys outside `watcher.github-build`. Adding new keys to other subtrees MUST NOT break the build watcher.
- **Override precedence**: `.maintenance.yaml` `watcher.github-build.<key>` > watcher CLI/env default for the matching key > hardcoded fallback. Lookup is per-key (a missing key in the file does not invalidate other keys).
- **Empty values in the file**: `assignee: ""` in YAML is treated as "no override; use default". Distinguishing "absent key" from "empty value" adds complexity for no operator benefit — both mean "fall through".
- **Phase semantics**: `phase: ""` in the file means "set the published frontmatter's `phase` to empty (= omit)". This matches the watcher-default behavior when `WATCHER_GITHUB_BUILD_TASK_PHASE` is unset. For explicit override to a non-empty phase, set `phase: planning` (or whatever).
- **Read-only**: the watcher MUST NOT write or modify `.maintenance.yaml`. It only reads.
- **GitHub API path**: `GET /repos/{owner}/{repo}/contents/.maintenance.yaml?ref={default_branch}` — go-github's `Repositories.GetContents` returns the base64-decoded content
- **Timeout / cancellation**: the fetch uses the same `ctx` as the rest of the poll cycle; no separate timeout (the surrounding poll cycle's deadline applies)
- **Caching**: NO local cache; refetched per publish event. If the GitHub API rate limit becomes a concern, revisit in a follow-up
- **Backwards compat**: a repo that doesn't add `.maintenance.yaml` sees zero behavior change. The whole feature is opt-in per repo.
- **File size sanity bound**: reject files larger than 1 MB as malformed (sanity bound — a config file should be a few hundred bytes; anything larger is either misuse or attack surface)
- **Testability**: the maintenance loader takes the GitHub client interface as a dependency and is mockable via counterfeiter for unit tests
- **Error wrapping**: `github.com/bborbe/errors`, never `fmt.Errorf`

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `.maintenance.yaml` absent (404) | Use defaults; no log | none |
| `.maintenance.yaml` malformed YAML | WARN log with parse error + file path; publish with defaults | repo owner fixes the file |
| `.maintenance.yaml` valid YAML, unknown key inside `watcher.github-build` (e.g. `priority: high`) | INFO log "ignored unknown key"; apply known keys; publish | repo owner removes/aligns with documented schema |
| `.maintenance.yaml` valid YAML, no `watcher.github-build` subtree (only other services configured) | Treated as no overrides for build watcher; defaults applied; no log | by design — subtree isolation |
| `.maintenance.yaml` has `watcher.github-build.assignee: ""` | Treated as absent; defaults applied for that field | by design |
| GitHub API 5xx fetching file | WARN log; publish with defaults | transient — next poll retries |
| GitHub API rate-limited fetching file | WARN log; publish with defaults; counter `poll_errors_total{reason="rate_limited"}` increments | rate limit recovery |
| File >1 MB | Reject as malformed; WARN log; publish with defaults | repo owner shrinks file (sanity bound — see Constraints) |

## Security / Abuse

- **Read-only API access**: the watcher's GH token has `Contents: Read`; reading `.maintenance.yaml` is within the existing scope. No permission expansion.
- **Untrusted repo content**: the maintenance file is content from a third-party repo (anyone in the allowlist). The parsed values flow into vault frontmatter, which the controller and downstream agents consume. **Risk**: a compromised allowlisted repo could set `assignee: arbitrary-string` and route tasks to a non-existent agent (DoS — task sits unconsumed) or to an existing agent that handles a different category (misroute — wrong fixer runs). **Mitigation**:
  - The allowlist is operator-controlled — only trusted repos can publish in the first place
  - Misroute blast radius: the wrong fixer reads its own input and either rejects or no-ops; it does not execute arbitrary code from the maintenance file
  - **Optional hardening (deferred)**: validate `assignee` against a closed set in watcher CLI/env (e.g. `--allowed-assignees=build-fixer-agent,go-deps-fixer-agent`); reject overrides not in the set. NOT in scope for v1; flag if the threat model changes.

## Acceptance Criteria

- [ ] Repo with `.maintenance.yaml` containing `watcher.github-build.{assignee,status,phase}` → published task carries those exact values
- [ ] Repo with only `watcher.github-build.assignee` populated → published task overrides assignee, falls back to watcher defaults for status and phase
- [ ] Repo with `.maintenance.yaml` whose only populated subtree is `watcher.github-pr` (or any non-build subtree) → build watcher uses defaults; no warning log
- [ ] Repo with no `.maintenance.yaml` (404) → published task uses watcher defaults; no error log
- [ ] Repo with malformed `.maintenance.yaml` → published task uses watcher defaults; WARN log records the parse error and the file path; the publish is NOT dropped
- [ ] Repo with `.maintenance.yaml` carrying an unknown key inside `watcher.github-build` (e.g. `priority: high`) → known keys still override; unknown key logged at INFO and ignored
- [ ] `watcher.github-build.assignee: ""` (empty string in the file) is treated identically to the key being absent — watcher default applies for that field
- [ ] `phase: <non-empty>` in the file → `phase` key appears in the published vault frontmatter; `phase` absent or empty in file AND empty default → no `phase` key in the published frontmatter
- [ ] GitHub 5xx while fetching `.maintenance.yaml` → WARN log; publish proceeds with watcher defaults (transient errors do not block publishes)
- [ ] `docs/build-watcher.md` documents the full hierarchical `.maintenance.yaml` schema including the `watcher.github-build` subtree, override precedence, file location, and forward-compatibility for additional service subtrees
- [ ] `bborbe/maintainer` repo carries a real `.maintenance.yaml` matching the watcher defaults — no behavior change but the loader path is exercised on every dev-cluster publish
- [ ] CHANGELOG entry under `## Unreleased`
- [ ] `make precommit` clean from `watcher/github-build/`

**No new scenario file.** Per `scenario-writing.md` four-condition test: the override behavior is fully exercisable via unit tests against the loader (mocked GH client) and integration tests against a counterfeit `GitHubClient` driving the full poll cycle. The Verification section's smoke test exists as informal manual verification, not a committed scenario.

## Verification

```bash
cd watcher/github-build && make precommit
```

Real-world smoke test (after deploy to dev):

```bash
# 1. Add .maintenance.yaml to bborbe/maintainer master:
echo 'assignee: go-deps-fixer-agent' > /tmp/.maintenance.yaml
gh api -X PUT /repos/bborbe/maintainer/contents/..maintenance.yaml \
  --input - <<EOF
{"message":"add .maintenance.yaml","content":"$(base64 < /tmp/.maintenance.yaml)"}
EOF

# 2. Wipe cursor + trigger watcher
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- rm /data/cursor.json
kubectlquant -n dev exec maintainer-watcher-github-build-0 -- wget -qO- http://localhost:9090/trigger

# 3. Verify the new vault task has assignee=go-deps-fixer-agent (not build-fixer-agent):
sleep 5 && grep "^assignee:" ~/Documents/Obsidian/OpenClaw/tasks/710db30e*.md
# expected: assignee: go-deps-fixer-agent

# 4. Cleanup: remove the file from the repo
gh api -X DELETE /repos/bborbe/maintainer/contents/..maintenance.yaml \
  -f message="remove .maintenance.yaml" -f sha=$(gh api /repos/bborbe/maintainer/contents/..maintenance.yaml --jq .sha)
```

## Do-Nothing Option

Leave per-repo routing as a deployment concern (run multiple watcher StatefulSets with disjoint allowlists, each with its own `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE`). Cost: operational complexity scales linearly with the number of routing variants. Five repos that each want a different fixer = five watchers. With this spec, one watcher serves all repos and each repo declares its own routing — same operator effort whether you have 5 routes or 50.

## Related

- Builds on: `configurable-task-frontmatter` (provides the watcher-level defaults this spec falls back to)
- Companion: `specs/ideas/build-fixer-agent.md` (the consumer that reads `assignee` and dispatches accordingly)
- Pattern source: `dependabot.yml`, `.golangci.yml`, `.goreleaser.yaml` — repo-rooted config files for cross-repo tooling
