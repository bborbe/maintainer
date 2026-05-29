---
status: completed
spec: [052-migrate-pr-reviewer-to-maintainer-yaml]
summary: 'Migrated pr-reviewer agent auto-approve config from .pr-reviewer.yaml to .maintainer.yaml: prReviewer.autoApprove via lib/maintainerconfig'
container: maintainer-pr-reviewer-yaml-exec-209-spec-052-agent-maintainer-yaml
dark-factory-version: v0.173.0
created: "2026-05-29T15:32:00Z"
queued: "2026-05-29T16:26:53Z"
started: "2026-05-29T16:34:21Z"
completed: "2026-05-29T16:37:24Z"
---

<summary>
- Switches the pr-reviewer agent's auto-approve decision from reading `.pr-reviewer.yaml` to reading `.maintainer.yaml: prReviewer.autoApprove` from the same cloned working directory.
- A clean break: the old `.pr-reviewer.yaml` reader, its config type, and all references to that filename are removed from the agent. No fallback, no transition mode.
- Parsing is delegated to the shared `lib/maintainerconfig` package (prompt 1) so the agent and the watcher agree on one schema.
- Behavior is identical to today: file missing → comment-only (no error); `prReviewer.autoApprove: true` → approving review on an approve verdict; absent key / false → comment-only; malformed YAML → loud wrapped error (not silently downgraded).
- The auto-approve decision tests are rewritten to exercise the new file and key path.
- The unrelated operator-side CLI config (`~/.config/maintainer/pr-reviewer.yaml`) is explicitly left alone — it is a different concern and keeps its name.
</summary>

<objective>
Make the pr-reviewer agent decide auto-approve exclusively by reading `.maintainer.yaml: prReviewer.autoApprove` from the cloned workDir, parsed via `github.com/bborbe/maintainer/lib/maintainerconfig` (prompt 1). Delete the `.pr-reviewer.yaml` reader's old behavior, the `AutoApproveConfig` type, and every per-repo `.pr-reviewer.yaml` reference in `agent/pr-reviewer/`. No fallback.

End state: `cd agent/pr-reviewer && make precommit` exits 0; `grep -rn "\.pr-reviewer\.yaml\|AutoApproveConfig" agent/pr-reviewer/pkg/` exits 1; `grep -rn "\.maintainer\.yaml" agent/pr-reviewer/pkg/githubposter/` is non-empty.
</objective>

<context>
Read before writing code:

