package revert

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("SAFEBOX_SESSION_ROOT", tmpRoot)

	workDir := t.TempDir()

	// 1. Create a session
	sess, err := CreateSession(workDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.LowerDir != workDir {
		t.Fatalf("expected lower dir %s, got %s", workDir, sess.LowerDir)
	}

	// Verify upper, work, merged dirs exist
	for _, dir := range []string{sess.UpperDir, sess.WorkDir, sess.MergedDir} {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Fatalf("expected directory %s to exist, err: %v", dir, err)
		}
	}

	// 2. Load session
	loaded, err := LoadSession(sess.BaseDir)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if loaded.ID != sess.ID {
		t.Fatalf("expected loaded ID %s, got %s", sess.ID, loaded.ID)
	}

	// 3. Find most recent session
	found, err := MostRecentSession(workDir)
	if err != nil {
		t.Fatalf("failed to find most recent session: %v", err)
	}
	if found.ID != sess.ID {
		t.Fatalf("expected found session ID %s, got %s", sess.ID, found.ID)
	}

	// Also find from subdirectory of workDir
	subDir := filepath.Join(workDir, "subdir")
	if err := os.MkdirAll(subDir, 0700); err != nil {
		t.Fatalf("failed to create subDir: %v", err)
	}
	foundSub, err := MostRecentSession(subDir)
	if err != nil {
		t.Fatalf("failed to find most recent session from subdir: %v", err)
	}
	if foundSub.ID != sess.ID {
		t.Fatalf("expected found sub session ID %s, got %s", sess.ID, foundSub.ID)
	}

	// 4. Create newer session and verify it is returned as most recent
	time.Sleep(10 * time.Millisecond)
	sess2, err := CreateSession(workDir)
	if err != nil {
		t.Fatalf("failed to create second session: %v", err)
	}
	found2, err := MostRecentSession(workDir)
	if err != nil {
		t.Fatalf("failed to find most recent session after second create: %v", err)
	}
	if found2.ID != sess2.ID {
		t.Fatalf("expected latest session ID %s, got %s", sess2.ID, found2.ID)
	}

	// 5. Discard session
	if err := DiscardSession(sess2); err != nil {
		t.Fatalf("failed to discard session: %v", err)
	}
	if _, err := os.Stat(sess2.BaseDir); !os.IsNotExist(err) {
		t.Fatalf("expected session directory %s to be removed", sess2.BaseDir)
	}

	// Now sess1 is the most recent
	found1, err := MostRecentSession(workDir)
	if err != nil {
		t.Fatalf("failed to find session after discarding newer session: %v", err)
	}
	if found1.ID != sess.ID {
		t.Fatalf("expected session ID %s, got %s", sess.ID, found1.ID)
	}
}

func TestMostRecentSessionNotFound(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("SAFEBOX_SESSION_ROOT", tmpRoot)

	otherDir := t.TempDir()
	_, err := MostRecentSession(otherDir)
	if err == nil || err != ErrNoSessionFound {
		t.Fatalf("expected ErrNoSessionFound, got: %v", err)
	}
}
