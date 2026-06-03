---
status: completed
spec: [058-changelog-rewrite-flow]
summary: 'Planning step now captures original ## Unreleased verbatim and emits a rewrite verdict (rewrite_needed + optional cleaned body) via a second focused Claude call using the embedded Changelog Quality Guide; all 11 requirements met, make precommit exit=0 with pkg/changelog 96.2% and pkg/prompts 90.4% coverage.'
container: maintainer-changelog-rewrite-exec-227-spec-058-planning-rewrite
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T16:30:08Z"
queued: "2026-06-02T16:43:47Z"
started: "2026-06-02T17:46:26Z"
completed: "2026-06-02T17:57:38Z"
---

<summary>
- Planning phase now captures the original `## Unreleased` text verbatim into the `## Plan` block on the task page.
- Planning decides whether the original needs to be rewritten and, when it does, emits a cleaned version alongside the original.
- The Changelog Quality Guide is baked into the planning prompt at build time so the LLM applies its rules without any runtime filesystem dependency.
- Already-clean `## Unreleased` content passes through untouched (`rewrite_needed=false`) — the existing bump classification continues to work.
- Noisy content (raw `git log` dumps, missing prefixes, ten-line dependabot blocks) is cleaned, normalized to conventional prefixes, and dependency dumps are folded into a single summary entry.
- Empty/absent `## Unreleased` continues to escalate via the existing needs_input path (no change to that contract).
- Adds Ginkgo spec coverage for clean / noisy / missing-prefix / dependency-dump / verbatim-capture cases.
</summary>

<objective>
Extend the planning step so the `## Plan` JSON section emitted at planning-phase completion contains the original `## Unreleased` body verbatim, a `rewrite_needed` boolean, and (when `rewrite_needed=true`) a `rewritten_unreleased` cleaned body produced by the planning LLM using the embedded Changelog Quality Guide as input. This is the spec-058 planning slice; execution will consume `rewritten_unreleased` in a follow-up prompt, ai-review will compare `original_unreleased` against the final published body in the prompt after that.
</objective>

<context>
Read `~/Documents/workspaces/maintainer-changelog-rewrite/CLAUDE.md` and `agent/github-releaser/CLAUDE.md` for project conventions.

Read these files BEFORE editing:
- `agent/github-releaser/pkg/steps_planning.go` — current planning step (uses `runner.Run(ctx, fullPrompt)` and `prompts.ParseBumpVerdict`).
- `agent/github-releaser/pkg/plan_output.go` — current `PlanOutput` struct (extend, do not break round-trip).
- `agent/github-releaser/pkg/prompts/prompts.go` — current embed (`bumpClassificationPrompt`) + `BumpVerdict` parser pattern.
- `agent/github-releaser/pkg/prompts/bump_classification.md` — current prompt text.
- `agent/github-releaser/pkg/changelog/changelog.go` — `ExtractUnreleasedBullets`, `ValidateUnreleased`, `RewriteUnreleasedHeader` (pure helpers; safe to add a new pure helper here).
- `agent/github-releaser/pkg/steps_planning_test.go` — existing Ginkgo style (counterfeiter mocks `ClaudeRunnerMock`, `mocks.Fetcher`; `agentlib.ParseMarkdown`; `agentlib.ExtractSection[pkg.PlanOutput]`).
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` (in-container path — this is the canonical Changelog Quality Guide; read it and copy it verbatim into the embedded file in step 1).

Read these coding plugin guides (in-container paths — the prompt runs inside the YOLO container):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

Verified symbols (from module source):
- `agentlib.ExtractSection[T any](ctx, *Markdown, string) (*T, error)` and `agentlib.MarshalSectionTyped[T any](ctx, string, T) (Section, error)` from `github.com/bborbe/agent/lib@v0.63.11`.
- `claudelib.ClaudeRunner` interface: `Run(ctx context.Context, prompt string) (*ClaudeResult, error)`; `ClaudeResult{Result string}`.
- `domain.TaskPhaseExecution = "execution"` from `github.com/bborbe/vault-cli@v0.67.5/pkg/domain/task_phase.go`.
- `github.com/bborbe/errors` wrap idioms: `errors.Wrapf(ctx, err, format, args...)`, `errors.Errorf(ctx, format, args...)`.
- `glog.V(2).Infof` for trace logs (existing pattern in `steps_planning.go`).
- `changelog.ExtractUnreleasedBullets(content) []string`, `changelog.InferHeaderPrefixStyle(content) string`.
</context>

<requirements>

1. **Copy the canonical guide into the package as the embed source.** As the FIRST step of the prompt, copy `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` → `agent/github-releaser/pkg/prompts/changelog-quality-guide.md` verbatim (`cp /home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md agent/github-releaser/pkg/prompts/changelog-quality-guide.md`). Preserve headings, tables, and code fences exactly. The coding-plugin doc is the canonical source; the in-package file is the embed target read at build time by `//go:embed`. The Obsidian-vault page is a local-rendered mirror — not the source — and is NOT referenced by this prompt or by the build.

