---
status: completed
summary: Added cmd/run-once smoke-test binary to github-release watcher with Poll-once semantics, mirroring github-build structure
container: maintainer-github-release-exec-194-add-run-once-binary
dark-factory-version: v0.173.0
created: "2026-05-28T00:00:00Z"
queued: "2026-05-28T05:41:42Z"
started: "2026-05-28T05:41:43Z"
completed: "2026-05-28T06:00:37Z"
---

<summary>
- Add a `cmd/run-once` smoke-test binary to the github-release watcher
- Runs exactly one `Watcher.Poll` cycle against real dev Kafka, prints the publish outcome, then exits
- Mirrors `watcher/github-build/cmd/run-once/` verbatim with github-release-specific env + factory wiring
- Required for the project's rung-1 verification per `docs/verifying-specs.md` — without `run-once`, spec 044 cannot pass formal verification
- Adds `make run-once` target so operator can fire a single cycle without spinning up the long-running daemon binary
- No HTTP server, no scheduled re-poll, no cursor PVC needed — the binary owns its lifetime
</summary>

<objective>
Give the github-release watcher a one-shot smoke-test entry point so operator + spec-verifier can fire a single poll cycle against real GitHub + dev Kafka without deploying the StatefulSet, completing the project's required rung-1 verification surface.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions (errors.Wrapf, glog, no fmt.Errorf in production, external `_test` packages, Ginkgo v2).

Read the canonical reference — copy structure verbatim, change only github-release-specific imports + env + factory wiring:
- `/workspace/watcher/github-build/cmd/run-once/main.go` — Application struct, `NewApplication`, `service.Main` wrapper, env binding, `Run(ctx, sentry.Client)` body, single Poll + exit. The shape is intentionally identical across watchers; do not invent a different structure.
- `/workspace/watcher/github-build/cmd/run-once/main_test.go` — Ginkgo test using counterfeiter `mocks.Watcher`, `CreateWatcher` factory hook on `Application`, table-driven error-case coverage.
- `/workspace/watcher/github-build/cmd/run-once/Makefile` — `run-once:` target with concrete `go run` invocation, ARG-overridable env defaults.

Read the existing github-release watcher surfaces that run-once will wire:
- `/workspace/watcher/github-release/main.go` — env struct + `resolveAuth` + Kafka setup (one-shot reuses the same env + auth; difference is no `pollLoop`, no HTTP server, no ticker).
- `/workspace/watcher/github-release/pkg/factory/factory.go` — `CreateWatcher` factory signature (httpClient, createSender, cursorPath, owner, taskCreationFilter, stage, metrics, allowlist).
- `/workspace/watcher/github-release/pkg/watcher.go` — `Watcher` interface + `Poll(ctx)` method to invoke exactly once.
- `/workspace/watcher/github-release/pkg/mocks/watcher.go` — counterfeiter `Watcher` mock for the unit test.

Project convention: a watcher's `cmd/run-once` is the project's canonical rung-1 verification mechanism per `/workspace/docs/verifying-specs.md`. Without it, the spec-verifier ladder cannot fire — `dark-factory spec complete` will not legitimately follow.
</context>

<requirements>

**Execute in order. Run `cd watcher/github-release && make test` after step 3. Run `make precommit` only at the final step.**

1. **Create directory `watcher/github-release/cmd/run-once/`**.

