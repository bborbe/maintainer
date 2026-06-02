// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package plugin provides pure-Go functions for detecting Claude Code plugin manifests
// and bumping their version fields.
package plugin

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	bborbeerrors "github.com/bborbe/errors"
)

// Package-level compiled regex for semver-shaped string validation.
// Matches only the bare N.N.N pattern — no leading 'v', no suffix.
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Package-level compiled regex for the "plugins": [ array-opening line.
var pluginsArrayLineRE = regexp.MustCompile(`^\s*"plugins"\s*:\s*\[`)

// Package-level compiled regex for a named-object scope opener such as `"metadata": {`.
var isOpenScopeKeyRE = regexp.MustCompile(`^\s*"[^"]+"\s*:\s*\{`)

// DetectManifests returns the subset of known plugin manifest paths that exist
// as regular files in the given workdir. The returned paths are repo-relative
// (e.g. ".claude-plugin/plugin.json").
//
// Existence detection is not an error condition: missing manifests are silently
// omitted from the result. Errors are returned only for unexpected I/O failures.
func DetectManifests(ctx context.Context, workdir string) ([]string, error) {
	known := []string{
		".claude-plugin/plugin.json",
		".claude-plugin/marketplace.json",
	}

	var result []string
	for _, rel := range known {
		path := filepath.Join(workdir, rel)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, bborbeerrors.Wrapf(ctx, err, "detect manifests in %s", workdir)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		result = append(result, rel)
	}
	return result, nil
}

// BumpPluginJSON rewrites the top-level "version" field in a plugin.json byte stream.
// It validates the version parameter against semverRE before touching content.
// All other bytes are preserved verbatim (same indentation, key order, trailing newline).
func BumpPluginJSON(ctx context.Context, content []byte, version string) ([]byte, error) {
	if !semverRE.MatchString(version) {
		return nil, bborbeerrors.Errorf(ctx,
			"plugin.json bump rejected: version parameter %q is not a semver-shaped string",
			version)
	}

	if len(content) == 0 {
		return nil, bborbeerrors.New(ctx, "plugin.json version field not found")
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))

	var out bytes.Buffer
	found := false

	for scanner.Scan() {
		line := scanner.Text()

		if !found && isVersionKeyLine(line) {
			rewritten, err := rewriteVersionValue(ctx, line, version, "plugin.json")
			if err != nil {
				return nil, err
			}
			out.WriteString(rewritten)
			out.WriteByte('\n')
			found = true
			continue
		}

		out.WriteString(line)
		out.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return nil, bborbeerrors.Wrap(ctx, err, "bump plugin.json")
	}

	if !found {
		return nil, bborbeerrors.New(ctx, "plugin.json version field not found")
	}

	result := out.Bytes()
	if len(content) > 0 && content[len(content)-1] != '\n' && len(result) > 0 &&
		result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result, nil
}

