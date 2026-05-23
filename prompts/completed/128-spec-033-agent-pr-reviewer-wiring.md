---
status: completed
spec: [033-migrate-pr-reviewer-to-github-app]
summary: Wired lib/githubapp into agent/pr-reviewer for GitHub App auth; added APP_ID/INSTALLATION_ID/PEM_KEY_FILE/BOT_GITHUB_LOGIN env vars; retained GH_TOKEN as fallback; eradicated pr-review-of-ben literal; switched checkBotIdentity from GET /user to GET /app
container: maintainer-pr-reviewer-app-exec-128-spec-033-agent-pr-reviewer-wiring
dark-factory-version: v0.164.0
created: "2026-05-21T20:30:30Z"
queued: "2026-05-21T20:58:04Z"
started: "2026-05-21T21:11:05Z"
completed: "2026-05-21T21:25:39Z"
---

<summary>
- The `agent/pr-reviewer` binary accepts new env vars for GitHub App auth (App ID, Installation ID, PEM file path, Bot login) plus retains the legacy `GH_TOKEN` env as a fallback.
- At pod startup, if App-auth env is configured the agent mints an installation access token via `lib/githubapp` and threads it into the existing `GHToken` field so downstream code (`githubposter`, `githubauth`, Claude subprocess) keeps working unchanged.
- The pod logs its auth mode (`github-app` or `pat-fallback`) at startup so operators can tell from `kubectl logs` which credential is in use.
- The `pr-review-of-ben` literal is eradicated from the agent's code; the bot login flowing into `githubposter` is sourced from env with the prod App slug as default.
- `checkBotIdentity` no longer calls `GET /user` (which doesn't work for Apps); it either calls `GET /app` or is dropped, whichever the implementer judges cleaner.
- Operator-facing error messages in `pkg/steps_gh_token.go` no longer say "rotate teamvault entry" as if it were a PAT; they speak of App credentials when in App-auth mode.
- The pod refuses to start with a clear error if neither App-auth env vars nor legacy `GH_TOKEN` are set.
</summary>

<objective>
Wire the `lib/githubapp` package from Prompt 1 into `agent/pr-reviewer` so the binary authenticates to GitHub as a GitHub App in dev and prod clusters, while retaining the legacy `GH_TOKEN` PAT path as a transition fallback. Remove every reference to the `pr-review-of-ben` user identity from the agent's code paths.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.

Read these coding guides in `~/.claude/plugins/marketplaces/coding/docs/`:
- `go-factory-pattern.md` — constructors return interfaces, no I/O at construction time
- `go-testing-guide.md` — Ginkgo/Gomega conventions
- `test-pyramid-triggers.md` — default unit; integration only when crossing a real out-of-process boundary

Read these files in the maintainer repo before making changes:
- `agent/pr-reviewer/main.go` — current `application` struct (lines 45–92), `Run` (94–168), `dispatchAgent` (170–207); the `GHToken` field at line 77 is the central handoff point this prompt re-sources
- `agent/pr-reviewer/cmd/run-task/main.go` — local-CLI mirror of `main.go`; same flags must be added here too for parity with the Kafka entry point
- `agent/pr-reviewer/pkg/githubposter/poster.go` — the `prPoster` struct and `checkBotIdentity` (lines 21–137); `botLogin` is a constructor parameter, NOT a hard-coded constant
- `agent/pr-reviewer/pkg/githubposter/types.go` — **source of truth for the bot-login defaults**: line 23 has `DefaultBotLogin = "pr-review-of-ben"` (this is the literal Step 6 must replace) and line 26 has `BotLoginEnv = "BOT_GITHUB_LOGIN"` (the existing env-var name we reuse, NOT a new one)
- `agent/pr-reviewer/pkg/factory/botlogin.go` — `ResolveBotLogin(env)` reads `env[githubposter.BotLoginEnv]` and falls back to `DefaultBotLogin`; used at three call sites in `pkg/factory/factory.go` and `pkg/factory/runner.go`. The new `application.BotLogin` field MUST flow into the same path (set `env["BOT_GITHUB_LOGIN"]` so `ResolveBotLogin` picks it up), or be wired into `ResolveBotLogin` directly. Pick the path with the smaller diff
- `agent/pr-reviewer/pkg/factory/factory.go` — line 37 and lines 136–137 contain the comment `pr-review-of-ben`; Step 6 must update or delete
- `agent/pr-reviewer/pkg/steps_gh_token.go` — `ghTokenCheckStep`; lines 105–115 and 134–138 contain the PAT-language error messages that need rewording for App-auth mode
- `agent/pr-reviewer/pkg/githubauth/github_auth_setup.go` — the `gh auth setup-git` runner that consumes `GH_TOKEN`; **no change needed if the IAT is placed in `GHToken` before this runs** — the IAT works as a `gh` CLI token just like a PAT
- `agent/pr-reviewer/pkg/factory/factory.go` and `pkg/factory/runner.go` — verify which constructors today take `GHToken` and confirm none of them care whether the string is a `ghp_...` PAT or a `ghs_...` IAT
- `lib/githubapp/githubapp.go` — the package from Prompt 1; this prompt's main.go calls `githubapp.NewClient(...)` and/or `githubapp.MintIAT(...)`
- `agent/pr-reviewer/docs/github-app-setup.md` — App identity reference; the new env-var names this prompt introduces will be documented here too

**Required env-var names (must match the k8s manifest prompt that follows):**

| Env var | Purpose | Example value |
|---------|---------|---------------|
| `APP_ID` | numeric GitHub App ID | `3798945` (prod) / `3800041` (dev) |
| `INSTALLATION_ID` | numeric Installation ID | `134414316` (prod) / `134435225` (dev) |
| `PEM_KEY_FILE` | path to PEM file mounted from k8s Secret | `/etc/github-app/pem` (or wherever the k8s mount lands) |
| `BOT_GITHUB_LOGIN` | bot identity string used by `githubposter` (**existing env name** — reused, not renamed) | `ben-s-pull-request-reviewer[bot]` (prod) / `ben-s-pull-request-reviewer-dev[bot]` (dev) |
| `GH_TOKEN` | legacy PAT (still accepted as fallback) | unchanged |

`APP_ID`, `INSTALLATION_ID`, and `PEM_KEY_FILE` mirror the existing `cmd/mint-iat` flags so the smoke-test tool and production binary stay congruent. `BOT_GITHUB_LOGIN` is the env name **already in use** (`pkg/githubposter/types.go:26`, consumed by `pkg/factory/botlogin.go`); reusing it means the existing factory plumbing keeps working with no edits to the resolver.
</context>

<requirements>
Execute steps in order.

---

## Step 1 — Read the auth surface area

Before writing code, run:

```bash
grep -rn 'GHToken\|GH_TOKEN\|botLogin\|pr-review-of-ben' agent/pr-reviewer/ --include='*.go' | grep -v '_test.go\|mocks/'
```

Inventory every call site that reads or sets `GHToken` / `GH_TOKEN`. The auth handoff is funneled through `application.GHToken` (`main.go:77`) — Step 4 places the minted IAT there, so most downstream code needs no change. Confirm that hypothesis by reading the matches.

Confirm the `pr-review-of-ben` literal appears in code paths (probably the default value of `botLogin` somewhere in `pkg/githubposter/config.go` or a const). All such literals are removed in Step 6.

---

## Step 2 — Add new fields to the `application` struct in `agent/pr-reviewer/main.go`

The struct uses `github.com/bborbe/argument/v2` tags. Add these fields immediately after `GHToken` (currently around line 77):

```go
// GitHub App authentication. When AppID + InstallationID + PEMKeyFile are
// set, the pod mints an installation access token at startup and uses it
// in place of GHToken; the legacy GHToken env stays accepted as a fallback
// (see Run() for the resolution order).
AppID          int64  `required:"false" arg:"app-id"          env:"APP_ID"          usage:"GitHub App ID (numeric); when set, App auth is used instead of GH_TOKEN"`
InstallationID int64  `required:"false" arg:"installation-id" env:"INSTALLATION_ID" usage:"GitHub App Installation ID (numeric)"`
PEMKeyFile     string `required:"false" arg:"pem-key-file"    env:"PEM_KEY_FILE"    usage:"Path to the GitHub App private key (PEM file mounted from k8s Secret)"`
BotLogin       string `required:"false" arg:"bot-login"       env:"BOT_GITHUB_LOGIN" usage:"Bot identity used by githubposter (e.g. ben-s-pull-request-reviewer[bot])" default:"ben-s-pull-request-reviewer[bot]"`
```

Add the same four fields to `agent/pr-reviewer/cmd/run-task/main.go::application` so local CLI runs accept the same auth shape.

---

## Step 3 — Detect auth mode

Add a new file `agent/pr-reviewer/pkg/authmode.go` (and `pkg/authmode_test.go`). Placing the helper in `pkg/` lets both `main.go` and `cmd/run-task/main.go` import it — avoids duplicating the resolver across the two entry points.

```go
type AuthMode int

const (
    AuthModeNone AuthMode = iota
    AuthModeGitHubApp
    AuthModePATFallback
)

// resolveAuthMode picks the credential type to use at pod startup.
//   - AppID and InstallationID and PEMKeyFile all set → AuthModeGitHubApp (App wins; if GHToken is also set, log a warning and discard it)
//   - Any of the three App fields unset, but GHToken non-empty → AuthModePATFallback (log a warning)
//   - Otherwise → AuthModeNone (caller MUST refuse to start)
func resolveAuthMode(appID, installationID int64, pemKeyFile, ghToken string) AuthMode { ... }
```

Add unit tests for this function under `pkg/authmode_test.go` (Ginkgo). Cover all four combinations and the both-set-warning case.

---

## Step 4 — Mint the IAT at startup and place it in `application.GHToken`

In `agent/pr-reviewer/main.go::Run`, **before** `a.dispatchAgent(ctx, ...)`, **before** `githubauth.NewGhAuthSetupGit(a.GHToken)`, and **before** any code that reads `a.GHToken`, add the auth resolution block:

```go
mode := resolveAuthMode(a.AppID, a.InstallationID, a.PEMKeyFile, a.GHToken)
switch mode {
case AuthModeGitHubApp:
    if a.GHToken != "" {
        glog.Warningf("pr-reviewer auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored")
    }
    iat, err := githubapp.MintIAT(ctx, githubapp.Config{
        AppID:          a.AppID,
        InstallationID: a.InstallationID,
        PEMPath:        a.PEMKeyFile,
    })
    if err != nil {
        // Wrap and return — caller's service.Main will surface the error.
        return errors.Wrap(ctx, err, "mint github app iat")
    }
    a.GHToken = iat
    glog.V(2).Infof(
        "pr-reviewer auth mode=github-app app_id=%d installation_id=%d",
        a.AppID, a.InstallationID,
    )
case AuthModePATFallback:
    glog.Warningf("pr-reviewer auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
case AuthModeNone:
    return errors.Errorf(
        ctx,
        "pr-reviewer auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY_FILE, or set GH_TOKEN",
    )
}
```

Import path: `githubapp "github.com/bborbe/maintainer/lib/githubapp"`. Note this requires `lib/go.mod`'s package to be importable from `agent/pr-reviewer/go.mod`; the existing `replace` directive (`replace github.com/bborbe/maintainer/lib => ../../lib`) already covers it — verify `grep -n 'maintainer/lib' agent/pr-reviewer/go.mod` shows the replace.

Mirror the same block in `agent/pr-reviewer/cmd/run-task/main.go::Run`.

---

## Step 5 — Rework `checkBotIdentity` in `pkg/githubposter/poster.go`

The existing `checkBotIdentity` (lines 80–137) calls `GET https://api.github.com/user` and compares the response's `.login` to `botLogin`. This call returns 403 / 404 under App auth — Apps have no user identity.

Pick ONE approach:

**Option A — Replace with `GET /app`:** the response shape is `{"slug": "...", "name": "...", "owner": {...}, "permissions": {...}}`. The slug-derived bot login is `<slug>[bot]`. Compare that against `p.botLogin`. This preserves the identity-mismatch defense for the App-auth case. Implementation: switch the URL and the response struct; keep the retry / failure-result plumbing intact.

**Option B — Drop the identity check entirely:** the IAT itself is identity proof (it is bound to the installation and the App). Remove the `checkBotIdentity` call site in `Post()` (line 54) and the function definition. The verify-after-POST step (`verifyAfterPost`, lines 327–387) ALREADY checks the bot login on the posted review, so we still get observable failure if the wrong bot somehow posted.

**Either option satisfies the spec's Desired Behavior #8.** Document the choice in a one-line code comment above the change. Whichever option is picked, ensure no `https://api.github.com/user` literal remains in `pkg/githubposter/`.

Adjust the existing Ginkgo tests in `pkg/githubposter/poster_test.go` to match the chosen direction (either rewrite the `GET /user` test for `GET /app`, or delete the test if Option B). Tests must still pass.

---

## Step 6 — Eradicate the `pr-review-of-ben` literal from code

Run the in-code scan (excluding historical README/setup-doc mentions which stay):

```bash
grep -rn 'pr-review-of-ben' agent/pr-reviewer/ --include='*.go'
```

Every hit gets one of three treatments:

1. **`DefaultBotLogin` constant at `pkg/githubposter/types.go:23`**: change the value from `"pr-review-of-ben"` to `"ben-s-pull-request-reviewer[bot]"`. This is the canonical fallback used by `factory.ResolveBotLogin` when `BOT_GITHUB_LOGIN` env is empty; keep the constant in place but flip the value to the new prod App slug.
2. **Test fixtures**: replace with `ben-s-pull-request-reviewer[bot]` literally; tests don't care which identity, only that the post-and-verify round-trips with a consistent value.
3. **Code comments referencing the old identity** (e.g. `pkg/factory/factory.go:37,136-137`): rewrite to reference the new identity or delete the comment if it is stale background.

After the sweep:

```bash
grep -rn 'pr-review-of-ben' agent/pr-reviewer/ --include='*.go'
```

Must return zero matches.

**Out of scope for this grep**: `agent/pr-reviewer/README.md` and `agent/pr-reviewer/docs/github-app-setup.md` legitimately reference `pr-review-of-ben` as historical context (the predecessor PAT identity). Do NOT scrub them — they remain as migration history. The `--include='*.go'` flag above keeps the grep scoped to code only.

---

## Step 7 — Reword `pkg/steps_gh_token.go` error messages

The current messages (lines 105–115, 134–138) say "rotate teamvault entry" as if the token is always a PAT. Reword them to be neutral or branch on auth mode:

Acceptable approach: reword neutrally, mentioning both PAT and App credentials. For example:

```go
return needsInput(fmt.Sprintf(
    "GH credentials unauthorized (HTTP 401) — rotate the PAT or the App PEM (whichever is in use): %s",
    truncate(string(body), 200),
)), nil
```

Apply the same reword to the anonymous-limit-floor branch. Keep the message under 200 characters; existing tests in `pkg/steps_gh_token_test.go` may need their substring assertions updated.

(Note: the `glog.V(2).Infof` startup line added in Step 4 already tells operators which mode is active; the error messages can stay mode-agnostic.)

---

## Step 8 — Update `agent/pr-reviewer/docs/github-app-setup.md`

In the "Migration Status" section, mark the production-code refactor block as in flight. Add a line referencing the new env vars (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY_FILE`, `BOT_GITHUB_LOGIN`) so operators reading the setup doc see them named.

No other doc changes required in this prompt — k8s manifest details are owned by Prompt 3.

---

## Step 9 — Run module-local precommit

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0. Likely failure modes:

- Lint warnings on the new fields' struct tag formatting → match the existing `display:"length"` alignment style if applicable, or use `gofmt` defaults
- Counterfeiter regeneration if any internal interface changed → run `go generate ./...` in the package and re-run precommit
- Test failures from `pkg/githubposter/poster_test.go` or `pkg/steps_gh_token_test.go` if Step 5 / 7 rewords broke a substring assertion → adjust the test substring to the new message

---

## Step 10 — Add CHANGELOG entry

Append one line to the existing `## Unreleased` section (created in Prompt 1):

```markdown
- feat(agent/pr-reviewer): add GitHub App auth via new `APP_ID` / `INSTALLATION_ID` / `PEM_KEY_FILE` / `BOT_GITHUB_LOGIN` env vars; legacy `GH_TOKEN` retained as fallback; bot login `ben-s-pull-request-reviewer[bot]` (prod) / `ben-s-pull-request-reviewer-dev[bot]` (dev); `pr-review-of-ben` literal eradicated (spec 033)
```
</requirements>

<constraints>
- The minted IAT MUST land in `application.GHToken` before any downstream code reads that field. Downstream code (`githubposter`, `githubauth`, Claude subprocess `GH_TOKEN` env) must require no behavioral change.
- Legacy `GH_TOKEN` env stays accepted. Removing it is out of scope for this spec.
- All errors via `github.com/bborbe/errors`. No `fmt.Errorf`. No stdlib `errors.New`.
- All logging via `github.com/golang/glog`. No `log.*`, no `fmt.Print*`.
- No PEM bytes or IAT bytes in logs (only mode + numeric IDs).
- The `pr-review-of-ben` literal MUST NOT survive in **code** (`*.go` files) under `agent/pr-reviewer/` — not in functional code, not in test fixtures, not in code comments. Historical references in `README.md` and `docs/github-app-setup.md` stay as migration history.
- New `.go` files (`pkg/authmode.go`, `pkg/authmode_test.go`) carry the BSD-style three-line license header matching the rest of the repo (`grep -l 'BSD-style' agent/pr-reviewer/pkg/*.go | head -1` shows the canonical form).
- `checkBotIdentity` no longer calls `GET /user`. Either `GET /app` or no call at all; both satisfy Desired Behavior #8.
- Bot login is configurable via env (`BOT_GITHUB_LOGIN`), default `ben-s-pull-request-reviewer[bot]`.
- `agent/pr-reviewer/cmd/run-task/main.go` mirrors `agent/pr-reviewer/main.go` — the four new env-fields and the auth-resolution block exist in both.
- Existing tests must still pass (after adjusting substring assertions broken by reworded messages).
- Do NOT commit — dark-factory handles git.
- Do NOT touch `lib/` — that package was finalized in Prompt 1.
- Do NOT touch any k8s manifests — those are owned by Prompt 3.
- Do NOT investigate or change branch-protection / required-approvals behavior; that's an operator-attested observation in Rung-3.
</constraints>

<verification>
```bash
# 1. New env fields exist on both binaries
grep -n 'APP_ID\|INSTALLATION_ID\|PEM_KEY_FILE\|BOT_GITHUB_LOGIN' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go
# Expected: at least 4 matches per file

# 2. lib/githubapp is imported by the main binary
grep -n 'maintainer/lib/githubapp' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go
# Expected: import line in both

# 3. The startup auth-mode log line exists
grep -n 'auth mode=github-app\|auth mode=pat-fallback' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go
# Expected: matches in both

# 4. checkBotIdentity no longer calls api.github.com/user
grep -n 'api.github.com/user"' agent/pr-reviewer/pkg/githubposter/*.go
# Expected: 0 matches

# 5. pr-review-of-ben literal eradicated from code (README + docs/github-app-setup.md historical references remain)
grep -rn 'pr-review-of-ben' agent/pr-reviewer/ --include='*.go'
# Expected: 0 matches

# 6. Bot login default is the prod App slug
grep -n 'ben-s-pull-request-reviewer\[bot\]' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/run-task/main.go
# Expected: at least the default value declaration in both

# 7. steps_gh_token.go errors no longer say "rotate teamvault entry" in PAT-only language
grep -n 'rotate teamvault entry' agent/pr-reviewer/pkg/steps_gh_token.go
# Expected: 0 matches
grep -n 'rotate the PAT\|App PEM\|App credentials' agent/pr-reviewer/pkg/steps_gh_token.go
# Expected: at least 1 match (the reworded message)

# 8. Module-local precommit
cd agent/pr-reviewer && make precommit
# Expected: exit code 0; final line `ready to commit`

# 9. authmode unit tests exist and pass
cd agent/pr-reviewer && go test ./... -run AuthMode -v 2>&1 | tail -20
# Expected: at least one PASS line referencing resolveAuthMode

# 10. CHANGELOG updated
grep -A4 '## Unreleased' CHANGELOG.md | head -10
# Expected: a line mentioning agent/pr-reviewer App auth migration
```
</verification>
