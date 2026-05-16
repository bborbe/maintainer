---
status: committing
summary: 'Added clickable GitHub repo links to both watcher task bodies: github-build H1 now uses markdown-link form, github-pr adds a **Repo:** line after the PR URL; tests updated and added; CHANGELOG updated.'
container: maintainer-107-add-repo-link-to-watcher-task-bodies
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-08T15:31:49Z"
queued: "2026-05-08T15:31:49Z"
started: "2026-05-16T11:20:47Z"
---

<summary>
- Add a clickable GitHub repo link to the body of both maintainer-watcher tasks (`github-pr` and `github-build`).
- **Build-watcher** (`watcher/github-build`): change the H1 from plain `# Build Failure: bborbe/maintainer` to a markdown-link form `# Build Failure: [bborbe/maintainer](https://github.com/bborbe/maintainer)`. One-line edit in `pkg/watcher.go:315`.
- **PR-watcher** (`watcher/github-pr`): keep the existing H1 (which links to the PR title via the PR URL — see below), and add a new `**Repo:** [owner/repo](https://github.com/owner/repo)` line right under the H1. The PR-watcher's existing body has the PR URL but no repo link.
- Both repo links use the constant provider `github` and the watcher's existing `owner` + `repo` strings — no new fields, no new API calls. URL shape: `https://github.com/{owner}/{repo}`.
- Existing wire-format contract tests stay unchanged. New tests assert each task body contains the repo link.
- CHANGELOG entry under the next unreleased section noting the body-shape change.
- `make precommit` clean in both watcher modules.
</summary>

<objective>
Operators reading vault tasks see only one clickable link today: the GitHub Actions run (build-watcher) or the PR URL (pr-watcher). The repository the failure/PR belongs to is plain text in the H1. Clicking through to the repo (to inspect README, recent commits, branch state, or open issues) requires manually URL-typing.

Adding a repo link is a 1-2-line aesthetic improvement that pays back every time someone triages a task. The owner+repo data is already in the watcher; this prompt just wires it into a `https://github.com/{owner}/{repo}` URL in the markdown body.

The two watchers have different existing body shapes, so they get slightly different treatments — build-watcher's repo link goes in the H1 (no other natural slot); pr-watcher's H1 already links to the PR title, so the repo link gets a dedicated line.
</objective>

<context>

## Files to edit

### `watcher/github-build/pkg/watcher.go` — `buildBodyHeader` (around line 313)

**Current:**

```go
// buildBodyHeader builds the markdown header lines for a build-failure task body.
func (w *buildWatcher) buildBodyHeader(firstRun WorkflowRun, owner, repo string) []string {
    lines := make([]string, 0, 10)
    lines = append(lines, fmt.Sprintf("# Build Failure: %s/%s", owner, repo), "")
    if firstRun.DisplayTitle != "" {
        lines = append(lines, fmt.Sprintf("**Commit:** %s", firstRun.DisplayTitle))
    }
    // ... rest unchanged
}
```

**After:**

```go
// buildBodyHeader builds the markdown header lines for a build-failure task body.
// The H1 includes a clickable GitHub repo link (https://github.com/{owner}/{repo}).
func (w *buildWatcher) buildBodyHeader(firstRun WorkflowRun, owner, repo string) []string {
    lines := make([]string, 0, 10)
    lines = append(
        lines,
        fmt.Sprintf("# Build Failure: [%s/%s](https://github.com/%s/%s)", owner, repo, owner, repo),
        "",
    )
    if firstRun.DisplayTitle != "" {
        lines = append(lines, fmt.Sprintf("**Commit:** %s", firstRun.DisplayTitle))
    }
    // ... rest unchanged
}
```

(Only the H1 line changes. Everything below it is byte-identical.)

### `watcher/github-pr/pkg/watcher.go` — `buildTaskBody` (around line 337)

**Current:**

```go
func buildTaskBody(pr PullRequest) string {
    return fmt.Sprintf("# PR Review: %s\n\n%s\n", pr.Title, pr.HTMLURL)
}
```

**After:**

```go
func buildTaskBody(pr PullRequest) string {
    repoLink := fmt.Sprintf("https://github.com/%s/%s", pr.Owner, pr.Repo)
    return fmt.Sprintf(
        "# PR Review: %s\n\n%s\n\n**Repo:** [%s/%s](%s)\n",
        pr.Title,
        pr.HTMLURL,
        pr.Owner,
        pr.Repo,
        repoLink,
    )
}
```

The PR's HTMLURL stays where it is (raw URL — Obsidian auto-links). The new `**Repo:**` line goes right after, separated by a blank line.

If `pr.Owner` or `pr.Repo` are not currently fields on `PullRequest`, verify they exist by reading the struct — they should, since `computePRTitle("github", pr.Owner, pr.Repo, pr.Number, pr.Title, ...)` already uses them. If absent, add them (small change, plumb from GitHub API response same as `pr.Title`).

### Tests

**`watcher/github-build/pkg/watcher_test.go`** — there is an existing assertion at line ~500 of the form `Expect(body).To(ContainSubstring("# Build Failure: owner/repo"))`. **Update that assertion** to match the new markdown-link H1: `Expect(body).To(ContainSubstring("# Build Failure: [owner/repo](https://github.com/owner/repo)"))`. Do NOT remove or alter any existing wire-format negative-assertion entries (e.g. `NotTo(ContainSubstring("filename_hint"))` — those are the wire contract from prompt 104; keep verbatim).

