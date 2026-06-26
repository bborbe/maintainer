---
status: completed
spec: [071-pr-reviewer-verdict-decides-review-event]
summary: Extracted spec-060 major-bump guard from runClassification into private applyMajorBumpGuard helper (plus a secondary resolveRewriteAndPublish helper for the rewrite-and-publish tail), dropping the function from 92 to 64 non-comment lines; preserved all glog/escalation/PlanOutput emissions bit-identically; both github-releaser and lib precommit exit 0
container: maintainer-major-bump-guard-exec-239-spec-060-fix-funlen-and-finish-docs
dark-factory-version: v0.175.0
created: "2026-06-03T15:10:15Z"
queued: "2026-06-03T15:10:15Z"
started: "2026-06-03T15:21:11Z"
completed: "2026-06-03T15:29:38Z"
---

<summary>
- Prompt 236 (guard logic) inlined the major-bump guard into `runClassification` in `agent/github-releaser/pkg/steps_planning.go`, pushing the function to 92 lines (funlen threshold = 80). Precommit fails on this lint.
- Prompt 238 (README+CHANGELOG) wrote correct docs but returned `status: partial` because precommit failed on the pre-existing funlen from 236.
- This fix-up prompt extracts the major-bump guard block from `runClassification` into a private helper function (e.g. `applyMajorBumpGuard` or similar) — purely mechanical, behavior-preserving — so funlen passes.
- The 238 README + CHANGELOG changes are already in the working tree; the spec-060 grep assertions for them already pass (4× allowMajorBump in README, 2× --allow-major, 2× ALLOW_MAJOR, 4× allowMajorBump in CHANGELOG `## Unreleased`).
- Any gofmt / mockery regen tweaks currently dirty (mocks/, factory/, go.mod, formatting in steps_planning.go) are tooling drift to include in the commit.
- Final state: `cd agent/github-releaser && make precommit` exits 0; one commit covering the helper extraction + 238's doc deliverable + tooling drift.
</summary>

<objective>
Make `cd agent/github-releaser && make precommit` exit 0 by extracting the major-bump guard decision block from `runClassification` (currently 92 lines) into a private helper function, while preserving every observable behavior (decision table, escalation contract, glog lines, plan-output fields). Commit the working-tree doc changes from prompt 238 (README.md + CHANGELOG.md `## Unreleased` bullet) and any gofmt/mockery drift in the same commit.
</objective>

<context>
Read `CLAUDE.md`, `agent/github-releaser/CLAUDE.md`, and the spec at `specs/in-progress/060-github-releaser-major-bump-guard.md` (especially the FROZEN escalation contract and § Acceptance Criteria for the doc grep checks).

Key spec context — the guard's decision table (FROZEN, do not change):

| bump | allowMajorBumpConfig | allowMajor (flag) | result |
|---|---|---|---|
| major | false | false | TRIP → NeedsInput, escalate (spec 047 contract) |
| major | true | * | proceed (repo opted in) |
| major | false | true | proceed + glog.V(2) override |
| not-major | * | * | proceed (no-op for guard) |

The current implementation is in `agent/github-releaser/pkg/steps_planning.go`:
- `runClassification` (line ~302) — too long, contains both the verdict call AND the guard decision
- `escalation` struct (line ~503) — populated by trip path
- `PlanOutput` fields `AllowMajorBumpConfig` / `AllowMajorBumpFlag` (in `pkg/plan_output.go`)
- Glog lines: "planning: major bump not allowed" (trip) and the override-path V(2) line

Extraction target: pull the decision block (everything after the verdict is computed, the three `if verdict.Bump == "major" …` branches and the glog calls) into a helper like:

```go
// applyMajorBumpGuard evaluates the spec 060 decision table on the
// Claude verdict + opt-in flags. Returns (escalate, escalation) on
// trip; (false, escalation{}) on proceed (with the override-path
// glog.V(2) line as a side-effect when bump=major + allowMajor flag
// without config opt-in).
func (s *planningStep) applyMajorBumpGuard(
    ctx context.Context,
    verdict ClaudeVerdict, // or whatever the local type is
    allowMajorBumpConfig bool,
) (escalate bool, esc escalation) { ... }
```

The exact helper name and signature is the executor's choice — what matters is `runClassification` drops under 80 lines and every glog/escalation/plan-output emission is preserved bit-identically.

Working-tree state at the start of this prompt (dirty files):
- `CHANGELOG.md`, `README.md` — prompt 238's deliverable, already correct per its grep ACs
- `agent/github-releaser/go.mod`, `agent/github-releaser/mocks/*.go`, `agent/github-releaser/pkg/factory/factory.go` — gofmt/mockery drift
- `agent/github-releaser/pkg/steps_planning.go`, `agent/github-releaser/pkg/steps_planning_test.go` — gofmt drift
- `prompts/in-progress/238-spec-060-readme-changelog.md` — daemon will handle prompt-file moves; do not touch

Do not edit `prompts/` files. Do not change the FROZEN escalation contract. Do not add new flags or config fields.
</context>

<acceptance>
- [ ] `runClassification` in `agent/github-releaser/pkg/steps_planning.go` is < 80 lines (funlen threshold).
- [ ] A new private helper carries the major-bump guard decision table; helper has GoDoc citing spec 060.
- [ ] `cd /workspace/agent/github-releaser && make precommit` exits 0.
- [ ] `cd /workspace/lib && make precommit` exits 0 (no regression).
- [ ] All four guard decision-table Ginkgo cases from prompt 237 still pass.
- [ ] `grep -c 'allowMajorBump' README.md` returns ≥ 4 (unchanged from 238).
- [ ] `grep -c 'allowMajorBump' CHANGELOG.md` returns ≥ 1 in the `## Unreleased` block (unchanged from 238).
- [ ] `grep -c '--allow-major' README.md` returns ≥ 2 (unchanged from 238).
- [ ] No `fmt.Errorf` introduced (per project rule — use `errors.Wrap`).
- [ ] Single commit covers helper extraction + doc deliverable + tooling drift, committed by the container.
</acceptance>

<verification>
```bash
cd /workspace/agent/github-releaser && make precommit
cd /workspace/lib && make precommit
awk '/^func \(s \*planningStep\) runClassification/,/^func \(/{print}' /workspace/agent/github-releaser/pkg/steps_planning.go | wc -l   # < 80
grep -c 'allowMajorBump' /workspace/README.md
grep -c '\-\-allow-major' /workspace/README.md
grep -c 'allowMajorBump' /workspace/CHANGELOG.md
```
</verification>
