---
status: completed
spec: ["069"]
summary: 'Parsed force query param via libparse.ParseBoolDefault in /trigger handler, populated TriggerPRReviewCommand.Force, added 4 Ginkgo tests covering true/false/absent/garbage, and appended feat bullet to CHANGELOG ## Unreleased'
container: maintainer-trigger-force-exec-254-spec-067-handler-force-param-and-changelog
dark-factory-version: v0.175.0
created: "2026-06-09T15:50:00Z"
queued: "2026-06-09T16:02:46Z"
started: "2026-06-09T16:38:00Z"
completed: "2026-06-09T16:42:59Z"
branch: dark-factory/force-trigger-on-github-pr-watcher
---

<summary>
- `POST /trigger?url=<u>&force=<bool>` on the watcher/github-pr HTTP handler now sets `Force` on the published `TriggerPRReviewCommand` from the parsed query parameter.
- The `force` query parameter is parsed via `libparse.ParseBoolDefault(ctx, raw, false)` — truthy values (`true`/`True`/`1` etc.) and falsy values (`false`/`False`/`0` etc.) are recognized; unparseable values (e.g. `force=banana`) fall back silently to `false` and the request still returns HTTP 202.
- The HTTP response body shape (`{"status":"accepted","url":<raw>}`) and status code (202) are unchanged — `force` only affects the published command, not the response.
- Four new Ginkgo unit tests cover: `force=true` sets the field, `force=false` sets the field, absent `force` keeps it `false`, and unparseable `force=banana` returns 202 with `Force=false` (no 400).
- The CHANGELOG gets a new `## Unreleased` section (if not already present) with a `feat:` bullet describing the new query parameter and its salted-identifier behavior.
</summary>

<objective>
Parse the `force` query parameter in the `POST /trigger` HTTP handler using `libparse.ParseBoolDefault`, populate the `Force` field on the published `TriggerPRReviewCommand`, and add a CHANGELOG bullet under `## Unreleased` describing the new operator-facing behavior. The non-force request shape is unchanged; an unparseable `force` value falls back to `false` and the request still returns 202.
</objective>

<context>
Read the project conventions and the relevant docs:
- `/workspace/CLAUDE.md` (project-wide rules)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-parse-pattern.md` (defines the `libparse.ParseBoolDefault` lenient-default contract)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-handler-refactoring-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-json-error-handler-guide.md` (for the 400 vs 502 wire shapes — confirm 202 is unchanged for valid+unparseable force)
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` (mandatory — defines the `## Unreleased` section rules and conventional prefixes)

Read these source files fully before editing:
- `/workspace/watcher/github-pr/pkg/handler/trigger_handler.go` — you are extending the handler to parse `force`; the rest of the file is unchanged.
- `/workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go` — existing Ginkgo test layout (`error cases` table-driven, `happy path` context, `Kafka send failure`, `GitHub client off the request path`). All existing tests must pass unmodified; you add a new `Context("force query param", ...)` block.
- `/workspace/CHANGELOG.md` — the global changelog. There is currently no `## Unreleased` section (the file goes straight from preamble to `## v0.37.0`). You must INSERT a `## Unreleased` section immediately above `## v0.37.0` (per changelog-guide rule "Newest version first — `## Unreleased` goes directly above the highest `## vX.Y.Z`"). Keep the preamble (`# Changelog` title, "All notable changes..." line, SemVer link, MAJOR/MINOR/PATCH bullets) byte-identical and at the top.
- `/workspace/specs/in-progress/067-force-trigger-on-github-pr-watcher.md` — Constraints (the lenient-default behavior is frozen at the spec level, not test-commit time), Acceptance Criteria (the four `TestTriggerHandler_*` ACs and the CHANGELOG bullet AC).

The Go symbols `libparse.ParseBoolDefault` and the `bborbe/parse v1.10.13` indirect dep are already in `go.mod` — no `go.mod` changes required. Confirm by `grep -n bborbe/parse /workspace/watcher/github-pr/go.mod` (expect at least one match in the `// indirect` block).
</context>

<requirements>

1. **Parse the `force` query parameter in `pkg/handler/trigger_handler.go`.** Add a new import: `libparse "github.com/bborbe/parse"`. In `ServeHTTP`, after `rawURL := req.URL.Query().Get("url")` and before the `validateTriggerURL` call, add:

   ```go
   force := libparse.ParseBoolDefault(
       ctx,
       req.URL.Query().Get("force"),
       false,
   )
   ```

   The lenient default (`false`) is pinned by the spec Constraint — unparseable values like `force=banana` must NOT cause a 400. They fall through to the non-force path and the request still returns 202.

   Then change the published command to set `Force` from the parsed value, not the hardcoded `false`:

   ```go
   if err := h.sender.SendCommand(ctx, command.TriggerPRReviewCommand{
       URL:   rawURL,
       Force: force,
   }); err != nil {
       // ... unchanged 502 path
   }
   ```

