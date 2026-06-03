---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-06-03T17:34:33Z"
generating: "2026-06-03T17:34:48Z"
prompted: "2026-06-03T17:50:47Z"
branch: dark-factory/extend-repoallowlist-exclusion-syntax
---

## Summary

- Extend the shared `lib/repoallowlist` parser to recognize a `!`-prefix on entries as **exclusion** semantics, so an operator can express "every bborbe repo except go-skeleton" as `github.com/bborbe/*,!github.com/bborbe/go-skeleton` in a single `REPO_ALLOWLIST` value.
- Matching becomes set-theoretic: a target is allowed iff `(includes empty OR any include matches) AND (no exclude matches)`. Excludes always override includes.
- Exclude-only lists (no include entries) treat every target as included by default, then apply excludes — the canonical "allow-all-except" case the operator most often wants.
- All five existing callsites (`watcher/github-release`, `watcher/github-pr`, `watcher/github-build`, `agent/pr-reviewer`, `agent/github-releaser`) delegate to `lib/repoallowlist.IsAllowed`, so no service code changes — verified by a grep proof in this spec's verification.
- Unblocks a single-line operator fix in `prod.env` to stop the dev/prod release pipeline collision on `go-skeleton` (out of scope here; the spec ships only the library change + docs + tests).

## Problem

Today `lib/repoallowlist` is **allowlist-only**. The only way to express "all bborbe repos except go-skeleton" is to enumerate every bborbe repo individually — a maintenance trap as new bborbe repos are added. The concrete pain: two release pipelines (dev with `github.com/bborbe/go-skeleton`, prod with `github.com/bborbe/*`) both match go-skeleton, both watchers write the same task filename in the shared OpenClaw vault, git-rest's YAML merge resolver quarantines the file, no Job spawns, and releases silently fail to ship. Evidence on 2026-06-03: 5 SHAs created → quarantined → only 1 (e19e5fb at 07:39) actually tagged v0.4.3 through post-quarantine re-create luck. A `!`-prefix exclusion syntax solves the duplication at the source (one of the two pipelines stops matching) while keeping the configuration in a single env var per stage.

## Goal

After this work, `lib/repoallowlist` accepts entries of the form `!host/owner/repo` and `!host/owner/*` as **exclusions** with the following lock-in semantics:

- A target is allowed iff `(includes is empty OR any include matches the target) AND (no exclude matches the target)`.
- Excludes always override includes — if both match, the target is rejected.
- An exclude-only allowlist (e.g. `!github.com/bborbe/go-skeleton`) means **allow everything except the excluded entries**, not "deny-all because no include matches".
- Order independence: `bborbe/*,!bborbe/go-skeleton` and `!bborbe/go-skeleton,bborbe/*` produce identical decisions for every target.

The five existing consumer services pick up the new semantics with zero code changes, because they all already call `repoallowlist.IsAllowed`.

## Non-goals

