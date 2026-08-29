package cli

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParseRunFlagsStandard(t *testing.T) {
	args := []string{"--allow-path=/usr/local/bin", "--allow-path-rw=/root/.gemini", "--quiet", "--", "echo", "hello"}
	flags, cmdArgs, err := ParseRunFlags(args)
	if err != nil {
		t.Fatalf("ParseRunFlags failed: %v", err)
	}

	if !flags.Quiet {
		t.Error("expected Quiet to be true")
	}
	if !reflect.DeepEqual(flags.AllowPathsRO, []string{"/usr/local/bin"}) {
		t.Errorf("expected AllowPathsRO %v, got %v", []string{"/usr/local/bin"}, flags.AllowPathsRO)
	}
	if !reflect.DeepEqual(flags.AllowPathsRW, []string{"/root/.gemini"}) {
		t.Errorf("expected AllowPathsRW %v, got %v", []string{"/root/.gemini"}, flags.AllowPathsRW)
	}
	if !reflect.DeepEqual(cmdArgs, []string{"echo", "hello"}) {
		t.Errorf("expected cmdArgs %v, got %v", []string{"echo", "hello"}, cmdArgs)
	}
}

func TestParseRunFlagsSeparateArgForm(t *testing.T) {
	args := []string{"--allow-path", "/usr/bin", "--allow-path-rw", "/tmp/state", "--session-dir", "/tmp/sess", "-q", "--probe", "--", "python3", "app.py"}
	flags, cmdArgs, err := ParseRunFlags(args)
	if err != nil {
		t.Fatalf("ParseRunFlags failed: %v", err)
	}

	if !flags.Quiet || !flags.Probe {
		t.Errorf("expected Quiet=true and Probe=true, got Quiet=%v Probe=%v", flags.Quiet, flags.Probe)
	}
	if flags.SessionDir != "/tmp/sess" {
		t.Errorf("expected SessionDir %q, got %q", "/tmp/sess", flags.SessionDir)
	}
	if !reflect.DeepEqual(flags.AllowPathsRO, []string{"/usr/bin"}) {
		t.Errorf("expected AllowPathsRO %v, got %v", []string{"/usr/bin"}, flags.AllowPathsRO)
	}
	if !reflect.DeepEqual(flags.AllowPathsRW, []string{"/tmp/state"}) {
		t.Errorf("expected AllowPathsRW %v, got %v", []string{"/tmp/state"}, flags.AllowPathsRW)
	}
	if !reflect.DeepEqual(cmdArgs, []string{"python3", "app.py"}) {
		t.Errorf("expected cmdArgs %v, got %v", []string{"python3", "app.py"}, cmdArgs)
	}
}

func TestParseRunFlagsRequireDoubleDash(t *testing.T) {
	args := []string{"--allow-path=/bin", "echo", "test"}
	_, _, err := ParseRunFlags(args)
	if err == nil {
		t.Fatal("expected error when '--' is omitted, got nil")
	}
	if !strings.Contains(err.Error(), "requires '--'") {
		t.Errorf("expected error to mention requires '--', got: %v", err)
	}
}

