---
status: completed
summary: Applied all six PR-36 review fixes (C1 fail-closed CommittedFiles, M1 MaxBytesReader cap, M2 bump cache, M3 SHA-256 redaction, M4 config_fetch_warning surfacing, M5 strict YAML decoding) with regression tests; both precommit suites exit 0.
container: maintainer-changelog-rewrite-exec-233-fail-closed-ai-review-checks-and-config-hardening
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T20:30:00Z"
queued: "2026-06-02T21:00:42Z"
started: "2026-06-02T21:00:44Z"
completed: "2026-06-02T21:17:37Z"
---

<summary>
- Fixes a fail-open bug in ai-review: when the `CommittedFiles` git call errors transiently, the unexpected-file-change check no longer silently passes — it now records the check as failed and surfaces in `FailedChecks` (matches the other two structural checks).
- Bounds the `.maintainer.yaml` HTTP response body to 1 MiB so a misconfigured or hostile upstream cannot exhaust agent memory.
- Caches the bump-classification LLM verdict on the task page so a transient rewrite-LLM failure does not re-spend the bump LLM call on every re-fire (was running 2 LLM calls per retry; now 1).
- Stops echoing non-2xx response bodies into the fetcher error string and V(2) log — replaces the 200-char body preview with the status code plus a short SHA-256 hash prefix.
- Surfaces a `.maintainer.yaml` transport-fetch failure to the task page so a repo that opted into rewrite is not silently downgraded on a transient flake (writes a `config_fetch_warning` field on the plan, plus glog.Warningf).
- Tightens the lib YAML parser to reject unknown fields so a typo like `changelogRwrite` fails loudly instead of producing default false.
- Updates one existing lib test (the "unknown top-level key tolerated" case is no longer valid with KnownFields(true)) and one godoc paragraph that asserted the old forward-compat behavior.
- Both `agent/github-releaser/` and `lib/` precommit suites stay green.
</summary>

<objective>
Address PR #36 reviewer (pr-reviewer-agent) CHANGES_REQUESTED verdict: one CRITICAL fail-open path in ai-review + five MAJOR items (HTTP body cap, bump-verdict caching, response-body redaction, transport-error surfacing, strict YAML decoding). No new functionality, no behavior change beyond the named fixes.
</objective>

<context>
Read `CLAUDE.md` at the repo root AND `agent/github-releaser/CLAUDE.md`.

Read these files BEFORE editing (verify current signatures, error idioms, test style):

