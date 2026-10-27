"""Extract resource summaries from frozen snapshots on ECS; never run workloads."""
import argparse
from collections import Counter
import hashlib
import json
import math
from pathlib import Path
import sys


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def checked(path, expected):
    raw = path.read_bytes()
    require(digest(raw) == expected, f"SHA mismatch: {path}")
    return raw


def evidence(sample):
    keys = ("sample_id", "evidence_ref", "source_line", "reading_index", "time_status",
            "observed_at_utc", "provider_timestamp", "requested_at_utc", "sampling_started_offset_ns", "sampling_finished_offset_ns")
    return {key: sample[key] for key in keys if key in sample}


def summarize(stream, category):
    allowed = {"controller": {"cpu_user_ns": "ns", "cpu_system_ns": "ns", "heap_alloc_bytes": "bytes",
                              "process_lifetime_peak_rss_bytes": "bytes"},
               "harness": {"cpu_usage_nanoseconds": "ns", "memory_usage_bytes": "bytes", "memory_limit_bytes": "bytes"},
               "provider": {"cpu_used_pct": "percent", "mem_used": "bytes", "mem_used_mib": "MiB"}}[category]
    samples, metrics = stream["samples"], {}
    for name, unit in allowed.items():
        points, seen, missing = [], {}, 0
        for sample in samples:
            values = [v for v in sample["values"] if v["name"] == name]
            require(len(values) <= 1, "Duplicate metric in one sample")
            if not values or values[0]["unit"] != unit or not values[0]["export_eligible"]:
                missing += 1
                continue
            value = values[0]["value"]
            require(type(value) in (int, float) and math.isfinite(value) and value >= 0, "Invalid resource number")
            stamp = sample.get("provider_timestamp") if stream["source"] == "agentsphere_gateway_envd_metrics" else sample.get("observed_at_utc")
            # Unknown timestamps remain separate; Huawei timestamps are opaque keys, never epochs.
            key = json.dumps(stamp, sort_keys=True) if stamp is not None else sample["sample_id"]
            readings = seen.setdefault(key, [])
            duplicate = next((p for p in readings if p["value"] == value), None)
            if duplicate is not None:
                duplicate["sample_ids"].append(sample["sample_id"])
                continue
            point = {"value": value, "evidence": evidence(sample), "sample_ids": [sample["sample_id"]]}
            readings.append(point)
            points.append(point)
        conflicts = [{"timestamp": group[0]["evidence"].get("provider_timestamp", group[0]["evidence"].get("observed_at_utc")),
                      "readings": group} for group in seen.values() if len(group) > 1]
        entry = {"name": name, "unit": unit, "reading_count": len(samples) - missing,
                 "unique_point_count": len(points), "duplicate_reading_count": len(samples) - missing - len(points),
                 "missing_or_unverified_unit_count": missing, "min": None, "max": None, "points": points,
                 "aggregation": "returned_reading_extrema_no_time_series", "extrema_scope": "all_returned_readings",
                 "conflicting_timestamp_count": len(conflicts), "timestamp_conflicts": conflicts,
                 "status": "present_with_timestamp_conflicts" if conflicts else "present" if points else "not_collected"}
        if points:
            entry.update(min=min(points, key=lambda p: p["value"]), max=max(points, key=lambda p: p["value"]))
        if category == "harness" and name == "cpu_usage_nanoseconds":
            require(all(b["value"] >= a["value"] for a, b in zip(points, points[1:])), "Harness CPU counter decreased")
            entry["observed_window_delta_ns"] = points[-1]["value"] - points[0]["value"] if len(points) > 1 else None
            entry["first"], entry["last"] = (points[0], points[-1]) if points else (None, None)
        metrics[name] = entry
    keys = ("stream_id", "source", "measurement_scope", "owner_id", "attempt_id", "purpose", "component",
            "clock_domain", "time_basis", "nominal_interval_seconds", "anchor_utc", "integrity_status")
    result = {key: stream[key] for key in keys if key in stream}
    result.update(category=category, sample_count=len(samples), metrics=metrics,
                  observation_window={"first": evidence(samples[0]) if samples else None, "last": evidence(samples[-1]) if samples else None},
                  sample_status_counts=dict(Counter(s["status"] for s in samples)),
                  time_status_counts=dict(Counter(s["time_status"] for s in samples)),
                  empty_value_sample_count=sum(not s["values"] for s in samples), evidence_refs=stream["evidence_refs"])
    if category == "controller":
        result["delta"] = [{k: v[k] for k in ("name", "value", "unit", "kind")} for v in stream.get("delta", [])
                           if v["name"] in ("cpu_user_ns", "cpu_system_ns", "total_alloc_bytes", "mallocs", "num_gc", "gc_pause_total_ns")]
        cpu = {v["name"]: v["value"] for v in result["delta"] if v["unit"] == "ns"}
        result["cpu_time_ns"] = cpu["cpu_user_ns"] + cpu["cpu_system_ns"] if {"cpu_user_ns", "cpu_system_ns"} <= cpu.keys() else None
    result["semantics"] = {"controller": "Shared controller process CPU delta; RSS is process lifetime high-water mark, not task peak.",
                           "harness": "Container CPU delta covers first-to-last observation only; memory maximum is sampled, not true peak.",
                           "provider": "Native gauges only; min/max cover all returned readings, including timestamp conflicts, not true peaks. No valid time series, CPU integration or per-Tool attribution is claimed."}[category]
    return result


