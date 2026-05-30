---
status: completed
summary: Removed `git clone --branch <ref>` from Clone method; Clone now shells out `git clone --depth 1 <cloneURL> <workdir>` to fetch the remote default-branch HEAD; the `ref` parameter is retained on the signature and logged for traceability. Updated doc comment, nosec justification, and boundary test to assert default-branch content is cloned when ref is a non-branch string.
container: maintainer-clone-https-exec-212-fix-github-releaser-clone-default-branch
dark-factory-version: v0.173.0
created: "2026-05-30T00:00:00Z"
queued: "2026-05-30T07:57:48Z"
started: "2026-05-30T07:58:09Z"
completed: "2026-05-30T08:01:04Z"
---

<summary>
- The github-releaser agent fails to clone repositories when the trigger ref is a commit SHA instead of a branch/tag name.
- A live dev e2e showed `git clone --branch <SHA>` aborting with `fatal: Remote branch <SHA> not found in upstream origin`.
- A release always operates on the default branch HEAD (rewrites Unreleased, commits on top, pushes a fast-forward), so cloning a specific ref is wrong by design.
- Fix: clone the remote's default-branch HEAD instead of cloning a named ref.
- The trigger ref stays in the success log line for traceability (it identifies what fired the release, not the clone target).
- No interface, mock, config, or signature changes — the `ref` parameter remains on the Clone method.
- The real-git Clone test is updated to clone the default branch and assert default-branch content landed, dropping the obsolete `--branch` assertion.
- Scope is intentionally tiny: one method's args plus its doc/log comments and the one Clone test.
</summary>

<objective>
Fix the github-releaser clone bug where `git clone --branch <ref>` fails when the watcher emits a commit SHA as `ref`. Clone the remote default-branch HEAD instead, keeping the `ref` parameter for traceability only.
</objective>

<context>
Read `agent/github-releaser/CLAUDE.md` (and the repo-root `CLAUDE.md` if present) for project conventions before editing.

Read these files first and confirm the exact current code:
- `agent/github-releaser/pkg/git/os_exec_git_ops.go` — the `Clone` method. Confirm the exact `exec.CommandContext` argument slice, the `#nosec G204` comment, the leading doc comment, and the success-log line.
- `agent/github-releaser/pkg/git/os_exec_git_ops_test.go` — the existing real-`git`-binary boundary tests. Confirm the `It("Clone fetches a known branch ...")` block and how the sibling Commit/Tag/Push tests build local repos with the real git binary (LookPath skip, temp dirs, `git init -b master`, seed commit, local bare/source repos).

Background on the bug (from a real dev run): the watcher emits `ref` = a commit SHA (e.g. `5f2118ac…`). `git clone --branch` only accepts a branch or tag name, never a commit SHA, so it fails with `fatal: Remote branch 5f2118ac… not found in upstream origin`. A release operates on the default branch HEAD: the agent rewrites `## Unreleased`, commits on top, and pushes a fast-forward to the default branch. Therefore the correct clone target is the default-branch HEAD, not the trigger ref.

Conventions reference (read if unfamiliar):
- /home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md
- /home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md
</context>

<requirements>
1. Edit `agent/github-releaser/pkg/git/os_exec_git_ops.go`, method `Clone(ctx context.Context, cloneURL, ref, workdir string) error`:
   a. Remove the `"--branch",` and `ref,` entries from the `exec.CommandContext` argument slice. The resulting command MUST be exactly:
      ```go
      cmd := exec.CommandContext(
          ctx,
          "git",
          "clone",
          "--depth",
          "1",
          cloneURL,
          workdir,
      )
      ```
      Keep `--depth` and `1` (shallow clone is still correct — we only rewrite CHANGELOG and push a single commit + tag).
   b. KEEP the `ref` parameter on the method signature unchanged. KEEP the existing success-log line using `ref`:
      ```go
      glog.V(2).Infof("git clone succeeded: ref=%s workdir=%s", ref, workdir)
      ```
      The `ref` here is the trigger SHA for traceability, NOT the clone target.
   c. Update the leading doc comment (currently begins `// git clone --branch <ref> --depth 1 <cloneURL> <workdir>`) to reflect the new behavior. Replace it so it documents that we clone the remote's default-branch HEAD, that `--depth 1` is acceptable because we only rewrite CHANGELOG and push a single commit + tag, and that `ref` is informational (the trigger SHA, not the clone target — a release operates on default-branch HEAD which the push must fast-forward).
   d. Update the `#nosec G204` comment if its wording references `--branch` or treats `ref` as the clone target. The current comment ends with `ref validated by caller`; revise the trailing clause so it no longer implies `ref` is passed to git as a clone target (e.g. note that `ref` is logged only / not passed to the git clone invocation). Keep the rest of the justification (cloneURL constructed in caller from validated frontmatter; workdir is os.TempDir-rooted).

