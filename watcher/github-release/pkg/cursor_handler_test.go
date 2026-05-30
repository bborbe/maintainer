// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-release/pkg"
)

var _ = Describe("ResetCursorHandler", func() {
	var (
		cursorPath string
		handler    http.Handler
	)

	BeforeEach(func() {
		cursorPath = filepath.Join(GinkgoT().TempDir(), "cursor.json")
		handler = pkg.NewResetCursorHandler(cursorPath)
	})

	routeAndServe := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		rec := httptest.NewRecorder()
		router := mux.NewRouter()
		router.Path("/resetcursor/{repo:.+}").Handler(handler)
		router.ServeHTTP(rec, req)
		return rec
	}

	It("deletes an existing cursor entry", func() {
		cursorState := &pkg.Cursor{Repos: map[string]*pkg.RepoState{
			"github.com/bborbe/foo": {LastSeenMasterSHA: "abc123"},
		}}
		Expect(pkg.SaveCursor(context.Background(), cursorPath, cursorState)).To(Succeed())

		rec := routeAndServe(http.MethodPost, "/resetcursor/github.com/bborbe/foo")
		Expect(rec.Code).To(Equal(http.StatusOK))

		reloaded, err := pkg.LoadCursor(context.Background(), cursorPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(reloaded.Repos).NotTo(HaveKey("github.com/bborbe/foo"))
	})

	It("is idempotent for an absent repo", func() {
		Expect(pkg.SaveCursor(context.Background(), cursorPath, &pkg.Cursor{
			Repos: map[string]*pkg.RepoState{},
		})).To(Succeed())

		rec := routeAndServe(http.MethodPost, "/resetcursor/github.com/bborbe/foo")
		Expect(rec.Code).To(Equal(http.StatusOK))

		var body map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed())
		Expect(body["existed"]).To(BeFalse())
	})
})

var _ = Describe("SetCursorHandler", func() {
	var (
		cursorPath string
		handler    http.Handler
	)

	BeforeEach(func() {
		cursorPath = filepath.Join(GinkgoT().TempDir(), "cursor.json")
		handler = pkg.NewSetCursorHandler(cursorPath)
	})

	routeAndServe := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		rec := httptest.NewRecorder()
		router := mux.NewRouter()
		router.Path("/setcursor/{repo:.+}").Handler(handler)
		router.ServeHTTP(rec, req)
		return rec
	}

	It("sets the last-seen SHA for a repo and reports the previous value", func() {
		Expect(
			pkg.SaveCursor(
				context.Background(),
				cursorPath,
				&pkg.Cursor{Repos: map[string]*pkg.RepoState{
					"github.com/bborbe/foo": {LastSeenMasterSHA: "old111"},
				}},
			),
		).To(Succeed())

		rec := routeAndServe(http.MethodPost, "/setcursor/github.com/bborbe/foo?sha=new222")
		Expect(rec.Code).To(Equal(http.StatusOK))

		var body map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed())
		Expect(body["sha"]).To(Equal("new222"))
		Expect(body["previous"]).To(Equal("old111"))

		reloaded, err := pkg.LoadCursor(context.Background(), cursorPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(reloaded.Repos["github.com/bborbe/foo"].LastSeenMasterSHA).To(Equal("new222"))
	})

	It("creates a cursor entry when the repo was previously absent", func() {
		Expect(pkg.SaveCursor(context.Background(), cursorPath, &pkg.Cursor{
			Repos: map[string]*pkg.RepoState{},
		})).To(Succeed())

		rec := routeAndServe(http.MethodPost, "/setcursor/github.com/bborbe/bar?sha=deadbeef")
		Expect(rec.Code).To(Equal(http.StatusOK))

		reloaded, err := pkg.LoadCursor(context.Background(), cursorPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(reloaded.Repos["github.com/bborbe/bar"].LastSeenMasterSHA).To(Equal("deadbeef"))
	})

	It("rejects a request with no sha query parameter", func() {
		rec := routeAndServe(http.MethodPost, "/setcursor/github.com/bborbe/foo")
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})
})
