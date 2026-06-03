// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"net/http"

	libhttp "github.com/bborbe/http"
	"github.com/golang/glog"
)

// NewTriggerHandler creates an HTTP handler that requests one poll cycle by
// signalling the trigger channel. The send is non-blocking: while a poll runs
// the size-1 buffer is full, so additional triggers coalesce into the single
// pending signal rather than queueing or blocking the request. The poll loop is
// the sole executor, so polls never overlap.
func NewTriggerHandler(trigger chan<- struct{}) http.Handler {
	return libhttp.NewErrorHandler(
		libhttp.WithErrorFunc(
			func(_ context.Context, resp http.ResponseWriter, _ *http.Request) error {
				select {
				case trigger <- struct{}{}:
					glog.V(2).Infof("trigger fired via HTTP")
				default:
					glog.V(2).Infof("trigger already pending, skipped")
				}
				_, _ = libhttp.WriteAndGlog(resp, "trigger fired")
				return nil
			},
		),
	)
}
