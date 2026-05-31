---
status: completed
spec: [056-github-releaser-ai-review-phase]
summary: Enabled ai_review phase in trigger.phases by removing the omission comment and adding ai_review to the phase list
container: maintainer-releaser-ai-review-exec-222-spec-056-k8s-manifest
dark-factory-version: v0.173.0
created: "2026-05-31T20:35:00Z"
queued: "2026-05-31T20:54:57Z"
started: "2026-05-31T21:05:19Z"
completed: "2026-05-31T21:05:41Z"
branch: dark-factory/github-releaser-ai-review-phase
---

<summary>
- Kubernetes Config manifest lists all three phases under trigger.phases
- Removed "ai_review intentionally omitted" comment
- No other manifest fields changed
</summary>

<objective>
Update the Kubernetes Config for the github-releaser agent to list all three phases (`planning`, `execution`, `ai_review`) under `trigger.phases`, removing the comment that explicitly omits `ai_review`. This change enables the controller to dispatch `ai_review` events to the agent.
</objective>

<context>
Read `agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml` to see the current trigger.phases configuration.
Read the spec's Failure Modes table for the "Two ai_review dispatches arrive concurrently" entry (controller lock serialization).
</context>

<requirements>
1. Open `agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml`.

2. In the `trigger.phases` block, remove the comment lines:
   ```
   # ai_review intentionally omitted until the ## Review step is implemented
   # (a separate spec). The execution step returns NextPhase=ai_review, but
   # the controller will not trigger a phase the agent does not yet handle.
   ```

3. Add `ai_review` as a listed phase alongside `planning` and `execution`. The final `trigger.phases` section must be:
   ```yaml
   trigger:
     statuses:
       - in_progress
     phases:
       - planning
       - execution
       - ai_review
   ```

4. Make no other changes to the manifest. The phase list is the only change.

5. After editing, confirm `grep -n 'intentionally omitted' agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml` returns no match.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Make no other changes to the manifest.
- The `ai_review` phase value is a string literal in the YAML — matching the factory's `domain.TaskPhaseAIReview = "ai_review"` by construction.
</constraints>

<verification>
Two assertions, both must pass (matches spec AC #3):

1. Positive: `grep -A4 'phases:' agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml` MUST output three lines `- planning`, `- execution`, `- ai_review` directly under the `phases:` key (no comments between them).
2. Negative: `grep -n 'intentionally omitted' agent/github-releaser/k8s/maintainer-agent-github-releaser.yaml` MUST return no match (exit 1).

The combination guarantees the comment block was removed AND all three phases are present — neither assertion alone is sufficient (a stale comment containing `ai_review` would pass an unscoped alternation grep).
</verification>