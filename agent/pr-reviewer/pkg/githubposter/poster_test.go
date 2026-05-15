// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubposter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/pr-reviewer/mocks"
	prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubposter"
)

const (
	testBotLogin = "pr-review-of-ben"
	testHeadSHA  = "sha123abc"
)

func makeHTTPResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func botUserJSON() string {
	return fmt.Sprintf(`{"login":%q}`, testBotLogin)
}

func reviewJSON(id int64, login, commitID, state string) string {
	return fmt.Sprintf(
		`{"id":%d,"user":{"login":%q},"commit_id":%q,"state":%q}`,
		id,
		login,
		commitID,
		state,
	)
}

func reviewListJSON(reviews ...string) string {
	if len(reviews) == 0 {
		return "[]"
	}
	return "[" + strings.Join(reviews, ",") + "]"
}

func postRespJSON(id int64) string {
	return fmt.Sprintf(`{"id":%d}`, id)
}

type callSpec struct {
	status int
	body   string
	err    error
}

func seqStub(specs []callSpec) func(*http.Request) (*http.Response, error) {
	idx := 0
	return func(req *http.Request) (*http.Response, error) {
		if idx >= len(specs) {
			return nil, fmt.Errorf("unexpected call %d: %s %s", idx, req.Method, req.URL.Path)
		}
		s := specs[idx]
		idx++
		if s.err != nil {
			return nil, s.err
		}
		return makeHTTPResp(s.status, s.body), nil
	}
}

