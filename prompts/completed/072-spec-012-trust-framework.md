---
status: completed
spec: [012-watcher-author-trust-filter]
summary: Created watcher/github/pkg/trust package with boolean-combinator Trust interface, And/Or/Not combinators, NewAuthorAllowlist leaf, and added ParseTrustedAuthors to pkg/filter.go with full test coverage
container: code-reviewer-072-spec-012-trust-framework
dark-factory-version: dev
created: "2026-05-02T10:00:00Z"
queued: "2026-05-02T10:14:28Z"
started: "2026-05-02T10:14:30Z"
completed: "2026-05-02T10:18:59Z"
branch: dark-factory/watcher-author-trust-filter
---

<summary>
- A new `pkg/trust` package provides the canonical Go boolean-combinator pattern for trust decisions (interface, result type, func adapter, And, Or, Not)
- A `PR` input type in the trust package carries the only field needed for trust evaluation — `AuthorLogin`
- The first trust leaf (`AuthorAllowlist`) decides by static GitHub-login membership; an empty members list always denies, as does a PR with no author login
- `And`, `Or`, and `Not` combinators collect per-leaf descriptions into a single audit-trail string so the human-review task body can explain exactly which rule rejected an author
- `ParseTrustedAuthors` is added to `pkg/filter.go`, mirroring `ParseBotAllowlist` in behavior and signature
- A counterfeiter directive on the `Trust` interface generates a mock at `pkg/mocks/trust.go` for use in watcher integration tests
- Full unit tests cover leaf isolation, `And`/`Or`/`Not` algebra, nested compositions, audit-trail presence, and empty-configuration fail-safe
- Existing `filter_test.go` gains `ParseTrustedAuthors` test cases alongside the existing `ParseBotAllowlist` tests
</summary>

<objective>
Create `watcher/github/pkg/trust/` — a standalone boolean-combinator trust-decision package — and add `ParseTrustedAuthors` to `pkg/filter.go`. This package is the foundation for spec-012: the watcher integration (prompt 2) and scenario (prompt 3) depend on it. No watcher code is changed in this prompt.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-boolean-combinator-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — the canonical structure this package must implement (interface, result, func adapter, And, Or, Not, fail-safe empty config).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface + constructor + private struct.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `errors.Wrapf`, `errors.Errorf`, never `fmt.Errorf`.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo v2/Gomega, external test packages, counterfeiter.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `New*` constructors return interfaces.

Files to read before making any changes (read ALL before writing any code):
- `watcher/github/pkg/filter.go` — `ParseBotAllowlist` pattern to mirror
- `watcher/github/pkg/filter_test.go` — style for adding `ParseTrustedAuthors` tests
- `watcher/github/pkg/suite_test.go` — Ginkgo suite file pattern (counterfeiter go:generate line)
- `watcher/github/pkg/githubclient.go` — `PullRequest.AuthorLogin` field (the only field needed from a PR)

Module path (from `go.mod`): `github.com/bborbe/maintainer/watcher/github-pr`
</context>

<requirements>
**Execute steps in this order. Run `make test` only in step 8.**

