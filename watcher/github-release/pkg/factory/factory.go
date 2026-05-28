// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-release binary.
package factory

import (
	"context"
	"net/http"

	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	"github.com/golang/glog"
	"golang.org/x/oauth2"

	task "github.com/bborbe/agent/lib/command/task"
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
	ctx context.Context,
	httpClient *http.Client,
	brokers libkafka.Brokers,
	cursorPath string,
	owner string,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	metrics pkg.Metrics,
	allowlist []string,
) (pkg.Watcher, func(), error) {
	ghClient := pkg.NewGitHubClient(httpClient)
	branch := base.Branch(stage)
	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx, brokers, "maintainer-watcher-github-release",
	)
	if err != nil {
		return nil, nil, errors.Wrapf(ctx, err, "create sync producer")
	}
	createSender := CreateKafkaSender(syncProducer, branch)
	publisher := pkg.NewTaskPublisher(
		createSender,
		metrics,
		pkg.TaskConfig{Stage: stage},
	)
	w := pkg.NewWatcher(
		ghClient,
		publisher,
		metrics,
		cursorPath,
		owner,
		taskCreationFilter,
		allowlist,
	)
	return w, func() {
		glog.Infof("cleanup watcher: closing httpClient and kafka producer")
		httpClient.CloseIdleConnections()
		syncProducer.Close()
	}, nil
}
