# Watcher Decision Chains

The `watcher/github` service makes two distinct decisions per PR. They live in
separate, composable chains so future filter additions land in the right place.

## Chain 1: TaskCreationFilter

**Question:** "Should we create a vault task for this PR at all?"

**Behavior on skip:** PR is silently ignored — no vault task, no audit trail.
Use this chain for PRs that are pure noise: drafts, bots, abandoned work,
work-in-progress that the author has explicitly marked as not ready.

**Pattern:** Filter (predicate, slice composite, "skip if any votes skip").
See `go-filter-pattern.md` and `go-boolean-combinator-pattern.md`.

**Package:** `watcher/github/pkg/filter/`

**Current leaves:**

| Leaf | Skips when |
|------|-----------|
| `DraftFilter` | `pr.IsDraft == true` |
| `BotAuthorFilter` | `pr.AuthorLogin` matches the configured bot allowlist |
| `RepoAllowlistFilter` | `pr.RepoKey` (`host/owner/repo`) is not on the configured `REPO_ALLOWLIST`; empty allowlist = allow-all |

**Future leaves (planned):**

- `WIPTitleFilter` — title starts with `WIP:` or `WIP ` (work-in-progress signal)
- `AgeFilter` — PR last updated more than `MAX_PR_AGE` ago (abandoned)
- `ArchivedRepoFilter` — repo archived (no reviewer cares)

## Chain 2: TrustGate

**Question:** "Given we create a task, should it auto-process or route to
human_review?"

**Behavior on untrusted:** Task is created with `phase: human_review` and
`status: todo`, body explains why. Reviewer can promote to `phase: in_progress`
if they decide to proceed.

**Pattern:** Boolean combinator (`And` / `Or` / `Not` over `Trust` interface).
See `go-boolean-combinator-pattern.md`.

**Package:** `watcher/github/pkg/trust/`

**Current leaves:**

| Leaf | Trusted when |
|------|-------------|
| `AuthorAllowlist` | `pr.AuthorLogin` matches the configured trusted-authors list (exact byte match) |

**Future leaves (planned):**

- `IsCollaborator` — author is a repo collaborator (queries GitHub API)
- `RequiredLabel` — PR has a specific opt-in label (`ok-to-review`)

## Decision: which chain does my new filter go in?

Ask: "Do I want this PR to be visible to a human reviewer at all?"

- **No, it's pure noise** → TaskCreationFilter (skip)
- **Yes, but I want a human to confirm before auto-processing** → TrustGate

Two examples that have come up:

| Filter | Chain | Why |
|--------|-------|-----|
| "WIP" in title | TaskCreationFilter | Author explicitly says "not ready" — same intent as draft |
| "PR > 90 days old" | TaskCreationFilter | Almost certainly abandoned; reviewer attention not warranted |
| "Author not in collaborator list" | TrustGate | Visible task with reviewer override; trust is the human's call |
| "Repo not in allowlist" | TaskCreationFilter (shipped via `RepoAllowlistFilter`) | Out-of-scope repos are pure noise per stage — hard skip, not a visible task |

The two chains are independent. A PR that survives TaskCreationFilter then
runs through TrustGate; both must agree before the task is auto-processed.
