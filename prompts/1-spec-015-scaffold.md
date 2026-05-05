---
spec: ["015"]
status: draft
created: "2026-05-05T21:00:00Z"
---

<summary>
- New Go module `watcher/github-build/` is created at `github.com/bborbe/maintainer/watcher/github-build`, mirroring `watcher/github-pr/` layout
- `go.mod` carries the same dependencies as `watcher/github-pr/go.mod` — deps are needed by subsequent prompts
- `Makefile` mirrors `watcher/github-pr/Makefile` with `SERVICE = github-build-watcher`
- `Dockerfile` mirrors `watcher/github-pr/Dockerfile` unchanged
- `main.go` defines the full `application` struct with all env-var fields for the build watcher: `GH_TOKEN`, `KAFKA_BROKERS`, `STAGE`, `LISTEN`, `POLL_INTERVAL`, `REPO_ALLOWLIST` (required), `SENTRY_DSN`; `Run()` is a no-op stub
- `pkg/doc.go` documents the package purpose
- `main_test.go` and `validate_test.go` mirror the PR watcher equivalents
- `make precommit` passes in `watcher/github-build/`
</summary>

<objective>
Create the `watcher/github-build/` module skeleton so subsequent prompts can add implementation incrementally. The scaffold establishes the module path, Go version, dependency list, build tooling, and the env-var schema in `main.go` — nothing more. The `Run()` method is a stub returning nil.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-precommit.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read before making any changes:
- `watcher/github-pr/go.mod` — copy module path pattern + full require block verbatim
- `watcher/github-pr/Makefile` — mirror exactly, changing only SERVICE
- `watcher/github-pr/Dockerfile` — mirror exactly
- `watcher/github-pr/main.go` — understand `application` struct + `service.Main()` usage; the build watcher's application struct differs (see requirements)
- `watcher/github-pr/validate_test.go` — mirror exactly
- `watcher/github-pr/main_test.go` — mirror suite setup structure
- `watcher/github-pr/pkg/doc.go` — mirror documentation style
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Verify the parent directory exists:**
   ```bash
   ls watcher/
   ```

2. **Create `watcher/github-build/go.mod`** — read `watcher/github-pr/go.mod` fully. Create a mirrored file with:
   - Module name: `github.com/bborbe/maintainer/watcher/github-build`
   - Same `go` version directive as `watcher/github-pr/go.mod`
   - Same full `require` block copied verbatim (all direct and indirect deps)

   Do NOT run `go mod tidy` yet; `go.sum` will be regenerated in prompt 4 once all source files exist.

3. **Create `watcher/github-build/Makefile`** — read `watcher/github-pr/Makefile` and copy it exactly. Change only:
   ```
   SERVICE = github-build-watcher
   ```

4. **Create `watcher/github-build/Dockerfile`** — copy `watcher/github-pr/Dockerfile` verbatim. Do not change Go version, base image, or build flags.

