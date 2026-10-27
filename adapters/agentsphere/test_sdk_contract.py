"""Inspect and construct the pinned SDK locally on ECS; never call an API."""
import importlib.util
import inspect
from importlib.metadata import version
import unittest

from agentsphere import Connection


@unittest.skipUnless(importlib.util.find_spec("e2b") and importlib.util.find_spec("agentsphere_e2b_patch"),
                     "isolated AgentSphere SDK not installed")
class SDKContract(unittest.TestCase):
    def test_process_missing_uses_pinned_sdk_exception_type(self):
        from connectrpc.code import Code
        from connectrpc.errors import ConnectError
        from e2b.envd.rpc import handle_rpc_exception
        from e2b.exceptions import NotFoundException
        error = handle_rpc_exception(ConnectError(code=Code.NOT_FOUND, message="process not found"))
        self.assertIs(type(error), NotFoundException)

    def test_real_sdk_transport_constructs_with_reported_version_and_gateway_headers(self):
        self.assertEqual(version("e2b"), "2.42.0")
        self.assertEqual(version("agentsphere-e2b-patch"), "0.1.0")
        import agentsphere_e2b_patch
        from e2b import Sandbox
        from e2b.sandbox_sync.commands.command import Commands
        from e2b.sandbox_sync.commands.command_handle import CommandHandle
        from e2b.sandbox_sync.filesystem.filesystem import Filesystem
        for method, names in [(Commands.run, ("cmd", "background", "stdin", "user", "request_timeout")),
                              (CommandHandle.send_stdin, ("data", "request_timeout")),
                              (CommandHandle.close_stdin, ("request_timeout",)),
                              (Filesystem.read, ("path", "format", "user", "request_timeout"))]:
            self.assertTrue(set(names) <= set(inspect.signature(method).parameters))
        connection = Connection(Sandbox, None, api_url="https://api.example", api_key="dummy-api-key",
                                gateway_url="https://gateway.example")
        sandbox = connection.connect({"sandboxID": "dummy-sandbox", "envdVersion": "0.6.1",
            "envdAccessToken": "dummy-envd-token", "trafficAccessToken": "dummy-traffic-token"}, 10)
        self.assertFalse(sandbox.connection_config.debug)
        headers = {k.lower(): v for k, v in sandbox.connection_config.sandbox_headers.items()}
        self.assertEqual(headers["x-api-key"], "dummy-api-key")
        self.assertEqual(headers["x-access-token"], "dummy-envd-token")
        self.assertEqual(headers["e2b-sandbox-id"], "dummy-sandbox")
        self.assertEqual(headers["e2b-sandbox-port"], "49983")
        self.assertEqual(headers["e2b-traffic-access-token"], "dummy-traffic-token")
        self.assertEqual(connection.guest_headers(sandbox, 18789)["e2b-sandbox-port"], "18789")


if __name__ == "__main__":
    unittest.main()
