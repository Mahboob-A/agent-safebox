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
// It allows read-write access to the current working directory (cwd) and any explicitly supplied allowPathsRW,
// read-only directory access to /usr, /usr/local, /lib, /lib64, and /etc/ld.so.conf.d,
// read-only file access to specific safe configuration files in /etc (/etc/ld.so.cache,
// /etc/ld.so.conf, /etc/nsswitch.conf, /etc/passwd, /etc/group, /etc/localtime),
// and any explicitly supplied allowPathsRO (FR10).
//
// In strict compliance with FR8 and NFR1, silent fallback is forbidden.
// Any Landlock error is treated as fatal and returned immediately.
func ApplyLandlock(allowPathsRO []string, allowPathsRW []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("safebox: cannot resolve working directory: %w", err)
	}

	defaultRODirs := []string{"/usr", "/usr/local", "/lib", "/lib64", "/etc/ld.so.conf.d"}
	allRODirs := append(defaultRODirs, allowPathsRO...)
	roDirs := filterExisting(allRODirs)

	roFiles := filterExisting([]string{
		"/etc/ld.so.cache",
		"/etc/ld.so.conf",
		"/etc/nsswitch.conf",
		"/etc/passwd",
		"/etc/group",
		"/etc/localtime",
	})

	filteredRW := filterExisting(allowPathsRW)
	for _, p := range allowPathsRW {
		found := false
		for _, f := range filteredRW {
			if p == f {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "[safebox] warning: allow-path-rw directory %q does not exist, skipping landlock grant\n", p)
		}
	}

	rwPaths := append([]string{cwd}, filteredRW...)

	var rules []landlock.Rule
	if len(roDirs) > 0 {
		rules = append(rules, landlock.RODirs(roDirs...))
	}
	if len(roFiles) > 0 {
		rules = append(rules, landlock.ROFiles(roFiles...))
	}
	if len(rwPaths) > 0 {
		rules = append(rules, landlock.RWDirs(rwPaths...))
	}

	err = landlock.V3.RestrictPaths(rules...)
	if err != nil {
		return fmt.Errorf("safebox: Landlock unavailable, refusing to run unsandboxed: %w", err)
	}
	return nil
}
