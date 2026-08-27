package revert

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestScanShadowChangesEmptyUpperDir(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")

	if err := os.MkdirAll(lowerDir, 0700); err != nil {
		t.Fatalf("failed to create lowerDir: %v", err)
	}
	if err := os.MkdirAll(upperDir, 0700); err != nil {
		t.Fatalf("failed to create upperDir: %v", err)
	}

	changes, err := ScanShadowChanges(lowerDir, upperDir)
	if err != nil {
		t.Fatalf("ScanShadowChanges failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for clean upperDir, got %d", len(changes))
	}

	// Non-existent upperDir returns empty slice
	nonExistentUpper := filepath.Join(tmpDir, "does-not-exist")
	changes, err = ScanShadowChanges(lowerDir, nonExistentUpper)
	if err != nil {
		t.Fatalf("ScanShadowChanges on non-existent dir failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for non-existent upperDir, got %d", len(changes))
	}

	// Empty string error
	if _, err := ScanShadowChanges(lowerDir, ""); err == nil {
		t.Error("expected error for empty upperDir")
	}

	// File instead of dir error
	filePath := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if _, err := ScanShadowChanges(lowerDir, filePath); err == nil {
		t.Error("expected error when upperDir is a file")
	}
}

func TestScanShadowChangesAddedAndModified(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")

	if err := os.MkdirAll(filepath.Join(lowerDir, "nested"), 0700); err != nil {
		t.Fatalf("failed to create lower dirs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(upperDir, "nested"), 0700); err != nil {
		t.Fatalf("failed to create upper dirs: %v", err)
	}

	// Lower files
	if err := os.WriteFile(filepath.Join(lowerDir, "base.txt"), []byte("original base\n"), 0600); err != nil {
		t.Fatalf("failed to write base.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lowerDir, "nested", "common.txt"), []byte("original common\n"), 0600); err != nil {
		t.Fatalf("failed to write common.txt: %v", err)
	}

	// Upper files (base.txt modified, new.txt added, nested/new_nested.txt added)
	if err := os.WriteFile(filepath.Join(upperDir, "base.txt"), []byte("modified base\n"), 0600); err != nil {
		t.Fatalf("failed to write upper base.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("new file\n"), 0600); err != nil {
		t.Fatalf("failed to write upper new.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(upperDir, "nested", "new_nested.txt"), []byte("nested new\n"), 0600); err != nil {
		t.Fatalf("failed to write upper nested/new_nested.txt: %v", err)
	}

	changes, err := ScanShadowChanges(lowerDir, upperDir)
	if err != nil {
		t.Fatalf("ScanShadowChanges failed: %v", err)
	}

	expected := []FileChange{
		{Path: "base.txt", Type: ChangeModified},
		{Path: "nested/new_nested.txt", Type: ChangeAdded},
		{Path: "new.txt", Type: ChangeAdded},
	}

	if len(changes) != len(expected) {
		t.Fatalf("expected %d changes, got %d: %+v", len(expected), len(changes), changes)
	}

	for i, exp := range expected {
		if changes[i].Path != exp.Path || changes[i].Type != exp.Type {
			t.Errorf("change %d mismatch: expected %+v, got %+v", i, exp, changes[i])
		}
	}
}

func TestScanShadowChangesWhiteoutDeleted(t *testing.T) {
	if os.Getenv("GO_WANT_WHITEOUT_HELPER_PROCESS") == "1" {
		lowerDir := os.Getenv("SAFEBOX_TEST_LOWER")
		upperDir := os.Getenv("SAFEBOX_TEST_UPPER")
		workDir := os.Getenv("SAFEBOX_TEST_WORK")
		mergedDir := os.Getenv("SAFEBOX_TEST_MERGED")

		opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
		if err := syscall.Mount("overlay", mergedDir, "overlay", 0, opts); err != nil {
			os.Stderr.WriteString("mount error: " + err.Error() + "\n")
			os.Exit(1)
		}

		// Delete victim file inside overlay
		victimFile := filepath.Join(mergedDir, "victim.txt")
		if err := os.Remove(victimFile); err != nil {
			os.Stderr.WriteString("remove error: " + err.Error() + "\n")
			os.Exit(2)
		}

		os.Exit(0)
	}

	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")
	workDir := filepath.Join(tmpDir, "work")
	mergedDir := filepath.Join(tmpDir, "merged")

	for _, d := range []string{lowerDir, upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatalf("failed to create directory %s: %v", d, err)
		}
	}

	victimPath := filepath.Join(lowerDir, "victim.txt")
	if err := os.WriteFile(victimPath, []byte("victim to delete\n"), 0600); err != nil {
		t.Fatalf("failed to write victim.txt: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestScanShadowChangesWhiteoutDeleted")
	cmd.Env = append(os.Environ(),
		"GO_WANT_WHITEOUT_HELPER_PROCESS=1",
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
		t.Fatalf("subprocess failed: %v, output: %s", err, string(out))
	}

	// Verify ScanShadowChanges reports victim.txt as Deleted
	changes, err := ScanShadowChanges(lowerDir, upperDir)
	if err != nil {
		t.Fatalf("ScanShadowChanges failed: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(changes), changes)
	}

	if changes[0].Path != "victim.txt" || changes[0].Type != ChangeDeleted {
		t.Errorf("expected victim.txt Deleted, got: %+v", changes[0])
	}
}

func TestRunShadowDiffOutput(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")

	if err := os.MkdirAll(lowerDir, 0700); err != nil {
		t.Fatalf("failed to create lowerDir: %v", err)
	}
	if err := os.MkdirAll(upperDir, 0700); err != nil {
		t.Fatalf("failed to create upperDir: %v", err)
	}

	// 1. Clean upperDir output
	var buf bytes.Buffer
	if err := RunShadowDiff(lowerDir, upperDir, &buf); err != nil {
		t.Fatalf("RunShadowDiff clean failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Working tree is clean. No changes detected.") {
		t.Errorf("expected clean message, got: %s", buf.String())
	}

	// 2. Add files to upperDir
	if err := os.WriteFile(filepath.Join(lowerDir, "existing.txt"), []byte("initial\n"), 0600); err != nil {
		t.Fatalf("failed to write existing.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("modified\n"), 0600); err != nil {
		t.Fatalf("failed to write upper existing.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(upperDir, "added.txt"), []byte("new\n"), 0600); err != nil {
		t.Fatalf("failed to write upper added.txt: %v", err)
	}

	buf.Reset()
	if err := RunShadowDiff(lowerDir, upperDir, &buf); err != nil {
		t.Fatalf("RunShadowDiff with changes failed: %v", err)
	}

	diffOut := buf.String()
	if !strings.Contains(diffOut, "+ [ADDED]") || !strings.Contains(diffOut, "added.txt") {
		t.Errorf("expected added.txt in diff output, got: %s", diffOut)
	}
	if !strings.Contains(diffOut, "~ [MODIFIED]") || !strings.Contains(diffOut, "existing.txt") {
		t.Errorf("expected existing.txt in diff output, got: %s", diffOut)
	}
}
