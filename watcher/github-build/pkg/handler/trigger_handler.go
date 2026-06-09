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

	"github.com/bborbe/maintainer/watcher/github-build/pkg/command"
)

//counterfeiter:generate -o ../../mocks/trigger_handler.go --fake-name TriggerBuildCheckHandler . TriggerBuildCheckHandler

// TriggerBuildCheckHandler handles POST /trigger.
// The handler is intentionally minimal: build a zero-value
// TriggerBuildCheckCommand, publish it to Kafka via the injected
// sender, and return HTTP 202. No request body or query string is
// consumed (both Scope and Force are reserved-unread). All scan
// cycle work is owned by the in-pod command consumer.
type TriggerBuildCheckHandler = libhttp.WithError

// NewTriggerBuildCheckHandler returns a handler that publishes a
// TriggerBuildCheckCommand to Kafka for each /trigger request and
// returns 202. The sender is the only collaborator.
func NewTriggerBuildCheckHandler(
	sender command.TriggerBuildCheckCommandSender,
) TriggerBuildCheckHandler {
	return &triggerBuildCheckHandler{
		sender: sender,
	}
}

type triggerBuildCheckHandler struct {
	sender command.TriggerBuildCheckCommandSender
}

func (h *triggerBuildCheckHandler) ServeHTTP(
	ctx context.Context,
	resp http.ResponseWriter,
	_ *http.Request,
) error {
	// Both fields are reserved-unread; build a zero-value command.
	if err := h.sender.SendCommand(ctx, command.TriggerBuildCheckCommand{}); err != nil {
		// 502 BadGateway over 500/503: upstream Kafka is the proximate cause,
		// not this service. 500 implies an unexpected handler bug; 503 implies
		// this service is unhealthy. Kafka publish failure is neither — it's
		// an upstream gateway dependency, so 502 is the most accurate signal
		// for operators + observability tools.
		return libhttp.WrapWithStatusCode(
			errors.Wrap(ctx, err, "send TriggerBuildCheckCommand"),
			http.StatusBadGateway,
		)
	}

	glog.V(2).Infof("trigger accepted op=%s", command.TriggerBuildCheckCommandOperation)
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
