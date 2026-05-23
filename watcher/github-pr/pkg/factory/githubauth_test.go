// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg/factory"
)

var _ = Describe("CreateGitHubHTTPClient", func() {
	ctx := context.Background()

	// generateValidPEM creates a valid RSA private key PEM for testing.
	// ghinstalation/v2 parses the PEM at client creation time, so a real key is needed.
	generateValidPEM := func() string {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		privBytes := x509.MarshalPKCS1PrivateKey(key)
		return string(pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privBytes,
		}))
	}

	DescribeTable(
		"auth mode selection",
		func(cfg factory.AuthConfig, expectNonNilClient bool, expectError bool, errorContains string) {
			client, err := factory.CreateGitHubHTTPClient(ctx, cfg)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(client).To(BeNil())
				if errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(errorContains))
				}
			} else {
				Expect(err).NotTo(HaveOccurred())
				if expectNonNilClient {
					Expect(client).NotTo(BeNil())
				} else {
					Expect(client).To(BeNil())
				}
			}
		},

		Entry("App happy path",
			factory.AuthConfig{AppID: 1, InstallationID: 2, PEMKey: generateValidPEM()},
			true, false, ""),

		Entry("PAT fallback",
			factory.AuthConfig{GHToken: "ghp_test"},
			true, false, ""),

		Entry(
			"both set (App wins)",
			factory.AuthConfig{
				AppID:          1,
				InstallationID: 2,
				PEMKey:         generateValidPEM(),
				GHToken:        "ghp_test",
			},
			true,
			false,
			"",
		),

		Entry("neither set",
			factory.AuthConfig{},
			false, true, "neither App nor PAT"),

		Entry("partial App config — only AppID",
			factory.AuthConfig{AppID: 1},
			false, true, "INSTALLATION_ID"),

		Entry("partial App config — missing PEM",
			factory.AuthConfig{AppID: 1, InstallationID: 2},
			false, true, "PEM_KEY"),
	)
})
