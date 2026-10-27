"""Controller-owned inventory, executed only after the Runner stop/revoke gate."""
import hashlib
import json
import os
import pathlib
import stat
import sys

MAX_FILES = 128
MAX_FILE_BYTES = 64 << 20
MAX_TOTAL_BYTES = 512 << 20


def capture(workspace):
    if not workspace.is_dir() or workspace.is_symlink() or workspace.resolve(strict=True) != workspace:
        raise ValueError("PPTX workspace must be a real absolute directory")
    files = []
    total = 0
    def fail_scan(error):
        raise error
    for directory, subdirectories, names in os.walk(workspace, followlinks=False, onerror=fail_scan):
        for name in subdirectories:
            if pathlib.Path(directory, name).is_symlink():
                raise ValueError("PPTX inventory cannot traverse a symlink directory")
        for name in sorted(names):
            if not name.lower().endswith(".pptx"):
                continue
            candidate = pathlib.Path(directory, name)
            info = candidate.lstat()
            if not stat.S_ISREG(info.st_mode):
                raise ValueError("PPTX artifact must be a regular file")
            if len(files) >= MAX_FILES or info.st_size > MAX_FILE_BYTES:
                raise ValueError("PPTX artifact count or individual size exceeds capture limit")
            total += info.st_size
            if total > MAX_TOTAL_BYTES:
                raise ValueError("PPTX artifact total size exceeds capture limit")
            digest = hashlib.sha256()
            with candidate.open("rb") as source:
                for block in iter(lambda: source.read(1 << 20), b""):
                    digest.update(block)
            files.append({"path": candidate.relative_to(workspace).as_posix(),
                          "size_bytes": info.st_size, "sha256": digest.hexdigest()})
    return sorted(files, key=lambda item: item["path"])


def main():
    result = {"schema_version": 1, "complete": False, "files": [], "error": ""}
    try:
        result["files"] = capture(pathlib.Path(sys.argv[1]))
        result["complete"] = True
    except (OSError, ValueError) as exc:
        result["error"] = str(exc)
    destination = pathlib.Path(sys.argv[2])
    destination.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with destination.open("x", encoding="utf-8") as output:
        json.dump(result, output, ensure_ascii=False, sort_keys=True)
    return 0 if result["complete"] else 1


if __name__ == "__main__":
    sys.exit(main())
