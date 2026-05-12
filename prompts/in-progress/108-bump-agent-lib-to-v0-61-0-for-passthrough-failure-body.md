---
status: committing
summary: 'Bumped github.com/bborbe/agent/lib from v0.57.0 to v0.61.0, added Passthrough content generator wiring regression tests in pkg/factory/factory_test.go, and added fix(pr-reviewer) entry to CHANGELOG.md under ## Unreleased.'
container: maintainer-108-bump-agent-lib-to-v0-61-0-for-passthrough-failure-body
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-12T22:56:49Z"
queued: "2026-05-12T22:56:49Z"
started: "2026-05-12T22:57:42Z"
---
<summary>
- Bump `github.com/bborbe/agent/lib` from `v0.57.0` to `lib/v0.61.0` in `agent/pr-reviewer/go.mod` so `passthroughContentGenerator` writes a `## Failure` body section on BOTH `status: failed` AND `status: needs_input` results — currently the v0.57.0 passthrough writes ONLY frontmatter (no body section at all), leaving operators no clue why a task transitioned to `phase: human_review`.
- Add a regression unit test at the factory level (`pkg/factory/factory_test.go`) that exercises the actual wired `delivery.NewPassthroughContentGenerator()` returned by `CreateFileResultDeliverer` (and a parallel assertion via `CreateKafkaResultDeliverer`'s content generator hook) and asserts: given `agentlib.AgentResultInfo{Status: AgentStatusNeedsInput, Message: "x", Output: ""}` and an empty original task body, the resulting markdown contains the literal string `## Failure` AND the `Message` value. A second `It` block does the same for `AgentStatusFailed`. The point of testing through the factory (not the bare lib constructor) is to catch future wiring regressions — the bug WAS a wiring mismatch.
- Update the global root-level `CHANGELOG.md` with a `fix:` entry under `## Unreleased`.
- Two-file change in `pr-reviewer` (`go.mod`, `go.sum` via `go mod tidy`, plus `pkg/factory/factory_test.go`) + one-file change at repo root (`CHANGELOG.md`).
- Zero source change in `agent/pr-reviewer/pkg/steps_gh_token.go` — the fix is intentionally generic, in the agent framework, not in pr-reviewer step code.
</summary>

<objective>
On 2026-05-12 a pr-reviewer run against `bborbe/trading#122` failed at the `pr-plan` step (Claude CLI 401 from Anthropic API). The task transitioned to `phase: human_review` with the task body containing only title + URL — no `## Failure` section, no reason string, no failing-step name. Operators had to race the agent pod's TTL cleanup to grab `kubectl logs` before the pod disappeared, then manually parse the Claude CLI error to diagnose the auth issue.

Root cause: pr-reviewer pins `github.com/bborbe/agent/lib v0.57.0`, whose `passthroughContentGenerator.Generate` is literally `return applyStatusFrontmatter(result.Output, result.Status), nil` — when an early step returns `Status: failed/needs_input` with empty `result.Output`, the deliverer applies frontmatter to an empty string and publishes a body-less task. The fix already exists at `lib/v0.61.0`: both `Failed` and `NeedsInput` paths now splice `buildFailureSection(result)` into a `## Failure` section before publishing.

This bump moves pr-reviewer onto the fixed lib. The change is intentionally generic (agent-framework level, not step-level) so every future agent in this repo and every `NeedsInput`/`Failed` call site automatically benefits — no per-step body-writing code, no per-agent duplication. CHANGELOG line 159 of `bborbe/agent` claimed the `## Failure` write path exists; the claim is true for the `fallback` and `section` generators but silently regressed for `passthrough`, which is the only generator pr-reviewer uses. The bump fixes the regression.

Acceptance proof is the regression test: after the bump, a `NeedsInput` result with empty `Output` produces a marshaled task whose body contains `## Failure` and the operator-facing message. Without the test, an accidental future downgrade silently re-breaks the body and the bug returns invisibly.
</objective>

<context>

## Files to edit

### `agent/pr-reviewer/go.mod` — line 11

**Current:**

```
require (
	github.com/bborbe/agent/lib v0.57.0
```

**After (achieved via `go get`, not hand-edit):**

```
require (
	github.com/bborbe/agent/lib v0.61.0
```

Run from `agent/pr-reviewer/`:

```bash
go get github.com/bborbe/agent/lib@v0.61.0
go mod tidy
```

Notes:
- The module path is `github.com/bborbe/agent/lib`. The submodule tag in the agent repo is `lib/v0.61.0` — Go's module proxy strips the `lib/` path prefix when resolving the version, so the canonical `go get` argument is `@v0.61.0` (NOT `@lib/v0.61.0`). If `@v0.61.0` fails with "unknown revision", try `@lib/v0.61.0` as a fallback (some proxy configurations require the literal tag).
- `go mod tidy` will adjust `go.sum` and may bump transitive indirect deps. That's expected — let it tidy.
- Do NOT manually edit `go.mod` to insert `v0.61.0`. Use `go get` so the resolver verifies the tag exists and pulls the matching `go.sum` hash.

### `agent/pr-reviewer/pkg/factory/factory_test.go` — add two new `Describe` blocks

The existing test file already has `Describe("CreateFileResultDeliverer", ...)` at line 82 and `Describe("CreateKafkaResultDeliverer", ...)` at line 107. Add a new sibling-level `Describe` block named `"Passthrough content generator wiring — failure body"` after line ~123 (after the existing `CreateKafkaResultDeliverer` block, before `Describe("RunConfig.AuthSetup wiring", ...)` at line 124).

Test must:

1. Import `agentlib "github.com/bborbe/agent/lib"` and `"github.com/bborbe/agent/lib/delivery"`.
2. Construct `gen := delivery.NewPassthroughContentGenerator()` — same constructor the factory wires at `factory.go:125,134`.
3. Two `Context` blocks:

   **Context: when result status is needs_input with empty Output**

   ```go
   result := agentlib.AgentResultInfo{
       Status:  agentlib.AgentStatusNeedsInput,
       Message: "GH_TOKEN unauthorized (HTTP 401)",
       Output:  "",
   }
   generated, err := gen.Generate(ctx, "", result)
   Expect(err).NotTo(HaveOccurred())
   Expect(generated).To(ContainSubstring("## Failure"))
   Expect(generated).To(ContainSubstring("GH_TOKEN unauthorized (HTTP 401)"))
   Expect(generated).To(ContainSubstring("phase: human_review"))
   ```

   **Context: when result status is failed with empty Output**

   ```go
   result := agentlib.AgentResultInfo{
       Status:  agentlib.AgentStatusFailed,
       Message: "claude CLI: 401 Invalid authentication credentials",
       Output:  "",
   }
   generated, err := gen.Generate(ctx, "", result)
   Expect(err).NotTo(HaveOccurred())
   Expect(generated).To(ContainSubstring("## Failure"))
   Expect(generated).To(ContainSubstring("claude CLI: 401 Invalid authentication credentials"))
   Expect(generated).To(ContainSubstring("phase: human_review"))
   ```

4. Use the suite's existing `ctx` variable (already declared in `factory_suite_test.go` if present; otherwise `ctx := context.Background()` is acceptable for a generator test, mirroring lib's own `content-generator_test.go`).

