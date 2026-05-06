---
status: committing
spec: [018-human-readable-filenames-for-build-tasks]
summary: Added WatcherCreateTaskCommand wrapper type, computeFilenameHint/slugifySegment helpers, updated CommandPublisher interface and impl, regenerated mock, added FilenameHint assertions to watcher_test.go, added unit tests for computeFilenameHint/slugifySegment/JSON marshalling to watcher_internal_test.go, updated docs/build-watcher.md and docs/architecture.md, and added CHANGELOG entry.
container: maintainer-095-spec-018-filename-hint-emit
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T21:00:00Z"
queued: "2026-05-06T20:30:06Z"
started: "2026-05-06T20:30:08Z"
branch: dark-factory/human-readable-filenames-for-build-tasks
---

<summary>
- Build-failure vault tasks get human-readable filenames: `Build Failure github - {owner}-{repo} - {sha7}`
- A new `filename_hint` field is emitted in every `CreateTaskCommand` Kafka message published by the github-build watcher
- The hint is computed deterministically from the hard-coded provider string (`github`), owner, repo, and the first 7 chars of the episode SHA
- Owner and repo segments are slugified independently (lowercase, non-`[a-z0-9-]` → `-`), then joined with `-`
- Hints longer than 200 characters are truncated with a WARN log (defensive guard; current repo names will not approach this limit)
- The `task_identifier: <UUID>` in frontmatter is unchanged; the controller's `FindTaskFilePath` (UUID-keyed lookup) works identically
- `CommandPublisher.PublishCreate` is updated to accept a new `WatcherCreateTaskCommand` wrapper type that embeds `agentlib.CreateTaskCommand` and adds `FilenameHint string`
- Existing controllers silently ignore the new field via Go's `encoding/json` permissive default — fully backward-compatible
- `docs/build-watcher.md` and `docs/architecture.md` updated to document the new field
</summary>

<objective>
Extend the github-build watcher to compute and emit a `filename_hint` in every `CreateTaskCommand` Kafka message, so the task controller (in `bborbe/agent`, a separate future spec) can name vault files `Build Failure github - {owner}-{repo} - {sha7}.md` instead of `<uuid>.md`. The watcher-side change is self-contained: define a `WatcherCreateTaskCommand` wrapper type, compute the hint in `buildCreateTaskCommand`, and emit it. The controller honoring the hint ships later in a separate `bborbe/agent` spec.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern, private helpers.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, Counterfeiter mocks, coverage ≥80%.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.

Files to read fully before making any changes:
- `watcher/github-build/pkg/publisher.go` — full file; `CommandPublisher` interface, `kafkaPublisher.PublishCreate`, `marshalEvent`; this file gains the new `WatcherCreateTaskCommand` type and the updated interface
- `watcher/github-build/pkg/watcher.go` — full file; `buildCreateTaskCommand` method at the bottom returns `agentlib.CreateTaskCommand` — change return type to `WatcherCreateTaskCommand` and populate `FilenameHint`
- `watcher/github-build/pkg/watcher_test.go` — full file; all existing assertions on `cmd.TaskIdentifier`, `cmd.Frontmatter` still work after the type change because `WatcherCreateTaskCommand` embeds `agentlib.CreateTaskCommand`; add `FilenameHint` assertions
- `watcher/github-build/pkg/watcher_internal_test.go` — full file; follow the `splitRepoKey` `DescribeTable` pattern for new unit tests
- `docs/build-watcher.md` — full file; understand existing sections before appending the new `filename_hint` section
- `docs/architecture.md` — read the "Watcher → Controller (Kafka)" JSON schema (around line 178); add `filename_hint` there

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
   grep -n "counterfeiter:generate" watcher/github-build/pkg/publisher.go
   ```
   The directive names `command_publisher.go` as the output file — use it to regenerate after
   changing the interface.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 7. Run `make precommit` only at the final step.**

1. **Add `WatcherCreateTaskCommand` to `watcher/github-build/pkg/publisher.go`**

   After the existing `//counterfeiter:generate` directive (before the `CommandPublisher` interface),
   insert the new type:

   ```go
   // WatcherCreateTaskCommand extends CreateTaskCommand with an optional filename hint.
   // FilenameHint lets the task controller name the vault file with a human-readable stem
   // (e.g. "Build Failure github - bborbe-maintainer - 5886450") instead of a UUID.
   // MUST NOT include the .md extension — the controller appends it.
   // Absent controllers (encoding/json default) silently ignore the new field.
   type WatcherCreateTaskCommand struct {
   	agentlib.CreateTaskCommand
   	FilenameHint string `json:"filename_hint,omitempty"`
   }
   ```

