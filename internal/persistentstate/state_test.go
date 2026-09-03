package persistentstate

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDefaultRoot_UniformXDG(t *testing.T) {
	t.Run("XDG_STATE_HOME set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_STATE_HOME", tmpDir)
		root, err := DefaultRoot()
		if err != nil {
			t.Fatalf("DefaultRoot failed: %v", err)
		}
		expected := filepath.Join(tmpDir, "safebox", "agents")
		if root != expected {
			t.Errorf("expected %q, got %q", expected, root)
		}
	})

	t.Run("XDG_STATE_HOME empty fallback", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir failed: %v", err)
		}
		root, err := DefaultRoot()
		if err != nil {
			t.Fatalf("DefaultRoot failed: %v", err)
		}
		expected := filepath.Join(home, ".local", "share", "safebox", "agents")
		if root != expected {
			t.Errorf("expected %q, got %q", expected, root)
		}
	})
}

func TestDefaultRoot_NonRootEnv(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	got, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := "/tmp/fakehome/.local/share/safebox/agents"
	if got != want {
		t.Errorf("DefaultRoot() = %q, want %q", got, want)
	}
}

func TestEnsure_CreatesDirectoryWith0700(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpDir)

	path, err := Ensure("test-agent")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "safebox", "agents", "test-agent")
	if path != expectedPath {
		t.Errorf("expected %q, got %q", expectedPath, path)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected directory at %s", path)
	}
	if perm := fi.Mode().Perm(); perm != 0700 {
		t.Errorf("expected 0700 permissions, got %o", perm)
	}

	// Verify idempotency and permission correction if existing with 0755
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	path2, err := Ensure("test-agent")
	if err != nil {
		t.Fatalf("Ensure failed on second call: %v", err)
	}
	if path2 != expectedPath {
		t.Errorf("expected %q, got %q", expectedPath, path2)
	}
	fi2, err := os.Stat(path2)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := fi2.Mode().Perm(); perm != 0700 {
		t.Errorf("expected 0700 permissions after ensure correction, got %o", perm)
	}
}

func TestEnsure_CrossToolIsolation(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)
	t.Setenv("HOME", base)

	agyPath, err := Ensure("agy")
	if err != nil {
		t.Fatal(err)
	}
	claudePath, err := Ensure("claude")
	if err != nil {
		t.Fatal(err)
	}

	expectedAgy := filepath.Join(base, "safebox", "agents", "agy")
	expectedClaude := filepath.Join(base, "safebox", "agents", "claude")

	if agyPath != expectedAgy {
		t.Errorf("expected agy path %q, got %q", expectedAgy, agyPath)
	}
	if claudePath != expectedClaude {
		t.Errorf("expected claude path %q, got %q", expectedClaude, claudePath)
	}
	if agyPath == claudePath {
		t.Fatal("tools share state directory")
	}

	agyFi, err := os.Stat(agyPath)
	if err != nil {
		t.Fatal(err)
	}
	claudeFi, err := os.Stat(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if agyFi.Mode().Perm() != 0700 {
		t.Errorf("agy mode %o, want 0700", agyFi.Mode().Perm())
	}
	if claudeFi.Mode().Perm() != 0700 {
		t.Errorf("claude mode %o, want 0700", claudeFi.Mode().Perm())
	}
}

func TestBindMount_SymlinkRejected(t *testing.T) {
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real")
	if err := os.Mkdir(realDir, 0700); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(tmp, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	dstDir := filepath.Join(tmp, "dst")
	if err := BindMount(linkDir, dstDir); err == nil {
		t.Errorf("BindMount accepted symlink source %q", linkDir)
	}
}

func TestBindMount_Directory(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	if err := os.Mkdir(srcDir, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(srcDir, "token.txt")
	if err := os.WriteFile(testFile, []byte("secret-token"), 0600); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(tmp, "dst")

	err := BindMount(srcDir, dstDir)
	if err != nil {
		t.Skipf("skipping BindMount test: %v (requires mount permissions/namespace)", err)
		return
	}
	defer syscall.Unmount(dstDir, 0)

	mountedFile := filepath.Join(dstDir, "token.txt")
	data, err := os.ReadFile(mountedFile)
	if err != nil {
		t.Fatalf("failed to read mounted file: %v", err)
	}
	if string(data) != "secret-token" {
		t.Errorf("expected %q, got %q", "secret-token", string(data))
	}
}

func TestBindMount_SingleFile(t *testing.T) {
	tmp := t.TempDir()
	srcFile := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(srcFile, []byte(`{"key":"value"}`), 0600); err != nil {
		t.Fatal(err)
	}

	dstFile := filepath.Join(tmp, "target_dir", "config.json")

	err := BindMount(srcFile, dstFile)
	if err != nil {
		t.Skipf("skipping BindMount single file test: %v (requires mount permissions/namespace)", err)
		return
	}
	defer syscall.Unmount(dstFile, 0)

	data, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read mounted file: %v", err)
	}
	if string(data) != `{"key":"value"}` {
		t.Errorf("expected %q, got %q", `{"key":"value"}`, string(data))
	}
}