Notes on why test at the factory level vs the bare lib level:
- The regression that hid this bug for months: lib `v0.57.0` shipped a passthrough generator with no body-write at all, and pr-reviewer's factory wired it without exercising the failure-body contract in any test. A bare `delivery.NewPassthroughContentGenerator()` test in the lib at v0.61.0 already exists upstream — replicating that here doesn't catch THIS class of regression.
- The catch: an unguarded `go get github.com/bborbe/agent/lib@latest` (or accidental downgrade) could regress pr-reviewer. The test must live in the same `go.mod` boundary as the version pin so a tidy-then-test cycle proves the contract holds at the pinned version.
- We import `delivery.NewPassthroughContentGenerator` directly rather than reach through `factory.CreateFileResultDeliverer().Deliver(...)` because the latter requires a temp file + filesystem assertions. The generator is the unit under test; the wiring is asserted by the import path matching `factory.go:125,134` symbol-for-symbol.

### `CHANGELOG.md` (repo root, NOT `agent/pr-reviewer/`)

Per `CLAUDE.md` lines 115-117: "Single global `CHANGELOG.md` at repo root. No per-module CHANGELOG."

Current state (verified): there is NO existing `## Unreleased` section. The file starts with `# Changelog` on line 1, the description on line 3, then jumps directly to `## v0.23.34` on line 5. Create a new `## Unreleased` section between line 3 and line 5 with this entry:

