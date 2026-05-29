---
status: approved
spec: [052-fleet-app-only-auth]
created: "2026-05-29T18:33:00Z"
queued: "2026-05-29T18:24:44Z"
completed: "2026-05-29T18:32:10Z"
lastFailReason: |-
    setup workflow: git merge origin default branch: merge origin/master: error: Merging is not possible because you have unmerged files.
    hint: Fix them up in the work tree, and then use 'git add/rm <file>'
    hint: as appropriate to mark resolution and make a commit.
    fatal: Exiting because of an unresolved conflict.: exit status 128
---

<summary>
- Audits the four services for any leftover `GH_TOKEN` wiring in env files and k8s manifests, now that the PAT input has been removed from the code.
- Reports each remaining `GH_TOKEN` reference, classifying it as a live env declaration (to remove) or a comment/doc mention (to leave or correct).
- Removes any trivially-isolated live `GH_TOKEN` env declaration for these four services so no orphaned auth env lingers.
- Corrects any comment that now misstates auth behavior, without deleting purely historical mentions.
- Does not change auth env in deployed manifests beyond removing an orphaned declaration — App env vars are untouched.
- Produces a written classification table in the implementation output for the human reviewer.
</summary>

<objective>
Surface and clean up stale `GH_TOKEN` environment wiring (env files, k8s manifests) for the four App-only services now that the `GH_TOKEN` PAT *input* has been removed from their code, per spec Desired Behavior 6.
</objective>

<context>
This prompt runs AFTER the three code-removal prompts (`1-`, `2-`, `3-`) have removed the `GH_TOKEN` input from `agent/pr-reviewer`, `watcher/github-pr`, `watcher/github-build`, and `watcher/github-release`.

Read `CLAUDE.md` at the repo root and `/home/node/.claude/plugins/marketplaces/coding/docs/k8s-manifest-guide.md` for manifest conventions.

**Known state from the pre-work audit (verify, do not assume):**
- `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml` contains a `GH_TOKEN` reference, but it is a *comment* ("The legacy GH_TOKEN PAT was retired 2026-05-24...") in the env-documentation block — NOT a live env declaration. This is a historical/explanatory comment; it does not misstate current behavior (it correctly says GH_TOKEN was retired). Leave it.
- No `*.env` file under the four services declared a live `GH_TOKEN` at audit time. Re-verify.

**Distinction (spec Desired Behavior 6):**
- A *live env declaration* (e.g. `GH_TOKEN: '{{ ... }}'` in a k8s manifest, or `GH_TOKEN=...` in a `.env` file) for one of the four services is now orphaned → remove it if trivial and isolated.
- A *comment or doc mention* of `GH_TOKEN` is not a live declaration → leave it, UNLESS it now misstates current behavior, in which case correct the wording.
</context>

<requirements>

1. **Run the audit grep** (from the worktree root `/workspace`) and capture the full output:

   ```bash
   grep -rn 'GH_TOKEN' --include='*.env' --include='*.yaml' --include='*.yml' \
     /workspace/agent/pr-reviewer /workspace/watcher/github-pr \
     /workspace/watcher/github-build /workspace/watcher/github-release
   ```

2. **Classify every hit** as one of:
   - **live env declaration** — an actual env-var assignment for the service (k8s `env:`/`stringData:` key, a `GH_TOKEN: ...` config-CR line, or a `GH_TOKEN=...` line in a `.env`/`*.env` file). These are orphaned now and should be removed (see step 3).
   - **comment / doc mention** — text inside a `#` comment, a Go-style comment in a non-Go config, or prose that merely references `GH_TOKEN`. Leave it, unless it misstates current behavior → correct the wording (do NOT delete a correct historical note).

3. **Remove live env declarations** that are trivial and isolated (a single orphaned key with no other coupling). For each removed line, ensure no other manifest construct (e.g. a `secretKeyRef`, an `envFrom`, or a templating reference) is left dangling. If a `GH_TOKEN` removal would require non-trivial restructuring of a manifest or touches shared infra beyond these four services, do NOT remove it — instead report it as "live, NOT removed (non-trivial)" with a one-line reason, leaving it for a follow-up.

4. **Correct misstating comments only.** If a comment claims `GH_TOKEN` is "accepted as a fallback" or "used when App creds absent" — that is now false → reword to reflect App-only behavior. A comment that says GH_TOKEN "was retired" or is "legacy" and explains history is accurate → leave it.

5. **Do NOT add or change any App auth env** (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY`, `PEM_KEY_FILE`) in any manifest — they already exist and are correct. This prompt only removes orphaned `GH_TOKEN` wiring and fixes stale comments.

6. **Produce a classification table in the implementation output** (the agent's final written report, not a committed file) listing every hit from step 1 with its classification and the action taken, e.g.:

   ```
   FILE                                                   | LINE | CLASSIFICATION            | ACTION
   agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml | 38   | comment/doc mention       | left (accurate historical note)
   ...
   ```

   If the grep returns zero hits, state that explicitly: "No GH_TOKEN env wiring found for the four services."

7. **Verify no live `GH_TOKEN` env declaration remains** for the four services after edits:

   ```bash
   grep -rn 'GH_TOKEN' --include='*.env' --include='*.yaml' --include='*.yml' \
     /workspace/agent/pr-reviewer /workspace/watcher/github-pr \
     /workspace/watcher/github-build /workspace/watcher/github-release
   ```

   Every surviving hit must be a comment/doc mention (or a "live, NOT removed (non-trivial)" item explicitly reported in step 6).

8. **If a code module's manifest was edited, do NOT run `make precommit` for a manifest-only change** unless the module's Makefile validates manifests; if it does (e.g. a `kubeval`/`yamllint` target runs in precommit), run `make precommit` in the affected module to confirm the manifest still validates. Otherwise, a YAML lint of the edited file is sufficient.

</requirements>

<constraints>
- This is an audit + minimal-cleanup prompt — do NOT touch Go code (the code prompts `1-`/`2-`/`3-` own that).
- Do NOT add, change, or remove App auth env (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY`, `PEM_KEY_FILE`).
- Do NOT delete accurate historical comments that mention `GH_TOKEN`.
- Do NOT touch manifests or env for any service other than the four named.
- Do NOT modify `agent/github-releaser` (out of scope per spec Non-goals).
- Manifest infra changes beyond removing an orphaned `GH_TOKEN` declaration are out of scope — report, do not restructure.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
```bash
# Final audit — every surviving hit must be a comment/doc mention (or explicitly
# reported as "live, NOT removed (non-trivial)" in the implementation output).
grep -rn 'GH_TOKEN' --include='*.env' --include='*.yaml' --include='*.yml' \
  /workspace/agent/pr-reviewer /workspace/watcher/github-pr \
  /workspace/watcher/github-build /workspace/watcher/github-release
```
The implementation output MUST contain the classification table from requirement 6.
</verification>
