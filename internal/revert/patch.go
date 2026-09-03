package revert

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"safebox/internal/ui"
)

// DiffOp represents the operation in a diff line.
type DiffOp int

const (
	DiffEqual DiffOp = iota
	DiffInsert
	DiffDelete
)

// DiffLine represents a single line in a computed diff.
type DiffLine struct {
	Op   DiffOp
	Text string
}

// isBinary checks if the byte slice contains null bytes.
func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) != -1
}

// splitLines splits data into lines without trailing newline characters.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return []string{}
	}
	s := string(data)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// computeLCSDiff computes the line-by-line diff between two slices of strings using LCS.
func computeLCSDiff(a, b []string) []DiffLine {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			if a[i] == b[j] {
				dp[i+1][j+1] = dp[i][j] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i+1][j+1] = dp[i][j]
			} else {
				dp[i+1][j+1] = dp[i][j+1]
			}
		}
	}

	var res []DiffLine
	i, j := n, m
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			res = append(res, DiffLine{Op: DiffEqual, Text: a[i-1]})
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			res = append(res, DiffLine{Op: DiffDelete, Text: a[i-1]})
			i--
		} else {
			res = append(res, DiffLine{Op: DiffInsert, Text: b[j-1]})
			j--
		}
	}
	for i > 0 {
		res = append(res, DiffLine{Op: DiffDelete, Text: a[i-1]})
		i--
	}
	for j > 0 {
		res = append(res, DiffLine{Op: DiffInsert, Text: b[j-1]})
		j--
	}

	// Reverse res to maintain original order
	for left, right := 0, len(res)-1; left < right; left, right = left+1, right-1 {
		res[left], res[right] = res[right], res[left]
	}

	return res
}

// hunk represents a contiguous block of diff lines with context.
type hunk struct {
	oldStart, oldCount int
	newStart, newCount int
	lines              []DiffLine
}

// buildHunks groups raw diff lines into unified diff hunks with context.
func buildHunks(diff []DiffLine, contextLen int) []hunk {
	var hunks []hunk
	n := len(diff)
	if n == 0 {
		return hunks
	}

	type indexedLine struct {
		line    DiffLine
		oldLine int
		newLine int
	}

	var indexed []indexedLine
	curOld, curNew := 1, 1
	for _, l := range diff {
		il := indexedLine{line: l, oldLine: curOld, newLine: curNew}
		switch l.Op {
		case DiffEqual:
			curOld++
			curNew++
		case DiffDelete:
			curOld++
		case DiffInsert:
			curNew++
		}
		indexed = append(indexed, il)
	}

	// Identify indices with changes
	hasChange := make([]bool, n)
	for i, il := range indexed {
		if il.line.Op != DiffEqual {
			start := max(0, i-contextLen)
			end := min(n-1, i+contextLen)
			for k := start; k <= end; k++ {
				hasChange[k] = true
			}
		}
	}

	i := 0
	for i < n {
		if !hasChange[i] {
			i++
			continue
		}
		hunkStart := i
		for i < n && hasChange[i] {
			i++
		}
		hunkEnd := i

		var h hunk
		h.oldStart = indexed[hunkStart].oldLine
		h.newStart = indexed[hunkStart].newLine

		for k := hunkStart; k < hunkEnd; k++ {
			h.lines = append(h.lines, indexed[k].line)
			switch indexed[k].line.Op {
			case DiffEqual:
				h.oldCount++
				h.newCount++
			case DiffDelete:
				h.oldCount++
			case DiffInsert:
				h.newCount++
			}
		}
		hunks = append(hunks, h)
	}

	return hunks
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// FormatFilePatch computes and formats a git-style unified diff for a single file change.
func FormatFilePatch(relPath string, oldBytes, newBytes []byte, changeType FileChangeType) string {
	var sb strings.Builder

	switch changeType {
	case ChangeAdded, ChangeUntracked:
		sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", relPath, relPath))
		sb.WriteString("new file mode 100644\n")
		sb.WriteString("--- /dev/null\n")
		sb.WriteString(fmt.Sprintf("+++ b/%s\n", relPath))

		if isBinary(newBytes) {
			sb.WriteString(fmt.Sprintf("Binary files /dev/null and b/%s differ\n", relPath))
			return sb.String()
		}

		lines := splitLines(newBytes)
		if len(lines) == 0 {
			return sb.String()
		}
		sb.WriteString(ui.StyleMeta.Render(fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines))) + "\n")
		for _, l := range lines {
			sb.WriteString(ui.StyleAllowed.Render("+"+l) + "\n")
		}

	case ChangeDeleted:
		sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", relPath, relPath))
		sb.WriteString("deleted file mode 100644\n")
		sb.WriteString(fmt.Sprintf("--- a/%s\n", relPath))
		sb.WriteString("+++ /dev/null\n")

		if isBinary(oldBytes) {
			sb.WriteString(fmt.Sprintf("Binary files a/%s and /dev/null differ\n", relPath))
			return sb.String()
		}

		lines := splitLines(oldBytes)
		if len(lines) == 0 {
			return sb.String()
		}
		sb.WriteString(ui.StyleMeta.Render(fmt.Sprintf("@@ -1,%d +0,0 @@", len(lines))) + "\n")
		for _, l := range lines {
			sb.WriteString(ui.StyleDenied.Render("-"+l) + "\n")
		}

	case ChangeModified:
		sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", relPath, relPath))
		sb.WriteString(fmt.Sprintf("--- a/%s\n", relPath))
		sb.WriteString(fmt.Sprintf("+++ b/%s\n", relPath))

		if isBinary(oldBytes) || isBinary(newBytes) {
			sb.WriteString(fmt.Sprintf("Binary files a/%s and b/%s differ\n", relPath, relPath))
			return sb.String()
		}

		oldLines := splitLines(oldBytes)
		newLines := splitLines(newBytes)
		diffLines := computeLCSDiff(oldLines, newLines)
		hunks := buildHunks(diffLines, 3)

		for _, h := range hunks {
			hunkHeader := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.oldStart, h.oldCount, h.newStart, h.newCount)
			sb.WriteString(ui.StyleMeta.Render(hunkHeader) + "\n")
			for _, l := range h.lines {
				switch l.Op {
				case DiffEqual:
					sb.WriteString(" " + l.Text + "\n")
				case DiffInsert:
					sb.WriteString(ui.StyleAllowed.Render("+"+l.Text) + "\n")
				case DiffDelete:
					sb.WriteString(ui.StyleDenied.Render("-"+l.Text) + "\n")
				}
			}
		}
	}

	return sb.String()
}

