# Versioned sandbox workloads

This benchmark is a small sandbox workload, not an agent leaderboard dataset.
It exposes one logical task through the existing `runner.Benchmark` contract.
`Options` contains `Workload`, `TaskID`, `Template`, `OutputDir`, `TaskTimeout`, and
`VerifierTimeout`; `ForTask` gives the outer scheduler's occurrence its own ID.
Default deadlines are five minutes for the agent and one minute for verification.

Choose a workload before recording:

| `Workload` | Task instructions |
| --- | --- |
| Empty or `merge-ranges` | Original instructions and public input bytes, unchanged. |
| `merge-ranges-portable-v1` | Explicit inspection, baseline test, repair, and retest sequence using the tool's default workspace and relative filenames. |
| `merge-ranges-multicall-v1` | Portable task with two independent initial reads in the same assistant response. |
| `llm-development-pptx-v1` | The user's exact request: `幻灯片 生成一份关于大模型发展的pptx材料，写5页。` |

Unknown versions are rejected. If `TaskID` is omitted, it defaults to the
selected workload name. A scheduler may still assign a separate occurrence ID;
that does not change the workload. The instruction delivered to the agent is
also the exact content of the task's `README.md`.

The three merge-ranges versions use the same buggy Python source, public tests, and independent
12-test verifier. Only the task instruction and uploaded `README.md` differ.
Existing tapes are immutable legacy evidence: do not rewrite their requests,
tool results, or responses. Record a new tape for `merge-ranges-portable-v1`,
then replay that unchanged tape against the same workload version on each cloud.
A successful replay still requires exact request and tool-result matching;
the new task instructions do not guarantee that every possible environment
difference has disappeared.

`Template` becomes `Environment.Image`, an explicit selection that cloud wiring
must map to a published E2B template. It is not a Docker image pushed by this
package. The template needs Python 3, pytest for merge-ranges, and the dedicated `aries-agent`
UID 60000. Controller commands use `root`; OpenClaw cloud tools use UID 60000.
The task workspace is always `/workspace/aries-case`.

`PrepareSandbox` creates only the buggy `intervals.py`, the contract, and four
public tests. The input contains integer closed intervals with ordered endpoints;
the function must sort and merge overlaps and adjacency, handle an empty list,
and preserve the input including nested lists. The gold implementation exists
only in `testdata/gold.py` and is never uploaded by the benchmark.

After upload and the existing ownership setup, preparation fixes the mtime of
`intervals.py`, `test_public.py`, `README.md`, `/workspace/aries-case`, and its
`/workspace` parent to Unix epoch `1704067200` (2024-01-01 00:00:00 UTC). Files
are finalized before directories; atime, permissions, and ownership are retained.
This makes the public input dates shown by `ls -la` repeatable, but does not
equalize directory sizes or block counts across cloud filesystems. The step belongs
to environment preparation and any failure fails preparation. It does not
preserve or fix ctime, normalize tool output, fix later Agent writes, or make
commands such as `date` deterministic.

Cloud A/B uses native OpenClaw `agents.defaults.skipBootstrap=true` so fresh
workspaces do not automatically acquire bootstrap files, workspace state, or a
Git repository. Existing bootstrap files may still be read; the benchmark
recreates its task directory before uploading the public fixture. This changes
the initial context as well as directory contents, so existing tapes must be
recorded again. Ordinary profiles retain the native bootstrap default.

The legacy task asks the agent to run `python3 -m pytest -qq test_public.py`.
The extra quiet flag suppresses pytest's default elapsed-time summary for both
passing and failing runs while retaining failure assertions. This avoids adding
that varying duration to the model history; it does not make other tool output
or native OpenClaw context deterministic. Strict replay still compares complete
requests, including tool results, without filtering changed content.

The portable task uses the native tool's default working directory, which is
already the task workspace. It asks the agent to leave that directory unchanged,
use relative filenames, and avoid another `aries-case` path component or a
`workdir` override. External A may expose a virtual workspace path while
embedded B uses the physical path; the instruction does not require those
paths to be identical. The physical `Environment.Workdir` remains unchanged.

The agent first runs `cat intervals.py test_public.py`, then runs this real
baseline test command before editing and the same command after repairing:

```sh
LC_ALL=C PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 PYTHONDONTWRITEBYTECODE=1 python3 -m pytest -qq -p no:cacheprovider --color=no test_public.py
```

These native environment variables and pytest options disable optional plugin
loading, bytecode/cache creation, and color output, and choose a fixed locale.
They preserve actual failure details and exit status. The task disallows
unrelated metadata probes such as `ls -la`, `stat`, `date`, or `hostname`, and
disallows filtering or replacing tool output. There is no command wrapper,
filesystem emulation, mock tool, or relaxed replay matching. Real failures
remain part of the recorded model history.

After Runner confirms harness stop and bridge revocation, `Evaluate` downloads
only `intervals.py` through `LimitedDownloader`, with a 128 KiB bound. It checks
that the downloaded file is a regular nonempty file, makes the private host copy
read-only, records SHA-256, and sets `Evaluation.Artifact.ObservedAt` from the
controller clock after capture. This is a conservative artifact-readiness bound;
it is not inferred from the agent's final text or the guest wall clock. A failed
later verifier still retains this already captured artifact.

The benchmark then asks Runner for a fresh sandbox with networking disabled.
It transfers that single candidate and reconstructs the known public tests plus
eight private cases. No agent-provided tests, pytest configuration, environment,
or repository metadata are transferred. The root-owned verifier directory is
mode 0700. Trusted pytest code runs candidate function calls in UID 60000 child
processes with `no_new_privs`, a three-second call timeout, and a 256 KiB file
output limit. Candidate code is never imported into the root pytest process.
The independent root process compares results and input preservation, and writes
the JUnit report. Pass requires exit code 0 and all 12 expected tests passing;
missing, skipped, contradictory, or incomplete reports cannot pass.

