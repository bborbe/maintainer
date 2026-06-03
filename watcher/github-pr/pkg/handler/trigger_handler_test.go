// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	task "github.com/bborbe/agent/lib/command/task"
	taskmocks "github.com/bborbe/agent/lib/command/task/mocks"
	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-pr/mocks"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

var _ = Describe("TriggerHandler", func() {
	var (
		ctx                context.Context
		ghClient           *mocks.GitHubClient
		createSender       *taskmocks.TaskCreateCommandSender
		taskCreationFilter *mocks.TaskCreationFilter
		trustDecision      *mocks.Trust
		h                  http.Handler
	)

	BeforeEach(func() {
		ctx = context.Background()
		ghClient = new(mocks.GitHubClient)
		createSender = new(taskmocks.TaskCreateCommandSender)
		taskCreationFilter = new(mocks.TaskCreationFilter)
		trustDecision = new(mocks.Trust)

		taskCreationFilter.SkipReturns(false)
		trustDecision.IsTrustedReturns(trust.NewResult(true, "trusted"), nil)

		h = libhttp.NewErrorHandler(handler.NewSinglePRTriggerHandler(
			ghClient,
			createSender,
			taskCreationFilter,
			trustDecision,
			"dev",
			80, 200, "",
			pkg.NewMetrics(),
		))
	})

	DescribeTable(
		"error cases",
		func(rawURL string, expectedStatus int) {
			req := httptest.NewRequest("POST", "/trigger?"+rawURL, nil)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(expectedStatus))
		},
		Entry("missing url returns 400", "foo=bar", http.StatusBadRequest),
		Entry("empty url returns 400", "url=", http.StatusBadRequest),
		Entry("invalid url returns 400", "url=not-a-url", http.StatusBadRequest),
		Entry(
			"non-github platform returns 400",
			"url=https://bitbucket.org/owner/repo/pull-requests/1",
			http.StatusBadRequest,
		),
	)

	Context("GitHub fetch failure", func() {
		BeforeEach(func() {
			ghClient.GetPRDetailsReturns(pkg.PRDetails{}, errors.Errorf(ctx, "network error"))
		})

		It("returns 502", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/1",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusBadGateway))
		})
	})

	Context("filter rejection", func() {
		BeforeEach(func() {
			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA:     "abc123",
				CloneURL:    "https://github.com/bborbe/repo.git",
				BaseRef:     "main",
				AuthorLogin: "dependabot[bot]",
				Title:       "Bump foo from 1.0 to 2.0",
				IsDraft:     false,
			}, nil)
		})

		Context("draft filter rejects", func() {
			BeforeEach(func() {
				taskCreationFilter.SkipStub = func(pr filter.PR) bool {
					return pr.AuthorLogin == "dependabot[bot]"
				}
			})

			It("returns 422", func() {
				req := httptest.NewRequest(
					"POST",
					"/trigger?url=https://github.com/bborbe/repo/pull/1",
					nil,
				)
				resp := httptest.NewRecorder()
				h.ServeHTTP(resp, req)
				Expect(resp.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})

		Context("WIP title filter rejects", func() {
			BeforeEach(func() {
				ghClient.GetPRDetailsReturns(pkg.PRDetails{
					HeadSHA:     "abc123",
					CloneURL:    "https://github.com/bborbe/repo.git",
					BaseRef:     "main",
					AuthorLogin: "regular-user",
					Title:       "WIP: work in progress",
					IsDraft:     false,
				}, nil)
				taskCreationFilter.SkipStub = func(pr filter.PR) bool {
					return pr.Title == "WIP: work in progress"
				}
			})

			It("returns 422", func() {
				req := httptest.NewRequest(
					"POST",
					"/trigger?url=https://github.com/bborbe/repo/pull/1",
					nil,
				)
				resp := httptest.NewRecorder()
				h.ServeHTTP(resp, req)
				Expect(resp.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})
	})

	Context("Kafka publish failure", func() {
		BeforeEach(func() {
			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA:  "abc123",
				CloneURL: "https://github.com/bborbe/repo.git",
				BaseRef:  "main",
			}, nil)
			createSender.SendCommandReturns(errors.Errorf(ctx, "kafka error"))
		})

		It("returns 502", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/1",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusBadGateway))
		})
	})

	Context("happy path", func() {
		BeforeEach(func() {
			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA:  "abc123",
				CloneURL: "https://github.com/bborbe/repo.git",
				BaseRef:  "main",
			}, nil)
		})

		It("returns 200 with task_id", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusOK))
			var body map[string]interface{}
			//nolint:errcheck // test code; response body is controlled
			_ = json.Unmarshal(resp.Body.Bytes(), &body)
			Expect(body["task_id"]).ToNot(BeEmpty())
			Expect(body["repo"]).To(Equal("bborbe/repo"))
			Expect(body["pr_number"]).To(Equal(float64(42)))
			Expect(body["head_sha"]).To(Equal("abc123"))
		})

		It("calls createSender with correct task", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(createSender.SendCommandCallCount()).To(Equal(1))
		})
	})

	Context("trust-branching: untrusted author", func() {
		var sentCmd task.CreateCommand

		BeforeEach(func() {
			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA:     "abc123",
				CloneURL:    "https://github.com/bborbe/repo.git",
				BaseRef:     "main",
				AuthorLogin: "unknown-user",
				Title:       "Fix bug",
				IsDraft:     false,
			}, nil)
			trustDecision.IsTrustedReturns(trust.NewResult(false, "author not in allowlist"), nil)
			createSender.SendCommandStub = func(ctx context.Context, cmd task.CreateCommand) error {
				sentCmd = cmd
				return nil
			}
		})

		It("routes to human_review frontmatter (phase=human_review, status=todo)", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusOK))
			Expect(sentCmd.Frontmatter["phase"]).To(Equal("human_review"))
			Expect(sentCmd.Frontmatter["status"]).To(Equal("todo"))
		})
	})

	Context("trust-branching: trusted author", func() {
		var sentCmd task.CreateCommand

		BeforeEach(func() {
			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA:     "abc123",
				CloneURL:    "https://github.com/bborbe/repo.git",
				BaseRef:     "main",
				AuthorLogin: "bborbe",
				Title:       "Feature: add support",
				IsDraft:     false,
			}, nil)
			trustDecision.IsTrustedReturns(trust.NewResult(true, "trusted"), nil)
			createSender.SendCommandStub = func(ctx context.Context, cmd task.CreateCommand) error {
				sentCmd = cmd
				return nil
			}
		})

		It("routes to in_progress frontmatter (phase=planning, status=in_progress)", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusOK))
			Expect(sentCmd.Frontmatter["phase"]).To(Equal("planning"))
			Expect(sentCmd.Frontmatter["status"]).To(Equal("in_progress"))
		})
	})

	Context("CreateCommand.Validate boundary (AC spec line 107)", func() {
		// task.CreateCommand.Validate rejects titles containing /, :, ?, etc.
		// computePRTitle slugifies — this test guards against a slugifier regression
		// silently breaking every triggered review on real-world PR titles.
		It("constructed CreateCommand passes Validate for adversarial PR titles with /:?", func() {
			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA:     "abc123",
				CloneURL:    "https://github.com/bborbe/repo.git",
				BaseRef:     "main",
				AuthorLogin: "bborbe",
				Title:       "feat: handle /api?id=1 in :backend",
				IsDraft:     false,
				UpdatedAt:   libtime.DateTime(time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)),
			}, nil)

			var sentCmd task.CreateCommand
			createSender.SendCommandStub = func(_ context.Context, cmd task.CreateCommand) error {
				sentCmd = cmd
				return nil
			}

			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/77",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusOK))
			Expect(createSender.SendCommandCallCount()).To(Equal(1))
			Expect(sentCmd.Validate(ctx)).To(Succeed(),
				"Validate must succeed; if it fails, computePRTitle slugifier regressed or PRDetails fields leaked unsanitized into the command title/frontmatter")
		})
	})
})
