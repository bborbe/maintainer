// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"
	"net/http"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
)

// NewResetCursorHandler returns an http.Handler that deletes the cursor entry
// for the {repo} path variable, forcing the next poll to treat the repo as
// first-seen and re-emit a release task.
//
// Route: /resetcursor/{repo:.+}  (`.+` captures slashes, e.g. github.com/bborbe/foo)
// Method-agnostic — wrap with libhttp.NewDangerousHandlerWrapper for the
// POST/confirm gate (it mutates persisted state). Mirrors
// watcher/github-build/pkg/reset_handler.go.
//
// Behavior:
//   - repo present in cursor → delete entry, save, 200 {reset, existed:true}
//   - repo absent → 200 {reset, existed:false} (idempotent)
//   - cursor load/save error → 500 {error}
func NewResetCursorHandler(cursorPath string) http.Handler {
	return &resetCursorHandler{cursorPath: cursorPath}
}

type resetCursorHandler struct {
	cursorPath string
}

func (h *resetCursorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	repo := mux.Vars(r)["repo"]
	if repo == "" {
		writeCursorJSON(
			w,
			http.StatusBadRequest,
			map[string]any{"error": "repo path variable required"},
		)
		return
	}
	cursor, err := LoadCursor(ctx, h.cursorPath)
	if err != nil {
		glog.Warningf("resetcursor load failed repo=%s err=%v", repo, err)
		writeCursorJSON(
			w,
			http.StatusInternalServerError,
			map[string]any{"error": "load cursor failed"},
		)
		return
	}
	_, existed := cursor.Repos[repo]
	delete(cursor.Repos, repo)
	if err := SaveCursor(ctx, h.cursorPath, cursor); err != nil {
		glog.Warningf("resetcursor save failed repo=%s err=%v", repo, err)
		writeCursorJSON(
			w,
			http.StatusInternalServerError,
			map[string]any{"error": "save cursor failed"},
		)
		return
	}
	glog.V(2).Infof("resetcursor ok repo=%s existed=%t", repo, existed)
	writeCursorJSON(w, http.StatusOK, map[string]any{"reset": repo, "existed": existed})
}

// NewSetCursorHandler returns an http.Handler that sets the cursor's last-seen
// master SHA for {repo} to the `sha` query value. Setting it to a SHA *older*
// than current master HEAD makes the next poll see HEAD as advanced and re-emit;
// setting it to the current HEAD suppresses re-emit. More precise than reset
// when you want to pin a specific baseline.
//
// Route: /setcursor/{repo:.+}?sha=<value>
// Method-agnostic — wrap with libhttp.NewDangerousHandlerWrapper.
//
// Behavior:
//   - missing/empty sha → 400 {error}
//   - else → set entry, save, 200 {set, sha, previous}
//   - cursor load/save error → 500 {error}
func NewSetCursorHandler(cursorPath string) http.Handler {
	return &setCursorHandler{cursorPath: cursorPath}
}

type setCursorHandler struct {
	cursorPath string
}

func (h *setCursorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	repo := mux.Vars(r)["repo"]
	if repo == "" {
		writeCursorJSON(
			w,
			http.StatusBadRequest,
			map[string]any{"error": "repo path variable required"},
		)
		return
	}
	sha := r.URL.Query().Get("sha")
	if sha == "" {
		writeCursorJSON(
			w,
			http.StatusBadRequest,
			map[string]any{"error": "sha query parameter required"},
		)
		return
	}
	cursor, err := LoadCursor(ctx, h.cursorPath)
	if err != nil {
		glog.Warningf("setcursor load failed repo=%s err=%v", repo, err)
		writeCursorJSON(
			w,
			http.StatusInternalServerError,
			map[string]any{"error": "load cursor failed"},
		)
		return
	}
	var previous string
	if existing := cursor.Repos[repo]; existing != nil {
		previous = existing.LastSeenMasterSHA
	}
	cursor.Repos[repo] = &RepoState{LastSeenMasterSHA: sha}
	if err := SaveCursor(ctx, h.cursorPath, cursor); err != nil {
		glog.Warningf("setcursor save failed repo=%s err=%v", repo, err)
		writeCursorJSON(
			w,
			http.StatusInternalServerError,
			map[string]any{"error": "save cursor failed"},
		)
		return
	}
	glog.V(2).Infof("setcursor ok repo=%s sha=%s previous=%s", repo, sha, previous)
	writeCursorJSON(w, http.StatusOK, map[string]any{"set": repo, "sha": sha, "previous": previous})
}

// writeCursorJSON writes a JSON body with the given status. Encode errors are
// best-effort (header already sent); logged at V(2).
func writeCursorJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		glog.V(2).Infof("cursor handler: encode response failed: %v", err)
	}
}
