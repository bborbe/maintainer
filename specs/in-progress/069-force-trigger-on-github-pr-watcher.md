---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-06-09T15:40:07Z"
generating: "2026-06-09T15:40:08Z"
prompted: "2026-06-09T15:57:15Z"
branch: dark-factory/force-trigger-on-github-pr-watcher
previous_id: 067
---

## Summary

- Operators today cannot re-run a PR review against the same head SHA. The CQRS `/trigger` pipeline derives a deterministic task identifier from `(owner, repo, number, sha)`, and the agent controller silently skips creating a task when a vault file with that identifier already exists.
- This spec adds an opt-in `force=true` query parameter to `POST /trigger` on the `watcher/github-pr` HTTP handler.
- When `force=true`, the executor derives a *salted* task identifier (extra nonce drawn from the current time) so the controller's file-exists skip does not fire and a fresh vault file is created. Prior review history is preserved.
- When `force=false` or absent, behavior is bit-identical to today: same published `CreateTaskCommand`, same task identifier, same metric label, same vault file path.
- The `Force` field is already wired through the Kafka command struct (shipped by spec 066); this spec finishes the plumbing in the HTTP handler and the executor and adds the new helper.

## Problem

The `/trigger` HTTP endpoint on `watcher/github-pr` lets an operator publish a `TriggerPRReviewCommand` for a single PR. The executor derives a deterministic `TaskIdentifier` from `(owner, repo, number, sha)`. Downstream, the agent controller treats this identifier as the dedup key: if a vault file with that identifier already exists, the controller logs `"create-task: task file already exists at %s for %s, skipping (idempotent)"` and returns success without creating a new file. As a consequence, when an operator wants to re-run a review against the same head SHA (e.g. after fixing a transient infrastructure problem, after a prompt-template change, or to validate a reviewer-config tweak), the trigger silently no-ops. There is no operator-facing way to demand a fresh review for an already-reviewed SHA. Spec 036 originally proposed a cross-repo `force_bypass_dedup` mechanism and dropped it for simplification; spec 066 plumbed the `Force bool` field on the Kafka command but did not wire it through. This spec closes the gap.

## Goal

After this work, operators can request a forced re-review by appending `?force=true` to the `/trigger` URL. A `force=true` request results in the publication of a new downstream `CreateTaskCommand` whose `TaskIdentifier` is distinct from the canonical `(owner, repo, number, sha)`-derived identifier, so the agent controller creates a brand-new vault file instead of skipping. The non-force path is untouched: every byte of the published `CreateTaskCommand`, the derived identifier, the metric label, and the vault filename produced by `force=false` (or absent) is identical to today's behavior.

## Non-goals

- Do NOT modify `watcher/github-release` — the same flag with a different mechanism is covered by a sibling task; out of scope here.
- Do NOT change the `/admin` gating on `/trigger` — separate task.
- Do NOT modify `agent/lib/command/task.CreateCommand` or any code in the agent repo — the salted identifier in maintainer accomplishes the bypass without touching the agent contract.
- Do NOT add rate-limiting, abuse prevention, or audit logging specifically for `force=true` — `/trigger` remains behind `/admin`; that's a separate task.
- Do NOT add a config flag that disables the `force` query param — invariant; if a future consumer demands a way to switch it off, that's a separate spec. An escape hatch on the Goal is a regression.
- Do NOT change the existing `DeriveTaskID` function, its signature, its namespace UUID, or its input encoding. The non-force path stays bit-identical so existing tests in `pkg/taskid_test.go` and `pkg/watcher_test.go` pass unmodified.
- Do NOT branch the poll path (`watcher.go:281`) on `Force` — `Force` is a `/trigger`-only concept; the poll path's in-pod cursor dedup is independent and untouched.
- Do NOT introduce a result topic for `TriggerPRReviewCommand` — `SendResultEnabled` stays `false`.

## Desired Behavior

