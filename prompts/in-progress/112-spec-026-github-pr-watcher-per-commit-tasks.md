---
status: committing
spec: [026-github-pr-watcher-per-commit-tasks]
summary: 'Switched github-pr watcher from per-PR to per-(PR, SHA) spawn model: DeriveTaskID now encodes full SHA in UUID5 key, computePRTitle adds sha[:8] segment, publishForcePush and updateFrontmatterSender removed entirely, factory simplified to single CreateKafkaSender, all tests rewritten to cover new behavior, CHANGELOG updated.'
container: maintainer-112-spec-026-github-pr-watcher-per-commit-tasks
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-15T12:00:00Z"
queued: "2026-05-15T13:39:49Z"
started: "2026-05-15T13:40:30Z"
branch: dark-factory/github-pr-watcher-per-commit-tasks
---

<summary>

- `DeriveTaskID` gains a `sha` parameter; its UUID5 key changes from `owner/repo#number` to `owner/repo#number@sha` (full SHA) — two calls with identical `(owner, repo, number, sha)` still produce the same UUID
- `computePRTitle` gains a `sha` parameter; the title now includes a `sha[:8]` segment between the PR number and the slug: `PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]} - {slug}`
- Each push to a watched PR produces a brand-new task file identified by `(PR, SHA)` — the old task file is never opened or mutated by the watcher again
- Same-SHA polling is a no-op (watcher sees the SHA-based task ID in the cursor and skips)
- The `publishForcePush` function and the `## Outdated by force-push` heading are removed entirely from production code
- `updateFrontmatterSender` is removed from the watcher struct, constructor, and factory (the force-push path was its only consumer)
- Fail-closed on empty head SHA: if `PRDetails.HeadSHA` is empty the watcher skips that PR on this poll
- Watcher tests rewritten: "force-push mutates old task" → "new SHA spawns a new CreateTaskCommand"; force-push-specific test assertions replaced with per-SHA spawn assertions
- CHANGELOG `## Unreleased` entry added

</summary>

<objective>

Switch the github-pr watcher's spawn model from per-PR to per-(PR, SHA). Every push produces a new immutable task file; the old file is never touched again. The `publishForcePush` mutation path — the source of planning-short-circuit failures (stale headings, clobbered agent state) — is removed by construction.

</objective>

<context>

Read `CLAUDE.md` at the repo root for project conventions.

Read these guides before making any changes:
- `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern
- `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, DescribeTable, coverage ≥80%
- `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`
- `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `Create*` prefix, zero-logic factories
- `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` — which test types to write for each code change
- `docs/architecture.md` in `/workspace/` — where this watcher sits in the pipeline
- `docs/watcher-decision-chains.md` in `/workspace/` — current watcher decision flow (documents the path being changed)

**Files to read fully before making any changes:**

- `watcher/github-pr/pkg/taskid.go` — `DeriveTaskID` signature and namespace constant
- `watcher/github-pr/pkg/taskid_test.go` — existing determinism, collision, and pinned-UUID tests
- `watcher/github-pr/pkg/filename.go` — `computePRTitle` and `slugifyTitle`
- `watcher/github-pr/pkg/filename_internal_test.go` — all `computePRTitle` + `slugifyTitle` + wire-format test cases
- `watcher/github-pr/pkg/watcher.go` — **read in full**: `processPRs`, `handlePR`, `publishCreate`, `publishForcePush`, `fetchPRDetails`, `buildFrontmatter`, `buildHumanReviewFrontmatter`, `buildUntrustedBody`
- `watcher/github-pr/pkg/watcher_test.go` — **read in full**: every `It` block — you will rewrite several
- `watcher/github-pr/pkg/factory/factory.go` — `CreateKafkaSenders` and `CreateWatcher` (currently wires `updateFrontmatterSender`)
- `CHANGELOG.md` at repo root — confirm no existing `## Unreleased` section before adding one

</context>

<requirements>

Execute steps in order. Step 5 IS the `make test` checkpoint after the production-code edits (steps 1-4); fix any compile errors there before proceeding to the test-file rewrites (steps 6-8). Run `make precommit` only at the final step.

---

## Step 1 — Update `watcher/github-pr/pkg/taskid.go`

Read the file fully. Then make two precise edits:

**1a. Add `sha string` parameter to `DeriveTaskID`.**

Change the signature and key construction:

```go
// DeriveTaskID returns a deterministic task identifier for a (PR, SHA) pair.
// Input: "<owner>/<repo>#<number>@<sha>", e.g. "bborbe/maintainer#42@abc123...".
// The full SHA is used (not truncated) to keep the dedup keyspace collision-free.
func DeriveTaskID(owner, repo string, number int, sha string) uuid.UUID {
	key := fmt.Sprintf("%s/%s#%d@%s", owner, repo, number, sha)
	return uuid.NewSHA1(prWatcherNamespace, []byte(key))
}
```

The `prWatcherNamespace` constant is unchanged. The UUID5 algorithm is unchanged. Only the key string shape changes.

---

## Step 2 — Update `watcher/github-pr/pkg/filename.go`

Read the file fully. Then make one edit:

**2a. Add `sha string` parameter to `computePRTitle` and insert the `sha[:8]` segment.**

The new grammar: `PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]} - {slug}` (with slug) or `PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]}` (no slug).

