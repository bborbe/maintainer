// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	libhttp "github.com/bborbe/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-pr/mocks"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/factory"
)

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
