// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// CreateSinglePRHandler wires a handler that fires a single-PR review by URL.
func CreateSinglePRHandler(
	httpClient *http.Client,
	createSender task.CreateCommandSender,
	taskCreationFilter filter.TaskCreationFilter,
	trustDecision trust.Trust,
	stage string,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
) handler.SinglePRTriggerHandler {
	ghClient := pkg.NewGitHubClient(httpClient)
	return handler.NewSinglePRTriggerHandler(
		ghClient,
		createSender,
		taskCreationFilter,
		trustDecision,
		stage,
		maxSlugLen,
		maxTitleLen,
		taskSuffix,
	)
}