```go
// computePRTitle returns the human-readable title for a PR-review task.
// Format (with slug): "PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]} - {slug}"
// Format (empty slug): "PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]}"
// The returned string MUST NOT include the .md extension; the controller appends it.
func computePRTitle(provider, owner, repo string, number int, sha, title string) string {
	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}
	base := fmt.Sprintf("PR Review %s - %s-%s - %d - %s", provider, owner, repo, number, shortSHA)
	slug := slugifyTitle(title)
	var t string
	if slug == "" {
		t = base
	} else {
		t = base + " - " + slug
	}
	if len(t) > maxTitleLen {
		glog.Warningf(
			"PR title exceeds max length: len=%d max=%d — truncating",
			len(t),
			maxTitleLen,
		)
		t = t[:maxTitleLen]
	}
	return t
}
```

`slugifyTitle` is unchanged. `maxTitleLen` and `maxSlugLen` are unchanged.

---

## Step 3 — Rewrite `watcher/github-pr/pkg/watcher.go`

Read the file fully before editing. This step has the most changes.

**3a. Remove `updateFrontmatterSender` from the struct and constructor.**

```go
type watcher struct {
	ghClient           GitHubClient
	createSender       task.CreateCommandSender
	cursorPath         string
	startTime          libtime.DateTime
	scope              string
	taskCreationFilter filter.TaskCreationFilter
	stage              string
	metrics            Metrics
	trustDecision      trust.Trust
}
```

```go
func NewWatcher(
	ghClient GitHubClient,
	createSender task.CreateCommandSender,
	cursorPath string,
	startTime libtime.DateTime,
	scope string,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	metrics Metrics,
	trustDecision trust.Trust,
) Watcher {
	return &watcher{
		ghClient:           ghClient,
		createSender:       createSender,
		cursorPath:         cursorPath,
		startTime:          startTime,
		scope:              scope,
		taskCreationFilter: taskCreationFilter,
		stage:              stage,
		metrics:            metrics,
		trustDecision:      trustDecision,
	}
}
```

**3b. Rewrite `processPRs`.**

The key change: task ID is now derived from `(owner, repo, number, details.HeadSHA)` — not from `(owner, repo, number)` alone. Filter check still happens before the details fetch (optimization). Fail-closed on empty SHA.

Replace the entire `processPRs` function with:

```go
// processPRs iterates over fetched PRs, publishes commands, and returns the max updated-at seen.
// It rebuilds HeadSHAs from only the current open-PR batch, pruning closed/merged PRs.
// Each (PR, SHA) pair produces at most one CreateTaskCommand across all poll cycles.
//
// Design note on cursor preservation for filter-skipped and details-fetch-error PRs:
//
// CRITICAL ASSUMPTION: the controller deduplicates incoming CreateTaskCommands by their
// task_identifier (UUID5). If the controller does NOT dedup, every transient filter toggle
// or transient GetPRDetails failure will produce a duplicate vault file on the next poll.
// VERIFY this assumption against the controller code before merging — search for the
// command consumer's idempotency check; if absent, this design must change to preserve
// per-PR cursor entries (which would require extending the cursor schema with the
// (owner, repo, number) tuple, since UUID5 is not reversible).
//
// Given the assumption holds, we accept that transient filter or fetch failures will cause
// the watcher to re-publish a CreateTaskCommand for the same (PR, SHA) on the next successful
// poll, and rely on controller dedup to make this a no-op. This matches the recovery path
// already documented in the spec failure-mode row "Watcher restart with empty cursor sees a
// PR whose head SHA already has a vault file" — same mechanism, slightly different trigger.
func (w *watcher) processPRs(
	ctx context.Context,
	cursorState *Cursor,
	allPRs []PullRequest,
) libtime.DateTime {
	since := cursorState.LastUpdatedAt
	maxUpdatedAt := since
	prDetailsCache := make(map[string]PRDetails)
	newHeadSHAs := make(map[string]string, len(allPRs))

	for _, pr := range allPRs {
		if w.taskCreationFilter.Skip(
			filter.PR{
				AuthorLogin: pr.AuthorLogin,
				IsDraft:     pr.IsDraft,
				Title:       pr.Title,
				UpdatedAt:   pr.UpdatedAt,
				RepoKey:     "github.com/" + pr.Owner + "/" + pr.Repo,
			},
		) {
			glog.V(3).Infof("skipping pr=%s/%s#%d reason=filtered", pr.Owner, pr.Repo, pr.Number)
			w.metrics.IncPRPublished("skipped")
			// Filtered PRs do not contribute entries to newHeadSHAs. If the PR was previously
			// published, its SHA-based cursor entry is pruned here and will be re-created on
			// the next successful (non-filtered) poll — controller dedup prevents a duplicate file.
			continue
		}

		details, err := w.fetchPRDetails(ctx, pr, prDetailsCache)
		if err != nil {
			glog.Errorf(
				"get pr details failed pr=%s/%s#%d err=%v",
				pr.Owner,
				pr.Repo,
				pr.Number,
				err,
			)
			// Same rationale as filtered PRs: cannot preserve old SHA-based entry without
			// knowing the SHA. Transient error → re-publish on next poll → controller deduplicates.
			continue
		}

		// Fail-closed: if head SHA is absent, skip this PR on this poll.
		if details.HeadSHA == "" {
			glog.Warningf("missing head SHA for pr=%s/%s#%d, skipping", pr.Owner, pr.Repo, pr.Number)
			continue
		}

		taskIDStr := DeriveTaskID(pr.Owner, pr.Repo, pr.Number, details.HeadSHA).String()

		if _, exists := cursorState.HeadSHAs[taskIDStr]; exists {
			// Same (PR, SHA) already spawned — no-op.
			glog.V(3).Infof(
				"no change, skipping pr=%s/%s#%d sha=%s taskID=%s",
				pr.Owner, pr.Repo, pr.Number, details.HeadSHA, taskIDStr,
			)
			newHeadSHAs[taskIDStr] = details.HeadSHA
			if pr.UpdatedAt.After(maxUpdatedAt) {
				maxUpdatedAt = pr.UpdatedAt
			}
			continue
		}

		// New (PR, SHA) pair — publish a fresh CreateTaskCommand.
		if w.publishCreate(ctx, pr, taskIDStr, details) {
			// Update cursorState in-place so duplicate PR entries in the same poll batch
			// are deduplicated without a second create publish.
			cursorState.HeadSHAs[taskIDStr] = details.HeadSHA
			newHeadSHAs[taskIDStr] = details.HeadSHA
			if pr.UpdatedAt.After(maxUpdatedAt) {
				maxUpdatedAt = pr.UpdatedAt
			}
		}
	}

	cursorState.HeadSHAs = newHeadSHAs
	return maxUpdatedAt
}
```

