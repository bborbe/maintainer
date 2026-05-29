// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package githubauth resolves the github-releaser agent's effective GitHub
// credential at startup. It mirrors the pr-reviewer agent's resolution
// order: GitHub App installation token (preferred) → GH_TOKEN PAT
// (fallback) → startup error. Extracted into its own package so the
// resolution outcomes are unit-testable against an httptest IAT endpoint
// (the pattern lib/githubapp tests already use).
package githubauth

import (
	"context"

	"github.com/bborbe/errors"
	"github.com/golang/glog"

	githubapp "github.com/bborbe/maintainer/lib/githubapp"
)

// AuthMode classifies which credential type is active at pod startup.
type AuthMode int

const (
	// AuthModeNone means no usable credential is configured; the caller
	// MUST refuse to start.
	AuthModeNone AuthMode = iota
	// AuthModeGitHubApp means App credentials are present and an IAT will
	// be minted.
	AuthModeGitHubApp
	// AuthModePATFallback means the legacy GH_TOKEN PAT is used.
	AuthModePATFallback
)

// Config carries the raw credential inputs read from env/flags. Either a
// PEM file path (PEMKeyFile) or PEM env content (PEMKey) may be supplied;
// PEMKeyFile is preferred when both are present. BaseURL overrides the
// GitHub API base (defaults to https://api.github.com); tests point it at
// an httptest server.
type Config struct {
	AppID          int64
	InstallationID int64
	PEMKeyFile     string
	PEMKey         string
	GHToken        string
	BaseURL        string
}

// ResolveAuthMode picks the credential type to use at startup.
//   - AppID>0 AND InstallationID>0 AND (PEMKeyFile set OR PEMKey set) → AuthModeGitHubApp
//   - else GHToken non-empty → AuthModePATFallback
//   - else → AuthModeNone
//
// NOTE: unlike pr-reviewer's ResolveAuthMode (which keys App mode on the
// PEM file path only), the releaser accepts PEM_KEY env content too, per
// spec 052 Desired Behavior 2.
func ResolveAuthMode(appID, installationID int64, pemKeyFile, pemKey, ghToken string) AuthMode {
	hasPEM := pemKeyFile != "" || pemKey != ""
	if appID > 0 && installationID > 0 && hasPEM {
		return AuthModeGitHubApp
	}
	if ghToken != "" {
		return AuthModePATFallback
	}
	return AuthModeNone
}

// Resolve returns the single effective GitHub token for the agent.
//
//   - App mode: mints an installation access token via lib/githubapp.MintIAT
//     (preferring PEMKeyFile over PEMKey when both are set). When GH_TOKEN
//     is ALSO set, logs that App wins and GH_TOKEN is ignored.
//   - PAT fallback: returns GHToken, logging a pat-fallback warning.
//   - None: returns a non-nil error naming the required env vars (both
//     APP_ID and GH_TOKEN appear in the message). Returns BEFORE any clone.
//
// The returned token is the bearer credential wired to BOTH the planning
// fetcher and the execution push. It is never logged in full (MintIAT logs
// only token_prefix).
func Resolve(ctx context.Context, cfg Config) (string, error) {
	switch ResolveAuthMode(cfg.AppID, cfg.InstallationID, cfg.PEMKeyFile, cfg.PEMKey, cfg.GHToken) {
	case AuthModeGitHubApp:
		if cfg.GHToken != "" {
			glog.Warningf(
				"github-releaser auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
			)
		}
		appCfg := githubapp.Config{
			AppID:          cfg.AppID,
			InstallationID: cfg.InstallationID,
			BaseURL:        cfg.BaseURL,
		}
		if cfg.PEMKeyFile != "" {
			appCfg.PEMPath = cfg.PEMKeyFile
		} else {
			appCfg.PEM = []byte(cfg.PEMKey)
		}
		iat, err := githubapp.MintIAT(ctx, appCfg)
		if err != nil {
			return "", errors.Wrap(ctx, err, "mint github app iat")
		}
		glog.V(2).Infof(
			"github-releaser auth mode=github-app app_id=%d installation_id=%d",
			cfg.AppID, cfg.InstallationID,
		)
		return iat, nil
	case AuthModePATFallback:
		glog.Warningf(
			"github-releaser auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)",
		)
		return cfg.GHToken, nil
	default:
		return "", errors.Errorf(
			ctx,
			"github-releaser auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY_FILE (or PEM_KEY), or set GH_TOKEN",
		)
	}
}
