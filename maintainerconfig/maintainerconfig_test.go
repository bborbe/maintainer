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
		Entry("release.allowFork: true -> AllowFork true",
			"release:\n  allowFork: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					AllowFork: true,
				},
			}),
		Entry("release: present but no allowFork field -> AllowFork false (default)",
			"release:\n  autoRelease: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					AutoRelease: true,
					AllowFork:   false,
				},
			}),
		Entry("release.autoRelease: true and release.allowFork: true -> both true (fork opted in)",
			"release:\n  autoRelease: true\n  allowFork: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{
					AutoRelease: true,
					AllowFork:   true,
				},
			}),
		Entry("no release: block -> AllowFork false",
			"prReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
				Release:    maintainerconfig.ReleaseConfig{AllowFork: false},
			}),
		Entry("goUpdate.autoUpdate: true -> GoUpdate.AutoUpdate true",
			"goUpdate:\n  autoUpdate: true\n",
			maintainerconfig.MaintainerConfig{
				GoUpdate: maintainerconfig.GoUpdateConfig{AutoUpdate: true},
			}),
		Entry("goUpdate.autoUpdate: false -> GoUpdate.AutoUpdate false",
			"goUpdate:\n  autoUpdate: false\n",
			maintainerconfig.MaintainerConfig{
				GoUpdate: maintainerconfig.GoUpdateConfig{AutoUpdate: false},
			}),
		Entry("no goUpdate: block -> GoUpdate.AutoUpdate false",
			"prReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
				GoUpdate:   maintainerconfig.GoUpdateConfig{AutoUpdate: false},
			}),
		Entry("empty bytes -> GoUpdate.AutoUpdate false (file absent equivalent)",
			"",
			maintainerconfig.MaintainerConfig{
				GoUpdate: maintainerconfig.GoUpdateConfig{AutoUpdate: false},
			}),
		Entry("autoMerge.trivial: true -> Trivial true",
			"autoMerge:\n  trivial: true\n",
			maintainerconfig.MaintainerConfig{
				AutoMerge: maintainerconfig.AutoMergeConfig{Trivial: true},
			}),
		Entry("autoMerge absent -> Trivial false",
			"prReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
				AutoMerge:  maintainerconfig.AutoMergeConfig{Trivial: false},
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

	It("ParseStrict ignores an unknown top-level namespace and still reads known ones", func() {
		// Forward compatibility. A repo may adopt a namespace belonging to a
		// bot this binary predates; that must not fail the parse, because the
		// binary reading it may not be rebuilt for days. Rejecting this wedged
		// two repos' releases on 2026-08-16 when `goUpdate:` was introduced.
		cfg, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("build-fix:\n  enabled: true\nprReviewer:\n  autoApprove: true\n"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.PrReviewer.AutoApprove).To(BeTrue())
	})

	It("ParseStrict ignores the goUpdate namespace on a binary that predates it", func() {
		// The exact document that broke github-releaser-agent in prod.
		cfg, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte(
				"release:\n  autoRelease: true\n  changelogRewrite: false\nprReviewer:\n  autoApprove: true\ngoUpdate:\n  autoUpdate: true\n",
			),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Release.AutoRelease).To(BeTrue())
		Expect(cfg.PrReviewer.AutoApprove).To(BeTrue())
	})

	It("ParseStrict still rejects a typo inside a known namespace", func() {
		// The property ParseStrict exists for must survive the change above:
		// forward compatibility applies to unknown NAMESPACES, never to
		// unknown keys inside a namespace this binary owns.
		_, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("build-fix:\n  enabled: true\nrelease:\n  autoReleese: true\n"),
		)
		Expect(err).To(HaveOccurred())
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

	It("ParseStrict accepts release.allowFork now that the field exists", func() {
		cfg, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("release:\n  allowFork: true\n"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Release.AllowFork).To(BeTrue())
	})

	It("ParseStrict no longer rejects a typo'd top-level namespace", func() {
		// The accepted cost of forward compatibility, pinned so it is a
		// decision rather than a surprise: a misspelled NAMESPACE is
		// indistinguishable from one belonging to a newer bot, so it is
		// ignored instead of fatal, and the gate it meant to set stays false.
		// It is logged at WARNING so the typo is still discoverable.
		// Typos INSIDE a known namespace remain fatal — asserted above.
		cfg, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("prRevierer:\n  autoApprove: true\n"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.PrReviewer.AutoApprove).To(BeFalse())
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

	It("release.allowFork: non-bool -> wrapped error", func() {
		// Same YAML-truthy-coercion caveat as allowMajorBump above — "foo"
		// is the non-truthy string that the type system actually rejects.
		cfg, err := maintainerconfig.Parse(
			ctx,
			[]byte("release:\n  allowFork: \"foo\"\n"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
	})

	It("autoMerge.trivial: non-bool -> strict error", func() {
		// Same YAML-truthy-coercion caveat as allowMajorBump above — "foo"
		// is the non-truthy string that the type system actually rejects.
		// yaml.v3 does not emit field paths in its error, so assert only on
		// the wrapped "unmarshal .maintainer.yaml" prefix.
		cfg, err := maintainerconfig.ParseStrict(
			ctx,
			[]byte("autoMerge:\n  trivial: \"foo\"\n"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
	})
})
