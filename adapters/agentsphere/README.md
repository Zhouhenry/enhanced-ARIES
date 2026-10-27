# Huawei AgentSphere Adapter

This transport implements the existing [Adapter protocol](../e2b/README.md) using
Huawei endpoints and an isolated E2B Python SDK **2.42.0** plus the supplied
**agentsphere-e2b-patch 0.1.0**. The original E2B Adapter keeps SDK 2.58.0.
No runtime driver, alternate Agent loop, or verifier bypass is introduced.
The shared file protocol accepts up to 64 MiB per upload/download and a 96 MiB
HTTP JSON body, including base64 expansion. Requested download bounds remain
enforced while reading the SDK stream.

## Current acceptance boundary

Huawei B real recording/replay and A real recording have passed independent
verification under the explicit, unverified verifier-egress assumption below.
The separate `merge-ranges-portable-v1` A experiment also passed one E2B recording
and strict recorded-timing replays of the unchanged tape on E2B and Huawei:
eight model turns, seven real tools and 12 independent tests per run, with
confirmed Stop/revocation and cleanup; the combined review passed 16/16 checks.
The legacy tape's filesystem-metadata divergence remains failed. These are
single attempts, not a capacity or provider ranking. See the
[same-tape results and provenance](../../docs/performance-archive.md#portable-crosscloud).
Native A tool details were independently checked from telemetry; the current
Trace exporter does not yet emit those native tool spans.

Earlier ECS probes observed binary file transfer, background stdin/EOF, process
listing/deletion, cgroup support, authenticated HTTP/WebSocket routing, and envd
JSON metrics. ECS candidate-4 has passed 13 Huawei tests, including the installed
SDK contract and built-image checks; the original E2B suite passed 24 tests.
It fixes the wrapper's SDK working directory to `/`: this image has no SDK
default `/home/user`. The added regression fails against the previous code and
passes with the fix while preserving the child task's requested working directory.
An isolated, network-disabled container also verified authenticated HTTP handling
and pre-allocation network-policy rejection. The candidate image then passed all
eight real-cloud protocol checks in `agentsphere-adapter-live-4`: one sandbox,
binary files and stdin/EOF, command results, managed-process removal, Gateway
HTTP/WebSocket marker traffic, a metrics snapshot, and independently confirmed
deletion. The sandbox and template were removed, the Adapter container was absent,
and temporary credentials were deleted. Managed-PID absence does not establish
descendant isolation. The first real OpenClaw run (`agentsphere-embedded-record-1`)
failed at Gateway startup (exit 78, three core-library manifest diagnostics) before
any model request. Its sandbox/template were removed; no-egress verification was
separately rejected. That first run did not establish OpenClaw/verifier acceptance;
the later accepted runs above do not rewrite this failure.
See [candidate and live artifacts](../../deploy/agentsphere/README.md).

The manifest failure is now traced to native plugin discovery: Node's numeric
inode values for `/app` and `/app/extensions` collide in the Huawei guest, while
their bigint values differ. OpenClaw mistakes this for a source mount and scans
three core libraries as plugins. A network-disabled ECS Gateway reproduction and
the real guest discovery probe both confirm the native fix:
`OPENCLAW_DISABLE_BUNDLED_SOURCE_OVERLAYS=1`. The remaining 69 compiled plugin
candidates exactly match the stock image's set. The shared Gateway launcher now
sets this flag for both placements. ECS candidate-5 passed all release checks and
archived its Runner image. The second formal task passed plugin startup but failed
because the Gateway connection did not grant `operator.write`. Its log reported
untrusted proxy headers. ECS `gateway-device-auth-probe-2` has since verified
native device authentication in the same image with networking disabled: exact
device pairing restores write scope, while bad nonces and signatures are rejected.
The Go client subsequently passed a separate native Gateway pairing/reconnection
probe, and the release passed its build, test, race, lint and integration checks.
Those protocol checks alone are not real Agent-task acceptance. The second formal
task recorded no model turns; its sandbox/template, containers and temporary
credentials were removed. Its failure remains preserved alongside the later
successful Agent runs summarized above.

The tested public template did **not** enforce `allow_internet_access=false`.
The separate explicit `network.denyOut` experiment stopped at a positive-control
allocation failure, so that policy remains untested. By default, this Adapter rejects false
or omitted network permission with HTTP 422 `network_isolation_unsupported`
before any allocation, creation-counter increment, or ownership-record write.
An authorized experiment may explicitly enable
`HUAWEI_AGENTSPHERE_ASSUME_NETWORK_ISOLATION=true` and the matching profile field
`sandbox.assume_network_isolation=true`; both default to false. The environment
accepts only lowercase `true` or `false`. Authenticated health must agree with the
profile before allocation, and this option is rejected for E2B Cloud.
The original false network permission is still sent to the provider. Responses
mark `verifier_egress_isolation=assumed-unverified`; the persisted profile and
run infrastructure label preserve the assumption. Stop, revocation, independent
hidden tests and cleanup remain mandatory. Scores and throughput under this
assumption are separate from an isolation-verified baseline. A private VPC alone
is not proof of outbound isolation.

The dedicated VPC/subnet/zero-egress security group have been prepared. The first
private template build ended in `error` before sandbox creation; see the
[network experiment evidence](../../deploy/agentsphere/README.md#专用网络的首次模板实验).
This does not prove outbound isolation; default rejection remains unless the
operator explicitly enables the assumption above.

The API reports envd 0.6.1, the observed serving binary reports 0.8.0, and the
archived image contains 0.10.0. The operator confirmed platform injection. Keep
these observations distinct; the SDK receives the unmodified API version.
Version reconciliation is a known limitation, not a new acceptance gate.

## Configuration and ownership

The entrypoint requires `--template TEMPLATE_ID:BUILD_ID` (repeatable) or
`HUAWEI_SANDBOX_TEMPLATE_ID`, plus `--state-file`. Use an immutable, independently prepared template/build.
The Adapter does not create, rebuild, or delete templates.

Provide only these service settings through private deployment configuration:

- `HUAWEI_AGENTSPHERE_API_URL`: the control-plane HTTPS origin.
- `HUAWEI_AGENTSPHERE_API_KEY`: the Huawei API credential.
- `HUAWEI_AGENTSPHERE_GATEWAY_URL`: the authenticated Gateway HTTPS origin.
- `ARIES_HUAWEI_ADAPTER_TOKEN`: a separate bearer token of at least 32 characters.

The default listener is `127.0.0.1:8788`; the container entrypoint listens on
`0.0.0.0:8788` for the separately authenticated internal Adapter connection.

Use a dedicated service and persistent volume, never the E2B Adapter's state
volume. Huawei ledgers carry `provider=huawei-agentsphere`; E2B's existing ledgers
retain their format. A provider mismatch fails before recovery touches resources.
Health/create/status report the provider and actual/pinned SDK versions.

Create uses the observed control-plane payload, then immediately persists any
returned cloud ID before validating lifecycle readback or constructing SDK
clients. A later failure retains deletion ownership. A request with an ambiguous
allocation outcome is never retried and blocks new allocation until reconciled.
Kill/recovery use the explicitly configured Huawei API endpoint; deletion is
confirmed only by subsequent control-plane NotFound. Limits on concurrent
sandboxes, processes and TTL still apply; the cumulative counter is accounting.

Guest endpoints use the configured Gateway with `X-API-Key`, `X-Access-Token`,
`e2b-sandbox-id`, `e2b-sandbox-port` and `e2b-traffic-access-token`. These values
are transport credentials, never result metadata or ledger fields. Do not log
GuestEndpoint responses or SDK objects. Redirects are not followed by the raw
control-plane/Gateway client.

Creation retains the probed `secure=true` payload. Live-4 received HTTP 200 with
the exact marker using complete credentials. Retaining sandbox/port routing while
removing authentication returned 401; removing only `X-Access-Token` also returned
401. The authenticated WebSocket returned 101 and the expected real frame. The
request with no headers returned 400 because routing was absent; it is not an
authentication rejection proof. These checks cover the tested route, not every
missing-header combination. The E2B `network.allow_public_traffic=false` setting
is not assumed to have a verified Huawei equivalent.

## Metrics

Metrics come from authenticated Gateway `/metrics` on port 49983, with source
`agentsphere_gateway_envd_metrics`, a raw JSON snapshot and an unknown (`null`)
nominal interval. This is not the upstream SDK's control-plane `get_metrics` API.
The `gateway.metrics` timing includes the local HTTP request and response
validation; it is not guest CPU time. `controlplane.create` additionally includes
ownership-ID persistence, and `controlplane.get_info` includes lifecycle readback
validation. These are operation intervals within the encompassing Adapter RPC,
not pure HTTP RTTs; do not sum nested intervals. HTTP or validation failure marks
the corresponding measured operation failed.

The supplied AgentSphere API examples, section B07, show the actual field names.
`cpu_count`, `cpu_used_pct`, `mem_total_mib` and `mem_used_mib` carry units directly
in their names; the Adapter labels only those as count, percent and MiB. Other
fields, including `ts` and unsuffixed memory/disk fields, remain unchanged without
an asserted unit. A timestamp-only response, boolean gauge, or nonfinite known
numeric field is a measurement failure. Unknown fields are retained. There is no
five-second sampling guarantee or Agent/Tool resource split.

## ECS build and validation

All builds and tests run on the designated ECS, never on the development laptop.
Stage a private allowlist build context containing:

```text
Dockerfile                 <- adapters/agentsphere/Dockerfile
requirements.txt           <- adapters/agentsphere/requirements.txt
e2b-2.42.0-py3-none-any.whl
agentsphere_e2b_patch-0.1.0-py3-none-any.whl
e2b/server.py              <- adapters/e2b/server.py
e2b/guest_exec.py          <- adapters/e2b/guest_exec.py
agentsphere/agentsphere.py <- adapters/agentsphere/agentsphere.py
```

The wheel archives are the already downloaded SDK and user-supplied patch in
the private review archive, not a newly downloaded lookalike package. Their
SHA256 values are checked in the Dockerfile. Archive the resolved dependency
inventory, source hashes, image ID and image export on ECS before deployment;
transitive dependencies are resolved at build time and become part of that
archived image. The build must not receive API credentials or `.env` files.

Run `python -m unittest discover -s adapters/agentsphere -p 'test_*.py' -v` in
the isolated Huawei environment. The installed-SDK contract test only constructs
local clients and inspects headers/signatures; it does not call an API. Run the
original `adapters/e2b` suite separately in its pinned 2.58 environment, including
its SDK-contract test. The inherited guest wrapper and original E2B behavior
must remain covered. Neither unit suite proves provider-side network isolation.
