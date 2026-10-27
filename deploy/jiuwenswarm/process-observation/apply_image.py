"""Install the process candidate over exact image5 and append its patch receipt."""
import argparse
import difflib
import hashlib
import json
from pathlib import Path
import shutil


ROOT = Path("/opt/jiuwenswarm")
PURELIB = ROOT / ".venv/lib/python3.12/site-packages"
INPUT = Path("/opt/aries-template/process-candidate")
BASE_MANIFEST_SHA = "99c7de4f0e221e446fe348d96dd51e22d58523b1f391fcecf0b041f868765bd3"


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-image", required=True)
    args = parser.parse_args()
    receipt = ROOT / "aries-patches.json"
    assert sha(receipt) == BASE_MANIFEST_SHA, "image5 patch manifest mismatch"
    overlay = json.loads(receipt.read_text())
    assert [p["name"] for p in overlay["patches"]] == ["agent-core-tool-context", "jiuwenswarm-remote-tools"]
    for prior in overlay["patches"]:
        assert sha(Path(prior["patch_file"])) == prior["patch_sha256"]
        for name, pins in prior["files"].items():
            assert sha(Path(name)) == pins["after_sha256"]
    build = ROOT / "aries-build.json"
    assert sha(build) == overlay["base_build_sha256"]
    candidate = json.loads((INPUT / "candidate/manifest.json").read_text())
    assert candidate["upstream_revision"] == "3460150aa5f319b0b8f84a4ed10eec0a08b0feb1"
    files = {}
    patch_text = ""
    entries = [(INPUT / "candidate" / relative, PURELIB / relative)
               for relative in candidate["files"]]
    entries.append((INPUT / "verify.py", Path("/opt/aries-template/verify.py")))
    for source, target in entries:
        relative = str(source.relative_to(INPUT / "candidate")) if source.is_relative_to(INPUT / "candidate") else None
        if relative is not None:
            assert sha(source) == candidate["files"][relative]
        before = target.read_bytes() if target.exists() else b""
        if target.name == "shell_operation.py":
            assert hashlib.sha256(before).hexdigest() == candidate["baseline_sha256"]
        elif target.name == "aries_process_observer.py":
            assert not target.exists(), "process overlay already exists"
        elif target.name == "verify.py":
            assert hashlib.sha256(before).hexdigest() == "4ed042d976fa96fe9003678eceb437862f04bcca7694af9f64d6e28023b62217"
        after = source.read_bytes()
        patch_text += "".join(difflib.unified_diff(before.decode().splitlines(keepends=True),
                             after.decode().splitlines(keepends=True),
                             fromfile="a" + str(target) if before else "/dev/null", tofile="b" + str(target)))
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
        target.chmod(0o444)
        files[str(target)] = {"before_sha256": hashlib.sha256(before).hexdigest() if before else None,
                              "after_sha256": sha(target)}
    archive = Path("/opt/aries-template/native-patches")
    archive.mkdir(exist_ok=True)
    archive.chmod(0o755)
    preserved = archive / "image5-aries-patches.json"
    assert not preserved.exists()
    shutil.copyfile(receipt, preserved)
    preserved.chmod(0o444)
    patch = archive / "agent-core-process-observation.patch"
    assert not patch.exists()
    patch.write_text(patch_text)
    patch.chmod(0o444)
    added = {"name": "agent-core-process-observation", "upstream_revision": candidate["upstream_revision"],
             "patch_file": str(patch), "patch_sha256": sha(patch), "files": files}
    overlay["patches"].append(added)
    overlay["process_base_image_reference"] = args.base_image
    overlay["process_base_patch_manifest_sha256"] = BASE_MANIFEST_SHA
    receipt.write_text(json.dumps(overlay, indent=2, sort_keys=True) + "\n")
    receipt.chmod(0o444)
    assert sha(build) == overlay["base_build_sha256"]
    print(json.dumps({"status": "PROCESS_OVERLAY_INSTALLED", "native_patch_manifest_sha256": sha(receipt),
                      "patches": [p["name"] for p in overlay["patches"]]}))


if __name__ == "__main__":
    main()
