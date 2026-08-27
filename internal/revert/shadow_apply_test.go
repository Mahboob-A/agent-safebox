package revert

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestApplyShadowChanges_Validation(t *testing.T) {
	if err := ApplyShadowChanges("", "/tmp/upper"); err == nil {
		t.Error("expected error for empty lowerDir")
	}
	if err := ApplyShadowChanges("/tmp/lower", ""); err == nil {
		t.Error("expected error for empty upperDir")
	}
}

func TestApplyShadowChanges_AddAndModify(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")

	if err := os.MkdirAll(filepath.Join(lowerDir, "nested"), 0700); err != nil {
		t.Fatalf("failed to create lower dirs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(upperDir, "nested"), 0700); err != nil {
		t.Fatalf("failed to create upper dirs: %v", err)
	}

	// Lower base files
	if err := os.WriteFile(filepath.Join(lowerDir, "initial.txt"), []byte("v1 content\n"), 0600); err != nil {
		t.Fatalf("failed to write initial.txt: %v", err)
	}

	// Upper modifications and additions
	if err := os.WriteFile(filepath.Join(upperDir, "initial.txt"), []byte("v2 modified\n"), 0600); err != nil {
		t.Fatalf("failed to write upper initial.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(upperDir, "new_file.txt"), []byte("newly added\n"), 0600); err != nil {
		t.Fatalf("failed to write new_file.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(upperDir, "nested", "deep.txt"), []byte("deep added\n"), 0600); err != nil {
		t.Fatalf("failed to write deep.txt: %v", err)
	}

	if err := ApplyShadowChanges(lowerDir, upperDir); err != nil {
		t.Fatalf("ApplyShadowChanges failed: %v", err)
	}

	// Verify lowerDir state
	modContent, err := os.ReadFile(filepath.Join(lowerDir, "initial.txt"))
	if err != nil {
		t.Fatalf("failed to read lower initial.txt: %v", err)
	}
	if string(modContent) != "v2 modified\n" {
		t.Errorf("expected initial.txt to be %q, got %q", "v2 modified\n", string(modContent))
	}

	newContent, err := os.ReadFile(filepath.Join(lowerDir, "new_file.txt"))
	if err != nil {
		t.Fatalf("failed to read lower new_file.txt: %v", err)
	}
	if string(newContent) != "newly added\n" {
		t.Errorf("expected new_file.txt to be %q, got %q", "newly added\n", string(newContent))
	}

	deepContent, err := os.ReadFile(filepath.Join(lowerDir, "nested", "deep.txt"))
	if err != nil {
		t.Fatalf("failed to read lower deep.txt: %v", err)
	}
	if string(deepContent) != "deep added\n" {
		t.Errorf("expected deep.txt to be %q, got %q", "deep added\n", string(deepContent))
	}
}

func TestDiscardShadowChanges(t *testing.T) {
	tmpDir := t.TempDir()
	sessionDir := filepath.Join(tmpDir, "shadow-session")

	if err := os.MkdirAll(filepath.Join(sessionDir, "upper"), 0700); err != nil {
		t.Fatalf("failed to create session upper dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "upper", "scratch.txt"), []byte("data"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if err := DiscardShadowChanges(sessionDir); err != nil {
		t.Fatalf("DiscardShadowChanges failed: %v", err)
	}

	if _, err := os.Stat(sessionDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected session directory to be deleted, err: %v", err)
	}

	if err := DiscardShadowChanges(""); err != nil {
		t.Errorf("expected no error for empty session path, got: %v", err)
	}
}

func TestApplyShadowChanges_Subprocess(t *testing.T) {
	if os.Getenv("GO_WANT_APPLY_HELPER_PROCESS") == "1" {
		lowerDir := os.Getenv("SAFEBOX_TEST_LOWER")
		upperDir := os.Getenv("SAFEBOX_TEST_UPPER")
		workDir := os.Getenv("SAFEBOX_TEST_WORK")
		mergedDir := os.Getenv("SAFEBOX_TEST_MERGED")

		opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
		if err := syscall.Mount("overlay", mergedDir, "overlay", 0, opts); err != nil {
			os.Stderr.WriteString("mount error: " + err.Error() + "\n")
			os.Exit(1)
		}

		// 1. Modify existing file
		if err := os.WriteFile(filepath.Join(mergedDir, "base.txt"), []byte("modified base\n"), 0600); err != nil {
			os.Stderr.WriteString("modify error: " + err.Error() + "\n")
			os.Exit(2)
		}

		// 2. Add new file
		if err := os.WriteFile(filepath.Join(mergedDir, "created.txt"), []byte("newly created\n"), 0600); err != nil {
			os.Stderr.WriteString("create error: " + err.Error() + "\n")
			os.Exit(3)
		}

		// 3. Delete file
		if err := os.Remove(filepath.Join(mergedDir, "to_delete.txt")); err != nil {
			os.Stderr.WriteString("delete error: " + err.Error() + "\n")
			os.Exit(4)
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
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Setup initial lower files
	if err := os.WriteFile(filepath.Join(lowerDir, "base.txt"), []byte("original base\n"), 0600); err != nil {
		t.Fatalf("failed to write base.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lowerDir, "to_delete.txt"), []byte("delete me\n"), 0600); err != nil {
		t.Fatalf("failed to write to_delete.txt: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestApplyShadowChanges_Subprocess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_APPLY_HELPER_PROCESS=1",
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

	// Apply shadow changes to lowerDir
	if err := ApplyShadowChanges(lowerDir, upperDir); err != nil {
		t.Fatalf("ApplyShadowChanges failed: %v", err)
	}

	// Verify lowerDir has all updates applied
	modContent, err := os.ReadFile(filepath.Join(lowerDir, "base.txt"))
	if err != nil {
		t.Fatalf("failed to read base.txt: %v", err)
	}
	if string(modContent) != "modified base\n" {
		t.Errorf("expected base.txt to be %q, got %q", "modified base\n", string(modContent))
	}

	newContent, err := os.ReadFile(filepath.Join(lowerDir, "created.txt"))
	if err != nil {
		t.Fatalf("failed to read created.txt: %v", err)
	}
	if string(newContent) != "newly created\n" {
		t.Errorf("expected created.txt to be %q, got %q", "newly created\n", string(newContent))
	}

	if _, err := os.Stat(filepath.Join(lowerDir, "to_delete.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected to_delete.txt to be deleted from lowerDir, err: %v", err)
	}
}