def extract(run, source):
    artifact = Path(run["artifact_root"]).resolve()
    require(artifact.is_relative_to(Path("/opt/aries-artifacts")), "Expected ECS artifact root")
    path = Path(source["path"])
    require(path.resolve() == artifact / "analysis-snapshot.json", "Unexpected snapshot path")
    snapshot = json.loads(checked(path, source["sha256"]))
    require(snapshot["run_id"] == run["run_id"] and snapshot["snapshot_id"] == run["snapshot"]["id"], "Snapshot identity mismatch")
    require(snapshot["integrity_status"] == "complete", "Snapshot incomplete")
    attempts = [a for a in snapshot["attempts"] if a["attempt_id"] == run["attempt_id"]]
    require(len(attempts) == 1 and attempts[0]["occurrence_id"] == run["occurrence_id"], "Attempt identity mismatch")
    require(snapshot["resources"]["schema_version"] == 1, "Unsupported resources schema")
    root, occurrence = artifact / "runs" / run["run_id"], run["occurrence_id"]
    allowed = {"controller-resources.start.json", "controller-resources.end.json"} | {
        occurrence + "/" + name for name in ("monitor/resources.jsonl", "monitor/index.json", "sandbox-metrics.jsonl", "cloud-observation.json")}
    sources = {s["evidence_ref"]: s for s in snapshot["sources"]}
    streams, receipts, raw_files = [], {}, {}
    for stream in snapshot["resources"]["streams"]:
        scope = stream["measurement_scope"]
        category = {"controller_shared_process_self": "controller", "container_resource": "harness", "sandbox_provider": "provider"}[scope]
        require(stream["integrity_status"] == "complete", "Resource stream incomplete")
        require(category == "controller" or stream.get("attempt_id") == run["attempt_id"], "Resource attempt mismatch")
        require(category != "harness" or stream.get("component") == "harness", "Unexpected container owner")
        for ref in stream["evidence_refs"]:
            src = sources[ref]
            require(src["file"] in allowed, "Unexpected resource source")
            location = (root / src["file"]).resolve()
            require(location.is_relative_to(root.resolve()), "Resource path escaped run root")
            if ref not in receipts:
                raw_files[ref] = checked(location, src["sha256"])
                require(len(raw_files[ref]) == src["bytes"], "Resource byte count mismatch")
                receipts[ref] = {"path": str(location), "sha256": src["sha256"], "bytes": src["bytes"]}
        require(all(s["evidence_ref"] in stream["evidence_refs"] for s in stream["samples"]), "Unverified sample source")
        streams.append(summarize(stream, category))
    unsupported, provider_receipts = set(), 0
    for ref, raw in raw_files.items():
        if not sources[ref]["file"].endswith("/sandbox-metrics.jsonl"):
            continue
        for line in raw.splitlines():
            row = json.loads(line)
            require(row["run_id"] == run["run_id"] and row["occurrence_id"] == occurrence, "Provider receipt identity mismatch")
            provider_receipts += 1
            unsupported.update(set(row.get("snapshot", {}).get("unsupported", [])) & {"disk_io", "network_io", "agent_tool_resource_split"})
    return {"run_id": run["run_id"], "attempt_id": run["attempt_id"], "snapshot_id": snapshot["snapshot_id"],
            "snapshot_source": {"path": str(path), "sha256": source["sha256"]}, "sources": receipts, "streams": streams,
            "provider_receipt_count": provider_receipts, "unsupported": sorted(unsupported),
            "io_status": "unsupported" if "disk_io" in unsupported else "not_collected",
            "network_status": "unsupported" if "network_io" in unsupported else "not_collected"}


def main():
    require(sys.platform.startswith("linux"), "Run only on the authorized ECS")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", type=Path, default=Path(__file__).with_name("data.json"))
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    raw = args.data.read_bytes()
    data = json.loads(raw)
    require(len(data["runs"]) == 3 and len({r["id"] for r in data["runs"]}) == 3, "Expected three distinct runs")
    result = {"schema_version": 1, "status": "PASS_EXISTING_RESOURCE_EXTRACTION_ONLY", "data_sha256": digest(raw), "script_sha256": digest(Path(__file__).read_bytes()),
              "runs": {r["id"]: extract(r, data["sources"][r["id"] + "/analysis-snapshot.json"]) for r in data["runs"]},
              "limitations": ["Existing frozen snapshots only; no event rematching or new workloads.",
                              "Owners remain separate; sampled maxima do not establish task peaks; absent values are not zero.",
                              "Huawei provider timestamps remain unverified and are used only as opaque deduplication keys.",
                              "Timestamp conflicts retain every distinct value and evidence; extrema are over returned readings, not a validated time series or metrics-export acceptance."]}
    if any(m["conflicting_timestamp_count"] for r in result["runs"].values() for s in r["streams"] for m in s["metrics"].values()):
        result["status"] = "PASS_EXISTING_RESOURCE_EXTRACTION_WITH_DISCLOSED_CONFLICTS"
    with args.output.open("x", encoding="utf-8") as output:
        json.dump(result, output, ensure_ascii=False, indent=2, allow_nan=False)
        output.write("\n")


if __name__ == "__main__":
    main()
