"""Offline checks against the installed SDK, with zero API calls."""
import importlib.util
import inspect
import unittest


@unittest.skipUnless(importlib.util.find_spec("e2b"), "pinned SDK not installed")
class SDKContract(unittest.TestCase):
    def test_public_http_version_connection_option(self):
        from e2b.connection_config import ConnectionConfig
        self.assertEqual(ConnectionConfig(http_version="1.1").http_version, "1.1")

    def test_process_missing_uses_pinned_sdk_exception_type(self):
        from connectrpc.code import Code
        from connectrpc.errors import ConnectError
        from e2b.envd.rpc import handle_rpc_exception
        from e2b.exceptions import NotFoundException
        error = handle_rpc_exception(ConnectError(code=Code.NOT_FOUND, message="process not found"))
        self.assertIs(type(error), NotFoundException)

    def test_pinned_public_methods(self):
        from importlib.metadata import version
        from e2b import Sandbox
        from e2b.sandbox_sync.commands.command import Commands
        from e2b.sandbox_sync.commands.command_handle import CommandHandle
        from e2b.sandbox_sync.filesystem.filesystem import Filesystem
        from e2b.sandbox.filesystem.filesystem import FileStreamReader
        self.assertEqual(version("e2b"), "2.58.0")
        for method, names in [(Sandbox.create, ["template", "timeout", "network", "lifecycle", "allow_internet_access"]),
                              (Commands.run, ["cmd", "background", "stdin", "user", "timeout", "request_timeout"]),
                              (CommandHandle.send_stdin, ["data", "request_timeout"]),
                              (CommandHandle.close_stdin, ["request_timeout"]),
                              (Filesystem.read, ["path", "format", "user", "request_timeout"]),
                              (Filesystem.write, ["path", "data", "user", "request_timeout"])]:
            for name in names: self.assertIn(name, inspect.signature(method).parameters)
        for name in ["kill", "get_info", "get_metrics", "get_host", "traffic_access_token"]:
            self.assertTrue(hasattr(Sandbox, name), name)
        self.assertTrue(hasattr(FileStreamReader, "__enter__"))
        self.assertTrue(hasattr(FileStreamReader, "__iter__"))


if __name__ == "__main__": unittest.main()