- Do NOT edit `prod.env` to add `!github.com/bborbe/go-skeleton` — operator action, separate PR.
- Do NOT deploy any consumer service — `make buca` to dev/prod is operator follow-up.
- Do NOT modify any of the five consumer services' code (`watcher/github-release`, `watcher/github-pr`, `watcher/github-build`, `agent/pr-reviewer`, `agent/github-releaser`). The library change is sufficient; verified by grep proof.
- Do NOT introduce a parallel `REPO_BLOCKLIST` env var — one knob (the `!`-prefix in `REPO_ALLOWLIST`) is the design, not two.
- Do NOT add `TASK_SUFFIX` to the release watcher — it would only mask the symptom (task filename) and expose a tag-race downstream.
- Do NOT recover today's quarantined SHAs — operational follow-up.
- Do NOT change the wildcard syntax (still `host/owner/*`; wildcards in host or owner remain malformed for both includes and excludes — invariant; if a future consumer demands wildcard owners, that's a separate spec).
- Do NOT add a config knob to disable exclusion parsing — `!`-prefix is the only new syntax and it's always on (an escape hatch on the Goal is itself a regression).

## Desired Behavior

1. The parser recognizes a leading `!` (immediately, no whitespace between `!` and the entry body) as marking the entry as an **exclude**; all other entries remain **includes**. Whitespace at the start of the original entry is trimmed before the `!` check.
2. `IsAllowed` returns `true` for a target iff `(includes is empty OR any include matches target) AND (no exclude matches target)`.
3. An allowlist consisting solely of exclude entries is treated as "includes is empty" for inclusion purposes, so every target passes the include gate and only the exclude gate filters — i.e. "allow everything except the excluded entries".
4. Exclude entries support both literal (`!host/owner/repo`) and wildcard (`!host/owner/*`) shapes, with identical matching rules to their include counterparts.
5. Malformed exclude entries (e.g. `!host/owner`, `!*/owner/repo`, `!host/*/repo`) are skipped at `IsAllowed` time with a WARN log identical in style to malformed includes; `Validate` returns them as aggregated errors.
6. `Validate` accepts `!`-prefix entries; well-formedness checks run on the post-`!` portion of the entry.
7. Existing all-include allowlists produce identical decisions before and after this change (backwards compatible).

## Constraints

- Public API of `lib/repoallowlist` (`IsAllowed(allowlist []string, target string) bool`, `Validate(ctx context.Context, allowlist []string) error`) remains frozen — signatures, return semantics, allow-all-on-empty behavior, empty-target-on-non-empty-list behavior unchanged.
- `IsAllowed` keeps logging malformed entries via `glog.Errorf` and skipping them; it does not return an error for malformed entries (matches existing behavior).
- `Validate` keeps returning aggregated errors via `errors.Join` from `github.com/bborbe/errors`.
- Error wrapping uses `bborbe/errors` (no bare `return err`, no `fmt.Errorf` for errors).
- Logging uses `glog`; any new Info-level logs gate behind `glog.V(n)`.
- No new dependencies on packages outside the current import set of `repoallowlist.go`.
- Existing test cases in `lib/repoallowlist/repoallowlist_test.go` continue to pass without modification.
- The five consumer callsites (`watcher/github-release`, `watcher/github-pr`, `watcher/github-build`, `agent/pr-reviewer`, `agent/github-releaser`) remain unchanged — any diff under those paths in this PR is a spec violation.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---------|-----------|-------------------|----------|
| Malformed exclude entry shape (e.g. `!host/owner`, `!host/*/repo`, `!*/owner/repo`) | `glog.Errorf` log line at `IsAllowed` call time; `Validate` returns aggregated error | Entry skipped at `IsAllowed`; `Validate` returns error naming the entry and reason | Operator fixes `REPO_ALLOWLIST` value; pod restart picks up corrected config |
| `!` with empty body (`!` or `!  `) | Same as above — classified as malformed | Skipped with WARN log | Operator fixes env value |
| `!!host/owner/repo` (double-bang) | Same — body after first `!` is `!host/owner/repo` which fails well-formedness on the second `!` segment | Skipped with WARN log | Operator fixes env value |
| Operator intent ambiguity: exclude-only list | N/A (not a failure — locked semantic) | Treated as allow-all-except; documented in package doc | N/A |
| Include and exclude both match a target | N/A (not a failure — locked semantic) | Exclude wins; target rejected | N/A |
| Whitespace-only entry (`"  "`, `"   "`) | None — silently skipped (existing behavior preserved) | Skipped, no log | N/A |
| `!` followed by whitespace then body (`"! host/owner/repo"`) | Classified as malformed (body starts with whitespace which is part of the post-`!` segments) | Skipped with WARN log | Operator removes whitespace |

## Security / Abuse Cases

Not applicable in the traditional sense — `REPO_ALLOWLIST` is operator-controlled, not user-controlled, and the library does not touch network or filesystem. One config-level concern worth noting:

- **Misconfiguration risk**: if an operator writes `!github.com/bborbe/*` intending "exclude all bborbe" but the allowlist is otherwise empty, the result is "allow nothing matching bborbe/*, allow everything else" — which may not be what they meant. Mitigation: documentation example in the package doc spelling out the exclude-only allow-all-except semantic, and the same example in each consumer README's `REPO_ALLOWLIST` table. Not a code constraint.

## Acceptance Criteria

- [ ] `IsAllowed([]string{"github.com/bborbe/*", "!github.com/bborbe/go-skeleton"}, "github.com/bborbe/go-skeleton")` returns `false` — evidence: `go test ./lib/repoallowlist/...` passes with a Ginkgo `Entry` covering this exact case.
- [ ] `IsAllowed([]string{"github.com/bborbe/*", "!github.com/bborbe/go-skeleton"}, "github.com/bborbe/maintainer")` returns `true` — evidence: Ginkgo `Entry` in the test table.
- [ ] `IsAllowed([]string{"!github.com/bborbe/go-skeleton"}, "github.com/bborbe/maintainer")` returns `true` (exclude-only list is allow-all-except) — evidence: Ginkgo `Entry`.
- [ ] `IsAllowed([]string{"!github.com/bborbe/go-skeleton"}, "github.com/bborbe/go-skeleton")` returns `false` — evidence: Ginkgo `Entry`.
- [ ] `IsAllowed([]string{"!github.com/bborbe/*"}, "github.com/bborbe/anything")` returns `false` (wildcard exclude) — evidence: Ginkgo `Entry`.
- [ ] `IsAllowed([]string{"!github.com/bborbe/*"}, "github.com/other/anything")` returns `true` (wildcard exclude does not over-reach) — evidence: Ginkgo `Entry`.
- [ ] Order independence: for the inputs `["github.com/bborbe/*", "!github.com/bborbe/go-skeleton"]` and `["!github.com/bborbe/go-skeleton", "github.com/bborbe/*"]`, `IsAllowed` returns the same decision for every target in `{github.com/bborbe/go-skeleton, github.com/bborbe/maintainer, github.com/other/repo}` — evidence: at least three Ginkgo `Entry` rows or a dedicated `It` block asserting equality across both orderings.
- [ ] Malformed exclude entry (e.g. `!github.com/bborbe`, `!github.com/*/repo`, `!*/bborbe/repo`) is skipped at `IsAllowed` call time with a `glog.Errorf` log naming the entry and reason — evidence: existing skip-on-malformed behavior is preserved by a Ginkgo `Entry` whose target verifies the function still returns a correct decision based on the remaining well-formed entries.
- [ ] `Validate(ctx, []string{"!github.com/bborbe/go-skeleton"})` returns `nil` — evidence: Ginkgo `Entry` in the `Validate` test table.
- [ ] `Validate(ctx, []string{"!github.com/bborbe"})` returns a non-nil error whose message contains the offending entry and the reason "must have exactly 3 path segments" — evidence: Ginkgo `Entry` asserting `err.Error()` substring.
- [ ] `Validate(ctx, []string{"!"})` returns a non-nil error — evidence: Ginkgo `Entry`.
- [ ] Existing include-only behavior unchanged: every existing `Entry` in `repoallowlist_test.go` passes without modification — evidence: `git diff lib/repoallowlist/repoallowlist_test.go` shows only additions, no deletions or modifications of existing `Entry` rows.
- [ ] Package documentation in `repoallowlist.go` (`IsAllowed` doc comment) documents the `!`-prefix syntax, the set-theoretic matching rule, the exclude-overrides-include rule, and the allow-all-except semantic for exclude-only lists — evidence: `grep -n '!' lib/repoallowlist/repoallowlist.go` shows the relevant doc lines.
- [ ] The three consumer READMEs that document `REPO_ALLOWLIST` today — `watcher/github-release/README.md`, `watcher/github-pr/README.md`, `watcher/github-build/README.md` — each include a short syntax table covering literal include, wildcard include, literal exclude, wildcard exclude, and the allow-all-except example. (Verified at spec-time: `grep -ln 'REPO_ALLOWLIST' watcher/*/README.md agent/*/README.md` returns exactly these three files.) Evidence: for each of the three files, `grep -c '!github.com' <file>` returns ≥1.
- [ ] `CHANGELOG.md` has an `## Unreleased` entry under a `lib/repoallowlist` heading describing the new `!`-prefix exclude syntax — evidence: `grep -n 'repoallowlist' CHANGELOG.md` returns a line under `## Unreleased`.
- [ ] Zero diff in consumer service code: `git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go` returns empty — evidence: exit code of that pipeline is non-zero (no matches), or stdout is empty.
- [ ] `cd lib/repoallowlist && make precommit` exits 0 — evidence: exit code.
- [ ] `go vet ./lib/repoallowlist/...` exits 0 — evidence: exit code (covered by precommit but called out for explicitness).

## Verification

```bash
cd lib/repoallowlist
make precommit
```

Plus the grep proof that consumer services need zero changes:

```bash
# From repo root — enumerates current callsites; spec asserts each is satisfied by the lib upgrade alone.
grep -rn 'repoallowlist\.\(IsAllowed\|Validate\|ParseRepoAllowlist\)' lib/ watcher/ agent/ | grep -v _test.go
```

Expected: every match is a delegation to the shared library; no consumer parses entries itself.

And the no-diff proof for consumer code:

```bash
git diff --name-only origin/master...HEAD -- watcher/ agent/ | grep -v README.md | grep -v _test.go
```

Expected: empty output.


## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Library: parser + matching rule + Validate + package docs | 1, 2, 3, 4, 5, 6, 7 | 1-12, 16, 17 | — |
| 2 | Docs ripple: consumer READMEs + CHANGELOG entry + grep proof of zero consumer code change | — | 13, 14, 15 | prompt 1 (so README examples reflect shipped semantics) |

Rationale: the library change (prompt 1) is self-contained and independently verifiable via Ginkgo. The doc ripple (prompt 2) depends on the shipped semantics to keep README examples honest, and isolates the cross-tree edits so reviewers can scan them quickly. The grep proof of zero consumer-service code change belongs with the docs prompt because it confirms the no-edit contract after both prompts complete.

## Do-Nothing Option

The two-pipeline collision on `go-skeleton` is real and recurring (5 quarantined SHAs today, only 1 recovered by chance). The do-nothing fallback is to enumerate every bborbe repo individually in `prod.env`, omitting `go-skeleton`. That works once but turns every new bborbe repo into a coupled prod.env edit, and operators will forget. Cost of doing nothing: ongoing release-pipeline reliability degradation and silent task-quarantine incidents. The `!`-prefix change is small, well-scoped to a single library, backwards compatible, and ships independently of any deployment.
