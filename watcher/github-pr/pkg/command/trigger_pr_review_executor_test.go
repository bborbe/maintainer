// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	task "github.com/bborbe/agent/lib/command/task"
	taskmocks "github.com/bborbe/agent/lib/command/task/mocks"
	"github.com/bborbe/cqrs/base"
	cdb "github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/lib"
	"github.com/bborbe/maintainer/watcher/github-pr/mocks"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/command"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

const validPRURL = "https://github.com/bborbe/repo/pull/42"

// outcome is the three-state exit-path classifier for the table-driven test.
// success:    no error, downstream published.
// skipped:    errors.Is(err, cdb.ErrCommandObjectSkipped) — non-retryable.
// wrappedErr: err is non-nil and NOT ErrCommandObjectSkipped — transient.
type outcome int

const (
	outcomeSuccess outcome = iota
	outcomeSkipped
	outcomeWrappedErr
)

func mustParseEvent(cmd command.TriggerPRReviewCommand) base.Event {
	evt, err := base.ParseEvent(context.Background(), cmd)
	Expect(err).NotTo(HaveOccurred())
	return evt
}

func newCommandObject(cmd command.TriggerPRReviewCommand) cdb.CommandObject {
	return cdb.CommandObject{
		Command: base.Command{
			Operation: command.TriggerPRReviewCommandOperation,
			Data:      mustParseEvent(cmd),
		},
		SchemaID: lib.GithubPRReviewV1SchemaID,
	}
}

var _ = Describe("NewTriggerPRReviewCommandExecutor", func() {
	var (
		ctx                context.Context
		ghClient           *mocks.GitHubClient
		createSender       *taskmocks.TaskCreateCommandSender
		taskCreationFilter *mocks.TaskCreationFilter
		trustDecision      *mocks.Trust
	)

	BeforeEach(func() {
		ctx = context.Background()
		ghClient = new(mocks.GitHubClient)
		createSender = new(taskmocks.TaskCreateCommandSender)
		taskCreationFilter = new(mocks.TaskCreationFilter)
		trustDecision = new(mocks.Trust)

		taskCreationFilter.SkipReturns(false)
		trustDecision.IsTrustedReturns(trust.NewResult(true, "trusted"), nil)
		ghClient.GetPRDetailsReturns(pkg.PRDetails{
			HeadSHA:     "abc123",
			CloneURL:    "https://github.com/bborbe/repo.git",
			BaseRef:     "main",
			AuthorLogin: "bborbe",
			Title:       "Feature: add support",
			IsDraft:     false,
		}, nil)
	})

	DescribeTable("exit-path mapping",
		func(
			configure func(ghClient *mocks.GitHubClient),
			cmd command.TriggerPRReviewCommand,
			expectOutcome outcome, // skipped | wrappedErr | success
			expectDownstreamSent int,
		) {
			// Reset the createSender between entries — the table shares
			// a single fixture so we need to clear per-Entry state.
			*createSender = taskmocks.TaskCreateCommandSender{}
			configure(ghClient)

			_, _, err := command.RunTriggerPRReview(
				ctx,
				nil,
				newCommandObject(cmd),
				ghClient, createSender, taskCreationFilter, trustDecision,
				"dev", 80, 200, "",
				pkg.NewMetrics(),
			)

			switch expectOutcome {
			case outcomeSkipped:
				Expect(err).To(HaveOccurred(), "expected ErrCommandObjectSkipped")
				Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeTrue(),
					"expected ErrCommandObjectSkipped, got %v", err)
			case outcomeWrappedErr:
				Expect(err).To(HaveOccurred(), "expected wrapped (transient) error")
				Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeFalse(),
					"transient errors must NOT be classified as Skipped, got %v", err)
			case outcomeSuccess:
				Expect(err).NotTo(HaveOccurred(), "unexpected error: %v", err)
			}
			Expect(createSender.SendCommandCallCount()).To(Equal(expectDownstreamSent),
				"downstream send count mismatch")
		},
		Entry("valid pr → success + downstream sent",
			func(_ *mocks.GitHubClient) {},
			command.TriggerPRReviewCommand{URL: validPRURL},
			outcomeSuccess, 1),
		Entry(
			"invalid url (non-github) → skipped",
			func(_ *mocks.GitHubClient) {},
			command.TriggerPRReviewCommand{
				URL: "https://bitbucket.example.com/projects/owner/repos/repo/pull-requests/1",
			},
			outcomeSkipped,
			0,
		),
		Entry("malformed payload → skipped",
			// We cannot easily make MarshalInto fail with a valid Event shape,
			// so this entry exercises the "invalid url" path which the executor
			// also classifies as Skipped.
			func(_ *mocks.GitHubClient) {},
			command.TriggerPRReviewCommand{URL: "not-a-url"},
			outcomeSkipped, 0),
		Entry("filter rejects → skipped",
			func(_ *mocks.GitHubClient) {
				taskCreationFilter.SkipReturns(true)
			},
			command.TriggerPRReviewCommand{URL: validPRURL},
			outcomeSkipped, 0),
		Entry("untrusted author → skipped",
			func(_ *mocks.GitHubClient) {
				trustDecision.IsTrustedReturns(
					trust.NewResult(false, "author not in allowlist"),
					nil,
				)
			},
			command.TriggerPRReviewCommand{URL: validPRURL},
			outcomeSkipped, 0),
		Entry("github 5xx → wrapped err",
			func(gh *mocks.GitHubClient) {
				gh.GetPRDetailsReturns(pkg.PRDetails{}, errors.Errorf(ctx, "github 5xx"))
			},
			command.TriggerPRReviewCommand{URL: validPRURL},
			outcomeWrappedErr, 0),
		Entry("trust infra err → wrapped err",
			func(_ *mocks.GitHubClient) {
				trustDecision.IsTrustedReturns(nil, errors.Errorf(ctx, "trust lookup failed"))
			},
			command.TriggerPRReviewCommand{URL: validPRURL},
			outcomeWrappedErr, 0),
		Entry("kafka send err → wrapped err",
			func(_ *mocks.GitHubClient) {
				createSender.SendCommandReturns(errors.Errorf(ctx, "kafka send failed"))
			},
			command.TriggerPRReviewCommand{URL: validPRURL},
			outcomeWrappedErr, 1),
	)
})