**`watcher/github-pr/pkg/watcher_test.go`** (or wherever `buildTaskBody` is tested) — add an assertion that the body contains `**Repo:** [bborbe/foo](https://github.com/bborbe/foo)`. If no such test exists yet, add a new `Describe("buildTaskBody", ...)` block.

### CHANGELOG.md — new unreleased-version entry

Top of file currently has `v0.23.32` as the latest entry. Add under the existing `## Unreleased` section (created by prompt 105) — append this line after the prompt 105 entry and any other unreleased entries:

```
- feat(watcher/github-pr,watcher/github-build): vault task bodies now include a clickable GitHub repo link. github-build's H1 becomes a link to https://github.com/{owner}/{repo}; github-pr adds a **Repo:** line under the existing PR-URL line. Operators triaging tasks no longer need to URL-type to reach the repo top-level.
```

</context>

<constraints>

- Wire format MUST NOT change. Only the markdown body content changes; the `Title` field, `task_identifier`, frontmatter keys, and Kafka schema are all unchanged.
- Existing wire-format contract tests (`NotTo(ContainSubstring("filename_hint"))` from prompt 104) MUST remain unchanged.
- Existing CHANGELOG entries MUST NOT be edited.
- Existing vault tasks (already-written `.md` files) are NOT modified — only future tasks get the new body shape.
- No new GitHub API calls. No new dependencies. The URL is constructed from existing `owner` + `repo` strings.
- Errors MUST be wrapped with `github.com/bborbe/errors`. (None expected here; pure string formatting.)
- `make precommit` MUST be clean in both `watcher/github-pr/` and `watcher/github-build/`.
- Coverage on changed packages MUST remain ≥80%.

</constraints>

<failure_modes>

| Trigger | Expected behaviour | Recovery |
|---|---|---|
| `pr.Owner` or `pr.Repo` not already fields on `PullRequest` struct | Compile error → add the fields, plumb from GitHub API client | Read `watcher/github-pr/pkg/githubclient.go` — owner/repo are derived during `ListOpenPRs`; ensure they're populated on the `PullRequest` struct |
| Markdown-renderer doesn't auto-link the URL inside the H1 | Obsidian + GitHub-flavored markdown both handle H1-with-link correctly. If a custom renderer in the consumer chain breaks, this prompt's scope ends here | Filing as a separate spec to escape-html the link if needed |
| Existing test asserts on the H1 string verbatim | Test fails — update the test to assert on the new link form | Change is intentional; tests follow the new shape |
| `owner` or `repo` contain characters that need URL-encoding (e.g. spaces) | **Do NOT add `url.PathEscape`** — GitHub's own naming rules constrain these to `[a-zA-Z0-9._-]+` (validated upstream by GitHub when the repo was created); raw `%s/%s` is safe by construction. Adding URL encoding speculatively is YAGNI and changes the visual form. | If the URL needs encoding for some other reason, file a separate prompt with concrete failing input |

</failure_modes>

<acceptance_criteria>

- [ ] `watcher/github-build/pkg/watcher.go` `buildBodyHeader` H1 line uses `[owner/repo](https://github.com/owner/repo)` markdown-link form.
- [ ] `watcher/github-pr/pkg/watcher.go` `buildTaskBody` includes a `**Repo:** [owner/repo](https://github.com/owner/repo)` line after the PR URL.
- [ ] Existing wire-format negative-assertion tests (`NotTo(ContainSubstring("filename_hint"))`) pass unchanged.
- [ ] New tests verify each watcher's body contains the repo link.
- [ ] No new fields added to Kafka command schema (only markdown body content changes).
- [ ] `cd watcher/github-pr && make precommit` exits 0; coverage ≥80%.
- [ ] `cd watcher/github-build && make precommit` exits 0; coverage ≥80%.
- [ ] CHANGELOG has the new entry under `## Unreleased`.
- [ ] `git diff --stat` shows changes in: 2 watcher files (`watcher/github-build/pkg/watcher.go`, `watcher/github-pr/pkg/watcher.go`), at least 1 test file per watcher, and `CHANGELOG.md`. No other files touched.

</acceptance_criteria>

<verification>

```bash
cd watcher/github-pr && make precommit
cd watcher/github-build && make precommit

# Manually inspect a sample body to confirm the link form
grep -A2 "Build Failure:" watcher/github-build/pkg/watcher.go    # expect: H1 with markdown link
grep -A4 "PR Review:" watcher/github-pr/pkg/watcher.go            # expect: **Repo:** line
```

After deploying:
- Open a fresh PR on `bborbe/go-skeleton` (dev) or `bborbe/maintainer` (prod) → vault task body should contain `**Repo:** [bborbe/...](https://github.com/...)`.
- A future build failure on any allowlisted repo → task body H1 should be `# Build Failure: [bborbe/...](https://github.com/bborbe/...)`.

(Existing tasks in the vault keep their old plain-text H1s — only newly-published tasks get the new shape.)

</verification>

<do_nothing_option>

Without the repo link, every triage of a build-failure or PR-review task requires the operator to mentally construct the URL or copy-paste from the H1 plaintext into a browser. Small per-task friction (~5 seconds), but tasks land daily, and the cost compounds. The fix is a one-line H1 edit and a one-line body insertion — change-budget cost is essentially zero.

If we don't ship this, expect the same complaint to surface again the next time an operator triages a failure. The change is so small that "do nothing" doesn't carry meaningful tradeoff value.

</do_nothing_option>
