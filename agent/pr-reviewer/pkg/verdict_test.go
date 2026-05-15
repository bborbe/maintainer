// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
)

var _ = Describe("Parse", func() {
	var (
		reviewText string
		result     pkg.Result
	)

	JustBeforeEach(func() {
		result = pkg.ParseVerdict(reviewText)
	})

	Context("empty review text", func() {
		BeforeEach(func() {
			reviewText = ""
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'empty review text'", func() {
			Expect(result.Reason).To(Equal("empty review text"))
		})
	})

	Context("review with no recognizable sections", func() {
		BeforeEach(func() {
			reviewText = "This is just some random text without any sections."
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'unparseable review format'", func() {
			Expect(result.Reason).To(Equal("unparseable review format"))
		})
	})

	Context("review with Must Fix section containing items", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix (Critical)

- Security issue in authentication
- SQL injection vulnerability`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'must-fix items found'", func() {
			Expect(result.Reason).To(Equal("must-fix items found"))
		})
	})

	Context("review with Must Fix section saying *None*", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix (Critical)

*None*

### Should Fix (Important)

- Add error handling`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("review with Must Fix section saying None identified", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix (Critical)

None identified.

### Should Fix (Important)

- Improve error messages`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("review with Must Fix section saying none (lowercase)", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix (Critical)

none

### Nice to Have (Optional)

- Add docstrings`
		})

		It("returns VerdictApprove", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
		})

		It("returns reason 'no must-fix items'", func() {
			Expect(result.Reason).To(Equal("no must-fix items"))
		})
	})

	Context("review with Must Fix section that is empty", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix (Critical)


### Should Fix (Important)

- Add tests`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("review with only Should Fix and Nice to Have sections", func() {
		BeforeEach(func() {
			reviewText = `### Should Fix (Important)

- Add error handling
- Improve logging

### Nice to Have (Optional)

- Add docstrings
- Refactor for clarity`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("review with only Should Fix section", func() {
		BeforeEach(func() {
			reviewText = `### Should Fix (Important)

- Add error handling`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("review with only Nice to Have section", func() {
		BeforeEach(func() {
			reviewText = `### Nice to Have (Optional)

- Add comments`
		})

		It("returns VerdictApprove", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
		})

		It("returns reason 'no must-fix section'", func() {
			Expect(result.Reason).To(Equal("no must-fix section"))
		})
	})

	Context("review with h2 Must Fix instead of h3", func() {
		BeforeEach(func() {
			reviewText = `## Must Fix (Critical)

- Critical security flaw`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'must-fix items found'", func() {
			Expect(result.Reason).To(Equal("must-fix items found"))
		})
	})

	Context("review with h2 Must Fix saying None", func() {
		BeforeEach(func() {
			reviewText = `## Must Fix (Critical)

None

## Should Fix (Important)

- Add tests`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("review with case variations in Must Fix header", func() {
		BeforeEach(func() {
			reviewText = `### MUST FIX (Critical)

- Issue found`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})
	})

	Context("review with Must Fix containing 'No issues found' text", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix (Critical)

No issues found.

### Should Fix (Important)

- Improve code structure`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("review with Must Fix at end of document", func() {
		BeforeEach(func() {
			reviewText = `### Should Fix (Important)

- Add tests

### Must Fix (Critical)

- Security vulnerability`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'must-fix items found'", func() {
			Expect(result.Reason).To(Equal("must-fix items found"))
		})
	})

	Context("review with Must Fix None separated by horizontal rules", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix

None.

---

### Should Fix

None.

---

### Nice to Have

- Minor style improvement`
		})

		It("returns VerdictApprove", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
		})

		It("returns reason 'no must-fix items'", func() {
			Expect(result.Reason).To(Equal("no must-fix items"))
		})
	})

	Context("complex review with multiple sections", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

## Summary

This PR adds new features.

### Must Fix (Critical)

*None*

### Should Fix (Important)

- Add error handling in main.go:45
- Missing input validation

### Nice to Have (Optional)

- Add docstrings
- Refactor for clarity

## Conclusion

Overall good work!`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("JSON verdict on bare line", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

Some review content here.

{"verdict": "approve", "reason": "all checks passed"}`
		})

		It("returns VerdictApprove from JSON", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
		})

		It("returns reason from JSON", func() {
			Expect(result.Reason).To(Equal("all checks passed"))
		})
	})

	Context("JSON verdict inside code fence", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

Some review content here.

` + "```json" + `
{"verdict": "request-changes", "reason": "critical security issues"}
` + "```" + ``
		})

		It("returns VerdictRequestChanges from JSON", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason from JSON", func() {
			Expect(result.Reason).To(Equal("critical security issues"))
		})
	})

	Context("JSON verdict with extra whitespace", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

Some content.

   {"verdict": "comment", "reason": "informational only"}   `
		})

		It("returns VerdictRequestChanges (JSON comment rejected, heuristic fallback)", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason from heuristic, not from JSON", func() {
			Expect(result.Reason).To(Equal("unparseable review format"))
		})
	})

	Context("invalid JSON falls back to heuristic", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix

- Security issue

{"verdict": "approve", "reason": invalid json}`
		})

		It("returns VerdictRequestChanges from heuristic", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason from heuristic", func() {
			Expect(result.Reason).To(Equal("must-fix items found"))
		})
	})

	Context("missing verdict field falls back to heuristic", func() {
		BeforeEach(func() {
			reviewText = `### Should Fix

- Add tests

{"reason": "just a reason"}`
		})

		It("returns VerdictRequestChanges from heuristic", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason from heuristic", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("unknown verdict value falls back to heuristic", func() {
		BeforeEach(func() {
			reviewText = `### Nice to Have

- Refactor code

{"verdict": "unknown-verdict", "reason": "some reason"}`
		})

		It("returns VerdictApprove from heuristic", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
		})

		It("returns reason from heuristic", func() {
			Expect(result.Reason).To(Equal("no must-fix section"))
		})
	})

	Context("no JSON at all uses heuristic", func() {
		BeforeEach(func() {
			reviewText = `### Must Fix

None

### Should Fix

- Add error handling`
		})

		It("returns VerdictRequestChanges from heuristic", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason from heuristic", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("JSON verdict in middle of review is ignored", func() {
		BeforeEach(func() {
			// Build a review with >50 lines so JSON in middle is outside the search window
			var lines []string
			lines = append(lines, "# Code Review")
			lines = append(lines, "")
			lines = append(lines, `{"verdict": "approve", "reason": "this should be ignored"}`)
			lines = append(lines, "")

			// Add 60 more lines to push JSON out of the last-50 window
			for i := 0; i < 60; i++ {
				lines = append(lines, "Some review content line.")
			}

			lines = append(lines, "### Must Fix")
			lines = append(lines, "- Critical issue")

			reviewText = strings.Join(lines, "\n")
		})

		It("ignores JSON in middle and uses heuristic", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason from heuristic", func() {
			Expect(result.Reason).To(Equal("must-fix items found"))
		})
	})

	Context("JSON verdict with 'comment' value is rejected — falls to heuristic", func() {
		BeforeEach(func() {
			// Should Fix has content → after JSON rejection, heuristic returns request-changes
			reviewText = `### Should Fix

- Add error handling

{"verdict": "comment", "reason": "informational only"}`
		})

		It("returns VerdictRequestChanges (not comment)", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason from heuristic, not from JSON", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("Should Fix only — non-empty content triggers request-changes", func() {
		BeforeEach(func() {
			reviewText = `### Should Fix (Important)

- Improve error handling in pkg/server.go:42`
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'should-fix items found'", func() {
			Expect(result.Reason).To(Equal("should-fix items found"))
		})
	})

	Context("Should Fix present but empty — approve", func() {
		BeforeEach(func() {
			reviewText = `### Should Fix (Important)

None.

### Nice to Have

- Add docstrings`
		})

		It("returns VerdictApprove", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
		})
	})

	Context("empty input returns request-changes (fail-closed)", func() {
		BeforeEach(func() {
			reviewText = ""
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'empty review text'", func() {
			Expect(result.Reason).To(Equal("empty review text"))
		})
	})

	Context("unparseable input (no sections) returns request-changes (fail-closed)", func() {
		BeforeEach(func() {
			reviewText = "Just some random prose without any review sections."
		})

		It("returns VerdictRequestChanges", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})

		It("returns reason 'unparseable review format'", func() {
			Expect(result.Reason).To(Equal("unparseable review format"))
		})
	})

	Context("multi-line fenced JSON with new spec-025 schema (no reason field) — approve", func() {
		BeforeEach(func() {
			reviewText = "# Code Review\n\nThe PR adds an HTML comment to README.md.\n\n" +
				"```json\n" +
				"{\n" +
				"  \"verdict\": \"approve\",\n" +
				"  \"summary\": \"Trivial doc-only change, no findings.\",\n" +
				"  \"comments\": [],\n" +
				"  \"concerns_addressed\": [\n" +
				"    \"correctness: no logic changes\"\n" +
				"  ]\n" +
				"}\n" +
				"```"
		})

		It("returns VerdictApprove from the multi-line block", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictApprove))
		})
	})

	Context("multi-line fenced JSON with new spec-025 schema — request-changes", func() {
		BeforeEach(func() {
			reviewText = "# Code Review\n\nThe PR has critical issues.\n\n" +
				"```json\n" +
				"{\n" +
				"  \"verdict\": \"request-changes\",\n" +
				"  \"summary\": \"Critical issues found.\",\n" +
				"  \"comments\": []\n" +
				"}\n" +
				"```"
		})

		It("returns VerdictRequestChanges from the multi-line block", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
		})
	})

	Context("malformed JSON in fenced block falls back to heuristic", func() {
		BeforeEach(func() {
			reviewText = "```json\n{verdict: invalid no quotes\n```\n## Must Fix\n- problem"
		})

		It("returns VerdictRequestChanges from heuristic must-fix items", func() {
			Expect(result.Verdict).To(Equal(pkg.VerdictRequestChanges))
			Expect(result.Reason).To(Equal("must-fix items found"))
		})
	})
})

