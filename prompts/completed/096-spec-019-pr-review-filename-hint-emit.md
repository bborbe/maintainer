---
status: completed
spec: [019-human-readable-filenames-for-pr-review-tasks]
summary: Added WatcherCreateTaskCommand wrapper type to github-pr watcher, implemented computePRFilenameHint/slugifyTitle helpers, updated publishCreate to emit FilenameHint on both trusted and untrusted paths, regenerated mock, added FilenameHint assertions to watcher_test.go, and created filename_internal_test.go with comprehensive slug and JSON marshal tests.
container: maintainer-096-spec-019-pr-review-filename-hint-emit
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T21:30:00Z"
queued: "2026-05-06T20:49:45Z"
started: "2026-05-06T20:49:53Z"
completed: "2026-05-06T20:58:22Z"
branch: dark-factory/human-readable-filenames-for-pr-review-tasks
---

<summary>
- PR-review vault tasks get human-readable filenames: `PR Review github - {owner}-{repo} - {number} - {slug}`
- A new `filename_hint` field is emitted in every `CreateTaskCommand` Kafka message published by the github-pr watcher
- The hint is computed deterministically from the hard-coded provider string (`github`), owner, repo, PR number, and a slugified PR title
- Slug rules: lowercase, replace any non-`[a-z0-9]` character with `-`, collapse consecutive hyphens to one, trim leading/trailing `-`, truncate at 50 chars (trim trailing `-` after truncation)
- If the PR title is empty or unicode-only the slug result is empty → the slug segment AND its leading ` - ` separator are omitted; filename ends with `... - {number}`
- `CommandPublisher.PublishCreate` is updated to accept a new `WatcherCreateTaskCommand` wrapper type that embeds `agentlib.CreateTaskCommand` and adds `FilenameHint string json:"filename_hint,omitempty"`
- `UpdateFrontmatterCommand` (force-push path) is NOT changed — only `CreateTaskCommand` carries the hint; filename is set once at create time
- Existing controllers silently ignore the new field via Go's `encoding/json` permissive default — fully backward-compatible
- The task `task_identifier: <UUID>` in frontmatter is unchanged; the controller's UUID-keyed lookup path works identically
</summary>

