// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/command"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// NewSinglePRTriggerHandler wires the thin CQRS handler that publishes a
// TriggerPRReviewCommand to Kafka for each valid /trigger request.
// All GitHub/filter/trust work lives in the in-pod command consumer
// (see pkg/command.NewTriggerPRReviewCommandExecutor). This is the
// signature main.go will use after prompt 4's rewiring.
func NewSinglePRTriggerHandler(
	sender command.TriggerPRReviewCommandSender,
) handler.SinglePRTriggerHandler {
	if sender == nil {
		panic("sender is required")
	}
	return handler.NewSinglePRTriggerHandler(sender)
}

// CreateSinglePRTriggerHandler is the LEGACY 9-arg adapter retained for
// ONE prompt (prompt 3) so main.go stays compilable while the per-prompt
// commits land sequentially. Prompt 4 swaps the main.go call site to
// NewSinglePRTriggerHandler and DELETES this adapter. Do NOT add new
// callers — every parameter is now ignored.
//
// DEPRECATED: remove in prompt 4. Tracked by spec 066.
func CreateSinglePRTriggerHandler(
	httpClient *http.Client,
	createSender task.CreateCommandSender, //nolint:revive // legacy adapter; removed in prompt 4
	taskCreationFilter filter.TaskCreationFilter, //nolint:revive // legacy adapter; removed in prompt 4
	trustDecision trust.Trust, //nolint:revive // legacy adapter; removed in prompt 4
	stage string, //nolint:revive // legacy adapter; removed in prompt 4
	maxSlugLen int, //nolint:revive // legacy adapter; removed in prompt 4
	maxTitleLen int, //nolint:revive // legacy adapter; removed in prompt 4
	taskSuffix string, //nolint:revive // legacy adapter; removed in prompt 4
	metrics pkg.Metrics, //nolint:revive // legacy adapter; removed in prompt 4
) handler.SinglePRTriggerHandler {
	// All args ignored: the legacy adapter returns a handler that always
	// returns HTTP 503 with "trigger endpoint reconfiguring — see spec 066".
	// This adapter exists only so main.go compiles between prompt 3 and
	// prompt 4 commits; prompt 4 deletes the call site and this function.
	_ = httpClient
	_ = createSender
	_ = taskCreationFilter
	_ = trustDecision
	_ = stage
	_ = maxSlugLen
	_ = maxTitleLen
	_ = taskSuffix
	_ = metrics
	return libhttp.WithErrorFunc(buildReconfiguringError)
}

// buildReconfiguringError returns the static "trigger endpoint
// reconfiguring" 503 error. It runs per request so it can use the
// request's ctx (never context.Background).
func buildReconfiguringError(
	ctx context.Context,
	_ http.ResponseWriter,
	_ *http.Request,
) error {
	return libhttp.WrapWithStatusCode(
		errors.Errorf(
			ctx,
			"trigger endpoint reconfiguring (spec 066 mid-rollout); retry after prompt 4 lands",
		),
		http.StatusServiceUnavailable,
	)
}
