# CLAUDE.md

Code-review agent pipeline — Pattern B Jobs consume PR tasks, post verdicts back.

## Dark Factory Workflow

The headline reason to use prompts/specs: **safe unattended execution** in a YOLO Claude container with permission checks disabled. Queue work, step away, no "approve this bash command?" interruptions. Documentation, decomposition (specs only), and token savings (Sonnet vs Opus) follow as side benefits.

### Choose a Flow

The decision is about **what artifact deserves to be committed alongside the change**, not size:

| Kind of change | Flow | What gets committed |
|----------------|------|---------------------|
| Doc / config / yaml — no code | **Direct** — edit + commit yourself | Just the diff |
| Code change of any size | **Prompt** — write a prompt, audit, approve, daemon executes | Prompt + diff (technical "how") |
| Feature delivering business value | **Spec → prompts** — write spec, audit, approve, daemon auto-generates prompts, audit each, approve, daemon executes | Spec + prompts + diff (business "why" + technical "how") |

Decide by: **Is code changing?** No → direct. Yes → prompt or spec. **Is there a business-level "why" that deserves its own document?** No → prompt. Yes → spec first.

The prompt/spec split is **business-why vs technical-how**, not big vs small. A 50-prompt mechanical refactor stays prompts. A 1-prompt user-visible feature may still warrant a spec.

### Complete Flow

**Direct (doc / config / yaml — no code):**

1. Edit + commit + push yourself. No dark-factory ceremony.

**Standalone prompts (code change of any size):**

1. Create prompt → `/dark-factory:create-prompt`
2. Audit prompt → `/dark-factory:audit-prompt`
3. User confirms → `dark-factory prompt approve <name>`
4. Start daemon → `dark-factory daemon` (use Bash `run_in_background: true`)
5. dark-factory executes prompt automatically

**Spec-based (feature delivering business value):**

1. Create spec → `/dark-factory:create-spec`
2. Audit spec → `/dark-factory:audit-spec`
3. User confirms → `dark-factory spec approve <name>`
4. dark-factory auto-generates prompts from spec
5. Audit prompts → `/dark-factory:audit-prompt`
6. User confirms → `dark-factory prompt approve <name>`
7. Start daemon → `dark-factory daemon` (use Bash `run_in_background: true`)
8. dark-factory executes prompts automatically

### Read the relevant guide before starting — every time, not from memory

