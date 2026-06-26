---
status: approved
spec: [069-force-trigger-on-github-build-watcher]
created: "2026-06-09T20:30:00Z"
queued: "2026-06-09T20:18:04Z"
---

<summary>
- Wires the `?force=true` query parameter at the HTTP edge of the github-build watcher's `/trigger` endpoint
- `force=true` publishes a `TriggerBuildCheckCommand{Force: true}`; missing, `false`, or unparseable values all resolve to `Force: false` (no 400 on garbage input)
- The handler's success v(2) log gains a `force=%t` substring so operators can grep cycles
- Stale "reserved-unread" comments on the handler doc, the command struct doc, and the command struct's `Force` field are rewritten to reflect the wired behaviour — Scope's reserved-unread paragraph stays
- A CHANGELOG entry under `## Unreleased` mentions the `?force=true` query parameter and the spec number
- Three new handler unit tests cover the force=true / force=false-and-absent / force=garbage paths
</summary>

<objective>
Make `POST /admin/maintainer-watcher-github-build/trigger?force=true` round-trip the `Force` flag from query string → published `TriggerBuildCheckCommand` → consumer → watcher → state machine. Prompt 2 already plumbed the executor and watcher; this prompt finishes the parse + publish at the HTTP edge, updates the stale comments, and adds a CHANGELOG entry.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these coding plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-parse-pattern.md` — `libparse.ParseBoolDefault` semantics (lenient default; unparseable → default value)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — thin HTTP handler conventions (parse → publish, no business logic)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-handler-refactoring-guide.md` — handler shape for `libhttp.WithError`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — Unreleased section placement, bullet phrasing
- `/home/node/.claude/plugins/marketplaces/coding/docs/dod.md`

Read the files this prompt changes (verify before writing):
- `watcher/github-build/pkg/handler/trigger_handler.go` — `TriggerBuildCheckHandler`, `NewTriggerBuildCheckHandler`, `ServeHTTP`. Current behaviour: publishes a zero-value `TriggerBuildCheckCommand{}` on every request. The doc block (lines 21-26) currently says `No request body or query string is consumed (both Scope and Force are reserved-unread).`
- `watcher/github-build/pkg/command/trigger_build_check_command.go` — `TriggerBuildCheckCommand` struct (lines 26-29). The struct doc-comment (lines 17-25) currently says `Force is reserved for the prerequisite Force-flag task; this spec plumbs the field but the executor does not branch on it.` This is now stale — the executor DOES branch on it.
- `watcher/github-build/pkg/handler/trigger_handler_test.go` — existing handler tests. The "publishes exactly one zero-value TriggerBuildCheckCommand" test at line 51-61 must keep working when `force` is absent (it asserts `sentCmd.Force` is false; still true).
- `watcher/github-build/CHANGELOG.md` — wait: changelog is at REPO ROOT, NOT the service dir. Verify with `ls CHANGELOG.md` at repo root.

Read the sibling pattern for shape reference (READ-ONLY — separate worktree):
- `/Users/bborbe/Documents/workspaces/maintainer-trigger-force/watcher/github-pr/pkg/handler/trigger_handler.go` lines 49-67 — exactly the pattern to mirror:
  ```go
  force := libparse.ParseBoolDefault(
      ctx,
      req.URL.Query().Get("force"),
      false,
  )
  // ...
  if err := h.sender.SendCommand(ctx, command.TriggerPRReviewCommand{
      URL:   rawURL,
      Force: force,
  }); err != nil { ... }
  ```

