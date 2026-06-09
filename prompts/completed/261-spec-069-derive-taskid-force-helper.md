---
status: approved
spec: ["069"]
created: "2026-06-09T20:30:00Z"
queued: "2026-06-09T20:18:04Z"
---

<summary>
- Adds a small, pure helper that builds a salted task identifier for build-failure episodes, alongside the existing canonical helper
- The salted variant is what later prompts will use when an operator forces a re-publish via `?force=true` on the github-build watcher
- For the same repo and episode SHA, the canonical and salted identifiers MUST differ, and two different nonces MUST produce two different salted identifiers
- The canonical helper, its UUID namespace, and its input encoding are deliberately left untouched so today's non-force path stays byte-identical
- Table tests in a new Ginkgo suite cover three properties: distinct-from-canonical, stable-for-same-nonce, distinct-across-nonces
- No callers wired yet — this prompt is intentionally a small additive diff
</summary>

<objective>
Add `DeriveTaskIDForce(owner, repo, episodeSHA, nonce string) uuid.UUID` to `watcher/github-build/pkg/taskid.go` so a later prompt can publish a salted `TaskIdentifier` on the force path. Reuse the existing `buildWatcherNamespace`. Key format `<owner>/<repo>#build-<episodeSHA>!<nonce>` — `!` is invalid in GitHub owner/repo names and SHAs, so the salted keyspace is collision-free with the canonical one. Pure helper, no wiring, three table tests.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these coding plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega conventions used in this repo
- `/home/node/.claude/plugins/marketplaces/coding/docs/dod.md` — Definition of Done (make precommit, no commit)

Read the existing helper and its callers (verify before writing):
- `watcher/github-build/pkg/taskid.go` — current `DeriveTaskID`, `buildWatcherNamespace`. Do NOT modify either.
- `watcher/github-build/pkg/watcher.go` (around `applyStateMachine`, line ~226) — `DeriveTaskID(owner, repo, episodeSHA)` is the only caller of the canonical helper today

Read the sibling helper for shape reference (READ-ONLY, do not import or copy verbatim — this is a separate worktree):
- `/Users/bborbe/Documents/workspaces/maintainer-trigger-force/watcher/github-pr/pkg/taskid.go` — `DeriveTaskIDForce(owner, repo string, number int, sha, nonce string) uuid.UUID`. github-build's signature drops `number` and uses `episodeSHA` for the SHA segment.

Key facts (verified by reading the files):
- `buildWatcherNamespace = uuid.MustParse("8e3f5a2c-7b14-4d9e-a017-3c6e8b9f2a1d")` — frozen, do NOT touch
- Canonical key is built by string concatenation: `owner + "/" + repo + "#build-" + episodeSHA` — do NOT switch to `fmt.Sprintf` for the canonical helper
- `uuid.NewSHA1(namespace, []byte(key))` is the SHA1-based v5 UUID constructor — same call for the salted variant
- The package already imports `"github.com/google/uuid"`; no new import needed for the helper itself
- The package has no existing test file for taskid; you will create `watcher/github-build/pkg/taskid_test.go` and a suite bootstrap file is already present at `watcher/github-build/pkg/suite_test.go` (verify with `ls watcher/github-build/pkg/suite_test.go`)
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Edit `watcher/github-build/pkg/taskid.go`** — append the new helper at the end of the file (keep `DeriveTaskID` and `buildWatcherNamespace` exactly as-is). Use this exact body:

   ```go
   // DeriveTaskIDForce returns a salted task identifier for a build-failure
   // episode plus an extra nonce. Used when an operator explicitly requests
   // a forced re-publish (HTTP /trigger?force=true) so the watcher emits a
   // CreateTaskCommand with a TaskIdentifier that the controller has not
   // already seen — bypassing the agent's deterministic-ID dedup-skip.
   //
   // For the same (owner, repo, episodeSHA) the result is always different
   // from DeriveTaskID(...): the key includes a "!<nonce>" suffix, e.g.
   // "bborbe/maintainer#build-abc123!1700000000000000".
   //
   // The "!" separator is intentionally invalid in GitHub owner/repo names
   // and in hex SHAs, so no canonical input can ever collide with a salted
   // key by construction.
   //
   // The nonce resolution is the caller's responsibility. Callers should
   // derive it from an injected libtime.CurrentDateTimeGetter; the helper
   // itself is a pure function over its inputs.
   func DeriveTaskIDForce(owner, repo, episodeSHA, nonce string) uuid.UUID {
       key := owner + "/" + repo + "#build-" + episodeSHA + "!" + nonce
       return uuid.NewSHA1(buildWatcherNamespace, []byte(key))
   }
   ```

   Constraints:
   - Signature MUST be exactly `func DeriveTaskIDForce(owner, repo, episodeSHA, nonce string) uuid.UUID` — no `ctx`, no error return, no `fmt.Sprintf`.
   - The key MUST be built by string concatenation matching the format above (NOT `fmt.Sprintf`) so the per-call cost stays equivalent to `DeriveTaskID` and the byte sequence is verifiable by eye.
   - Reuse `buildWatcherNamespace`. Do NOT introduce a second namespace.

