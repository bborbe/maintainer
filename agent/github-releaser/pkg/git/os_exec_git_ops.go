// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/bborbe/errors"
	"github.com/golang/glog"
)

// BotIdentity holds the commit author/committer identity. Hardcoded
// intentionally: the only consumer is github-releaser, and a single
// value is the contract. If a future spec needs override capability,
// that spec adds the seam; until then, parameterization is YAGNI.
//
// Per spec 049 § Constraints + [[GitHub Release Agent Phase 1 Learnings]],
// this MUST match the Phase 1 slash-command identity verbatim — otherwise
// v1.7.8's release commit history breaks attribution continuity.
type BotIdentity struct {
	Name  string
	Email string
}

// DefaultBotIdentity returns the Phase 1 verbatim identity. The osExecGitOps
// struct reads this internally on every Commit / Tag — there is no override
// path. Exposed publicly for test assertions.
func DefaultBotIdentity() BotIdentity {
	return BotIdentity{
		Name:  "Benjamin Borbe",
		Email: "bborbe@users.noreply.github.com",
	}
}

// NewOSExecGitOps returns a GitOps implementation that shells out to the
// git binary via os/exec. Zero-arg: the bot identity is constant via
// DefaultBotIdentity().
func NewOSExecGitOps() GitOps {
	return &osExecGitOps{}
}

type osExecGitOps struct{}

// cmdEnv returns the env allowlist for git subprocesses: HOME (for ~/.gitconfig
// fallback) + PATH (to resolve git). Strict allowlist prevents pod-level
// secrets from leaking. Mirrors pr-reviewer's repoManager.cmdEnv.
func (g *osExecGitOps) cmdEnv() []string {
	return []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
	}
}

func (g *osExecGitOps) Clone(ctx context.Context, cloneURL, ref, workdir string) error {
	// git clone --branch <ref> --depth 1 <cloneURL> <workdir>
	// --depth 1 is acceptable because we only rewrite CHANGELOG and push a single
	// commit + tag; we don't need history beyond HEAD.
	// #nosec G204 -- cloneURL constructed in caller from validated frontmatter; workdir is os.TempDir-rooted; ref validated by caller
	cmd := exec.CommandContext(
		ctx,
		"git",
		"clone",
		"--branch",
		ref,
		"--depth",
		"1",
		cloneURL,
		workdir,
	)
	cmd.Env = g.cmdEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Errorf(ctx, "git clone: %s", redactToken(strings.TrimSpace(stderr.String())))
	}
	glog.V(2).Infof("git clone succeeded: ref=%s workdir=%s", ref, workdir)
	return nil
}

func (g *osExecGitOps) Commit(
	ctx context.Context,
	workdir, message string,
	paths ...string,
) (string, error) {
	// git -C <workdir> add <paths...>
	if len(paths) > 0 {
		addArgs := append([]string{"-C", workdir, "add", "--"}, paths...)
		// #nosec G204 -- workdir is os.TempDir-rooted; paths come from execution step (CHANGELOG.md only)
		if out, err := exec.CommandContext(ctx, "git", addArgs...).CombinedOutput(); err != nil {
			return "", errors.Errorf(ctx, "git add: %s", strings.TrimSpace(string(out)))
		}
	}

	// git -C <workdir> -c user.name=<name> -c user.email=<email> commit -m <message>
	commitArgs := []string{
		"-C", workdir,
		"-c", "user.name=" + DefaultBotIdentity().Name,
		"-c", "user.email=" + DefaultBotIdentity().Email,
		"commit",
		"-m", message,
	}
	// #nosec G204 -- workdir is os.TempDir-rooted; identity is the bot constant; message comes from execution step
	if out, err := exec.CommandContext(ctx, "git", commitArgs...).CombinedOutput(); err != nil {
		return "", errors.Errorf(ctx, "git commit: %s", strings.TrimSpace(string(out)))
	}

	// git -C <workdir> rev-parse --short HEAD → short SHA
	// #nosec G204 -- workdir is os.TempDir-rooted; args are hardcoded
	shaBytes, err := exec.CommandContext(ctx, "git", "-C", workdir, "rev-parse", "--short", "HEAD").
		Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "git rev-parse HEAD")
	}
	return strings.TrimSpace(string(shaBytes)), nil
}

func (g *osExecGitOps) Tag(ctx context.Context, workdir, tag, message string) error {
	// git -C <workdir> -c user.name=<name> -c user.email=<email> tag -a <tag> -m <message>
	args := []string{
		"-C", workdir,
		"-c", "user.name=" + DefaultBotIdentity().Name,
		"-c", "user.email=" + DefaultBotIdentity().Email,
		"tag", "-a", tag, "-m", message,
	}
	// #nosec G204 -- workdir is os.TempDir-rooted; identity is the bot constant; tag and message come from execution step
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = g.cmdEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Errorf(ctx, "git tag: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (g *osExecGitOps) Push(ctx context.Context, workdir string, refs ...string) error {
	// git -C <workdir> push --atomic origin <refs...>
	// --atomic ensures HEAD + tag land together or neither lands. Without it,
	// GitHub may accept HEAD and reject the tag (or vice versa), leaving an
	// inconsistent state on the remote.
	args := append([]string{"-C", workdir, "push", "--atomic", "origin"}, refs...)
	// No --force / --force-with-lease — non-fast-forward maps to retry, not overwrite.
	// #nosec G204 -- workdir is os.TempDir-rooted; refs are constructed by execution step from validated frontmatter ref / tag
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = g.cmdEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Errorf(ctx, "git push: %s", redactToken(strings.TrimSpace(stderr.String())))
	}
	return nil
}

// redactToken strips x-access-token:<TOK>@ patterns from stderr to prevent
// GH_TOKEN from landing in error logs. Git can echo the URL with embedded
// credentials on auth/clone failures (e.g.
// "fatal: unable to access 'https://x-access-token:ghp_AAA@github.com/...'").
// Apply to ALL Clone/Push stderr that gets wrapped into errors.
func redactToken(s string) string {
	// Replace x-access-token:<anything-up-to-@> with x-access-token:[REDACTED]
	return tokenURLRegexp.ReplaceAllString(s, "x-access-token:[REDACTED]@")
}

var tokenURLRegexp = regexp.MustCompile(`x-access-token:[^@\s]+@`)
