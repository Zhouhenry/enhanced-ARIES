package codingcase

const instruction = `Fix intervals.py in /workspace/aries-case.

Implement merge_ranges(ranges) as a pure function. The input is a list of
two-element lists [start, end] containing integers with start <= end. Intervals
are closed, may arrive in any order, and may overlap, repeat, or touch. Merge
both overlapping and adjacent intervals: [1, 2] and [3, 4] become [1, 4]. Return
a list of two-element lists sorted by start. Return [] for empty input. Do not
modify the input list or any of its nested lists. Negative and large integers
are valid. Invalid input handling is outside this task.

Read the code and public tests, repair the function, then run
python3 -m pytest -qq test_public.py. Submit only your change to intervals.py.
The independent verifier exports only that file and runs known public and
private tests in a fresh sandbox. Edits to tests or additional files are not
part of the submitted candidate. Do not rely on extra packages.
`

const portableInstruction = `Workload: merge-ranges-portable-v1

Fix intervals.py in the tool's current workspace. That workspace is already the
task directory. For every exec call, keep the native default working directory
and use relative filenames. Do not set a workdir parameter, prepend cd, append
another aries-case directory, or invent an absolute path.

Implement merge_ranges(ranges) as a pure function. The input is a list of
two-element lists [start, end] containing integers with start <= end. Intervals
are closed, may arrive in any order, and may overlap, repeat, or touch. Merge
both overlapping and adjacent intervals: [1, 2] and [3, 4] become [1, 4]. Return
a list of two-element lists sorted by start. Return [] for empty input. Do not
modify the input list or any of its nested lists. Negative and large integers
are valid. Invalid input handling is outside this task.

Use exec and follow these steps in order:
1. Read the supplied source and public tests with:
   cat intervals.py test_public.py
2. Run the unchanged public tests before editing. Keep their real failure
   output and exit status. Use this exact command:
   LC_ALL=C PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 PYTHONDONTWRITEBYTECODE=1 python3 -m pytest -qq -p no:cacheprovider --color=no test_public.py
3. Repair only intervals.py, using relative filenames in the current workspace.
4. Run the exact command from step 2 again to check the repair. If it fails,
   retain the actual error, repair the function, and rerun the same tests.

Do not inspect unrelated filesystem or host metadata, including directory
listings such as ls -la, stat, date, hostname, or environment dumps. Do not
filter, replace, or hide tool output or failures. Do not install extra packages
or modify the tests. The native pytest options above select the output format;
they do not replace the real tests or their results.

Submit only your change to intervals.py. The independent verifier exports only
that file and runs known public and private tests in a fresh sandbox. Edits to
tests or additional files are not part of the submitted candidate.
`

const multiCallInstruction = `Workload: merge-ranges-multicall-v1

Fix intervals.py in the tool's current workspace. That workspace is already the
task directory. For every exec call, keep the native default working directory
and use relative filenames. Do not set a workdir parameter, prepend cd, append
another aries-case directory, or invent an absolute path.

Implement merge_ranges(ranges) as a pure function. The input is a list of
two-element lists [start, end] containing integers with start <= end. Intervals
are closed, may arrive in any order, and may overlap, repeat, or touch. Merge
both overlapping and adjacent intervals: [1, 2] and [3, 4] become [1, 4]. Return
a list of two-element lists sorted by start. Return [] for empty input. Do not
modify the input list or any of its nested lists. Negative and large integers
are valid. Invalid input handling is outside this task.

Use exec and follow these steps in order:
1. Read the supplied source and public tests with two separate exec tool calls
   in the same assistant response: one with command "cat intervals.py" and one
   with command "cat test_public.py". These reads are independent. Do not combine
   them into one shell command or wait for the first result before requesting
   the second. Wait for both real results before proceeding to step 2.
2. Run the unchanged public tests before editing. Keep their real failure
   output and exit status. Use this exact command:
   LC_ALL=C PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 PYTHONDONTWRITEBYTECODE=1 python3 -m pytest -qq -p no:cacheprovider --color=no test_public.py
3. Diagnose the failures and repair only intervals.py, using relative filenames
   in the current workspace.
4. Run the exact command from step 2 again to check the repair. If it fails,
   retain the actual error, repair the function, and rerun the same tests.

Do not inspect unrelated filesystem or host metadata, including directory
listings such as ls -la, stat, date, hostname, or environment dumps. Do not
filter, replace, or hide tool output or failures. Do not install extra packages
or modify the tests. The native pytest options above select the output format;
they do not replace the real tests or their results.

Submit only your change to intervals.py. The independent verifier exports only
that file and runs known public and private tests in a fresh sandbox. Edits to
tests or additional files are not part of the submitted candidate.
`

const buggySource = `def merge_ranges(ranges):
    ranges.sort(key=lambda item: item[1])
    merged = [list(ranges[0])]
    for start, end in ranges[1:]:
        if start < merged[-1][1]:
            merged[-1][1] = end
        else:
            merged.append([start, end])
    return merged
`

// These four cases are public in both environments. The verifier executes the
// candidate in a separate unprivileged process instead of importing it as root.
const publicCases = `[
    [[[1, 2], [6, 8]], [[1, 2], [6, 8]]],
    [[[5, 7], [1, 3], [2, 6]], [[1, 7]]],
    [[[1, 2], [3, 4]], [[1, 4]]],
    [[], []]
]`

const publicTests = `import copy
import pytest
from intervals import merge_ranges

@pytest.mark.parametrize("ranges, expected", ` + publicCases + `)
def test_public(ranges, expected):
    before = copy.deepcopy(ranges)
    assert merge_ranges(ranges) == expected
    assert ranges == before
`

