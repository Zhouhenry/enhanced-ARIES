"""Generate a separate pinned candidate; never modify the supplied native source."""
import argparse
import difflib
import hashlib
import json
from pathlib import Path
import shutil


REVISION = "3460150aa5f319b0b8f84a4ed10eec0a08b0feb1"
BASELINE_SHA256 = "62e4b0bc5198629177bc502acc66a171b577552c8ba5870f4339aec5888ceaf9"
RELATIVE = Path("openjiuwen/core/sys_operation/local/shell_operation.py")
MODULE = "openjiuwen.core.sys_operation.local.aries_process_observer"


def sha(data):
    return hashlib.sha256(data).hexdigest()


def prepare(source, output):
    original = source.read_bytes()
    if sha(original) != BASELINE_SHA256:
        raise ValueError("pinned shell_operation.py hash mismatch")
    text = original.decode("utf-8")
    insertion = "import asyncio\n"
    if text.count(insertion) != 1:
        raise ValueError("unexpected asyncio import")
    candidate = text.replace(insertion, insertion +
        "from openjiuwen.core.sys_operation.local import aries_process_observer\n", 1)
    for kind in ("shell", "exec"):
        old = "return await asyncio.create_subprocess_" + kind + "("
        if candidate.count(old) != 1:
            raise ValueError("unexpected subprocess creation sites")
        candidate = candidate.replace(old, "return await aries_process_observer.create_subprocess_" + kind + "(", 1)
    output.mkdir(parents=True, exist_ok=False)
    target = output / RELATIVE
    target.parent.mkdir(parents=True)
    target.write_text(candidate, encoding="utf-8")
    observer = Path(__file__).with_name("native_process.py")
    installed = target.with_name("aries_process_observer.py")
    shutil.copyfile(observer, installed)
    patch = "".join(difflib.unified_diff(text.splitlines(keepends=True),
                        candidate.splitlines(keepends=True), fromfile="a/" + str(RELATIVE),
                        tofile="b/" + str(RELATIVE)))
    (output / "native-process.patch").write_text(patch, encoding="utf-8")
    manifest = {"schema_version": 1, "kind": "jiuwen_native_process_candidate",
                "upstream_revision": REVISION, "installed_module": MODULE,
                "baseline_sha256": BASELINE_SHA256,
                "files": {str(RELATIVE): sha(target.read_bytes()),
                          str(RELATIVE.with_name("aries_process_observer.py")): sha(installed.read_bytes())},
                "patch_sha256": sha(patch.encode()), "candidate_only": True}
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    return manifest


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", type=Path, required=True, help="exact pinned shell_operation.py")
    parser.add_argument("--output", type=Path, required=True, help="new candidate directory")
    args = parser.parse_args()
    prepare(args.source, args.output)
