package isolation

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFilterExisting(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "exists.txt")
	if err := os.WriteFile(existingFile, []byte("ok"), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	nonExisting := filepath.Join(tmpDir, "does_not_exist.txt")
	filtered := filterExisting([]string{existingFile, nonExisting, "/usr"})
	if len(filtered) < 2 {
		t.Fatalf("expected at least 2 existing paths, got %v", filtered)
	}
	for _, p := range filtered {
		if p == nonExisting {
			t.Fatalf("filtered list should not contain non-existing path %s", p)
		}
	}
}

func TestApplyLandlockSubprocess(t *testing.T) {
	if os.Getenv("TEST_LANDLOCK_CHILD") == "1" {
		if err := ApplyLandlock(); err != nil {
			os.Exit(2)
		}
		// Verify reading /root is denied specifically with EACCES
		_, err := os.ReadDir("/root")
		if err == nil {
			os.Exit(3)
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			if !errors.Is(pathErr.Err, syscall.EACCES) {
				os.Exit(5)
			}
		} else if !errors.Is(err, syscall.EACCES) {
			os.Exit(5)
		}

		// Verify reading /etc/shadow is denied specifically with EACCES (Rule 4)
		_, err = os.ReadFile("/etc/shadow")
		if err == nil {
			os.Exit(6)
		}
		if errors.As(err, &pathErr) {
			if !errors.Is(pathErr.Err, syscall.EACCES) {
				os.Exit(7)
			}
		} else if !errors.Is(err, syscall.EACCES) {
			os.Exit(7)
		}

		// Verify reading /etc/passwd is allowed
		if _, err := os.ReadFile("/etc/passwd"); err != nil {
			os.Exit(8)
		}

		// Verify writing in current working directory is allowed
		testFile := filepath.Join(".", ".landlock_test_probe")
		if err := os.WriteFile(testFile, []byte("ok"), 0600); err != nil {
			os.Exit(4)
		}
		os.Remove(testFile)
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestApplyLandlockSubprocess")
	cmd.Env = append(os.Environ(), "TEST_LANDLOCK_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("landlock subprocess test failed: %v, output: %s", err, string(out))
	}
}

func TestApplyLandlockCustomAllowPathSubprocess(t *testing.T) {
	if customPath := os.Getenv("TEST_LANDLOCK_CUSTOM_DIR"); customPath != "" {
		if err := ApplyLandlock(customPath); err != nil {
			os.Exit(2)
		}
		// Verify reading customPath succeeds
		probeFile := filepath.Join(customPath, "probe.txt")
		content, err := os.ReadFile(probeFile)
		if err != nil || string(content) != "allowed" {
			os.Exit(3)
		}
		os.Exit(0)
	}

	tmpDir := t.TempDir()
	probeFile := filepath.Join(tmpDir, "probe.txt")
	if err := os.WriteFile(probeFile, []byte("allowed"), 0600); err != nil {
		t.Fatalf("failed to create probe file: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestApplyLandlockCustomAllowPathSubprocess")
	cmd.Env = append(os.Environ(), "TEST_LANDLOCK_CUSTOM_DIR="+tmpDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("landlock custom allow path subprocess test failed: %v, output: %s", err, string(out))
	}
}
