---
status: draft
spec: [063-releaser-no-major-bump]
created: "2026-06-06T21:46:08Z"
branch: dark-factory/releaser-no-major-bump
---

<summary>
- The planning step's `resolveBumpVerdict` method now prepends a `## Current version` section containing the resolved `current_version` to the assembled prompt, between the embedded rules and the `## Bullets to classify` heading
- `current_version` flows from `Run` → `runClassification` → `resolveBumpVerdict` (one new string parameter on the private helper; no public signature change)
- The `cachedBump != ""` short-circuit in `resolveBumpVerdict` is UNTOUCHED — cached verdicts still skip the LLM call (M2 cache path is unaffected; spec § Desired Behavior 5)
- The `applyMajorBumpGuard` function is byte-identical (no diff); the prompt-assembly change is upstream of the guard, never inside it
- A new Ginkgo case in `steps_planning_test.go` captures the prompt string sent to the runner mock and asserts: `## Current version` heading is present, the literal version string is present, and `## Current version` appears BEFORE `## Bullets to classify` in the string
- The runner is still called via `s.runner.Run(ctx, fullPrompt)` — no new runner method, no new context plumbing
- The `RunReturns(...)` mock pattern is unchanged — the existing ClaudeRunnerMock mock supports the assertion via `RunArgsForCall(0)` (or whichever call index is the bump call)
</summary>

<objective>
Plumb the resolved `current_version` from the task frontmatter into the bump-classification LLM call so Claude sees the version context when deciding `bump`. The new `## Current version` section is the bridge between the prompt rule shipped in prompt 1 (the cap semantics) and the LLM that enforces it. Without this injection, the rule text in the prompt is unreachable — Claude would have to guess the version from the bullets, which is precisely the false-negative class the spec 063 vault-cli incident exposed.

The change is a localized edit to `resolveBumpVerdict` in `agent/github-releaser/pkg/steps_planning.go` plus one new Ginkgo case in `steps_planning_test.go`. The guard (`applyMajorBumpGuard`), the verdict schema (`BumpVerdict`), the parser (`ParseBumpVerdict`), the cached-verdict short-circuit, the public `Run`/`ShouldRun`/`Name` shape, and the `factory.CreateAgent` wiring are all untouched in this prompt.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions.

Read these files fully BEFORE editing:

