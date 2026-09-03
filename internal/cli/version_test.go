package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	v := FormatVersion()
	if !strings.HasPrefix(v, "safebox v") {
		t.Errorf("expected version to start with 'safebox v', got: %s", v)
	}
	if !strings.Contains(v, "linux/") && !strings.Contains(v, "darwin/") {
		t.Errorf("expected OS/Arch in version string, got: %s", v)
	}
}

func TestRunVersion(t *testing.T) {
	var buf bytes.Buffer
	code := RunVersion(&buf)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "safebox v0.5.0") {
		t.Errorf("expected output to contain 'safebox v0.5.0', got: %s", out)
	}
}
