// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/watcher/github-release/pkg/command"
)

//counterfeiter:generate -o ../../mocks/trigger_release_check_handler.go --fake-name TriggerReleaseCheckHandler . TriggerReleaseCheckHandler

// TriggerReleaseCheckHandler handles POST /trigger.
// The handler is intentionally minimal: build a zero-value
// TriggerReleaseCheckCommand, publish it to Kafka via the injected
// sender, and return HTTP 202. No request body or query string is
// consumed (both Scope and Force are reserved-unread). All scan
// cycle work is owned by the in-pod command consumer.
type TriggerReleaseCheckHandler = libhttp.WithError

// NewTriggerReleaseCheckHandler returns a handler that publishes a
// TriggerReleaseCheckCommand to Kafka for each /trigger request and
// returns 202. The sender is the only collaborator.
func NewTriggerReleaseCheckHandler(
	sender command.TriggerReleaseCheckCommandSender,
) TriggerReleaseCheckHandler {
	return &triggerReleaseCheckHandler{
		sender: sender,
	}
}

type triggerReleaseCheckHandler struct {
	sender command.TriggerReleaseCheckCommandSender
}

func (h *triggerReleaseCheckHandler) ServeHTTP(
	ctx context.Context,
	resp http.ResponseWriter,
	_ *http.Request,
) error {
	// Both fields are reserved-unread; build a zero-value command.
	if err := h.sender.SendCommand(ctx, command.TriggerReleaseCheckCommand{}); err != nil {
		return libhttp.WrapWithStatusCode(
			errors.Wrap(ctx, err, "send TriggerReleaseCheckCommand"),
			http.StatusBadGateway,
		)
	}

	glog.V(2).Infof("trigger accepted op=%s", command.TriggerReleaseCheckCommandOperation)
	return writeAccepted(resp)
}

// writeAccepted emits the 202 response with body {"status":"accepted"}.
func writeAccepted(resp http.ResponseWriter) error {
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(http.StatusAccepted)
	return json.NewEncoder(resp).Encode(map[string]interface{}{
		"status": "accepted",
	})
}
