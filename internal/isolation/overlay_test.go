package isolation

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestEnsureOverlayDirs(t *testing.T) {
	tmpDir := t.TempDir()
	sessionDir := filepath.Join(tmpDir, "session-123")

	upper, work, err := EnsureOverlayDirs(sessionDir)
	if err != nil {
		t.Fatalf("EnsureOverlayDirs failed: %v", err)
	}

	if upper != filepath.Join(sessionDir, "upper") {
		t.Errorf("expected upper path %q, got %q", filepath.Join(sessionDir, "upper"), upper)
	}
	if work != filepath.Join(sessionDir, "work") {
		t.Errorf("expected work path %q, got %q", filepath.Join(sessionDir, "work"), work)
	}

	if info, err := os.Stat(upper); err != nil || !info.IsDir() {
		t.Errorf("expected upper dir to exist, err: %v", err)
	}
	if info, err := os.Stat(work); err != nil || !info.IsDir() {
		t.Errorf("expected work dir to exist, err: %v", err)
	}

	// Test empty path error
	if _, _, err := EnsureOverlayDirs(""); err == nil {
		t.Error("expected error for empty base directory")
	}
}

func TestCleanupOverlayDirs(t *testing.T) {
	tmpDir := t.TempDir()
	sessionDir := filepath.Join(tmpDir, "session-456")

	_, _, err := EnsureOverlayDirs(sessionDir)
	if err != nil {
		t.Fatalf("EnsureOverlayDirs failed: %v", err)
	}

	if err := CleanupOverlayDirs(sessionDir); err != nil {
		t.Fatalf("CleanupOverlayDirs failed: %v", err)
	}

	if _, err := os.Stat(sessionDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected session directory to be deleted, err: %v", err)
	}

	// Cleanup empty path should be no-op
	if err := CleanupOverlayDirs(""); err != nil {
		t.Errorf("expected no-op on empty path, got: %v", err)
	}
}

func TestMountOverlayValidation(t *testing.T) {
	invalidConfigs := []OverlayConfig{
		{LowerDir: "", UpperDir: "/u", WorkDir: "/w", TargetDir: "/t"},
		{LowerDir: "/l", UpperDir: "", WorkDir: "/w", TargetDir: "/t"},
		{LowerDir: "/l", UpperDir: "/u", WorkDir: "", TargetDir: "/t"},
		{LowerDir: "/l", UpperDir: "/u", WorkDir: "/w", TargetDir: ""},
	}

	for _, cfg := range invalidConfigs {
		if err := MountOverlay(cfg); err == nil {
			t.Errorf("expected error for invalid config %+v, got nil", cfg)
		}
	}
}

