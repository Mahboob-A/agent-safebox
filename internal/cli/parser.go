// Package cli provides shared CLI flag parsing for safebox parent and child re-exec processes.
// Runners (run.go, child.go, etc.) are added in Phase 12 per master plan.
package cli

import (
	"errors"
	"fmt"
	"strings"
)

// RunFlags represents parsed command-line flags for 'safebox run' and 'safebox __child__'.
type RunFlags struct {
	AllowPathsRO []string
	AllowPathsRW []string
	SessionDir   string
	Quiet        bool
	Probe        bool
}

// DiffApplyFlags represents parsed command-line flags for 'diff', 'apply', and 'revert'.
type DiffApplyFlags struct {
	Quiet bool
	Yes   bool
	Paths []string
}

// ParseRunFlags parses command-line arguments for 'safebox run', strictly requiring the '--' delimiter.
func ParseRunFlags(args []string) (RunFlags, []string, error) {
	var (
		allowPathsRO   []string
		allowPathsRW   []string
		sessionDir     string
		quiet          bool
		probe          bool
		cmdArgs        []string
		seenDoubleDash bool
		i              int
	)

	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			seenDoubleDash = true
			// Check for misplaced flags placed after '--'
			for _, postArg := range args[i+1:] {
				if postArg == "--allow-path" || strings.HasPrefix(postArg, "--allow-path=") {
					return RunFlags{}, nil, errors.New("safebox: --allow-path must precede the -- delimiter; current invocation places it inside the wrapped command arguments")
				}
				if postArg == "--allow-path-rw" || strings.HasPrefix(postArg, "--allow-path-rw=") {
					return RunFlags{}, nil, errors.New("safebox: --allow-path-rw must precede the -- delimiter; current invocation places it inside the wrapped command arguments")
				}
			}
			cmdArgs = append(cmdArgs, args[i+1:]...)
			break
		}

		if strings.HasPrefix(arg, "--allow-path=") {
			p := strings.TrimPrefix(arg, "--allow-path=")
			if p != "" {
				allowPathsRO = append(allowPathsRO, p)
			}
			i++
			continue
		}
		if arg == "--allow-path" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --allow-path requires a directory argument")
			}
			allowPathsRO = append(allowPathsRO, args[i+1])
			i += 2
			continue
		}

		if strings.HasPrefix(arg, "--allow-path-rw=") {
			p := strings.TrimPrefix(arg, "--allow-path-rw=")
			if p != "" {
				allowPathsRW = append(allowPathsRW, p)
			}
			i++
			continue
		}
		if arg == "--allow-path-rw" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --allow-path-rw requires a directory argument")
			}
			allowPathsRW = append(allowPathsRW, args[i+1])
			i += 2
			continue
		}

		if strings.HasPrefix(arg, "--session-dir=") {
			sessionDir = strings.TrimPrefix(arg, "--session-dir=")
			i++
			continue
		}
		if arg == "--session-dir" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --session-dir requires a directory argument")
			}
			sessionDir = args[i+1]
			i += 2
			continue
		}

		if arg == "--quiet" || arg == "-q" || arg == "--quiet=true" {
			quiet = true
			i++
			continue
		}
		if arg == "--probe" || arg == "--probe=true" {
			probe = true
			i++
			continue
		}

		// Encountered non-flag argument before '--'
		return RunFlags{}, nil, errors.New("safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)")
	}

	if !seenDoubleDash {
		return RunFlags{}, nil, errors.New("safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)")
	}
	if len(cmdArgs) == 0 {
		return RunFlags{}, nil, errors.New("safebox run: no command specified")
	}

	return RunFlags{
		AllowPathsRO: allowPathsRO,
		AllowPathsRW: allowPathsRW,
		SessionDir:   sessionDir,
		Quiet:        quiet,
		Probe:        probe,
	}, cmdArgs, nil
}

// ParseChildFlags parses command-line arguments for 'safebox __child__', leniently without requiring '--'.
func ParseChildFlags(args []string) (RunFlags, []string, error) {
	var (
		allowPathsRO []string
		allowPathsRW []string
		sessionDir   string
		quiet        bool
		probe        bool
		cmdArgs      []string
		i            int
	)

	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			cmdArgs = append(cmdArgs, args[i+1:]...)
			break
		}

		if strings.HasPrefix(arg, "--allow-path=") {
			p := strings.TrimPrefix(arg, "--allow-path=")
			if p != "" {
				allowPathsRO = append(allowPathsRO, p)
			}
			i++
			continue
		}
		if arg == "--allow-path" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --allow-path requires a directory argument")
			}
			allowPathsRO = append(allowPathsRO, args[i+1])
			i += 2
			continue
		}

		if strings.HasPrefix(arg, "--allow-path-rw=") {
			p := strings.TrimPrefix(arg, "--allow-path-rw=")
			if p != "" {
				allowPathsRW = append(allowPathsRW, p)
			}
			i++
			continue
		}
		if arg == "--allow-path-rw" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --allow-path-rw requires a directory argument")
			}
			allowPathsRW = append(allowPathsRW, args[i+1])
			i += 2
			continue
		}

		if strings.HasPrefix(arg, "--session-dir=") {
			sessionDir = strings.TrimPrefix(arg, "--session-dir=")
			i++
			continue
		}
		if arg == "--session-dir" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --session-dir requires a directory argument")
			}
			sessionDir = args[i+1]
			i += 2
			continue
		}

		if arg == "--quiet" || arg == "-q" || arg == "--quiet=true" {
			quiet = true
			i++
			continue
		}
		if arg == "--probe" || arg == "--probe=true" {
			probe = true
			i++
			continue
		}

		// First non-flag argument starts the command
		cmdArgs = append(cmdArgs, args[i:]...)
		break
	}

	return RunFlags{
		AllowPathsRO: allowPathsRO,
		AllowPathsRW: allowPathsRW,
		SessionDir:   sessionDir,
		Quiet:        quiet,
		Probe:        probe,
	}, cmdArgs, nil
}

// ParseDiffApplyFlags parses command-line flags for 'diff', 'apply', and 'revert'.
func ParseDiffApplyFlags(args []string, command string) (DiffApplyFlags, error) {
	var (
		quiet bool
		yes   bool
		paths []string
	)

	for _, arg := range args {
		if arg == "--shadow" || strings.HasPrefix(arg, "--shadow=") {
			return DiffApplyFlags{}, fmt.Errorf("safebox %s: unknown flag '--shadow'", command)
		}
		if arg == "--quiet" || arg == "-q" || arg == "--quiet=true" {
			quiet = true
			continue
		}
		if arg == "--yes" || arg == "-y" || arg == "--yes=true" {
			yes = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return DiffApplyFlags{}, fmt.Errorf("safebox %s: unknown flag %q", command, arg)
		}
		if command == "diff" {
			paths = append(paths, arg)
		} else {
			return DiffApplyFlags{}, fmt.Errorf("safebox %s: takes no positional arguments, got %q", command, arg)
		}
	}

	return DiffApplyFlags{
		Quiet: quiet,
		Yes:   yes,
		Paths: paths,
	}, nil
}
