// Package cli provides shared CLI flag parsing for safebox parent and child re-exec processes.
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// RunFlags represents parsed command-line flags for 'safebox run' and 'safebox __child__'.
type RunFlags struct {
	AllowPathsRO          []string
	AllowPathsRW          []string
	AllowFilesRW          []string
	PersistentStateMounts []string
	AllowNetwork          []string
	AllowNet              bool // v1 binary toggle: --allow-net (bare) or --allow-net=* grants full internet egress
	SessionDir            string
	NetConfigPath         string
	Quiet                 bool
	Probe                 bool
}

// DiffApplyFlags represents parsed command-line flags for 'diff', 'apply', and 'revert'.
type DiffApplyFlags struct {
	Quiet        bool
	Yes          bool
	ForceDiscard bool
	Paths        []string
}

// ParseRunFlags parses command-line arguments for 'safebox run', strictly requiring the '--' delimiter.
func ParseRunFlags(args []string) (RunFlags, []string, error) {
	var (
		allowPathsRO          []string
		allowPathsRW          []string
		allowFilesRW          []string
		persistentStateMounts []string
		allowNetwork          []string
		allowNet              bool
		sessionDir            string
		netConfigPath         string
		quiet                 bool
		probe                 bool
		cmdArgs               []string
		seenDoubleDash        bool
		i                     int
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
				if postArg == "--allow-file-rw" || strings.HasPrefix(postArg, "--allow-file-rw=") {
					return RunFlags{}, nil, errors.New("safebox: --allow-file-rw must precede the -- delimiter; current invocation places it inside the wrapped command arguments")
				}
				if postArg == "--persistent-state" || strings.HasPrefix(postArg, "--persistent-state=") {
					return RunFlags{}, nil, errors.New("safebox: --persistent-state must precede the -- delimiter; current invocation places it inside the wrapped command arguments")
				}
				if postArg == "--allow-network" || strings.HasPrefix(postArg, "--allow-network=") {
					return RunFlags{}, nil, errors.New("safebox: --allow-network was deprecated in v0.4; use --allow-net (binary toggle, full internet egress) instead")
				}
				if postArg == "--allow-net" || strings.HasPrefix(postArg, "--allow-net=") {
					return RunFlags{}, nil, errors.New("safebox: --allow-net must precede the -- delimiter; current invocation places it inside the wrapped command arguments")
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

		if strings.HasPrefix(arg, "--allow-file-rw=") {
			p := strings.TrimPrefix(arg, "--allow-file-rw=")
			if p != "" {
				allowFilesRW = append(allowFilesRW, p)
			}
			i++
			continue
		}
		if arg == "--allow-file-rw" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --allow-file-rw requires a file argument")
			}
			allowFilesRW = append(allowFilesRW, args[i+1])
			i += 2
			continue
		}

		if strings.HasPrefix(arg, "--persistent-state=") {
			p := strings.TrimPrefix(arg, "--persistent-state=")
			if p != "" {
				persistentStateMounts = append(persistentStateMounts, p)
			}
			i++
			continue
		}
		if arg == "--persistent-state" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --persistent-state requires a host:mount mapping")
			}
			persistentStateMounts = append(persistentStateMounts, args[i+1])
			i += 2
			continue
		}

		if strings.HasPrefix(arg, "--allow-network=") {
			return RunFlags{}, nil, errors.New("safebox: --allow-network=<domain> was deprecated in v0.4; use --allow-net (binary toggle) for full internet egress. Domain-restricted egress is planned for v0.5")
		}
		if arg == "--allow-network" {
			return RunFlags{}, nil, errors.New("safebox: --allow-network <domain> was deprecated in v0.4; use --allow-net (binary toggle) for full internet egress. Domain-restricted egress is planned for v0.5")
		}

		if strings.HasPrefix(arg, "--allow-net=") {
			val := strings.TrimPrefix(arg, "--allow-net=")
			switch val {
			case "*", "true", "1":
				allowNet = true
			case "false", "0":
				allowNet = false
			default:
				return RunFlags{}, nil, fmt.Errorf("safebox: --allow-net=%q not supported in v1 (only --allow-net, --allow-net=*, or --allow-net=false; domain-restricted egress is planned for v0.5)", val)
			}
			i++
			continue
		}
		if arg == "--allow-net" {
			allowNet = true
			i++
			continue
		}
		if arg == "--allow-net=false" {
			allowNet = false
			i++
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
		AllowPathsRO:          allowPathsRO,
		AllowPathsRW:          allowPathsRW,
		AllowFilesRW:          allowFilesRW,
		PersistentStateMounts: persistentStateMounts,
		AllowNetwork:          allowNetwork,
		AllowNet:              allowNet,
		SessionDir:            sessionDir,
		NetConfigPath:         netConfigPath,
		Quiet:                 quiet,
		Probe:                 probe,
	}, cmdArgs, nil
}

