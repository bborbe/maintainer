---
status: completed
spec: [060-github-releaser-major-bump-guard]
summary: Wired --allow-major / ALLOW_MAJOR CLI flag through both application structs and the BuildEnv helper; both entry points and the test suite compile and pass with the new fifth BuildEnv parameter
container: maintainer-major-bump-guard-exec-235-spec-060-cli-flag-buildenv-allowmajor
dark-factory-version: v0.175.0
created: "2026-06-03T15:05:00Z"
queued: "2026-06-03T14:34:34Z"
started: "2026-06-03T14:38:04Z"
completed: "2026-06-03T14:42:25Z"
---

<summary>
- The github-releaser-agent binary gains a new CLI flag `--allow-major` (env `ALLOW_MAJOR`, default `false`) on both entry points — the Kafka `agent/github-releaser/main.go` and the local-CLI `agent/github-releaser/cmd/run-task/main.go`
- A per-run override is now possible: an operator can re-fire a stuck task with `--allow-major=true` or `ALLOW_MAJOR=true` instead of editing the target repo's `.maintainer.yaml` and waiting for the commit to propagate
- `BuildEnv(ghToken, anthropicBaseURL, anthropicAuthToken, anthropicModel, allowMajor bool)` signature gains a fifth parameter; the env map forwards `ALLOW_MAJOR=true` to the Claude subprocess only when the flag is set (same pattern as the other optional env vars)
- Both `main.go` call sites (Kafka entry + local CLI entry) pass `a.AllowMajor` to `BuildEnv`; no global / package-level state, no signature change to `factory.CreateAgent` in this prompt
- This is the second lever for the spec 060 guard (the first lever — `release.allowMajorBump` in `.maintainer.yaml` — is shipped in prompt 1); the planning step (prompt 3) consumes both values to decide whether a `major` bump verdict can proceed
</summary>

<objective>
Wire the `--allow-major` / `ALLOW_MAJOR` CLI flag from the `application` struct in both `agent/github-releaser/main.go` and `agent/github-releaser/cmd/run-task/main.go` through the `pkg.BuildEnv` helper, so the value is forwarded to the Claude subprocess env ONLY when set true. The planning step (prompt 3) will pick up the value via a new `NewPlanningStep` parameter (covered in that prompt) — this prompt ships only the CLI plumbing + `BuildEnv` signature change. The `factory.CreateAgent` signature does NOT change in this prompt; the new value stays inside the env map (consumed by the planning step via a follow-up constructor parameter in prompt 3).
</objective>

<context>
Read `CLAUDE.md` and `agent/github-releaser/CLAUDE.md` for project conventions.

