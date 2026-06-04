---
status: completed
approved: "2026-05-29T16:14:45Z"
generating: "2026-05-29T16:22:07Z"
prompted: "2026-05-29T16:22:07Z"
verifying: "2026-05-29T16:34:51Z"
completed: "2026-06-04T21:31:43Z"
branch: dark-factory/github-releaser-app-auth
---

## Summary

- Migrate `agent/github-releaser` from PAT-only auth (`GH_TOKEN`) to **GitHub App installation-token auth ONLY**, reusing the same agent-lib mint helper as `agent/pr-reviewer`.
- App fields (`APP_ID` + `INSTALLATION_ID` + `PEM_KEY_FILE` or `PEM_KEY`) mint an installation access token at pod startup. **No PAT fallback** — if App credentials are absent, the binary errors before any clone. (The PAT credential type was retired fleet-wide; a dormant PAT path would only widen the auth surface on a push-capable agent.)
- This is the code half of "make the releaser deployable/runnable" — fleet-wide PAT (`GH_TOKEN`) auth was retired on 2026-05-24, so the agent cannot authenticate in the cluster until it speaks App auth.
- The single minted IAT flows to BOTH the planning fetcher (changelog HTTP fetch) AND the execution step's push, via the existing single-token wiring through the factory. (The IAT is still forwarded to the Claude/git subprocess as the `GH_TOKEN` env var — that is the credential *value*, not the retired PAT input path.)
- Code-only: this spec wires the binary to READ `APP_ID` / `INSTALLATION_ID` / `PEM_KEY*` from env/secret. Writing k8s manifests, creating the App, and granting branch-protection bypass are out of scope.

## Problem

The github-releaser agent (Phase 2) authenticates with `GH_TOKEN` only — the config field comment literally reads "PAT for now; App auth in a follow-up spec." On 2026-05-24, PAT (`GH_TOKEN`) authentication was retired fleet-wide, so the binary can no longer authenticate inside the cluster: every clone, changelog fetch, and push would fail. The pr-reviewer agent already solved this by minting a GitHub App installation access token at pod startup (resolving `APP_ID` + `INSTALLATION_ID` + `PEM_KEY_FILE`/`PEM_KEY`), and the releaser must adopt the identical resolution so the Config CR maps cluster env the same way. Until it does, the releaser is undeployable.

## Goal

After this work, the github-releaser binary authenticates **only** as a GitHub App installation: when App credentials are present it mints an IAT; when they are absent it errors clearly before any clone. There is no PAT fallback. The minted installation token reaches both the planning fetcher and the execution push, identically to the current single-token wiring. The App env-var names match the pr-reviewer agent exactly.

## Non-goals

- Do NOT add the pre-push diff guard or the `unexpected_diff` error category — SEPARATE follow-up hardening spec.
- Do NOT write k8s manifests, CRD, Secret, or deploy wiring — authored directly as yaml outside dark-factory. This spec only makes the code READ the env/secret values.
- Do NOT create the GitHub App, grant Contents write, or register the branch-protection bypass actor — operator GitHub-side action, not code.
- Do NOT implement the `ai_review` phase (separate spec).
- Do NOT split the fetch-token from the push-token — invariant; one minted token serves both. If a future consumer demands two identities, that is a separate spec.

## Desired Behavior

1. The binary accepts four new config fields with env names **identical to pr-reviewer**: `APP_ID` (int64, env `APP_ID`), `INSTALLATION_ID` (int64, env `INSTALLATION_ID`), `PEM_KEY_FILE` (string, env `PEM_KEY_FILE`), `PEM_KEY` (string, env `PEM_KEY`, `display:"length"`).
2. At startup, before any clone, auth resolves: (a) when `APP_ID != 0` AND `INSTALLATION_ID != 0` AND (`PEM_KEY_FILE` set OR `PEM_KEY` set) → mint an installation access token via `lib/githubapp.MintIAT` (preferring `PEMPath` when both PEM forms are present) and use it as the effective token; (b) else → return a clear error naming the required App env vars and exit non-zero. There is no `GH_TOKEN` PAT fallback.
3. The minted IAT flows to BOTH the planning fetcher (`githubchangelog.NewHTTPFetcher`) AND the execution step (`NewExecutionStep`), via the existing single-token wiring through `pkg/factory` — no second token is introduced. It is also forwarded to the Claude/git subprocess as the `GH_TOKEN` env var (the credential value, carrying the IAT — not the retired PAT input path).

## Constraints