1. A `POST /trigger?url=<pr_url>` request with no `force` parameter publishes a `TriggerPRReviewCommand{URL: <url>, Force: false}` and produces identical observable output to today (same `TaskIdentifier`, same `CreateTaskCommand` payload, same `github_pr_published{result="create"}` increment).
2. A `POST /trigger?url=<pr_url>&force=true` request publishes a `TriggerPRReviewCommand{URL: <url>, Force: true}`. The HTTP handler returns 202 with the same response body shape as today.
3. A `POST /trigger?url=<pr_url>&force=false` request behaves identically to omitting the parameter: `Force=false` is published.
4. The executor, when it receives a command with `Force=true`, derives a salted `TaskIdentifier` distinct from the canonical one for the same `(owner, repo, number, sha)` tuple. The salt is a nonce derived from the current time obtained via an injected `libtime.CurrentDateTimeGetter` — no `time.Now()` call in business logic.
5. The executor, when it receives a command with `Force=false`, calls the existing `pkg.DeriveTaskID(owner, repo, number, sha)` unchanged. The published `CreateTaskCommand` is byte-identical to today's output for the same inputs.
6. Two `force=true` triggers fired against the same PR with the same head SHA, separated by enough wall-clock distance for the nonce to differ, produce two distinct `TaskIdentifier` values and therefore two distinct downstream vault files.
7. The `Force` parameter parsing tolerates the standard truthy/falsy values accepted by `libparse.ParseBoolDefault` (the same parser used elsewhere in maintainer for query-param booleans). An unparseable value (e.g. `force=banana`) falls back silently to the default `false` and produces a 202 with the non-force command published — pinned in Constraints. This is the lenient default chosen to (a) avoid surprising operators on a typo, (b) match `libparse.ParseBoolDefault`'s native contract, and (c) preserve idempotency of the non-force path.
8. The metric label emitted by the executor for a `force=true` trigger that successfully publishes is the existing `create` label — no new label is introduced. Forced reruns are accounted as ordinary creates.

## Constraints

- `pkg.DeriveTaskID` signature, namespace UUID (`prWatcherNamespace`), and input encoding (`"<owner>/<repo>#<number>@<sha>"`) are frozen. Do NOT modify.
- `TriggerPRReviewCommand` struct field set, JSON tags, and `Validate` rules are frozen. The `Force` field already exists; do not rename, retype, or drop it.
- `TriggerPRReviewCommandOperation` wire string `"trigger-pr-review"` is frozen.
- `SinglePRTriggerHandler` type alias (`libhttp.WithError`) and `NewSinglePRTriggerHandler` constructor name are frozen. The constructor's exported signature may be extended only if existing callers in `main.go` and `pkg/factory/` are updated within the same prompt.
- The HTTP response shape on the success path (`{"status":"accepted","url":<raw>}`, HTTP 202) is frozen — no new fields added when `force=true`.
- `make precommit` in `watcher/github-pr/` must exit 0 with no new lint warnings.
- Existing tests in `watcher/github-pr/pkg/taskid_test.go` and `watcher/github-pr/pkg/watcher_test.go` must pass unmodified — they pin the non-force path.
- The `libtime.CurrentDateTimeGetter` from `github.com/bborbe/time` is the only allowed clock source in business logic (see `~/.claude/plugins/marketplaces/coding/docs/go-time-injection.md`). No direct `time.Now()` in executor or helper code.
- Query-param boolean parsing uses `libparse.ParseBoolDefault` from `github.com/bborbe/parse` (see `~/.claude/plugins/marketplaces/coding/docs/go-parse-pattern.md`).
- CQRS layering rules from `~/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` apply: HTTP handler stays thin (parse → publish); business logic lives in the executor.
- Unparseable `force` query-param values resolve to `false` (lenient default via `libparse.ParseBoolDefault`). The handler MUST NOT return HTTP 400 for an unparseable `force` value; the request proceeds as if `force` were omitted. This decision is operator-facing and frozen here, not deferred to test-commit time.
- The time-derived nonce uses nanosecond resolution: `strconv.FormatInt(currentDateTimeGetter.Now().UnixNano(), 10)`. This is fine enough that any human-driven trigger cadence (≥1 ms apart) produces distinct nonces, and on a clock-frozen test the nonce is deterministic per fixed clock value.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility |
|---------|-----------|-------------------|----------|---------------|
| `force=true` with otherwise-valid URL, executor publishes successfully | `github_pr_published{result="create"}` increments; new vault file appears with salted identifier | New `CreateTaskCommand` sent with salted `TaskIdentifier`; downstream controller creates a fresh vault file; prior file untouched | None — intended behavior | Reversible only by deleting the new vault file manually |
| `force=true` but URL parse / filter / trust fails | Same skip / error paths as today | Identical to `force=false` path: skip → `cdb.ErrCommandObjectSkipped`; transient error → wrapped error retried by Kafka. `Force` does not bypass URL validation, filter, or trust. | None — handled | Same as today |
| `force=garbage` (unparseable bool) | Handler-side `libparse.ParseBoolDefault` returns default `false` | Treated as `force=false` (lenient default per Constraints); 202 returned, non-force command published. Pinned in Constraints; unit test asserts the lenient path. | None — handled | Reversible (operator retries with `force=true`) |
| Two `force=true` triggers fire so close together that the time-derived nonce collides | Both reach executor within the nonce's resolution window | Both produce the *same* salted `TaskIdentifier`; the second is deduped by the controller (existing skip path); only one new vault file is created | Operator retries after a short delay; nonce resolution is fine enough that human-driven trigger cadence rarely collides | Reversible — the second trigger silently no-ops, no corruption |
| `force=true` with a URL that is not a GitHub PR URL | URL validation in handler returns HTTP 400 | Same rejection as `force=false`; `Force` does not bypass URL validation | Operator fixes URL | Reversible |
| Kafka send failure when publishing `TriggerPRReviewCommand` from handler | Sender returns error; handler returns 502 | Same as today: HTTP 502, no `CreateTaskCommand` published, no metric increment | Operator retries | Reversible |
| Executor crashes between deriving the salted ID and publishing `CreateTaskCommand` | Framework emits Failure on result topic; Kafka redelivers | Same `TriggerPRReviewCommand` is redelivered; executor re-derives a salted ID. The re-derivation uses the *new* current time, so the salted ID differs from the crashed attempt. The downstream controller will create a new vault file under the second identifier; the first identifier never produced a vault file. No duplicate file. | None — handled by framework | Partial — at-least-once delivery: one identifier is consumed without producing a vault file, the second produces the actual file. Not user-visible. |
| Clock jump backward between two `force=true` triggers | Time-derived nonce may decrease | Two distinct nonces still likely (resolution finer than the jump); even on collision, see "nonce collides" row above | None — handled | Reversible |

