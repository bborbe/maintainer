// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

// BuildEnv assembles the env map forwarded into the Claude CLI subprocess.
// Only non-empty values are set so the subprocess sees a clean env. Shared
// by both the Kafka entry point (main.go) and the local-CLI entry point
// (cmd/run-task/main.go).
func BuildEnv(
	ghToken, anthropicBaseURL, anthropicAuthToken, anthropicModel string,
) map[string]string {
	env := map[string]string{}
	if ghToken != "" {
		env["GH_TOKEN"] = ghToken
	}
	if anthropicBaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = anthropicBaseURL
	}
	if anthropicAuthToken != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = anthropicAuthToken
	}
	if anthropicModel != "" {
		env["ANTHROPIC_MODEL"] = anthropicModel
	}
	return env
}
