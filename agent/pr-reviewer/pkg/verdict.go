// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"
	"strings"
)

// Verdict represents the review verdict type
type Verdict string

const (
	VerdictApprove        Verdict = "approve"
	VerdictRequestChanges Verdict = "request-changes"
)

// Result holds the verdict and reason
type Result struct {
	Verdict Verdict
	Reason  string
}

// jsonVerdict is used for unmarshaling JSON verdict blocks (legacy use by StripJSONVerdict)
type jsonVerdict struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

// findLastJSONVerdictBlock returns the LAST JSON object (string content) in
// reviewText that contains a "verdict" field. Handles single-line objects and
// multi-line fenced ```json blocks. Returns empty + false if none found.
// Only the last 50 lines are scanned for the closing brace, to avoid matching
// JSON examples quoted earlier in the review text.
func findLastJSONVerdictBlock(reviewText string) (string, bool) {
	lines := strings.Split(reviewText, "\n")
	startIdx := 0
	if len(lines) > 50 {
		startIdx = len(lines) - 50
	}
	// Search backwards for the last line containing "verdict" within the window.
	verdictLine := -1
	for i := len(lines) - 1; i >= startIdx; i-- {
		if strings.Contains(lines[i], `"verdict"`) {
			verdictLine = i
			break
		}
	}
	if verdictLine < 0 {
		return "", false
	}
	// Find the enclosing {...} block by walking back to the nearest '{' and
	// forward to its matching '}'. Single-line {"verdict":"..."} works too.
	startCh := lastIndexOfBrace(lines, verdictLine, '{')
	if startCh.line < 0 {
		return "", false
	}
	endCh := nextIndexOfMatchingClose(lines, startCh)
	if endCh.line < 0 {
		return "", false
	}
	return extractBlock(lines, startCh, endCh), true
}

type charPos struct{ line, col int }

func lastIndexOfBrace(lines []string, fromLine int, b byte) charPos {
	for li := fromLine; li >= 0; li-- {
		s := lines[li]
		end := len(s)
		if li == fromLine {
			end = strings.Index(s, `"verdict"`)
			if end < 0 {
				end = len(s)
			}
		}
		for ci := end - 1; ci >= 0; ci-- {
			if s[ci] == b {
				return charPos{li, ci}
			}
		}
	}
	return charPos{-1, -1}
}

func nextIndexOfMatchingClose(lines []string, start charPos) charPos {
	depth := 0
	for li := start.line; li < len(lines); li++ {
		s := lines[li]
		ci := 0
		if li == start.line {
			ci = start.col
		}
		for ; ci < len(s); ci++ {
			switch s[ci] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return charPos{li, ci}
				}
			}
		}
	}
	return charPos{-1, -1}
}

func extractBlock(lines []string, start, end charPos) string {
	if start.line == end.line {
		return lines[start.line][start.col : end.col+1]
	}
	var b strings.Builder
	b.WriteString(lines[start.line][start.col:])
	b.WriteByte('\n')
	for i := start.line + 1; i < end.line; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}
	b.WriteString(lines[end.line][:end.col+1])
	return b.String()
}

// ParseVerdict analyzes Claude review output and determines the appropriate verdict.
// The verdict is binary: approve or request-changes. No other value is returned.
// Fail-closed: empty or unparseable output returns request-changes.
func ParseVerdict(reviewText string) Result {
	if reviewText == "" {
		return Result{
			Verdict: VerdictRequestChanges,
			Reason:  "empty review text",
		}
	}

	block, ok := findLastJSONVerdictBlock(reviewText)
	if !ok {
		return Result{
			Verdict: VerdictRequestChanges,
			Reason:  "no verdict block",
		}
	}

	var jv jsonVerdict
	if err := json.Unmarshal([]byte(block), &jv); err != nil {
		return Result{
			Verdict: VerdictRequestChanges,
			Reason:  "malformed JSON: " + err.Error(),
		}
	}

	// Normalise: lowercase + replace underscores with hyphens
	// so "request_changes", "REQUEST-CHANGES", "Request-Changes" all parse correctly.
	normalized := strings.ToLower(strings.ReplaceAll(jv.Verdict, "_", "-"))
	switch normalized {
	case "approve":
		return Result{Verdict: VerdictApprove, Reason: jv.Reason}
	case "request-changes":
		return Result{Verdict: VerdictRequestChanges, Reason: jv.Reason}
	default:
		return Result{Verdict: VerdictRequestChanges, Reason: "unknown verdict: " + jv.Verdict}
	}
}

