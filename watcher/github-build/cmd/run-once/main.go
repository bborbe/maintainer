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

	repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/factory"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
)

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	GHToken       string           `required:"true" arg:"gh-token"       env:"GH_TOKEN"       usage:"GitHub token (read scope sufficient)"                                               display:"length"`
	KafkaBrokers  libkafka.Brokers `required:"true" arg:"kafka-brokers"  env:"KAFKA_BROKERS"  usage:"Comma-separated Kafka broker list"`
	Stage         string           `required:"true" arg:"stage"          env:"STAGE"          usage:"Deployment stage (dev|prod)"`
	RepoAllowlist string           `required:"true" arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); MUST be non-empty"`

	BuildAssignee   string `required:"true"  arg:"build-assignee"    env:"TASK_ASSIGNEE" usage:"Frontmatter assignee for published tasks"                    default:"build-fixer-agent"`
	BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"TASK_STATUS"   usage:"Frontmatter status for published tasks"                      default:"todo"`
	BuildTaskPhase  string `required:"false" arg:"build-task-phase"  env:"TASK_PHASE"    usage:"Frontmatter phase for published tasks; empty = omit field"`
	MaxTitleLen     int    `required:"true"  arg:"max-title-len"     env:"MAX_TITLE_LEN" usage:"Max length of vault task filename (whole title; safety cap)" default:"200"`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		return err
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

	w, cleanup, err := factory.CreateWatcher(
		ctx,
		a.GHToken,
		a.KafkaBrokers,
		a.Stage,
		repoAllowlist,
		"/data/cursor.json",
		a.BuildAssignee,
		a.BuildTaskStatus,
		a.BuildTaskPhase,
		a.MaxTitleLen,
	)
	if err != nil {
		return errors.Wrap(ctx, err, "create watcher")
	}
	defer cleanup()

	return w.Poll(ctx)
}
