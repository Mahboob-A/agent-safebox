package isolation

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ErrLandlockDenied represents a Landlock filesystem permission violation.
// Hint takes binPath because ErrLandlockDenied only knows the denied path,
// requiring the caller's context to determine if the denied path belongs to the binary.
type ErrLandlockDenied struct {
	Path string
	Op   string
}

func (e *ErrLandlockDenied) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("landlock denied access to %s (%s)", e.Path, e.Op)
	}
	return fmt.Sprintf("landlock denied access to %s", e.Path)
}

func (e *ErrLandlockDenied) Hint(binPath string) string {
	binDir := filepath.Dir(binPath)
	if binDir != "" && binDir != "." && (e.Path == binDir || strings.HasPrefix(e.Path, binDir+"/")) {
		return fmt.Sprintf("rerun with --allow-path=%s", binDir)
	}
	if e.Path != "" {
		return fmt.Sprintf("rerun with --allow-path-rw=%s", e.Path)
	}
	return ""
}

// ErrExecNotFound represents failure to locate a binary on the host or in PATH.
type ErrExecNotFound struct {
	Bin string
}

func (e *ErrExecNotFound) Error() string {
	return fmt.Sprintf("executable not found: %s", e.Bin)
}

func (e *ErrExecNotFound) Hint() string {
	if strings.Contains(e.Bin, "/") {
		return fmt.Sprintf("rerun with --allow-path=%s", filepath.Dir(e.Bin))
	}
	return ""
}

// ErrExecDenied represents Landlock blocking execution or access to a binary.
type ErrExecDenied struct {
	Bin  string
	Path string
}

func (e *ErrExecDenied) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("executable %s blocked by landlock (denied path: %s)", e.Bin, e.Path)
	}
	return fmt.Sprintf("executable %s blocked by landlock", e.Bin)
}

func (e *ErrExecDenied) Hint() string {
	binDir := filepath.Dir(e.Bin)
	if binDir != "" && binDir != "." {
		if e.Path != "" && e.Path != binDir {
			return fmt.Sprintf("rerun with --allow-path=%s --allow-path-rw=%s", binDir, e.Path)
		}
		return fmt.Sprintf("rerun with --allow-path=%s", binDir)
	}
	if e.Path != "" {
		return fmt.Sprintf("rerun with --allow-path-rw=%s", e.Path)
	}
	return ""
}
