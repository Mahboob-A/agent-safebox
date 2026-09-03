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
	// Pre-emptively make OverlayFS work directory accessible
	workDir := filepath.Join(baseDir, "work")
	_ = os.Chmod(workDir, 0700)
	_ = os.Chmod(filepath.Join(workDir, "work"), 0700)

	err := os.RemoveAll(baseDir)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	_ = makeTreeRemovable(baseDir)
	return os.RemoveAll(baseDir)
}

func makeTreeRemovable(dir string) error {
	_ = os.Chmod(dir, 0700)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sub := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			_ = makeTreeRemovable(sub)
		} else {
			_ = os.Chmod(sub, 0600)
		}
	}
	return nil
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

// MountSessionOverlay mounts an unprivileged OverlayFS on mergedDir using
// lowerDir (read-only), upperDir (read-write), and workDir.
func MountSessionOverlay(lowerDir, upperDir, workDir, mergedDir string) error {
	if lowerDir == "" || upperDir == "" || workDir == "" || mergedDir == "" {
		return errors.New("safebox: all overlay directory paths (lower, upper, work, merged) must be specified")
	}
	if err := os.MkdirAll(mergedDir, 0700); err != nil {
		return fmt.Errorf("safebox: failed to create merged overlay mount point: %w", err)
	}
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	if err := syscall.Mount("overlay", mergedDir, "overlay", 0, opts); err != nil {
		return fmt.Errorf("safebox: failed to mount overlayfs: %w", err)
	}
	return nil
}

// UnmountOverlay unmounts an active OverlayFS filesystem from TargetDir.
func UnmountOverlay(targetDir string) error {
	if targetDir == "" {
		return errors.New("safebox: target directory cannot be empty")
	}
	if err := syscall.Unmount(targetDir, 0); err != nil {
		return fmt.Errorf("safebox: failed to unmount overlayfs: %w", err)
	}
	return nil
}
