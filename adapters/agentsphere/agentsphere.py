"""Huawei AgentSphere transport for the existing, single-owner Adapter protocol.

Network-disabled allocations are rejected unless the explicitly configured
assumption mode is enabled. This mode does not verify provider egress isolation.
"""
import argparse
import datetime
import json
import math
import os
from pathlib import Path
import sys
from urllib.parse import quote, urlsplit

sys.path.insert(1, str(Path(__file__).resolve().parents[1] / "e2b"))
from server import Adapter, Failure, serve, string

SDK_VERSION = "2.42.0"
PATCH_VERSION = "0.1.0"
MAX_JSON = 2 << 20
# These units are explicit in the field names of the supplied B07 example.
# Other fields stay raw; numeric magnitude is not a documented unit contract.
UNITS = {"cpu_count": "count", "cpu_used_pct": "percent",
         "mem_total_mib": "MiB", "mem_used_mib": "MiB"}
GAUGES = frozenset(UNITS) | {"mem_total", "mem_used", "mem_cache", "disk_used", "disk_total"}


def parse_network_isolation_assumption(value):
    if value not in ("true", "false"):
        raise ValueError("HUAWEI_AGENTSPHERE_ASSUME_NETWORK_ISOLATION must be true or false")
    return value == "true"


def https_origin(value):
    value = string(value).rstrip("/")
    parsed = urlsplit(value)
    if (parsed.scheme != "https" or not parsed.hostname or parsed.port not in (None, 443)
            or parsed.username or parsed.password or parsed.path or parsed.query or parsed.fragment):
        raise ValueError("AgentSphere endpoints must be HTTPS origins")
    return value


class Connection:
    """Provider endpoints are explicit, including during ledger recovery."""

    def __init__(self, sdk, client, *, api_url, api_key, gateway_url):
        self.sdk, self.client = sdk, client
        self.api_url, self.gateway_url = https_origin(api_url), https_origin(gateway_url)
        self.api_key = string(api_key)
        if any(c in self.api_key for c in "\r\n"):
            raise ValueError("invalid AgentSphere credential")

    def request(self, method, path, *, payload=None, timeout=30, gateway_headers=None):
        gateway = gateway_headers is not None
        url = (self.gateway_url if gateway else self.api_url) + path
        headers = dict(gateway_headers) if gateway else {"X-API-Key": self.api_key}
        headers["Accept-Encoding"] = "identity"
        with self.client.stream(method, url, json=payload, headers=headers, timeout=timeout,
                                follow_redirects=False) as response:
            body = bytearray()
            for chunk in response.iter_raw():
                if len(body) + len(chunk) > MAX_JSON:
                    raise Failure("provider_response_too_large", 502)
                body.extend(chunk)
            if response.headers.get("content-encoding", "").lower().strip() not in ("", "identity"):
                raise Failure("provider_response_encoding_unsupported", 502)
            if not body:
                return response.status_code, None
            try:
                value = json.loads(body)
            except (ValueError, UnicodeError):
                raise Failure("provider_response_invalid_json", 502) from None
            return response.status_code, value

    def connect(self, created, request_timeout):
        # Preserve the API-reported version; do not replace it with the image's
        # envd version or the separately observed serving-binary version.
        from e2b.connection_config import ConnectionConfig
        from packaging.version import Version
        config = ConnectionConfig(api_key=self.api_key, api_url=self.api_url, sandbox_url=self.gateway_url, debug=False,
            validate_api_key=False, request_timeout=request_timeout,
            extra_sandbox_headers={"X-API-Key": self.api_key, "X-Access-Token": created["envdAccessToken"],
                "E2b-Sandbox-Id": created["sandboxID"], "E2b-Sandbox-Port": "49983"})
        return self.sdk(sandbox_id=created["sandboxID"], envd_version=Version(created["envdVersion"]),
            sandbox_domain=created.get("domain"), envd_access_token=created["envdAccessToken"],
            traffic_access_token=created["trafficAccessToken"], connection_config=config)

    def kill(self, sandbox_id, *, request_timeout):
        return self.sdk.kill(sandbox_id, api_key=self.api_key, api_url=self.api_url,
                             validate_api_key=False, debug=False, request_timeout=request_timeout)

    def get_info(self, sandbox_id, *, request_timeout):
        return self.sdk.get_info(sandbox_id, api_key=self.api_key, api_url=self.api_url,
                                 validate_api_key=False, debug=False, request_timeout=request_timeout)

    def guest_headers(self, sandbox, port):
        # Read the public config header view; do not depend on SDK private token
        # fields. The pinned patch supplies the traffic header at construction.
        headers = {k.lower(): v for k, v in sandbox.connection_config.sandbox_headers.items()}
        access, traffic = headers.get("x-access-token"), sandbox.traffic_access_token
        if not access or not traffic:
            raise Failure("private_ingress_token_missing", 502)
        return {"X-API-Key": self.api_key, "X-Access-Token": access,
                "e2b-sandbox-id": sandbox.sandbox_id, "e2b-sandbox-port": str(port),
                "e2b-traffic-access-token": traffic}