**3c. Update `fetchPRDetails` — change cache key from `taskIDStr` to PR-based key.**

The cache key must not depend on the SHA (because we don't have the SHA yet when we first call this function):

```go
func (w *watcher) fetchPRDetails(
	ctx context.Context,
	pr PullRequest,
	cache map[string]PRDetails,
) (PRDetails, error) {
	cacheKey := fmt.Sprintf("%s/%s#%d", pr.Owner, pr.Repo, pr.Number)
	if details, ok := cache[cacheKey]; ok {
		return details, nil
	}
	details, err := w.ghClient.GetPRDetails(ctx, pr.Owner, pr.Repo, pr.Number)
	if err != nil {
		return PRDetails{}, errors.Wrapf(
			ctx,
			err,
			"get pr details pr=%s/%s#%d",
			pr.Owner,
			pr.Repo,
			pr.Number,
		)
	}
	cache[cacheKey] = details
	return details, nil
}
```

**3d. Update `publishCreate` — remove `cursorState *Cursor` parameter and update `computePRTitle` call.**

The caller (`processPRs`) now handles the cursor update. Pass `details.HeadSHA` as the new `sha` argument to `computePRTitle`:

```go
func (w *watcher) publishCreate(
	ctx context.Context,
	pr PullRequest,
	taskIDStr string,
	details PRDetails,
) bool {
	author := pr.AuthorLogin

	trustResult, err := w.trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: author})
	if err != nil {
		glog.Errorf("trust check failed pr=%s err=%v", pr.HTMLURL, err)
		w.metrics.IncPRPublished("error")
		return false
	}

	var cmd task.CreateCommand
	if trustResult.Success() {
		cmd = task.CreateCommand{
			Title:          computePRTitle("github", pr.Owner, pr.Repo, pr.Number, details.HeadSHA, pr.Title),
			TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
			Frontmatter:    buildFrontmatter(pr, taskIDStr, w.stage, details),
			Body:           buildTaskBody(pr),
		}
	} else {
		if author == "" {
			author = "(unknown)"
		}
		glog.V(2).Infof("untrusted author=%q trust=%s pr=%s", author, trustResult.Description(), pr.HTMLURL)
		cmd = task.CreateCommand{
			Title:          computePRTitle("github", pr.Owner, pr.Repo, pr.Number, details.HeadSHA, pr.Title),
			TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
			Frontmatter:    buildHumanReviewFrontmatter(pr, taskIDStr, w.stage, details),
			Body:           buildUntrustedBody(author, trustResult.Description()),
		}
	}

	if err := w.createSender.SendCommand(ctx, cmd); err != nil {
		glog.Errorf("publish create-task failed pr=%s err=%v", pr.HTMLURL, err)
		w.metrics.IncPRPublished("error")
		return false
	}
	glog.V(2).Infof("published CreateTaskCommand pr=%s/%s#%d sha=%s taskID=%s trusted=%t",
		pr.Owner, pr.Repo, pr.Number, details.HeadSHA, taskIDStr, trustResult.Success())
	w.metrics.IncPRPublished("create")
	return true
}
```

**3e. Delete `handlePR` and `publishForcePush` entirely.** Remove both functions in full. Remove any `fmt.Sprintf("## Outdated by force-push %s", ...)` string.

**3f. Update imports in `watcher.go`.**

After the changes:
- Remove `task "github.com/bborbe/agent/lib/command/task"` — wait, `task.CreateCommandSender` and `task.CreateCommand` are still used. Keep the import.
- `task.UpdateFrontmatterCommandSender`, `task.UpdateFrontmatterCommand`, `task.BodySection` are all gone (were only used by `publishForcePush`). Remove any direct references to these types.
- `"fmt"` — still needed for `buildTaskBody`, `buildUntrustedBody`, and the new `fetchPRDetails` cache key.
- Verify: run `grep -n "UpdateFrontmatter\|BodySection\|publishForcePush\|Outdated by force-push" watcher/github-pr/pkg/watcher.go` → 0 matches.

---

## Step 4 — Update `watcher/github-pr/pkg/factory/factory.go`

Read the file fully. Make two changes:

**4a. Simplify `CreateKafkaSenders` — return only `task.CreateCommandSender`.**

```go
// CreateKafkaSender constructs a typed create-task command sender backed by a Kafka sync producer.
// The cleanup function closes the underlying sync producer on shutdown.
func CreateKafkaSender(
	ctx context.Context,
	brokers libkafka.Brokers,
	branch base.Branch,
) (task.CreateCommandSender, func(), error) {
	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx,
		brokers,
		"maintainer-watcher-github-pr",
	)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create sync producer")
	}
	sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
	cleanup := func() {
		if err := syncProducer.Close(); err != nil {
			glog.Warningf("close kafka sync producer: %v", err)
		}
	}
	return task.NewCreateCommandSender(sender), cleanup, nil
}
```

**4b. Update `CreateWatcher` to call `CreateKafkaSender` and pass only `createSender` to `pkg.NewWatcher`.**

```go
// CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
func CreateWatcher(
	ctx context.Context,
	ghToken string,
	brokers libkafka.Brokers,
	stage string,
	repoScope string,
	taskCreationFilter filter.TaskCreationFilter,
	startTime libtime.DateTime,
	trustedAuthors []string,
) (pkg.Watcher, func(), error) {
	branch := base.Branch(stage)
	createSender, cleanup, err := CreateKafkaSender(ctx, brokers, branch)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create kafka sender")
	}

	trustDecision := trust.And{trust.NewAuthorAllowlist(trustedAuthors)}
	ghClient := pkg.NewGitHubClient(ghToken)
	w := pkg.NewWatcher(
		ghClient,
		createSender,
		pkg.DefaultCursorPath,
		startTime,
		repoScope,
		taskCreationFilter,
		stage,
		pkg.NewMetrics(),
		trustDecision,
	)
	return w, cleanup, nil
}
```

**4c. Update imports in `factory.go`** — `task.UpdateFrontmatterCommandSender` is no longer used; verify no stale import references remain.

---

## Step 5 — Run `make test` (fast feedback)

```bash
cd watcher/github-pr && make test
```

The tests will fail at this point because `watcher_test.go` still uses the old API (old `NewWatcher` signature with `updateFrontmatterSender`, old `DeriveTaskID`, old title strings). Fix all compile errors before proceeding to step 6.

---

## Step 6 — Update `watcher/github-pr/pkg/taskid_test.go`

Read the file fully. Make these changes:

**6a. Update all `DeriveTaskID` calls to include a SHA argument.**

Everywhere `pkg.DeriveTaskID(owner, repo, number)` appears, add a SHA:
- `pkg.DeriveTaskID("bborbe", "code-reviewer", 42)` → `pkg.DeriveTaskID("bborbe", "code-reviewer", 42, "abc123def456789a")`
- Use a consistent test SHA like `"abc123def456789a"` for all determinism/collision tests.

**6b. Update the "pinned UUID" test** to reflect the new key format:

```go
It("produces the expected pinned UUID for bborbe/code-reviewer#42@abc123def456789a", func() {
    expected := uuid.NewSHA1(prWatcherNamespace, []byte("bborbe/code-reviewer#42@abc123def456789a"))
    Expect(pkg.DeriveTaskID("bborbe", "code-reviewer", 42, "abc123def456789a")).To(Equal(expected))
})
```

**6c. Add a UUID5 stability test** — two calls with identical inputs produce identical UUIDs; any change to any input produces a different UUID:

```go
It("produces different UUIDs for same PR but different SHAs", func() {
    a := pkg.DeriveTaskID("bborbe", "code-reviewer", 42, "sha-aaa")
    b := pkg.DeriveTaskID("bborbe", "code-reviewer", 42, "sha-bbb")
    Expect(a).NotTo(Equal(b))
})

It("two calls with identical (owner, repo, number, sha) produce the same UUID", func() {
    a := pkg.DeriveTaskID("bborbe", "code-reviewer", 42, "sha-stable")
    b := pkg.DeriveTaskID("bborbe", "code-reviewer", 42, "sha-stable")
    Expect(a).To(Equal(b))
})
```

---

## Step 7 — Update `watcher/github-pr/pkg/filename_internal_test.go`

Read the file fully. Make these changes:

**7a. Update the `computePRTitle` describe block** — add `sha` to every `DescribeTable` entry call and update expected strings.

The new call signature is `computePRTitle(provider, owner, repo, number, sha, title)`.

Replace the `Describe("computePRTitle", ...)` block's table entries. For each entry, insert a `sha` argument and add the `sha[:8]` segment in the expected string. The SHA segment comes immediately after the PR number.

Use `sha = "abc12345def67890"` (16 hex chars) for most entries so `sha[:8] = "abc12345"`. For each entry:
- Expected prefix changes from `"PR Review ... - {number} - {slug}"` to `"PR Review ... - {number} - abc12345 - {slug}"`
- Empty-slug variant: `"PR Review ... - {number}"` → `"PR Review ... - {number} - abc12345"`

Example entries:
```go
Entry("normal PR with title",
    "github", "bborbe", "maintainer", 2, "abc12345def67890", "test: delete this PR never",
    "PR Review github - bborbe-maintainer - 2 - abc12345 - test-delete-this-pr-never"),
Entry("empty title → no slug segment",
    "github", "bborbe", "x", 7, "abc12345def67890", "",
    "PR Review github - bborbe-x - 7 - abc12345"),
Entry("whitespace-only title → no slug segment",
    "github", "bborbe", "x", 7, "abc12345def67890", "   ",
    "PR Review github - bborbe-x - 7 - abc12345"),
Entry("unicode-only title → no slug segment",
    "github", "bborbe", "x", 7, "abc12345def67890", "🚀🎉",
    "PR Review github - bborbe-x - 7 - abc12345"),
```

Update ALL entries in the table consistently.

**7b. Update the wire-format `DescribeTable` entries** in the `task.CreateCommand.Validate` block — add SHA argument to `computePRTitle` calls:

```go
Entry("typical PR", "github", "bborbe", "maintainer", 2, "abc12345def67890", "test: delete this PR never"),
Entry("hyphenated repo", "github", "my-org", "my-repo", 99, "abc12345def67890", "bump deps"),
// ...
```

---

## Step 8 — Rewrite `watcher/github-pr/pkg/watcher_test.go`

Read the file fully before making any changes. This step is the most extensive.

**8a. Update `newTestWatcher` — remove `updateFrontmatterSender` parameter.**

```go
func newTestWatcher(
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	cursorPath string,
	startTime libtime.DateTime,
	fakeMetrics *mocks.Metrics,
	trustDecision trust.Trust,
) pkg.Watcher {
	return pkg.NewWatcher(
		ghClient,
		createSender,
		cursorPath,
		startTime,
		"bborbe",
		filter.TaskCreationFilters{
			filter.NewDraftFilter(),
			filter.NewBotAuthorFilter([]string{"dependabot[bot]"}),
		},
		"dev",
		fakeMetrics,
		trustDecision,
	)
}
```

**8b. Remove `updateFrontmatterSender` from the top-level `Describe` variable block and `BeforeEach`.**

Remove:
```go
updateFrontmatterSender *taskmocks.TaskUpdateFrontmatterCommandSender
```
and:
```go
updateFrontmatterSender = new(taskmocks.TaskUpdateFrontmatterCommandSender)
```

**8c. Update ALL `newTestWatcher(...)` call sites** — remove the `updateFrontmatterSender` argument from every call.

**8d. Remove ALL `Expect(updateFrontmatterSender.SendCommandCallCount()).To(Equal(0))` assertions** — they're no longer meaningful (the sender doesn't exist).

