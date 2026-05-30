// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func TestResolveAuthAbsentCredentials(t *testing.T) {
	g := NewGomegaWithT(t)
	app := &application{}
	err := app.resolveAuth(context.Background())
	g.Expect(err).NotTo(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("APP_ID"))
	g.Expect(err.Error()).NotTo(ContainSubstring("GH_TOKEN"))
}

// TestResolveAuthPartialAppConfig ensures incomplete App credentials (App ID +
// Installation ID set, but no PEM file or PEM content) error out before any
// mint and never fall back to a PAT path. Hermetic — useGitHubApp is false, so
// githubapp.MintIAT (a live HTTP call) is never reached. The App-mode *success*
// path is covered by lib/githubapp's own httptest-backed MintIAT tests.
func TestResolveAuthPartialAppConfig(t *testing.T) {
	g := NewGomegaWithT(t)
	app := &application{AppID: 1, InstallationID: 2}
	err := app.resolveAuth(context.Background())
	g.Expect(err).NotTo(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("APP_ID"))
	g.Expect(err.Error()).NotTo(ContainSubstring("GH_TOKEN"))
	g.Expect(app.resolvedToken).To(BeEmpty())
}
