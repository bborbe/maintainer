// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("runPollLoop", func() {
	It("continues the loop when poll returns an error", func() {
		app := &application{}
		var pollCount atomic.Int64

		pollFunc := func(ctx context.Context) error {
			count := pollCount.Add(1)
			if count == 1 {
				return errors.New("transient poll error")
			}
			<-ctx.Done()
			return ctx.Err()
		}

		ctx, cancel := context.WithCancel(context.Background())
		loopFunc := app.runPollLoop(pollFunc, 10*time.Millisecond)

		done := make(chan error, 1)
		go func() {
			done <- loopFunc(ctx)
		}()

		Eventually(
			func() int64 { return pollCount.Load() },
			"500ms",
			"10ms",
		).Should(BeNumerically(">=", 2))

		cancel()
		<-done
	})
})