## Security / Abuse Cases

- `/trigger` remains behind `/admin` (out of scope: removing that gating). The `force` parameter adds no new trust boundary.
- An attacker with `/admin` access can flood `force=true` to create unbounded vault files for the same PR. Mitigation is the existing `/admin` access control plus a sibling rate-limiting task — not this spec.
- The `force` parameter is a single boolean; no path, no shell, no template injection surface.
- The salted identifier is still a UUID5 — same shape, same length, same wire encoding as the canonical one. Downstream code that treats `TaskIdentifier` as an opaque UUID is unaffected.
- The time-derived nonce is not security-sensitive (it is not a secret, not a token); it only needs uniqueness across human-trigger cadence. Predictability is acceptable.

## Acceptance Criteria

- [ ] `cd watcher/github-pr && make precommit` exits 0 — evidence: exit code 0
- [ ] `grep -nE 'libparse\.ParseBoolDefault|FormValue\("force"\)' watcher/github-pr/pkg/handler/trigger_handler.go` matches at least one line — evidence: grep prints ≥1 line
- [ ] `grep -n 'Force:' watcher/github-pr/pkg/handler/trigger_handler.go` shows the handler sets `Force` from the parsed value (not a hardcoded `false`) — evidence: grep prints a line where `Force:` is followed by an identifier or function call, not the literal `false`
- [ ] A new exported helper exists in `watcher/github-pr/pkg/` (recommended name `DeriveTaskIDForce`) with signature `func(owner, repo string, number int, sha, nonce string) uuid.UUID` — evidence: `grep -nE 'func DeriveTaskIDForce' watcher/github-pr/pkg/taskid.go` returns one line; signature matches
- [ ] Unit test `TestDeriveTaskIDForce_DiffersFromCanonical` (or Ginkgo equivalent) asserts that for the same `(owner, repo, number, sha)` inputs, `DeriveTaskID` and `DeriveTaskIDForce(..., nonce="x")` produce *different* UUIDs — evidence: `go test ./pkg -run DeriveTaskIDForce -v` shows PASS
- [ ] Unit test `TestDeriveTaskIDForce_StableForSameNonce` asserts that two calls to `DeriveTaskIDForce` with identical inputs (including nonce) return equal UUIDs — evidence: assertion in test source, test PASS
- [ ] Unit test `TestDeriveTaskIDForce_DiffersAcrossNonces` asserts that two calls with different nonces but identical `(owner, repo, number, sha)` return different UUIDs — evidence: assertion in test source, test PASS
- [ ] Unit test `TestTriggerHandler_ParsesForceTrue` asserts that `POST /trigger?url=<valid>&force=true` results in the sender receiving a `TriggerPRReviewCommand` with `Force == true` — evidence: mock sender's captured command field assertion
- [ ] Unit test `TestTriggerHandler_ParsesForceFalse` asserts that `POST /trigger?url=<valid>&force=false` and `POST /trigger?url=<valid>` (no param) both produce `Force == false` — evidence: two assertions in test source
- [ ] Unit test `TestTriggerHandler_ParsesForceGarbage` asserts that `POST /trigger?url=<valid>&force=banana` returns HTTP 202, publishes a `TriggerPRReviewCommand` with `Force == false`, and does NOT return 400 — evidence: response code assertion + captured-command field assertion + negative assertion that HTTP 400 was not returned
- [ ] Unit test `TestExecutor_ForceTrueUsesSaltedID` asserts that when the executor processes a command with `Force=true`, the `CreateTaskCommand` published downstream carries a `TaskIdentifier` *different* from `pkg.DeriveTaskID(owner, repo, number, sha)` for the same inputs — evidence: mock create-sender's captured command's `TaskIdentifier` assertion
- [ ] Unit test `TestExecutor_ForceFalseUsesCanonicalID` asserts that when `Force=false`, the published `TaskIdentifier` *equals* `pkg.DeriveTaskID(owner, repo, number, sha)` — evidence: mock create-sender's captured command's `TaskIdentifier` assertion
- [ ] Unit test `TestExecutor_ForceFalseProducesIdenticalCreateCommand` asserts that the entire `CreateTaskCommand` produced by `Force=false` is byte-identical (via deep equality or marshalled JSON compare) to the command produced by the executor at master HEAD for the same fixture — evidence: deep-equal assertion in test source; fixture documented inline
- [ ] Unit test `TestExecutor_TwoForceTriggersProduceDifferentIDs` asserts that two consecutive `Force=true` invocations of the executor, with the clock advanced via the injected `CurrentDateTimeGetter` between them, produce two distinct `TaskIdentifier` values — evidence: mock create-sender captures two commands; assertion on identifier inequality
- [ ] No call to `time.Now()` exists in business-logic code paths (executor, helper, factory). `grep -nE '\btime\.Now\(\)' watcher/github-pr/pkg/command/ watcher/github-pr/pkg/taskid.go watcher/github-pr/pkg/factory/` returns no matches — evidence: grep exit code 1
- [ ] Unit test `TestExecutor_ForceTrueIncrementsCreateLabel` asserts that on a successful `Force=true` publish, the executor increments `github_pr_published{result="create"}` exactly once and does NOT register a new label value (no `result="force"`, no `result="forced_create"`, etc.) — evidence: mock `pkg.Metrics`'s captured label values; assertion that the captured label set equals the canonical `{"create"}` and contains no force-suffixed label
- [ ] `git diff master -- watcher/github-pr/pkg/metrics.go` shows the metric label set (the slice initialized at `pkg/metrics.go:38`) unchanged — no new label string added — evidence: diff inspection
- [ ] `git diff master -- watcher/github-pr/pkg/watcher.go` shows no functional change to the poll path's `DeriveTaskID` call site (line ~279 today). Whitespace-only diff is acceptable; any logic change fails this AC — evidence: diff inspection, line-by-line review
- [ ] `git diff master -- watcher/github-pr/pkg/taskid.go` shows the `DeriveTaskID` function body and `prWatcherNamespace` constant unchanged (only additions for `DeriveTaskIDForce`) — evidence: diff inspection
- [ ] `CHANGELOG.md` at repo root contains a new bullet under `## Unreleased` describing the `?force=true` query parameter and its salted-identifier behavior — evidence: `grep -A 30 '^## Unreleased' CHANGELOG.md` shows a bullet starting `- ` mentioning `force` and `trigger`

