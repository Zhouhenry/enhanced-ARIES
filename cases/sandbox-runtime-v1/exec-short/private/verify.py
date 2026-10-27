"""Private artifact/receipt checks; does not certify native invocation evidence."""

import argparse
import hashlib
import json
from pathlib import Path


SEED = 20261024
CALLS = {"small": 24, "medium": 96, "large": 384}


def verify(candidate, profile, receipts):
    count = CALLS[profile]
    checks = []

    def check(name, passed):
        checks.append({"name": name, "passed": bool(passed)})

    steps = Path(candidate) / "steps"
    expected_names = {f"{i:06d}.txt" for i in range(count)}
    directory_ok = not Path(candidate).is_symlink() and not steps.is_symlink() and steps.is_dir()
    check("steps_directory", directory_ok)
    if directory_ok:
        try:
            check("exact_step_paths", {p.name for p in steps.iterdir()} == expected_names)
        except OSError:
            check("exact_step_paths", False)

    receipt_map = {}
    receipts_ok = isinstance(receipts, list) and len(receipts) == count
    if isinstance(receipts, list):
        for entry in receipts:
            index = entry.get("index") if isinstance(entry, dict) else None
            if type(index) is not int or index < 0 or index >= count or index in receipt_map:
                receipts_ok = False
                continue
            receipt_map[index] = entry
    check("receipt_indices", receipts_ok and set(receipt_map) == set(range(count)))

    previous = "START"
    for index in range(count):
        previous = hashlib.sha256(f"{previous}:{index}:{SEED}".encode("utf-8")).hexdigest()
        path = steps / f"{index:06d}.txt"
        file_ok = False
        if directory_ok:
            try:
                file_ok = not path.is_symlink() and path.is_file() and path.read_bytes() == previous.encode("ascii")
            except OSError:
                pass
        check(f"step_{index}", file_ok)

        entry = receipt_map.get(index, {})
        receipt_ok = type(entry.get("exit_code")) is int and entry["exit_code"] == 0
        try:
            output = json.loads(entry["stdout"]) if isinstance(entry.get("stdout"), str) else None
        except (ValueError, TypeError):
            output = None
        receipt_ok = receipt_ok and isinstance(output, dict) and type(output.get("index")) is int
        receipt_ok = receipt_ok and output == {"index": index, "digest": previous}
        check(f"receipt_{index}", receipt_ok)

    return {
        "schema": "asb.exec-short.verification.v1",
        "case_id": "asb-exec-short-v1",
        "profile": profile,
        "seed": SEED,
        "expected_calls": count,
        "functional_pass": all(item["passed"] for item in checks),
        "checks": checks,
        "execution_evidence": {
            "status": "NOT_EVALUATED",
            "reason": "Artifact/receipt checks cannot establish actual native call count, identity or process timing."
        },
        "performance_qualified": None,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidate", required=True, type=Path)
    parser.add_argument("--profile", required=True, choices=CALLS)
    parser.add_argument("--receipts", required=True, type=Path)
    args = parser.parse_args()
    receipt_error = None
    try:
        receipts = json.loads(args.receipts.read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        receipts = None
        receipt_error = type(exc).__name__
    report = verify(args.candidate, args.profile, receipts)
    if receipt_error:
        report["receipt_input_error"] = receipt_error
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if report["functional_pass"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
