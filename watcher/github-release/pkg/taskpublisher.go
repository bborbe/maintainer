// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"

	agentlib "github.com/bborbe/agent/lib"
	task "github.com/bborbe/agent/lib/command/task"
	"github.com/golang/glog"
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

func (p *taskPublisher) PublishCreate(ctx context.Context, release Release) bool {
	cmd := BuildCreateCommand(release, p.cfg)

	if err := p.sender.SendCommand(ctx, cmd); err != nil {
		glog.Errorf(
			"publish create-task failed repo=%s sha=%s taskID=%s err=%v",
			release.Repo.Key(),
			release.HeadSHA,
			string(cmd.TaskIdentifier),
			err,
		)
		p.metrics.IncPublished("error")
		return false
	}
	glog.V(2).Infof(
		"published CreateTaskCommand repo=%s sha=%s taskID=%s stage=%s",
		release.Repo.Key(),
		release.HeadSHA,
		string(cmd.TaskIdentifier),
		p.cfg.Stage,
	)
	p.metrics.IncPublished("create")
	return true
}

// BuildCreateCommand assembles the CreateTaskCommand for a Release.
func BuildCreateCommand(release Release, cfg TaskConfig) task.CreateCommand {
	taskIDStr := DeriveTaskID(release.Repo.Owner, release.Repo.Name, release.HeadSHA).String()
	return task.CreateCommand{
		Title:          ComputeTaskTitle(release),
		TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
		Frontmatter:    buildFrontmatter(release, taskIDStr, cfg),
		Body:           buildTaskBody(release),
	}
}

func buildFrontmatter(release Release, taskIDStr string, cfg TaskConfig) agentlib.TaskFrontmatter {
	return agentlib.TaskFrontmatter{
		"task_type":       "github-release",
		"assignee":        "github-releaser-agent",
		"phase":           "planning",
		"status":          "in_progress",
		"stage":           cfg.Stage,
		"task_identifier": taskIDStr,
		"title":           ComputeTaskTitle(release),
		"repo":            fmt.Sprintf("%s/%s", release.Repo.Owner, release.Repo.Name),
		"clone_url": fmt.Sprintf(
			"git@github.com:%s/%s.git",
			release.Repo.Owner,
			release.Repo.Name,
		),
		"ref":             release.HeadSHA,
		"current_version": release.CurrentVersion,
	}
}

func buildTaskBody(release Release) string {
	owner := release.Repo.Owner
	name := release.Repo.Name
	return fmt.Sprintf(
		"# Release: %s/%s\n\n**Current version:** %s\n**HEAD:** %s\n**Changelog:** https://github.com/%s/%s/blob/master/CHANGELOG.md\n**Repo:** [%s/%s](https://github.com/%s/%s)\n",
		owner,
		name,
		release.CurrentVersion,
		release.ShortSHA(),
		owner,
		name,
		owner,
		name,
		owner,
		name,
	)
}