Read these files BEFORE editing:
- `/workspace/agent/github-releaser/main.go` — Kafka entry point. The `application` struct lives at lines 50-86; the `BuildEnv` call site is at lines 125-130. Add the new `AllowMajor` field to the struct and pass `a.AllowMajor` as the new fifth `BuildEnv` argument. The struct fields are the canonical `libargument` pattern (`required:"false" arg:"..." env:"..." usage:"..." default:"..."`) — copy the existing `AnthropicModel` field shape, NOT the `SentryDSN` shape, because `AnthropicModel` is the closest semantic match (an env-forwarded boolean-as-string with a default).
- `/workspace/agent/github-releaser/cmd/run-task/main.go` — local-CLI entry point. The `application` struct mirrors the Kafka entry but is smaller (no Kafka / task controller). The `BuildEnv` call site is at lines 79-84. Add the same field to this struct with the same tag set, and pass `a.AllowMajor` to the new `BuildEnv` call. The two `application` structs are deliberately redundant; do not attempt to share them — the spec 060 § Constraints require the flag on BOTH entry points.
- `/workspace/agent/github-releaser/pkg/buildenv.go` — the `BuildEnv` function. Current signature is `func BuildEnv(ghToken, anthropicBaseURL, anthropicAuthToken, anthropicModel string) map[string]string`. Add a fifth parameter `allowMajor bool` (NOT a pointer — a plain `bool`, mirroring how the planning step (prompt 3) will read it as a constructor parameter). The function body appends `if allowMajor { env["ALLOW_MAJOR"] = "true" }` — same gating pattern as the other optional env vars.
- `/workspace/agent/github-releaser/pkg/factory/factory.go` — DO NOT EDIT in this prompt. `CreateAgent` builds the planning step today with `(planningRunner, fetcher, maintainerConfigFetcher)`. Prompt 3 changes the `NewPlanningStep` signature to add a new parameter (the resolved `allowMajor` value); the factory is updated in that prompt. This prompt leaves the factory alone so the diffs are atomic and the cross-prompt dependency is explicit.
- `/workspace/agent/github-releaser/pkg/steps_planning.go` — DO NOT EDIT in this prompt. The planning step picks up the value in prompt 3. Read it for context only so you understand how the value flows after the BuildEnv call.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cli-guide.md` — `libargument` struct-tag conventions and the `required:"false" arg:"..." env:"..." usage:"..." default:"..."` pattern.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

Verified symbols (from module source — grep-confirmed):
- `pkg.BuildEnv(ghToken, anthropicBaseURL, anthropicAuthToken, anthropicModel string) map[string]string` from `agent/github-releaser/pkg/buildenv.go`. The current body is the four `if X != "" { env["X"] = X }` blocks. Add a fifth `if allowMajor { env["ALLOW_MAJOR"] = "true" }` block as the LAST branch.
- `application` struct in `agent/github-releaser/main.go:50` — fields use `libargument` tags like `required:"false" arg:"..." env:"..." usage:"..." default:"..."`. The exact field shape to mirror is `AnthropicModel` (line 61) — it has a default and is env-forwarded, the closest semantic match for the new `AllowMajor` field.
- `application` struct in `agent/github-releaser/cmd/run-task/main.go:34` — same `libargument` pattern; mirror the `AnthropicModel` field at line 56.
- `errors.Wrap(ctx, err, "...")` and `errors.Wrapf(ctx, err, "...", args...)` from `github.com/bborbe/errors` — used at the surrounding error sites; no new error paths are added in this prompt.
- `glog.V(2).Infof` / `glog.Warningf` are the existing observability patterns; no new logging is added in this prompt (the planning step in prompt 3 is where the override gets its `glog.V(2) "--allow-major override"` line).
</context>

<requirements>

1. **Add the `AllowMajor` field to the Kafka `application` struct.** In `/workspace/agent/github-releaser/main.go`, insert a new field on the `application` struct (line 50-86) immediately AFTER the `AnthropicModel` field at line 61 (the closest semantic match: an env-forwarded knob with a default). Use the existing `libargument` pattern verbatim:

   ```go
   // Per-run override for the spec 060 major-bump guard. When true, a
   // bump verdict of `major` proceeds to execution even when the
   // target repo's `.maintainer.yaml` does not have
   // `release.allowMajorBump: true`. Equivalent opt-in semantics;
   // either source is sufficient. Default false; the planning step
   // emits `glog.V(2) --allow-major override` so kubectl-logs greps
   // surface operator overrides.
   AllowMajor bool `required:"false" arg:"allow-major" env:"ALLOW_MAJOR" usage:"Per-run override: allow `major` bump verdict even if repo has no release.allowMajorBump opt-in" default:"false"`
   ```

   Acceptance criterion: `grep -c 'allow-major' agent/github-releaser/main.go` returns ≥ 1; `grep -c 'ALLOW_MAJOR' agent/github-releaser/main.go` returns ≥ 1. Both will hit the new field.

2. **Add the `AllowMajor` field to the local-CLI `application` struct.** In `/workspace/agent/github-releaser/cmd/run-task/main.go`, insert the same field on the `application` struct (lines 34-57) immediately AFTER the `AnthropicModel` field at line 56. Use the EXACT same `libargument` tag set as the Kafka entry — the two `application` structs are deliberately redundant and must stay in lock-step on this field. The `usage:` string is identical to the Kafka entry's.

3. **Extend the `BuildEnv` signature.** In `/workspace/agent/github-releaser/pkg/buildenv.go`, change the function signature to add a fifth parameter `allowMajor bool`:

   ```go
   // BuildEnv assembles the env map forwarded into the Claude CLI subprocess.
   // Only non-empty / set values are set so the subprocess sees a clean env.
   // Shared by both the Kafka entry point (main.go) and the local-CLI entry
   // point (cmd/run-task/main.go).
   //
   // allowMajor is the spec 060 per-run opt-in for the major-bump guard.
   // When true, the subprocess sees `ALLOW_MAJOR=true` in its env so the
   // planning step's guard (which reads the same env) can audit the
   // operator's override on the task page.
   func BuildEnv(
       ghToken, anthropicBaseURL, anthropicAuthToken, anthropicModel string,
       allowMajor bool,
   ) map[string]string {
   ```

   The function body gets ONE new branch as the LAST `if` block (preserve the existing four branches verbatim, do not reorder):

   ```go
       if allowMajor {
           env["ALLOW_MAJOR"] = "true"
       }
       return env
   ```

   Update the file's doc-comment to mention the new parameter and the spec 060 reference. The "Only non-empty values are set" comment is now slightly inaccurate (the boolean is set ONLY when true, not "non-empty"); update it to "Only set values are forwarded so the subprocess sees a clean env — non-empty strings and `allowMajor=true`."

4. **Update the Kafka `BuildEnv` call site.** In `/workspace/agent/github-releaser/main.go`, change the call at lines 125-130 to pass the new value:

   ```go
   env := pkg.BuildEnv(
       resolvedToken,
       a.AnthropicBaseURL,
       a.AnthropicAuthToken,
       a.AnthropicModel.String(),
       a.AllowMajor,
   )
   ```

   Do not add a `glog.V(2)` line here — the planning step (prompt 3) is the canonical observability surface for the override. The `BuildEnv` call is plumbing; logging the value here would duplicate the planning-step log.

5. **Update the local-CLI `BuildEnv` call site.** In `/workspace/agent/github-releaser/cmd/run-task/main.go`, change the call at lines 79-84 to pass the new value:

   ```go
   env := pkg.BuildEnv(
       resolvedToken,
       a.AnthropicBaseURL,
       a.AnthropicAuthToken,
       a.AnthropicModel.String(),
       a.AllowMajor,
   )
   ```

6. **Sibling entry-point check (Go).** Before running `make precommit`, run:

   ```
   grep -rn 'pkg.BuildEnv\|BuildEnv(' /workspace/agent/github-releaser/
   ```

   Expected hits (after this prompt's edits):
   - `agent/github-releaser/pkg/buildenv.go` — the function declaration.
   - `agent/github-releaser/main.go` — the call site at lines 125-130, now passing 5 arguments.
   - `agent/github-releaser/cmd/run-task/main.go` — the call site at lines 79-84, now passing 5 arguments.

   Three hits total. If `pkg/steps_planning.go` (or any other file) also references `BuildEnv`, the sibling entry-point check has a gap — re-scope this prompt or add the missing site. The existing test file `pkg/steps_planning_test.go` does NOT call `BuildEnv` directly (it calls `NewPlanningStep` and constructs the step manually), so no test file needs editing in this prompt.

7. **Acceptance gate — `go build ./...` exits 0 in `agent/github-releaser`.** Run `cd /workspace/agent/github-releaser && go build ./...` and confirm exit code 0. Every `BuildEnv` call site is updated; every `application` struct carries the new field with consistent tags. Investigate and fix any compile errors. Do NOT run `make precommit` in this prompt — the planning step's `NewPlanningStep` signature is not yet updated (that's prompt 3's job), so the planning-step Ginkgo tests in `pkg/steps_planning_test.go` will continue to compile against the old `NewPlanningStep(runner, fetcher, maintainerConfigFetcher)` signature. The fast `go build ./...` loop catches the production-binary regressions without dragging the test suite into the iteration.

8. **No changelog entry in this prompt.** Prompt 5 is the canonical CHANGELOG entry point. Do not add a `## Unreleased` bullet for the CLI flag alone — the spec requires a single combined entry naming both `allowMajorBump` and `--allow-major`, and splitting that into two prompts would produce two feat-bullets when the spec is one logical change.

9. **Cross-prompt dependency declaration.** This prompt depends on prompt 1 having shipped first (the `ReleaseConfig.AllowMajorBump` symbol is not yet consumed by the planning step in this prompt; the CLI flag is a parallel lever). Prompt 3 depends on this prompt having shipped (the `NewPlanningStep` signature changes to add a new `allowMajor bool` parameter that this prompt's `BuildEnv` call forwards as `env["ALLOW_MAJOR"]` — but the planning step reads the value from a constructor parameter, not from the env map, so prompt 3 may choose either: thread the value through `factory.CreateAgent` (the cleaner path) OR have the planning step read `os.Getenv("ALLOW_MAJOR")` itself. The default expectation: prompt 3 threads the value through `factory.CreateAgent` as a sixth `BuildEnv` consumer, and the planning step's constructor parameter is the second lever mirroring `cfg.Release.AllowMajorBump`).
</requirements>

<constraints>
- The `AllowMajor` field MUST follow the existing `libargument` struct-tag pattern verbatim (`required:"false" arg:"..." env:"..." usage:"..." default:"..."`). The `arg:"allow-major"` and `env:"ALLOW_MAJOR"` tag values are FIXED; the spec's AC `grep -c 'allow-major' agent/github-releaser/main.go` returns ≥ 1 and `grep -c 'ALLOW_MAJOR' agent/github-releaser/main.go` returns ≥ 1.
- The `BuildEnv` signature change is BREAKING — every call site (currently 2: the Kafka `main.go` and the local-CLI `cmd/run-task/main.go`) MUST be updated in this same prompt. Go has no default-parameter values, so leaving one call site on the old 4-arg signature would fail the build.
- The CLI flag value MUST be forwarded to the Claude subprocess env ONLY when `true`. The same gating pattern as the other optional env vars — never unconditionally set `ALLOW_MAJOR=true` (a default-false CLI flag with a "true" env would mislead the subprocess into believing every run was an override).
- The new `allowMajor bool` parameter on `BuildEnv` is a plain `bool`, NOT a `*bool`. The factory and the planning step will read it as a plain `bool` too — pointer encoding is reserved for the planning step's `PlanOutput` audit-trail fields, where `omitempty` distinguishes "resolved false" from "not resolved" in the marshaled JSON. `BuildEnv` returns a `map[string]string` where the absence of the key IS the default; no pointer needed.
- The `application` struct fields are deliberately duplicated between the Kafka entry and the local-CLI entry. Do NOT attempt to share them via an embedded sub-struct — the project convention is two flat `application` structs that mirror each other.
- Do NOT add a `--allow-major` short-form alias (e.g. `-m`) — the spec uses the long form only; do not invent a short flag.
- Do NOT add a `release.allowMajorBump` CLI override on the agent binary (e.g. `--config-allow-major`). The two levers are independent: the YAML field is the durable per-repo policy, the CLI flag is the transient per-run override. An operator who wants both can use both.
- Do NOT log the override in `BuildEnv` — the planning step (prompt 3) is the canonical observability surface.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass — the planning-step Ginkgo tests in `pkg/steps_planning_test.go` are not affected by this prompt (the `NewPlanningStep` signature is unchanged here).
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && go build ./...
```
Expected: exit code 0; both `application` structs compile with the new field; both `BuildEnv` call sites pass 5 arguments; the new function signature accepts the new parameter.

Evidence commands the auditor will run:
- `grep -c 'allow-major' /workspace/agent/github-releaser/main.go` → ≥ 1 (the `arg:"allow-major"` tag in the new field).
- `grep -c 'ALLOW_MAJOR' /workspace/agent/github-releaser/main.go` → ≥ 1 (the `env:"ALLOW_MAJOR"` tag in the new field).
- `grep -c 'allow-major' /workspace/agent/github-releaser/cmd/run-task/main.go` → ≥ 1.
- `grep -c 'ALLOW_MAJOR' /workspace/agent/github-releaser/cmd/run-task/main.go` → ≥ 1.
- `grep -c 'AllowMajor' /workspace/agent/github-releaser/pkg/buildenv.go` → ≥ 1 (the new function parameter and the `if allowMajor` branch).
- `grep -n 'func BuildEnv' /workspace/agent/github-releaser/pkg/buildenv.go` → signature shows 5 parameters: `(ghToken, anthropicBaseURL, anthropicAuthToken, anthropicModel string, allowMajor bool)`.
- `grep -rn 'pkg.BuildEnv' /workspace/agent/github-releaser/` → exactly 2 call sites (Kafka + local CLI), each passing 5 arguments.
- `git diff master -- agent/github-releaser/main.go agent/github-releaser/cmd/run-task/main.go agent/github-releaser/pkg/buildenv.go | grep -c '^+.*fmt\.Errorf'` → 0 (no `fmt.Errorf` introduced; the touched files use `github.com/bborbe/errors` only).
- `cd /workspace/agent/github-releaser && go build ./...` → exit code 0.
</verification>