Key facts (verified):
- `libparse` import path: `libparse "github.com/bborbe/parse"`
- `libparse.ParseBoolDefault(ctx context.Context, raw string, def bool) bool` — lenient (unparseable returns `def`, no error). Verify in the sibling handler.
- The handler's `ServeHTTP` currently takes `_ *http.Request` (line 47) — change to `req *http.Request` to read the query.
- The HTTP 202 success body shape `{"status":"accepted"}` is frozen by spec — do NOT add a new field when force=true. The body stays identical.
- The handler's success v(2) log line at line 62 currently reads `glog.V(2).Infof("trigger accepted op=%s", command.TriggerBuildCheckCommandOperation)`. Extend it to include `force=%t`.
- `TriggerBuildCheckCommand.Validate` (lines 35-37 of trigger_build_check_command.go) returns `nil` unconditionally and accepts the empty payload. Leave Validate alone — Force=true is still a valid empty-Scope command.
- CHANGELOG.md at repo root currently has its most recent heading at `## v0.39.0` (line 11). There is NO `## Unreleased` section yet — you will add one above `## v0.39.0`.
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Update `watcher/github-build/pkg/handler/trigger_handler.go`**:

   a. **Add import** `libparse "github.com/bborbe/parse"` to the existing import block.

   b. **Rewrite the doc-block** on `TriggerBuildCheckHandler` (lines 21-26 in current master). Replace this:

      ```
      // TriggerBuildCheckHandler handles POST /trigger.
      // The handler is intentionally minimal: build a zero-value
      // TriggerBuildCheckCommand, publish it to Kafka via the injected
      // sender, and return HTTP 202. No request body or query string is
      // consumed (both Scope and Force are reserved-unread). All scan
      // cycle work is owned by the in-pod command consumer.
      ```

      with this:

      ```
      // TriggerBuildCheckHandler handles POST /trigger.
      // The handler is intentionally minimal: parse the optional ?force=
      // boolean query parameter via libparse.ParseBoolDefault (unparseable
      // values resolve to false), publish a TriggerBuildCheckCommand{Force:
      // force} to Kafka via the injected sender, and return HTTP 202.
      // Scope remains reserved-unread (per-repo filter UX is a future spec).
      // All poll-cycle work is owned by the in-pod command consumer.
      ```

   c. **Rewrite `ServeHTTP`** (lines 44-64 in current master). The new body:

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
          if err := h.sender.SendCommand(ctx, command.TriggerBuildCheckCommand{
              Force: force,
          }); err != nil {
              // 502 BadGateway over 500/503: upstream Kafka is the proximate
              // cause, not this service. 500 implies an unexpected handler
              // bug; 503 implies this service is unhealthy. Kafka publish
              // failure is neither — it's an upstream gateway dependency,
              // so 502 is the most accurate signal for operators +
              // observability tools.
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
      - The zero-value `TriggerBuildCheckCommand{}` becomes `TriggerBuildCheckCommand{Force: force}`. `Scope` stays the zero-value empty string — spec Non-goal says only `Force` is wired.
      - The success v(2) log gains `force=%t` formatted with the parsed `force` value.
      - Keep the inline comment about the 502 mapping unchanged.

   d. **Do NOT modify** `writeAccepted` (lines 67-73) — body shape `{"status":"accepted"}` is frozen by spec.

   e. **AC pin**: after this edit,
      ```bash
      grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' watcher/github-build/pkg/handler/trigger_handler.go
      ```
      must return zero matches.

2. **Update `watcher/github-build/pkg/command/trigger_build_check_command.go`** — rewrite the struct-level doc block (lines 17-25 in current master). Replace:

   ```
   // TriggerBuildCheckCommand is the payload for TriggerBuildCheckCommandOperation.
   // It is published to the github-build watcher's request topic by the /trigger
   // HTTP handler and consumed by the in-pod command consumer.
   //
   // Scope is reserved for a future per-repo filter UX; this spec plumbs the
   // field but the executor ignores it. Force is reserved for the prerequisite
   // Force-flag task; this spec plumbs the field but the executor does not
   // branch on it. Both fields are present on the wire so the schema does
   // not need to change later.
   ```

   with:

   ```
   // TriggerBuildCheckCommand is the payload for TriggerBuildCheckCommandOperation.
   // It is published to the github-build watcher's request topic by the /trigger
   // HTTP handler and consumed by the in-pod command consumer.
   //
   // Scope is reserved for a future per-repo filter UX; this spec plumbs the
   // field but the executor ignores it.
   //
   // Force, when true, instructs the watcher to bypass the red×red episode
   // lock and emit a salted TaskIdentifier so the agent controller does not
   // dedup-skip the resulting CreateTaskCommand. Operators set this via
   // POST /trigger?force=true. force=false (or absent / unparseable) is
   // byte-identical to a non-force cycle.
   ```

   Also re-check the `Validate` method doc block (lines 31-34): it currently says `The empty payload {} is accepted because both fields are reserved-unread — there's no per-request field with meaning today.` Rewrite to:

   ```
   // Validate enforces the command's schema rules. The empty payload {}
   // is accepted: Scope is still reserved-unread (future per-repo filter
   // UX), and Force=false is the documented default. A future spec will
   // add per-repo or per-stage validation here.
   ```

   Do NOT change the struct field set, JSON tags, or the `Validate` body. Only the comments change.

   **AC pin**: after this edit,
   ```bash
   grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' watcher/github-build/pkg/command/trigger_build_check_command.go
   ```
   must return zero matches. (Scope-only references such as "Scope is still reserved-unread" are explicitly NOT matched by this grep because every alternative includes either "deferred" or "Force" — re-check by eye after editing.)

3. **Add three new handler unit tests** to `watcher/github-build/pkg/handler/trigger_handler_test.go`. Add them as sibling `Context` blocks inside the existing `Describe("TriggerHandler", ...)` block. Use the existing fixture pattern (sender mock + `httptest.NewRecorder`). Required test names (or near-equivalents — the AC grep uses the substring `Force`):

   a. `Context("force=true", ...)` containing `It("publishes Force=true on POST /trigger?force=true", ...)`:
      ```go
      req := httptest.NewRequest("POST", "/trigger?force=true", nil)
      resp := httptest.NewRecorder()
      h.ServeHTTP(resp, req)

      Expect(resp.Code).To(Equal(http.StatusAccepted))
      Expect(sender.SendCommandCallCount()).To(Equal(1))
      _, sentCmd := sender.SendCommandArgsForCall(0)
      Expect(sentCmd.Force).To(BeTrue())
      Expect(sentCmd.Scope).To(BeEmpty())
      ```

   b. `Context("force=false and absent", ...)` containing two `It` blocks (or a table):
      - `POST /trigger?force=false` → `sentCmd.Force == false`
      - `POST /trigger` (no param)   → `sentCmd.Force == false`
      Both must return HTTP 202 and exactly one published command per request.

   c. `Context("force=garbage (unparseable)", ...)` containing `It("treats unparseable force as false, returns 202", ...)`:
      ```go
      req := httptest.NewRequest("POST", "/trigger?force=banana", nil)
      resp := httptest.NewRecorder()
      h.ServeHTTP(resp, req)

      // Must NOT return 400.
      Expect(resp.Code).To(Equal(http.StatusAccepted))
      Expect(resp.Code).NotTo(Equal(http.StatusBadRequest))
      Expect(sender.SendCommandCallCount()).To(Equal(1))
      _, sentCmd := sender.SendCommandArgsForCall(0)
      Expect(sentCmd.Force).To(BeFalse())
      ```

   The existing "publishes exactly one zero-value TriggerBuildCheckCommand" test at lines 51-61 of master overlaps with (b)'s absent-param case. Either delete that older `It` (since it is now subsumed) OR keep it and let the new tests be additive — choose the additive path: KEEP the existing test, add the three new contexts. The older test still passes verbatim because `Force=false` is the zero value and `Scope` is empty.

4. **v(2) log substring assertion** — the spec AC requires the handler's v(2) log to include `force=true` / `force=false`. The command-arg capture in tests (a/b/c) already proves `Force` is propagated to the published command; the log substring is a separate observable. Implementation order:

   a. **First**, grep for an existing glog-capture pattern in this repo: `grep -rn "glog\.Buffer\|klog\.SetOutput\|FlushAndReset\|bytes\.Buffer.*glog" watcher/`. If a helper exists, use it.

   b. **Otherwise**, the spec AC is over-specified for what the handler observably does. The handler is a thin wrapper: parse → publish → 202 + log. Command-arg capture (tests a/b/c) covers parse + publish; the log line is decorative. In this case, edit the spec AC (line ~107 of `specs/in-progress/069-force-trigger-on-github-build-watcher.md`) to drop the v(2) log substring requirement — note the rationale ("handler is too thin to justify glog-capture infrastructure; command-arg capture covers the load-bearing observable") in the spec edit. Do NOT silently skip the AC and ship; explicitly amend the spec.

   c. **If you find capture infrastructure**, add ONE new `It` per existing block (true/false), capturing glog v(2) output and asserting the substring. Otherwise the spec amendment is the resolution.

5. **Update `CHANGELOG.md` at repo root** — add `## Unreleased` ABOVE `## v0.39.0` (line 11), with one bullet:

   ```markdown
   ## Unreleased

   - feat(watcher/github-build): Add `?force=true` query parameter to `POST /trigger` so operators can re-publish a `CreateTaskCommand` for a repo whose build is currently red. The forced cycle bypasses the `red × red` episode lock and emits a salted `TaskIdentifier` (via new `pkg.DeriveTaskIDForce`) so the agent controller creates a fresh vault file. `force=false`, absent, or unparseable values are byte-identical to the current behaviour (spec 069).
   ```

   Verify the bullet lands INSIDE the `## Unreleased` section by running:
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
   # Handler reads force:
   grep -nE 'libparse\.ParseBoolDefault' watcher/github-build/pkg/handler/trigger_handler.go
   # one line

   # No stale reserved-unread Force phrasing anywhere on the watcher:
   ! grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' \
       watcher/github-build/pkg/command/trigger_build_check_command.go \
       watcher/github-build/pkg/command/trigger_build_check_executor.go \
       watcher/github-build/pkg/handler/trigger_handler.go

   # CHANGELOG mentions force inside Unreleased:
   awk '/^## Unreleased/,/^## /' CHANGELOG.md | grep -ni "force"
   ```

</requirements>

<constraints>
- Edit only: `watcher/github-build/pkg/handler/trigger_handler.go`, `watcher/github-build/pkg/command/trigger_build_check_command.go`, `watcher/github-build/pkg/handler/trigger_handler_test.go`, and `CHANGELOG.md` at repo root. Do NOT touch `watcher/github-build/pkg/watcher.go`, the executor, the factory, or `main.go` — prompt 2 owns those.
- Do NOT commit — dark-factory handles git.
- Do NOT change the HTTP response shape on the 202 success path. Body stays `{"status":"accepted"}`. No new fields when `force=true`.
- Do NOT return 400 on unparseable `force` values. `libparse.ParseBoolDefault` is lenient by design — preserve that contract.
- Do NOT change `TriggerBuildCheckCommand`'s field set, JSON tags, or `Validate`'s behaviour. Only doc comments change.
- Do NOT touch `Scope`. It stays reserved-unread; the spec scopes only `Force` wiring.
- Do NOT change `TriggerBuildCheckCommandOperation`'s wire string (`"trigger-build-check"`).
- Do NOT add a request body parser. The query string is the only input.
- Use `github.com/bborbe/errors` for any new error wrapping. Use `libparse "github.com/bborbe/parse"` for boolean parsing.
- Ginkgo v2 + Gomega + counterfeiter for new tests, matching the existing `trigger_handler_test.go` style.
- CHANGELOG bullet lives at the REPO ROOT `CHANGELOG.md` (not `watcher/github-build/CHANGELOG.md`).
- `make precommit` runs from `watcher/github-build/`, never from repo root.
</constraints>

<verification>
cd watcher/github-build && make precommit

# Handler parses force:
grep -nE 'libparse\.ParseBoolDefault' pkg/handler/trigger_handler.go
# Expect: 1 line.

# No stale reserved-unread Force phrasing left:
! grep -nE 'reserved-unread|deferred.*Force|Force.*deferred|TODO.*[Ff]orce|prerequisite Force-flag' \
    pkg/command/trigger_build_check_command.go \
    pkg/command/trigger_build_check_executor.go \
    pkg/handler/trigger_handler.go
# Expect: exit 0 (no matches).

# Three new Force-named tests pass:
go test ./pkg/handler -run 'Force' -v
# Expect: ≥3 PASS entries.

# CHANGELOG bullet under Unreleased mentions force:
cd ..
awk '/^## Unreleased/,/^## /' CHANGELOG.md | grep -ni "force"
# Expect: ≥1 line.

# git diff confinement (informational — dark-factory tracks the actual confinement):
git diff --stat
# Expect changes confined to watcher/github-build/pkg/handler/, watcher/github-build/pkg/command/, and CHANGELOG.md.
</verification>
