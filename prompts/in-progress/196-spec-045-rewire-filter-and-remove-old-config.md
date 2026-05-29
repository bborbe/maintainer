---
status: approved
spec: [045-introduce-maintainer-yaml-release-gate]
created: "2026-05-29T12:00:00Z"
queued: "2026-05-29T09:15:05Z"
---

<summary>
- The release watcher will switch its trust gate from "skip if AutoRelease=true" (legacy dark-factory inversion) to "emit ONLY if `release.autoRelease` is true" — a positive opt-in gate sourced from `.maintainer.yaml`.
- The old `GetAutoReleaseConfig` method, its parser, its struct, its tests, and every reference to `.dark-factory/config.yml` in the watcher are removed in this prompt — clean break, no fallback.
- The filter's pass/skip semantics are inverted to match the new gate: `autoRelease: true` passes; everything else (file absent, key absent, value false) skips with the same metric label so existing dashboards keep working.
- Counterfeiter mocks for `GitHubClient` are regenerated so the old method is no longer mockable and existing tests that referenced it stop compiling — those tests are updated as part of this prompt.
- The watcher's gather step now reads `MaintainerConfig` and the `Release` struct's bool now means "opted in," not "skip me."
- README, godoc comments, and `docs/watcher-decision-chains.md` are updated so no surface lies about which file the watcher reads.
</summary>

<objective>
Rewire the release watcher's trust gate to source from `.maintainer.yaml` via the `GetMaintainerConfig` method added in prompt 1, flip the `AutoReleaseFilter` semantics from "skip when true" to "pass when true," delete every trace of `GetAutoReleaseConfig` / `parseAutoReleaseConfig` / `darkFactoryConfig` / `.dark-factory/config.yml` from `watcher/github-release/`, update the watcher gatherer and existing tests to use the new method, regenerate the counterfeiter mock so the old method is no longer mockable, and update README + decision-chains doc + godoc so no surface references the old config path.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

This prompt depends on prompt 1 (`1-spec-045-maintainer-yaml-fetch-and-parse.md`) having landed: the `MaintainerConfig` type, the `GetMaintainerConfig` interface method + implementation, the `parseMaintainerConfig` helper, and the new test block must already exist in `watcher/github-release/pkg/`.

Read these files in full before changing anything:

- `/workspace/specs/in-progress/045-introduce-maintainer-yaml-release-gate.md` — full spec.
- `/workspace/watcher/github-release/pkg/githubclient.go` — full file (now has both old and new methods).
- `/workspace/watcher/github-release/pkg/githubclient_test.go` — full file (test block for the old method is deleted in this prompt).
- `/workspace/watcher/github-release/pkg/release.go` — `Release.AutoRelease` field; godoc and field meaning change.
- `/workspace/watcher/github-release/pkg/watcher.go` — the `gatherRelease` method calls `GetAutoReleaseConfig`; rewire to `GetMaintainerConfig`.
- `/workspace/watcher/github-release/pkg/watcher_test.go` — four `Describe` blocks reference `GetAutoReleaseConfigStub`/`GetAutoReleaseConfigReturns`; all must be migrated.
- `/workspace/watcher/github-release/pkg/filter/auto_release_filter.go` — semantics flip happens here.
- `/workspace/watcher/github-release/pkg/filter/auto_release_filter_test.go` — assertions are inverted.
- `/workspace/watcher/github-release/pkg/filter/filter.go` — `Release.AutoRelease` field godoc and package-level chain godoc both reference the old config path.
- `/workspace/watcher/github-release/pkg/filter/filter_test.go` — chain test asserts "skip if AutoRelease=true"; semantics flip.
- `/workspace/watcher/github-release/pkg/factory/factory.go` — godoc references `auto_release`; the constructor name stays, but the godoc must read correctly under the new semantics.
- `/workspace/watcher/github-release/pkg/metrics.go` — pre-registered label list; the `auto_release` string is KEPT for dashboard continuity per spec § Open Questions.
- `/workspace/watcher/github-release/README.md` — lines 27 and 31 reference the old config path; rewrite under the new gate. Line 95's metrics-table reference to the `auto_release` label is intentionally PRESERVED (metric label unchanged for dashboard continuity).
- `/workspace/docs/watcher-decision-chains.md` — must reference `.maintainer.yaml` per AC #10.
- `/workspace/watcher/github-release/pkg/taskpublisher_test.go` — has `AutoRelease: false` fixture literals; semantically still correct (these tests do not exercise the gate), so they DO NOT NEED CHANGES — leave alone.

