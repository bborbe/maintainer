// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import "context"

//counterfeiter:generate -o ../mocks/pr-poster.go --fake-name PrPoster . PrPoster

// PrPoster posts a completed review verdict to GitHub as a pull-request review event.
// The concrete implementation lives in pkg/githubposter and is wired by the factory.
// Defining the interface in pkg (rather than pkg/githubposter) breaks the import cycle:
// pkg/githubposter already imports pkg for PRInfo/Verdict.
type PrPoster interface {
	Post(ctx context.Context, req PostRequest) PostResult
}

// PostRequest carries all inputs needed for a single posting sequence.
type PostRequest struct {
	PR      PRInfo
	HeadSHA string
	Verdict Verdict
	Summary string
	WorkDir string
}

// PostResult carries all diagnostic fields needed for the ## Diagnostics block.
type PostResult struct {
	Outcome      string
	ReviewID     int64
	PostedEvent  string
	FailureStep  string
	Class        ErrorClass
	EscalateHint bool
	Attempt      int
	HTTPStatus   int
	ErrorMessage string
	ResponseBody string
	ElapsedMs    int64
	Warnings     []string
}

// ErrorClass categorizes a posting failure for retry and escalation decisions.
type ErrorClass string

const (
	// ErrorClassTransient indicates a transient failure that may succeed on retry.
	ErrorClassTransient ErrorClass = "transient"
	// ErrorClassPermanent indicates a permanent failure that will not succeed on retry.
	ErrorClassPermanent ErrorClass = "permanent"
	// ErrorClassUnknown indicates an unknown error class.
	ErrorClassUnknown ErrorClass = "unknown"
	// ErrorClassNotAFailure indicates a non-failure outcome (e.g., 422 PR closed).
	ErrorClassNotAFailure ErrorClass = "not-a-failure"
	// ErrorClassSoftWarning indicates a soft warning that does not block posting.
	ErrorClassSoftWarning ErrorClass = "soft-warning"
)
