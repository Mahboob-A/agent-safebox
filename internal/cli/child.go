package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"safebox/internal/isolation"
	"safebox/internal/netpolicy"
	"safebox/internal/persistentstate"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunChild executes the '__child__' unshared namespace process.
func RunChild(args []string, _ *trace.Tracer) int {
	if ourNS, err := os.Readlink("/proc/self/ns/net"); err == nil {
		if parentNS, pErr := os.Readlink("/proc/1/ns/net"); pErr == nil && ourNS == parentNS {
			fmt.Fprintf(os.Stderr, "FATAL: child shares network namespace with init (CLONE_NEWNET bypassed)\n")
			return 7
		}
	}

	// NFR1 hard-fail: a failed make-private leaves us in a shared mount
	// namespace where any subsequent mount operation leaks to the host.
	// This was the root cause of the v0.4 P0 bug that bricked host DNS.
	if err := syscall.Mount("none", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: child mount make-private failed (would leak host mounts): %v\n", err)
		return 1
	}

	flags, cmdArgs, err := ParseChildFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 2
	}

	// Block on the net-ready gate: the parent must complete (or skip) egress
	// backend attachment before the child applies its egress config. Without
	// this barrier the child races the parent and bind-mounts synthetic
	// /etc/hosts and /etc/resolv.conf before the NAT backend is up.
	if fdStr := os.Getenv("SAFEBOX_NET_READY_FD"); fdStr != "" {
		fd, perr := strconv.Atoi(fdStr)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "FATAL: invalid SAFEBOX_NET_READY_FD env var %q: %v\n", fdStr, perr)
			return 1
		}
		// Read exactly one byte; the parent writes one byte (or closes the
		// fd) once backend readiness is confirmed. Either way, we proceed.
		buf := make([]byte, 1)
		if _, rerr := syscall.Read(fd, buf); rerr != nil {
			fmt.Fprintf(os.Stderr, "FATAL: net-ready gate read failed: %v\n", rerr)
			return 1
		}
		_ = syscall.Close(fd)
	}

	tr := trace.NewChild(!flags.Quiet)

	if flags.NetConfigPath != "" {
		if err := applyEgressConfig(flags.NetConfigPath, flags.SessionDir, "/etc", tr); err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: egress config apply failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
			return 1
		}
	}

	// Remount procfs over /proc inside the child mount and PID namespace so that
	// /proc/self accurately points to container PID 1 rather than host PID 1
	// (systemd). This prevents crashes in runtimes (such as Bun, WebKit, and
	// Node) that assert on /proc/self status.
	_ = tr.Step("procfs mount", func() error {
		return isolation.MountProc()
	})

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

	// Persistent state bind-mounts MUST execute AFTER OverlayFS at cwd
	// (so cwd overlay is in place) and BEFORE Landlock restriction
	// (so Landlock can see and allow the bind-mount target).
	// See safebox-v0.4-build.md T14.2 and FR15.
	for _, mapping := range flags.PersistentStateMounts {
		parts := strings.SplitN(mapping, ":", 2)
		if len(parts) != 2 {
			continue
		}
		hostPath, mountPath := parts[0], parts[1]
		if err := tr.Step("persistent state mount", func() error {
			return persistentstate.BindMount(hostPath, mountPath)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: persistent state mount DENIED: %v\n", ui.StyleDenied.Render("ERROR"), err)
			if hint := HintFor(&isolation.ErrPersistentStateMount{HostPath: hostPath, MountPath: mountPath, Err: err}, cmdArgs); hint != "" {
				fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
			}
			return 5 // Exit code 5 for NFR1 hard-fail
		}
	}

	if err := tr.Step("landlock restrict", func() error {
		return isolation.ApplyLandlock(flags.AllowPathsRO, flags.AllowPathsRW, flags.AllowFilesRW)
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

// applyEgressConfig bind-mounts synthetic /etc/hosts and /etc/resolv.conf over the
// real ones inside the child's already-private mount namespace. It never writes
// directly to /etc - every mutation goes through mountEtcFile and is contained
// by the namespace until process exit.
//
// etcRoot is the base directory containing the real /etc/hosts and
// /etc/resolv.conf; production callers pass "/etc", tests pass t.TempDir()
// so the host filesystem is never touched by unit tests.
//
// tmpDirBase is the directory in which the temporary content files used as
// bind-mount sources are created. Production callers pass sessionDir so the
// tmpfiles ride along with session cleanup. The tmpfile is unlinked immediately
// after a successful bind mount - the kernel retains its own inode reference,
// so this is safe and prevents the unbounded /tmp/safebox-* growth seen in the
// rejected v0.4 Phase 16 follow-up plan.
func applyEgressConfig(netConfigPath, tmpDirBase, etcRoot string, tr *trace.Tracer) error {
	data, err := os.ReadFile(netConfigPath)
	if err != nil {
		return fmt.Errorf("read net config %s: %w", netConfigPath, err)
	}
	var cfg netpolicy.NetConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse net config: %w", err)
	}
	if len(cfg.AllowedDomains) == 0 && cfg.GatewayIP == "" {
		return nil
	}

	return tr.Step("netpolicy apply", func() error {
		gateway := cfg.GatewayIP
		if gateway == "" {
			gateway = "10.0.2.2"
		}

		// Synthetic /etc/hosts: preserve the real file's content then append
		// the safebox egress marker. Per-domain pin entries are intentionally
		// skipped in v1 because --allow-net=* grants full internet access and
		// domain-level enforcement is deferred to v0.5 (SNI inspection).
		hostsPath := filepath.Join(etcRoot, "hosts")
		resolvPath := filepath.Join(etcRoot, "resolv.conf")

		existingHosts, err := os.ReadFile(hostsPath)
		if err != nil {
			existingHosts = []byte("127.0.0.1\tlocalhost\n::1\tlocalhost\n")
		}
		hostsStr := string(existingHosts)
		if !strings.HasSuffix(hostsStr, "\n") {
			hostsStr += "\n"
		}
		hostsStr += "# safebox egress\n" + gateway + "\tallow-net\n"

		if err := mountEtcFile(hostsPath, []byte(hostsStr), tmpDirBase); err != nil {
			return fmt.Errorf("mount %s: %w", hostsPath, err)
		}

		dnsServer := cfg.DNSIP
		if dnsServer == "" {
			dnsServer = gateway
		}
		resolvContent := fmt.Sprintf("nameserver %s\noptions edns0\n", dnsServer)
		if err := mountEtcFile(resolvPath, []byte(resolvContent), tmpDirBase); err != nil {
			return fmt.Errorf("mount %s: %w", resolvPath, err)
		}
		return nil
	})
}

// mountEtcFile bind-mounts content over target inside the caller's mount
// namespace. The source tmpfile is created in tmpDirBase (production:
// sessionDir; tests: t.TempDir()), written, then bind-mounted. On success
// the tmpfile is unlinked immediately - the kernel holds its own reference
// to the mounted inode, so unlink-after-bind is safe.
//
// Returns a wrapped error on any failure (no silent fallback per NFR1).
func mountEtcFile(target string, content []byte, tmpDirBase string) error {
	tmpFile, err := os.CreateTemp(tmpDirBase, "safebox-etc-*.conf")
	if err != nil {
		return fmt.Errorf("create tmp file for %s: %w", target, err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write tmp file %s: %w", tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close tmp file %s: %w", tmpPath, err)
	}

	if err := syscall.Mount(tmpPath, target, "", syscall.MS_BIND, ""); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("bind-mount %s -> %s: %w", tmpPath, target, err)
	}
	// Safe to remove: the kernel retains its own reference to the inode now
	// that the bind mount is in place. This prevents the unbounded /tmp
	// growth that was a documented defect in the rejected v0.4 follow-up.
	_ = os.Remove(tmpPath)
	return nil
}
