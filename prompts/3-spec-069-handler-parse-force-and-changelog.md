---
status: draft
spec: [069-force-trigger-on-github-build-watcher]
created: "2026-06-26T12:20:00Z"
branch: dark-factory/force-trigger-on-github-build-watcher
---

<summary>
- Wires the `?force=true` query parameter at the HTTP edge of the github-build watcher's `/trigger` endpoint
- `force=true` publishes a `TriggerBuildCheckCommand{Force: true}`; missing, `false`, or unparseable values all resolve to `Force: false` (no 400 on garbage input)
- The handler's success v(2) log gains a `force=%t` substring so operators can grep cycles
- Stale "reserved-unread" comments on the handler doc, the command struct doc, and the struct's `Validate` doc are rewritten to reflect the wired behaviour — Scope's reserved-unread mention stays
- A CHANGELOG entry under `## Unreleased` mentions the `?force=true` query parameter and the spec number
- Three new handler unit tests cover the force=true / force=false-and-absent / force=garbage paths
</summary>

<objective>
Make `POST /admin/maintainer-watcher-github-build/trigger?force=true` round-trip the `Force` flag from query string → published `TriggerBuildCheckCommand` → consumer → watcher → state machine. Prompt 2 already plumbed the executor and watcher; this prompt finishes the parse + publish at the HTTP edge, rewrites the stale comments, and adds a CHANGELOG entry.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these coding plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-parse-pattern.md` — `libparse.ParseBoolDefault` semantics (lenient default; unparseable → default value)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — thin HTTP handler conventions (parse → publish, no business logic)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-handler-refactoring-guide.md` — handler shape for `libhttp.WithError`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — Unreleased section placement, bullet phrasing
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`

Read the files this prompt changes (verify before writing):
- `watcher/github-build/pkg/handler/trigger_handler.go` — `TriggerBuildCheckHandler`, `NewTriggerBuildCheckHandler`, `ServeHTTP`. Current master behaviour: publishes a zero-value `TriggerBuildCheckCommand{}` on every request; `ServeHTTP` takes `_ *http.Request`. The doc block currently says no query string is consumed (both Scope and Force reserved-unread).
- `watcher/github-build/pkg/command/trigger_build_check_command.go` — `TriggerBuildCheckCommand` struct + `Validate`. The struct doc-comment and `Validate` doc-comment currently describe `Force` as reserved-unread / plumbed-but-not-branched. This is now stale — the executor (prompt 2) DOES branch on it.
- `watcher/github-build/pkg/handler/trigger_handler_test.go` — existing handler tests. Any existing "publishes a zero-value command" test must keep passing when `force` is absent (it asserts `Force` is false; still true).
- `CHANGELOG.md` at REPO ROOT (NOT the service dir). Verify with `ls CHANGELOG.md`. The top-most release heading is currently `## v0.41.0`; there is NO `## Unreleased` section yet.

Key facts (verified):
- `libparse` import path: `libparse "github.com/bborbe/parse"`. The handler already imports it in master — verify; if absent, add it.
- `libparse.ParseBoolDefault(ctx context.Context, value interface{}, defaultValue bool) bool` — lenient (unparseable returns the default, no error).
- The HTTP 202 success body shape `{"status":"accepted"}` is frozen by spec — do NOT add a new field when force=true. `writeAccepted` stays unchanged.
- `TriggerBuildCheckCommand.Validate` returns `nil` unconditionally and accepts the empty payload. Leave the `Validate` body alone — `Force=true` is still a valid empty-Scope command.
- There is no glog-capture helper in this repo (verified). The v(2) log-substring assertion is therefore OPTIONAL (see requirement 4); the load-bearing observable — that `Force` is propagated to the published command — is covered by command-arg capture in tests a/b/c.
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Update `watcher/github-build/pkg/handler/trigger_handler.go`**:

   a. **Ensure import** `libparse "github.com/bborbe/parse"` is present in the import block (add if missing).

   b. **Rewrite the doc-block** on `TriggerBuildCheckHandler` so it describes the wired behaviour. Replacement text:
      ```
      // TriggerBuildCheckHandler handles POST /trigger.
      // The handler is intentionally minimal: parse the optional ?force=<bool>
      // query parameter via libparse.ParseBoolDefault (spec 069), build a
      // TriggerBuildCheckCommand carrying the parsed Force value, publish
      // it to Kafka via the injected sender, and return HTTP 202. The Scope
      // field stays reserved-unread (spec Non-goal: per-repo filter UX is a
      // separate spec). All scan-cycle work is owned by the in-pod command
      // consumer.
      ```

   c. **Rewrite `ServeHTTP`** so it parses `force` and publishes it:
      ```go
      func (h *triggerBuildCheckHandler) ServeHTTP(
          ctx context.Context,
          resp http.ResponseWriter,
          req *http.Request,
      ) error {
          force := libparse.ParseBoolDefault(
              ctx,
              req.URL.Query().Get("force"),
              false,
          )
          if err := h.sender.SendCommand(ctx, command.TriggerBuildCheckCommand{Force: force}); err != nil {
              // 502 BadGateway over 500/503: upstream Kafka is the proximate cause,
              // not this service. 500 implies an unexpected handler bug; 503 implies
              // this service is unhealthy. Kafka publish failure is neither — it's
              // an upstream gateway dependency, so 502 is the most accurate signal
              // for operators + observability tools.
              return libhttp.WrapWithStatusCode(
                  errors.Wrap(ctx, err, "send TriggerBuildCheckCommand"),
                  http.StatusBadGateway,
              )
          }

          glog.V(2).Infof(
              "trigger accepted op=%s force=%t",
              command.TriggerBuildCheckCommandOperation, force,
          )
          return writeAccepted(resp)
      }
      ```

      Changes from master:
      - `_ *http.Request` becomes `req *http.Request` (we read the query).
      - The zero-value `TriggerBuildCheckCommand{}` becomes `TriggerBuildCheckCommand{Force: force}`. `Scope` stays the zero-value empty string.
      - The success v(2) log gains `force=%t`.
      - Keep the inline 502-mapping comment unchanged.

   d. **Do NOT modify** `writeAccepted` — body shape `{"status":"accepted"}` is frozen.

   e. **AC pin**: after this edit,
      ```bash
      grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' watcher/github-build/pkg/handler/trigger_handler.go
      ```
      must return zero matches.

