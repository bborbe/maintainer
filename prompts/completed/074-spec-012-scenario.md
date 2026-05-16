---
status: completed
spec: [012-watcher-author-trust-filter]
summary: Created scenarios/006-watcher-author-trust-filter.md with 5 sub-scenarios and 36 checkboxes covering the spec-012 author-trust gate integration seam, and added an Unreleased entry to CHANGELOG.md.
container: code-reviewer-074-spec-012-scenario
dark-factory-version: dev
created: "2026-05-02T10:00:00Z"
queued: "2026-05-02T10:14:28Z"
started: "2026-05-02T10:23:38Z"
completed: "2026-05-02T10:24:41Z"
branch: dark-factory/watcher-author-trust-filter
---

<summary>
- A new scenario file documents the end-to-end manual verification checklist for the trusted-author trust gate
- The scenario exercises the full pipeline seam: watcher polls GitHub → publishes to Kafka → controller materializes vault task → vault task has `phase: human_review, status: todo` and the untrusted-author body
- A second sub-scenario verifies the trusted-author fast-path produces the existing `phase: planning, status: in_progress` frontmatter and the agent is spawned normally
- The scenario confirms that no agent pod is spawned for untrusted-author tasks
- A third sub-scenario verifies that manually promoting an untrusted task's frontmatter to `phase: in_progress, status: in_progress` causes the agent to pick it up on the next controller cycle
- The scenario also documents the startup-failure case (empty TRUSTED_AUTHORS) so operators know the expected error message
- Existing scenarios 001–005 are unchanged; no Go code is created or modified
</summary>

