package isolation

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLandlockSubprocess(t *testing.T) {
	if os.Getenv("TEST_LANDLOCK_CHILD") == "1" {
		if err := ApplyLandlock(); err != nil {
			os.Exit(2)
		}
		// Verify reading /root is denied (EACCES)
		_, err := os.ReadDir("/root")
		if err == nil {
			os.Exit(3)
		}
		// Verify writing in current working directory is allowed
		testFile := filepath.Join(".", ".landlock_test_probe")
		if err := os.WriteFile(testFile, []byte("ok"), 0600); err != nil {
			os.Exit(4)
		}
		os.Remove(testFile)
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestLandlockSubprocess")
	cmd.Env = append(os.Environ(), "TEST_LANDLOCK_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("landlock subprocess test failed: %v, output: %s", err, string(out))
	}
}
