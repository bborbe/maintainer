---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-15T18:54:13Z"
generating: "2026-05-15T18:54:14Z"
prompted: "2026-05-15T19:04:22Z"
verifying: "2026-05-15T19:18:47Z"
branch: dark-factory/repoallowlist-lib-bootstrap
---

## Summary

- Bootstrap a new `lib/` Go module inside the maintainer repo as the first shared-library extraction, mirroring the pattern already in use in the agent repo.
- Inside that module, ship a single small package that knows how to evaluate a repo-allowlist entry against a target host/owner/repo string, with support for a single new wildcard syntax: `github.com/<owner>/*`.
- Library is self-contained: no caller in the maintainer repo is changed by this spec. A sibling spec will migrate the five existing inline parsers in a follow-up wave.
- The library is the precondition for adding wildcard support without parser drift across the five binaries that currently consume `REPO_ALLOWLIST`.
- Scope is intentionally narrow — pure-Go library, ginkgo-tested, no I/O, no Kafka, no deploy. Risk surface stays at Rung-1 (`make precommit` only).

## Problem

The `REPO_ALLOWLIST` environment variable is consumed by five separate binaries in the maintainer repo, and each currently has its own inline parser that performs literal string matching only. The success criteria for the broader code-reviewer goal call for a `bborbe/*` wildcard syntax so the operator does not have to enumerate every repo. Adding wildcard support without first consolidating the parsers would either require five parallel changes (review-and-verification cost) or accept permanent drift between the five copies. The maintainer repo also has a deferred design decision that a `lib/` directory should only be introduced when a second consumer requires shared types — the repo-allowlist parser is exactly that trigger, so this spec also serves as the first-time bootstrap of `lib/`.

## Goal

After this work, the maintainer repo contains a new self-contained Go module at `lib/` whose only inhabitant is a small package that exports two functions:

1. A predicate that answers "is this host/owner/repo target allowed by this allowlist?" — supporting literal entries, the new `github.com/<owner>/*` wildcard, and the existing "empty allowlist means allow all" semantics.
2. A validator that answers "is this allowlist well-formed?" so callers can fail fast at startup instead of silently skipping malformed entries at match time.

No existing binary imports or depends on this new module yet — the migration is a separate, sibling spec.

## Non-goals

- Do NOT modify any of the five binaries that currently parse `REPO_ALLOWLIST` inline. They keep their existing parsers until a follow-up spec migrates them.
- Do NOT modify `dev.env`, `prod.env`, `common.env`, or `local.env`. The env still ships literal entries.
- Do NOT update the vault runbook for repo-allowlist changes.
- Do NOT add an end-to-end smoke test. No caller imports the library yet, so e2e is impossible at this layer.
- Do NOT generate a counterfeiter mock. The exported API is a pure function; no caller has expressed a mocking need.
- Do NOT support negative/exclusion patterns (`!github.com/bborbe/foo`).
- Do NOT support prefix patterns (`github.com/bborbe/agent-*`). Only the `github.com/<owner>/*` form is in scope.
- Do NOT support non-GitHub platforms.

## Desired Behavior

1. The maintainer repo contains a new Go module rooted at `lib/` whose module path is `github.com/bborbe/maintainer/lib`, with its own `go.mod`, `go.sum`, and `Makefile`, integrated into the repo-root `make precommit` via the existing `Makefile.folder` mechanism so root-level precommit picks it up automatically.
2. The new module exposes one package that provides a predicate function answering "does this allowlist permit this host/owner/repo target?"; given an empty or nil allowlist, the predicate returns true (allow-all), preserving the existing behavior of the optional consumers.
3. The same package exposes a validator function that callers can invoke at startup to fail fast on malformed entries; this is distinct from the predicate, which silently skips and logs malformed entries during matching.
4. A literal entry such as `github.com/bborbe/maintainer` matches the exact target `github.com/bborbe/maintainer` and nothing else; case is significant — no case-insensitive matching.
5. A wildcard entry of the shape `github.com/<owner>/*` matches any target whose host and owner segments equal the entry's, regardless of the repo segment.
6. Cross-owner or otherwise malformed wildcard entries (`github.com/*`, `github.com/*/foo`, `github.com/*/*`, anything with `*` outside the repo segment) do NOT match anything and are logged as errors at predicate time; the same shapes cause the validator to return an error.
7. Entries with surrounding whitespace are trimmed before matching or validating, to be forgiving of comma-split env values; entries that are empty after trimming are skipped.
8. Ginkgo unit tests with `DescribeTable` cover every failure mode and the happy paths, achieving at least 80% coverage in the new package; `make precommit` passes inside `lib/`.

## Constraints

