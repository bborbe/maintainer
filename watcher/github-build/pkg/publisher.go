// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"encoding/json"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
)

//counterfeiter:generate -o mocks/command_publisher.go --fake-name CommandPublisher . CommandPublisher

// WatcherCreateTaskCommand extends CreateTaskCommand with an optional filename hint.
// FilenameHint lets the task controller name the vault file with a human-readable stem
// (e.g. "Build Failure github - bborbe-maintainer - 5886450") instead of a UUID.
// MUST NOT include the .md extension — the controller appends it.
// Absent controllers (encoding/json default) silently ignore the new field.
type WatcherCreateTaskCommand struct {
	agentlib.CreateTaskCommand
	FilenameHint string `json:"filename_hint,omitempty"`
}

// CommandPublisher publishes task commands to Kafka.
type CommandPublisher interface {
	PublishCreate(ctx context.Context, cmd WatcherCreateTaskCommand) error
}

// NewCommandPublisher returns a CommandPublisher backed by the given CommandObjectSender.
func NewCommandPublisher(ctx context.Context, sender cdb.CommandObjectSender) CommandPublisher {
	return &kafkaPublisher{
		sender:         sender,
		commandCreator: base.NewCommandCreator(base.RequestIDChannel(ctx)),
	}
}

type kafkaPublisher struct {
	sender         cdb.CommandObjectSender
	commandCreator base.CommandCreator
}

func (p *kafkaPublisher) PublishCreate(ctx context.Context, cmd WatcherCreateTaskCommand) error {
	event, err := marshalEvent(ctx, cmd)
	if err != nil {
		return errors.Wrap(ctx, err, "marshal create-task command")
	}
	commandObject := p.buildCommandObject(agentlib.CreateTaskCommandOperation, event)
	if err := p.sender.SendCommandObject(ctx, commandObject); err != nil {
		return errors.Wrap(ctx, err, "publish create-task")
	}
	return nil
}

func marshalEvent(ctx context.Context, v interface{}) (base.Event, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "marshal command to json")
	}
	event, err := base.ParseEvent(ctx, data)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "parse event from json")
	}
	return event, nil
}

func (p *kafkaPublisher) buildCommandObject(
	op base.CommandOperation,
	event base.Event,
) cdb.CommandObject {
	return cdb.CommandObject{
		Command: p.commandCreator.NewCommand(
			op,
			"maintainer-watcher-github-build",
			"",
			event,
		),
		SchemaID: agentlib.TaskV1SchemaID,
	}
}
