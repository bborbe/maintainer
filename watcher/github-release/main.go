// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command maintainer-watcher-github-release polls a configured GitHub owner for
// repos with non-empty ## Unreleased in CHANGELOG.md and publishes one
// CreateTaskCommand to Kafka per affected repo so github-releaser-agent picks
// it up automatically.
//
// See [[Build github-release watcher]] for scope + DoD; [[Watcher Writing Guide]]
// for the producer-side contract; [[Agent Task File Contract]] for the
// frontmatter/body shape this watcher emits.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/run"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bborbe/maintainer/watcher/github-release/pkg"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/factory"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	Listen         string           `required:"false" arg:"listen"          env:"LISTEN"          usage:"HTTP listen address (healthz/readiness/metrics)"                                                 default:":9090"`
	Stage          string           `required:"true"  arg:"stage"           env:"STAGE"           usage:"Deployment stage (dev|prod)"`
	Owner          string           `required:"true"  arg:"owner"           env:"OWNER"           usage:"GitHub owner / org to scan (e.g. bborbe)"`
	RepoAllowlist  string           `required:"false" arg:"repo-allowlist"  env:"REPO_ALLOWLIST"  usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); empty = allow-all within owner"`
	PollInterval   string           `required:"false" arg:"poll-interval"   env:"POLL_INTERVAL"   usage:"Poll interval (Go duration)"                                                                     default:"10m"`
	CursorPath     string           `required:"false" arg:"cursor-path"     env:"CURSOR_PATH"     usage:"Cursor persistence path (mount a PVC)"                                                           default:"/data/cursor.json"`
	KafkaBrokers   libkafka.Brokers `required:"true"  arg:"kafka-brokers"   env:"KAFKA_BROKERS"   usage:"Comma-separated Kafka broker list"`
	AppID          int64            `required:"false" arg:"app-id"          env:"APP_ID"          usage:"GitHub App ID (preferred auth path)"`
	InstallationID int64            `required:"false" arg:"installation-id" env:"INSTALLATION_ID" usage:"GitHub App Installation ID"`
	PEMKey         string           `required:"false" arg:"pem-key"         env:"PEM_KEY"         usage:"GitHub App PEM key (populated from k8s Secret)"                                                                              display:"length"`
	GHToken        string           `required:"false" arg:"gh-token"        env:"GH_TOKEN"        usage:"Legacy PAT fallback (prefer APP_ID + INSTALLATION_ID + PEM_KEY)"                                                             display:"length"`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	pollInterval, err := time.ParseDuration(a.PollInterval)
	if err != nil {
		return errors.Wrapf(ctx, err, "parse poll interval %q", a.PollInterval)
	}

	allowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		return errors.Wrap(ctx, err, "parse repo allowlist")
	}
	if len(allowlist) == 0 {
		glog.V(2).Infof("repo-allowlist count=0 (allow-all within owner=%s)", a.Owner)
	} else {
		glog.V(2).Infof("repo-allowlist count=%d", len(allowlist))
	}

	httpClient, err := a.resolveAuth(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "resolve auth")
	}
	defer httpClient.CloseIdleConnections()

	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx, a.KafkaBrokers, "maintainer-watcher-github-release",
	)
	if err != nil {
		return errors.Wrap(ctx, err, "create sync producer")
	}
	defer func() {
		if cerr := syncProducer.Close(); cerr != nil {
			glog.Warningf("close kafka sync producer: %v", cerr)
		}
	}()

	metrics := pkg.NewMetrics()

	// Cycle-invariant filters. SHAUnchangedFilter is composed in by the watcher
	// each poll (it needs a fresh CursorReader per cycle).
	staticFilters := filter.TaskCreationFilters{
		filter.NewRepoAllowlistFilter(allowlist),
		filter.NewEmptyUnreleasedFilter(),
		filter.NewAutoReleaseFilter(),
	}

	w := factory.CreateWatcher(
		httpClient,
		syncProducer,
		base.Branch(a.Stage),
		a.CursorPath,
		a.Owner,
		staticFilters,
		metrics,
		allowlist,
		a.Stage,
	)

	glog.V(2).Infof(
		"maintainer-watcher-github-release starting stage=%s owner=%s interval=%s listen=%s",
		a.Stage, a.Owner, pollInterval, a.Listen,
	)

	return run.CancelOnFirstFinish(ctx,
		a.pollLoop(w, pollInterval),
		a.runHTTPServer(),
	)
}

// runHTTPServer serves the mandatory triple (/healthz, /readiness, /metrics)
// per coding-guidelines/go-k8s-binary-conventions.md. Without these, k8s
// probes can never pass and Prometheus never scrapes — silent operational
// failure.
func (a *application) runHTTPServer() run.Func {
	return func(ctx context.Context) error {
		router := mux.NewRouter()
		router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/metrics").Handler(promhttp.Handler())
		glog.V(2).Infof("http server listening on %s", a.Listen)
		return libhttp.NewServer(a.Listen, router).Run(ctx)
	}
}

func (a *application) pollLoop(w pkg.Watcher, interval time.Duration) run.Func {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// Fire one cycle immediately on start, then on each tick.
		if err := w.Poll(ctx); err != nil {
			glog.Errorf("initial poll: %v", err)
		}
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if err := w.Poll(ctx); err != nil {
					glog.Errorf("poll: %v", err)
				}
			}
		}
	}
}

// resolveAuth chooses GitHub App auth (preferred) over PAT.
// Mirrors watcher/github-pr/main.go resolveAuth shape; reads from the parsed
// argument struct (populated by argument.Parse) instead of os.Getenv directly
// so the framework's defaults / validation / display-mode-length-redaction all
// apply uniformly.
func (a *application) resolveAuth(ctx context.Context) (*http.Client, error) {
	pemKey := []byte(a.PEMKey)

	appPartial := (a.AppID != 0) || (a.InstallationID != 0) || (len(pemKey) != 0)
	appComplete := (a.AppID != 0) && (a.InstallationID != 0) && (len(pemKey) != 0)
	if appPartial && !appComplete {
		var missing []string
		if a.AppID == 0 {
			missing = append(missing, "APP_ID")
		}
		if a.InstallationID == 0 {
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
		if a.GHToken != "" {
			glog.Warningf(
				"watcher auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
			)
		}
		glog.Infof(
			"watcher auth mode=github-app app_id=%d installation_id=%d",
			a.AppID,
			a.InstallationID,
		)
		return factory.CreateGitHubAppClient(ctx, a.AppID, a.InstallationID, pemKey)
	}
	if a.GHToken != "" {
		glog.Warningf("watcher auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
		return factory.CreateGitHubPATClient(ctx, a.GHToken), nil
	}
	return nil, errors.Errorf(
		ctx,
		"watcher auth: neither App nor PAT configured — set APP_ID + INSTALLATION_ID + PEM_KEY, or set GH_TOKEN",
	)
}
