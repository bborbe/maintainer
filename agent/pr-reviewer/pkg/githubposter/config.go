// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package githubposter

import (
	"context"
	"os"
	"path/filepath"

	errors "github.com/bborbe/errors"
	"gopkg.in/yaml.v3"
)

// ReadAutoApproveConfig reads .pr-reviewer.yaml from workDir.
// A missing file is not an error — returns AutoApprove: false (spec default).
func ReadAutoApproveConfig(ctx context.Context, workDir string) (AutoApproveConfig, error) {
	path := filepath.Join(workDir, ".pr-reviewer.yaml")
	data, err := os.ReadFile(
		path,
	) // #nosec G304 -- workDir is an internal trusted path, not user-controlled input
	if err != nil {
		if os.IsNotExist(err) {
			return AutoApproveConfig{}, nil
		}
		return AutoApproveConfig{}, errors.Wrapf(ctx, err, "read .pr-reviewer.yaml at %s", path)
	}
	var cfg AutoApproveConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AutoApproveConfig{}, errors.Wrapf(ctx, err, "parse .pr-reviewer.yaml at %s", path)
	}
	return cfg, nil
}