2. **Embed the guide into the prompts package.** In `agent/github-releaser/pkg/prompts/prompts.go`, add a second `//go:embed` directive alongside the existing `bumpClassificationPrompt`:

   ```go
   //go:embed changelog-quality-guide.md
   var changelogQualityGuide string

   // ChangelogQualityGuide returns the embedded Changelog Quality Guide text.
   // It is concatenated into the planning prompt as the ruleset the LLM
   // applies when producing `rewritten_unreleased`.
   func ChangelogQualityGuide() string {
       return changelogQualityGuide
   }
   ```

   Do NOT touch the existing `BumpClassificationPrompt()`/`ParseBumpVerdict` exported API.

3. **Extend `PlanOutput`.** In `agent/github-releaser/pkg/plan_output.go`, ADD three new fields to the existing struct (keep all existing fields untouched so the round-trip with prior persisted task pages still decodes):

   ```go
   // OriginalUnreleased is the raw ## Unreleased body (verbatim, line-endings
   // preserved) captured at planning time. ai-review reads this from the task
   // page — never re-derives it from the repo — so an attacker who modifies
   // the repo between planning and review cannot mask drift.
   OriginalUnreleased string `json:"original_unreleased,omitempty"`

   // RewriteNeeded is true when the planning LLM judged the original body
   // does not conform to the Changelog Quality Guide and produced a cleaned
   // body in RewrittenUnreleased. When false, execution renames the header
   // only and leaves the body untouched.
   RewriteNeeded bool `json:"rewrite_needed"`

   // RewrittenUnreleased is the cleaned body. Populated only when
   // RewriteNeeded is true. Execution replaces the ## Unreleased body with
   // this text before renaming the header.
   RewrittenUnreleased string `json:"rewritten_unreleased,omitempty"`
   ```

   Note the `omitempty` placement: `RewriteNeeded` deliberately omits `omitempty` so a `false` decision is always written explicitly (ai-review needs to distinguish "not decided" from "decided no").