1. **Create `watcher/github/pkg/trust/trust.go`** (package `trust`):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package trust provides the boolean-combinator trust-decision framework
   // for the github-pr-watcher. Callers compose leaves with And, Or, and Not
   // to express compound trust policies without modifying the watcher core.
   package trust

   import (
       "context"
       "fmt"
       "strings"

       "github.com/bborbe/errors"
   )

   // PR is the trust-evaluation input derived from a GitHub pull request.
   // Only the fields needed for trust decisions are included.
   type PR struct {
       AuthorLogin string
   }

   // Result captures a trust decision and a human-readable description.
   // The description is the audit trail: why was this PR trusted or denied?
   type Result interface {
       Success() bool
       Description() string
   }

   type result struct {
       success     bool
       description string
   }

   // NewResult constructs a Result with the given outcome and description.
   func NewResult(success bool, description string) Result {
       return &result{success: success, description: description}
   }

   func (r *result) Success() bool       { return r.success }
   func (r *result) Description() string { return r.description }

   //counterfeiter:generate -o ../mocks/trust.go --fake-name Trust . Trust

   // Trust is the single-method trust-decision interface.
   // Each implementation returns both an outcome (trusted/denied) and a
   // human-readable description forming the audit trail for the decision.
   type Trust interface {
       IsTrusted(ctx context.Context, pr PR) (Result, error)
   }

   // TrustFunc adapts a plain function to the Trust interface.
   // Useful for inline trust checks without struct boilerplate.
   type TrustFunc func(ctx context.Context, pr PR) (Result, error)

   func (f TrustFunc) IsTrusted(ctx context.Context, pr PR) (Result, error) {
       return f(ctx, pr)
   }

   // And succeeds only if every member succeeds.
   // Evaluates ALL members (no short-circuit) to produce a complete audit trail.
   // An empty And returns vacuous success — callers must ensure non-empty slices
   // for security-relevant decisions (startup validation enforces this).
   type And []Trust

   func (a And) IsTrusted(ctx context.Context, pr PR) (Result, error) {
       var descriptions []string
       success := true
       for _, t := range a {
           r, err := t.IsTrusted(ctx, pr)
           if err != nil {
               return nil, errors.Wrapf(ctx, err, "and trust check")
           }
           if !r.Success() {
               success = false
           }
           descriptions = append(descriptions, fmt.Sprintf("[%t] %s", r.Success(), r.Description()))
       }
       return NewResult(success, "and("+strings.Join(descriptions, ", ")+")"), nil
   }

   // Or succeeds if any member succeeds.
   // Evaluates ALL members (no short-circuit) to produce a complete audit trail.
   // An empty Or returns vacuous failure — callers must ensure non-empty slices
   // for security-relevant decisions.
   type Or []Trust

   func (o Or) IsTrusted(ctx context.Context, pr PR) (Result, error) {
       var descriptions []string
       success := false
       for _, t := range o {
           r, err := t.IsTrusted(ctx, pr)
           if err != nil {
               return nil, errors.Wrapf(ctx, err, "or trust check")
           }
           if r.Success() {
               success = true
           }
           descriptions = append(descriptions, fmt.Sprintf("[%t] %s", r.Success(), r.Description()))
       }
       return NewResult(success, "or("+strings.Join(descriptions, ", ")+")"), nil
   }

   // Not inverts the decision of the wrapped Trust, preserving the description.
   func Not(t Trust) Trust {
       return TrustFunc(func(ctx context.Context, pr PR) (Result, error) {
           r, err := t.IsTrusted(ctx, pr)
           if err != nil {
               return nil, errors.Wrapf(ctx, err, "not trust check")
           }
           return NewResult(!r.Success(), "not("+r.Description()+")"), nil
       })
   }
   ```

2. **Create `watcher/github/pkg/trust/author_allowlist.go`** (package `trust`):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package trust

   import (
       "context"
       "fmt"
   )

   // NewAuthorAllowlist returns a Trust leaf that grants trust by exact GitHub
   // login membership (case-sensitive byte equality).
   //
   // Fail-safe rules:
   //   - Empty members list → always denies.
   //   - PR with empty AuthorLogin → always denies (author unknown).
   //
   // Operators are responsible for matching GitHub's stored casing.
   func NewAuthorAllowlist(members []string) Trust {
       return TrustFunc(func(_ context.Context, pr PR) (Result, error) {
           if len(members) == 0 {
               return NewResult(false, "author-allowlist is empty (always denies)"), nil
           }
           if pr.AuthorLogin == "" {
               return NewResult(false, "author unknown (no login provided)"), nil
           }
           for _, m := range members {
               if m == pr.AuthorLogin {
                   return NewResult(true, fmt.Sprintf("author %q in trusted-authors allowlist", pr.AuthorLogin)), nil
               }
           }
           return NewResult(false, fmt.Sprintf("author %q not in trusted-authors allowlist", pr.AuthorLogin)), nil
       })
   }
   ```

