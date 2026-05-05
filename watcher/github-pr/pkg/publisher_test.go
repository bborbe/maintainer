// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"encoding/json"
	"errors"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/cqrs/cdb"
	cqrsmocks "github.com/bborbe/cqrs/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
)

var _ = Describe("Publisher", func() {
	var (
		ctx    context.Context
		sender *cqrsmocks.CDBCommandObjectSender
		pub    pkg.CommandPublisher
	)

	BeforeEach(func() {
		ctx = context.Background()
		sender = new(cqrsmocks.CDBCommandObjectSender)
		pub = pkg.NewCommandPublisher(ctx, sender)
	})

	Describe("PublishCreate", func() {
		Context("sender succeeds", func() {
			It("calls SendCommandObject once with correct operation", func() {
				sender.SendCommandObjectReturns(nil)
				cmd := agentlib.CreateTaskCommand{
					TaskIdentifier: agentlib.TaskIdentifier("task-uuid-123"),
					Frontmatter:    agentlib.TaskFrontmatter{"assignee": "pr-reviewer-agent"},
					Body:           "# PR Review: test\n\nhttps://example.com\n",
				}
				err := pub.PublishCreate(ctx, cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(sender.SendCommandObjectCallCount()).To(Equal(1))
				_, obj := sender.SendCommandObjectArgsForCall(0)
				Expect(obj.Command.Operation).To(Equal(agentlib.CreateTaskCommandOperation))
				Expect(obj.SchemaID).To(Equal(agentlib.TaskV1SchemaID))
			})
		})

		Context("sender returns error", func() {
			It("returns wrapped error", func() {
				sender.SendCommandObjectReturns(errors.New("kafka down"))
				cmd := agentlib.CreateTaskCommand{
					TaskIdentifier: "t1",
					Frontmatter:    agentlib.TaskFrontmatter{},
				}
				err := pub.PublishCreate(ctx, cmd)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("publish create-task"))
			})
		})

		Context("event data contains task identifier", func() {
			It("serializes taskIdentifier into the command event", func() {
				sender.SendCommandObjectReturns(nil)
				cmd := agentlib.CreateTaskCommand{
					TaskIdentifier: agentlib.TaskIdentifier("my-task-id"),
					Frontmatter:    agentlib.TaskFrontmatter{"status": "in_progress"},
				}
				Expect(pub.PublishCreate(ctx, cmd)).To(Succeed())
				_, obj := sender.SendCommandObjectArgsForCall(0)
				data, err := json.Marshal(obj.Command.Data)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(ContainSubstring("my-task-id"))
			})
		})
	})

	Describe("PublishUpdateFrontmatter", func() {
		Context("sender succeeds", func() {
			It("calls SendCommandObject once with correct operation", func() {
				sender.SendCommandObjectReturns(nil)
				cmd := agentlib.UpdateFrontmatterCommand{
					TaskIdentifier: agentlib.TaskIdentifier("task-456"),
					Updates:        agentlib.TaskFrontmatter{"phase": "planning"},
					Body: &agentlib.BodySection{
						Heading: "## Outdated by force-push abc123",
						Section: "## Outdated by force-push abc123\n",
					},
				}
				err := pub.PublishUpdateFrontmatter(ctx, cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(sender.SendCommandObjectCallCount()).To(Equal(1))
				_, obj := sender.SendCommandObjectArgsForCall(0)
				Expect(obj.Command.Operation).To(Equal(agentlib.UpdateFrontmatterCommandOperation))
			})
		})

		Context("sender returns error", func() {
			It("returns wrapped error", func() {
				sender.SendCommandObjectReturns(errors.New("broker unreachable"))
				cmd := agentlib.UpdateFrontmatterCommand{
					TaskIdentifier: "t2",
					Updates:        agentlib.TaskFrontmatter{},
				}
				err := pub.PublishUpdateFrontmatter(ctx, cmd)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("publish update-frontmatter"))
			})
		})
	})

	Describe("CommandObject shape", func() {
		It("has non-empty RequestID", func() {
			sender.SendCommandObjectReturns(nil)
			cmd := agentlib.CreateTaskCommand{
				TaskIdentifier: "x",
				Frontmatter:    agentlib.TaskFrontmatter{},
			}
			Expect(pub.PublishCreate(ctx, cmd)).To(Succeed())
			_, obj := sender.SendCommandObjectArgsForCall(0)
			Expect(string(obj.Command.RequestID)).NotTo(BeEmpty())
		})

		It("SchemaID is agent-task-v1", func() {
			sender.SendCommandObjectReturns(nil)
			Expect(pub.PublishCreate(ctx, agentlib.CreateTaskCommand{
				TaskIdentifier: "x",
				Frontmatter:    agentlib.TaskFrontmatter{},
			})).To(Succeed())
			_, obj := sender.SendCommandObjectArgsForCall(0)
			Expect(obj.SchemaID).To(Equal(cdb.SchemaID{
				Group:   "agent",
				Kind:    "task",
				Version: "v1",
			}))
		})
	})
})
