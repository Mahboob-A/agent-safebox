package exec

import (
	"testing"
)

func TestRunEmptyArgs(t *testing.T) {
	err := Run([]string{})
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}

func TestRunNonExistentBinary(t *testing.T) {
	err := Run([]string{"non_existent_binary_12345_xyz"})
	if err == nil {
		t.Fatal("expected error for non-existent binary, got nil")
	}
}
