package sandboxprocess

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/runner"
)

type sandboxStub struct {
	calls       []core.Command
	prepareErr  error
	prepareCode int
	stopErr     error
	stopResult  core.CommandResult
}

func (*sandboxStub) Upload(context.Context, string, string) error   { return nil }
func (*sandboxStub) Download(context.Context, string, string) error { return nil }
func (*sandboxStub) Connectivity() core.HarnessConnectivity         { return core.HarnessConnectivity{} }
func (s *sandboxStub) Exec(_ context.Context, c core.Command) (core.CommandResult, error) {
	s.calls = append(s.calls, c)
	if c.Args[3] == "prepare" {
		return core.CommandResult{ExitCode: s.prepareCode}, s.prepareErr
	}
	return s.stopResult, s.stopErr
}
func (*sandboxStub) ExecStream(context.Context, core.Command, io.Reader, io.Writer, io.Writer) (core.CommandResult, error) {
	return core.CommandResult{}, nil
}

func TestPrepareFailureRetainsExactSandboxAndGroupForStop(t *testing.T) {
	for _, transportFailure := range []bool{false, true} {
		sandbox := &sandboxStub{prepareCode: 1, stopResult: core.CommandResult{Stdout: "ARIES_GROUP_EMPTY\n"}}
		if transportFailure {
			sandbox.prepareErr = errors.New("prepare transport failed")
		}
		group, err := New(context.Background(), sandbox, "/workspace")
		if err == nil || group == nil || group.Sandbox() != sandbox {
			t.Fatalf("partial ownership lost: %v %v", group, err)
		}
		if !transportFailure && !errors.Is(err, ErrUnsupported) {
			t.Fatalf("capability failure changed: %v", err)
		}
		if err := group.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(sandbox.calls) != 2 || sandbox.calls[1].Args[3] != "stop" || sandbox.calls[1].Args[4] != group.ControlDir() || sandbox.calls[1].Args[5] != group.CgroupPath() || sandbox.calls[0].Args[4] != group.ControlDir() {
			t.Fatal("cleanup changed partial ownership")
		}
	}
}

func TestStopRequiresPositiveEmptyAndPreservesRetry(t *testing.T) {
	for _, test := range []struct {
		name   string
		result core.CommandResult
		err    error
	}{
		{"missing_receipt", core.CommandResult{}, nil},
		{"nonzero_with_receipt", core.CommandResult{ExitCode: 1, Stdout: "ARIES_GROUP_EMPTY\n"}, nil},
		{"transport_failure", core.CommandResult{Stdout: "ARIES_GROUP_EMPTY\n"}, errors.New("transport failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			sandbox := &sandboxStub{stopResult: test.result, stopErr: test.err}
			group, err := New(context.Background(), sandbox, "/workspace")
			if err != nil {
				t.Fatal(err)
			}
			if err := group.Stop(context.Background()); err == nil {
				t.Fatal("unconfirmed stop succeeded")
			}
			sandbox.stopResult = core.CommandResult{Stdout: "ARIES_GROUP_EMPTY\n"}
			sandbox.stopErr = nil
			if err := group.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := group.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(sandbox.calls) != 3 {
				t.Fatal("failed stop was forgotten or confirmed stop repeated")
			}
		})
	}
}

func TestNewRejectsUnsupportedOrInvalidWorkspaceBeforeSubmission(t *testing.T) {
	sandbox := &sandboxStub{}
	var noStreaming runner.Sandbox = struct{ runner.Sandbox }{Sandbox: sandbox}
	if group, err := New(context.Background(), noStreaming, "/workspace"); group != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("streaming capability: %v %v", group, err)
	}
	for _, dir := range []string{"/", "relative", "/workspace/../other", "/workspace\x00"} {
		if group, err := New(context.Background(), sandbox, dir); group != nil || err == nil {
			t.Fatalf("workspace %q accepted", dir)
		}
	}
	if len(sandbox.calls) != 0 {
		t.Fatal("invalid inputs submitted guest commands")
	}
}

