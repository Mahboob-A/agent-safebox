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
// It allows read-write access to the current working directory (cwd), any explicitly supplied allowPathsRW,
// single file read-write access for allowFilesRW (FR14 / T13.3),
// read-only directory access to /usr, /usr/local, /lib, /lib64, and /etc/ld.so.conf.d,
// read-only file access to specific safe configuration files in /etc (/etc/ld.so.cache,
// /etc/ld.so.conf, /etc/nsswitch.conf, /etc/passwd, /etc/group, /etc/localtime,
// /etc/hosts, /etc/resolv.conf),
// and any explicitly supplied allowPathsRO (FR10).
//
// /etc/hosts and /etc/resolv.conf are read-only to the wrapped agent so it cannot rewrite
// its own shadowed (bind-mount) DNS config mid-session; the synthetic content remains
// pinned for the lifetime of the sandbox. These files MUST already exist on disk (either
// the host originals or bind-mount shadowed copies from RunChild) when this function
// is called.
//
// In strict compliance with FR8 and NFR1, silent fallback is forbidden.
// Any Landlock error is treated as fatal and returned immediately.
func ApplyLandlock(allowPathsRO []string, allowPathsRW []string, allowFilesRW []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("safebox: cannot resolve working directory: %w", err)
	}

	defaultRODirs := []string{"/usr", "/usr/local", "/lib", "/lib64", "/etc/ld.so.conf.d", "/etc/ssl", "/etc/pki"}
	allRODirs := append(defaultRODirs, allowPathsRO...)
	roDirs := filterExisting(allRODirs)

	roFiles := filterExisting([]string{
		"/etc/ld.so.cache",
		"/etc/ld.so.conf",
		"/etc/nsswitch.conf",
		"/etc/passwd",
		"/etc/group",
		"/etc/localtime",
		"/etc/hosts",
		"/etc/resolv.conf",
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

	filteredFilesRW := filterExisting(allowFilesRW)
	for _, p := range allowFilesRW {
		found := false
		for _, f := range filteredFilesRW {
			if p == f {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "[safebox] warning: allow-file-rw file %q does not exist, skipping landlock grant\n", p)
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
	if len(filteredFilesRW) > 0 {
		rules = append(rules, landlock.RWFiles(filteredFilesRW...))
	}

	err = landlock.V3.RestrictPaths(rules...)
	if err != nil {
		return fmt.Errorf("safebox: Landlock unavailable, refusing to run unsandboxed: %w", err)
	}
	return nil
}

// ProbeReport represents the resolved filesystem and namespace isolation policies.
type ProbeReport struct {
	WorkingDir       string
	EffectiveRW      []string
	EffectiveRO      []string
	ROFiles          []string
	EffectiveRWFiles []string
}

// ProbeLandlock constructs the resolved ruleset without applying it to the kernel.
func ProbeLandlock(allowPathsRO, allowPathsRW, allowFilesRW []string) (ProbeReport, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ProbeReport{}, fmt.Errorf("safebox: cannot resolve working directory: %w", err)
	}

	defaultRODirs := []string{"/usr", "/usr/local", "/lib", "/lib64", "/etc/ld.so.conf.d", "/etc/ssl", "/etc/pki"}
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
	rwPaths := append([]string{cwd}, filteredRW...)

	filteredFilesRW := filterExisting(allowFilesRW)

	return ProbeReport{
		WorkingDir:       cwd,
		EffectiveRW:      rwPaths,
		EffectiveRO:      roDirs,
		ROFiles:          roFiles,
		EffectiveRWFiles: filteredFilesRW,
	}, nil
}