- Writing a spec → read [[Dark Factory - Write Spec]] and [[Dark Factory Guide#Specs What Makes a Good Spec]]
- Writing prompts → read [[Dark Factory - Write Prompts]] and [[Dark Factory Guide#Prompts What Makes a Good Prompt]]
- Running prompts → read [[Dark Factory - Run Prompt]]

### Claude Code Commands

| Command | Purpose |
|---------|---------|
| `/dark-factory:create-spec` | Create a spec file interactively |
| `/dark-factory:create-prompt` | Create a prompt file from spec or task description |
| `/dark-factory:audit-spec` | Audit spec against preflight checklist |
| `/dark-factory:audit-prompt` | Audit prompt against Definition of Done |

### CLI Commands

| Command | Purpose |
|---------|---------|
| `dark-factory spec approve <name>` | Approve spec (inbox → queue, triggers prompt generation) |
| `dark-factory prompt approve <name>` | Approve prompt (inbox → queue) |
| `dark-factory daemon` | Start daemon (watches queue, executes prompts) |
| `dark-factory run` | One-shot mode (process all queued, then exit) |
| `dark-factory status` | Show combined status of prompts and specs |
| `dark-factory prompt list` | List all prompts with status |
| `dark-factory spec list` | List all specs with status |
| `dark-factory prompt retry` | Re-queue failed prompts for retry |
| `dark-factory prompt cancel <name>` | Cancel a running or queued prompt (never use `docker kill`) |

### Key rules

- Prompts go to **`prompts/`** (inbox) — never to `prompts/in-progress/` or `prompts/completed/`
- Specs go to **`specs/`** (inbox) — never to `specs/in-progress/` or `specs/completed/`
- Idea specs live under **`specs/ideas/`** — rough kernels, not part of the dark-factory pipeline; promote to `specs/` when ready
- Never number filenames — dark-factory assigns numbers on approve
- Never manually edit frontmatter status — use CLI commands above
- Always audit before approving (`/dark-factory:audit-prompt`, `/dark-factory:audit-spec`)
- **Spec-linked prompts are daemon-generated.** After `dark-factory spec approve`, the daemon spawns a `dark-factory-gen-<spec>` container that creates the prompts automatically. **Never hand-write prompts for an approved spec.** Hand-written prompts are only for standalone changes (no spec).
- **BLOCKING: Never run `dark-factory prompt approve`, `dark-factory spec approve`, or `dark-factory daemon` without explicit user confirmation.** Write the prompt/spec, then STOP and ask the user to approve.
- **Before starting daemon** — run `dark-factory status` first to check if one is already running. Only start if not running.
- **Start daemon in background** — use Bash tool with `run_in_background: true` (not foreground, not detached with `&`)

## Development Standards

This project follows the [coding-guidelines](https://github.com/bborbe/coding-guidelines).

### Key Reference Guides

- **go-architecture-patterns.md** — Interface → Constructor → Struct → Method
- **go-testing-guide.md** — Ginkgo v2/Gomega testing
- **go-makefile-commands.md** — Build commands

### Build and test

- `make precommit` — format + generate + test + lint + license
- `make test` — tests only
- **Run in service dir**, never at root for single-service edits. Root `make precommit` delegates via `Makefile.folder` (finds 2-level dirs like `agent/pr-reviewer`).

### Deploy (`make buca`)

- Always use `/make-buca <service-dir> <branch>` slash command (delegates to simple-bash-runner, concise output). Never raw `make buca`.
- Only `dev` or `prod` are valid branches. Never `master` / feature branches.
- Example: `/make-buca agent/pr-reviewer dev`

### Versioning and tags

- Single global `CHANGELOG.md` at repo root. No per-module CHANGELOG.
- Releases use `## Unreleased` on master; `/coding:commit` converts to `## vX.Y.Z` + tag.
- When `lib/` is extracted in the future, every release pairs two tags at the same commit (`vX.Y.Z` + `lib/vX.Y.Z`). Not yet — only root `vX.Y.Z` today.

### Test conventions

- Ginkgo/Gomega test framework
- Counterfeiter for mocks (`mocks/` dir)
- External test packages (`*_test`)

### Dependencies

- `github.com/bborbe/errors` — error handling
- `github.com/bborbe/agent/lib` — shared agent types, Claude runner, delivery
- `github.com/bborbe/cqrs/base` — Branch type
- `github.com/bborbe/kafka`, `github.com/bborbe/sentry`, `github.com/bborbe/service`, `github.com/bborbe/time` — platform
- `github.com/onsi/ginkgo/v2` / `github.com/onsi/gomega` — testing
- `github.com/maxbrunsfeld/counterfeiter/v6` — mock generation

## Architecture

Multi-module layout (one `go.mod` per service), modeled after `bborbe/agent`. Repo root has **no** `go.mod`.

```
code-reviewer/
├── .golangci.yml, .osv-scanner.toml, .trivyignore   shared tooling
├── Makefile, Makefile.folder, Makefile.*            root delegation
├── common.env, default.env, dev.env, prod.env       branch-based config
├── CHANGELOG.md                                     single global changelog
├── agent/pr-reviewer/                               Pattern B Job (current)
│   ├── go.mod
│   ├── main.go, main_test.go                       k8s job entry (TASK_CONTENT)
│   ├── pkg/factory/                                TaskRunner + Kafka deliverer
│   ├── pkg/prompts/                                embedded workflow + output-format
│   ├── agent/.claude/CLAUDE.md                     headless agent guardrails
│   ├── cmd/run-task/                               local test runner (task file)
│   ├── cmd/cli/                                    legacy direct-CLI reviewer
│   └── k8s/                                        Config CRD + secret + PVC + priority + quota
└── (future) watcher/github/, watcher/bitbucket/, agent/repo-review/
```

### Entry points

| Binary | Purpose |
|--------|---------|
| `agent/pr-reviewer/main.go` | K8s Job runner — reads `TASK_CONTENT`, runs Claude CLI, publishes verdict to Kafka (if `TASK_ID` set) |
| `agent/pr-reviewer/cmd/run-task/main.go` | Local test — reads task markdown file, writes result back into file |
| `agent/pr-reviewer/cmd/cli/main.go` | Legacy user-facing CLI — resolves PR URL → worktree → runs Claude → posts comment |

### Agent pattern

Pattern B Job (from `bborbe/agent`): stateless, one-shot, spawned per task by the task-executor.

- Reads task from `TASK_CONTENT` env var
- Runs Claude CLI with `--allowedTools` gate and embedded workflow/output-format prompt
- Returns JSON verdict on stdout (`done` / `needs_input` / `failed`)
- Optional Kafka result delivery when `TASK_ID` is set

Guardrails for the containerized agent live at `agent/pr-reviewer/agent/.claude/CLAUDE.md`.

## Key Design Decisions

- **Mirror `bborbe/agent/agent/claude` verbatim** — this service exists as a domain-specific copy of the generic claude agent. Do not reinvent; diff against claude first.
- **Multi-module mono-repo** — each service has its own `go.mod`, its own `make precommit`, its own Dockerfile + k8s manifests.
- **`lib/` + `review/` extraction deferred** — only when a second consumer (`watcher/`, `agent/repo-review/`) needs shared types.
- **Factory functions are pure composition** — no conditionals, no I/O, no `context.Background()`.
- **No vendor** — `go mod tidy` keeps deps direct; no vendor dir.
- **Verdict schema is the contract** — `{"status":"done|needs_input|failed","message":"..."}`. Other consumers depend on this; don't break it.
- **Container-local writes only** — agent writes nothing outside `/home/claude/.claude` PVC and the task file it was given.
