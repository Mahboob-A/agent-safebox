package cli

import (
	"os"
	"path/filepath"
	"testing"

	"safebox/internal/trace"
)

func TestRunCat_Help(t *testing.T) {
	tr := trace.New(false)
	code := RunCat([]string{"--help"}, tr)
	if code != 0 {
		t.Errorf("expected 0 for --help, got %d", code)
	}

	codeShort := RunCat([]string{"-h"}, tr)
	if codeShort != 0 {
		t.Errorf("expected 0 for -h, got %d", codeShort)
	}
}

func TestRunCat_NoArgs(t *testing.T) {
	tr := trace.New(false)
	code := RunCat([]string{}, tr)
	if code != 1 {
		t.Errorf("expected 1 for no args, got %d", code)
	}
}

func TestRunCat_UnknownFlag(t *testing.T) {
	tr := trace.New(false)
	code := RunCat([]string{"--unknown"}, tr)
	if code != 2 {
		t.Errorf("expected 2 for unknown flag, got %d", code)
	}
}

func TestRunCat_HostFile(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	testFile := "sample.txt"
	expectedContent := "hello safebox cat\n"
	if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte(expectedContent), 0644); err != nil {
		t.Fatal(err)
	}

	tr := trace.New(false)
	code := RunCat([]string{testFile}, tr)
	if code != 0 {
		t.Errorf("expected 0 for existing host file, got %d", code)
	}
}

func TestRunCat_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	tr := trace.New(false)
	code := RunCat([]string{"non-existent.txt"}, tr)
	if code != 1 {
		t.Errorf("expected 1 for non-existent file, got %d", code)
	}
}
