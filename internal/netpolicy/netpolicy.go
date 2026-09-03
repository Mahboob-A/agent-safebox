package netpolicy

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// NetConfig holds the configuration for userspace network egress passed to child.
type NetConfig struct {
	TAPDevice      string              `json:"tap_device,omitempty"`
	GatewayIP      string              `json:"gateway_ip,omitempty"`
	GatewayPort    int                 `json:"gateway_port,omitempty"`
	PinnedIPs      map[string][]string `json:"pinned_ips,omitempty"`
	AllowedDomains []string            `json:"allowed_domains"`
	BackendName    string              `json:"backend_name"` // "slirp4netns" | "pasta" | "builtin"
	DNSIP          string              `json:"dns_ip,omitempty"`
}

// DiscoverBackend finds the first available userspace NAT backend.
// Order: pasta -> slirp4netns -> builtin (requires /dev/net/tun access and TUNSETIFF capability).
// Pasta is preferred as of v1 because it reflects the host's network configuration
// directly into the container instead of performing NAT in userspace, matching
// the upstream rootless-container default since Podman 5.8 (March 2026).
func DiscoverBackend() (string, error) {
	if _, err := exec.LookPath("pasta"); err == nil {
		return "pasta", nil
	}
	if _, err := exec.LookPath("slirp4netns"); err == nil {
		return "slirp4netns", nil
	}
	if probeBuiltinCapability() {
		return "builtin", nil
	}
	return "", fmt.Errorf("no network backend available: pasta not found, slirp4netns not found, /dev/net/tun not accessible (requires CAP_NET_ADMIN or membership in 'netdev' group)")
}

func probeBuiltinCapability() bool {
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()

	ifaceName := fmt.Sprintf("sbxprobe%d", os.Getpid())
	ifr, err := unix.NewIfreq(ifaceName)
	if err != nil {
		return false
	}
	ifr.SetUint16(unix.IFF_TAP | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(int(f.Fd()), unix.TUNSETIFF, ifr); err != nil {
		return false
	}
	return true
}

// SetupEgress initializes the network policy and attaches the selected NAT backend
// to the already-existing child network namespace identified by childPID.
//
// The child PID MUST be passed in; the backend will not function correctly otherwise.
// This enforces the create-then-attach-then-resume pattern (parent spawns child first,
// then attaches the userspace NAT backend to the child's namespace).
//
// The closure returned as the second value tears the backend down on parent exit.
//
// allowNet is the v1 binary toggle (--allow-net / --allow-net=*): when true and
// allowDomains is empty, the backend is spawned in allow-all mode (full internet
// egress via userspace NAT). When allowNet is false and allowDomains is empty,
// this is default-deny and a no-op cleanup is returned.
//
// Domain-restricted egress (allowDomains non-empty, allowNet false) is preserved
// as a dormant seam for v0.5+ SNI inspection; the backend is spawned with the
// pinned IP set so the v0.5 enforcement code has the data it needs.
func SetupEgress(allowDomains []string, allowNet bool, childPID int) (*NetConfig, func() error, error) {
	if childPID <= 0 {
		return nil, nil, fmt.Errorf("safebox netpolicy: childPID must be a positive integer (got %d)", childPID)
	}
	if !allowNet && len(allowDomains) == 0 {
		return nil, func() error { return nil }, nil
	}

	backend, err := DiscoverBackend()
	if err != nil {
		return nil, nil, err
	}

	var pinSet *PinnedIPSet
	if len(allowDomains) > 0 {
		pinSet, err = LookupAndPinDomains(allowDomains)
		if err != nil {
			return nil, nil, fmt.Errorf("safebox netpolicy: dns lookup failed: %w", err)
		}
	}

	cfg := &NetConfig{
		TAPDevice:      fmt.Sprintf("sbx%d", os.Getpid()),
		GatewayIP:      "10.0.2.2",
		GatewayPort:    53,
		AllowedDomains: allowDomains,
		BackendName:    backend,
		DNSIP:          "10.0.2.3",
	}
	if pinSet != nil {
		cfg.PinnedIPs = pinSet.ToMap()
	}

	var teardownBackend func() error
	switch backend {
	case "slirp4netns":
		teardown, sErr := spawnSlirp4netns(cfg, childPID)
		if sErr != nil {
			if pinSet != nil {
				pinSet.Close()
			}
			return nil, nil, fmt.Errorf("safebox netpolicy: slirp4netns spawn failed: %w", sErr)
		}
		teardownBackend = teardown
	case "pasta":
		teardown, pErr := spawnPasta(cfg, childPID)
		if pErr != nil {
			if pinSet != nil {
				pinSet.Close()
			}
			return nil, nil, fmt.Errorf("safebox netpolicy: pasta spawn failed: %w", pErr)
		}
		teardownBackend = teardown
	case "builtin":
		b, bErr := NewBuiltinBackend(cfg, pinSet)
		if bErr != nil {
			if pinSet != nil {
				pinSet.Close()
			}
			return nil, nil, fmt.Errorf("safebox netpolicy: builtin backend setup failed: %w", bErr)
		}
		go b.Run()
		teardownBackend = b.Close
	default:
		if pinSet != nil {
			pinSet.Close()
		}
		return nil, nil, fmt.Errorf("safebox netpolicy: unknown backend %q", backend)
	}

	cleanup := func() error {
		if pinSet != nil {
			pinSet.Close()
		}
		if teardownBackend != nil {
			return teardownBackend()
		}
		return nil
	}

	return cfg, cleanup, nil
}

// readyFDFiles returns the next slot index in cmd.ExtraFiles for a newly created
// pipe whose write end will be passed to the backend via --ready-fd.
// slirp4netns writes one byte to this fd when its TAP device is ready.
func makeReadyFD() (writeEnd *os.File, readEnd *os.File, err error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create ready-fd pipe: %w", err)
	}
	return w, r, nil
}

