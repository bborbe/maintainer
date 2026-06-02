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

var _ = Describe("ChangelogQualityGuide", func() {
	It("returns non-empty string", func() {
		g := prompts.ChangelogQualityGuide()
		Expect(g).NotTo(BeEmpty())
	})

	It("contains the Conventional Prefixes heading", func() {
		g := prompts.ChangelogQualityGuide()
		Expect(g).To(ContainSubstring("Conventional Prefixes"))
	})

	It("contains feat: rule", func() {
		g := prompts.ChangelogQualityGuide()
		Expect(g).To(ContainSubstring("feat:"))
	})

	It("contains fix: rule", func() {
		g := prompts.ChangelogQualityGuide()
		Expect(g).To(ContainSubstring("fix:"))
	})

	It("contains the Anti-Patterns section", func() {
		g := prompts.ChangelogQualityGuide()
		Expect(g).To(ContainSubstring("Anti-Patterns"))
	})
})

var _ = Describe("ChangelogRewritePrompt", func() {
	It("returns non-empty string", func() {
		p := prompts.ChangelogRewritePrompt()
		Expect(p).NotTo(BeEmpty())
	})

	It("mentions cleaning the ## Unreleased section", func() {
		p := prompts.ChangelogRewritePrompt()
		Expect(p).To(ContainSubstring("## Unreleased"))
	})

	It("cites the Changelog Quality Guide", func() {
		p := prompts.ChangelogRewritePrompt()
		Expect(p).To(ContainSubstring("Changelog Quality Guide"))
	})

	It("contains faithfulness constraint", func() {
		p := prompts.ChangelogRewritePrompt()
		Expect(p).To(ContainSubstring("Faithfulness"))
	})

	It("contains JSON output format", func() {
		p := prompts.ChangelogRewritePrompt()
		Expect(p).To(ContainSubstring("rewrite_needed"))
		Expect(p).To(ContainSubstring("rewritten_unreleased"))
		Expect(p).To(ContainSubstring("reasoning"))
	})
})

var _ = DescribeTable("ParseRewriteVerdict",
	func(
		input string,
		wantRewriteNeeded bool,
		wantRewritten string,
		wantReasoning string,
		wantErrSubstr string,
	) {
		verdict, err := prompts.ParseRewriteVerdict(context.Background(), input)
		if wantErrSubstr == "" {
			Expect(err).NotTo(HaveOccurred())
			Expect(verdict.RewriteNeeded).To(Equal(wantRewriteNeeded))
			Expect(verdict.RewrittenUnreleased).To(Equal(wantRewritten))
			Expect(verdict.Reasoning).To(Equal(wantReasoning))
		} else {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse rewrite verdict"))
			Expect(err.Error()).To(ContainSubstring(wantErrSubstr))
			Expect(verdict).To(Equal(prompts.RewriteVerdict{}))
		}
	},
	Entry(
		"plain JSON parsed — rewrite_needed=true",
		`{"rewrite_needed":true,"rewritten_unreleased":"- feat: add foo\n","reasoning":"missing prefix"}`,
		true,
		"- feat: add foo\n",
		"missing prefix",
		"",
	),
	Entry(
		"fenced JSON block extracted from prose",
		"Here is my verdict:\n\n```json\n{\"rewrite_needed\":true,\"rewritten_unreleased\":\"- fix: x\\n\",\"reasoning\":\"git log style\"}\n```\n",
		true,
		"- fix: x\n",
		"git log style",
		"",
	),
	Entry(
		"rewrite_needed=false with empty rewritten_unreleased passes",
		`{"rewrite_needed":false,"rewritten_unreleased":"","reasoning":"all bullets already conform"}`,
		false,
		"",
		"all bullets already conform",
		"",
	),
	Entry("rewrite_needed=true with empty rewritten_unreleased errors",
		`{"rewrite_needed":true,"rewritten_unreleased":"","reasoning":"x"}`,
		true, "", "x", "rewritten_unreleased is empty"),
	Entry("rewrite_needed=false with non-empty rewritten_unreleased errors",
		`{"rewrite_needed":false,"rewritten_unreleased":"- feat: x\n","reasoning":"x"}`,
		false, "- feat: x\n", "x", "rewritten_unreleased is non-empty"),
	Entry("missing reasoning errors",
		`{"rewrite_needed":true,"rewritten_unreleased":"- feat: x\n","reasoning":""}`,
		true, "- feat: x\n", "", "missing reasoning"),
	Entry("empty input errors",
		``,
		false, "", "", "no JSON found"),
	Entry("malformed JSON errors with parse rewrite verdict substring",
		`{"rewrite_needed": true`,
		false, "", "", "parse rewrite verdict"),
	Entry("prose only no JSON errors",
		`Claude says: the answer is yes but I am not formatting JSON.`,
		false, "", "", "no JSON found"),
	Entry(
		"plain JSON with extra fields tolerated",
		`{"rewrite_needed":true,"rewritten_unreleased":"- chore: deps\n","reasoning":"bump dump","confidence":0.8}`,
		true,
		"- chore: deps\n",
		"bump dump",
		"",
	),
)