Reference guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` context-form rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 conventions.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter regeneration.

Pinned implementation decisions (spec § Open Questions — agent decides; locking them here):

- **Metric label**: KEEP the string literal `"auto_release"` exactly as it is today in `metrics.go` pre-registration AND as the filter's return value when it skips. Spec § Open Questions explicitly allows this for dashboard continuity. Document the choice in the filter's godoc as instructed in requirement 5.
- **Filter file name**: KEEP `auto_release_filter.go` and `NewAutoReleaseFilter`. The spec allows renaming but does not require it; keeping the name minimizes churn across `factory.go`, `watcher.go`, `filter.go`'s package godoc, and `watcher_test.go`.
</context>

<requirements>

1. **Flip the gate semantics in `watcher/github-release/pkg/filter/auto_release_filter.go`**. Replace the entire file body (keep the existing copyright header and `package filter` line) with:

   ```go
   // NewAutoReleaseFilter is the trust-gate predicate sourced from
   // `.maintainer.yaml: release.autoRelease`. It emits the skip label
   // "auto_release" for every Release whose backing config did NOT opt in —
   // i.e., it is a POSITIVE-OPT-IN gate: only `autoRelease: true` passes.
   //
   // The skip-label string is intentionally retained as "auto_release"
   // (rather than renamed to e.g. "not_opted_in") so existing Prometheus
   // dashboards and alerts keyed on that label keep working. The metric
   // semantics shift from "this repo is handled by the dark-factory auto-
   // release daemon, skip" to "this repo did not opt into maintainer-bot
   // auto-release, skip" — both legitimate "do not emit" reasons; the label
   // surface is the same.
   //
   // Release.AutoRelease is sourced once per cycle by
   // GitHubClient.GetMaintainerConfig and mirrored into filter.Release by
   // the watcher's gatherer.
   func NewAutoReleaseFilter() TaskCreationFilter {
       return TaskCreationFilterFunc(func(release Release) string {
           if release.AutoRelease {
               return ""
           }
           return "auto_release"
       })
   }
   ```

2. **Update `watcher/github-release/pkg/filter/auto_release_filter_test.go`** to assert the new semantics. Replace the three existing `It` blocks with:

   ```go
   var _ = Describe("filter.AutoReleaseFilter", func() {
       It("AutoReleaseFilter passes when AutoRelease is true", func() {
           f := filter.NewAutoReleaseFilter()
           Expect(f.Skip(filter.Release{AutoRelease: true})).To(BeEmpty())
       })

       It("AutoReleaseFilter skips with 'auto_release' label when AutoRelease is false", func() {
           f := filter.NewAutoReleaseFilter()
           Expect(f.Skip(filter.Release{AutoRelease: false})).To(Equal("auto_release"))
       })

       It("AutoReleaseFilter skips the zero-value Release with 'auto_release' label", func() {
           f := filter.NewAutoReleaseFilter()
           Expect(f.Skip(filter.Release{})).To(Equal("auto_release"))
       })
   })
   ```

3. **Update `watcher/github-release/pkg/filter/filter.go`**:
   - In the package-level godoc, replace the line `//  3. AutoReleaseFilter — skip if .dark-factory/config.yml: autoRelease: true` with `//  3. AutoReleaseFilter — pass only if .maintainer.yaml: release.autoRelease: true; skip otherwise (gate)`.
   - In the `Release` struct field godoc, replace `AutoRelease       bool   // for AutoReleaseFilter` with `AutoRelease       bool   // .maintainer.yaml: release.autoRelease — true means opted in to maintainer-bot auto-release`.
   - The `TaskCreationFilter` interface comment that lists labels (`"scope", "empty_unreleased", "auto_release", "sha_unchanged"`) stays unchanged — the label set is unchanged.

4. **Update `watcher/github-release/pkg/filter/filter_test.go`** to reflect the flipped gate. The three existing `It` blocks reference the chain with `AutoRelease: false` (used to pass) and `AutoRelease: true` (used to skip with `"auto_release"`). Rewrite them so the chain-passing case sets `AutoRelease: true` and the gate-skip case sets `AutoRelease: false`:

   - `It("TaskCreationFilters returns empty when every filter passes", ...)`: set `AutoRelease: true` in the input Release.
   - `It("TaskCreationFilters returns reason of first filter that votes skip", ...)`: set `AutoRelease: true` in the input (so the empty-unreleased filter is the one that fires first); the assertion stays `Equal("empty_unreleased")`.
   - `It("TaskCreationFilters returns reason of later filter when earlier passes", ...)`: set `UnreleasedBullets: 3, AutoRelease: false`; assertion stays `Equal("auto_release")` (the gate is what skips now).
   - The empty-slice case is unchanged.

