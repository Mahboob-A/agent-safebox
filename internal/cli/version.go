package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// Version metadata set at compile time or defaults to current release.
var (
	Version   = "v0.5.2"
	GitCommit = ""
	BuildDate = ""
)

// FormatVersion returns a structured version string containing version number,
// git commit hash (if available), and target OS/architecture.
func FormatVersion() string {
	commit := GitCommit
	if commit == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					if len(setting.Value) > 7 {
						commit = setting.Value[:7]
					} else {
						commit = setting.Value
					}
					break
				}
			}
		}
	}

	if commit != "" {
		return fmt.Sprintf("safebox %s (commit: %s, %s/%s)", Version, commit, runtime.GOOS, runtime.GOARCH)
	}
	return fmt.Sprintf("safebox %s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH)
}

// RunVersion prints the formatted version string to out and returns exit code 0.
func RunVersion(out io.Writer) int {
	fmt.Fprintln(out, FormatVersion())
	return 0
}
