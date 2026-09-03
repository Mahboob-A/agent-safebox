package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProfile_Valid(t *testing.T) {
	data := []byte(`
# Agent profile for testing
[binary]
name = "test-agent"

[paths]
allow_ro = ["/usr/local/bin", "/opt/tools"]
allow_rw = ["$HOME/.test-agent", "/tmp/scratch"]
allow_rw_files = ["$HOME/.test-agent-config.json"]

[persistent_state]
host_dir = "$XDG_STATE_HOME/safebox/agents/test-agent"
mount_at = "$HOME/.test-agent"

[network]
allow_domains = ["api.example.com", "auth.example.com"]
`)

	prof, err := parseProfile(data)
	if err != nil {
		t.Fatalf("parseProfile failed: %v", err)
	}

	if prof.Binary.Name != "test-agent" {
		t.Errorf("expected binary name 'test-agent', got %q", prof.Binary.Name)
	}

	if len(prof.Paths.AllowRO) != 2 || prof.Paths.AllowRO[0] != "/usr/local/bin" {
		t.Errorf("unexpected allow_ro: %v", prof.Paths.AllowRO)
	}

	home, _ := os.UserHomeDir()
	expectedRW := filepath.Join(home, ".test-agent")
	if len(prof.Paths.AllowRW) != 2 || prof.Paths.AllowRW[0] != expectedRW {
		t.Errorf("unexpected allow_rw: %v, expected first to be %q", prof.Paths.AllowRW, expectedRW)
	}

	expectedFile := filepath.Join(home, ".test-agent-config.json")
	if len(prof.Paths.AllowRWFiles) != 1 || prof.Paths.AllowRWFiles[0] != expectedFile {
		t.Errorf("unexpected allow_rw_files: %v, expected %q", prof.Paths.AllowRWFiles, expectedFile)
	}

	if len(prof.Network.AllowDomains) != 2 || prof.Network.AllowDomains[0] != "api.example.com" {
		t.Errorf("unexpected allow_domains: %v", prof.Network.AllowDomains)
	}
}

func TestParseProfile_MultiLineArray(t *testing.T) {
	data := []byte(`
[binary]
name = "multiline-agent"

[paths]
allow_ro = [
    "/usr/local/bin",
    "/opt/extra/bin",
]
allow_rw = [
    "/var/data",
    "/var/log",
]
`)

	prof, err := parseProfile(data)
	if err != nil {
		t.Fatalf("parseProfile with multi-line arrays failed: %v", err)
	}

	if len(prof.Paths.AllowRO) != 2 || prof.Paths.AllowRO[1] != "/opt/extra/bin" {
		t.Errorf("unexpected allow_ro: %v", prof.Paths.AllowRO)
	}
	if len(prof.Paths.AllowRW) != 2 || prof.Paths.AllowRW[0] != "/var/data" {
		t.Errorf("unexpected allow_rw: %v", prof.Paths.AllowRW)
	}
}

func TestParseProfile_UnknownField(t *testing.T) {
	data := []byte(`
[binary]
name = "agent"
unknown_key = "value"
`)

	_, err := parseProfile(data)
	if err == nil {
		t.Fatalf("expected error for unknown key, got nil")
	}
}

func TestParseProfile_UnknownSection(t *testing.T) {
	data := []byte(`
[binary]
name = "agent"

[unsupported_section]
foo = "bar"
`)

	_, err := parseProfile(data)
	if err == nil {
		t.Fatalf("expected error for unknown section, got nil")
	}
}

func TestParseProfile_MissingBinary(t *testing.T) {
	data := []byte(`
[paths]
allow_rw = ["/tmp"]
`)

	_, err := parseProfile(data)
	if err == nil {
		t.Fatalf("expected error for missing [binary] section, got nil")
	}
}

func TestParseProfile_InvalidNameChars(t *testing.T) {
	data := []byte(`
[binary]
name = "bad name with spaces!"
`)

	_, err := parseProfile(data)
	if err == nil {
		t.Fatalf("expected error for invalid binary name characters, got nil")
	}
}

