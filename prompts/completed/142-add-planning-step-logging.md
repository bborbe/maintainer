---
status: completed
summary: Added glog.V(2) logging to all 12 decision/entry points in steps_planning.go (10 physical lines, 2 split by golines), extracted handleEmptyPRURL helper to stay under funlen=80, added Unreleased changelog entry
container: maintainer-exec-142-add-planning-step-logging
dark-factory-version: v0.169.0
created: "2026-05-23T22:58:18Z"
queued: "2026-05-23T22:58:18Z"
started: "2026-05-23T22:58:20Z"
completed: "2026-05-23T23:04:50Z"
---

<summary>
- `agent/pr-reviewer/pkg/steps_planning.go` has zero `glog` calls across 11 distinct `agentlib.Result` return sites — every routing decision (LGTM short-circuit, non-empty concerns → execution, parse failure, non-GitHub URL skip, LGTM POST failure, nil-poster local mode, etc.) returns silently
- Add a single `glog.V(2)` entry log at step start + one `glog.V(2)` log before every return site naming the decision (concern count + nextPhase + message)
- Mirror the existing `steps_review.go` pattern (5 glog calls) so the planning step is symmetric with its siblings
- Pure observability — no behavior change, no new dependencies
</summary>

<objective>
After this work, every planning Job run produces ≥1 log line per decision branch, making routing decisions visible via `kubectl logs <pod>`. A future "why did the bot route to human_review?" investigation becomes a one-grep job (`kubectl logs | grep planning`) instead of source-archaeology + pod-TTL races.
</objective>

<context>
Read CLAUDE.md for project conventions.

**Why now:** discovered 2026-05-23 while diagnosing `bborbe/trading#133` unexpectedly routing to `human_review` despite emitting 5 concerns. The planner code shows non-empty concerns should advance to `execution` (line 105-108 of `steps_planning.go`), but the vault landed at `human_review` with no logs available to confirm which branch ran. Investigation hit a wall because the planner step is silent.

**Files to read before implementing:**
- `agent/pr-reviewer/pkg/steps_planning.go` — the target file. Identify all 11 `return &agentlib.Result{...}` sites at lines 74, 90, 105, 122, 127, 137, 145, 154, 174, 183, 190.
- `agent/pr-reviewer/pkg/steps_review.go` — sibling step that uses glog for skip/error decision points. For planning we standardise on `glog.V(2).Infof` because every branch is a routing decision worth seeing at `-v=2`.

**Logging conventions (existing repo style):**
- `glog.V(2).Infof("step-name: <event> field1=%v field2=%v", ...)` — V(2) is "per-Job decision points"
- Single line per event — no multi-line, no JSON
- Lowercase, terse, machine-greppable: `planning: <event> concerns=N nextPhase=X`
- Never log secrets / IATs / tokens
</context>

<requirements>

1. **Add an entry log** at the top of the planning step's `Run` method (after argument extraction, before any conditional return). Capture the PR URL and ref so an operator can correlate by SHA:

   ```go
   prURL := ExtractPRURL(md)
   ref, _ := md.Frontmatter.String("ref")
   glog.V(2).Infof("planning: starting pr_url=%q ref=%s", prURL, ref)
   ```

   Place this BEFORE the existing logic so it always fires, even for early-exit paths.

