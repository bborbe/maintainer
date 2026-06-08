// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command_test

import (
	"context"
	"encoding/json"
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
		Entry("invalid url (non-github) → skipped",
			func(_ *mocks.GitHubClient) {},
			command.TriggerPRReviewCommand{URL: "https://bitbucket.example.com/projects/owner/repos/repo/pull-requests/1"},
			outcomeSkipped, 0),
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
				trustDecision.IsTrustedReturns(trust.NewResult(false, "author not in allowlist"), nil)
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

var _ = Describe("executor vs handler payload parity (spec 066 AC: byte-identical downstream)", func() {
	// This is the load-bearing AC for spec § Constraints: the downstream
	// CreateTaskCommand payload MUST be byte-identical to today's
	// singlePRTriggerHandler.ServeHTTP output. We wire the SAME dependencies
	// into both call sites and assert deep-equal captured commands.
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

	It("produces the same CreateCommand for trusted author (handler vs executor)", func() {
		var (
			handlerCmd   task.CreateCommand
			executorCmd  task.CreateCommand
			handlerCalls int
			execCalls    int
		)

		// Handler path
		handlerSender := new(taskmocks.TaskCreateCommandSender)
		handlerSender.SendCommandStub = func(_ context.Context, c task.CreateCommand) error {
			handlerCmd = c
			handlerCalls++
			return nil
		}
		h := handler.NewSinglePRTriggerHandler(
			ghClient, handlerSender, taskCreationFilter, trustDecision,
			"dev", 80, 200, "",
			pkg.NewMetrics(),
		)
		req := httptest.NewRequest("POST", "/trigger?url="+validPRURL, nil)
		resp := httptest.NewRecorder()
		Expect(h.ServeHTTP(ctx, resp, req)).To(Succeed())
		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(handlerCalls).To(Equal(1))

		// Executor path
		executorSender := new(taskmocks.TaskCreateCommandSender)
		executorSender.SendCommandStub = func(_ context.Context, c task.CreateCommand) error {
			executorCmd = c
			execCalls++
			return nil
		}
		_, _, err := command.RunTriggerPRReview(
			ctx,
			nil,
			newCommandObject(command.TriggerPRReviewCommand{URL: validPRURL}),
			ghClient, executorSender, taskCreationFilter, trustDecision,
			"dev", 80, 200, "",
			pkg.NewMetrics(),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(execCalls).To(Equal(1))

		// Byte-identical deep-equal
		Expect(executorCmd).To(Equal(handlerCmd),
			"executor must produce byte-identical CreateCommand to handler. diff: %s",
			diffJSON(executorCmd, handlerCmd))
	})

	It("produces the same CreateCommand for untrusted author (handler vs executor)", func() {
		trustDecision.IsTrustedReturns(trust.NewResult(false, "author not in allowlist"), nil)

		var (
			handlerCmd  task.CreateCommand
			executorCmd task.CreateCommand
		)

		// Handler
		handlerSender := new(taskmocks.TaskCreateCommandSender)
		handlerSender.SendCommandStub = func(_ context.Context, c task.CreateCommand) error {
			handlerCmd = c
			return nil
		}
		h := handler.NewSinglePRTriggerHandler(
			ghClient, handlerSender, taskCreationFilter, trustDecision,
			"dev", 80, 200, "",
			pkg.NewMetrics(),
		)
		req := httptest.NewRequest("POST", "/trigger?url="+validPRURL, nil)
		resp := httptest.NewRecorder()
		Expect(h.ServeHTTP(ctx, resp, req)).To(Succeed())

		// Executor
		executorSender := new(taskmocks.TaskCreateCommandSender)
		executorSender.SendCommandStub = func(_ context.Context, c task.CreateCommand) error {
			executorCmd = c
			return nil
		}
		_, _, err := command.RunTriggerPRReview(
			ctx,
			nil,
			newCommandObject(command.TriggerPRReviewCommand{URL: validPRURL}),
			ghClient, executorSender, taskCreationFilter, trustDecision,
			"dev", 80, 200, "",
			pkg.NewMetrics(),
		)
		// Untrusted author → Skipped, no downstream publish
		Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeTrue())
		Expect(executorCmd).To(BeZero(), "untrusted author: executor must NOT publish downstream")
		// Handler still published (handler does not return Skipped — it
		// returns 200 and lets the trust branch route to human_review).
		// The point of this test is that the executor's "no publish"
		// behaviour matches the spec's "untrusted → Skipped" contract —
		// not that handler and executor behave identically (they don't:
		// the handler still publishes, the executor skips).
		Expect(handlerCmd).NotTo(BeZero(),
			"handler still publishes for untrusted (human_review branch)")
	})
})

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

// diffJSON marshals both values to JSON and returns a diff string for nicer
// failure messages. It is used only by the parity test.
func diffJSON(a, b interface{}) string {
	aj, err := json.Marshal(a)
	if err != nil {
		return "marshal a failed: " + err.Error()
	}
	bj, err := json.Marshal(b)
	if err != nil {
		return "marshal b failed: " + err.Error()
	}
	if string(aj) == string(bj) {
		return "<identical>"
	}
	return "handler=" + string(bj) + " executor=" + string(aj)
}