4. **New pure helper `ExtractUnreleasedBody`.** In `agent/github-releaser/pkg/changelog/changelog.go`, add a pure function that returns the verbatim body of the `## Unreleased` section (every line after `## Unreleased`, up to but excluding the next `## ` heading or EOF). Preserve line endings; do NOT trim. Return `("", error)` if `## Unreleased` is not present. Signature:

   ```go
   // ExtractUnreleasedBody returns the verbatim body of the ## Unreleased
   // section: every line after the `## Unreleased` heading up to (but
   // excluding) the next `## ` heading or EOF. Line endings are preserved.
   // Returns a wrapped error if ## Unreleased is not present.
   func ExtractUnreleasedBody(ctx context.Context, content []byte) (string, error)
   ```

   Use `bufio.Scanner` over `bytes.NewReader(content)` and re-emit lines with `\n` between them (matching the existing `RewriteUnreleasedHeader` line-ending convention). Add focused table tests in `agent/github-releaser/pkg/changelog/changelog_test.go` covering: (a) typical body with bullets; (b) ## Unreleased with no body before next heading → empty string, no error; (c) no ## Unreleased heading → error.

5. **New rewrite prompt asset.** Create `agent/github-releaser/pkg/prompts/changelog_rewrite.md`. Content shape:
   - Opening line: "You are cleaning the `## Unreleased` section of a CHANGELOG before release."
   - Reference the Changelog Quality Guide (the embedded content will be concatenated below at runtime; the prompt text should say "Apply the rules in the guide concatenated below as `## Changelog Quality Guide`.").
   - Rule of thumb: if every bullet already starts with a conventional prefix (`feat:`/`fix:`/`refactor:`/`chore:`/`docs:`/`test:`/`build:`/`ci:`/`perf:`/`style:`), the body is already clean and `rewrite_needed` should be `false`.
   - Concrete cleaning operations the LLM must apply when `rewrite_needed=true`:
     - Add a conventional prefix to entries that lack one.
     - Strip raw `git log` style lines (commit hashes, author names, dates) and reframe as user-visible effects.
     - Fold a dependency-bump dump (≥ 5 adjacent `chore: bump`/`chore(deps):` lines) into a single `chore: routine dependency updates` entry.
     - Remove invisible-to-users entries (e.g. internal renames, mocks regeneration) per the guide's "Describe the Effect, Not the Implementation" rule.
   - **Faithfulness constraint** (critical — ai-review will check this): every entry from the original that describes a user-observable change MUST be present in the cleaned output. The LLM may merge or reword entries but MUST NOT silently drop a user-visible change and MUST NOT add an entry whose meaning is not present in the original.
   - Output format: a single JSON object inside one fenced ```json block, no prose outside:
     ```json
     {
       "rewrite_needed": true,
       "rewritten_unreleased": "- feat: …\n- fix: …\n",
       "reasoning": "one sentence naming the deciding rule"
     }
     ```
   - When `rewrite_needed` is `false`, `rewritten_unreleased` MUST be the empty string and `reasoning` MUST cite that every bullet already conforms.

6. **Embed the rewrite prompt.** In `agent/github-releaser/pkg/prompts/prompts.go`, add:

   ```go
   //go:embed changelog_rewrite.md
   var changelogRewritePrompt string

   // ChangelogRewritePrompt returns the LLM instructions for producing the
   // rewrite verdict. The caller is responsible for concatenating
   // ChangelogQualityGuide() and the verbatim ## Unreleased body onto the
   // returned string before invoking Claude.
   func ChangelogRewritePrompt() string {
       return changelogRewritePrompt
   }
   ```

7. **New verdict type + parser.** In `agent/github-releaser/pkg/prompts/prompts.go`, add:

   ```go
   // RewriteVerdict is the typed shape of Claude's JSON response to the
   // changelog-rewrite prompt.
   //   RewriteNeeded=true  → RewrittenUnreleased is the cleaned body (non-empty)
   //   RewriteNeeded=false → RewrittenUnreleased is the empty string
   // Reasoning is always non-empty.
   type RewriteVerdict struct {
       RewriteNeeded       bool   `json:"rewrite_needed"`
       RewrittenUnreleased string `json:"rewritten_unreleased"`
       Reasoning           string `json:"reasoning"`
   }

   // ParseRewriteVerdict extracts a RewriteVerdict from Claude's raw output.
   // Uses the same three-strategy extraction as ParseBumpVerdict (plain JSON,
   // fenced ```json block, last balanced {...} block). After unmarshal:
   //   - Reasoning MUST be non-empty.
   //   - When RewriteNeeded=true,  RewrittenUnreleased MUST be non-empty.
   //   - When RewriteNeeded=false, RewrittenUnreleased MUST be empty.
   // Errors are wrapped via github.com/bborbe/errors and always contain the
   // literal substring "parse rewrite verdict".
   func ParseRewriteVerdict(ctx context.Context, claudeOutput string) (RewriteVerdict, error)
   ```

   Implementation: extract using the existing `lastJSONBlock` helper (same file) — do NOT duplicate that logic. Validation enforces the three invariants above.

   Add Ginkgo tests in `agent/github-releaser/pkg/prompts/prompts_test.go` covering: plain JSON; fenced JSON in prose; `rewrite_needed=false` with empty `rewritten_unreleased` passes; `rewrite_needed=true` with empty `rewritten_unreleased` errors; missing `reasoning` errors; malformed JSON errors with the literal `"parse rewrite verdict"` substring.

8. **Wire rewrite verdict into the planning step.** In `agent/github-releaser/pkg/steps_planning.go`:

   a. After `bullets := changelog.ExtractUnreleasedBullets(changelogBytes)` and `prefixStyle := changelog.InferHeaderPrefixStyle(changelogBytes)`, ALSO capture the verbatim body:
      ```go
      originalBody, err := changelog.ExtractUnreleasedBody(ctx, changelogBytes)
      if err != nil {
          glog.V(2).Infof("planning: extract unreleased body failed: %v", err)
          return &agentlib.Result{Status: agentlib.AgentStatusFailed, Message: "extract unreleased body: " + err.Error()}, nil
      }
      ```

   b. Add a new helper method `runRewrite` on `*planningStep` that runs the rewrite classification AFTER the bump classification has succeeded (and before `runClassification` returns its PlanOutput). The flow in `runClassification` becomes: bump verdict → bump-version → rewrite verdict → assemble `PlanOutput`. Keep the bump and rewrite as separate Claude calls (two `runner.Run` invocations) so each LLM gets a focused prompt and ai-review failure modes stay distinguishable.

   c. The rewrite prompt assembled at the call site:
      ```go
      rewritePrompt := prompts.ChangelogRewritePrompt() +
          "\n\n## Changelog Quality Guide\n\n" + prompts.ChangelogQualityGuide() +
          "\n\n## Current ## Unreleased body\n\n" + originalBody
      runResult, err := s.runner.Run(ctx, rewritePrompt)
      if err != nil {
          glog.V(2).Infof("planning: claude runner (rewrite) failed: %v", err)
          return &agentlib.Result{Status: agentlib.AgentStatusFailed, Message: "claude run rewrite: " + err.Error()}, nil
      }
      verdict, err := prompts.ParseRewriteVerdict(ctx, runResult.Result)
      if err != nil {
          glog.V(2).Infof("planning: parse rewrite verdict failed: %v", err)
          return &agentlib.Result{Status: agentlib.AgentStatusFailed, Message: "parse rewrite verdict: " + err.Error()}, nil
      }
      ```

   d. Populate the new `PlanOutput` fields:
      ```go
      output := PlanOutput{
          // existing fields …
          OriginalUnreleased:  originalBody,
          RewriteNeeded:       verdict.RewriteNeeded,
          RewrittenUnreleased: verdict.RewrittenUnreleased,
      }
      ```

   Do NOT change the existing escalation path or `parseOwnerRepo`/`readRequired` helpers.

9. **Planning Ginkgo spec coverage.** Extend `agent/github-releaser/pkg/steps_planning_test.go` with a new `Context("rewrite decision")` block containing five `It` cases — each must assert on the parsed `## Plan` JSON via `agentlib.ExtractSection[pkg.PlanOutput]`:

   a. `It("clean Unreleased → rewrite_needed=false with empty rewritten_unreleased")` — fetcher returns a CHANGELOG whose `## Unreleased` body is `- feat: add foo\n- fix: bar\n`. Mock `ClaudeRunnerMock` with two `RunReturns` configured via `RunReturnsOnCall(0, …)` (bump = patch/minor) and `RunReturnsOnCall(1, …)` (rewrite verdict: `rewrite_needed=false`, `rewritten_unreleased=""`, reasoning non-empty). Assert `plan.RewriteNeeded == false`, `plan.RewrittenUnreleased == ""`.

   b. `It("noisy git log dump → rewrite_needed=true with every line conventional-prefix-conformant")` — fetcher returns `## Unreleased` body with raw commit lines (no prefix). Mock rewrite verdict returns a cleaned body. Assert `plan.RewriteNeeded == true` AND every non-blank line in `plan.RewrittenUnreleased` matches the regex `^- (feat|fix|refactor|chore|docs|test|build|ci|perf|style)(\([^)]*\))?:\s+\S`.

   c. `It("missing-prefix entry → rewrite adds prefix")` — fetcher body contains a bullet `- add foo` (no prefix). Mock rewrite verdict returns `- feat: add foo`. Assert the rewritten output contains `feat: add foo`.

   d. `It("chore: bump dump (10 lines) → folded into a single dependency-updates entry")` — fetcher body contains 10 consecutive `chore: bump x-vY.Z` lines. Mock rewrite verdict returns one `- chore: routine dependency updates` entry. Assert the rewritten output is exactly one bullet line and contains the literal `routine dependency updates`.

   e. `It("captures original_unreleased verbatim regardless of rewrite decision")` — pick the noisy fixture from case (b). After the step runs, assert `plan.OriginalUnreleased` is BYTE-EQUAL to the body slice the fetcher emitted between `## Unreleased\n\n` and the next `## `. This is the security-relevant invariant — capture-time must match the bytes ai-review later reads.

   For each `It`, follow the existing test fixture shape in this file: build `taskMD` as a `---` frontmatter + heading string, `agentlib.ParseMarkdown`, run the step, then `agentlib.ExtractSection[pkg.PlanOutput](ctx, md, "## Plan")`. Use `fakeRunner.RunReturnsOnCall(0, ...)` for the bump call and `fakeRunner.RunReturnsOnCall(1, ...)` for the rewrite call — order matters; the planning step now invokes Claude twice per task.

