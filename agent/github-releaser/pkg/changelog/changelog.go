// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package changelog provides pure-Go functions for parsing CHANGELOG.md byte streams.
// It supports three operations: validating the Unreleased section for releaseability,
// extracting bullet entries from the Unreleased section, and inferring the historic
// header prefix style (versioned with "v" prefix or unprefixed).
package changelog

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
)

// Package-level compiled regexes for InferHeaderPrefixStyle.
// These are read-only and deterministic, preserving the pure-function contract.
var (
	vPrefixRE  = regexp.MustCompile(`^v[0-9]+\.`)
	noPrefixRE = regexp.MustCompile(`^[0-9]+\.`)
)

// ValidateUnreleased checks whether the CHANGELOG content is in a releaseable state.
// It returns (valid, reason, line) where valid is true if the Unreleased section exists
// as the first ## heading and contains at least one "- " bullet entry.
// The line number is 1-indexed and indicates where the issue was found (0 if valid or generic error).
func ValidateUnreleased(content []byte) (valid bool, reason string, line int) {
	if len(content) == 0 {
		return false, "Unreleased section not found.", 0
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))

	firstHeading, unreleasedLine := findFirstAndUnreleased(scanner)
	if firstHeading == nil {
		return false, "Unreleased section not found.", 0
	}

	if unreleasedLine == 0 {
		return false, "Unreleased section not found.", 0
	}

	if firstHeading.line != unreleasedLine {
		return false, fmt.Sprintf(
			"Unreleased is not the first ## section; found '%s' at line %d. Move ## Unreleased above all release headings.",
			firstHeading.text,
			firstHeading.line,
		), firstHeading.line
	}

	scanner = bufio.NewScanner(bytes.NewReader(content))
	skipLines(scanner, unreleasedLine)

	if !hasBulletInBlock(scanner) {
		return false, "Unreleased section has no bullet entries.", unreleasedLine
	}

	return true, "", 0
}

// heading represents a parsed ## heading.
type heading struct {
	line int
	text string
}

// findFirstAndUnreleased scans the content and returns the first ## heading
// and the line number of the ## Unreleased heading (0 if not found).
func findFirstAndUnreleased(scanner *bufio.Scanner) (*heading, int) {
	var firstHeading *heading
	unreleasedLine := 0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if !isHeading(line) {
			continue
		}

		headingText := parseHeading(line)

		if firstHeading == nil {
			firstHeading = &heading{line: lineNum, text: headingText}
		}

		if headingText == "Unreleased" {
			unreleasedLine = lineNum
			break
		}
	}

	return firstHeading, unreleasedLine
}

// isHeading returns true if the line is a ## heading.
func isHeading(line string) bool {
	return len(line) >= 3 && line[0] == '#' && line[1] == '#' && line[2] == ' '
}

// parseHeading extracts the heading text from a ## heading line.
func parseHeading(line string) string {
	return trimTrailingWhitespace(line[3:])
}

// skipLines advances the scanner by n lines.
func skipLines(scanner *bufio.Scanner, n int) {
	for i := 0; i < n; i++ {
		if !scanner.Scan() {
			return
		}
	}
}

// hasBulletInBlock returns true if the block after the Unreleased heading contains a -  bullet.
func hasBulletInBlock(scanner *bufio.Scanner) bool {
	for scanner.Scan() {
		line := scanner.Text()
		if isHeading(line) {
			break
		}
		if isBullet(line) {
			return true
		}
	}
	return false
}

// isBullet returns true if the line is a -  bullet.
func isBullet(line string) bool {
	return len(line) >= 2 && line[0] == '-' && line[1] == ' '
}

// ExtractUnreleasedBullets returns the bullet entries from the ## Unreleased section.
// It locates the first ## Unreleased heading and returns all lines starting with "- "
// until the next ## heading or EOF.
// Returns nil if no ## Unreleased section exists.
// Returns a non-nil empty slice if ## Unreleased exists but contains no bullets.
func ExtractUnreleasedBullets(content []byte) []string {
	if len(content) == 0 {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))

	lineNum := 0
	inUnreleased := false

	// Find the first ## Unreleased heading
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if len(line) >= 3 && line[0] == '#' && line[1] == '#' && line[2] == ' ' {
			headingText := line[3:] // Strip "## "
			headingText = trimTrailingWhitespace(headingText)

			if headingText == "Unreleased" {
				inUnreleased = true
				break
			}
		}
	}

	if !inUnreleased {
		return nil
	}

	// Scan from the next line until next ## heading or EOF
	result := []string{}
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Stop at next ## heading
		if len(line) >= 3 && line[0] == '#' && line[1] == '#' && line[2] == ' ' {
			break
		}

		// Extract -  bullet
		if len(line) >= 2 && line[0] == '-' && line[1] == ' ' {
			// Strip exactly "- " (2 chars) from the beginning
			bullet := line[2:]
			result = append(result, bullet)
		}
	}

	return result
}

// InferHeaderPrefixStyle examines the first historic release heading (the first ## heading
// that is not "Unreleased") and returns the prefix style used.
// Returns "v" if the heading matches "vX.Y.Z" format, "" if it matches "X.Y.Z" format,
// and "v" as a default if no historic release heading exists.
func InferHeaderPrefixStyle(content []byte) string {
	if len(content) == 0 {
		return "v"
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()

		if len(line) >= 3 && line[0] == '#' && line[1] == '#' && line[2] == ' ' {
			headingText := line[3:] // Strip "## "
			headingText = trimTrailingWhitespace(headingText)

			// Skip Unreleased
			if headingText == "Unreleased" {
				continue
			}

			// This is the first historic release heading
			if vPrefixRE.MatchString(headingText) {
				return "v"
			}
			if noPrefixRE.MatchString(headingText) {
				return ""
			}
			// If heading doesn't match either pattern, keep scanning
		}
	}

	// No historic release heading found, default to "v"
	return "v"
}

// trimTrailingWhitespace removes trailing whitespace from a string.
func trimTrailingWhitespace(s string) string {
	end := len(s)
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[:end]
}
