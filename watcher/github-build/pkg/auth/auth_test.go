// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"context"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-build/pkg/auth"
)

var _ = Describe("auth.Resolve", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		// Ensure glog flags are parsed so we can set them in tests.
		_ = flag.CommandLine.Parse([]string{})
	})

	Describe("PAT fallback mode", func() {
		Context("AppID=0, InstallationID=0, GHToken set", func() {
			It("returns an http.Client that sends Bearer token to the server", func() {
				httpClient, err := auth.Resolve(ctx, auth.Config{
					GHToken:   "my-token",
					LogPrefix: "test",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(httpClient).NotTo(BeNil())

				var captured string
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						captured = r.Header.Get("Authorization")
						w.WriteHeader(http.StatusOK)
					}),
				)
				defer server.Close()

				_, err = httpClient.Get(server.URL)
				Expect(err).NotTo(HaveOccurred())
				Expect(captured).To(Equal("Bearer my-token"))
			})
		})
	})

	Describe("Conflict mode", func() {
		Context("both App credentials and GH_TOKEN are set", func() {
			It("emits the exact warning literal to stderr and continues with App auth", func() {
				origStderr := os.Stderr
				r, w, _ := os.Pipe()
				os.Stderr = w
				_ = flag.Set("logtostderr", "true")
				_ = flag.Set("stderrthreshold", "WARNING")

				_, _ = auth.Resolve(ctx, auth.Config{
					AppID:          123456,
					InstallationID: 789012,
					PEMKey:         "not-a-real-pem",
					GHToken:        "some-token",
					LogPrefix:      "test",
				})

				_ = w.Close()
				os.Stderr = origStderr

				out, _ := io.ReadAll(r)
				captured := string(out)
				Expect(captured).To(ContainSubstring(
					"both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored",
				))
			})
		})
	})

	Describe("Refusal mode", func() {
		Context("neither App nor PAT configured", func() {
			It("returns nil client and an error mentioning APP_ID and GH_TOKEN", func() {
				httpClient, err := auth.Resolve(ctx, auth.Config{
					LogPrefix: "test",
				})
				Expect(httpClient).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("APP_ID"))
				Expect(err.Error()).To(ContainSubstring("GH_TOKEN"))
			})
		})
	})

	Describe("Missing PEMKeyFile", func() {
		Context("AppID and InstallationID set but PEMKeyFile does not exist", func() {
			It("returns an error mentioning the missing path", func() {
				_, err := auth.Resolve(ctx, auth.Config{
					AppID:          123456,
					InstallationID: 789012,
					PEMKeyFile:     "/nonexistent/path",
					LogPrefix:      "test",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("/nonexistent/path"),
					ContainSubstring("no such file"),
				))
			})
		})
	})
})
