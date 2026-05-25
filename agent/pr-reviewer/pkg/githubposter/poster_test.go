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
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/pr-reviewer/mocks"
	pkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubposter"
	prpkg "github.com/bborbe/maintainer/lib/prurl"
)

const (
	testBotLogin = "ben-s-pull-request-reviewer[bot]"
	testHeadSHA  = "sha123abc"
	testPriorSHA = "sha-prior"
)

func makeHTTPResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
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

// redirectingHTTPClient proxies requests to api.github.com to a test server.
type redirectingHTTPClient struct {
	base    *http.Transport
	testURL *url.URL
}

func (c *redirectingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.Host, "api.github.com") {
		newReq := *req
		newReq.URL = &url.URL{
			Scheme: c.testURL.Scheme,
			Host:   c.testURL.Host,
			Path:   req.URL.Path,
		}
		newReq.Host = newReq.URL.Host
		return c.base.RoundTrip(&newReq)
	}
	return c.base.RoundTrip(req)
}

var _ = Describe("PrPoster", func() {
	var (
		fakeClient *mocks.HTTPClient
		poster     pkg.PrPoster
		pr         prpkg.PRInfo
		tmpDir     string
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = &mocks.HTTPClient{}
		currentDateTime := libtime.NewCurrentDateTime()
		poster = githubposter.NewPrPoster(fakeClient, "test-token", testBotLogin, currentDateTime)
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
			{200, reviewListJSON(), nil},
			{201, postRespJSON(42), nil},
			{200, reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, state)), nil},
		}
	}

	DescribeTable("verdict to event/state mapping",
		func(verdict pkg.Verdict, autoApprove bool, wantEvent, wantState, wantBodyPrefix string) {
			writeYAML(autoApprove)
			fakeClient.DoStub = seqStub(happySpecs(wantState))
			req := pkg.PostRequest{
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
			pkg.VerdictApprove, true, "APPROVE", "APPROVED", ""),
		Entry("approve+autoApprove:false → COMMENT",
			pkg.VerdictApprove, false, "COMMENT", "COMMENTED",
			"auto-approve disabled for this repo"),
		Entry("request-changes → REQUEST_CHANGES",
			pkg.VerdictRequestChanges, false, "REQUEST_CHANGES", "CHANGES_REQUESTED", ""),
	)

	DescribeTable("ErrorClass string values",
		func(class pkg.ErrorClass, want string) {
			Expect(string(class)).To(Equal(want))
		},
		Entry("transient", pkg.ErrorClassTransient, "transient"),
		Entry("permanent", pkg.ErrorClassPermanent, "permanent"),
		Entry("unknown", pkg.ErrorClassUnknown, "unknown"),
		Entry("not-a-failure", pkg.ErrorClassNotAFailure, "not-a-failure"),
		Entry("soft-warning", pkg.ErrorClassSoftWarning, "soft-warning"),
	)

	Context("dismissal before POST", func() {
		It("dismisses prior bot review then POSTs in that order", func() {
			writeYAML(true)
			priorReview := reviewJSON(99, testBotLogin, "sha-prior", "APPROVED")
			fakeClient.DoStub = seqStub([]callSpec{
				{200, reviewListJSON(priorReview), nil},
				{200, `{}`, nil}, // PUT dismissal
				{201, postRespJSON(42), nil},
				{200, reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, "APPROVED")), nil},
			})
			result := poster.Post(ctx, pkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictApprove,
				Summary: "ok", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			invs := fakeClient.Invocations()["Do"]
			Expect(len(invs)).To(Equal(4))
			putReq, ok := invs[1][0].(*http.Request)
			Expect(ok).To(BeTrue())
			Expect(putReq.Method).To(Equal("PUT"))
			Expect(putReq.URL.Path).To(ContainSubstring("dismissals"))
			postReq, ok := invs[2][0].(*http.Request)
			Expect(ok).To(BeTrue())
			Expect(postReq.Method).To(Equal("POST"))
		})
	})

	Context("phantom POST → retry succeeds", func() {
		It("retries verify-GET and succeeds on second attempt", func() {
			writeYAML(true)
			fakeClient.DoStub = seqStub([]callSpec{
				{200, reviewListJSON(), nil},
				{201, postRespJSON(42), nil},
				{200, reviewListJSON(), nil}, // first verify: phantom (empty list)
				{200, reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, "APPROVED")), nil},
			})
			result := poster.Post(ctx, pkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictApprove,
				Summary: "ok", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			Expect(fakeClient.DoCallCount()).To(Equal(4))
		})
	})

	Context("phantom POST → exhausted retry", func() {
		It("returns transient failure after both verify attempts find no review", func() {
			writeYAML(true)
			fakeClient.DoStub = seqStub([]callSpec{
				{200, reviewListJSON(), nil},
				{201, postRespJSON(42), nil},
				{200, reviewListJSON(), nil}, // verify attempt 1: empty
				{200, reviewListJSON(), nil}, // verify attempt 2: still empty
			})
			result := poster.Post(ctx, pkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictApprove,
				Summary: "ok", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("failed"))
			Expect(result.Class).To(Equal(pkg.ErrorClassTransient))
			Expect(result.FailureStep).To(Equal("GET /pulls/N/reviews (verify)"))
			Expect(result.ErrorMessage).To(ContainSubstring("phantom POST"))
		})
	})

	Context("POST 422 (PR closed)", func() {
		It("returns success with not-a-failure class and no verify-GET", func() {
			fakeClient.DoStub = seqStub([]callSpec{
				{200, reviewListJSON(), nil},
				{422, `{"message":"Unprocessable Entity"}`, nil},
			})
			result := poster.Post(ctx, pkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictApprove, WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			Expect(result.Class).To(Equal(pkg.ErrorClassNotAFailure))
			Expect(result.HTTPStatus).To(Equal(422))
			Expect(fakeClient.DoCallCount()).To(Equal(2))
		})
	})

	Context("POST 403 permanent failure", func() {
		It("returns permanent failure without retry", func() {
			fakeClient.DoStub = seqStub([]callSpec{
				{200, reviewListJSON(), nil},
				{403, `{"message":"Forbidden"}`, nil},
			})
			result := poster.Post(ctx, pkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictApprove, WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("failed"))
			Expect(result.Class).To(Equal(pkg.ErrorClassPermanent))
			Expect(result.EscalateHint).To(BeTrue())
			Expect(result.Attempt).To(Equal(1))
			Expect(fakeClient.DoCallCount()).To(Equal(2))
		})
	})

	Context("empty summary → soft-warning", func() {
		It("substitutes default summary and records warning but succeeds", func() {
			writeYAML(true)
			fakeClient.DoStub = seqStub(happySpecs("APPROVED"))
			result := poster.Post(ctx, pkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictApprove,
				Summary: "", WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"))
			Expect(result.Warnings).To(ContainElement(ContainSubstring("soft-warning")))
		})
	})

	Context("permanent dismissal failure", func() {
		It("stops after PUT dismissal fails and does not POST", func() {
			priorReview := reviewJSON(99, testBotLogin, "sha-prior", "APPROVED")
			fakeClient.DoStub = seqStub([]callSpec{
				{200, reviewListJSON(priorReview), nil},
				{403, `{"message":"Forbidden"}`, nil}, // PUT dismissal fails
			})
			result := poster.Post(ctx, pkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictApprove, WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("failed"))
			Expect(result.Class).To(Equal(pkg.ErrorClassPermanent))
			Expect(result.FailureStep).To(Equal("PUT .../dismissals"))
			Expect(fakeClient.DoCallCount()).To(Equal(2))
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
				// Three prior reviews by the bot on this SHA: COMMENTED, APPROVED, CHANGES_REQUESTED
				{200, reviewListJSON(
					reviewJSON(commentedID, testBotLogin, "sha-prior", "COMMENTED"),
					reviewJSON(approvedID, testBotLogin, "sha-prior", "APPROVED"),
					reviewJSON(changesReqID, testBotLogin, "sha-prior", "CHANGES_REQUESTED"),
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
				result := poster.Post(ctx, pkg.PostRequest{
					PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictRequestChanges,
					Summary: "issues found", WorkDir: tmpDir,
				})
				Expect(result.Outcome).To(Equal("success"))
				// 5 calls: GET /reviews (list) + 2x PUT dismissal + POST + GET /reviews (verify)
				Expect(fakeClient.DoCallCount()).To(Equal(5))

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
				if idx == 1 && req.Body != nil {
					b, _ := io.ReadAll(req.Body)
					capturedBody = b
					return makeHTTPResp(201, postRespJSON(42)), nil
				}
				bodies := []string{
					reviewListJSON(),
					"", // POST body captured above
					reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, "APPROVED")),
				}
				if idx < len(bodies) {
					return makeHTTPResp(200, bodies[idx]), nil
				}
				return nil, fmt.Errorf("unexpected call %d", idx)
			}
			result := poster.Post(ctx, pkg.PostRequest{
				PR: pr, HeadSHA: testHeadSHA, Verdict: pkg.VerdictApprove,
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

	DescribeTable("listBotReviews SHA filter — dismissal eligibility",
		func(inputReviews []string, expectedDismissedIDs []int64) {
			// Build the full HTTP call sequence:
			//   GET dismiss-list + N×PUT dismissal + POST + GET verify
			specs := make([]callSpec, 0, 1+len(expectedDismissedIDs)+2)
			specs = append(specs,
				callSpec{200, reviewListJSON(inputReviews...), nil},
			)
			for range expectedDismissedIDs {
				specs = append(specs, callSpec{200, `{}`, nil})
			}
			specs = append(specs, callSpec{201, postRespJSON(999), nil})
			specs = append(specs, callSpec{200, reviewListJSON(
				reviewJSON(999, testBotLogin, testHeadSHA, "CHANGES_REQUESTED"),
			), nil})

			fakeClient.DoStub = seqStub(specs)

			result := poster.Post(ctx, pkg.PostRequest{
				PR:      pr,
				HeadSHA: testHeadSHA,
				Verdict: pkg.VerdictRequestChanges,
				Summary: "test",
				WorkDir: tmpDir,
			})
			Expect(result.Outcome).To(Equal("success"),
				"expected full posting sequence to complete; got outcome=%s class=%s step=%s msg=%s",
				result.Outcome, result.Class, result.FailureStep, result.ErrorMessage,
			)

			// Collect dismissed review IDs from PUT /dismissals calls
			invs := fakeClient.Invocations()["Do"]
			var dismissedIDs []int64
			for _, call := range invs {
				req, ok := call[0].(*http.Request)
				Expect(ok).To(BeTrue())
				if req.Method == "PUT" && strings.Contains(req.URL.Path, "dismissals") {
					// URL.Path: /repos/owner/repo/pulls/1/reviews/<id>/dismissals
					// Split: ["", "repos", "owner", "repo", "pulls", "1", "reviews", "<id>", "dismissals"]
					parts := strings.Split(req.URL.Path, "/")
					if len(parts) == 9 && parts[8] == "dismissals" {
						var id int64
						_, _ = fmt.Sscanf(parts[7], "%d", &id)
						dismissedIDs = append(dismissedIDs, id)
					}
				}
			}
			if len(expectedDismissedIDs) == 0 {
				Expect(dismissedIDs).To(BeEmpty(),
					"expected no dismissals but got %v", dismissedIDs)
			} else {
				Expect(dismissedIDs).To(ConsistOf(expectedDismissedIDs))
			}
		},
		// Row A: two bot reviews — one at older SHA (dismissable), one at head SHA (preserved)
		Entry("Row A: older-SHA+head-SHA reviews → only older-SHA dismissed",
			[]string{
				reviewJSON(10, testBotLogin, testPriorSHA, "APPROVED"),
				reviewJSON(20, testBotLogin, testHeadSHA, "APPROVED"),
			},
			[]int64{10},
		),
		// Row B: single review at head SHA → nothing dismissed
		Entry("Row B: single review at head SHA → nothing dismissed",
			[]string{
				reviewJSON(30, testBotLogin, testHeadSHA, "APPROVED"),
			},
			[]int64{},
		),
		// Row C: two reviews both at head SHA → neither dismissed
		Entry("Row C: two reviews at head SHA → neither dismissed",
			[]string{
				reviewJSON(40, testBotLogin, testHeadSHA, "APPROVED"),
				reviewJSON(41, testBotLogin, testHeadSHA, "CHANGES_REQUESTED"),
			},
			[]int64{},
		),
		// Row D: COMMENTED at older SHA excluded by state filter; CHANGES_REQUESTED at older SHA dismissed
		Entry("Row D: COMMENTED+CHANGES_REQUESTED at older SHA → only CHANGES_REQUESTED dismissed",
			[]string{
				reviewJSON(50, testBotLogin, testPriorSHA, "COMMENTED"),
				reviewJSON(51, testBotLogin, testPriorSHA, "CHANGES_REQUESTED"),
			},
			[]int64{51},
		),
		// Row E: non-bot review at older SHA → never dismissed (botLogin filter)
		Entry("Row E: non-bot review at older SHA → nothing dismissed",
			[]string{
				reviewJSON(60, "someone-else", testPriorSHA, "APPROVED"),
			},
			[]int64{},
		),
		// Row F: empty review list → nothing dismissed
		Entry("Row F: empty review list → nothing dismissed",
			[]string{},
			[]int64{},
		),
	)

	Describe("*prPoster.PostLGTM (integration boundary)", func() {
		var (
			server *httptest.Server
		)

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		It(
			"posts a COMMENT review with the canonical LGTM body and reports success",
			func(ctx context.Context) {
				const botLogin = "ben-s-pull-request-reviewer-dev[bot]"
				const headSHA = "abc123def456abc123def456abc123def456abc1"

				var capturedBody string
				server = httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/app" {
							_, _ = w.Write([]byte(`{"slug":"ben-s-pull-request-reviewer-dev"}`))
							return
						}
						if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/reviews") {
							body, _ := io.ReadAll(r.Body)
							capturedBody = string(body)
							_, _ = w.Write([]byte(`{"id":99999}`))
							return
						}
						if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/reviews") {
							_, _ = w.Write(
								[]byte(
									`[{"id":99999,"user":{"login":"` + botLogin + `"},"commit_id":"` + headSHA + `","state":"COMMENTED"}]`,
								),
							)
							return
						}
						http.NotFound(w, r)
					}),
				)

				// Use a transport that redirects api.github.com to the test server.
				baseURL := server.URL
				testURL, _ := url.Parse(baseURL)
				transport := &http.Transport{}
				redirectingClient := &redirectingHTTPClient{base: transport, testURL: testURL}
				currentDateTime := libtime.NewCurrentDateTime()
				poster := githubposter.NewPrPoster(
					redirectingClient,
					"test-iat",
					botLogin,
					currentDateTime,
				)

				result := poster.PostLGTM(
					ctx,
					prpkg.PRInfo{Owner: "bborbe", Repo: "go-skeleton", Number: 99},
					headSHA,
					"",
					botLogin,
				)

				Expect(result.Outcome).To(Equal("success"))
				Expect(result.PostedEvent).To(Equal("COMMENT"))
				Expect(result.ReviewID).To(Equal(int64(99999)))
				// The load-bearing assertion: the actual POST body matches the LGTM template, NOT a typo.
				Expect(
					capturedBody,
				).To(MatchRegexp(`"body":\s*"Reviewed by ` + regexp.QuoteMeta(botLogin) + ` — no concerns flagged\."`))
				Expect(capturedBody).To(MatchRegexp(`"event":\s*"COMMENT"`))
				Expect(capturedBody).To(MatchRegexp(`"commit_id":\s*"` + headSHA + `"`))
			},
		)
	})
})
