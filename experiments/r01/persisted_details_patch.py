"""Preserve small terminal-protocol evidence through native transcript caps."""
import argparse
import hashlib
import json
from pathlib import Path
import sys

MODULE = "attempt.model-diagnostic-events-Dg8sP6iR.js"
SOURCE_SHA256 = "4cc02f9636ae2f35f5b41edb44e09ae80238d169b3d14bfbab37eeebd52e2e08"


def patch(text):
    for function, following, indent in (
        ("buildPersistedDetailsFallback", "enforcePersistedDetailsByteCap", "\t\t\t"),
        ("sanitizeToolResultDetailsForPersistence", "capToolResultDetails", "\t\t"),
    ):
        start = text.index("function " + function + "(")
        end = text.index("function " + following + "(", start)
        original = text[start:end]
        anchor = indent + '"exitSignal",'
        if original.count(anchor) != 1:
            raise ValueError("persisted details patch anchor is not unique: " + function)
        replacement = anchor + "".join("\n" + indent + '"' + field + '",' for field in (
            "protocol", "outputRenderer", "outputTruncated"))
        text = text[:start] + original.replace(anchor, replacement, 1) + text[end:]
    return text


def main():
    if not sys.platform.startswith("linux"):
        raise SystemExit("authorized Linux ECS only")
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    raw = (args.source / MODULE).read_bytes()
    if hashlib.sha256(raw).hexdigest() != SOURCE_SHA256:
        raise ValueError("pinned native persistence source SHA differs")
    candidate = patch(raw.decode()).encode()
    args.output.mkdir(mode=0o755, parents=True, exist_ok=False)
    (args.output / MODULE).write_bytes(candidate)
    (args.output / MODULE).chmod(0o644)
    (args.output / "manifest.json").write_text(json.dumps({
        "module": MODULE, "input_sha256": SOURCE_SHA256,
        "output_sha256": hashlib.sha256(candidate).hexdigest(),
        "preserved_fields": ["protocol", "outputRenderer", "outputTruncated"],
        "unchanged": ["8192-byte cap", "content and aggregated output rules", "redaction",
                      "live ToolResult", "default results without these fields"],
    }, indent=2) + "\n")


if __name__ == "__main__":
    main()
