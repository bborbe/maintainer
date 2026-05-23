// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestResolveAuthMode(t *testing.T) {
	tests := []struct {
		name      string
		appID     int64
		installID int64
		pemFile   string
		ghToken   string
		want      AuthMode
	}{
		{
			name:      "github app wins when all three app fields set",
			appID:     3798945,
			installID: 134414316,
			pemFile:   "/etc/github-app/pem",
			ghToken:   "ghp_legacy",
			want:      AuthModeGitHubApp,
		},
		{
			name:      "pat fallback when app fields missing but gh token present",
			appID:     0,
			installID: 0,
			pemFile:   "",
			ghToken:   "ghp_legacy",
			want:      AuthModePATFallback,
		},
		{
			name:      "pat fallback when only app id set",
			appID:     3798945,
			installID: 0,
			pemFile:   "",
			ghToken:   "ghp_legacy",
			want:      AuthModePATFallback,
		},
		{
			name:      "pat fallback when only installation id set",
			appID:     0,
			installID: 134414316,
			pemFile:   "",
			ghToken:   "ghp_legacy",
			want:      AuthModePATFallback,
		},
		{
			name:      "pat fallback when only pem file set",
			appID:     0,
			installID: 0,
			pemFile:   "/etc/github-app/pem",
			ghToken:   "ghp_legacy",
			want:      AuthModePATFallback,
		},
		{
			name:      "none when neither app nor pat configured",
			appID:     0,
			installID: 0,
			pemFile:   "",
			ghToken:   "",
			want:      AuthModeNone,
		},
		{
			name:      "none when app fields set but no gh token",
			appID:     3798945,
			installID: 134414316,
			pemFile:   "/etc/github-app/pem",
			ghToken:   "",
			want:      AuthModeGitHubApp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			got := ResolveAuthMode(tt.appID, tt.installID, tt.pemFile, tt.ghToken)
			g.Expect(got).
				To(Equal(tt.want), "resolveAuthMode(%d, %d, %q, %q)", tt.appID, tt.installID, tt.pemFile, tt.ghToken)
		})
	}
}
