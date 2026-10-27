package codingcase

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This opt-in check executes trusted fixtures on the remote ECS. It is not
// proof that E2B, OpenClaw, or the isolated hidden verifier has run.
func TestPythonFixtureBugFailsAndGoldPasses(t *testing.T) {
	if os.Getenv("ARIES_RUN_PYTHON_CASE_TESTS") != "1" {
		t.Skip("run explicitly on remote ECS with Python and pytest installed")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	gold, err := os.ReadFile("testdata/gold.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source string
		pass         bool
	}{{"bug", buggySource, false}, {"gold", string(gold), true}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range map[string]string{"intervals.py": tc.source, "test_public.py": publicTests} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(python, "-m", "pytest", "-qq", "-p", "no:cacheprovider", "--color=no", "test_public.py")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "LC_ALL=C", "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1", "PYTHONDONTWRITEBYTECODE=1")
			output, err := cmd.CombinedOutput()
			if (err == nil) != tc.pass {
				t.Fatalf("fixture pass=%v: %v\n%s", tc.pass, err, output)
			}
			if !tc.pass {
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
					t.Fatalf("bug fixture failed for infrastructure, not tests: %v\n%s", err, output)
				}
				if !strings.Contains(string(output), "FAILED") || !strings.Contains(string(output), "test_public.py") {
					t.Fatalf("portable pytest options hid failure details:\n%s", output)
				}
			}
		})
	}
}

func TestPythonFinalizeOnlyChangesPublicInputMtimes(t *testing.T) {
	if os.Getenv("ARIES_RUN_PYTHON_CASE_TESTS") != "1" {
		t.Skip("run explicitly on remote ECS with Python installed")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	const originalRoot = `pathlib.Path("/workspace")`
	if strings.Count(finalizeScript, originalRoot) != 1 {
		t.Fatal("finalizer must have exactly one explicit workspace root")
	}
	const check = `import os, pathlib, stat, sys
parent=pathlib.Path(sys.argv[1])
workspace=parent/"aries-case"
workspace.mkdir(mode=0o750)
public=[workspace/name for name in ("intervals.py","test_public.py","README.md")]
for target in public:
    target.write_bytes((target.name+"\npublic fixture\n").encode())
    target.chmod(0o640)
unrelated=workspace/"untouched"/"extra.txt"
unrelated.parent.mkdir(mode=0o700)
unrelated.write_bytes(b"unrelated contents\n")
outside=parent/"outside.txt"
outside.write_bytes(b"symlink target must remain unchanged\n")
variant=sys.argv[2]
if variant != "valid":
    public[0].unlink()
    if variant == "directory": public[0].mkdir(mode=0o700)
    else: public[0].symlink_to(outside)
targets=public+[workspace,parent]
before={}
for target in targets+[unrelated,unrelated.parent,outside]:
    info=target.lstat()
    contents=target.read_bytes() if stat.S_ISREG(info.st_mode) else None
    os.utime(target,ns=(1609459200123456789,1640995200987654321),follow_symlinks=False)
    before[target]=(target.lstat(),contents)
try:
    exec(sys.argv[3],{})
except RuntimeError:
    assert variant != "valid", "valid fixture was rejected"
else:
    assert variant == "valid", "malformed public file was accepted"
for target,(old,contents) in before.items():
    now=target.lstat()
    expected=1704067200*1_000_000_000 if variant == "valid" and target in targets else old.st_mtime_ns
    assert now.st_mtime_ns == expected, f"wrong target mtime: {target}"
    # A rejected symlink's atime can change independently of this finalizer
    # (also reproduced with a no-op). Preserve the regular-file/dir guarantee.
    if not stat.S_ISLNK(old.st_mode):
        assert now.st_atime_ns == old.st_atime_ns, f"atime changed: {target}"
    assert (now.st_mode,now.st_uid,now.st_gid) == (old.st_mode,old.st_uid,old.st_gid), "mode or ownership changed"
    if contents is not None: assert target.read_bytes() == contents, "file contents changed"
if variant == "symlink": assert os.readlink(public[0]) == str(outside), "symlink was retargeted"
`
	for _, variant := range []string{"valid", "directory", "symlink"} {
		t.Run(variant, func(t *testing.T) {
			root := t.TempDir()
			script := strings.Replace(finalizeScript, originalRoot, "pathlib.Path("+strconv.Quote(root)+")", 1)
			cmd := exec.Command(python, "-I", "-c", check, root, variant, script)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("real Python finalizer check: %v\n%s", err, output)
			}
		})
	}
}
