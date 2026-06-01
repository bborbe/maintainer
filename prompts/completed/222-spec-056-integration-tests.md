---
status: completed
spec: [056-plugin-version-bump]
summary: Added 6 integration tests for manifest bumping in executeDirectPush with 4 golden fixture files
container: maintainer-plugin-version-bump-exec-222-spec-056-integration-tests
dark-factory-version: v0.173.0
created: "2026-06-01T00:00:00Z"
queued: "2026-05-31T22:32:08Z"
started: "2026-05-31T23:19:04Z"
completed: "2026-05-31T23:22:07Z"
branch: feature/plugin-version-bump
---

<summary>
- Golden fixture files created in `pkg/testdata/` for plugin.json and marketplace.json
- Integration tests verify executeDirectPush bumps both manifests when present
- Integration tests verify backward compatibility (no manifests → only CHANGELOG.md committed)
- Integration tests verify fail-closed on unexpected_diff (extra files in commit)
- Integration tests verify fail-closed on plugin_manifest_invalid (malformed JSON)
- All GitOps mock call-count and call-argument assertions included
</summary>

<objective>
Add integration tests to `pkg/steps_execution_test.go` that verify the manifest bumping integration in `executeDirectPush`. Tests use the counterfeiter `GitOps` mock and golden fixture files in `pkg/testdata/`. The tests cover: (a) both manifests present and bumped, (b) no manifests present (backward compatibility), (c) unexpected file in committed files (guard fail-closed), (d) malformed plugin.json (bump fail-closed). Test coverage for the manifest package functions is in Prompt 1; these tests cover the execution-step integration path.
</objective>

<context>
Read these files before implementing:
- `/workspace/agent/github-releaser/pkg/steps_execution_test.go` — the test file to extend (already imports `os`, defines `taskMD`, `writeChangelog`)
- `/workspace/agent/github-releaser/mocks/git_ops.go` — the counterfeiter mock API (`CommitArgsForCall`, `CommitCallCount`, `CommitStub`, `CommittedFilesReturns`, `TagCallCount`, `PushCallCount`)
- `/workspace/agent/github-releaser/pkg/plugin/manifest.go` — the manifest package functions (from Prompt 1)
- `/workspace/agent/github-releaser/pkg/result_output.go` — the `ResultOutput` struct (`Outcome`, `ErrorCategory`, `Tag`, `Error`, `CommitSHA` fields)

Test data directory: `agent/github-releaser/pkg/testdata/` (Go test cwd is the package dir, so reference fixtures via relative path `testdata/...` — NOT `pkg/testdata/...`).

**Plan-version constraint:** the existing `taskMD` constant declares `next_version: 1.2.8`. The new integration tests use a **local `taskMDPlugin` constant** declared inside the plugin-manifests `Context` with `next_version: 0.10.0` (and `next_version_header: ## v0.10.0`) so the bumped value written into the fixtures (`0.9.12 → 0.10.0`) matches the spec ACs. This is intentional — do NOT modify the existing `taskMD`; that constant is shared with happy-path tests at `1.2.8` and changing it would break unrelated assertions.
</context>

<requirements>
1. **Create golden fixture directory**: `mkdir -p /workspace/agent/github-releaser/pkg/testdata/`

2. **Create `/workspace/agent/github-releaser/pkg/testdata/plugin.json.pre`**:
   ```json
   {
     "name": "example",
     "version": "0.9.12",
     "description": "A Claude Code plugin"
   }
   ```

3. **Create `/workspace/agent/github-releaser/pkg/testdata/plugin.json.post`**:
   ```json
   {
     "name": "example",
     "version": "0.10.0",
     "description": "A Claude Code plugin"
   }
   ```

4. **Create `/workspace/agent/github-releaser/pkg/testdata/marketplace.json.pre`**:
   ```json
   {
     "metadata": {
       "version": "0.9.12"
     },
     "plugins": [
       {"name": "plugin-a", "version": "0.9.12"},
       {"name": "plugin-b", "version": "0.9.12"},
       {"name": "plugin-c", "version": "0.9.12"}
     ]
   }
   ```

