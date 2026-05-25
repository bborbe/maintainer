// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command maintainer-agent-pr-reviewer is the Kafka entry point for the PR-review
// agent — spawned as a K8s Job by task/executor with TASK_CONTENT +
// TASK_ID + PHASE + KAFKA_BROKERS env. For local CLI mode (file-based),
// see cmd/run-task/main.go.
package main

import (
	"context"
	"os"
	"time"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	delivery "github.com/bborbe/agent/lib/delivery"
	libmetrics "github.com/bborbe/agent/lib/metrics"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	libtime "github.com/bborbe/time"
	"github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"

	prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/factory"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubauth"
	githubapp "github.com/bborbe/maintainer/lib/githubapp"
	repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"
)

const agentName = "pr-reviewer-agent"

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	// Claude Code CLI configuration
	ClaudeConfigDir claudelib.ClaudeConfigDir `required:"false" arg:"claude-config-dir" env:"CLAUDE_CONFIG_DIR" usage:"Claude Code config directory" default:"~/.claude"`

	// Agent directory (contains .claude/ with CLAUDE.md and commands)
	AgentDir claudelib.AgentDir `required:"false" arg:"agent-dir" env:"AGENT_DIR" usage:"Agent directory with .claude/ config" default:"agent"`

	// Workdir paths for bare-clone cache and per-task worktrees
	ReposPath string `required:"false" arg:"repos-path" env:"REPOS_PATH" usage:"Root path for bare-clone cache"   default:"/repos"`
	WorkPath  string `required:"false" arg:"work-path"  env:"WORK_PATH"  usage:"Root path for per-task worktrees" default:"/work"`

	// Review depth passed to /coding:pr-review (short | standard | full)
	ReviewMode string `required:"false" arg:"review-mode" env:"REVIEW_MODE" usage:"Review depth: short | standard | full" default:"standard"`

	// Task content from agent pipeline
	TaskContent string `required:"true" arg:"task-content" env:"TASK_CONTENT" usage:"Raw task markdown from vault"`

	// Branch for Kafka result delivery
	Branch base.Branch `required:"true" arg:"branch" env:"BRANCH" usage:"branch"`

	// Phase to run (framework requires explicit phase)
	Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | execution | ai_review" default:"execution"`

	// Kafka delivery (optional — only active when TASK_ID is set)
	KafkaBrokers libkafka.Brokers        `required:"false" arg:"kafka-brokers" env:"KAFKA_BROKERS" usage:"Comma separated list of Kafka brokers"`
	TaskID       agentlib.TaskIdentifier `required:"false" arg:"task-id"       env:"TASK_ID"       usage:"Agent task identifier for publishing results back to task controller"`

	// GitHub token forwarded to the Claude CLI subprocess as GH_TOKEN for gh auth.
	// Also used by the real GitHubAuthSetup to configure git credential helper at pod startup.
	GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub token for gh CLI auth and git credential helper at pod startup" display:"length"`

	// GitHub App authentication. When AppID + InstallationID + PEMKeyFile are
	// set, the pod mints an installation access token at startup and uses it
	// in place of GHToken; the legacy GHToken env stays accepted as a fallback
	// (see Run() for the resolution order).
	AppID          int64  `required:"false" arg:"app-id"          env:"APP_ID"           usage:"GitHub App ID (numeric); when set, App auth is used instead of GH_TOKEN"`
	InstallationID int64  `required:"false" arg:"installation-id" env:"INSTALLATION_ID"  usage:"GitHub App Installation ID (numeric)"`
	PEMKeyFile     string `required:"false" arg:"pem-key-file"    env:"PEM_KEY_FILE"     usage:"Path to the GitHub App private key (PEM file mounted from k8s Secret)"`
	PEMKey         string `required:"false" arg:"pem-key"         env:"PEM_KEY"          usage:"GitHub App private key (PEM) as env var content; mutually exclusive with PEM_KEY_FILE" display:"length"`
	BotLogin       string `required:"false" arg:"bot-login"       env:"BOT_GITHUB_LOGIN" usage:"Bot identity used by githubposter (e.g. ben-s-pull-request-reviewer[bot])"                              default:"ben-s-pull-request-reviewer[bot]"`

	// Anthropic-compatible provider routing. Setting AnthropicBaseURL + AnthropicAuthToken
	// routes the claude CLI to an alt-provider (e.g. MiniMax via https://api.minimax.io/anthropic).
	// AnthropicModel drives both the `--model` CLI flag and the ANTHROPIC_MODEL env var seen by
	// the claude subprocess.
	AnthropicBaseURL   string                `required:"false" arg:"anthropic-base-url"   env:"ANTHROPIC_BASE_URL"   usage:"Anthropic-compatible API base URL"`
	AnthropicAuthToken string                `required:"false" arg:"anthropic-auth-token" env:"ANTHROPIC_AUTH_TOKEN" usage:"Bearer token for ANTHROPIC_BASE_URL"                                  display:"length"`
	AnthropicModel     claudelib.ClaudeModel `required:"false" arg:"anthropic-model"      env:"ANTHROPIC_MODEL"      usage:"Model name; also exposed to the claude subprocess as ANTHROPIC_MODEL"                  default:"sonnet"`

	// Repo allowlist — comma-separated host/owner/repo entries; empty means allow-all.
	RepoAllowlist string `required:"false" arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); empty means allow-all"`

	PushgatewayURL string `required:"false" arg:"pushgateway-url" env:"PUSHGATEWAY_URL" usage:"Prometheus PushGateway URL"          default:"http://pushgateway:9090"`
	TaskType       string `required:"false" arg:"task-type"       env:"TASK_TYPE"       usage:"Task type label for metric grouping" default:"unknown"`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	registry := prometheus.NewRegistry()
	jobMetrics := libmetrics.NewJobMetrics(registry, libtime.NewCurrentDateTime())
	pusher := push.New(a.PushgatewayURL, libmetrics.BuildJobMetricsName(agentName)).
		Grouping("agent", agentName).
		Grouping("task_type", a.TaskType).
		Collector(registry)
	defer func() {
		if err := pusher.PushContext(ctx); err != nil {
			glog.Warningf("prometheus push failed: %v", err)
			return
		}
		glog.V(2).Infof("prometheus push completed")
	}()
	start := libtime.NewCurrentDateTime().Now().Time()
	glog.V(2).Infof("maintainer-agent-pr-reviewer started phase=%s", a.Phase)

	if err := a.resolveAuth(ctx); err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return err
	}

	repoAllowlist, err := prpkg.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return err
	}
	// Warn on malformed entries; allow-all and wildcard semantics handled by IsAllowed at match time.
	if validationErr := repoallowlist.Validate(ctx, repoAllowlist); validationErr != nil {
		glog.Warningf(
			"REPO_ALLOWLIST contains malformed entries (will be ignored at match time): %v",
			validationErr,
		)
	}
	glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))

	deliverer, cleanup, err := a.createDeliverer(ctx)
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return err
	}
	defer cleanup()

	agent, err := a.dispatchAgent(ctx, repoAllowlist)
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Wrap(ctx, err, "task type dispatch")
	}
	result, err := factory.RunAgent(ctx, factory.RunConfig{
		ClaudeConfigDir:    a.ClaudeConfigDir,
		AgentDir:           a.AgentDir,
		Model:              a.AnthropicModel,
		GHToken:            a.GHToken,
		AnthropicBaseURL:   a.AnthropicBaseURL,
		AnthropicAuthToken: a.AnthropicAuthToken,
		ReposPath:          a.ReposPath,
		WorkPath:           a.WorkPath,
		ReviewMode:         a.ReviewMode,
		RepoAllowlist:      repoAllowlist,
		AuthSetup:          githubauth.NewGhAuthSetupGit(a.GHToken),
		Phase:              a.Phase,
		BotLogin:           a.BotLogin,
		TaskContent:        a.TaskContent,
		Deliverer:          deliverer,
		Agent:              agent,
		CurrentDateTime:    libtime.NewCurrentDateTime(),
	})
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Wrap(ctx, err, "agent run failed")
	}
	jobMetrics.RecordRun(result.Status)
	jobMetrics.RecordDuration(time.Since(start))
	return agentlib.PrintResult(ctx, result)
}

