// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("parseMaxPRAge",
	func(raw string, expected libtime.Duration, expectError bool, errContains string) {
		ctx := context.Background()
		got, err := parseMaxPRAge(ctx, raw)
		if expectError {
			Expect(err).To(HaveOccurred())
			if errContains != "" {
				Expect(err.Error()).To(ContainSubstring(errContains))
			}
		} else {
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(expected))
		}
	},
	Entry("empty string disables age filter", "", libtime.Duration(0), false, ""),
	Entry("2160h parses correctly", "2160h", libtime.Duration(2160*time.Hour), false, ""),
	Entry("90d equals 2160h", "90d", libtime.Duration(2160*time.Hour), false, ""),
	Entry("negative duration is rejected", "-1h", libtime.Duration(0), true, "negative"),
	Entry("garbage input returns parse error", "not-a-duration", libtime.Duration(0), true, ""),
)

var _ = DescribeTable("parseBackfillDuration",
	func(raw string, expected libtime.Duration, expectError bool, errContains string) {
		ctx := context.Background()
		got, err := parseBackfillDuration(ctx, raw)
		if expectError {
			Expect(err).To(HaveOccurred())
			if errContains != "" {
				Expect(err.Error()).To(ContainSubstring(errContains))
			}
		} else {
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(expected))
		}
	},
	Entry("empty string disables backfill", "", libtime.Duration(0), false, ""),
	Entry("720h parses correctly", "720h", libtime.Duration(720*time.Hour), false, ""),
	Entry("30d equals 720h", "30d", libtime.Duration(720*time.Hour), false, ""),
	Entry("negative duration is rejected", "-1h", libtime.Duration(0), true, "negative"),
	Entry("garbage input returns parse error", "not-a-duration", libtime.Duration(0), true, ""),
)

var _ = DescribeTable("validateRepoScope",
	func(scope string, expectError bool) {
		ctx := context.Background()
		err := validateRepoScope(ctx, scope)
		if expectError {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
	},
	Entry("simple username", "bborbe", false),
	Entry("org with hyphen", "my-org", false),
	Entry("org with dot", "org.name", false),
	Entry("org with underscore", "org_name", false),
	Entry("mixed case and digits", "Org123", false),
	Entry("space injection", "user is:issue", true),
	Entry("semicolon injection", "user;drop", true),
	Entry("empty string", "", true),
	Entry("plus injection", "user+more", true),
)
