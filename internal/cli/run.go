package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"safebox/internal/isolation"
	"safebox/internal/netpolicy"
	"safebox/internal/persistentstate"
	"safebox/internal/profiles"
	"safebox/internal/revert"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunRun executes the 'safebox run' parent command.
func RunRun(args []string, tr *trace.Tracer) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "%s safebox run: no command specified\n\n", ui.StyleDenied.Render("ERROR"))
		PrintUsageTo(os.Stderr)
		return 1
	}

	flags, cmdArgs, err := ParseRunFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n\n", ui.StyleDenied.Render("ERROR"), err)
		PrintUsageTo(os.Stderr)
		return 2
	}

	if flags.Probe {
		return RunProbe(flags, cmdArgs)
	}

	if len(cmdArgs) == 0 {
		fmt.Fprintf(os.Stderr, "%s safebox: no command specified after '--'\n\n", ui.StyleDenied.Render("ERROR"))
		PrintUsageTo(os.Stderr)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s safebox: cannot resolve current directory: %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 1
	}

	// Profile resolution: combine profile-derived allow paths with CLI-flag allow paths.
	combinedRO := flags.AllowPathsRO
	combinedRW := flags.AllowPathsRW
	combinedFilesRW := flags.AllowFilesRW
	combinedNetwork := append([]string(nil), flags.AllowNetwork...)
	persistentStateMounts := append([]string(nil), flags.PersistentStateMounts...)
	resolvedBin, lookErr := exec.LookPath(cmdArgs[0])
	if lookErr == nil {
		if prof, lookupErr := profiles.Lookup(resolvedBin); lookupErr == nil && prof != nil {
			_ = tr.Step("profile resolution", func() error {
				if tr.Enabled {
					out := tr.Out
					if out == nil {
						out = os.Stderr
					}
					fmt.Fprintf(out, "  -> using profile: %s\n", prof.Binary.Name)
				}
				return nil
			})
			combinedRO = append(combinedRO, prof.Paths.AllowRO...)
			combinedRW = append(combinedRW, prof.Paths.AllowRW...)
			combinedFilesRW = append(combinedFilesRW, prof.Paths.AllowRWFiles...)
			combinedNetwork = append(combinedNetwork, prof.Network.AllowDomains...)
			// Profile-level allow_net flag is OR'd with the CLI --allow-net
			// flag below at the egress-decision point (allowNet := flags.AllowNet
			// || dedupedNetwork-from-profiles-combo); here we set the profile's
			// value so the OR is wired correctly.
			flags.AllowNet = flags.AllowNet || prof.Network.AllowNet

			if prof.PersistentState.MountAt != "" {
				hostDir := prof.PersistentState.HostDir
				if hostDir == "" {
					var hErr error
					hostDir, hErr = persistentstate.Ensure(prof.Binary.Name)
					if hErr != nil {
						fmt.Fprintf(os.Stderr, "%s safebox: failed to prepare persistent state dir: %v\n", ui.StyleDenied.Render("ERROR"), hErr)
						return 1
					}
				} else {
					if _, hErr := persistentstate.EnsurePath(hostDir, 0700); hErr != nil {
						fmt.Fprintf(os.Stderr, "%s safebox: failed to prepare persistent state dir: %v\n", ui.StyleDenied.Render("ERROR"), hErr)
						return 1
					}
				}
				mapping := fmt.Sprintf("%s:%s", hostDir, prof.PersistentState.MountAt)
				persistentStateMounts = append(persistentStateMounts, mapping)
				combinedRW = append(combinedRW, prof.PersistentState.MountAt)
			}
		} else if lookupErr == nil {
			_ = tr.Step("profile resolution", func() error {
				if tr.Enabled {
					out := tr.Out
					if out == nil {
						out = os.Stderr
					}
					fmt.Fprintf(out, "  -> no profile found for %s\n", filepath.Base(resolvedBin))
				}
				return nil
			})
		} else {
			_ = tr.Step("profile resolution", func() error {
				if tr.Enabled {
					out := tr.Out
					if out == nil {
						out = os.Stderr
					}
					fmt.Fprintf(out, "  -> profile lookup warning: %v\n", lookupErr)
				}
				return nil
			})
			fmt.Fprintf(os.Stderr, "[safebox:profiles] warning: %s: %v\n", filepath.Base(resolvedBin), lookupErr)
		}
	}

	// Deduplicate allowed network domains
	var dedupedNetwork []string
	seenDomain := make(map[string]bool)
	for _, d := range combinedNetwork {
		d = strings.TrimSpace(d)
		if d != "" && !seenDomain[d] {
			seenDomain[d] = true
			dedupedNetwork = append(dedupedNetwork, d)
		}
	}

	var sess *revert.Session
	if err := tr.Step("session initialize", func() error {
		var sErr error
		sess, sErr = revert.CreateSession(cwd)
		return sErr
	}); err != nil {
		var activeErr *revert.ErrSessionAlreadyActive
		if errors.As(err, &activeErr) {
			fmt.Fprintf(os.Stderr, "%s safebox: session is already active in %s (PID %d)\n", ui.StyleDenied.Render("ERROR"), cwd, activeErr.PID)
			fmt.Fprintf(os.Stderr, "  -> hint: %s\n", activeErr.Hint())
			return 6
		}
		fmt.Fprintf(os.Stderr, "%s safebox: failed to initialize session: %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 1
	}
	defer sess.ReleaseActiveLock()

	// Decide whether egress is requested (v1 binary toggle).
	// In v1, only the explicit --allow-net flag activates egress. Profile-derived
	// [network].allow_domains entries are parsed and combined into
	// dedupedNetwork for v0.5 seam preservation, but they do NOT themselves
	// trigger egress - they ride along only when --allow-net is also set.
	allowNet := flags.AllowNet

	// Pre-allocate the netconfig path so we can pass it to the child before
	// writing the file. The child is told where to look via --net-config;
	// it will block on the net-ready pipe first, by which time the parent
	// will have written the file.
	netConfigPath := filepath.Join(sess.BaseDir, "netconfig.json")

	var teardownEgress func() error
	var backend string

	if allowNet {
		var bErr error
		backend, bErr = netpolicy.DiscoverBackend()
		if bErr != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: no network backend available: %v\n", ui.StyleDenied.Render("ERROR"), bErr)
			fmt.Fprintf(os.Stderr, "  -> hint: install slirp4netns (apt install slirp4netns) or pasta (apt install passt)\n")
			return 4
		}
	}

	// Start the namespaced child BEFORE setting up the network backend.
	// This enforces the create-then-attach-then-resume pattern: the child
	// must exist (and own its CLONE_NEWNET netns) before slirp4netns/pasta
	// can attach to it. The child blocks on the net-ready pipe until the
	// parent either confirms backend readiness (pipe closed) or determines
	// default-deny (also pipe closed).
	egressConfigPathForChild := netConfigPath
	if !allowNet {
		egressConfigPathForChild = ""
	}

	var handle *isolation.ChildHandle
	err = tr.Step("wrapped command spawn", func() error {
		var sErr error
		handle, sErr = isolation.StartChild(combinedRO, combinedRW, combinedFilesRW, persistentStateMounts, egressConfigPathForChild, sess.BaseDir, flags.Quiet, cmdArgs)
		return sErr
	})
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "%s safebox: exec failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
		if hint := HintFor(err, cmdArgs); hint != "" {
			fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
		}
		return 1
	}

	if allowNet {
		// Attach the userspace NAT backend to the child's PID now that
		// the child's netns exists. SetupEgress always returns a populated
		// cfg and an active backend when allowNet is true (v1 binary toggle
		// is full-internet via userspace NAT, regardless of allowDomains).
		childPID := handle.Cmd.Process.Pid
		cfg, teardown, sErr := netpolicy.SetupEgress(dedupedNetwork, allowNet, childPID)
		if sErr != nil {
			if wErr := os.WriteFile(netConfigPath, []byte("{}"), 0600); wErr != nil {
				handle.CloseNetReady()
				_ = handle.Wait()
				fmt.Fprintf(os.Stderr, "%s safebox: failed to write empty net config: %v\n", ui.StyleDenied.Render("ERROR"), wErr)
				return 1
			}
			handle.CloseNetReady()
			if wErr := handle.Wait(); wErr != nil {
				var exitErr *exec.ExitError
				if errors.As(wErr, &exitErr) {
					return exitErr.ExitCode()
				}
			}
			if len(flags.AllowNetwork) > 0 || flags.AllowNet {
				fmt.Fprintf(os.Stderr, "%s safebox: network egress setup failed: %v\n", ui.StyleDenied.Render("ERROR"), sErr)
				return 1
			}
			if tr.Enabled {
				out := tr.Out
				if out == nil {
					out = os.Stderr
				}
				fmt.Fprintf(out, "[safebox] warning: profile network egress offline: %v\n", sErr)
			}
			teardownEgress = nil
		} else {
			if tr.Enabled {
				out := tr.Out
				if out == nil {
					out = os.Stderr
				}
				fmt.Fprintf(out, "[safebox] egress setup ok %s allowed=%s\n", cfg.BackendName, strings.Join(cfg.AllowedDomains, ","))
			}
			teardownEgress = teardown
			backend = cfg.BackendName

			data, mErr := json.Marshal(cfg)
			if mErr != nil {
				handle.CloseNetReady()
				_ = handle.Wait()
				fmt.Fprintf(os.Stderr, "%s safebox: failed to marshal net config: %v\n", ui.StyleDenied.Render("ERROR"), mErr)
				return 1
			}
			if wErr := os.WriteFile(netConfigPath, data, 0600); wErr != nil {
				handle.CloseNetReady()
				_ = handle.Wait()
				fmt.Fprintf(os.Stderr, "%s safebox: failed to write net config to %s: %v\n", ui.StyleDenied.Render("ERROR"), netConfigPath, wErr)
				return 1
			}
		}
	}

	// Release the net-ready gate so the child proceeds past its pre-egress
	// barrier. If we never set up egress (default-deny), closing the fd
	// without writing unblocks the child's read with EOF, which is fine.
	handle.CloseNetReady()

	if err := handle.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if teardownEgress != nil {
				_ = teardownEgress()
			}
			return exitErr.ExitCode()
		}
		if teardownEgress != nil {
			_ = teardownEgress()
		}
		fmt.Fprintf(os.Stderr, "%s safebox: exec failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
		if hint := HintFor(err, cmdArgs); hint != "" {
			fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
		}
		return 1
	}

	if teardownEgress != nil {
		start := time.Now()
		_ = teardownEgress()
		if tr.Enabled {
			out := tr.Out
			if out == nil {
				out = os.Stderr
			}
			fmt.Fprintf(out, "[safebox] egress teardown ok %s backend=%s\n", time.Since(start), backend)
		}
	}

	return 0
}
