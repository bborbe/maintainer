---
status: completed
summary: 'Made -race flag opt-in via RACE Makefile variable (default true), set RACE=false in CI workflow, and added CHANGELOG entry under ## Unreleased.'
container: maintainer-105-make-race-optional-disable-in-ci
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-08T14:49:18Z"
queued: "2026-05-08T14:49:18Z"
started: "2026-05-08T14:49:20Z"
completed: "2026-05-08T14:51:41Z"
---

<summary>
- Make `-race` flag in `make test` opt-in via a `RACE` environment variable, defaulting to `true` so local dev behaviour is unchanged.
- Update `.github/workflows/ci.yml` to set `RACE=false` for the precommit step in CI — the GitHub Actions runner has been segfaulting under `-race` (see https://github.com/bborbe/maintainer/actions/runs/25558544578: `signal: segmentation fault (core dumped)` in `agent/pr-reviewer` test). The same test passes locally with `-race` and same Go version (1.26.3), so the segfault is environmental (likely race-detector + cgo + linux/amd64 runner quirk or OOM under -race's ~5x memory cost).
- Two-file change: `Makefile.precommit` (one line edited) + `.github/workflows/ci.yml` (one line edited).
- Add a CHANGELOG entry under the next unreleased version.
- `make precommit` MUST stay clean locally with the new conditional — both `RACE=true` (default) and `RACE=false` paths exercised at least manually.
</summary>

<objective>
Sidestep the linux/amd64 CI runner segfault under `-race` without losing race-detector coverage on developer machines. Keep `-race` as the default for local `make test` / `make precommit` runs (where it works fine), and explicitly opt out in CI via `RACE=false`. The CI signal then becomes "code compiles, tests pass, lint clean, security clean" — race-condition detection moves to local dev + future scheduled runs.

This is a deliberate tradeoff: race detection is most valuable where it's reliable. Forcing it on a runner where it segfaults is anti-signal — every commit fails CI for environment reasons, masking any real test failure underneath. Better to disable it in the unstable place and keep it where it works.
</objective>

<context>

## Files to edit

### `Makefile.precommit` — line 29

**Current:**

```
.PHONY: test
test:
	go test -mod=mod -p=$${GO_TEST_PARALLEL:-1} -cover -race $(shell go list -mod=mod ./... | grep -v /vendor/)
```

**After:**

```
RACE ?= true

.PHONY: test
test:
	go test -mod=mod -p=$${GO_TEST_PARALLEL:-1} -cover $(if $(filter true,$(RACE)),-race) $(shell go list -mod=mod ./... | grep -v /vendor/)
```

Notes:
- `RACE ?= true` placed near the top of `Makefile.precommit` (alongside the existing `export ROOTDIR ?= ...` block) so it documents the variable visibly.
- `$(if $(filter true,$(RACE)),-race)` returns `-race` when `RACE=true` (default) and empty otherwise. Strict equality — any value other than the literal `true` is treated as off, including `RACE=1` or `RACE=yes`. This keeps the on/off contract narrow and predictable.
- Default behaviour for any caller that doesn't set `RACE` (humans running `make test` locally, other Makefile targets, future CI jobs) is unchanged: `-race` is on.

### `.github/workflows/ci.yml` — "Run precommit checks" step (around line 32)

**Current:**

```yaml
      - name: Run precommit checks
        run: make precommit
```

**After:**

```yaml
      - name: Run precommit checks
        env:
          RACE: "false"
        run: make precommit
```

Notes:
- Use the `env:` block on the step so the variable is scoped to this single command — no cross-contamination into other jobs / future steps.
- Quoted `"false"` for explicitness; YAML would also accept unquoted `false`, but quoted-string makes the make-side comparison `$(filter true,$(RACE))` unambiguous (it compares against the literal token `true`, never the YAML boolean).

### CHANGELOG.md — new unreleased-version entry

Top of file currently has `v0.23.32` as the latest entry. Create a NEW `v0.23.33` section ABOVE it with this entry:

```
- chore(test): make `-race` flag opt-in via `RACE` Makefile variable (default `true` preserves local behaviour). CI sets `RACE=false` to sidestep ubuntu-latest+go1.26.3 segfault under `-race` in `agent/pr-reviewer` (run 25558544578). Race detection still on for local dev + can be re-enabled in CI by removing the env block when the runner issue is resolved.
```

</context>

<constraints>

- `make precommit` MUST exit 0 locally with no env vars set — `-race` stays the default.
- `make precommit` MUST exit 0 with `RACE=false make precommit` — confirms the conditional works.
- `RACE=true make precommit` MUST be byte-identical in invoked commands to plain `make precommit` (default-true means setting it explicitly changes nothing).
- No other Makefile targets touched; no other CI steps touched.
- Errors must be wrapped with `github.com/bborbe/errors` (no errors introduced here).
- All existing tests pass — this is a build/CI tweak, not a test change.

</constraints>

<failure_modes>

| Trigger | Expected behaviour | Recovery |
|---|---|---|
| `RACE=false make test` actually still passes `-race` | Make conditional was misused; investigate `$(filter ...)` behaviour | Inspect with `make -n test RACE=false` to see expanded command |
| Local `make precommit` (no env set) drops `-race` | Default `?=` lost; investigate `RACE ?= true` placement | Ensure `RACE ?= true` is at file scope, not inside a recipe |
| CI step continues to segfault even with `RACE=false` | Segfault unrelated to race detector | Investigate further (runner OS upgrade, Go version, OOM) — this prompt's scope ends here |
| Other Makefile targets that called `-race` directly | Search for hardcoded `-race` references | None expected — only `Makefile.precommit:29` has `-race`; verify with `grep -rn ' -race ' Makefile* .github/` |

</failure_modes>

<acceptance_criteria>

- [ ] `Makefile.precommit` defines `RACE ?= true` at file scope.
- [ ] `Makefile.precommit` `test` target uses `$(if $(filter true,$(RACE)),-race)` (or equivalent conditional that yields the same expansion).
- [ ] `make -n test` with no env set expands to a command containing ` -race `.
- [ ] `RACE=false make -n test` expands to a command **not** containing ` -race `.
- [ ] `RACE=true make -n test` is byte-identical to plain `make -n test`.
- [ ] `.github/workflows/ci.yml` "Run precommit checks" step has `env: RACE: "false"`.
- [ ] CHANGELOG has a new unreleased-version entry describing this change.
- [ ] `make precommit` exits 0 locally with no env set.
- [ ] `RACE=false make precommit` exits 0 locally.
- [ ] No other files touched (verify with `git diff --name-only` showing exactly `Makefile.precommit`, `.github/workflows/ci.yml`, `CHANGELOG.md`).

</acceptance_criteria>

<verification>

```bash
# Default-on path
make -n test 2>&1 | grep ' -race '       # expect match
make precommit                              # expect exit 0

# Opt-out path (CI shape)
RACE=false make -n test 2>&1 | grep ' -race '   # expect no match (exit 1 from grep is success here)
RACE=false make precommit                        # expect exit 0

# Files changed
git diff --name-only                              # expect exactly Makefile.precommit, .github/workflows/ci.yml, CHANGELOG.md
```

Expected:
- Default `make` runs with `-race` (unchanged from today)
- `RACE=false make` drops `-race`
- Both still pass precommit
- No collateral file changes

</verification>

<do_nothing_option>

Leaving CI on `-race` keeps the segfault in place: every push gets a red CI status for an environmental reason, which makes real test failures invisible (the bot sends the same alert regardless). Doing nothing means accepting that the maintainer repo's CI signal is currently meaningless until either (a) the runner issue resolves itself, (b) Go's race detector + linux/amd64 cgo combination stops segfaulting, or (c) someone investigates the root cause.

This change is the smallest possible move that restores CI signal without losing race detection where it's actually useful (local dev). Reverting is one line if the runner issue gets fixed upstream.

</do_nothing_option>
