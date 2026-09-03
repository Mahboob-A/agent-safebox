package profiles

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed builtin/*.toml
var builtinFS embed.FS

var (
	builtinsOnce   sync.Once
	builtinsResult []*Profile
	builtinsErr    error
)

// Builtins returns parsed built-in profiles cached in memory.
func Builtins() ([]*Profile, error) {
	builtinsOnce.Do(func() {
		entries, err := builtinFS.ReadDir("builtin")
		if err != nil {
			builtinsErr = err
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			data, err := builtinFS.ReadFile("builtin/" + e.Name())
			if err != nil {
				builtinsErr = err
				return
			}
			prof, err := parseProfile(data)
			if err != nil {
				builtinsErr = fmt.Errorf("%s: %w", e.Name(), err)
				return
			}
			builtinsResult = append(builtinsResult, prof)
		}
	})
	return builtinsResult, builtinsErr
}

// Lookup checks built-in profiles first, then user profiles.
// If a user profile has the same Binary.Name as a built-in, the user profile wins.
func Lookup(argv0 string) (*Profile, error) {
	target := filepath.Base(argv0)
	userProfiles, _ := LoadUserProfiles()
	userMap := make(map[string]*Profile)
	for _, up := range userProfiles {
		userMap[up.Binary.Name] = up
	}

	builtins, err := Builtins()
	if err != nil {
		return nil, err
	}

	// Walk built-ins. If a built-in matches and a user override exists, user profile wins.
	for _, bp := range builtins {
		if strings.Contains(target, bp.Binary.Name) {
			if override, exists := userMap[bp.Binary.Name]; exists {
				return override, nil
			}
			return bp, nil
		}
	}

	// Check any user-only profiles not overriding a built-in
	for _, up := range userProfiles {
		if strings.Contains(target, up.Binary.Name) {
			return up, nil
		}
	}

	return nil, nil
}

// RawProfile returns the raw TOML contents of the specified profile.
// Checks user profiles directory first, then embedded built-ins.
func RawProfile(name string) (string, error) {
	// Check user config directory
	if dir, err := ConfigDir(); err == nil {
		userFile := filepath.Join(dir, name+".toml")
		if data, err := os.ReadFile(userFile); err == nil {
			return string(data), nil
		}
	}

	// Check embedded built-ins
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded profiles: %w", err)
	}

	for _, e := range entries {
		if strings.TrimSuffix(e.Name(), ".toml") == name {
			data, err := builtinFS.ReadFile("builtin/" + e.Name())
			if err != nil {
				return "", fmt.Errorf("failed to read embedded profile %s: %w", e.Name(), err)
			}
			return string(data), nil
		}
	}

	return "", fmt.Errorf("unknown profile '%s'", name)
}