- `agent/github-releaser/pkg/steps_ai_review.go` — current ai-review step. Touched in Fix C1 (lines 560-604 around `checkUnexpectedFileChange`). The CORRECT fail-closed pattern lives at lines 459-468 (`verifyTagAtExpectedCommit`) and 498-507 (`verifyChangelogHeaderRewritten`) — mirror those exactly.
- `agent/github-releaser/pkg/steps_ai_review_test.go` — Ginkgo style; new spec for C1 lands here.
- `agent/github-releaser/pkg/maintainerconfig/fetcher.go` — Touched in M1 (body cap), M3 (status-error redaction). Read the `httpFetcher.readBody` (line 189-195), `httpFetcher.checkStatus` (line 197-228), and the `fetchTimeout` constant (line 45).
- `agent/github-releaser/pkg/maintainerconfig/fetcher_test.go` — Ginkgo style; new specs for M1 + M3 land here. Read `NewHTTPFetcherForTest` usage at line 38 (existing export-for-test pattern).
- `agent/github-releaser/pkg/maintainerconfig/export_test.go` — verify whether `NewHTTPFetcherForTest` already exposes what M1 needs; do NOT add another exported test seam.
- `agent/github-releaser/pkg/steps_planning.go` — Touched in M2 (bump-verdict caching) at `Run` (line 90), `runClassification` (line 201-254), and M4 (warning surfacing) at `resolveChangelogRewrite` (line 173-199).
- `agent/github-releaser/pkg/steps_planning_test.go` — Ginkgo style; new specs for M2 + M4 land here. Existing call-count pattern is `fakeRunner.RunCallCount()` (lines 119, 368, 593 …) and per-call programming is `fakeRunner.RunReturnsOnCall(0, …)` / `RunReturnsOnCall(1, …)` (lines 354-359). Reuse.
- `agent/github-releaser/pkg/plan_output.go` — `PlanOutput` struct. Touched in M4 (new `ConfigFetchWarning` field — see Fix M4 below for exact JSON tag).
- `lib/maintainerconfig/maintainerconfig.go` — Touched in M5 (KnownFields). Read the package godoc lines 5-23 — line 16-18 explicitly claims "Unknown top-level keys are tolerated by design (yaml.Unmarshal ignores fields it does not know)". That paragraph MUST be replaced in M5; do not leave a contradiction.
- `lib/maintainerconfig/maintainerconfig_test.go` — Touched in M5. The entry "unknown top-level key ignored, no error" (lines 53-57) becomes invalid with KnownFields(true) and MUST be replaced with a rejection spec (see Fix M5 details).
- `agent/github-releaser/mocks/git_ops.go` — counterfeiter fake `GitOps`; `CommittedFilesReturns([]string, error)` used to inject the error for the C1 regression test.
- `agent/github-releaser/mocks/maintainer_config_fetcher.go` — counterfeiter fake `MaintainerConfigFetcher`; `FetchReturns([]byte, error)` and `FetchReturnsOnCall(n, …)` used for M2 + M4 specs.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`

Verified symbols (do NOT change):
- `agentlib.ExtractSection[T any](ctx, md, "## Plan")` returns `(T, error)` — used in `steps_ai_review.go:192` and `steps_execution.go:139` as `agentlib.ExtractSection[PlanOutput](ctx, md, "## Plan")`. Use the same form on entry to `planningStep.Run` for the M2 cache lookup.
- `agentlib.MarshalSectionTyped(ctx, "## Plan", output)` round-trips with the above.
- `mocks.ClaudeRunnerMock` API: `RunCallCount() int`, `RunReturns(*claudelib.ClaudeResult, error)`, `RunReturnsOnCall(n int, *claudelib.ClaudeResult, error)`, `RunArgsForCall(n int) (context.Context, string)` — last one inspects the prompt that the n-th call received (use for the M2 assertion that the bump prompt was NOT re-issued).
- `mocks.GitOpsMock.CommittedFilesReturns([]string, error)` is the injection seam for C1.
- `mocks.MaintainerConfigFetcher.FetchReturns([]byte, error)` and `FetchReturnsOnCall(n, …)` are the injection seams for M4 and M2.
- Failed-check constant in `pkg/steps_ai_review.go`: `CheckUnexpectedFileChange` (already defined; do NOT add).
- `bborbe/errors.Wrapf(ctx, err, "…")` is the project's error-wrap idiom (NOT stdlib `fmt.Errorf`).
- `glog.Warningf` and `glog.V(2).Infof` are the project logging idioms.
- `lib/maintainerconfig` imports `gopkg.in/yaml.v3` (verified in import block lines 25-30). `yaml.NewDecoder(r io.Reader).KnownFields(true).Decode(v interface{}) error` is the v3 API used by M5.

Verified existing constants / fields:
- `fetchTimeout = 15 * time.Second` already exists in `fetcher.go:45` (no need to re-introduce).
- `PlanOutput` already has `ConfigFetchWarning` as a NEW field added by this prompt — verify it does NOT already exist before adding. If it does, reuse it (`grep -n ConfigFetchWarning agent/github-releaser/pkg/plan_output.go`).
- `crypto/sha256` is a stdlib import; `encoding/hex` likewise. Both are project-policy-acceptable.
</context>

<requirements>

## Fix C1 — Fail closed when `CommittedFiles` errors in ai-review (CRITICAL correctness)

In `agent/github-releaser/pkg/steps_ai_review.go`, the helper `checkUnexpectedFileChange` (function signature at line 560-565, body lines 566-604) silently returns `nil` when `s.ops.CommittedFiles(ctx, result.Workdir)` errors (current lines 569-575):

```go
files, err := s.ops.CommittedFiles(ctx, result.Workdir)
if err != nil {
    // Transient / missing workdir → controller retries; not a
    // semantic check failure.
    glog.Warningf("ai_review: CommittedFiles failed: %v", err)
    return nil
}
```

This leaves `checks.UnexpectedFileChange` at its zero value `false` (i.e. "no unexpected file change detected") and does NOT append `CheckUnexpectedFileChange` to `*failedChecks` — a transient git error silently passes the check. The two sibling structural checks `verifyTagAtExpectedCommit` (lines 459-468) and `verifyChangelogHeaderRewritten` (lines 498-507) fail closed under the same condition. Mirror them.

C1.1. Replace the error branch at lines 569-575 with the fail-closed pattern:

```go
files, err := s.ops.CommittedFiles(ctx, result.Workdir)
if err != nil {
    checks.UnexpectedFileChange = true
    *failedChecks = append(*failedChecks, CheckUnexpectedFileChange)
    glog.V(2).Infof(
        "ai_review: check=%s result=false: CommittedFiles error: %v",
        CheckUnexpectedFileChange,
        err,
    )
    return nil
}
```

The sentinel sense: `checks.UnexpectedFileChange = true` means "unexpected change detected" — i.e. the check FAILED. Verify by reading the field's consumer in `rollupVerdict` / `finishApproved` (grep `UnexpectedFileChange` in this file) to confirm `true` is the failure state. The existing happy-path branch at lines 591-601 already sets it to `true` on detection, confirming the sense.

C1.2. Update the godoc above `checkUnexpectedFileChange` (currently lines 545-559) to reflect the new contract. Replace the paragraph beginning "Returns the diff …" through "… the structural checks in that case." with:

```
// Returns the diff (committed - expected) for the UnexpectedFiles
// output slice. On workdir-empty short-circuit the check is skipped
// silently and an empty diff is returned. On CommittedFiles error
// the check fails closed: checks.UnexpectedFileChange=true,
// CheckUnexpectedFileChange appended to failedChecks, empty diff
// returned. The release trust model requires fail-closed on
// transient errors — a git blip must not leave the check passing.
```

Keep the `result.Workdir == ""` branch at lines 566-568 unchanged (workdir-empty is a known precondition handled elsewhere via the faithfulness `OverallUnknown` rollup, not a check failure).

C1.3. Add a Ginkgo spec to `agent/github-releaser/pkg/steps_ai_review_test.go` inside the existing `Describe("AIReviewStep", …)` block (or in the same nested `Describe`/`Context` that holds the prior "transport-error fail-closed" specs added in PR 232). Description: `"CommittedFiles error sets UnexpectedFileChange=true and appends CheckUnexpectedFileChange"`.

Scaffolding (reuse the existing pattern from the prior PR-232 transport-error specs — look near the `verifyChangelogHeaderRewritten transport error …` spec for the template):
- `mocks.ReviewClient`: `TagExistsReturns("abc123", nil)`, `ResolveTagCommitReturns("abc123", nil)`, `FetchChangelogReturns(<bytes whose first ## heading is "## v1.0.0">, nil)` so the other two structural checks pass.
- `mocks.GitOpsMock`: `CommittedFilesReturns(nil, errors.New("git: lstat workdir: no such file or directory"))`.
- Use a real on-disk temp workdir created with `os.MkdirTemp("", "ai-review-test-")` + `DeferCleanup(func() { _ = os.RemoveAll(dir) })`. Seed `<workdir>/CHANGELOG.md` with any minimal valid bytes so `checkFaithfulness` does NOT immediately short-circuit on missing file (set `result.Workdir = dir`).
- Stub `mocks.ClaudeRunnerMock.RunReturns(nil, errors.New("claude unavailable"))` to drive `checkFaithfulness` to `OverallUnknown` (so the assertion below on `output.Overall` is unambiguous and not contaminated by faithfulness-pass paths).
- Build a `## Result` with `outcome: released`, `commit_sha: abc123`, `tag: v1.0.0`, `local_tag: v1.0.0`, `workdir: <dir>`, plus a minimal `## Plan` so `Run` proceeds past the short-circuit.
- Invoke `Run`. Parse `## Review` via `agentlib.ExtractSection[pkg.ReviewOutput](ctx, md, "## Review")`.

Assertions:
- `output.Checks.UnexpectedFileChange` is `true` (the failure-sense value — verify above).
- `output.FailedChecks` contains `pkg.CheckUnexpectedFileChange` via `ContainElement`.
- `output.Approved` is `false`.
- Result's `Status == agentlib.AgentStatusFailed`.
- Result's `NextPhase == string(domain.TaskPhaseHumanReview)`.

Do NOT assert on `output.UnexpectedFiles` length (the helper returns `nil` on the error path; that is fine — the failure has already been recorded in `FailedChecks`).

## Fix M1 — Bound `.maintainer.yaml` response body with `http.MaxBytesReader` (DoS hardening)

In `agent/github-releaser/pkg/maintainerconfig/fetcher.go`:

M1.1. Add a package-level constant near `fetchTimeout` (line 45):

```go
// maxConfigBodyBytes caps how many bytes the fetcher will read from the
// GitHub contents API response. .maintainer.yaml is realistically a few
// hundred bytes; 1 MiB is ~3000x that and still bounds malicious or
// misconfigured upstreams from exhausting agent memory.
const maxConfigBodyBytes = 1 << 20 // 1 MiB
```

M1.2. In `httpFetcher.readBody` (lines 189-195), wrap `resp.Body` with `http.MaxBytesReader` BEFORE `io.ReadAll`. Replace the function body with:

```go
func (f *httpFetcher) readBody(ctx context.Context, resp *http.Response) ([]byte, error) {
    body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, maxConfigBodyBytes))
    if err != nil {
        return nil, errors.Wrapf(
            ctx,
            err,
            "fetch .maintainer.yaml: read body (cap=%d bytes)",
            maxConfigBodyBytes,
        )
    }
    return body, nil
}
```

Note: the `nil` first arg is intentional — `http.MaxBytesReader` accepts a `http.ResponseWriter` for server-side use and `nil` for client-side use; the cap still applies.

M1.3. Add a Ginkgo spec to `agent/github-releaser/pkg/maintainerconfig/fetcher_test.go` inside the existing `Describe("httpFetcher", …)` block. Description: `"oversize body rejected with cap in error"`.

```go
It("oversize body rejected with cap in error", func() {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        // Write 2 MiB of data — exceeds the 1 MiB cap. Shape doesn't
        // matter; MaxBytesReader trips before JSON parse.
        big := make([]byte, 2<<20)
        _, _ = w.Write(big)
    }))
    defer server.Close()

    fetcher := maintainerconfig.NewHTTPFetcherForTest("", server.URL)
    data, err := fetcher.Fetch(ctx, "bborbe", "maintainer", "master")

    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("read body"))
    Expect(err.Error()).To(ContainSubstring("cap="))
    Expect(data).To(BeNil())
})
```

Pre-existing tests (the "happy path", "404", "500", and the spec-232 "empty 200 OK body" spec) must continue to pass — none of them write bodies >1 MiB.

## Fix M2 — Cache bump verdict in `## Plan` so re-fires don't re-run bump LLM (cost / determinism)

Problem: on a transient rewrite-LLM failure, `planningStep.Run` returns `AgentStatusFailed` and the controller re-fires. The re-fire re-runs `fetcher.Fetch` (CHANGELOG.md) + `resolveChangelogRewrite` (.maintainer.yaml) + `runner.Run` for BUMP classification + `runner.Run` for REWRITE classification. The bump verdict is deterministic for a given Unreleased body — re-running it is wasted LLM cost and risks a non-deterministic re-verdict. The previous run's `## Plan` section IS still on the task page (the controller does not delete it on failure), so the cache lives there.

M2.1. In `planningStep.Run` (`agent/github-releaser/pkg/steps_planning.go:90`), AFTER the missing-frontmatter / repo-parse / CHANGELOG-fetch / validation block (i.e. just BEFORE the call to `s.resolveChangelogRewrite` at line 146), add a cached-plan lookup. Plan: read the existing `## Plan` section if any; if it carries a non-empty `Bump`, the bump LLM call is already done — pass it through to `runClassification`.

Add this near the top of `Run` (insert AFTER the `readRequired` + `parseOwnerRepo` block, BEFORE `s.fetcher.Fetch`):

```go
// cachedBump is the bump-verdict from a prior partial run (e.g. a
// re-fire after the rewrite LLM transiently failed). Empty on a
// fresh run or when the prior plan was outcome=needs_input/failed
// (those don't carry a real bump). When set, runClassification
// skips the bump LLM call and reuses verdict+reasoning verbatim.
//
// ExtractSection error is non-fatal — a fresh task page has no
// ## Plan section yet; that is the common case.
var cachedBump, cachedReasoning string
if prior, perr := agentlib.ExtractSection[PlanOutput](ctx, md, "## Plan"); perr == nil {
    if prior.Outcome == PlanOutcomeReady && prior.Bump != "" {
        cachedBump = prior.Bump
        cachedReasoning = prior.Reasoning
        glog.V(2).Infof(
            "planning: reusing cached bump=%s from prior ## Plan (skipping bump LLM)",
            cachedBump,
        )
    }
}
```

Note: do NOT add a `cachedBump != ""` early-exit before `s.fetcher.Fetch` — the CHANGELOG bytes, bullets, prefix style, and `originalBody` still need to be re-derived (they are inputs to the rewrite LLM call and to the plan-output assembly). The cache strictly skips the bump LLM call.

M2.2. Plumb `cachedBump` + `cachedReasoning` into `runClassification`. Change the function signature at line 201-209 from:

```go
func (s *planningStep) runClassification(
    ctx context.Context,
    md *agentlib.Markdown,
    currentVersion string,
    bullets []string,
    prefixStyle string,
    originalBody string,
    changelogRewrite bool,
) (*agentlib.Result, error) {
```

to:

```go
func (s *planningStep) runClassification(
    ctx context.Context,
    md *agentlib.Markdown,
    currentVersion string,
    bullets []string,
    prefixStyle string,
    originalBody string,
    changelogRewrite bool,
    cachedBump, cachedReasoning string,
) (*agentlib.Result, error) {
```

Update the single call site in `Run` (currently lines 153-155) to pass `cachedBump, cachedReasoning` as the new trailing args.

M2.3. Inside `runClassification`, branch on the cache BEFORE the bump LLM call. Replace the existing bump pipeline (lines 210-229: build `fullPrompt`, `runner.Run`, `ParseBumpVerdict`) with:

```go
var verdict prompts.BumpVerdict
if cachedBump != "" {
    verdict = prompts.BumpVerdict{Bump: cachedBump, Reasoning: cachedReasoning}
    glog.V(2).Infof("planning: skipping bump LLM call (cached bump=%s)", cachedBump)
} else {
    userMsg := strings.Join(bullets, "\n")
    fullPrompt := prompts.BumpClassificationPrompt() + "\n\n## Bullets to classify\n\n" + userMsg
    runResult, err := s.runner.Run(ctx, fullPrompt)
    if err != nil {
        glog.V(2).Infof("planning: claude runner failed: %v", err)
        return &agentlib.Result{
            Status:  agentlib.AgentStatusFailed,
            Message: "claude run: " + err.Error(),
        }, nil
    }
    v, err := prompts.ParseBumpVerdict(ctx, runResult.Result)
    if err != nil {
        glog.V(2).Infof("planning: parse verdict failed: %v", err)
        return &agentlib.Result{
            Status:  agentlib.AgentStatusFailed,
            Message: "parse bump verdict: " + err.Error(),
        }, nil
    }
    verdict = v
}
```

The downstream `semver.BumpVersion(ctx, currentVersion, verdict.Bump)` call (currently line 231) and the rest of the function stay unchanged.

M2.4. The `prompts.BumpVerdict` struct is verified to have EXACTLY two string fields (confirmed by reading `agent/github-releaser/pkg/prompts/prompts.go` lines 58-64):

```go
type BumpVerdict struct {
    Bump      string `json:"bump"`
    Reasoning string `json:"reasoning"`
}
```

Both fields are load-bearing for downstream code (`semver.BumpVersion(ctx, currentVersion, verdict.Bump)` consumes `Bump`; `PlanOutput.Reasoning` carries `Reasoning` through to the task page). The cache restoration `verdict = prompts.BumpVerdict{Bump: cachedBump, Reasoning: cachedReasoning}` therefore captures the complete struct — no zero-valued fields, no missing downstream data. Do NOT add discovery / grep steps here; the verification is done.

M2.5. Add a Ginkgo spec to `agent/github-releaser/pkg/steps_planning_test.go` inside the existing `Describe(…)` block. Description: `"re-fire after rewrite failure reuses cached bump verdict across a write+reload of the task page (bump LLM called once across two Run invocations)"`.

The test MUST model the production round-trip: after Run #1, serialize `md` via `md.Marshal(ctx)` (the verified method on `*agentlib.Markdown` at `agent_markdown.go:108` — signature `(m *Markdown) Marshal(ctx context.Context) (string, error)`), then re-parse via `agentlib.ParseMarkdown(ctx, serialized)` to obtain a FRESH `*Markdown` for Run #2. In production the controller persists the task page and the next fire reads it fresh — re-using the same in-memory `*md` between Run #1 and Run #2 would prove nothing about the production cache path. The write+reload step is the load-bearing assertion of the entire M2 fix.

```go
It("re-fire after rewrite failure reuses cached bump verdict across write+reload", func() {
    ctx := context.Background()
    fixture := "## Unreleased\n\n- feat: add foo\n\n## v1.7.7\n\n- old\n"
    fakeFetcher := &mocks.Fetcher{}
    fakeFetcher.FetchReturns([]byte(fixture), nil)

    fakeRunner := &mocks.ClaudeRunnerMock{}
    // Run #1: bump LLM call (#0) succeeds, rewrite LLM call (#1) fails.
    fakeRunner.RunReturnsOnCall(0, &claudelib.ClaudeResult{
        Result: `{"bump":"minor","reasoning":"feat detected"}`,
    }, nil)
    fakeRunner.RunReturnsOnCall(1, nil, errors.New("rewrite transient"))
    // Run #2: ONLY the rewrite LLM should fire (call index 2).
    // Bump LLM call must NOT be invoked again — the cache short-circuits it.
    fakeRunner.RunReturnsOnCall(2, &claudelib.ClaudeResult{
        Result: `{"rewrite_needed":false,"rewritten_unreleased":"","reasoning":"already conforms"}`,
    }, nil)

    step := pkg.NewPlanningStep(fakeRunner, fakeFetcher, withChangelogRewriteTrue())

    // Run #1 — fresh task page.
    md1, err := agentlib.ParseMarkdown(ctx, taskMD)
    Expect(err).NotTo(HaveOccurred())
    res1, err := step.Run(ctx, md1)
    Expect(err).NotTo(HaveOccurred())
    Expect(res1.Status).To(Equal(agentlib.AgentStatusFailed))
    Expect(fakeRunner.RunCallCount()).To(Equal(2)) // bump + rewrite both fired in Run #1

    // CRITICAL: model the production round-trip. The controller persists
    // the task page after each fire and the next fire reads it fresh —
    // re-using the in-memory `md1` would not prove the cache survives
    // the on-disk round-trip. Serialize via (*Markdown).Marshal then
    // re-parse via ParseMarkdown to obtain a fresh *Markdown for Run #2.
    serialized, err := md1.Marshal(ctx)
    Expect(err).NotTo(HaveOccurred())
    Expect(serialized).To(ContainSubstring("## Plan"))
    Expect(serialized).To(ContainSubstring(`"bump":"minor"`))

    md2, err := agentlib.ParseMarkdown(ctx, serialized)
    Expect(err).NotTo(HaveOccurred())

    // Sanity: the re-parsed plan section MUST carry bump=minor — this is
    // the precondition the cache lookup depends on. If this fails, the
    // M2 design needs revisiting (the cache cannot survive the round-trip).
    prior, perr := agentlib.ExtractSection[pkg.PlanOutput](ctx, md2, "## Plan")
    Expect(perr).NotTo(HaveOccurred())
    Expect(prior.Bump).To(Equal("minor"))

    // Run #2 — fresh *Markdown re-parsed from serialized form. Expect Done
    // and ONLY ONE additional LLM call (the rewrite retry). Total = 3,
    // proving bump LLM was NOT re-invoked.
    res2, err := step.Run(ctx, md2)
    Expect(err).NotTo(HaveOccurred())
    Expect(res2.Status).To(Equal(agentlib.AgentStatusDone))
    Expect(fakeRunner.RunCallCount()).To(Equal(3)) // +1 rewrite only; bump NOT re-fired

    // Defensive: confirm the third runner call was the rewrite prompt,
    // not a re-issued bump prompt. Use RunArgsForCall(2) to inspect.
    _, promptArg := fakeRunner.RunArgsForCall(2)
    Expect(promptArg).To(ContainSubstring("rewrite"))
})
```

If, during implementation, the write+reload reveals that `(*Markdown).Marshal` does NOT persist the `## Plan` section verbatim (e.g. the framework strips JSON sections it considers transient, or `## Plan` is rewritten on the failure path), then the M2 design premise is invalid — the cache cannot survive a real re-fire. In that case the agent MUST report the finding in the completion summary naming the exact behavior observed (which method drops the section, on which return path), and STOP. Do NOT silently fall back to single-run testing or to re-using the same `*md` instance — the original cache-survival claim is then unproven and the M2 design needs revisiting before any test ships.

Read these specific lines in `agent/github-releaser/pkg/steps_planning.go` to confirm the failure-path invariant BEFORE writing the test: the three `return &agentlib.Result{Status: agentlib.AgentStatusFailed, ...}` sites at lines 115-119, 140-143, and the runner.Run failure path inside `runClassification` (lines 213-228). None of these call `publishPlan`, so on the pure-claude-fail path the prior `## Plan` is not rewritten — that is what makes the write+reload cache lookup viable.

## Fix M3 — Redact non-2xx response body in error string and log (data hygiene)

In `agent/github-releaser/pkg/maintainerconfig/fetcher.go`, `httpFetcher.checkStatus` (lines 197-228) currently embeds the first 200 chars of the response body verbatim in the returned error message. A 5xx body from GitHub or a reverse proxy can contain internal paths, header echoes, or partial stack traces. Strip the body bytes from operator-visible output; keep the status code and a short SHA-256 fingerprint.

M3.1. Add `crypto/sha256` and `encoding/hex` to the import block (lines 12-22). Keep imports grouped per the existing pattern (stdlib block first, third-party second).

M3.2. Replace the non-2xx branch (lines 210-222) with:

```go
if resp.StatusCode < 200 || resp.StatusCode >= 300 {
    // Body is redacted: a 5xx body from GitHub / a proxy can contain
    // internal paths, header echoes, or partial stack traces. Surface
    // the status code (operator-actionable) and a short SHA-256
    // fingerprint (so two reports of the same body can be correlated
    // without exposing the bytes).
    sum := sha256.Sum256(body)
    fingerprint := hex.EncodeToString(sum[:])[:8]
    return errors.Errorf(
        ctx,
        "fetch .maintainer.yaml: status %d body_sha256_prefix=%s body_bytes=%d",
        resp.StatusCode,
        fingerprint,
        len(body),
    )
}
```

Note: do NOT log the raw body in V(2) either — there is no V(2) log in this branch today; the redaction is now self-contained.

M3.3. Add a Ginkgo spec to `agent/github-releaser/pkg/maintainerconfig/fetcher_test.go` inside the existing `Describe("httpFetcher", …)` block. Description: `"500 response with sensitive body returns redacted error (status + sha256 prefix, no raw body bytes)"`.

```go
It("500 response with sensitive body returns redacted error", func() {
    secret := "/internal/path/credentials.json missing — uid=42"
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
        _, _ = w.Write([]byte(secret))
    }))
    defer server.Close()

    fetcher := maintainerconfig.NewHTTPFetcherForTest("", server.URL)
    _, err := fetcher.Fetch(ctx, "bborbe", "maintainer", "master")

    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("status 500"))
    Expect(err.Error()).To(ContainSubstring("body_sha256_prefix="))
    Expect(err.Error()).To(ContainSubstring("body_bytes="))
    // CRITICAL: the raw body bytes MUST NOT leak into the error.
    Expect(err.Error()).NotTo(ContainSubstring("/internal/path"))
    Expect(err.Error()).NotTo(ContainSubstring("credentials.json"))
    Expect(err.Error()).NotTo(ContainSubstring("uid=42"))
})
```

The existing `"500: server error returns wrapped non-2xx error"` spec (fetcher_test.go around line 59-73) asserts only `ContainSubstring("status 500")` on a body of `"boom"`; it continues to pass under M3.

## Fix M4 — Surface `.maintainer.yaml` transport-fetch failure on the task page (observability)

In `agent/github-releaser/pkg/steps_planning.go`, `resolveChangelogRewrite` (lines 173-199) currently silently defaults to `false` AND logs at `glog.Warningf` on any non-`ErrFileNotFound` fetch error (lines 186-189). The log is operator-invisible during normal task review. A repo that opted into `changelogRewrite: true` is silently downgraded on a transient flake — the operator sees the rewrite did not happen but has no signal of WHY.

Fix: keep the `false` default (correct per spec 059 § Failure Modes) AND add a warning string to the plan output that surfaces on the task page.

M4.1. In `agent/github-releaser/pkg/plan_output.go`, FIRST grep for any existing `ConfigFetchWarning` field on `PlanOutput`. If it does not exist, add it after the `ChangelogRewrite` field (around line 67) with:

```go
// ConfigFetchWarning records a non-fatal .maintainer.yaml fetch failure
// that the planning step recovered from by using the default
// changelogRewrite=false. Populated only when the fetch errored with
// something OTHER than ErrFileNotFound (transport, DNS, 5xx, timeout).
// Operators reading the task page can grep this field to confirm
// whether a repo that opted into rewrite was silently downgraded.
// Empty on the happy path (file present) and on the legitimate-absent
// path (404 → ErrFileNotFound).
ConfigFetchWarning string `json:"config_fetch_warning,omitempty"`
```

M4.2. Change the return type of `resolveChangelogRewrite` from `(bool, error)` to `(bool, string, error)` where the new middle string is the warning text (`""` on no-warning paths). Update the four return sites:

- Line 184 (ErrFileNotFound branch): `return false, "", nil`
- Line 189 (non-404 fetch error branch): `return false, fmt.Sprintf(".maintainer.yaml fetch failed (treated as default changelogRewrite=false): %s", err.Error()), nil` — keep the existing `glog.Warningf` line for log-stream observability. Add `"fmt"` to the import block if not already present (it IS already present at line 17).
- Line 196 (parse error branch): `return false, "", errors.Wrapf(ctx, err, "parse .maintainer.yaml")` — parse errors fail closed via the existing path, no warning needed.
- Line 198 (happy path): `return cfg.Release.ChangelogRewrite, "", nil`

M4.3. Update the single call site in `Run` (currently line 146):

```go
changelogRewrite, fetchWarning, err := s.resolveChangelogRewrite(ctx, owner, name, ref)
```

Plumb `fetchWarning` through to `runClassification` and then to `publishPlan` so it lands on the assembled `PlanOutput`. Add `fetchWarning string` as the LAST parameter of both functions (after the M2-introduced `cachedBump, cachedReasoning` params on `runClassification`). In `publishPlan` (line 279-316), set `output.ConfigFetchWarning = fetchWarning` on the `PlanOutput` literal at line 292.

M4.4. Add a Ginkgo spec to `agent/github-releaser/pkg/steps_planning_test.go`. Description: `"non-404 .maintainer.yaml fetch error surfaces config_fetch_warning on plan and logs glog.Warningf"`.

```go
It("non-404 .maintainer.yaml fetch error surfaces config_fetch_warning", func() {
    fixture := "## Unreleased\n\n- feat: add foo\n\n## v1.7.7\n\n- old\n"
    fakeFetcher := &mocks.Fetcher{}
    fakeFetcher.FetchReturns([]byte(fixture), nil)

    // .maintainer.yaml fetcher returns a transport error (not ErrFileNotFound).
    fakeMaintainerCfg := &mocks.MaintainerConfigFetcher{}
    fakeMaintainerCfg.FetchReturns(nil, errors.New("dial tcp api.github.com:443: connection refused"))

    fakeRunner := &mocks.ClaudeRunnerMock{}
    fakeRunner.RunReturnsOnCall(0, &claudelib.ClaudeResult{
        Result: `{"bump":"minor","reasoning":"feat detected"}`,
    }, nil)
    // Rewrite LLM should NOT fire because changelogRewrite resolves to false.

    step := pkg.NewPlanningStep(fakeRunner, fakeFetcher, fakeMaintainerCfg)
    md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
    Expect(err).NotTo(HaveOccurred())
    result, err := step.Run(context.Background(), md)
    Expect(err).NotTo(HaveOccurred())
    Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
    // Only the bump LLM fired (no rewrite — flag defaulted to false).
    Expect(fakeRunner.RunCallCount()).To(Equal(1))

    plan, err := agentlib.ExtractSection[pkg.PlanOutput](
        context.Background(), md, "## Plan",
    )
    Expect(err).NotTo(HaveOccurred())
    Expect(plan.Outcome).To(Equal(pkg.PlanOutcomeReady))
    Expect(plan.ConfigFetchWarning).NotTo(BeEmpty())
    Expect(plan.ConfigFetchWarning).To(ContainSubstring(".maintainer.yaml fetch failed"))
    Expect(plan.ConfigFetchWarning).To(ContainSubstring("connection refused"))
    // ChangelogRewrite resolved to default false.
    Expect(plan.ChangelogRewrite).NotTo(BeNil())
    Expect(*plan.ChangelogRewrite).To(BeFalse())
})
```

Also add a sibling regression spec: `"happy path leaves config_fetch_warning empty"` — reuse any existing happy-path test scaffolding and add `Expect(plan.ConfigFetchWarning).To(BeEmpty())`. This guards against accidentally always-populating the field.

Also add: `"ErrFileNotFound leaves config_fetch_warning empty (legitimate-absent file is not a warning)"` — the existing 404-path test (find it via `grep -n "ErrFileNotFound" agent/github-releaser/pkg/steps_planning_test.go` or, if none, add a fresh one) appends `Expect(plan.ConfigFetchWarning).To(BeEmpty())`.

## Fix M5 — Strict YAML decoding in lib (reject unknown fields)

In `lib/maintainerconfig/maintainerconfig.go`:

M5.1. Replace `Parse` (lines 71-77) with the `KnownFields(true)` decoder pattern:

```go
func Parse(ctx context.Context, content []byte) (MaintainerConfig, error) {
    var cfg MaintainerConfig
    if len(content) == 0 {
        // Preserve the existing "empty bytes -> zero-value, nil" contract
        // (asserted by maintainerconfig_test.go lines 29-31 and 87-89).
        // yaml.NewDecoder(empty).Decode would return io.EOF; the explicit
        // short-circuit keeps the contract crisp.
        return cfg, nil
    }
    dec := yaml.NewDecoder(bytes.NewReader(content))
    dec.KnownFields(true)
    if err := dec.Decode(&cfg); err != nil {
        return MaintainerConfig{}, errors.Wrap(ctx, err, "unmarshal .maintainer.yaml")
    }
    return cfg, nil
}
```

Add `"bytes"` to the import block (line 25-30).

M5.2. Update the package godoc (lines 5-23). Replace the sentence "Unknown top-level keys are tolerated by design (yaml.Unmarshal ignores fields it does not know), which is the forward-compat behavior the spec mandates." with:

```
// Unknown fields (top-level OR nested) are REJECTED at parse time
// (yaml.NewDecoder + KnownFields(true)). This catches typos like
// `changelogRwrite` or `prRevierer` that would otherwise produce a
// silent default-false config — a high-trust .maintainer.yaml is
// load-bearing for release gating, so a typo must fail loudly. To
// add a new bot's namespace, extend MaintainerConfig with the new
// field FIRST (one PR), then deploy the bot (next PR); the brief
// window between the two is the only time a forward-incompat
// .maintainer.yaml would error, and it errors loudly rather than
// silently downgrading.
```

M5.3. Update `lib/maintainerconfig/maintainerconfig_test.go`:

- DELETE the entry at lines 53-57 (`"unknown top-level key ignored, no error"`) — KnownFields(true) makes this case an error, not a success.
- Add a new `It(...)` spec immediately AFTER the `DescribeTable` block, before the existing "malformed YAML" spec (line 100):

```go
It("unknown top-level field rejected", func() {
    _, err := maintainerconfig.Parse(
        ctx,
        []byte("build-fix:\n  enabled: true\nprReviewer:\n  autoApprove: true\n"),
    )
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
    Expect(err.Error()).To(ContainSubstring("field build-fix not found"))
})

It("typo in nested release field rejected", func() {
    // changelogRwrite is the canonical typo from PR-36 review.
    _, err := maintainerconfig.Parse(
        ctx,
        []byte("release:\n  changelogRwrite: true\n"),
    )
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
    Expect(err.Error()).To(ContainSubstring("field changelogRwrite not found"))
})

It("typo in top-level prReviewer key rejected", func() {
    _, err := maintainerconfig.Parse(
        ctx,
        []byte("prRevierer:\n  autoApprove: true\n"),
    )
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
})
```

The exact substring `field <name> not found` is yaml.v3's standard message for `KnownFields(true)` rejection — if precommit fails because the actual message differs, relax to the substring `"not found"` only (still distinguishes from the malformed-YAML test which asserts on `"unmarshal .maintainer.yaml"` alone).

M5.4. After M5.1-M5.3, audit the entire codebase for callers of `lib/maintainerconfig.Parse` that pass YAML containing intentional unknown fields (e.g., fixtures with future-bot stubs). Run:

```
grep -rn "maintainerconfig.Parse(" agent/ lib/ watcher/ scenarios/
```

For each hit, open the file and verify the inputs contain only known fields (`release`, `autoRelease`, `changelogRewrite`, `prReviewer`, `autoApprove`). Report any caller whose input would now error. If you find one, EITHER remove the unknown field from the fixture OR (if the fixture is asserting forward-compat tolerance, which contradicts M5's intent) flag it in the completion summary as a finding for human review — do NOT silently re-add tolerance.

## Out of scope (do NOT change)

- NIT items from the PR review: boolean sentinel patterns, `isBullet`/`isHeading` duplication, `*bool ChangelogRewrite` docs, `Workdir` as plain string — all deferred to a follow-up styling pass.
- Test refactors not directly proving a finding.
- Any change to `lib/maintainerconfig.MaintainerConfig` field set — M5 adds NO new field, it only tightens decoding.
- Any change to the rewrite-LLM prompt content.
- Any change to ai-review `verifyTagExists` (its existing transport-error → retry path is intentional).
- Any change to the watcher (`watcher/`) or the pr-reviewer agent (`agent/pr-reviewer/`).
- Counterfeiter mock regeneration — none of the fixes change an interface signature. `Fetcher.Fetch`, `GitOps.CommittedFiles`, `ClaudeRunner.Run` are all unchanged. The `resolveChangelogRewrite` and `runClassification` signature changes in M2/M4 are on unexported methods of `planningStep`; no mock surface.

</requirements>

<constraints>
- All write edits must be inside `agent/github-releaser/` AND `lib/maintainerconfig/` (M5 is the sole lib edit). Do NOT modify any file under `watcher/`, `agent/pr-reviewer/`, `scenarios/`, or `specs/`.
- Existing tests must continue to pass (the one explicit exception: the `lib/maintainerconfig` test "unknown top-level key ignored" at lines 53-57 is DELETED by M5 and REPLACED with strict-rejection specs).
- Do NOT commit — dark-factory handles git.
- Use `bborbe/errors` for error wrapping (`errors.Wrapf`, `errors.Errorf`, `errors.Wrap`); never stdlib `fmt.Errorf` for new wrapping.
- Use Ginkgo v2 + Gomega for any new test. Match existing file style (table-driven where the existing block uses it, individual `It(...)` blocks otherwise).
- Use counterfeiter fakes from `agent/github-releaser/mocks/` for `GitOps`, `ReviewClient`, `ClaudeRunner`, `Fetcher`, `MaintainerConfigFetcher`. Do NOT hand-roll fakes.
- For any new test that needs the workdir on disk, use `os.MkdirTemp` + `DeferCleanup(func() { _ = os.RemoveAll(dir) })` — mirror existing patterns in `steps_execution_test.go`.
- Do NOT regenerate counterfeiter mocks — no interface signatures changed.
- Do NOT modify the `bborbe/errors` import path or any `go.mod`.
- All new godoc must follow `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md`.
- For M3, ensure `sha256.Sum256` and `hex.EncodeToString` imports use stdlib paths (`crypto/sha256`, `encoding/hex`); no third-party additions.
- For M5, the explicit empty-bytes short-circuit is REQUIRED (yaml.v3 decoder returns `io.EOF` on empty input; the existing contract expects nil error).
- Container-internal coding-doc paths (`/home/node/.claude/plugins/marketplaces/coding/docs/...`) are the ONLY valid form in this prompt — the Obsidian vault is NOT mounted in the YOLO container.
</constraints>

<verification>
Run BOTH precommit suites — both must exit 0:

```
cd agent/github-releaser && make precommit
cd lib && make precommit
```

After both pass, report a completion summary with these explicit sections:

1. **CRITICAL fix-closed pattern applied** — name the file and the specific lines in `pkg/steps_ai_review.go` where the new `checks.UnexpectedFileChange = true; *failedChecks = append(...)` lines now live, plus the `It("...")` description added to `steps_ai_review_test.go` and the four assertions it verifies (`UnexpectedFileChange=true`, `ContainElement(CheckUnexpectedFileChange)`, `Approved=false`, `NextPhase=human_review`).

2. **Each MAJOR item** with file:line and brief approach:
   - M1 — `pkg/maintainerconfig/fetcher.go` `readBody` line range, `maxConfigBodyBytes` constant location, oversize-body test description.
   - M2 — `pkg/steps_planning.go` `Run` cache-lookup line range, `runClassification` signature change, re-fire test description (the write+reload variant), the `RunCallCount()` numbers (must be 2 after Run #1, 3 after Run #2), AND a verbatim quote of the two assertions that prove the round-trip survived: `Expect(serialized).To(ContainSubstring("\"bump\":\"minor\""))` and `Expect(prior.Bump).To(Equal("minor"))` against the re-parsed `md2`.
   - M3 — `pkg/maintainerconfig/fetcher.go` `checkStatus` line range, sha256 import line, redaction test description + the four NOT-containing assertions that prove the body is stripped.
   - M4 — `pkg/plan_output.go` `ConfigFetchWarning` field line, `resolveChangelogRewrite` new return arity, plumb-through path to `publishPlan`, three test descriptions (transport-warning surfacing, happy-path empty, 404-empty).
   - M5 — `lib/maintainerconfig/maintainerconfig.go` `Parse` line range, godoc paragraph replaced (line range), the test entry DELETED (lines 53-57), the three new rejection specs added, results of the `grep -rn "maintainerconfig.Parse(" …` audit (which callers were inspected, none broken / N broken with details).

3. **New tests added per item** — flat list of `(file path, It description)` tuples, one per spec, so a reviewer can locate each.

4. **Precommit pass evidence** — the final exit line of each `make precommit` invocation, or an explicit "exit 0 confirmed" with the trailing make output.

5. **M2 cache-survival verification (write+reload, not in-memory reuse)** — confirm BOTH: (a) the failure path in `planningStep.Run` does NOT rewrite the `## Plan` section — name the lines in `steps_planning.go` inspected (the three return-Failed paths at 115-119, 140-143, and the runner.Run failure path inside `runClassification` at 213-228 — none call `publishPlan`); AND (b) the M2.5 test models the production round-trip by calling `md1.Marshal(ctx)` then `agentlib.ParseMarkdown(ctx, serialized)` to produce a FRESH `*Markdown` (`md2`) for Run #2 — quote the two lines of the test where the marshal and the re-parse happen. If `Marshal` is observed to drop the `## Plan` section on the failure path, do NOT downgrade to single-run testing — report the finding under section 5 and stop.
</verification>
