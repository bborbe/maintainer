// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lib_test

import (
	"github.com/bborbe/cqrs/cdb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/lib"
)

var _ = Describe("CDBSchema", func() {
	Describe("GithubPRReviewV1SchemaID", func() {
		It("has expected group/kind/version", func() {
			Expect(lib.GithubPRReviewV1SchemaID.Group).To(Equal(cdb.Group("maintainer")))
			Expect(lib.GithubPRReviewV1SchemaID.Kind).To(Equal(cdb.Kind("githubprreview")))
			Expect(lib.GithubPRReviewV1SchemaID.Version).To(Equal(cdb.Version("v1")))
		})

		It("serializes to canonical string", func() {
			Expect(lib.GithubPRReviewV1SchemaID.String()).To(Equal("maintainer-githubprreview-v1"))
		})
	})

	Describe("GithubReleaserV1SchemaID", func() {
		It("has expected group/kind/version", func() {
			Expect(lib.GithubReleaserV1SchemaID.Group).To(Equal(cdb.Group("maintainer")))
			Expect(lib.GithubReleaserV1SchemaID.Kind).To(Equal(cdb.Kind("githubreleaser")))
			Expect(lib.GithubReleaserV1SchemaID.Version).To(Equal(cdb.Version("v1")))
		})

		It("serializes to canonical string", func() {
			Expect(lib.GithubReleaserV1SchemaID.String()).To(Equal("maintainer-githubreleaser-v1"))
		})
	})

	Describe("GithubBuildV1SchemaID", func() {
		It("has expected group/kind/version", func() {
			Expect(lib.GithubBuildV1SchemaID.Group).To(Equal(cdb.Group("maintainer")))
			Expect(lib.GithubBuildV1SchemaID.Kind).To(Equal(cdb.Kind("githubbuild")))
			Expect(lib.GithubBuildV1SchemaID.Version).To(Equal(cdb.Version("v1")))
		})

		It("serializes to canonical string", func() {
			Expect(lib.GithubBuildV1SchemaID.String()).To(Equal("maintainer-githubbuild-v1"))
		})
	})

	Describe("CDBSchemaIDs registry", func() {
		It("contains GithubPRReviewV1SchemaID", func() {
			Expect(lib.CDBSchemaIDs.Contains(lib.GithubPRReviewV1SchemaID)).To(BeTrue())
		})

		It("contains GithubReleaserV1SchemaID", func() {
			Expect(lib.CDBSchemaIDs.Contains(lib.GithubReleaserV1SchemaID)).To(BeTrue())
		})

		It("contains GithubBuildV1SchemaID", func() {
			Expect(lib.CDBSchemaIDs.Contains(lib.GithubBuildV1SchemaID)).To(BeTrue())
		})

		It("has no duplicate entries", func() {
			seen := map[string]bool{}
			for _, id := range lib.CDBSchemaIDs {
				Expect(seen[id.String()]).To(BeFalse(), "duplicate schema id: %s", id.String())
				seen[id.String()] = true
			}
		})
	})
})