2. **Update `CommandPublisher` interface in `watcher/github-build/pkg/publisher.go`**

   Change `PublishCreate` from `agentlib.CreateTaskCommand` to `WatcherCreateTaskCommand`:

   ```go
   // CommandPublisher publishes task commands to Kafka.
   type CommandPublisher interface {
   	PublishCreate(ctx context.Context, cmd WatcherCreateTaskCommand) error
   }
   ```

3. **Update `kafkaPublisher.PublishCreate` in `watcher/github-build/pkg/publisher.go`**

   Change the parameter type only — no other changes needed (`marshalEvent` takes `interface{}`):

   ```go
   func (p *kafkaPublisher) PublishCreate(ctx context.Context, cmd WatcherCreateTaskCommand) error {
   	event, err := marshalEvent(ctx, cmd)
   	...
   ```

4. **Create `watcher/github-build/pkg/filename.go`** with hint computation helpers:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import (
   	"strings"

   	"github.com/golang/glog"
   )

   // maxFilenameHintLen is the maximum byte length of a filename_hint value.
   // Hints that exceed this limit are truncated with a WARN log to prevent filesystem aliasing.
   const maxFilenameHintLen = 200

   // computeFilenameHint returns the human-readable filename hint for a build-failure task.
   // Format: "Build Failure {provider} - {slugifySegment(owner)}-{slugifySegment(repo)} - {sha7}"
   // The returned string MUST NOT include the .md extension; the controller appends it.
   func computeFilenameHint(provider, owner, repo, episodeSHA string) string {
   	sha7 := episodeSHA
   	if len(sha7) > 7 {
   		sha7 = sha7[:7]
   	}
   	ownerRepo := slugifySegment(owner) + "-" + slugifySegment(repo)
   	hint := "Build Failure " + provider + " - " + ownerRepo + " - " + sha7
   	if len(hint) > maxFilenameHintLen {
   		glog.Warningf("filename_hint exceeds max length: len=%d max=%d — truncating", len(hint), maxFilenameHintLen)
   		hint = hint[:maxFilenameHintLen]
   	}
   	return hint
   }

   // slugifySegment converts s to a filesystem-safe lowercase segment.
   // Non-[a-z0-9] characters (including uppercase letters) are replaced with hyphens;
   // leading and trailing hyphens are stripped.
   func slugifySegment(s string) string {
   	var b strings.Builder
   	for _, r := range strings.ToLower(s) {
   		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
   			b.WriteRune(r)
   		} else {
   			b.WriteRune('-')
   		}
   	}
   	return strings.Trim(b.String(), "-")
   }
   ```

5. **Update `buildCreateTaskCommand` in `watcher/github-build/pkg/watcher.go`**

   a. Change the return type from `agentlib.CreateTaskCommand` to `WatcherCreateTaskCommand`:
   ```go
   func (w *buildWatcher) buildCreateTaskCommand(
   	taskID uuid.UUID,
   	owner, repo, episodeSHA string,
   	failingRuns []WorkflowRun,
   	assignee, taskStatus, taskPhase string,
   ) WatcherCreateTaskCommand {
   ```

   b. Replace the `return agentlib.CreateTaskCommand{...}` at the bottom of the method with:
   ```go
   return WatcherCreateTaskCommand{
   	CreateTaskCommand: agentlib.CreateTaskCommand{
   		TaskIdentifier: agentlib.TaskIdentifier(taskID.String()),
   		Frontmatter:    fm,
   		Body:           body,
   	},
   	FilenameHint: computeFilenameHint("github", owner, repo, episodeSHA),
   }
   ```

   No other changes to the method body are needed.

6. **Regenerate `watcher/github-build/pkg/mocks/command_publisher.go`**

   Run go generate first:
   ```bash
   cd watcher/github-build && go generate ./pkg/...
   ```

   If the directive is absent or doesn't trigger, run counterfeiter directly:
   ```bash
   cd watcher/github-build && \
     go run github.com/maxbrunsfeld/counterfeiter/v6 \
       -o pkg/mocks/command_publisher.go \
       --fake-name CommandPublisher \
       ./pkg/. CommandPublisher
   ```

   Confirm the regenerated mock's `PublishCreateStub` takes `pkg.WatcherCreateTaskCommand`,
   NOT `lib.CreateTaskCommand`.

7. **Run `make test`** to confirm compilation and all existing tests pass:
   ```bash
   cd watcher/github-build && make test
   ```
   Fix any compile errors before proceeding.

8. **Update `watcher/github-build/pkg/watcher_test.go`** — add `FilenameHint` assertions.

   In each of the following existing `It` blocks, after the last existing assertion on `cmd`,
   add a `FilenameHint` check:

   a. `"cold start (empty cursor) + repo currently red"` — SHA is `"sha-abc"` (7 chars, used as-is):
   ```go
   Expect(cmd.FilenameHint).To(Equal("Build Failure github - owner-repo - sha-abc"))
   ```

   b. `"green → red"` (the second poll publishes with SHA `"sha-b"` which is 5 chars):
   ```go
   _, cmd := publisher.PublishCreateArgsForCall(0)
   Expect(cmd.Frontmatter["episode_sha"]).To(Equal("sha-b"))
   Expect(cmd.Frontmatter["repo"]).To(Equal("owner/repo"))
   Expect(cmd.FilenameHint).To(Equal("Build Failure github - owner-repo - sha-b"))
   ```

   c. `"host-prefixed allowlist entry (github.com/owner/repo)"` — SHA is `"sha-abc"`,
      owner/repo are extracted as `owner`, `repo` after host drop:
   ```go
   Expect(cmd.FilenameHint).To(Equal("Build Failure github - owner-repo - sha-abc"))
   ```

   Note: the SHA portion is NOT slugified. `sha-abc` contains a hyphen but is used as-is because
   it is already filesystem-safe and hyphens are valid in filenames.

9. **Add unit tests in `watcher/github-build/pkg/watcher_internal_test.go`**

   Append after the existing `splitRepoKey` `DescribeTable`, following the same pattern:

   ```go
   var _ = Describe("computeFilenameHint", func() {
   	DescribeTable("produces correct filename hint",
   		func(provider, owner, repo, sha, want string) {
   			Expect(computeFilenameHint(provider, owner, repo, sha)).To(Equal(want))
   		},
   		Entry("normal github repo", "github", "bborbe", "maintainer", "5886450abcdef", "Build Failure github - bborbe-maintainer - 5886450"),
   		Entry("sha shorter than 7 chars", "github", "org", "repo", "abc12", "Build Failure github - org-repo - abc12"),
   		Entry("sha exactly 7 chars", "github", "org", "repo", "abc1234", "Build Failure github - org-repo - abc1234"),
   		Entry("sha longer than 7 chars is truncated to 7", "github", "org", "repo", "abc1234xyz", "Build Failure github - org-repo - abc1234"),
   		Entry("repo name with uppercase is slugified", "github", "MyOrg", "MyRepo", "abcdef0", "Build Failure github - myorg-myrepo - abcdef0"),
   		Entry("repo name with dot is slugified", "github", "org", "my.repo", "abcdef0", "Build Failure github - org-my-repo - abcdef0"),
   		Entry("repo name with colon (illegal on vault fs) is slugified", "github", "org", "my:repo", "abcdef0", "Build Failure github - org-my-repo - abcdef0"),
   		Entry("hyphenated names preserved", "github", "my-org", "my-repo", "abcdef0", "Build Failure github - my-org-my-repo - abcdef0"),
   		Entry("future bitbucket provider", "bitbucket", "team", "svc", "a1b2c3d", "Build Failure bitbucket - team-svc - a1b2c3d"),
   	)
   })

   var _ = Describe("slugifySegment", func() {
   	DescribeTable("produces filesystem-safe segment",
   		func(input, want string) {
   			Expect(slugifySegment(input)).To(Equal(want))
   		},
   		Entry("already safe lowercase", "bborbe", "bborbe"),
   		Entry("uppercase converted", "MyOrg", "myorg"),
   		Entry("dot replaced with hyphen", "my.repo", "my-repo"),
   		Entry("colon replaced with hyphen", "my:repo", "my-repo"),
   		Entry("asterisk replaced with hyphen", "my*repo", "my-repo"),
   		Entry("leading special char stripped", ".leading", "leading"),
   		Entry("trailing special char stripped", "trailing.", "trailing"),
   		Entry("only special chars becomes empty", ":::", ""),
   		Entry("mixed chars", "My.Org_1", "my-org-1"),
   		Entry("digits preserved", "repo2", "repo2"),
   		Entry("hyphen preserved", "my-repo", "my-repo"),
   	)
   })

   // JSON marshal contract: lock the wire-format tag for the controller boundary.
   var _ = Describe("WatcherCreateTaskCommand JSON marshalling", func() {
   	It("emits filename_hint as a top-level snake_case field alongside embedded fields", func() {
   		cmd := WatcherCreateTaskCommand{
   			CreateTaskCommand: agentlib.CreateTaskCommand{
   				TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
   				Frontmatter:    agentlib.TaskFrontmatter{"assignee": "bborbe"},
   				Body:           "# body",
   			},
   			FilenameHint: "Build Failure github - bborbe-maintainer - 5886450",
   		}
   		raw, err := json.Marshal(cmd)
   		Expect(err).NotTo(HaveOccurred())

   		// Top-level snake_case key for the controller's consumer
   		Expect(string(raw)).To(ContainSubstring(`"filename_hint":"Build Failure github - bborbe-maintainer - 5886450"`))

   		// Embedded fields stay at top level (struct embedding, not nested under a key)
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

10. **Update `docs/build-watcher.md`** — append a new section after the existing
    `## Known Deviations from Spec 015` section and before `## Per-Repo Configuration`:

    ````markdown
    ## `filename_hint` Field

    Every `CreateTaskCommand` published by the build watcher includes a `filename_hint` field
    with the human-readable filename stem for the vault task file:

    ```
    Build Failure {provider} - {owner}-{repo} - {sha7}
    ```

    | Component | Source | Notes |
    |---|---|---|
    | `Build Failure` | constant | literal |
    | `{provider}` | hard-coded `github` in this watcher | future watchers carry their own constant |
    | `{owner}-{repo}` | `owner` and `repo` from allowlist entry, slugified independently, joined with `-` | lowercase; non-`[a-z0-9-]` → `-`; leading/trailing hyphens stripped |
    | `{sha7}` | first 7 chars of `episode_sha` | matches git's default short-hash length; not slugified |

    **Example:** `Build Failure github - bborbe-maintainer - 5886450`

    **Future provider slots:** `Build Failure bitbucket - team-svc - a1b2c3d.md`

    **Controller behavior (future):** The task controller (`bborbe/agent`) will name the vault file
    `tasks/{filename_hint}.md` when the hint is present and valid. If absent or invalid, the controller
    falls back to `tasks/{uuid}.md`. Controller-side validation and fallback logic ships in a separate
    `bborbe/agent` spec. Until that spec lands, the `filename_hint` field is emitted but ignored.

    **Schema compatibility:** The field uses `json:"filename_hint,omitempty"`. Controllers that do not
    recognize `filename_hint` process the message correctly via Go's `encoding/json` permissive default.
    ````

11. **Update `docs/architecture.md`** — in the "Watcher → Controller (Kafka)" section, add
    `filename_hint` to the JSON schema example and a note below it.

    Find the JSON block that starts with `{"type": "CreateTaskCommand"` (around line 178).
    Add `"filename_hint"` as an optional field before `"source"`:

    ```json
    {
      "type": "CreateTaskCommand",
      "assignee": "pr-reviewer-agent",
      "stage": "dev|prod",
      "task_id": "<uuid>",
      "clone_url": "https://github.com/bborbe/maintainer.git",
      "ref": "<head_sha>",
      "base_ref": "master",
      "filename_hint": "Build Failure github - bborbe-maintainer - 5886450",
      "source": {
        "provider": "github",
        "pr_url": "https://github.com/bborbe/maintainer/pull/42",
        "pr_number": 42
      }
    }
    ```

    After the closing ` ``` ` of the JSON block, insert a note:
    ```
    `filename_hint` (optional) — human-readable filename stem for the vault task file;
    absent in messages from older watchers; controller falls back to UUID-based name.
    ```

12. **Add CHANGELOG entry** in root `CHANGELOG.md` under `## Unreleased`:

    ```
    - feat(watcher/github-build): emit filename_hint in CreateTaskCommand — build-failure vault tasks will land at "Build Failure github - {owner}-{repo} - {sha7}.md" once the companion bborbe/agent controller PR lands; existing controllers silently ignore the new field
    ```

13. **Run `make precommit`** from `watcher/github-build/`:
    ```bash
    cd watcher/github-build && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/pkg/` (including `mocks/command_publisher.go`), `docs/build-watcher.md`, `docs/architecture.md`, and root `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- `WatcherCreateTaskCommand` MUST embed `agentlib.CreateTaskCommand` (not duplicate its fields) so existing callers access `cmd.TaskIdentifier`, `cmd.Frontmatter`, `cmd.Body` via Go field promotion without change
- `filename_hint` JSON key MUST be lowercase snake_case — NOT camelCase (`filenameHint`)
- The hint MUST NOT include the `.md` extension — the controller appends it
- The watcher MUST NOT validate the hint for `..`, `/`, length, or filesystem safety beyond the `slugifySegment` per-segment pass + the 200-char total cap. Path-traversal validation, terminal-fallback-to-uuid behavior, and reject-on-malformed lives controller-side (in a separate `bborbe/agent` spec). The watcher's job is to emit a well-formed hint; the controller's job is to defend against ill-formed ones
- SHA truncation: take the first min(7, len(sha)) characters of `episodeSHA`; do NOT pad short SHAs
- `slugifySegment` MUST be applied to `owner` and `repo` SEPARATELY, then joined with `-` — NOT applied to the composite `owner/repo` string
- The SHA7 portion of the hint is NOT slugified — hex digits and hyphens are already filesystem-safe
- Hints > 200 characters MUST be truncated with `glog.Warningf` (not dropped, not returned as error)
- `computeFilenameHint` and `slugifySegment` MUST be unexported — they are implementation details of the pkg package
- The `CommandPublisher` interface MUST accept `WatcherCreateTaskCommand`; the mock MUST be regenerated
- All existing `watcher_test.go` tests must still pass — field promotion on the embedded struct makes all existing `cmd.Frontmatter`, `cmd.TaskIdentifier`, `cmd.Body` assertions work unchanged
- `buildCreateTaskCommand` MUST return `WatcherCreateTaskCommand`, not `agentlib.CreateTaskCommand`
- The provider string is the hard-coded literal `"github"` — NOT derived from config, env, or the allowlist entry
- `applyStateMachine` in `watcher.go` calls `w.publisher.PublishCreate(ctx, cmd)` where `cmd` is the return value of `buildCreateTaskCommand` — the call site requires no change after the return type is updated
- Do NOT change `main.go`, `cmd/run-once/main.go`, `factory.go`, or any file outside `pkg/` except docs and CHANGELOG
- `make precommit` runs from `watcher/github-build/`, never at repo root
- Error wrapping: `github.com/bborbe/errors`; never `fmt.Errorf`
- Coverage ≥80% for changed packages
- Dependency on spec 017 (maintenance loader wiring): this prompt assumes spec 017 is already merged — `NewWatcher` takes 10 parameters including `maintenanceLoader maintenance.Loader`
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm WatcherCreateTaskCommand type defined with correct json tag:
grep -n "WatcherCreateTaskCommand\|FilenameHint\|filename_hint" watcher/github-build/pkg/publisher.go
# Expected: type declaration + json:"filename_hint,omitempty" tag

# Confirm CommandPublisher interface accepts WatcherCreateTaskCommand:
grep -A 3 "type CommandPublisher interface" watcher/github-build/pkg/publisher.go
# Expected: PublishCreate takes WatcherCreateTaskCommand

# Confirm kafkaPublisher.PublishCreate parameter type updated:
grep -A 3 "func.*kafkaPublisher.*PublishCreate" watcher/github-build/pkg/publisher.go
# Expected: cmd WatcherCreateTaskCommand

# Confirm pkg/filename.go exists with all three symbols:
ls watcher/github-build/pkg/filename.go
grep -n "func computeFilenameHint\|func slugifySegment\|maxFilenameHintLen" watcher/github-build/pkg/filename.go
# Expected: all three present

# Confirm buildCreateTaskCommand returns WatcherCreateTaskCommand:
grep -A 8 "func.*buildWatcher.*buildCreateTaskCommand" watcher/github-build/pkg/watcher.go
# Expected: return type is WatcherCreateTaskCommand

# Confirm FilenameHint populated in buildCreateTaskCommand:
grep -n "FilenameHint\|computeFilenameHint" watcher/github-build/pkg/watcher.go
# Expected: FilenameHint: computeFilenameHint("github", owner, repo, episodeSHA)

# Confirm mock regenerated with WatcherCreateTaskCommand:
grep -n "WatcherCreateTaskCommand" watcher/github-build/pkg/mocks/command_publisher.go
# Expected: at least 3 matches (stub, argsForCall struct, method signature)

# Confirm old lib.CreateTaskCommand type absent from mock:
grep -n "lib\.CreateTaskCommand" watcher/github-build/pkg/mocks/command_publisher.go
# Expected: zero matches

# Confirm FilenameHint assertions added to integration tests:
grep -n "FilenameHint" watcher/github-build/pkg/watcher_test.go
# Expected: at least 3 assertions

# Confirm internal unit tests for computeFilenameHint and slugifySegment:
grep -n "computeFilenameHint\|slugifySegment" watcher/github-build/pkg/watcher_internal_test.go
# Expected: DescribeTable blocks for both

# Confirm SHA is not slugified (hyphen survives in sha7):
grep -n "sha-abc\|sha-b\b" watcher/github-build/pkg/watcher_test.go | grep FilenameHint
# Expected: assertions with "sha-abc" and "sha-b" in the hint

# Confirm provider is hard-coded as "github":
grep -n '"github"' watcher/github-build/pkg/watcher.go | grep computeFilenameHint
# Expected: computeFilenameHint("github", ...

# Confirm docs/build-watcher.md has filename_hint section:
grep -n "filename_hint\|Build Failure" docs/build-watcher.md
# Expected: new section header + format table

# Confirm docs/architecture.md updated:
grep -n "filename_hint" docs/architecture.md
# Expected: present in JSON schema block

# Confirm CHANGELOG entry:
grep -n "filename_hint\|human-readable" CHANGELOG.md | head -5
# Expected: one match under ## Unreleased
</verification>
