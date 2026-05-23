// Copyright (c) 2025 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prurl_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prurl "github.com/bborbe/maintainer/lib/prurl"
)

var _ = Describe("Parse", func() {
	var (
		rawURL string
		info   *prurl.PRInfo
		err    error
	)

	JustBeforeEach(func() {
		info, err = prurl.ParsePRURL(context.Background(), rawURL)
	})

	Context("GitHub URLs", func() {
		Context("with valid GitHub PR URL", func() {
			BeforeEach(func() {
				rawURL = "https://github.com/bborbe/teamvault-docker/pull/4"
			})

			It("returns no error", func() {
				Expect(err).To(BeNil())
			})

			It("extracts platform correctly", func() {
				Expect(info.Platform).To(Equal(prurl.PlatformGitHub))
			})

			It("extracts host correctly", func() {
				Expect(info.Host).To(Equal("github.com"))
			})

			It("extracts owner correctly", func() {
				Expect(info.Owner).To(Equal("bborbe"))
			})

			It("has empty project for GitHub", func() {
				Expect(info.Project).To(Equal(""))
			})

			It("extracts repo correctly", func() {
				Expect(info.Repo).To(Equal("teamvault-docker"))
			})

			It("extracts PR number correctly", func() {
				Expect(info.Number).To(Equal(4))
			})

			It("constructs repo URL correctly", func() {
				Expect(info.RepoURL).To(Equal("https://github.com/bborbe/teamvault-docker"))
			})
		})

		Context("with valid GitHub PR URL with trailing slash", func() {
			BeforeEach(func() {
				rawURL = "https://github.com/bborbe/teamvault-docker/pull/4/"
			})

			It("returns no error", func() {
				Expect(err).To(BeNil())
			})

			It("extracts platform correctly", func() {
				Expect(info.Platform).To(Equal(prurl.PlatformGitHub))
			})

			It("extracts owner correctly", func() {
				Expect(info.Owner).To(Equal("bborbe"))
			})

			It("extracts repo correctly", func() {
				Expect(info.Repo).To(Equal("teamvault-docker"))
			})

			It("extracts PR number correctly", func() {
				Expect(info.Number).To(Equal(4))
			})

			It("constructs repo URL correctly", func() {
				Expect(info.RepoURL).To(Equal("https://github.com/bborbe/teamvault-docker"))
			})
		})

		Context("with GitHub URL but not a PR (issue)", func() {
			BeforeEach(func() {
				rawURL = "https://github.com/bborbe/maintainer/issues/123"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with GitHub PR URL with non-numeric number", func() {
			BeforeEach(func() {
				rawURL = "https://github.com/bborbe/maintainer/pull/abc"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with malformed GitHub URL (missing parts)", func() {
			BeforeEach(func() {
				rawURL = "https://github.com/bborbe/pull/4"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with malformed GitHub URL (too many parts)", func() {
			BeforeEach(func() {
				rawURL = "https://github.com/bborbe/repo/extra/pull/4"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})
	})

	Context("Bitbucket Server URLs", func() {
		Context("with valid Bitbucket PR URL", func() {
			BeforeEach(func() {
				rawURL = "https://bitbucket.example.com/projects/PROJ/repos/my-repo/pull-requests/42"
			})

			It("returns no error", func() {
				Expect(err).To(BeNil())
			})

			It("extracts platform correctly", func() {
				Expect(info.Platform).To(Equal(prurl.PlatformBitbucket))
			})

			It("extracts host correctly", func() {
				Expect(info.Host).To(Equal("bitbucket.example.com"))
			})

			It("extracts project correctly", func() {
				Expect(info.Project).To(Equal("PROJ"))
			})

			It("extracts owner correctly (same as project)", func() {
				Expect(info.Owner).To(Equal("PROJ"))
			})

			It("extracts repo correctly", func() {
				Expect(info.Repo).To(Equal("my-repo"))
			})

			It("extracts PR number correctly", func() {
				Expect(info.Number).To(Equal(42))
			})

			It("constructs repo URL correctly", func() {
				Expect(
					info.RepoURL,
				).To(Equal("https://bitbucket.example.com/projects/PROJ/repos/my-repo"))
			})
		})

		Context("with valid Bitbucket PR URL with /overview suffix", func() {
			BeforeEach(func() {
				rawURL = "https://bitbucket.example.com/projects/PROJ/repos/my-repo/pull-requests/42/overview"
			})

			It("returns no error", func() {
				Expect(err).To(BeNil())
			})

			It("extracts platform correctly", func() {
				Expect(info.Platform).To(Equal(prurl.PlatformBitbucket))
			})

			It("extracts host correctly", func() {
				Expect(info.Host).To(Equal("bitbucket.example.com"))
			})

			It("extracts project correctly", func() {
				Expect(info.Project).To(Equal("PROJ"))
			})

			It("extracts repo correctly", func() {
				Expect(info.Repo).To(Equal("my-repo"))
			})

			It("extracts PR number correctly", func() {
				Expect(info.Number).To(Equal(42))
			})

			It("constructs repo URL correctly", func() {
				Expect(
					info.RepoURL,
				).To(Equal("https://bitbucket.example.com/projects/PROJ/repos/my-repo"))
			})
		})

		Context("with valid Bitbucket PR URL with trailing slash", func() {
			BeforeEach(func() {
				rawURL = "https://bitbucket.example.com/projects/PROJ/repos/my-repo/pull-requests/42/"
			})

			It("returns no error", func() {
				Expect(err).To(BeNil())
			})

			It("extracts platform correctly", func() {
				Expect(info.Platform).To(Equal(prurl.PlatformBitbucket))
			})

			It("extracts project correctly", func() {
				Expect(info.Project).To(Equal("PROJ"))
			})

			It("extracts repo correctly", func() {
				Expect(info.Repo).To(Equal("my-repo"))
			})

			It("extracts PR number correctly", func() {
				Expect(info.Number).To(Equal(42))
			})
		})

		Context("with Bitbucket PR URL with non-numeric number", func() {
			BeforeEach(func() {
				rawURL = "https://bitbucket.example.com/projects/PROJ/repos/my-repo/pull-requests/abc"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with malformed Bitbucket URL (missing parts)", func() {
			BeforeEach(func() {
				rawURL = "https://bitbucket.example.com/projects/PROJ/pull-requests/42"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with malformed Bitbucket URL (wrong keywords)", func() {
			BeforeEach(func() {
				rawURL = "https://bitbucket.example.com/projects/PROJ/repositories/my-repo/pull-requests/42"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})
	})

	Context("Generic URL validation", func() {
		Context("with empty string", func() {
			BeforeEach(func() {
				rawURL = ""
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with HTTP URL (not HTTPS)", func() {
			BeforeEach(func() {
				rawURL = "http://github.com/bborbe/maintainer/pull/4"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with URL without scheme", func() {
			BeforeEach(func() {
				rawURL = "github.com/bborbe/maintainer/pull/4"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with non-GitHub, non-Bitbucket URL", func() {
			BeforeEach(func() {
				rawURL = "https://gitlab.com/user/repo/merge_requests/1"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})

		Context("with malformed URL", func() {
			BeforeEach(func() {
				rawURL = "not a url at all"
			})

			It("returns error", func() {
				Expect(err).NotTo(BeNil())
			})

			It("error contains 'unsupported URL format'", func() {
				Expect(err.Error()).To(ContainSubstring("unsupported URL format"))
			})
		})
	})
})
