// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"

	"github.com/bborbe/errors"

	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

//counterfeiter:generate -o mocks/watcher.go --fake-name Watcher . Watcher

// Watcher polls GitHub for repos with non-empty ## Unreleased and publishes
// CreateTaskCommands to Kafka for github-releaser-agent to consume.
type Watcher interface {
	// Poll runs one scan cycle. Safe to call repeatedly on an interval.
	Poll(ctx context.Context) error
}

// NewWatcher wires the watcher's collaborators.
//
// Owner = single GitHub org per watcher instance (multi-org = multiple deployments).
func NewWatcher(
	ghClient GitHubClient,
	publisher TaskPublisher,
	metrics Metrics,
	cursorPath string,
	owner string,
	taskCreationFilter filter.TaskCreationFilter,
) Watcher {
	return &watcher{
		ghClient:           ghClient,
		publisher:          publisher,
		metrics:            metrics,
		cursorPath:         cursorPath,
		owner:              owner,
		taskCreationFilter: taskCreationFilter,
	}
}

type watcher struct {
	ghClient           GitHubClient
	publisher          TaskPublisher
	metrics            Metrics
	cursorPath         string
	owner              string
	taskCreationFilter filter.TaskCreationFilter
}

// Poll implements Watcher. One cycle:
//  1. Load cursor (cold-start safe)
//  2. ListRepos(owner) — full set per cycle
//  3. For each repo (parallel ≤10):
//     a. GetMasterSHA — abort cycle on github_error / rate_limited
//     b. GetChangelogContent → ParseChangelog → ChangelogSummary
//     c. GetAutoReleaseConfig
//     d. Build Release struct
//     e. taskCreationFilter.Skip(release) — bump filter metric on hit
//     f. publisher.PublishCreate(release)
//     g. On successful publish: cursor.Repos[repo.Key()].LastSeenMasterSHA = release.HeadSHA
//  4. SaveCursor
//  5. metrics.IncPollCycle("success")
//
// Reference: watcher/github-pr/pkg/watcher.go Poll loop; watcher/github-build/pkg/watcher.go
// per-repo state-machine loop.
//
// TODO: implement.
func (w *watcher) Poll(ctx context.Context) error {
	return errors.New(ctx, "watcher: Poll not implemented")
}

// CursorReader adapter — wraps *Cursor to satisfy filter.CursorReader without
// introducing the import cycle (filter cannot import pkg).
type cursorReader struct{ c *Cursor }

// NewCursorReader exposes a filter-compatible read view over a Cursor.
func NewCursorReader(c *Cursor) filter.CursorReader {
	return &cursorReader{c: c}
}

func (r *cursorReader) LastSeenSHA(repoKey string) string {
	if r.c == nil || r.c.Repos == nil {
		return ""
	}
	if state, ok := r.c.Repos[repoKey]; ok && state != nil {
		return state.LastSeenMasterSHA
	}
	return ""
}