// dispatchAgent builds the correct agent for the configured task type.
func (a *application) dispatchAgent(
	ctx context.Context,
	repoAllowlist []string,
) (*agentlib.Agent, error) {
	env := map[string]string{}
	if a.GHToken != "" {
		env["GH_TOKEN"] = a.GHToken
	}
	if a.AnthropicBaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = a.AnthropicBaseURL
	}
	if a.AnthropicAuthToken != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = a.AnthropicAuthToken
	}
	if a.AnthropicModel != "" {
		env["ANTHROPIC_MODEL"] = a.AnthropicModel.String()
	}
	if a.BotLogin != "" {
		env["BOT_GITHUB_LOGIN"] = a.BotLogin
	}
	repoManager := git.NewRepoManager(git.WorkdirConfig{
		ReposPath: a.ReposPath,
		WorkPath:  a.WorkPath,
	}, a.GHToken)
	provider := factory.CreateAgentProvider(
		a.ClaudeConfigDir,
		a.AgentDir,
		a.AnthropicModel,
		a.GHToken,
		env,
		repoManager,
		a.ReviewMode,
		repoAllowlist,
		libtime.NewCurrentDateTime(),
	)
	agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))
	if err != nil {
		return nil, errors.Wrap(ctx, err, "select agent for task_type")
	}
	return agent, nil
}