10. **Escalation paths unchanged.** Verify that all existing `Context("P1 escalation")`/`Context("missing frontmatter")` cases still pass without modification. The rewrite call MUST be skipped on any escalation path (because `runClassification` is never reached on escalation). Do not add a Claude call before the existing escalation gates.

11. **Acceptance gate — `make precommit` exits 0 in `agent/github-releaser`.** Run it from that directory after edits. Investigate and fix any failures (do not skip linters). `make precommit` runs `go generate ./...`; counterfeiter regen should be a no-op for this prompt because no counterfeiter interface is changed here.

</requirements>

<constraints>
- The `## Plan` block on the task page is the single source of truth ai-review will read for `original_unreleased` — capture-time MUST be planning, NOT re-derived from the repo at review time.
- The Changelog Quality Guide is embedded via `//go:embed` from `agent/github-releaser/pkg/prompts/changelog-quality-guide.md`. No runtime filesystem dependency. The canonical source is the coding-plugin doc at `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`; the embedded mirror MUST be re-copied whenever that source changes (manual; no automated sync in this spec).
- Conventional-prefix bump detection in planning (`patch | minor | major`) must continue to work; the rewrite must not change which prefixes appear if the originals were already conformant.
- Do NOT add an `ErrorCategoryChangelogQuality` enum entry — ai-review failure (in a later prompt) uses the existing `human_review` phase exit.
- Do NOT add a per-repo bypass switch (e.g. `.maintainer.yaml: release.skipChangelogValidation: true`) — already-clean changelogs naturally pass through with `rewrite_needed=false`.
- The 3-phase task lifecycle (`planning → execution → ai_review`) and its `human_review` exit point are frozen — this prompt extends the contents of planning only.
- Empty/absent `## Unreleased` continues to escalate via the existing `ValidateUnreleased`/`PreconditionP2UnreleasedEmpty` path; the rewrite step never runs on such inputs.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
```
cd ~/Documents/workspaces/maintainer-changelog-rewrite/agent/github-releaser
make precommit
```

Expected: exit code 0. All existing Ginkgo specs in `pkg/steps_planning_test.go` pass; the new `Context("rewrite decision")` block adds five passing `It` cases. The new tests in `pkg/changelog/changelog_test.go` for `ExtractUnreleasedBody` and in `pkg/prompts/prompts_test.go` for `ParseRewriteVerdict` pass.

Evidence commands the auditor will run:
- `grep -n '//go:embed' agent/github-releaser/pkg/prompts/prompts.go` → must show both `bump_classification.md` and `changelog-quality-guide.md`.
- `ls -la agent/github-releaser/pkg/prompts/changelog-quality-guide.md` → non-zero file size.
- `grep -n 'OriginalUnreleased\|RewriteNeeded\|RewrittenUnreleased' agent/github-releaser/pkg/plan_output.go` → three fields present with the documented JSON tags.
</verification>