3. **Create `watcher/github/pkg/trust/suite_test.go`** (package `trust_test`):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package trust_test

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   //go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate

   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Trust Suite", suiteConfig, reporterConfig)
   }
   ```

4. **Create `watcher/github/pkg/trust/trust_test.go`** (package `trust_test`):

   Cover: And algebra, Or algebra, Not algebra, nested compositions, audit-trail presence in compound descriptions, error propagation, empty-list behavior.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package trust_test

   import (
       "context"
       "fmt"

       "github.com/bborbe/errors"
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
   )

   // alwaysTrust returns a Trust that always succeeds with the given label.
   func alwaysTrust(label string) trust.Trust {
       return trust.TrustFunc(func(_ context.Context, _ trust.PR) (trust.Result, error) {
           return trust.NewResult(true, label), nil
       })
   }

   // alwaysDeny returns a Trust that always denies with the given label.
   func alwaysDeny(label string) trust.Trust {
       return trust.TrustFunc(func(_ context.Context, _ trust.PR) (trust.Result, error) {
           return trust.NewResult(false, label), nil
       })
   }

   // alwaysError returns a Trust that always returns an error.
   func alwaysError(msg string) trust.Trust {
       return trust.TrustFunc(func(ctx context.Context, _ trust.PR) (trust.Result, error) {
           return nil, errors.Errorf(ctx, "%s", msg)
       })
   }

   var _ = Describe("trust.And", func() {
       pr := trust.PR{AuthorLogin: "alice"}

       It("succeeds when all members trust", func() {
           a := trust.And{alwaysTrust("leaf-a"), alwaysTrust("leaf-b")}
           r, err := a.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeTrue())
           Expect(r.Description()).To(ContainSubstring("leaf-a"))
           Expect(r.Description()).To(ContainSubstring("leaf-b"))
       })

       It("denies when any member denies, collecting all descriptions", func() {
           a := trust.And{alwaysTrust("leaf-a"), alwaysDeny("leaf-b")}
           r, err := a.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
           Expect(r.Description()).To(ContainSubstring("leaf-a"))
           Expect(r.Description()).To(ContainSubstring("leaf-b"))
       })

       It("evaluates all members even after first denial (full audit trail)", func() {
           callCount := 0
           counter := trust.TrustFunc(func(_ context.Context, _ trust.PR) (trust.Result, error) {
               callCount++
               return trust.NewResult(false, fmt.Sprintf("leaf-%d", callCount)), nil
           })
           a := trust.And{alwaysDeny("first"), counter, counter}
           r, err := a.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
           Expect(callCount).To(Equal(2), "And must evaluate all members for complete audit trail")
       })

       It("wraps errors from members", func() {
           a := trust.And{alwaysTrust("ok"), alwaysError("boom")}
           _, err := a.IsTrusted(context.Background(), pr)
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("boom"))
       })

       It("empty And returns vacuous success", func() {
           a := trust.And{}
           r, err := a.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeTrue())
       })
   })

   var _ = Describe("trust.Or", func() {
       pr := trust.PR{AuthorLogin: "alice"}

       It("succeeds when any member trusts", func() {
           o := trust.Or{alwaysDeny("leaf-a"), alwaysTrust("leaf-b")}
           r, err := o.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeTrue())
           Expect(r.Description()).To(ContainSubstring("leaf-a"))
           Expect(r.Description()).To(ContainSubstring("leaf-b"))
       })

       It("denies when all members deny", func() {
           o := trust.Or{alwaysDeny("leaf-a"), alwaysDeny("leaf-b")}
           r, err := o.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
       })

       It("evaluates all members (full audit trail)", func() {
           callCount := 0
           counter := trust.TrustFunc(func(_ context.Context, _ trust.PR) (trust.Result, error) {
               callCount++
               return trust.NewResult(true, fmt.Sprintf("leaf-%d", callCount)), nil
           })
           o := trust.Or{alwaysTrust("first"), counter, counter}
           _, err := o.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(callCount).To(Equal(2), "Or must evaluate all members for complete audit trail")
       })

       It("wraps errors from members", func() {
           o := trust.Or{alwaysError("boom"), alwaysTrust("ok")}
           _, err := o.IsTrusted(context.Background(), pr)
           Expect(err).To(HaveOccurred())
       })

       It("empty Or returns vacuous failure", func() {
           o := trust.Or{}
           r, err := o.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
       })
   })

   var _ = Describe("trust.Not", func() {
       pr := trust.PR{AuthorLogin: "alice"}

       It("inverts a trusting leaf", func() {
           r, err := trust.Not(alwaysTrust("leaf")).IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
           Expect(r.Description()).To(ContainSubstring("not("))
           Expect(r.Description()).To(ContainSubstring("leaf"))
       })

       It("inverts a denying leaf", func() {
           r, err := trust.Not(alwaysDeny("leaf")).IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeTrue())
       })

       It("propagates errors from the wrapped leaf", func() {
           _, err := trust.Not(alwaysError("boom")).IsTrusted(context.Background(), pr)
           Expect(err).To(HaveOccurred())
       })
   })

   var _ = Describe("nested compositions", func() {
       pr := trust.PR{AuthorLogin: "alice"}

       It("And{Or{trusted,denied}, Not{denied}} succeeds", func() {
           compound := trust.And{
               trust.Or{alwaysTrust("or-a"), alwaysDeny("or-b")},
               trust.Not(alwaysDeny("not-leaf")),
           }
           r, err := compound.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeTrue())
           // Audit trail contains leaf labels from all levels
           Expect(r.Description()).To(ContainSubstring("or-a"))
           Expect(r.Description()).To(ContainSubstring("not-leaf"))
       })

       It("And{Or{denied,denied}, Not{trusted}} denies with full trail", func() {
           compound := trust.And{
               trust.Or{alwaysDeny("or-a"), alwaysDeny("or-b")},
               trust.Not(alwaysTrust("not-leaf")),
           }
           r, err := compound.IsTrusted(context.Background(), pr)
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
           Expect(r.Description()).To(ContainSubstring("or-a"))
           Expect(r.Description()).To(ContainSubstring("or-b"))
           Expect(r.Description()).To(ContainSubstring("not-leaf"))
       })
   })
   ```

