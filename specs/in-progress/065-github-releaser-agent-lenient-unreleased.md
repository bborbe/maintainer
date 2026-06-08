---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-06-08T15:09:04Z"
generating: "2026-06-08T15:09:15Z"
prompted: "2026-06-08T15:31:16Z"
branch: dark-factory/github-releaser-agent-lenient-unreleased
---

## Summary

- Spec 064 made the github-release **watcher**'s changelog parser lenient. Live-fire on 2026-06-08T14:36:40Z proved the **agent** has its own parser (`agent/github-releaser/pkg/changelog/changelog.go`) that ALSO strict-matches `## Unreleased`, so the agent silently rejected vault-cli's `## unreleased` after the watcher correctly fired.
- This spec extends the same lenient rule to the agent: the first H2 that is NOT a version header (`vX.Y.Z` / `X.Y.Z`) is the unreleased section. Five exported functions and one internal helper are updated.
- The rewrite step still emits canonical output (`## vX.Y.Z`) regardless of the input heading text — lenient on input, canonical on output.
- All existing tests for the literal `## Unreleased` continue to pass unmodified. New table-driven cases cover each lenient variant.
- Verification rung 1 (unit tests) + rung 2 (dev k8s redeploy and re-fire vault-cli's stuck Kafka task) + rung 3 (prod observes a tag cut for vault-cli@78ea51a). The live-fire failure that motivated this spec doubles as the rung-3 proof.

## Problem

After spec 064 shipped, vault-cli@78ea51a pushed a CHANGELOG with `## unreleased` (lowercase). The watcher detected it and published a Kafka task with taskID `9c39e665-b07b-5d09-af63-a5d22758154b` at 14:36:40Z. The agent's planning step in `steps_planning.go:131` invoked `changelog.ValidateUnreleased`, which still strict-matches `headingText == "Unreleased"` inside `findFirstAndUnreleased`. The verdict came back `valid=false reason="Unreleased section not found."` and the step exited with `precondition=P2_unreleased_empty`. No release was cut. The bug class spec 064 set out to close — "typo'd heading silently halts release" — is still open at the agent layer. The parser parity between watcher and agent is broken, so the spec-064 invariant is unobservable end-to-end.

## Goal

The agent's changelog parser recognizes the unreleased section by the same structural rule the watcher uses: the FIRST `## ` heading that does NOT match a version-header pattern (`vX.Y.Z` / `X.Y.Z`) is the unreleased section. Every function in `agent/github-releaser/pkg/changelog/changelog.go` that today gates on `headingText == "Unreleased"` uses the lenient rule instead. The rewrite step continues to emit the canonical `## vX.Y.Z` heading regardless of the input variant, so post-release CHANGELOGs remain canonical.

## Non-goals

- Do NOT touch `watcher/github-release/` — already lenient via spec 064.
- Do NOT change the bump classifier prompt or its output schema.
- Do NOT add a config flag to toggle lenient vs strict — invariant; if a future consumer demands strict matching, that's a separate spec.
- Do NOT change the Pattern B Job contract, the agent's JSON verdict shape, or any Kafka schema.
- Do NOT remove `ErrorCategoryUnreleasedNotFound` in `git/error_classifier.go` — the category remains valid for the truly-empty case (no `## ` H2 anywhere).
- Do NOT change how `RewriteUnreleasedHeader` formats its output — the heading on disk after rewrite is always `## vX.Y.Z`, never the lenient input variant.
- Do NOT change `ExtractSectionBody`'s contract when called with a version-string heading (e.g. `"v1.2.8"`) — version-heading lookup remains exact-match; only the "Unreleased" lookup path becomes lenient.

## Desired Behavior

1. `ValidateUnreleased` returns `valid=true` when the first H2 is any non-version-header text (e.g. `## Unreleased`, `## unreleased`, `## Unreleased changes`, `## WIP`, `## Next`) AND that section contains at least one `- ` bullet.
2. `ValidateUnreleased` returns `valid=false reason="Unreleased is not the first ## section; …"` when a version header (`## v0.35.0`) is the first H2 — version-header path still wins over any non-version heading appearing later.
3. `ValidateUnreleased` returns `valid=false reason="Unreleased section has no bullet entries."` when the lenient-detected unreleased section has zero `- ` bullets before the next `## ` heading or EOF.
4. `ValidateUnreleased` returns `valid=false reason="Unreleased section not found."` when there are NO `## ` headings at all OR every `## ` heading is a version header.
5. `ExtractUnreleasedBullets` returns the bullets under the lenient-detected unreleased section. `nil` when no non-version H2 exists. Non-nil empty slice when the section exists but has zero bullets.
6. `ExtractUnreleasedBody` returns the verbatim body block under the lenient-detected unreleased section, terminating at the next `## ` heading or EOF. Returns a wrapped error when no non-version H2 exists.
7. `ReplaceUnreleasedBody` swaps the body under the lenient-detected unreleased section. The heading line itself is preserved VERBATIM — input `## unreleased` stays `## unreleased`, input `## WIP` stays `## WIP`. The rewrite step that follows is responsible for canonicalizing the heading separately.
8. `RewriteUnreleasedHeader` replaces the lenient-detected unreleased heading line with `newHeader` (typically `## vX.Y.Z`). Input `## unreleased` / `## WIP` / `## Next` all become `## vX.Y.Z`. Output is canonical regardless of input.
9. Subsequent non-version H2 headings AFTER the first one do NOT re-open unreleased state. Once a version header has been emitted (rewrite) or the section closes, the second non-version H2 is treated as a normal heading boundary, not as another unreleased section.
10. `InferHeaderPrefixStyle` skips the lenient-detected unreleased heading and infers the prefix style from the FIRST version-header H2 it encounters (unchanged outcome for canonical inputs; just no longer keyed off the literal `"Unreleased"`).

## Constraints

- Exported function signatures are FROZEN:
  - `ValidateUnreleased(content []byte) (valid bool, reason string, line int)`
  - `ExtractUnreleasedBullets(content []byte) []string`
  - `ExtractUnreleasedBody(ctx context.Context, content []byte) (string, error)`
  - `ExtractSectionBody(ctx context.Context, content []byte, heading string) (string, error)`
  - `ReplaceUnreleasedBody(ctx context.Context, content []byte, newBody string) ([]byte, error)`
  - `RewriteUnreleasedHeader(ctx context.Context, content []byte, newHeader string) ([]byte, error)`
  - `InferHeaderPrefixStyle(content []byte) string`
- Add a non-exported helper `isVersionHeader(headingText string) bool` whose regex matches `^v?\d+\.\d+\.\d+$` — parity with the watcher's helper of the same name.
- `ExtractSectionBody` retains exact-match semantics when called with a heading argument that is NOT the literal `"Unreleased"` (e.g. `"v1.2.8"`). Only `ExtractUnreleasedBody`'s wrapper layer becomes lenient. This preserves the post-release re-extract path in `steps_ai_review.go:463`.
- The error message returned by `ReplaceUnreleasedBody` / `RewriteUnreleasedHeader` / `ExtractUnreleasedBody` when no unreleased section is found MUST continue to contain the substring `"unreleased header not found"` (lowercase) so the existing `ErrorCategoryUnreleasedNotFound` classifier in `git/error_classifier.go` continues to match.
- All existing tests in `agent/github-releaser/pkg/changelog/*_test.go` that exercise the literal `## Unreleased` must pass UNMODIFIED — no happy-path regression.
- `make precommit` in `agent/github-releaser/` must pass with no new lint warnings.
- The `heading` struct and `findFirstAndUnreleased`'s signature are internal — they MAY change.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Concurrency |
|---|---|---|---|---|
| CHANGELOG has no `## ` heading at all | `ValidateUnreleased` returns `false, "Unreleased section not found.", 0`; `Extract*` returns nil/error; rewrite returns wrapped error containing `"unreleased header not found"` | Author adds a heading | Planning step logs `precondition=P2_unreleased_empty reason="Unreleased section not found."` | N/A — pure parsing |
| First H2 is a version header, no non-version H2 anywhere | `ValidateUnreleased` returns `false, "Unreleased is not the first ## section; found 'v0.35.0' at line N. Move ## Unreleased above all release headings.", N` | Author adds an unreleased section above the version | Planning step logs the reason verbatim | N/A |
| First H2 is `## WIP` with zero bullets | `ValidateUnreleased` returns `false, "Unreleased section has no bullet entries.", N` | Author adds a bullet | Planning step logs the reason verbatim | N/A |
| Two consecutive non-version H2s (`## Unreleased` then `## Next`) | First one wins as unreleased; second is a section boundary — its bullets are NOT counted under unreleased | Author consolidates to one section | Bullet count surfaces in plan-step output | N/A |
| Input `## unreleased`, planning succeeds, execution invokes `ReplaceUnreleasedBody` then `RewriteUnreleasedHeader` | After both calls, the on-disk CHANGELOG starts with `## vX.Y.Z` followed by the new body, then the prior history | Manual fix if mid-write crashes (see next row) | `git diff` shows `## unreleased` → `## vX.Y.Z` | Mid-write crash: file write is atomic (single `os.WriteFile` in execution step); partial state is impossible. If the agent crashes between `ReplaceUnreleasedBody` and `RewriteUnreleasedHeader`, only the in-memory `rewritten` byte slice is lost — the on-disk file is untouched. Retry from scratch on the next task. |
| `ExtractSectionBody(ctx, content, "v1.2.8")` called after release to re-extract the just-cut version body | Exact-match path used; lenient rule does NOT apply when heading argument is a version string | None — works as today | `steps_ai_review.go` log shows extracted body | N/A |
| CHANGELOG ends mid-section (no trailing newline) | All functions preserve the trailing-newline-less convention as today | None — handled | Byte-equal diff | N/A |

## Security / Abuse Cases

- The input is a CHANGELOG.md byte slice fetched from a git remote the agent already trusts (the bborbe-org repos it monitors). No new trust boundary is introduced.
- A maliciously-crafted heading (e.g. `## $(rm -rf /)`) flows only through string comparison and `bytes.Buffer` writes; no shell, no eval, no path traversal. Risk: same as today.
- A pathologically long heading line is bounded by `bufio.Scanner`'s default 64KB token limit — same backstop as the watcher.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0 — evidence: exit code 0
- [ ] `go test ./pkg/changelog -v` passes; test output includes named Ginkgo cases `literal_Unreleased`, `lowercase_unreleased`, `extended_Unreleased_changes`, `WIP_heading`, `Next_heading`, `version_header_first_no_unreleased`, `empty_lenient_section`, `two_non_version_h2s_first_wins` — evidence: test output shows each named case PASS
- [ ] Test case `literal_Unreleased` asserts `ValidateUnreleased` returns `(true, "", 0)` and `ExtractUnreleasedBullets` returns the fixture's bullets in order — evidence: assertion in test source
- [ ] Test case `lowercase_unreleased` (fixture: `## unreleased` + bullets) asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ExtractUnreleasedBody` returns the fixture body — evidence: assertion in test source
- [ ] Test case `extended_Unreleased_changes` (fixture: `## Unreleased changes` + bullets) asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ExtractUnreleasedBullets` returns the bullets — evidence: assertion in test source
- [ ] Test case `WIP_heading` (fixture: `## WIP` + bullets) asserts `ValidateUnreleased` returns `(true, "", 0)` AND `ReplaceUnreleasedBody` preserves the `## WIP` heading line verbatim while replacing the body — evidence: byte-equal assertion in test source
- [ ] Test case `version_header_first_no_unreleased` (fixture: `## v0.35.0` first, no non-version H2) asserts `ValidateUnreleased` returns `(false, "Unreleased is not the first ## section; …", lineOfV0_35_0)` — evidence: assertion in test source
- [ ] Test case `empty_lenient_section` (fixture: `## WIP` then immediately `## v0.35.0`) asserts `ValidateUnreleased` returns `(false, "Unreleased section has no bullet entries.", lineOfWIP)` — evidence: assertion in test source
- [ ] Test case `two_non_version_h2s_first_wins` (fixture: `## Unreleased` with bullets, then `## Next` with more bullets, then `## v0.35.0`) asserts `ExtractUnreleasedBullets` returns ONLY the bullets between `## Unreleased` and `## Next` — evidence: assertion in test source
- [ ] Test case `rewrite_lowercase_to_canonical` (fixture: `## unreleased` + bullets) calls `RewriteUnreleasedHeader(ctx, content, "## v0.73.0")` and asserts the result starts with `## v0.73.0\n` (NOT `## unreleased`) — evidence: byte-prefix assertion in test source
- [ ] Test case `extract_section_body_version_exact` (fixture: `## Unreleased` then `## v1.2.8` + body) calls `ExtractSectionBody(ctx, content, "v1.2.8")` and asserts exact-match still returns the v1.2.8 body — evidence: string-equal assertion in test source (proves the lenient rule does not bleed into the version-heading lookup path)
- [ ] `grep -n 'headingText == "Unreleased"' agent/github-releaser/pkg/changelog/changelog.go` returns no matches — evidence: exit code 1 (no match)
- [ ] `grep -n 'parseHeading(line) == "Unreleased"' agent/github-releaser/pkg/changelog/changelog.go` returns no matches — evidence: exit code 1 (no match)
- [ ] `grep -n 'func isVersionHeader' agent/github-releaser/pkg/changelog/changelog.go` returns exactly one match — evidence: exit code 0 with line number
- [ ] All pre-existing tests in `agent/github-releaser/pkg/changelog/*_test.go` continue to pass without source modification — evidence: `git diff` on the test files shows additions only (new lenient cases), no edits or deletions of pre-existing case bodies
- [ ] **Post-Deploy (Rung-2):** deploy the new image to dev, re-fire a Kafka task for vault-cli@78ea51a (either taskID `9c39e665-b07b-5d09-af63-a5d22758154b` or a freshly-published one). Agent planning Job exits 0 with no `needs_input` verdict.
    - deploy_target: dev (k8s ns=dev, statefulset/maintainer-agent-github-releaser)
    - deploy_check: `kubectlquant -n dev logs job/<job-name> | grep 'precondition=P2_unreleased_empty'` returns no match AND `kubectlquant -n dev get job <job-name> -o jsonpath='{.status.succeeded}'` returns `1`
- [ ] **Post-Deploy (Rung-2):** agent execution Job rewrites the CHANGELOG and pushes a commit + tag.
    - deploy_target: dev (k8s ns=dev, agent execution Pattern B Job)
    - deploy_check: `kubectlquant -n dev logs job/<exec-job-name> | grep -E 'git push|tag created'` shows a successful push line
- [ ] **Post-Deploy (Rung-3):** after prod deploy, the next polling cycle picks up vault-cli@78ea51a's `## unreleased` and cuts a tag.
    - deploy_target: prod (k8s ns=prod, statefulset/maintainer-agent-github-releaser)
    - deploy_check: `gh release list -R bborbe/vault-cli --limit 3` shows a new tag past v0.72.0 (or the current latest at deploy time) within one polling cycle
- [ ] Root `CHANGELOG.md` contains a single new bullet under `## Unreleased`: `- fix(agent/github-releaser): lenient unreleased-section detection (parity with watcher spec 064)` — evidence: `grep -A 20 '^## Unreleased' CHANGELOG.md` shows the bullet on a line starting with `- `

## Verification

```
cd agent/github-releaser
make precommit
go test ./pkg/changelog -v -ginkgo.focus="lenient"
grep -n 'headingText == "Unreleased"' pkg/changelog/changelog.go && echo "FAIL: strict literal still present" || echo "OK: strict literal removed"
grep -n 'parseHeading(line) == "Unreleased"' pkg/changelog/changelog.go && echo "FAIL: strict literal still present" || echo "OK: strict literal removed"
grep -n 'func isVersionHeader' pkg/changelog/changelog.go
cd ../..
grep -A 20 '^## Unreleased' CHANGELOG.md
```

Expected:
- `make precommit` exits 0
- Ginkgo focus shows each new lenient case PASS
- Both strict-literal greps return no match (exit 1) — prints `OK: strict literal removed` twice
- `isVersionHeader` grep returns exactly one line
- CHANGELOG grep shows the new bullet

**Rung 2 (dev k8s)**:

```
cd ~/Documents/workspaces/agent-dev
git pull && git merge master --no-edit && git push
cd task/controller && BRANCH=dev make buca
cd ../executor   && BRANCH=dev make buca
# Re-fire vault-cli task. If the prior 9c39e665 Kafka message is still on the topic, the controller will pick it up.
# Otherwise produce a fresh one by waking the watcher.
kubectlquant -n dev logs -l job-name --tail=200 --since=10m | grep -E 'P2_unreleased_empty|precondition'
kubectlquant -n dev get jobs --sort-by=.metadata.creationTimestamp | tail -5
```

Expected: no `P2_unreleased_empty` lines; latest planning + execution jobs both `SUCCEEDED 1`.

**Rung 3 (prod)**:

```
cd ~/Documents/workspaces/agent-prod
git pull && git merge master --no-edit && git push
cd task/controller && BRANCH=prod make buca
cd ../executor   && BRANCH=prod make buca
# Wait one polling cycle, then:
gh release list -R bborbe/vault-cli --limit 3
```

Expected: a new release tag past v0.72.0 within one polling cycle.

**Verification rungs (per `docs/verifying-specs.md`):**
- Rung 1: unit tests via `make precommit` in `agent/github-releaser/`. Pure parsing logic, no I/O.
- Rung 2: dev k8s deploy + re-fire vault-cli's Kafka task. Required because spec 064 waived this and live-fire then exposed the parser-parity gap — we now insist on observing the planning + execution Jobs succeed in dev before promoting.
- Rung 3: prod cutover; the existing vault-cli@78ea51a backlog is itself the proof. A new tag appearing past v0.72.0 within one polling cycle is the rung-3 evidence.

## Suggested Decomposition

Single layer (`agent/github-releaser/pkg/changelog/`); no decomposition needed across files. Suggested prompt split for the daemon (auto-generated; tune as needed):

| Prompt | Surface | Evidence |
|--------|---------|----------|
| 1 | Add `isVersionHeader` helper + refactor all 5 functions (`ValidateUnreleased`, `ExtractUnreleasedBullets`, `ExtractUnreleasedBody`, `ReplaceUnreleasedBody`, `RewriteUnreleasedHeader`) to detect via first-non-version-H2 rule; add `DescribeTable` covering ACs 1-9 | `make precommit` exit 0; new Entry rows PASS; existing tests pass unmodified; grep for strict literal returns no match |
| 2 | Add CHANGELOG bullet under `## Unreleased` | `grep '^- fix(agent/github-releaser):' CHANGELOG.md` |

Dev + prod deploy steps are out-of-band of the prompt pipeline (post-merge tail per [[Development Guide]] K8s service flow).

## Do-Nothing Option

Leave the agent's parser strict. The watcher continues to publish Kafka tasks for lowercase / WIP / Next-style headings; the agent continues to reject them. Spec 064's promise — "typo'd heading still releases" — remains observably false end-to-end. Authors will continue to silently lose releases on heading typos, and the operator burden of diagnosing "why did the tag not cut" recurs. The cost of the fix is one file change + table-driven tests + one dev/prod redeploy; the cost of do-nothing is recurring silent breakage at the system boundary the watcher cannot fix alone. Not acceptable.
