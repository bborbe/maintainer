---
status: committing
spec: [015-github-build-watcher-mvp]
summary: 'Wired all github-build watcher components: factory.go (CreateKafkaPublisher + CreateWatcher), full Run implementation in main.go (poll loop + HTTP server via gorilla/mux + libhttp), cmd/run-once single-cycle binary, ParseRepoAllowlist added to pkg/filter, CHANGELOG.md updated under Unreleased.'
container: maintainer-088-spec-015-factory-main
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T21:00:00Z"
queued: "2026-05-05T21:18:21Z"
started: "2026-05-05T21:42:01Z"
---

<summary>
- The build watcher factory composes the Kafka publisher, GitHub client, watcher, and metrics — pure wiring, no business logic
- The long-running binary's startup is replaced with the full implementation: validates the repo allowlist, parses the poll interval, runs the poll loop and HTTP endpoints concurrently, and shuts down cleanly on context cancel
- HTTP endpoints `/healthz`, `/readiness`, `/metrics`, `/trigger` mirror the PR watcher; `/trigger` runs a poll cycle in the background and returns 200 immediately (matches PR watcher pattern)
- A separate one-shot CLI binary runs a single poll cycle then exits — no HTTP server, no loop — for local smoke testing against a real repo
- The repo-allowlist parser lives next to the filter that uses it (mirroring the PR watcher's package layout)
- `go mod tidy` regenerates `go.sum` once all source files exist
- Factory wiring includes a contract test asserting the Kafka producer name is `maintainer-watcher-github-build`
- A CHANGELOG entry is added under `## Unreleased`
- `make precommit` passes in the new module
</summary>

<objective>
Wire all the components from prompts 1–3 into a running service. `main.go`'s `Run` gets its full implementation — poll loop, HTTP server, allowlist validation. The factory composes all deps (no business logic). `cmd/run-once` provides a deterministic single-cycle test runner per the spec's constraint.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read fully before making any changes — these are canonical patterns to mirror:
- `watcher/github-pr/pkg/factory/factory.go` — `CreateKafkaPublisher(ctx, brokers, branch) (CommandPublisher, cleanup, error)` using `cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)` and `pkg.NewCommandPublisher(ctx, sender)`. `CreateWatcher` calls `pkg.NewGitHubClient(ghToken)` (token, NOT an http.Client)
- `watcher/github-pr/main.go` — full file. Canonical patterns to mirror: `Run(ctx, _ libsentry.Client) error`; `time.ParseDuration` for `POLL_INTERVAL`; `run.CancelOnFirstFinish(ctx, runPollLoop, runHTTPServer)`; HTTP server uses `github.com/gorilla/mux` + `github.com/bborbe/http` (`libhttp.NewPrintHandler`, `libhttp.NewBackgroundRunHandler`, `libhttp.NewServer(addr, router).Run(ctx)`); `/trigger` uses `libhttp.NewBackgroundRunHandler(ctx, poll)` (async — returns 200 immediately)
- `watcher/github-pr/pkg/filter/repo_allowlist_filter.go` — `ParseRepoAllowlist(ctx, raw) ([]string, error)` lives in `pkg/filter/`, validates `host/owner/repo` shape; `NewRepoAllowlistFilter(allowlist) TaskCreationFilter`
- `watcher/github-build/main.go` — current stub; `Run` is what you replace
- `watcher/github-build/pkg/watcher.go` — `NewWatcher` constructor signature
- `watcher/github-build/pkg/publisher.go` — `NewCommandPublisher(ctx, sender)` constructor
- `watcher/github-build/pkg/githubclient.go` — `NewGitHubClient(token string)` constructor
- `watcher/github-build/pkg/filter/repo_allowlist_filter.go` — created in prompt 3 (mirrors PR watcher)

**Symbol verification before writing factory code:**
```bash
# run.CancelOnFirstFinish:
grep -rn "func CancelOnFirstFinish" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/run@*/... 2>/dev/null | head -5

# libkafka.NewSyncProducerWithName:
grep -rn "func NewSyncProducerWithName" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/kafka@*/... 2>/dev/null | head -5

# cdb.NewCommandObjectSender:
grep -rn "func NewCommandObjectSender" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/cqrs@*/... 2>/dev/null | head -5

# libhttp helpers used by PR watcher:
grep -n "libhttp\." watcher/github-pr/main.go
```
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Create `watcher/github-build/pkg/factory/factory.go`** — mirror `watcher/github-pr/pkg/factory/factory.go` nearly verbatim. The factory has zero business logic (no conditionals except `if err != nil { return ..., err }`).

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package factory wires concrete dependencies for the maintainer-watcher-github-build binary.
   package factory

   import (
   	"context"

   	"github.com/bborbe/cqrs/base"
   	"github.com/bborbe/cqrs/cdb"
   	"github.com/bborbe/errors"
   	libkafka "github.com/bborbe/kafka"
   	"github.com/bborbe/log"
   	"github.com/golang/glog"

   	"github.com/bborbe/maintainer/watcher/github-build/pkg"
   	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
   )

   // CreateKafkaPublisher constructs a CommandPublisher backed by a Kafka sync producer.
   // The cleanup function closes the underlying sync producer on shutdown.
   func CreateKafkaPublisher(
   	ctx context.Context,
   	brokers libkafka.Brokers,
   	branch base.Branch,
   ) (pkg.CommandPublisher, func(), error) {
   	syncProducer, err := libkafka.NewSyncProducerWithName(ctx, brokers, "maintainer-watcher-github-build")
   	if err != nil {
   		return nil, nil, errors.Wrap(ctx, err, "create sync producer")
   	}
   	sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
   	cleanup := func() {
   		if err := syncProducer.Close(); err != nil {
   			glog.Warningf("close kafka sync producer: %v", err)
   		}
   	}
   	return pkg.NewCommandPublisher(ctx, sender), cleanup, nil
   }

   // CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
   func CreateWatcher(
   	ctx context.Context,
   	ghToken string,
   	brokers libkafka.Brokers,
   	stage string,
   	allowlist []string,
   	cursorPath string,
   ) (pkg.Watcher, func(), error) {
   	branch := base.Branch(stage)
   	pub, cleanup, err := CreateKafkaPublisher(ctx, brokers, branch)
   	if err != nil {
   		return nil, nil, errors.Wrap(ctx, err, "create kafka publisher")
   	}
   	ghClient := pkg.NewGitHubClient(ghToken)
   	repoFilter := filter.RepoFilters{filter.NewRepoAllowlistFilter(allowlist)}
   	w := pkg.NewWatcher(
   		ghClient,
   		pub,
   		pkg.NewMetrics(),
   		repoFilter,
   		allowlist,
   		cursorPath,
   	)
   	return w, cleanup, nil
   }
   ```

2. **Create `watcher/github-build/pkg/factory/factory_test.go`** — boundary contract test. Without spinning a real Kafka cluster, assert at minimum:
   - `CreateKafkaPublisher` accepts the `libkafka.NewSyncProducerWithName` call with producer name `"maintainer-watcher-github-build"` (parameterize the test with a fake `Brokers` of `nil` and assert it returns a wrapped error rather than panic; OR use a counterfeit `SyncProducer` if the helper exposes a hook)
   - The producer-name string literal is present in the source file (`grep`-style assertion via `runtime.Func` is unnecessary; a static assertion via `//go:embed` of the file is unnecessary; instead, mirror whatever testing pattern exists in `watcher/github-pr/pkg/factory/`)

   If `watcher/github-pr/pkg/factory/` has no test, skip step 2 and rely on the verification grep below.

3. **Replace the stub `Run` in `watcher/github-build/main.go`** with the full implementation. Mirror `watcher/github-pr/main.go` lines 101–217 (the `Run` method + `pollOnce`/`runPollLoop`/`runHTTPServer` helpers) almost verbatim. The build watcher Run differs only in:
   - No `validateRepoScope`, `parseMaxPRAge`, `parseBackfillDuration` calls (those fields don't exist)
   - No `botAllowlist` / `trustedAuthors` / `taskCreationFilter` chain (those concepts don't exist for builds)
   - `REPO_ALLOWLIST` MUST be non-empty (build watcher refuses startup without a list); use `filter.ParseRepoAllowlist` and add an explicit `len == 0` check returning an error
   - `factory.CreateWatcher` call passes `(ctx, ghToken, kafkaBrokers, stage, repoAllowlist, "/data/cursor.json")`
   - HTTP server registration is identical to PR watcher: `gorilla/mux`, `libhttp.NewPrintHandler` for `/healthz` + `/readiness`, `promhttp.Handler()` for `/metrics`, `libhttp.NewBackgroundRunHandler(ctx, poll)` for `/trigger`, `libhttp.NewServer(a.Listen, router).Run(ctx)`

   Required `Run` signature (matches PR watcher exactly):
   ```go
   func (a *application) Run(ctx context.Context, _ libsentry.Client) error
   ```

   Add the helper methods `pollOnce(w pkg.Watcher) run.Func`, `runPollLoop(poll run.Func, interval time.Duration) run.Func`, and `runHTTPServer(poll run.Func) run.Func` mirroring PR watcher.

4. **Create `watcher/github-build/cmd/run-once/main.go`** — one-shot binary: no poll loop, no HTTP server. Runs a single `Poll` and exits.

   Env vars: `GH_TOKEN`, `KAFKA_BROKERS`, `STAGE`, `REPO_ALLOWLIST` only (no `LISTEN`, no `POLL_INTERVAL`). Mirror PR watcher's `Run` signature: `Run(ctx, _ libsentry.Client) error`. Body:
   - Parse + validate allowlist
   - Call `factory.CreateWatcher`
   - Call `watcher.Poll(ctx)` once
   - Return its error (no loop, no HTTP server)

5. **Create `watcher/github-build/cmd/run-once/main_test.go`** — Ginkgo suite stub matching `watcher/github-pr/cmd/.../main_test.go` if a similar file exists; otherwise omit.

6. **Run `go mod tidy`** in `watcher/github-build/`:
   ```bash
   cd watcher/github-build && go mod tidy
   ```
   Generates `go.sum`. If it fails due to missing deps, add them to the `require` block.

7. **Add CHANGELOG entry** to the root `CHANGELOG.md`. Append under the existing `## Unreleased` section (or create one at the top of the version list):
   ```markdown
   - feat(watcher/github-build): new service polls GitHub Actions API for failed CI workflow runs on default branches; publishes `CreateTaskCommand` to Kafka on `green → red` transitions with deterministic UUID5 task ID (`assignee: build-fixer-agent`); re-polls are idempotent (same episode SHA = same task ID); `red → green` clears state without publishing closure (follow-up spec)
   ```

8. **Run `make precommit`** in `watcher/github-build/`:
   ```bash
   cd watcher/github-build && make precommit
   ```

9. **Coverage check**: ensure new code (factory, allowlist parsing, run-once) has tests. Re-run `make precommit` until coverage gate passes. Failing-coverage acceptable threshold: match whatever the PR watcher's threshold is.
</requirements>

<constraints>
- Only edit files under `watcher/github-build/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- Factory functions MUST have zero business logic (no conditionals except `if err != nil { return ..., err }`)
- `Run` signature MUST be `Run(ctx context.Context, _ libsentry.Client) error` (mirror PR watcher; required by `service.Application` interface)
- HTTP server MUST use `gorilla/mux` + `libhttp` helpers (`NewPrintHandler`, `NewBackgroundRunHandler`, `NewServer`) — NOT stdlib `http.ServeMux`
- `/trigger` handler MUST be `libhttp.NewBackgroundRunHandler(ctx, poll)` — async, returns 200 immediately (matches PR watcher; the spec's "synchronous" claim was wrong)
- `REPO_ALLOWLIST` MUST be validated non-empty before creating the watcher; `Run` returns an error if empty
- `ParseRepoAllowlist` and `NewRepoAllowlistFilter` MUST live in `watcher/github-build/pkg/filter/` (NOT in `watcher/github-build/pkg/`) — mirror PR watcher's package layout
- `cmd/run-once` MUST NOT start an HTTP server or poll loop — single Poll then exit
- Kafka producer name MUST be `"maintainer-watcher-github-build"` (passed to `libkafka.NewSyncProducerWithName`)
- Error wrapping uses `github.com/bborbe/errors`; never `fmt.Errorf`
- `runPollLoop` MUST use `time.NewTicker` + `select { case <-ctx.Done(): ... case <-ticker.C: ... }` — mirror PR watcher; do NOT use `time.Sleep`
- `run.CancelOnFirstFinish` is the pattern for composing goroutines — NO raw `go func()`
- New code MUST have tests; `make precommit` enforces the coverage gate
- `make precommit` runs from `watcher/github-build/`, never at repo root
- All tests from prompts 1–3 must still pass
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm factory file exists with zero-logic functions:
ls watcher/github-build/pkg/factory/factory.go
# All "if" lines should be err-propagation only:
grep -n -E "^\s*(if|for|switch)\b" watcher/github-build/pkg/factory/factory.go | grep -v "err != nil"
# Expected: zero matches (all conditionals are err propagation)

# Confirm Kafka producer name:
grep -n "maintainer-watcher-github-build" watcher/github-build/pkg/factory/factory.go
# Expected: at least one match — the libkafka.NewSyncProducerWithName call

# Confirm Run signature mirrors PR watcher:
grep -n "Run(ctx context.Context" watcher/github-build/main.go
# Expected: Run(ctx context.Context, _ libsentry.Client) error

# Confirm HTTP server uses gorilla/mux + libhttp (NOT stdlib ServeMux):
grep -n "mux.NewRouter\|libhttp.NewServer\|libhttp.NewPrintHandler\|libhttp.NewBackgroundRunHandler" watcher/github-build/main.go
# Expected: at least 3 matches

grep -n "http.ServeMux\|http.NewServeMux" watcher/github-build/main.go
# Expected: zero matches

# Confirm /trigger uses BackgroundRunHandler (async):
grep -n "trigger.*BackgroundRunHandler\|BackgroundRunHandler.*trigger" watcher/github-build/main.go
# Expected: at least one match

# Confirm allowlist parser lives in filter package:
ls watcher/github-build/pkg/filter/repo_allowlist_filter.go
# Expected: file exists
test ! -f watcher/github-build/pkg/allowlist.go && echo "OK: no top-level allowlist.go"

# Confirm REPO_ALLOWLIST non-empty check:
grep -n "REPO_ALLOWLIST.*non-empty\|len(repoAllowlist) == 0" watcher/github-build/main.go
# Expected: at least one match

# Confirm cmd/run-once exists and has no HTTP server:
ls watcher/github-build/cmd/run-once/main.go
grep -n "http.\|Listen\|Serve\|ticker\|time.Sleep" watcher/github-build/cmd/run-once/main.go
# Expected: zero matches (no HTTP, no ticker, no sleep)

# Confirm go.sum was generated:
ls watcher/github-build/go.sum

# Confirm CHANGELOG entry:
grep -n "watcher/github-build\|build-fixer-agent" CHANGELOG.md
</verification>