- maintainer is a multi-module Go mono-repo; the module is `agent/github-releaser/` (own `go.mod`). Build/verify with `make precommit` in that directory.
- Error wrapping: `github.com/bborbe/errors` context-form ONLY (`errors.New/Wrap/Errorf/Wrapf`, ctx first). NEVER `fmt.Errorf`. No `context.Background()` in business logic.
- Tests: Ginkgo v2 + Gomega, external `_test` package, counterfeiter v6 mocks.
- Reuse `lib/githubapp.MintIAT` — do NOT hand-roll JWT or IAT exchange. The helper uses `ghinstallation/v2` and exposes `Config.BaseURL` for `httptest`-backed unit tests (the pattern `lib/githubapp` tests already use).
- Coverage targets match existing module targets; `make precommit` enforces the gate.
- No releaser code change may alter the pr-reviewer agent or its auth.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility | Concurrency |
|---------|-----------|-------------------|----------|---------------|-------------|
| App creds absent/incomplete (no `APP_ID`/`INSTALLATION_ID`/PEM) | startup error returned before any clone | binary exits non-zero with message naming required App env vars; no clone, no fetch, no push | operator sets the App env and re-runs | reversible (nothing pushed) | n/a — fails before work |
| `MintIAT` fails: malformed PEM | pod exits non-zero at startup; log line `resolve PEM: ...` / `mint IAT: ...`; CrashLoopBackOff | startup error returned; no clone, no push | operator fixes the PEM in the Secret; controller retry re-mints | reversible | n/a — fails before work |
| GitHub IAT exchange unavailable / 4xx-5xx / network error mid-mint | pod exits non-zero; startup log line `mint IAT: ...`; CrashLoopBackOff | startup error; no clone, no push | controller retry (per its cap) re-mints later | reversible | n/a |
| GitHub IAT endpoint rate-limited | pod exits non-zero; startup log line `mint IAT: ...` citing 403/429 | startup error; no clone, no push | controller retry after backoff re-mints | reversible | n/a |
| Clock skew makes minted JWT appear expired (`iat`/`exp` rejected) | pod exits non-zero; startup log line `mint IAT: ...` citing JWT rejection | startup error; no clone, no push | controller retry; `ghinstallation/v2` re-mints a fresh JWT each call | reversible | n/a |

## Security / Abuse Cases

- **Token in logs.** The minted IAT is a bearer secret. `PEM_KEY` carries `display:"length"` so its content is never printed by the config dumper. `MintIAT` logs only `token_prefix=<first 8>...`; the releaser code must never log the full IAT, mirroring pr-reviewer.
- **Input validation.** `APP_ID` / `INSTALLATION_ID` are validated as positive and `PEM`/`PEMPath` mutual-exclusivity is enforced inside `lib/githubapp.Config.validate` — the releaser reuses this rather than re-validating. Invalid combinations surface as a startup error before any clone.
- **Trust boundary — PEM source.** `PEM_KEY` / `PEM_KEY_FILE` are operator inputs supplied via a k8s Secret mount (manifests out of scope). The code treats them as opaque secret bytes; it does not log, echo, or write them elsewhere.
- **Hang / retry forever.** `MintIAT` is context-aware (`transport.Token(ctx)`); the controller's retry cap bounds repeated mint failures. No unbounded loop is introduced.

## Acceptance Criteria

- [ ] `make precommit` exits 0 in `agent/github-releaser/` — evidence: exit code 0.
- [ ] The binary's config struct declares `APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`, `PEM_KEY` with the pr-reviewer env names — evidence: `grep -nE 'env:"(APP_ID|INSTALLATION_ID|PEM_KEY_FILE|PEM_KEY)"' agent/github-releaser/main.go` returns 4 lines; the `PEM_KEY` line also matches `display:"length"`.
- [ ] App-mode auth mints via `lib/githubapp` and no JWT is hand-rolled — evidence: `grep -rn 'githubapp.MintIAT' agent/github-releaser/` returns ≥1 line (location is an impl choice per the implementation note — main.go or a pkg helper), AND `grep -rn 'golang-jwt' agent/github-releaser/ --include='*.go' | wc -l` returns `0`. **Amended (2026-06-04, during verify):** original grep was unscoped → it caught transitive indirect deps in `go.mod`/`go.sum` (`golang-jwt/jwt/v4 // indirect` pulled by `ghinstallation/v2`, which `lib/githubapp.MintIAT` itself uses). Intent is "no hand-rolled JWT in Go source files" — scope corrected to `--include='*.go'`.
- [ ] With App creds set, the effective token is the minted IAT and is the value wired to both fetcher and execution step — evidence: a Ginkgo unit test on the auth-resolution function (using `githubapp.Config.BaseURL` pointed at an `httptest` IAT endpoint, the pattern `lib/githubapp` tests already use) asserts the resolved effective token equals the stubbed IAT string; test passes under `make precommit`.
- [ ] With both PEM forms set, `PEM_KEY_FILE` wins silently — evidence: Ginkgo unit test sets a valid `PEM_KEY_FILE` plus a garbage `PEM_KEY`, asserts the resolved token equals the stubbed IAT (proving the file PEM was used); test passes.
- [ ] With App creds absent, startup returns a non-nil error before any clone and there is no PAT fallback — evidence: Ginkgo unit test asserts a non-nil error whose message names the required App env vars (`APP_ID`/`INSTALLATION_ID`/`PEM_KEY*`) and does NOT mention `GH_TOKEN`; test passes. Plus `grep -rn 'AuthModePATFallback' agent/github-releaser/ | wc -l` returns `0`.

