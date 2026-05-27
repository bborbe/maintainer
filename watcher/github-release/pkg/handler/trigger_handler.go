// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"context"
	"encoding/json"
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/lib/prurl"
	"github.com/bborbe/maintainer/watcher/github-release/pkg"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

// SinglePRTriggerHandler handles POST /trigger?url=<pr_url>
//counterfeiter:generate -o ../mocks/single_pr_trigger_handler.go --fake-name SinglePRTriggerHandler . SinglePRTriggerHandler

type SinglePRTriggerHandler = libhttp.WithError

// NewSinglePRTriggerHandler returns a handler that fires a single PR review by URL.
// The filter is passed in (reused from the poll path) — not created here.
func NewSinglePRTriggerHandler(
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
	metrics pkg.Metrics,
) SinglePRTriggerHandler {
	return &singlePRTriggerHandler{
		ghClient:           ghClient,
		createSender:       createSender,
		taskCreationFilter: taskCreationFilter,
		stage:              stage,
		maxSlugLen:         maxSlugLen,
		maxTitleLen:        maxTitleLen,
		taskSuffix:         taskSuffix,
		metrics:            metrics,
	}
}

type singlePRTriggerHandler struct {
	ghClient           pkg.GitHubClient
	createSender       task.CreateCommandSender
	taskCreationFilter filter.TaskCreationFilter
	stage              string
	maxSlugLen         int
	maxTitleLen        int
	taskSuffix         string
	metrics            pkg.Metrics
}

func (h *singlePRTriggerHandler) ServeHTTP(
	ctx context.Context,
	resp http.ResponseWriter,
	req *http.Request,
) error {
	rawURL := req.URL.Query().Get("url")
	prInfo, err := h.parseAndValidateURL(ctx, rawURL)
	if err != nil {
		return err
	}

	details, err := h.ghClient.GetPRDetails(ctx, prInfo.Owner, prInfo.Repo, prInfo.Number)
	if err != nil {
		return libhttp.WrapWithStatusCode(
			errors.Wrap(ctx, err, "get PR details"),
			http.StatusBadGateway,
		)
	}

	filterPR := h.buildFilterPR(prInfo, details)
	if h.taskCreationFilter.Skip(filterPR) {
		h.metrics.IncPRPublished("skipped")
		return libhttp.WrapWithStatusCode(
			errors.Errorf(ctx, "PR skipped by filter"),
			http.StatusUnprocessableEntity,
		)
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
	)

	if err := h.createSender.SendCommand(ctx, cmd); err != nil {
		h.metrics.IncPRPublished("kafka_error")
		return libhttp.WrapWithStatusCode(
			errors.Wrap(ctx, err, "send create task command"),
			http.StatusBadGateway,
		)
	}

	h.metrics.IncPRPublished("create")
	glog.V(2).Infof(
		"trigger: published task_id=%s pr=%s/%s#%d sha=%s",
		taskIDStr,
		prInfo.Owner,
		prInfo.Repo,
		prInfo.Number,
		details.HeadSHA,
	)

	if err := h.writeSuccess(resp, taskIDStr, prInfo, details.HeadSHA); err != nil {
		glog.V(4).Infof("failed to encode success response: %v", err)
	}
	return nil
}

func (h *singlePRTriggerHandler) parseAndValidateURL(
	ctx context.Context,
	rawURL string,
) (*prurl.PRInfo, error) {
	if rawURL == "" {
		return nil, libhttp.WrapWithStatusCode(
			errors.Errorf(ctx, "url query parameter is required"),
			http.StatusBadRequest,
		)
	}

	prInfo, err := prurl.ParsePRURL(ctx, rawURL)
	if err != nil {
		return nil, libhttp.WrapWithStatusCode(
			errors.Wrap(ctx, err, "parse PR URL"),
			http.StatusBadRequest,
		)
	}
	if prInfo.Platform != prurl.PlatformGitHub {
		return nil, libhttp.WrapWithStatusCode(
			errors.Errorf(ctx, "only github platform is supported, got %s", prInfo.Platform),
			http.StatusBadRequest,
		)
	}
	return prInfo, nil
}

func (h *singlePRTriggerHandler) writeSuccess(
	resp http.ResponseWriter,
	taskIDStr string,
	prInfo *prurl.PRInfo,
	headSHA string,
) error {
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(http.StatusOK)
	return json.NewEncoder(resp).Encode(map[string]interface{}{
		"status":    "ok",
		"task_id":   taskIDStr,
		"repo":      prInfo.Owner + "/" + prInfo.Repo,
		"pr_number": prInfo.Number,
		"head_sha":  headSHA,
	})
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
