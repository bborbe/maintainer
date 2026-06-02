---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-06-02T19:06:30Z"
generating: "2026-06-02T19:16:26Z"
prompted: "2026-06-02T19:34:40Z"
verifying: "2026-06-02T20:19:41Z"
branch: dark-factory/manifest-complexity-refactor
---

## Summary

- Two functions in the github-releaser plugin manifest rewriter currently carry blanket `//nolint:gocognit,gocyclo,funlen` annotations added on commit `c92bc64` as a parking spot.
- This spec retires those nolints by splitting the two functions into smaller, independently testable units while keeping their externally visible behavior byte-for-byte identical.
- No public API changes, no behavior changes, no new lint rules — pure refactor under the existing test suite.
- Scope is limited to the two manifest functions; the third nolint site (`executeDirectPush`) is owned by a separate in-flight spec and is explicitly out of scope here.
- The lint suppressions disappear from the source tree as the observable success signal.

## Problem

The github-releaser plugin manifest rewriter contains two functions whose complexity metrics exceed the project's enforced thresholds (gocognit 20, gocyclo 30, funlen 80 lines / 50 stmts). They were marked `//nolint:gocognit,gocyclo,funlen` on commit `c92bc64` to unblock a local lint run; the suppressions were always intended as parking spots, not verdicts. Both functions are JSON-streaming state machines whose scope-tracking flags and value-rewrite logic live entangled in a single body. They work, they are well tested, but any future change to the marketplace.json shape (new key, nested plugin object) compounds the cost because the state graph lives in the developer's head, not the code. Every nolint left in place is a small invitation for the next one.

## Goal

After this work, the two complexity-suppressed functions in `agent/github-releaser/pkg/plugin/manifest.go` exist with their current exported signatures and produce byte-identical output for every input the existing test suite covers, but their bodies fall under the project's enforced complexity thresholds without `//nolint` suppressions. The package coverage stays at or above its current baseline, and `make precommit` in the github-releaser module passes clean.

## Non-goals

- Do NOT change the public API of either function — signatures and exported names are frozen.
- Do NOT modify output bytes for any input the current test suite exercises — this is a pure refactor.
- Do NOT touch `executeDirectPush` in `pkg/steps_execution.go` — owned by spec 058, which naturally splits it.
- Do NOT remove or weaken `//nolint` directives anywhere else in the maintainer repo — this spec only addresses the two manifest sites.
- Do NOT introduce new lint rules, raise thresholds, or relax existing ones to make the refactor pass.
- Do NOT rename the exported functions or change their package location.
- Do NOT switch the rewrite implementation from line-based streaming to a full JSON decode/encode round-trip — byte-level structure preservation (indentation, key order, trailing newline) is load-bearing per spec 056.
- Do NOT add the golines `//nolint` line-affinity config tweak — deferred to a separate concern noted on the parent task.
- Do NOT add behavior toggles, feature flags, or compatibility shims — invariant; if a future consumer demands variation, that's a separate spec.

## Desired Behavior

1. The two complexity-suppressed functions in `pkg/plugin/manifest.go` no longer carry `//nolint:gocognit`, `//nolint:gocyclo`, or `//nolint:funlen` directives, individually or combined.
2. Running `golangci-lint` over the `pkg/plugin/` package reports zero complexity, length, or nesting issues against either function.
3. Every existing `It` block in `manifest_test.go` passes without modification — the test file's content is unchanged except, optionally, for new tests added to cover newly extracted helpers.
4. Coverage on `pkg/plugin/...` stays ≥ 94.9% (pinned baseline established on commit `c92bc64`, verified locally 2026-06-02).
5. `go test -race -count=1 ./pkg/plugin/...` passes — proves no shared-state hazards introduced by any extracted helpers.
6. Any helpers introduced during the refactor are exercised by their own focused unit tests covering their boundary conditions, not only transitively via the two top-level functions.
7. `make precommit` in `agent/github-releaser/` exits clean — no lint, no test failure, no coverage regression.

## Constraints

