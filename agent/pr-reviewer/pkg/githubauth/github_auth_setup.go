// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubauth

import (
	"context"
	"os/exec"

	"github.com/bborbe/errors"
	"github.com/golang/glog"
)

// Configurator configures git credential helpers for GitHub at pod startup.
// The pod implementation invokes `gh auth setup-git`; the local-CLI noop
// returns nil without touching any config file.
//
//counterfeiter:generate -o ../../mocks/github-auth-setup.go --fake-name GitHubAuthSetup . Configurator
type Configurator interface {
	Setup(ctx context.Context) error
}

// NewGhAuthSetupGit returns a Configurator that invokes `gh auth setup-git`
// when ghToken is non-empty. When ghToken is empty the Setup call is a no-op
// so pods that target non-GitHub hosts start cleanly without a token.
func NewGhAuthSetupGit(ghToken string) Configurator {
	return &ghAuthSetupGit{
		ghToken:  ghToken,
		execFunc: defaultExecFunc,
	}
}

// ghAuthSetupGit is the real implementation; execFunc is injectable for testing.
type ghAuthSetupGit struct {
	ghToken  string
	execFunc func(ctx context.Context, name string, args ...string) error
}

func (g *ghAuthSetupGit) Setup(ctx context.Context) error {
	if g.ghToken == "" {
		glog.V(2).Infof("github-auth-setup: GH_TOKEN not set, skipping gh auth setup-git")
		return nil
	}
	glog.V(2).Infof("github-auth-setup: running gh auth setup-git")
	if err := g.execFunc(ctx, "gh", "auth", "setup-git"); err != nil {
		// Intentionally discarding underlying error: gh output may contain GH_TOKEN value.
		// The exec func already sanitizes its output (defaultExecFunc uses errors.Errorf).
		// This defense-in-depth ensures injected test funcs can't leak secrets either.
		glog.V(4).Infof("github-auth-setup: gh auth setup-git error (detail suppressed): %T", err)
		return errors.Errorf(ctx, "gh auth setup-git failed")
	}
	glog.V(2).Infof("github-auth-setup: gh auth setup-git complete")
	return nil
}

// defaultExecFunc is the production exec.CommandContext wrapper.
func defaultExecFunc(ctx context.Context, name string, args ...string) error {
	// #nosec G204 -- binary is hardcoded "gh" and args are hardcoded ["auth", "setup-git"]; no user input
	cmd := exec.CommandContext(ctx, name, args...)
	if _, err := cmd.CombinedOutput(); err != nil {
		// out intentionally omitted: gh may print messages including the token; safer to drop entirely than risk leak
		return errors.Errorf(ctx, "%s %v failed", name, args)
	}
	return nil
}

// NewNoopAuthSetup returns a Configurator that always returns nil.
// Used by cmd/run-task so the developer's existing gh auth login continues
// to handle credentials; ~/.gitconfig is never mutated by the agent.
func NewNoopAuthSetup() Configurator {
	return &noopAuthSetup{}
}

type noopAuthSetup struct{}

func (n *noopAuthSetup) Setup(_ context.Context) error { return nil }

// NewGhAuthSetupGitWithExecFunc constructs a GhAuthSetupGit with an injected
// exec function for testing. Do not use in production code.
func NewGhAuthSetupGitWithExecFunc(
	ghToken string,
	execFunc func(ctx context.Context, name string, args ...string) error,
) Configurator {
	return &ghAuthSetupGit{ghToken: ghToken, execFunc: execFunc}
}
