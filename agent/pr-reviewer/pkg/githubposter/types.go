// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubposter

import (
	"context"
	"net/http"

	prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
)

//counterfeiter:generate -o ../../mocks/http-client.go --fake-name HTTPClient . HTTPClient
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

//counterfeiter:generate -o ../../mocks/review-verifier.go --fake-name ReviewVerifier . ReviewVerifier
type ReviewVerifier interface {
	VerifyReview(ctx context.Context, req VerifyRequest) VerifyResult
}

// VerifyRequest carries all inputs for a single review-existence check.
type VerifyRequest struct {
	PR             prpkg.PRInfo
	HeadSHA        string
	ExpectedStates []string
}

// VerifyResult carries all diagnostic fields from the ai_review verification GET.
type VerifyResult struct {
	Found        bool
	Outcome      string
	FoundState   string
	FailureStep  string
	Class        prpkg.ErrorClass
	EscalateHint bool
	Attempt      int
	HTTPStatus   int
	ErrorMessage string
	ResponseBody string
	ElapsedMs    int64
}

// AutoApproveConfig holds the parsed .pr-reviewer.yaml content.
type AutoApproveConfig struct {
	AutoApprove bool `yaml:"autoApprove"`
}

const (
	// DefaultBotLogin is the GitHub login the agent posts as by default.
	DefaultBotLogin = "pr-review-of-ben"

	// BotLoginEnv is the env var that overrides DefaultBotLogin (read by the factory).
	BotLoginEnv = "BOT_GITHUB_LOGIN"
)