All task and evaluation sandboxes remain Runner-owned. This benchmark does not
stop resources, bypass the isolation gate, or implement a snapshot-and-destroy
fallback. Its exported host candidate is immutable with respect to the agent;
the trusted operator/controller can still remove its private result files.

Run the following only on the remote ECS, after synchronizing the source and
creating the cache paths:

```sh
gofmt -w pkg/benchmark/codingcase
GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" TMPDIR="$PWD/.cache/test-tmp" go test -race ./pkg/benchmark/codingcase
ARIES_RUN_PYTHON_CASE_TESTS=1 GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" TMPDIR="$PWD/.cache/test-tmp" go test ./pkg/benchmark/codingcase -run TestPython -v
```

The default contract tests use fake sandbox boundaries to check workload
selection and separation, matching task/README contents, unchanged verifier
inputs and commands across versions, privacy,
candidate identity, denominator-preserving failures, fresh verification,
report consistency, cancellation, and distinct concurrent artifacts. The opt-in
checks run trusted public Python fixtures and the real metadata finalizer in
temporary directories on ECS; neither proves an actual
E2B/OpenClaw/DeepSeek run. Real A and B acceptance still requires their complete
Runner results, native execution evidence, isolation, independent verifier
score, and confirmed cloud cleanup.

## PPTX task and independent structural verification

`llm-development-pptx-v1` uses the same Benchmark / Runner lifecycle. Its task
instruction is exactly `幻灯片 生成一份关于大模型发展的pptx材料，写5页。` and its
initial workspace contains only a `README.md` with those exact bytes. No deck,
generation script, preferred library, answer, or private verifier is supplied
to the Agent. It uses the selected harness's real native tools and skills;
this benchmark does not load or simulate skills. Skill readiness and model
configuration are harness/deployment responsibilities.

After the existing confirmed stop and bridge-revocation gate, `Evaluate`
recursively inventories every regular `.pptx` file in the task workspace,
case-insensitively, including nested directories and unsuccessful intermediate
files. It does not assume a final filename. Each unchanged original is copied
under the private evaluation directory's `decks/`, preserving its relative
name, and sealed read-only. `manifest.json` records every relative path, local
copy path, byte count, and SHA-256. `Evaluation.Artifact` identifies that sealed
manifest and the controller time after all copies are complete. The separate
`capture.json` retains the guest inventory. This is a conservative capture
time, not a claim about when the Agent finished writing any deck.

Capture is limited to 128 decks, 64 MiB per deck, and 512 MiB total. Each download
uses the captured byte count as its bound (one byte for an empty file), then
checks the exact size and SHA-256. The 64 MiB per-file limit also applies to
the E2B/Huawei upload into the fresh verifier. An exceeded
limit, a symlink in the inventory, a failed download, or a hash mismatch fails
capture explicitly; it cannot silently omit a file and pass. Empty or malformed
regular files within these bounds are preserved and sent for independent
verification. A task producing no deck still receives an independent failed
report. Files produced outside the declared task workspace are not artifacts
of this workload.

A fresh Runner-owned sandbox receives only those PPTX bytes, the sealed
manifest, and the controller-owned `pptx_verifier.py`. Networking is requested
disabled, subject to the provider's separately recorded enforcement/assumption.
The verifier uses only Python standard-library ZIP/XML parsing; it never
executes candidate code, opens external relationships, or relies on the Agent's
installed PPT library. Pass requires at least one deck that:

1. Is a readable PPTX package with valid CRCs, presentation metadata, and actual
   presentation-to-slide relationships. Counting unrelated slide-shaped ZIP
   entries does not determine the page count.
2. Has exactly five slides in its presentation slide list.
3. Has nonempty body content on every slide: non-whitespace drawing text, an
   embedded image relationship whose target exists and is nonempty, or an
   embedded chart relationship whose chart contains text or data values.

Notes alone, empty shapes, and images merely present in the ZIP but unused on a
slide do not satisfy the third check. The verifier resolves internal part
relationships and reports broken references as failures. Its parsing bounds
are 8 MiB per XML part, 256 MiB expanded ZIP contents, and 8192 ZIP members.
These are structural checks: topic accuracy, factual quality, visual layout,
image decoding/rendering, and content inherited only from master/layout slides
are not graded. There is no reference solution and no claim of official
benchmark validity.

`pptx-report.json` contains a check result for **every** captured deck, with
identity, parseability, slide count, per-slide content evidence, and errors.
A valid final deck can pass while an invalid draft remains visibly failed in
the same report. The controller checks report coverage and identity against
the sealed manifest and reconciles overall pass with verifier exit status.
All original decks, reports, and errors remain available for copying back.

The opt-in `TestPythonPPTXVerifierFixtures` executes the actual private Python
verifier and capture scripts on ECS with trusted synthetic fixtures: five
nonempty slides, wrong count, an empty slide, corrupt/zero-byte files, no files,
a passing final plus a failed draft, referenced images/charts, notes-only or
unused/broken-image cases, Unicode filenames, symlinks, and explicit capture
limits. Go tests check the unchanged prompt, separation of public/private
inputs, full export before fresh verification, sealed hashes, and rejection
of missing/contradictory reports. These checks do not constitute a successful
real Jiuwen A/B task; cloud acceptance is recorded separately.
