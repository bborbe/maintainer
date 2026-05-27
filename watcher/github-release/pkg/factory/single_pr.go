// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"
	libhttp "github.com/bborbe/http"

	"github.com/bborbe/maintainer/watcher/github-release/pkg"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/handler"
)

// CreateSinglePRTriggerHandler wires a handler that fires a single-PR review by URL.
func CreateSinglePRTriggerHandler(
	httpClient *http.Client,
	createSender task.CreateCommandSender,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
	metrics pkg.Metrics,
) handler.SinglePRTriggerHandler {
	if httpClient == nil {
		panic("httpClient is required")
	}
	if createSender == nil {
		panic("createSender is required")
	}
	if taskCreationFilter == nil {
		panic("taskCreationFilter is required")
	}
	ghClient := pkg.NewGitHubClient(httpClient)
	h := handler.NewSinglePRTriggerHandler(
		ghClient,
		createSender,
		taskCreationFilter,
		stage,
		maxSlugLen,
		maxTitleLen,
		taskSuffix,
		metrics,
	)
	return libhttp.WithErrorFunc(h.ServeHTTP)
}
