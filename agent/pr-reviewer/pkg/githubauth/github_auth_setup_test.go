// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubauth_test

import (
	"context"
	stderrors "errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubauth"
)

var _ = Describe("GhAuthSetupGit", func() {
	var (
		ctx       context.Context
		callCount int
		lastName  string
		lastArgs  []string
		fakeExec  func(ctx context.Context, name string, args ...string) ([]byte, error)
	)

	BeforeEach(func() {
		ctx = context.Background()
		callCount = 0
		fakeExec = func(_ context.Context, name string, args ...string) ([]byte, error) {
			callCount++
			lastName = name
			lastArgs = args
			return nil, nil
		}
	})

	It("does not invoke gh when GH_TOKEN is empty", func() {
		setup := githubauth.NewGhAuthSetupGitWithExecFunc("", fakeExec)
		err := setup.Setup(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(callCount).To(Equal(0))
	})

	It("invokes gh auth setup-git exactly once when GH_TOKEN is non-empty", func() {
		setup := githubauth.NewGhAuthSetupGitWithExecFunc("fake-token", fakeExec)
		err := setup.Setup(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(callCount).To(Equal(1))
		Expect(lastName).To(Equal("gh"))
		Expect(lastArgs).To(Equal([]string{"auth", "setup-git"}))
	})

	It("propagates exec error when gh fails", func() {
		fakeExec = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, stderrors.New("gh not found")
		}
		setup := githubauth.NewGhAuthSetupGitWithExecFunc("some-token", fakeExec)
		err := setup.Setup(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("gh auth setup-git failed"))
	})

	It("does not include the token value in any argument", func() {
		const fakeToken = "ghp_SUPERSECRET123"
		fakeExec = func(_ context.Context, name string, args ...string) ([]byte, error) {
			Expect(name).NotTo(ContainSubstring(fakeToken))
			for _, a := range args {
				Expect(a).NotTo(ContainSubstring(fakeToken))
			}
			return nil, nil
		}
		setup := githubauth.NewGhAuthSetupGitWithExecFunc(fakeToken, fakeExec)
		Expect(setup.Setup(ctx)).To(Succeed())
	})

	It("does not leak the token via wrapped exec error output", func() {
		const fakeToken = "ghp_SUPERSECRET123"
		// Simulate gh failure where stdout/stderr contains the literal token string;
		// both the error message AND the captured output bytes must be scrubbed.
		fakeExec = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			out := []byte("gh stdout: authenticated as " + fakeToken + " (failed)")
			return out, stderrors.New("gh stderr: authenticated as " + fakeToken + " (failed)")
		}
		setup := githubauth.NewGhAuthSetupGitWithExecFunc(fakeToken, fakeExec)
		err := setup.Setup(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring(fakeToken))
	})

	It("includes captured gh stderr in the wrapped error so operators can diagnose", func() {
		// Mirrors the prod incident: gh auth setup-git exits non-zero with a real
		// diagnostic on stderr; without this fix that diagnostic is dropped and
		// only "gh auth setup-git failed" surfaces in the OpenClaw task body.
		const stderrText = "X11 connection rejected because of wrong authentication"
		fakeExec = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(stderrText), stderrors.New("exit status 1")
		}
		setup := githubauth.NewGhAuthSetupGitWithExecFunc("some-token", fakeExec)
		err := setup.Setup(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("gh auth setup-git failed"))
		Expect(err.Error()).To(ContainSubstring(stderrText))
	})

	It(
		"truncates captured output to a bounded tail so a runaway stderr cannot blow up the error body",
		func() {
			// 16 KiB of stderr → bounded tail must keep the message readable and
			// must surface the most recent bytes (last line) rather than the first.
			const tailMarker = "FINAL_ERROR_LINE_MARKER"
			huge := strings.Repeat("A", 16*1024) + tailMarker
			fakeExec = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return []byte(huge), stderrors.New("exit status 1")
			}
			setup := githubauth.NewGhAuthSetupGitWithExecFunc("some-token", fakeExec)
			err := setup.Setup(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tailMarker))
			Expect(err.Error()).To(ContainSubstring("[truncated]"))
			Expect(len(err.Error())).To(BeNumerically("<", 8*1024))
		},
	)
})

var _ = Describe("NoopAuthSetup", func() {
	It("always returns nil", func() {
		setup := githubauth.NewNoopAuthSetup()
		Expect(setup.Setup(context.Background())).To(Succeed())
	})
})

var _ = Describe("NewGhAuthSetupGit", func() {
	It("returns a non-nil setup when token is empty and Setup is a no-op", func() {
		// Covers the constructor; empty token → no subprocess invoked.
		setup := githubauth.NewGhAuthSetupGit("")
		Expect(setup).NotTo(BeNil())
		Expect(setup.Setup(context.Background())).To(Succeed())
	})
})

var _ = Describe("DefaultExecFunc", func() {
	It("returns nil error and empty output when the command succeeds", func() {
		// `true` is in /bin on alpine/Linux containers and in /usr/bin on macOS;
		// rely on PATH resolution rather than hardcoding either prefix
		out, err := githubauth.DefaultExecFunc(context.Background(), "true")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeEmpty())
	})

	It("returns an error and captured output when the command fails", func() {
		out, err := githubauth.DefaultExecFunc(context.Background(), "false")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed"))
		// `false` produces no output but the return value must still be a slice
		// (not nil) so callers can safely string-convert it.
		Expect(out).NotTo(BeNil())
	})
})
