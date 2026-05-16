---
status: completed
summary: Created watcher/github/tools.go with //go:build tools tag and blank imports for all CLI tool dependencies, then ran go mod tidy and make precommit successfully.
container: code-reviewer-043-review-watcher-github-add-tools-go
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T14:51:25Z"
started: "2026-04-28T14:51:26Z"
completed: "2026-04-28T14:54:20Z"
---

<summary>
- watcher/github has no tools.go file to pin tool dependencies as direct imports
- Without tools.go, go mod tidy will drop all tool dependencies from go.mod as unused
- This silently breaks make precommit for any developer or CI run starting from a clean state
- The sibling service agent/pr-reviewer has tools.go with the exact set of tools needed
- The fix is to copy agent/pr-reviewer/tools.go into watcher/github/tools.go with updated package
</summary>

<objective>
Create `watcher/github/tools.go` with a `//go:build tools` tag that imports all CLI tools needed by `make precommit` as blank imports. This pins them as direct dependencies so `go mod tidy` does not drop them.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/tools.go` (full): the reference tools.go to copy from
- `watcher/github/go.mod`: verify all tools listed in tools.go are already in go.mod (they should be as indirect deps)
</context>

<requirements>
1. **Create `watcher/github/tools.go`** with the exact content from `agent/pr-reviewer/tools.go` but with `package tools`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   //go:build tools
   // +build tools

   package tools

   import (
       _ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
       _ "github.com/google/addlicense"
       _ "github.com/google/osv-scanner/v2/cmd/osv-scanner"
       _ "github.com/incu6us/goimports-reviser/v3"
       _ "github.com/kisielk/errcheck"
       _ "github.com/maxbrunsfeld/counterfeiter/v6"
       _ "github.com/onsi/ginkgo/v2/ginkgo"
       _ "github.com/securego/gosec/v2/cmd/gosec"
       _ "github.com/segmentio/golines"
       _ "github.com/shoenig/go-modtool"
       _ "golang.org/x/vuln/cmd/govulncheck"
   )
   ```

   Note: the copyright year should be 2026 (consistent with all other files in this module).

2. **Verify all imported packages are in `watcher/github/go.mod`** before writing:
   ```bash
   cd watcher/github && for pkg in \
     "golangci/golangci-lint/v2" \
     "google/addlicense" \
     "google/osv-scanner/v2" \
     "incu6us/goimports-reviser/v3" \
     "kisielk/errcheck" \
     "maxbrunsfeld/counterfeiter/v6" \
     "onsi/ginkgo/v2" \
     "securego/gosec/v2" \
     "segmentio/golines" \
     "shoenig/go-modtool" \
     "golang.org/x/vuln"; do
     grep -q "$pkg" go.mod && echo "OK: $pkg" || echo "MISSING: $pkg"
   done
   ```
   If any are missing, add them with `cd watcher/github && go get <package>@latest` before writing `tools.go`.

3. **Run `go mod tidy`** to promote the tool imports from `// indirect` to direct:
   ```bash
   cd watcher/github && go mod tidy
   ```

4. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- The `//go:build tools` tag ensures this file is excluded from normal compilation
- Do NOT run `go mod vendor` — this project has no vendor directory
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
cd watcher/github && ls tools.go
# Expected: file exists

cd watcher/github && head -6 tools.go
# Expected: copyright header + build tag

cd watcher/github && make precommit
</verification>
