// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubposter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	errors "github.com/bborbe/errors"

	prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
)

type prPoster struct {
	httpClient HTTPClient
	ghToken    string
	botLogin   string
}

// NewPrPoster creates a prpkg.PrPoster. botLogin must already be resolved by the caller.
func NewPrPoster(httpClient HTTPClient, ghToken string, botLogin string) prpkg.PrPoster {
	return &prPoster{httpClient: httpClient, ghToken: ghToken, botLogin: botLogin}
}

// reviewEntry is the GitHub API shape for a single pull-request review.
type reviewEntry struct {
	ID   int64 `json:"id"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	CommitID string `json:"commit_id"`
	State    string `json:"state"`
}

type postReviewReq struct {
	Event    string `json:"event"`
	CommitID string `json:"commit_id"`
	Body     string `json:"body"`
}

type postReviewResp struct {
	ID int64 `json:"id"`
}

func (p *prPoster) Post(ctx context.Context, req prpkg.PostRequest) prpkg.PostResult {
	start := time.Now()
	if result, ok := p.checkBotIdentity(ctx); !ok {
		result.ElapsedMs = time.Since(start).Milliseconds()
		return result
	}
	config, err := ReadAutoApproveConfig(ctx, req.WorkDir)
	if err != nil {
		return prpkg.PostResult{
			Outcome:      "failed",
			FailureStep:  "read .pr-reviewer.yaml",
			Class:        prpkg.ErrorClassPermanent,
			EscalateHint: true,
			Attempt:      1,
			ErrorMessage: err.Error(),
			ElapsedMs:    time.Since(start).Milliseconds(),
		}
	}
	if result, ok := p.dismissPriorReviews(ctx, req.PR, req.HeadSHA); !ok {
		result.ElapsedMs = time.Since(start).Milliseconds()
		return result
	}
	event, body, warnings := mapVerdictAndSummary(req.Verdict, config.AutoApprove, req.Summary)
	result := p.postAndVerify(ctx, req.PR, req.HeadSHA, event, body, warnings)
	result.ElapsedMs = time.Since(start).Milliseconds()
	return result
}

func (p *prPoster) checkBotIdentity(ctx context.Context) (prpkg.PostResult, bool) {
	type userResp struct {
		Login string `json:"login"`
	}
	step := "GET /user"
	cr := retryCall(ctx, step, func(ctx context.Context) (userResp, int, string, error) {
		status, body, err := doRequest(
			ctx,
			p.httpClient,
			p.ghToken,
			"GET",
			"https://api.github.com/user",
			nil,
		)
		if err != nil {
			return userResp{}, status, truncateBody(body), err
		}
		if status != 200 {
			return userResp{}, status, truncateBody(
					body,
				), errors.Errorf(
					ctx,
					"unexpected status %d",
					status,
				)
		}
		var u userResp
		if err := json.Unmarshal(body, &u); err != nil {
			return userResp{}, status, truncateBody(
					body,
				), errors.Wrapf(
					ctx,
					err,
					"parse /user response",
				)
		}
		return u, status, truncateBody(body), nil
	})
	if cr.Err != nil {
		return buildFailedResult(step, cr), false
	}
	if cr.Value.Login != p.botLogin {
		return prpkg.PostResult{
			Outcome:      "failed",
			FailureStep:  step,
			Class:        prpkg.ErrorClassPermanent,
			EscalateHint: true,
			Attempt:      cr.Attempts,
			HTTPStatus:   cr.HTTPStatus,
			ErrorMessage: fmt.Sprintf(
				"bot identity mismatch: expected %s got %s",
				p.botLogin,
				cr.Value.Login,
			),
		}, false
	}
	return prpkg.PostResult{}, true
}

func (p *prPoster) dismissPriorReviews(
	ctx context.Context,
	pr prpkg.PRInfo,
	headSHA string,
) (prpkg.PostResult, bool) {
	step := "GET /pulls/N/reviews (dismiss-list)"
	reviews, result, ok := p.listBotReviews(ctx, pr, headSHA, step)
	if !ok {
		return result, false
	}
	for _, r := range reviews {
		if r.State == "DISMISSED" {
			continue
		}
		if result, ok := p.dismissOne(ctx, pr, r.ID); !ok {
			return result, false
		}
	}
	return prpkg.PostResult{}, true
}

func (p *prPoster) listBotReviews(
	ctx context.Context,
	pr prpkg.PRInfo,
	headSHA, step string,
) ([]reviewEntry, prpkg.PostResult, bool) {
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/pulls/%d/reviews",
		pr.Owner,
		pr.Repo,
		pr.Number,
	)
	cr := retryCall(ctx, step, func(ctx context.Context) ([]reviewEntry, int, string, error) {
		status, body, err := doRequest(ctx, p.httpClient, p.ghToken, "GET", url, nil)
		if err != nil {
			return nil, status, truncateBody(body), err
		}
		if status != 200 {
			return nil, status, truncateBody(
					body,
				), errors.Errorf(
					ctx,
					"unexpected status %d",
					status,
				)
		}
		var all []reviewEntry
		if err := json.Unmarshal(body, &all); err != nil {
			return nil, status, truncateBody(body), errors.Wrapf(ctx, err, "parse reviews")
		}
		var filtered []reviewEntry
		for _, r := range all {
			if r.User.Login == p.botLogin && r.CommitID == headSHA {
				filtered = append(filtered, r)
			}
		}
		return filtered, status, truncateBody(body), nil
	})
	if cr.Err != nil {
		return nil, buildFailedResult(step, cr), false
	}
	return cr.Value, prpkg.PostResult{}, true
}

func (p *prPoster) dismissOne(
	ctx context.Context,
	pr prpkg.PRInfo,
	reviewID int64,
) (prpkg.PostResult, bool) {
	step := "PUT .../dismissals"
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/pulls/%d/reviews/%d/dismissals",
		pr.Owner, pr.Repo, pr.Number, reviewID,
	)
	payload := []byte(`{"message":"superseded by new automated review"}`)
	cr := retryCall(ctx, step, func(ctx context.Context) (struct{}, int, string, error) {
		status, body, err := doRequest(
			ctx,
			p.httpClient,
			p.ghToken,
			"PUT",
			url,
			bytes.NewReader(payload),
		)
		if err != nil {
			return struct{}{}, status, truncateBody(body), err
		}
		if status < 200 || status >= 300 {
			return struct{}{}, status, truncateBody(
					body,
				), errors.Errorf(
					ctx,
					"unexpected status %d",
					status,
				)
		}
		return struct{}{}, status, truncateBody(body), nil
	})
	if cr.Err != nil {
		return buildFailedResult(step, cr), false
	}
	return prpkg.PostResult{}, true
}

func (p *prPoster) postAndVerify(
	ctx context.Context,
	pr prpkg.PRInfo,
	headSHA, event, body string,
	warnings []string,
) prpkg.PostResult {
	reviewID, result, proceed := p.postReview(ctx, pr, headSHA, event, body)
	if !proceed {
		result.Warnings = warnings
		return result
	}
	result = p.verifyAfterPost(ctx, pr, headSHA, event, warnings)
	result.ReviewID = reviewID
	result.PostedEvent = event
	return result
}

func (p *prPoster) postReview(
	ctx context.Context,
	pr prpkg.PRInfo,
	headSHA, event, body string,
) (int64, prpkg.PostResult, bool) {
	const step = "POST /pulls/N/reviews"
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/pulls/%d/reviews",
		pr.Owner,
		pr.Repo,
		pr.Number,
	)
	payload, err := json.Marshal(postReviewReq{Event: event, CommitID: headSHA, Body: body})
	if err != nil {
		return 0, prpkg.PostResult{
			Outcome: "failed", FailureStep: step,
			Class: prpkg.ErrorClassPermanent, EscalateHint: true, Attempt: 1, ErrorMessage: err.Error(),
		}, false
	}
	cr := retryCall(ctx, step, func(ctx context.Context) (postReviewResp, int, string, error) {
		status, rb, err := doRequest(
			ctx,
			p.httpClient,
			p.ghToken,
			"POST",
			url,
			bytes.NewReader(payload),
		)
		tb := truncateBody(rb)
		if err != nil {
			return postReviewResp{}, status, tb, err
		}
		if status == 422 {
			return postReviewResp{}, status, tb, errors.Errorf(
				ctx,
				"PR closed or validation error (422)",
			)
		}
		if status < 200 || status >= 300 {
			return postReviewResp{}, status, tb, errors.Errorf(ctx, "unexpected status %d", status)
		}
		var r postReviewResp
		if err := json.Unmarshal(rb, &r); err != nil {
			return postReviewResp{}, status, tb, errors.Wrapf(ctx, err, "parse POST response")
		}
		return r, status, tb, nil
	})
	if cr.HTTPStatus == 422 {
		return 0, prpkg.PostResult{
			Outcome: "success", Class: prpkg.ErrorClassNotAFailure,
			FailureStep: step, HTTPStatus: 422, Attempt: cr.Attempts,
		}, false
	}
	if cr.Err != nil {
		return 0, buildFailedResult(step, cr), false
	}
	return cr.Value.ID, prpkg.PostResult{}, true
}

func (p *prPoster) verifyAfterPost(
	ctx context.Context,
	pr prpkg.PRInfo,
	headSHA, event string,
	warnings []string,
) prpkg.PostResult {
	step := "GET /pulls/N/reviews (verify)"
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/pulls/%d/reviews",
		pr.Owner,
		pr.Repo,
		pr.Number,
	)
	expectedState := eventToState(event)
	cr := retryCall(ctx, step, func(ctx context.Context) (bool, int, string, error) {
		status, body, err := doRequest(ctx, p.httpClient, p.ghToken, "GET", url, nil)
		if err != nil {
			return false, status, truncateBody(body), err
		}
		if status != 200 {
			return false, status, truncateBody(
					body,
				), errors.Errorf(
					ctx,
					"unexpected status %d",
					status,
				)
		}
		var reviews []reviewEntry
		if err := json.Unmarshal(body, &reviews); err != nil {
			return false, status, truncateBody(body), errors.Wrapf(ctx, err, "parse reviews")
		}
		for _, r := range reviews {
			if r.User.Login == p.botLogin && r.CommitID == headSHA && r.State == expectedState {
				return true, status, truncateBody(body), nil
			}
		}
		return false, 200, truncateBody(body), errPhantomPOST
	})
	if errors.Is(cr.Err, errPhantomPOST) {
		return prpkg.PostResult{
			Outcome:      "failed",
			FailureStep:  step,
			Class:        prpkg.ErrorClassTransient,
			Attempt:      cr.Attempts,
			ErrorMessage: "phantom POST: review absent in GET after POST",
			Warnings:     warnings,
		}
	}
	if cr.Err != nil {
		r := buildFailedResult(step, cr)
		r.Warnings = warnings
		return r
	}
	return prpkg.PostResult{
		Outcome:    "success",
		HTTPStatus: cr.HTTPStatus,
		Attempt:    cr.Attempts,
		Warnings:   warnings,
	}
}

// doRequest executes an HTTP request with GitHub auth headers and returns the full response body.
// The caller is responsible for truncating the body if storing for diagnostics.
func doRequest(
	ctx context.Context,
	client HTTPClient,
	token, method, rawURL string,
	body io.Reader,
) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return 0, nil, errors.Wrapf(ctx, err, "create request %s %s", method, rawURL)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, errors.Wrapf(ctx, err, "do request %s %s", method, rawURL)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, errors.Wrapf(ctx, err, "read response body")
	}
	return resp.StatusCode, bodyBytes, nil
}

// truncateBody returns at most 500 bytes of the body as a string, for diagnostics.
func truncateBody(b []byte) string {
	if len(b) > 500 {
		return string(b[:500])
	}
	return string(b)
}

// mapVerdictAndSummary maps verdict + autoApprove to a GitHub review event and body.
// Empty summary is substituted with a default and recorded as a soft-warning.
func mapVerdictAndSummary(
	verdict prpkg.Verdict,
	autoApprove bool,
	summary string,
) (event, body string, warnings []string) {
	switch {
	case verdict == prpkg.VerdictRequestChanges:
		event = "REQUEST_CHANGES"
	case autoApprove:
		event = "APPROVE"
	default:
		event = "COMMENT"
		body = "auto-approve disabled for this repo, review submitted as comment\n\n"
	}
	if summary == "" {
		summary = "automated review — no summary produced"
		warnings = []string{"soft-warning: empty summary substituted with default"}
	}
	body += summary
	return event, body, warnings
}

// eventToState converts a GitHub review event string to its resulting state string.
func eventToState(event string) string {
	switch event {
	case "APPROVE":
		return "APPROVED"
	case "REQUEST_CHANGES":
		return "CHANGES_REQUESTED"
	default:
		return "COMMENTED"
	}
}

// buildFailedResult builds a PostResult representing a failed step from a CallResult.
func buildFailedResult[T any](step string, cr CallResult[T]) prpkg.PostResult {
	msg := ""
	if cr.Err != nil {
		msg = cr.Err.Error()
	}
	return prpkg.PostResult{
		Outcome:      "failed",
		FailureStep:  step,
		Class:        cr.Class,
		EscalateHint: cr.Class == prpkg.ErrorClassPermanent || cr.Class == prpkg.ErrorClassUnknown,
		Attempt:      cr.Attempts,
		HTTPStatus:   cr.HTTPStatus,
		ErrorMessage: msg,
		ResponseBody: cr.ResponseBody,
	}
}