- Public API frozen: the exported `BumpMarketplaceJSON` signature `(ctx, content, version) → (bytes, error)` and the `rewriteVersionValue` signature stay exactly as they are today. Callers compile unchanged.
- Byte-equivalence frozen: for every input covered by the existing test suite, output bytes are character-for-character identical to the pre-refactor implementation. The test suite is the regression net.
- Coverage floor frozen: Coverage MUST stay ≥ 94.9% on `pkg/plugin/...` as measured by `go test -cover` (baseline established on commit `c92bc64`, verified locally 2026-06-02). This is the verifier's threshold — not a moving target.
- Spec 056 byte-structure invariants apply: indentation, key order, trailing newline, and one-line-per-field diff shape are preserved. See spec 056 (`056-plugin-version-bump.md`) for the rewrite contract this refactor must preserve.
- Helper visibility: any new helpers extracted in service of the refactor are unexported unless they need to be exercised from outside the package (which the existing test layout does not require).
- No dependency on spec 058 — that spec touches `steps_execution.go`, `steps_planning.go`, `steps_ai_review.go`; this spec touches only `pkg/plugin/manifest.go` and its test file. Both can ship in parallel.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility |
|---------|-------------------|----------|-----------|---------------|
| Refactor changes output bytes for a covered input | Existing Ginkgo assertions fail at byte/string diff level | Inspect diff, restore byte-equivalent path; if equivalence cannot be reached, revert PR | `make test` in `agent/github-releaser/` reports failing `It` block | Reversible — `git revert` |
| Extracted helper exported when it has no external caller | Reviewer flags as unnecessary API surface | Lowercase the identifier; rerun `make precommit` | `/coding:pr-review` or `golangci-lint` `unused`/`revive` | Reversible — rename in-place |
| Coverage drops because new helpers are not independently tested | Precommit coverage gate reports regression vs baseline | Add focused unit tests for the helpers; rerun precommit | `go test -cover ./pkg/plugin/...` output below baseline | Reversible — add tests |
| Helper extraction introduces a goroutine or mutable shared state hazard | `-race` flag surfaces a data race | Remove shared mutability, make helpers pure | `go test -race -count=1 ./pkg/plugin/...` fails | Reversible — revert helper |
| `//nolint` removed but a different lint (e.g. `nestif`) now fires on a residual nested block | `golangci-lint` reports new issue | Continue splitting until the residual block passes — do NOT re-add `//nolint` to silence it | `golangci-lint run` on `pkg/plugin/` | Reversible — keep splitting or revert |
| New helpers shadow or duplicate logic that already lives elsewhere in `pkg/plugin` | Reviewer or duplicate-code linter flags | Consolidate into a single helper | Code review / `dupl` lint output | Reversible — dedupe |

## Security / Abuse Cases

Not applicable — pure internal refactor of in-process byte transforms over content already loaded into memory by callers under existing trust assumptions. No new I/O, no new user-input surface, no new trust boundary crossings.

## Acceptance Criteria

- [ ] `grep -c '//nolint:\(gocognit\|gocyclo\|funlen\)' agent/github-releaser/pkg/plugin/manifest.go` outputs `0` — evidence: stdout literal `0` (unambiguous, `set -e` safe; avoids relying on grep's exit-1 no-match semantics)
- [ ] `cd agent/github-releaser && golangci-lint run ./pkg/plugin/...` exits 0 — evidence: exit code
- [ ] `cd agent/github-releaser && golangci-lint run ./pkg/...` exits 0 — evidence: exit code (proves removing nolints does not surface new failures elsewhere)
- [ ] `cd agent/github-releaser && go test -count=1 ./pkg/plugin/...` exits 0 with the pre-existing test file unmodified except for additive helper-test blocks — evidence: exit code; `git diff` on `manifest_test.go` shows no removals or rewrites of existing `It` blocks
- [ ] `cd agent/github-releaser && go test -race -count=1 ./pkg/plugin/...` exits 0 — evidence: exit code
- [ ] `cd agent/github-releaser && go test -cover ./pkg/plugin/...` reports coverage ≥ 94.9% on `pkg/plugin/...` (pinned baseline, commit `c92bc64`, verified 2026-06-02) — evidence: stdout coverage percentage ≥ 94.9
- [ ] Each extracted helper has a Ginkgo `Describe(...)` block whose first argument string contains the helper's exact Go identifier as a verbatim substring (e.g. `Describe("scopeTracker", ...)` for a `scopeTracker` helper) — evidence: `grep -E 'Describe\("([A-Za-z_][A-Za-z0-9_]*)"' pkg/plugin/manifest_test.go` returns one match per new helper identifier, and `go test -run` over that identifier pattern exits 0
- [ ] `cd agent/github-releaser && make precommit` exits 0 — evidence: exit code

Scenario coverage: NO new scenario. This is a pure refactor; unit + integration tests in the implementation prompt(s) cover the surface. The `make precommit` AC is the integration-level gate.

## Verification

```
cd agent/github-releaser
grep -c '//nolint:\(gocognit\|gocyclo\|funlen\)' pkg/plugin/manifest.go   # expect: stdout `0`
golangci-lint run ./pkg/plugin/...                                     # expect: exit 0
golangci-lint run ./pkg/...                                            # expect: exit 0
go test -race -count=1 ./pkg/plugin/...                                # expect: exit 0
go test -cover ./pkg/plugin/...                                        # expect: coverage ≥ baseline
make precommit                                                         # expect: exit 0
```

## Do-Nothing Option

The two functions work today and are covered by tests. Doing nothing means the `//nolint` directives stay as load-bearing parking spots. The cost is incremental: every future edit to the marketplace.json shape (new key, nested plugin object, schema bump from spec 056 follow-ups) compounds against an unsplit state machine whose graph lives only in the next developer's head. The CI matrix gap that originally hid the complexity has been closed on master, so the suppressions cannot quietly grow — but they also will not shrink on their own. The do-nothing option is acceptable; this spec is the deliberate choice to spend bounded refactor effort now instead of paying the unbounded amortized cost later.
