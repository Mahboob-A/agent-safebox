package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"safebox/internal/isolation"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunChild executes the '__child__' unshared namespace process.
func RunChild(args []string, _ *trace.Tracer) int {
	if err := syscall.Mount("none", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		fmt.Fprintf(os.Stderr, "[safebox] warning: child mount make-private failed: %v\n", err)
	}

	flags, cmdArgs, err := ParseChildFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 2
	}

	tr := trace.NewChild(!flags.Quiet)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s safebox: cannot resolve current directory: %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 1
	}

	if flags.SessionDir != "" {
		upperDir := filepath.Join(flags.SessionDir, "upper")
		workDir := filepath.Join(flags.SessionDir, "work")
		mergedDir := filepath.Join(flags.SessionDir, "merged")

		if err := tr.Step("overlayfs mount", func() error {
			return isolation.MountSessionOverlay(cwd, upperDir, workDir, mergedDir)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: overlay mount failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
			return 1
		}
		defer isolation.UnmountOverlay(mergedDir)
		if err := os.Chdir(mergedDir); err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: cannot change directory to overlay: %v\n", ui.StyleDenied.Render("ERROR"), err)
			return 1
		}
	}

	if err := tr.Step("landlock restrict", func() error {
		return isolation.ApplyLandlock(flags.AllowPathsRO, flags.AllowPathsRW)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "%s safebox: landlock sandbox failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 1
	}

	code, err := isolation.RunShim(cmdArgs, tr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s safebox: exec failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
		if hint := HintFor(err, cmdArgs); hint != "" {
			fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
		}
		return 1
	}

	return code
}
