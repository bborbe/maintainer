// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-pr binary.
package factory

import (
	"context"
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	libtime "github.com/bborbe/time"
	"golang.org/x/oauth2"

	"github.com/bborbe/maintainer/lib/githubapp"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// CreateGitHubAppClient creates an HTTP client authenticated as a GitHub App installation.
func CreateGitHubAppClient(
	ctx context.Context,
	appID int64,
	installationID int64,
	pemKey []byte,
) (*http.Client, error) {
	cfg := githubapp.Config{
		AppID:          appID,
		InstallationID: installationID,
		PEM:            pemKey,
	}
	return githubapp.NewClient(ctx, cfg)
}

// CreateGitHubPATClient creates an HTTP client authenticated with a personal access token.
func CreateGitHubPATClient(ctx context.Context, token string) *http.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return oauth2.NewClient(ctx, ts)
}

// CreateKafkaSender constructs a typed create-task command sender backed by a Kafka sync producer.
func CreateKafkaSender(
	syncProducer libkafka.SyncProducer,
	branch base.Branch,
) task.CreateCommandSender {
	sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
	return task.NewCreateCommandSender(sender)
}

// CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
func CreateWatcher(
	httpClient *http.Client,
	createSender task.CreateCommandSender,
	cursorPath string,
	startTime libtime.DateTime,
	scope string,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	metrics pkg.Metrics,
	trustDecision trust.Trust,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
) pkg.Watcher {
	ghClient := pkg.NewGitHubClient(httpClient)
	return pkg.NewWatcher(
		ghClient,
		createSender,
		cursorPath,
		startTime,
		scope,
		taskCreationFilter,
		stage,
		metrics,
		trustDecision,
		maxSlugLen,
		maxTitleLen,
		taskSuffix,
	)
}
