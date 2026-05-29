// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompts_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/prompts"
)

var _ = Describe("BumpClassificationPrompt", func() {
	It("returns non-empty string", func() {
		p := prompts.BumpClassificationPrompt()
		Expect(p).NotTo(BeEmpty())
	})

	It("contains patch | minor | major", func() {
		p := prompts.BumpClassificationPrompt()
		Expect(p).To(ContainSubstring("patch | minor | major"))
	})

	It("contains BREAKING CHANGE", func() {
		p := prompts.BumpClassificationPrompt()
		Expect(p).To(ContainSubstring("BREAKING CHANGE"))
	})

	It("contains feat:", func() {
		p := prompts.BumpClassificationPrompt()
		Expect(p).To(ContainSubstring("feat:"))
	})

	It("contains bump field", func() {
		p := prompts.BumpClassificationPrompt()
		Expect(p).To(ContainSubstring(`"bump":`))
	})

	It("contains major → minor → patch priority order", func() {
		p := prompts.BumpClassificationPrompt()
		Expect(p).To(ContainSubstring("major → minor → patch"))
	})
})

var _ = DescribeTable("ParseBumpVerdict",
	func(input, wantBump, wantReasoning, wantErrSubstr string) {
		verdict, err := prompts.ParseBumpVerdict(context.Background(), input)
		if wantErrSubstr == "" {
			Expect(err).NotTo(HaveOccurred())
			Expect(verdict.Bump).To(Equal(wantBump))
			Expect(verdict.Reasoning).To(Equal(wantReasoning))
		} else {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse bump verdict"))
			Expect(err.Error()).To(ContainSubstring(wantErrSubstr))
			Expect(verdict).To(Equal(prompts.BumpVerdict{}))
		}
	},
	Entry("plain JSON parsed",
		`{"bump":"patch","reasoning":"bug fix only"}`,
		"patch", "bug fix only", ""),
	Entry(
		"fenced JSON block extracted from prose",
		"Here is my verdict:\n\n```json\n{\"bump\":\"minor\",\"reasoning\":\"new feat: foo\"}\n```\n",
		"minor",
		"new feat: foo",
		"",
	),
	Entry("plain JSON with extra fields tolerated",
		`{"bump":"major","reasoning":"removed API","confidence":0.9}`,
		"major", "removed API", ""),
	Entry("empty input errors",
		``,
		"", "", "no JSON found"),
	Entry("invalid bump value errors",
		`{"bump":"giant","reasoning":"x"}`,
		"", "", "invalid bump value"),
	Entry("missing reasoning errors",
		`{"bump":"patch","reasoning":""}`,
		"", "", "missing reasoning"),
	Entry("malformed JSON errors",
		`{"bump": "patch"`,
		"", "", "no JSON found"),
	Entry("prose only no JSON errors",
		`Claude says: the answer is patch but I am not formatting JSON.`,
		"", "", "no JSON found"),
)
