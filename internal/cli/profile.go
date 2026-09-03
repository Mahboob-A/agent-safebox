package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"safebox/internal/profiles"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunProfile executes the 'safebox profile' inspection subcommand.
func RunProfile(args []string, _ *trace.Tracer) int {
	if len(args) == 0 || args[0] == "list" {
		return RunProfileList()
	}

	if args[0] == "show" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			fmt.Fprintf(os.Stderr, "%s safebox profile show: missing profile name\n", ui.StyleDenied.Render("ERROR"))
			return 1
		}
		return RunProfileShow(args[1])
	}

	fmt.Fprintf(os.Stderr, "%s safebox profile: unknown subcommand %q (expected 'list' or 'show <name>')\n", ui.StyleDenied.Render("ERROR"), args[0])
	return 1
}

// RunProfileList prints registered built-in and user profiles in line-oriented format.
func RunProfileList() int {
	builtins, err := profiles.Builtins()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s safebox: failed to load built-in profiles: %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 1
	}

	fmt.Println("BUILT-IN PROFILES:")
	sort.Slice(builtins, func(i, j int) bool {
		return builtins[i].Binary.Name < builtins[j].Binary.Name
	})
	for _, prof := range builtins {
		fmt.Printf("  %s\n", formatProfileSummary(prof))
	}

	fmt.Println("\nUSER PROFILES:")
	userProfiles, err := profiles.LoadUserProfiles()
	if err != nil || len(userProfiles) == 0 {
		fmt.Println("  (none)")
	} else {
		sort.Slice(userProfiles, func(i, j int) bool {
			return userProfiles[i].Binary.Name < userProfiles[j].Binary.Name
		})
		for _, prof := range userProfiles {
			fmt.Printf("  %s\n", formatProfileSummary(prof))
		}
	}

	return 0
}

// RunProfileShow prints the raw TOML content of the specified profile.
func RunProfileShow(name string) int {
	raw, err := profiles.RawProfile(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s safebox: unknown profile %q\n", ui.StyleDenied.Render("ERROR"), name)
		return 1
	}
	fmt.Print(raw)
	if !strings.HasSuffix(raw, "\n") {
		fmt.Println()
	}
	return 0
}

func formatProfileSummary(prof *profiles.Profile) string {
	var parts []string
	if len(prof.Paths.AllowRO) > 0 {
		parts = append(parts, "ro="+strings.Join(prof.Paths.AllowRO, ","))
	}
	if len(prof.Paths.AllowRW) > 0 {
		parts = append(parts, "rw="+strings.Join(prof.Paths.AllowRW, ","))
	}
	if len(prof.Paths.AllowRWFiles) > 0 {
		parts = append(parts, "rwf="+strings.Join(prof.Paths.AllowRWFiles, ","))
	}
	if len(prof.Network.AllowDomains) > 0 {
		parts = append(parts, "net="+strings.Join(prof.Network.AllowDomains, ","))
	}

	details := strings.Join(parts, "  ")
	if details == "" {
		return prof.Binary.Name
	}
	return fmt.Sprintf("%-10s %s", prof.Binary.Name, details)
}
