// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"

	"github.com/bborbe/errors"
)

// DefaultCursorPath is the default cursor persistence location.
// k8s mounts /data as a PVC; main.go binds CURSOR_PATH=DefaultCursorPath.
const DefaultCursorPath = "/data/cursor.json"

// Cursor is the per-repo head-SHA dedup state.
//
// Shape rationale (vs watcher/github-pr's time-based cursor):
//   - Release watcher only emits a task when master HEAD advances on a repo
//     with non-empty ## Unreleased. The relevant "last seen" is per-repo master SHA.
//   - No PR-update-time scan (no upstream "since" filter for repo head moves).
//   - Per-repo map mirrors watcher/github-build's shape.
//
// Concurrency: not safe for concurrent use. The Watcher loads at poll start
// and saves at poll end (single goroutine).
type Cursor struct {
	Repos map[string]*RepoState `json:"repos"` // key: "owner/repo"
}

// RepoState is the cursor entry per repo.
type RepoState struct {
	LastSeenMasterSHA string `json:"last_seen_master_sha"`
}

// LoadCursor reads cursor state from path.
// Missing file → fresh empty cursor (cold start is valid).
// Corrupt file → error (caller should refuse to proceed; mirrors github-build policy).
//
// Reference: watcher/github-pr/pkg/cursor.go LoadCursor (time-based variant),
// watcher/github-build/pkg/cursor.go LoadCursor (per-repo state-machine variant).
//
// TODO: implement (atomic read; JSON unmarshal; cold-start on missing).
func LoadCursor(ctx context.Context, _ string) (*Cursor, error) {
	return nil, errors.New(ctx, "cursor: LoadCursor not implemented")
}

// SaveCursor persists cursor state to path atomically via temp file + rename.
//
// TODO: implement (json.Marshal; os.WriteFile(path+".tmp", 0600); os.Rename).
func SaveCursor(ctx context.Context, _ string, _ *Cursor) error {
	return errors.New(ctx, "cursor: SaveCursor not implemented")
}
