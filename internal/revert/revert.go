package revert

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	osexec "os/exec"
	"strings"

	"safebox/internal/ui"
)

var ErrRevertCancelled = errors.New("revert cancelled")

// Revert discards all working tree modifications (modified, deleted, and untracked files).
// If force is false, it prompts the user on out and reads from in before proceeding.
func Revert(workDir string, force bool, in io.Reader, out io.Writer) error {
	isRepo, err := IsGitRepo(workDir)
	if err != nil {
		return err
	}
	if !isRepo {
		return ErrNotGitRepo
	}

	if !force {
		if in == nil {
			fmt.Fprintln(out, ui.StyleDenied.Render("Revert cancelled. Pass --yes to skip confirmation."))
			return ErrRevertCancelled
		}
		fmt.Fprint(out, ui.StyleMeta.Render("Are you sure you want to discard all working tree changes? [y/N]: "))
		reader := bufio.NewReader(in)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("safebox: failed to read confirmation: %w", err)
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, ui.StyleDenied.Render("Revert cancelled. Pass --yes to skip confirmation."))
			return ErrRevertCancelled
		}
	}

	checkoutCmd := osexec.Command("git", "-C", workDir, "checkout", "--", ".")
	if outBytes, err := checkoutCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("safebox: git checkout failed: %w (output: %s)", err, string(outBytes))
	}

	cleanCmd := osexec.Command("git", "-C", workDir, "clean", "-fd")
	if outBytes, err := cleanCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("safebox: git clean failed: %w (output: %s)", err, string(outBytes))
	}

	fmt.Fprintln(out, ui.StyleAllowed.Render("Working tree restored."))
	return nil
}