// ParseChildFlags parses command-line arguments for 'safebox __child__', leniently without requiring '--'.
func ParseChildFlags(args []string) (RunFlags, []string, error) {
	var (
		allowPathsRO          []string
		allowPathsRW          []string
		allowFilesRW          []string
		persistentStateMounts []string
		allowNetwork          []string
		allowNet              bool
		sessionDir            string
		netConfigPath         string
		quiet                 bool
		probe                 bool
		cmdArgs               []string
		i                     int
	)

	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			cmdArgs = append(cmdArgs, args[i+1:]...)
			break
		}

		if strings.HasPrefix(arg, "--net-config=") {
			netConfigPath = strings.TrimPrefix(arg, "--net-config=")
			i++
			continue
		}
		if arg == "--net-config" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --net-config requires a path argument")
			}
			netConfigPath = args[i+1]
			i += 2
			continue
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

		if strings.HasPrefix(arg, "--allow-file-rw=") {
			p := strings.TrimPrefix(arg, "--allow-file-rw=")
			if p != "" {
				allowFilesRW = append(allowFilesRW, p)
			}
			i++
			continue
		}
		if arg == "--allow-file-rw" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --allow-file-rw requires a file argument")
			}
			allowFilesRW = append(allowFilesRW, args[i+1])
			i += 2
			continue
		}

		if strings.HasPrefix(arg, "--persistent-state=") {
			p := strings.TrimPrefix(arg, "--persistent-state=")
			if p != "" {
				persistentStateMounts = append(persistentStateMounts, p)
			}
			i++
			continue
		}
		if arg == "--persistent-state" {
			if i+1 >= len(args) {
				return RunFlags{}, nil, errors.New("safebox: --persistent-state requires a host:mount mapping")
			}
			persistentStateMounts = append(persistentStateMounts, args[i+1])
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

	// Environment variable SAFEBOX_NET_CONFIG takes precedence if set
	if envNet := os.Getenv("SAFEBOX_NET_CONFIG"); envNet != "" {
		netConfigPath = envNet
	}

	return RunFlags{
		AllowPathsRO:          allowPathsRO,
		AllowPathsRW:          allowPathsRW,
		AllowFilesRW:          allowFilesRW,
		PersistentStateMounts: persistentStateMounts,
		AllowNetwork:          allowNetwork,
		AllowNet:              allowNet,
		SessionDir:            sessionDir,
		NetConfigPath:         netConfigPath,
		Quiet:                 quiet,
		Probe:                 probe,
	}, cmdArgs, nil
}

// ParseDiffApplyFlags parses command-line flags for 'diff', 'apply', and 'revert'.
func ParseDiffApplyFlags(args []string, command string) (DiffApplyFlags, error) {
	var (
		quiet        bool
		yes          bool
		forceDiscard bool
		paths        []string
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
		if arg == "--force-discard" || arg == "--force-discard=true" {
			forceDiscard = true
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
		Quiet:        quiet,
		Yes:          yes,
		ForceDiscard: forceDiscard,
		Paths:        paths,
	}, nil
}
