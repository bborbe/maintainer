// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/command"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
)

// NewSinglePRTriggerHandler wires the thin CQRS handler that publishes a
// TriggerPRReviewCommand to Kafka for each valid /trigger request.
// All GitHub/filter/trust work lives in the in-pod command consumer
// (see pkg/command.NewTriggerPRReviewCommandExecutor). This is the
// signature main.go uses after prompt 4's rewiring.
func NewSinglePRTriggerHandler(
	sender command.TriggerPRReviewCommandSender,
) handler.SinglePRTriggerHandler {
	if sender == nil {
		panic("sender is required")
	}
	return handler.NewSinglePRTriggerHandler(sender)
}