var _ = Describe("StripJSONVerdict", func() {
	var (
		reviewText string
		stripped   string
	)

	JustBeforeEach(func() {
		stripped = pkg.StripJSONVerdict(reviewText)
	})

	Context("removes JSON verdict on bare line", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

Some review content here.

{"verdict": "approve", "reason": "all checks passed"}`
		})

		It("removes the JSON line", func() {
			Expect(stripped).NotTo(ContainSubstring(`"verdict"`))
			Expect(stripped).To(ContainSubstring("Some review content here."))
		})
	})

	Context("removes JSON verdict inside code fence", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

Some review content.

` + "```json" + `
{"verdict": "approve", "reason": "looks good"}
` + "```" + `

End of review.`
		})

		It("removes the JSON and code fence", func() {
			Expect(stripped).NotTo(ContainSubstring(`"verdict"`))
			Expect(stripped).NotTo(ContainSubstring("```json"))
			Expect(stripped).To(ContainSubstring("Some review content."))
			Expect(stripped).To(ContainSubstring("End of review."))
		})
	})

	Context("preserves text when no JSON found", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

Some review content without JSON verdict.`
		})

		It("returns unchanged text", func() {
			Expect(stripped).To(Equal(reviewText))
		})
	})

	Context("handles multiple trailing newlines", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

Some content.

{"verdict": "approve", "reason": "good"}


`
		})

		It("collapses trailing blank lines", func() {
			Expect(stripped).NotTo(ContainSubstring(`"verdict"`))
			Expect(stripped).NotTo(HaveSuffix("\n\n\n"))
			Expect(stripped).To(ContainSubstring("Some content."))
		})
	})

	Context("preserves other JSON in review", func() {
		BeforeEach(func() {
			reviewText = `# Code Review

Here's an example:
` + "```json" + `
{"config": "value"}
` + "```" + `

{"verdict": "approve", "reason": "ok"}`
		})

		It("only removes verdict JSON", func() {
			Expect(stripped).NotTo(ContainSubstring(`"verdict"`))
			Expect(stripped).To(ContainSubstring(`"config"`))
		})
	})
})