- `CLAUDE.md` at repo root — project conventions.
- `specs/in-progress/052-migrate-pr-reviewer-to-maintainer-yaml.md` — re-read Desired Behavior 3, 5, 7, the Failure Modes table (every pr-reviewer-path row), the Security section (malformed YAML must fail loudly, not silently false), and the Acceptance Criteria rows that grep `agent/pr-reviewer/`. NOTE the Open Question: keeping the reader named `ReadAutoApproveConfig` (returns just the bool) OR renaming. This prompt keeps a small reader that returns the bool, renamed to `ReadAutoApprove` for accuracy, returning `(bool, error)` — see step 2. (Either name satisfies the spec; this choice avoids leaving a now-misnamed `*Config` reader after the `AutoApproveConfig` type is deleted.)
- `lib/maintainerconfig/maintainerconfig.go` (created in prompt 1) — the shared parser. The agent calls `maintainerconfig.Parse(ctx, data)` and reads `cfg.PrReviewer.AutoApprove`.
- `agent/pr-reviewer/pkg/githubposter/config.go` — the file to rewrite. Current `ReadAutoApproveConfig(ctx, workDir) (AutoApproveConfig, error)` reads `.pr-reviewer.yaml` via `os.ReadFile`, returns zero-value on `os.IsNotExist`, wraps other read/parse errors. The new reader keeps the missing-file-is-not-an-error contract but reads `.maintainer.yaml` and delegates parsing to the lib.
- `agent/pr-reviewer/pkg/githubposter/types.go` — defines `AutoApproveConfig` (lines 16-19), which is DELETED by this prompt. Leave `HTTPClient` and the `DefaultBotLogin` / `BotLoginEnv` consts untouched.
- `agent/pr-reviewer/pkg/githubposter/poster.go` — the consumer. Line 91 `config, err := ReadAutoApproveConfig(ctx, req.WorkDir)`; line 95 `FailureStep: "read .pr-reviewer.yaml"`; line 107 `mapVerdictAndSummary(req.Verdict, config.AutoApprove, req.Summary)`. Also line 66 godoc `(no .pr-reviewer.yaml lookup needed for LGTM)`. All four must change to the new reader / new filename.
- `agent/pr-reviewer/pkg/githubposter/config_test.go` — the existing reader test (DescribeTable writing `.pr-reviewer.yaml`). REWRITE to write `.maintainer.yaml` with `prReviewer:` shapes.
- `agent/pr-reviewer/pkg/githubposter/poster_test.go` lines 127-132 — the `writeYAML` helper writes `.pr-reviewer.yaml` with `autoApprove: %v`. REWRITE to write `.maintainer.yaml` with the `prReviewer.autoApprove` shape. This is the auto-approve DECISION test (verdict→event/state mapping); it must exercise the new file + key.
- `agent/pr-reviewer/pkg/poster_types.go` line 24 — godoc comment `no .pr-reviewer.yaml lookup needed for LGTM`. Update the filename in this comment (per-repo reference; spec § Desired Behavior 5 says NO `.pr-reviewer.yaml` string remains in source/tests/godoc under `agent/pr-reviewer/` for the per-repo file).
- `agent/pr-reviewer/pkg/factory/factory.go` line 37 — godoc comment `per-repo .pr-reviewer.yaml (autoApprove: bool)`. Per-repo reference under `pkg/`; MUST change to `.maintainer.yaml` or the per-repo grep in step 8 / verification fails.
- `agent/pr-reviewer/docs/pr-post-back.md` lines 21 and 41 — per-repo references to reading `.pr-reviewer.yaml` in the worktree. Update both to `.maintainer.yaml` (spec § Acceptance Criteria greps the WHOLE `agent/pr-reviewer/` dir for the per-repo file; only the `~/.config/maintainer/pr-reviewer.yaml` operator refs may remain).
- `agent/pr-reviewer/go.mod` — already requires the lib module with `replace … => ../../lib` (the agent already imports `lib/repoallowlist`, `lib/githubapp`, `lib/prurl`), so importing `lib/maintainerconfig` needs only `go mod tidy`.
- DO NOT TOUCH (out of scope — operator-side global CLI config, NOT the per-repo file): `agent/pr-reviewer/cmd/cli/main.go:71`, `agent/pr-reviewer/pkg/config.go:135` and `:206`. These reference `~/.config/maintainer/pr-reviewer.yaml`, the operator's machine allowlist — spec § Non-goals keeps that file's name. Leave them exactly as-is.

Coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` `Wrap`/`Wrapf` form. NO `fmt.Errorf`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + `DescribeTable` conventions, external `_test` package.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-parse-pattern.md` — read-then-delegate-to-pure-parse idiom.
</context>

<requirements>

**Run order: rewrite `config.go`, delete the type in `types.go`, update `poster.go` + `poster_types.go`, rewrite the two test files, then `go mod tidy`, then `make precommit` as final verification.**

1. **Delete the `AutoApproveConfig` type from `agent/pr-reviewer/pkg/githubposter/types.go`.** Remove lines 16-19 (the godoc comment + the struct). Keep the `HTTPClient` interface (with its `//counterfeiter:generate` directive) and the `DefaultBotLogin` / `BotLoginEnv` const block exactly as-is.