// BumpMarketplaceJSON rewrites metadata.version and every plugins[].version
// in a marketplace.json byte stream. It validates the version parameter against
// semverRE before touching content. All other bytes are preserved verbatim.
//
//nolint:gocognit,gocyclo,funlen // line-based JSON streamer with intentional scope state tracking; refactor candidate tracked separately
func BumpMarketplaceJSON(ctx context.Context, content []byte, version string) ([]byte, error) {
	if !semverRE.MatchString(version) {
		return nil, bborbeerrors.Errorf(ctx,
			"marketplace.json bump rejected: version parameter %q is not a semver-shaped string",
			version)
	}

	if len(content) == 0 {
		return nil, bborbeerrors.New(ctx, "marketplace.json version field not found")
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))

	var out bytes.Buffer
	depth := 0
	// inMetadata is true when we are inside the "metadata" object
	inMetadata := false
	// inPlugin is true when we are directly inside an element of the "plugins" array
	inPlugin := false
	// inPluginsArray is true when we are inside the "plugins" array
	inPluginsArray := false
	foundAny := false

	// Helper to check if a line contains a "version" key (not inside another string value)
	lineHasVersionKey := func(l string) bool {
		idx := strings.Index(l, `"version":`)
		if idx < 0 {
			idx = strings.Index(l, `"version" :`)
		}
		if idx < 0 {
			return false
		}
		if idx == 0 {
			return true
		}
		before := l[idx-1]
		return before == ',' || before == '{' || before == '[' || before == ' ' || before == '\t'
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := trimLine(line)

		oldDepth := depth

		// Handle closing at oldDepth BEFORE we update depth
		if oldDepth == 2 {
			if isCloseBrace(line) {
				// Closing metadata object or a plugin object
				inMetadata = false
				inPlugin = false
			}
		}

		// Detect plugins array opening when we're already inside the root object (oldDepth >= 1).
		// This handles multi-line "plugins": [ on its own line.
		// At oldDepth >= 1, "plugins": [ means we're entering the plugins array.
		if oldDepth >= 1 && pluginsArrayLineRE.MatchString(trimmed) {
			inPluginsArray = true
		}

		// Update depth
		depthDelta := countOpenBraces(line) - countCloseBraces(line)
		depth = oldDepth + depthDelta

		// Handle scope entry at the new depth. oldDepth tells us what we came FROM.
		if depthDelta > 0 {
			if oldDepth == 1 && isOpenScopeKey(trimmed) {
				scopeKey := extractScopeKey(trimmed)
				if scopeKey == "metadata" {
					inMetadata = true
					inPlugin = false
					inPluginsArray = false
				}
			}
		}

		// Entering a plugin object: we're in the plugins array and see a line starting with {.
		// This handles:
		// - Multi-line: { on its own line (depth goes 2->3)
		// - Single-line: { on the same line as "plugins": [ (depth goes 1->1, no change)
		// For multi-line plugin entries, oldDepth==2 when we enter (depth increased from 2 to 3).
		// For single-line plugin entries (same line as plugins: [), oldDepth==1 when we enter.
		// For multi-line with { on next line, oldDepth==0 (depth went from 1->2 after [ on prev line).
		if inPluginsArray && strings.HasPrefix(trimmed, "{") {
			if oldDepth == 2 || oldDepth == 1 || oldDepth == 0 {
				inPlugin = true
				inMetadata = false
			}
		}

		// Handle plugins array close: ] at oldDepth==2
		if oldDepth == 2 && isCloseBracket(line) && inPluginsArray {
			inPluginsArray = false
			inPlugin = false
		}

		// Full scope exit
		if depth == 0 {
			inMetadata = false
			inPlugin = false
			inPluginsArray = false
		}

		// Process version key if in scope
		if lineHasVersionKey(trimmed) {
			inScope := inMetadata || inPlugin
			if inScope {
				rewritten, err := rewriteVersionValue(ctx, line, version, "marketplace.json")
				if err != nil {
					return nil, err
				}
				out.WriteString(rewritten)
				out.WriteByte('\n')
				foundAny = true
				continue
			}
		}

		out.WriteString(line)
		out.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return nil, bborbeerrors.Wrap(ctx, err, "bump marketplace.json")
	}

	if !foundAny {
		return nil, bborbeerrors.New(ctx, "marketplace.json version field not found")
	}

	result := out.Bytes()
	if len(content) > 0 && content[len(content)-1] != '\n' && len(result) > 0 &&
		result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result, nil
}

// isOpenScopeKey returns true if the trimmed line opens a named object scope
// with a key like "metadata" or "plugins" (e.g. `"metadata": {`).
func isOpenScopeKey(trimmed string) bool {
	return isOpenScopeKeyRE.MatchString(trimmed)
}