var _ = Describe("PrPoster", func() {
	var (
		fakeClient *mocks.HTTPClient
		poster     prpkg.PrPoster
		pr         prpkg.PRInfo
		tmpDir     string
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = &mocks.HTTPClient{}
		poster = githubposter.NewPrPoster(fakeClient, "test-token", testBotLogin)
		pr = prpkg.PRInfo{Owner: "owner", Repo: "repo", Number: 1}
		var err error
		tmpDir, err = os.MkdirTemp("", "poster-test-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, tmpDir)
	})

	writeYAML := func(autoApprove bool) {
		content := fmt.Sprintf("autoApprove: %v\n", autoApprove)
		Expect(
			os.WriteFile(filepath.Join(tmpDir, ".pr-reviewer.yaml"), []byte(content), 0600),
		).To(Succeed())
	}

	happySpecs := func(state string) []callSpec {
		return []callSpec{
			{200, botUserJSON(), nil},
			{200, reviewListJSON(), nil},
			{201, postRespJSON(42), nil},
			{200, reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, state)), nil},
		}
	}

	DescribeTable("verdict to event/state mapping",
		func(verdict prpkg.Verdict, autoApprove bool, wantEvent, wantState, wantBodyPrefix string) {
			writeYAML(autoApprove)
			fakeClient.DoStub = seqStub(happySpecs(wantState))
			req := prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: verdict, Summary: "looks good", WorkDir: tmpDir,
			}
			result := poster.Post(ctx, req)
			Expect(result.Outcome).To(Equal("success"))
			Expect(result.PostedEvent).To(Equal(wantEvent))
			Expect(result.ReviewID).To(Equal(int64(42)))
			if wantBodyPrefix != "" {
				Expect(result.Warnings).To(BeEmpty())
			}
		},
		Entry("approve+autoApprove:true → APPROVE",
			prpkg.VerdictApprove, true, "APPROVE", "APPROVED", ""),
		Entry("approve+autoApprove:false → COMMENT",
			prpkg.VerdictApprove, false, "COMMENT", "COMMENTED",
			"auto-approve disabled for this repo"),
		Entry("request-changes → REQUEST_CHANGES",
			prpkg.VerdictRequestChanges, false, "REQUEST_CHANGES", "CHANGES_REQUESTED", ""),
	)

	DescribeTable("ErrorClass string values",
		func(class prpkg.ErrorClass, want string) {
			Expect(string(class)).To(Equal(want))
		},
		Entry("transient", prpkg.ErrorClassTransient, "transient"),
		Entry("permanent", prpkg.ErrorClassPermanent, "permanent"),
		Entry("unknown", prpkg.ErrorClassUnknown, "unknown"),
		Entry("not-a-failure", prpkg.ErrorClassNotAFailure, "not-a-failure"),
		Entry("soft-warning", prpkg.ErrorClassSoftWarning, "soft-warning"),
	)

	Context("bot identity mismatch", func() {
		It("returns permanent failure without posting", func() {
			fakeClient.DoReturns(makeHTTPResp(200, `{"login":"someone-else"}`), nil)
			req := prpkg.PostRequest{PR: pr, HeadSHA: testHeadSHA, WorkDir: tmpDir}
			result := poster.Post(ctx, req)
			Expect(result.Outcome).To(Equal("failed"))
			Expect(result.Class).To(Equal(prpkg.ErrorClassPermanent))
			Expect(result.EscalateHint).To(BeTrue())
			Expect(result.FailureStep).To(Equal("GET /user"))
			Expect(result.ErrorMessage).To(ContainSubstring("bot identity mismatch"))
			Expect(fakeClient.DoCallCount()).To(Equal(1))
		})
	})

	Context("dismissal before POST", func() {
		It("dismisses prior bot review then POSTs in that order", func() {
			writeYAML(true)
			priorReview := reviewJSON(99, testBotLogin, testHeadSHA, "APPROVED")
			fakeClient.DoStub = seqStub([]callSpec{
				{200, botUserJSON(), nil},
				{200, reviewListJSON(priorReview), nil},
				{200, `{}`, nil}, // PUT dismissal
				{201, postRespJSON(42), nil},
				{200, reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, "APPROVED")), nil},
			})
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove,
				Summary: "ok", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			invs := fakeClient.Invocations()["Do"]
			Expect(len(invs)).To(Equal(5))
			putReq, ok := invs[2][0].(*http.Request)
			Expect(ok).To(BeTrue())
			Expect(putReq.Method).To(Equal("PUT"))
			Expect(putReq.URL.Path).To(ContainSubstring("dismissals"))
			postReq, ok := invs[3][0].(*http.Request)
			Expect(ok).To(BeTrue())
			Expect(postReq.Method).To(Equal("POST"))
		})
	})

	Context("phantom POST → retry succeeds", func() {
		It("retries verify-GET and succeeds on second attempt", func() {
			writeYAML(true)
			fakeClient.DoStub = seqStub([]callSpec{
				{200, botUserJSON(), nil},
				{200, reviewListJSON(), nil},
				{201, postRespJSON(42), nil},
				{200, reviewListJSON(), nil}, // first verify: phantom (empty list)
				{200, reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, "APPROVED")), nil},
			})
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove,
				Summary: "ok", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			Expect(fakeClient.DoCallCount()).To(Equal(5))
		})
	})

	Context("phantom POST → exhausted retry", func() {
		It("returns transient failure after both verify attempts find no review", func() {
			writeYAML(true)
			fakeClient.DoStub = seqStub([]callSpec{
				{200, botUserJSON(), nil},
				{200, reviewListJSON(), nil},
				{201, postRespJSON(42), nil},
				{200, reviewListJSON(), nil}, // verify attempt 1: empty
				{200, reviewListJSON(), nil}, // verify attempt 2: still empty
			})
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove,
				Summary: "ok", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("failed"))
			Expect(result.Class).To(Equal(prpkg.ErrorClassTransient))
			Expect(result.FailureStep).To(Equal("GET /pulls/N/reviews (verify)"))
			Expect(result.ErrorMessage).To(ContainSubstring("phantom POST"))
		})
	})

	Context("POST 422 (PR closed)", func() {
		It("returns success with not-a-failure class and no verify-GET", func() {
			fakeClient.DoStub = seqStub([]callSpec{
				{200, botUserJSON(), nil},
				{200, reviewListJSON(), nil},
				{422, `{"message":"Unprocessable Entity"}`, nil},
			})
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove, WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			Expect(result.Class).To(Equal(prpkg.ErrorClassNotAFailure))
			Expect(result.HTTPStatus).To(Equal(422))
			Expect(fakeClient.DoCallCount()).To(Equal(3))
		})
	})

	Context("POST 403 permanent failure", func() {
		It("returns permanent failure without retry", func() {
			fakeClient.DoStub = seqStub([]callSpec{
				{200, botUserJSON(), nil},
				{200, reviewListJSON(), nil},
				{403, `{"message":"Forbidden"}`, nil},
			})
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove, WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("failed"))
			Expect(result.Class).To(Equal(prpkg.ErrorClassPermanent))
			Expect(result.EscalateHint).To(BeTrue())
			Expect(result.Attempt).To(Equal(1))
			Expect(fakeClient.DoCallCount()).To(Equal(3))
		})
	})

	Context("transient 5xx retry succeeds", func() {
		It("retries GET /user on 503 and continues to success", func() {
			writeYAML(true)
			fakeClient.DoStub = seqStub([]callSpec{
				{503, `service unavailable`, nil}, // GET /user attempt 1
				{200, botUserJSON(), nil},         // GET /user attempt 2
				{200, reviewListJSON(), nil},
				{201, postRespJSON(42), nil},
				{200, reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, "APPROVED")), nil},
			})
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove,
				Summary: "ok", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			Expect(fakeClient.DoCallCount()).To(Equal(5))
		})
	})

	Context("empty summary → soft-warning", func() {
		It("substitutes default summary and records warning but succeeds", func() {
			writeYAML(true)
			fakeClient.DoStub = seqStub(happySpecs("APPROVED"))
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove,
				Summary: "", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			Expect(result.Warnings).To(ContainElement(ContainSubstring("soft-warning")))
		})
	})

	Context("permanent dismissal failure", func() {
		It("stops after PUT dismissal fails and does not POST", func() {
			priorReview := reviewJSON(99, testBotLogin, testHeadSHA, "APPROVED")
			fakeClient.DoStub = seqStub([]callSpec{
				{200, botUserJSON(), nil},
				{200, reviewListJSON(priorReview), nil},
				{403, `{"message":"Forbidden"}`, nil}, // PUT dismissal fails
			})
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove, WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("failed"))
			Expect(result.Class).To(Equal(prpkg.ErrorClassPermanent))
			Expect(result.FailureStep).To(Equal("PUT .../dismissals"))
			Expect(fakeClient.DoCallCount()).To(Equal(3))
		})
	})

	Context("unknown class from non-JSON /user response", func() {
		It("returns unknown class and no retry", func() {
			fakeClient.DoReturns(makeHTTPResp(200, "not-json-at-all"), nil)
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("failed"))
			Expect(result.Class).To(Equal(prpkg.ErrorClassUnknown))
			Expect(result.EscalateHint).To(BeTrue())
			Expect(result.Attempt).To(Equal(1))
			Expect(fakeClient.DoCallCount()).To(Equal(1))
		})
	})

	Context("dismissal skips state=COMMENTED prior bot reviews", func() {
		const (
			commentedID  = int64(100)
			approvedID   = int64(101)
			changesReqID = int64(102)
			newReviewID  = int64(42)
		)

		BeforeEach(func() {
			fakeClient.DoStub = seqStub([]callSpec{
				{200, botUserJSON(), nil},
				// Three prior reviews by the bot on this SHA: COMMENTED, APPROVED, CHANGES_REQUESTED
				{200, reviewListJSON(
					reviewJSON(commentedID, testBotLogin, testHeadSHA, "COMMENTED"),
					reviewJSON(approvedID, testBotLogin, testHeadSHA, "APPROVED"),
					reviewJSON(changesReqID, testBotLogin, testHeadSHA, "CHANGES_REQUESTED"),
				), nil},
				{200, `{}`, nil}, // PUT dismissal for APPROVED
				{200, `{}`, nil}, // PUT dismissal for CHANGES_REQUESTED
				{201, postRespJSON(newReviewID), nil},
				{
					200,
					reviewListJSON(
						reviewJSON(newReviewID, testBotLogin, testHeadSHA, "CHANGES_REQUESTED"),
					),
					nil,
				},
			})
		})

		It(
			"returns success and dismisses exactly APPROVED and CHANGES_REQUESTED, not COMMENTED",
			func() {
				result := poster.Post(ctx, prpkg.PostRequest{
					PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictRequestChanges,
					Summary: "issues found", WorkDir: tmpDir,
				})
				Expect(result.Outcome).To(Equal("success"))
				// 6 calls: GET /user + GET /reviews (list) + 2x PUT dismissal + POST + GET /reviews (verify)
				Expect(fakeClient.DoCallCount()).To(Equal(6))

				invs := fakeClient.Invocations()["Do"]
				var dismissedPaths []string
				for _, call := range invs {
					req, ok := call[0].(*http.Request)
					Expect(ok).To(BeTrue())
					if req.Method == "PUT" && strings.Contains(req.URL.Path, "dismissals") {
						dismissedPaths = append(dismissedPaths, req.URL.Path)
					}
				}
				// Exactly 2 PUT dismissal calls (APPROVED + CHANGES_REQUESTED)
				Expect(dismissedPaths).To(HaveLen(2))
				// Both approved and changes-requested review IDs appear in the dismissed paths
				Expect(
					dismissedPaths,
				).To(ContainElement(ContainSubstring(fmt.Sprintf("%d", approvedID))))
				Expect(
					dismissedPaths,
				).To(ContainElement(ContainSubstring(fmt.Sprintf("%d", changesReqID))))
				// COMMENTED review ID must NOT appear in any dismissal call
				Expect(
					dismissedPaths,
				).NotTo(ContainElement(ContainSubstring(fmt.Sprintf("%d", commentedID))))
			},
		)
	})

	Context("POST request body shape", func() {
		It("sends correct JSON fields to GitHub", func() {
			writeYAML(true)
			var capturedBody []byte
			callIdx := 0
			fakeClient.DoStub = func(req *http.Request) (*http.Response, error) {
				idx := callIdx
				callIdx++
				if idx == 2 && req.Body != nil {
					b, _ := io.ReadAll(req.Body)
					capturedBody = b
					return makeHTTPResp(201, postRespJSON(42)), nil
				}
				bodies := []string{
					botUserJSON(), reviewListJSON(),
					"", // POST body captured above
					reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, "APPROVED")),
				}
				if idx < len(bodies) {
					return makeHTTPResp(200, bodies[idx]), nil
				}
				return nil, fmt.Errorf("unexpected call %d", idx)
			}
			result := poster.Post(ctx, prpkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: prpkg.VerdictApprove,
				Summary: "my review summary", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			Expect(capturedBody).NotTo(BeEmpty())
			var body map[string]interface{}
			Expect(json.Unmarshal(capturedBody, &body)).To(Succeed())
			Expect(body["event"]).To(Equal("APPROVE"))
			Expect(body["commit_id"]).To(Equal(testHeadSHA))
			Expect(body["body"]).To(Equal("my review summary"))
		})
	})
})
