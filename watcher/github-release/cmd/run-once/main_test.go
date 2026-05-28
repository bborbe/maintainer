// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main_test

import (
	"context"
	stderrors "errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/IBM/sarama"
	saramamocks "github.com/IBM/sarama/mocks"
	"github.com/bborbe/cqrs/base"
	libkafka "github.com/bborbe/kafka"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/gexec"

	runonce "github.com/bborbe/maintainer/watcher/github-release/cmd/run-once"
	"github.com/bborbe/maintainer/watcher/github-release/pkg"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-release/pkg/mocks"
)

// fakeProducerFactory returns a sarama mock SyncProducer that records calls in
// memory — no network connection. Tests inject this so producer creation in
// Run() succeeds without hitting a real broker.
func fakeProducerFactory(
	t GinkgoTInterface,
) func(context.Context, libkafka.Brokers, string) (libkafka.SyncProducer, error) {
	return func(_ context.Context, _ libkafka.Brokers, _ string) (libkafka.SyncProducer, error) {
		return libkafka.NewSyncProducerFromSaramaSyncProducer(
			saramamocks.NewSyncProducer(t, sarama.NewConfig()),
		), nil
	}
}

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
			Stage:          "dev",
			Owner:          "test-owner",
			RepoAllowlist:  "github.com/owner/repo",
			CursorPath:     "/tmp/cursor.json",
			KafkaBrokers:   libkafka.Brokers{"localhost:9092"},
			CreateProducer: fakeProducerFactory(GinkgoT()),
		}
		os.Setenv("GH_TOKEN", "fake-token")
	})

	mockWatcherFactory := func() runonce.WatcherFactory {
		return func(
			_ *http.Client,
			_ libkafka.SyncProducer,
			_ base.Branch,
			_ string,
			_ string,
			_ filter.TaskCreationFilter,
			_ pkg.Metrics,
			_ []string,
			_ string,
		) pkg.Watcher {
			return mockWatcher
		}
	}

	It("Poll succeeds returns nil", func() {
		mockWatcher.PollReturns(nil)
		app.CreateWatcher = mockWatcherFactory()

		err := app.Run(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockWatcher.PollCallCount()).To(Equal(1))
	})

	It("Poll fails returns wrapped error", func() {
		mockWatcher.PollReturns(stderrors.New("kafka unavailable"))
		app.CreateWatcher = mockWatcherFactory()

		err := app.Run(ctx, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("poll failed"))
	})

	It("empty REPO_ALLOWLIST returns error", func() {
		app.RepoAllowlist = ""
		app.CreateWatcher = mockWatcherFactory()

		err := app.Run(ctx, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("REPO_ALLOWLIST must be non-empty"))
	})
})

var _ = Describe("Main", func() {
	It("Compiles", func() {
		var err error
		_, err = gexec.Build(
			"github.com/bborbe/maintainer/watcher/github-release/cmd/run-once",
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