2. **Add a decision log before every `return &agentlib.Result{...}` site.** Each log line names the (status, nextPhase) it's about to return + the disambiguating field for that branch. Use this template per site:

   - Line 74 (Claude failure during planning):
     `glog.V(2).Infof("planning: claude failed nextPhase=human_review err=%v", err)`

   - Line 90 (parse error on JSON):
     `glog.V(2).Infof("planning: parse failed nextPhase=human_review err=%v", parseErr)`

   - Line 105 (non-empty concerns → execution):
     `glog.V(2).Infof("planning: %d concerns nextPhase=execution", len(concerns))`

   - Line 122 (non-GitHub PR URL present, `hasAnyPRURL` true):
     `glog.V(2).Infof("planning: non-github PR URL present nextPhase=done")`

   - Line 127 (no PR URL at all → escalate):
     `glog.V(2).Infof("planning: no PR URL nextPhase=human_review")`

   - Line 137 (GitHub-URL string check fails — non-GitHub URL skip):
     `glog.V(2).Infof("planning: non-github PR URL nextPhase=done url=%s", prURLStr)`

   - Line 145 (PR URL parse failure):
     `glog.V(2).Infof("planning: PR URL parse failed nextPhase=human_review url=%q err=%v", prURLStr, parseErr)`

   - Line 154 (non-GitHub platform):
     `glog.V(2).Infof("planning: non-github platform nextPhase=done platform=%s", prInfo.Platform)`

   - Line 174 (LGTM POST failure):
     `glog.V(2).Infof("planning: LGTM POST failed nextPhase=human_review outcome=%s class=%s http=%d err=%s", result.Outcome, result.Class, result.HTTPStatus, result.ErrorMessage)`

   - Line 183 (LGTM POST success):
     `glog.V(2).Infof("planning: LGTM POST success nextPhase=done review_id=%d", result.ReviewID)`

   - Line 190 (nil poster — cmd/run-task local mode):
     `glog.V(2).Infof("planning: nil poster (local mode) nextPhase=done")`

   Adapt the exact field names if local variables differ — the principle is: name the decision branch + nextPhase + any disambiguating value (error, count, URL, review id).

3. **Do NOT add logs anywhere else.** No mid-iteration logging, no entry-into-helpers logging. Decision points only — one per branch.

4. **Run `make precommit`** in `agent/pr-reviewer/`:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```

   Must exit 0.

5. **Add a CHANGELOG entry** under `## Unreleased` in project root `CHANGELOG.md`. The project does not currently have an `## Unreleased` section — top section is `## v0.26.4`. Create `## Unreleased` immediately above the top-most existing version heading:

   ```markdown
   - chore(agent/pr-reviewer): add `glog.V(2)` logging to every planning-step return site so routing decisions (LGTM short-circuit, execution advance, human_review escalation, POST failures) are visible in pod logs; mirrors the existing `steps_review.go` pattern
   ```

</requirements>

<constraints>
- Single file changed: `agent/pr-reviewer/pkg/steps_planning.go`
- Plus one test file IF existing tests rely on log capture or stderr buffering (unlikely; do not add new tests for log content)
- Plus one CHANGELOG line under `## Unreleased`
- All new logs at `V(2)` — operators can disable via `/setloglevel/1` if too noisy
- No behavior change: the function returns the same `Result` for the same inputs as before
- Total new log call sites: 12 (1 entry + 11 decisions). Do not exceed 13.
- Error wrapping convention unchanged (`errors.Wrapf` for actual error wrapping; `glog.V(2).Infof` is for observability only)
- Do NOT commit — dark-factory handles git
- License header on any new file (n/a; we're only editing an existing file)
</constraints>

<verification>
```bash
# AC1: 12-13 new glog.V(2) calls in the file
grep -c 'glog.V(2).Infof("planning:' agent/pr-reviewer/pkg/steps_planning.go
# Expected: 12 (1 entry + 11 decisions); accept 13

# AC2: entry log is first glog call in the file
grep -n 'glog' agent/pr-reviewer/pkg/steps_planning.go | head -1 | grep -q 'starting pr_url' && echo OK
# Expected: OK

# AC3: every distinct decision branch is named (catches mislabel + copy-paste errors)
for branch in 'starting pr_url' 'parse failed' 'concerns nextPhase=execution' 'non-github PR URL present' 'no PR URL nextPhase=human_review' 'non-github PR URL nextPhase=done' 'PR URL parse failed' 'non-github platform' 'LGTM POST failed' 'LGTM POST success' 'nil poster'; do
  grep -q "planning: $branch" agent/pr-reviewer/pkg/steps_planning.go || echo "MISSING: $branch"
done
# Expected: no MISSING lines

# AC4: precommit green
cd agent/pr-reviewer && make precommit
# Expected: exit 0

# AC5: CHANGELOG entry under Unreleased (note -E for alternation)
grep -A5 '## Unreleased' CHANGELOG.md | grep -qE 'planning-step|steps_planning|glog\.V\(2\)'
# Expected: match
```
</verification>
