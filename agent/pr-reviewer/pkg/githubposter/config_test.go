// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubposter_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubposter"
)

var _ = Describe("ReadAutoApproveConfig", func() {
	var ctx context.Context
	var tmpDir string

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "pr-reviewer-config-test-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, tmpDir)
	})

	DescribeTable(
		"file variants",
		func(content string, writeFile bool, expected githubposter.AutoApproveConfig, expectErr bool, errContains string) {
			if writeFile {
				err := os.WriteFile(
					filepath.Join(tmpDir, ".pr-reviewer.yaml"),
					[]byte(content),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			}
			cfg, err := githubposter.ReadAutoApproveConfig(ctx, tmpDir)
			if expectErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(errContains))
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).To(Equal(expected))
			}
		},
		Entry("file missing → AutoApprove false, no error",
			"", false,
			githubposter.AutoApproveConfig{AutoApprove: false}, false, ""),
		Entry("autoApprove: true → AutoApprove true",
			"autoApprove: true\n", true,
			githubposter.AutoApproveConfig{AutoApprove: true}, false, ""),
		Entry("autoApprove: false → AutoApprove false",
			"autoApprove: false\n", true,
			githubposter.AutoApproveConfig{AutoApprove: false}, false, ""),
		Entry("field absent → AutoApprove false",
			"someOtherField: hello\n", true,
			githubposter.AutoApproveConfig{AutoApprove: false}, false, ""),
		Entry("malformed YAML → error with parse prefix",
			"autoApprove: [unclosed", true,
			githubposter.AutoApproveConfig{}, true, "parse .pr-reviewer.yaml"),
	)
})
