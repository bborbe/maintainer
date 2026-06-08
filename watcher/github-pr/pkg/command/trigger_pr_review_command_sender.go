// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/bborbe/cqrs/base"
	cdb "github.com/bborbe/cqrs/cdb"
	cqrsiam "github.com/bborbe/cqrs/iam"
	"github.com/bborbe/errors"

	"github.com/bborbe/maintainer/lib"
)

//counterfeiter:generate -o ../../mocks/trigger_pr_review_command_sender.go --fake-name TriggerPRReviewCommandSender . TriggerPRReviewCommandSender

// TriggerPRReviewCommandSender sends TriggerPRReviewCommand payloads to
// Kafka. Calls Validate before publishing — a validation error is
// returned without touching Kafka.
type TriggerPRReviewCommandSender interface {
	SendCommand(ctx context.Context, cmd TriggerPRReviewCommand) error
}

// NewTriggerPRReviewCommandSender creates a TriggerPRReviewCommandSender
// using the given cdb.CommandObjectSender.
func NewTriggerPRReviewCommandSender(
	commandObjectSender cdb.CommandObjectSender,
) TriggerPRReviewCommandSender {
	return &triggerPRReviewCommandSender{
		commandObjectSender: commandObjectSender,
	}
}

type triggerPRReviewCommandSender struct {
	commandObjectSender cdb.CommandObjectSender
}

func (s *triggerPRReviewCommandSender) SendCommand(
	ctx context.Context,
	cmd TriggerPRReviewCommand,
) error {
	if err := cmd.Validate(ctx); err != nil {
		return errors.Wrapf(ctx, err, "validate TriggerPRReviewCommand")
	}
	event, err := base.ParseEvent(ctx, cmd)
	if err != nil {
		return errors.Wrapf(ctx, err, "parse TriggerPRReviewCommand event")
	}
	requestIDCh := make(chan base.RequestID, 1)
	requestIDCh <- base.NewRequestID()
	commandCreator := base.NewCommandCreator(requestIDCh)
	commandObject := cdb.CommandObject{
		Command: commandCreator.NewCommand(
			TriggerPRReviewCommandOperation,
			cqrsiam.Initiator("watcher-github-pr"),
			"",
			event,
		),
		SchemaID: lib.GithubPRReviewV1SchemaID,
	}
	if err := s.commandObjectSender.SendCommandObject(ctx, commandObject); err != nil {
		return errors.Wrapf(ctx, err, "send TriggerPRReviewCommand to Kafka")
	}
	return nil
}