2. **Append to the EXISTING `watcher/github-build/pkg/taskid_test.go`.** The file already exists with 4 `Describe("DeriveTaskID", ...)` specs (canonical helper tests). DO NOT recreate the file, DO NOT redeclare `package pkg_test`, DO NOT duplicate imports — the imports `ginkgo/v2`, `gomega`, and `github.com/bborbe/maintainer/watcher/github-build/pkg` are already present. Append a new top-level `var _ = Describe("DeriveTaskIDForce", func() { ... })` block at file scope, AFTER the existing `DeriveTaskID` block. The new block alone:

   ```go
   var _ = Describe("DeriveTaskIDForce", func() {
       const (
           owner      = "bborbe"
           repo       = "maintainer"
           episodeSHA = "abc123def456abc123def456abc123def456abcd"
       )

       It("differs from DeriveTaskID for the same (owner, repo, episodeSHA)", func() {
           canonical := pkg.DeriveTaskID(owner, repo, episodeSHA)
           salted := pkg.DeriveTaskIDForce(owner, repo, episodeSHA, "x")
           Expect(salted.String()).NotTo(Equal(canonical.String()))
       })

       It("is stable for the same (owner, repo, episodeSHA, nonce)", func() {
           a := pkg.DeriveTaskIDForce(owner, repo, episodeSHA, "1700000000000000")
           b := pkg.DeriveTaskIDForce(owner, repo, episodeSHA, "1700000000000000")
           Expect(a.String()).To(Equal(b.String()))
       })

       It("differs across nonces with the same (owner, repo, episodeSHA)", func() {
           a := pkg.DeriveTaskIDForce(owner, repo, episodeSHA, "1700000000000000")
           b := pkg.DeriveTaskIDForce(owner, repo, episodeSHA, "1700000000000001")
           Expect(a.String()).NotTo(Equal(b.String()))
       })
   })
   ```

   Notes:
   - The Ginkgo `Describe` titles (`"DeriveTaskIDForce"`) plus the three `It` strings are what the AC greps target via `go test -run DeriveTaskIDForce`.
   - Use external `pkg_test` package so the test exercises only the exported surface.
   - Do NOT add an extra `func TestXxx` bootstrap here — the existing `watcher/github-build/pkg/suite_test.go` already registers the Ginkgo runner (verify with `ls watcher/github-build/pkg/suite_test.go`; if missing for some reason, add a minimal `TestPkg(t *testing.T)` runner in this same file).

3. **Run precommit** from the service directory:

   ```bash
   cd watcher/github-build && make precommit
   ```

   Must exit 0. If it fails on the new test file's import path, check that the module path is `github.com/bborbe/maintainer` (top-level `go.mod`) and adjust nothing else.

4. **Sanity-grep the new helper exists**:

   ```bash
   grep -nE 'func DeriveTaskIDForce' watcher/github-build/pkg/taskid.go
   ```

   Must return exactly one line.

</requirements>

<constraints>
- Only edit `watcher/github-build/pkg/taskid.go` and create `watcher/github-build/pkg/taskid_test.go`. No other files.
- Do NOT commit — dark-factory handles git.
- Do NOT modify `DeriveTaskID`, `buildWatcherNamespace`, or the canonical key encoding. Non-force path must remain bit-identical.
- Do NOT add a second namespace UUID. Reuse `buildWatcherNamespace`.
- Do NOT use `fmt.Sprintf` for the salted key — match the canonical concatenation style.
- Do NOT wire the new helper to any caller in this prompt. Wiring happens in prompt 2.
- Use `github.com/bborbe/errors` for any error wrapping in tests (not expected here — pure-function tests have no error returns).
- Ginkgo v2 + Gomega per `go-testing-guide.md`. External `pkg_test` package.
- `make precommit` runs from `watcher/github-build/`, never from repo root.
</constraints>

<verification>
cd watcher/github-build && make precommit
grep -nE 'func DeriveTaskIDForce\(owner, repo, episodeSHA, nonce string\) uuid\.UUID' pkg/taskid.go
# Expect: one line.

go test ./pkg -run DeriveTaskIDForce -v
# Expect: three PASS lines, one per It.

# Confirm canonical helper is untouched:
grep -nE 'buildWatcherNamespace = uuid\.MustParse\("8e3f5a2c-7b14-4d9e-a017-3c6e8b9f2a1d"\)' pkg/taskid.go
grep -nE 'func DeriveTaskID\(owner, repo, episodeSHA string\) uuid\.UUID' pkg/taskid.go
# Both expected: one line each.
</verification>
