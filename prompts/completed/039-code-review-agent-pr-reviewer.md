---
status: completed
summary: Ran full code review of agent/pr-reviewer with 13 specialist agents and generated 11 fix prompts covering 2 critical security findings and 9 important architectural/quality findings
container: code-reviewer-039-code-review-agent-pr-reviewer
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-27T22:24:13Z"
queued: "2026-04-27T22:28:25Z"
started: "2026-04-27T22:29:35Z"
completed: "2026-04-27T23:14:22Z"
---

<summary>
- Service reviewed using full automated code review with all specialist agents
- Fix prompts generated for each Critical or Important finding
- Each fix prompt is independently verifiable and scoped to one concern
- No code changes made — review-only prompt that produces fix prompts
- Clean services produce no fix prompts
</summary>

<objective>
Run a full code review of `agent/pr-reviewer` and generate a fix prompt for each Critical or Important finding.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for Definition of Done criteria (if exists).

Read 3 recent completed prompts from the prompts completed directory (highest-numbered) to understand prompt style and XML tag structure.

Service directory: `agent/pr-reviewer/`
</context>

<requirements>

## 1. Read Config

Read `.dark-factory.yaml` to find `prompts.inboxDir` (default: `prompts`). Use this as the output directory for fix prompts.

## 2. Run Code Review

Run `/coding:code-review full agent/pr-reviewer` to get a comprehensive review with all specialist agents.

Collect the consolidated findings categorized as:
- **Must Fix (Critical)** — will generate fix prompts
- **Should Fix (Important)** — will generate fix prompts
- **Nice to Have** — skip, do NOT generate prompts

## 3. Generate Fix Prompts

For each Critical or Important finding (or group of related findings in the same file/package), write a prompt file to the prompts inbox directory.

**Filename:** `review-agent-pr-reviewer-<fix-description>.md`

**Pre-fill the child prompt's `<context>` section** with the actual files cited by the finding (file paths + line numbers as hints) — do NOT leave the placeholder string "list specific files with line numbers as hints" in the generated prompt

Each fix prompt must follow this exact structure:

```
---
status: draft
created: "<current UTC timestamp in ISO8601>"
---

<summary>
5-10 plain-language bullets. No file paths, struct names, or function signatures.
</summary>

<objective>
What to fix and why (1-3 sentences). End state, not steps.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- list specific files with line numbers as hints
</context>

<requirements>
Numbered, specific, unambiguous steps.
Anchor by function/type name (~line N as hint only).
Include function signatures where helpful.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` (always pass `ctx` as first arg) — never `fmt.Errorf`, never bare `return err`
- Code changes MUST add or update tests for changed paths — paths covered before the fix must remain covered, new paths must be tested
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
```

**Grouping rules:**
- One concern per prompt (e.g., "fix error wrapping in package X")
- Group coupled findings that must change together
- Split unrelated findings into separate prompts
- Soft cap: ≤5 files per fix prompt — if a finding spans more files, split into multiple sequenced prompts (`1-`, `2-`, …)
- If order matters, prefix filenames with `1-`, `2-`, `3-`

## 4. Summary

Print a summary of findings and generated prompt files.

</requirements>

<constraints>
- Do NOT modify any source code — this is a review-only prompt
- Only write files to the prompts inbox directory
- Never write to `in-progress/` or `completed/` subdirectories
- Never number prompt filenames — dark-factory assigns numbers on approve
- Repo-relative paths only in generated prompts (no absolute, no `~/`)
- If no findings at Critical/Important level → report clean bill of health, generate no prompts
</constraints>

<verification>
This prompt only generates markdown files — no code changes, no build needed.
ls prompts/review-agent-pr-reviewer-*.md 2>/dev/null || echo "no findings — clean bill of health"
</verification>