## Verification

```
cd watcher/github-pr
make precommit
go test ./pkg -run 'DeriveTaskIDForce|Trigger' -v
go test ./pkg/handler -run 'ParsesForce' -v
go test ./pkg/command -run 'Force' -v
grep -nE 'libparse\.ParseBoolDefault|FormValue\("force"\)' pkg/handler/trigger_handler.go
grep -nE 'func DeriveTaskIDForce' pkg/taskid.go
! grep -rnE '\btime\.Now\(\)' pkg/command/ pkg/taskid.go pkg/factory/
cd ../..
grep -A 30 '^## Unreleased' CHANGELOG.md
```

Expected:
- `make precommit` exits 0
- Test runs show each named case PASS
- Grep for parse helper returns ≥1 line (exit 0)
- Grep for `DeriveTaskIDForce` returns exactly one line (exit 0)
- `! grep` for `time.Now()` exits 0 only when grep itself found nothing — propagates the AC's required negative evidence; if any `time.Now()` is present the line exits non-zero and the verification block fails
- CHANGELOG grep shows the new bullet

**Verification rungs (per `docs/verifying-specs.md`):**
- Rung 1 (unit tests via `make precommit`) is the primary and sufficient surface. All behavior changes (handler parse, executor branch, helper derivation, time injection) are reachable by unit tests with mocked sender, mocked clock, and mocked create-sender.
- Rung 2 (dev k8s deploy + e2e) is NOT required. No new env var, no Dockerfile change, no k8s manifest change beyond what spec 066 already shipped. The `Force` field is already on the wire schema.
- Rung 3 (prod) — operator-initiated manual verification (hitting prod `/admin/.../trigger?url=<existing-reviewed-PR>&force=true` and observing a new vault file) is OPTIONAL post-deploy validation, not a required AC. It is recorded in the spec log on first prod use, not gated on merge.

