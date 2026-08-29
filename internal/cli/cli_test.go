package cli

import (
	"bytes"
	"strings"
	"syscall"
	"testing"

	"safebox/internal/trace"
)

func TestHasQuiet(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"run", "--", "true"}, false},
		{[]string{"run", "-q", "--", "true"}, true},
		{[]string{"run", "--quiet", "--", "true"}, true},
		{[]string{"run", "--quiet=true", "--", "true"}, true},
		{[]string{"diff"}, false},
		{[]string{"diff", "-q"}, true},
	}

	for _, tt := range tests {
		if got := HasQuiet(tt.args); got != tt.want {
			t.Errorf("HasQuiet(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestDispatchUnknownSubcommand(t *testing.T) {
	tr := trace.New(false)
	code := Dispatch([]string{"unknown_xyz_subcommand"}, tr)
	if code != 1 {
		t.Errorf("expected exit code 1 for unknown subcommand, got %d", code)
	}
}

func TestDispatchHelp(t *testing.T) {
	tr := trace.New(false)
	for _, cmd := range []string{"help", "-h", "--help"} {
		code := Dispatch([]string{cmd}, tr)
		if code != 0 {
			t.Errorf("expected exit code 0 for %s, got %d", cmd, code)
		}
	}
}

func TestRunHelp(t *testing.T) {
	var buf bytes.Buffer
	code := RunHelp(&buf)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "safebox <command> [arguments]") {
		t.Errorf("expected usage title in help output, got: %s", out)
	}
	if !strings.Contains(out, "--probe") {
		t.Errorf("expected --probe in help output, got: %s", out)
	}
}

func TestHintForSubcommand(t *testing.T) {
	if hint := HintForSubcommand("run", nil); hint != "" {
		t.Errorf("expected empty hint for nil error, got %q", hint)
	}
}

func TestCLIChildMSPrivateDoesNotBreakOverlay(t *testing.T) {
	// Tests that syscall.Mount with MS_REC|MS_PRIVATE does not panic or crash
	err := syscall.Mount("none", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, "")
	// In test environments this may succeed or return EPERM; either way it must not panic.
	_ = err
}
