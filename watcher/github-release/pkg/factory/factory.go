// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-release binary.
package factory

import (
	"context"
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	"golang.org/x/oauth2"

	"github.com/bborbe/maintainer/lib/githubapp"
	"github.com/bborbe/maintainer/watcher/github-release/pkg"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

// CreateGitHubAppClient creates an HTTP client authenticated as a GitHub App installation.
//
// Carried verbatim from watcher/github-pr — same auth shape.
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
//
// taskCreationFilter is constructed by main.go (scope filter, empty-unreleased,
// auto-release, sha-unchanged — cursor-aware) and passed in. Cursor itself is
// loaded inside Watcher.Poll on each cycle.
func CreateWatcher(
	httpClient *http.Client,
	createSender task.CreateCommandSender,
	cursorPath string,
	owner string,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	metrics pkg.Metrics,
) pkg.Watcher {
	ghClient := pkg.NewGitHubClient(httpClient)
	publisher := pkg.NewTaskPublisher(
		createSender,
		metrics,
		pkg.TaskConfig{Stage: stage},
	)
	return pkg.NewWatcher(
		ghClient,
		publisher,
		metrics,
		cursorPath,
		owner,
		taskCreationFilter,
	)
}