```
## Unreleased

- fix(pr-reviewer): bump `github.com/bborbe/agent/lib` v0.57.0 → v0.61.0 so `passthroughContentGenerator` writes a `## Failure` body section on BOTH `status: failed` AND `status: needs_input` results. Fixes 2026-05-12 incident on PR `bborbe/trading#122` where a Claude CLI 401 left the task page with no failure reason, forcing operators to race the agent pod's TTL cleanup to grab `kubectl logs`. Adds a factory-level regression test guarding the version pin against future accidental downgrade.
```

If another in-flight prompt has already created `## Unreleased` between the time this prompt was drafted and the time it executes, append as a new bullet under the existing `## Unreleased` — do NOT create a second `## Unreleased` section.

</context>

<constraints>

- **Do NOT touch `agent/pr-reviewer/pkg/steps_gh_token.go`** or any other step file. The fix is intentionally generic at the lib boundary. Per-step body-writing would duplicate the framework's job and be in scope only if the lib bump alone failed to satisfy the regression test (it won't).
- **Do NOT add a new file under `agent/pr-reviewer/pkg/`** — the regression test must live in the existing `pkg/factory/factory_test.go`, beside the wiring it guards.
- `make precommit` MUST exit 0 in `agent/pr-reviewer/`. Run it after the bump + test addition.
- Errors must be wrapped with `github.com/bborbe/errors` (no new errors introduced here; the existing wrapping in steps is untouched).
- `go mod tidy` MUST run after `go get` so `go.sum` is current. Without tidy, CI will fail on `go mod verify` / lint's `go mod download` check.
- Test setup MUST use Ginkgo's existing `Describe`/`Context`/`It` BDD structure already in use in `factory_test.go`. Do NOT introduce `testing.T`-style tests in this file — the suite is Ginkgo-only.
- Test assertion strings MUST use `ContainSubstring` (not `Equal`) — the generator's exact line breaks and frontmatter ordering are implementation details of the lib and may shift across patch versions without breaking the body-write contract.
- No changes to `agent/pr-reviewer/Dockerfile`, `agent/pr-reviewer/k8s/`, or any non-Go file beyond `go.mod`, `go.sum`, the test file, and `CHANGELOG.md`.
- Do NOT bump any other module's `go.mod` (e.g. `watcher/github-pr`, `watcher/github-build`). Only `agent/pr-reviewer/go.mod` is in scope. There is currently no second agent (verified: `ls maintainer/agent/` returns only `pr-reviewer` and `Makefile`).

</constraints>

<failure_modes>

| Trigger | Expected behaviour | Recovery |
|---|---|---|
| `go get github.com/bborbe/agent/lib@lib/v0.61.0` fails with "unknown revision" | Tag name format mismatch. The tag in the agent repo is `lib/v0.61.0` but Go module proxy may require `v0.61.0` after the path-prefix strip | Try `go get github.com/bborbe/agent/lib@v0.61.0` — Go's submodule convention strips the `lib/` prefix when resolving |
| `go mod tidy` bumps transitive deps that break compilation | Unrelated breakage from a transitive bump | Inspect `go.mod` diff; if a transitive bump is the cause, pin it back. Do NOT downgrade `github.com/bborbe/agent/lib` to dodge the issue — the lib bump is the whole point |
| New test fails because `generated` does NOT contain `## Failure` | Either `lib/v0.61.0` actually does NOT have the both-branch fix (re-read `lib/v0.61.0:lib/delivery/content-generator.go`) OR the wrong constructor was tested | Confirm constructor matches `factory.go:125,134` (`delivery.NewPassthroughContentGenerator`) and confirm tag content — DO NOT add hand-rolled body-writing to pass the test |
| `make precommit` lint fails on the new test (e.g. funlen, lll) | New test exceeded golangci-lint thresholds | Split into smaller `It` blocks or move helpers to a `factory_test_helpers.go` (but NOT into a new file under `pkg/` — keep it under `pkg/factory/`) |
| CHANGELOG entry placement wrong (under released version instead of Unreleased) | `/coding:commit` would tag the wrong release | Open `CHANGELOG.md` and verify the bullet sits under the literal `## Unreleased` header before committing |
| Test imports `agentlib` but file already aliases it differently | Compile error: redeclared import | Use the existing alias in `factory_test.go` if one exists; otherwise add `agentlib "github.com/bborbe/agent/lib"` |

