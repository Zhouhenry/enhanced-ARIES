"""Prepare an allowlisted, credential-free Adapter build context on Linux ECS."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sys

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--wheels-dir", required=True, type=Path)
parser.add_argument("--output", type=Path, default=Path(".cache/agentsphere-adapter-context"))
args = parser.parse_args()
if sys.platform != "linux":
    parser.error("build preparation runs only on the Linux ECS")
repo = Path(__file__).resolve().parents[2]
sources = {
    "Dockerfile": repo / "adapters/agentsphere/Dockerfile",
    "requirements.txt": repo / "adapters/agentsphere/requirements.txt",
    "e2b/server.py": repo / "adapters/e2b/server.py",
    "e2b/guest_exec.py": repo / "adapters/e2b/guest_exec.py",
    "agentsphere/agentsphere.py": repo / "adapters/agentsphere/agentsphere.py",
}
wheels = {
    "e2b-2.42.0-py3-none-any.whl": "e0bb408fa1ee9675e8ca999e47d9c548dd47e3073a96378532329abb7899d194",
    "agentsphere_e2b_patch-0.1.0-py3-none-any.whl": "9fd380dbccfa8b86e6249722c273600824f42df6974d1fd35e918ca18751ffc4",
}
sources.update({name: args.wheels_dir / name for name in wheels})
contents, manifest = {}, {}
for name, path in sources.items():
    if path.is_symlink() or not path.is_file():
        parser.error("build input is not a regular file: " + name)
    data = path.read_bytes()
    digest = hashlib.sha256(data).hexdigest()
    if name in wheels and digest != wheels[name]:
        parser.error("wheel checksum mismatch: " + name)
    contents[name], manifest[name] = data, digest
os.umask(0o077)
args.output.mkdir(mode=0o700)
for name, data in contents.items():
    target = args.output / name
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(data)
    assert hashlib.sha256(target.read_bytes()).hexdigest() == manifest[name]
(args.output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
print(json.dumps({"status": "PREPARED", "files": len(contents), "output": str(args.output)}))