const verifierPublicTests = `import pytest

@pytest.mark.parametrize("ranges, expected", ` + publicCases + `)
def test_public(check, ranges, expected):
    check(ranges, expected)
`

const hiddenTests = `import pytest

@pytest.mark.parametrize("ranges, expected", [
    ([[1, 20], [4, 5], [7, 8]], [[1, 20]]),
    ([[-8, -6], [-5, -1], [0, 0]], [[-8, 0]]),
    ([[9, 9], [1, 1], [9, 9], [2, 2]], [[1, 2], [9, 9]]),
    ([[4, 12], [1, 4], [2, 3]], [[1, 12]]),
    ([[10**40, 10**40 + 2], [10**40 + 3, 10**40 + 9]], [[10**40, 10**40 + 9]]),
    ([[7, 9], [3, 5], [1, 2], [6, 6]], [[1, 9]]),
    ([[3, 8], [3, 5], [3, 8]], [[3, 8]]),
    ([[8, 10], [-2, -1], [2, 3], [5, 5]], [[-2, -1], [2, 3], [5, 5], [8, 10]])
])
def test_hidden(check, ranges, expected):
    check(ranges, expected)
`

const prepareScript = `import os, pathlib, pwd, shutil
assert os.geteuid() == 0
assert pwd.getpwuid(60000).pw_name == "aries-agent"
parent=pathlib.Path("/workspace")
if parent.is_symlink(): raise RuntimeError("workspace parent is a symlink")
parent.mkdir(mode=0o755,exist_ok=True)
workspace=parent/"aries-case"
if workspace.is_symlink(): raise RuntimeError("task workspace is a symlink")
if workspace.exists(): shutil.rmtree(workspace)
workspace.mkdir(mode=0o755)
private=pathlib.Path("/run/aries-verifier")
if private.is_symlink(): private.unlink()
elif private.exists(): shutil.rmtree(private)
`

// Fix only public input mtimes before the agent starts; preserve atime and modes.
const finalizeScript = `import os, pathlib, stat
epoch_ns=1704067200*1_000_000_000
parent=pathlib.Path("/workspace")
workspace=parent/"aries-case"
if workspace.resolve(strict=True) != workspace: raise RuntimeError("task workspace contains a symlink")
targets=[(workspace/name,stat.S_IFREG) for name in ("intervals.py","test_public.py","README.md")]
targets.extend(((workspace,stat.S_IFDIR),(parent,stat.S_IFDIR)))
for target,kind in targets:
    info=target.lstat()
    if stat.S_IFMT(info.st_mode) != kind: raise RuntimeError("fixture metadata target has unexpected type")
    os.utime(target,ns=(info.st_atime_ns,epoch_ns),follow_symlinks=False)
`

const verifierPrepareScript = `import os, pathlib, pwd
assert os.geteuid() == 0
assert pwd.getpwuid(60000).pw_name == "aries-agent"
for directory,mode in [("/workspace",0o755),("/workspace/aries-case",0o755),("/run/aries-verifier",0o700)]:
    p=pathlib.Path(directory)
    if p.is_symlink(): raise RuntimeError("verifier path is a symlink")
    p.mkdir(mode=mode,exist_ok=True)
    os.chown(p,0,0);os.chmod(p,mode)
`

const candidateValidateScript = `import os, pathlib, stat
workspace=pathlib.Path("/workspace/aries-case")
if workspace.resolve(strict=True) != workspace: raise RuntimeError("candidate workspace changed into a symlink")
candidate=workspace/"intervals.py"
info=os.lstat(candidate)
if not stat.S_ISREG(info.st_mode) or info.st_size < 1 or info.st_size > 131072:
    raise RuntimeError("candidate must be a bounded regular file")
`

// Root owns the verifier and report; candidate Python is never imported by
// pytest itself. No model credential, task environment, or agent test is copied.
const verifierHelper = `import ctypes
import json
import subprocess
import sys
import tempfile
import pytest

CALL = r'''
import copy, importlib.util, json, sys
ranges = json.loads(sys.stdin.read())
spec = importlib.util.spec_from_file_location("candidate", "/workspace/aries-case/intervals.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
result = module.merge_ranges(ranges)
assert isinstance(result, list)
assert all(isinstance(item, list) and len(item) == 2 and all(type(x) is int for x in item) for item in result)
print(json.dumps({"result": result, "after": ranges}))
'''

def unprivileged():
    import os
    import resource
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(38, 1, 0, 0, 0) != 0:
        raise OSError(ctypes.get_errno(), "no_new_privs failed")
    os.setgroups([])
    resource.setrlimit(resource.RLIMIT_FSIZE, (262144, 262144))
    os.setgid(60000)
    os.setuid(60000)

@pytest.fixture
def check():
    def run(ranges, expected):
        with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
            process = subprocess.run(
                [sys.executable, "-I", "-B", "-c", CALL],
                input=json.dumps(ranges), text=True, stdout=stdout, stderr=stderr,
                cwd="/workspace/aries-case", timeout=3, close_fds=True,
                env={"PATH": "/usr/local/bin:/usr/bin:/bin", "HOME": "/tmp"},
                preexec_fn=unprivileged,
            )
            stdout.seek(0)
            output = stdout.read(262145)
        assert process.returncode == 0, "candidate did not complete normally"
        assert len(output) <= 262144, "candidate output exceeded its bound"
        actual = json.loads(output)
        assert actual["result"] == expected
        assert actual["after"] == ranges, "candidate mutated its input"
    return run
`
