// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/errors"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// CreateSinglePRHandler wires a handler that fires a single-PR review by URL.
func CreateSinglePRHandler(
	ctx context.Context,
	auth AuthConfig,
	createSender task.CreateCommandSender,
	taskCreationFilter filter.TaskCreationFilter,
	trustDecision trust.Trust,
	stage string,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
) (handler.SinglePRTriggerHandler, error) {
	httpClient, err := CreateGitHubHTTPClient(ctx, auth)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "create GitHub HTTP client")
	}
	ghClient := pkg.NewGitHubClient(httpClient)

	if ghClient == nil {
		return nil, errors.Errorf(ctx, "ghClient is required")
	}
	if createSender == nil {
		return nil, errors.Errorf(ctx, "createSender is required")
	}
	if taskCreationFilter == nil {
		return nil, errors.Errorf(ctx, "taskCreationFilter is required")
	}
	if trustDecision == nil {
		return nil, errors.Errorf(ctx, "trustDecision is required")
	}
	return handler.NewSinglePRTriggerHandler(
		ghClient,
		createSender,
		taskCreationFilter,
		trustDecision,
		stage,
		maxSlugLen,
		maxTitleLen,
		taskSuffix,
	), nil
}