2. **Rewrite `agent/pr-reviewer/pkg/githubposter/config.go`** to read `.maintainer.yaml` and delegate parsing to the shared lib. Exact shape:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package githubposter

   import (
       "context"
       "os"
       "path/filepath"

       errors "github.com/bborbe/errors"

       "github.com/bborbe/maintainer/lib/maintainerconfig"
   )

   // ReadAutoApprove reads `.maintainer.yaml` from workDir and returns the
   // prReviewer.autoApprove gate. A missing file is NOT an error — returns
   // false (the spec default: comment-only). Malformed YAML surfaces as a
   // wrapped error (NOT silently false) so the ai_review step fails loudly
   // rather than masking an operator typo.
   func ReadAutoApprove(ctx context.Context, workDir string) (bool, error) {
       path := filepath.Join(workDir, ".maintainer.yaml")
       data, err := os.ReadFile(
           path,
       ) // #nosec G304 -- workDir is an internal trusted path, not user-controlled input
       if err != nil {
           if os.IsNotExist(err) {
               return false, nil
           }
           return false, errors.Wrapf(ctx, err, "read .maintainer.yaml at %s", path)
       }
       cfg, err := maintainerconfig.Parse(ctx, data)
       if err != nil {
           return false, errors.Wrapf(ctx, err, "parse .maintainer.yaml at %s", path)
       }
       return cfg.PrReviewer.AutoApprove, nil
   }
   ```

   Notes:
   - Missing file → `(false, nil)` preserves the unchanged default (spec § Failure Modes row 1).
   - Malformed YAML → wrapped error (spec § Security / § Failure Modes "fail loudly"). `maintainerconfig.Parse` already wraps once; the agent wraps again with the file path — both wraps are intended.
   - `errors.Wrapf` from `github.com/bborbe/errors`. NO `fmt.Errorf`.
   - The function now returns `(bool, error)` — there is no longer an `AutoApproveConfig` to return (its type was deleted in step 1).

3. **Update the caller `agent/pr-reviewer/pkg/githubposter/poster.go`:**
   - Line 91: change `config, err := ReadAutoApproveConfig(ctx, req.WorkDir)` to `autoApprove, err := ReadAutoApprove(ctx, req.WorkDir)`.
   - Line 95: change `FailureStep: "read .pr-reviewer.yaml"` to `FailureStep: "read .maintainer.yaml"`.
   - Line 107: change `mapVerdictAndSummary(req.Verdict, config.AutoApprove, req.Summary)` to `mapVerdictAndSummary(req.Verdict, autoApprove, req.Summary)`.
   - Line 66 godoc: change `(no .pr-reviewer.yaml lookup needed for LGTM)` to `(no .maintainer.yaml lookup needed for LGTM)`.

4. **Update the remaining per-repo `.pr-reviewer.yaml` references in godoc + docs** (spec § Desired Behavior 5 + § Acceptance Criteria require zero per-repo `.pr-reviewer.yaml` strings under `agent/pr-reviewer/`):
   - `agent/pr-reviewer/pkg/poster_types.go` line 24 godoc: `no .pr-reviewer.yaml lookup needed for LGTM` → `no .maintainer.yaml lookup needed for LGTM`.
   - `agent/pr-reviewer/pkg/factory/factory.go` line 37 godoc: `per-repo .pr-reviewer.yaml (autoApprove: bool)` → `per-repo .maintainer.yaml (prReviewer.autoApprove: bool)`.
   - `agent/pr-reviewer/docs/pr-post-back.md` line 21: `read \`.pr-reviewer.yaml\` for \`autoApprove\` config` → `read \`.maintainer.yaml\` for \`prReviewer.autoApprove\` config`.
   - `agent/pr-reviewer/docs/pr-post-back.md` line 41: `autoApprove config read (.pr-reviewer.yaml in worktree)` → `autoApprove config read (.maintainer.yaml in worktree)`.

