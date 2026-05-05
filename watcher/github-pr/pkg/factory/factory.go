// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-pr binary.
package factory

import (
	"context"

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

// CreateKafkaPublisher constructs a CommandPublisher backed by a Kafka sync producer.
// The cleanup function closes the underlying sync producer on shutdown.
func CreateKafkaPublisher(
	ctx context.Context,
	brokers libkafka.Brokers,
	branch base.Branch,
) (pkg.CommandPublisher, func(), error) {
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
	return pkg.NewCommandPublisher(ctx, sender), cleanup, nil
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
) (pkg.Watcher, func(), error) {
	branch := base.Branch(stage)
	pub, cleanup, err := CreateKafkaPublisher(ctx, brokers, branch)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create kafka publisher")
	}

	trustDecision := trust.And{trust.NewAuthorAllowlist(trustedAuthors)}

	ghClient := pkg.NewGitHubClient(ghToken)
	w := pkg.NewWatcher(
		ghClient,
		pub,
		pkg.DefaultCursorPath,
		startTime,
		repoScope,
		taskCreationFilter,
		stage,
		pkg.NewMetrics(),
		trustDecision,
	)
	return w, cleanup, nil
}
