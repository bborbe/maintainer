---
status: completed
spec: [053-migrate-pr-reviewer-to-maintainer-yaml]
summary: Created shared lib/maintainerconfig package defining .maintainer.yaml schema with release and prReviewer namespaces and pure Parse function
container: maintainer-pr-reviewer-yaml-exec-207-spec-052-maintainerconfig-lib
dark-factory-version: v0.173.0
created: "2026-05-29T15:30:00Z"
queued: "2026-05-29T16:26:53Z"
started: "2026-05-29T16:27:18Z"
completed: "2026-05-29T16:30:00Z"
---

<summary>
- Creates one shared definition of the `.maintainer.yaml` schema that every maintainer bot will parse, instead of each bot rolling its own copy.
- The schema covers both bot namespaces that exist today: the release gate (`release.autoRelease`) and the new pr-reviewer gate (`prReviewer.autoApprove`).
- Adds a pure parse function that turns raw YAML bytes into the parsed config and surfaces malformed YAML as a wrapped error — it does no file or network I/O (each consumer fetches the bytes its own way).
- Unknown top-level keys (e.g. a future `build-fix:`) are silently tolerated so adding the next bot is a one-line schema edit, not a parser rewrite.
- Ships a thorough Ginkgo test suite covering empty input, each namespace alone, both together, an unknown key, and malformed YAML.
- This package is the foundation; the two consuming bots are migrated onto it in the following prompts.
</summary>

<objective>
Create a shared package `lib/maintainerconfig` (plain package inside the existing `lib` Go module `github.com/bborbe/maintainer/lib`) that defines the single `.maintainer.yaml` schema with both the `release` and `prReviewer` namespaces and a pure `Parse([]byte)` function. This package is the one source of truth for the file's shape; the github-release watcher (prompt 2) and the pr-reviewer agent (prompt 3) both import it.

End state: `cd lib && make precommit` exits 0; `go test -cover ./maintainerconfig/...` reports ≥90%.
</objective>

<context>
Read before writing code:

- `CLAUDE.md` at repo root — project conventions.
- `specs/in-progress/052-migrate-pr-reviewer-to-maintainer-yaml.md` — re-read Desired Behavior 1 & 2, Constraints, the Failure Modes table, the Security section, and Acceptance Criteria (the lib-suite criterion lists the exact 7 cases this prompt must cover: empty bytes; `prReviewer.autoApprove: true`; `prReviewer:` absent; `release.autoRelease: true`; both namespaces; unknown top-level key; malformed YAML).
- `lib/repoallowlist/repoallowlist.go` — the package-layout PRECEDENT: a plain package directly under `lib/`, no per-package Makefile, license header, `github.com/bborbe/errors` for wrapping. Mirror this layout exactly.
- `lib/repoallowlist/repoallowlist_suite_test.go` — the Ginkgo bootstrap shape to mirror for the new suite file (external `_test` package, `RegisterFailHandler(Fail)`, `RunSpecs`).
- `lib/go.mod` — confirms the module path `github.com/bborbe/maintainer/lib` and that `gopkg.in/yaml.v3` is already available (present in `lib/go.sum`). The local-watcher `MaintainerConfig` this package replaces was parsed with `gopkg.in/yaml.v3`; use the same import here so YAML behavior is identical.
- `watcher/github-release/pkg/githubclient.go` lines 254-284 — the EXISTING local `MaintainerConfig`, `MaintainerReleaseConfig`, and `parseMaintainerConfig`. The new lib types are these two structs (verbatim field names + yaml tags `release` / `autoRelease`) PLUS a new `PrReviewer` section. The new `Parse` is the existing `parseMaintainerConfig` body, moved to the lib and made exported + ctx-aware. Do NOT change the `release` semantics — only add the sibling namespace.

Coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` `Wrap(ctx, err, msg)` form. NO `fmt.Errorf`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + `DescribeTable`/`It` conventions, external `_test` package.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-parse-pattern.md` — pure-parse-function idiom (bytes in, typed value + error out, no I/O).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-library-guide.md` — plain library package conventions.
</context>

<requirements>

**Run order: create files in sequence, then run `cd lib && go test ./maintainerconfig/...`, then `cd lib && make precommit` as final verification.**

1. **Create `lib/maintainerconfig/maintainerconfig.go`** — the schema types and the pure parse function. Exact shape:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package maintainerconfig defines the single schema of the per-repo
   // `.maintainer.yaml` trust file shared by all maintainer bots, plus a
   // pure parser. Each top-level key is one bot's namespace:
   //
   //   release:
   //     autoRelease: true     # github-release watcher gate
   //   prReviewer:
   //     autoApprove: true     # pr-reviewer agent gate
   //
   // Adding the next bot (build-fix, dep-pin, …) is a one-field edit to
   // MaintainerConfig — every consumer imports this one type, so there is
   // never a divergent copy of the file's shape. Unknown top-level keys are
   // tolerated by design (yaml.Unmarshal ignores fields it does not know),
   // which is the forward-compat behavior the spec mandates.
   //
   // Parse does NO I/O — fetching the bytes is each consumer's job (the
   // watcher fetches via the GitHub API; the agent reads the cloned workDir
   // on disk).
   package maintainerconfig

   import (
       "context"

       "github.com/bborbe/errors"
       "gopkg.in/yaml.v3"
   )

   // MaintainerConfig is the parsed shape of `.maintainer.yaml`. Each field is
   // one bot's namespace; siblings are independent. A consumer reads only its
   // own namespace and ignores the rest.
   type MaintainerConfig struct {
       // Release is the github-release watcher namespace.
       Release ReleaseConfig `yaml:"release"`
       // PrReviewer is the pr-reviewer agent namespace.
       PrReviewer PrReviewerConfig `yaml:"prReviewer"`
   }

   // ReleaseConfig is the `release:` namespace. AutoRelease=true is the ONLY
   // shape that lets the github-release watcher emit a release task; everything
   // else (key absent, value false, file absent) skips the repo.
   type ReleaseConfig struct {
       AutoRelease bool `yaml:"autoRelease"`
   }

   // PrReviewerConfig is the `prReviewer:` namespace. AutoApprove=true means
   // "post an approving review on an approve verdict"; absence/false means
   // comment-only.
   type PrReviewerConfig struct {
       AutoApprove bool `yaml:"autoApprove"`
   }

   // Parse unmarshals a `.maintainer.yaml` document and returns the parsed
   // config. Pure data extraction — no I/O. Empty input returns a zero-value
   // MaintainerConfig with nil error. Malformed YAML returns a wrapped error
   // (NOT a silent zero-value) so callers can fail loudly.
   func Parse(ctx context.Context, content []byte) (MaintainerConfig, error) {
       var cfg MaintainerConfig
       if err := yaml.Unmarshal(content, &cfg); err != nil {
           return MaintainerConfig{}, errors.Wrap(ctx, err, "unmarshal .maintainer.yaml")
       }
       return cfg, nil
   }
   ```

   Notes:
   - The yaml tag for the new namespace MUST be camelCase `prReviewer` (per spec § Constraints) — NOT kebab `pr-reviewer`.
   - The `release` / `autoRelease` tags MUST stay byte-identical to the watcher's existing struct (`yaml:"release"` / `yaml:"autoRelease"`) so the watcher's behavior is unchanged after prompt 2 swaps the type.
   - Use `gopkg.in/yaml.v3` (already in `lib/go.sum`) — same package the watcher used, so unmarshal semantics are identical.
   - `errors.Wrap` from `github.com/bborbe/errors`. NO `fmt.Errorf`. The watcher's old private parser returned a bare error and let the caller wrap; here we wrap at the lib boundary AND the watcher caller will still add repo context on top (that double-wrap is intended and matches spec § Failure Modes "wrapped error").

