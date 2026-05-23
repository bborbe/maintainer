// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package auth resolves the GitHub auth mode (App vs PAT) for the watcher binaries.
package auth

import (
	"context"
	"net/http"
	"os"

	"github.com/bborbe/errors"
	"github.com/golang/glog"

	githubapp "github.com/bborbe/maintainer/lib/githubapp"
)

// Config is the resolver input.
type Config struct {
	AppID          int64
	InstallationID int64
	PEMKeyFile     string
	PEMKey         string
	GHToken        string
	LogPrefix      string // e.g. "watcher/github-build" or "watcher/github-build-run-once"
}

// Resolve picks the active auth mode (App-when-configured, PAT-when-not,
// error-when-neither) and returns an *http.Client suitable for go-github.
// The App-mode client has an auto-refreshing transport (lib/githubapp.NewClient).
// The PAT-mode client has a static-Bearer transport (tokenTransport below).
func Resolve(ctx context.Context, cfg Config) (*http.Client, error) {
	hasPEMFile := cfg.PEMKeyFile != ""
	hasPEMContent := cfg.PEMKey != ""
	useGitHubApp := cfg.AppID != 0 && cfg.InstallationID != 0 && (hasPEMFile || hasPEMContent)

	if cfg.GHToken != "" && useGitHubApp {
		glog.Warningf(
			"%s auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
			cfg.LogPrefix,
		)
	}

	switch {
	case useGitHubApp:
		appCfg := githubapp.Config{AppID: cfg.AppID, InstallationID: cfg.InstallationID}
		if hasPEMFile {
			pemBytes, err := os.ReadFile(cfg.PEMKeyFile)
			if err != nil {
				return nil, errors.Wrapf(ctx, err, "read PEM file %s", cfg.PEMKeyFile)
			}
			appCfg.PEM = pemBytes
		} else {
			appCfg.PEM = []byte(cfg.PEMKey)
		}
		httpClient, err := githubapp.NewClient(ctx, appCfg)
		if err != nil {
			return nil, errors.Wrap(ctx, err, "create githubapp client")
		}
		glog.V(2).Infof("%s auth mode=github-app app_id=%d installation_id=%d",
			cfg.LogPrefix, cfg.AppID, cfg.InstallationID)
		return httpClient, nil

	case cfg.GHToken != "":
		glog.Warningf(
			"%s auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)",
			cfg.LogPrefix,
		)
		return &http.Client{Transport: &tokenTransport{token: cfg.GHToken}}, nil

	default:
		return nil, errors.Errorf(
			ctx,
			"%s auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY_FILE (or PEM_KEY), or set GH_TOKEN",
			cfg.LogPrefix,
		)
	}
}

// tokenTransport injects a static Bearer token. Unexported — only Resolve
// constructs it.
type tokenTransport struct{ token string }

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(req)
}
