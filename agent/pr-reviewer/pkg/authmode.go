// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

// AuthMode classifies which credential type is active at pod startup.
type AuthMode int

const (
	AuthModeNone AuthMode = iota
	AuthModeGitHubApp
	AuthModePATFallback
)

// ResolveAuthMode picks the credential type to use at pod startup.
//   - AppID and InstallationID and PEMKeyFile all set → AuthModeGitHubApp
//   - Any of the three App fields unset, but GHToken non-empty → AuthModePATFallback
//   - Otherwise → AuthModeNone (caller MUST refuse to start)
func ResolveAuthMode(appID, installationID int64, pemKeyFile, ghToken string) AuthMode {
	if appID > 0 && installationID > 0 && pemKeyFile != "" {
		return AuthModeGitHubApp
	}
	if ghToken != "" {
		return AuthModePATFallback
	}
	return AuthModeNone
}
