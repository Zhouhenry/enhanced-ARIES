"""ECS-only assembly of a self-contained, offline dashboard; no network access."""
from pathlib import Path
import json
import sys
import hashlib

if not sys.platform.startswith("linux"):
    raise SystemExit("Build on the authorized ECS")

root = Path(__file__).resolve().parent
data = json.loads((root / "data.json").read_text())
actions = json.loads((root / "case-actions.json").read_text())
assert actions["tape_sha256"] == data["experiment"]["tape_sha256"]
assert len(actions["actions"]) == 20
action_ids = {a["tool_call_id"]: a for a in actions["actions"]}
for tool in data["tool_calls"]:
    action = action_ids[tool["tool_call_id"]]
    assert action["exec_number"] == tool["exec_number"] and action["model_turn"] == tool["model_turn"]
    assert all(m["exit_code"] == action["wrapper_exit_code"] for m in tool["measurements"].values())
    tool["action"] = action
data["case"] = actions["case"]
data["action_sources"] = actions["sources"]
if "trace_views" not in data:
    data["trace_views"] = json.loads((root / "trace_views.json").read_text())["trace_views"]
assert len(data["trace_views"]) == 6
data["sources"]["dashboard/case-actions.json"] = {"path": "case-actions.json", "sha256": hashlib.sha256((root / "case-actions.json").read_bytes()).hexdigest()}
template = (root / "template.html").read_text()
assert template.count("__ARIES_TIME_HELP__") == 1
template = template.replace("__ARIES_TIME_HELP__", (root / "time-help.html").read_text())
assert template.count("__ARIES_RESOURCE_VIEW__") == 1
template = template.replace("__ARIES_RESOURCE_VIEW__", (root / "resource-view.html").read_text())
assert template.count("__ARIES_TRACE_VIEW__") == 1
template = template.replace("__ARIES_TRACE_VIEW__", (root / "trace-view.html").read_text())
marker = "__ARIES_DASHBOARD_DATA__"
assert template.count(marker) == 1
assert len(data["runs"]) == 3 and len(data["tool_calls"]) == 20
assert all(r["version"]["tape_sha256"] == data["experiment"]["tape_sha256"] for r in data["runs"])
payload = json.dumps(data, ensure_ascii=False).replace("<", "\\u003c")
(root / "index.html").write_text(template.replace(marker, payload))
(root / "data.json").write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
print("Built offline index.html")