5. **Rewire the watcher's gatherer in `watcher/github-release/pkg/watcher.go`**:

   - In `gatherRelease`, replace the `autoRelease, err := w.ghClient.GetAutoReleaseConfig(ctx, repo)` call and the subsequent error block with:

     ```go
     maintainerCfg, err := w.ghClient.GetMaintainerConfig(ctx, repo)
     if err != nil {
         if stderrors.Is(err, ErrRateLimited) {
             return Release{}, "rate_limited", false
         }
         glog.V(2).
             Infof("repo dropped from cycle: owner=%s repo=%s err=%v", repo.Owner, repo.Name, err)
         return Release{}, "", true
     }
     ```

   - In the same function's return statement, replace `AutoRelease:       autoRelease,` with `AutoRelease:       maintainerCfg.Release.AutoRelease,`.

   - Update the godoc on `gatherRelease` (the comment block that begins `// gatherRelease fetches HeadSHA, ChangelogContent, AutoReleaseConfig for one repo.`): replace `AutoReleaseConfig` with `MaintainerConfig`.

   - Update the godoc on the `Poll` method: in the step-3c line `//     c. GetAutoReleaseConfig`, replace with `//     c. GetMaintainerConfig`.

   - Update the godoc on `processRepos` (the comment block beginning `// processRepos iterates repos sequentially`): in the bullet that says `//   - Per-repo prune (continue loop): GetMasterSHA / GetChangelogContent / GetAutoReleaseConfig transient`, replace `GetAutoReleaseConfig` with `GetMaintainerConfig`.

6. **Update `watcher/github-release/pkg/release.go`**:
   - Replace the `Release` struct field comment `AutoRelease       bool   // .dark-factory/config.yml "autoRelease: true" — skip path` with `AutoRelease       bool   // .maintainer.yaml: release.autoRelease — true means the repo is opted into maintainer-bot auto-release (gate input)`.
   - In the struct-level godoc bullet list (the one that begins `//  4. AutoRelease (from GetAutoReleaseConfig`), replace it with `//  4. AutoRelease (from GetMaintainerConfig — zero-value if .maintainer.yaml absent)`.

7. **Update `watcher/github-release/pkg/watcher_test.go`** (four `Describe` blocks reference the old mock surface). For each occurrence:
   - Replace `ghClient.GetAutoReleaseConfigStub = func(_ context.Context, r pkg.Repo) (bool, error) { return false, nil }` (and any close variant) with `ghClient.GetMaintainerConfigStub = func(_ context.Context, r pkg.Repo) (pkg.MaintainerConfig, error) { return pkg.MaintainerConfig{Release: pkg.MaintainerReleaseConfig{AutoRelease: true}}, nil }`. The flip from `false` to `true` is required: under the OLD semantics `false` meant "not handled by dark-factory daemon, please proceed" (i.e., pass the chain); under the NEW semantics `true` means "opted in, please proceed" (i.e., pass the chain). Every test that previously stubbed `false` to mean "proceed past the gate" must now stub `AutoRelease: true` to mean the same thing.
   - Replace `ghClient.GetAutoReleaseConfigReturns(false, nil)` with `ghClient.GetMaintainerConfigReturns(pkg.MaintainerConfig{Release: pkg.MaintainerReleaseConfig{AutoRelease: true}}, nil)`.
   - The four `Describe` blocks affected, by their existing names: "Poll publishes one task per non-skipped repo and saves cursor", "Poll prunes individual repos with transient GetMasterSHA errors and continues", "Poll aborts mid-cycle on per-repo rate-limit during GetChangelogContent", "Poll updates cursor only for repos that successfully publish". The two `Describe` blocks that stub `ListReposReturns` with an error (`rate_limited` and `github_error` aborts) never reach the gather step and therefore do not stub `GetAutoReleaseConfig` — they need no change.
   - Verify after the rewrite that the metric assertion `Expect(metrics.IncFilterSkippedArgsForCall(0)).To(Equal("empty_unreleased"))` in the first `Describe` block still holds — the empty-unreleased filter still fires first because the gate now passes (was: skipped previously for "auto_release"; now passes because `AutoRelease: true`).

