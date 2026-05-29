// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func TestResolveAuthAbsentCredentials(t *testing.T) {
	g := NewGomegaWithT(t)
	app := &application{}
	err := app.resolveAuth(context.Background())
	g.Expect(err).NotTo(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("APP_ID"))
	g.Expect(err.Error()).NotTo(ContainSubstring("GH_TOKEN"))
}
