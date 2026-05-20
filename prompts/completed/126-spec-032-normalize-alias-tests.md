---
status: completed
spec: [032-rename-task-status-phase-taxonomy]
summary: 'Created agent/pr-reviewer/domain_normalize_test.go with two Ginkgo It blocks exercising NormalizeTaskStatus("todo")→TaskStatusNext and NormalizeTaskPhase("in_progress")→TaskPhaseExecution alias round-trips; added CHANGELOG entry under ## Unreleased.'
container: maintainer-exec-126-spec-032-normalize-alias-tests
dark-factory-version: v0.162.0
created: "2026-05-20T16:50:00Z"
queued: "2026-05-20T17:20:01Z"
started: "2026-05-20T17:53:39Z"
completed: "2026-05-20T17:56:21Z"
branch: dark-factory/rename-task-status-phase-taxonomy
---

<summary>
- A new test file `agent/pr-reviewer/domain_normalize_test.go` is added that explicitly tests vault-cli's normalization of legacy alias values to the new canonical taxonomy.
- `domain.NormalizeTaskStatus("todo")` is asserted to return `domain.TaskStatusNext` — confirming that existing vault tasks with `status: todo` are correctly recognized as canonical `next` when read via normalize.
- `domain.NormalizeTaskPhase("in_progress")` is asserted to return `domain.TaskPhaseExecution` — confirming the same for tasks with `phase: in_progress`.
- These tests serve as living documentation of the alias contract and will catch any future vault-cli regression that accidentally removes the aliases.
- `make precommit` in `agent/pr-reviewer` exits 0.
</summary>

<objective>
Add two alias round-trip tests in `agent/pr-reviewer` that exercise `domain.NormalizeTaskStatus("todo")` → `domain.TaskStatusNext` and `domain.NormalizeTaskPhase("in_progress")` → `domain.TaskPhaseExecution`, satisfying the spec-032 acceptance criterion that at least one normalize test per dimension is present and verifiable via grep.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions and YOLO container rules.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo v2, Gomega, external test packages (`*_test`), DescribeTable/Entry style.
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

**Precondition — this prompt requires `1-spec-032-dep-bump-and-defaults` to have completed successfully.**
If vault-cli has not been bumped to the version with `TaskStatusNext` and `TaskPhaseExecution`, stop immediately with `status: failed` (do not create any test file).

Verify the precondition before any code change:
```bash
cd agent/pr-reviewer && go doc github.com/bborbe/vault-cli/pkg/domain TaskStatusNext
```
If this exits non-zero, stop with `status: failed`, message: "Precondition not met: requires 1-spec-032-dep-bump-and-defaults to complete first — `domain.TaskStatusNext` not accessible in agent/pr-reviewer module."

**Files to read before writing any code:**
- `agent/pr-reviewer/main_test.go` — existing Ginkgo suite runner in `package main_test`; the new file shares this suite (no new `TestSuite` function needed)
- `agent/pr-reviewer/go.mod` — vault-cli version; must show the TARGET_VERSION from 1-spec-032-dep-bump-and-defaults (not the old v0.64.1)

**Grep-verify constant names before writing:**
The Go identifier names for the new canonical values depend on the vault-cli author's choice. Before writing any assertion, grep the cached module source to confirm exact names:

```bash
# Find the Go constant whose value is "next"
grep -n 'TaskStatus.*= "next"' $(go env GOPATH)/pkg/mod/github.com/bborbe/vault-cli@$(cd agent/pr-reviewer && go list -m github.com/bborbe/vault-cli | awk '{print $2}')/pkg/domain/task_status.go

# Find the Go constant whose value is "execution"
grep -n 'TaskPhase.*= "execution"' $(go env GOPATH)/pkg/mod/github.com/bborbe/vault-cli@$(cd agent/pr-reviewer && go list -m github.com/bborbe/vault-cli | awk '{print $2}')/pkg/domain/task_phase.go
```

Use the actual constant names from the grep output in Step 2. Do NOT assume `TaskStatusNext` or `TaskPhaseExecution` are the exact identifiers — grep is mandatory.

