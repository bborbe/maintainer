---
status: draft
spec: [056-ai-review-actionable]
created: "2026-06-01T00:00:00Z"
branch: dark-factory/ai-review-actionable
---

<summary>
- Updates the Obsidian `GitHub PR Reviewer Agent.md` page with a new subsection documenting the hallucination dismiss-and-comment behavior and the `## Parked Because` section meaning
- No code changes — documentation only
</summary>

<objective>
Update the Obsidian knowledge base page `GitHub PR Reviewer Agent.md` to document the two new behaviors added by spec 056: (a) the dismiss-and-comment behavior triggered when ai_review flags hallucinations, and (b) the `## Parked Because` section written into re-spawned tasks after a prior ai_review fail.
</objective>

<context>
Read these files before making changes:
- The spec's Acceptance Criteria section (criteria #12) — the documentation requirement
- The existing Obsidian file (if accessible from the container) at `/workspace/Users/bborbe/Documents/Obsidian/Personal/50 Knowledge Base/GitHub PR Reviewer Agent.md`

Read coding docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — changelog style (if changelog update needed)

Note: The Obsidian file path is a macOS host path. The container may or may not have access. If the file is not accessible from the container, use `grep -nE "Parked Because|hallucinated review" "<path>"` to verify the file exists at the host path, then write the updated content and note the path in the completion report so dark-factory can copy it to the correct location.
</context>

<requirements>
1. **Locate the Obsidian file.** Try reading at the host path:
   ```bash
   grep -n "ai_review\|## Verdict\|verdict:" "/Users/bborbe/Documents/Obsidian/Personal/50 Knowledge Base/GitHub PR Reviewer Agent.md" 2>/dev/null
   ```
   If the file is not accessible from the container, proceed to step 3 (write the updated content to a temp location).

2. **Read the existing file** to find where to insert the new documentation. Look for sections related to ai_review, verdict, or phase routing.

3. **Append a new subsection** to the file (or create the file if it doesn't exist) with this content:

   ```markdown
   ## Hallucination Dismissal & Parked Because

   ### Dismiss-and-comment behavior

   When the ai_review step returns `verdict: fail` with at least one hallucination entry, the agent performs two GitHub actions before routing to `human_review`:

   1. **Dismissal**: Calls `PUT /repos/{owner}/{repo}/pulls/{n}/reviews/{review_id}/dismissals` on the bot's review at the current head SHA (state `APPROVED` or `CHANGES_REQUESTED`). The dismissal message is `"hallucinated review — see follow-up COMMENT for evidence"`.
   2. **Follow-up COMMENT**: Posts a new GitHub review with `event: COMMENT` at the same head SHA, listing each hallucination in the format:
      ```
      hallucinated review — see follow-up COMMENT for evidence

      - {file}:{line} — {issue}
      ```
   3. **Routing**: After both steps (whether successful or failed), the task routes to `human_review`. A human owns the final call; dismissal only unblocks the merge gate — it does not auto-merge.

   **Trigger gate**: All three conditions must be true simultaneously:
   - `verdict == "fail"`
   - `len(hallucinations) > 0`
   - Bot-authored review exists at current head SHA in state `APPROVED` or `CHANGES_REQUESTED`

   If any condition fails (e.g. `verdict: fail` with empty hallucinations), no dismissal is made and routing matches the existing behavior.

   **Diagnostics**: Dismissal outcomes (success, 404, 422, 5xx) are logged to `## Diagnostics` with the prefix `ai_review dismiss:`.

   ### ## Parked Because section

   When the watcher re-spawns a task for the same PR after a prior task had `ai_review verdict: fail`, the spawned task body contains a `## Parked Because` section (the ONLY top-level section — no parent `# PR Review` wrapper):

   ```markdown
   ## Parked Because

   - **Prior task ID:** `{task-uuid}`
   - **Prior head SHA:** `{sha}`
   - **Prior verdict:** `fail`

   **Hallucinations from prior ai_review:**

   - {file}:{line} — {issue}
   - {file}:{line} — {issue}
   ```

   The section appears ONLY for the prior-ai-review-fail reason. Untrusted-author parks use the existing `## Untrusted author` body and do NOT include `## Parked Because`.
   ```

4. **Verify the change** by running:
   ```bash
   grep -nE "Parked Because|hallucinated review" "/Users/bborbe/Documents/Obsidian/Personal/50 Knowledge Base/GitHub PR Reviewer Agent.md"
   ```
   The grep should return at least 2 matches (one for each new section title or body).

5. **Note in completion report**: If the file was not accessible from the container, document the full updated file content and the exact host path so dark-factory can apply it.
</requirements>

<constraints>
- Do NOT change any existing sections in the file
- Only add new content at an appropriate location (end of file or after the most related existing section)
- Preserve the existing file's formatting style (markdown headers, bullet points, etc.)
- If the file does not exist at the host path, create it with the new subsection only — do not fabricate unrelated existing content
</constraints>

<verification>
```bash
grep -nE "Parked Because|hallucinated review" "/Users/bborbe/Documents/Obsidian/Personal/50 Knowledge Base/GitHub PR Reviewer Agent.md"
```

Expected: at least 2 matches returned.
</verification>