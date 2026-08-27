package isolation

import (
	"os"
	"syscall"
	"testing"
)

func TestDefaultSysProcAttr(t *testing.T) {
	attr := DefaultSysProcAttr()
	if attr == nil {
		t.Fatal("expected non-nil SysProcAttr")
	}

	expectedFlags := uintptr(syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET)
	if attr.Cloneflags != expectedFlags {
		t.Fatalf("expected Cloneflags == %v, got %v", expectedFlags, attr.Cloneflags)
	}

	if len(attr.UidMappings) != 1 {
		t.Fatalf("expected 1 UidMapping, got %d", len(attr.UidMappings))
	}
	if attr.UidMappings[0].ContainerID != 0 || attr.UidMappings[0].HostID != os.Getuid() || attr.UidMappings[0].Size != 1 {
		t.Fatalf("unexpected UidMapping: %+v", attr.UidMappings[0])
	}

	if len(attr.GidMappings) != 1 {
		t.Fatalf("expected 1 GidMapping, got %d", len(attr.GidMappings))
	}
	if attr.GidMappings[0].ContainerID != 0 || attr.GidMappings[0].HostID != os.Getgid() || attr.GidMappings[0].Size != 1 {
		t.Fatalf("unexpected GidMapping: %+v", attr.GidMappings[0])
	}
}
