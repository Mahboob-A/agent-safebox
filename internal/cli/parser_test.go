package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRunFlagsStandard(t *testing.T) {
	args := []string{"--allow-path=/usr/local/bin", "--allow-path-rw=/root/.gemini", "--allow-file-rw=/root/.claude.json", "--quiet", "--", "echo", "hello"}
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
	if !reflect.DeepEqual(flags.AllowFilesRW, []string{"/root/.claude.json"}) {
		t.Errorf("expected AllowFilesRW %v, got %v", []string{"/root/.claude.json"}, flags.AllowFilesRW)
	}
	if !reflect.DeepEqual(cmdArgs, []string{"echo", "hello"}) {
		t.Errorf("expected cmdArgs %v, got %v", []string{"echo", "hello"}, cmdArgs)
	}
}

func TestParseRunFlagsSeparateArgForm(t *testing.T) {
	args := []string{"--allow-path", "/usr/bin", "--allow-path-rw", "/tmp/state", "--allow-file-rw", "/tmp/conf.json", "--session-dir", "/tmp/sess", "-q", "--probe", "--", "python3", "app.py"}
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
	if !reflect.DeepEqual(flags.AllowFilesRW, []string{"/tmp/conf.json"}) {
		t.Errorf("expected AllowFilesRW %v, got %v", []string{"/tmp/conf.json"}, flags.AllowFilesRW)
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

func TestParseRunFlagsRejectsMisplacedAllowFileRW(t *testing.T) {
	args := []string{"--", "echo", "--allow-file-rw=/tmp/f.json"}
	_, _, err := ParseRunFlags(args)
	if err == nil {
		t.Fatal("expected error for misplaced --allow-file-rw after '--', got nil")
	}
	if !strings.Contains(err.Error(), "--allow-file-rw must precede the -- delimiter") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseRunFlagsRejectsMisplacedPersistentState(t *testing.T) {
	args := []string{"--", "echo", "--persistent-state=/tmp/host:/tmp/mount"}
	_, _, err := ParseRunFlags(args)
	if err == nil {
		t.Fatal("expected error for misplaced --persistent-state after '--', got nil")
	}
	if !strings.Contains(err.Error(), "--persistent-state must precede the -- delimiter") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseRunFlagsPersistentState(t *testing.T) {
	args := []string{"--persistent-state=/host/dir:/mount/dir", "--persistent-state", "/host/file.json:/mount/file.json", "--", "echo", "test"}
	flags, _, err := ParseRunFlags(args)
	if err != nil {
		t.Fatalf("ParseRunFlags failed: %v", err)
	}
	expected := []string{"/host/dir:/mount/dir", "/host/file.json:/mount/file.json"}
	if !reflect.DeepEqual(flags.PersistentStateMounts, expected) {
		t.Errorf("expected PersistentStateMounts %v, got %v", expected, flags.PersistentStateMounts)
	}
}


func TestParseChildFlagsLenient(t *testing.T) {
	args := []string{"--quiet", "--allow-path=/usr/local/bin", "--allow-path-rw=/root/.gemini", "--allow-file-rw=/root/.claude.json", "--session-dir=/tmp/sess", "echo", "hello"}
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
	if !reflect.DeepEqual(flags.AllowFilesRW, []string{"/root/.claude.json"}) {
		t.Errorf("expected AllowFilesRW %v, got %v", []string{"/root/.claude.json"}, flags.AllowFilesRW)
	}
	if !reflect.DeepEqual(cmdArgs, []string{"echo", "hello"}) {
		t.Errorf("expected cmdArgs %v, got %v", []string{"echo", "hello"}, cmdArgs)
	}
}

func TestParseChildFlagsRejectsTrailingFlags(t *testing.T) {
	for _, tc := range []struct {
		flag string
		want string
	}{
		{"--allow-path", "safebox: --allow-path requires a directory argument"},
		{"--allow-path-rw", "safebox: --allow-path-rw requires a directory argument"},
		{"--allow-file-rw", "safebox: --allow-file-rw requires a file argument"},
		{"--session-dir", "safebox: --session-dir requires a directory argument"},
	} {
		_, _, err := ParseChildFlags([]string{"--quiet", tc.flag})
		if err == nil {
			t.Fatalf("expected error on trailing %s in ParseChildFlags, got nil", tc.flag)
		}
		if err.Error() != tc.want {
			t.Errorf("expected %q, got %q", tc.want, err.Error())
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
	_, errDiff := ParseDiffApplyFlags([]string{"--shadow-custom"}, "diff")
	if errDiff == nil {
		t.Fatal("expected error on unknown flag '--shadow-custom', got nil")
	}
	if strings.Contains(errDiff.Error(), "unknown flag '--shadow'") {
		t.Errorf("expected error to name full flag '--shadow-custom', got: %v", errDiff)
	}

	flags, err := ParseDiffApplyFlags([]string{"my-shadow-file.txt"}, "diff")
	if err != nil {
		t.Fatalf("unexpected error for diff positional arg: %v", err)
	}
	if !reflect.DeepEqual(flags.Paths, []string{"my-shadow-file.txt"}) {
		t.Errorf("expected Paths %v, got %v", []string{"my-shadow-file.txt"}, flags.Paths)
	}

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

func TestParseDiffApplyFlags_ForceDiscard(t *testing.T) {
	for _, cmd := range []string{"apply", "revert", "diff"} {
		flags, err := ParseDiffApplyFlags([]string{"--force-discard"}, cmd)
		if err != nil {
			t.Fatalf("ParseDiffApplyFlags failed for %s: %v", cmd, err)
		}
		if !flags.ForceDiscard {
			t.Errorf("expected ForceDiscard=true for %s", cmd)
		}

		flagsEq, err := ParseDiffApplyFlags([]string{"--force-discard=true"}, cmd)
		if err != nil {
			t.Fatalf("ParseDiffApplyFlags failed for %s with value: %v", cmd, err)
		}
		if !flagsEq.ForceDiscard {
			t.Errorf("expected ForceDiscard=true for %s with value", cmd)
		}
	}
}


func TestParserChildArgsMatchParentArgs(t *testing.T) {
	parentArgs := []string{"--quiet", "--allow-path=/usr/local/bin", "--allow-path-rw=/root/.gemini", "--allow-file-rw=/root/.claude.json", "--session-dir=/tmp/sess", "--", "node", "index.js"}
	parentFlags, parentCmd, err := ParseRunFlags(parentArgs)
	if err != nil {
		t.Fatalf("ParseRunFlags failed: %v", err)
	}

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
	for _, f := range parentFlags.AllowFilesRW {
		childArgv = append(childArgv, "--allow-file-rw="+f)
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
	if !reflect.DeepEqual(parentFlags.AllowFilesRW, childFlags.AllowFilesRW) {
		t.Errorf("AllowFilesRW mismatch: parent %v, child %v", parentFlags.AllowFilesRW, childFlags.AllowFilesRW)
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

func TestParseRunFlags_AllowNet(t *testing.T) {
	// 1. Bare flag sets AllowNet=true
	args := []string{"--allow-net", "--", "curl", "https://example.com"}
	flags, cmdArgs, err := ParseRunFlags(args)
	if err != nil {
		t.Fatalf("ParseRunFlags failed: %v", err)
	}
	if !flags.AllowNet {
		t.Errorf("expected AllowNet=true from bare --allow-net, got false")
	}
	if !reflect.DeepEqual(cmdArgs, []string{"curl", "https://example.com"}) {
		t.Errorf("expected cmdArgs %v, got %v", []string{"curl", "https://example.com"}, cmdArgs)
	}

	// 2. --allow-net=* sets AllowNet=true
	args2 := []string{"--allow-net=*", "--", "true"}
	flags2, _, err := ParseRunFlags(args2)
	if err != nil {
		t.Fatalf("ParseRunFlags --allow-net=* failed: %v", err)
	}
	if !flags2.AllowNet {
		t.Errorf("expected AllowNet=true from --allow-net=*, got false")
	}

	// 3. --allow-net=false leaves AllowNet=false
	args3 := []string{"--allow-net=false", "--", "true"}
	flags3, _, err := ParseRunFlags(args3)
	if err != nil {
		t.Fatalf("ParseRunFlags --allow-net=false failed: %v", err)
	}
	if flags3.AllowNet {
		t.Errorf("expected AllowNet=false from --allow-net=false, got true")
	}

	// 4. Default: AllowNet=false
	args4 := []string{"--", "true"}
	flags4, _, err := ParseRunFlags(args4)
	if err != nil {
		t.Fatalf("ParseRunFlags default failed: %v", err)
	}
	if flags4.AllowNet {
		t.Errorf("expected AllowNet=false by default, got true")
	}

	// 5. --allow-net=<domain> is rejected in v1
	reject := []string{"--allow-net=api.openai.com", "--", "true"}
	if _, _, err := ParseRunFlags(reject); err == nil {
		t.Errorf("expected error for --allow-net=<domain>, got nil")
	}

	// 6. Misplaced after --
	misplaced := []string{"--", "curl", "--allow-net"}
	if _, _, err := ParseRunFlags(misplaced); err == nil {
		t.Errorf("expected error for misplaced --allow-net, got nil")
	}
}

func TestParseRunFlags_AllowNetworkDeprecated(t *testing.T) {
	// --allow-network=<domain> is rejected (v0.5 will re-add via --allow-net=<domain>)
	if _, _, err := ParseRunFlags([]string{"--allow-network=api.openai.com", "--", "true"}); err == nil {
		t.Errorf("expected error for --allow-network=<domain>, got nil")
	}
	if _, _, err := ParseRunFlags([]string{"--allow-network", "api.openai.com", "--", "true"}); err == nil {
		t.Errorf("expected error for --allow-network <domain>, got nil")
	}
}

func TestParseChildFlags_NetConfig(t *testing.T) {
	// Flag form
	args := []string{"--net-config=/tmp/sess/netconfig.json", "--", "curl", "https://api.openai.com"}
	flags, _, err := ParseChildFlags(args)
	if err != nil {
		t.Fatalf("ParseChildFlags failed: %v", err)
	}
	if flags.NetConfigPath != "/tmp/sess/netconfig.json" {
		t.Errorf("expected NetConfigPath /tmp/sess/netconfig.json, got %q", flags.NetConfigPath)
	}

	// Env var form
	t.Setenv("SAFEBOX_NET_CONFIG", "/tmp/env/netconfig.json")
	flagsEnv, _, err := ParseChildFlags([]string{"--", "curl"})
	if err != nil {
		t.Fatalf("ParseChildFlags with env failed: %v", err)
	}
	if flagsEnv.NetConfigPath != "/tmp/env/netconfig.json" {
		t.Errorf("expected NetConfigPath from env /tmp/env/netconfig.json, got %q", flagsEnv.NetConfigPath)
	}

	// Missing arg error
	_, _, err = ParseChildFlags([]string{"--net-config"})
	if err == nil {
		t.Errorf("expected error for --net-config without argument, got nil")
	}
}


