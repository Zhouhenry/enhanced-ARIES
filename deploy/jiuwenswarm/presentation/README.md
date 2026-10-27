# Jiuwen presentation dependency layer

Image8 has been built and published as `0.2.3-e2c9d519-image8-presentation1`.
[image.lock.json](image.lock.json) records the immutable registry reference and
the build, publication, dependency and native-source evidence. This recipe adds
a presentation toolchain to the pinned image7. It does not edit
Jiuwen source, native skills, the source dependency lock, or the native patch
manifest. Keep image7 available for earlier recordings.

The additional tools are Debian Node.js/npm, LibreOffice Impress, Poppler,
fontconfig, Noto CJK and Liberation fonts; PptxGenJS is pinned to 4.0.1. The native
`ppt-creation/scripts/opc/thumbnail.py` imports `fitz`, so the existing native
venv receives only the hash-pinned PyMuPDF 1.26.6 wheel, without dependency updates.
The other native finalize/OPC/thumbnail Python dependencies are already present.
This does not install optional network services, credentials, React/sharp visual
helpers, or every dependency of all native skills.

The image declares a `PATH` starting with `/opt/jiuwenswarm/.venv/bin` and
`NODE_PATH=/opt/aries-presentation/node_modules`. Docker ENV is not automatically
preserved through cloud envd and the contained process launcher. A's
`remote/worker.py:main` and B's `bootstrap.py:run` therefore prepend the native
venv directory and set `NODE_PATH` only when that presentation dependency
directory exists. Image7 keeps its original environment. Ordinary native shell
commands `python3` and `node` then resolve these dependencies without writing
into a skill directory or requiring explicit venv paths in model commands.

The native skill discovery input is:

```text
JIUWENSWARM_SHARED_SKILLS_DIRS=/opt/jiuwenswarm/jiuwenswarm/resources/agent/workspace/skills
```

This exposes the 20 original builtin skill directories to `SkillUseRail`; it is
independent of the optional `flash.skill_selection` retrieval extension. It does
not copy PPT resources into an Agent's private home. In external placement,
both Agent and task Sandbox must use the same pinned image/resource paths.
Registration does not prove that every skill's services or dependencies work.

Build, install, format and test only on the designated ECS. The initial build
resolves the pinned npm dependency into `package-lock.json`; the frozen lock is
then retained with this recipe and used by `npm ci` on subsequent builds.
The image records its complete Debian package versions and npm dependency tree
under `/opt/aries-presentation/`. No image tar transfer is needed.

`verify_presentation.py` runs as UID 60000 with a read-only image and no network.
It checks the native 20-skill catalog, actually reads `ppt-creation` using the
registered `skill_tool`, and confirms that no PPT private copy was installed.
It generates a small unscored fixture with PptxGenJS, uses the original native
template finalizer and thumbnail script, then renders through LibreOffice and
Poppler and checks three pages plus Chinese/English text. No model is called.
The fixture is an environment check, not an answer for a benchmark or user task.

The build receipt binds the tested immutable image ID, recipe manifest, native
source/resource hashes, unchanged existing Python package versions, and fixture
receipt SHA-256. Registry publication and cloud templates are separate steps.

The 2026-10-27 ECS build and UID 60000 fixture passed. The tested immutable
image is `sha256:c6eca9c7caeb67f544148df444b5bfb0e1f2be2250090e530d0e619ec83a33dd`;
the exact receipts and hashes are recorded in [local-build-evidence.json](local-build-evidence.json).
The rendered content page was also visually checked: Chinese and English are
legible. The original cover and closing template remain unchanged. Available
font substitutions do not establish pixel equivalence to Microsoft fonts.

The focused ECS entrance regression also passed on image8 and image7. Starting
from the launcher's minimal environment, the actual staged A worker main and
B bootstrap configuration both ran as UID 60000. Native `execute_cmd` and
`FlashBashTool` successfully used ordinary `python3` to import `pptx`, `fitz`,
`PIL` and `lxml`, and ordinary `node` to load PptxGenJS. Exit status 7 and native
cancellation checks passed. B discovered all 20 skills and read PPT through the
registered `skill_tool`; only its final AgentServer exec was mocked. Image7's
PATH and absent `NODE_PATH` stayed unchanged. The exact runtime receipt is pinned
in [image.lock.json](image.lock.json); this fixture made no model calls.

The E2B/Huawei **4 vCPU / 8192 MiB** templates passed actual native runtime
capability probes using the frozen entrance fixture above. Both clouds verified
the native A/B dependency paths, UID 60000 and exit/cancellation behavior, plus
B's 20-skill catalog with PPT `skill_tool` access. Provider-default envd commands
still use the system Python without these Python packages or `NODE_PATH`; that
diagnostic is recorded separately from the native session result. Both probe
sandboxes were deleted and independently confirmed absent. Exact receipts and
resource readbacks are in [templates.lock.json](templates.lock.json).

These cloud probes made no model calls and retained the mocked final B
AgentServer exec. The four real-cloud `llm-development-pptx-v1` Agent task runs
remain **UNRUN**. The generation and entrance fixtures do not establish those
task outcomes or complete dependencies for all registered skills.

The retained [package-lock.json](package-lock.json) is the byte-for-byte npm lock
produced by that build. [debian-packages.tsv](debian-packages.tsv) records every
installed Debian package version, and [npm-installed.json](npm-installed.json)
records the resolved npm tree. These files describe the tested image; the APT
recipe uses the configured Debian repositories, rather than a frozen archive
snapshot. Use the immutable published image to reproduce the exact environment.
[native-skills-sha256.json](native-skills-sha256.json) records all 237 native skill
files; all 2,552 native source/resource files were compared with image7 and were
unchanged. The original native dependency lock and patch manifest also match.

| Dependency | Tested version |
| --- | --- |
| Node.js | 18.20.4+dfsg-1~deb12u3 |
| npm | 9.2.0~ds1-1 |
| PptxGenJS | 4.0.1 |
| LibreOffice Impress | 4:7.4.7-1+deb12u14 |
| Poppler utilities | 22.12.0-2+deb12u3 |
| Noto CJK fonts | 1:20220127+repack1-1 |
| Liberation fonts | 2.1.5-1 |
| PyMuPDF | 1.26.6 |

The first candidate failed the existing image ownership check because APT
created `/usr/local/share/fonts` as `root:staff`. The second build adds a single
layer restoring `root:root` and mode `0755`, and reuses the dependency layers.
The failed receipt remains separate; all five validation containers were removed.
