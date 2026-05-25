---
status: approved
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:00:21Z"
---

<summary>
- Fix over-aggressive log redaction regex that matches any 40+ character hex string
- Narrow the generic hex pattern to exactly 40 characters (SHA-1 length) to reduce false positives
- Add #nosec annotation to AWS secret key regex pattern to prevent gosec false positives
</summary>

<objective>
Fix the over-aggressive `redactOpaqueHexRE` regex in `pkg/watcher.go` that matches any 40+ character hex string. Narrow it to exactly 40 characters (`\b[a-f0-9]{40}\b`) to reduce false positives. Also add `#nosec G101` annotation to the AWS secret key regex since it triggers gosec's hardcoded credential check incorrectly.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/pkg/watcher.go` lines 480-522 (`redactLogSnippet` and regex definitions)
</context>

<requirements>
1. Change `redactOpaqueHexRE` from:
   ```go
   redactOpaqueHexRE = regexp.MustCompile(`\b[a-f0-9]{40,}\b`)
   ```
   To exactly 40 characters:
   ```go
   redactOpaqueHexRE = regexp.MustCompile(`\b[a-f0-9]{40}\b`)
   ```
   The comment at line 488-489 already acknowledges the 40+ pattern redact the episode SHA — narrowing to exactly 40 reduces false positives while still catching SHA-1 hashes.

2. Add a `#nosec G101` annotation above the `redactAWSSecretKeyRE` line to prevent gosec from flagging this as a hardcoded credential. The pattern is intentionally a redaction rule, not an actual credential:
   ```go
   // #nosec G101 — this pattern redacts user-provided AWS secret keys from CI logs, it is not a hardcoded credential
   redactAWSSecretKeyRE = regexp.MustCompile(
       `(aws_secret_access_key[\s=:]+["']?)[A-Za-z0-9/+]{40}["']?`,
   )
   ```

3. Update the comment at line 516-518 to reflect the exact-40 change:
   ```go
   // 5. SHA-1 hashes (exactly 40 hex chars) — generic auth hash catch-all.
   //    Runs last so specific patterns above (1-4) match their tokens first.
   ```

4. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.
</requirements>

<constraints>
- Only change `watcher/github-build/pkg/watcher.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
