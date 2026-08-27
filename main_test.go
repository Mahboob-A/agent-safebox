package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIRunEmptyArgs(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "run")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for empty run, got success with output: %s", string(out))
	}
	if !strings.Contains(string(out), "no command specified") {
		t.Errorf("expected 'no command specified' in output, got: %s", string(out))
	}
}

func TestCLIRunUIDMapping(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "run", "--", "id", "-u")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run id -u failed: %v, output: %s", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "0" {
		t.Errorf("expected container UID 0, got %s", strings.TrimSpace(string(out)))
	}
}

func TestCLIRunNetworkIsolation(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "run", "--", "ping", "-c", "1", "-W", "1", "8.8.8.8")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected network ping to fail, got success: %s", string(out))
	}
}

func TestCLIRunLandlockDenial(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "run", "--", "ls", "/root")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected ls /root to fail, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "Permission denied") {
		t.Errorf("expected 'Permission denied' in output, got: %s", string(out))
	}
}

func TestCLIHelpCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help command failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Usage: safebox") {
		t.Errorf("expected usage header in help output, got: %s", string(out))
	}
}
