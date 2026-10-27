"""Offline provider-contract tests; no cloud, SDK execution, or credentials."""
import copy
import base64
import contextlib
import hashlib
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest

from agentsphere import AgentSphereAdapter, Connection, MAX_JSON, UNITS, parse_network_isolation_assumption
from server import Adapter, Failure


class Missing(Exception):
    pass


class ProcessMissing(Exception):
    pass


class FakeConnection:
    gateway_url = "https://gateway.example"

    def __init__(self):
        self.calls = []
        self.present = False
        self.fail_create = False
        self.fail_connect = False
        self.fail_kill = False
        self.create_status, self.readback_status, self.metrics_status = 201, 200, 200
        self.before_connect = lambda: None
        self.created = {"sandboxID": "private-cloud-id", "envdVersion": "0.6.1",
                        "envdAccessToken": "private-envd-token", "trafficAccessToken": "private-traffic-token"}
        self.metric = {"ts": 1792565546, "cpu_count": 2, "cpu_used_pct": 1.83,
                       "mem_total_mib": 7963, "mem_used_mib": 7629, "mem_total": 8350466048,
                       "mem_used": 8000483328, "mem_cache": 5177344, "disk_used": 1627455488,
                       "disk_total": 31660609536, "future_field": "preserved"}

    def request(self, method, path, **options):
        self.calls.append((method, path, options))
        if method == "POST":
            if self.fail_create:
                raise TimeoutError("private-error-body")
            self.present = True
            self.payload = options["payload"]
            return self.create_status, copy.deepcopy(self.created)
        if path == "/metrics":
            return self.metrics_status, copy.deepcopy(self.metric)
        return self.readback_status, {"templateID": "template", "buildID": "build", "timeout": self.payload["timeout"],
                     "lifecycle": {"onTimeout": "kill", "autoResume": False}}

    def connect(self, created, timeout):
        self.before_connect()
        if self.fail_connect:
            raise RuntimeError("private-connection-failure")
        return SimpleNamespace(sandbox_id=created["sandboxID"], traffic_access_token=created["trafficAccessToken"],
            connection_config=SimpleNamespace(sandbox_headers={"X-Access-Token": created["envdAccessToken"]}))

    def kill(self, sandbox_id, **options):
        self.calls.append(("kill", sandbox_id, options))
        if self.fail_kill:
            raise TimeoutError("private-delete-failure")
        self.present = False

    def get_info(self, sandbox_id, **options):
        self.calls.append(("get_info", sandbox_id, options))
        if not self.present:
            raise Missing()
        return object()

    def guest_headers(self, sandbox, port):
        return {"X-API-Key": "private-api-key", "X-Access-Token": "private-envd-token",
                "e2b-sandbox-id": sandbox.sandbox_id, "e2b-sandbox-port": str(port),
                "e2b-traffic-access-token": sandbox.traffic_access_token}