**8e. Update title assertions throughout** — all `cmd.Title` expectations must include the SHA segment. The SHA in test fixtures is typically short (e.g., `"abc123"`, `"sha1"`, `"sha-existing"`). The `shortSHA` helper returns `sha[:min(8, len(sha))]`, so:
- SHA `"abc123"` (6 chars) → segment = `"abc123"`
- SHA `"sha1"` (4 chars) → segment = `"sha1"`
- SHA `"sha-existing"` (12 chars) → segment = `"sha-exis"`

Update every `Expect(cmd.Title).To(Equal(...))` to include the SHA segment. For example:
- `"PR Review github - bborbe-code-reviewer - 42 - feat-new-feature"` → `"PR Review github - bborbe-code-reviewer - 42 - abc123 - feat-new-feature"` (sha="abc123")
- `"PR Review github - bborbe-repo - 5 - my-title"` → `"PR Review github - bborbe-repo - 5 - sha1 - my-title"` (sha="sha1")
- `"PR Review github - bborbe-repo - 10 - some-pr"` → `"PR Review github - bborbe-repo - 10 - sha1 - some-pr"` (sha="sha1")

**8f. Rewrite the "Force-push (existing entry, different SHA)" Describe block.**

This test previously asserted `UpdateFrontmatterCommand` was published. Rewrite to assert a new `CreateTaskCommand` is published for the new SHA:

```go
Describe("New commit on existing PR (different SHA) — per-SHA spawn", func() {
    It("publishes a new CreateTaskCommand for the new SHA; does not publish UpdateFrontmatterCommand", func() {
        pr := pkg.PullRequest{
            Number:      42,
            Owner:       "bborbe",
            Repo:        "code-reviewer",
            Title:       "force pushed pr",
            AuthorLogin: "alice",
            UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
        }

        // First poll: SHA=old-sha → CreateTaskCommand published
        ghClient.SearchPRsReturns(pkg.SearchResult{
            PullRequests:  []pkg.PullRequest{pr},
            HasNextPage:   false,
            RateRemaining: 100,
        }, nil)
        ghClient.GetPRDetailsReturns(
            pkg.PRDetails{
                HeadSHA:  "old-sha",
                CloneURL: "https://github.com/owner/repo.git",
                BaseRef:  "master",
            },
            nil,
        )
        createSender.SendCommandReturns(nil)

        w := newTestWatcher(
            ghClient,
            createSender,
            cursorPath,
            startTime,
            fakeMetrics,
            trust.NewAuthorAllowlist([]string{"alice"}),
        )
        Expect(w.Poll(ctx)).NotTo(HaveOccurred())
        Expect(createSender.SendCommandCallCount()).To(Equal(1))

        // Second poll: SHA=new-sha → new CreateTaskCommand, NOT UpdateFrontmatterCommand
        createSender2 := new(taskmocks.TaskCreateCommandSender)
        ghClient.GetPRDetailsReturns(
            pkg.PRDetails{
                HeadSHA:  "new-sha",
                CloneURL: "https://github.com/owner/repo.git",
                BaseRef:  "master",
            },
            nil,
        )
        createSender2.SendCommandReturns(nil)

        w2 := newTestWatcher(
            ghClient,
            createSender2,
            cursorPath,
            startTime,
            fakeMetrics,
            trust.NewAuthorAllowlist([]string{"alice"}),
        )
        Expect(w2.Poll(ctx)).NotTo(HaveOccurred())

        // New SHA → new CreateTaskCommand published
        Expect(createSender2.SendCommandCallCount()).To(Equal(1))
        _, cmd := createSender2.SendCommandArgsForCall(0)
        Expect(cmd.Title).To(ContainSubstring("new-sha"))
        // Old task identifier (old-sha) must differ from new task identifier (new-sha)
        _, cmd1 := createSender.SendCommandArgsForCall(0)
        Expect(string(cmd.TaskIdentifier)).NotTo(Equal(string(cmd1.TaskIdentifier)))
    })
})
```

