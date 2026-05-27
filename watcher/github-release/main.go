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
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/run"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	"github.com/golang/glog"

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

	branch := base.Branch(a.Stage)
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
	createSender := factory.CreateKafkaSender(syncProducer, branch)

	httpClient, err := a.resolveAuth(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "resolve auth")
	}

	metrics := pkg.NewMetrics()

	// Cycle-invariant filters. SHAUnchangedFilter is composed in by the watcher
	// each poll (it needs a fresh CursorReader per cycle).
	staticFilters := filter.TaskCreationFilters{
		filter.NewRepoAllowlistFilter(allowlist),
		filter.NewEmptyUnreleasedFilter(),
		filter.NewAutoReleaseFilter(),
	}

	w := factory.CreateWatcher(
		httpClient, createSender, a.CursorPath, a.Owner, staticFilters, a.Stage, metrics,
	)

	glog.V(2).Infof(
		"maintainer-watcher-github-release starting stage=%s owner=%s interval=%s listen=%s",
		a.Stage, a.Owner, pollInterval, a.Listen,
	)

	// TODO: HTTP server for healthz / metrics; parallel with poll loop via run.CancelOnFirstFinish.
	return run.CancelOnFirstFinish(ctx, a.pollLoop(w, pollInterval))
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
// TODO: implement (mirror watcher/github-pr/main.go resolveAuth).
func (a *application) resolveAuth(ctx context.Context) (*http.Client, error) {
	return nil, errors.New(ctx, "main: resolveAuth not implemented")
}
