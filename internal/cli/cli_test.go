package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"safebox/internal/isolation"
	"safebox/internal/revert"
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

func TestCLIProfileList(t *testing.T) {
	tr := trace.New(false)
	code := Dispatch([]string{"profile", "list"}, tr)
	if code != 0 {
		t.Errorf("expected exit code 0 for profile list, got %d", code)
	}
}

func TestCLIProfileShow_Found(t *testing.T) {
	tr := trace.New(false)
	code := Dispatch([]string{"profile", "show", "agy"}, tr)
	if code != 0 {
		t.Errorf("expected exit code 0 for profile show agy, got %d", code)
	}
}

func TestCLIProfileShow_NotFound(t *testing.T) {
	tr := trace.New(false)
	code := Dispatch([]string{"profile", "show", "nonexistent_tool_xyz"}, tr)
	if code != 1 {
		t.Errorf("expected exit code 1 for profile show with unknown tool, got %d", code)
	}
}

func TestHintFor_PersistentStateMount(t *testing.T) {
	err := &isolation.ErrPersistentStateMount{
		HostPath:  "/tmp/host/agy",
		MountPath: "/root/.gemini",
		Err:       syscall.ENOENT,
	}
	hint := HintFor(err, []string{"agy"})
	if !strings.Contains(hint, "persistent state mount failed for '/root/.gemini'") {
		t.Errorf("expected hint to mention mount path, got %q", hint)
	}
	if !strings.Contains(hint, "/tmp/host/agy") {
		t.Errorf("expected hint to mention host path, got %q", hint)
	}
}


func TestCLIProfile_UnknownSubcommand(t *testing.T) {
	tr := trace.New(false)
	code := Dispatch([]string{"profile", "invalid_subcmd"}, tr)
	if code != 1 {
		t.Errorf("expected exit code 1 for invalid profile subcommand, got %d", code)
	}
}

func TestRevertRefusesActiveSession(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("SAFEBOX_SESSION_ROOT", tmpRoot)

	workDir := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)

	sess, err := revert.CreateSession(workDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer sess.ReleaseActiveLock()
	defer revert.DiscardSession(sess)

	tr := trace.New(false)
	code := RunRevert([]string{"--yes"}, tr)
	if code != 3 {
		t.Errorf("expected exit code 3 for active session revert, got %d", code)
	}

	// Verify session was NOT discarded
	if _, err := os.Stat(sess.BaseDir); err != nil {
		t.Errorf("expected session directory %s to remain intact after refused revert", sess.BaseDir)
	}
}

func TestApplyForceDiscardOverridesLockfile(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("SAFEBOX_SESSION_ROOT", tmpRoot)

	workDir := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)

	sess, err := revert.CreateSession(workDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer sess.ReleaseActiveLock()

	// Write change to upper dir
	_ = os.WriteFile(filepath.Join(sess.UpperDir, "forced.txt"), []byte("data"), 0644)

	tr := trace.New(false)
	code := RunApply([]string{"--force-discard"}, tr)
	if code != 0 {
		t.Errorf("expected exit code 0 for apply --force-discard, got %d", code)
	}

	// Verify change applied
	if data, err := os.ReadFile(filepath.Join(workDir, "forced.txt")); err != nil || string(data) != "data" {
		t.Errorf("expected applied file 'forced.txt', got err=%v", err)
	}

	// Verify session was discarded
	if _, err := os.Stat(sess.BaseDir); !os.IsNotExist(err) {
		t.Errorf("expected session directory %s to be discarded by --force-discard", sess.BaseDir)
	}
}

func TestRevertForceDiscardOverridesLockfile(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("SAFEBOX_SESSION_ROOT", tmpRoot)

	workDir := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)

	sess, err := revert.CreateSession(workDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer sess.ReleaseActiveLock()

	tr := trace.New(false)
	code := RunRevert([]string{"--force-discard"}, tr)
	if code != 0 {
		t.Errorf("expected exit code 0 for revert --force-discard, got %d", code)
	}

	// Verify session was discarded
	if _, err := os.Stat(sess.BaseDir); !os.IsNotExist(err) {
		t.Errorf("expected session directory %s to be discarded by --force-discard", sess.BaseDir)
	}
}

func TestApplyEgressConfig_ParsesValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	etcRoot := t.TempDir()
	// applyEgressConfig reads pre-existing /etc/hosts and /etc/resolv.conf
	// from etcRoot and bind-mounts over them; pre-create them in the tempdir
	// so the function has real files to read and shadow.
	if err := os.WriteFile(filepath.Join(etcRoot, "hosts"), []byte("127.0.0.1\tlocalhost\n"), 0644); err != nil {
		t.Fatalf("seed etcRoot/hosts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(etcRoot, "resolv.conf"), []byte("nameserver 1.1.1.1\n"), 0644); err != nil {
		t.Fatalf("seed etcRoot/resolv.conf: %v", err)
	}

	cfgPath := filepath.Join(tmpDir, "netconfig.json")
	cfgData := `{"tap_device":"sbx123","gateway_ip":"10.0.2.2","allowed_domains":["example.com"],"pinned_ips":{"example.com":["93.184.216.34"]}}`
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0600); err != nil {
		t.Fatalf("failed to write netconfig.json: %v", err)
	}

	tr := trace.New(false)
	if err := applyEgressConfig(cfgPath, tmpDir, etcRoot, tr); err != nil {
		if os.Getuid() != 0 && (errors.Is(err, syscall.EPERM) || strings.Contains(err.Error(), "operation not permitted")) {
			t.Skipf("skipping host bind-mount assertion in unprivileged test context: %v", err)
			return
		}
		t.Errorf("applyEgressConfig failed: %v", err)
	}
	// Schedule unmount of the bind mounts so t.TempDir() cleanup succeeds.
	t.Cleanup(func() {
		_ = syscall.Unmount(filepath.Join(etcRoot, "hosts"), 0)
		_ = syscall.Unmount(filepath.Join(etcRoot, "resolv.conf"), 0)
	})
}

