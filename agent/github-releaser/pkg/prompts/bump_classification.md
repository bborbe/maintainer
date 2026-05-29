# Classify the next semantic-version bump

You are classifying a software release. Given the CHANGELOG bullets for the upcoming
release, decide whether the next version bump is `patch`, `minor`, or `major`.

## Rules (apply in order)

Evaluate the bullets in priority order: major → minor → patch. The FIRST rule that
matches wins. Do not pick a weaker bump when a stronger one applies.

1. **major** — at least one bullet describes a BREAKING CHANGE: a removed or renamed
   public API, an incompatible behavior change, a config key removal, a database
   migration that is not backwards compatible, or any change that requires callers
   to update their code or configuration.
2. **minor** — at least one bullet starts with `feat:` or otherwise describes a new
   additive capability (new flag, new endpoint, new exported function) that does
   NOT break existing callers.
3. **patch** — everything else: bug fixes, refactors, doc edits, dependency bumps,
   test additions, internal cleanup.

Note: if a bullet contains BOTH a `feat:` prefix AND the literal text `BREAKING CHANGE`,
the correct answer is `major` — priority order is strict.

## Output

Output a single JSON object inside a fenced code block. The output MUST be valid JSON
with exactly two string fields. Do not include any prose outside the fenced block.

The `bump` field MUST be one of: patch | minor | major.

```json
{
  "bump": "patch",
  "reasoning": "one sentence justifying the classification, referencing the deciding bullet"
}
```