"""Binary-safe argv execution inside the sandbox; stdout is a framed control stream."""
import base64
import json
import os
import selectors
import subprocess
import sys
import threading
import time
import unicodedata


def strict_json_loads(value):
    def object_fields(pairs):
        result = {}
        for key, item in pairs:
            if key in result:
                raise ValueError("duplicate_json_key")
            result[key] = item
        return result

    def invalid_constant(_):
        raise ValueError("invalid_json_constant")

    return json.loads(value, object_pairs_hook=object_fields, parse_constant=invalid_constant)


def validate_observation(value, process_id=None):
    fields = {"scope_id", "agent_session_id", "tool_call_id", "native_invocation_id", "ssh_request_id"}
    if (not isinstance(value, dict)
            or set(value) != {"schema_version", "native_identity", "execution_id", "process_id"}
            or type(value["schema_version"]) is not int or value["schema_version"] != 1):
        raise ValueError("invalid_observation")
    native = value["native_identity"]
    if (not isinstance(native, dict) or set(native) != fields | {"schema_version"}
            or type(native["schema_version"]) is not int or native["schema_version"] != 1):
        raise ValueError("invalid_observation")
    for item in [*(native[key] for key in fields), value["execution_id"], value["process_id"]]:
        if (not isinstance(item, str) or not item or len(item.encode("utf-8")) > 256
                or item.strip() != item or any(unicodedata.category(char) == "Cc" for char in item)):
            raise ValueError("invalid_observation")
    if process_id is not None and value["process_id"] != process_id:
        raise ValueError("observation_process_mismatch")
    return {**value, "native_identity": dict(native)}


def main():
    spec = strict_json_loads(base64.b64decode(sys.argv[1], validate=True))
    observation = validate_observation(spec["observation"]) if "observation" in spec else None

    def emit(frame):
        if observation is not None:
            frame = {**frame, "observation": observation}
        print(json.dumps(frame, ensure_ascii=observation is None), flush=True)

    if observation is not None:
        emit({"kind": "execution_identity", "phase": "before_popen"})
    # stdin is inherited from envd, including an explicit EOF from close_stdin.
    started = time.monotonic_ns()
    child = subprocess.Popen(spec["argv"], cwd=spec.get("cwd") or None,
                             env={**os.environ, **spec.get("env", {})},
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    popen_return_ns = time.monotonic_ns() - started
    # Reap the direct child exactly once, independently of pipe draining.  A
    # descendant may retain either pipe after the direct child exits; observing
    # that exit separately keeps the two boundaries explicit without changing
    # argv, signals, stdio or the command's exit status.
    child_exit = {"code": None, "observed_ns": None}
    child_waited = threading.Event()

    def reap_child():
        child_exit["code"] = child.wait()
        child_exit["observed_ns"] = time.monotonic_ns() - started
        child_waited.set()

    threading.Thread(target=reap_child, name="aries-guest-reaper", daemon=False).start()
    first_output_ns = None
    selector = selectors.DefaultSelector()
    selector.register(child.stdout, selectors.EVENT_READ, "stdout")
    selector.register(child.stderr, selectors.EVENT_READ, "stderr")
    while selector.get_map():
        for key, _ in selector.select():
            chunk = os.read(key.fileobj.fileno(), 16384)
            if not chunk:
                selector.unregister(key.fileobj)
                key.fileobj.close()
                continue
            if first_output_ns is None:
                first_output_ns = time.monotonic_ns() - started
            emit({"stream": key.data, "data_base64": base64.b64encode(chunk).decode()})
    pipe_eof_ns = time.monotonic_ns() - started
    child_waited.wait()
    exit_code = child_exit["code"]
    # Wall time includes pipe draining, including descendants retaining a pipe.
    duration = time.monotonic_ns() - started
    emit({"exit_code": exit_code, "guest_duration_ns": duration,
          "popen_return_ns": popen_return_ns,
          "first_output_ns": first_output_ns,
          "child_exit_observed_ns": child_exit["observed_ns"],
          "pipe_eof_ns": pipe_eof_ns})


if __name__ == "__main__":
    main()
