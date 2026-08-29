package revert

import (
	"fmt"
	"io"
	"strings"

	"safebox/internal/ui"
)

// FormatChanges formats structured file changes into styled terminal output
// conforming to FR7 color tokens:
// - Added and modified files are rendered with StyleAllowed (green 34)
// - Deleted files are rendered with StyleDenied (red 196 bold)
// - Clean working tree notification is rendered with StyleMeta (gray 240)
func FormatChanges(changes []FileChange) string {
	if len(changes) == 0 {
		return ui.StyleMeta.Render("Working tree is clean. No changes detected.")
	}

	var sb strings.Builder
	for _, change := range changes {
		switch change.Type {
		case ChangeAdded, ChangeUntracked:
			sb.WriteString(fmt.Sprintf("%s %s\n", ui.StyleAllowed.Render("+ [ADDED]"), ui.StyleAllowed.Render(change.Path)))
		case ChangeModified:
			sb.WriteString(fmt.Sprintf("%s %s\n", ui.StyleAllowed.Render("~ [MODIFIED]"), ui.StyleAllowed.Render(change.Path)))
		case ChangeDeleted:
			sb.WriteString(fmt.Sprintf("%s %s\n", ui.StyleDenied.Render("- [DELETED]"), ui.StyleDenied.Render(change.Path)))
		default:
			sb.WriteString(fmt.Sprintf("%s %s\n", ui.StyleAllowed.Render("~ [MODIFIED]"), ui.StyleAllowed.Render(change.Path)))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// RunDiff inspects the working directory and outputs the formatted change summary.
// If paths are supplied, only changes falling under those paths are displayed.
func RunDiff(workDir string, out io.Writer, paths ...string) error {
	changes, err := GetStatus(workDir)
	if err != nil {
		return err
	}
	if len(paths) > 0 {
		changes = filterChanges(changes, paths, workDir)
	}
	fmt.Fprintln(out, FormatChanges(changes))
	return nil
}
