// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command maintainer-watcher-github-build-run-once runs a single GitHub Actions
// poll cycle then exits. Intended for local smoke-testing against a real repo.
// No HTTP server, no poll loop.
package main

import (
	"context"
	"os"

	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	"github.com/golang/glog"

	repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"
	"github.com/bborbe/maintainer/watcher/github-build/pkg"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/auth"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/factory"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/wildcard"
)

func main() {
	app := NewApplication()
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

// NewApplication creates an Application with default dependencies.
func NewApplication() *Application {
	return &Application{
		CreateWatcher: factory.CreateWatcher,
	}
}

type Application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	AppID          int64            `required:"false" arg:"app-id"          env:"APP_ID"          usage:"GitHub App ID (numeric); required for App auth"`
	InstallationID int64            `required:"false" arg:"installation-id" env:"INSTALLATION_ID" usage:"GitHub App Installation ID (numeric)"`
	PEMKeyFile     string           `required:"false" arg:"pem-key-file"    env:"PEM_KEY_FILE"    usage:"Path to the GitHub App private key (PEM) mounted from k8s Secret"`
	PEMKey         string           `required:"false" arg:"pem-key"         env:"PEM_KEY"         usage:"GitHub App private key (PEM) as env var content; mutually exclusive with PEM_KEY_FILE" display:"length"`
	KafkaBrokers   libkafka.Brokers `required:"true"  arg:"kafka-brokers"   env:"KAFKA_BROKERS"   usage:"Comma-separated Kafka broker list"`
	Stage          string           `required:"true"  arg:"stage"           env:"STAGE"           usage:"Deployment stage (dev|prod)"`
	RepoAllowlist  string           `required:"true"  arg:"repo-allowlist"  env:"REPO_ALLOWLIST"  usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); MUST be non-empty"`

	BuildAssignee   string `required:"true"  arg:"build-assignee"    env:"TASK_ASSIGNEE" usage:"Frontmatter assignee for published tasks"                                                                                                                                                                             default:"build-fixer-agent"`
	BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"TASK_STATUS"   usage:"Frontmatter status for published tasks"                                                                                                                                                                               default:"next"`
	BuildTaskPhase  string `required:"false" arg:"build-task-phase"  env:"TASK_PHASE"    usage:"Frontmatter phase for published tasks; empty = omit field"`
	MaxTitleLen     int    `required:"true"  arg:"max-title-len"     env:"MAX_TITLE_LEN" usage:"Max length of vault task filename (whole title; safety cap)"                                                                                                                                                          default:"200"`
	TaskSuffix      string `required:"false" arg:"task-suffix"       env:"TASK_SUFFIX"   usage:"Optional suffix appended to build-failure task filenames as ' - suffix'; empty = no suffix. Use distinct values per stage to prevent task-file collisions when both watchers poll the same repo into the same vault."`

	CreateWatcher WatcherFactory
}

// WatcherFactory creates a Watcher and its cleanup function.
type WatcherFactory func(
	ctx context.Context,
	ghClient pkg.GitHubClient,
	brokers libkafka.Brokers,
	stage string,
	inputAllowlist []string,
	resolved pkg.AllowlistSnapshot,
	cursorPath string,
	assignee string,
	taskStatus string,
	taskPhase string,
	maxTitleLen int,
	taskSuffix string,
) (pkg.Watcher, func(), error)

func (a *Application) Run(ctx context.Context, _ libsentry.Client) error {
	repoAllowlist, err := filter.ParseRepoAllowlist(a.RepoAllowlist)
	if err != nil {
		return errors.Wrap(ctx, err, "parse repo allowlist")
	}
	// Validate ALL entries at startup — aggregate error names every malformed entry.
	if validationErr := repoallowlist.Validate(ctx, repoAllowlist); validationErr != nil {
		return errors.Wrap(ctx, validationErr, "REPO_ALLOWLIST contains malformed entries")
	}
	if len(repoAllowlist) == 0 {
		return errors.Errorf(
			ctx,
			"REPO_ALLOWLIST must be non-empty: set at least one host/owner/repo entry",
		)
	}

	httpClient, err := auth.Resolve(ctx, auth.Config{
		AppID:          a.AppID,
		InstallationID: a.InstallationID,
		PEMKeyFile:     a.PEMKeyFile,
		PEMKey:         a.PEMKey,
		LogPrefix:      "watcher/github-build-run-once",
	})
	if err != nil {
		return errors.Wrap(ctx, err, "resolve auth")
	}
	defer httpClient.CloseIdleConnections()

	ghClient := pkg.NewGitHubClient(httpClient)

	resolved, refreshTask, err := factory.CreateAllowlistSnapshot(ghClient, repoAllowlist)
	if err != nil {
		return errors.Wrap(ctx, err, "create allowlist snapshot")
	}
	// For run-once, we call Refresh synchronously instead of using the background refresh task.
	if refreshTask != nil {
		resolvedSet, ok := resolved.(*wildcard.ResolvedAllowlist)
		if !ok {
			return errors.Errorf(ctx, "expected *ResolvedAllowlist when refreshTask is non-nil")
		}
		if err := resolvedSet.Refresh(ctx); err != nil {
			glog.Warningf("initial wildcard refresh failed: %v", err)
		}
	}

	w, cleanup, err := a.CreateWatcher(
		ctx,
		ghClient,
		a.KafkaBrokers,
		a.Stage,
		repoAllowlist,
		resolved,
		"/data/cursor.json",
		a.BuildAssignee,
		a.BuildTaskStatus,
		a.BuildTaskPhase,
		a.MaxTitleLen,
		a.TaskSuffix,
	)
	if err != nil {
		return errors.Wrap(ctx, err, "create watcher")
	}
	defer cleanup()

	return w.Poll(ctx)
}