func TestParseProfile_DirectoryInAllowRWFiles(t *testing.T) {
	tempDir := t.TempDir()
	data := []byte(`
[binary]
name = "bad-file-agent"

[paths]
allow_rw_files = ["` + tempDir + `"]
`)

	_, err := parseProfile(data)
	if err == nil {
		t.Fatalf("expected error when allow_rw_files points to a directory, got nil")
	}
}

func TestLookupByArgv0Substring(t *testing.T) {
	prof, err := Lookup("agy")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if prof == nil {
		t.Fatalf("expected profile for 'agy', got nil")
	}
	if prof.Binary.Name != "agy" {
		t.Errorf("expected binary name 'agy', got %q", prof.Binary.Name)
	}

	// Test full path with substring match
	profPath, err := Lookup("/usr/local/bin/agy-cli")
	if err != nil {
		t.Fatalf("Lookup with path failed: %v", err)
	}
	if profPath == nil || profPath.Binary.Name != "agy" {
		t.Errorf("expected profile match for substring in full path, got %v", profPath)
	}
}

func TestLookupNoMatch(t *testing.T) {
	prof, err := Lookup("unknown-random-tool-xyz")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if prof != nil {
		t.Fatalf("expected nil for unknown tool, got %v", prof)
	}
}

func TestLookupUserOverridesBuiltin(t *testing.T) {
	tempConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempConfig)

	profileDir := filepath.Join(tempConfig, "safebox", "profiles")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatalf("failed to create profile dir: %v", err)
	}

	customAgy := []byte(`
[binary]
name = "agy"

[paths]
allow_rw = ["/custom/agy/override"]
`)
	if err := os.WriteFile(filepath.Join(profileDir, "agy.toml"), customAgy, 0600); err != nil {
		t.Fatalf("failed to write custom profile: %v", err)
	}

	prof, err := Lookup("agy")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if prof == nil {
		t.Fatalf("expected overridden profile, got nil")
	}
	if len(prof.Paths.AllowRW) != 1 || prof.Paths.AllowRW[0] != "/custom/agy/override" {
		t.Errorf("expected custom allow_rw path, got %v", prof.Paths.AllowRW)
	}
}

func TestLoadUserProfiles_ErrorTolerance(t *testing.T) {
	tempConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempConfig)

	profileDir := filepath.Join(tempConfig, "safebox", "profiles")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatalf("failed to create profile dir: %v", err)
	}

	// 1. Valid profile
	valid := []byte(`
[binary]
name = "valid-tool"
[paths]
allow_rw = ["/tmp/valid"]
`)
	_ = os.WriteFile(filepath.Join(profileDir, "valid.toml"), valid, 0600)

	// 2. Corrupt/invalid TOML profile (should be skipped with warning)
	invalid := []byte(`
[binary
name = "corrupt
`)
	_ = os.WriteFile(filepath.Join(profileDir, "corrupt.toml"), invalid, 0600)

	profs, err := LoadUserProfiles()
	if err != nil {
		t.Fatalf("LoadUserProfiles returned error: %v", err)
	}

	if len(profs) != 1 {
		t.Fatalf("expected 1 valid profile loaded, got %d", len(profs))
	}
	if profs[0].Binary.Name != "valid-tool" {
		t.Errorf("expected 'valid-tool', got %q", profs[0].Binary.Name)
	}
}

func TestBuiltins_All16Profiles(t *testing.T) {
	builtins, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins failed: %v", err)
	}

	if len(builtins) != 16 {
		t.Fatalf("expected 16 built-in profiles, got %d", len(builtins))
	}

	expectedTools := map[string]bool{
		"agy":      false,
		"claude":   false,
		"codex":    false,
		"puku":     false,
		"gemini":   false,
		"cursor":   false,
		"kilo":     false,
		"opencode": false,
		"aider":    false,
		"pi":       false,
		"cline":    false,
		"amp":      false,
		"goose":    false,
		"mentat":   false,
		"continue": false,
		"plandex":  false,
	}

	for _, prof := range builtins {
		if _, ok := expectedTools[prof.Binary.Name]; !ok {
			t.Errorf("unexpected profile in built-ins: %q", prof.Binary.Name)
		}
		expectedTools[prof.Binary.Name] = true

		if len(prof.Paths.AllowRW) == 0 {
			t.Errorf("profile %q has empty allow_rw paths", prof.Binary.Name)
		}
	}

	for tool, found := range expectedTools {
		if !found {
			t.Errorf("expected built-in profile %q not found", tool)
		}
	}
}

