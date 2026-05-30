// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("newTriggerHandler", func() {
	var trigger chan struct{}
	var handler http.HandlerFunc

	BeforeEach(func() {
		trigger = make(chan struct{}, 1)
		handler = newTriggerHandler(trigger)
	})

	serve := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodPost, "/trigger", nil))
		return rec
	}

	It("signals the trigger channel and acknowledges", func() {
		rec := serve()
		Expect(rec.Body.String()).To(Equal("trigger fired"))
		Expect(trigger).To(Receive())
	})

	It("coalesces when a trigger is already pending without blocking", func() {
		// First request fills the size-1 buffer.
		Expect(serve().Body.String()).To(Equal("trigger fired"))
		// Second request must not block and must not enqueue a second signal.
		Expect(serve().Body.String()).To(Equal("trigger fired"))
		Expect(len(trigger)).To(Equal(1))
	})
})