class AgentSphereAdapter(Adapter):
    provider = "huawei-agentsphere"
    sdk_pinned_version = SDK_VERSION

    def __init__(self, *args, assume_network_isolation=False, **kwargs):
        if type(assume_network_isolation) is not bool:
            raise ValueError("assume_network_isolation must be a boolean")
        self.assume_network_isolation = assume_network_isolation
        super().__init__(*args, **kwargs)

    def validate_network(self, internet):
        if not internet and not self.assume_network_isolation:
            raise Failure("network_isolation_unsupported", 422)

    def runtime_metadata(self):
        return {**super().runtime_metadata(), "sdk_patch_version": PATCH_VERSION,
                "assume_network_isolation": self.assume_network_isolation,
                "verifier_egress_isolation": "assumed-unverified" if self.assume_network_isolation else "unsupported"}

    def create_sandbox(self, handle, template, timeout, internet):
        payload = {"templateID": template, "timeout": timeout, "autoPause": False,
                   "autoResume": {"enabled": False}, "secure": True, "allow_internet_access": internet,
                   "metadata": {"aries_adapter_handle": handle}, "envVars": {}}
        with self.sdk_measurement("controlplane.create"):
            status, created = self.sdk.request("POST", "/sandboxes", payload=payload, timeout=self.request_timeout)
            # Persist any returned identity before validating remaining fields or
            # constructing the SDK clients. Later failures remain safely deletable.
            if isinstance(created, dict) and isinstance(created.get("sandboxID"), str) and created["sandboxID"]:
                self.remember_cloud_id(handle, created["sandboxID"])
            if status != 201 or not isinstance(created, dict) or not created.get("sandboxID"):
                raise Failure("provider_create_failed", 502)
            for name in ("envdAccessToken", "trafficAccessToken", "envdVersion"):
                string(created.get(name))
        with self.sdk_measurement("controlplane.get_info"):
            status, detail = self.sdk.request("GET", "/sandboxes/" + quote(created["sandboxID"], safe=""),
                                              timeout=self.request_timeout)
            template_id, separator, build_id = template.partition(":")
            if (status != 200 or not isinstance(detail, dict) or detail.get("templateID") != template_id
                    or (separator and detail.get("buildID") != build_id) or detail.get("timeout") != timeout
                    or detail.get("lifecycle", {}).get("onTimeout") != "kill"
                    or detail.get("lifecycle", {}).get("autoResume") is not False):
                raise Failure("provider_lifecycle_unconfirmed", 502)
        return self.sdk.connect(created, self.request_timeout)

    def guest_endpoint(self, sandbox, port):
        return {"url": self.sdk.gateway_url, "headers": self.sdk.guest_headers(sandbox, port)}

    def metrics_snapshot(self, sandbox):
        observed_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
        with self.sdk_measurement("gateway.metrics"):
            status, value = self.sdk.request("GET", "/metrics", timeout=self.request_timeout,
                                             gateway_headers=self.sdk.guest_headers(sandbox, 49983))
            if (status != 200 or not isinstance(value, dict)
                    or not any(k in value for k in GAUGES)
                    or any(type(value[k]) not in (int, float) or not math.isfinite(value[k])
                           for k in GAUGES | {"ts"} if k in value)):
                raise Failure("provider_metrics_invalid", 502)
        return {"source": "agentsphere_gateway_envd_metrics", "nominal_interval_seconds": None,
                "observed_at": observed_at, "metrics": [value],
                "units": {k: unit for k, unit in UNITS.items() if k in value},
                "unsupported": ["nominal_sampling_interval", "disk_io", "network_io", "agent_tool_resource_split"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--listen", default="127.0.0.1:8788")
    parser.add_argument("--template", action="append")
    parser.add_argument("--state-file", required=True)
    parser.add_argument("--max-sandboxes", type=int, default=2)
    parser.add_argument("--max-processes", type=int, default=8)
    parser.add_argument("--max-timeout", type=int, default=900)
    args = parser.parse_args()
    try:
        assume_network_isolation = parse_network_isolation_assumption(
            os.environ.get("HUAWEI_AGENTSPHERE_ASSUME_NETWORK_ISOLATION", "false"))
    except ValueError as error:
        parser.error(str(error))
    templates = args.template or [os.environ.get("HUAWEI_SANDBOX_TEMPLATE_ID", "")]
    if not all(templates):
        parser.error("--template or HUAWEI_SANDBOX_TEMPLATE_ID is required")
    from importlib.metadata import version
    if version("e2b") != SDK_VERSION or version("agentsphere-e2b-patch") != PATCH_VERSION:
        parser.error("install the pinned AgentSphere requirements")
    import httpx
    import agentsphere_e2b_patch  # Installs the vendor's SDK traffic-token hook.
    from e2b import Sandbox
    from e2b.exceptions import SandboxNotFoundException, FileNotFoundException, NotFoundException
    names = ("HUAWEI_AGENTSPHERE_API_URL", "HUAWEI_AGENTSPHERE_API_KEY", "HUAWEI_AGENTSPHERE_GATEWAY_URL", "ARIES_HUAWEI_ADAPTER_TOKEN")
    if any(not os.environ.get(name) for name in names):
        parser.error("explicit AgentSphere endpoint and credential settings are required")
    token = os.environ[names[-1]]
    if len(token) < 32 or any(c in token for c in "\r\n\0"):
        parser.error("ARIES_HUAWEI_ADAPTER_TOKEN must contain at least 32 valid characters")
    if min(args.max_sandboxes, args.max_processes, args.max_timeout) < 1:
        parser.error("limits and template allowlist must be nonempty and positive")
    with httpx.Client(timeout=30, trust_env=False, follow_redirects=False) as client:
        connection = Connection(Sandbox, client, api_url=os.environ[names[0]], api_key=os.environ[names[1]],
                                gateway_url=os.environ[names[2]])
        adapter = AgentSphereAdapter(connection, SandboxNotFoundException, FileNotFoundException, NotFoundException,
            templates=templates, state_file=args.state_file, max_sandboxes=args.max_sandboxes,
            max_processes=args.max_processes, max_timeout=args.max_timeout, sdk_version=version("e2b"),
            assume_network_isolation=assume_network_isolation)
        host, port = args.listen.rsplit(":", 1)
        serve(adapter, token, (host, int(port)))


if __name__ == "__main__":
    main()