**8g. Rewrite the "UpdateFrontmatterCommand fields" Describe block.**

This block currently asserts heading format `"## Outdated by force-push sha-v1"`. Rewrite as:

```go
Describe("New commit (per-SHA spawn) — title contains SHA segment", func() {
    It("CreateTaskCommand title contains sha[:8] between PR number and slug", func() {
        pr := pkg.PullRequest{
            Number:      7,
            Owner:       "bborbe",
            Repo:        "repo",
            AuthorLogin: "alice",
            UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
        }

        // Poll 1: SHA=sha-v1
        ghClient.SearchPRsReturns(pkg.SearchResult{
            PullRequests:  []pkg.PullRequest{pr},
            HasNextPage:   false,
            RateRemaining: 100,
        }, nil)
        ghClient.GetPRDetailsReturns(
            pkg.PRDetails{
                HeadSHA:  "sha-v1xx",
                CloneURL: "https://github.com/owner/repo.git",
                BaseRef:  "master",
            },
            nil,
        )
        createSender.SendCommandReturns(nil)
        w := newTestWatcher(ghClient, createSender, cursorPath, startTime, fakeMetrics,
            trust.NewAuthorAllowlist([]string{"alice"}))
        Expect(w.Poll(ctx)).NotTo(HaveOccurred())

        // Poll 2: SHA=sha-v2xx — new CreateTaskCommand, title contains "sha-v2xx"[:8]="sha-v2xx"
        createSender2 := new(taskmocks.TaskCreateCommandSender)
        ghClient.GetPRDetailsReturns(
            pkg.PRDetails{
                HeadSHA:  "sha-v2xx",
                CloneURL: "https://github.com/owner/repo.git",
                BaseRef:  "master",
            },
            nil,
        )
        createSender2.SendCommandReturns(nil)
        w2 := newTestWatcher(ghClient, createSender2, cursorPath, startTime, fakeMetrics,
            trust.NewAuthorAllowlist([]string{"alice"}))
        Expect(w2.Poll(ctx)).NotTo(HaveOccurred())

        Expect(createSender2.SendCommandCallCount()).To(Equal(1))
        _, cmd := createSender2.SendCommandArgsForCall(0)
        Expect(cmd.Title).To(ContainSubstring("sha-v2xx"))
        Expect(cmd.Frontmatter["ref"]).To(Equal("sha-v2xx"))
        Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
    })
})
```

**8h. Rewrite "publishForcePush Kafka publish error" Describe block.**

Rename to "New SHA create Kafka error — cursor does not advance to new SHA" and rewrite:

```go
Describe("New SHA create Kafka error — cursor does not advance to new SHA", func() {
    It("does not add new SHA's task ID to cursor when publish fails, Poll returns nil", func() {
        pr := pkg.PullRequest{
            Number:      55,
            Owner:       "bborbe",
            Repo:        "repo",
            AuthorLogin: "alice",
            UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
        }

        // Poll 1: SHA=sha-v1 → create succeeds
        ghClient.SearchPRsReturns(pkg.SearchResult{
            PullRequests:  []pkg.PullRequest{pr},
            HasNextPage:   false,
            RateRemaining: 100,
        }, nil)
        ghClient.GetPRDetailsReturns(
            pkg.PRDetails{
                HeadSHA:  "sha-v1",
                CloneURL: "https://github.com/owner/repo.git",
                BaseRef:  "master",
            },
            nil,
        )
        createSender.SendCommandReturns(nil)

        w := newTestWatcher(ghClient, createSender, cursorPath, startTime, fakeMetrics,
            trust.NewAuthorAllowlist([]string{"alice"}))
        Expect(w.Poll(ctx)).NotTo(HaveOccurred())
        Expect(createSender.SendCommandCallCount()).To(Equal(1))

        // Poll 2: SHA=sha-v2 → create FAILS
        ghClient.GetPRDetailsReturns(
            pkg.PRDetails{
                HeadSHA:  "sha-v2",
                CloneURL: "https://github.com/owner/repo.git",
                BaseRef:  "master",
            },
            nil,
        )
        createSender2 := new(taskmocks.TaskCreateCommandSender)
        createSender2.SendCommandReturns(errors.New("kafka unavailable"))
        // w2 loads cursor from disk (written by w's Poll above); mocks reset to isolate assertions.
        w2 := newTestWatcher(ghClient, createSender2, cursorPath, startTime, fakeMetrics,
            trust.NewAuthorAllowlist([]string{"alice"}))
        Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
        Expect(createSender2.SendCommandCallCount()).To(Equal(1))

        // SHA-v2 task ID must NOT be in cursor after failed publish
        taskIDv2 := pkg.DeriveTaskID(pr.Owner, pr.Repo, pr.Number, "sha-v2").String()
        cursor, err := pkg.LoadCursor(ctx, cursorPath, startTime)
        Expect(err).NotTo(HaveOccurred())
        Expect(cursor.HeadSHAs).NotTo(HaveKey(taskIDv2))
    })
})
```