**Test file placement:**
Create a new file `agent/pr-reviewer/domain_normalize_test.go` in `package main_test`. This file sits alongside `main_test.go` in the same directory and package. It participates in the existing `TestSuite` registered in `main_test.go` — no new `TestSuite` function is needed.
</context>

<requirements>
Execute steps in order. Run `make test` after Step 2. Run `make precommit` only at the final step.

---

## Step 1 — Verify precondition

```bash
cd agent/pr-reviewer && go doc github.com/bborbe/vault-cli/pkg/domain TaskStatusNext
cd agent/pr-reviewer && go doc github.com/bborbe/vault-cli/pkg/domain TaskPhaseExecution
```

- Both must exit 0.
- If either exits non-zero: stop. Report `status: failed`, message: "vault-cli with TaskStatusNext/TaskPhaseExecution not yet available in agent/pr-reviewer. Ensure 1-spec-032-dep-bump-and-defaults completed successfully."

Also grep-verify the exact Go constant identifier names (the identifiers may differ from the expected `TaskStatusNext`/`TaskPhaseExecution`):

```bash
# Resolve the cached vault-cli version path
VAULT_CLI_VERSION=$(cd agent/pr-reviewer && go list -m github.com/bborbe/vault-cli | awk '{print $2}')
VAULT_CLI_PATH=$(go env GOPATH)/pkg/mod/github.com/bborbe/vault-cli@${VAULT_CLI_VERSION}

# Find the constant identifier for the "next" status
grep -n 'TaskStatus.*= "next"' ${VAULT_CLI_PATH}/pkg/domain/task_status.go

# Find the constant identifier for the "execution" phase
grep -n 'TaskPhase.*= "execution"' ${VAULT_CLI_PATH}/pkg/domain/task_phase.go
```

Note the exact identifier names (e.g. `TaskStatusNext`, `TaskStatusActive`, etc.) — use those in Step 2.

---

## Step 2 — Create `agent/pr-reviewer/domain_normalize_test.go`

Create the file with the following structure. Substitute the actual constant names identified in Step 1 for `<STATUS_CONST>` and `<PHASE_CONST>`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/vault-cli/pkg/domain"
)

var _ = Describe("vault-cli normalize alias round-trips", func() {
	Describe("NormalizeTaskStatus", func() {
		It("maps legacy 'todo' to the canonical <STATUS_CONST>", func() {
			status, ok := domain.NormalizeTaskStatus("todo")
			Expect(ok).To(BeTrue())
			Expect(status).To(Equal(domain.<STATUS_CONST>))
		})
	})

	Describe("NormalizeTaskPhase", func() {
		It("maps legacy 'in_progress' to the canonical <PHASE_CONST>", func() {
			phase, ok := domain.NormalizeTaskPhase("in_progress")
			Expect(ok).To(BeTrue())
			Expect(phase).To(Equal(domain.<PHASE_CONST>))
		})
	})
})
```

The copyright header and package declaration must match `main_test.go` exactly (same year, same license).

**Example** (if grep confirmed `TaskStatusNext` and `TaskPhaseExecution`):

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/vault-cli/pkg/domain"
)

var _ = Describe("vault-cli normalize alias round-trips", func() {
	Describe("NormalizeTaskStatus", func() {
		It("maps legacy 'todo' to the canonical TaskStatusNext", func() {
			status, ok := domain.NormalizeTaskStatus("todo")
			Expect(ok).To(BeTrue())
			Expect(status).To(Equal(domain.TaskStatusNext))
		})
	})

	Describe("NormalizeTaskPhase", func() {
		It("maps legacy 'in_progress' to the canonical TaskPhaseExecution", func() {
			phase, ok := domain.NormalizeTaskPhase("in_progress")
			Expect(ok).To(BeTrue())
			Expect(phase).To(Equal(domain.TaskPhaseExecution))
		})
	})
})
```

---

## Step 3 — Run `make test` (fast feedback)

```bash
cd agent/pr-reviewer && make test
```

Both new `It` blocks must pass. Likely failure modes:

