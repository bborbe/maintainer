// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-pr/pkg/factory"
)

var _ = Describe("CreateGitHubAppClient", func() {
	ctx := context.Background()

	It("returns error when appID is zero", func() {
		client, err := factory.CreateGitHubAppClient(ctx, 0, 1, []byte("invalid"))
		Expect(err).To(HaveOccurred())
		Expect(client).To(BeNil())
	})

	It("returns error when installationID is zero", func() {
		client, err := factory.CreateGitHubAppClient(ctx, 1, 0, []byte("invalid"))
		Expect(err).To(HaveOccurred())
		Expect(client).To(BeNil())
	})
})