5. **Rewrite `agent/pr-reviewer/pkg/githubposter/config_test.go`** to test the new reader against `.maintainer.yaml: prReviewer.autoApprove`. Replace the whole `Describe` body:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package githubposter_test

   import (
       "context"
       "os"
       "path/filepath"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubposter"
   )

   var _ = Describe("ReadAutoApprove", func() {
       var ctx context.Context
       var tmpDir string

       BeforeEach(func() {
           ctx = context.Background()
           var err error
           tmpDir, err = os.MkdirTemp("", "pr-reviewer-config-test-*")
           Expect(err).NotTo(HaveOccurred())
           DeferCleanup(os.RemoveAll, tmpDir)
       })

       DescribeTable(
           "maintainer.yaml variants",
           func(content string, writeFile bool, expected bool, expectErr bool, errContains string) {
               if writeFile {
                   err := os.WriteFile(
                       filepath.Join(tmpDir, ".maintainer.yaml"),
                       []byte(content),
                       0600,
                   )
                   Expect(err).NotTo(HaveOccurred())
               }
               autoApprove, err := githubposter.ReadAutoApprove(ctx, tmpDir)
               if expectErr {
                   Expect(err).To(HaveOccurred())
                   Expect(err.Error()).To(ContainSubstring(errContains))
               } else {
                   Expect(err).NotTo(HaveOccurred())
                   Expect(autoApprove).To(Equal(expected))
               }
           },
           Entry("file missing -> false, no error",
               "", false, false, false, ""),
           Entry("prReviewer.autoApprove: true -> true",
               "prReviewer:\n  autoApprove: true\n", true, true, false, ""),
           Entry("prReviewer.autoApprove: false -> false",
               "prReviewer:\n  autoApprove: false\n", true, false, false, ""),
           Entry("prReviewer key absent -> false",
               "release:\n  autoRelease: true\n", true, false, false, ""),
           Entry("only release populated (no prReviewer) -> false",
               "release:\n  autoRelease: true\n", true, false, false, ""),
           Entry("malformed YAML -> wrapped error",
               "prReviewer:\n  autoApprove: [unclosed\n", true, false, true, "parse .maintainer.yaml"),
       )
   })
   ```

   Notes:
   - These cases map directly to the spec § Failure Modes rows: missing→false; true→true; false→false; key absent→false; only release→false; malformed→wrapped error.
   - The malformed case proves the loud failure (spec § Security).

6. **Rewrite the auto-approve decision path test in `agent/pr-reviewer/pkg/githubposter/poster_test.go`.** Replace the `writeYAML` helper (lines ~127-132):

   old:
   ```go
   writeYAML := func(autoApprove bool) {
       content := fmt.Sprintf("autoApprove: %v\n", autoApprove)
       Expect(
           os.WriteFile(filepath.Join(tmpDir, ".pr-reviewer.yaml"), []byte(content), 0600),
       ).To(Succeed())
   }
   ```

   new:
   ```go
   writeYAML := func(autoApprove bool) {
       content := fmt.Sprintf("prReviewer:\n  autoApprove: %v\n", autoApprove)
       Expect(
           os.WriteFile(filepath.Join(tmpDir, ".maintainer.yaml"), []byte(content), 0600),
       ).To(Succeed())
   }
   ```

   This keeps the existing `DescribeTable("verdict to event/state mapping", …)` intact — it now exercises the auto-approve DECISION (verdict + autoApprove → approving review vs comment-only) against the new file + key (spec § Desired Behavior 7). Do not change the table entries themselves; only the file the helper writes.

7. **Tidy modules.** From `agent/pr-reviewer/`:

   ```bash
   go mod tidy
   ```

   to record the direct dependency on `lib/maintainerconfig`.

8. **Verify no per-repo `.pr-reviewer.yaml` reference remains.** Run:

   ```bash
   grep -rn "\.pr-reviewer\.yaml\|AutoApproveConfig" agent/pr-reviewer/pkg/
   grep -rn "pr-reviewer\.yaml" agent/pr-reviewer/ | grep -v "config/maintainer/pr-reviewer.yaml"
   ```

   First grep must be empty (exit 1). Second grep (whole agent dir, excluding the operator-CLI global config) must ALSO be empty — this matches the spec § Acceptance Criteria whole-dir check, catching `pkg/factory/factory.go` and `docs/pr-post-back.md`. If any stray per-repo reference remains, fix it. The operator-CLI `~/.config/maintainer/pr-reviewer.yaml` references in `cmd/cli/main.go` and `pkg/config.go` are OUT OF SCOPE and stay — they are the global operator config per spec § Non-goals, and the `grep -v` above intentionally excludes them.

9. **Final verification** — from `agent/pr-reviewer/`:

   ```bash
   make precommit
   ```

   Must exit 0. No `fmt.Errorf` in `pkg/githubposter/`.

</requirements>

<constraints>
- The agent reads `.maintainer.yaml: prReviewer.autoApprove` EXCLUSIVELY from the cloned workDir, parsed via `github.com/bborbe/maintainer/lib/maintainerconfig`. (Spec § Goal, § Desired Behavior 3.)
- Clean break: the `.pr-reviewer.yaml` reader behavior, the `AutoApproveConfig` type, and the per-repo `.pr-reviewer.yaml` filepath constant/string are REMOVED. NO fallback to `.pr-reviewer.yaml`. (Spec § Desired Behavior 5, § Non-goals: no backward-compat fallback, no read-both transition.)
- Missing `.maintainer.yaml` → `autoApprove: false`, nil error (comment-only). Malformed YAML → wrapped error (fail loudly, NOT silently false). (Spec § Failure Modes, § Security.)
- Auto-approve SEMANTICS unchanged: `true` → approving review on `approve` verdict; absent/false → comment-only. (Spec § Non-goals.)
- Do NOT add any `prReviewer` field beyond `autoApprove`. (Spec § Non-goals.)
- Do NOT touch the operator-side `~/.config/maintainer/pr-reviewer.yaml` references in `cmd/cli/main.go` and `pkg/config.go` — out of scope, keeps its name. (Spec § Non-goals, § Acceptance Criteria.)
- Error wrapping via `github.com/bborbe/errors` (`errors.Wrapf(ctx, err, …)`). `fmt.Errorf` BANNED on production paths. (Spec § Constraints.)
- Tests stay Ginkgo v2 + Gomega, external `_test` package. (Spec § Constraints.)
- The pr-reviewer task/frontmatter contract is unchanged. (Spec § Constraints.)
- Coverage on `agent/pr-reviewer/pkg/...` stays ≥80%. (Spec § Constraints.)
- License header preserved on every edited `.go` file.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/pr-reviewer && make test` green after the change.
</constraints>

