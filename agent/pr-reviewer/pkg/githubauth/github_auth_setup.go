// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubauth

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
	"github.com/golang/glog"
)

// maxCapturedOutputBytes bounds the captured gh stdout+stderr included in the
// wrapped error to avoid runaway error bodies if gh ever spews large output.
const maxCapturedOutputBytes = 4096

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
//
// The token is bound into the exec wrapper here so the gh subprocess receives
// GH_TOKEN in its env — without that, gh inherits the pod env (which has no
// token under GitHub App auth) and dies with "You are not logged into any
// GitHub hosts" even though the IAT was minted successfully.
func NewGhAuthSetupGit(ghToken string) Configurator {
	return &ghAuthSetupGit{
		ghToken: ghToken,
		execFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return defaultExecFunc(ctx, ghToken, name, args...)
		},
	}
}

// ghAuthSetupGit is the real implementation; execFunc is injectable for testing.
type ghAuthSetupGit struct {
	ghToken  string
	execFunc func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (g *ghAuthSetupGit) Setup(ctx context.Context) error {
	if g.ghToken == "" {
		glog.V(2).Infof("github-auth-setup: GH_TOKEN not set, skipping gh auth setup-git")
		return nil
	}
	glog.V(2).Infof("github-auth-setup: running gh auth setup-git")
	out, err := g.execFunc(ctx, "gh", "auth", "setup-git")
	if err != nil {
		// Scrub the literal token from both the captured output and the wrapped
		// underlying error message: gh's stdout/stderr (and exec wrappers that
		// echo arguments) may embed the token. Bounded tail keeps error bodies
		// sane even if gh ever produces large output.
		captured := truncateTail(scrubToken(string(out), g.ghToken), maxCapturedOutputBytes)
		sanitizedErr := stderrors.New(scrubToken(err.Error(), g.ghToken))
		glog.V(4).Infof("github-auth-setup: gh auth setup-git failed: %s", captured)
		if captured == "" {
			return errors.Wrapf(ctx, sanitizedErr, "gh auth setup-git failed")
		}
		return errors.Wrapf(ctx, sanitizedErr, "gh auth setup-git failed: %s", captured)
	}
	glog.V(2).Infof("github-auth-setup: gh auth setup-git complete")
	return nil
}

// defaultExecFunc is the production exec.CommandContext wrapper. It returns
// the combined stdout+stderr alongside the exec error so the caller can scrub
// secrets out of the output before including it in the surfaced error.
//
// ghToken is exported as GH_TOKEN in the subprocess env so gh can authenticate
// without relying on `gh auth login` state in ~/.config/gh/hosts.yml. When
// ghToken is empty the env entry is omitted so the subprocess inherits the
// pod env unchanged (preserving the prior behavior for tests that don't care
// about auth, e.g. `true` / `false`).
func defaultExecFunc(
	ctx context.Context,
	ghToken string,
	name string,
	args ...string,
) ([]byte, error) {
	// #nosec G204 -- binary is hardcoded "gh" and args are hardcoded ["auth", "setup-git"]; no user input
	cmd := exec.CommandContext(ctx, name, args...)
	if ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+ghToken)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, errors.Wrapf(ctx, err, "%s %v failed", name, args)
	}
	return out, nil
}

// scrubToken removes every occurrence of token from s. When token is empty the
// string is returned unchanged so the helper is safe to call unconditionally.
func scrubToken(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "***")
}

// truncateTail returns the last maxBytes bytes of s. When s is within the
// limit it is returned unchanged; an oversized prefix is replaced with a
// "...[truncated]" marker so operators reading the error know output was clipped.
func truncateTail(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return "...[truncated]" + s[len(s)-maxBytes:]
}

// NewNoopAuthSetup returns a Configurator that always returns nil.
// Used by cmd/run-task so the developer's existing gh auth login continues
// to handle credentials; ~/.gitconfig is never mutated by the agent.
func NewNoopAuthSetup() Configurator {
	return &noopAuthSetup{}
}

type noopAuthSetup struct{}

func (n *noopAuthSetup) Setup(_ context.Context) error { return nil }

// newGhAuthSetupGitWithExecFunc constructs a GhAuthSetupGit with an injected
// exec function for testing. Exposed to external _test packages via
// export_test.go (NewGhAuthSetupGitWithExecFunc); kept unexported here so
// production callers cannot bypass the production exec wrapper.
func newGhAuthSetupGitWithExecFunc(
	ghToken string,
	execFunc func(ctx context.Context, name string, args ...string) ([]byte, error),
) Configurator {
	return &ghAuthSetupGit{ghToken: ghToken, execFunc: execFunc}
}