class Tests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.path = Path(self.directory.name) / "state.json"
        self.connection = FakeConnection()
        self.adapter = self.make_adapter()
        self.body = {"sandbox": "attempt_0123456789", "template": "template:build",
                     "timeout_seconds": 120, "allow_internet_access": True}

    def make_adapter(self, *, assume_network_isolation=False):
        adapter = AgentSphereAdapter(self.connection, Missing, Missing, ProcessMissing, templates=["template:build"],
                                     state_file=self.path, max_sandboxes=2,
                                     assume_network_isolation=assume_network_isolation)
        self.addCleanup(adapter.state_lock.close)
        return adapter

    def test_inherited_file_transfer_exceeds_old_16_mib_without_truncation(self):
        self.adapter.create(self.body)
        _, sandbox = self.adapter.entry(self.body)
        stored = {}
        def write(path, data, **_):
            stored[path] = data
        def read(path, **_):
            return contextlib.nullcontext([stored[path]])
        sandbox.files = SimpleNamespace(write=write, read=read)
        content = b"native-span-fixture\n" * 1000000
        expected = hashlib.sha256(content).digest()
        uploaded = self.adapter.dispatch("upload", {**self.body, "path": "/spans.jsonl",
            "data_base64": base64.b64encode(content).decode()})
        self.assertEqual(uploaded["bytes"], len(content))
        downloaded = self.adapter.dispatch("download", {**self.body, "path": "/spans.jsonl", "max_bytes": 64 << 20})
        actual = base64.b64decode(downloaded["data_base64"], validate=True)
        self.assertEqual(len(actual), len(content))
        self.assertEqual(hashlib.sha256(actual).digest(), expected)
        with self.assertRaisesRegex(Failure, "file_too_large"):
            self.adapter.dispatch("download", {**self.body, "path": "/spans.jsonl", "max_bytes": len(content)-1})

    def test_false_and_default_network_reject_before_any_allocation(self):
        for explicit in (False, None):
            body = dict(self.body)
            if explicit is None:
                del body["allow_internet_access"]
            else:
                body["allow_internet_access"] = explicit
            with self.assertRaisesRegex(Failure, "network_isolation_unsupported") as failure:
                self.adapter.create(body)
            self.assertEqual(failure.exception.status, 422)
        self.assertEqual(self.connection.calls, [])
        self.assertEqual(self.adapter.creates, 0)
        self.assertEqual(self.adapter.records, {})
        self.assertFalse(self.path.exists())
        health = self.adapter.dispatch("health", {})
        self.assertIs(health["assume_network_isolation"], False)
        self.assertEqual(health["verifier_egress_isolation"], "unsupported")

    def test_explicit_assumption_preserves_false_provider_request_and_labels_evidence(self):
        self.adapter.state_lock.close()
        self.adapter = self.make_adapter(assume_network_isolation=True)
        for index, explicit in enumerate((False, None)):
            body = {**self.body, "sandbox": "assumed_attempt_" + str(index)}
            if explicit is None:
                del body["allow_internet_access"]
            else:
                body["allow_internet_access"] = explicit
            result = self.adapter.create(body)
            self.assertIs(self.connection.payload["allow_internet_access"], False)
            for evidence in (result, self.adapter.dispatch("health", {}), self.adapter.dispatch("status", body)):
                self.assertIs(evidence["assume_network_isolation"], True)
                self.assertEqual(evidence["verifier_egress_isolation"], "assumed-unverified")
            self.assertTrue(self.adapter.kill(body)["absence_confirmed"])
        self.assertEqual(self.adapter.creates, 2)
        self.assertFalse(self.connection.present)

    def test_assumption_requires_exact_boolean_configuration(self):
        self.assertIs(parse_network_isolation_assumption("true"), True)
        self.assertIs(parse_network_isolation_assumption("false"), False)
        for value in ("", "True", "FALSE", "1", "yes", " false ", None, True):
            with self.subTest(value=value), self.assertRaises(ValueError):
                parse_network_isolation_assumption(value)
        for value in ("false", "true", 0, 1, None):
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.make_adapter(assume_network_isolation=value)
        self.assertEqual(self.connection.calls, [])
        self.assertFalse(self.path.exists())

    def test_identity_persisted_before_connect_and_metadata_has_no_credentials(self):
        def check_saved():
            saved = json.loads(self.path.read_text())
            self.assertEqual(saved["provider"], "huawei-agentsphere")
            self.assertEqual(saved["records"][self.body["sandbox"]]["cloud_id"], "private-cloud-id")
            self.assertNotIn("private-envd-token", self.path.read_text())
        self.connection.before_connect = check_saved
        result = self.adapter.create(self.body)
        self.assertEqual(result["provider"], "huawei-agentsphere")
        self.assertEqual(result["sdk_version"], "2.42.0")
        self.assertEqual(result["sdk_pinned_version"], "2.42.0")
        self.assertEqual(self.connection.payload["metadata"], {"aries_adapter_handle": self.body["sandbox"]})
        self.assertTrue(self.connection.payload["allow_internet_access"])
        self.assertTrue(self.connection.payload["secure"])
        self.assertFalse(self.connection.payload["autoPause"])
        self.assertEqual(self.connection.payload["autoResume"], {"enabled": False})
        self.assertNotIn("network", self.connection.payload)
        for action in ("health", "status"):
            self.assertEqual(self.adapter.dispatch(action, self.body)["provider"], "huawei-agentsphere")
        self.assertNotIn("private-", json.dumps(result))
        self.assertTrue(self.adapter.kill(self.body)["absence_confirmed"])

    def test_construction_failure_retains_identity_and_recovery_deletes_it(self):
        self.connection.fail_connect = True
        with self.assertRaisesRegex(Failure, "sandbox_connection_failed") as failure:
            self.adapter.create(self.body)
        self.assertEqual(failure.exception.status, 502)
        saved = json.loads(self.path.read_text())
        self.assertEqual(saved["records"][self.body["sandbox"]]["state"], "stopping")
        self.assertEqual(saved["records"][self.body["sandbox"]]["cloud_id"], "private-cloud-id")
        with self.assertRaisesRegex(Failure, "cleanup_required_before_create"):
            self.adapter.create({**self.body, "sandbox": "second_attempt"})
        self.assertEqual(self.adapter.creates, 1)
        self.adapter.state_lock.close()
        recovered = self.make_adapter()
        self.assertEqual(recovered.records[self.body["sandbox"]]["state"], "absent")
        self.assertFalse(self.connection.present)

    def test_missing_token_after_response_still_retains_cloud_identity(self):
        del self.connection.created["envdAccessToken"]
        with self.assertRaisesRegex(Failure, "sandbox_connection_failed"):
            self.adapter.create(self.body)
        self.assertEqual(json.loads(self.path.read_text())["records"][self.body["sandbox"]]["cloud_id"], "private-cloud-id")
        self.assertTrue(self.adapter.kill(self.body)["absence_confirmed"])

    def test_http_failures_mark_the_measured_operation_failed_and_keep_known_identity(self):
        for phase, operation in (("create", "controlplane.create"), ("readback", "controlplane.get_info"),
                                 ("metrics", "gateway.metrics")):
            with self.subTest(phase=phase):
                body = {**self.body, "sandbox": "failed_" + phase + "_attempt"}
                setattr(self.connection, phase + "_status", 500)
                self.adapter.measurements.calls = []
                if phase == "metrics":
                    self.adapter.create(body)
                    invoke = lambda: self.adapter.dispatch("metrics", body)
                else:
                    invoke = lambda: self.adapter.create(body)
                with self.assertRaises(Failure) as failure:
                    invoke()
                self.assertEqual(failure.exception.status, 502)
                self.assertEqual(self.adapter.measurements.calls[-1]["operation"], operation)
                self.assertEqual(self.adapter.measurements.calls[-1]["status"], "failed")
                self.assertEqual(json.loads(self.path.read_text())["records"][body["sandbox"]]["cloud_id"],
                                 "private-cloud-id")
                self.assertTrue(self.adapter.kill(body)["absence_confirmed"])
                setattr(self.connection, phase + "_status", 201 if phase == "create" else 200)

    def test_unknown_create_no_retry_and_delete_requires_get_404(self):
        self.connection.fail_create = True
        with self.assertRaisesRegex(Failure, "allocation_unconfirmed"):
            self.adapter.create(self.body)
        self.assertEqual(len(self.connection.calls), 1)
        with self.assertRaisesRegex(Failure, "cleanup_required_before_create"):
            self.adapter.create({**self.body, "sandbox": "another_attempt"})
        self.assertEqual(len(self.connection.calls), 1)
        self.assertEqual(self.adapter.close(), [self.body["sandbox"]])

    def test_failed_delete_stays_owned_until_not_found(self):
        self.adapter.create(self.body)
        self.connection.fail_kill = True
        with self.assertRaisesRegex(Failure, "absence_unconfirmed"):
            self.adapter.kill(self.body)
        self.assertEqual(self.adapter.records[self.body["sandbox"]]["state"], "stopping")
        self.connection.fail_kill = False
        self.assertTrue(self.adapter.kill(self.body)["absence_confirmed"])

    def test_provider_mismatch_prevents_cross_cloud_recovery(self):
        self.adapter.state_lock.close()
        self.path.write_text(json.dumps({"schema": 1, "creates": 1, "records": {
            "old_attempt": {"state": "active", "cloud_id": "old-e2b-id"}}}))
        before = self.path.read_bytes()
        with self.assertRaisesRegex(Failure, "ledger_provider_mismatch"):
            self.make_adapter()
        self.assertEqual(self.connection.calls, [])
        self.assertEqual(self.path.read_bytes(), before)
        saved = json.loads(before)
        saved["provider"] = "huawei-agentsphere"
        self.path.write_text(json.dumps(saved))
        with self.assertRaisesRegex(Failure, "ledger_provider_mismatch"):
            Adapter(self.connection, Missing, Missing, ProcessMissing, templates=["template:build"], state_file=self.path)
        self.assertEqual(self.connection.calls, [])

    def test_gateway_and_metrics_preserve_auth_source_raw_fields_and_unknown_interval(self):
        self.adapter.create(self.body)
        endpoint = self.adapter.dispatch("guest", {**self.body, "port": 18789})
        self.assertEqual(endpoint["url"], self.connection.gateway_url)
        self.assertEqual(set(endpoint["headers"]), {"X-API-Key", "X-Access-Token", "e2b-sandbox-id",
                                                   "e2b-sandbox-port", "e2b-traffic-access-token"})
        self.assertEqual(endpoint["headers"]["e2b-sandbox-port"], "18789")
        self.adapter.measurements.calls = []
        result = self.adapter.dispatch("metrics", self.body)
        self.assertEqual(result["source"], "agentsphere_gateway_envd_metrics")
        self.assertIsNone(result["nominal_interval_seconds"])
        self.assertEqual(result["metrics"], [self.connection.metric])
        self.assertEqual(result["units"], UNITS)
        self.assertEqual(self.adapter.measurements.calls[0]["operation"], "gateway.metrics")
        self.assertEqual(self.connection.calls[-1][2]["gateway_headers"]["e2b-sandbox-port"], "49983")
        self.assertNotIn("private-", json.dumps(result))
        self.assertNotIn("private-envd-token", self.path.read_text())
        self.adapter.kill(self.body)

    def test_metrics_timestamp_only_bool_and_nonfinite_values_are_not_success(self):
        self.adapter.create(self.body)
        for value in ({"ts": 123}, {"cpu_count": True}, {"mem_used": float("nan")},
                      {"cpu_count": 2, "ts": float("inf")}):
            self.connection.metric = value
            with self.assertRaisesRegex(Failure, "provider_metrics_invalid"):
                self.adapter.dispatch("metrics", self.body)
        self.adapter.kill(self.body)


