package isolation

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestApplyLandlockSubprocess(t *testing.T) {
	if os.Getenv("TEST_LANDLOCK_CHILD") == "1" {
		if err := ApplyLandlock(); err != nil {
			os.Exit(2)
		}
		// Verify reading /root is denied specifically with EACCES
		_, err := os.ReadDir("/root")
		if err == nil {
			os.Exit(3)
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			if !errors.Is(pathErr.Err, syscall.EACCES) {
				os.Exit(5)
			}
		} else if !errors.Is(err, syscall.EACCES) {
			os.Exit(5)
		}

		// Verify writing to read-only system path /etc is denied specifically with EACCES
		err = os.WriteFile("/etc/.landlock_test_probe_fail", []byte("bad"), 0600)
		if err == nil {
			os.Remove("/etc/.landlock_test_probe_fail")
			os.Exit(6)
		}
		if errors.As(err, &pathErr) {
			if !errors.Is(pathErr.Err, syscall.EACCES) {
				os.Exit(7)
			}
		} else if !errors.Is(err, syscall.EACCES) {
			os.Exit(7)
		}

		// Verify writing in current working directory is allowed
		testFile := filepath.Join(".", ".landlock_test_probe")
		if err := os.WriteFile(testFile, []byte("ok"), 0600); err != nil {
			os.Exit(4)
		}
		os.Remove(testFile)
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestApplyLandlockSubprocess")
	cmd.Env = append(os.Environ(), "TEST_LANDLOCK_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("landlock subprocess test failed: %v, output: %s", err, string(out))
	}
}