- **Compile error `domain.TaskStatusNext undefined`**: the constant name differs from your grep result. Re-check Step 1 grep output and fix the identifier name in the file.
- **`Expect(status).To(Equal(domain.TaskStatusNext))` fails** with actual = `TaskStatusTodo`: the vault-cli version was not bumped. Verify `agent/pr-reviewer/go.mod` shows the TARGET_VERSION — if it still shows v0.64.1, prompt 1 did not complete. Stop with `status: failed`.
- **`NormalizeTaskStatus` returns `(domain.TaskStatusTodo, true)` for input `"todo"`**: the new vault-cli still treats "todo" as canonical (not an alias) in this version. This means the selected version pre-dates the taxonomy flip. This prompt cannot recover by re-selecting a version — that decision is owned by prompt 1. Stop with `status: failed` and document that prompt 1 chose a vault-cli version whose taxonomy flip is incomplete; the operator must rerun prompt 1 against a published vault-cli that has both `TaskStatusNext` as canonical AND `"todo"` as a legacy alias.

---

## Step 4 — Add CHANGELOG entry

Read `CHANGELOG.md` at the repo root. Append to existing `## Unreleased` section (created by prompt 1) or create it:

```
- test(agent/pr-reviewer): add `NormalizeTaskStatus("todo")` → `TaskStatusNext` and `NormalizeTaskPhase("in_progress")` → `TaskPhaseExecution` alias round-trip tests to document and guard vault-cli's legacy alias contract (spec 032)
```

---

## Step 5 — Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0.
</requirements>

<constraints>
- **Precondition gate** — if `TaskStatusNext` or `TaskPhaseExecution` don't exist in the current vault-cli dep in `agent/pr-reviewer/go.mod`, stop with `status: failed`. Do NOT create the test file with made-up or assumed constant names.
- **Grep-verify constant names** — do not hard-code `TaskStatusNext` / `TaskPhaseExecution` without running the grep in Step 1. Those identifiers were inferred from the spec and may differ.
- **Only `agent/pr-reviewer`** — the acceptance criterion requires "at least one test" per dimension in the codebase; one module is sufficient. Do not create matching test files in `watcher/github-build` or `watcher/github-pr`.
- **Two separate `It` blocks** — one for NormalizeTaskStatus, one for NormalizeTaskPhase. Do not merge into a single DescribeTable; independent test granularity is more legible.
- **External `_test` package** — file is `package main_test`, not `package main`.
- **No new `TestSuite` function** — the file shares the suite runner already declared in `agent/pr-reviewer/main_test.go`.
- **Do NOT commit** — dark-factory handles git.
- **`make precommit` in `agent/pr-reviewer/` only** — never at repo root.
- **No `fmt.Errorf`** — `bborbe/errors` for any error wrapping (not applicable here, but the global constraint applies).
</constraints>

<verification>
```bash
# 1. Confirm new test file exists with both Describe blocks
grep -n "NormalizeTaskStatus\|NormalizeTaskPhase" agent/pr-reviewer/domain_normalize_test.go
# Expected: ≥2 matches (one per normalize function)

# 2. Confirm spec-032 acceptance criterion: ≥2 lines in test files with the normalize calls
grep -rn 'NormalizeTaskStatus("todo")\|NormalizeTaskPhase("in_progress")' --include='*_test.go' --exclude-dir=vendor
# Expected: ≥2 lines total (one per dimension)

# 3. Confirm assertions use new canonical constants, not string literals
grep -n 'TaskStatusNext\|TaskPhaseExecution' agent/pr-reviewer/domain_normalize_test.go
# Expected: ≥2 matches (the Equal() matchers)
# If the vault-cli author used different constant names, substitute them in the above grep

# 4. Confirm test file is in the correct package
head -5 agent/pr-reviewer/domain_normalize_test.go
# Expected: package main_test

# 5. Confirm CHANGELOG entry
grep -n 'NormalizeTaskStatus\|normalize.*alias\|alias.*round' CHANGELOG.md | head -3
# Expected: one match under ## Unreleased

# 6. Final precommit
(cd agent/pr-reviewer && make precommit)
# Expected: exit 0
```
</verification>
