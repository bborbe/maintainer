---
status: completed
spec: [027-post-verdict-to-github-pr]
summary: Built pkg/githubposter/ with PrPoster + ReviewVerifier interfaces, per-call retry policy, phantom-POST transient classification, autoApprove config reader, Counterfeiter mocks, and Ginkgo tests (88.4% coverage); make precommit exits 0.
container: maintainer-113-spec-027-poster-package
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-15T18:00:00Z"
queued: "2026-05-15T17:04:33Z"
started: "2026-05-15T18:01:24Z"
completed: "2026-05-15T18:03:54Z"
---

<summary>
- Create `agent/pr-reviewer/pkg/githubposter/` — the data plane for posting GitHub PR reviews
- Two interfaces: `PrPoster.Post` (full posting sequence) and `ReviewVerifier.VerifyReview` (ai_review's separate check)
- HTTP transport via Go `net/http` only — no `gh` CLI shell-out anywhere
- All HTTP behind a `HTTPClient` interface so tests inject Counterfeiter fakes; no live network in unit tests
- Per-HTTP-call retry policy per spec DB#8: at most one retry for transient errors (5s + jitter); no retry for permanent or unknown
- Error classes: `transient` | `permanent` | `unknown` | `not-a-failure` | `soft-warning` (matches spec DB#9 enum)
- Phantom POST (200 returned but review absent in subsequent GET — empirical 2026-05-15) is classified **transient** so retry kicks in — this is the self-healing pivot
- `PostResult` carries every field prompt 2 needs for the `## Diagnostics` block (per spec DB#9)
- Prompts 2 and 3 import this package as a library; this prompt does NOT modify in_progress or ai_review wiring
- `make precommit` in `agent/pr-reviewer/` clean at the end
</summary>

<objective>
Build `pkg/githubposter/` with `PrPoster` + `ReviewVerifier` interfaces + their default implementations + Counterfeiter mocks + Ginkgo tests covering the spec's failure-mode table. Slim, behavior-driven; no inline implementation noise — follow existing patterns in `pkg/github/client.go`.
</objective>

<context>

**Spec section anchors (read these first, in this order):**
- `specs/in-progress/027-post-verdict-to-github-pr.md` Desired Behavior §1–§9 (the full contract this prompt implements at the data-plane layer)
- Spec §Failure Modes table (defines the error-class mapping)
- Spec §Constraints (lists "dismissal MUST precede POST" — invariant)

**Existing in-module patterns to mirror (read these for style):**
- `agent/pr-reviewer/pkg/github/client.go` — canonical HTTP + `errors.Wrapf(ctx, err, ...)` style. Match this verbatim for error handling, response reading, status-code branching.
- `agent/pr-reviewer/pkg/github/client_test.go` — Ginkgo + Counterfeiter test structure.
- `agent/pr-reviewer/pkg/githubauth/` (if present) — config-reading pattern.
- `agent/pr-reviewer/mocks/github-client.go` — Counterfeiter output convention (filename, package, fake-type naming).

**Coding guides (in `~/.claude/plugins/marketplaces/coding/docs/`):**
- `go-error-wrapping-guide.md` — `bborbe/errors` only; **no `fmt.Errorf` anywhere**; ctx wrapping
- `go-factory-pattern.md` — `Create*` zero-logic factories returning interfaces
- `go-patterns.md` — interface → constructor → struct → method
- `go-testing-guide.md` — Ginkgo v2 + Gomega + Counterfeiter; external `*_test` package; coverage ≥80%
- `go-composition.md` — small interfaces, DI only, no package-level mutable state
- `test-pyramid-triggers.md` — which test types to write per change kind

**Existing types this package depends on:**
- `agent/pr-reviewer/pkg/verdict.go` — `Verdict`, `VerdictApprove`, `VerdictRequestChanges` (no `VerdictComment` — spec 025 removed it)
- `agent/pr-reviewer/pkg/prurl.go` — `ParsePRURL`, `PRInfo{Owner, Repo, Number}`
- `gopkg.in/yaml.v3` — already in `go.mod` line 22; use it directly for `.pr-reviewer.yaml` parsing

**Files this prompt creates (and only these):**
- `agent/pr-reviewer/pkg/githubposter/types.go` — `HTTPClient`, `PrPoster`, `ReviewVerifier`, `PostRequest`, `PostResult`, `VerifyRequest`, `VerifyResult`, `ErrorClass`, `AutoApproveConfig`
- `agent/pr-reviewer/pkg/githubposter/config.go` — `ReadAutoApproveConfig(workdir)` via `yaml.v3`
- `agent/pr-reviewer/pkg/githubposter/retry.go` — `retryCall` helper + `classifyError` (per-call retry policy)
- `agent/pr-reviewer/pkg/githubposter/poster.go` — `prPoster` struct + `Post` method (full sequence)
- `agent/pr-reviewer/pkg/githubposter/verifier.go` — `reviewVerifier` struct + `VerifyReview` method
- `agent/pr-reviewer/pkg/githubposter/githubposter_suite_test.go`
- `agent/pr-reviewer/pkg/githubposter/config_test.go`
- `agent/pr-reviewer/pkg/githubposter/poster_test.go`
- `agent/pr-reviewer/pkg/githubposter/verifier_test.go`
- Counterfeiter-generated mocks under `agent/pr-reviewer/mocks/` (after `make generate`)
- CHANGELOG.md `## Unreleased` entry

**Files this prompt does NOT touch:**
- `pkg/factory/factory.go`, `pkg/steps_*.go`, `pkg/prompts/*` — those are prompts 2 and 3
- `agent/.claude/CLAUDE.md`, any `.allowedTools` — not modified by this spec at all

</context>

<requirements>

Execute steps in order. Run `make test` after step 5 for fast feedback. Run `make precommit` only at the final step.

---

## Step 1 — Create `pkg/githubposter/` directory

```bash
mkdir -p agent/pr-reviewer/pkg/githubposter
```

---

## Step 2 — `types.go` — interfaces and value types

Define exactly these types. License header per project convention (mirror `pkg/github/client.go`).

```go
package githubposter

import (
    "context"
    "io"
    "net/http"
)

//counterfeiter:generate -o ../../mocks/http-client.go --fake-name HTTPClient . HTTPClient
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}

//counterfeiter:generate -o ../../mocks/pr-poster.go --fake-name PrPoster . PrPoster
type PrPoster interface {
    Post(ctx context.Context, req PostRequest) PostResult
}

//counterfeiter:generate -o ../../mocks/review-verifier.go --fake-name ReviewVerifier . ReviewVerifier
type ReviewVerifier interface {
    VerifyReview(ctx context.Context, req VerifyRequest) VerifyResult
}

type ErrorClass string

const (
    ErrorClassTransient    ErrorClass = "transient"
    ErrorClassPermanent    ErrorClass = "permanent"
    ErrorClassUnknown      ErrorClass = "unknown"
    ErrorClassNotAFailure  ErrorClass = "not-a-failure"
    ErrorClassSoftWarning  ErrorClass = "soft-warning"
)

type PostRequest struct {
    PR        PRInfo     // pkg.PRInfo — owner, repo, number
    HeadSHA   string
    Verdict   Verdict    // pkg.Verdict — approve | request-changes only
    Summary   string
    WorkDir   string     // for reading .pr-reviewer.yaml
}

type PostResult struct {
    Outcome       string      // "success" | "failed"
    ReviewID      int64       // 0 if not posted
    PostedEvent   string      // "APPROVE" | "REQUEST_CHANGES" | "COMMENT" | ""
    FailureStep   string      // one of: "GET /user", "GET /pulls/N/reviews (dismiss-list)", "PUT .../dismissals", "POST /pulls/N/reviews", "GET /pulls/N/reviews (verify)"
    Class         ErrorClass
    EscalateHint  bool        // true for permanent + unknown; lets operator skip waiting for trigger_count cap
    Attempt       int         // 1 or 2 (in-process retry count of the failing call)
    HTTPStatus    int         // 0 if pre-HTTP (DNS, connection)
    ErrorMessage  string      // short summary
    ResponseBody  string      // first 500 bytes; empty if pre-HTTP
    ElapsedMs     int64
    Warnings      []string    // soft-warning entries (e.g. empty summary substituted with default)
}

type VerifyRequest struct {
    PR             PRInfo
    HeadSHA        string
    ExpectedStates []string   // accepted GitHub review states: e.g. ["APPROVED"], ["CHANGES_REQUESTED"], ["COMMENTED"]
}

type VerifyResult struct {
    Found         bool
    Outcome       string      // "success" | "failed"
    FoundState    string      // populated when Found=true
    FailureStep   string      // "GET /pulls/N/reviews (ai_review verify)" on failure
    Class         ErrorClass
    EscalateHint  bool
    Attempt       int
    HTTPStatus    int
    ErrorMessage  string
    ResponseBody  string
    ElapsedMs     int64
}

type AutoApproveConfig struct {
    AutoApprove bool `yaml:"autoApprove"`
}

const (
    // DefaultBotLogin is the GitHub login the agent posts as by default.
    DefaultBotLogin = "pr-review-of-ben"

    // BotLoginEnv is the env var name that overrides DefaultBotLogin (read by the factory, not this package).
    BotLoginEnv = "BOT_GITHUB_LOGIN"
)
```

Import `PRInfo` from `agent/pr-reviewer/pkg` (`prurl.go`); import `Verdict` from same. Use the project's existing license header.

---

## Step 3 — `config.go` — `.pr-reviewer.yaml` reader

Read `<workdir>/.pr-reviewer.yaml` using `gopkg.in/yaml.v3` (already in `go.mod`). Signature:

```go
func ReadAutoApproveConfig(ctx context.Context, workDir string) (AutoApproveConfig, error)
```

Behavior:
- File missing (`os.IsNotExist`) → return `AutoApproveConfig{AutoApprove: false}, nil` (NOT an error — documented spec default)
- File present → `yaml.Unmarshal` into `AutoApproveConfig`; on parse error wrap via `errors.Wrapf(ctx, err, "parse .pr-reviewer.yaml at %s", path)`
- File read error other than NotExist → `errors.Wrapf(ctx, err, "read .pr-reviewer.yaml at %s", path)`

≤30 lines including license header. Do NOT write a manual line-scanning fallback — `yaml.v3` is present.

---

## Step 4 — `retry.go` — per-call retry policy + error classification

Two functions:

**`classifyError(httpStatus int, err error) ErrorClass`:**

| Condition | Returns |
|---|---|
| `err != nil` and `errors.Is(err, context.Canceled)` or `context.DeadlineExceeded` | `ErrorClassTransient` |
| `err != nil` and net.Error timeout | `ErrorClassTransient` |
| `err != nil` (other network/connection) | `ErrorClassTransient` |
| `httpStatus == 0` | `ErrorClassTransient` (pre-HTTP failure) |
| `httpStatus >= 500` | `ErrorClassTransient` |
| `httpStatus == 429` | `ErrorClassTransient` |
| `httpStatus == 401` | `ErrorClassPermanent` |
| `httpStatus == 403` | `ErrorClassPermanent` |
| `httpStatus == 404` | `ErrorClassPermanent` |
| `httpStatus == 422` | `ErrorClassPermanent` (caller decides if it's `not-a-failure` for the closed-PR-on-POST case) |
| `httpStatus 2xx` AND caller passes a sentinel error indicating phantom (200 + post-check assertion failed) | `ErrorClassTransient` |
| Anything else | `ErrorClassUnknown` |

Phantom POST is classified **transient** so the verify-after-POST retry path actually triggers. This is the spec's self-healing pivot — get this wrong and re-spawns can't recover.

**`retryCall[T any]`** — generic helper running an HTTP call with at-most-one retry:

```go
type CallResult[T any] struct {
    Value        T
    HTTPStatus   int
    ResponseBody string  // first 500 bytes
    Err          error
    Attempts     int     // 1 or 2
    Class        ErrorClass
}

func retryCall[T any](
    ctx context.Context,
    label string,                    // for error wrapping
    call func(ctx context.Context) (T, int, string, error),
) CallResult[T]
```

Behavior:
- Attempt 1: run `call(ctx)`. Compute `Class = classifyError(status, err)`.
- If `Class == ErrorClassTransient` → wait `5s + jitter(0..1s)` → attempt 2.
- Otherwise → return immediately.
- On attempt 2 same classification logic. Return whatever attempt 2 produced.
- `Attempts` field always reflects the final attempt count (1 or 2).

Single name throughout: `retryCall`. No `retryOnce` alias. No internal rename steps.

---

## Step 5 — `poster.go` — `prPoster` + `Post`

`Post` implements the full sequence per spec DB#2 (b)–(g). **Sequence order is invariant; dismissal MUST precede POST** (spec Constraints).

Sequence:

1. **Bot-identity check.** `retryCall` wrapping `GET https://api.github.com/user`. Assert `user.login == p.botLogin` (struct field, defaulted to `pr-review-of-ben` in factory; this struct receives it already-resolved). Mismatch → return `PostResult{Outcome:"failed", Class:permanent, EscalateHint:true, FailureStep:"GET /user", ErrorMessage:"bot identity mismatch: expected X got Y"}`.

2. **Read autoApprove.** `ReadAutoApproveConfig(ctx, req.WorkDir)`. Error → wrap via `errors.Wrapf`, return permanent failure. Use the result to decide `event` mapping (step 5).

3. **Dismiss prior bot reviews.** `retryCall` wrapping `GET https://api.github.com/repos/{owner}/{repo}/pulls/{number}/reviews`. Parse response. Filter: `user.login == p.botLogin && commit_id == req.HeadSHA && state != "DISMISSED"`. For each match, `retryCall` wrapping `PUT https://api.github.com/repos/{owner}/{repo}/pulls/{number}/reviews/{review_id}/dismissals` with body `{"message": "superseded by new automated review"}`. If any individual `PUT` fails with `ErrorClassPermanent` → return failure (don't post on top of stale).

4. **Map verdict to event:**
   | Verdict | autoApprove | Event | Body prefix |
   |---|---|---|---|
   | `VerdictApprove` | `true` | `APPROVE` | (none) |
   | `VerdictApprove` | `false` | `COMMENT` | `"auto-approve disabled for this repo, review submitted as comment\n\n"` |
   | `VerdictRequestChanges` | (ignored) | `REQUEST_CHANGES` | (none) |

   If `req.Summary == ""` → substitute `"automated review — no summary produced"` and append to `result.Warnings = append(result.Warnings, "soft-warning: empty summary substituted with default")`. Class remains as it would be otherwise (NOT escalated to permanent for this alone).

5. **POST review.** `retryCall` wrapping `POST https://api.github.com/repos/{owner}/{repo}/pulls/{number}/reviews` with body `{"event": event, "commit_id": req.HeadSHA, "body": bodyPrefix + req.Summary}`. Read response, parse `ReviewID`.

   **422 on POST is `not-a-failure`** (PR closed/merged race): return `PostResult{Outcome:"success", Class:not-a-failure, FailureStep:"POST /pulls/N/reviews", HTTPStatus:422}`. Do NOT proceed to verify.

6. **Verify-after-POST.** `retryCall` wrapping `GET https://api.github.com/repos/{owner}/{repo}/pulls/{number}/reviews` again. Filter: `user.login == p.botLogin && commit_id == req.HeadSHA`. Assert at least one entry with state matching `eventToState(event)` (`APPROVE→APPROVED`, `REQUEST_CHANGES→CHANGES_REQUESTED`, `COMMENT→COMMENTED`).

   **If filter returns empty list:** the inner `call` returns a sentinel error `errPhantomPOST` with `httpStatus=200`. `classifyError` must treat (`200`, `errPhantomPOST`) as `ErrorClassTransient` so `retryCall` retries. If both attempts find no review → return `PostResult{Outcome:"failed", Class:transient, FailureStep:"GET /pulls/N/reviews (verify)", ErrorMessage:"phantom POST: review absent in GET after POST"}`.

   On success: return `PostResult{Outcome:"success", ReviewID, PostedEvent:event, HTTPStatus:200, ElapsedMs:..., Warnings:result.Warnings}`.

**HTTP response reading pattern (CRITICAL):**

```go
// Read full body first (for parsing), THEN truncate a copy for diagnostics.
// io.LimitReader BEFORE json.Unmarshal breaks the happy path.
bodyBytes, err := io.ReadAll(resp.Body)
defer resp.Body.Close()
// ... use bodyBytes for json.Unmarshal ...
truncated := bodyBytes
if len(truncated) > 500 { truncated = truncated[:500] }
result.ResponseBody = string(truncated)
```

**Error wrapping:**
- All errors via `errors.Wrapf(ctx, err, "label: %s", ...)` — see `pkg/github/client.go` for the pattern. **No `fmt.Errorf` anywhere** in this package.
- Sentinel errors: use `errors.New` from `github.com/bborbe/errors` (NOT stdlib `errors` — import alias if needed). Define once at package scope: `var errPhantomPOST = errors.New("phantom-POST sentinel")`. Compare via `errors.Is(err, errPhantomPOST)`. Import line: `import errors "github.com/bborbe/errors"` (shadow stdlib).

**Constructor:**

```go
func NewPrPoster(httpClient HTTPClient, ghToken string, botLogin string) PrPoster
```

Pure composition. No defaults computed in constructor — caller (factory) resolves `botLogin` env var before calling.

---

## Step 6 — `verifier.go` — `reviewVerifier` + `VerifyReview`

Single HTTP call: `retryCall` wrapping `GET /repos/{owner}/{repo}/pulls/{number}/reviews`. Filter: `user.login == v.botLogin && commit_id == req.HeadSHA && state ∈ req.ExpectedStates`.

- Found → `VerifyResult{Found:true, Outcome:"success", FoundState: matched.State}`
- Not found, both attempts → `VerifyResult{Found:false, Outcome:"failed", Class:transient, ErrorMessage:"no matching bot review for head SHA"}`
- HTTP permanent error → `VerifyResult{Found:false, Outcome:"failed", Class:permanent, EscalateHint:true, ...}`

Constructor: `NewReviewVerifier(httpClient HTTPClient, ghToken string, botLogin string) ReviewVerifier`. Same pattern as `NewPrPoster`.

---

## Step 7 — Run `make generate`

```bash
cd agent/pr-reviewer && make generate
```

Generates Counterfeiter mocks for `HTTPClient`, `PrPoster`, `ReviewVerifier` based on the `//counterfeiter:generate` annotations in `types.go`.

---

## Step 8 — Suite + config tests

`githubposter_suite_test.go` — standard Ginkgo bootstrap (mirror `pkg/github/client_test.go` setup).

`config_test.go` — `DescribeTable` covering:
- File missing → `{AutoApprove: false}, nil`
- File with `autoApprove: true` → `{AutoApprove: true}, nil`
- File with `autoApprove: false` → `{AutoApprove: false}, nil`
- File with malformed YAML → error wrapped with `parse .pr-reviewer.yaml`
- File present but `autoApprove` field absent → `{AutoApprove: false}, nil`

---

## Step 9 — Poster tests

`poster_test.go` — Inject `FakeHTTPClient`. Use Counterfeiter's `DoStub` to drive responses per call sequence. Cover **at least** these scenarios as separate `Context` blocks:

1. **Happy path APPROVE** — bot-identity OK; no prior reviews; `autoApprove: true`; verdict `approve`; POST returns 201 with `id: 42` + `state: APPROVED`; verify-GET returns the review → `PostResult{Outcome:"success", ReviewID:42, PostedEvent:"APPROVE"}`
2. **Happy path REQUEST_CHANGES** — verdict `request-changes`; POST/verify state `CHANGES_REQUESTED`
3. **Happy path COMMENT (demoted)** — verdict `approve` + `autoApprove: false`; POST/verify state `COMMENTED`; body contains the demotion prefix
4. **Bot identity mismatch** — `GET /user` returns `login: someone-else` → `PostResult{Outcome:"failed", Class:permanent, EscalateHint:true, FailureStep:"GET /user"}`
5. **Dismissal happens before POST** — `GET /reviews` returns one bot review on same SHA; assert order: GET → PUT dismissal → POST → GET verify (use `Invocations()` on the fake)
6. **Phantom POST → retry** — first verify-GET returns empty list; second verify-GET returns the review → `PostResult{Outcome:"success", Attempt:2}` for the verify step
7. **Phantom POST → exhausted retry** — both verify-GETs return empty → `PostResult{Outcome:"failed", Class:transient, FailureStep:"GET /pulls/N/reviews (verify)"}`
8. **POST 422 (PR closed)** → `PostResult{Outcome:"success", Class:not-a-failure}`, no verify-GET issued
9. **POST 403 (permanent)** → `PostResult{Outcome:"failed", Class:permanent, EscalateHint:true, Attempt:1}` (no retry)
10. **Transient 5xx → retry succeeds** — GET /user returns 503 first then 200 → success path continues with `Attempt:2` recorded for /user
11. **Empty summary → soft-warning** — `req.Summary == ""`; POST/verify succeed; `result.Warnings` contains the soft-warning entry; `Outcome:"success"`
12. **Permanent dismissal failure** — `PUT .../dismissals` returns 403 → `PostResult{Outcome:"failed", Class:permanent, FailureStep:"PUT .../dismissals"}`; POST is NOT issued (assert via Invocations count)
13. **Unknown class (non-JSON response)** — `GET /user` returns 200 with body `"not-json-at-all"` → JSON parse fails → `PostResult{Outcome:"failed", Class:unknown, EscalateHint:true, Attempt:1}` (no retry; classifier returns `unknown` for parse failures)
14. **POST request body shape** — for a happy-path APPROVE post, capture the `*http.Request.Body` via `FakeHTTPClient.Invocations()`, read it, and assert it parses to `{"event":"APPROVE","commit_id":"<head-sha>","body":"<summary>"}` exactly. Prevents struct-tag typos shipping silently.

Use `DescribeTable` for the verdict→event→state round-trip (scenarios 1+2+3) to keep the test compact. Also use `DescribeTable` to assert `string(ErrorClassX) == "x"` for all five class constants — prevents a rename from drifting the YAML diagnostic schema.

---

## Step 10 — Verifier tests

`verifier_test.go` — Cover:
1. Review found with matching state → `VerifyResult{Found:true, FoundState:"APPROVED"}`
2. Review absent on first GET, found on second → `VerifyResult{Found:true, Attempt:2}` (transient retry succeeded)
3. Review absent both attempts → `VerifyResult{Found:false, Class:transient}`
4. GET returns 404 → `VerifyResult{Found:false, Class:permanent, EscalateHint:true}` (no retry)
5. GET returns 429 → transient retry; second call returns 200 with review → `Found:true, Attempt:2`

---

## Step 11 — Run `make test`

```bash
cd agent/pr-reviewer && make test
```

All tests pass. Coverage for `pkg/githubposter` ≥80% (the package is small; should easily clear).

---

## Step 12 — CHANGELOG entry

Add to root `CHANGELOG.md` under `## Unreleased` (create the section if absent):

```
- feat(agent/pr-reviewer): add `pkg/githubposter/` — GitHub REST API client for posting PR reviews. Implements bot-identity self-check, `.pr-reviewer.yaml` autoApprove config, prior-review dismissal, POST review, verify-after-POST (catches phantom POSTs), and per-call retry policy (one retry max for transient errors; no retry for permanent). Used by `in_progress` and `ai_review` phases in subsequent prompts. (spec 027 prompt 1/3)
```

---

## Step 13 — Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0. If lint/license/exhaustruct fails, fix it; re-run only the failing target before re-running `make precommit`.

</requirements>

<constraints>

- **`bborbe/errors` only.** Every error wrapped via `errors.Wrapf(ctx, err, "...")` or constructed via `errors.New` / `errors.Errorf(ctx, ...)`. **Zero `fmt.Errorf` in `pkg/githubposter/`** — `grep -rn 'fmt.Errorf' agent/pr-reviewer/pkg/githubposter/` must return zero matches. See `pkg/github/client.go` for the canonical pattern.
- **Phantom POST → transient.** If `verifyAfterPost` returns 200 but the bot review for the current head SHA is absent in the response, `classifyError` MUST return `ErrorClassTransient` so `retryCall` retries. Spec's self-healing property depends on this. Sentinel error pattern with `errors.Is` check inside `classifyError`.
- **Full body read before truncation.** `io.LimitReader` may only be used to truncate a COPY of the body for `PostResult.ResponseBody`, never on the body used for `json.Unmarshal`. The empirical phantom-POST case (2026-05-15) returned a multi-hundred-byte JSON object that won't parse if pre-truncated.
- **Dismissal precedes POST.** Spec invariant. Tests assert order via Counterfeiter's `Invocations()`.
- **`yaml.v3` is already in `go.mod`** (line 22). Use it directly. No line-scanning fallback.
- **`gh` CLI is forbidden.** No `os/exec` invocation of `gh`, no `Bash(gh ...)` allowlist patterns. `grep -rn '"gh "\|gh pr\|gh api' agent/pr-reviewer/pkg/githubposter/` must return zero matches.
- **No `Bash` allowlist changes.** This prompt creates a new Go package; it does NOT touch the agent's tool allowlist. `pkg/factory/factory.go` is owned by prompt 2.
- **Factory functions are pure composition.** `NewPrPoster` and `NewReviewVerifier` receive already-resolved `botLogin` from the caller; no env-var reads or defaulting inside the constructor.
- **No package-level mutable state.** All state lives on the struct.
- **No `context.Background()`** anywhere in non-test code.
- **External `_test` package** (`package githubposter_test`). Coverage ≥80% for the package.
- **Counterfeiter mock paths.** Mocks land in `agent/pr-reviewer/mocks/` per existing convention. Filenames: `http-client.go`, `pr-poster.go`, `review-verifier.go`. No `fake-` prefix on filenames (existing module convention).
- **No `go mod vendor`.** Module uses direct deps.
- **Do not commit.** Dark-factory handles git.
- **`make precommit` in `agent/pr-reviewer/`**, never repo root.

</constraints>

<verification>

```bash
cd agent/pr-reviewer && make precommit
```

Then sanity-grep:

```bash
# No fmt.Errorf in the new package — bborbe/errors only:
grep -rn "fmt\.Errorf" agent/pr-reviewer/pkg/githubposter/
# Expected: zero matches

# Phantom POST classified as transient (sentinel + check in classifyError):
grep -n "phantom\|errPhantomPOST" agent/pr-reviewer/pkg/githubposter/retry.go agent/pr-reviewer/pkg/githubposter/poster.go
# Expected: sentinel defined; classifyError treats it as transient

# No gh CLI shell-out:
grep -rEn "\"gh \"|gh pr |gh api " agent/pr-reviewer/pkg/githubposter/
# Expected: zero matches

# Dismissal-before-POST asserted in tests:
grep -n "Invocations\|dismissal.*before.*POST\|order.*dismissal" agent/pr-reviewer/pkg/githubposter/poster_test.go
# Expected: at least one assertion of call ordering

# Counterfeiter mocks generated:
ls agent/pr-reviewer/mocks/http-client.go agent/pr-reviewer/mocks/pr-poster.go agent/pr-reviewer/mocks/review-verifier.go
# Expected: all three files present

# Coverage check (informational):
cd agent/pr-reviewer && go test -cover ./pkg/githubposter/...
# Expected: coverage ≥80%

# Soft-warning surface present on PostResult:
grep -n "Warnings\s*\[\]string\|ErrorClassSoftWarning" agent/pr-reviewer/pkg/githubposter/types.go
# Expected: both present

# CHANGELOG entry:
grep -n "githubposter" CHANGELOG.md
# Expected: one entry under ## Unreleased
```

</verification>
