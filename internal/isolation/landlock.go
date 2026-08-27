package isolation

import (
	"fmt"
	"os"

	"github.com/landlock-lsm/go-landlock/landlock"
)

// ApplyLandlock constructs and applies the deny-by-default Landlock ruleset.
// It allows read-write access to the current working directory (cwd) and
// minimal read-only access to /usr, /lib, /lib64, and /etc.
//
// In strict compliance with FR8 and NFR1, silent fallback is forbidden.
// Any Landlock error is treated as fatal and returned immediately.
func ApplyLandlock() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("safebox: cannot resolve working directory: %w", err)
	}

	err = landlock.V3.RestrictPaths(
		landlock.RODirs("/usr", "/lib", "/lib64", "/etc"),
		landlock.RWDirs(cwd),
	)
	if err != nil {
		return fmt.Errorf("safebox: Landlock unavailable, refusing to run unsandboxed: %w", err)
	}
	return nil
}