// resolveAuth determines the auth mode and, for GitHub App mode, mints an IAT
// into a.GHToken. The legacy GH_TOKEN env is accepted as a fallback.
func (a *application) resolveAuth(ctx context.Context) error {
	// Determine the effective auth mode: prefer PEMKeyFile (file mount), fall back
	// to PEMKey (env var content) if PEMKeyFile is not set.
	hasPEMFile := a.PEMKeyFile != ""
	hasPEMContent := a.PEMKey != ""
	useGitHubApp := a.AppID != 0 && a.InstallationID != 0 && (hasPEMFile || hasPEMContent)

	if a.GHToken != "" && useGitHubApp {
		glog.Warningf(
			"pr-reviewer auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
		)
	}

	switch {
	case useGitHubApp:
		var iat string
		var err error
		if hasPEMFile {
			iat, err = githubapp.MintIAT(ctx, githubapp.Config{
				AppID:          a.AppID,
				InstallationID: a.InstallationID,
				PEMPath:        a.PEMKeyFile,
			})
		} else {
			iat, err = githubapp.MintIAT(ctx, githubapp.Config{
				AppID:          a.AppID,
				InstallationID: a.InstallationID,
				PEM:            []byte(a.PEMKey),
			})
		}
		if err != nil {
			return errors.Wrap(ctx, err, "mint github app iat")
		}
		a.GHToken = iat
		glog.V(2).Infof(
			"pr-reviewer auth mode=github-app app_id=%d installation_id=%d",
			a.AppID, a.InstallationID,
		)
	case a.GHToken != "":
		glog.Warningf(
			"pr-reviewer auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)",
		)
	default:
		return errors.Errorf(
			ctx,
			"pr-reviewer auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY_FILE (or PEM_KEY), or set GH_TOKEN",
		)
	}
	return nil
}

// createDeliverer builds the Kafka result deliverer when TASK_ID is set,
// otherwise returns a noop deliverer (for local-pod debugging without Kafka).
func (a *application) createDeliverer(
	ctx context.Context,
) (agentlib.ResultDeliverer, func(), error) {
	if a.TaskID == "" {
		glog.V(2).Infof("TASK_ID not set, skipping task result publishing")
		return delivery.NewNoopResultDeliverer(), func() {}, nil
	}
	if len(a.KafkaBrokers) == 0 {
		return nil, nil, errors.Errorf(ctx, "KAFKA_BROKERS must be set when TASK_ID is set")
	}
	currentDateTime := libtime.NewCurrentDateTime()
	deliverer, cleanup, err := factory.CreateDeliverer(
		ctx,
		a.TaskID,
		a.KafkaBrokers,
		a.Branch,
		a.TaskContent,
		currentDateTime,
	)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create deliverer")
	}
	return deliverer, cleanup, nil
}
