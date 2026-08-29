package isolation

import (
	"strings"
	"testing"
)

func TestErrLandlockDeniedHint(t *testing.T) {
	err := &ErrLandlockDenied{Path: "/home/user/.local/bin", Op: "execute"}
	if !strings.Contains(err.Error(), "/home/user/.local/bin") {
		t.Errorf("expected path in error string, got: %s", err.Error())
	}

	// Case 1: Denied path is binary's directory -> suggest --allow-path
	hint := err.Hint("/home/user/.local/bin/agy")
	expected := "rerun with --allow-path=/home/user/.local/bin"
	if hint != expected {
		t.Errorf("expected hint %q, got %q", expected, hint)
	}

	// Case 2: Denied path is state directory -> suggest --allow-path-rw
	errState := &ErrLandlockDenied{Path: "/home/user/.gemini", Op: "write"}
	hintState := errState.Hint("/home/user/.local/bin/agy")
	expectedState := "rerun with --allow-path-rw=/home/user/.gemini"
	if hintState != expectedState {
		t.Errorf("expected hint %q, got %q", expectedState, hintState)
	}
}

func TestErrExecNotFoundHint(t *testing.T) {
	err := &ErrExecNotFound{Bin: "/custom/path/to/binary"}
	if !strings.Contains(err.Error(), "/custom/path/to/binary") {
		t.Errorf("expected bin name in error string, got: %s", err.Error())
	}

	hint := err.Hint()
	expected := "rerun with --allow-path=/custom/path/to"
	if hint != expected {
		t.Errorf("expected hint %q, got %q", expected, hint)
	}

	// Plain binary name without slash has empty hint
	errPlain := &ErrExecNotFound{Bin: "custom_tool"}
	if errPlain.Hint() != "" {
		t.Errorf("expected empty hint for plain binary name, got: %q", errPlain.Hint())
	}
}

func TestErrExecDeniedHint(t *testing.T) {
	// Union hint when both binary dir and denied state dir exist
	err := &ErrExecDenied{Bin: "/home/user/.local/bin/tool", Path: "/home/user/.gemini"}
	if !strings.Contains(err.Error(), "blocked by landlock") {
		t.Errorf("expected blocked error message, got: %s", err.Error())
	}

	hint := err.Hint()
	expected := "rerun with --allow-path=/home/user/.local/bin --allow-path-rw=/home/user/.gemini"
	if hint != expected {
		t.Errorf("expected hint %q, got %q", expected, hint)
	}

	// Single hint when only binary dir is relevant
	errBinOnly := &ErrExecDenied{Bin: "/home/user/.local/bin/tool"}
	hintBinOnly := errBinOnly.Hint()
	expectedBinOnly := "rerun with --allow-path=/home/user/.local/bin"
	if hintBinOnly != expectedBinOnly {
		t.Errorf("expected hint %q, got %q", expectedBinOnly, hintBinOnly)
	}
}