// awaitReadyFD reads one byte from the ready-fd read end within timeout,
// then closes both ends. Returns nil on success, an error on timeout or read failure.
// The byte itself is discarded; its purpose is purely to signal readiness.
func awaitReadyFD(readEnd *os.File, writeEnd *os.File, backendName string, pid int) error {
	defer readEnd.Close()
	defer writeEnd.Close()

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := readEnd.Read(buf)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s pid %d ready-fd read failed: %w", backendName, pid, err)
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("%s pid %d did not signal ready within 10s", backendName, pid)
	}
}

// spawnSlirp4netns attaches slirp4netns to the given child PID.
// Correct invocation: slirp4netns [OPTION]... PID|PATH [TAPNAME]
// We pass -c to configure the interface, the child PID as positional arg,
// and "tap0" as the TAP name. We use --ready-fd to confirm the backend
// has configured the network before returning.
func spawnSlirp4netns(cfg *NetConfig, childPID int) (func() error, error) {
	readyWrite, readyRead, err := makeReadyFD()
	if err != nil {
		return nil, err
	}
	// cmd.ExtraFiles indices map to fd 3, 4, 5, ... in the child process.
	// slirp4netns treats --ready-fd as an integer file descriptor number.
	readyFDNum := 3
	cmd := exec.Command("slirp4netns",
		"-c",
		"--mtu", "1500",
		"--disable-host-loopback",
		fmt.Sprintf("--ready-fd=%d", readyFDNum),
		fmt.Sprintf("%d", childPID),
		"tap0",
	)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	cmd.ExtraFiles = []*os.File{readyWrite}
	if err := cmd.Start(); err != nil {
		readyWrite.Close()
		readyRead.Close()
		return nil, fmt.Errorf("slirp4netns start: %w", err)
	}
	// Parent no longer needs the write end directly; readyRead carries the signal.
	readyWrite.Close()

	if err := awaitReadyFD(readyRead, readyWrite, "slirp4netns", cmd.Process.Pid); err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
		if stderrBuf.Len() > 0 {
			return nil, fmt.Errorf("%w (slirp4netns stderr: %s)", err, strings.TrimSpace(stderrBuf.String()))
		}
		return nil, err
	}

	teardown := func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return nil
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			return fmt.Errorf("slirp4netns pid %d did not exit on SIGTERM; SIGKILL sent", cmd.Process.Pid)
		}
	}
	return teardown, nil
}

// spawnPasta attaches pasta to the given child PID.
// Correct invocation: pasta [OPTION]... PID|PATH
// pasta prints to stderr once its TAP interface is up; we poll the child
// process state with Signal(0) (no-op signal used as a liveness probe) for
// up to 3 s and treat early exit as a setup failure.
func spawnPasta(cfg *NetConfig, childPID int) (func() error, error) {
	cmd := exec.Command("pasta",
		"--mtu", "1500",
		"--no-map-gw",
		fmt.Sprintf("%d", childPID),
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("pasta start: %w", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
			// Process is no longer running; check why.
			waitErr := cmd.Wait()
			if waitErr != nil {
				return nil, fmt.Errorf("pasta pid %d exited prematurely: %w", cmd.Process.Pid, waitErr)
			}
			return nil, fmt.Errorf("pasta pid %d exited prematurely", cmd.Process.Pid)
		}
		time.Sleep(200 * time.Millisecond)
	}

	teardown := func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return nil
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			return fmt.Errorf("pasta pid %d did not exit on SIGTERM; SIGKILL sent", cmd.Process.Pid)
		}
	}
	return teardown, nil
}
