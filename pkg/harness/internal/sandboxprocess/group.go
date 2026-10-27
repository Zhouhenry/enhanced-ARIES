// Package sandboxprocess contains processes in an already Runner-owned Sandbox.
package sandboxprocess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/runner"
)

var ErrUnsupported = errors.New("sandbox process capability unsupported")

// Sandbox is the exact borrowed task environment with binary streaming execution.
type Sandbox interface {
	runner.Sandbox
	runner.StreamExecutor
}

// Group is not a remote PID tracker. Its root-owned admission file and
// cgroup contain descendants, including detached children, until confirmed empty.
type Group struct {
	sandbox Sandbox
	dir     string
	cgroup  string
	workdir string
	mu      sync.Mutex
	stopped bool
}

// New retains a group after a submitted prepare fails; callers must keep it for Stop.
func New(ctx context.Context, sandbox runner.Sandbox, workdir string) (*Group, error) {
	streaming, ok := sandbox.(Sandbox)
	if !ok {
		return nil, fmt.Errorf("%w: binary streaming execution is required", ErrUnsupported)
	}
	if !path.IsAbs(workdir) || path.Clean(workdir) != workdir || workdir == "/" || strings.ContainsRune(workdir, 0) {
		return nil, errors.New("cloud task workspace must be a clean non-root absolute path")
	}
	id, err := privatefiles.RandomID()
	if err != nil {
		return nil, err
	}
	g := &Group{sandbox: streaming, dir: "/run/aries-cloud-" + id, cgroup: "/sys/fs/cgroup/aries-" + id, workdir: workdir}
	result, err := sandbox.Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", ControlScript, "prepare", g.dir, g.cgroup, workdir}, User: "root", Dir: "/", OutputLimitBytes: 4096})
	if err != nil {
		return g, fmt.Errorf("prepare cloud process containment: %w", err)
	}
	if result.ExitCode != 0 {
		return g, fmt.Errorf("%w: template must provide private UID 60000, writable cgroup v2 with cgroup.kill, and a writable task workspace", ErrUnsupported)
	}
	return g, nil
}

func (g *Group) Command(command core.Command) (core.Command, error) {
	if command.Path == "" || strings.ContainsRune(command.Path, 0) {
		return core.Command{}, errors.New("cloud command path is required")
	}
	if command.User != "" && command.User != "aries-agent" && command.User != "60000" {
		return core.Command{}, errors.New("cloud agent commands cannot change user")
	}
	if command.Dir == "" {
		command.Dir = g.workdir
	}
	payload, err := json.Marshal(struct {
		Path string            `json:"path"`
		Args []string          `json:"args"`
		Dir  string            `json:"dir"`
		Env  map[string]string `json:"env"`
	}{command.Path, command.Args, command.Dir, command.Env})
	if err != nil {
		return core.Command{}, err
	}
	return core.Command{Path: "python3", Args: []string{"-I", "-c", ControlScript, "exec", g.dir, g.cgroup, string(payload)}, Dir: "/", User: "root", Stdin: command.Stdin, Timeout: command.Timeout, OutputLimitBytes: command.OutputLimitBytes}, nil
}

func (g *Group) ExecStream(ctx context.Context, command core.Command, in io.Reader, out, errout io.Writer) (core.CommandResult, error) {
	wrapped, err := g.Command(command)
	if err != nil {
		return core.CommandResult{ExitCode: -1}, err
	}
	return g.sandbox.ExecStream(ctx, wrapped, in, out, errout)
}

