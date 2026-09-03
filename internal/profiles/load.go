package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigDir returns the path to user profiles directory under XDG config home.
func ConfigDir() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "safebox", "profiles"), nil
}

// LoadUserProfiles reads and parses all .toml profiles in the user config directory.
// Any invalid individual profile logs a warning to stderr and is skipped.
func LoadUserProfiles() ([]*Profile, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var userProfiles []*Profile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[safebox:profiles] warning: %s: %v\n", filePath, err)
			continue
		}

		prof, err := parseProfile(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[safebox:profiles] warning: %s: %v\n", filePath, err)
			continue
		}

		userProfiles = append(userProfiles, prof)
	}

	return userProfiles, nil
}
