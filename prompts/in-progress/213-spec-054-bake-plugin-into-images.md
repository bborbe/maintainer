---
status: approved
spec: ["054"]
created: "2026-05-30T12:45:00Z"
queued: "2026-05-30T18:56:57Z"
---

<summary>
- The PR-reviewer and github-releaser container images will carry the `bborbe/coding` plugin baked in at build time, so every pod has it without mounting any shared volume.
- After this change, running either freshly built image with no volume mounted still exposes the `/coding:pr-review` slash command.
- Plugin updates now ship via image rebuild instead of a runtime download at pod startup.
- A build that cannot reach the plugin marketplace (github.com) fails loudly instead of silently producing an image without the plugin.
- No Go code, no entrypoint, no auth, and no review behavior changes — this is purely a Dockerfile change applied identically to both agents.
</summary>

<objective>
Bake the `bborbe/coding` Claude Code plugin into both agent images at build time so each pod has the `/coding:pr-review` command available from the image alone, with no mounted volume. This is the prerequisite for removing the shared PVC and enabling concurrency (done in the next prompt).
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before changing anything (paths are repo-relative; the container starts at the repo root):
- `agent/pr-reviewer/Dockerfile`
- `agent/github-releaser/Dockerfile`
- `docs/claude-plugin-cli.md` (authoritative plugin CLI conventions for this repo)

Current state (verified): both Dockerfiles are byte-identical. The final stage does:
```
ENV HOME=/home/claude
RUN mkdir -p /home/claude/.claude
```
Neither image sets `CLAUDE_CONFIG_DIR` — that env var is supplied by the Config CR at pod runtime as `/home/claude/.claude`. `@anthropic-ai/claude-code` is installed globally via npm in the `alpine` deps stage, and `git`, `github-cli`, `bash`, `curl`, `ca-certificates` are present.

Plugin facts (from `docs/claude-plugin-cli.md`, verified):
- Marketplace slug: `bborbe/coding` (GitHub repo).
- Marketplace alias = last path segment of slug = `coding`.
- Add marketplace: `claude plugin marketplace add bborbe/coding`.
- Install plugin: `claude plugin install coding`.
- Enabled-plugin identifier: `coding@coding`. After install, settings record `enabledPlugins: {"coding@coding": true}`.
- Plugins land under `$CLAUDE_CONFIG_DIR/plugins/`; the marketplace clone lives under `$CLAUDE_CONFIG_DIR/plugins/marketplaces/<alias>/`; the installed plugin's commands resolve from a `commands/` directory under that clone.
</context>

<requirements>
1. Edit `agent/pr-reviewer/Dockerfile`. In the FINAL stage (`FROM alpine`), bake the plugin into `/home/claude/.claude` at build time using the `claude` CLI. Replace the single `RUN mkdir -p /home/claude/.claude` line with a bake step. Use the install mechanism from `docs/claude-plugin-cli.md` (preferred — it produces the canonical on-disk layout the runtime expects):

   ```dockerfile
   ENV HOME=/home/claude
   ENV CLAUDE_CONFIG_DIR=/home/claude/.claude
   RUN set -eux \
    && mkdir -p /home/claude/.claude \
    && timeout 300 claude plugin marketplace add bborbe/coding \
    && timeout 300 claude plugin install coding \
    && claude plugin list | grep -q coding
   ```

   Notes:
   - The trailing `claude plugin list | grep -q coding` makes the build FAIL (non-zero exit under `set -e`) if the plugin did not install — this satisfies the spec's "fail the build, never silently produce a pluginless image" constraint and the "Marketplace unreachable at build" failure mode.
   - The `timeout 300` wrapping the network steps makes a hung/slow github.com fail the build fast instead of blocking the build indefinitely.
   - `ENV CLAUDE_CONFIG_DIR=/home/claude/.claude` MUST be set in the image so the build-time `claude plugin install` writes to the exact path the pod's `CLAUDE_CONFIG_DIR` env (from the Config CR) will point to. Setting it in the image is harmless — the Config CR sets the identical value at runtime. Do NOT use a different path.
   - Keep `ENV HOME=/home/claude` (it already exists; the bake must run under this HOME).
   - If `claude plugin marketplace add bborbe/coding` requires a different invocation in this CLI version, adjust per `docs/claude-plugin-cli.md`, but the end result must be: plugin installed under `/home/claude/.claude/plugins/`, `enabledPlugins` recording `coding@coding`, and a `pr-review` command file present under the installed marketplace's `commands/` directory.