**8i. Rewrite "Untrusted-author force-push" Describe block.**

In the per-SHA model, a "force-push" from an untrusted author is just a new SHA that produces a new `CreateTaskCommand` with `human_review/todo` frontmatter. This block lives **inside** the `Describe("Trust decisions", func() { ... })` block so that the outer `BeforeEach` (which sets up `pr`, `ghClient.SearchPRsReturns`, and `ghClient.GetPRDetailsReturns` with `HeadSHA: "sha1"`) applies. Rewrite:

```go
Describe("Untrusted-author new commit", func() {
    It("publishes CreateTaskCommand with human_review/todo frontmatter for new SHA", func() {
        createSender.SendCommandReturns(nil)
        w := newTestWatcher(ghClient, createSender, cursorPath, startTime, fakeMetrics,
            trust.NewAuthorAllowlist([]string{"bob"}))
        Expect(w.Poll(ctx)).NotTo(HaveOccurred())
        Expect(createSender.SendCommandCallCount()).To(Equal(1))

        createSender2 := new(taskmocks.TaskCreateCommandSender)
        ghClient.GetPRDetailsReturns(
            pkg.PRDetails{
                HeadSHA:  "sha2",
                CloneURL: "https://github.com/owner/repo.git",
                BaseRef:  "master",
            },
            nil,
        )
        createSender2.SendCommandReturns(nil)
        w2 := newTestWatcher(ghClient, createSender2, cursorPath, startTime, fakeMetrics,
            trust.NewAuthorAllowlist([]string{"bob"}))
        Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
        Expect(createSender2.SendCommandCallCount()).To(Equal(1))
        _, cmd := createSender2.SendCommandArgsForCall(0)
        Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
        Expect(cmd.Frontmatter["assignee"]).To(Equal(""))
        Expect(cmd.Frontmatter["phase"]).To(Equal("human_review"))
        Expect(cmd.Frontmatter["status"]).To(Equal("todo"))
    })
})
```

**8j. Update "Closed PR pruned from cursor" — update `DeriveTaskID` calls to include SHA.**

The test uses `SHA = "sha-initial"` for all PRs. Update the assertions:

```go
taskIDA := pkg.DeriveTaskID(prA.Owner, prA.Repo, prA.Number, "sha-initial").String()
taskIDB := pkg.DeriveTaskID(prB.Owner, prB.Repo, prB.Number, "sha-initial").String()
Expect(cursor.HeadSHAs).To(HaveKey(taskIDA))
Expect(cursor.HeadSHAs).NotTo(HaveKey(taskIDB))
```

**8k. Update "Trust check returns an error" — update `DeriveTaskID` call to include SHA.**

The `BeforeEach` for the "Trust decisions" block sets `HeadSHA: "sha1"`. Update:

```go
taskIDStr := pkg.DeriveTaskID(pr.Owner, pr.Repo, pr.Number, "sha1").String()
Expect(cursor.HeadSHAs).NotTo(HaveKey(taskIDStr))
```

**8l. Remove import of `taskmocks "github.com/bborbe/agent/lib/command/task/mocks"`** ONLY if `TaskUpdateFrontmatterCommandSender` was the only type used from that package. If `TaskCreateCommandSender` is still used (it is), keep the import.

**8m. Remove `agentlib` import from `watcher_test.go`** if no longer used — check by grepping for `agentlib.` references after all edits. If `agentlib.TaskIdentifier` still appears, keep it.

---

## Step 9 — Run `make test` again

```bash
cd watcher/github-pr && make test
```

All tests must pass. Fix any remaining compilation errors or assertion mismatches.

---

## Step 10 — Add CHANGELOG entry

Prepend a new `## Unreleased` section to root `CHANGELOG.md` above the latest `## vX.Y.Z` header:

