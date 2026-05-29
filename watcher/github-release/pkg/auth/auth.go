// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package auth resolves a GitHub HTTP client from the supplied credentials.
// Encapsulates the GitHub App vs PAT decision so the watcher (long-running
// poll loop) and run-once binaries share one implementation.
//
// I/O happens here (GitHub App JWT exchange + installation token fetch), so
// this code lives outside pkg/factory — the factory package is constrained to
// pure composition (no I/O, no error returns).
package auth

import (
	"context"
	"net/http"

	"github.com/bborbe/errors"
	"github.com/golang/glog"
	"golang.org/x/oauth2"

	"github.com/bborbe/maintainer/lib/githubapp"
)

// Credentials carries the union of inputs needed to choose between GitHub
// App auth and PAT fallback. Read from binary-specific argument structs by
// the caller.
type Credentials struct {
	AppID          int64
	InstallationID int64
	PEMKey         []byte
	Token          string
}

// ResolveGitHubClient chooses GitHub App auth (preferred) over PAT and
// returns the authenticated *http.Client.
//
// Rules:
//   - All three App fields set → App wins; Token (if also set) ignored with warning.
//   - Any subset of App fields set without the other two → error (partial config).
//   - Only Token set → PAT fallback (warning logged: prefer App).
//   - Nothing set → error (neither auth path configured).
func ResolveGitHubClient(ctx context.Context, creds Credentials) (*http.Client, error) {
	appPartial := (creds.AppID != 0) || (creds.InstallationID != 0) || (len(creds.PEMKey) != 0)
	appComplete := (creds.AppID != 0) && (creds.InstallationID != 0) &&
		(len(creds.PEMKey) != 0)
	if appPartial && !appComplete {
		var missing []string
		if creds.AppID == 0 {
			missing = append(missing, "APP_ID")
		}
		if creds.InstallationID == 0 {
			missing = append(missing, "INSTALLATION_ID")
		}
		if len(creds.PEMKey) == 0 {
			missing = append(missing, "PEM_KEY")
		}
		return nil, errors.Errorf(
			ctx,
			"watcher auth: partial GitHub App config — missing %v; set all three or none",
			missing,
		)
	}

	if appComplete {
		if creds.Token != "" {
			glog.Warningf(
				"watcher auth: both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
			)
		}
		glog.Infof(
			"watcher auth mode=github-app app_id=%d installation_id=%d",
			creds.AppID,
			creds.InstallationID,
		)
		client, err := githubapp.NewClient(ctx, githubapp.Config{
			AppID:          creds.AppID,
			InstallationID: creds.InstallationID,
			PEM:            creds.PEMKey,
		})
		if err != nil {
			return nil, errors.Wrap(ctx, err, "create github app client")
		}
		return client, nil
	}
	if creds.Token != "" {
		glog.Warningf("watcher auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: creds.Token})
		return oauth2.NewClient(ctx, ts), nil
	}
	return nil, errors.Errorf(
		ctx,
		"watcher auth: neither App nor PAT configured — set APP_ID + INSTALLATION_ID + PEM_KEY, or set GH_TOKEN",
	)
}
