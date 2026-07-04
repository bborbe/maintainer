// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package maintainerconfig_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/maintainerconfig"
)

var _ = Describe("Parse", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	DescribeTable("valid documents",
		func(content string, expected maintainerconfig.MaintainerConfig) {
			cfg, err := maintainerconfig.Parse(ctx, []byte(content))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).To(Equal(expected))
		},
		Entry("empty bytes -> zero-value, nil",
			"",
			maintainerconfig.MaintainerConfig{}),
		Entry("prReviewer.autoApprove: true -> PrReviewer.AutoApprove true",
			"prReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
			}),
		Entry("prReviewer absent -> AutoApprove false",
			"release:\n  autoRelease: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{AutoRelease: true},
			}),
		Entry("release.autoRelease: true still parses -> Release.AutoRelease true",
			"release:\n  autoRelease: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{AutoRelease: true},
			}),
		Entry("both namespaces populated -> both parsed",
			"release:\n  autoRelease: true\nprReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				Release:    maintainerconfig.ReleaseConfig{AutoRelease: true},
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
			}),
		Entry("release.changelogRewrite: true -> ChangelogRewrite true",
			"release:\n  changelogRewrite: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					AutoRelease:      false,
					ChangelogRewrite: true,
				},
			}),
		Entry("release.changelogRewrite: false -> ChangelogRewrite false",
			"release:\n  changelogRewrite: false\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					ChangelogRewrite: false,
				},
			}),
		Entry("release: present but no changelogRewrite field -> ChangelogRewrite false (default)",
			"release:\n  autoRelease: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					AutoRelease:      true,
					ChangelogRewrite: false,
				},
			}),
		Entry("no release: block -> ChangelogRewrite false",
			"prReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
				Release:    maintainerconfig.ReleaseConfig{ChangelogRewrite: false},
			}),
		Entry("empty bytes -> zero-value config, nil error",
			"",
			maintainerconfig.MaintainerConfig{}),
		Entry("both autoRelease and changelogRewrite populated -> both true",
			"release:\n  autoRelease: true\n  changelogRewrite: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					AutoRelease:      true,
					ChangelogRewrite: true,
				},
			}),
		Entry("release.allowMajorBump: true -> AllowMajorBump true",
			"release:\n  allowMajorBump: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					AutoRelease:      false,
					ChangelogRewrite: false,
					AllowMajorBump:   true,
				},
			}),
		Entry("release: present but no allowMajorBump field -> AllowMajorBump false (default)",
			"release:\n  autoRelease: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					AutoRelease:      true,
					ChangelogRewrite: false,
					AllowMajorBump:   false,
				},
			}),
		Entry("no release: block -> AllowMajorBump false",
			"prReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
				Release:    maintainerconfig.ReleaseConfig{AllowMajorBump: false},
			}),
	)

	It("malformed YAML -> wrapped error", func() {
		cfg, err := maintainerconfig.Parse(ctx, []byte("prReviewer:\n  autoApprove: [unclosed\n"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
	})

	It("Parse ignores unknown top-level field (lenient — fleet tolerance)", func() {
		cfg, err := maintainerconfig.Parse(
			ctx,
			[]byte("build-fix:\n  enabled: true\nprReviewer:\n  autoApprove: true\n"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.PrReviewer.AutoApprove).To(BeTrue())
	})

	It("ParseStrict rejects unknown top-level field", func() {
		_, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("build-fix:\n  enabled: true\nprReviewer:\n  autoApprove: true\n"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("ParseStrict rejects typo in nested release field", func() {
		// changelogRwrite is the canonical typo from PR-36 review.
		_, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("release:\n  changelogRwrite: true\n"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("ParseStrict rejects typo in top-level prReviewer key", func() {
		_, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("prRevierer:\n  autoApprove: true\n"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
	})

	It("release.changelogRewrite: non-bool string value -> wrapped error", func() {
		// yaml.v3 coerces the YAML-truthy strings "yes"/"on"/"true" to bool
		// and "no"/"off"/"false" to bool (per the YAML 1.2 spec), so a
		// truthy string is NOT a load-bearing invalid-value test. A
		// non-truthy string ("foo") IS rejected by the type system.
		cfg, err := maintainerconfig.Parse(
			ctx,
			[]byte("release:\n  changelogRewrite: \"foo\"\n"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
	})

	It("release.changelogRewrite: number value -> wrapped error", func() {
		cfg, err := maintainerconfig.Parse(
			ctx,
			[]byte("release:\n  changelogRewrite: 1\n"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
	})

	It("release.allowMajorBump: non-bool -> strict error", func() {
		// yaml.v3 coerces the YAML-truthy strings "yes"/"on"/"true" to bool
		// and "no"/"off"/"false" to bool (per the YAML 1.2 spec), so a
		// truthy string is NOT a load-bearing invalid-value test. A
		// non-truthy string ("foo") IS rejected by the type system.
		cfg, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("release:\n  allowMajorBump: \"foo\"\n"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
	})
})