2. **Create `lib/maintainerconfig/maintainerconfig_suite_test.go`** — Ginkgo bootstrap. Mirror `lib/repoallowlist/repoallowlist_suite_test.go` exactly:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package maintainerconfig_test

   import (
       "testing"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   func TestMaintainerconfig(t *testing.T) {
       RegisterFailHandler(Fail)
       RunSpecs(t, "Maintainerconfig Suite")
   }
   ```

3. **Create `lib/maintainerconfig/maintainerconfig_test.go`** — external `_test` package, covering ALL seven acceptance cases. Each must appear as a distinct named case so `go test -v` lists them:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package maintainerconfig_test

   import (
       "context"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/lib/maintainerconfig"
   )

   var _ = Describe("Parse", func() {
       var ctx context.Context

       BeforeEach(func() {
           ctx = context.Background()
       })

       DescribeTable("valid documents",
           func(content string, expected maintainerconfig.MaintainerConfig) {
               cfg, err := maintainerconfig.Parse(ctx, []byte(content))
               Expect(err).NotTo(HaveOccurred())
               Expect(cfg).To(Equal(expected))
           },
           Entry("empty bytes -> zero-value, nil",
               "",
               maintainerconfig.MaintainerConfig{}),
           Entry("prReviewer.autoApprove: true -> PrReviewer.AutoApprove true",
               "prReviewer:\n  autoApprove: true\n",
               maintainerconfig.MaintainerConfig{
                   PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
               }),
           Entry("prReviewer absent -> AutoApprove false",
               "release:\n  autoRelease: true\n",
               maintainerconfig.MaintainerConfig{
                   Release: maintainerconfig.ReleaseConfig{AutoRelease: true},
               }),
           Entry("release.autoRelease: true still parses -> Release.AutoRelease true",
               "release:\n  autoRelease: true\n",
               maintainerconfig.MaintainerConfig{
                   Release: maintainerconfig.ReleaseConfig{AutoRelease: true},
               }),
           Entry("both namespaces populated -> both parsed",
               "release:\n  autoRelease: true\nprReviewer:\n  autoApprove: true\n",
               maintainerconfig.MaintainerConfig{
                   Release:    maintainerconfig.ReleaseConfig{AutoRelease: true},
                   PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
               }),
           Entry("unknown top-level key ignored, no error",
               "build-fix:\n  enabled: true\nprReviewer:\n  autoApprove: true\n",
               maintainerconfig.MaintainerConfig{
                   PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
               }),
       )

       It("malformed YAML -> wrapped error", func() {
           cfg, err := maintainerconfig.Parse(ctx, []byte("prReviewer:\n  autoApprove: [unclosed\n"))
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
           Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
       })
   })
   ```

   Notes:
   - The "unknown top-level key ignored" case is the Security / forward-compat requirement (spec § Security: unknown keys tolerated, deserialized only into plain-data fields). Keep the `build-fix:` key in it.
   - The malformed-YAML case proves the wrapped error (spec § Failure Modes: malformed YAML surfaces, NOT silently zero-value).

4. **Coverage check** — from `lib/`:

   ```bash
   go test -cover ./maintainerconfig/...
   ```

   Must report ≥90% (spec § Constraints). The parser is tiny; the seven cases exercise both the success and the error branch, which is every line. If somehow below 90%, add one more `Entry` exercising a not-yet-hit shape (e.g. `prReviewer:\n  autoApprove: false\n`).

5. **Final verification** — from `lib/`:

   ```bash
   make precommit
   ```

   Must exit 0. No `fmt.Errorf` in `lib/maintainerconfig/`.

</requirements>

<constraints>
- New package path: `github.com/bborbe/maintainer/lib/maintainerconfig`. Plain package directly under `lib/` — NOT its own Go module, NOT its own Makefile. Built/tested via the `lib` module's top-level `make precommit`. (Spec § Constraints, mirroring the `lib/repoallowlist` precedent.)
- Files (exactly these in `lib/maintainerconfig/`): `maintainerconfig.go`, `maintainerconfig_suite_test.go`, `maintainerconfig_test.go`. No mocks, no per-package Makefile.
- The new namespace yaml key is camelCase `prReviewer` (struct tag `yaml:"prReviewer"`) — NOT kebab. The whole document stays camelCase. (Spec § Constraints.)
- The `release` / `autoRelease` tags are byte-identical to the watcher's existing struct so its behavior is unchanged.
- Error wrapping via `github.com/bborbe/errors` (`errors.Wrap(ctx, err, …)`). `fmt.Errorf` is BANNED on production paths. (Spec § Constraints.)
- `Parse` does NO I/O — bytes in, typed value + wrapped error out. (Spec § Desired Behavior 2.)
- Tests use Ginkgo v2 + Gomega, external `_test` package. (Spec § Constraints.)
- No `time.Now()` anywhere — time is not needed in this package. (Spec § Constraints.)
- Coverage ≥90% on `lib/maintainerconfig/`. (Spec § Constraints.)
- License header (3 lines) at the top of every `.go` file.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd lib && make test` green before AND after.
</constraints>

<verification>

Run from repo root unless noted.

```bash
# Build + tests + coverage
cd lib && make precommit                                   # exit 0
cd lib && go test -cover ./maintainerconfig/...            # >= 90%
cd lib && go test -v ./maintainerconfig/...                # lists all 7 cases, 0 failures

# Files exist
ls lib/maintainerconfig/maintainerconfig.go                # exists
ls lib/maintainerconfig/maintainerconfig_suite_test.go     # exists
ls lib/maintainerconfig/maintainerconfig_test.go           # exists

# All three fields present (spec AC)
grep -rn "PrReviewer\|AutoApprove\|AutoRelease" lib/maintainerconfig/   # all three appear

# camelCase yaml tag, NOT kebab
grep -c 'yaml:"prReviewer"' lib/maintainerconfig/maintainerconfig.go    # =1
grep -c 'pr-reviewer' lib/maintainerconfig/maintainerconfig.go          # =0

# Error-wrapping convention
grep -c 'fmt.Errorf' lib/maintainerconfig/                              # =0
```

</verification>