8. **Delete every trace of the legacy config surface** from `watcher/github-release/`:

   - In `watcher/github-release/pkg/githubclient.go`:
     - Remove the `GetAutoReleaseConfig` method from the `GitHubClient` interface.
     - Remove the entire `func (c *githubClient) GetAutoReleaseConfig(...)` implementation.
     - Remove the entire `func parseAutoReleaseConfig(...)` helper.
     - Remove the entire `type darkFactoryConfig struct { ... }`.
     - Do NOT remove the `"gopkg.in/yaml.v3"` import — `parseMaintainerConfig` still uses it.

   - In `watcher/github-release/pkg/githubclient_test.go`:
     - Remove the entire `Describe("GetAutoReleaseConfig", func() { ... })` block (the one beginning at the line `Describe("GetAutoReleaseConfig", func() {`).
     - The `Describe("GetMaintainerConfig", ...)` block added in prompt 1 remains.
     - The `strings` import may become unused (the malformed-YAML assertion used it); remove it if so.

9. **Regenerate the counterfeiter mock**. Run from `watcher/github-release/`:

   ```
   make generate
   ```

   This rewrites `watcher/github-release/pkg/mocks/github_client.go`. After regeneration, the mock MUST:
   - Contain `GetMaintainerConfigStub`, `GetMaintainerConfigReturns`, `GetMaintainerConfigCallCount`, `GetMaintainerConfigArgsForCall`, `GetMaintainerConfigReturnsOnCall`, `GetMaintainerConfigCalls`.
   - NOT contain any symbol matching `GetAutoReleaseConfig`.

   Do not hand-edit the mock. If the regeneration does not produce this shape, the underlying interface change in `githubclient.go` is incomplete — fix the interface and rerun.

10. **Update the README at `watcher/github-release/README.md`**:

    - Replace the line containing `Fetch \`.dark-factory/config.yml\` to check \`autoRelease: true\` (skip if so — existing autorelease path handles it).` with `Fetch \`.maintainer.yaml\` to read \`release.autoRelease\`; the watcher proceeds only when this is \`true\` (trust gate — repos without the file are skipped).`.
    - Replace the line `   - \`AutoReleaseFilter\` — skip repos with dark-factory autoRelease enabled` with `   - \`AutoReleaseFilter\` — gate; passes only when \`.maintainer.yaml: release.autoRelease: true\`, skips every other shape (file absent, key absent, false)`.
    - The metrics table row that lists `auto_release` as a `filter_skipped_total` label stays unchanged — the label string is preserved for dashboard continuity.

11. **Update `docs/watcher-decision-chains.md`** so AC #10 holds: the auto-release predicate node must reference `.maintainer.yaml`, not `.dark-factory/config.yml`. Read the file first to locate the node. If the file does NOT currently mention the auto-release predicate or the dark-factory config path (the initial check showed it does not — it describes generic Chain 1 / Chain 2 / TrustGate concepts), append a new section at the end of the file using this exact text:

    ```markdown

    ## Release Watcher Trust Gate (`.maintainer.yaml`)

    The `maintainer-watcher-github-release` predicate chain reads `.maintainer.yaml` from the target repo's default branch and emits a release task only when `release.autoRelease: true`. Absent file, absent `release:` key, absent `autoRelease` key, and explicit `false` all skip the repo with the `auto_release` `filter_skipped_total` label.

    The github-releaser agent itself does NOT consult `.maintainer.yaml` — the gate is watcher-side only, so operator-authored manual tasks bypass it.
    ```

    If the file ALREADY mentions `.dark-factory/config.yml` somewhere, replace that string with `.maintainer.yaml` instead of appending a new section.

12. **Update `watcher/github-release/pkg/factory/factory.go`** godoc:
    - In the comment block above `CreateStaticFilters`, replace `// empty_unreleased + auto_release). SHAUnchangedFilter is composed in per` with `// empty_unreleased + auto_release gate). SHAUnchangedFilter is composed in per`. The label string `auto_release` stays.

13. **Leave `watcher/github-release/pkg/metrics.go` UNCHANGED.** The pre-registered label list at line 71 contains `"auto_release"`; that label is intentionally preserved for dashboard continuity per spec § Open Questions. Editing the list breaks the very dashboards we are trying not to break.

