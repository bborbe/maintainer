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
	libkv "github.com/bborbe/kv"
	"github.com/bborbe/log"
	"github.com/bborbe/run"
	libtime "github.com/bborbe/time"

	lib "github.com/bborbe/maintainer/lib"
	"github.com/bborbe/maintainer/lib/githubapp"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/command"
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
	ghClient pkg.GitHubClient,
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
	publisher := pkg.NewTaskPublisher(
		createSender,
		trustDecision,
		metrics,
		pkg.TaskConfig{
			Stage:       stage,
			MaxSlugLen:  maxSlugLen,
			MaxTitleLen: maxTitleLen,
			TaskSuffix:  taskSuffix,
		},
	)
	return pkg.NewWatcher(
		ghClient,
		publisher,
		metrics,
		cursorPath,
		startTime,
		scope,
		taskCreationFilter,
	)
}

// CreateTriggerPRReviewCommandSender constructs a typed trigger-PR-review
// command sender backed by a Kafka sync producer. This is the HTTP-side
// sender: the /trigger handler publishes TriggerPRReviewCommand messages
// through it.
func CreateTriggerPRReviewCommandSender(
	syncProducer libkafka.SyncProducer,
	branch base.Branch,
) command.TriggerPRReviewCommandSender {
	sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
	return command.NewTriggerPRReviewCommandSender(sender)
}

// CreateCommandConsumer wires a run.Func that consumes TriggerPRReviewCommand
// messages from the github-pr watcher's request topic and runs them through
// the single-PR review pipeline (GitHub fetch → filter → trust → publish).
//
// The function is pure composition: no business logic, no conditionals.
// It uses cdb.RunCommandConsumerTxDefault (auto-wraps the transaction) per
// the go-cqrs/auto-tx-wrapper-no-manual-wrap rule — do NOT manually wrap
// the executor with kv.NewTransactionMiddleware.
func CreateCommandConsumer(
	saramaClientProvider libkafka.SaramaClientProvider,
	syncProducer libkafka.SyncProducer,
	db libkv.DB,
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	taskCreationFilter filter.TaskCreationFilter,
	trustDecision trust.Trust,
	stage string,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
	metrics pkg.Metrics,
	branch base.Branch,
) run.Func {
	executors := cdb.CommandObjectExecutorTxs{
		command.NewTriggerPRReviewCommandExecutor(
			ghClient,
			createSender,
			taskCreationFilter,
			trustDecision,
			stage,
			maxSlugLen,
			maxTitleLen,
			taskSuffix,
			metrics,
		),
	}
	return cdb.RunCommandConsumerTxDefault(
		saramaClientProvider,
		syncProducer,
		db,
		lib.GithubPRReviewV1SchemaID,
		branch,
		false, // ignoreUnsupported
		executors,
	)
}
