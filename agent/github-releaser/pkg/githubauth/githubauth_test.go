// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubauth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubauth"
)

const stubIAT = "ghs_test123456789"

// newIATServer returns an httptest server that mints stubIAT on the
// installation access-tokens endpoint. Mirrors lib/githubapp tests.
func newIATServer(installationID string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/"+installationID+"/access_tokens" &&
			r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(
				[]byte(`{"token":"` + stubIAT + `","expires_at":"2099-01-01T00:00:00Z"}`),
			)
			return
		}
		http.NotFound(w, r)
	}))
}

var _ = Describe("Resolve", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
			server = nil
		}
	})

	It("App creds set and GH_TOKEN empty → effective token is the minted IAT", func(ctx context.Context) {
		server = newIATServer("456")
		token, err := githubauth.Resolve(ctx, githubauth.Config{
			AppID:          123,
			InstallationID: 456,
			PEMKey:         string(generateRSAKey()),
			BaseURL:        server.URL,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal(stubIAT))
	})

	It("GH_TOKEN set and no App creds → effective token is the PAT", func(ctx context.Context) {
		token, err := githubauth.Resolve(ctx, githubauth.Config{
			GHToken: "pat-abc",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal("pat-abc"))
	})

	It("both App creds and GH_TOKEN set → App wins (token is the IAT, not the PAT)", func(ctx context.Context) {
		server = newIATServer("456")
		token, err := githubauth.Resolve(ctx, githubauth.Config{
			AppID:          123,
			InstallationID: 456,
			PEMKey:         string(generateRSAKey()),
			GHToken:        "pat-abc",
			BaseURL:        server.URL,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal(stubIAT))
		Expect(token).NotTo(Equal("pat-abc"))
	})

	It("neither App creds nor GH_TOKEN → error naming APP_ID and GH_TOKEN", func(ctx context.Context) {
		token, err := githubauth.Resolve(ctx, githubauth.Config{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("APP_ID"))
		Expect(err.Error()).To(ContainSubstring("GH_TOKEN"))
		Expect(token).To(BeEmpty())
	})

	It("App creds with PEM file path → mints IAT (PEMKeyFile preferred over PEMKey)", func(ctx context.Context) {
		server = newIATServer("456")
		pemPath := writeTempPEM()
		token, err := githubauth.Resolve(ctx, githubauth.Config{
			AppID:          123,
			InstallationID: 456,
			PEMKeyFile:     pemPath,
			BaseURL:        server.URL,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal(stubIAT))
	})

	It("malformed PEM → mint error before any token returned", func(ctx context.Context) {
		token, err := githubauth.Resolve(ctx, githubauth.Config{
			AppID:          123,
			InstallationID: 456,
			PEMKey:         "not-a-valid-pem",
		})
		Expect(err).To(HaveOccurred())
		Expect(token).To(BeEmpty())
	})
})

// ResolveAuthMode is exercised directly to lock the App-vs-PAT decision
// table independent of network.
var _ = Describe("ResolveAuthMode", func() {
	It("App when AppID+InstallationID+PEMKeyFile set", func() {
		Expect(githubauth.ResolveAuthMode(1, 2, "/k.pem", "", "")).
			To(Equal(githubauth.AuthModeGitHubApp))
	})
	It("App when AppID+InstallationID+PEMKey (env content) set", func() {
		Expect(githubauth.ResolveAuthMode(1, 2, "", "pem-content", "")).
			To(Equal(githubauth.AuthModeGitHubApp))
	})
	It("App wins when both App creds and GH_TOKEN set", func() {
		Expect(githubauth.ResolveAuthMode(1, 2, "/k.pem", "", "pat")).
			To(Equal(githubauth.AuthModeGitHubApp))
	})
	It("PAT fallback when only GH_TOKEN set", func() {
		Expect(githubauth.ResolveAuthMode(0, 0, "", "", "pat")).
			To(Equal(githubauth.AuthModePATFallback))
	})
	It("PAT fallback when App ids present but no PEM", func() {
		Expect(githubauth.ResolveAuthMode(1, 2, "", "", "pat")).
			To(Equal(githubauth.AuthModePATFallback))
	})
	It("None when nothing set", func() {
		Expect(githubauth.ResolveAuthMode(0, 0, "", "", "")).
			To(Equal(githubauth.AuthModeNone))
	})
})

// writeTempPEM writes a valid RSA PEM to a temp file and returns its path.
func writeTempPEM() string {
	f, err := os.CreateTemp("", "githubauth-test-*.pem")
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = f.Close() }()
	_, err = f.Write(generateRSAKey())
	Expect(err).NotTo(HaveOccurred())
	return f.Name()
}

// generateRSAKey generates a valid RSA private key for testing.
// Copied verbatim from lib/githubapp/githubapp_test.go.
func generateRSAKey() []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(block)
}