</failure_modes>

<acceptance_criteria>

- [ ] `agent/pr-reviewer/go.mod` requires `github.com/bborbe/agent/lib v0.61.0` (verify with `grep 'bborbe/agent/lib' agent/pr-reviewer/go.mod`).
- [ ] `agent/pr-reviewer/go.sum` updated by `go mod tidy` — no manual edits.
- [ ] `agent/pr-reviewer/pkg/factory/factory_test.go` contains a new `Describe("Passthrough content generator wiring — failure body", ...)` block with at minimum two `Context` blocks: one for `AgentStatusNeedsInput`, one for `AgentStatusFailed`.
- [ ] Both new tests assert `ContainSubstring("## Failure")` AND `ContainSubstring(<message string>)` AND `ContainSubstring("phase: human_review")`.
- [ ] Both new tests use empty `result.Output` (the regression-trigger condition — `Output != ""` would mask the bug).
- [ ] `CHANGELOG.md` (repo root) has a new `fix(pr-reviewer):` bullet under `## Unreleased`.
- [ ] `cd agent/pr-reviewer && make precommit` exits 0.
- [ ] `git diff --name-only HEAD -- CHANGELOG.md agent/pr-reviewer/` shows EXACTLY these files (no extras):
  - `CHANGELOG.md`
  - `agent/pr-reviewer/go.mod`
  - `agent/pr-reviewer/go.sum`
  - `agent/pr-reviewer/pkg/factory/factory_test.go`
  - (dark-factory's own prompt-file move into `prompts/in-progress/` is excluded by the path scope)
- [ ] `agent/pr-reviewer/pkg/steps_gh_token.go` is **unchanged** (verify with `git diff agent/pr-reviewer/pkg/steps_gh_token.go` returning empty).
- [ ] No new file created under `agent/pr-reviewer/pkg/` (verify with `git status` showing no untracked Go files).

</acceptance_criteria>

<verification>

```bash
cd agent/pr-reviewer

# Version pin landed
grep 'github.com/bborbe/agent/lib ' go.mod
# expect: github.com/bborbe/agent/lib v0.61.0

# Regression test exists
grep -n 'Passthrough content generator wiring' pkg/factory/factory_test.go
# expect: match

# Regression test exercises both statuses
grep -nE 'AgentStatusNeedsInput|AgentStatusFailed' pkg/factory/factory_test.go
# expect: at least one of each in the new Describe block

# steps_gh_token.go untouched
git diff pkg/steps_gh_token.go
# expect: empty output

# Precommit clean
make precommit
# expect: exit 0

# Files in diff (scoped to the change paths; excludes dark-factory's prompt move)
git diff --name-only HEAD -- CHANGELOG.md agent/pr-reviewer/
# expect exactly:
#   CHANGELOG.md
#   agent/pr-reviewer/go.mod
#   agent/pr-reviewer/go.sum
#   agent/pr-reviewer/pkg/factory/factory_test.go
```

Expected end state:
- pr-reviewer pinned to `lib v0.61.0`
- Factory test guards the version pin with concrete body-section assertions
- CHANGELOG documents the fix under Unreleased
- No source change in pr-reviewer pkg code
- Precommit clean

</verification>

<do_nothing_option>
Leaving pr-reviewer on `lib v0.57.0` means every future `pr-plan` claude-CLI failure, every `gh_token` `NeedsInput` preflight, and every other transient agent failure produces a body-less `phase: human_review` task. Operators continue to race the agent pod's ~5-minute TTL cleanup window to grab `kubectl logs` for diagnosis. The 2026-05-12 incident reproduces every time the Anthropic API or GitHub token has a hiccup. CHANGELOG.md:159's claim that `status: failed` writes a `## Failure` section remains technically true for the fallback/section generators but silently false for the passthrough generator pr-reviewer uses — a hidden contract gap that masks every transient failure.

This change is the smallest possible move: one version bump, one regression test, one changelog line. Zero source change. Reverting is one `go get @v0.57.0` if the lib bump introduces an unforeseen issue (none expected — the diff between v0.57.0 and v0.61.0 in `lib/delivery/content-generator.go` is purely additive in the passthrough branch).
</do_nothing_option>