```markdown
## Unreleased

- feat(watcher/github-pr): per-(PR, SHA) spawn model — each push produces a new task file identified by the head commit SHA; the old task file is never mutated; removes `publishForcePush` and the `## Outdated by force-push` mutation path; `DeriveTaskID` now encodes full SHA in UUID5 key; `computePRTitle` adds `sha[:8]` segment between PR number and slug
```

---

## Step 11 — Vault runbook note (manual — outside container scope)

The spec requires updating the Obsidian vault runbook `Create PR Review Agent Task.md` to remove the "Re-run on the same PR (force fresh agent run)" section and document the new per-commit behavior. This file lives in the Obsidian vault (outside this container) and must be updated by the operator after deploy. Add no code change for this step; it is a post-deploy manual action.

---

## Step 12 — Verify removal of force-push path

```bash
grep -rn "publishForcePush\|Outdated by force-push" watcher/github-pr/pkg/
```

Expected: zero matches. (The `task.UpdateFrontmatterCommandSender` type lives in the imported library; the above grep deliberately excludes `UpdateFrontmatter` from the check to avoid false positives from library import comments or test-mock stubs that may mention the type name.)

---

## Step 13 — Run `make precommit`

```bash
cd watcher/github-pr && make precommit
```

Must exit 0.

</requirements>

<constraints>

- `prWatcherNamespace` UUID constant (`7d4b3e5f-8a21-4c9d-b036-2e5f7a8c1d0e`) is frozen — do NOT change it.
- The UUID5 key changes from `<owner>/<repo>#<number>` to `<owner>/<repo>#<number>@<sha>` (full SHA, not `sha[:8]`). The filename uses `sha[:8]`; the dedup key uses the full SHA. These are independent by design.
- `computePRTitle` grammar: `sha[:8]` segment is always present (even for empty-slug variants). New no-slug format: `"PR Review {provider} - {owner}-{repo} - {number} - {sha[:8]}"` — no trailing ` - `.
- `shortSHA` logic: use `if len(sha) > 8 { sha = sha[:8] }` — do NOT use `sha[:8]` directly without length guard (test SHAs are often fewer than 8 chars).
- `publishForcePush` must be removed entirely. No remnant call sites, no remnant test assertions about `## Outdated by force-push`.
- `updateFrontmatterSender` must be removed from the watcher struct, `NewWatcher` constructor, `factory.go`. The `task.UpdateFrontmatterCommandSender` type and `task.UpdateFrontmatterCommand` command type remain in the agent library and are not renamed.
- Filter check (draft, bot, etc.) continues to happen BEFORE fetching `PRDetails` — no optimization regression.
- `fetchPRDetails` cache key changes from `taskIDStr` to `owner/repo#number` — one API call per PR per poll, regardless of how many times the same PR appears in search results.
- In-poll dedup: `processPRs` updates `cursorState.HeadSHAs` in-place after a successful publish so the second occurrence of the same PR in the same poll batch is treated as "already spawned" and skipped.
- Only edit files under `watcher/github-pr/` and root `CHANGELOG.md`.
- Do NOT commit — dark-factory handles git.
- `make precommit` runs from `watcher/github-pr/`, never at repo root.
- **Sibling spec 025 (pr-reviewer binary-verdict) may land in parallel.** This prompt touches only watcher code; no overlap with the pr-reviewer agent's verdict refactor. No coordination needed.
- **CRITICAL pre-merge check (manual, before approving the PR generated from this prompt):** verify the controller (in the `bborbe/agent` repo or wherever `task.CreateCommand` is consumed) deduplicates incoming creates by `task_identifier`. The cursor-preservation behavior dropped in Step 3b's `processPRs` rewrite assumes this dedup exists. If it doesn't, every transient `GetPRDetails` failure or filter-toggle will produce a duplicate vault file. If verification reveals the dedup does NOT exist, the prompt is unsafe to ship — file a follow-up issue and pause the merge.
- Error wrapping: `github.com/bborbe/errors` — never `fmt.Errorf`.
- Test coverage ≥80% for changed packages.
- `slugifyTitle` is unchanged.
- No new scenario file — the spec explicitly states no new automated scenario test is needed.

</constraints>

<verification>

Run precommit:
```bash
cd watcher/github-pr && make precommit
```
Expected: exit 0.

Confirm force-push path gone:
```bash
grep -rn "publishForcePush\|Outdated by force-push" watcher/github-pr/pkg/
```
Expected: zero matches.

Confirm new DeriveTaskID signature:
```bash
grep -n "func DeriveTaskID" watcher/github-pr/pkg/taskid.go
```
Expected: signature includes `sha string` parameter.

Confirm SHA segment in computePRTitle:
```bash
grep -n "func computePRTitle\|sha\[:8\]\|shortSHA" watcher/github-pr/pkg/filename.go
```
Expected: `computePRTitle` accepts `sha` param; `shortSHA` guard present.

Confirm updateFrontmatterSender removed from watcher:
```bash
grep -n "updateFrontmatterSender\|UpdateFrontmatterCommandSender" watcher/github-pr/pkg/watcher.go
```
Expected: zero matches.

Confirm factory simplified:
```bash
grep -n "UpdateFrontmatterCommandSender\|updateFrontmatterSender" watcher/github-pr/pkg/factory/factory.go
```
Expected: zero matches.

Confirm test coverage:
```bash
cd watcher/github-pr && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/... && go tool cover -func=/tmp/cover.out | grep -E "^total|pkg/watcher|pkg/taskid|pkg/filename"
```
Expected: total coverage ≥80% for changed packages.

Confirm CHANGELOG:
```bash
grep -n "per-.*SHA\|sha.*spawn\|publishForcePush\|Outdated by force-push" CHANGELOG.md | head -5
```
Expected: one match under `## Unreleased`.

</verification>
