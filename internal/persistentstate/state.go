package persistentstate

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DefaultRoot returns the per-user persistent state root.
//
// Returns $XDG_STATE_HOME/safebox/agents if XDG_STATE_HOME is set;
// otherwise returns <os.UserHomeDir()>/.local/share/safebox/agents.
//
// Uniform across all UIDs: root users resolve to
// /root/.local/share/safebox/agents via os.UserHomeDir() returning
// /root inside the user namespace. No os.Geteuid() == 0 special case.
//
// See safebox-v0.4-build.md T14.1 (amended per review R14-A)
// and master-plan.md Section 4 Phase 14 for the policy decision.
func DefaultRoot() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "safebox", "agents"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "safebox", "agents"), nil
}

// EnsurePath creates hostPath with the requested mode if missing. If
// hostPath already exists, ensures its permission bits match the
// requested mode (chmod'ing if necessary).
func EnsurePath(hostPath string, mode os.FileMode) (string, error) {
	if err := os.MkdirAll(hostPath, mode); err != nil {
		return "", fmt.Errorf("ensure path %s: %w", hostPath, err)
	}
	if fi, err := os.Stat(hostPath); err == nil {
		if fi.Mode().Perm() != mode.Perm() {
			if err := os.Chmod(hostPath, mode); err != nil {
				return "", fmt.Errorf("chmod %s: %w", hostPath, err)
			}
		}
	}
	return hostPath, nil
}

// Ensure creates <root>/<tool> with 0700 mode if missing.
func Ensure(tool string) (string, error) {
	root, err := DefaultRoot()
	if err != nil {
		return "", err
	}
	return EnsurePath(filepath.Join(root, tool), 0700)
}

// BindMount mounts src at dst inside the private mount namespace.
// Rejects symlink sources to prevent link substitution attacks.
func BindMount(src, dst string) error {
	if fi, err := os.Lstat(src); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("persistent state host %s is a symlink; refusing bind-mount", src)
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, 0700); err != nil {
			return fmt.Errorf("mkdir %s: %w", dst, err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return fmt.Errorf("mkdir parent %s: %w", dst, err)
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("create file %s: %w", dst, err)
		}
		_ = f.Close()
	}
	if err := syscall.Mount(src, dst, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("mount %s -> %s: %w", src, dst, err)
	}
	return nil
}