5. **Create `watcher/github/pkg/trust/author_allowlist_test.go`** (package `trust_test`):

   Cover: empty members, author in list, author not in list, empty AuthorLogin, case sensitivity, description content.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package trust_test

   import (
       "context"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
   )

   var _ = Describe("trust.NewAuthorAllowlist", func() {
       var ctx context.Context

       BeforeEach(func() {
           ctx = context.Background()
       })

       It("always denies when members list is empty", func() {
           leaf := trust.NewAuthorAllowlist(nil)
           r, err := leaf.IsTrusted(ctx, trust.PR{AuthorLogin: "alice"})
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
           Expect(r.Description()).To(ContainSubstring("empty"))
       })

       It("always denies when members list is empty slice", func() {
           leaf := trust.NewAuthorAllowlist([]string{})
           r, err := leaf.IsTrusted(ctx, trust.PR{AuthorLogin: "alice"})
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
       })

       It("trusts an author present in the list (exact match)", func() {
           leaf := trust.NewAuthorAllowlist([]string{"alice", "bob"})
           r, err := leaf.IsTrusted(ctx, trust.PR{AuthorLogin: "alice"})
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeTrue())
           Expect(r.Description()).To(ContainSubstring("alice"))
       })

       It("denies an author not in the list", func() {
           leaf := trust.NewAuthorAllowlist([]string{"alice", "bob"})
           r, err := leaf.IsTrusted(ctx, trust.PR{AuthorLogin: "carol"})
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
           Expect(r.Description()).To(ContainSubstring("carol"))
       })

       It("is case-sensitive: Alice does not match alice", func() {
           leaf := trust.NewAuthorAllowlist([]string{"alice"})
           r, err := leaf.IsTrusted(ctx, trust.PR{AuthorLogin: "Alice"})
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
       })

       It("denies when AuthorLogin is empty, with 'unknown' in description", func() {
           leaf := trust.NewAuthorAllowlist([]string{"alice"})
           r, err := leaf.IsTrusted(ctx, trust.PR{AuthorLogin: ""})
           Expect(err).NotTo(HaveOccurred())
           Expect(r.Success()).To(BeFalse())
           Expect(r.Description()).To(ContainSubstring("unknown"))
       })

       It("description is non-empty for all outcomes", func() {
           leaf := trust.NewAuthorAllowlist([]string{"alice"})
           for _, login := range []string{"alice", "stranger", ""} {
               r, err := leaf.IsTrusted(ctx, trust.PR{AuthorLogin: login})
               Expect(err).NotTo(HaveOccurred())
               Expect(r.Description()).NotTo(BeEmpty())
           }
       })
   })
   ```

6. **Add `ParseTrustedAuthors` to `watcher/github/pkg/filter.go`**:

   Append this function after `ParseBotAllowlist`. The implementation is identical — the separate function name is intentional for operator-facing clarity:

   ```go
   // ParseTrustedAuthors splits a comma-separated trusted-authors string into a
   // slice of trimmed, non-empty entries. Mirrors ParseBotAllowlist in behavior.
   func ParseTrustedAuthors(raw string) []string {
       if raw == "" {
           return nil
       }
       parts := strings.Split(raw, ",")
       result := make([]string, 0, len(parts))
       for _, p := range parts {
           p = strings.TrimSpace(p)
           if p != "" {
               result = append(result, p)
           }
       }
       return result
   }
   ```

7. **Add `ParseTrustedAuthors` test cases to `watcher/github/pkg/filter_test.go`**:

   Add a new `Describe("ParseTrustedAuthors", ...)` block (after the existing `ParseBotAllowlist` block):

   ```go
   Describe("ParseTrustedAuthors", func() {
       It("returns nil for empty string", func() {
           Expect(pkg.ParseTrustedAuthors("")).To(BeNil())
       })

       It("returns single entry", func() {
           Expect(pkg.ParseTrustedAuthors("alice")).To(Equal([]string{"alice"}))
       })

       It("returns multiple comma-separated entries trimmed", func() {
           Expect(
               pkg.ParseTrustedAuthors("alice , bob"),
           ).To(Equal([]string{"alice", "bob"}))
       })

       It("filters out whitespace-only entries", func() {
           Expect(
               pkg.ParseTrustedAuthors("alice,  ,bob"),
           ).To(Equal([]string{"alice", "bob"}))
       })

       It("filters out trailing empty entry from trailing comma", func() {
           Expect(pkg.ParseTrustedAuthors("alice,")).To(Equal([]string{"alice"}))
       })
   })
   ```

8. **Compile and run tests**:
   ```bash
   cd watcher/github && go build ./...
   cd watcher/github && make test
   ```
</requirements>

<constraints>
- Only create/modify files in `watcher/github/pkg/trust/` and `watcher/github/pkg/filter.go` and `watcher/github/pkg/filter_test.go`
- Do NOT modify any watcher, factory, or main.go files — that is prompt 2
- Do NOT commit — dark-factory handles git
- All error wrapping uses `github.com/bborbe/errors` (`errors.Wrapf`, `errors.Errorf`) — never `fmt.Errorf`
- The `Trust` interface must use `IsTrusted(ctx context.Context, pr PR) (Result, error)` as its method signature
- `And` and `Or` must evaluate ALL members (no short-circuit) to preserve full audit trails
- `NewAuthorAllowlist` with empty `members` must always deny — this is a security invariant
- `NewAuthorAllowlist` with empty `AuthorLogin` must always deny with a description containing "unknown"
- Case matching is exact byte equality — do NOT use `strings.EqualFold` or similar
- The `//counterfeiter:generate` directive on the `Trust` interface generates `../mocks/trust.go` — do NOT run `go generate` in this prompt; that is done in prompt 2 via `make generate`
- Test packages are external (`package trust_test`), not `package trust`
- `make precommit` is NOT run in this prompt — only `make test`; precommit runs in prompt 2
- `CHANGELOG.md` is NOT updated in this prompt — that is done in prompt 2
</constraints>

<verification>
cd watcher/github && go build ./...

cd watcher/github && make test

# Confirm trust package compiles standalone
go build github.com/bborbe/maintainer/watcher/github-pr/pkg/trust

# Confirm ParseTrustedAuthors exists in filter.go
grep -n "ParseTrustedAuthors" watcher/github/pkg/filter.go

# Confirm counterfeiter directive is present
grep -n "counterfeiter:generate" watcher/github/pkg/trust/trust.go

# Confirm go:generate is present in trust suite
grep -n "go:generate" watcher/github/pkg/trust/suite_test.go
</verification>
