// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

// ParseOwnerRepoForTest exposes the unexported parseOwnerRepo helper so
// the external _test package can exercise it directly.
var ParseOwnerRepoForTest = parseOwnerRepo

// ClassifyValidationFailureForTest exposes the unexported
// classifyValidationFailure helper for direct testing of its branches.
var ClassifyValidationFailureForTest = classifyValidationFailure
