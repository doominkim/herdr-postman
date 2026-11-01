"""Real PTY + remote CLI + Postman CLI acceptance against a local fixture."""
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import signal
import struct
import subprocess
import tempfile
import termios
import threading
import time
import unittest

BINARY = Path(__file__).resolve().parents[1] / "bin/herdr-postman"


@unittest.skipUnless(BINARY.exists(), "build bin/herdr-postman first")
class RemoteRuntime(unittest.TestCase):
    def test_native_send_through_running_tui(self):
        graphql_calls = []
        http_headers = []
        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                http_headers.append((self.headers.get("X-Layer"), self.headers.get("X-Base")))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"native remote fixture")

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                graphql_calls.append(body)
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b'{"data":{"hello":"fixture"}}')

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with tempfile.TemporaryDirectory(prefix="hpm-pty-", dir="/tmp") as temp:
                root = Path(temp)
                collection = root / "collection"
                collection.mkdir()
                source = ("$kind: http-request\nname: Runtime fixture\nmethod: GET\n"
                          f"url: http://127.0.0.1:{server.server_port}/\n"
                          "headers:\n  - key: X-Layer\n    value: '{{layer}}'\n  - key: X-Base\n    value: '{{base_only}}'\n"
                          "scripts:\n  - type: afterResponse\n    language: text/javascript\n"
                          "    code: pm.test('remote fixture assertion', () => { pm.expect(1).to.eql(2); });\n")
                request = collection / "get.request.yaml"
                request.write_text(source)
                graphql_payload = {"query": "query Hello { hello }", "variables": {"id": "fixture"}, "operationName": "Hello"}
                graphql_source = ("$kind: http-request\nname: GraphQL HTTP fixture\nmethod: POST\n"
                                  f"url: http://127.0.0.1:{server.server_port}/graphql\n"
                                  # Desktop compacts ordinary header rows when saving.
                                  "headers:\n  Accept: application/graphql-response+json\n  Content-Type: application/json\n"
                                  "body:\n  type: json\n  content: " + json.dumps(json.dumps(graphql_payload)) + "\n")
                graphql_file = collection / "graphql.request.yaml"
                graphql_file.write_text(graphql_source)
                folder = collection / "folder-path"
                (folder / ".resources").mkdir(parents=True)
                folder_definition = "$kind: collection\nname: GUI folder name\n"
                (folder / ".resources" / "definition.yaml").write_text(folder_definition)
                nested_request = folder / "get.request.yaml"
                nested_request.write_text(source)
                app_config = root / "config" / "herdr-postman"
                app_config.mkdir(parents=True)
                theme_dir = root / "themes"
                theme_dir.mkdir()
                (theme_dir / "fixture.yaml").write_text("name: runtime_fixture\nprimary: '#abcdef'\nbackground: '#123456'\n")
                (app_config / "config.yaml").write_text(f"theme: runtime_fixture\ntheme_directory: {theme_dir}\nnerd_fonts: false\n")
                env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor",
                           XDG_CONFIG_HOME=str(root / "config"),
                           XDG_DATA_HOME=str(root / "data"), XDG_RUNTIME_DIR=str(root))
                env.pop("NO_COLOR", None)  # This fixture explicitly verifies truecolor rendering.
                env.pop("HERDR_POSTMAN_CONFIG_FILE", None)
                env.pop("HERDR_POSTMAN_INSTANCE", None)
                base_env = root / "base.environment.yaml"
                overlay_env = root / "overlay.environment.yaml"
                base_source = "name: Base\nvalues:\n  - key: layer\n    value: base\n  - key: base_only\n    value: kept\n"
                overlay_source = "name: Overlay\nvalues:\n  - key: layer\n    value: overlay\n"
                base_env.write_text(base_source)
                overlay_env.write_text(overlay_source)
                master, slave = pty.openpty()
                fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 140, 0, 0))
                process = subprocess.Popen([str(BINARY), "--collection", str(collection),
                                            "--environment", str(base_env), "--environment", str(overlay_env)],
                                           stdin=slave, stdout=slave, stderr=slave,
                                           env=env, start_new_session=True)
                os.close(slave)
                screen = bytearray()

                def drain():
                    try:
                        while True:
                            chunk = os.read(master, 65536)
                            if not chunk:
                                return
                            screen.extend(chunk)
                    except OSError:
                        pass

                reader = threading.Thread(target=drain, daemon=True)
                reader.start()

                def remote(*args):
                    result = subprocess.run([str(BINARY), "remote", *args], env=env,
                                            capture_output=True, text=True, timeout=20)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    return json.loads(result.stdout)

                try:
                    deadline = time.monotonic() + 10
                    while not list((root / "herdr-postman").glob("*.sock")):
                        self.assertIsNone(process.poll(), "TUI exited before socket startup")
                        self.assertLess(time.monotonic(), deadline, "socket startup timed out")
                        time.sleep(0.05)
                    instances = remote("list", "--json")
                    self.assertEqual(len(instances), 1)
                    requests = remote("requests", "--json")
                    self.assertTrue(any(item["file"] == "get.request.yaml" for item in requests))
                    shown = remote("open", "get", "--json")
                    self.assertIn("$kind: http-request", shown["yaml"])
                    exchange = remote("send", "get", "--json")
                    self.assertEqual(exchange["outcome"], "done")
                    self.assertEqual(exchange["response"]["body"], "native remote fixture")
                    self.assertEqual(exchange["response"]["postmanRun"]["exitCode"], 1)
                    self.assertIn("remote fixture assertion", exchange["response"]["postmanRun"]["output"])
                    self.assertEqual(request.read_text(), source)
                    self.assertEqual(http_headers, [("overlay", "kept")])
                    self.assertEqual(base_env.read_text(), base_source)
                    self.assertEqual(overlay_env.read_text(), overlay_source)
                    # Styles can split a visible word across several terminal writes.
                    visible_text = re.sub(rb"\x1b\[[0-9;:]*m", b"", bytes(screen))
                    self.assertTrue(b"herdr-postman" in visible_text, "application name was not rendered")
                    self.assertTrue(b"48;2;18;52;86" in screen, "custom theme background was not rendered")
                    gql = next(item for item in requests if item["file"] == "graphql.request.yaml")
                    self.assertEqual(gql["kind"], "graphql")
                    exchange = remote("send", "graphql", "--json")
                    self.assertEqual(exchange["outcome"], "done")
                    self.assertEqual(json.loads(exchange["response"]["body"]), {"data": {"hello": "fixture"}})
                    self.assertEqual(graphql_calls, [graphql_payload])
                    self.assertEqual(graphql_file.read_text(), graphql_source)
                    exchange = remote("send", "folder-path/get", "--json")
                    self.assertEqual(exchange["outcome"], "done")
                    self.assertEqual(exchange["response"]["body"], "native remote fixture")
                    self.assertEqual(http_headers, [("overlay", "kept"), ("overlay", "kept")])
                    self.assertEqual(nested_request.read_text(), source)
                    self.assertEqual((folder / ".resources" / "definition.yaml").read_text(), folder_definition)
                    os.kill(process.pid, signal.SIGINT)
                    process.wait(timeout=10)
                    self.assertFalse(list((root / "herdr-postman").glob("*.sock")),
                                     "TUI exit left a remote socket")
                finally:
                    if process.poll() is None:
                        process.terminate()
                        process.wait(timeout=10)
                    os.close(master)
                    reader.join(timeout=2)
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)


if __name__ == "__main__":
    unittest.main()