// extractScopeKey extracts the key name from a line like `"metadata": {` or `"plugins": {`.
func extractScopeKey(trimmed string) string {
	// Find the opening quote
	start := -1
	for i, ch := range trimmed {
		if ch == '"' {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return ""
	}
	// Find the closing quote
	end := start
	for end < len(trimmed) && trimmed[end] != '"' {
		end++
	}
	if end > start {
		return trimmed[start:end]
	}
	return ""
}

// isVersionKeyLine returns true if the line contains a "version" key.
// It distinguishes the "version" key from "version" appearing inside
// another string value by checking that "version" is preceded by a
// JSON structural character (",", "{", "[", or whitespace).
func isVersionKeyLine(line string) bool {
	trimmed := trimLine(line)
	idx := strings.Index(trimmed, `"version":`)
	if idx < 0 {
		idx = strings.Index(trimmed, `"version" :`)
	}
	if idx < 0 {
		return false
	}
	if idx == 0 {
		return true
	}
	// Check that the char before '"' is a JSON structural char
	before := trimmed[idx-1]
	return before == ',' || before == '{' || before == '[' || before == ' ' || before == '\t'
}

// rewriteVersionValue replaces the value after ": " on the given line with the quoted version.
// The line must be a "version" key line. Returns an error if the existing value is not a quoted semver.
//
//nolint:gocognit,funlen // single-line JSON value rewriter with embedded whitespace/quote state; refactor candidate tracked separately
func rewriteVersionValue(
	ctx context.Context,
	line string,
	version string,
	fileType string,
) (string, error) {
	trimmed := trimLine(line)

	// Find the position of "version": in the line to locate the correct colon
	versionKeyIdx := strings.Index(trimmed, `"version":`)
	if versionKeyIdx < 0 {
		versionKeyIdx = strings.Index(trimmed, `"version" :`)
	}
	if versionKeyIdx < 0 {
		return "", bborbeerrors.New(ctx, fileType+" version line has no version key")
	}

	// Find the colon that follows the "version" key specifically
	colonIdx := -1
	for i := versionKeyIdx; i < len(trimmed); i++ {
		if trimmed[i] == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx == -1 {
		return "", bborbeerrors.New(ctx, fileType+" version line has no colon")
	}

	// Extract the value part after ": "
	valuePart := trimmed[colonIdx+1:]
	if len(valuePart) == 0 {
		return "", bborbeerrors.Errorf(
			ctx,
			fileType+" existing version field is not a semver-shaped string: %q",
			"",
		)
	}

	// Skip whitespace after colon
	rest := valuePart
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}

	// Determine quote character
	var quote byte
	if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
		quote = rest[0]
		rest = rest[1:]
	} else {
		// Unquoted value
		end := 0
		for end < len(rest) && rest[end] != ',' && rest[end] != '}' && rest[end] != ' ' && rest[end] != '\t' {
			end++
		}
		valueStr := rest[:end]

		if !semverRE.MatchString(valueStr) {
			return "", bborbeerrors.Errorf(ctx,
				fileType+" existing version field is not a semver-shaped string: %q", valueStr)
		}

		indent := getIndent(line)
		keyPart := trimmed[:colonIdx+1]
		trailing := rest[end:]
		return fmt.Sprintf("%s%s %s%s", indent, keyPart, version, trailing), nil
	}

	// Find closing quote (handling escape sequences)
	closeIdx := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == quote {
			// Count preceding backslashes
			backslashes := 0
			for j := i - 1; j >= 0 && rest[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx == -1 {
		return "", bborbeerrors.Errorf(ctx,
			fileType+" existing version field is not a semver-shaped string: %q", "")
	}

	valueStr := rest[:closeIdx]

	if !semverRE.MatchString(valueStr) {
		return "", bborbeerrors.Errorf(ctx,
			fileType+" existing version field is not a semver-shaped string: %q", valueStr)
	}

	// Reconstruct the line
	indent := getIndent(line)
	keyPart := trimmed[:colonIdx+1]
	trailing := rest[closeIdx+1:]

	return fmt.Sprintf("%s%s \"%s\"%s", indent, keyPart, version, trailing), nil
}

// countOpenBraces returns the number of '{' and '[' characters in the line.
func countOpenBraces(line string) int {
	c := 0
	for _, ch := range line {
		if ch == '{' || ch == '[' {
			c++
		}
	}
	return c
}

// countCloseBraces returns the number of '}' and ']' characters in the line.
func countCloseBraces(line string) int {
	c := 0
	for _, ch := range line {
		if ch == '}' || ch == ']' {
			c++
		}
	}
	return c
}

// isCloseBrace returns true if the line contains a '}'.
func isCloseBrace(line string) bool {
	for _, ch := range line {
		if ch == '}' {
			return true
		}
	}
	return false
}

// isCloseBracket returns true if the line contains a ']'.
func isCloseBracket(line string) bool {
	for _, ch := range line {
		if ch == ']' {
			return true
		}
	}
	return false
}

// trimLine returns the line with leading and trailing whitespace removed.
func trimLine(line string) string {
	start := 0
	end := len(line)
	for start < end && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	for end > start && (line[end-1] == ' ' || line[end-1] == '\t' || line[end-1] == '\r') {
		end--
	}
	return line[start:end]
}

// getIndent returns the leading whitespace of the line.
func getIndent(line string) string {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[:i]
}
