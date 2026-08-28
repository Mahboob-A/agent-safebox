package isolation

import (
	"fmt"
	"os"

	"github.com/landlock-lsm/go-landlock/landlock"
)

func filterExisting(paths []string) []string {
	var existing []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	return existing
}

// ApplyLandlock constructs and applies the deny-by-default Landlock ruleset.
// It allows read-write access to the current working directory (cwd),
// read-only directory access to /usr, /usr/local, /lib, /lib64, and /etc/ld.so.conf.d,
// read-only file access to specific safe configuration files in /etc (/etc/ld.so.cache,
// /etc/ld.so.conf, /etc/nsswitch.conf, /etc/passwd, /etc/group, /etc/localtime),
// and any explicitly supplied allowPaths (FR10).
//
// In strict compliance with FR8 and NFR1, silent fallback is forbidden.
// Any Landlock error is treated as fatal and returned immediately.
func ApplyLandlock(allowPaths ...string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("safebox: cannot resolve working directory: %w", err)
	}

	defaultRODirs := []string{"/usr", "/usr/local", "/lib", "/lib64", "/etc/ld.so.conf.d"}
	allRODirs := append(defaultRODirs, allowPaths...)
	roDirs := filterExisting(allRODirs)

	roFiles := filterExisting([]string{
		"/etc/ld.so.cache",
		"/etc/ld.so.conf",
		"/etc/nsswitch.conf",
		"/etc/passwd",
		"/etc/group",
		"/etc/localtime",
	})

	var rules []landlock.Rule
	if len(roDirs) > 0 {
		rules = append(rules, landlock.RODirs(roDirs...))
	}
	if len(roFiles) > 0 {
		rules = append(rules, landlock.ROFiles(roFiles...))
	}
	rules = append(rules, landlock.RWDirs(cwd))

	err = landlock.V3.RestrictPaths(rules...)
	if err != nil {
		return fmt.Errorf("safebox: Landlock unavailable, refusing to run unsandboxed: %w", err)
	}
	return nil
}