// RunShadowPatch generates a unified diff of changes in upperDir relative to lowerDir.
// If paths are supplied, only changes matching those paths are included.
func RunShadowPatch(lowerDir, upperDir string, out io.Writer, paths ...string) error {
	changes, err := ScanShadowChanges(lowerDir, upperDir)
	if err != nil {
		return err
	}
	if len(paths) > 0 {
		changes = filterChanges(changes, paths, lowerDir)
	}

	if len(changes) == 0 {
		fmt.Fprintln(out, ui.StyleMeta.Render("Working tree is clean. No changes detected."))
		return nil
	}

	var fullOutput strings.Builder
	for _, change := range changes {
		var oldBytes, newBytes []byte
		var readErr error

		if change.Type == ChangeModified || change.Type == ChangeDeleted {
			oldPath := filepath.Join(lowerDir, change.Path)
			oldBytes, readErr = os.ReadFile(oldPath)
			if readErr != nil && !os.IsNotExist(readErr) {
				return fmt.Errorf("safebox: failed to read original file %q: %w", oldPath, readErr)
			}
		}

		if change.Type == ChangeAdded || change.Type == ChangeModified {
			newPath := filepath.Join(upperDir, change.Path)
			newBytes, readErr = os.ReadFile(newPath)
			if readErr != nil {
				return fmt.Errorf("safebox: failed to read staged file %q: %w", newPath, readErr)
			}
		}

		patch := FormatFilePatch(change.Path, oldBytes, newBytes, change.Type)
		fullOutput.WriteString(patch)
	}

	_, err = fmt.Fprint(out, fullOutput.String())
	return err
}

// RunGitPatch generates a unified diff for git-tracked workspaces without an active session.
func RunGitPatch(workDir string, out io.Writer, paths ...string) error {
	args := []string{"diff"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	cmd.Env = os.Environ()

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("safebox: git diff failed: %w", err)
	}

	if len(bytes.TrimSpace(output)) == 0 {
		fmt.Fprintln(out, ui.StyleMeta.Render("Working tree is clean. No changes detected."))
		return nil
	}

	_, err = out.Write(output)
	return err
}
