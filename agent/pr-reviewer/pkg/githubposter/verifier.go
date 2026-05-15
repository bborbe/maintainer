// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubposter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	errors "github.com/bborbe/errors"

	prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
)

type reviewVerifier struct {
	httpClient HTTPClient
	ghToken    string
	botLogin   string
}

// NewReviewVerifier creates a ReviewVerifier. botLogin must already be resolved by the caller.
func NewReviewVerifier(httpClient HTTPClient, ghToken string, botLogin string) ReviewVerifier {
	return &reviewVerifier{httpClient: httpClient, ghToken: ghToken, botLogin: botLogin}
}

// findReview scans a list of reviews for one that matches botLogin, headSHA, and any expected state.
func findReview(
	reviews []reviewEntry,
	botLogin, headSHA string,
	expectedStates []string,
) (reviewEntry, bool) {
	for _, r := range reviews {
		if r.User.Login != botLogin || r.CommitID != headSHA {
			continue
		}
		for _, expected := range expectedStates {
			if r.State == expected {
				return r, true
			}
		}
	}
	return reviewEntry{}, false
}

func (v *reviewVerifier) VerifyReview(ctx context.Context, req VerifyRequest) VerifyResult {
	start := time.Now()
	step := "GET /pulls/N/reviews (ai_review verify)"
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/pulls/%d/reviews",
		req.PR.Owner, req.PR.Repo, req.PR.Number,
	)
	cr := retryCall(ctx, step, func(ctx context.Context) (reviewEntry, int, string, error) {
		status, body, err := doRequest(ctx, v.httpClient, v.ghToken, "GET", url, nil)
		if err != nil {
			return reviewEntry{}, status, truncateBody(body), err
		}
		if status != 200 {
			return reviewEntry{}, status, truncateBody(body),
				errors.Errorf(ctx, "unexpected status %d", status)
		}
		var reviews []reviewEntry
		if err := json.Unmarshal(body, &reviews); err != nil {
			return reviewEntry{}, status, truncateBody(body),
				errors.Wrapf(ctx, err, "parse reviews")
		}
		if r, ok := findReview(reviews, v.botLogin, req.HeadSHA, req.ExpectedStates); ok {
			return r, status, truncateBody(body), nil
		}
		return reviewEntry{}, 200, truncateBody(body), errPhantomPOST
	})
	elapsed := time.Since(start).Milliseconds()
	if errors.Is(cr.Err, errPhantomPOST) {
		return VerifyResult{
			Found:        false,
			Outcome:      "failed",
			FailureStep:  step,
			Class:        prpkg.ErrorClassTransient,
			Attempt:      cr.Attempts,
			ErrorMessage: "no matching bot review for head SHA",
			ElapsedMs:    elapsed,
		}
	}
	if cr.Err != nil {
		return VerifyResult{
			Found:       false,
			Outcome:     "failed",
			FailureStep: step,
			Class:       cr.Class,
			EscalateHint: cr.Class == prpkg.ErrorClassPermanent ||
				cr.Class == prpkg.ErrorClassUnknown,
			Attempt:      cr.Attempts,
			HTTPStatus:   cr.HTTPStatus,
			ErrorMessage: cr.Err.Error(),
			ResponseBody: cr.ResponseBody,
			ElapsedMs:    elapsed,
		}
	}
	return VerifyResult{
		Found:      true,
		Outcome:    "success",
		FoundState: cr.Value.State,
		Attempt:    cr.Attempts,
		HTTPStatus: cr.HTTPStatus,
		ElapsedMs:  elapsed,
	}
}
