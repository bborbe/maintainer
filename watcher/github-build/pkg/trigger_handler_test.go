// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-build/pkg"
)

var _ = Describe("NewTriggerHandler", func() {
	var trigger chan struct{}
	var handler http.Handler

	BeforeEach(func() {
		trigger = make(chan struct{}, 1)
		handler = pkg.NewTriggerHandler(trigger)
	})

	serve := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/trigger", nil))
		return rec
	}

	It("signals the trigger channel and acknowledges", func() {
		rec := serve()
		Expect(rec.Body.String()).To(ContainSubstring("trigger fired"))
		Expect(trigger).To(Receive())
	})

	It("coalesces when a trigger is already pending without blocking", func() {
		// First request fills the size-1 buffer.
		Expect(serve().Body.String()).To(ContainSubstring("trigger fired"))
		// Second request must not block and must not enqueue a second signal.
		Expect(serve().Body.String()).To(ContainSubstring("trigger fired"))
		// Exactly one pending signal: drain it, then assert none remain.
		Expect(trigger).To(Receive())
		Consistently(trigger, "50ms").ShouldNot(Receive())
	})
})
