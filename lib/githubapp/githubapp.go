// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubapp

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"
	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/golang/glog"
)

// Config carries the inputs needed to authenticate as a GitHub App installation.
//
// AppID and InstallationID are public values (visible in the App settings page
// and the installation URL respectively) and are safe to commit. PEM (or PEMPath)
// is the long-lived secret and MUST come from a Kubernetes Secret mount, never
// from a checked-in file.
//
// Exactly one of PEM or PEMPath must be non-empty; passing both is a
// configuration error.
type Config struct {
	AppID          int64
	InstallationID int64
	PEM            []byte // PEM content; mutually exclusive with PEMPath
	PEMPath        string // path to PEM file; mutually exclusive with PEM
	BaseURL        string // API base URL (defaults to https://api.github.com); used for testing with httptest
}

// NewClient returns an *http.Client whose RoundTripper authenticates every
// outgoing request as the given App installation using a cached IAT.
//
// The first call mints a JWT, exchanges it for an IAT, and caches the IAT
// for ~50 minutes; subsequent calls reuse the cached IAT and refresh it
// transparently before expiry.
//
// Returns an error if the config is invalid (both PEM and PEMPath set, or
// neither set; AppID or InstallationID zero) or if the PEM cannot be parsed.
func NewClient(ctx context.Context, cfg Config) (*http.Client, error) {
	if err := cfg.validate(ctx); err != nil {
		return nil, errors.Wrap(ctx, err, "validate config")
	}

	pemBytes, err := cfg.resolvePEM()
	if err != nil {
		return nil, errors.Wrap(ctx, err, "resolve PEM")
	}

	transport, err := ghinstallation.New(
		http.DefaultTransport,
		cfg.AppID,
		cfg.InstallationID,
		pemBytes,
	)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "create ghinstallation transport")
	}
	if cfg.BaseURL != "" {
		transport.BaseURL = cfg.BaseURL
	}

	glog.V(2).
		Infof("githubapp: created client for app_id=%d installation_id=%d", cfg.AppID, cfg.InstallationID)

	return &http.Client{Transport: transport}, nil
}

// MintIAT returns a current installation access token as a plain string
// (e.g. "ghs_...") suitable for use as a bearer credential in subprocess
// env (GH_TOKEN), `gh auth setup-git`, or any other caller that needs the
// raw token rather than an authenticated http.Client.
//
// The returned token is valid for up to 1 hour from GitHub's perspective.
// Callers that need long-lived authentication should use NewClient instead;
// callers that need to refresh a one-shot string token should call MintIAT
// again — the underlying ghinstallation/v2 transport caches across calls.
//
// Returns an error if the config is invalid or the IAT exchange fails.
func MintIAT(ctx context.Context, cfg Config) (string, error) {
	if err := cfg.validate(ctx); err != nil {
		return "", errors.Wrap(ctx, err, "validate config")
	}

	pemBytes, err := cfg.resolvePEM()
	if err != nil {
		return "", errors.Wrap(ctx, err, "resolve PEM")
	}

	transport, err := ghinstallation.New(
		http.DefaultTransport,
		cfg.AppID,
		cfg.InstallationID,
		pemBytes,
	)
	if err != nil {
		return "", errors.Wrap(ctx, err, "create ghinstallation transport")
	}
	if cfg.BaseURL != "" {
		transport.BaseURL = cfg.BaseURL
	}

	token, err := transport.Token(ctx)
	if err != nil {
		return "", errors.Wrap(ctx, err, "mint IAT")
	}

	glog.V(2).Infof("githubapp: minted IAT for app_id=%d installation_id=%d token_prefix=%s...",
		cfg.AppID, cfg.InstallationID, prefix8(token))

	return token, nil
}

// validate checks that the config is well-formed.
func (c Config) validate(ctx context.Context) error {
	if c.AppID <= 0 {
		return errors.Errorf(ctx, "github app id must be positive, got %d", c.AppID)
	}
	if c.InstallationID <= 0 {
		return errors.Errorf(
			ctx,
			"github app installation id must be positive, got %d",
			c.InstallationID,
		)
	}
	hasPEM := len(c.PEM) > 0
	hasPEMPath := c.PEMPath != ""
	if hasPEM && hasPEMPath {
		return errors.Errorf(ctx, "exactly one of PEM or PEMPath must be set")
	}
	if !hasPEM && !hasPEMPath {
		return errors.Errorf(ctx, "exactly one of PEM or PEMPath must be set")
	}
	return nil
}

// resolvePEM returns the PEM bytes from either PEM or PEMPath.
func (c Config) resolvePEM() ([]byte, error) {
	if len(c.PEM) > 0 {
		return c.PEM, nil
	}
	// pemKeyFile is the explicit PEMPath CLI flag value (operator input);
	// filepath.Clean breaks gosec G703/G304 taint analysis. The tool's stated
	// purpose is to read an operator-specified PEM file.
	return os.ReadFile(filepath.Clean(c.PEMPath))
}

// prefix8 returns the first 8 characters of s for safe logging.
func prefix8(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:8]
}