2. **Do not change the HTTP response body shape.** `writeAccepted(resp, rawURL)` is unchanged. The 202 body remains `{"status":"accepted","url":<raw>}` — no new fields added when `force=true` (spec Constraint: HTTP response shape is frozen).

3. **Do not touch the executor, factory, main.go, or any non-handler file.** This prompt is HTTP-edge-only. The executor branch from prompt 2 reads `cmd.Force`; this prompt just populates that field on the wire.

4. **Add four new Ginkgo tests in `pkg/handler/trigger_handler_test.go`.** Add a new `Context("force query param (spec 067)", ...)` block at the end of the existing `Describe("TriggerHandler", func() { ... })`. Do NOT modify any existing `Context` or `DescribeTable`. Reuse the `sender` mock and the `h` handler from the existing `BeforeEach`.

   The four tests, with Ginkgo `It` names mapped to spec ACs:

   - `It("TestTriggerHandler_ParsesForceTrue")` — build `httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42&force=true", nil)`. Run `h.ServeHTTP(resp, req)`. Assert `resp.Code == 202`. Assert `sender.SendCommandCallCount() == 1`. Capture `_, sentCmd := sender.SendCommandArgsForCall(0)`. Assert `sentCmd.Force == true`. The existing tests in the file inline the PR URL literally (`"https://github.com/bborbe/repo/pull/42"`); there is no shared `validPRURL` constant — match that style by inlining the literal.
   - `It("TestTriggerHandler_ParsesForceFalse")` — two assertions, one per request: (a) `httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42&force=false", nil)` → `sentCmd.Force == false`; (b) `httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42", nil)` → `sentCmd.Force == false`. Use `sender.SendCommandArgsForCall(0)` (after the most recent call). Reset the mock between requests with `*sender = mocks.TriggerPRReviewCommandSender{}` if call counts collide.
   - `It("TestTriggerHandler_ParsesForceAbsent")` — explicit assertion that an absent query parameter defaults to `false`. `httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42", nil)`. Assert `sentCmd.Force == false`. (This is technically a subset of the `ParsesForceFalse` second branch, but the spec names the AC explicitly so the dedicated test pins it.)
   - `It("TestTriggerHandler_ParsesForceGarbage")` — `httptest.NewRequest("POST", "/trigger?url=https://github.com/bborbe/repo/pull/42&force=banana", nil)`. Assert three things: (a) `resp.Code == 202` (NOT 400); (b) `sender.SendCommandCallCount() == 1` (the request proceeds); (c) `sentCmd.Force == false` (lenient default). The negative assertion `Expect(resp.Code).NotTo(Equal(400))` is also acceptable to mirror the spec wording, but a positive `== 202` is clearer.

   The existing `h` in `BeforeEach` is wrapped via `libhttp.NewErrorHandler(handler.NewSinglePRTriggerHandler(sender))` — this is what writes the status code, so the test asserts on `resp.Code` (the recorded HTTP response code, not the error from `ServeHTTP`). Note: the existing tests in this file call `h.ServeHTTP(resp, req)` directly and assert on `resp.Code` — follow the same pattern.

5. **Do not add a new label or any new config field for the `force` parameter.** The spec Non-goals explicitly forbid an opt-out flag ("invariant"). No `ENABLE_FORCE_TRIGGER` env var, no feature toggle, no kill switch.

6. **Add the `## Unreleased` section to `/workspace/CHANGELOG.md` and a new bullet.** The file currently has no `## Unreleased` (it goes straight from the SemVer preamble to `## v0.37.0`). Insert `## Unreleased` immediately above the first `## vX.Y.Z` heading. Keep the preamble byte-identical and at the top.

   Bullet format (per changelog-guide.md): one entry, prefix `feat:` (this is a new operator-facing capability → minor version bump). Example wording — use this exact content (or a tight equivalent that names the parameter and the salted-identifier mechanism):

   ```
   - feat(watcher/github-pr): add `?force=true` query parameter to `POST /trigger` so operators can request a forced re-review against an already-reviewed head SHA — the executor derives a salted `TaskIdentifier` (extra nonce from the current time) so the agent controller's dedup-skip does not fire and a fresh vault file is created. Non-force requests are unchanged byte-for-byte. Unparseable `force` values fall back to `false` and the request still returns 202 (lenient default).
   ```

   Follow the changelog-guide rule "Be specific: name types, commands, packages — never write `- feat: add force`." The bullet above names the exact endpoint, the field on the command, the mechanism (salted identifier), and the fallback behavior — that is specific enough.