2. **Write `watcher/github-release/cmd/run-once/main.go`** mirroring the structure of `/workspace/watcher/github-build/cmd/run-once/main.go`:
   - Package `main`. Standard copyright header.
   - `func main()` invokes `service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy)` — the `context.Background()` here is the same allowed exception documented in spec 044 AC #10 (canonical service-entry-point pattern, identical to `watcher/github-pr/main.go` and `watcher/github-build/main.go`).
   - `NewApplication()` constructor returning `*Application` with `CreateWatcher: factory.CreateWatcher` default — same indirection-for-testability pattern as github-build.
   - `Application` struct with the SAME env fields as `watcher/github-release/main.go` (Sentry, Stage, Owner, RepoAllowlist, KafkaBrokers, AppID, InstallationID, PEMKey, GHToken, CursorPath). Drop fields the one-shot binary cannot use: `Listen` (no HTTP server), `PollInterval` (no ticker).
   - `CreateWatcher WatcherFactory` field (typedef the factory function signature as a local `WatcherFactory` type so tests can swap in a mock-returning constructor — see github-build's pattern).
   - `Run(ctx context.Context, _ libsentry.Client) error` body:
     - Parse + validate env (RepoAllowlist via `filter.ParseRepoAllowlist`).
     - Resolve auth. **Heuristic:** inline the same flow with an explicit `// keep in sync with watcher/github-release/main.go resolveAuth` comment. Do NOT extract to a shared helper now — extraction only justified once a third caller appears (YAGNI). The two-caller duplication is tolerable + matches the github-build convention (run-once + main both have their own resolveAuth at parity).
     - Create Kafka sync producer + sender (`factory.CreateKafkaSender`).
     - Build `staticFilters` identical to `watcher/github-release/main.go` (RepoAllowlist + EmptyUnreleased + AutoRelease).
     - Call `a.CreateWatcher(httpClient, createSender, a.CursorPath, a.Owner, staticFilters, a.Stage, metrics, allowlist)`.
     - Invoke `w.Poll(ctx)` exactly once. Return its error.
     - `defer syncProducer.Close()` with warning log on close error.

3. **Write `watcher/github-release/cmd/run-once/main_test.go`** mirroring `/workspace/watcher/github-build/cmd/run-once/main_test.go`:
   - `package main_test` (external test).
   - `Describe("Run", ...)` block. Use **discrete `It` blocks** (NOT `DescribeTable`) so the named `It` strings appear verbatim in `make test` output and match the verification step below. (DescribeTable would emit `error cases <entry>` composites which are harder to grep for.)
   - Three `It` blocks with these exact names:
     - `It("Poll succeeds returns nil")` — mockWatcher.PollReturns(nil) → expect nil error.
     - `It("Poll fails returns wrapped error")` — mockWatcher.PollReturns(stderrors.New("kafka unavailable")) → expect non-nil error.
     - `It("empty REPO_ALLOWLIST returns error")` — app.RepoAllowlist = "" → expect non-nil error.
   - Setup uses `mocks.Watcher` + sets `app.CreateWatcher` to a closure returning the mock.
   - Use `stderrors "errors"` alias matching the convention in `/workspace/watcher/github-build/pkg/watcher_test.go:9`.
   - Use Ginkgo v2 + Gomega per `coding-guidelines/go-testing-guide.md`; never stdlib `t.Run` table loops.

4. **Write `watcher/github-release/cmd/run-once/Makefile`** mirroring `/workspace/watcher/github-build/cmd/run-once/Makefile`:
   - `run-once:` target invoking `go run . -alsologtostderr -v=2` with ARG-overridable env (`STAGE`, `OWNER`, `REPO_ALLOWLIST`, `KAFKA_BROKERS`, `CURSOR_PATH`).
   - Defaults that match dev Kafka NodePort (look up the value in github-build's Makefile — same Kafka cluster).

5. **Verify build**: `cd watcher/github-release && go build ./...` exits 0.

6. **Verify test**: `cd watcher/github-release && make test` — new run-once tests pass; existing tests unchanged.

7. **Run `make precommit`** — all stages clean (license header, lint, vet, tests). If the addlicense step modifies any generated mocks, that is expected — commit those alongside.

</requirements>

<constraints>
- Mirror `watcher/github-build/cmd/run-once/` structure verbatim. Do not invent a new shape — operator muscle memory + spec-verifier rung-1 procedure expect the same surface.
- One-shot semantics: NO ticker, NO HTTP server, NO long-running goroutine. `Poll` runs once, error returned, binary exits.
- `context.Background()` is allowed ONLY in `main()`'s `service.Main(...)` call — same allowed exception documented in spec 044 AC #10. Anywhere else in the new files use the context passed to `Run`.
- `errors.Wrapf(ctx, err, ...)` for error wrapping, NEVER `fmt.Errorf` in the production paths (`main.go`).
- Tests use `stderrors "errors"` alias for stdlib errors; reserve unprefixed `errors` for `github.com/bborbe/errors` (production only).
- Do NOT commit — dark-factory handles git.
- Frontmatter contract for emitted tasks MUST continue to match the FROZEN Phase 1 contract per [[Agent Task File Contract]] (run-once exercises the same Poll → publish path; nothing changes there).
- Existing tests must still pass.
</constraints>

<verification>
1. `cd watcher/github-release && go build ./...` exits 0
2. `cd watcher/github-release && make test` passes; new test names visible verbatim in output:
   - `Run Poll succeeds returns nil`
   - `Run Poll fails returns wrapped error`
   - `Run empty REPO_ALLOWLIST returns error`
3. `ls watcher/github-release/cmd/run-once/` shows three files: `main.go`, `main_test.go`, `Makefile`
4. `grep -rn "TODO\|fmt.Errorf" watcher/github-release/cmd/run-once/ --include='*.go'` returns 0 lines
5. `grep -rn "context.Background()" watcher/github-release/cmd/run-once/ --include='*.go' | grep -v _test.go` returns exactly one line in `main.go` matching `service.Main(context.Background()`
6. `cd watcher/github-release && make precommit` — all stages clean
</verification>