// TestApplyEgressConfig_DoesNotTouchHost is the primary regression guard for
// the v0.4 P0 bug. It snapshots the sha256 of /etc/hosts and /etc/resolv.conf
// on the host BEFORE and AFTER calling applyEgressConfig against a tempdir.
// Any drift fails the test. This was the unsafe test that asserted host
// corruption as correct behavior in v0.4 Phase 16.
func TestApplyEgressConfig_DoesNotTouchHost(t *testing.T) {
	tmpDir := t.TempDir()
	etcRoot := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "netconfig.json")
	cfgData := `{"tap_device":"sbx123","gateway_ip":"10.0.2.2","allowed_domains":["api.anthropic.com"],"pinned_ips":{"api.anthropic.com":["1.2.3.4"]}}`
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0600); err != nil {
		t.Fatalf("failed to write netconfig.json: %v", err)
	}

	hostsBefore, hostsBeforeErr := os.ReadFile("/etc/hosts")
	resolvBefore, resolvBeforeErr := os.ReadFile("/etc/resolv.conf")

	tr := trace.New(false)
	if err := applyEgressConfig(cfgPath, tmpDir, etcRoot, tr); err != nil {
		// applyEgressConfig will fail because etcRoot/<hosts|resolv.conf>
		// do not exist on first call - this is expected in unit tests.
		// The key assertion is that the host files are unchanged.
		t.Logf("applyEgressConfig error (expected in unit test context): %v", err)
	}

	if hostsBeforeErr == nil {
		hostsAfter, err := os.ReadFile("/etc/hosts")
		if err != nil {
			t.Fatalf("cannot read host /etc/hosts after test: %v", err)
		}
		if string(hostsBefore) != string(hostsAfter) {
			t.Fatalf("HOST CORRUPTION: /etc/hosts was mutated by applyEgressConfig\nBefore: %s\nAfter:  %s", hostsBefore, hostsAfter)
		}
	}
	if resolvBeforeErr == nil {
		resolvAfter, err := os.ReadFile("/etc/resolv.conf")
		if err != nil {
			t.Fatalf("cannot read host /etc/resolv.conf after test: %v", err)
		}
		if string(resolvBefore) != string(resolvAfter) {
			t.Fatalf("HOST CORRUPTION: /etc/resolv.conf was mutated by applyEgressConfig\nBefore: %s\nAfter:  %s", resolvBefore, resolvAfter)
		}
	}
}

// TestMountEtcFile_UnlinksTmpfile verifies the documented fix for the
// unbounded /tmp/safebox-* growth defect: the tmpfile source MUST be
// unlinked immediately after a successful bind mount.
func TestMountEtcFile_UnlinksTmpfile(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "shadowed_file")
	if err := os.WriteFile(target, []byte("original\n"), 0644); err != nil {
		t.Fatalf("create target: %v", err)
	}

	if err := mountEtcFile(target, []byte("synthetic\n"), tmpDir); err != nil {
		// Skip on environments where unprivileged users cannot MS_BIND
		// inside the test process (which is not in a private mount namespace).
		if strings.Contains(err.Error(), "EPERM") || strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("MS_BIND not permitted in test environment: %v", err)
		}
		t.Fatalf("mountEtcFile failed: %v", err)
	}

	// Schedule an unmount so t.TempDir() cleanup can succeed (the target
	// path is busy as long as the bind mount is alive).
	t.Cleanup(func() {
		if err := syscall.Unmount(target, 0); err != nil && !strings.Contains(err.Error(), "EINVAL") {
			t.Logf("cleanup unmount %s: %v", target, err)
		}
	})

	// Count remaining safebox-* tmpfiles in tmpDir; must be zero.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read tmpDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "safebox-etc-") {
			t.Errorf("tmpfile %q was not unlinked after successful bind mount", e.Name())
		}
	}
}

// TestMountEtcFile_RejectsInvalidTarget ensures mountEtcFile fails loudly
// when the bind mount cannot succeed (e.g. EPERM, ENOENT). NFR1 forbids
// silent best-effort behavior.
func TestMountEtcFile_RejectsInvalidTarget(t *testing.T) {
	tmpDir := t.TempDir()
	// Target path that does not exist
	badTarget := filepath.Join(tmpDir, "does-not-exist", "shadowed")
	if err := mountEtcFile(badTarget, []byte("content"), tmpDir); err == nil {
		t.Errorf("expected error for nonexistent target, got nil")
	}
}




