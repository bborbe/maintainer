// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command maintainer-watcher-github-pr polls GitHub for open pull requests in
// configured repos and publishes a CreateTaskCommand to Kafka per new
// PR so the existing pr-reviewer agent picks it up automatically.
package main

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	"github.com/bborbe/run"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/factory"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

var repoScopePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func validateLengthCaps(ctx context.Context, maxSlugLen, maxTitleLen int) error {
	if maxSlugLen <= 0 {
		return errors.Errorf(ctx, "MAX_SLUG_LEN must be > 0; got %d", maxSlugLen)
	}
	if maxTitleLen <= 0 {
		return errors.Errorf(ctx, "MAX_TITLE_LEN must be > 0; got %d", maxTitleLen)
	}
	if maxSlugLen >= maxTitleLen {
		return errors.Errorf(
			ctx,
			"MAX_SLUG_LEN (%d) must be < MAX_TITLE_LEN (%d)",
			maxSlugLen,
			maxTitleLen,
		)
	}
	return nil
}

func validateRepoScope(ctx context.Context, scope string) error {
	if !repoScopePattern.MatchString(scope) {
		return errors.Errorf(ctx, "repo scope %q must match ^[a-zA-Z0-9_.-]+$", scope)
	}
	return nil
}

// parseMaxPRAge parses raw as a libtime.Duration. Empty string returns 0 (disabled).
// Negative values are rejected.
func parseMaxPRAge(ctx context.Context, raw string) (libtime.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	parsed, err := libtime.ParseDuration(ctx, raw)
	if err != nil {
		return 0, errors.Wrapf(ctx, err, "parse MAX_PR_AGE")
	}
	if parsed != nil && *parsed < 0 {
		return 0, errors.Errorf(ctx, "MAX_PR_AGE must not be negative, got %s", *parsed)
	}
	if parsed == nil {
		return 0, nil
	}
	return *parsed, nil
}

// parseBackfillDuration parses raw as a libtime.Duration. Empty string returns 0 (disabled).
// Negative values are rejected.
func parseBackfillDuration(ctx context.Context, raw string) (libtime.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	parsed, err := libtime.ParseDuration(ctx, raw)
	if err != nil {
		return 0, errors.Wrapf(ctx, err, "parse BACKFILL_DURATION")
	}
	if parsed != nil && *parsed < 0 {
		return 0, errors.Errorf(ctx, "BACKFILL_DURATION must not be negative, got %s", *parsed)
	}
	if parsed == nil {
		return 0, nil
	}
	return *parsed, nil
}

func getEnvInt(name string) int64 {
	v, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	Listen           string           `required:"false" arg:"listen"            env:"LISTEN"            usage:"HTTP listen address (healthz/readiness/metrics)"                                                                                                                                                           default:":9090"`
	GHToken          string           `required:"false" arg:"gh-token"          env:"GH_TOKEN"          usage:"GitHub PAT (legacy fallback when App credentials are not set)"                                                                                                                                                                                     display:"length"`
	AppID            int64            `required:"false" arg:"app-id"            env:"APP_ID"            usage:"GitHub App ID (numeric); when set with InstallationID + PEMKey, App auth is used instead of GH_TOKEN"`
	InstallationID   int64            `required:"false" arg:"installation-id"   env:"INSTALLATION_ID"   usage:"GitHub App Installation ID (numeric)"`
	PEMKey           string           `required:"false" arg:"pem-key"           env:"PEM_KEY"           usage:"GitHub App private key (PEM content from k8s Secret envFrom)"                                                                                                                                                                                      display:"length"`
	KafkaBrokers     libkafka.Brokers `required:"true"  arg:"kafka-brokers"     env:"KAFKA_BROKERS"     usage:"Comma-separated Kafka broker list"`
	Stage            string           `required:"true"  arg:"stage"             env:"STAGE"             usage:"Deployment stage (dev|prod)"`
	PollInterval     string           `required:"false" arg:"poll-interval"     env:"POLL_INTERVAL"     usage:"Poll interval (Go duration)"                                                                                                                                                                               default:"5m"`
	RepoScope        string           `required:"false" arg:"repo-scope"        env:"REPO_SCOPE"        usage:"GitHub user/org scope"                                                                                                                                                                                     default:"bborbe"`
	BotAllowlist     string           `required:"false" arg:"bot-allowlist"     env:"BOT_ALLOWLIST"     usage:"Comma-separated bot author allowlist"                                                                                                                                                                      default:"dependabot[bot],renovate[bot]"`
	TrustedAuthors   string           `required:"false" arg:"trusted-authors"   env:"TRUSTED_AUTHORS"   usage:"Comma-separated trusted GitHub author logins (required; empty list refuses startup)"`
	MaxPRAge         string           `required:"false" arg:"max-pr-age"        env:"MAX_PR_AGE"        usage:"Skip PRs older than this (Go duration; empty disables)"                                                                                                                                                    default:"2160h"`
	BackfillDuration string           `required:"false" arg:"backfill-duration" env:"BACKFILL_DURATION" usage:"On cold start, backdate the initial cursor by this duration (Go duration; empty disables)"                                                                                                                 default:"720h"`
	RepoAllowlist    string           `required:"false" arg:"repo-allowlist"    env:"REPO_ALLOWLIST"    usage:"Comma-separated host-qualified repo allowlist (host/owner/repo format); empty means allow-all"`
	MaxSlugLen       int              `required:"false" arg:"max-slug-len"      env:"MAX_SLUG_LEN"      usage:"Max length of slugified PR-title segment in vault filenames"                                                                                                                                               default:"80"`
	MaxTitleLen      int              `required:"false" arg:"max-title-len"     env:"MAX_TITLE_LEN"     usage:"Max length of vault task filename (whole title; safety cap)"                                                                                                                                               default:"200"`
	TaskSuffix       string           `required:"false" arg:"task-suffix"       env:"TASK_SUFFIX"       usage:"Optional suffix appended to PR task filenames as ' - suffix'; empty = no suffix. Use distinct values per stage to prevent task-file collisions when both watchers poll the same repo into the same vault."`

	TriggerHandler http.Handler
}

