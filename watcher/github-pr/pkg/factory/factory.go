// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the maintainer-watcher-github-pr binary.
package factory

import (
	"context"
	"net/http"

	task "github.com/bborbe/agent/lib/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"
	"golang.org/x/oauth2"

	"github.com/bborbe/maintainer/lib/githubapp"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

// AuthConfig selects GitHub auth mode. When AppID + InstallationID + PEMKey
// are all set, App auth is used with auto-refreshing IATs (required because
// the watcher is long-lived; a one-shot MintIAT would expire after 1 hour).
// When only GHToken is set, a static-PAT oauth2 client is returned as a
// fallback. Partial App env config returns an error naming the missing
// fields so operators see the misconfig in kubectl logs immediately.
type AuthConfig struct {
	AppID          int64
	InstallationID int64
	PEMKey         string // PEM content (env), not a file path
	GHToken        string
}

// CreateGitHubHTTPClient returns an authenticated *http.Client.
func CreateGitHubHTTPClient(ctx context.Context, cfg AuthConfig) (*http.Client, error) {
	appPartial := (cfg.AppID != 0) || (cfg.InstallationID != 0) || (cfg.PEMKey != "")
	appComplete := (cfg.AppID != 0) && (cfg.InstallationID != 0) && (cfg.PEMKey != "")
	if appPartial && !appComplete {
		var missing []string
		if cfg.AppID == 0 {
			missing = append(missing, "APP_ID")
		}
		if cfg.InstallationID == 0 {
			missing = append(missing, "INSTALLATION_ID")
		}
		if cfg.PEMKey == "" {
			missing = append(missing, "PEM_KEY")
		}
		return nil, errors.Errorf(
			ctx,
			"watcher auth: partial GitHub App config — missing %v; set all three or none",
			missing,
		)
	}
	if appComplete {
		if cfg.GHToken != "" {
			glog.Warningf(
				"watcher auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
			)
		}
		httpClient, err := githubapp.NewClient(ctx, githubapp.Config{
			AppID:          cfg.AppID,
			InstallationID: cfg.InstallationID,
			PEM:            []byte(cfg.PEMKey),
		})
		if err != nil {
			return nil, errors.Wrap(ctx, err, "github app client")
		}
		glog.Infof(
			"watcher auth mode=github-app app_id=%d installation_id=%d",
			cfg.AppID, cfg.InstallationID,
		)
		return httpClient, nil
	}
	if cfg.GHToken != "" {
		glog.Warningf("watcher auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GHToken})
		return oauth2.NewClient(ctx, ts), nil
	}
	return nil, errors.Errorf(
		ctx,
		"watcher auth: neither App nor PAT configured — set APP_ID + INSTALLATION_ID + PEM_KEY, or set GH_TOKEN",
	)
}

// CreateKafkaSender constructs a typed create-task command sender backed by a Kafka sync producer.
// The cleanup function closes the underlying sync producer on shutdown.
func CreateKafkaSender(
	ctx context.Context,
	brokers libkafka.Brokers,
	branch base.Branch,
) (task.CreateCommandSender, func(), error) {
	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx,
		brokers,
		"maintainer-watcher-github-pr",
	)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create sync producer")
	}
	sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
	cleanup := func() {
		if err := syncProducer.Close(); err != nil {
			glog.Warningf("close kafka sync producer: %v", err)
		}
	}
	return task.NewCreateCommandSender(sender), cleanup, nil
}

// CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
func CreateWatcher(
	ctx context.Context,
	auth AuthConfig,
	createSender task.CreateCommandSender,
	stage string,
	repoScope string,
	taskCreationFilter filter.TaskCreationFilter,
	startTime libtime.DateTime,
	trustedAuthors []string,
	maxSlugLen int,
	maxTitleLen int,
	taskSuffix string,
) (pkg.Watcher, error) {
	httpClient, err := CreateGitHubHTTPClient(ctx, auth)
	if err != nil {
		return nil, err
	}
	ghClient := pkg.NewGitHubClient(httpClient)

	trustDecision := trust.And{trust.NewAuthorAllowlist(trustedAuthors)}
	w := pkg.NewWatcher(
		ghClient,
		createSender,
		pkg.DefaultCursorPath,
		startTime,
		repoScope,
		taskCreationFilter,
		stage,
		pkg.NewMetrics(),
		trustDecision,
		maxSlugLen,
		maxTitleLen,
		taskSuffix,
	)
	return w, nil
}