5. **Create `watcher/github-build/pkg/doc.go`**:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package pkg implements the GitHub build watcher service.
   // It polls the GitHub Actions API for failed CI workflow runs on default
   // branches, derives a per-repo green/red state, and publishes
   // CreateTaskCommand to Kafka on green → red transitions.
   package pkg
   ```

6. **Create `watcher/github-build/main.go`** with the full `application` struct (all env vars) and a no-op `Run()`:

   Read `watcher/github-pr/main.go` fully for the exact pattern. The build watcher differs:
   - No `RepoScope`, `TrustedAuthors`, `BotAllowlist`, `MaxPRAge`, `BackfillDuration` fields
   - `RepoAllowlist` is `required:"true"` (startup fails if empty — unlike PR watcher where it is optional allow-all)
   - No `SentryDSN` field is needed but match whatever the PR watcher exposes for Sentry wiring

   Minimum stub that compiles and satisfies the `service.Application` interface:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package main

   import (
   	"context"

   	"github.com/golang/glog"
   	"github.com/bborbe/service"
   )

   func main() {
   	service.Main(context.Background(), &application{})
   }

   type application struct {
   	Listen        string `required:"true"  arg:"listen"         env:"LISTEN"         usage:"HTTP listen address"                                           default:":9090"`
   	GHToken       string `required:"true"  arg:"gh-token"       env:"GH_TOKEN"       usage:"GitHub personal access token (read:repo scope)"                display:"length"`
   	KafkaBrokers  string `required:"true"  arg:"kafka-brokers"  env:"KAFKA_BROKERS"  usage:"Comma-separated Kafka broker addresses"`
   	Stage         string `required:"true"  arg:"stage"          env:"STAGE"          usage:"Deployment stage (dev|prod)"`
   	PollInterval  string `required:"false" arg:"poll-interval"  env:"POLL_INTERVAL"  usage:"How often to poll GitHub Actions API"                          default:"5m"`
   	RepoAllowlist string `required:"true"  arg:"repo-allowlist" env:"REPO_ALLOWLIST"  usage:"Comma-separated host/owner/repo entries to watch (non-empty)"`
   	SentryDSN     string `required:"false" arg:"sentry-dsn"     env:"SENTRY_DSN"     usage:"Sentry DSN for error reporting"`
   }

   func (a *application) Run(ctx context.Context) error {
   	glog.V(2).Infof("github-build-watcher starting — stub Run(); full implementation in prompt 4")
   	return nil
   }
   ```

   **Note:** The real `Run()` (poll loop + HTTP server) is implemented in prompt 4. The stub lets the module compile and pass `make precommit` now.

7. **Create `watcher/github-build/main_test.go`** — read `watcher/github-pr/main_test.go` and mirror the Ginkgo suite setup for package `main_test`. Use the service name "Main Suite" as description.

8. **Create `watcher/github-build/validate_test.go`** — read `watcher/github-pr/validate_test.go` and copy it exactly, adjusting the import path to `github.com/bborbe/maintainer/watcher/github-build` if the file has any hardcoded paths. The purpose of `validate_test.go` is to confirm that `application` satisfies the `service.Application` interface at compile time (or similar static assertion); mirror whatever the PR watcher has.

9. **Run `make precommit`** from `watcher/github-build/`:
   ```bash
   cd watcher/github-build && make precommit
   ```

   If lint fails on the stub `Run()` (e.g. glog import unused, or funlen), adjust the stub. Goal: clean precommit with just the skeleton.

   Common fix if `glog` is flagged as unused: call `_ = a.GHToken` or add a minimal `glog.V(2).Infof(...)` line (as shown in step 6).
</requirements>

<constraints>
- Only create files under `watcher/github-build/` — do NOT touch any other service directory or CHANGELOG.md
- Do NOT commit — dark-factory handles git
- Module name MUST be `github.com/bborbe/maintainer/watcher/github-build`
- `SERVICE` in Makefile MUST be `github-build-watcher`
- `Run()` MUST be a no-op stub — full implementation is prompt 4
- `REPO_ALLOWLIST` MUST be `required:"true"` (the build watcher refuses startup without a list; this differs from the PR watcher)
- No `RepoScope`, `TrustedAuthors`, `BotAllowlist`, `MaxPRAge`, or `BackfillDuration` fields
- Mirror `watcher/github-pr/` build tooling (Makefile, Dockerfile) changing only what is specific to the build watcher
- Do NOT run `go mod tidy` — the go.sum is generated in prompt 4 once all packages exist
- `make precommit` runs from `watcher/github-build/`, never at repo root
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm module name:
head -3 watcher/github-build/go.mod
# Expected: module github.com/bborbe/maintainer/watcher/github-build

# Confirm SERVICE in Makefile:
grep "SERVICE" watcher/github-build/Makefile
# Expected: SERVICE = github-build-watcher

# Confirm REPO_ALLOWLIST is required and GH_TOKEN has display:length:
grep "RepoAllowlist\|GHToken" watcher/github-build/main.go
# Expected: required:"true" for RepoAllowlist; display:"length" for GHToken

# Confirm pkg/doc.go exists:
ls watcher/github-build/pkg/doc.go
</verification>