func TestControlScriptAdmissionAndPositiveEmptyBoundaries(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	// Execute the shared Python control flow against temporary files; privilege
	// and process-entry effects are intercepted, not claimed as kernel isolation.
	probe := `import contextlib, fcntl, io, os, pathlib, sys, tempfile, types
namespace={}
exec(sys.stdin.read(),namespace)
with tempfile.TemporaryDirectory() as root:
    control=pathlib.Path(root)/"control";control.mkdir()
    group=pathlib.Path(root)/"group";group.mkdir()
    (control/"lock").touch();(group/"cgroup.events").write_text("populated 0\n")
    observed=[]
    namespace["trusted"]=lambda p: observed.append(("trusted",str(p)))
    fake_os=types.SimpleNamespace(geteuid=lambda:0,path=os.path,getpid=lambda:123,
        chdir=lambda d:observed.append(("chdir",d)),
        setgroups=lambda g:observed.append(("groups",g)),
        setgid=lambda g:observed.append(("gid",g)),setuid=lambda u:observed.append(("uid",u)))
    namespace["os"]=fake_os
    namespace["ctypes"]=types.SimpleNamespace(CDLL=lambda *a,**k:types.SimpleNamespace(prctl=lambda *a:observed.append(("no_new_privs",a)) or 0),get_errno=lambda:1)
    def load_spec():
        with (control/"lock").open("r+") as lock:
            try: fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
            except BlockingIOError: pass
            else: raise AssertionError("native spec loaded outside admission lock")
        observed.append(("spec_loaded",))
        return {"path":"/native/runtime","args":["literal ; $value"],"dir":"/workspace","env":{"TOKEN":"private-test-token"}}
    class Executed(Exception):pass
    def execute(path,args,env):
        with (control/"lock").open("r+") as lock: fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        assert path=="/native/runtime" and args==[path,"literal ; $value"]
        assert env["TOKEN"]=="private-test-token" and "private-test-token" not in str(args)
        assert (group/"cgroup.procs").read_text()=="123"
        observed.append(("exec",));raise Executed()
    fake_os.execvpe=execute
    run=namespace["group_control"]
    try:run("exec",str(control),str(group),load_spec)
    except Executed:pass
    else:raise AssertionError("exec did not reach native process")
    assert [x[0] for x in observed]==["trusted","trusted","spec_loaded","chdir","no_new_privs","groups","gid","uid","exec"],observed
    assert ("gid",60000) in observed and ("uid",60000) in observed and ("groups",[]) in observed
    assert ("no_new_privs",(38,1,0,0,0)) in observed
    observed.clear();(control/"revoked").write_text("closed")
    try:run("exec",str(control),str(group),load_spec)
    except RuntimeError as e:assert str(e)=="agent admission revoked"
    else:raise AssertionError("revoked admission accepted")
    assert all(x[0] not in ("spec_loaded","exec") for x in observed)
    (control/"revoked").unlink();observed.clear()
    namespace["ctypes"].CDLL=lambda *a,**k:types.SimpleNamespace(prctl=lambda *a:-1)
    try:run("exec",str(control),str(group),load_spec)
    except OSError:pass
    else:raise AssertionError("no_new_privs failure ignored")
    assert all(x[0] not in ("uid","exec") for x in observed)
    output=io.StringIO()
    with contextlib.redirect_stdout(output):run("stop",str(control),str(group))
    assert output.getvalue()=="ARIES_GROUP_EMPTY\n"
    assert (control/"revoked").read_text()=="closed" and (group/"cgroup.kill").read_text()=="1"
    (group/"cgroup.events").write_text("populated 1\n")
    ticks=iter([0,11]);namespace["time"]=types.SimpleNamespace(monotonic=lambda:next(ticks),sleep=lambda _:None)
    output=io.StringIO()
    try:
        with contextlib.redirect_stdout(output):run("stop",str(control),str(group))
    except TimeoutError:pass
    else:raise AssertionError("populated cgroup accepted")
    assert output.getvalue()==""
    fake_os.geteuid=lambda:60000
    try:run("stop",str(control),str(group))
    except RuntimeError as e:assert str(e)=="root controller required"
    else:raise AssertionError("unprivileged control accepted")
`
	command := exec.Command(python, "-I", "-c", probe)
	command.Stdin = strings.NewReader(ScriptLibrary)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("shared containment boundary: %s %v", output, err)
	}
}
