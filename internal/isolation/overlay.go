package isolation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// OverlayConfig holds the directories required to mount an unprivileged OverlayFS.
type OverlayConfig struct {
	LowerDir  string
	UpperDir  string
	WorkDir   string
	TargetDir string
}

// EnsureOverlayDirs initializes upper and work directories under the specified session base directory.
func EnsureOverlayDirs(baseDir string) (upperDir, workDir string, err error) {
	if baseDir == "" {
		return "", "", errors.New("safebox: overlay base directory cannot be empty")
	}

	upperDir = filepath.Join(baseDir, "upper")
	workDir = filepath.Join(baseDir, "work")

	if err := os.MkdirAll(upperDir, 0700); err != nil {
		return "", "", fmt.Errorf("safebox: failed to create overlay upper directory: %w", err)
	}
	if err := os.MkdirAll(workDir, 0700); err != nil {
		return "", "", fmt.Errorf("safebox: failed to create overlay work directory: %w", err)
	}

	return upperDir, workDir, nil
}

// CleanupOverlayDirs removes the session base directory and all its contents.
func CleanupOverlayDirs(baseDir string) error {
	if baseDir == "" {
		return nil
	}
	return os.RemoveAll(baseDir)
}

// MountOverlay mounts an OverlayFS filesystem on TargetDir using LowerDir (RO), UpperDir (RW), and WorkDir.
// It must be called within an unprivileged mount namespace (CLONE_NEWNS) and user namespace (CLONE_NEWUSER).
func MountOverlay(cfg OverlayConfig) error {
	if cfg.LowerDir == "" || cfg.UpperDir == "" || cfg.WorkDir == "" || cfg.TargetDir == "" {
		return errors.New("safebox: all overlay directory paths (lower, upper, work, target) must be specified")
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", cfg.LowerDir, cfg.UpperDir, cfg.WorkDir)
	if err := syscall.Mount("overlay", cfg.TargetDir, "overlay", 0, opts); err != nil {
		return fmt.Errorf("safebox: failed to mount overlayfs: %w", err)
	}
	return nil
}