<objective>
Extend the github-pr watcher to compute and emit a `filename_hint` in every `CreateTaskCommand` Kafka message, so the task controller (in `bborbe/agent`, which will honor the field after spec-018's controller-side change lands) can name vault files `PR Review github - {owner}-{repo} - {number} - {slug}.md` instead of `<uuid>.md`. The watcher-side change is self-contained: define a `WatcherCreateTaskCommand` wrapper type, add slug and hint computation helpers, and populate `FilenameHint` in `publishCreate`. The controller honoring the hint ships via spec-018's controller follow-up in `bborbe/agent`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern, private helpers.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, Counterfeiter mocks, coverage ≥80%.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.

Files to read fully before making any changes:
- `watcher/github-pr/pkg/publisher.go` — full file; `CommandPublisher` interface, `kafkaPublisher.PublishCreate`, `marshalEvent`; this file gains the new `WatcherCreateTaskCommand` type and the updated interface
- `watcher/github-pr/pkg/watcher.go` — full file; `publishCreate` (lines 202–247) builds `agentlib.CreateTaskCommand` twice (trusted and untrusted paths) — both paths must use `WatcherCreateTaskCommand` with `FilenameHint` populated; `publishForcePush` must NOT change
- `watcher/github-pr/pkg/watcher_test.go` — full file; understand which `It` blocks call `pub.PublishCreateArgsForCall(0)` and inspect `cmd` — all existing assertions on `cmd.TaskIdentifier`, `cmd.Frontmatter`, `cmd.Body` still work after the type change because `WatcherCreateTaskCommand` embeds `agentlib.CreateTaskCommand`; add `FilenameHint` assertions to each of these blocks
- `watcher/github-pr/pkg/mocks/command_publisher.go` — understand the counterfeiter-generated mock structure; this file is regenerated after changing the interface

**Key facts — read before writing any code:**

1. `agentlib.CreateTaskCommand` at `github.com/bborbe/agent/lib@v0.57.0` has three fields only:
   ```go
   type CreateTaskCommand struct {
       TaskIdentifier TaskIdentifier  `json:"taskIdentifier"`
       Frontmatter    TaskFrontmatter `json:"frontmatter"`
       Body           string          `json:"body,omitempty"`
   }
   ```
   No `FilenameHint` field exists in any cached version. Do NOT bump the dep.

2. `WatcherCreateTaskCommand` embeds `agentlib.CreateTaskCommand` so all promoted fields
   (`TaskIdentifier`, `Frontmatter`, `Body`) continue to work at every call site without change.

3. `json.Marshal(WatcherCreateTaskCommand{...})` outputs all embedded fields at the top level
   plus `"filename_hint"` — the exact schema extension the spec requires.

4. `marshalEvent(ctx, v interface{})` takes `interface{}`, so passing `WatcherCreateTaskCommand`
   works without any other changes to the marshal path.

5. Verify the counterfeiter directive on `CommandPublisher` before running `go generate`:
   ```bash
   grep -n "counterfeiter:generate" watcher/github-pr/pkg/publisher.go
   ```
   The directive names `command_publisher.go` as the output file — use it to regenerate after
   changing the interface.

6. The `go.mod` for the PR watcher is at `watcher/github-pr/go.mod` — all commands run from
   `watcher/github-pr/`, not the repo root.

7. The `{owner}` and `{repo}` segments in the filename hint are taken as-is from `pr.Owner` and
   `pr.Repo` (no slugification needed today — bborbe's repo names are already filesystem-safe);
   they are joined with `-` to form the `{owner}-{repo}` component.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 7. Run `make precommit` only at the final step.**

1. **Add `WatcherCreateTaskCommand` to `watcher/github-pr/pkg/publisher.go`**

   After the existing `//counterfeiter:generate` directive (before the `CommandPublisher` interface),
   insert the new type:

   ```go
   // WatcherCreateTaskCommand extends CreateTaskCommand with an optional filename hint.
   // FilenameHint lets the task controller name the vault file with a human-readable stem
   // (e.g. "PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never") instead of a UUID.
   // MUST NOT include the .md extension — the controller appends it.
   // Absent controllers (encoding/json default) silently ignore the new field.
   type WatcherCreateTaskCommand struct {
   	agentlib.CreateTaskCommand
   	FilenameHint string `json:"filename_hint,omitempty"`
   }
   ```

2. **Update `CommandPublisher` interface in `watcher/github-pr/pkg/publisher.go`**

   Change `PublishCreate` from accepting `agentlib.CreateTaskCommand` to `WatcherCreateTaskCommand`:

   ```go
   // CommandPublisher publishes task commands to Kafka.
   type CommandPublisher interface {
   	PublishCreate(ctx context.Context, cmd WatcherCreateTaskCommand) error
   	PublishUpdateFrontmatter(ctx context.Context, cmd agentlib.UpdateFrontmatterCommand) error
   }
   ```

3. **Update `kafkaPublisher.PublishCreate` in `watcher/github-pr/pkg/publisher.go`**

   Change the parameter type only — no other changes needed (`marshalEvent` takes `interface{}`):

   ```go
   func (p *kafkaPublisher) PublishCreate(ctx context.Context, cmd WatcherCreateTaskCommand) error {
   	event, err := marshalEvent(ctx, cmd)
   	if err != nil {
   		return errors.Wrap(ctx, err, "marshal create-task command")
   	}
   	commandObject := p.buildCommandObject(agentlib.CreateTaskCommandOperation, event)
   	if err := p.sender.SendCommandObject(ctx, commandObject); err != nil {
   		return errors.Wrap(ctx, err, "publish create-task")
   	}
   	return nil
   }
   ```

4. **Create `watcher/github-pr/pkg/filename.go`** with hint computation helpers:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import (
   	"fmt"
   	"strings"

   	"github.com/golang/glog"
   )

   // maxFilenameHintLen is the maximum byte length of a filename_hint value.
   // Hints that exceed this limit are truncated with a WARN log.
   const maxFilenameHintLen = 200

   // maxSlugLen is the maximum character length of the slugified PR-title segment.
   const maxSlugLen = 50

   // computePRFilenameHint returns the human-readable filename hint for a PR-review task.
   // Format (with slug): "PR Review {provider} - {owner}-{repo} - {number} - {slug}"
   // Format (empty slug): "PR Review {provider} - {owner}-{repo} - {number}"
   // The returned string MUST NOT include the .md extension; the controller appends it.
   func computePRFilenameHint(provider, owner, repo string, number int, title string) string {
   	base := fmt.Sprintf("PR Review %s - %s-%s - %d", provider, owner, repo, number)
   	slug := slugifyTitle(title)
   	var hint string
   	if slug == "" {
   		hint = base
   	} else {
   		hint = base + " - " + slug
   	}
   	if len(hint) > maxFilenameHintLen {
   		glog.Warningf("filename_hint exceeds max length: len=%d max=%d — truncating", len(hint), maxFilenameHintLen)
   		hint = hint[:maxFilenameHintLen]
   	}
   	return hint
   }

   // slugifyTitle converts a PR title to a filesystem-safe, human-readable slug.
   // Rules (applied in order):
   // 1. Lowercase the entire input
   // 2. Replace any character that is not [a-z0-9] with a hyphen
   // 3. Collapse consecutive hyphens into a single hyphen
   // 4. Trim leading and trailing hyphens
   // 5. Truncate to maxSlugLen (50) characters; trim any trailing hyphen left by truncation
   // Returns empty string if the result after step 4 is empty (e.g. unicode-only or whitespace-only title).
   func slugifyTitle(title string) string {
   	lower := strings.ToLower(title)
   	var b strings.Builder
   	prevHyphen := false
   	for _, r := range lower {
   		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
   			b.WriteRune(r)
   			prevHyphen = false
   		} else if !prevHyphen {
   			b.WriteRune('-')
   			prevHyphen = true
   		}
   	}
   	result := strings.Trim(b.String(), "-")
   	if len(result) > maxSlugLen {
   		result = result[:maxSlugLen]
   		result = strings.TrimRight(result, "-")
   	}
   	return result
   }
   ```

5. **Update `publishCreate` in `watcher/github-pr/pkg/watcher.go`** to use `WatcherCreateTaskCommand`.

   In the `publishCreate` method (lines ~202–247), both the trusted and untrusted paths build an
   `agentlib.CreateTaskCommand`. Change both to build a `WatcherCreateTaskCommand` with
   `FilenameHint` populated.

   Replace the two `agentlib.CreateTaskCommand{...}` literals with `WatcherCreateTaskCommand`:

   ```go
   // Trusted path:
   cmd = WatcherCreateTaskCommand{
   	CreateTaskCommand: agentlib.CreateTaskCommand{
   		TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
   		Frontmatter:    buildFrontmatter(pr, taskIDStr, w.stage, details),
   		Body:           buildTaskBody(pr),
   	},
   	FilenameHint: computePRFilenameHint("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
   }

   // Untrusted path:
   cmd = WatcherCreateTaskCommand{
   	CreateTaskCommand: agentlib.CreateTaskCommand{
   		TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
   		Frontmatter:    buildHumanReviewFrontmatter(pr, taskIDStr, w.stage, details),
   		Body:           buildUntrustedBody(author, trustResult.Description()),
   	},
   	FilenameHint: computePRFilenameHint("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
   }
   ```

   Update the variable declaration at the top of `publishCreate` to use the new type:
   ```go
   var cmd WatcherCreateTaskCommand
   ```

   The `publishForcePush` method builds `agentlib.UpdateFrontmatterCommand` — leave it completely
   unchanged.

6. **Regenerate `watcher/github-pr/pkg/mocks/command_publisher.go`**

   Run go generate first:
   ```bash
   cd watcher/github-pr && go generate ./pkg/...
   ```

   If the directive is absent or doesn't trigger, run counterfeiter directly:
   ```bash
   cd watcher/github-pr && \
     go run github.com/maxbrunsfeld/counterfeiter/v6 \
       -o pkg/mocks/command_publisher.go \
       --fake-name CommandPublisher \
       ./pkg/. CommandPublisher
   ```

   Confirm the regenerated mock's `PublishCreateStub` takes `pkg.WatcherCreateTaskCommand`,
   NOT `lib.CreateTaskCommand`.

7. **Run `make test`** to confirm compilation and all existing tests pass:
   ```bash
   cd watcher/github-pr && make test
   ```
   Fix any compile errors before proceeding.

8. **Update `watcher/github-pr/pkg/watcher_test.go`** — add `FilenameHint` assertions.

   In each `It` block that calls `pub.PublishCreateArgsForCall(0)` and inspects `cmd`, add a
   `FilenameHint` assertion after the last existing `cmd` assertion.

   The PR watcher uses `pr.Owner`, `pr.Repo`, `pr.Number`, and `pr.Title`. Match the hint to the
   values in each test case:

   a. **`"publishes CreateTaskCommand"` (Owner: "bborbe", Repo: "code-reviewer", Number: 42, Title: "feat: new feature")**:
   ```go
   Expect(cmd.FilenameHint).To(Equal("PR Review github - bborbe-code-reviewer - 42 - feat-new-feature"))
   ```

   b. **`"includes required keys"` (Owner: "bborbe", Repo: "repo", Number: 5, Title: "my title")**:
   ```go
   Expect(cmd.FilenameHint).To(Equal("PR Review github - bborbe-repo - 5 - my-title"))
   ```

   c. **`"publishes CreateTaskCommand with planning/in_progress frontmatter"` (Owner: "bborbe", Repo: "repo", Number: 10, Title: "some PR")**:
   ```go
   Expect(cmd.FilenameHint).To(Equal("PR Review github - bborbe-repo - 10 - some-pr"))
   ```

   d. **`"publishes CreateTaskCommand with human_review/todo frontmatter and untrusted body"` (Owner: "bborbe", Repo: "repo", Number: 10, Title: "some PR")**:
   ```go
   Expect(cmd.FilenameHint).To(Equal("PR Review github - bborbe-repo - 10 - some-pr"))
   ```

   e. **`"treats as untrusted and publishes human_review task"` (Owner: "bborbe", Repo: "repo", Number: 10, Title: "some PR" — same PR struct with empty AuthorLogin)**:
   ```go
   Expect(cmd.FilenameHint).To(Equal("PR Review github - bborbe-repo - 10 - some-pr"))
   ```

   f. **Other `It` blocks that call `pub.PublishCreateArgsForCall`** — check for any additional
   blocks (e.g. the Kafka-fail retry test at line ~361 uses `pub2.PublishCreateCallCount()` but
   does not inspect `cmd` — no assertion needed there).

   Note: existing assertions on `cmd.TaskIdentifier`, `cmd.Frontmatter["assignee"]`, `cmd.Body`,
   etc. are unchanged because `WatcherCreateTaskCommand` promotes all embedded fields.

9. **Create `watcher/github-pr/pkg/filename_internal_test.go`** — unit tests for slug functions.

   Use package `pkg` (internal test) to access unexported `computePRFilenameHint` and `slugifyTitle`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import (
   	. "github.com/onsi/ginkgo/v2"
   	. "github.com/onsi/gomega"
   )

   var _ = Describe("slugifyTitle", func() {
   	DescribeTable("produces correct slug",
   		func(input, want string) {
   			Expect(slugifyTitle(input)).To(Equal(want))
   		},
   		Entry("simple lowercase", "fix bug", "fix-bug"),
   		Entry("uppercase converted", "Fix Bug", "fix-bug"),
   		Entry("special chars replaced with hyphen", "feat: new-feature!", "feat-new-feature"),
   		Entry("consecutive special chars collapsed to one hyphen", "hello   world", "hello-world"),
   		Entry("leading special char stripped", "!leading", "leading"),
   		Entry("trailing special char stripped", "trailing!", "trailing"),
   		Entry("only special chars → empty string", "!!!", ""),
   		Entry("empty string → empty string", "", ""),
   		Entry("unicode-only → empty string", "🚀🎉", ""),
   		Entry("mixed unicode and ascii", "fix 🐛 bug", "fix-bug"),
   		Entry("digits preserved", "v1 release", "v1-release"),
   		Entry("already slug-safe", "my-feature", "my-feature"),
   		Entry("truncation at 50 chars", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm"),
   		Entry("truncation trims trailing hyphen", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijk-extra-words-here", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijk"),
   		Entry("pr title with colon", "feat: add new endpoint", "feat-add-new-endpoint"),
   		Entry("pr title with slash", "fix/auth bug", "fix-auth-bug"),
   		Entry("pr title with dots", "bump v1.2.3", "bump-v1-2-3"),
   	)
   })

   var _ = Describe("computePRFilenameHint", func() {
   	DescribeTable("produces correct filename hint",
   		func(provider, owner, repo string, number int, title, want string) {
   			Expect(computePRFilenameHint(provider, owner, repo, number, title)).To(Equal(want))
   		},
   		Entry("normal PR with title",
   			"github", "bborbe", "maintainer", 2, "test: delete this PR never",
   			"PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never"),
   		Entry("title with special chars",
   			"github", "bborbe", "trading", 110, "fix: chromium trixie",
   			"PR Review github - bborbe-trading - 110 - fix-chromium-trixie"),
   		Entry("empty title → no slug segment",
   			"github", "bborbe", "x", 7, "",
   			"PR Review github - bborbe-x - 7"),
   		Entry("whitespace-only title → no slug segment",
   			"github", "bborbe", "x", 7, "   ",
   			"PR Review github - bborbe-x - 7"),
   		Entry("unicode-only title → no slug segment",
   			"github", "bborbe", "x", 7, "🚀🎉",
   			"PR Review github - bborbe-x - 7"),
   		Entry("slug truncated at 50 chars",
   			"github", "org", "repo", 1,
   			"this is a very long pull request title that exceeds the maximum slug length limit here",
   			"PR Review github - org-repo - 1 - this-is-a-very-long-pull-request-title-that-exceeds"),
   		Entry("future bitbucket provider",
   			"bitbucket", "team", "svc", 42, "fix auth bug",
   			"PR Review bitbucket - team-svc - 42 - fix-auth-bug"),
   		Entry("hyphenated repo name joined correctly",
   			"github", "my-org", "my-repo", 99, "bump deps",
   			"PR Review github - my-org-my-repo - 99 - bump-deps"),
   	)
   })

   // JSON marshal contract: lock the wire-format tag for the controller boundary.
   // Even though spec-018 already verified this shape for the build watcher's
   // wrapper, this watcher's wrapper is a separate declaration — re-verify locally
   // so a future struct-tag drift doesn't leak past PR-review reviews.
   var _ = Describe("WatcherCreateTaskCommand JSON marshalling", func() {
   	It("emits filename_hint as a top-level snake_case field alongside embedded fields", func() {
   		cmd := WatcherCreateTaskCommand{
   			CreateTaskCommand: agentlib.CreateTaskCommand{
   				TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
   				Frontmatter:    agentlib.TaskFrontmatter{"assignee": "pr-reviewer-agent"},
   				Body:           "# body",
   			},
   			FilenameHint: "PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never",
   		}
   		raw, err := json.Marshal(cmd)
   		Expect(err).NotTo(HaveOccurred())

   		// Top-level snake_case key consumed by the controller
   		Expect(string(raw)).To(ContainSubstring(`"filename_hint":"PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never"`))

   		// Embedded fields stay at top level (struct embedding, not nested)
   		Expect(string(raw)).To(ContainSubstring(`"taskIdentifier":"00000000-0000-0000-0000-000000000000"`))

   		// No nesting under a "CreateTaskCommand" key
   		Expect(string(raw)).NotTo(ContainSubstring(`"CreateTaskCommand"`))
   	})

   	It("omits filename_hint when empty (omitempty contract)", func() {
   		cmd := WatcherCreateTaskCommand{
   			CreateTaskCommand: agentlib.CreateTaskCommand{
   				TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
   				Frontmatter:    agentlib.TaskFrontmatter{},
   				Body:           "",
   			},
   			FilenameHint: "",
   		}
   		raw, err := json.Marshal(cmd)
   		Expect(err).NotTo(HaveOccurred())
   		Expect(string(raw)).NotTo(ContainSubstring(`"filename_hint"`))
   	})
   })
   ```

   Add `"encoding/json"` and `agentlib "github.com/bborbe/agent/lib"` to the import block of `filename_internal_test.go` if not already present.

   The internal test file uses the same Ginkgo test runner registered in `watcher/github-pr/pkg/suite_test.go` — no new suite file needed.

10. **Add CHANGELOG entry** in root `CHANGELOG.md` under `## Unreleased`:

    ```
    - feat(watcher/github-pr): emit filename_hint in CreateTaskCommand — PR-review vault tasks will land at "PR Review github - {owner}-{repo} - {number} - {slug}.md" once the companion bborbe/agent controller PR lands; existing controllers silently ignore the new field
    ```

11. **Run `make precommit`** from `watcher/github-pr/`:
    ```bash
    cd watcher/github-pr && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-pr/pkg/` (including `mocks/command_publisher.go`) and root `CHANGELOG.md`; create `watcher/github-pr/pkg/filename.go` and `watcher/github-pr/pkg/filename_internal_test.go`
- Do NOT commit — dark-factory handles git
- **Dependency on spec-018 watcher-side prompt**: do NOT approve this prompt until `prompts/spec-018-filename-hint-emit.md` has been approved and executed — both prompts edit `agentlib`/`cqrs`-adjacent types and must not run concurrently. If `watcher/github-build/pkg/filename.go` does NOT exist, STOP and report `status: failed` with reason "spec-018 watcher-side prompt not yet executed"
- `WatcherCreateTaskCommand` MUST embed `agentlib.CreateTaskCommand` (not duplicate its fields) so existing callers access `cmd.TaskIdentifier`, `cmd.Frontmatter`, `cmd.Body` via Go field promotion without change
- `filename_hint` JSON key MUST be lowercase snake_case — NOT camelCase (`filenameHint`)
- The hint MUST NOT include the `.md` extension — the controller appends it
- `FilenameHint` MUST be populated for BOTH the trusted path and the untrusted (human_review) path in `publishCreate` — both produce `CreateTaskCommand`; both get a hint
- `publishForcePush` MUST NOT be changed — it produces `UpdateFrontmatterCommand`, which carries no hint by design
- Slug truncation: hard-truncate at 50 characters, then call `strings.TrimRight(result, "-")` once — no ellipsis, no hash markers
- Slug: collapse consecutive non-`[a-z0-9]` characters to a single `-` during building (do NOT use regexp; use a `prevHyphen bool` flag in the loop)
- Empty slug (empty/unicode-only title) → OMIT slug segment AND its leading ` - ` separator; the hint ends with `... - {number}` NOT `... - {number} - `
- `computePRFilenameHint` and `slugifyTitle` MUST be unexported — they are implementation details of the pkg package
- The `CommandPublisher` interface MUST accept `WatcherCreateTaskCommand`; the mock MUST be regenerated to reflect the new signature
- All existing `watcher_test.go` tests must still pass — field promotion on the embedded struct makes all existing `cmd.Frontmatter`, `cmd.TaskIdentifier`, `cmd.Body` assertions work unchanged
- The provider string is the hard-coded literal `"github"` in all `computePRFilenameHint` call sites — NOT derived from config, env, or the allowlist entry
- `{owner}` and `{repo}` are taken as-is from `pr.Owner` and `pr.Repo` — no slugification applied to owner/repo today (bborbe repos are filesystem-safe)
- Do NOT change `main.go`, `factory.go`, or any file outside `pkg/` except CHANGELOG
- `make precommit` runs from `watcher/github-pr/`, never at repo root
- Error wrapping: `github.com/bborbe/errors`; never `fmt.Errorf`
- Coverage ≥80% for changed packages
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm WatcherCreateTaskCommand type defined with correct json tag:
grep -n "WatcherCreateTaskCommand\|FilenameHint\|filename_hint" watcher/github-pr/pkg/publisher.go
# Expected: type declaration + json:"filename_hint,omitempty" tag

# Confirm CommandPublisher interface accepts WatcherCreateTaskCommand:
grep -A 4 "type CommandPublisher interface" watcher/github-pr/pkg/publisher.go
# Expected: PublishCreate takes WatcherCreateTaskCommand

# Confirm kafkaPublisher.PublishCreate parameter type updated:
grep -A 3 "func.*kafkaPublisher.*PublishCreate" watcher/github-pr/pkg/publisher.go
# Expected: cmd WatcherCreateTaskCommand

# Confirm pkg/filename.go exists with all three symbols:
ls watcher/github-pr/pkg/filename.go
grep -n "func computePRFilenameHint\|func slugifyTitle\|maxFilenameHintLen\|maxSlugLen" watcher/github-pr/pkg/filename.go
# Expected: all four present

# Confirm publishCreate uses WatcherCreateTaskCommand for both trusted and untrusted paths:
grep -n "WatcherCreateTaskCommand\|FilenameHint\|computePRFilenameHint" watcher/github-pr/pkg/watcher.go
# Expected: at least 3 matches (var declaration + 2 FilenameHint field assignments)

# Confirm publishForcePush unchanged (still builds UpdateFrontmatterCommand):
grep -n "UpdateFrontmatterCommand" watcher/github-pr/pkg/watcher.go
# Expected: still present in publishForcePush, no FilenameHint there

# Confirm mock regenerated with WatcherCreateTaskCommand:
grep -n "WatcherCreateTaskCommand" watcher/github-pr/pkg/mocks/command_publisher.go
# Expected: at least 3 matches (stub, argsForCall struct, method signature)

# Confirm old lib.CreateTaskCommand type absent from mock:
grep -n "lib\.CreateTaskCommand" watcher/github-pr/pkg/mocks/command_publisher.go
# Expected: zero matches

# Confirm FilenameHint assertions added to watcher_test.go:
grep -n "FilenameHint" watcher/github-pr/pkg/watcher_test.go
# Expected: at least 4 assertions

# Confirm internal unit tests exist:
ls watcher/github-pr/pkg/filename_internal_test.go
grep -n "slugifyTitle\|computePRFilenameHint" watcher/github-pr/pkg/filename_internal_test.go
# Expected: DescribeTable blocks for both functions

# Confirm empty-slug path produces no trailing " - ":
grep -n "unicode-only\|empty title\|bborbe-x - 7" watcher/github-pr/pkg/filename_internal_test.go
# Expected: entries that assert hint ends with "- 7" (no trailing " - ")

# Confirm provider is hard-coded as "github":
grep -n '"github"' watcher/github-pr/pkg/watcher.go | grep computePRFilenameHint
# Expected: computePRFilenameHint("github", ...

# Confirm CHANGELOG entry:
grep -n "filename_hint\|human-readable\|PR Review" CHANGELOG.md | head -5
# Expected: one match under ## Unreleased
</verification>
