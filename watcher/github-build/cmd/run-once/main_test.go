// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main_test

import (
	"context"
	"errors"
	"testing"
	"time"

	libkafka "github.com/bborbe/kafka"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/gexec"

	runonce "github.com/bborbe/maintainer/watcher/github-build/cmd/run-once"
	"github.com/bborbe/maintainer/watcher/github-build/pkg"
	"github.com/bborbe/maintainer/watcher/github-build/pkg/mocks"
)

var _ = Describe("Run", func() {
	var (
		ctx         context.Context
		mockWatcher *mocks.Watcher
		app         *runonce.Application
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockWatcher = &mocks.Watcher{}
		app = &runonce.Application{
			KafkaBrokers:    libkafka.Brokers{"localhost:9092"},
			Stage:           "dev",
			RepoAllowlist:   "github.com/owner/repo",
			BuildAssignee:   "test-assignee",
			BuildTaskStatus: "next",
			MaxTitleLen:     200,
			GHToken:         "fake-token",
		}
	})

	DescribeTable("error cases",
		func(setupFn func(), expectError bool, errorContains string) {
			mockWatcher.PollReturns(nil)
			mockWatcher.PollStub = nil

			app.CreateWatcher = func(
				ctx context.Context,
				ghClient pkg.GitHubClient,
				brokers libkafka.Brokers,
				stage string,
				inputAllowlist []string,
				resolved pkg.AllowlistSnapshot,
				cursorPath string,
				assignee string,
				taskStatus string,
				taskPhase string,
				maxTitleLen int,
				taskSuffix string,
			) (pkg.Watcher, func(), error) {
				if len(brokers) == 0 {
					return nil, nil, errors.New("create kafka create sender: brokers empty")
				}
				return mockWatcher, func() {}, nil
			}

			setupFn()

			err := app.Run(ctx, nil)

			if expectError {
				Expect(err).To(HaveOccurred())
				if errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(errorContains))
				}
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("returns error when KAFKA_BROKERS is empty",
			func() {
				app.KafkaBrokers = libkafka.Brokers{}
				mockWatcher.PollReturns(errors.New("should not be called"))
			},
			true,
			"create kafka create sender",
		),
		Entry("returns error when REPO_ALLOWLIST is empty",
			func() {
				app.RepoAllowlist = ""
				mockWatcher.PollReturns(errors.New("should not be called"))
			},
			true,
			"REPO_ALLOWLIST must be non-empty",
		),
		Entry("returns error when Poll fails",
			func() {
				mockWatcher.PollReturns(errors.New("poll failed"))
			},
			true,
			"poll failed",
		),
	)

	Context("success path", func() {
		It("succeeds when all required env vars are set and Poll succeeds", func() {
			mockWatcher.PollReturns(nil)

			app.CreateWatcher = func(
				ctx context.Context,
				ghClient pkg.GitHubClient,
				brokers libkafka.Brokers,
				stage string,
				inputAllowlist []string,
				resolved pkg.AllowlistSnapshot,
				cursorPath string,
				assignee string,
				taskStatus string,
				taskPhase string,
				maxTitleLen int,
				taskSuffix string,
			) (pkg.Watcher, func(), error) {
				return mockWatcher, func() {}, nil
			}

			err := app.Run(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockWatcher.PollCallCount()).To(Equal(1))
		})
	})
})

var _ = Describe("Main", func() {
	It("Compiles", func() {
		var err error
		_, err = gexec.Build(
			"github.com/bborbe/maintainer/watcher/github-build/cmd/run-once",
			"-mod=mod",
		)
		Expect(err).NotTo(HaveOccurred())
	})
})

func TestSuite(t *testing.T) {
	time.Local = time.UTC
	format.TruncatedDiff = false
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	suiteConfig.Timeout = 60 * time.Second
	RunSpecs(t, "Run-Once Suite", suiteConfig, reporterConfig)
}
