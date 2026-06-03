// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"net/http"
	"time"

	taskmocks "github.com/bborbe/agent/lib/command/task/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-pr/mocks"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/factory"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
)

var _ = Describe("CreateSinglePRTriggerHandler", func() {
	validHTTPClient := &http.Client{Timeout: 30 * time.Second}
	validSender := new(taskmocks.TaskCreateCommandSender)
	validFilter := filter.TaskCreationFilters{}
	validTrust := new(mocks.Trust)

	It("returns non-nil handler when all params are non-nil", func() {
		handler := factory.CreateSinglePRTriggerHandler(
			validHTTPClient,
			validSender,
			validFilter,
			validTrust,
			"dev",
			80, 60, "pr-reviewer",
			pkg.NewMetrics(),
		)
		Expect(handler).NotTo(BeNil())
	})

	It("panics when httpClient is nil", func() {
		Expect(func() {
			factory.CreateSinglePRTriggerHandler(
				nil,
				validSender,
				validFilter,
				validTrust,
				"dev",
				80, 60, "pr-reviewer",
				pkg.NewMetrics(),
			)
		}).To(PanicWith("httpClient is required"))
	})

	It("panics when createSender is nil", func() {
		Expect(func() {
			factory.CreateSinglePRTriggerHandler(
				validHTTPClient,
				nil,
				validFilter,
				validTrust,
				"dev",
				80, 60, "pr-reviewer",
				pkg.NewMetrics(),
			)
		}).To(PanicWith("createSender is required"))
	})

	It("panics when taskCreationFilter is nil", func() {
		Expect(func() {
			factory.CreateSinglePRTriggerHandler(
				validHTTPClient,
				validSender,
				nil,
				validTrust,
				"dev",
				80, 60, "pr-reviewer",
				pkg.NewMetrics(),
			)
		}).To(PanicWith("taskCreationFilter is required"))
	})

	It("panics when trustDecision is nil", func() {
		Expect(func() {
			factory.CreateSinglePRTriggerHandler(
				validHTTPClient,
				validSender,
				validFilter,
				nil,
				"dev",
				80, 60, "pr-reviewer",
				pkg.NewMetrics(),
			)
		}).To(PanicWith("trustDecision is required"))
	})
})