2. **Update `watcher/github-build/pkg/command/trigger_build_check_command.go`** — rewrite the struct-level doc block so it reflects the wired Force behaviour. Replacement text:
   ```
   // TriggerBuildCheckCommand is the payload for TriggerBuildCheckCommandOperation.
   // It is published to the github-build watcher's request topic by the /trigger
   // HTTP handler and consumed by the in-pod command consumer.
   //
   // Scope is reserved for a future per-repo filter UX; the executor still
   // ignores it. Force is wired (spec 069): when true, the consuming watcher's
   // red×red episode-lock arm publishes a salted CreateTaskCommand via
   // pkg.DeriveTaskIDForce instead of skipping — operators can force a
   // re-publish for a still-red build even when the episode is already locked.
   // All other state-machine arms (green→red, red→green) ignore Force.
   ```

   Also rewrite the `Validate` method's doc block:
   ```
   // Validate enforces the command's schema rules. The empty payload {} is
   // still accepted: Force defaults to false (engages the episode-lock skip,
   // the canonical poll-loop behaviour), and Scope remains reserved-unread.
   // A future spec will add per-repo or per-stage validation here.
   ```

   Do NOT change the struct field set, JSON tags, or the `Validate` body. Only the comments change.

   **AC pin**: after this edit,
   ```bash
   grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' watcher/github-build/pkg/command/trigger_build_check_command.go
   ```
   must return zero matches. (The phrase "Scope remains reserved-unread" DOES contain `reserved-unread` and WOULD match this grep — so phrase the Scope mention WITHOUT the literal `reserved-unread`. Use e.g. "Scope is reserved for a future per-repo filter UX" / "Scope stays reserved for a later spec". Re-check by eye after editing that no line matches any alternative.)

3. **Add three new handler unit tests** to `watcher/github-build/pkg/handler/trigger_handler_test.go`, as sibling `Context`/`It` blocks inside the existing `Describe`. Use the existing fixture pattern (sender counterfeiter mock + `httptest.NewRecorder`/`httptest.NewRequest`). Required coverage (the spec AC greps for the substring `Force` in test names):

   a. `force=true` → `POST /trigger?force=true` returns HTTP 202; `sender.SendCommandCallCount() == 1`; captured `sentCmd.Force == true`; `sentCmd.Scope == ""`.

   b. `force=false and absent` → `POST /trigger?force=false` and `POST /trigger` (no param) each return HTTP 202, publish exactly one command, and capture `sentCmd.Force == false`.

   c. `force=garbage` → `POST /trigger?force=banana` returns HTTP 202 (and explicitly NOT HTTP 400); `sender.SendCommandCallCount() == 1`; captured `sentCmd.Force == false`.

   Note on the handler's `ServeHTTP` signature: it is `ServeHTTP(ctx context.Context, resp http.ResponseWriter, req *http.Request) error` (a `libhttp.WithError`), so tests call it as `h.ServeHTTP(ctx, resp, req)` and may assert on the returned error being nil for the success paths. Mirror however the existing tests in this file invoke the handler.

   If master already has a "publishes exactly one zero-value command" test, KEEP it (it still passes — `Force=false` is the zero value) and add the three new contexts additively.

