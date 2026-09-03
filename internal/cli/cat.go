package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"safebox/internal/revert"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunCat streams the contents of a file staged in the active session upper layer (or host).
func RunCat(args []string, tr *trace.Tracer) int {
	var paths []string
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			PrintCatHelp(os.Stdout)
			return 0
		}
		if arg == "-q" || arg == "--quiet" || arg == "--quiet=true" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "%s safebox cat: unknown flag %q\n\n", ui.StyleDenied.Render("ERROR"), arg)
			PrintCatHelp(os.Stderr)
			return 2
		}
		paths = append(paths, arg)
	}

	if len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "%s safebox cat: requires at least one file path\n\n", ui.StyleDenied.Render("ERROR"))
		PrintCatHelp(os.Stderr)
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		PrintSubcommandError("cat", fmt.Errorf("failed to get working directory: %w", err))
		return 1
	}

	var sess *revert.Session
	_ = tr.Step("session discovery", func() error {
		var sErr error
		sess, sErr = revert.MostRecentSession(cwd, false)
		return sErr
	})

	for _, target := range paths {
		relPath := filepath.Clean(target)
		if filepath.IsAbs(target) {
			rel, err := filepath.Rel(cwd, target)
			if err == nil && !strings.HasPrefix(rel, "..") {
				relPath = rel
			}
		}

		found := false

		// 1. Check session upper layer if session is active
		if sess != nil && sess.UpperDir != "" {
			upperFile := filepath.Join(sess.UpperDir, relPath)
			if fi, err := os.Lstat(upperFile); err == nil {
				if revert.IsWhiteout(fi) {
					fmt.Fprintf(os.Stderr, "%s safebox cat: file %q was deleted in active session\n", ui.StyleDenied.Render("ERROR"), target)
					return 1
				}
				if fi.IsDir() {
					fmt.Fprintf(os.Stderr, "%s safebox cat: %q is a directory\n", ui.StyleDenied.Render("ERROR"), target)
					return 1
				}
				f, err := os.Open(upperFile)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s safebox cat: failed to read %q: %v\n", ui.StyleDenied.Render("ERROR"), target, err)
					return 1
				}
				_, copyErr := io.Copy(os.Stdout, f)
				f.Close()
				if copyErr != nil {
					return 1
				}
				found = true
			}
		}

		// 2. Fall back to host working directory
		if !found {
			hostFile := filepath.Join(cwd, relPath)
			if fi, err := os.Stat(hostFile); err == nil {
				if fi.IsDir() {
					fmt.Fprintf(os.Stderr, "%s safebox cat: %q is a directory\n", ui.StyleDenied.Render("ERROR"), target)
					return 1
				}
				f, err := os.Open(hostFile)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s safebox cat: failed to read %q: %v\n", ui.StyleDenied.Render("ERROR"), target, err)
					return 1
				}
				_, copyErr := io.Copy(os.Stdout, f)
				f.Close()
				if copyErr != nil {
					return 1
				}
				found = true
			}
		}

		if !found {
			fmt.Fprintf(os.Stderr, "%s safebox cat: file %q not found\n", ui.StyleDenied.Render("ERROR"), target)
			return 1
		}
	}

	return 0
}

// PrintCatHelp outputs the help text for 'safebox cat'.
func PrintCatHelp(out io.Writer) {
	fmt.Fprintf(out, "Usage: safebox cat [options] <file...>\n\n")
	fmt.Fprintf(out, "Output the contents of a file from the active sandbox session (or host).\n\n")
	fmt.Fprintf(out, "Arguments:\n")
	fmt.Fprintf(out, "  <file...>  Path to one or more files to output\n\n")
	fmt.Fprintf(out, "Options:\n")
	fmt.Fprintf(out, "  -q, --quiet  Suppress trace step timing on stderr\n")
	fmt.Fprintf(out, "  -h, --help   Show this help message\n\n")
	fmt.Fprintf(out, "Examples:\n")
	fmt.Fprintf(out, "  safebox cat go-server.go\n")
	fmt.Fprintf(out, "  safebox cat -q go-server.go > local-server.go\n")
	fmt.Fprintf(out, "  safebox cat internal/cli/cat.go | less\n")
}