2. Do NOT change the golang `build` stage, the `alpine` deps stage (`@anthropic-ai/claude-code` install, apk packages), the `COPY --from=build /main /main`, `COPY agent/ /agent/`, the zoneinfo lines, the `BUILD_GIT_COMMIT`/`BUILD_DATE` args/env, or the `ENTRYPOINT ["/main", "-v=2"]`.

3. Apply the IDENTICAL change to `agent/github-releaser/Dockerfile`. The two Dockerfiles are byte-identical today and must remain byte-identical after this change. (If you find a stale "baked into image" comment in either file, remove it and replace with the real bake.)

4. Do NOT hardcode the plugin command-file path. The exact nesting `claude plugin install` produces in-container is the source of truth (the host cache nests it deeper than a naive guess, e.g. `.../marketplaces/coding/plugins/coding/commands/pr-review.md`). The Dockerfile's own gate (`claude plugin list | grep -q coding`) is the build-time correctness check; the command-file path is asserted empirically with `find` in the verification block below.
</requirements>

<constraints>
- Bake runs in the FINAL image build under `HOME=/home/claude`; it must write to `/home/claude/.claude`.
- The build reaches github.com at BUILD time only; pod RUNTIME must NOT require github.com to load the plugin. A build that cannot reach the marketplace MUST fail the build (the trailing `grep -q coding` under `set -e` enforces this; `timeout` keeps the failure fast).
- Keep the existing multi-stage Dockerfile structure (golang build → alpine deps → final). Do NOT change the Go build, the `@anthropic-ai/claude-code` install, or the entrypoint.
- Plugin tracks marketplace latest at build time, frozen for the life of the image tag (acceptable — rebuild to refresh). Do NOT pin a plugin version.
- Apply the identical change to BOTH `agent/pr-reviewer/Dockerfile` and `agent/github-releaser/Dockerfile`.
- No Go behavior changes — do not touch any `.go` file.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Build and inspect each image with NO volume mounted and the runtime env pinned (matches pod runtime, not the build-shell defaults). Run from the repo root:

```
docker build -t parallel-test-pr-reviewer agent/pr-reviewer
docker run --rm -e HOME=/home/claude -e CLAUDE_CONFIG_DIR=/home/claude/.claude parallel-test-pr-reviewer claude plugin list | grep -q coding && echo "PLUGIN OK"
# assert the pr-review command file exists at whatever path the install produced:
docker run --rm parallel-test-pr-reviewer sh -c "find /home/claude/.claude/plugins -name 'pr-review*' | grep -q . && echo 'CMD OK'" || { echo 'pr-review command file missing'; exit 1; }

docker build -t parallel-test-github-releaser agent/github-releaser
docker run --rm -e HOME=/home/claude -e CLAUDE_CONFIG_DIR=/home/claude/.claude parallel-test-github-releaser claude plugin list | grep -q coding && echo "PLUGIN OK"
docker run --rm parallel-test-github-releaser sh -c "find /home/claude/.claude/plugins -name 'pr-review*' | grep -q . && echo 'CMD OK'" || { echo 'pr-review command file missing'; exit 1; }
```

Confirm the two Dockerfiles are byte-identical:
```
diff agent/pr-reviewer/Dockerfile agent/github-releaser/Dockerfile   # expect: no output
```

No Go changes expected; if any Go service dir was touched, run `make precommit` there (expected: N/A).
</verification>
