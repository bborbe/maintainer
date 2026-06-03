// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubapp_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/lib/githubapp"
)

var _ = Describe("NewClient", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("returns error when both PEM and PEMPath are set", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
			PEM:            []byte("pem-content"),
			PEMPath:        "/path/to/pem",
		}
		_, err := githubapp.NewClient(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one"))
	})

	It("returns error when neither PEM nor PEMPath is set", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
		}
		_, err := githubapp.NewClient(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one"))
	})

	It("returns error when AppID is zero", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          0,
			InstallationID: 456,
			PEM:            []byte("pem-content"),
		}
		_, err := githubapp.NewClient(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("app id"))
	})

	It("returns error when InstallationID is zero", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 0,
			PEM:            []byte("pem-content"),
		}
		_, err := githubapp.NewClient(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installation id"))
	})

	It("returns error when PEMPath does not exist", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
			PEMPath:        "/nonexistent/path/to/pem",
		}
		_, err := githubapp.NewClient(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/nonexistent/path/to/pem"))
	})

	It("returns error when PEM is malformed", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
			PEM:            []byte("not-a-valid-pem"),
		}
		_, err := githubapp.NewClient(ctx, cfg)
		Expect(err).To(HaveOccurred())
	})

	It("returns client and makes authenticated request to test server", func(ctx context.Context) {
		token := "ghs_test123456789"
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/app/installations/456/access_tokens" && r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(
					[]byte(`{"token":"` + token + `","expires_at":"2099-01-01T00:00:00Z"}`),
				)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/repos/") && r.Method == http.MethodGet {
				auth := r.Header.Get("Authorization")
				if matchAuthHeader(auth, token) {
					w.WriteHeader(http.StatusOK)
					return
				}
				http.Error(w, "Unauthorized: got "+auth, http.StatusUnauthorized)
				return
			}
			http.NotFound(w, r)
		}))

		pemBytes := generateRSAKey()
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
			PEM:            pemBytes,
			BaseURL:        server.URL,
		}
		client, err := githubapp.NewClient(ctx, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(client).NotTo(BeNil())

		// Make a request through the client to the test server
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			server.URL+"/repos/owner/repo",
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})

var _ = Describe("MintIAT", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("returns error when both PEM and PEMPath are set", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
			PEM:            []byte("pem-content"),
			PEMPath:        "/path/to/pem",
		}
		_, err := githubapp.MintIAT(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one"))
	})

	It("returns error when neither PEM nor PEMPath is set", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
		}
		_, err := githubapp.MintIAT(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one"))
	})

	It("returns error when AppID is zero", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          0,
			InstallationID: 456,
			PEM:            []byte("pem-content"),
		}
		_, err := githubapp.MintIAT(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("app id"))
	})

	It("returns error when InstallationID is zero", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 0,
			PEM:            []byte("pem-content"),
		}
		_, err := githubapp.MintIAT(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installation id"))
	})

	It("returns error when PEMPath does not exist", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
			PEMPath:        "/nonexistent/path/to/pem",
		}
		_, err := githubapp.MintIAT(ctx, cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/nonexistent/path/to/pem"))
	})

	It("returns error when PEM is malformed", func(ctx context.Context) {
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
			PEM:            []byte("not-a-valid-pem"),
		}
		_, err := githubapp.MintIAT(ctx, cfg)
		Expect(err).To(HaveOccurred())
	})

	It("returns IAT token from test server", func(ctx context.Context) {
		token := "ghs_test123456789"
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/app/installations/456/access_tokens" && r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(
					[]byte(`{"token":"` + token + `","expires_at":"2099-01-01T00:00:00Z"}`),
				)
				return
			}
			http.NotFound(w, r)
		}))

		pemBytes := generateRSAKey()
		cfg := githubapp.Config{
			AppID:          123,
			InstallationID: 456,
			PEM:            pemBytes,
			BaseURL:        server.URL,
		}
		iat, err := githubapp.MintIAT(ctx, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(iat).To(Equal(token))
	})
})

// matchAuthHeader checks if the Authorization header matches the expected token format.
func matchAuthHeader(auth, token string) bool {
	matched, _ := regexp.MatchString(`^(token|Bearer) `+regexp.QuoteMeta(token)+`$`, auth)
	return matched
}

// generateRSAKey generates a valid RSA private key for testing.
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