5. **Create `/workspace/agent/github-releaser/pkg/testdata/marketplace.json.post`**:
   ```json
   {
     "metadata": {
       "version": "0.10.0"
     },
     "plugins": [
       {"name": "plugin-a", "version": "0.10.0"},
       {"name": "plugin-b", "version": "0.10.0"},
       {"name": "plugin-c", "version": "0.10.0"}
     ]
   }
   ```

6. **Add tests to `/workspace/agent/github-releaser/pkg/steps_execution_test.go`** inside a new top-level `Context("plugin manifests", func() { … })`:

   a. **Inside the new `Context`, declare a local `taskMDPlugin` constant** (mirrors the existing `taskMD` shape, only `current_version` / `next_version` / `next_version_header` differ — the bumped value `0.10.0` must match the fixtures):
   ```go
   const taskMDPlugin = "<frontmatter + Plan section identical to taskMD except `current_version: v0.9.12`, `next_version: 0.10.0`, `next_version_header: \"## v0.10.0\"`>"
   ```
   Copy the existing `taskMD` text verbatim, change only those three plan fields. Do NOT modify the shared `taskMD`.

   b. **Helper functions** in the new `Context` (use relative path `testdata/...` — Go test cwd is the package dir, NOT the agent root):
   ```go
   readFixture := func(name string) []byte {
       data, err := os.ReadFile(filepath.Join("testdata", name))
       Expect(err).NotTo(HaveOccurred())
       return data
   }
   writeManifest := func(workdir, relPath, fixtureName string) {
       Expect(os.MkdirAll(filepath.Join(workdir, ".claude-plugin"), 0o750)).To(Succeed())
       Expect(os.WriteFile(filepath.Join(workdir, relPath), readFixture(fixtureName), 0o600)).To(Succeed())
   }
   writeChangelogAndBothManifests := func(workdir string) {
       writeChangelog(workdir)
       writeManifest(workdir, ".claude-plugin/plugin.json", "plugin.json.pre")
       writeManifest(workdir, ".claude-plugin/marketplace.json", "marketplace.json.pre")
   }
   ```

   c. **Test: both manifests present, both bumped, byte-exact equality against `.post` fixtures**:
   ```go
   It("bumps plugin.json and marketplace.json to unprefixed semver; commits exactly those files plus CHANGELOG.md; guard passes",
       func() {
           fakeOps := &gitmocks.GitOps{}
           fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
               writeChangelogAndBothManifests(workdir)
               return nil
           }
           fakeOps.CommitStub = func(_ context.Context, workdir, _ string, _ ...string) (string, error) {
               // Byte-exact equality vs the .post golden fixtures
               pluginActual, err := os.ReadFile(filepath.Join(workdir, ".claude-plugin", "plugin.json"))
               Expect(err).NotTo(HaveOccurred())
               Expect(pluginActual).To(Equal(readFixture("plugin.json.post")))

               marketplaceActual, err := os.ReadFile(filepath.Join(workdir, ".claude-plugin", "marketplace.json"))
               Expect(err).NotTo(HaveOccurred())
               Expect(marketplaceActual).To(Equal(readFixture("marketplace.json.post")))
               return "abc1234", nil
           }
           fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md", ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"}, nil)
           fakeOps.TagReturns(nil)
           fakeOps.PushReturns(nil)

           step := pkg.NewExecutionStep(fakeOps, "test-token")
           md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
           Expect(err).NotTo(HaveOccurred())

           result, err := step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())
           Expect(result.Status).To(Equal(agentlib.AgentStatusDone))

           // Commit called with exactly the right files (order pinned by Prompt 3's
           // commitPaths := append([]string{changelogFileName}, detectedManifests...))
           _, _, _, commitPaths := fakeOps.CommitArgsForCall(0)
           Expect(commitPaths).To(Equal([]string{"CHANGELOG.md", ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"}))

           Expect(fakeOps.TagCallCount()).To(Equal(1))
           Expect(fakeOps.PushCallCount()).To(Equal(1))

           got, _ := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
           Expect(got.Outcome).To(Equal("released"))
       })
   ```

   d. **Test: only plugin.json present** (covers Failure-Mode row "one manifest exists, the other does not"):
   ```go
   It("plugin.json only → commits {CHANGELOG.md, .claude-plugin/plugin.json}; guard passes",
       func() {
           fakeOps := &gitmocks.GitOps{}
           fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
               writeChangelog(workdir)
               writeManifest(workdir, ".claude-plugin/plugin.json", "plugin.json.pre")
               return nil
           }
           fakeOps.CommitStub = func(_ context.Context, _, _ string, paths ...string) (string, error) {
               Expect(paths).To(Equal([]string{"CHANGELOG.md", ".claude-plugin/plugin.json"}))
               return "abc1234", nil
           }
           fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md", ".claude-plugin/plugin.json"}, nil)
           fakeOps.TagReturns(nil)
           fakeOps.PushReturns(nil)

           step := pkg.NewExecutionStep(fakeOps, "")
           md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
           Expect(err).NotTo(HaveOccurred())

           result, err := step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())
           Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
       })
   ```

   e. **Test: only marketplace.json present**:
   ```go
   It("marketplace.json only → commits {CHANGELOG.md, .claude-plugin/marketplace.json}; guard passes",
       func() {
           fakeOps := &gitmocks.GitOps{}
           fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
               writeChangelog(workdir)
               writeManifest(workdir, ".claude-plugin/marketplace.json", "marketplace.json.pre")
               return nil
           }
           fakeOps.CommitStub = func(_ context.Context, _, _ string, paths ...string) (string, error) {
               Expect(paths).To(Equal([]string{"CHANGELOG.md", ".claude-plugin/marketplace.json"}))
               return "abc1234", nil
           }
           fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md", ".claude-plugin/marketplace.json"}, nil)
           fakeOps.TagReturns(nil)
           fakeOps.PushReturns(nil)

           step := pkg.NewExecutionStep(fakeOps, "")
           md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
           Expect(err).NotTo(HaveOccurred())

           result, err := step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())
           Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
       })
   ```

   f. **Test: no manifests (backward compatibility)** — uses the original `taskMD` (since no manifests = no fixture-version dependency):
   ```go
   It("no .claude-plugin/ dir → commits only CHANGELOG.md; guard passes",
       func() {
           fakeOps := &gitmocks.GitOps{}
           fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
               writeChangelog(workdir)
               return nil
           }
           fakeOps.CommitStub = func(_ context.Context, _, _ string, paths ...string) (string, error) {
               Expect(paths).To(Equal([]string{"CHANGELOG.md"}),
                   "commit paths must be exactly [CHANGELOG.md] when no manifests exist")
               return "abc1234", nil
           }
           fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)
           fakeOps.TagReturns(nil)
           fakeOps.PushReturns(nil)

           step := pkg.NewExecutionStep(fakeOps, "")
           md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
           Expect(err).NotTo(HaveOccurred())

           result, err := step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())
           Expect(result.Status).To(Equal(agentlib.AgentStatusDone))

           got, _ := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
           Expect(got.Outcome).To(Equal("released"))
       })
   ```

   g. **Test: unexpected_diff guard (extra file)**:
   ```go
   It("CommittedFiles returns unexpected file → Result(failed, error_category=unexpected_diff); Tag NOT called; Push NOT called",
       func() {
           fakeOps := &gitmocks.GitOps{}
           fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
               writeChangelogAndBothManifests(workdir)
               return nil
           }
           fakeOps.CommitStub = func(_ context.Context, _, _ string, _ ...string) (string, error) {
               return "def5678", nil
           }
           // Simulate: something wrote README.md between write and commit (attack scenario)
           fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md", ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json", "README.md"}, nil)
           fakeOps.TagReturns(nil)
           fakeOps.PushReturns(nil)

           step := pkg.NewExecutionStep(fakeOps, "")
           md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
           Expect(err).NotTo(HaveOccurred())

           result, err := step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())
           Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

           Expect(fakeOps.CommittedFilesCallCount()).To(Equal(1))
           Expect(fakeOps.TagCallCount()).To(Equal(0))
           Expect(fakeOps.PushCallCount()).To(Equal(0))

           got, _ := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
           Expect(got.Outcome).To(Equal("failed"))
           Expect(string(got.ErrorCategory)).To(Equal("unexpected_diff"))
           Expect(got.Tag).To(BeEmpty())
           Expect(got.CommitSHA).To(BeEmpty())
       })
   ```

   h. **Test: plugin_manifest_invalid (malformed JSON)**:
   ```go
   It("plugin.json is malformed JSON → Result(failed, error_category=plugin_manifest_invalid); Commit NOT called; Tag NOT called; Push NOT called",
       func() {
           fakeOps := &gitmocks.GitOps{}
           fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
               writeChangelog(workdir)
               Expect(os.MkdirAll(filepath.Join(workdir, ".claude-plugin"), 0o750)).To(Succeed())
               malformedPlugin := []byte(`{"name": "example", "version": }`)
               Expect(os.WriteFile(filepath.Join(workdir, ".claude-plugin", "plugin.json"), malformedPlugin, 0o600)).To(Succeed())
               return nil
           }
           fakeOps.TagReturns(nil)
           fakeOps.PushReturns(nil)

           step := pkg.NewExecutionStep(fakeOps, "")
           md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
           Expect(err).NotTo(HaveOccurred())

           result, err := step.Run(context.Background(), md)
           Expect(err).NotTo(HaveOccurred())
           Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

           Expect(fakeOps.CommitCallCount()).To(Equal(0))
           Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))
           Expect(fakeOps.TagCallCount()).To(Equal(0))
           Expect(fakeOps.PushCallCount()).To(Equal(0))

           got, _ := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
           Expect(got.Outcome).To(Equal("failed"))
           Expect(string(got.ErrorCategory)).To(Equal("plugin_manifest_invalid"))
           Expect(got.Error).To(ContainSubstring(".claude-plugin/plugin.json"))
           Expect(got.Tag).To(BeEmpty())
           Expect(got.CommitSHA).To(BeEmpty())
       })
   ```

7. **Imports**: verify `"os"` and `"path/filepath"` are already imported in `steps_execution_test.go` (they are — used elsewhere); no new imports beyond what the file already has.

8. **Run `make test`** after each meaningful change to verify tests pass.
</requirements>

<constraints>
- Tests use counterfeiter `GitOps` mock — no real git, no real filesystem outside `t.TempDir()`-rooted workdirs
- Golden fixtures are checked-in (not generated at runtime); paths reference them as `testdata/...` (Go test cwd is the package dir)
- Tests must be in `package pkg_test` (external test package) — matches the rest of the file
- Ginkgo v2 + Gomega patterns
- All error paths assert mock call counts = 0 for Commit/Tag/Push (and `got.Tag`/`got.CommitSHA` empty for `failed` outcomes)
- `taskMD` is shared with other tests at `next_version: 1.2.8` — do NOT modify it. The new `Context` declares a local `taskMDPlugin` at `0.10.0` so the bumped value matches the `0.9.12 → 0.10.0` fixtures (which match the spec ACs verbatim).
</constraints>

<verification>
From `agent/github-releaser/`, run:
```
make precommit
```
All tests must pass. The Ginkgo suite includes the new manifest integration tests; the byte-exact `Expect(actual).To(Equal(readFixture("…post")))` assertions own the per-file diff verification (no manual `diff -u` spot-checks needed — the test fails if the fixture isn't byte-for-byte the expected output).
</verification>