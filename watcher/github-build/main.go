// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command maintainer-watcher-github-build polls GitHub Actions for failed
// workflow runs on the default branches of configured repos and publishes
// a CreateTaskCommand per green→red transition so a build-fixer agent can
// pick it up.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	"github.com/bborbe/run"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bborbe/maintainer/lib/repoallowlist"
	"github.com/bborbe/maintainer/watcher/github-build/pkg"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/auth"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/factory"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
)

func validateMaxTitleLen(ctx context.Context, maxTitleLen int) error {
	if maxTitleLen <= 0 {
		return errors.Errorf(ctx, "MAX_TITLE_LEN must be > 0; got %d", maxTitleLen)
	}
	return nil
}

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	Listen         string           `required:"false" arg:"listen"          env:"LISTEN"          usage:"HTTP listen address (healthz/readiness/metrics/trigger)"                               default:":9090"`
	AppID          int64            `required:"false" arg:"app-id"          env:"APP_ID"          usage:"GitHub App ID (numeric); required for App auth"`
	InstallationID int64            `required:"false" arg:"installation-id" env:"INSTALLATION_ID" usage:"GitHub App Installation ID (numeric)"`
	PEMKeyFile     string           `required:"false" arg:"pem-key-file"    env:"PEM_KEY_FILE"    usage:"Path to the GitHub App private key (PEM) mounted from k8s Secret"`
	PEMKey         string           `required:"false" arg:"pem-key"         env:"PEM_KEY"         usage:"GitHub App private key (PEM) as env var content; mutually exclusive with PEM_KEY_FILE"                 display:"length"`
	KafkaBrokers   libkafka.Brokers `required:"true"  arg:"kafka-brokers"   env:"KAFKA_BROKERS"   usage:"Comma-separated Kafka broker list"`
	Stage          string           `required:"true"  arg:"stage"           env:"STAGE"           usage:"Deployment stage (dev|prod)"`
	PollInterval   string           `required:"false" arg:"poll-interval"   env:"POLL_INTERVAL"   usage:"Poll interval (Go duration)"                                                           default:"5m"`
	RepoAllowlist  string           `required:"true"  arg:"repo-allowlist"  env:"REPO_ALLOWLIST"  usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); MUST be non-empty"`

	BuildAssignee   string `required:"true"  arg:"build-assignee"    env:"TASK_ASSIGNEE" usage:"Frontmatter assignee for published tasks"                                                                                                                                                                             default:"build-fixer-agent"`
	BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"TASK_STATUS"   usage:"Frontmatter status for published tasks"                                                                                                                                                                               default:"next"`
	BuildTaskPhase  string `required:"false" arg:"build-task-phase"  env:"TASK_PHASE"    usage:"Frontmatter phase for published tasks; empty = omit field"`
	MaxTitleLen     int    `required:"false" arg:"max-title-len"     env:"MAX_TITLE_LEN" usage:"Max length of vault task filename (whole title; safety cap)"                                                                                                                                                          default:"200"`
	TaskSuffix      string `required:"false" arg:"task-suffix"       env:"TASK_SUFFIX"   usage:"Optional suffix appended to build-failure task filenames as ' - suffix'; empty = no suffix. Use distinct values per stage to prevent task-file collisions when both watchers poll the same repo into the same vault."`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	if err := validateMaxTitleLen(ctx, a.MaxTitleLen); err != nil {
		return err
	}

	pollInterval, err := time.ParseDuration(a.PollInterval)
	if err != nil {
		return errors.Wrapf(ctx, err, "parse poll interval %q", a.PollInterval)
	}

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
	glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))

	httpClient, err := auth.Resolve(ctx, auth.Config{
		AppID:          a.AppID,
		InstallationID: a.InstallationID,
		PEMKeyFile:     a.PEMKeyFile,
		PEMKey:         a.PEMKey,
		LogPrefix:      "watcher/github-build",
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

	w, cleanup, err := factory.CreateWatcher(
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

	glog.V(2).
		Infof("maintainer-watcher-github-build starting stage=%s interval=%s listen=%s", a.Stage, a.PollInterval, a.Listen)

	pollOnce := a.pollOnce(w)

	// trigger is buffered (size 1) so an HTTP /trigger never blocks: while a poll
	// runs, further triggers coalesce into a single pending signal. The poll loop
	// is the sole executor, so polls never overlap (natural single-flight).
	trigger := make(chan struct{}, 1)

	tasks := []run.Func{
		a.runPollLoop(pollOnce, pollInterval, trigger),
		a.createHTTPServer(trigger),
	}
	if refreshTask != nil {
		tasks = append(tasks, refreshTask)
	}
	return run.CancelOnFirstFinish(ctx, tasks...)
}

func (a *application) pollOnce(w pkg.Watcher) run.Func {
	return func(ctx context.Context) error {
		glog.V(2).Infof("poll cycle start stage=%s", a.Stage)
		return w.Poll(ctx)
	}
}

func (a *application) runPollLoop(
	poll run.Func,
	interval time.Duration,
	trigger <-chan struct{},
) run.Func {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				glog.V(2).Infof("poll loop: context cancelled, exiting cleanly")
				return nil
			case <-ticker.C:
				if err := poll(ctx); err != nil {
					glog.Errorf("poll cycle error: %v", err)
				}
			case <-trigger:
				glog.V(2).Infof("poll loop: triggered via HTTP")
				if err := poll(ctx); err != nil {
					glog.Errorf("poll cycle error: %v", err)
				}
			}
		}
	}
}

func (a *application) createHTTPServer(trigger chan<- struct{}) run.Func {
	return func(ctx context.Context) error {
		router := mux.NewRouter()
		router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/metrics").Handler(promhttp.Handler())
		router.Path("/setloglevel/{level}").
			Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
		router.Path("/resetcursor/{repo:.+}").
			Handler(libhttp.NewDangerousHandlerWrapper(pkg.NewResetCursorHandler(pkg.DefaultCursorPath)))
		router.Path("/trigger").HandlerFunc(newTriggerHandler(trigger))
		glog.V(2).Infof("http server listening on %s", a.Listen)
		return libhttp.NewServer(a.Listen, router).Run(ctx)
	}
}

// newTriggerHandler returns the /trigger HTTP handler. The send is non-blocking:
// while a poll runs the buffer is full, so additional triggers coalesce into the
// single pending signal rather than queueing or blocking the request.
func newTriggerHandler(trigger chan<- struct{}) http.HandlerFunc {
	return func(resp http.ResponseWriter, _ *http.Request) {
		select {
		case trigger <- struct{}{}:
			glog.V(2).Infof("trigger fired via HTTP")
		default:
			glog.V(2).Infof("trigger already pending, skipped")
		}
		_, _ = resp.Write([]byte("trigger fired"))
	}
}
