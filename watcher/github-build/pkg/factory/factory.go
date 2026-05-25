// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-build binary.
package factory

import (
	"context"
	"strings"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	"github.com/bborbe/run"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/watcher/github-build/pkg"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/maintenance"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/wildcard"
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
	ghClient pkg.GitHubClient,
	brokers libkafka.Brokers,
	stage string,
	inputAllowlist []string,
	resolved pkg.AllowlistSnapshot,
	cursorPath string,
	assignee string,
	taskStatus string,
	taskPhase string,
	maxTitleLen int,
	taskSuffix string,
) (pkg.Watcher, func(), error) {
	branch := base.Branch(stage)
	createSender, cleanup, err := CreateKafkaCreateSender(ctx, brokers, branch)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create kafka create sender")
	}
	maintenanceLoader := maintenance.NewLoader(ghClient)
	repoFilter := filter.RepoFilters{filter.NewRepoAllowlistFilter(inputAllowlist)}
	w := pkg.NewWatcher(
		ghClient,
		createSender,
		pkg.NewMetrics(),
		repoFilter,
		resolved,
		cursorPath,
		assignee,
		taskStatus,
		taskPhase,
		maintenanceLoader,
		maxTitleLen,
		taskSuffix,
	)
	return w, cleanup, nil
}

// countWildcards returns the number of wildcard entries (host/owner/*) in the list.
func countWildcards(entries []string) int {
	n := 0
	for _, e := range entries {
		parts := strings.Split(strings.TrimSpace(e), "/")
		if len(parts) == 3 && parts[2] == "*" {
			n++
		}
	}
	return n
}

// CreateAllowlistSnapshot returns a snapshot provider and (optionally) a background
// refresh task for the daemon's run loop.
// If the input allowlist contains wildcards, a ResolvedAllowlist with a refresh goroutine
// is returned. Otherwise, a static snapshot with no background refresh is returned.
func CreateAllowlistSnapshot(
	ghClient pkg.GitHubClient,
	repoAllowlist []string,
) (pkg.AllowlistSnapshot, run.Func, error) {
	if wildcard.HasWildcard(repoAllowlist) {
		expander := wildcard.NewExpander(ghClient)
		resolvedSet := wildcard.NewResolvedAllowlist(expander, repoAllowlist)
		glog.V(2).Infof(
			"wildcard_refresh_enabled entries=%d (interval=%s)",
			countWildcards(repoAllowlist), wildcard.RefreshInterval(),
		)
		return resolvedSet, func(ctx context.Context) error {
			defer func() {
				if rec := recover(); rec != nil {
					glog.Errorf("wildcard refresh loop panic recovered: %v", rec)
				}
			}()
			return resolvedSet.RunRefreshLoop(ctx)
		}, nil
	}
	glog.V(2).Infof("wildcard_refresh_disabled allowlist=pure-literal")
	return pkg.NewStaticSnapshot(repoAllowlist), nil, nil
}