- `/workspace/agent/github-releaser/pkg/steps_planning.go` — the file under edit. The relevant function is `resolveBumpVerdict` at lines 271-300, called from `runClassification` at line 314. The `currentVersion` string is already a parameter on `runClassification` (line 305) — it just needs to be threaded one more function down. The `cachedBump != ""` short-circuit (lines 276-279) MUST stay byte-identical; only the `cachedBump == ""` branch grows. The `userMsg := strings.Join(bullets, "\n")` and `fullPrompt := prompts.BumpClassificationPrompt() + "\n\n## Bullets to classify\n\n" + userMsg` lines (lines 280-282) are the only lines that change shape.
- `/workspace/agent/github-releaser/pkg/steps_planning_test.go` — existing Ginkgo test patterns. The `Context("happy path")` block at line 42 has the closest shape to the new test (CHANGELOG fixture + runner mock + step Run + plan extraction). The `Context("bump verdict cache")` block at line 595 already uses `RunArgsForCall(2)` to inspect the prompt string sent to the runner (line 675) — mirror that style for the new test (use `RunArgsForCall(0)` because the bump call is the first call to the runner in the new test's setup). The `runClassification` → `resolveBumpVerdict` → `s.runner.Run(ctx, fullPrompt)` chain means the prompt string is exactly what the new test inspects.
- `/workspace/agent/github-releaser/pkg/prompts/bump_classification.md` — read the new pre-1.0 cap rule shipped in prompt 1. The rule references `current_version` literally; the LLM needs the section name `## Current version` to be unambiguous, and the version string to appear in the section body. The exact heading `## Current version` is the contract for the new test assertion.

Current shape of the assembly code (lines 280-282, the only lines that change):

```go
userMsg := strings.Join(bullets, "\n")
fullPrompt := prompts.BumpClassificationPrompt() +
    "\n\n## Bullets to classify\n\n" + userMsg
runResult, err := s.runner.Run(ctx, fullPrompt)
```

The new shape threads `currentVersion` through and inserts the version section between the rules and the bullets:

```go
userMsg := strings.Join(bullets, "\n")
versionSection := "\n\n## Current version\n\n" + currentVersion
fullPrompt := prompts.BumpClassificationPrompt() +
    versionSection +
    "\n\n## Bullets to classify\n\n" + userMsg
runResult, err := s.runner.Run(ctx, fullPrompt)
```

The exact section name `## Current version` and the exact heading `## Bullets to classify` are part of the new test's load-bearing assertions (placement + literal-string match). The `currentVersion` value is concatenated verbatim — no escaping, no JSON-quoting; the planning step trusts the value (it has already passed the `requiredFrontmatterFields` non-empty check at line 97-105, and `semver.BumpVersion` validates it later at line 324).

The `resolveBumpVerdict` signature changes from:

```go
func (s *planningStep) resolveBumpVerdict(
    ctx context.Context,
    bullets []string,
    cachedBump, cachedReasoning string,
) (prompts.BumpVerdict, *agentlib.Result)
```

to:

```go
func (s *planningStep) resolveBumpVerdict(
    ctx context.Context,
    bullets []string,
    currentVersion string,
    cachedBump, cachedReasoning string,
) (prompts.BumpVerdict, *agentlib.Result)
```

The call site in `runClassification` (line 314) grows one argument: `s.resolveBumpVerdict(ctx, bullets, currentVersion, cachedBump, cachedReasoning)`. The `currentVersion` parameter is already in scope in `runClassification` (declared on line 305). The `// exported function` signature is `NewPlanningStep` (line 59) which DOES NOT change — `currentVersion` is a per-run field read from the markdown, not a constructor argument.

The cached-verdict path is the FIRST two lines of `resolveBumpVerdict` (the `if cachedBump != ""` check). That block MUST stay byte-identical (per spec § Constraints and AC 5 — "the cache-hit early return is untouched"). Only the `cachedBump == ""` branch grows.

The spec's AC list for this prompt:

> AC 4: The planning step assembles the classification prompt with a `## Current version` section containing the resolved `current_version` value before the bullets — evidence: a unit test in `agent/github-releaser/pkg/steps_planning_test.go` captures the prompt string sent to the runner mock and asserts both `## Current version` and the literal version string are present and appear before the `## Bullets to classify` heading.

AC 4 is the load-bearing assertion for this prompt. The negative-evidence companion — that the cache-hit path is unchanged — is AC 5, which is verified by the diff itself (no changes inside the `if cachedBump != ""` block). Prompt 3 covers AC 5 with a dedicated test.

Coding plugin guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2/Gomega `It` style, mock inspection via `RunArgsForCall`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — error wrapping with `github.com/bborbe/errors` (no `fmt.Errorf`, no `context.Background()` in pkg/).
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — coverage rules: ≥80% on changed code; the new lines are 1-3 lines of string concatenation so the coverage bar is met if the new test calls the runner.
</context>

<requirements>

1. **Extend `resolveBumpVerdict` with a `currentVersion string` parameter.** In `/workspace/agent/github-releaser/pkg/steps_planning.go`, change the signature from `(ctx, bullets, cachedBump, cachedReasoning)` to `(ctx, bullets, currentVersion, cachedBump, cachedReasoning)`. The `cachedBump != ""` short-circuit block (lines 276-279) stays byte-identical — do NOT touch it. Only the `cachedBump == ""` branch grows.

2. **Assemble the new prompt with a `## Current version` section.** Replace the existing two lines at the top of the `cachedBump == ""` branch (currently `userMsg := strings.Join(bullets, "\n")` and the `fullPrompt := ...` concatenation) with the new four-line shape:

   ```go
   userMsg := strings.Join(bullets, "\n")
   versionSection := "\n\n## Current version\n\n" + currentVersion
   fullPrompt := prompts.BumpClassificationPrompt() +
       versionSection +
       "\n\n## Bullets to classify\n\n" + userMsg
   ```

   The new section appears between the embedded rules (returned by `BumpClassificationPrompt()`) and the `## Bullets to classify` heading. The `currentVersion` string is concatenated verbatim. The `s.runner.Run(ctx, fullPrompt)` call shape is unchanged.

3. **Thread `currentVersion` from `runClassification` into `resolveBumpVerdict`.** In the call site at line 314-319, change the call from:

   ```go
   verdict, result := s.resolveBumpVerdict(
       ctx,
       bullets,
       cachedBump,
       cachedReasoning,
   )
   ```

   to:

   ```go
   verdict, result := s.resolveBumpVerdict(
       ctx,
       bullets,
       currentVersion,
       cachedBump,
       cachedReasoning,
   )
   ```

   The `currentVersion` parameter is already in scope in `runClassification` (declared at line 305). No other change in `runClassification`. The `applyMajorBumpGuard` call below (line 336-345) is unchanged — it still takes `currentVersion` as a separate argument from its own signature.

4. **Do NOT touch `applyMajorBumpGuard`.** The function body (lines 451-488), the signature, and the glog lines stay byte-identical. The diff must show zero changed lines inside the `func (s *planningStep) applyMajorBumpGuard` body — only the call site upstream (`resolveBumpVerdict`'s `fullPrompt` concatenation) changes. This is spec AC 9; the auditor will `git diff` and grep for changed lines inside the function.

5. **Do NOT touch the `if cachedBump != ""` block.** The two-line early return (lines 276-279) stays byte-identical. The cache-hit path does NOT receive a `currentVersion` section because the LLM is not invoked — the cached verdict is reused verbatim. This is spec AC 5; the auditor verifies via `git diff` that the only changes inside `resolveBumpVerdict` are within the `cachedBump == ""` branch.

6. **Add a new Ginkgo test in `steps_planning_test.go`.** Append a new `Context("prompt assembly with current_version (spec 063)")` block inside the existing `Describe("steps_planning", func() { ... })` Describe (the outermost one at line 40). The new test, mirroring the `Context("happy path")` block's setup shape (lines 42-87):

   ```go
   Context("prompt assembly with current_version (spec 063)", func() {
       It("assembled prompt contains ## Current version section before ## Bullets to classify", func() {
           fakeFetcher := &mocks.Fetcher{}
           fakeFetcher.FetchReturns(
               []byte("## Unreleased\n\n- refactor: rename /refine-task to /plan-task\n\n## v0.69.0\n\n- old\n"),
               nil,
           )

           fakeRunner := &mocks.ClaudeRunnerMock{}
           fakeRunner.RunReturns(&claudelib.ClaudeResult{
               Result: `{"bump":"minor","reasoning":"stub"}`,
           }, nil)

           step := pkg.NewPlanningStep(
               fakeRunner,
               fakeFetcher,
               &mocks.MaintainerConfigFetcher{},
               false,
           )

           taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v0.69.0\ntask_identifier: gh-release-bborbe-vault-cli-001\n---\n\n# release task\n"

           md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
           Expect(err).NotTo(HaveOccurred())

           _, err = step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())

           Expect(fakeRunner.RunCallCount()).To(Equal(1))

           // Inspect the prompt string the runner received.
           _, promptArg := fakeRunner.RunArgsForCall(0)

           // (a) ## Current version heading present.
           Expect(promptArg).To(ContainSubstring("## Current version"))

           // (b) The literal version string is in the section body.
           Expect(promptArg).To(ContainSubstring("v0.69.0"))

           // (c) ## Current version appears BEFORE ## Bullets to classify.
           currentVersionIdx := strings.Index(promptArg, "## Current version")
           bulletsIdx := strings.Index(promptArg, "## Bullets to classify")
           Expect(currentVersionIdx).To(BeNumerically(">=", 0))
           Expect(bulletsIdx).To(BeNumerically(">=", 0))
           Expect(currentVersionIdx).To(BeNumerically("<", bulletsIdx))

           // (d) The embedded rules (returned by BumpClassificationPrompt)
           // appear BEFORE ## Current version. This proves the version
           // section is sandwiched between the rules and the bullets,
           // not prepended to the entire prompt.
           rulesIdx := strings.Index(promptArg, "# Classify the next semantic-version bump")
           Expect(rulesIdx).To(Equal(0))
           Expect(currentVersionIdx).To(BeNumerically(">", rulesIdx))
       })
   })
   ```

   The fixture mirrors the spec's concrete regression case (bborbe-vault-cli v0.69.0, one bullet, the rename). The test asserts AC 4. The `RunArgsForCall(0)` call is the new code path — the bump LLM call is the first (and only) call in this fixture because `changelogRewrite=false` (default empty `MaintainerConfigFetcher` mock).

7. **Use `strings.Index` in the new test.** The existing `steps_planning_test.go` already imports `strings` (line 12) and uses `strings.Split` + `strings.HasPrefix` in the rewrite-bucket tests. The new test's `currentVersionIdx` / `bulletsIdx` / `rulesIdx` comparisons are pure string operations on the captured `promptArg` value — no new imports needed.

8. **Do NOT add a CHANGELOG entry in this prompt.** Prompt 1 owns the `## Unreleased` block; this prompt only ships the prompt-assembly change.

9. **Run `make test` in the changed module.** From repo root: `cd /workspace/agent/github-releaser && make test`. Expected: exit code 0; all pre-existing tests pass (29 call sites, all 5 Context blocks); the new `It` case passes. Do NOT run `make precommit` — prompt 3 owns that gate.

10. **YAGNI guard.** Do NOT add a new helper function for prompt assembly (e.g., `assembleBumpClassificationPrompt(currentVersion, bullets) string`); the assembly is two lines and inline is the right shape. Do NOT change `applyMajorBumpGuard`. Do NOT change the public `Run` / `ShouldRun` / `Name` shape. Do NOT touch the `BumpVerdict` schema or `ParseBumpVerdict`. Do NOT add a CLI flag or env var to override the version injection. Do NOT touch `pkg/prompts/prompts.go` (prompt 1 owned that file).
</requirements>

<constraints>
- The change is localized to `agent/github-releaser/pkg/steps_planning.go` (signature of `resolveBumpVerdict` + 1-line change to the `fullPrompt` concatenation) and `agent/github-releaser/pkg/steps_planning_test.go` (one new `It` case).
- The `if cachedBump != ""` block in `resolveBumpVerdict` (lines 276-279) MUST stay byte-identical. The diff must show zero changed lines in that block. This is AC 5.
- The `func (s *planningStep) applyMajorBumpGuard(...)` body (lines 451-488) MUST stay byte-identical. The diff must show zero changed lines inside that function. This is AC 9.
- The `NewPlanningStep` public signature (line 59-71) MUST NOT change — `currentVersion` is a per-run field, not a constructor argument.
- The `Run` / `ShouldRun` / `Name` interface (lines 73-96) MUST NOT change.
- The `factory.CreateAgent` wiring in `/workspace/agent/github-releaser/pkg/factory/` is UNTOUCHED. The compile-time assertion at the bottom of `steps_planning_test.go` (lines 1781-1790) stays valid because the factory signature does not change.
- The runner call shape is `s.runner.Run(ctx, fullPrompt)` — no new method, no new context field, no goroutine, no channel.
- The `## Current version` section is the EXACT heading — not `## Current Version`, not `## Version`, not `### Current version`. The LLM rule text in the prompt (shipped in prompt 1) does not reference the heading by name, but the new test's load-bearing assertion depends on the literal string match.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass — the new `Context` block is additive; the `resolveBumpVerdict` signature change is the only call-site ripple, and the one call site is updated in this same prompt.
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && go test ./pkg/... -v -count=1 -run "Spec063|spec.063|prompt.assembly"
```
Expected: exit code 0; the new `It("assembled prompt contains ## Current version section before ## Bullets to classify", ...)` case passes. The grep pattern matches the new test's `Context` heading because Ginkgo v2 maps `Context` text to a substring of the spec name.

Then run the full module test suite to confirm the signature change is propagated to all 29 call sites:
```
cd /workspace/agent/github-releaser && make test
```
Expected: exit code 0; all pre-existing tests pass with the new 5-argument `resolveBumpVerdict` call site.

Evidence commands the auditor will run:
- `grep -n 'resolveBumpVerdict' /workspace/agent/github-releaser/pkg/steps_planning.go` → 2 occurrences: the function definition and the call site in `runClassification`. The call site passes 5 arguments (`ctx`, `bullets`, `currentVersion`, `cachedBump`, `cachedReasoning`).
- `grep -n '## Current version' /workspace/agent/github-releaser/pkg/steps_planning.go` → 1 occurrence: the new section header inside `resolveBumpVerdict`.
- `git diff HEAD -- /workspace/agent/github-releaser/pkg/steps_planning.go | grep -A 1 -E 'applyMajorBumpGuard'` → shows the function signature unchanged; the diff inside the function body is empty.
- `git diff HEAD -- /workspace/agent/github-releaser/pkg/steps_planning.go | grep -B 1 -A 4 'cachedBump != ""'` → shows that the diff is outside the cache-hit block; only the `cachedBump == ""` branch grows.
- `cd /workspace/agent/github-releaser && go test ./pkg/...` → exit code 0.
- `cd /workspace/agent/github-releaser && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/... && go tool cover -func=/tmp/cover.out | grep steps_planning.go` → coverage on `steps_planning.go` stays at or above the pre-prompt baseline (90.7% per CHANGELOG § v0.33.0; the new lines are string concatenation so the new test exercising them is sufficient to maintain coverage).
</verification>