## Suggested Decomposition

Two layers + helper + factory wiring + changelog = 4 concerns. Decompose into 3 prompts:

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Add `DeriveTaskIDForce` helper + its unit tests. No callers wired yet. Pure additive change to `pkg/taskid.go` and `pkg/taskid_test.go`. | 4 (helper exists), 6 (different nonces → different IDs) | helper-existence AC, three `DeriveTaskIDForce_*` test ACs, `time.Now` grep AC, `DeriveTaskID unchanged` diff AC | — |
| 2 | Plumb `libtime.CurrentDateTimeGetter` into the executor via the factory; branch executor on `cmd.Force`; call `DeriveTaskIDForce` with a time-derived nonce (nanosecond resolution per Constraints) when `Force=true`; otherwise call `DeriveTaskID`. Update factory and `main.go` wiring. Tests on the executor cover force vs non-force divergence, byte-identity of non-force, distinct IDs across two forced calls with advanced clock, and metric-label invariance. | 4 (executor uses salted ID when Force=true), 5 (executor uses canonical when Force=false), 6 (distinct IDs across clock advance), 8 (metric label unchanged) | the four `TestExecutor_*` ACs, byte-identical non-force AC, `TestExecutor_ForceTrueIncrementsCreateLabel` AC, `metrics.go` diff AC | prompt 1 |
| 3 | Parse `force` query param in `pkg/handler/trigger_handler.go` via `libparse.ParseBoolDefault`; set `Force` on the published command. Handler unit tests cover truthy / falsy / absent / unparseable values. Add CHANGELOG bullet under `## Unreleased`. | 1, 2, 3, 7 (handler parsing variants) | three `TestTriggerHandler_*` ACs, `make precommit`, handler grep ACs, CHANGELOG bullet AC | prompt 2 |

Rationale: prompt 1 is the smallest reviewable diff (one helper, deterministic tests, no wiring). Prompt 2 owns the cross-layer wiring (factory → executor → clock) and the byte-identity guarantee for the non-force path — this is the riskiest seam and gets its own prompt to keep review focus tight. Prompt 3 is the HTTP-edge change, fully reachable once the executor accepts and acts on the field; bundling the changelog here keeps the user-visible-behavior commit and the user-facing release note in the same prompt. Ordering prevents cycles: prompt 2 cannot test "force uses salted ID" without the helper from prompt 1; prompt 3 cannot meaningfully test end-to-end handler behavior without the executor branch from prompt 2.

## Do-Nothing Option

Leave `/trigger` as-is. Operators continue to have no way to re-run a review against an already-reviewed SHA. The recurring workarounds — pushing a no-op commit to change the SHA, manually editing or deleting vault files, restarting the pod — are all higher-effort and higher-risk than a one-character query-param flip. The cost of doing nothing is recurring operator friction during every prompt-template change, reviewer-config tweak, or post-incident replay. The cost of doing it is ~3 small prompts, all bounded by unit tests with no production risk on the non-force path. Not acceptable to leave undone.