func TestMountOverlaySubprocess(t *testing.T) {
	if os.Getenv("GO_WANT_OVERLAY_HELPER_PROCESS") == "1" {
		lowerDir := os.Getenv("SAFEBOX_TEST_LOWER")
		upperDir := os.Getenv("SAFEBOX_TEST_UPPER")
		workDir := os.Getenv("SAFEBOX_TEST_WORK")
		targetDir := os.Getenv("SAFEBOX_TEST_TARGET")

		cfg := OverlayConfig{
			LowerDir:  lowerDir,
			UpperDir:  upperDir,
			WorkDir:   workDir,
			TargetDir: targetDir,
		}

		if err := MountOverlay(cfg); err != nil {
			os.Stderr.WriteString("mount error: " + err.Error() + "\n")
			os.Exit(1)
		}

		// Write a file to target
		testFile := filepath.Join(targetDir, "created_inside_overlay.txt")
		if err := os.WriteFile(testFile, []byte("shadow content\n"), 0600); err != nil {
			os.Stderr.WriteString("write error: " + err.Error() + "\n")
			os.Exit(2)
		}

		// Modify existing lower file
		modFile := filepath.Join(targetDir, "existing.txt")
		if err := os.WriteFile(modFile, []byte("modified in overlay\n"), 0600); err != nil {
			os.Stderr.WriteString("modify error: " + err.Error() + "\n")
			os.Exit(3)
		}

		if err := UnmountOverlay(targetDir); err != nil {
			os.Stderr.WriteString("unmount error: " + err.Error() + "\n")
			os.Exit(4)
		}

		os.Exit(0)
	}

	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	sessionDir := filepath.Join(tmpDir, "session")

	if err := os.MkdirAll(lowerDir, 0700); err != nil {
		t.Fatalf("failed to create lowerDir: %v", err)
	}

	// Create initial file in lowerDir
	existingFile := filepath.Join(lowerDir, "existing.txt")
	if err := os.WriteFile(existingFile, []byte("original content\n"), 0600); err != nil {
		t.Fatalf("failed to write existing file: %v", err)
	}

	upperDir, workDir, err := EnsureOverlayDirs(sessionDir)
	if err != nil {
		t.Fatalf("failed to create overlay dirs: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMountOverlaySubprocess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_OVERLAY_HELPER_PROCESS=1",
		"SAFEBOX_TEST_LOWER="+lowerDir,
		"SAFEBOX_TEST_UPPER="+upperDir,
		"SAFEBOX_TEST_WORK="+workDir,
		"SAFEBOX_TEST_TARGET="+lowerDir,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v, output: %s", err, string(out))
	}

	// Verify on host: lowerDir is completely unmodified
	if _, err := os.Stat(filepath.Join(lowerDir, "created_inside_overlay.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("expected created file NOT to exist in host lowerDir")
	}

	content, err := os.ReadFile(existingFile)
	if err != nil {
		t.Fatalf("failed to read existing file on host: %v", err)
	}
	if string(content) != "original content\n" {
		t.Errorf("expected host existing file to remain %q, got %q", "original content\n", string(content))
	}

	// Verify upperDir contains the shadow modifications
	upperCreated := filepath.Join(upperDir, "created_inside_overlay.txt")
	if _, err := os.Stat(upperCreated); err != nil {
		t.Errorf("expected created file to exist in upperDir, err: %v", err)
	}

	upperModified := filepath.Join(upperDir, "existing.txt")
	modContent, err := os.ReadFile(upperModified)
	if err != nil {
		t.Fatalf("failed to read modified file in upperDir: %v", err)
	}
	if string(modContent) != "modified in overlay\n" {
		t.Errorf("expected upperDir file to be %q, got %q", "modified in overlay\n", string(modContent))
	}
}

func TestUnmountOverlayValidation(t *testing.T) {
	if err := UnmountOverlay(""); err == nil {
		t.Error("expected error for empty target directory in UnmountOverlay")
	}
}

func TestMountSessionOverlaySubprocess(t *testing.T) {
	if os.Getenv("GO_WANT_SESSION_OVERLAY_HELPER") == "1" {
		lowerDir := os.Getenv("SAFEBOX_TEST_LOWER")
		upperDir := os.Getenv("SAFEBOX_TEST_UPPER")
		workDir := os.Getenv("SAFEBOX_TEST_WORK")
		mergedDir := os.Getenv("SAFEBOX_TEST_MERGED")

		if err := MountSessionOverlay(lowerDir, upperDir, workDir, mergedDir); err != nil {
			os.Stderr.WriteString("mount error: " + err.Error() + "\n")
			os.Exit(1)
		}

		testFile := filepath.Join(mergedDir, "session_probe.txt")
		if err := os.WriteFile(testFile, []byte("overlay session data\n"), 0600); err != nil {
			os.Stderr.WriteString("write error: " + err.Error() + "\n")
			os.Exit(2)
		}

		if err := UnmountOverlay(mergedDir); err != nil {
			os.Stderr.WriteString("unmount error: " + err.Error() + "\n")
			os.Exit(3)
		}
		os.Exit(0)
	}

	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")
	workDir := filepath.Join(tmpDir, "work")
	mergedDir := filepath.Join(tmpDir, "merged")

	if err := os.MkdirAll(lowerDir, 0700); err != nil {
		t.Fatalf("failed to create lowerDir: %v", err)
	}
	if err := os.MkdirAll(upperDir, 0700); err != nil {
		t.Fatalf("failed to create upperDir: %v", err)
	}
	if err := os.MkdirAll(workDir, 0700); err != nil {
		t.Fatalf("failed to create workDir: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMountSessionOverlaySubprocess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_SESSION_OVERLAY_HELPER=1",
		"SAFEBOX_TEST_LOWER="+lowerDir,
		"SAFEBOX_TEST_UPPER="+upperDir,
		"SAFEBOX_TEST_WORK="+workDir,
		"SAFEBOX_TEST_MERGED="+mergedDir,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("session overlay subprocess failed: %v, output: %s", err, string(out))
	}

	// Verify upper contains the written file
	upperCreated := filepath.Join(upperDir, "session_probe.txt")
	if _, err := os.Stat(upperCreated); err != nil {
		t.Errorf("expected session_probe.txt to exist in upperDir, err: %v", err)
	}
	// Verify lower is unchanged
	lowerCreated := filepath.Join(lowerDir, "session_probe.txt")
	if _, err := os.Stat(lowerCreated); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected session_probe.txt NOT to exist in lowerDir")
	}
}
