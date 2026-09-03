package profiles

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// expandEnv expands $HOME, $XDG_STATE_HOME, and $XDG_CONFIG_HOME in path strings.
func expandEnv(s string) string {
	home, _ := os.UserHomeDir()
	if envHome := os.Getenv("HOME"); envHome != "" {
		home = envHome
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" && home != "" {
		stateHome = filepath.Join(home, ".local", "share")
	}

	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" && home != "" {
		configHome = filepath.Join(home, ".config")
	}

	mapping := func(key string) string {
		switch key {
		case "HOME":
			return home
		case "XDG_STATE_HOME":
			return stateHome
		case "XDG_CONFIG_HOME":
			return configHome
		default:
			return os.Getenv(key)
		}
	}

	return os.Expand(s, mapping)
}

// parseString unquotes a string formatted with double or single quotes.
func parseString(val string) (string, error) {
	val = strings.TrimSpace(val)
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			return val[1 : len(val)-1], nil
		}
	}
	return "", fmt.Errorf("invalid string literal: %s", val)
}

// parseBool parses "true" or "false" (case-insensitive).
func parseBool(val string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("invalid bool: %s", val)
}

// parseStringArray parses a comma-separated array of quoted strings inside brackets.
func parseStringArray(val string) ([]string, error) {
	val = strings.TrimSpace(val)
	if !strings.HasPrefix(val, "[") || !strings.HasSuffix(val, "]") {
		return nil, fmt.Errorf("invalid array syntax: %s", val)
	}

	inner := strings.TrimSpace(val[1 : len(val)-1])
	if inner == "" {
		return []string{}, nil
	}

	var items []string
	var token strings.Builder
	inQuote := false
	var quoteChar rune

	for _, r := range inner {
		if inQuote {
			if r == quoteChar {
				inQuote = false
			}
			token.WriteRune(r)
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
				token.WriteRune(r)
			} else if r == ',' {
				t := strings.TrimSpace(token.String())
				if t != "" {
					parsed, err := parseString(t)
					if err != nil {
						return nil, err
					}
					items = append(items, expandEnv(parsed))
				}
				token.Reset()
			} else if !strings.ContainsRune(" \t\r\n", r) {
				token.WriteRune(r)
			}
		}
	}

	if inQuote {
		return nil, fmt.Errorf("unterminated quote in array: %s", val)
	}

	t := strings.TrimSpace(token.String())
	if t != "" {
		parsed, err := parseString(t)
		if err != nil {
			return nil, err
		}
		items = append(items, expandEnv(parsed))
	}

	return items, nil
}

// parseProfile parses a TOML byte slice into a Profile struct.
func parseProfile(data []byte) (*Profile, error) {
	prof := &Profile{}
	scanner := bufio.NewScanner(bytes.NewReader(data))

	currentSection := ""
	hasBinarySection := false
	lineNum := 0

	var multilineKey string
	var multilineBuffer strings.Builder
	inMultilineArray := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Strip comments
		if idx := strings.Index(line, "#"); idx != -1 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle multi-line array accumulation
		if inMultilineArray {
			multilineBuffer.WriteString(" ")
			multilineBuffer.WriteString(line)
			if strings.Contains(line, "]") {
				inMultilineArray = false
				arr, err := parseStringArray(multilineBuffer.String())
				if err != nil {
					return nil, fmt.Errorf("line %d: invalid array: %w", lineNum, err)
				}
				if err := assignArrayField(prof, currentSection, multilineKey, arr); err != nil {
					return nil, fmt.Errorf("line %d: %w", lineNum, err)
				}
				multilineKey = ""
				multilineBuffer.Reset()
			}
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			switch currentSection {
			case "binary":
				hasBinarySection = true
			case "paths", "persistent_state", "network":
				// valid sections
			default:
				return nil, fmt.Errorf("line %d: unknown section [%s]", lineNum, currentSection)
			}
			continue
		}

		// Key-value pair
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("line %d: expected key = value, got %q", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Check if multi-line array starts
		if strings.HasPrefix(val, "[") && !strings.HasSuffix(val, "]") {
			inMultilineArray = true
			multilineKey = key
			multilineBuffer.WriteString(val)
			continue
		}

		if err := assignField(prof, currentSection, key, val); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
	}

	if inMultilineArray {
		return nil, fmt.Errorf("unterminated multi-line array for key %q", multilineKey)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}

	if !hasBinarySection || prof.Binary.Name == "" {
		return nil, fmt.Errorf("missing required [binary] section with non-empty name")
	}

	if !validNameRegex.MatchString(prof.Binary.Name) {
		return nil, fmt.Errorf("invalid binary name %q: must contain only [a-zA-Z0-9_-]", prof.Binary.Name)
	}

	// Validate allow_rw_files does not contain existing directories
	for _, file := range prof.Paths.AllowRWFiles {
		if fi, err := os.Stat(file); err == nil && fi.IsDir() {
			return nil, fmt.Errorf("allow_rw_files entry %q is a directory, not a file", file)
		}
	}

	return prof, nil
}

func assignField(prof *Profile, section, key, val string) error {
	switch section {
	case "binary":
		if key == "name" {
			parsed, err := parseString(val)
			if err != nil {
				return err
			}
			prof.Binary.Name = parsed
			return nil
		}
		return fmt.Errorf("unknown key %q in [binary]", key)

	case "paths":
		switch key {
		case "allow_ro", "allow_rw", "allow_rw_files":
			arr, err := parseStringArray(val)
			if err != nil {
				return err
			}
			return assignArrayField(prof, section, key, arr)
		default:
			return fmt.Errorf("unknown key %q in [paths]", key)
		}

	case "persistent_state":
		switch key {
		case "host_dir":
			parsed, err := parseString(val)
			if err != nil {
				return err
			}
			prof.PersistentState.HostDir = expandEnv(parsed)
			return nil
		case "mount_at":
			parsed, err := parseString(val)
			if err != nil {
				return err
			}
			prof.PersistentState.MountAt = expandEnv(parsed)
			return nil
		default:
			return fmt.Errorf("unknown key %q in [persistent_state]", key)
		}

	case "network":
		switch key {
		case "allow_domains":
			arr, err := parseStringArray(val)
			if err != nil {
				return err
			}
			prof.Network.AllowDomains = arr
			return nil
		case "allow_net":
			b, err := parseBool(val)
			if err != nil {
				return err
			}
			prof.Network.AllowNet = b
			return nil
		}
		return fmt.Errorf("unknown key %q in [network]", key)

	default:
		return fmt.Errorf("key %q outside of any section", key)
	}
}

func assignArrayField(prof *Profile, section, key string, arr []string) error {
	switch section {
	case "paths":
		switch key {
		case "allow_ro":
			prof.Paths.AllowRO = append(prof.Paths.AllowRO, arr...)
			return nil
		case "allow_rw":
			prof.Paths.AllowRW = append(prof.Paths.AllowRW, arr...)
			return nil
		case "allow_rw_files":
			prof.Paths.AllowRWFiles = append(prof.Paths.AllowRWFiles, arr...)
			return nil
		default:
			return fmt.Errorf("unknown array key %q in [paths]", key)
		}
	case "network":
		if key == "allow_domains" {
			prof.Network.AllowDomains = append(prof.Network.AllowDomains, arr...)
			return nil
		}
		return fmt.Errorf("unknown array key %q in [network] (only allow_domains is accepted; allow_net is a bool)", key)
	default:
		return fmt.Errorf("unknown section %q for array key %q", section, key)
	}
}