2. Do NOT change the `GitOps` interface, the counterfeiter mock, or any other method (`Commit`, `Tag`, `Push`). The `ref` parameter stays on the interface and the mock — they are unchanged.

3. Update the Clone boundary test in `agent/github-releaser/pkg/git/os_exec_git_ops_test.go` (the `It("Clone fetches a known branch into an empty workdir via --branch <ref> --depth 1", ...)` block):
   a. Rename the spec text to describe the new behavior, e.g. `It("Clone fetches the default-branch HEAD into an empty workdir via --depth 1", func() {`.
   b. Keep the real-git fixture pattern already used in the file: build a local source repo with the real `git` binary, seed a known file + commit on the default branch (`git init -b master`, write a marker file, `git add`, `git -c user.name=Test -c user.email=test@example.com commit -m seed`).
   c. Call `ops.Clone(ctx, source, "release-branch", dest)` — i.e. pass a `ref` value that is NOT the source repo's branch name, to prove the `ref` argument is ignored as a clone target and the default-branch HEAD is cloned regardless. (Any non-empty string is acceptable for `ref`; do not create a branch matching it.)
   d. Assert the known default-branch file landed in `dest` (read the marker file, expect its content) — this proves the default-branch HEAD content was cloned.
   e. REMOVE the assertion that the checked-out branch equals `release-branch` (the `git rev-parse --abbrev-ref HEAD` equals `"release-branch"` check). It is now obsolete because `--branch` is gone. You may keep a `git -C dest rev-parse HEAD` non-empty assertion to confirm HEAD resolves.

4. Do NOT touch the watcher, the Dockerfile, or the SSH→HTTPS clone-URL normalizer (a separate fix already on this branch). Do NOT add any new config knob. Do NOT add SSH support.

5. Preserve the BSD license header at the top of both files (do not remove or alter lines 1-3).
</requirements>

<constraints>
- Error wrapping: use `github.com/bborbe/errors` context-form only (`errors.Errorf(ctx, ...)` / `errors.Wrap(ctx, err, ...)`). NO `fmt.Errorf`. The existing Clone error path already uses `errors.Errorf` — leave it intact.
- NO `context.Background()` in business logic (test files may use it as they already do).
- Tests: Ginkgo v2 + Gomega, external `_test` package (`package git_test`), counterfeiter v6. The GitOps interface and its generated mock stay UNCHANGED — the `ref` parameter remains.
- Minimal change only: the `Clone` argument slice + its doc/`#nosec`/log comments, and the one Clone test. No other files.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run `make precommit` in `agent/github-releaser/` — must pass (exit 0).

Evidence-shape checks (run from repo root):
- `grep -n '"--branch"' agent/github-releaser/pkg/git/os_exec_git_ops.go` returns 0 matches (the `--branch` arg was removed from Clone).
- `grep -n '"--depth"' agent/github-releaser/pkg/git/os_exec_git_ops.go` still matches in Clone (shallow clone retained).
- `grep -n 'ref=%s workdir=%s' agent/github-releaser/pkg/git/os_exec_git_ops.go` still matches (success log keeps `ref` for traceability).
- The Clone boundary test asserts default-branch HEAD content (marker file content) and contains NO assertion comparing the checked-out branch to `"release-branch"`.
- `func (g *osExecGitOps) Clone(ctx context.Context, cloneURL, ref, workdir string) error` signature is unchanged (the `ref` parameter is still present).
- The `GitOps` interface and the counterfeiter mock are unchanged in `git diff`.
</verification>