func TestBuiltins_All16ProfilesPersistentState(t *testing.T) {
	builtins, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins failed: %v", err)
	}

	for _, prof := range builtins {
		if prof.PersistentState.HostDir == "" {
			t.Errorf("profile %q has empty persistent_state.host_dir", prof.Binary.Name)
		}
		if prof.PersistentState.MountAt == "" {
			t.Errorf("profile %q has empty persistent_state.mount_at", prof.Binary.Name)
		}

		if prof.Binary.Name == "claude" {
			foundJson := false
			for _, f := range prof.Paths.AllowRWFiles {
				if strings.HasSuffix(f, ".claude.json") {
					foundJson = true
					break
				}
			}
			if !foundJson {
				t.Errorf("claude profile missing allow_rw_files for .claude.json")
			}
		}

		if prof.Binary.Name == "goose" {
			if !strings.HasSuffix(prof.PersistentState.MountAt, "goose") {
				t.Errorf("goose profile persistent state mount_at expected to end with 'goose', got %q", prof.PersistentState.MountAt)
			}
		}
	}
}

func TestParseProfile_NetworkSection(t *testing.T) {
	data := []byte(`
[binary]
name = "net-agent"

[network]
allow_domains = [
    "api.openai.com",
    "*.googleusercontent.com",
    "pypi.org"
]
`)

	prof, err := parseProfile(data)
	if err != nil {
		t.Fatalf("parseProfile failed: %v", err)
	}

	if len(prof.Network.AllowDomains) != 3 {
		t.Fatalf("expected 3 allow_domains, got %d", len(prof.Network.AllowDomains))
	}
	if prof.Network.AllowDomains[0] != "api.openai.com" ||
		prof.Network.AllowDomains[1] != "*.googleusercontent.com" ||
		prof.Network.AllowDomains[2] != "pypi.org" {
		t.Errorf("unexpected allow_domains: %v", prof.Network.AllowDomains)
	}
}

func TestParseProfile_NetworkSectionErrors(t *testing.T) {
	// 1. Unknown key in [network]
	badKey := []byte(`
[binary]
name = "bad-net"

[network]
unknown_field = ["api.example.com"]
`)
	if _, err := parseProfile(badKey); err == nil {
		t.Errorf("expected error for unknown key in [network], got nil")
	}

	// 2. Invalid array syntax in allow_domains
	badArray := []byte(`
[binary]
name = "bad-array"

[network]
allow_domains = "api.example.com"
`)
	if _, err := parseProfile(badArray); err == nil {
		t.Errorf("expected error for invalid array syntax, got nil")
	}
}

func TestBuiltins_NetworkAllowNet(t *testing.T) {
	builtins, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins failed: %v", err)
	}

	// Profiles for known AI coding agents set allow_net = true in v1.
	allowNetProfiles := map[string]bool{
		"agy":    true,
		"gemini": true,
		"claude": true,
		"codex":  true,
		"amp":    true,
		"mentat": true,
	}

	for _, prof := range builtins {
		expected, isKnown := allowNetProfiles[prof.Binary.Name]
		if isKnown {
			if prof.Network.AllowNet != expected {
				t.Errorf("profile %q expected AllowNet=%v, got %v", prof.Binary.Name, expected, prof.Network.AllowNet)
			}
			if len(prof.Network.AllowDomains) != 0 {
				t.Errorf("profile %q expected empty AllowDomains (dormant in v1), got %v", prof.Binary.Name, prof.Network.AllowDomains)
			}
		} else {
			if prof.Network.AllowNet {
				t.Errorf("non-cloud profile %q expected AllowNet=false, got true", prof.Binary.Name)
			}
		}
	}
}