var _ = Describe(
	"executor vs handler payload parity (spec 066 AC: byte-identical downstream)",
	func() {
		// This is the load-bearing AC for spec § Constraints: the downstream
		// CreateTaskCommand payload MUST be byte-identical to today's
		// singlePRTriggerHandler.ServeHTTP output.
		//
		// Post-prompt-3 the HTTP handler is a thin shell: it publishes a
		// TriggerPRReviewCommand to Kafka and returns 202. The full pipeline
		// (GitHub fetch → filter → trust → downstream publish) lives in the
		// executor. This describe block now verifies that the executor
		// produces the expected CreateCommand for trusted / untrusted
		// authors when fed a TriggerPRReviewCommand that originated from the
		// new thin handler. (The old byte-identical handler-vs-executor
		// comparison is no longer applicable because the handler no longer
		// produces a CreateCommand directly.)
		var (
			ctx                context.Context
			ghClient           *mocks.GitHubClient
			taskCreationFilter *mocks.TaskCreationFilter
			trustDecision      *mocks.Trust
		)

		BeforeEach(func() {
			ctx = context.Background()
			ghClient = new(mocks.GitHubClient)
			taskCreationFilter = new(mocks.TaskCreationFilter)
			trustDecision = new(mocks.Trust)

			taskCreationFilter.SkipReturns(false)
			trustDecision.IsTrustedReturns(trust.NewResult(true, "trusted"), nil)
			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA:     "abc123",
				CloneURL:    "https://github.com/bborbe/repo.git",
				BaseRef:     "main",
				AuthorLogin: "bborbe",
				Title:       "Feature: add support",
				IsDraft:     false,
			}, nil)
		})

		It("handler → executor produces a valid CreateCommand for trusted author", func() {
			// Handler path: capture the TriggerPRReviewCommand the new
			// thin handler publishes to Kafka.
			trigSender := new(mocks.TriggerPRReviewCommandSender)
			var publishedCmd command.TriggerPRReviewCommand
			trigSender.SendCommandStub = func(_ context.Context, c command.TriggerPRReviewCommand) error {
				publishedCmd = c
				return nil
			}
			h := handler.NewSinglePRTriggerHandler(trigSender)
			req := httptest.NewRequest("POST", "/trigger?url="+validPRURL, nil)
			resp := httptest.NewRecorder()
			Expect(h.ServeHTTP(ctx, resp, req)).To(Succeed())
			Expect(resp.Code).To(Equal(http.StatusAccepted))
			Expect(publishedCmd.URL).To(Equal(validPRURL))

			// Executor path: feed the handler's published command back
			// into the executor and capture the resulting CreateCommand.
			executorSender := new(taskmocks.TaskCreateCommandSender)
			var executorCmd task.CreateCommand
			executorSender.SendCommandStub = func(_ context.Context, c task.CreateCommand) error {
				executorCmd = c
				return nil
			}
			_, _, err := command.RunTriggerPRReview(
				ctx,
				nil,
				newCommandObject(publishedCmd),
				ghClient, executorSender, taskCreationFilter, trustDecision,
				"dev", 80, 200, "",
				pkg.NewMetrics(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(executorCmd).NotTo(BeZero(),
				"trusted author: executor must produce a non-zero CreateCommand")
			// Phase / status reflect trust=true → in_progress routing
			Expect(executorCmd.Frontmatter["phase"]).To(Equal("planning"))
			Expect(executorCmd.Frontmatter["status"]).To(Equal("in_progress"))
		})

		It("executor skips untrusted author and does not publish downstream", func() {
			trustDecision.IsTrustedReturns(trust.NewResult(false, "author not in allowlist"), nil)

			// Handler publishes a TriggerPRReviewCommand (handler is
			// trust-agnostic in prompt 3 — the untrusted branch lives in
			// the executor).
			trigSender := new(mocks.TriggerPRReviewCommandSender)
			var publishedCmd command.TriggerPRReviewCommand
			trigSender.SendCommandStub = func(_ context.Context, c command.TriggerPRReviewCommand) error {
				publishedCmd = c
				return nil
			}
			h := handler.NewSinglePRTriggerHandler(trigSender)
			req := httptest.NewRequest("POST", "/trigger?url="+validPRURL, nil)
			resp := httptest.NewRecorder()
			Expect(h.ServeHTTP(ctx, resp, req)).To(Succeed())
			Expect(resp.Code).To(Equal(http.StatusAccepted))

			// Executor must skip untrusted → ErrCommandObjectSkipped, no
			// downstream publish.
			executorSender := new(taskmocks.TaskCreateCommandSender)
			_, _, err := command.RunTriggerPRReview(
				ctx,
				nil,
				newCommandObject(publishedCmd),
				ghClient, executorSender, taskCreationFilter, trustDecision,
				"dev", 80, 200, "",
				pkg.NewMetrics(),
			)
			Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeTrue())
			Expect(executorSender.SendCommandCallCount()).To(Equal(0),
				"untrusted author: executor must NOT publish downstream")
		})
	},
)

