// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-pr binary.
package factory

import (
	"context"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// CreateKafkaSender constructs a typed create-task command sender backed by a Kafka sync producer.
// The cleanup function closes the underlying sync producer on shutdown.
func CreateKafkaSender(
	ctx context.Context,
	brokers libkafka.Brokers,
	branch base.Branch,
) (task.CreateCommandSender, func(), error) {
	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx,
		brokers,
		"maintainer-watcher-github-pr",
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
	repoScope string,
	taskCreationFilter filter.TaskCreationFilter,
	startTime libtime.DateTime,
	trustedAuthors []string,
	maxSlugLen int,
	maxTitleLen int,
) (pkg.Watcher, func(), error) {
	branch := base.Branch(stage)
	createSender, cleanup, err := CreateKafkaSender(ctx, brokers, branch)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create kafka sender")
	}

	trustDecision := trust.And{trust.NewAuthorAllowlist(trustedAuthors)}
	ghClient := pkg.NewGitHubClient(ghToken)
	w := pkg.NewWatcher(
		ghClient,
		createSender,
		pkg.DefaultCursorPath,
		startTime,
		repoScope,
		taskCreationFilter,
		stage,
		pkg.NewMetrics(),
		trustDecision,
		maxSlugLen,
		maxTitleLen,
	)
	return w, cleanup, nil
}