Scenario coverage: NO new scenario. The behavior is reachable by unit tests on the auth-resolution function against an `httptest` IAT endpoint (the exact pattern `lib/githubapp` tests already use) — real Docker / real `gh` / real cluster are not needed, and the pr-reviewer migration of the same shape shipped with no scenario. Live App-auth end-to-end validation belongs to the deploy step (which owns the manifests and the real App).

## Verification

```
cd agent/github-releaser
make precommit
```

Expected: exit 0; all Ginkgo suites green; coverage gate satisfied.

Spot checks:

```
grep -nE 'env:"(APP_ID|INSTALLATION_ID|PEM_KEY_FILE|PEM_KEY)"' agent/github-releaser/main.go
grep -rn 'githubapp.MintIAT' agent/github-releaser/
grep -rn 'golang-jwt' agent/github-releaser/ | wc -l
```

## Do-Nothing Option

If we don't do this, the github-releaser agent cannot authenticate inside the cluster at all: fleet-wide PAT auth was retired on 2026-05-24, so every clone, changelog fetch, and push fails. The agent is undeployable and the release loop stays broken at the auth link. There is no acceptable workaround that keeps PAT auth, since the credential type itself is gone fleet-wide. Not acceptable.

<!-- IMPLEMENTATION NOTE: pr-reviewer resolves auth in a dedicated resolveAuth(ctx) method that mutates a.GHToken in place (main.go ~lines 229-280). The releaser has no such method yet — its Run() reads a.GHToken directly. The cleanest mirror is to add an equivalent resolution function and call it at the top of Run() before createDeliverer/CreateAgentProvider, so the single resolved token flows unchanged through factory.CreateAgentProvider(..., ghToken, ...) → CreateAgent → both NewHTTPFetcher and NewExecutionStep. Whether the resolution lives in main.go (like pr-reviewer) or a small testable pkg helper is an agent decision at impl time; ACs only require the four observable resolution outcomes above. -->

## Verification Result

**Verified:** 2026-06-04T21:30:53Z (HEAD ee6481e)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory (v0.175.0)
**Scenario:** Walked all 6 ACs against fresh evidence in agent/github-releaser/ (no runtime scenario per spec — unit tests against httptest IAT endpoint mirror lib/githubapp pattern).
**Evidence:**
- AC#1: `make precommit` exit 0; all 13 packages green; coverage 85-100% in pkg/*; `0 issues` golangci-lint; `ready to commit`.
- AC#2: `grep -nE 'env:"(APP_ID|INSTALLATION_ID|PEM_KEY_FILE|PEM_KEY)"' agent/github-releaser/main.go` → 4 lines (88-91); `PEM_KEY` line carries `display:"length"`.
- AC#3: `grep -rn 'githubapp.MintIAT' agent/github-releaser/` → 2 hits in pkg/githubauth/githubauth.go (line 69 doc, line 99 call); `grep -rn 'golang-jwt' agent/github-releaser/ --include='*.go' | wc -l` → 0 (amended scope: `.go` files only — transitive indirect deps in go.mod/go.sum brought in by ghinstallation/v2 that lib/githubapp.MintIAT itself uses).
- AC#4/5/6: `go test -count=1 -v ./pkg/githubauth/...` → "Ran 11 of 11 Specs … SUCCESS! -- 11 Passed | 0 Failed". Specs include "App creds set → effective token is the minted IAT" (AC#4), "both PEMKeyFile and PEMKey set → PEMKeyFile wins silently" (AC#5), "App creds incomplete → error naming the required App env vars (no GH_TOKEN mention)" (AC#6).
- AC#6: `grep -rn 'AuthModePATFallback' agent/github-releaser/ | wc -l` → 0.
**Verdict:** PASS
