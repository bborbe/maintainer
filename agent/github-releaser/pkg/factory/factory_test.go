// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/factory"
)

var _ = Describe("CreateAgentProvider", func() {
	var provider agentlib.AgentProvider

	BeforeEach(func() {
		provider = factory.CreateAgentProvider(
			claudelib.ClaudeConfigDir("/tmp/claude"),
			claudelib.AgentDir("/tmp/agent"),
			claudelib.ClaudeModel("sonnet"),
			"",
			map[string]string{},
		)
	})

	It("routes task_type: github-release", func() {
		a, err := provider.Get(context.Background(), agentlib.TaskType("github-release"))
		Expect(err).NotTo(HaveOccurred())
		Expect(a).NotTo(BeNil())
	})

	It("routes task_type: healthcheck", func() {
		a, err := provider.Get(context.Background(), agentlib.TaskTypeHealthcheck)
		Expect(err).NotTo(HaveOccurred())
		Expect(a).NotTo(BeNil())
	})

	It("returns error for unknown task_type", func() {
		_, err := provider.Get(context.Background(), agentlib.TaskType("not-a-real-type"))
		Expect(err).To(HaveOccurred())
	})
})
