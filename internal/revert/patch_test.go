package revert

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatFilePatch_Added(t *testing.T) {
	patch := FormatFilePatch("hello.go", nil, []byte("package main\n\nfunc main() {}\n"), ChangeAdded)
	if !strings.Contains(patch, "diff --git a/hello.go b/hello.go") {
		t.Errorf("expected diff --git header, got: %s", patch)
	}
	if !strings.Contains(patch, "new file mode 100644") {
		t.Errorf("expected new file mode, got: %s", patch)
	}
	if !strings.Contains(patch, "--- /dev/null") {
		t.Errorf("expected --- /dev/null, got: %s", patch)
	}
	if !strings.Contains(patch, "+++ b/hello.go") {
		t.Errorf("expected +++ b/hello.go, got: %s", patch)
	}
	if !strings.Contains(patch, "+package main") {
		t.Errorf("expected +package main, got: %s", patch)
	}
}

func TestFormatFilePatch_Deleted(t *testing.T) {
	patch := FormatFilePatch("old.txt", []byte("line1\nline2\n"), nil, ChangeDeleted)
	if !strings.Contains(patch, "diff --git a/old.txt b/old.txt") {
		t.Errorf("expected diff --git header, got: %s", patch)
	}
	if !strings.Contains(patch, "deleted file mode 100644") {
		t.Errorf("expected deleted file mode, got: %s", patch)
	}
	if !strings.Contains(patch, "--- a/old.txt") {
		t.Errorf("expected --- a/old.txt, got: %s", patch)
	}
	if !strings.Contains(patch, "+++ /dev/null") {
		t.Errorf("expected +++ /dev/null, got: %s", patch)
	}
	if !strings.Contains(patch, "-line1") {
		t.Errorf("expected -line1, got: %s", patch)
	}
}

func TestFormatFilePatch_Modified(t *testing.T) {
	oldBytes := []byte("line1\nline2\nline3\n")
	newBytes := []byte("line1\nline2_modified\nline3\n")
	patch := FormatFilePatch("code.py", oldBytes, newBytes, ChangeModified)

	if !strings.Contains(patch, "--- a/code.py") {
		t.Errorf("expected --- a/code.py, got: %s", patch)
	}
	if !strings.Contains(patch, "+++ b/code.py") {
		t.Errorf("expected +++ b/code.py, got: %s", patch)
	}
	if !strings.Contains(patch, "-line2") {
		t.Errorf("expected -line2, got: %s", patch)
	}
	if !strings.Contains(patch, "+line2_modified") {
		t.Errorf("expected +line2_modified, got: %s", patch)
	}
}

func TestFormatFilePatch_Binary(t *testing.T) {
	binBytes := []byte{0x00, 0x01, 0x02}
	patch := FormatFilePatch("image.png", nil, binBytes, ChangeAdded)
	if !strings.Contains(patch, "Binary files /dev/null and b/image.png differ") {
		t.Errorf("expected binary differ message, got: %s", patch)
	}
}

func TestRunShadowPatch_Integration(t *testing.T) {
	lowerDir := t.TempDir()
	upperDir := t.TempDir()

	// 1. Clean test
	var buf bytes.Buffer
	if err := RunShadowPatch(lowerDir, upperDir, &buf); err != nil {
		t.Fatalf("RunShadowPatch clean failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Working tree is clean") {
		t.Errorf("expected clean message, got: %s", buf.String())
	}

	// 2. Add file
	buf.Reset()
	if err := os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RunShadowPatch(lowerDir, upperDir, &buf); err != nil {
		t.Fatalf("RunShadowPatch added failed: %v", err)
	}
	if !strings.Contains(buf.String(), "+++ b/new.txt") || !strings.Contains(buf.String(), "+hello") {
		t.Errorf("expected patch for new.txt, got: %s", buf.String())
	}

	// 3. Path filter matching
	buf.Reset()
	if err := RunShadowPatch(lowerDir, upperDir, &buf, "new.txt"); err != nil {
		t.Fatalf("RunShadowPatch filter matching failed: %v", err)
	}
	if !strings.Contains(buf.String(), "+++ b/new.txt") {
		t.Errorf("expected patch for new.txt with filter, got: %s", buf.String())
	}

	// 4. Path filter non-matching
	buf.Reset()
	if err := RunShadowPatch(lowerDir, upperDir, &buf, "other.txt"); err != nil {
		t.Fatalf("RunShadowPatch filter non-matching failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Working tree is clean") {
		t.Errorf("expected clean message for non-matching filter, got: %s", buf.String())
	}
}
