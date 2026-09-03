package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"safebox/internal/isolation"
	"safebox/internal/revert"
	"safebox/internal/ui"
)

// HintFor formats structured, actionable hints for runtime denials.
func HintFor(err error, cmdArgs []string) string {
	if len(cmdArgs) == 0 {
		return ""
	}

	var deniedErr *isolation.ErrLandlockDenied
	if errors.As(err, &deniedErr) {
		return deniedErr.Hint(cmdArgs[0])
	}

	var execNotFound *isolation.ErrExecNotFound
	if errors.As(err, &execNotFound) {
		return execNotFound.Hint()
	}

	var execDenied *isolation.ErrExecDenied
	if errors.As(err, &execDenied) {
		return execDenied.Hint()
	}

	var persistMountErr *isolation.ErrPersistentStateMount
	if errors.As(err, &persistMountErr) {
		return persistMountErr.Hint()
	}

	if CheckUbuntuAppArmorUserNS() && (strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "namespace isolation failed")) {
		return "Ubuntu 24.04+ has restricted unprivileged user namespaces. Run: sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0"
	}

	if (strings.Contains(err.Error(), "namespace isolation failed") || strings.Contains(err.Error(), "operation not permitted")) && CheckInsideContainer() {
		return "running inside Docker/container requires user namespace support. Start container with: docker run --security-opt seccomp=unconfined (or --cap-add=SYS_ADMIN)"
	}

	bin := cmdArgs[0]
	binDir := filepath.Dir(bin)
	if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "operation not permitted") {
		if binDir != "" && binDir != "." {
			return fmt.Sprintf("rerun with --allow-path=%s", binDir)
		}
	}
	return ""
}

// CheckUbuntuAppArmorUserNS checks if Ubuntu 24.04+ has restricted unprivileged user namespaces.
func CheckUbuntuAppArmorUserNS() bool {
	data, err := os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns")
	return err == nil && strings.TrimSpace(string(data)) == "1"
}

// CheckInsideContainer checks if the current process is running inside a Docker or OCI container.
func CheckInsideContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") || strings.Contains(content, "containerd") || strings.Contains(content, "kubepods") {
			return true
		}
	}
	return false
}

// HintForSubcommand returns actionable remediation hints for subcommand-level failures.
func HintForSubcommand(subcommand string, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, revert.ErrNoSessionFound) {
		return "run 'safebox run' first to create an overlay session"
	}
	if errors.Is(err, revert.ErrNotGitRepo) {
		return "run inside a git repository or use 'safebox run' to create an overlay session"
	}
	return ""
}

// PrintSubcommandError formats and prints subcommand errors conforming to FR7 tokens.
func PrintSubcommandError(subcommand string, err error) {
	fmt.Fprintf(os.Stderr, "%s safebox %s: %v\n", ui.StyleDenied.Render("ERROR"), subcommand, err)
	if hint := HintForSubcommand(subcommand, err); hint != "" {
		fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
	}
}
