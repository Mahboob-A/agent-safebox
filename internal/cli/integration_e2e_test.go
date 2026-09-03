//go:build integration
// +build integration

package cli_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestE2E_AllowNet_NoHostMutation is the SHELL-LEVEL regression guard for the
// v0.4 P0 host-corruption incident. It runs `safebox run --allow-net=* -- true`
// and asserts the host's /etc/hosts and /etc/resolv.conf are byte-identical
// before and after. Tolerates exit 0 (clean run), exit 1 (slirp4netns/pasta
// spawn failure in non-CLONE_NEWNET env), or exit 4 (no backend available).
// Any other exit code is a regression.
func TestE2E_AllowNet_NoHostMutation(t *testing.T) {
	bin := integrationSafeboxBin(t)

	hostsBefore, ok := integrationSha256File("/etc/hosts")
	if !ok {
		t.Skip("cannot read host /etc/hosts; skipping E2E test")
	}
	resolvBefore, ok := integrationSha256File("/etc/resolv.conf")
	if !ok {
		t.Skip("cannot read host /etc/resolv.conf; skipping E2E test")
	}

	cmd := exec.Command(bin, "run", "--allow-net=*", "--", "true")
	out, err := cmd.CombinedOutput()
	rc := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		}
	}
	if rc != 0 && rc != 1 && rc != 4 {
		t.Fatalf("safebox run --allow-net=* -- true exited %d (expected 0, 1=spawn-failure, or 4=no-backend); output: %s", rc, out)
	}

	hostsAfter, _ := integrationSha256File("/etc/hosts")
	resolvAfter, _ := integrationSha256File("/etc/resolv.conf")
	if hostsBefore != hostsAfter {
		t.Fatalf("HOST CORRUPTION: /etc/hosts mutated (E2E P0 regression)\nbefore sha256: %s\nafter  sha256: %s", hostsBefore, hostsAfter)
	}
	if resolvBefore != resolvAfter {
		t.Fatalf("HOST CORRUPTION: /etc/resolv.conf mutated (E2E P0 regression)\nbefore sha256: %s\nafter  sha256: %s", resolvBefore, resolvAfter)
	}
}

// TestE2E_AllowNet_DNSResolution is the HEADLINE v1-feature regression test.
// It runs `safebox run --allow-net=* -- sh -c 'getent hosts github.com ||
// echo offline'` and asserts the wrapper returns cleanly (exit 0/1/4) AND
// produced non-empty output. DNS may legitimately fail if outbound network is
// unavailable; the assertion is on the wrapper side and shadow-file visibility,
// not on DNS success in offline CI environments.
func TestE2E_AllowNet_DNSResolution(t *testing.T) {
	bin := integrationSafeboxBin(t)

	cmd := exec.Command(bin, "run", "--allow-net=*", "--", "sh", "-c",
		"getent hosts github.com || echo offline")
	out, err := cmd.CombinedOutput()
	rc := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		}
	}
	if rc != 0 && rc != 1 && rc != 4 {
		t.Fatalf("unexpected exit code %d: %s", rc, out)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		t.Fatalf("expected getent output (IP or 'offline'), got empty: rc=%d", rc)
	}
}

// TestE2E_AllowNet_ShadowFilesVisible asserts that the synthetic
// /etc/resolv.conf and /etc/hosts are visible inside the sandbox (via
// bind-mount) rather than the host's real files. Combined with
// TestE2E_AllowNet_NoHostMutation's host-content-hash invariant, this proves
// the bind-mount path is exercised end-to-end without leaking host writes.
//
// We assert the synthetic resolv.conf contains "nameserver" and the synthetic
// hosts contains the safebox egress marker. Real host files must remain
// byte-identical before/after.
func TestE2E_AllowNet_ShadowFilesVisible(t *testing.T) {
	bin := integrationSafeboxBin(t)

	hostsBefore, ok := integrationSha256File("/etc/hosts")
	if !ok {
		t.Skip("cannot read host /etc/hosts; skipping E2E test")
	}
	resolvBefore, ok := integrationSha256File("/etc/resolv.conf")
	if !ok {
		t.Skip("cannot read host /etc/resolv.conf; skipping E2E test")
	}

	cmd := exec.Command(bin, "run", "--allow-net=*", "--", "sh", "-c",
		"echo ---RESOLV---; cat /etc/resolv.conf; echo ---HOSTS---; cat /etc/hosts")
	out, err := cmd.CombinedOutput()
	rc := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		}
	}
	if rc != 0 && rc != 1 && rc != 4 {
		t.Fatalf("unexpected exit code %d: %s", rc, out)
	}
	if !bytes.Contains(out, []byte("nameserver")) {
		t.Errorf("child did not see synthetic /etc/resolv.conf (no 'nameserver' line in output):\n%s", out)
	}
	if !bytes.Contains(out, []byte("safebox egress")) {
		t.Errorf("child did not see synthetic /etc/hosts (no 'safebox egress' marker in output):\n%s", out)
	}

	hostsAfter, _ := integrationSha256File("/etc/hosts")
	resolvAfter, _ := integrationSha256File("/etc/resolv.conf")
	if hostsBefore != hostsAfter {
		t.Fatalf("HOST CORRUPTION: /etc/hosts mutated (E2E P0 regression)\nbefore sha256: %s\nafter  sha256: %s", hostsBefore, hostsAfter)
	}
	if resolvBefore != resolvAfter {
		t.Fatalf("HOST CORRUPTION: /etc/resolv.conf mutated (E2E P0 regression)\nbefore sha256: %s\nafter  sha256: %s", resolvBefore, resolvAfter)
	}
}

func integrationSafeboxBin(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("SAFEBOX_BIN"); v != "" {
		return v
	}
	// Always prefer the freshly-built safebox binary in the working directory
	// over whatever may be on PATH. This avoids using a stale system-wide
	// binary that lacks the v0.4 allowNet plumbing.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	local := wd + "/safebox"
	if _, err := os.Stat(local); err == nil {
		return local
	}
	// Walk up parent dirs looking for a safebox binary (in case go test
	// changes the cwd).
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "safebox")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if filepath.Base(dir) == "safebox" {
			return candidate
		}
	}
	p, err := exec.LookPath("safebox")
	if err != nil {
		t.Skipf("safebox binary not found in repo or on PATH (set SAFEBOX_BIN to override): %v", err)
	}
	t.Logf("warning: using safebox from PATH (%s); verify this is the freshly-built binary, not a stale system-wide install", p)
	return p
}

func integrationSha256File(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), true
}