// resolveAuth determines the GitHub auth mode from environment variables and returns
// an authenticated *http.Client. App auth wins when APP_ID + INSTALLATION_ID + PEM_KEY
// are all set; otherwise GH_TOKEN PAT fallback is used.
func (a *application) resolveAuth(ctx context.Context) (*http.Client, error) {
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

func (a *application) validateConfig(ctx context.Context) error {
	if err := validateRepoScope(ctx, a.RepoScope); err != nil {
		return err
	}
	return validateLengthCaps(ctx, a.MaxSlugLen, a.MaxTitleLen)
}

//nolint:funlen // wires Run from validated config — extracting any chunk hurts readability without reducing complexity. 82 lines, 2 over the 80-line cap.
func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	if err := a.validateConfig(ctx); err != nil {
		return err
	}

	pollInterval, err := time.ParseDuration(a.PollInterval)
	if err != nil {
		return errors.Wrapf(ctx, err, "parse poll interval %q", a.PollInterval)
	}

	botAllowlist := pkg.ParseBotAllowlist(a.BotAllowlist)
	startTime := libtime.NewCurrentDateTime().Now()

	maxAge, err := parseMaxPRAge(ctx, a.MaxPRAge)
	if err != nil {
		return err
	}

	backfillDuration, err := parseBackfillDuration(ctx, a.BackfillDuration)
	if err != nil {
		return err
	}
	if backfillDuration > 0 {
		startTime = startTime.Add(-backfillDuration)
		glog.V(2).
			Infof("cursor cold-start backfilled by %s; initial=%s", backfillDuration, startTime)
	}

	repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		return err
	}
	if validationErr := repoallowlist.Validate(ctx, repoAllowlist); validationErr != nil {
		glog.Warningf("repo-allowlist: malformed entries ignored at match time: %v", validationErr)
	}
	if len(repoAllowlist) == 0 {
		glog.V(2).Infof("repo-allowlist count=0 (allow-all)")
	} else {
		glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))
	}
	taskCreationFilter := filter.TaskCreationFilters{
		filter.NewDraftFilter(),
		filter.NewBotAuthorFilter(botAllowlist),
		filter.NewWIPTitleFilter(),
		filter.NewAgeFilter(maxAge, startTime),
		filter.NewRepoAllowlistFilter(repoAllowlist),
	}

	trustedAuthors := pkg.ParseTrustedAuthors(a.TrustedAuthors)
	if len(trustedAuthors) == 0 {
		return errors.Errorf(
			ctx,
			"no trusted authors configured: set TRUSTED_AUTHORS to a comma-separated list of GitHub logins",
		)
	}
	glog.V(2).Infof("trusted-authors count=%d", len(trustedAuthors))

	branch := base.Branch(a.Stage)

	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx,
		a.KafkaBrokers,
		"maintainer-watcher-github-pr",
	)
	if err != nil {
		return errors.Wrap(ctx, err, "create sync producer")
	}
	defer func() {
		if err := syncProducer.Close(); err != nil {
			glog.Warningf("close kafka sync producer: %v", err)
		}
	}()
	createSender := factory.CreateKafkaSender(syncProducer, branch)

	trustDecision := trust.And{trust.NewAuthorAllowlist(trustedAuthors)}

	httpClient, err := a.resolveAuth(ctx)
	if err != nil {
		return err
	}

	w := factory.CreateWatcher(
		httpClient,
		createSender,
		pkg.DefaultCursorPath,
		startTime,
		a.RepoScope,
		taskCreationFilter,
		a.Stage,
		pkg.NewMetrics(),
		trustDecision,
		a.MaxSlugLen,
		a.MaxTitleLen,
		a.TaskSuffix,
	)

	triggerHandler := factory.CreateSinglePRHandler(
		httpClient,
		createSender,
		taskCreationFilter,
		trustDecision,
		a.Stage,
		a.MaxSlugLen,
		a.MaxTitleLen,
		a.TaskSuffix,
	)
	a.TriggerHandler = triggerHandler

	glog.V(2).
		Infof("maintainer-watcher-github-pr starting stage=%s scope=%s interval=%s listen=%s", a.Stage, a.RepoScope, a.PollInterval, a.Listen)

	pollOnce := a.pollOnce(w)

	return run.CancelOnFirstFinish(ctx,
		a.runPollLoop(pollOnce, pollInterval),
		a.runHTTPServer(pollOnce),
	)
}

func (a *application) pollOnce(w pkg.Watcher) run.Func {
	return func(ctx context.Context) error {
		glog.V(2).Infof("poll cycle start stage=%s", a.Stage)
		return w.Poll(ctx)
	}
}

func (a *application) runPollLoop(poll run.Func, interval time.Duration) run.Func {
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
			}
		}
	}
}

func (a *application) runHTTPServer(poll run.Func) run.Func {
	return func(ctx context.Context) error {
		router := mux.NewRouter()
		router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/metrics").Handler(promhttp.Handler())
		router.Path("/setloglevel/{level}").
			Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
		router.Path("/check").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
		router.Path("/trigger").Handler(a.TriggerHandler)
		glog.V(2).Infof("http server listening on %s", a.Listen)
		return libhttp.NewServer(a.Listen, router).Run(ctx)
	}
}
