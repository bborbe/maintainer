---
status: committing
spec: [015-github-build-watcher-mvp]
summary: Created watcher/github-build/ module scaffold with go.mod, Makefile, Dockerfile, pkg/doc.go, main.go (stub Run), and main_test.go; make precommit passes clean.
container: maintainer-085-spec-015-scaffold
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T21:00:00Z"
queued: "2026-05-05T21:18:21Z"
started: "2026-05-05T21:19:23Z"
---

<summary>
- New Go module under `watcher/github-build/` mirrors the existing PR watcher layout
- Build tooling (Makefile, Dockerfile, go.mod) is copied from the PR watcher with only the service name changed
- The binary's startup struct declares the env-var schema for the build watcher: GitHub token, Kafka brokers, deployment stage, HTTP listen, poll interval, mandatory repo allowlist, Sentry DSN, Sentry proxy
- The startup `Run` is a stub that returns nil — full implementation lands in a later prompt
- Package documentation explains the service: poll GitHub Actions, derive green/red state per repo, publish a Kafka task per green→red transition
- `make precommit` passes in the new module
</summary>

<objective>
Create the `watcher/github-build/` module skeleton so subsequent prompts can add implementation incrementally. The scaffold establishes the module path, Go version, dependency list, build tooling, and the env-var schema in `main.go` — nothing more. The `Run` method is a stub returning nil.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read before making any changes:
- `watcher/github-pr/go.mod` — copy module path pattern + full require block verbatim
- `watcher/github-pr/Makefile` — mirror exactly, changing only SERVICE
- `watcher/github-pr/Dockerfile` — mirror exactly
- `watcher/github-pr/main.go` — canonical pattern: `application` struct, `service.Main(ctx, app, &app.SentryDSN, &app.SentryProxy)`, `Run(ctx, _ libsentry.Client) error` signature
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

   Do NOT run `go mod tidy` yet; `go.sum` is regenerated in prompt 4 once all source files exist. Copy `watcher/github-pr/go.sum` verbatim to `watcher/github-build/go.sum` so the stub compiles.