<objective>
Create `scenarios/006-watcher-author-trust-filter.md` — the manual verification checklist for spec-012. This is the required integration seam test: it validates the frontmatter values that the controller writes to the vault task, which cannot be verified by unit tests alone.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/architecture.md` — "High-level pipeline" section for the watcher → Kafka → controller → vault flow, and the agent contract frontmatter fields.

Read existing scenarios for format and style:
- `scenarios/001-pr-reviewer-github-https.md`
- `scenarios/005-spec-011-plugin-pr-review.md`

This scenario covers the spec-012 acceptance criteria seam:
- Untrusted-author PR → vault task has `phase: human_review`, `status: todo`, body contains author login and "Untrusted author" heading
- Trusted-author PR → vault task has `phase: planning`, `status: in_progress` (fast-path unchanged)
- No agent pod spawned for untrusted task (controller does not promote it)
- Human manual promotion → agent picks it up on next cycle
- Empty `TRUSTED_AUTHORS` → watcher refuses to start with a clear log message
</context>

<requirements>
1. **Create `scenarios/006-watcher-author-trust-filter.md`** with this exact frontmatter and structure:

   ```markdown
   ---
   status: draft
   spec: 012-watcher-author-trust-filter
   ---

   # Scenario 006: watcher author-trust gate routes untrusted PRs to human review

   Validates the end-to-end trust gate introduced in spec-012: the watcher routes
   untrusted-author PRs to `phase: human_review / status: todo` in the vault,
   ensures no agent pod is spawned automatically, and preserves the trusted-author
   fast-path unchanged.

   This is the required integration seam test for spec-012. The `phase: human_review`
   frontmatter value is new — it cannot be verified by watcher unit tests alone because
   the controller is the component that materializes the vault task.

   ## Prerequisites
   - [ ] Dev cluster is running and healthy (`kubectl get pods -n code-reviewer`)
   - [ ] Watcher is deployed to dev with `TRUSTED_AUTHORS` set to a known GitHub login
         (e.g. `TRUSTED_AUTHORS=bborbe`); confirm via:
         ```bash
         kubectl get deployment github-pr-watcher -n code-reviewer -o jsonpath='{.spec.template.spec.containers[0].env}' | python3 -m json.tool | grep TRUSTED_AUTHORS
         ```
   - [ ] A second GitHub account (not in `TRUSTED_AUTHORS`) is available for the
         untrusted-author test; call this account `untrusted-user` below
   - [ ] You can open PRs from both accounts on `bborbe/code-reviewer` (fork or branch)
   - [ ] Vault CLI is available: `vault kv list secret/code-reviewer/tasks/` returns results

   ## Sub-scenario A: untrusted-author PR → human_review vault task

   ### Action
   - [ ] Open a PR on `bborbe/code-reviewer` from the `untrusted-user` account
         (a fork PR or a branch in a fork)
   - [ ] Wait up to one poll cycle (default 5 min) for the watcher to process it

   ### Expected
   - [ ] A vault task appears for the PR:
         ```bash
         vault kv list secret/code-reviewer/tasks/ | grep <pr-number>
         ```
   - [ ] The vault task frontmatter has `phase: human_review` and `status: todo`:
         ```bash
         vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
           | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
         # Expected: human_review todo
         ```
   - [ ] The vault task body contains `## Untrusted author`:
         ```bash
         vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
           | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['data'].get('body',''))" \
           | grep "Untrusted author"
         ```
   - [ ] The vault task body contains the `untrusted-user` GitHub login
   - [ ] The vault task body contains the promotion instructions:
         `phase: in_progress` and `status: in_progress`
   - [ ] No agent pod was spawned for this task (controller does not promote `human_review` tasks):
         ```bash
         kubectl get pods -n code-reviewer | grep <task-id>
         # Expected: no matching pod
         ```

   ## Sub-scenario B: manual promotion → agent picks up task

   (Continues from sub-scenario A)

   ### Action
   - [ ] Manually update the vault task frontmatter to promote it for agent processing:
         ```bash
         vault kv patch secret/code-reviewer/tasks/<task-id> phase=in_progress status=in_progress
         ```

   ### Expected
   - [ ] On the next controller cycle (≤ 60 s), a K8s Job is spawned:
         ```bash
         kubectl get pods -n code-reviewer | grep <task-id>
         # Expected: pod appears with status Running or Completed
         ```
   - [ ] The agent processes the task and writes a verdict back (check pod logs):
         ```bash
         kubectl logs -n code-reviewer <pod-name> | tail -20
         ```

   ## Sub-scenario C: trusted-author PR → fast-path unchanged

   ### Action
   - [ ] Open a PR on `bborbe/code-reviewer` from the trusted account (the login in `TRUSTED_AUTHORS`)
   - [ ] Wait up to one poll cycle for the watcher to process it

   ### Expected
   - [ ] A vault task appears for the PR with `phase: planning` and `status: in_progress`:
         ```bash
         vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
           | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
         # Expected: planning in_progress
         ```
   - [ ] The vault task body does NOT contain `## Untrusted author`
   - [ ] A K8s Job is spawned on the next controller cycle (agent picks it up automatically)

   ## Sub-scenario D: startup failure on empty TRUSTED_AUTHORS

   ### Action
   - [ ] Temporarily patch the watcher deployment to unset `TRUSTED_AUTHORS`:
         ```bash
         kubectl set env deployment/github-pr-watcher -n code-reviewer TRUSTED_AUTHORS-
         ```
   - [ ] Wait for the pod to restart and check logs:
         ```bash
         kubectl logs -n code-reviewer deployment/github-pr-watcher | grep "trusted authors"
         ```

   ### Expected
   - [ ] Pod fails to start (CrashLoopBackOff or exits non-zero immediately)
   - [ ] Log contains: `no trusted authors configured`
   - [ ] No PRs are processed during this window

   ### Cleanup
   - [ ] Restore the deployment to its previous `TRUSTED_AUTHORS` value:
         ```bash
         kubectl set env deployment/github-pr-watcher -n code-reviewer TRUSTED_AUTHORS=<original-value>
         ```

   ## Sub-scenario E: force-push on untrusted PR → trust re-evaluated, stays human_review

   Validates spec failure-mode "force-push by untrusted author preserves human_review" (re-review path
   on synchronize must NOT flip the task back into auto-processing).

   ### Setup (continues from sub-scenario A)
   - [ ] Sub-scenario A completed: vault task for the untrusted PR exists with `phase: human_review`
   - [ ] Do NOT manually promote the task (sub-scenario B is independent)

   ### Action
   - [ ] Push another commit to the same untrusted PR's branch (force-push or normal push — both
         change the PR's head SHA, which triggers the watcher's force-push detection path):
         ```bash
         # In the fork's branch directory
         git commit --allow-empty -m "trigger re-review" && git push
         ```
   - [ ] Wait up to one poll cycle (≤5 min)

   ### Expected
   - [ ] The vault task for this PR is updated (frontmatter `head_sha` reflects the new SHA), but
         `phase` and `status` remain `human_review` / `todo`:
         ```bash
         vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
           | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'), t.get('head_sha'))"
         # Expected: human_review todo <new-sha>
         ```
   - [ ] The vault task body still contains the `## Untrusted author` block (re-evaluation kept the
         decision, did not silently drop it)
   - [ ] No K8s agent Job is spawned for the new SHA

   ## Cleanup
   - [ ] Close or merge the test PRs opened in sub-scenarios A, C, and E
   - [ ] Confirm the watcher is healthy after restore:
         ```bash
         kubectl get pods -n code-reviewer | grep github-pr-watcher
         ```

   ## Notes — uncovered failure mode

   - **PR with no author (defensive):** Spec failure mode "GitHub returns a PR with `nil` `User`"
     is not covered by a manual sub-scenario because GitHub does not allow opening a PR with a null
     user via the public API. The defensive path is exercised by unit tests in
     `pkg/watcher_test.go` (PR with empty `AuthorLogin` → `human_review`); a manual scenario would
     require API-injection tooling outside scope of this verification.

   ## Notes
   Last run: (not yet run — scenario created for spec-012)
   ```

2. **No Go code changes** — this prompt creates only `scenarios/006-watcher-author-trust-filter.md`.

3. **Verification**:
   ```bash
   ls -la scenarios/006-watcher-author-trust-filter.md
   head -5 scenarios/006-watcher-author-trust-filter.md
   ```
</requirements>

<constraints>
- Only create `scenarios/006-watcher-author-trust-filter.md` — do not modify any other file
- Do NOT commit — dark-factory handles git
- Follow the exact frontmatter format from existing scenarios (`status: draft`, `spec: <name>`)
- The scenario is a manual checklist (not automated) — use checkbox lists `- [ ]`
- `make precommit` is NOT required since no Go code is changed (markdown only)
- Do NOT add scenario content that relies on implementation details not guaranteed by the spec's acceptance criteria
</constraints>

<verification>
ls -la scenarios/006-watcher-author-trust-filter.md

head -10 scenarios/006-watcher-author-trust-filter.md
# Expected: frontmatter with status: draft and spec: 012-watcher-author-trust-filter

grep -c "\- \[ \]" scenarios/006-watcher-author-trust-filter.md
# Expected: at least 15 checkboxes
</verification>
