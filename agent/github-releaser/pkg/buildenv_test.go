// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
)

var _ = Describe("BuildEnv", func() {
	DescribeTable(
		"assembles only non-empty values",
		func(ghToken, anthropicBaseURL, anthropicAuthToken, anthropicModel string, expected map[string]string) {
			Expect(pkg.BuildEnv(ghToken, anthropicBaseURL, anthropicAuthToken, anthropicModel)).
				To(Equal(expected))
		},
		Entry("all empty -> empty map",
			"", "", "", "",
			map[string]string{}),
		Entry("only GH_TOKEN",
			"gh", "", "", "",
			map[string]string{"GH_TOKEN": "gh"}),
		Entry("only ANTHROPIC_BASE_URL",
			"", "https://api.example", "", "",
			map[string]string{"ANTHROPIC_BASE_URL": "https://api.example"}),
		Entry("only ANTHROPIC_AUTH_TOKEN",
			"", "", "tok", "",
			map[string]string{"ANTHROPIC_AUTH_TOKEN": "tok"}),
		Entry("only ANTHROPIC_MODEL",
			"", "", "", "sonnet",
			map[string]string{"ANTHROPIC_MODEL": "sonnet"}),
		Entry("all four set",
			"gh", "https://api.example", "tok", "sonnet",
			map[string]string{
				"GH_TOKEN":             "gh",
				"ANTHROPIC_BASE_URL":   "https://api.example",
				"ANTHROPIC_AUTH_TOKEN": "tok",
				"ANTHROPIC_MODEL":      "sonnet",
			}),
	)

	It("omits empty values rather than setting them to empty string", func() {
		env := pkg.BuildEnv("gh", "", "", "sonnet")
		Expect(env).To(HaveKey("GH_TOKEN"))
		Expect(env).To(HaveKey("ANTHROPIC_MODEL"))
		Expect(env).NotTo(HaveKey("ANTHROPIC_BASE_URL"))
		Expect(env).NotTo(HaveKey("ANTHROPIC_AUTH_TOKEN"))
		Expect(env).To(Equal(map[string]string{
			"GH_TOKEN":        "gh",
			"ANTHROPIC_MODEL": "sonnet",
		}))
	})
})
