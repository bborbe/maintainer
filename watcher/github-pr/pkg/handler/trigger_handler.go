// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/lib/prurl"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// successResponse is the JSON body on HTTP 200.
type successResponse struct {
	TaskID   string `json:"task_id"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"pr_number"`
	HeadSHA  string `json:"head_sha"`
}

// errorResponse is the JSON body on HTTP 4xx/5xx.
type errorResponse struct {
	Error  string `json:"error"`
	Filter string `json:"filter,omitempty"`
	PRURL  string `json:"pr_url,omitempty"`
}

// SinglePRTriggerHandler handles POST /trigger?url=<pr_url>
//counterfeiter:generate -o ../mocks/single_pr_trigger_handler.go --fake-name SinglePRTriggerHandler . SinglePRTriggerHandler

type SinglePRTriggerHandler interface {
	ServeHTTP(resp http.ResponseWriter, req *http.Request)
}

// NewSinglePRTriggerHandler returns a handler that fires a single PR review by URL.
// The filter and trustDecision are passed in (reused from the poll path) — not created here.
func NewSinglePRTriggerHandler(
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	taskCreationFilter filter.TaskCreationFilter,
	trustDecision trust.Trust,
	stage string,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
) SinglePRTriggerHandler {
	return &singlePRTriggerHandler{
		ghClient:           ghClient,
		createSender:       createSender,
		taskCreationFilter: taskCreationFilter,
		trustDecision:      trustDecision,
		stage:              stage,
		maxSlugLen:         maxSlugLen,
		maxTitleLen:        maxTitleLen,
		taskSuffix:         taskSuffix,
	}
}

type singlePRTriggerHandler struct {
	ghClient           pkg.GitHubClient
	createSender       task.CreateCommandSender
	taskCreationFilter filter.TaskCreationFilter
	trustDecision      trust.Trust
	stage              string
	maxSlugLen         int
	maxTitleLen        int
	taskSuffix         string
}

func (h *singlePRTriggerHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	rawURL := req.URL.Query().Get("url")
	if rawURL == "" {
		h.writeError(resp, http.StatusBadRequest, "url query parameter required", "", "")
		return
	}

	prInfo, err := prurl.ParsePRURL(ctx, rawURL)
	if err != nil {
		h.writeError(resp, http.StatusBadRequest, err.Error(), "", rawURL)
		return
	}
	if prInfo.Platform != prurl.PlatformGitHub {
		h.writeError(resp, http.StatusBadRequest,
			fmt.Sprintf("unsupported platform: %s (only github supported)", prInfo.Platform),
			"", rawURL)
		return
	}

	details, err := h.ghClient.GetPRDetails(ctx, prInfo.Owner, prInfo.Repo, prInfo.Number)
	if err != nil {
		h.writeError(resp, http.StatusBadGateway,
			fmt.Sprintf("github fetch failed: %v", err), "", rawURL)
		return
	}

	filterPR := h.buildFilterPR(prInfo, details)
	if h.taskCreationFilter.Skip(filterPR) {
		filterName := h.determineRejectingFilter(filterPR)
		glog.V(2).Infof("trigger: PR filtered by %s pr=%s", filterName, rawURL)
		h.writeError(resp, http.StatusUnprocessableEntity,
			fmt.Sprintf("PR filtered by %s", filterName), filterName, rawURL)
		return
	}

	trustResult, err := h.trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: details.AuthorLogin})
	if err != nil {
		h.writeError(resp, http.StatusBadGateway,
			fmt.Sprintf("trust evaluation failed: %v", err), "", rawURL)
		return
	}

	pr := h.buildPullRequest(prInfo, details, rawURL)
	taskIDStr := pkg.DeriveTaskID(prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA).
		String()

	cmd := pkg.BuildCreateCommand(
		pr,
		details,
		taskIDStr,
		h.stage,
		h.maxSlugLen,
		h.maxTitleLen,
		h.taskSuffix,
		trustResult,
	)

	if err := h.createSender.SendCommand(ctx, cmd); err != nil {
		h.writeError(resp, http.StatusBadGateway,
			fmt.Sprintf("kafka publish failed: %v", err), "", rawURL)
		return
	}

	h.writeSuccess(resp, taskIDStr, prInfo, details)
}

func (h *singlePRTriggerHandler) buildFilterPR(
	prInfo *prurl.PRInfo,
	details pkg.PRDetails,
) filter.PR {
	return filter.PR{
		AuthorLogin: details.AuthorLogin,
		IsDraft:     details.IsDraft,
		Title:       details.Title,
		UpdatedAt:   details.UpdatedAt,
		RepoKey:     "github.com/" + prInfo.Owner + "/" + prInfo.Repo,
	}
}

func (h *singlePRTriggerHandler) buildPullRequest(
	prInfo *prurl.PRInfo,
	details pkg.PRDetails,
	rawURL string,
) pkg.PullRequest {
	return pkg.PullRequest{
		Number:      prInfo.Number,
		Owner:       prInfo.Owner,
		Repo:        prInfo.Repo,
		Title:       details.Title,
		AuthorLogin: details.AuthorLogin,
		HTMLURL:     rawURL,
		IsDraft:     details.IsDraft,
	}
}

func (h *singlePRTriggerHandler) writeSuccess(
	resp http.ResponseWriter,
	taskIDStr string,
	prInfo *prurl.PRInfo,
	details pkg.PRDetails,
) {
	glog.V(2).Infof(
		"trigger: published task_id=%s pr=%s/%s#%d sha=%s",
		taskIDStr,
		prInfo.Owner,
		prInfo.Repo,
		prInfo.Number,
		details.HeadSHA,
	)
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(resp).Encode(successResponse{
		TaskID:   taskIDStr,
		Repo:     prInfo.Owner + "/" + prInfo.Repo,
		PRNumber: prInfo.Number,
		HeadSHA:  details.HeadSHA,
	}); err != nil {
		glog.V(4).Infof("failed to encode success response: %v", err)
	}
}

// determineRejectingFilter identifies which filter rejected the PR.
// Called when taskCreationFilter.Skip returned true.
func (h *singlePRTriggerHandler) determineRejectingFilter(pr filter.PR) string {
	if filter.NewDraftFilter().Skip(pr) {
		return "DraftFilter"
	}
	if filter.NewBotAuthorFilter(nil).Skip(pr) {
		return "BotAuthorFilter"
	}
	if filter.NewWIPTitleFilter().Skip(pr) {
		return "WIPTitleFilter"
	}
	return "TaskCreationFilter"
}

func (h *singlePRTriggerHandler) writeError(
	resp http.ResponseWriter,
	status int,
	errMsg, filterName, prURL string,
) {
	glog.Errorf(
		"trigger error status=%d error=%s filter=%s pr_url=%s",
		status,
		errMsg,
		filterName,
		prURL,
	)
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(status)
	if err := json.NewEncoder(resp).Encode(errorResponse{
		Error:  errMsg,
		Filter: filterName,
		PRURL:  prURL,
	}); err != nil {
		glog.V(4).Infof("failed to encode error response: %v", err)
	}
}
