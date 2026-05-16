// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"github.com/google/uuid"
)

// buildWatcherNamespace is the fixed v5 UUID namespace for all build-watcher task identifiers.
// Distinct from prWatcherNamespace to prevent cross-service ID collisions.
var buildWatcherNamespace = uuid.MustParse("8e3f5a2c-7b14-4d9e-a017-3c6e8b9f2a1d")

// DeriveTaskID returns a deterministic task identifier for a build-failure episode.
// Input: "<owner>/<repo>#build-<episodeSHA>", e.g. "bborbe/maintainer#build-abc123".
func DeriveTaskID(owner, repo, episodeSHA string) uuid.UUID {
	key := owner + "/" + repo + "#build-" + episodeSHA
	return uuid.NewSHA1(buildWatcherNamespace, []byte(key))
}
