package history

import "strings"

// splitLines breaks text into lines without trailing-newline artifacts. An
// empty string yields no lines (not one empty line), so an added/removed file
// diffs cleanly against nothing.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	// A trailing newline produces a final empty element; drop it so it isn't
	// diffed as a phantom line.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// diffLines produces a unified line diff of old vs new using the classic
// longest-common-subsequence backtrack. Output order matches the files:
// equal and deleted lines follow the old file, additions appear where new
// lines diverge.
func diffLines(oldLines, newLines []string) []DiffLine {
	n, m := len(oldLines), len(newLines)
	// lcs[i][j] = length of LCS of oldLines[i:] and newLines[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < n && j < m {
		if oldLines[i] == newLines[j] {
			out = append(out, DiffLine{Op: "eq", Text: oldLines[i]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			out = append(out, DiffLine{Op: "del", Text: oldLines[i]})
			i++
		} else {
			out = append(out, DiffLine{Op: "add", Text: newLines[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, DiffLine{Op: "del", Text: oldLines[i]})
	}
	for ; j < m; j++ {
		out = append(out, DiffLine{Op: "add", Text: newLines[j]})
	}
	return out
}
