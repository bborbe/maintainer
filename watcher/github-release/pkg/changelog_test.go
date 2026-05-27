// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-release/pkg"
)

var _ = Describe("pkg.ParseChangelog", func() {
	It("ParseChangelog handles Unreleased at bottom with mixed v-prefix", func() {
		content := `# Changelog

## 1.2.6

- old

## v1.2.5

- older

## Unreleased

- new entry
`
		summary := pkg.ParseChangelog([]byte(content))
		Expect(summary.UnreleasedBullets).To(Equal(1))
		Expect(summary.UnreleasedIsFirst).To(Equal(false))
		Expect(summary.LatestVersion).To(Equal("1.2.6"))
	})

	It("canonical ordering: Unreleased first with two bullets", func() {
		content := `# Changelog

## Unreleased

- entry one
- entry two

## v1.2.3

- old
`
		summary := pkg.ParseChangelog([]byte(content))
		Expect(summary.UnreleasedBullets).To(Equal(2))
		Expect(summary.UnreleasedIsFirst).To(Equal(true))
		Expect(summary.LatestVersion).To(Equal("v1.2.3"))
	})

	It("empty Unreleased header", func() {
		content := `## Unreleased

## v1.0.0

- x
`
		summary := pkg.ParseChangelog([]byte(content))
		Expect(summary.UnreleasedBullets).To(Equal(0))
		Expect(summary.UnreleasedIsFirst).To(Equal(true))
		Expect(summary.LatestVersion).To(Equal("v1.0.0"))
	})

	It("missing Unreleased section", func() {
		content := `## v1.0.0

- x
`
		summary := pkg.ParseChangelog([]byte(content))
		Expect(summary.UnreleasedBullets).To(Equal(0))
		Expect(summary.UnreleasedIsFirst).To(Equal(false))
		Expect(summary.LatestVersion).To(Equal("v1.0.0"))
	})

	It("no versions, no unreleased", func() {
		content := `# Changelog

Intro paragraph
`
		summary := pkg.ParseChangelog([]byte(content))
		Expect(summary.UnreleasedBullets).To(Equal(0))
		Expect(summary.UnreleasedIsFirst).To(Equal(false))
		Expect(summary.LatestVersion).To(Equal(""))
	})

	It("nil input returns zero values", func() {
		summary := pkg.ParseChangelog(nil)
		Expect(summary.UnreleasedBullets).To(Equal(0))
		Expect(summary.UnreleasedIsFirst).To(Equal(false))
		Expect(summary.LatestVersion).To(Equal(""))
	})

	It("empty bytes returns zero values", func() {
		summary := pkg.ParseChangelog([]byte(""))
		Expect(summary.UnreleasedBullets).To(Equal(0))
		Expect(summary.UnreleasedIsFirst).To(Equal(false))
		Expect(summary.LatestVersion).To(Equal(""))
	})

	It("H3 under Unreleased does not terminate counting", func() {
		content := `## Unreleased

### Added

- a
- b

## v1.0.0
`
		summary := pkg.ParseChangelog([]byte(content))
		Expect(summary.UnreleasedBullets).To(Equal(2))
		Expect(summary.UnreleasedIsFirst).To(Equal(true))
		Expect(summary.LatestVersion).To(Equal("v1.0.0"))
	})
})