class ConnectionTests(unittest.TestCase):
    def test_http_transport_does_not_follow_redirects_and_bounds_private_body(self):
        class Response:
            status_code = 302
            headers = {"Location": "https://unrelated.example"}
            body = b'{"private":"not-for-output"}'
            def __enter__(self): return self
            def __exit__(self, *_): pass
            def iter_raw(self): yield self.body
        class Client:
            def stream(self, method, url, **kwargs):
                self.observed = method, url, kwargs
                return response
        response, client = Response(), Client()
        connection = Connection(None, client, api_url="https://api.example", api_key="dummy-api-key",
                                gateway_url="https://gateway.example")
        status, value = connection.request("POST", "/sandboxes", payload={"templateID": "fixed:build"}, timeout=7)
        self.assertEqual(status, 302)
        self.assertEqual(value, {"private": "not-for-output"})
        method, url, options = client.observed
        self.assertEqual((method, url), ("POST", "https://api.example/sandboxes"))
        self.assertFalse(options["follow_redirects"])
        self.assertEqual(options["headers"]["X-API-Key"], "dummy-api-key")
        response.body = b"private-invalid-json"
        with self.assertRaisesRegex(Failure, "provider_response_invalid_json") as failure:
            connection.request("GET", "/sandboxes/id")
        self.assertNotIn("private", str(failure.exception))
        response.body = b"x" * (MAX_JSON + 1)
        with self.assertRaisesRegex(Failure, "provider_response_too_large"):
            connection.request("GET", "/sandboxes/id")

    def test_recovery_sdk_calls_use_explicit_huawei_endpoint_and_credential(self):
        calls = []
        class SDK:
            @staticmethod
            def kill(identity, **kwargs): calls.append((identity, kwargs))
            @staticmethod
            def get_info(identity, **kwargs): calls.append((identity, kwargs)); raise Missing()
        connection = Connection(SDK, None, api_url="https://api.example", api_key="test-only-key",
                                gateway_url="https://gateway.example")
        connection.kill("sandbox", request_timeout=7)
        with self.assertRaises(Missing):
            connection.get_info("sandbox", request_timeout=7)
        self.assertEqual(len(calls), 2)
        for identity, options in calls:
            self.assertEqual(identity, "sandbox")
            self.assertEqual(options, {"api_url": "https://api.example", "api_key": "test-only-key",
                                      "validate_api_key": False, "debug": False, "request_timeout": 7})


if __name__ == "__main__":
    unittest.main()
