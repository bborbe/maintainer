---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T21:00:21Z"
---

<summary>
- `pkg/factory/factory_suite_test.go`, `pkg/filter/suite_test.go`, and `pkg/handler/suite_test.go` all lack `time.Local = time.UTC`, `format.TruncatedDiff = false`, and Ginkgo suite configuration with timeout
- These minimal suites risk timezone-dependent test failures, truncated diffs hiding assertion details, and indefinite test hangs
</summary>

<objective>
Update three Ginkgo suite files to include the standard setup pattern: UTC timezone, untruncated diffs, and suite configuration with timeout.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega suite setup.

Files to read before making changes:
- `watcher/github-pr/pkg/suite_test.go` — read as reference for the correct complete suite pattern
- `watcher/github-pr/pkg/factory/factory_suite_test.go`
- `watcher/github-pr/pkg/filter/suite_test.go`
- `watcher/github-pr/pkg/handler/suite_test.go`
</context>

<requirements>
1. **Update `watcher/github-pr/pkg/factory/factory_suite_test.go`:**

   Replace the minimal suite with:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package factory_test

   import (
       "testing"
       "time"

       "github.com/onsi/ginkgo/v2"
       "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestFactory(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       gomega.RegisterFailHandler(ginkgo.Fail)
       suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       ginkgo.RunSpecs(t, "Factory Suite", suiteConfig, reporterConfig)
   }
   ```

2. **Update `watcher/github-pr/pkg/filter/suite_test.go`:**

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter_test

   import (
       "testing"
       "time"

       "github.com/onsi/ginkgo/v2"
       "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestFilter(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       gomega.RegisterFailHandler(ginkgo.Fail)
       suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       ginkgo.RunSpecs(t, "Filter Suite", suiteConfig, reporterConfig)
   }
   ```

3. **Update `watcher/github-pr/pkg/handler/suite_test.go`:**

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package handler_test

   import (
       "testing"
       "time"

       "github.com/onsi/ginkgo/v2"
       "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestHandler(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       gomega.RegisterFailHandler(ginkgo.Fail)
       suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       ginkgo.RunSpecs(t, "Handler Suite", suiteConfig, reporterConfig)
   }
   ```

4. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```
   All suites should run without issues.

5. **Run `make precommit`:**
   ```bash
   cd watcher/github-pr && make precommit
   ```
</requirements>

<constraints>
- Only change the three suite test files listed above
- Do NOT commit — dark-factory handles git
- Keep `package <name>_test` (external test package convention)
- Keep the BSD copyright header
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm all three suites have timeout:
grep -l "suiteConfig.Timeout" watcher/github-pr/pkg/factory/factory_suite_test.go \
  watcher/github-pr/pkg/filter/suite_test.go \
  watcher/github-pr/pkg/handler/suite_test.go
</verification>