14. **Verify nothing escaped**. Run from the repo root `/workspace/`:

    ```
    grep -rn "GetAutoReleaseConfig\|dark-factory/config\|darkFactoryConfig\|parseAutoReleaseConfig" watcher/github-release/
    ```

    Expected: zero matches, grep exits 1.

    ```
    grep -rn "dark-factory/config" watcher/github-release/
    ```

    Expected: zero matches, grep exits 1.

    ```
    grep -rn "\.maintainer\.yaml" watcher/github-release/pkg/
    ```

    Expected: at least one match in `pkg/githubclient.go` (the `Repositories.GetContents` path argument and the wrap messages).

    ```
    grep -n "maintainer.yaml" docs/watcher-decision-chains.md
    grep -n "dark-factory/config" docs/watcher-decision-chains.md
    ```

    Expected: first grep returns at least one line; second grep returns none.

15. **Verify the suite passes and coverage holds**. Run from `watcher/github-release/`:

    ```
    make precommit
    go test -cover ./pkg/...
    ```

    `make precommit` must exit 0. `go test -cover ./pkg/...` emits one coverage line per package; EACH non-mock package (`./pkg/`, `./pkg/filter/`, `./pkg/factory/`, `./pkg/auth/`) must report ≥ 80.0%. The `./pkg/mocks/` package is generated code and excluded. If any non-mock package drops below 80%, the test deletions in step 8 went too far — add `It` blocks back in `Describe("GetMaintainerConfig", ...)` rather than re-introducing the old test block.

</requirements>

<constraints>
- All code lives under `watcher/github-release/` (plus the single doc file `docs/watcher-decision-chains.md`). No other watcher, no other agent, no shared lib package is modified.
- Error wrapping uses `github.com/bborbe/errors` context-form only: `errors.New(ctx, msg)`, `errors.Wrap(ctx, err, msg)`, `errors.Wrapf(ctx, err, fmt, args...)`, `errors.Errorf(ctx, fmt, args...)`. NEVER `fmt.Errorf` on production paths.
- Tests use Ginkgo v2 + Gomega in an external `_test` package.
- Mocks are regenerated via `make generate` (counterfeiter v6). No new tooling. No hand-editing the mock.
- The fetch path mirrors the existing `GetChangelogContent` shape — already enforced in prompt 1; this prompt does not touch the fetch implementation.
- No `time.Now()` in business logic; if needed, use `github.com/bborbe/time` `libtime`. No new time dependencies are needed for this prompt.
- The frontmatter contract for tasks the watcher emits is unchanged.
- The github-releaser agent's behavior is unchanged. It never reads `.maintainer.yaml`. Manual tasks (operator-authored, assigned to `github-releaser-agent`) continue to flow without consulting any config file.
- Watcher coverage target ≥80% for the `pkg/` directory.
- The skip-label string `"auto_release"` is preserved verbatim in the filter return value, the metrics pre-registration list, and the README metrics table — dashboard continuity.
- No backward-compat fallback to `.dark-factory/config.yml`. Do NOT add a "read maintainer.yaml first, fall back to dark-factory config" path — explicit non-goal in the spec.
- No per-repo opt-out flag inside `.maintainer.yaml` for the gate itself — the absence of the file or field IS the opt-out.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass — except for the explicitly-deleted `GetAutoReleaseConfig` test block.
</constraints>

<verification>
Run from `watcher/github-release/`:

```
make precommit
```

Must exit 0.

```
go test -cover ./pkg/...
```

Coverage line must report ≥ 80.0% for `./pkg/...`.

Run from `/workspace/`:

```
grep -rn "GetAutoReleaseConfig\|dark-factory/config\|darkFactoryConfig\|parseAutoReleaseConfig" watcher/github-release/
```

Expected: empty output, exit code 1.

```
grep -rn "\.maintainer\.yaml" watcher/github-release/pkg/
```

Expected: at least one match in `pkg/githubclient.go`.

```
grep -n "GetMaintainerConfig\|GetAutoReleaseConfig" watcher/github-release/pkg/githubclient.go
```

Expected: only `GetMaintainerConfig` matches; `GetAutoReleaseConfig` returns nothing.

```
grep -n "GetMaintainerConfigStub\|GetAutoReleaseConfigStub" watcher/github-release/pkg/mocks/github_client.go
```

Expected: only `GetMaintainerConfigStub` matches.

```
grep -n "maintainer.yaml" docs/watcher-decision-chains.md
grep -n "dark-factory/config" docs/watcher-decision-chains.md
```

Expected: first returns at least one line; second returns none.

```
grep -n "auto_release" watcher/github-release/pkg/metrics.go watcher/github-release/pkg/filter/auto_release_filter.go
```

Expected: matches in both files — the label string is preserved.
</verification>