func TestParseRunFlagsRejectsMisplacedAllowPath(t *testing.T) {
	args := []string{"--", "echo", "--allow-path=/bin"}
	_, _, err := ParseRunFlags(args)
	if err == nil {
		t.Fatal("expected error for misplaced --allow-path after '--', got nil")
	}
	if !strings.Contains(err.Error(), "--allow-path must precede the -- delimiter") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseRunFlagsRejectsMisplacedAllowPathRW(t *testing.T) {
	args := []string{"--", "echo", "--allow-path-rw=/tmp"}
	_, _, err := ParseRunFlags(args)
	if err == nil {
		t.Fatal("expected error for misplaced --allow-path-rw after '--', got nil")
	}
	if !strings.Contains(err.Error(), "--allow-path-rw must precede the -- delimiter") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseChildFlagsLenient(t *testing.T) {
	args := []string{"--quiet", "--allow-path=/usr/local/bin", "--allow-path-rw=/root/.gemini", "--session-dir=/tmp/sess", "echo", "hello"}
	flags, cmdArgs, err := ParseChildFlags(args)
	if err != nil {
		t.Fatalf("ParseChildFlags failed: %v", err)
	}

	if !flags.Quiet {
		t.Error("expected Quiet=true")
	}
	if flags.SessionDir != "/tmp/sess" {
		t.Errorf("expected SessionDir %q, got %q", "/tmp/sess", flags.SessionDir)
	}
	if !reflect.DeepEqual(flags.AllowPathsRO, []string{"/usr/local/bin"}) {
		t.Errorf("expected AllowPathsRO %v, got %v", []string{"/usr/local/bin"}, flags.AllowPathsRO)
	}
	if !reflect.DeepEqual(flags.AllowPathsRW, []string{"/root/.gemini"}) {
		t.Errorf("expected AllowPathsRW %v, got %v", []string{"/root/.gemini"}, flags.AllowPathsRW)
	}
	if !reflect.DeepEqual(cmdArgs, []string{"echo", "hello"}) {
		t.Errorf("expected cmdArgs %v, got %v", []string{"echo", "hello"}, cmdArgs)
	}
}

func TestParseChildFlagsRejectsTrailingFlags(t *testing.T) {
	for _, flag := range []string{"--allow-path", "--allow-path-rw", "--session-dir"} {
		_, _, err := ParseChildFlags([]string{"--quiet", flag})
		if err == nil {
			t.Fatalf("expected error on trailing %s in ParseChildFlags, got nil", flag)
		}
		expectedErr := fmt.Sprintf("safebox: %s requires a directory argument", flag)
		if err.Error() != expectedErr {
			t.Errorf("expected %q, got %q", expectedErr, err.Error())
		}
	}
}

func TestParseDiffApplyFlagsOptions(t *testing.T) {
	flags, err := ParseDiffApplyFlags([]string{"--quiet", "--yes"}, "apply")
	if err != nil {
		t.Fatalf("ParseDiffApplyFlags failed: %v", err)
	}
	if !flags.Quiet || !flags.Yes {
		t.Errorf("expected Quiet=true, Yes=true; got %v, %v", flags.Quiet, flags.Yes)
	}

	flagsShort, err := ParseDiffApplyFlags([]string{"-q", "-y"}, "revert")
	if err != nil {
		t.Fatalf("ParseDiffApplyFlags with short flags failed: %v", err)
	}
	if !flagsShort.Quiet || !flagsShort.Yes {
		t.Errorf("expected Quiet=true, Yes=true; got %v, %v", flagsShort.Quiet, flagsShort.Yes)
	}
}

func TestParseDiffApplyFlagsRejectsExactShadow(t *testing.T) {
	for _, arg := range []string{"--shadow", "--shadow=/tmp/xyz", "--shadow=", "--shadow=   "} {
		_, errDiff := ParseDiffApplyFlags([]string{arg}, "diff")
		if errDiff == nil {
			t.Fatalf("expected error on %q for diff, got nil", arg)
		}
		if !strings.Contains(errDiff.Error(), "safebox diff: unknown flag '--shadow'") {
			t.Errorf("unexpected error for %q: %v", arg, errDiff)
		}

		_, errApply := ParseDiffApplyFlags([]string{arg}, "apply")
		if errApply == nil {
			t.Fatalf("expected error on %q for apply, got nil", arg)
		}
		if !strings.Contains(errApply.Error(), "safebox apply: unknown flag '--shadow'") {
			t.Errorf("unexpected error for %q: %v", arg, errApply)
		}
	}
}

func TestParseDiffApplyFlagsDoesNotRejectSubstringShadow(t *testing.T) {
	// Flags starting with '-' that contain "shadow" as substring must return standard unknown flag error, not the specific '--shadow' deprecation error
	_, errDiff := ParseDiffApplyFlags([]string{"--shadow-custom"}, "diff")
	if errDiff == nil {
		t.Fatal("expected error on unknown flag '--shadow-custom', got nil")
	}
	if strings.Contains(errDiff.Error(), "unknown flag '--shadow'") {
		t.Errorf("expected error to name full flag '--shadow-custom', got: %v", errDiff)
	}

	// Positional arguments containing "shadow" should be collected for diff
	flags, err := ParseDiffApplyFlags([]string{"my-shadow-file.txt"}, "diff")
	if err != nil {
		t.Fatalf("unexpected error for diff positional arg: %v", err)
	}
	if !reflect.DeepEqual(flags.Paths, []string{"my-shadow-file.txt"}) {
		t.Errorf("expected Paths %v, got %v", []string{"my-shadow-file.txt"}, flags.Paths)
	}

	// Positional arguments containing "shadow" must be rejected for apply and revert
	_, errApply := ParseDiffApplyFlags([]string{"my-shadow-file.txt"}, "apply")
	if errApply == nil {
		t.Fatal("expected error on positional arg for apply, got nil")
	}
	if !strings.Contains(errApply.Error(), "takes no positional arguments") {
		t.Errorf("expected takes no positional arguments error, got: %v", errApply)
	}
}

func TestParseDiffApplyFlagsPositionalArgs(t *testing.T) {
	flags, err := ParseDiffApplyFlags([]string{"--quiet", "file1.txt", "dir/file2.txt"}, "diff")
	if err != nil {
		t.Fatalf("ParseDiffApplyFlags for diff failed: %v", err)
	}
	if !flags.Quiet {
		t.Error("expected Quiet=true")
	}
	if !reflect.DeepEqual(flags.Paths, []string{"file1.txt", "dir/file2.txt"}) {
		t.Errorf("expected Paths %v, got %v", []string{"file1.txt", "dir/file2.txt"}, flags.Paths)
	}

	_, errRevert := ParseDiffApplyFlags([]string{"file1.txt"}, "revert")
	if errRevert == nil {
		t.Fatal("expected error on positional arg for revert, got nil")
	}
	if !strings.Contains(errRevert.Error(), "safebox revert: takes no positional arguments") {
		t.Errorf("expected revert positional arg error, got: %v", errRevert)
	}
}

func TestParserChildArgsMatchParentArgs(t *testing.T) {
	parentArgs := []string{"--quiet", "--allow-path=/usr/local/bin", "--allow-path-rw=/root/.gemini", "--session-dir=/tmp/sess", "--", "node", "index.js"}
	parentFlags, parentCmd, err := ParseRunFlags(parentArgs)
	if err != nil {
		t.Fatalf("ParseRunFlags failed: %v", err)
	}

	// Reconstruct child args as ReexecChild does
	var childArgv []string
	if parentFlags.Quiet {
		childArgv = append(childArgv, "--quiet")
	}
	if parentFlags.SessionDir != "" {
		childArgv = append(childArgv, "--session-dir="+parentFlags.SessionDir)
	}
	for _, p := range parentFlags.AllowPathsRO {
		childArgv = append(childArgv, "--allow-path="+p)
	}
	for _, p := range parentFlags.AllowPathsRW {
		childArgv = append(childArgv, "--allow-path-rw="+p)
	}
	childArgv = append(childArgv, parentCmd...)

	childFlags, childCmd, err := ParseChildFlags(childArgv)
	if err != nil {
		t.Fatalf("ParseChildFlags failed: %v", err)
	}

	if !reflect.DeepEqual(parentFlags.AllowPathsRO, childFlags.AllowPathsRO) {
		t.Errorf("AllowPathsRO mismatch: parent %v, child %v", parentFlags.AllowPathsRO, childFlags.AllowPathsRO)
	}
	if !reflect.DeepEqual(parentFlags.AllowPathsRW, childFlags.AllowPathsRW) {
		t.Errorf("AllowPathsRW mismatch: parent %v, child %v", parentFlags.AllowPathsRW, childFlags.AllowPathsRW)
	}
	if parentFlags.SessionDir != childFlags.SessionDir {
		t.Errorf("SessionDir mismatch: parent %q, child %q", parentFlags.SessionDir, childFlags.SessionDir)
	}
	if parentFlags.Quiet != childFlags.Quiet {
		t.Errorf("Quiet mismatch: parent %v, child %v", parentFlags.Quiet, childFlags.Quiet)
	}
	if !reflect.DeepEqual(parentCmd, childCmd) {
		t.Errorf("CmdArgs mismatch: parent %v, child %v", parentCmd, childCmd)
	}
}
