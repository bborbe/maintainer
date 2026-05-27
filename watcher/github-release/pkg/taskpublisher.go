// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"

	agentlib "github.com/bborbe/agent/lib"
	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/errors"
)

//counterfeiter:generate -o mocks/task_publisher.go --fake-name TaskPublisher . TaskPublisher

// TaskPublisher builds the CreateTaskCommand per [[Agent Task File Contract]] and
// sends it via the supplied CreateCommandSender. Returns true on successful send.
type TaskPublisher interface {
	PublishCreate(ctx context.Context, release Release) bool
}

// TaskConfig groups per-task envelope settings (stage routing).
type TaskConfig struct {
	Stage string // "dev" or "prod" — frontmatter `stage`
}

// NewTaskPublisher returns a TaskPublisher that wraps the given sender + metrics.
func NewTaskPublisher(
	sender task.CreateCommandSender,
	metrics Metrics,
	cfg TaskConfig,
) TaskPublisher {
	return &taskPublisher{sender: sender, metrics: metrics, cfg: cfg}
}

type taskPublisher struct {
	sender  task.CreateCommandSender
	metrics Metrics
	cfg     TaskConfig
}

// PublishCreate implements TaskPublisher. TODO: implement send path + metrics.
func (p *taskPublisher) PublishCreate(ctx context.Context, release Release) bool {
	_ = BuildCreateCommand(release, p.cfg)
	_ = ctx
	return false
}

// BuildCreateCommand assembles the CreateTaskCommand for a Release.
//
// Frontmatter per [[Agent Task File Contract]]:
//
//	task_type: github-release
//	assignee: github-releaser-agent
//	phase: planning
//	status: in_progress
//	stage: <cfg.Stage>
//	task_identifier: <UUID5(owner, repo, head_sha)>
//	title: Release <owner>-<repo> <sha[:7]>
//	repo: owner/name
//	clone_url: git@github.com:owner/name.git
//	ref: <full HEAD SHA>
//	current_version: <vX.Y.Z or v0.0.0>
//
// Body = operator-readable header only (title + version + HEAD + changelog URL +
// repo link). Agent does NOT parse body — clones at `ref` and reads CHANGELOG itself.
//
// Reference: watcher/github-pr/pkg/watcher.go BuildCreateCommand (PR-shaped analogue;
// release version has no untrusted-author branch, no base_ref).
//
// TODO: implement (build agentlib.TaskFrontmatter + body string).
func BuildCreateCommand(release Release, _ TaskConfig) task.CreateCommand {
	taskID := DeriveTaskID(release.Repo.Owner, release.Repo.Name, release.HeadSHA).String()
	return task.CreateCommand{
		Title:          ComputeTaskTitle(release),
		TaskIdentifier: agentlib.TaskIdentifier(taskID),
		// TODO: Frontmatter, Body.
	}
}

// errUnimplemented helps callers distinguish "not built yet" from real errors.
var errUnimplemented = errors.New(context.Background(), "task publisher: not implemented")

// ErrUnimplemented is the sentinel returned by stubs.
func ErrUnimplemented() error { return errUnimplemented }