func (g *Group) Stop(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return nil
	}
	result, err := g.sandbox.Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", ControlScript, "stop", g.dir, g.cgroup}, User: "root", Dir: "/", Timeout: 15 * time.Second, OutputLimitBytes: 4096})
	if err != nil {
		return fmt.Errorf("confirm cloud process group termination: %w", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "ARIES_GROUP_EMPTY" {
		return errors.New("cloud process group termination is unconfirmed")
	}
	g.stopped = true
	return nil
}

func (g *Group) Sandbox() Sandbox   { return g.sandbox }
func (g *Group) ControlDir() string { return g.dir }
func (g *Group) CgroupPath() string { return g.cgroup }

// ScriptLibrary keeps the admission lock around native-specific specification loading.
const ScriptLibrary = `import ctypes, fcntl, json, os, pathlib, pwd, stat, sys, time
uid = 60000
def trusted(p):
    p = pathlib.Path(p)
    while True:
        s = os.lstat(p)
        if not stat.S_ISDIR(s.st_mode) or s.st_uid != 0 or s.st_mode & 0o022:
            raise RuntimeError("untrusted control path")
        if p == p.parent: break
        p = p.parent
def group_control(action, control, group, load_spec=None, workspace=None):
    if os.geteuid() != 0: raise RuntimeError("root controller required")
    if action == "prepare":
        trusted("/run")
        trusted("/sys/fs/cgroup")
        if not os.path.isfile("/sys/fs/cgroup/cgroup.controllers"): raise RuntimeError("cgroup v2 required")
        account = pwd.getpwuid(uid)
        if account.pw_name != "aries-agent": raise RuntimeError("dedicated agent identity required")
        for p in pathlib.Path("/proc").iterdir():
            if p.name.isdigit():
                try:
                    if p.stat().st_uid == uid: raise RuntimeError("agent identity already active")
                except FileNotFoundError: pass
        workspace = pathlib.Path(workspace)
        if workspace.is_symlink() or not workspace.is_dir() or str(workspace.resolve()) != str(workspace): raise RuntimeError("invalid workspace")
        os.mkdir(control, 0o700)
        os.mkdir(group, 0o700)
        try:
            with open(group + "/cgroup.kill", "w") as f: f.write("1")
            with open(control + "/lock", "x"): pass
            for root, dirs, files in os.walk(workspace, followlinks=False):
                os.chown(root, uid, uid, follow_symlinks=False)
                for name in dirs + files: os.chown(os.path.join(root, name), uid, uid, follow_symlinks=False)
        except BaseException:
            os.rmdir(group)
            raise
    elif action in ("exec", "stop"):
        trusted(control)
        trusted(group)
        with open(control + "/lock", "r+") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            if action == "stop":
                with open(control + "/revoked", "w") as f: f.write("closed")
                with open(group + "/cgroup.kill", "w") as f: f.write("1")
                end = time.monotonic() + 10
                while True:
                    values = dict(line.split() for line in open(group + "/cgroup.events"))
                    if values.get("populated") == "0":
                        print("ARIES_GROUP_EMPTY", flush=True)
                        break
                    if time.monotonic() >= end: raise TimeoutError("process group remains populated")
                    time.sleep(0.02)
            else:
                if os.path.exists(control + "/revoked"): raise RuntimeError("agent admission revoked")
                spec = load_spec()
                os.chdir(spec["dir"])
                with open(group + "/cgroup.procs", "w") as f: f.write(str(os.getpid()))
                libc = ctypes.CDLL(None, use_errno=True)
                if libc.prctl(38, 1, 0, 0, 0) != 0: raise OSError(ctypes.get_errno(), "no_new_privs failed")
                os.setgroups([])
                os.setgid(uid)
                os.setuid(uid)
                env = {"PATH":"/usr/local/bin:/usr/bin:/bin", "HOME":"/tmp", "USER":"aries-agent", "LOGNAME":"aries-agent"}
                env.update(spec.get("env") or {})
                fcntl.flock(lock, fcntl.LOCK_UN)
                os.execvpe(spec["path"], [spec["path"]] + (spec.get("args") or []), env)
    else: raise RuntimeError("invalid control operation")
`

// ControlScript handles only generic prepare, command execution and confirmed stop.
const ControlScript = ScriptLibrary + `
action, control, group = sys.argv[1:4]
group_control(action, control, group, lambda: json.loads(sys.argv[4]), sys.argv[4] if action == "prepare" else None)
`