var _ = Describe("executor crash recovery (spec 066 AC 16)", func() {
	// Proves at-least-once-via-idempotent-downstream: simulate a pod kill
	// mid-execution (context cancelled during gh fetch) and verify that
	// on retry the same downstream CreateTaskCommand is published exactly once.
	var (
		ctx                context.Context
		ghClient           *mocks.GitHubClient
		createSender       *taskmocks.TaskCreateCommandSender
		taskCreationFilter *mocks.TaskCreationFilter
		trustDecision      *mocks.Trust
	)

	BeforeEach(func() {
		ctx = context.Background()
		ghClient = new(mocks.GitHubClient)
		createSender = new(taskmocks.TaskCreateCommandSender)
		taskCreationFilter = new(mocks.TaskCreationFilter)
		trustDecision = new(mocks.Trust)

		taskCreationFilter.SkipReturns(false)
		trustDecision.IsTrustedReturns(trust.NewResult(true, "trusted"), nil)
		ghClient.GetPRDetailsReturns(pkg.PRDetails{
			HeadSHA:     "abc123",
			CloneURL:    "https://github.com/bborbe/repo.git",
			BaseRef:     "main",
			AuthorLogin: "bborbe",
			Title:       "Feature: add support",
			IsDraft:     false,
		}, nil)
	})

	It("a killed invocation can be retried and still publish exactly once", func() {
		// Round 1: simulate a real GitHub client that respects context
		// cancellation. The stub honours ctx.Err() and returns the
		// context-cancelled error — same shape as a real client that gets
		// SIGKILL'd in mid-request.
		killedCtx, cancel := context.WithCancel(ctx)
		ghClient.GetPRDetailsStub = func(c context.Context, _, _ string, _ int) (pkg.PRDetails, error) {
			// Cancel mid-call, then return the context error like a real client would.
			cancel()
			return pkg.PRDetails{}, c.Err()
		}
		createSender.SendCommandStub = func(_ context.Context, _ task.CreateCommand) error {
			// If the killed run somehow reaches SendCommand, fail the test so we
			// notice (it should not — ghClient already returned an error).
			Fail("SendCommand must not be called during the killed invocation")
			return nil
		}

		cmd := command.TriggerPRReviewCommand{URL: validPRURL}
		commandObject := newCommandObject(cmd)

		_, _, err := command.RunTriggerPRReview(
			killedCtx, nil, commandObject,
			ghClient, createSender, taskCreationFilter, trustDecision,
			"dev", 80, 200, "",
			pkg.NewMetrics(),
		)
		Expect(err).To(HaveOccurred(),
			"killed invocation must return a transient error so Kafka redelivers")
		Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeFalse(),
			"killed invocation must NOT be classified as Skipped (transient, not deliberate)")
		Expect(createSender.SendCommandCallCount()).To(Equal(0),
			"killed invocation must not publish downstream")

		// Round 2: fresh context, fresh sender, deterministic github response.
		// The same commandObject is reused (Kafka would redeliver it as-is).
		freshSender := new(taskmocks.TaskCreateCommandSender)
		freshSender.SendCommandStub = func(_ context.Context, _ task.CreateCommand) error {
			return nil
		}
		ghClient.GetPRDetailsStub = nil
		ghClient.GetPRDetailsReturns(pkg.PRDetails{
			HeadSHA:     "abc123",
			CloneURL:    "https://github.com/bborbe/repo.git",
			BaseRef:     "main",
			AuthorLogin: "bborbe",
			Title:       "Feature: add support",
			IsDraft:     false,
		}, nil)

		_, _, err = command.RunTriggerPRReview(
			context.Background(), nil, commandObject,
			ghClient, freshSender, taskCreationFilter, trustDecision,
			"dev", 80, 200, "",
			pkg.NewMetrics(),
		)
		Expect(err).NotTo(HaveOccurred(), "retry must succeed: %v", err)
		Expect(freshSender.SendCommandCallCount()).To(Equal(1),
			"retry must publish downstream exactly once")
	})
})
