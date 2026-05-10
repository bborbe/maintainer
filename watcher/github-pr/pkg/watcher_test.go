// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	agentlib "github.com/bborbe/agent/lib"
	task "github.com/bborbe/agent/lib/command/task"
	taskmocks "github.com/bborbe/agent/lib/command/task/mocks"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/mocks"
	"github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
)

func newTestWatcher(
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	updateFrontmatterSender task.UpdateFrontmatterCommandSender,
	cursorPath string,
	startTime libtime.DateTime,
	fakeMetrics *mocks.Metrics,
	trustDecision trust.Trust,
) pkg.Watcher {
	return pkg.NewWatcher(
		ghClient,
		createSender,
		updateFrontmatterSender,
		cursorPath,
		startTime,
		"bborbe",
		filter.TaskCreationFilters{
			filter.NewDraftFilter(),
			filter.NewBotAuthorFilter([]string{"dependabot[bot]"}),
		},
		"dev",
		fakeMetrics,
		trustDecision,
	)
}

var _ = Describe("pkg.Watcher", func() {
	var (
		ctx                     context.Context
		cancel                  context.CancelFunc
		ghClient                *mocks.GitHubClient
		createSender            *taskmocks.TaskCreateCommandSender
		updateFrontmatterSender *taskmocks.TaskUpdateFrontmatterCommandSender
		fakeMetrics             *mocks.Metrics
		tmpDir                  string
		cursorPath              string
		startTime               libtime.DateTime
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		ghClient = new(mocks.GitHubClient)
		createSender = new(taskmocks.TaskCreateCommandSender)
		updateFrontmatterSender = new(taskmocks.TaskUpdateFrontmatterCommandSender)
		fakeMetrics = new(mocks.Metrics)
		startTime = libtime.DateTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		var err error
		tmpDir, err = os.MkdirTemp("", "watcher-test-*")
		Expect(err).NotTo(HaveOccurred())
		cursorPath = filepath.Join(tmpDir, "cursor.json")
	})

	AfterEach(func() {
		cancel()
		_ = os.RemoveAll(tmpDir) // #nosec G104 -- best-effort temp dir cleanup
	})

	Describe("No PRs returned", func() {
		It("returns nil, cursor saved", func() {
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  nil,
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			err := w.Poll(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(updateFrontmatterSender.SendCommandCallCount()).To(Equal(0))
			_, err = os.Stat(cursorPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeMetrics.IncPollCycleCallCount()).To(Equal(1))
			result := fakeMetrics.IncPollCycleArgsForCall(0)
			Expect(result).To(Equal("success"))
		})
	})

	Describe("New PR (no existing cursor entry)", func() {
		It("publishes CreateTaskCommand", func() {
			pr := pkg.PullRequest{
				Number:      42,
				Owner:       "bborbe",
				Repo:        "code-reviewer",
				Title:       "feat: new feature",
				HTMLURL:     "https://github.com/bborbe/maintainer/pull/42",
				AuthorLogin: "alice",
				IsDraft:     false,
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "abc123",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			err := w.Poll(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(createSender.SendCommandCallCount()).To(Equal(1))
			Expect(updateFrontmatterSender.SendCommandCallCount()).To(Equal(0))

			_, cmd := createSender.SendCommandArgsForCall(0)
			Expect(string(cmd.TaskIdentifier)).NotTo(BeEmpty())
			Expect(cmd.Frontmatter["assignee"]).To(Equal("pr-reviewer-agent"))
			Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
			Expect(
				cmd.Title,
			).To(Equal("PR Review github - bborbe-code-reviewer - 42 - feat-new-feature"))
			Expect(fakeMetrics.IncPRPublishedCallCount()).To(Equal(1))
			command := fakeMetrics.IncPRPublishedArgsForCall(0)
			Expect(command).To(Equal("create"))
		})
	})

	Describe("Existing PR, same SHA", func() {
		It("publishes nothing", func() {
			pr := pkg.PullRequest{
				Number:      42,
				Owner:       "bborbe",
				Repo:        "code-reviewer",
				Title:       "existing pr",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha-existing",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)

			// Pre-populate cursor with the same SHA
			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			// First poll: creates the entry
			createSender.SendCommandReturns(nil)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(1))

			// Second poll: same SHA, no publish
			createSender = new(taskmocks.TaskCreateCommandSender)
			updateFrontmatterSender = new(taskmocks.TaskUpdateFrontmatterCommandSender)
			w = newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(updateFrontmatterSender.SendCommandCallCount()).To(Equal(0))
		})
	})

	Describe("Force-push (existing entry, different SHA)", func() {
		It("publishes UpdateFrontmatterCommand with correct body section", func() {
			pr := pkg.PullRequest{
				Number:      42,
				Owner:       "bborbe",
				Repo:        "code-reviewer",
				Title:       "force pushed pr",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}

			// First poll: register initial SHA
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "old-sha",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(1))

			// Second poll: new SHA (force-push)
			createSender = new(taskmocks.TaskCreateCommandSender)
			updateFrontmatterSender = new(taskmocks.TaskUpdateFrontmatterCommandSender)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "new-sha",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			updateFrontmatterSender.SendCommandReturns(nil)

			w = newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())

			Expect(updateFrontmatterSender.SendCommandCallCount()).To(Equal(1))
			_, cmd := updateFrontmatterSender.SendCommandArgsForCall(0)
			Expect(cmd.Body).NotTo(BeNil())
			Expect(cmd.Body.Heading).To(ContainSubstring("## Outdated by force-push old-sha"))
			Expect(cmd.Updates["task_type"]).To(Equal("pr-review"))
			Expect(cmd.Updates["assignee"]).To(Equal("pr-reviewer-agent"))
			Expect(cmd.Updates["phase"]).To(Equal("planning"))
		})
	})

	Describe("Draft PR", func() {
		It("is skipped, no publish calls", func() {
			pr := pkg.PullRequest{
				Number:      1,
				Owner:       "bborbe",
				Repo:        "repo",
				AuthorLogin: "alice",
				IsDraft:     true,
				UpdatedAt:   libtime.DateTime(fixedNow),
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(updateFrontmatterSender.SendCommandCallCount()).To(Equal(0))
		})
	})

	Describe("Bot-authored PR", func() {
		It("is skipped, no publish calls", func() {
			pr := pkg.PullRequest{
				Number:      2,
				Owner:       "bborbe",
				Repo:        "repo",
				AuthorLogin: "dependabot[bot]",
				IsDraft:     false,
				UpdatedAt:   libtime.DateTime(fixedNow),
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
		})
	})

	Describe("GitHub error", func() {
		It("Poll returns nil, cursor unchanged, no publish calls", func() {
			ghClient.SearchPRsReturns(pkg.SearchResult{}, errors.New("network timeout"))

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			err := w.Poll(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			// pkg.Cursor file should not exist (no save after error)
			_, statErr := os.Stat(cursorPath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
			Expect(fakeMetrics.IncPollCycleCallCount()).To(Equal(1))
			result := fakeMetrics.IncPollCycleArgsForCall(0)
			Expect(result).To(Equal("github_error"))
		})
	})

	Describe("Kafka publish fails (CreateTaskCommand)", func() {
		It("Poll returns nil, cursor not updated for that PR", func() {
			pr := pkg.PullRequest{
				Number:      99,
				Owner:       "bborbe",
				Repo:        "repo",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)),
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha123",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(errors.New("kafka unavailable"))

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			err := w.Poll(ctx)
			Expect(err).NotTo(HaveOccurred())

			// HeadSHAs should not include this PR since publish failed
			// Verify by doing a second poll and checking SendCommand is called again
			createSender2 := new(taskmocks.TaskCreateCommandSender)
			updateFrontmatterSender2 := new(taskmocks.TaskUpdateFrontmatterCommandSender)
			createSender2.SendCommandReturns(nil)
			w2 := newTestWatcher(
				ghClient,
				createSender2,
				updateFrontmatterSender2,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender2.SendCommandCallCount()).To(Equal(1))
		})
	})

	Describe("Context cancellation between pages", func() {
		It("Poll returns nil cleanly", func() {
			cancelCtx, cancelFn := context.WithCancel(context.Background())

			callCount := 0
			ghClient.SearchPRsStub = func(c context.Context, scope string, since libtime.DateTime, page int) (pkg.SearchResult, error) {
				callCount++
				if callCount == 1 {
					cancelFn() // cancel context after first page
					return pkg.SearchResult{
						PullRequests:  nil,
						HasNextPage:   true,
						NextPage:      2,
						RateRemaining: 100,
					}, nil
				}
				return pkg.SearchResult{}, nil
			}

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			err := w.Poll(cancelCtx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("pkg.Cursor file missing on first poll", func() {
		It("uses startTime as since, calls SearchPRs correctly", func() {
			ghClient.SearchPRsReturns(pkg.SearchResult{
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())

			Expect(ghClient.SearchPRsCallCount()).To(Equal(1))
			_, _, since, _ := ghClient.SearchPRsArgsForCall(0)
			Expect(since).To(Equal(startTime))
		})
	})

	Describe("LoadCursor error (unreadable cursor file)", func() {
		It("Poll returns a non-nil error", func() {
			if os.Getuid() == 0 {
				Skip("running as root, skipping permission test")
			}
			Expect(os.WriteFile(cursorPath, []byte("{}"), 0600)).To(Succeed())
			Expect(os.Chmod(cursorPath, 0000)).To(Succeed())
			defer func() { _ = os.Chmod(cursorPath, 0600) }()

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			err := w.Poll(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Closed PR pruned from cursor after poll", func() {
		It("removes closed PR's task ID from HeadSHAs after second poll", func() {
			prA := pkg.PullRequest{
				Number:      42,
				Owner:       "bborbe",
				Repo:        "code-reviewer",
				Title:       "pr A",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}
			prB := pkg.PullRequest{
				Number:      43,
				Owner:       "bborbe",
				Repo:        "code-reviewer",
				Title:       "pr B",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}

			// First poll: both PRs open
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{prA, prB},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha-initial",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(2))

			// Second poll: only PR A returned (PR B closed/merged)
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{prA},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			createSender2 := new(taskmocks.TaskCreateCommandSender)
			updateFrontmatterSender2 := new(taskmocks.TaskUpdateFrontmatterCommandSender)
			createSender2.SendCommandReturns(nil)
			w2 := newTestWatcher(
				ghClient,
				createSender2,
				updateFrontmatterSender2,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w2.Poll(ctx)).NotTo(HaveOccurred())

			cursor, err := pkg.LoadCursor(ctx, cursorPath, startTime)
			Expect(err).NotTo(HaveOccurred())

			taskIDA := pkg.DeriveTaskID(prA.Owner, prA.Repo, prA.Number).String()
			taskIDB := pkg.DeriveTaskID(prB.Owner, prB.Repo, prB.Number).String()
			Expect(cursor.HeadSHAs).To(HaveKey(taskIDA))
			Expect(cursor.HeadSHAs).NotTo(HaveKey(taskIDB))
		})
	})

	Describe("publishForcePush Kafka publish error", func() {
		It("does not update cursor SHA to new SHA, Poll returns nil", func() {
			pr := pkg.PullRequest{
				Number:      55,
				Owner:       "bborbe",
				Repo:        "repo",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}

			// First poll: register initial SHA via create
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha-v1",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(1))

			// Second poll: force-push detected, but Kafka publish fails
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha-v2",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender2 := new(taskmocks.TaskCreateCommandSender)
			updateFrontmatterSender2 := new(taskmocks.TaskUpdateFrontmatterCommandSender)
			updateFrontmatterSender2.SendCommandReturns(errors.New("kafka unavailable"))
			w2 := newTestWatcher(
				ghClient,
				createSender2,
				updateFrontmatterSender2,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
			Expect(updateFrontmatterSender2.SendCommandCallCount()).To(Equal(1))

			// Cursor SHA must NOT be updated to sha-v2 (the new SHA) after a failed publish
			cursor, err := pkg.LoadCursor(ctx, cursorPath, startTime)
			Expect(err).NotTo(HaveOccurred())
			taskIDStr := pkg.DeriveTaskID(pr.Owner, pr.Repo, pr.Number).String()
			Expect(cursor.HeadSHAs[taskIDStr]).NotTo(Equal("sha-v2"))
		})
	})

	Describe("fetchHeadSHA returns an error", func() {
		It("Poll returns nil, SendCommand never called", func() {
			pr := pkg.PullRequest{
				Number:      77,
				Owner:       "bborbe",
				Repo:        "repo",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(pkg.PRDetails{}, errors.New("github api error"))

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))

			// Cursor LastUpdatedAt should not advance past startTime
			cursor, err := pkg.LoadCursor(ctx, cursorPath, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(cursor.LastUpdatedAt).To(Equal(startTime))
		})
	})

	Describe("fetchHeadSHA cache hit with duplicate task ID", func() {
		It("calls GetPRDetails once and SendCommand once for same PR twice in results", func() {
			pr1 := pkg.PullRequest{
				Number:      10,
				Owner:       "bborbe",
				Repo:        "repo",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}
			// Identical Owner/Repo/Number → same task ID
			pr2 := pr1

			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr1, pr2},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha-dedup",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())

			Expect(ghClient.GetPRDetailsCallCount()).To(Equal(1))
			Expect(createSender.SendCommandCallCount()).To(Equal(1))
		})
	})

	Describe("pkg.Cursor save fails", func() {
		It("Poll returns nil (non-crash)", func() {
			ghClient.SearchPRsReturns(pkg.SearchResult{
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)

			// Use an unwritable path
			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				"/nonexistent/path/cursor.json",
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			err := w.Poll(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("GetPRDetails caches result", func() {
		It("calls GetPRDetails once per unique PR", func() {
			prs := []pkg.PullRequest{
				{
					Number:      1,
					Owner:       "bborbe",
					Repo:        "repo",
					AuthorLogin: "alice",
					UpdatedAt:   libtime.DateTime(fixedNow),
				},
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  prs,
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha1",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(ghClient.GetPRDetailsCallCount()).To(Equal(1))
		})
	})

	Describe("buildFrontmatter fields", func() {
		It("includes required keys", func() {
			pr := pkg.PullRequest{
				Number:      5,
				Owner:       "bborbe",
				Repo:        "repo",
				Title:       "my title",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(fixedNow),
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha1",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())

			Expect(createSender.SendCommandCallCount()).To(Equal(1))
			_, cmd := createSender.SendCommandArgsForCall(0)
			Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
			Expect(cmd.Frontmatter["assignee"]).To(Equal("pr-reviewer-agent"))
			Expect(cmd.Frontmatter["phase"]).To(Equal("planning"))
			Expect(cmd.Frontmatter["status"]).To(Equal("in_progress"))
			Expect(cmd.Frontmatter["stage"]).To(Equal("dev"))
			Expect(cmd.Frontmatter["title"]).To(Equal("my title"))
			Expect(cmd.Frontmatter["task_identifier"]).NotTo(BeEmpty())
			Expect(cmd.Title).To(Equal("PR Review github - bborbe-repo - 5 - my-title"))
		})
	})

	Describe("UpdateFrontmatterCommand fields", func() {
		It("body section heading matches exact format", func() {
			pr := pkg.PullRequest{
				Number:      7,
				Owner:       "bborbe",
				Repo:        "repo",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			}

			// First poll: create
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha-v1",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			createSender.SendCommandReturns(nil)
			w := newTestWatcher(
				ghClient,
				createSender,
				updateFrontmatterSender,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())

			// Second poll: force-push
			createSender2 := new(taskmocks.TaskCreateCommandSender)
			updateFrontmatterSender2 := new(taskmocks.TaskUpdateFrontmatterCommandSender)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha-v2",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
			updateFrontmatterSender2.SendCommandReturns(nil)
			w2 := newTestWatcher(
				ghClient,
				createSender2,
				updateFrontmatterSender2,
				cursorPath,
				startTime,
				fakeMetrics,
				trust.NewAuthorAllowlist([]string{"alice"}),
			)
			Expect(w2.Poll(ctx)).NotTo(HaveOccurred())

			Expect(updateFrontmatterSender2.SendCommandCallCount()).To(Equal(1))
			_, cmd := updateFrontmatterSender2.SendCommandArgsForCall(0)
			Expect(cmd.Body.Heading).To(Equal("## Outdated by force-push sha-v1"))
			Expect(cmd.Body.Section).To(Equal("## Outdated by force-push sha-v1\n"))
			Expect(cmd.Updates["task_type"]).To(Equal("pr-review"))
			Expect(cmd.Updates["assignee"]).To(Equal("pr-reviewer-agent"))
			Expect(cmd.Updates["phase"]).To(Equal("planning"))
			Expect(cmd.Updates["status"]).To(Equal("in_progress"))
			Expect(cmd.Updates["trigger_count"]).To(Equal(0))
			taskID := agentlib.TaskIdentifier("")
			_, cmd2 := updateFrontmatterSender2.SendCommandArgsForCall(0)
			taskID = cmd2.TaskIdentifier
			Expect(string(taskID)).NotTo(BeEmpty())
		})
	})

	Describe("Trust decisions", func() {
		var (
			pr pkg.PullRequest
		)

		BeforeEach(func() {
			pr = pkg.PullRequest{
				Number:      10,
				Owner:       "bborbe",
				Repo:        "repo",
				Title:       "some PR",
				HTMLURL:     "https://github.com/bborbe/repo/pull/10",
				AuthorLogin: "alice",
				UpdatedAt:   libtime.DateTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)),
			}
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests:  []pkg.PullRequest{pr},
				HasNextPage:   false,
				RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{
					HeadSHA:  "sha1",
					CloneURL: "https://github.com/owner/repo.git",
					BaseRef:  "master",
				},
				nil,
			)
		})

		Describe("Trusted-author new PR", func() {
			It("publishes CreateTaskCommand with planning/in_progress frontmatter", func() {
				createSender.SendCommandReturns(nil)
				w := newTestWatcher(
					ghClient,
					createSender,
					updateFrontmatterSender,
					cursorPath,
					startTime,
					fakeMetrics,
					trust.NewAuthorAllowlist([]string{"alice"}),
				)
				Expect(w.Poll(ctx)).NotTo(HaveOccurred())
				Expect(createSender.SendCommandCallCount()).To(Equal(1))
				_, cmd := createSender.SendCommandArgsForCall(0)
				Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
				Expect(cmd.Frontmatter["assignee"]).To(Equal("pr-reviewer-agent"))
				Expect(cmd.Frontmatter["phase"]).To(Equal("planning"))
				Expect(cmd.Frontmatter["status"]).To(Equal("in_progress"))
				Expect(cmd.Title).To(Equal("PR Review github - bborbe-repo - 10 - some-pr"))
			})
		})

		Describe("Untrusted-author new PR", func() {
			It(
				"publishes CreateTaskCommand with human_review/todo frontmatter and untrusted body",
				func() {
					createSender.SendCommandReturns(nil)
					w := newTestWatcher(
						ghClient,
						createSender,
						updateFrontmatterSender,
						cursorPath,
						startTime,
						fakeMetrics,
						trust.NewAuthorAllowlist([]string{"bob"}),
					)
					Expect(w.Poll(ctx)).NotTo(HaveOccurred())
					Expect(createSender.SendCommandCallCount()).To(Equal(1))
					_, cmd := createSender.SendCommandArgsForCall(0)
					Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
					Expect(cmd.Frontmatter["assignee"]).To(Equal(""))
					Expect(cmd.Frontmatter["phase"]).To(Equal("human_review"))
					Expect(cmd.Frontmatter["status"]).To(Equal("todo"))
					Expect(cmd.Body).To(ContainSubstring("alice"))
					Expect(cmd.Body).To(ContainSubstring("Untrusted author"))
					Expect(cmd.Body).To(ContainSubstring("phase: in_progress"))
					Expect(
						cmd.Title,
					).To(Equal("PR Review github - bborbe-repo - 10 - some-pr"))
				},
			)
		})

		Describe("Untrusted-author force-push", func() {
			It("re-evaluates trust and preserves human_review/todo state", func() {
				createSender.SendCommandReturns(nil)
				w := newTestWatcher(
					ghClient,
					createSender,
					updateFrontmatterSender,
					cursorPath,
					startTime,
					fakeMetrics,
					trust.NewAuthorAllowlist([]string{"bob"}),
				)
				Expect(w.Poll(ctx)).NotTo(HaveOccurred())
				Expect(createSender.SendCommandCallCount()).To(Equal(1))

				createSender2 := new(taskmocks.TaskCreateCommandSender)
				updateFrontmatterSender2 := new(taskmocks.TaskUpdateFrontmatterCommandSender)
				ghClient.GetPRDetailsReturns(
					pkg.PRDetails{
						HeadSHA:  "sha2",
						CloneURL: "https://github.com/owner/repo.git",
						BaseRef:  "master",
					},
					nil,
				)
				updateFrontmatterSender2.SendCommandReturns(nil)
				w2 := newTestWatcher(
					ghClient,
					createSender2,
					updateFrontmatterSender2,
					cursorPath,
					startTime,
					fakeMetrics,
					trust.NewAuthorAllowlist([]string{"bob"}),
				)
				Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
				Expect(updateFrontmatterSender2.SendCommandCallCount()).To(Equal(1))
				_, cmd := updateFrontmatterSender2.SendCommandArgsForCall(0)
				Expect(cmd.Updates["task_type"]).To(Equal("pr-review"))
				Expect(cmd.Updates["assignee"]).To(Equal(""))
				Expect(cmd.Updates["phase"]).To(Equal("human_review"))
				Expect(cmd.Updates["status"]).To(Equal("todo"))
			})
		})

		Describe("Trust check returns an error", func() {
			It(
				"skips the PR, no publish, Poll returns nil, cursor NOT advanced past failed PR",
				func() {
					fakeErr := errors.New("github rate limit")
					fakeTrust := new(mocks.Trust)
					fakeTrust.IsTrustedReturns(nil, fakeErr)
					w := newTestWatcher(
						ghClient,
						createSender,
						updateFrontmatterSender,
						cursorPath,
						startTime,
						fakeMetrics,
						fakeTrust,
					)
					Expect(w.Poll(ctx)).NotTo(HaveOccurred())
					Expect(createSender.SendCommandCallCount()).To(Equal(0))
					taskIDStr := pkg.DeriveTaskID(pr.Owner, pr.Repo, pr.Number).String()
					cursor, loadErr := pkg.LoadCursor(ctx, cursorPath, startTime)
					Expect(loadErr).NotTo(HaveOccurred())
					Expect(cursor.HeadSHAs).NotTo(HaveKey(taskIDStr))
				},
			)
		})

		Describe("PR with missing AuthorLogin (defensive)", func() {
			It("treats as untrusted and publishes human_review task", func() {
				pr.AuthorLogin = ""
				ghClient.SearchPRsReturns(pkg.SearchResult{
					PullRequests:  []pkg.PullRequest{pr},
					HasNextPage:   false,
					RateRemaining: 100,
				}, nil)
				createSender.SendCommandReturns(nil)
				w := newTestWatcher(
					ghClient,
					createSender,
					updateFrontmatterSender,
					cursorPath,
					startTime,
					fakeMetrics,
					trust.NewAuthorAllowlist([]string{"alice"}),
				)
				Expect(w.Poll(ctx)).NotTo(HaveOccurred())
				Expect(createSender.SendCommandCallCount()).To(Equal(1))
				_, cmd := createSender.SendCommandArgsForCall(0)
				Expect(cmd.Frontmatter["task_type"]).To(Equal("pr-review"))
				Expect(cmd.Frontmatter["assignee"]).To(Equal(""))
				Expect(cmd.Frontmatter["phase"]).To(Equal("human_review"))
				Expect(cmd.Body).To(ContainSubstring("unknown"))
				Expect(cmd.Title).To(Equal("PR Review github - bborbe-repo - 10 - some-pr"))
			})
		})
	})
})
