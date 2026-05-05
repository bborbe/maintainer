// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"fmt"

	"github.com/google/uuid"
)

// prWatcherNamespace is the fixed v5 UUID namespace for all watcher-derived task identifiers.
// This value is a constant — changing it invalidates all existing task identifiers.
var prWatcherNamespace = uuid.MustParse("7d4b3e5f-8a21-4c9d-b036-2e5f7a8c1d0e")

// DeriveTaskID returns a deterministic task identifier for a PR.
// Input: "<owner>/<repo>#<number>", e.g. "bborbe/code-reviewer#42".
func DeriveTaskID(owner, repo string, number int) uuid.UUID {
	key := fmt.Sprintf("%s/%s#%d", owner, repo, number)
	return uuid.NewSHA1(prWatcherNamespace, []byte(key))
}
