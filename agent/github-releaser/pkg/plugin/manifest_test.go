// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package plugin_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/plugin"
)

var _ = Describe("DetectManifests", func() {
	DescribeTable("DetectManifests",
		func(workdir string, want []string) {
			got, err := plugin.DetectManifests(context.Background(), workdir)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(want))
		},
		Entry("neither exists → returns nil slice",
			func() string {
				dir, _ := os.MkdirTemp("", "detect-neither")
				return dir
			}(),
			nil),
		Entry("plugin.json only → returns [plugin.json]",
			func() string {
				dir, _ := os.MkdirTemp("", "detect-plugin-only")
				os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0755)
				os.WriteFile(
					filepath.Join(dir, ".claude-plugin", "plugin.json"),
					[]byte(`{"name":"test","version":"0.1.0"}`),
					0644,
				)
				return dir
			}(),
			[]string{".claude-plugin/plugin.json"}),
		Entry("marketplace.json only → returns [marketplace.json]",
			func() string {
				dir, _ := os.MkdirTemp("", "detect-marketplace-only")
				os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0755)
				os.WriteFile(
					filepath.Join(dir, ".claude-plugin", "marketplace.json"),
					[]byte(`{"metadata":{"version":"0.1.0"},"plugins":[]}`),
					0644,
				)
				return dir
			}(),
			[]string{".claude-plugin/marketplace.json"}),
		Entry("both exist → returns both",
			func() string {
				dir, _ := os.MkdirTemp("", "detect-both")
				os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0755)
				os.WriteFile(
					filepath.Join(dir, ".claude-plugin", "plugin.json"),
					[]byte(`{"name":"test","version":"0.1.0"}`),
					0644,
				)
				os.WriteFile(
					filepath.Join(dir, ".claude-plugin", "marketplace.json"),
					[]byte(`{"metadata":{"version":"0.1.0"},"plugins":[]}`),
					0644,
				)
				return dir
			}(),
			[]string{".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"}),
	)
})

var _ = Describe("BumpPluginJson", func() {
	DescribeTable("BumpPluginJson version-parameter boundary",
		func(version string, wantErr bool) {
			input := []byte(`{
  "name": "test",
  "version": "0.9.12"
}`)
			got, err := plugin.BumpPluginJson(context.Background(), input, version)
			if wantErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("version parameter"))
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(got).NotTo(BeNil())
			}
			// Prevent unused variable if wantErr path prints got
			_ = got
		},
		Entry("0.10.0 is valid", "0.10.0", false),
		Entry("1.2.8 is valid", "1.2.8", false),
		Entry("v0.10.0 rejected (leading v)", "v0.10.0", true),
		Entry("0.10.0-rc1 rejected (suffix)", "0.10.0-rc1", true),
		Entry("0.10 rejected (missing patch)", "0.10", true),
		Entry("latest rejected (not semver)", "latest", true),
		Entry("empty string rejected", "", true),
		Entry("00.10.0 accepted (leading zeros OK per shape)", "00.10.0", false),
	)

	DescribeTable("BumpPluginJson file content",
		func(input []byte, version string, expected []byte, wantErr bool, errMsgRegex string) {
			got, err := plugin.BumpPluginJson(context.Background(), input, version)
			if wantErr {
				Expect(err).To(HaveOccurred())
				if errMsgRegex != "" {
					Expect(err.Error()).To(MatchRegexp(errMsgRegex))
				}
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(got).To(Equal(expected))
			}
			// Silence unused var
			_ = got
		},
		Entry("happy path — bumps version line, preserves rest",
			[]byte(`{
  "name": "test",
  "version": "0.9.12",
  "description": "a plugin"
}`),
			"0.10.0",
			[]byte(`{
  "name": "test",
  "version": "0.10.0",
  "description": "a plugin"
}`),
			false, ""),
		Entry("version field not found",
			[]byte(`{"name": "test"}`),
			"0.10.0",
			nil,
			true, "version.*not found"),
		Entry("existing version non-semver → error",
			[]byte(`{"name": "test", "version": "latest"}`),
			"0.10.0",
			nil,
			true, "existing version"),
		Entry("malformed — version line with no value",
			[]byte(`{"name": "test", "version": }`),
			"0.10.0",
			nil,
			true, "(not found|not a semver)"),
		Entry("empty content → error",
			[]byte{},
			"0.10.0",
			nil,
			true, "not found"),
		Entry("trailing newline preserved when input ends with \\n",
			[]byte("{\n  \"version\": \"0.9.12\"\n}\n"),
			"0.10.0",
			[]byte("{\n  \"version\": \"0.10.0\"\n}\n"),
			false, ""),
		Entry("no trailing newline — output also has no trailing newline",
			[]byte("{\n  \"version\": \"0.9.12\"}"),
			"0.10.0",
			[]byte("{\n  \"version\": \"0.10.0\"}"),
			false, ""),
	)
})

