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
	t.Cleanup(func() {
		_ = SafeRemoveAll(workDir)
	})

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

func TestApplyShadowChangesAtomicOnCopyFailure(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")

	if err := os.MkdirAll(lowerDir, 0700); err != nil {
		t.Fatalf("failed to create lowerDir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(upperDir, "conflict", "nested"), 0700); err != nil {
		t.Fatalf("failed to create upperDir: %v", err)
	}

	// 1. Initial lower file
	if err := os.WriteFile(filepath.Join(lowerDir, "existing.txt"), []byte("original content"), 0600); err != nil {
		t.Fatalf("failed to write lower file: %v", err)
	}

	// 2. Upper file 1 (valid modification)
	if err := os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("mutated content"), 0600); err != nil {
		t.Fatalf("failed to write upper modified file: %v", err)
	}

	// 3. Upper file 2 in subfolder
	if err := os.WriteFile(filepath.Join(upperDir, "conflict", "nested", "sub.txt"), []byte("sub content"), 0600); err != nil {
		t.Fatalf("failed to write upper nested file: %v", err)
	}

	// Pre-create a non-directory file in staging directory that will cause MkdirAll to fail with ENOTDIR
	sessionID := filepath.Base(filepath.Dir(upperDir))
	stagingDir := filepath.Join(SessionRoot(), "apply-staging", sessionID)
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		t.Fatalf("failed to create staging dir: %v", err)
	}
	// Conflict: stagingDir/conflict is a regular file instead of a directory
	if err := os.WriteFile(filepath.Join(stagingDir, "conflict"), []byte("blocking file"), 0600); err != nil {
		t.Fatalf("failed to write conflict file: %v", err)
	}
	defer os.RemoveAll(stagingDir)

	// Apply should fail during staging before touching lowerDir
	err := ApplyShadowChanges(lowerDir, upperDir)
	if err == nil {
		t.Fatal("expected ApplyShadowChanges to fail on staging copy error, got nil")
	}

	// Lower file MUST remain untouched
	content, err := os.ReadFile(filepath.Join(lowerDir, "existing.txt"))
	if err != nil {
		t.Fatalf("failed to read lower file: %v", err)
	}
	if string(content) != "original content" {
		t.Errorf("expected lower file to remain %q, but got %q", "original content", string(content))
	}

	// Nested file must not exist in lowerDir
	if _, err := os.Stat(filepath.Join(lowerDir, "conflict", "nested", "sub.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected nested file to not exist in lowerDir, err: %v", err)
	}
}

func TestApplyShadowChangesEXDEVCrossDeviceFallback(t *testing.T) {
	if !isEXDEV(syscall.EXDEV) {
		t.Error("expected isEXDEV(syscall.EXDEV) to be true")
	}
	if !isEXDEV(&os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV}) {
		t.Error("expected isEXDEV(LinkError with EXDEV) to be true")
	}
	if isEXDEV(os.ErrNotExist) {
		t.Error("expected isEXDEV(ErrNotExist) to be false")
	}
	if isEXDEV(nil) {
		t.Error("expected isEXDEV(nil) to be false")
	}
}

func TestRemoveLower_Direct(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Regular file deletion
	regFile := filepath.Join(tmpDir, "regular.txt")
	if err := os.WriteFile(regFile, []byte("data"), 0600); err != nil {
		t.Fatalf("failed to write regular file: %v", err)
	}
	if err := removeLower(regFile); err != nil {
		t.Errorf("removeLower on regular file failed: %v", err)
	}
	if _, err := os.Stat(regFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected regular file to be deleted, err: %v", err)
	}

	// 2. Directory tree recursive deletion
	treeDir := filepath.Join(tmpDir, "tree", "sub")
	if err := os.MkdirAll(treeDir, 0700); err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(treeDir, "leaf.txt"), []byte("leaf"), 0600); err != nil {
		t.Fatalf("failed to write leaf: %v", err)
	}
	if err := removeLower(filepath.Join(tmpDir, "tree")); err != nil {
		t.Errorf("removeLower on directory tree failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "tree")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected tree to be deleted, err: %v", err)
	}

	// 3. Non-existent path returns nil
	if err := removeLower(filepath.Join(tmpDir, "does-not-exist-123")); err != nil {
		t.Errorf("expected removeLower on non-existent path to return nil, got: %v", err)
	}
}

