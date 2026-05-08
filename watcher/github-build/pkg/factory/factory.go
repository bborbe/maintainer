// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-build binary.
package factory

import (
	"context"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/watcher/github-build/pkg"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/maintenance"
)

// CreateKafkaCreateSender constructs a typed create-task command sender backed by a Kafka sync producer.
// The cleanup function closes the underlying sync producer on shutdown.
func CreateKafkaCreateSender(
	ctx context.Context,
	brokers libkafka.Brokers,
	branch base.Branch,
) (task.CreateCommandSender, func(), error) {
	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx,
		brokers,
		"maintainer-watcher-github-build",
	)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create sync producer")
	}
	sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
	cleanup := func() {
		if err := syncProducer.Close(); err != nil {
			glog.Warningf("close kafka sync producer: %v", err)
		}
	}
	return task.NewCreateCommandSender(sender), cleanup, nil
}

// CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
func CreateWatcher(
	ctx context.Context,
	ghToken string,
	brokers libkafka.Brokers,
	stage string,
	allowlist []string,
	cursorPath string,
	assignee string,
	taskStatus string,
	taskPhase string,
	maxTitleLen int,
) (pkg.Watcher, func(), error) {
	branch := base.Branch(stage)
	createSender, cleanup, err := CreateKafkaCreateSender(ctx, brokers, branch)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create kafka create sender")
	}
	ghClient := pkg.NewGitHubClient(ghToken)
	maintenanceLoader := maintenance.NewLoader(ghClient)
	repoFilter := filter.RepoFilters{filter.NewRepoAllowlistFilter(allowlist)}
	w := pkg.NewWatcher(
		ghClient,
		createSender,
		pkg.NewMetrics(),
		repoFilter,
		allowlist,
		cursorPath,
		assignee,
		taskStatus,
		taskPhase,
		maintenanceLoader,
		maxTitleLen,
	)
	return w, cleanup, nil
}
