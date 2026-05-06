// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	gogithub "github.com/google/go-github/v62/github"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-build/pkg"
)

var _ = Describe("pkg.GitHubClient", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	buildClient := func(server *httptest.Server) pkg.GitHubClient {
		ghc := gogithub.NewClient(server.Client())
		baseURL, _ := url.Parse(server.URL + "/")
		ghc.BaseURL = baseURL
		return pkg.NewForTest(ghc)
	}

	Describe("GetWorkflowRuns", func() {
		Context("server returns two completed workflow runs", func() {
			It("returns both runs with correct field mapping", func() {
				t1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
				t2 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprintf(w, `{
							"total_count": 2,
							"workflow_runs": [
								{
									"id": 1,
									"workflow_id": 101,
									"name": "CI",
									"head_sha": "abc123",
									"conclusion": "failure",
									"html_url": "https://github.com/owner/repo/actions/runs/1",
									"created_at": "%s"
								},
								{
									"id": 2,
									"workflow_id": 102,
									"name": "Build",
									"head_sha": "def456",
									"conclusion": "success",
									"html_url": "https://github.com/owner/repo/actions/runs/2",
									"created_at": "%s"
								}
							]
						}`, t1.Format(time.RFC3339), t2.Format(time.RFC3339))
					}),
				)
				defer server.Close()

				client := buildClient(server)
				result, err := client.GetWorkflowRuns(ctx, "owner", "repo", "main")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(2))

				Expect(result[0].WorkflowID).To(Equal(int64(101)))
				Expect(result[0].Name).To(Equal("CI"))
				Expect(result[0].HeadSHA).To(Equal("abc123"))
				Expect(result[0].Conclusion).To(Equal("failure"))
				Expect(result[0].HTMLURL).To(Equal("https://github.com/owner/repo/actions/runs/1"))
				Expect(result[0].CreatedAt).To(BeTemporally("~", t1, time.Second))

				Expect(result[1].WorkflowID).To(Equal(int64(102)))
				Expect(result[1].Conclusion).To(Equal("success"))
			})
		})

		Context("server returns a mix of completed and in-progress runs", func() {
			It("filters out runs with empty Conclusion", func() {
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprintf(w, `{
							"total_count": 3,
							"workflow_runs": [
								{
									"id": 1,
									"workflow_id": 201,
									"name": "CI",
									"head_sha": "sha1",
									"conclusion": "failure",
									"html_url": "https://github.com/owner/repo/actions/runs/1",
									"created_at": "2026-01-01T12:00:00Z"
								},
								{
									"id": 2,
									"workflow_id": 202,
									"name": "Deploy",
									"head_sha": "sha2",
									"conclusion": "",
									"html_url": "https://github.com/owner/repo/actions/runs/2",
									"created_at": "2026-01-01T12:00:00Z"
								},
								{
									"id": 3,
									"workflow_id": 203,
									"name": "Lint",
									"head_sha": "sha3",
									"conclusion": "success",
									"html_url": "https://github.com/owner/repo/actions/runs/3",
									"created_at": "2026-01-01T12:00:00Z"
								}
							]
						}`)
					}),
				)
				defer server.Close()

				client := buildClient(server)
				result, err := client.GetWorkflowRuns(ctx, "owner", "repo", "main")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(2))
				for _, run := range result {
					Expect(run.Conclusion).NotTo(BeEmpty())
				}
			})
		})

		Context("API returns HTTP error", func() {
			It("returns a non-nil error", func() {
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusUnauthorized)
						fmt.Fprintf(w, `{"message":"Bad credentials"}`)
					}),
				)
				defer server.Close()

				client := buildClient(server)
				_, err := client.GetWorkflowRuns(ctx, "owner", "repo", "main")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("GetDefaultBranch", func() {
		It("returns the default_branch field from the repository", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/repos/owner/repo"))
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"id": 1, "name": "repo", "default_branch": "master"}`)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			result, err := client.GetDefaultBranch(ctx, "owner", "repo")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("master"))
		})

		It("returns an error on HTTP failure", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(w, `{"message":"Not Found"}`)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			_, err := client.GetDefaultBranch(ctx, "owner", "repo")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetFileContent", func() {
		Context("file exists", func() {
			It("returns the decoded file content", func() {
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						Expect(r.URL.Path).To(Equal("/repos/owner/repo/contents/.maintenance.yaml"))
						w.Header().Set("Content-Type", "application/json")
						// "hello: world\n" base64-encoded
						fmt.Fprintf(
							w,
							`{"type":"file","encoding":"base64","content":"aGVsbG86IHdvcmxkCg==\n","size":14}`,
						)
					}),
				)
				defer server.Close()

				client := buildClient(server)
				content, err := client.GetFileContent(
					ctx,
					"owner",
					"repo",
					".maintenance.yaml",
					"main",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(content).NotTo(BeNil())
				Expect(string(content)).To(Equal("hello: world\n"))
			})
		})

		Context("file not found (404)", func() {
			It("returns (nil, nil) silently", func() {
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprintf(w, `{"message":"Not Found"}`)
					}),
				)
				defer server.Close()

				client := buildClient(server)
				content, err := client.GetFileContent(
					ctx,
					"owner",
					"repo",
					".maintenance.yaml",
					"main",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(content).To(BeNil())
			})
		})

		Context("server returns HTTP 500", func() {
			It("returns a non-nil error", func() {
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusInternalServerError)
						fmt.Fprintf(w, `{"message":"Internal Server Error"}`)
					}),
				)
				defer server.Close()

				client := buildClient(server)
				_, err := client.GetFileContent(ctx, "owner", "repo", ".maintenance.yaml", "main")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("path resolves to a directory (API returns array)", func() {
			It("returns (nil, nil)", func() {
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprintf(
							w,
							`[{"type":"file","name":"README.md","path":"README.md","size":42}]`,
						)
					}),
				)
				defer server.Close()

				client := buildClient(server)
				content, err := client.GetFileContent(ctx, "owner", "repo", "somedir", "main")
				Expect(err).NotTo(HaveOccurred())
				Expect(content).To(BeNil())
			})
		})
	})
})
