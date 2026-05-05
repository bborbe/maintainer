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
	"os"

	libkafka "github.com/bborbe/kafka"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	"github.com/golang/glog"
)

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	Listen        string           `required:"false" arg:"listen"         env:"LISTEN"         usage:"HTTP listen address (healthz/readiness/metrics/trigger)"                            default:":9090"`
	GHToken       string           `required:"true"  arg:"gh-token"       env:"GH_TOKEN"       usage:"GitHub token (read scope sufficient)"                                                               display:"length"`
	KafkaBrokers  libkafka.Brokers `required:"true"  arg:"kafka-brokers"  env:"KAFKA_BROKERS"  usage:"Comma-separated Kafka broker list"`
	Stage         string           `required:"true"  arg:"stage"          env:"STAGE"          usage:"Deployment stage (dev|prod)"`
	PollInterval  string           `required:"false" arg:"poll-interval"  env:"POLL_INTERVAL"  usage:"Poll interval (Go duration)"                                                        default:"5m"`
	RepoAllowlist string           `required:"true"  arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); MUST be non-empty"`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	glog.V(2).
		Infof("maintainer-watcher-github-build starting — stub Run; full implementation in prompt 4")
	return nil
}
