// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/bborbe/cqrs/base"
)

// TriggerBuildCheckCommandOperation is the Kafka command operation for
// triggering a github-build poll cycle. Wire string: "trigger-build-check".
const TriggerBuildCheckCommandOperation base.CommandOperation = "trigger-build-check"

// TriggerBuildCheckCommand is the payload for TriggerBuildCheckCommandOperation.
// It is published to the github-build watcher's request topic by the /trigger
// HTTP handler and consumed by the in-pod command consumer.
//
// Scope is reserved for a future per-repo filter UX; this spec plumbs the
// field but the executor ignores it. Force is reserved for the prerequisite
// Force-flag task; this spec plumbs the field but the executor does not
// branch on it. Both fields are present on the wire so the schema does
// not need to change later.
type TriggerBuildCheckCommand struct {
	Scope string `json:"scope,omitempty"`
	Force bool   `json:"force,omitempty"`
}

// Validate enforces the command's schema rules. The empty payload {} is
// accepted because both fields are reserved-unread — there's no
// per-request field with meaning today. A future spec will add
// per-repo or per-stage validation here.
func (cmd TriggerBuildCheckCommand) Validate(_ context.Context) error {
	return nil
}