func TestApplyShadowChangesRemovesDirectoriesRecursively_Subprocess(t *testing.T) {
	if os.Getenv("GO_WANT_DIR_WHITEOUT_HELPER_PROCESS") == "1" {
		lowerDir := os.Getenv("SAFEBOX_TEST_LOWER")
		upperDir := os.Getenv("SAFEBOX_TEST_UPPER")
		workDir := os.Getenv("SAFEBOX_TEST_WORK")
		mergedDir := os.Getenv("SAFEBOX_TEST_MERGED")

		opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
		if err := syscall.Mount("overlay", mergedDir, "overlay", 0, opts); err != nil {
			os.Stderr.WriteString("mount error: " + err.Error() + "\n")
			os.Exit(1)
		}

		// Delete directory tree inside overlay
		if err := os.RemoveAll(filepath.Join(mergedDir, "tree_to_delete")); err != nil {
			os.Stderr.WriteString("remove tree error: " + err.Error() + "\n")
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
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}
	t.Cleanup(func() {
		_ = SafeRemoveAll(workDir)
	})

	// Initial tree in lower
	targetDir := filepath.Join(lowerDir, "tree_to_delete", "nested")
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		t.Fatalf("failed to create nested lower dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "file.txt"), []byte("nested content"), 0600); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestApplyShadowChangesRemovesDirectoriesRecursively_Subprocess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_DIR_WHITEOUT_HELPER_PROCESS=1",
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
		t.Fatalf("helper subprocess failed: %v, output: %s", err, string(out))
	}

	// Apply shadow changes
	if err := ApplyShadowChanges(lowerDir, upperDir); err != nil {
		t.Fatalf("ApplyShadowChanges failed: %v", err)
	}

	// Verify lowerDir tree_to_delete is completely gone
	if _, err := os.Stat(filepath.Join(lowerDir, "tree_to_delete")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected tree_to_delete to be completely removed from lowerDir, err: %v", err)
	}
}

func TestApplyShadowChangesNonExistentWhiteoutIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")

	if err := os.MkdirAll(lowerDir, 0700); err != nil {
		t.Fatalf("failed to create lowerDir: %v", err)
	}
	if err := os.MkdirAll(upperDir, 0700); err != nil {
		t.Fatalf("failed to create upperDir: %v", err)
	}

	// Empty changes or non-existent whiteout target apply cleanly
	if err := ApplyShadowChanges(lowerDir, upperDir); err != nil {
		t.Errorf("expected empty upperDir apply to return nil, got: %v", err)
	}
}

func TestRemoveLower_PermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root UID to validate DAC permission denial")
	}

	parentDir := t.TempDir()
	childFile := filepath.Join(parentDir, "child.txt")
	siblingFile := filepath.Join(parentDir, "sibling.txt")

	// Register cleanup BEFORE revoking permissions.
	t.Cleanup(func() {
		_ = os.Chmod(parentDir, 0755)
	})

	// Create child and sibling files.
	if err := os.WriteFile(childFile, []byte("child"), 0644); err != nil {
		t.Fatalf("write child: %v", err)
	}
	if err := os.WriteFile(siblingFile, []byte("sibling"), 0644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}

	// Revoke parent write permission.
	if err := os.Chmod(parentDir, 0555); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}

	// removeLower should fail (cannot unlink child without parent write).
	err := removeLower(childFile)
	if err == nil {
		t.Errorf("expected removeLower to fail on permission denial, got nil")
	}

	// Sibling must remain untouched.
	if _, err := os.Stat(siblingFile); err != nil {
		t.Errorf("sibling file was deleted by removeLower: %v", err)
	}
	// Child must remain untouched (no destructive escalation).
	if _, err := os.Stat(childFile); err != nil {
		t.Errorf("child file was deleted by removeLower: %v", err)
	}
}
