// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import "github.com/google/uuid"

// taskIDNamespace is the UUID5 namespace for github-release tasks.
// Stable across releases — changing it would break controller dedup.
var taskIDNamespace = uuid.MustParse("4f9e2c1a-7b30-4d8f-9a2e-1c5b8d4f3a90")

// DeriveTaskID returns a UUID5 derived deterministically from (owner, repo, headSHA).
//
// Uniqueness set rationale (per [[Watcher Writing Guide]] § Deterministic task_identifier):
//   - Same master HEAD on a repo → same task_id → controller dedup makes re-emit a no-op
//   - New commit advances master → new SHA → new task_id → fresh task
//
// Reference: watcher/github-pr/pkg/taskid.go uses (owner, repo, pr_number, head_sha);
// watcher/github-build/pkg/taskid.go uses (owner, repo, episode_sha).
//
// TODO: implement (uuid.NewSHA1 over taskIDNamespace + canonical "owner/repo@sha" bytes).
func DeriveTaskID(_, _, _ string) uuid.UUID {
	return uuid.Nil
}