var _ = Describe("BumpMarketplaceJson", func() {
	DescribeTable("BumpMarketplaceJson version-parameter boundary",
		func(version string, wantErr bool) {
			input := []byte(`{
  "metadata": {
    "version": "0.9.12"
  },
  "plugins": []
}`)
			got, err := plugin.BumpMarketplaceJson(context.Background(), input, version)
			if wantErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("version parameter"))
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(got).NotTo(BeNil())
			}
			_ = got
		},
		Entry("0.10.0 is valid", "0.10.0", false),
		Entry("1.2.8 is valid", "1.2.8", false),
		Entry("v0.10.0 rejected (leading v)", "v0.10.0", true),
		Entry("0.10.0-rc1 rejected (suffix)", "0.10.0-rc1", true),
		Entry("0.10 rejected (missing patch)", "0.10", true),
		Entry("latest rejected (not semver)", "latest", true),
		Entry("empty string rejected", "", true),
		Entry("00.10.0 accepted (leading zeros OK per shape)", "00.10.0", false),
	)

	DescribeTable("BumpMarketplaceJson file content",
		func(input []byte, version string, expected []byte, wantErr bool, errMsgRegex string) {
			got, err := plugin.BumpMarketplaceJson(context.Background(), input, version)
			if wantErr {
				Expect(err).To(HaveOccurred())
				if errMsgRegex != "" {
					Expect(err.Error()).To(MatchRegexp(errMsgRegex))
				}
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(got).To(Equal(expected))
			}
			_ = got
		},
		Entry("N=0 plugins — bumps metadata.version only",
			[]byte(`{
  "metadata": {
    "version": "0.9.12"
  },
  "plugins": []
}`),
			"0.10.0",
			[]byte(`{
  "metadata": {
    "version": "0.10.0"
  },
  "plugins": []
}`),
			false, ""),
		Entry("N=1 plugin — bumps metadata + plugin",
			[]byte(`{
  "metadata": {
    "version": "0.9.12"
  },
  "plugins": [
    {
      "name": "plugin-a",
      "version": "0.9.12"
    }
  ]
}`),
			"0.10.0",
			[]byte(`{
  "metadata": {
    "version": "0.10.0"
  },
  "plugins": [
    {
      "name": "plugin-a",
      "version": "0.10.0"
    }
  ]
}`),
			false, ""),
		Entry("N=3 plugins — bumps all 4 version fields",
			[]byte(`{
  "metadata": {
    "version": "0.9.12"
  },
  "plugins": [
    {"name": "a", "version": "0.9.12"},
    {"name": "b", "version": "0.9.12"},
    {"name": "c", "version": "0.9.12"}
  ]
}`),
			"0.10.0",
			[]byte(`{
  "metadata": {
    "version": "0.10.0"
  },
  "plugins": [
    {"name": "a", "version": "0.10.0"},
    {"name": "b", "version": "0.10.0"},
    {"name": "c", "version": "0.10.0"}
  ]
}`),
			false, ""),
		Entry("metadata.version not found",
			[]byte(`{"plugins": []}`),
			"0.10.0",
			nil,
			true, "version.*not found"),
		Entry("plugin version non-semver → error with index",
			[]byte(`{
  "metadata": {
    "version": "0.9.12"
  },
  "plugins": [
    {"name": "bad", "version": "latest"}
  ]
}`),
			"0.10.0",
			nil,
			true, "existing version"),
		Entry("empty content → error",
			[]byte{},
			"0.10.0",
			nil,
			true, "not found"),
		Entry("trailing newline preserved",
			[]byte("{\n  \"metadata\": {\n    \"version\": \"0.9.12\"\n  }\n}\n"),
			"0.10.0",
			[]byte("{\n  \"metadata\": {\n    \"version\": \"0.10.0\"\n  }\n}\n"),
			false, ""),
	)
})
