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
		if err := ApplyLandlock(nil, nil); err != nil {
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
		if err := ApplyLandlock([]string{customPath}, nil); err != nil {
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

func TestLandlockAllowPathRW(t *testing.T) {
	if rwDir := os.Getenv("TEST_LANDLOCK_RW_DIR"); rwDir != "" {
		if err := ApplyLandlock(nil, []string{rwDir}); err != nil {
			os.Exit(2)
		}
		// Write to RW dir should succeed
		writeFile := filepath.Join(rwDir, "written.txt")
		if err := os.WriteFile(writeFile, []byte("rw_ok"), 0600); err != nil {
			os.Exit(3)
		}
		// Write to /tmp/unallowed_probe should fail with EACCES (if not cwd)
		unallowedFile := filepath.Join("/tmp", "safebox_unallowed_probe_"+os.Getenv("TEST_PID"))
		if err := os.WriteFile(unallowedFile, []byte("fail"), 0600); err == nil {
			os.Remove(unallowedFile)
			os.Exit(4)
		}
		os.Exit(0)
	}

	tmpDir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestLandlockAllowPathRW")
	cmd.Env = append(os.Environ(), "TEST_LANDLOCK_RW_DIR="+tmpDir, "TEST_PID=landlock_rw_test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("landlock allow-path-rw test failed: %v, output: %s", err, string(out))
	}
	writtenContent, err := os.ReadFile(filepath.Join(tmpDir, "written.txt"))
	if err != nil || string(writtenContent) != "rw_ok" {
		t.Fatalf("expected written file with content 'rw_ok', got err: %v, content: %q", err, string(writtenContent))
	}
}

func TestProbeLandlock(t *testing.T) {
	tmpRO := t.TempDir()
	tmpRW := t.TempDir()

	report, err := ProbeLandlock([]string{tmpRO}, []string{tmpRW})
	if err != nil {
		t.Fatalf("unexpected error from ProbeLandlock: %v", err)
	}

	if report.WorkingDir == "" {
		t.Errorf("expected non-empty WorkingDir in ProbeReport")
	}

	foundRO := false
	for _, ro := range report.EffectiveRO {
		if ro == tmpRO {
			foundRO = true
			break
		}
	}
	if !foundRO {
		t.Errorf("expected tmpRO %s in EffectiveRO, got %v", tmpRO, report.EffectiveRO)
	}

	foundRW := false
	for _, rw := range report.EffectiveRW {
		if rw == tmpRW {
			foundRW = true
			break
		}
	}
	if !foundRW {
		t.Errorf("expected tmpRW %s in EffectiveRW, got %v", tmpRW, report.EffectiveRW)
	}
}
