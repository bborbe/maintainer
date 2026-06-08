// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	taskmocks "github.com/bborbe/agent/lib/command/task/mocks"
	libhttp "github.com/bborbe/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-pr/mocks"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/factory"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
)

var _ = Describe("CreateSinglePRTriggerHandler (legacy adapter, prompt 3 only)", func() {
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

	It("returns a non-nil handler when httpClient is nil (adapter ignores args)", func() {
		handler := factory.CreateSinglePRTriggerHandler(
			nil,
			validSender,
			validFilter,
			validTrust,
			"dev",
			80, 60, "pr-reviewer",
			pkg.NewMetrics(),
		)
		Expect(handler).NotTo(BeNil())
	})

	It("returns a non-nil handler when createSender is nil (adapter ignores args)", func() {
		handler := factory.CreateSinglePRTriggerHandler(
			validHTTPClient,
			nil,
			validFilter,
			validTrust,
			"dev",
			80, 60, "pr-reviewer",
			pkg.NewMetrics(),
		)
		Expect(handler).NotTo(BeNil())
	})

	It("returns a non-nil handler when taskCreationFilter is nil (adapter ignores args)", func() {
		handler := factory.CreateSinglePRTriggerHandler(
			validHTTPClient,
			validSender,
			nil,
			validTrust,
			"dev",
			80, 60, "pr-reviewer",
			pkg.NewMetrics(),
		)
		Expect(handler).NotTo(BeNil())
	})

	It("returns a non-nil handler when trustDecision is nil (adapter ignores args)", func() {
		handler := factory.CreateSinglePRTriggerHandler(
			validHTTPClient,
			validSender,
			validFilter,
			nil,
			"dev",
			80, 60, "pr-reviewer",
			pkg.NewMetrics(),
		)
		Expect(handler).NotTo(BeNil())
	})

	It("legacy CreateSinglePRTriggerHandler returns a 503 stub during the transition", func() {
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

		// Wrap in NewErrorHandler so the libhttp.WithErrorFunc error
		// surfaces as a proper HTTP response.
		wrapped := libhttp.NewErrorHandler(handler)
		req := httptest.NewRequest(
			"POST",
			"/trigger?url=https://github.com/bborbe/repo/pull/1",
			nil,
		)
		resp := httptest.NewRecorder()
		wrapped.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(resp.Body.String()).To(ContainSubstring("reconfiguring"))
	})
})

var _ = Describe("NewSinglePRTriggerHandler", func() {
	var sender *mocks.TriggerPRReviewCommandSender

	BeforeEach(func() {
		sender = new(mocks.TriggerPRReviewCommandSender)
	})

	It("panics when sender is nil", func() {
		Expect(func() {
			factory.NewSinglePRTriggerHandler(nil)
		}).To(PanicWith("sender is required"))
	})

	It("returns a non-nil handler when sender is non-nil", func() {
		handler := factory.NewSinglePRTriggerHandler(sender)
		Expect(handler).NotTo(BeNil())
	})

	It("handler responds to a request", func() {
		handler := factory.NewSinglePRTriggerHandler(sender)
		wrapped := libhttp.NewErrorHandler(handler)
		sender.SendCommandReturns(nil)
		req := httptest.NewRequest(
			"POST",
			"/trigger?url=https://github.com/bborbe/repo/pull/42",
			nil,
		)
		//nolint:contextcheck // test setup uses Background; safe in tests
		req = req.WithContext(context.Background())
		resp := httptest.NewRecorder()
		wrapped.ServeHTTP(resp, req)
		Expect(resp.Code).To(Equal(http.StatusAccepted))
		Expect(sender.SendCommandCallCount()).To(Equal(1))
	})
})