7. **Verify the ACs in the spec's "Verification" section.** After the prompt, run the same grep evidence checks:

   ```bash
   grep -nE 'libparse\.ParseBoolDefault|FormValue\("force"\)' /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
   grep -nE 'Force:' /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
   grep -A 30 '^## Unreleased' /workspace/CHANGELOG.md
   ```

   - First grep: must return at least one line (the `libparse.ParseBoolDefault` import call site OR the `FormValue("force")` — note: the requirement uses `req.URL.Query().Get("force")`, NOT `FormValue`, but the spec's verification regex is `libparse\.ParseBoolDefault|FormValue\("force"\)`. Either side matches; the implementation uses `req.URL.Query().Get` which produces the same wire behavior. Both sides of the OR match. Make sure the implementation uses `req.URL.Query().Get("force")` AND `libparse.ParseBoolDefault(...)` so the grep returns at least one line.
   - Second grep: must return a line where `Force:` is followed by an identifier (the local var `force`), NOT the literal `false`. The required pattern is `Force: force,`.
   - Third grep: must show the new bullet under `## Unreleased`.

</requirements>

<constraints>
- The HTTP response shape (`{"status":"accepted","url":<raw>}`) and status code (202) on the success path are FROZEN (spec Constraint). No new fields are added when `force=true`.
- Unparseable `force` values (e.g. `force=banana`) MUST return HTTP 202 with `Force=false` published — NOT HTTP 400. The lenient-default behavior is operator-facing and frozen at the spec level (spec Constraint: "This decision is operator-facing and frozen here, not deferred to test-commit time"). The `TestTriggerHandler_ParsesForceGarbage` test pins this.
- `SinglePRTriggerHandler` type alias (`libhttp.WithError`) and `NewSinglePRTriggerHandler` constructor name are FROZEN. Do not rename or retype. The constructor signature is also FROZEN in this prompt — adding a new dependency to the handler would ripple into the factory and `main.go` wiring, and the spec keeps the handler thin (parse → publish). Stay with the existing `(sender command.TriggerPRReviewCommandSender) SinglePRTriggerHandler` signature.
- `TriggerPRReviewCommand` struct field set, JSON tags, and `Validate` rules are FROZEN (spec Constraint). The `Force` field is already shipped (spec 066) and is not modified here — only populated from the query param.
- The handler remains thin (parse → publish). The factory, executor, and main.go are NOT touched in this prompt.
- Do NOT add a config flag, opt-out, or kill switch for the `force` parameter (spec Non-goal: "Do NOT add a config flag that disables the `force` query param — invariant").
- The CHANGELOG preamble (`# Changelog` title, "All notable changes..." line, SemVer link, MAJOR/MINOR/PATCH bullets) is FROZEN — insert `## Unreleased` AFTER the preamble and BEFORE the first `## vX.Y.Z` heading, never above or inside the preamble.
- The CHANGELOG bullet MUST start with a conventional prefix (`feat:` here, since this is a new feature). No `- Add force param` — that would fail the `changelog/conventional-prefix-required` rule.
- Do not touch `pkg/command/`, `pkg/factory/`, `pkg/watcher.go`, `pkg/metrics.go`, `pkg/taskid.go`, or `main.go` in this prompt. The diff is HTTP handler + handler test + CHANGELOG only.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
Run from `/workspace/`:

```bash
grep -nE 'libparse\.ParseBoolDefault|FormValue\("force"\)' /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
grep -n 'Force:' /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
go test ./watcher/github-pr/pkg/handler -run 'ParsesForce' -v
go test ./watcher/github-pr/pkg/handler -v
grep -A 30 '^## Unreleased' /workspace/CHANGELOG.md
```

Then run the module precommit (per spec's verification rung-1):

```bash
cd /workspace/watcher/github-pr && make precommit
```

Expected:
- First grep returns at least one line (the `libparse.ParseBoolDefault` call site).
- Second grep returns a line where `Force:` is followed by the identifier `force,` — NOT the literal `false`.
- `go test ./pkg/handler -v` shows the new four `ParsesForce*` tests PASS and the existing `TriggerHandler` tests PASS unmodified.
- The CHANGELOG grep shows the new `feat(watcher/github-pr): add \`?force=true\`...` bullet under `## Unreleased`.
- `make precommit` exits 0.

Also confirm the no-touch constraint: `git diff master -- watcher/github-pr/pkg/command/ watcher/github-pr/pkg/factory/ watcher/github-pr/pkg/watcher.go watcher/github-pr/pkg/metrics.go watcher/github-pr/pkg/taskid.go watcher/github-pr/main.go` should be empty (or show only the prompt 1 / prompt 2 changes already in master since the spec ships as a single branch). If those paths show additional changes introduced by this prompt, the prompt drifted and must be redone.
</verification>