- Module path is `github.com/bborbe/maintainer/lib` — this is a frozen interface for the follow-up migration spec.
- Existing "empty allowlist means allow all" semantics of `agent/pr-reviewer`, `agent/pr-reviewer/cmd/run-task`, and `watcher/github-pr` must be preserved by the predicate. The "required, non-empty" enforcement currently in `watcher/github-build` and `watcher/github-build/cmd/run-once` stays at the main.go level in the follow-up migration — the library never refuses an empty list.
- Errors are constructed and wrapped exclusively with `github.com/bborbe/errors`. No `fmt.Errorf`. No standard `errors.New`.
- Logging uses `glog` (matching the rest of the maintainer codebase).
- License headers (BSD-style, matching the existing maintainer files) are present on every new `.go` file.
- A `CHANGELOG.md` entry is added under the existing `## Unreleased` heading at repo root.
- No caller code is touched. No env file is touched. No runbook is touched.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Allowlist entry has fewer than the three expected segments (e.g. `github.com/bborbe`) | Predicate logs an error and skips the entry; validator returns a wrapped error. | Operator fixes the env. |
| Wildcard appears outside the repo segment (`*/bborbe/x`, `github.com/*/x`, `github.com/*`) | Predicate logs an error and skips the entry — does NOT silently fall back to allow-all; validator returns a wrapped error. | Operator fixes the env. |
| Multiple wildcards in one entry (`github.com/*/*`) | Predicate logs an error and skips; validator returns a wrapped error. | Operator fixes the env. |
| Empty allowlist `[]string{}` | Predicate returns true (allow-all). Validator returns nil. | None — intentional. |
| Nil allowlist | Predicate returns true (allow-all), same as empty. Validator returns nil. | None — intentional. |
| Whitespace surrounding an entry (`" github.com/bborbe/maintainer "`) | Trimmed before matching/validating. | None — intentional, forgiving of env splits. |
| Empty string entry, or entry that is whitespace-only | Skipped silently (treated as not present). | None. |
| Target string passed to predicate is empty | Predicate returns false unless allowlist is empty/nil (in which case allow-all wins). | Caller validates upstream. |

## Do-Nothing Option

If we don't do this, adding wildcard support to `REPO_ALLOWLIST` requires either five parallel inline-parser changes — high review cost, drift risk, regression surface across two binary families (`agent/pr-reviewer*` and `watcher/github-*`) — or accepting that wildcards only land in a subset of consumers, which would silently break the operator's mental model (one env var, one behavior). Neither alternative is acceptable for a config that gates which repos the agent will act on. Doing nothing also indefinitely defers the maintainer-repo `lib/` bootstrap that the deferred design decision committed to.

## Acceptance Criteria

- [ ] A new directory `lib/` exists at the maintainer repo root, containing its own `go.mod` with module path `github.com/bborbe/maintainer/lib`.
- [ ] The new module is wired into root-level `make precommit` via `Makefile.folder` so a single root invocation exercises it.
- [ ] The module contains exactly one package whose responsibility is repo-allowlist evaluation.
- [ ] The package exports a predicate function that, given an allowlist and a host/owner/repo target string, returns a boolean answer.
- [ ] The package exports a validator function that, given an allowlist, returns an aggregate error (via `errors.Join` or equivalent) describing **every** malformed entry it found, or nil if all entries are well-formed. Aggregation lets the operator see all mistakes in one iteration rather than fixing-and-retrying per entry.
- [ ] Empty allowlist and nil allowlist both cause the predicate to return true.
- [ ] Literal entries match exactly; case is significant.
- [ ] Wildcard entries of shape `github.com/<owner>/*` match any repo under that owner.
- [ ] Wildcards outside the repo segment are rejected: the predicate logs an error and does not match; the validator returns an error.
- [ ] Surrounding whitespace on entries is trimmed; empty-after-trim entries are skipped.
- [ ] Ginkgo `DescribeTable` covers: literal match, literal no-match, wildcard match, wildcard no-match (different owner), cross-owner wildcard rejected, multi-wildcard rejected, fewer-than-three-segments rejected, empty allowlist allow-all, nil allowlist allow-all, whitespace trimmed, case-sensitivity preserved.
- [ ] Package coverage is at least 80%.
- [ ] All `.go` files carry the BSD-style license header used elsewhere in the maintainer repo.
- [ ] All errors use `github.com/bborbe/errors`; no `fmt.Errorf` or stdlib `errors.New`.
- [ ] `CHANGELOG.md` has an entry under `## Unreleased` describing the bootstrap of `lib/` and the new package.
- [ ] `make precommit` passes inside `lib/`.
- [ ] No caller binary (the five existing `REPO_ALLOWLIST` consumers) is modified by this spec.
- [ ] No `*.env` file is modified by this spec.

## Verification

```
cd lib && make precommit
```

Rung-1 only. No deploy, no integration, no scenario.
