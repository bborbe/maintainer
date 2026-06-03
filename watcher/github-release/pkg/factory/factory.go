// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-release binary.
package factory

import (
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"

	"github.com/bborbe/maintainer/watcher/github-release/pkg"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

// CreateKafkaSender constructs a typed create-task command sender backed by a Kafka sync producer.
func CreateKafkaSender(
	syncProducer libkafka.SyncProducer,
	branch base.Branch,
) task.CreateCommandSender {
	sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
	return task.NewCreateCommandSender(sender)
}

// CreateStaticFilters builds the cycle-invariant filter chain (scope +
// empty_unreleased + auto_release gate). SHAUnchangedFilter is composed in per
// cycle inside Watcher.Poll because it needs a fresh CursorReader.
//
// Shared by main.go and cmd/run-once/main.go so adding a new filter only
// touches one place.
func CreateStaticFilters(allowlist []string) filter.TaskCreationFilter {
	return filter.TaskCreationFilters{
		filter.NewRepoAllowlistFilter(allowlist),
		filter.NewEmptyUnreleasedFilter(),
		filter.NewAutoReleaseFilter(),
	}
}

// CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
//
// Pure composition — no I/O. The Kafka sync producer and the HTTP-resolved
// task sender are constructed by main.go (so the caller controls connection
// lifecycle + cleanup). The HTTP client is constructed by pkg/auth before
// this is called. taskCreationFilter is built by CreateStaticFilters.
//
// Reference: watcher/github-pr/pkg/factory/factory.go follows the same
// no-I/O-in-factory pattern.
func CreateWatcher(
	httpClient *http.Client,
	sender task.CreateCommandSender,
	cursorPath string,
	owner string,
	taskCreationFilter filter.TaskCreationFilter,
	metrics pkg.Metrics,
	stage string,
) pkg.Watcher {
	ghClient := pkg.NewGitHubClient(httpClient)
	publisher := pkg.NewTaskPublisher(
		sender,
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
