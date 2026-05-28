// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command maintainer-watcher-github-release-run-once runs a single GitHub release
// poll cycle then exits. Intended for local smoke-testing against a real repo.
// No HTTP server, no poll loop.
package main

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/watcher/github-release/pkg"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/factory"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
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

	Stage          string           `required:"true"  arg:"stage"           env:"STAGE"           usage:"Deployment stage (dev|prod)"`
	Owner          string           `required:"true"  arg:"owner"           env:"OWNER"           usage:"GitHub owner / org to scan (e.g. bborbe)"`
	RepoAllowlist  string           `required:"false" arg:"repo-allowlist"  env:"REPO_ALLOWLIST"  usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); empty = allow-all within owner"`
	CursorPath     string           `required:"false" arg:"cursor-path"     env:"CURSOR_PATH"     usage:"Cursor persistence path"                                                                              default:"/data/cursor.json"`
	KafkaBrokers   libkafka.Brokers `required:"true"  arg:"kafka-brokers"   env:"KAFKA_BROKERS"   usage:"Comma-separated Kafka broker list"`
	AppID          int64            `required:"false" arg:"app-id"          env:"APP_ID"          usage:"GitHub App ID (preferred auth path)"`
	InstallationID int64            `required:"false" arg:"installation-id" env:"INSTALLATION_ID" usage:"GitHub App Installation ID"`
	PEMKey         string           `required:"false" arg:"pem-key"         env:"PEM_KEY"         usage:"GitHub App PEM key (populated from k8s Secret)"                                                                              display:"length"`
	GHToken        string           `required:"false" arg:"gh-token"        env:"GH_TOKEN"        usage:"Legacy PAT fallback (prefer APP_ID + INSTALLATION_ID + PEM_KEY)"                                                             display:"length"`

	CreateWatcher WatcherFactory
}

// WatcherFactory creates a Watcher.
type WatcherFactory func(
	ctx context.Context,
	httpClient *http.Client,
	brokers libkafka.Brokers,
	cursorPath string,
	owner string,
	taskCreationFilter filter.TaskCreationFilter,
	stage string,
	metrics pkg.Metrics,
	allowlist []string,
) (pkg.Watcher, func(), error)

func (a *Application) Run(ctx context.Context, _ libsentry.Client) error {
	allowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		return errors.Wrap(ctx, err, "parse repo allowlist")
	}
	if a.RepoAllowlist == "" {
		return errors.Errorf(
			ctx,
			"REPO_ALLOWLIST must be non-empty: set at least one host/owner/repo entry",
		)
	}
	if len(allowlist) == 0 {
		glog.V(2).Infof("repo-allowlist count=0 (allow-all within owner=%s)", a.Owner)
	} else {
		glog.V(2).Infof("repo-allowlist count=%d", len(allowlist))
	}

	// keep in sync with watcher/github-release/main.go resolveAuth
	httpClient, err := a.resolveAuth(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "resolve auth")
	}

	metrics := pkg.NewMetrics()

	staticFilters := filter.TaskCreationFilters{
		filter.NewRepoAllowlistFilter(allowlist),
		filter.NewEmptyUnreleasedFilter(),
		filter.NewAutoReleaseFilter(),
	}

	w, cleanup, err := a.CreateWatcher(
		ctx,
		httpClient,
		a.KafkaBrokers,
		a.CursorPath,
		a.Owner,
		staticFilters,
		a.Stage,
		metrics,
		allowlist,
	)
	if err != nil {
		return errors.Wrap(ctx, err, "create watcher")
	}
	defer cleanup()

	if err := w.Poll(ctx); err != nil {
		return errors.Wrap(ctx, err, "poll failed")
	}
	return nil
}

// resolveAuth chooses GitHub App auth (preferred) over PAT.
// keep in sync with watcher/github-release/main.go resolveAuth
func (a *Application) resolveAuth(ctx context.Context) (*http.Client, error) {
	appID := getEnvInt("APP_ID")
	installationID := getEnvInt("INSTALLATION_ID")
	pemKey := []byte(os.Getenv("PEM_KEY"))
	token := os.Getenv("GH_TOKEN")

	appPartial := (appID != 0) || (installationID != 0) || (len(pemKey) != 0)
	appComplete := (appID != 0) && (installationID != 0) && (len(pemKey) != 0)
	if appPartial && !appComplete {
		var missing []string
		if appID == 0 {
			missing = append(missing, "APP_ID")
		}
		if installationID == 0 {
			missing = append(missing, "INSTALLATION_ID")
		}
		if len(pemKey) == 0 {
			missing = append(missing, "PEM_KEY")
		}
		return nil, errors.Errorf(
			ctx,
			"watcher auth: partial GitHub App config — missing %v; set all three or none",
			missing,
		)
	}

	if appComplete {
		if token != "" {
			glog.Warningf(
				"watcher auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
			)
		}
		glog.Infof(
			"watcher auth mode=github-app app_id=%d installation_id=%d",
			appID,
			installationID,
		)
		return factory.CreateGitHubAppClient(ctx, appID, installationID, pemKey)
	}
	if token != "" {
		glog.Warningf("watcher auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
		return factory.CreateGitHubPATClient(ctx, token), nil
	}
	return nil, errors.Errorf(
		ctx,
		"watcher auth: neither App nor PAT configured — set APP_ID + INSTALLATION_ID + PEM_KEY, or set GH_TOKEN",
	)
}

func getEnvInt(name string) int64 {
	v, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