// StripJSONVerdict removes the JSON verdict line (and surrounding code fence if present)
// from the review text. Returns the cleaned review text for posting as a PR comment.
// If no JSON verdict found, returns the text unchanged.
func StripJSONVerdict(reviewText string) string {
	lines := strings.Split(reviewText, "\n")
	linesToRemove := findVerdictLinesToRemove(lines)

	if len(linesToRemove) == 0 {
		return reviewText
	}

	return buildCleanedText(lines, linesToRemove)
}

// findVerdictLinesToRemove scans lines and returns a map of line indices to remove
func findVerdictLinesToRemove(lines []string) map[int]bool {
	startIdx := calculateStartIndex(lines)
	linesToRemove := make(map[int]bool)
	inCodeFence := false
	fenceStartIdx := -1

	for i := startIdx; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		if handleCodeFenceStart(line, &inCodeFence, &fenceStartIdx, i) {
			continue
		}

		if handleCodeFenceEnd(line, &inCodeFence, &fenceStartIdx) {
			continue
		}

		if containsVerdictJSON(line) {
			processVerdictLine(lines, i, line, inCodeFence, fenceStartIdx, linesToRemove)
		}
	}

	return linesToRemove
}

// calculateStartIndex returns the index to start searching (last 50 lines)
func calculateStartIndex(lines []string) int {
	if len(lines) > 50 {
		return len(lines) - 50
	}
	return 0
}

// handleCodeFenceStart checks for code fence start and updates state
func handleCodeFenceStart(line string, inCodeFence *bool, fenceStartIdx *int, i int) bool {
	if line == "```json" && !*inCodeFence {
		*inCodeFence = true
		*fenceStartIdx = i
		return true
	}
	return false
}

// handleCodeFenceEnd checks for code fence end and updates state
func handleCodeFenceEnd(line string, inCodeFence *bool, fenceStartIdx *int) bool {
	if line == "```" && *inCodeFence {
		*inCodeFence = false
		*fenceStartIdx = -1
		return true
	}
	return false
}

// containsVerdictJSON checks if a line contains verdict JSON markers
func containsVerdictJSON(line string) bool {
	return strings.Contains(line, `"verdict"`) && strings.Contains(line, `"reason"`)
}

// processVerdictLine validates and marks lines for removal if valid verdict found
func processVerdictLine(
	lines []string,
	i int,
	line string,
	inCodeFence bool,
	fenceStartIdx int,
	linesToRemove map[int]bool,
) {
	if !isValidVerdictJSON(line) {
		return
	}

	// Mark verdict line for removal
	linesToRemove[i] = true

	// If inside code fence, mark fence lines too
	if inCodeFence && fenceStartIdx >= 0 {
		markCodeFenceLinesForRemoval(lines, i, fenceStartIdx, linesToRemove)
	}
}

// isValidVerdictJSON checks if the line contains a valid verdict JSON
func isValidVerdictJSON(line string) bool {
	jsonStr := strings.TrimSpace(line)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var jv jsonVerdict
	if err := json.Unmarshal([]byte(jsonStr), &jv); err != nil {
		return false
	}

	return jv.Verdict != ""
}

// markCodeFenceLinesForRemoval marks fence start and end lines for removal
func markCodeFenceLinesForRemoval(
	lines []string,
	currentIdx int,
	fenceStartIdx int,
	linesToRemove map[int]bool,
) {
	linesToRemove[fenceStartIdx] = true

	// Find and mark the closing fence
	for j := currentIdx + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "```" {
			linesToRemove[j] = true
			break
		}
	}
}

// buildCleanedText constructs the final text without removed lines
func buildCleanedText(lines []string, linesToRemove map[int]bool) string {
	var cleaned []string
	for i, line := range lines {
		if !linesToRemove[i] {
			cleaned = append(cleaned, line)
		}
	}

	result := strings.Join(cleaned, "\n")
	return strings.TrimRight(result, "\n")
}
