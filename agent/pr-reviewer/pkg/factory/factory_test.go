// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"reflect"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	libkafka "github.com/bborbe/kafka"
	libkafkamocks "github.com/bborbe/kafka/mocks"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg/factory"
	"github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg/git"
	"github.com/bborbe/code-reviewer/agent/pr-reviewer/pkg/githubauth"
)

var _ = Describe("Factory", func() {
	Describe("CreateClaudeRunner", func() {
		It("returns a non-nil runner with empty env", func() {
			runner := factory.CreateClaudeRunner(
				"",
				"agent",
				"sonnet",
				map[string]string{},
				claudelib.AllowedTools{"Read"},
			)
			Expect(runner).NotTo(BeNil())
		})

		It("returns a non-nil runner with GH_TOKEN in env", func() {
			runner := factory.CreateClaudeRunner(
				"",
				"agent",
				"sonnet",
				map[string]string{"GH_TOKEN": "test-token"},
				claudelib.AllowedTools{"Read"},
			)
			Expect(runner).NotTo(BeNil())
		})
	})

	Describe("CreateAgent", func() {
		It("returns a non-nil AgentRunner with empty token and env", func() {
			var repoManager git.RepoManager
			agent := factory.CreateAgent(
				"",
				"agent",
				"sonnet",
				"",
				map[string]string{},
				repoManager,
				"standard",
				nil,
			)
			Expect(agent).NotTo(BeNil())
		})

		It("returns a non-nil AgentRunner with token set in env", func() {
			var repoManager git.RepoManager
			agent := factory.CreateAgent(
				"",
				"agent",
				"sonnet",
				"test-token",
				map[string]string{"GH_TOKEN": "test-token"},
				repoManager,
				"standard",
				nil,
			)
			Expect(agent).NotTo(BeNil())
		})

	})

	Describe("CreateFileResultDeliverer", func() {
		It("returns a non-nil deliverer", func() {
			deliverer := factory.CreateFileResultDeliverer("/tmp/task.md")
			Expect(deliverer).NotTo(BeNil())
		})
	})

	Describe("CreateDeliverer", func() {
		It("returns an error when brokers are unreachable", func() {
			currentDateTime := libtime.CurrentDateTimeGetterFunc(func() libtime.DateTime {
				return libtime.DateTime{}
			})
			ctx := context.Background()
			_, _, err := factory.CreateDeliverer(
				ctx,
				agentlib.TaskIdentifier("task-123"),
				libkafka.Brokers{"localhost:1"},
				"dev",
				"content",
				currentDateTime,
			)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CreateKafkaResultDeliverer", func() {
		It("returns a non-nil deliverer", func() {
			syncProducer := &libkafkamocks.KafkaSyncProducer{}
			currentDateTime := libtime.CurrentDateTimeGetterFunc(func() libtime.DateTime {
				return libtime.DateTime{}
			})
			deliverer := factory.CreateKafkaResultDeliverer(
				syncProducer,
				"dev",
				agentlib.TaskIdentifier("task-123"),
				"original content",
				currentDateTime,
			)
			Expect(deliverer).NotTo(BeNil())
		})
	})

	Describe("RunConfig.AuthSetup wiring", func() {
		It("pod path: NewGhAuthSetupGit produces the real impl type", func() {
			cfg := factory.RunConfig{
				AuthSetup: githubauth.NewGhAuthSetupGit("fake-token"),
			}
			Expect(cfg.AuthSetup).NotTo(BeNil())
			Expect(reflect.TypeOf(cfg.AuthSetup).String()).To(Equal("*githubauth.ghAuthSetupGit"))
		})

		It("local-CLI path: NewNoopAuthSetup produces the noop impl type", func() {
			cfg := factory.RunConfig{
				AuthSetup: githubauth.NewNoopAuthSetup(),
			}
			Expect(cfg.AuthSetup).NotTo(BeNil())
			Expect(reflect.TypeOf(cfg.AuthSetup).String()).To(Equal("*githubauth.noopAuthSetup"))
		})
	})
})