<verification>

Run from repo root unless noted.

```bash
# Build + tests
cd agent/pr-reviewer && make precommit                                       # exit 0
cd agent/pr-reviewer && go test -cover ./pkg/...                             # >= 80%
cd agent/pr-reviewer && go test ./pkg/githubposter/...                       # exit 0

# Per-repo .pr-reviewer.yaml + AutoApproveConfig fully removed from pkg
grep -rn "\.pr-reviewer\.yaml\|AutoApproveConfig" agent/pr-reviewer/pkg/     # empty (exit 1)

# Per-repo .pr-reviewer.yaml gone from WHOLE agent dir (operator global config excluded)
grep -rn "pr-reviewer\.yaml" agent/pr-reviewer/ | grep -v "config/maintainer/pr-reviewer.yaml"  # empty

# Agent now reads .maintainer.yaml in githubposter
grep -rn "\.maintainer\.yaml" agent/pr-reviewer/pkg/githubposter/           # non-empty

# Shared lib imported
grep -n "lib/maintainerconfig" agent/pr-reviewer/pkg/githubposter/config.go # present

# Operator-CLI global config left untouched (out of scope, MUST still be present)
grep -c "config/maintainer/pr-reviewer.yaml" agent/pr-reviewer/cmd/cli/main.go agent/pr-reviewer/pkg/config.go   # each >= 1

# Error-wrapping convention (production files only — test mock RoundTripper
# in poster_test.go legitimately uses fmt.Errorf and is out of scope)
grep -c "fmt.Errorf" agent/pr-reviewer/pkg/githubposter/config.go agent/pr-reviewer/pkg/githubposter/poster.go   # each =0
```

</verification>