3. **Create `watcher/github-build/Makefile`** — read `watcher/github-pr/Makefile` and copy it exactly. Change only:
   ```
   SERVICE = maintainer-watcher-github-build
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

6. **Create `watcher/github-build/main.go`** — mirror `watcher/github-pr/main.go` skeleton EXACTLY for `main()`, the `application` struct field shape (struct tags style), and the `Run` signature. The build watcher differs only in field set and Run body.

   Required differences from PR watcher's `application` struct:
   - REMOVE: `RepoScope`, `TrustedAuthors`, `BotAllowlist`, `MaxPRAge`, `BackfillDuration`
   - KEEP: `SentryDSN`, `SentryProxy`, `Listen`, `GHToken`, `KafkaBrokers`, `Stage`, `PollInterval`, `RepoAllowlist`
   - CHANGE: `RepoAllowlist` becomes `required:"true"` (build watcher refuses startup without a list)
   - `KafkaBrokers` MUST be typed `libkafka.Brokers` (NOT `string`) — match PR watcher

   The exact stub:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Command maintainer-watcher-github-build polls GitHub Actions for failed
   // workflow runs on the default branches of configured repos and publishes
   // a CreateTaskCommand per green→red transition so a build-fixer agent can
   // pick it up.
   package main

   import (
   	"context"
   	"os"

   	libkafka "github.com/bborbe/kafka"
   	libsentry "github.com/bborbe/sentry"
   	"github.com/bborbe/service"
   	"github.com/golang/glog"
   )

   func main() {
   	app := &application{}
   	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
   }

   type application struct {
   	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
   	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

   	Listen        string           `required:"false" arg:"listen"         env:"LISTEN"         usage:"HTTP listen address (healthz/readiness/metrics/trigger)" default:":9090"`
   	GHToken       string           `required:"true"  arg:"gh-token"       env:"GH_TOKEN"       usage:"GitHub token (read scope sufficient)"                                                                                  display:"length"`
   	KafkaBrokers  libkafka.Brokers `required:"true"  arg:"kafka-brokers"  env:"KAFKA_BROKERS"  usage:"Comma-separated Kafka broker list"`
   	Stage         string           `required:"true"  arg:"stage"          env:"STAGE"          usage:"Deployment stage (dev|prod)"`
   	PollInterval  string           `required:"false" arg:"poll-interval"  env:"POLL_INTERVAL"  usage:"Poll interval (Go duration)"                              default:"5m"`
   	RepoAllowlist string           `required:"true"  arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); MUST be non-empty"`
   }

   func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
   	glog.V(2).Infof("maintainer-watcher-github-build starting — stub Run; full implementation in prompt 4")
   	return nil
   }
   ```

7. **Create `watcher/github-build/main_test.go`** — read `watcher/github-pr/main_test.go` and mirror the Ginkgo suite setup for package `main_test`. Use description "Main Suite".

8. **Do NOT create `validate_test.go`.** PR watcher's `validate_test.go` exercises `parseMaxPRAge`/`parseBackfillDuration`/`validateRepoScope` — none of these functions exist in the build watcher's stub (and are intentionally absent per spec). Skip the file; future prompts may add validation tests for build-watcher-specific helpers.

9. **Run `make precommit`** from `watcher/github-build/`:
   ```bash
   cd watcher/github-build && make precommit
   ```

   If lint flags `glog`/`libsentry`/`libkafka` as unused (the stub uses all three), the imports above are correct. If `funlen` or another linter flags the stub, fix the stub minimally. Goal: clean precommit with just the skeleton.
</requirements>

<constraints>
- Only create files under `watcher/github-build/` — do NOT touch any other service directory or CHANGELOG.md
- Do NOT commit — dark-factory handles git
- Module name MUST be `github.com/bborbe/maintainer/watcher/github-build`
- `SERVICE` in Makefile MUST be `maintainer-watcher-github-build` (image name)
- `Run` MUST be a no-op stub — full implementation is prompt 4
- `Run` signature MUST be `Run(ctx context.Context, _ libsentry.Client) error` — matches PR watcher's `service.Application` interface
- `main()` MUST be `os.Exit(service.Main(ctx, app, &app.SentryDSN, &app.SentryProxy))` — matches PR watcher
- `KafkaBrokers` MUST be typed `libkafka.Brokers` — matches PR watcher
- `RepoAllowlist` MUST be `required:"true"` (the build watcher refuses startup without a list; differs from PR watcher)
- No `RepoScope`, `TrustedAuthors`, `BotAllowlist`, `MaxPRAge`, or `BackfillDuration` fields
- Mirror `watcher/github-pr/` build tooling (Makefile, Dockerfile) changing only `SERVICE`
- Copy `go.sum` verbatim from PR watcher; do NOT run `go mod tidy`
- Do NOT create `validate_test.go` — none of the PR watcher's helpers apply
- `make precommit` runs from `watcher/github-build/`, never at repo root
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm module name:
head -3 watcher/github-build/go.mod
# Expected: module github.com/bborbe/maintainer/watcher/github-build

# Confirm SERVICE in Makefile:
grep "^SERVICE" watcher/github-build/Makefile
# Expected: SERVICE = maintainer-watcher-github-build

# Confirm Run signature + service.Main mirror PR watcher:
grep -E "service.Main|libsentry.Client" watcher/github-build/main.go
# Expected: os.Exit(service.Main(... &app.SentryDSN, &app.SentryProxy)) AND Run(ctx context.Context, _ libsentry.Client) error

# Confirm KafkaBrokers is typed:
grep "KafkaBrokers" watcher/github-build/main.go
# Expected: KafkaBrokers libkafka.Brokers ...

# Confirm RepoAllowlist is required + GHToken displays length:
grep "RepoAllowlist\|GHToken" watcher/github-build/main.go
# Expected: required:"true" for RepoAllowlist; display:"length" for GHToken

# Confirm pkg/doc.go exists:
ls watcher/github-build/pkg/doc.go

# Confirm validate_test.go was NOT created:
test ! -f watcher/github-build/validate_test.go && echo OK
</verification>
