---
status: completed
spec: [034-pr-reviewer-always-post-review]
summary: Updated pr-post-back.md with Always-Post Review Invariant section, planning-phase LGTM posting note, and LGTM failure routing table rows
container: maintainer-exec-131-pr-reviewer-always-post-review-docs
dark-factory-version: v0.164.0
created: "2026-05-23T00:00:00Z"
queued: "2026-05-23T11:30:19Z"
started: "2026-05-23T12:35:30Z"
completed: "2026-05-23T12:38:40Z"
---

<summary>
- `agent/pr-reviewer/docs/pr-post-back.md` updated to document the always-post-review invariant and the new LGTM branch of the post-back contract
- Root `CHANGELOG.md` `## Unreleased` entry already added in prompt 1 (do not duplicate — verify it's present)
</summary>

<objective>
Update the pr-post-back documentation to describe the always-post-review behavior introduced by spec 034. Document the LGTM path, the vault `## Verdict` section format for both paths, and the failure routing.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `agent/pr-reviewer/docs/pr-post-back.md` — full file; understand the existing sections and style before editing.

**Files to read fully before making any changes:**
- `agent/pr-reviewer/docs/pr-post-back.md` — full file; understand existing structure: Vault-First Invariant, What Gets Posted, The Posting Flow, Diagnostic Block Format, Failure Routing, nil Poster, Dismissal Contract, Key Files
- `CHANGELOG.md` — confirm the spec-034 entry is present (under any heading). Dark-factory's auto-release flow cuts a new version (e.g. `## v0.25.10`) the moment prompt 1 commits, so the canonical post-release state is the entry under that released heading — NOT under `## Unreleased`.

**Dependency check — run before making any changes:**

```bash
# Confirm the spec-034 CHANGELOG entry was added by prompt 1 (any heading):
grep -nE "LGTM|no concerns|always.*post" CHANGELOG.md | head -5
# Expected: at least one match anywhere in CHANGELOG.md
```

If the grep returns no match, STOP and report `status: failed` with reason "spec-034 prompt 1 not yet executed — CHANGELOG entry missing".
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Update `agent/pr-reviewer/docs/pr-post-back.md`** to document the always-post-review behavior.

   Add a new section **"Always-Post Review Invariant"** after the existing "nil Poster — Local / Backward-Compatible Mode" section (before the Dismissal Contract section). The section should document:

   ```
   ## Always-Post Review Invariant

   After spec 034, every PR that reaches the planning phase produces at least one
   visible artifact on the GitHub PR — there is no silent-skip path.

   **Empty-concerns path (LGTM):** When the planning phase's `## Plan` JSON has
   `concerns: []` (no concerns flagged), the agent POSTs a `COMMENT` review with
   body `Reviewed by <BotLogin> — no concerns flagged.` via `PrPoster.PostLGTM`.
   The `## Verdict` section is written to the vault after the POST succeeds, naming
   the review id and `COMMENT` event. The task advances to `phase: done`.

   **Non-empty-concerns path:** When concerns are non-empty, the existing
   planning → execution → ai_review flow proceeds unchanged. `## Verdict` is
   written by the `ai_review` phase after the full review is posted.

   **Failure routing:** If the LGTM POST fails (network error, GitHub 5xx/4xx),
   the task escalates to `human_review`. The `## Diagnostics` block records the
   failure. This is the same failure routing as the execution-phase posting path.

   **BotLogin:** The LGTM body interpolates `BotLogin` (the `BOT_GITHUB_LOGIN` env
   value resolved by the factory) at runtime. No hardcoded bot login literals in
   agent code or templates.

   **Vault `## Verdict` section (LGTM path):**
   ```
   review_id: 12345
   event: COMMENT
   ```

   **Vault `## Verdict` section (full review path — written by ai_review):**
   ```
   review_id: 67890
   event: APPROVE  # or REQUEST_CHANGES
   verdict: pass
   reason: <meta-verdict reason>
   ```

   **Non-GitHub platforms:** If the PR URL resolves to a non-GitHub platform,
   the LGTM path skips posting and returns `done` immediately. No `human_review`
   escalation for platform mismatches.

   **nil poster (cmd/run-task):** When `prPoster` is nil (local CLI mode),
   the LGTM path skips posting and returns `done` without writing `## Verdict`
   or `## Diagnostics`. This preserves backward compatibility with the local
   test runner.
   ```

   Also update the **"The Posting Flow"** section diagram to show the planning-phase
   LGTM branch. The existing diagram shows `postAndRoute` in the execution step. Add
   a note after the diagram:

   ```
   Note: The planning phase has a separate LGTM posting path via `PrPoster.PostLGTM`
   for the empty-concerns route. This path bypasses the worktree checkout and
   posts directly from the planning phase. Failure routes to `human_review` in the
   same way as the execution posting path.
   ```

   Also add a row to the **"Failure Routing"** table documenting the LGTM path:

   | Posting outcome | Class | Next phase |
   |---|---|---|
   | `success` on LGTM POST | any | `done` |
   | `not-a-failure` on LGTM POST | `not-a-failure` | `done` |
   | failure on LGTM POST | `transient` / `permanent` / `unknown` | `human_review` |

2. **Verify the CHANGELOG entry** from prompt 1 is still present (do not modify):

   ```bash
   grep -A2 "## Unreleased" CHANGELOG.md | head -10
   # Expected: feat(agent/pr-reviewer): planning phase now posts an LGTM... entry present
   ```

3. **Run `make precommit`** from `agent/pr-reviewer/`:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only edit `agent/pr-reviewer/docs/pr-post-back.md`
- Do NOT commit — dark-factory handles git
- Do NOT modify the `## Verdict` section format for the ai_review path — that is written by `reviewStep` and documented elsewhere; only add documentation for the LGTM-path format
- The CHANGELOG entry must already exist from prompt 1 — do NOT add it again
- Keep the doc style consistent with existing sections (plain prose, code blocks for examples, tables for routing)
- Do NOT add a new scenario file — the httptest ginkgo tests in prompt 1 cover the observable contract; live cluster verification in Rung 3 covers the integration path
</constraints>

<verification>
cd agent/pr-reviewer && make precommit

# Confirm pr-post-back.md updated with Always-Post Review Invariant section:
grep -n "Always-Post Review Invariant\|empty-concerns\|LGTM.*COMMENT" agent/pr-reviewer/docs/pr-post-back.md
# Expected: section heading and content present

# Confirm LGTM path documented in Failure Routing table:
grep -n "LGTM.*POST\|success on LGTM" agent/pr-reviewer/docs/pr-post-back.md
# Expected: at least one match

# Confirm vault ## Verdict format documented for both paths:
grep -n "review_id.*COMMENT\|review_id.*APPROVE" agent/pr-reviewer/docs/pr-post-back.md
# Expected: both COMMENT and APPROVE format examples present

# Confirm note about planning-phase LGTM path added to Posting Flow section:
grep -n "planning phase.*LGTM\|PostLGTM" agent/pr-reviewer/docs/pr-post-back.md
# Expected: at least one match

# Confirm Non-GitHub and nil poster handling documented:
grep -n "Non-GitHub\|nil poster.*cmd/run-task\|cmd/run-task" agent/pr-reviewer/docs/pr-post-back.md
# Expected: nil poster backward-compat section updated

# Confirm CHANGELOG entry present (from prompt 1; any heading — dark-factory auto-released):
grep -nE "LGTM|no concerns flagged|always.*post" CHANGELOG.md | head -5
# Expected: at least one match anywhere in CHANGELOG.md

# Confirm doc parses as valid markdown (no broken links):
grep -n "^## " agent/pr-reviewer/docs/pr-post-back.md
# Expected: section headings at lines with ## (markdown header syntax)
</verification>