4. **v(2) log substring (OPTIONAL — repo has no glog-capture helper)**:
   - First grep for any existing capture infrastructure: `grep -rn "glog.*Buffer\|SetOutput\|FlushAndReset\|bytes\.Buffer.*glog" watcher/`.
   - If a helper exists, add ONE `It` per branch capturing v(2) output and asserting the substring `force=true` / `force=false`.
   - If no helper exists (expected), do NOT build glog-capture infrastructure for a thin decorative log line. The command-arg capture in tests a/b/c is the load-bearing observable and satisfies the intent. The `force=%t` log line is still added to the handler in requirement 1c regardless — it just is not separately asserted. Do NOT skip adding the log line; only the assertion is optional.

5. **Update `CHANGELOG.md` at repo root** — add a `## Unreleased` section ABOVE the current top heading `## v0.41.0`, with one bullet:
   ```markdown
   ## Unreleased

   - feat(watcher/github-build): add `?force=true` query parameter to `POST /trigger` so operators can re-publish a `CreateTaskCommand` for a repo whose build is currently red. The forced cycle bypasses the `red × red` episode lock and emits a salted `TaskIdentifier` (via new `pkg.DeriveTaskIDForce`) so the agent controller creates a fresh vault file. `force=false`, absent, or unparseable values are byte-identical to current behaviour (spec 069).
   ```
   Verify the bullet lands INSIDE the `## Unreleased` section:
   ```bash
   awk '/^## Unreleased/,/^## /' CHANGELOG.md | grep -ni "force" | head -1
   ```
   Expected: at least one matching line.

6. **Run precommit** from the service directory:
   ```bash
   cd watcher/github-build && make precommit
   ```
   Must exit 0.

7. **Final sanity greps**:
   ```bash
   grep -nE 'libparse\.ParseBoolDefault' watcher/github-build/pkg/handler/trigger_handler.go
   ! grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' \
       watcher/github-build/pkg/command/trigger_build_check_command.go \
       watcher/github-build/pkg/command/trigger_build_check_executor.go \
       watcher/github-build/pkg/handler/trigger_handler.go
   awk '/^## Unreleased/,/^## /' CHANGELOG.md | grep -ni "force"
   ```

</requirements>

<constraints>
- Edit only: `watcher/github-build/pkg/handler/trigger_handler.go`, `watcher/github-build/pkg/command/trigger_build_check_command.go`, `watcher/github-build/pkg/handler/trigger_handler_test.go`, and `CHANGELOG.md` at repo root. Do NOT touch `watcher/github-build/pkg/watcher.go`, the executor, the factory, or `main.go` — prompt 2 owns those.
- Do NOT commit — dark-factory handles git.
- Do NOT change the HTTP response shape on the 202 success path. Body stays `{"status":"accepted"}`. No new fields when `force=true`.
- Do NOT return 400 on unparseable `force` values. `libparse.ParseBoolDefault` is lenient by design — preserve that contract.
- Do NOT change `TriggerBuildCheckCommand`'s field set, JSON tags, or `Validate`'s behaviour. Only doc comments change.
- Do NOT touch `Scope`. It stays reserved; only `Force` wiring is in scope.
- Do NOT change `TriggerBuildCheckCommandOperation`'s wire string (`"trigger-build-check"`).
- Do NOT add a request body parser. The query string is the only input.
- Use `github.com/bborbe/errors` for error wrapping. Use `libparse "github.com/bborbe/parse"` for boolean parsing.
- Ginkgo v2 + Gomega + counterfeiter for new tests, matching the existing `trigger_handler_test.go` style.
- The CHANGELOG bullet lives at the REPO ROOT `CHANGELOG.md` (not `watcher/github-build/CHANGELOG.md`).
- `make precommit` runs from `watcher/github-build/`, never from repo root.
</constraints>

<verification>
cd watcher/github-build && make precommit

# Handler parses force:
grep -nE 'libparse\.ParseBoolDefault' pkg/handler/trigger_handler.go
# Expect: 1 line.

# No stale Force reservation phrasing left across all three files:
! grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' \
    pkg/command/trigger_build_check_command.go \
    pkg/command/trigger_build_check_executor.go \
    pkg/handler/trigger_handler.go
# Expect: exit 0 (no matches).

# Three new Force-named handler tests pass:
go test ./pkg/handler -run 'Force' -v
# Expect: ≥3 PASS entries.

# CHANGELOG bullet under Unreleased mentions force:
cd ..
awk '/^## Unreleased/,/^## /' CHANGELOG.md | grep -ni "force"
# Expect: ≥1 line.

# git diff confinement (informational):
git diff --stat
</verification>
