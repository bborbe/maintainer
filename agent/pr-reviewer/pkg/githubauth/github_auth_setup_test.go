// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubauth_test

import (
	"context"
	stderrors "errors"

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
		fakeExec  func(ctx context.Context, name string, args ...string) error
	)

	BeforeEach(func() {
		ctx = context.Background()
		callCount = 0
		fakeExec = func(_ context.Context, name string, args ...string) error {
			callCount++
			lastName = name
			lastArgs = args
			return nil
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
		fakeExec = func(_ context.Context, _ string, _ ...string) error {
			return stderrors.New("gh not found")
		}
		setup := githubauth.NewGhAuthSetupGitWithExecFunc("some-token", fakeExec)
		err := setup.Setup(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("gh auth setup-git failed"))
	})

	It("does not include the token value in any argument", func() {
		const fakeToken = "ghp_SUPERSECRET123"
		fakeExec = func(_ context.Context, name string, args ...string) error {
			Expect(name).NotTo(ContainSubstring(fakeToken))
			for _, a := range args {
				Expect(a).NotTo(ContainSubstring(fakeToken))
			}
			return nil
		}
		setup := githubauth.NewGhAuthSetupGitWithExecFunc(fakeToken, fakeExec)
		Expect(setup.Setup(ctx)).To(Succeed())
	})

	It("does not leak the token via wrapped exec error output", func() {
		const fakeToken = "ghp_SUPERSECRET123"
		// Simulate gh failure where stdout/stderr contains the literal token string.
		fakeExec = func(_ context.Context, _ string, _ ...string) error {
			return stderrors.New("gh stderr: authenticated as " + fakeToken + " (failed)")
		}
		setup := githubauth.NewGhAuthSetupGitWithExecFunc(fakeToken, fakeExec)
		err := setup.Setup(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring(fakeToken))
	})
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
	It("returns nil when the command succeeds", func() {
		// `true` is in /bin on alpine/Linux containers and in /usr/bin on macOS;
		// rely on PATH resolution rather than hardcoding either prefix
		err := githubauth.DefaultExecFunc(context.Background(), "true")
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns an error when the command fails", func() {
		err := githubauth.DefaultExecFunc(context.Background(), "false")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed"))
	})
})
